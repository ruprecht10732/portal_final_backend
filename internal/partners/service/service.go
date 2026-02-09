package service

import (
	"context"
	"regexp"
	"strings"
	"time"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/partners/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/sanitize"

	"github.com/google/uuid"
)

const (
	inviteTokenBytes = 32
	inviteTTL        = 72 * time.Hour
)

// Service provides business logic for partners.
type Service struct {
	repo       *repository.Repository
	eventBus   events.Bus
	storage          storage.StorageService
	logoBucket       string
	summaryGenerator OfferSummaryGenerator
}

// New creates a new partners service.
func New(repo *repository.Repository, eventBus events.Bus, storageSvc storage.StorageService, logoBucket string) *Service {
	return &Service{repo: repo, eventBus: eventBus, storage: storageSvc, logoBucket: logoBucket}
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, req transport.CreatePartnerRequest) (transport.PartnerResponse, error) {
	if err := s.ensureServiceTypeIDsValid(ctx, tenantID, req.ServiceTypeIDs); err != nil {
		return transport.PartnerResponse{}, err
	}

	partner := repository.Partner{
		ID:             uuid.New(),
		OrganizationID: tenantID,
		BusinessName:   sanitize.Text(req.BusinessName),
		KVKNumber:      strings.TrimSpace(req.KVKNumber),
		VATNumber:      strings.TrimSpace(req.VATNumber),
		AddressLine1:   sanitize.Text(req.AddressLine1),
		AddressLine2:   normalizeOptional(req.AddressLine2),
		HouseNumber:    normalizeOptional(req.HouseNumber),
		PostalCode:     strings.TrimSpace(req.PostalCode),
		City:           sanitize.Text(req.City),
		Country:        sanitize.Text(req.Country),
		Latitude:       req.Latitude,
		Longitude:      req.Longitude,
		ContactName:    sanitize.Text(req.ContactName),
		ContactEmail:   normalizeEmail(req.ContactEmail),
		ContactPhone:   strings.TrimSpace(req.ContactPhone),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := validatePartnerNumbers(partner.KVKNumber, partner.VATNumber); err != nil {
		return transport.PartnerResponse{}, err
	}

	created, err := s.repo.Create(ctx, partner)
	if err != nil {
		return transport.PartnerResponse{}, err
	}

	if len(req.ServiceTypeIDs) > 0 {
		if err := s.repo.ReplaceServiceTypes(ctx, created.ID, req.ServiceTypeIDs); err != nil {
			return transport.PartnerResponse{}, err
		}
	}

	return mapPartnerResponse(created, req.ServiceTypeIDs), nil
}

func (s *Service) SetOfferSummaryGenerator(generator OfferSummaryGenerator) {
	s.summaryGenerator = generator
}

func (s *Service) GetByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (transport.PartnerResponse, error) {
	partner, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return transport.PartnerResponse{}, err
	}
	serviceTypeIDs, err := s.repo.ListServiceTypeIDs(ctx, tenantID, id)
	if err != nil {
		return transport.PartnerResponse{}, err
	}
	return mapPartnerResponse(partner, serviceTypeIDs), nil
}

func (s *Service) Update(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, req transport.UpdatePartnerRequest) (transport.PartnerResponse, error) {
	if req.ServiceTypeIDs != nil {
		if err := s.ensureServiceTypeIDsValid(ctx, tenantID, *req.ServiceTypeIDs); err != nil {
			return transport.PartnerResponse{}, err
		}
	}

	update := repository.PartnerUpdate{
		ID:             id,
		OrganizationID: tenantID,
		BusinessName:   normalizeOptionalString(req.BusinessName, sanitize.Text),
		KVKNumber:      normalizeOptionalString(req.KVKNumber, strings.TrimSpace),
		VATNumber:      normalizeOptionalString(req.VATNumber, strings.TrimSpace),
		AddressLine1:   normalizeOptionalString(req.AddressLine1, sanitize.Text),
		AddressLine2:   normalizeOptionalString(req.AddressLine2, sanitize.Text),
		HouseNumber:    normalizeOptionalString(req.HouseNumber, strings.TrimSpace),
		PostalCode:     normalizeOptionalString(req.PostalCode, strings.TrimSpace),
		City:           normalizeOptionalString(req.City, sanitize.Text),
		Country:        normalizeOptionalString(req.Country, sanitize.Text),
		Latitude:       req.Latitude,
		Longitude:      req.Longitude,
		ContactName:    normalizeOptionalString(req.ContactName, sanitize.Text),
		ContactEmail:   normalizeOptionalString(req.ContactEmail, normalizeEmail),
		ContactPhone:   normalizeOptionalString(req.ContactPhone, strings.TrimSpace),
	}

	if err := validatePartnerNumbersUpdate(update); err != nil {
		return transport.PartnerResponse{}, err
	}

	updated, err := s.repo.Update(ctx, update)
	if err != nil {
		return transport.PartnerResponse{}, err
	}

	serviceTypeIDs, err := s.resolveServiceTypeIDs(ctx, tenantID, id, req.ServiceTypeIDs)
	if err != nil {
		return transport.PartnerResponse{}, err
	}

	return mapPartnerResponse(updated, serviceTypeIDs), nil
}

func (s *Service) Delete(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) error {
	return s.repo.Delete(ctx, id, tenantID)
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, req transport.ListPartnersRequest) (transport.ListPartnersResponse, error) {
	result, err := s.repo.List(ctx, repository.ListParams{
		OrganizationID: tenantID,
		Search:         req.Search,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
		Page:           req.Page,
		PageSize:       req.PageSize,
	})
	if err != nil {
		return transport.ListPartnersResponse{}, err
	}

	items := make([]transport.PartnerResponse, 0, len(result.Items))
	for _, partner := range result.Items {
		items = append(items, mapPartnerResponse(partner, nil))
	}

	return transport.ListPartnersResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}, nil
}

func (s *Service) PresignLogoUpload(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID, req transport.PartnerLogoPresignRequest) (transport.PartnerLogoPresignResponse, error) {
	if err := s.ensurePartnerExists(ctx, tenantID, partnerID); err != nil {
		return transport.PartnerLogoPresignResponse{}, err
	}
	if !storage.IsImageContentType(req.ContentType) {
		return transport.PartnerLogoPresignResponse{}, apperr.Validation("logo must be an image")
	}

	presigned, err := s.storage.GenerateUploadURL(
		ctx,
		s.logoBucket,
		logoFolder(tenantID, partnerID),
		req.FileName,
		req.ContentType,
		req.SizeBytes,
	)
	if err != nil {
		return transport.PartnerLogoPresignResponse{}, err
	}

	return transport.PartnerLogoPresignResponse{
		UploadURL: presigned.URL,
		FileKey:   presigned.FileKey,
		ExpiresAt: presigned.ExpiresAt.Unix(),
	}, nil
}

func (s *Service) SetLogo(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID, req transport.SetPartnerLogoRequest) (transport.PartnerResponse, error) {
	partner, err := s.repo.GetByID(ctx, partnerID, tenantID)
	if err != nil {
		return transport.PartnerResponse{}, err
	}
	if !storage.IsImageContentType(req.ContentType) {
		return transport.PartnerResponse{}, apperr.Validation("logo must be an image")
	}
	if err := s.storage.ValidateContentType(req.ContentType); err != nil {
		return transport.PartnerResponse{}, err
	}
	if err := s.storage.ValidateFileSize(req.SizeBytes); err != nil {
		return transport.PartnerResponse{}, err
	}
	if !strings.HasPrefix(req.FileKey, logoFolder(tenantID, partnerID)+"/") {
		return transport.PartnerResponse{}, apperr.Validation("invalid logo file key")
	}

	if partner.LogoFileKey != nil && *partner.LogoFileKey != req.FileKey {
		_ = s.storage.DeleteObject(ctx, s.logoBucket, *partner.LogoFileKey)
	}

	updated, err := s.repo.UpdateLogo(ctx, tenantID, partnerID, repository.PartnerLogo{
		FileKey:     req.FileKey,
		FileName:    req.FileName,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
	})
	if err != nil {
		return transport.PartnerResponse{}, err
	}

	serviceTypeIDs, err := s.repo.ListServiceTypeIDs(ctx, tenantID, partnerID)
	if err != nil {
		return transport.PartnerResponse{}, err
	}

	return mapPartnerResponse(updated, serviceTypeIDs), nil
}

func (s *Service) GetLogoDownloadURL(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID) (transport.PartnerLogoDownloadResponse, error) {
	partner, err := s.repo.GetByID(ctx, partnerID, tenantID)
	if err != nil {
		return transport.PartnerLogoDownloadResponse{}, err
	}
	if partner.LogoFileKey == nil || *partner.LogoFileKey == "" {
		return transport.PartnerLogoDownloadResponse{}, apperr.NotFound("logo not found")
	}

	presigned, err := s.storage.GenerateDownloadURL(ctx, s.logoBucket, *partner.LogoFileKey)
	if err != nil {
		return transport.PartnerLogoDownloadResponse{}, err
	}

	return transport.PartnerLogoDownloadResponse{
		DownloadURL: presigned.URL,
		ExpiresAt:   presigned.ExpiresAt.Unix(),
	}, nil
}

func (s *Service) DeleteLogo(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID) (transport.PartnerResponse, error) {
	partner, err := s.repo.GetByID(ctx, partnerID, tenantID)
	if err != nil {
		return transport.PartnerResponse{}, err
	}
	if partner.LogoFileKey != nil && *partner.LogoFileKey != "" {
		_ = s.storage.DeleteObject(ctx, s.logoBucket, *partner.LogoFileKey)
	}

	updated, err := s.repo.ClearLogo(ctx, tenantID, partnerID)
	if err != nil {
		return transport.PartnerResponse{}, err
	}

	serviceTypeIDs, err := s.repo.ListServiceTypeIDs(ctx, tenantID, partnerID)
	if err != nil {
		return transport.PartnerResponse{}, err
	}

	return mapPartnerResponse(updated, serviceTypeIDs), nil
}

func (s *Service) LinkLead(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID, leadID uuid.UUID) error {
	if err := s.ensurePartnerExists(ctx, tenantID, partnerID); err != nil {
		return err
	}
	if err := s.ensureLeadExists(ctx, tenantID, leadID); err != nil {
		return err
	}
	return s.repo.LinkLead(ctx, tenantID, partnerID, leadID)
}

func (s *Service) UnlinkLead(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID, leadID uuid.UUID) error {
	if err := s.ensurePartnerExists(ctx, tenantID, partnerID); err != nil {
		return err
	}
	return s.repo.UnlinkLead(ctx, tenantID, partnerID, leadID)
}

func (s *Service) ListLeads(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID) ([]transport.PartnerLeadResponse, error) {
	if err := s.ensurePartnerExists(ctx, tenantID, partnerID); err != nil {
		return nil, err
	}

	items, err := s.repo.ListLeads(ctx, tenantID, partnerID)
	if err != nil {
		return nil, err
	}

	resp := make([]transport.PartnerLeadResponse, 0, len(items))
	for _, lead := range items {
		resp = append(resp, transport.PartnerLeadResponse{
			ID:        lead.ID,
			FirstName: lead.FirstName,
			LastName:  lead.LastName,
			Phone:     lead.Phone,
			Address:   formatAddress(lead.Street, lead.HouseNumber, lead.City),
		})
	}

	return resp, nil
}

func (s *Service) CreateInvite(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID, createdBy uuid.UUID, req transport.CreatePartnerInviteRequest) (transport.CreatePartnerInviteResponse, error) {
	partner, err := s.repo.GetByID(ctx, partnerID, tenantID)
	if err != nil {
		return transport.CreatePartnerInviteResponse{}, err
	}

	if req.LeadID != nil {
		if err := s.ensureLeadExists(ctx, tenantID, *req.LeadID); err != nil {
			return transport.CreatePartnerInviteResponse{}, err
		}
	}
	if req.LeadServiceID != nil {
		if err := s.ensureLeadServiceExists(ctx, tenantID, *req.LeadServiceID); err != nil {
			return transport.CreatePartnerInviteResponse{}, err
		}
	}

	rawToken, err := token.GenerateRandomToken(inviteTokenBytes)
	if err != nil {
		return transport.CreatePartnerInviteResponse{}, err
	}

	expiresAt := time.Now().Add(inviteTTL)
	invite := repository.PartnerInvite{
		ID:             uuid.New(),
		OrganizationID: tenantID,
		PartnerID:      partnerID,
		Email:          normalizeEmail(req.Email),
		TokenHash:      token.HashSHA256(rawToken),
		ExpiresAt:      expiresAt,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now(),
		LeadID:         req.LeadID,
		LeadServiceID:  req.LeadServiceID,
	}

	if _, err := s.repo.CreateInvite(ctx, invite); err != nil {
		return transport.CreatePartnerInviteResponse{}, err
	}

	if s.eventBus != nil {
		organizationName, _ := s.repo.GetOrganizationName(ctx, tenantID)
		s.eventBus.Publish(ctx, events.PartnerInviteCreated{
			BaseEvent:        events.NewBaseEvent(),
			OrganizationID:   tenantID,
			OrganizationName: organizationName,
			PartnerID:        partnerID,
			PartnerName:      partner.BusinessName,
			Email:            invite.Email,
			InviteToken:      rawToken,
			LeadID:           req.LeadID,
			LeadServiceID:    req.LeadServiceID,
		})
	}

	return transport.CreatePartnerInviteResponse{Token: rawToken, ExpiresAt: expiresAt}, nil
}

func (s *Service) ListInvites(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID) (transport.ListPartnerInvitesResponse, error) {
	if err := s.ensurePartnerExists(ctx, tenantID, partnerID); err != nil {
		return transport.ListPartnerInvitesResponse{}, err
	}

	items, err := s.repo.ListInvites(ctx, tenantID, partnerID)
	if err != nil {
		return transport.ListPartnerInvitesResponse{}, err
	}

	resp := make([]transport.PartnerInviteResponse, 0, len(items))
	for _, invite := range items {
		resp = append(resp, transport.PartnerInviteResponse{
			ID:            invite.ID,
			Email:         invite.Email,
			LeadID:        invite.LeadID,
			LeadServiceID: invite.LeadServiceID,
			ExpiresAt:     invite.ExpiresAt,
			CreatedAt:     invite.CreatedAt,
			UsedAt:        invite.UsedAt,
		})
	}

	return transport.ListPartnerInvitesResponse{Invites: resp}, nil
}

func (s *Service) RevokeInvite(ctx context.Context, tenantID uuid.UUID, inviteID uuid.UUID) (transport.PartnerInviteResponse, error) {
	invite, err := s.repo.RevokeInvite(ctx, tenantID, inviteID)
	if err != nil {
		return transport.PartnerInviteResponse{}, err
	}

	return transport.PartnerInviteResponse{
		ID:            invite.ID,
		Email:         invite.Email,
		LeadID:        invite.LeadID,
		LeadServiceID: invite.LeadServiceID,
		ExpiresAt:     invite.ExpiresAt,
		CreatedAt:     invite.CreatedAt,
		UsedAt:        invite.UsedAt,
	}, nil
}

func (s *Service) ensurePartnerExists(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID) error {
	exists, err := s.repo.Exists(ctx, partnerID, tenantID)
	if err != nil {
		return err
	}
	if !exists {
		return apperr.NotFound("partner not found")
	}
	return nil
}

func (s *Service) ensureServiceTypeIDsValid(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return s.repo.ValidateServiceTypeIDs(ctx, tenantID, ids)
}

func (s *Service) ensureLeadExists(ctx context.Context, tenantID uuid.UUID, leadID uuid.UUID) error {
	exists, err := s.repo.LeadExists(ctx, leadID, tenantID)
	if err != nil {
		return err
	}
	if !exists {
		return apperr.NotFound("lead not found")
	}
	return nil
}

func (s *Service) ensureLeadServiceExists(ctx context.Context, tenantID uuid.UUID, leadServiceID uuid.UUID) error {
	exists, err := s.repo.LeadServiceExists(ctx, leadServiceID, tenantID)
	if err != nil {
		return err
	}
	if !exists {
		return apperr.NotFound("lead service not found")
	}
	return nil
}

func (s *Service) resolveServiceTypeIDs(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID, ids *[]uuid.UUID) ([]uuid.UUID, error) {
	if ids != nil {
		if err := s.repo.ReplaceServiceTypes(ctx, partnerID, *ids); err != nil {
			return nil, err
		}
		return *ids, nil
	}
	return s.repo.ListServiceTypeIDs(ctx, tenantID, partnerID)
}

func mapPartnerResponse(partner repository.Partner, serviceTypeIDs []uuid.UUID) transport.PartnerResponse {
	return transport.PartnerResponse{
		ID:              partner.ID,
		BusinessName:    partner.BusinessName,
		KVKNumber:       partner.KVKNumber,
		VATNumber:       partner.VATNumber,
		AddressLine1:    partner.AddressLine1,
		AddressLine2:    partner.AddressLine2,
		HouseNumber:     partner.HouseNumber,
		PostalCode:      partner.PostalCode,
		City:            partner.City,
		Country:         partner.Country,
		Latitude:        partner.Latitude,
		Longitude:       partner.Longitude,
		ContactName:     partner.ContactName,
		ContactEmail:    partner.ContactEmail,
		ContactPhone:    partner.ContactPhone,
		LogoFileKey:     partner.LogoFileKey,
		LogoFileName:    partner.LogoFileName,
		LogoContentType: partner.LogoContentType,
		LogoSizeBytes:   partner.LogoSizeBytes,
		ServiceTypeIDs:  serviceTypeIDs,
		CreatedAt:       partner.CreatedAt,
		UpdatedAt:       partner.UpdatedAt,
	}
}

func logoFolder(tenantID uuid.UUID, partnerID uuid.UUID) string {
	return "partners/" + tenantID.String() + "/" + partnerID.String()
}

func formatAddress(street string, houseNumber string, city string) string {
	parts := strings.TrimSpace(strings.Join([]string{street, houseNumber}, " "))
	if city == "" {
		return parts
	}
	if parts == "" {
		return city
	}
	return parts + ", " + city
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeOptional(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	clean := sanitize.Text(trimmed)
	if clean == "" {
		return nil
	}
	return &clean
}

func normalizeOptionalString(value *string, normalize func(string) string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	normalized := normalize(trimmed)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func validatePartnerNumbersUpdate(update repository.PartnerUpdate) error {
	if update.KVKNumber == nil && update.VATNumber == nil {
		return nil
	}
	kvk := ""
	vat := ""
	if update.KVKNumber != nil {
		kvk = *update.KVKNumber
	}
	if update.VATNumber != nil {
		vat = *update.VATNumber
	}
	return validatePartnerNumbers(kvk, vat)
}

var nlVATPattern = regexp.MustCompile(`^NL[0-9]{9}B[0-9]{2}$`)
var kvkPattern = regexp.MustCompile(`^[0-9]{8}$`)

func validatePartnerNumbers(kvk string, vat string) error {
	if kvk != "" && !kvkPattern.MatchString(strings.TrimSpace(kvk)) {
		return apperr.Validation("invalid KVK number")
	}
	if vat != "" && !nlVATPattern.MatchString(strings.ToUpper(strings.TrimSpace(vat))) {
		return apperr.Validation("invalid VAT number")
	}
	return nil
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/partners/transport"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/sanitize"

	"github.com/google/uuid"
)

type OfferSummaryItem struct {
	Description string
	Quantity    string
}

// OfferSummaryInput defines the fields sent to the summary generator (no PII).
type OfferSummaryInput struct {
	LeadID        uuid.UUID
	LeadServiceID uuid.UUID
	ServiceType   string
	Scope         *string
	UrgencyLevel  *string
	Items         []OfferSummaryItem
}

var emptyMarkdownHeadingPattern = regexp.MustCompile(`(?m)^\s*#{1,6}\s*$`)
var markdownHeadingPrefixPattern = regexp.MustCompile(`(?m)^\s*#{1,6}\s*`)
var markdownBulletPrefixPattern = regexp.MustCompile(`(?m)^\s*(?:-|\*|\d+\.)\s+`)

// OfferSummaryGenerator generates a markdown summary for partner offers.
type OfferSummaryGenerator interface {
	GenerateSummary(ctx context.Context, tenantID uuid.UUID, input OfferSummaryInput) (string, error)
}

const (
	offerTokenBytes               = 32
	defaultOfferMarginBasisPoints = 1000
	defaultOfferExpiryHours       = 12
	maxOfferExpiryHours           = 72
)

// CreateOfferFromQuote creates an offer based on a specific quote.
// This enforces that the quote is Accepted and has a linked leadServiceId.
func (s *Service) CreateOfferFromQuote(ctx context.Context, tenantID uuid.UUID, req transport.CreateOfferFromQuoteRequest) (transport.CreateOfferResponse, error) {
	partner, err := s.repo.GetByID(ctx, req.PartnerID, tenantID)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	q, err := s.repo.GetQuoteForOffer(ctx, req.QuoteID, tenantID)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	if err := validateQuoteForOffer(q); err != nil {
		return transport.CreateOfferResponse{}, err
	}

	leadServiceID := *q.LeadServiceID

	serviceCtx, err := s.resolveOfferContext(ctx, tenantID, leadServiceID)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	if err := s.ensureOfferAvailable(ctx, leadServiceID); err != nil {
		return transport.CreateOfferResponse{}, err
	}

	rawToken, err := token.GenerateRandomToken(offerTokenBytes)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	effectiveExpiryHours := req.ExpiresInHours
	if effectiveExpiryHours <= 0 {
		effectiveExpiryHours = defaultOfferExpiryHours
	}
	if effectiveExpiryHours > maxOfferExpiryHours {
		effectiveExpiryHours = maxOfferExpiryHours
	}
	expiry := time.Now().UTC().Add(time.Duration(effectiveExpiryHours) * time.Hour)

	items, itemsErr := s.repo.GetQuoteItemsForQuote(ctx, req.QuoteID, tenantID)
	if itemsErr != nil {
		items = nil
	}
	items = selectOfferItems(items, req.SelectedItemIDs)
	if len(items) == 0 {
		return transport.CreateOfferResponse{}, apperr.Validation("offer must include at least one quote item")
	}

	customerPrice := calculateCustomerPrice(items)
	if customerPrice <= 0 {
		return transport.CreateOfferResponse{}, apperr.Validation("offer total must be greater than 0")
	}

	marginBasisPoints := s.resolveOfferMarginBasisPoints(ctx, tenantID, req.MarginBasisPoints)
	vakmanPrice := resolveVakmanPrice(customerPrice, marginBasisPoints, req.VakmanPriceCents)

	scopeAssessment := buildScopeAssessment(items)
	jobSummaryPtr := sanitizeJobSummary(req.JobSummaryShort)
	offerLineItems := buildOfferLineItems(items)

	offer, err := s.repo.CreateOffer(ctx, repository.PartnerOffer{
		OrganizationID:     tenantID,
		PartnerID:          req.PartnerID,
		LeadServiceID:      leadServiceID,
		PublicToken:        rawToken,
		ExpiresAt:          expiry,
		PricingSource:      "quote",
		CustomerPriceCents: customerPrice,
		VakmanPriceCents:   vakmanPrice,
		MarginBasisPoints:  marginBasisPoints,
		OfferLineItems:     offerLineItems,
		JobSummaryShort:    jobSummaryPtr,
		RequiresInspection: resolveRequiresInspection(req.RequiresInspection),
	})
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	if payload, ok := s.buildOfferSummaryPayload(offer.ID, tenantID, leadServiceID, serviceCtx, scopeAssessment, items); ok {
		if err := s.summaryQueue.EnqueuePartnerOfferSummary(ctx, payload); err != nil {
			log.Printf("partners: failed to enqueue offer summary generation for offer=%s tenant=%s: %v", offer.ID, tenantID, err)
		}
	}

	organizationName, _ := s.repo.GetOrganizationName(ctx, tenantID)

	s.publishOfferCreated(ctx, offerCreatedParams{
		offerID:       offer.ID,
		tenantID:      tenantID,
		orgName:       organizationName,
		partnerID:     req.PartnerID,
		leadServiceID: leadServiceID,
		leadID:        serviceCtx.LeadID,
		vakmanPrice:   vakmanPrice,
		rawToken:      rawToken,
		partner:       partner,
	})

	return transport.CreateOfferResponse{
		ID:               offer.ID,
		PublicToken:      rawToken,
		VakmanPriceCents: vakmanPrice,
		ExpiresAt:        expiry,
	}, nil
}

// GetPublicOffer retrieves offer details for the vakman-facing view.
// Only exposes the vakman's price — customer markup is never served.
func (s *Service) GetPublicOffer(ctx context.Context, publicToken string) (transport.PublicOfferResponse, error) {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return transport.PublicOfferResponse{}, err
	}

	items, photos := s.resolveOfferViewData(ctx, oc)
	scopeAssessment := buildScopeAssessment(items)
	builderSummary := normalizeBuilderSummary(oc.BuilderSummary)

	return transport.PublicOfferResponse{
		OfferID:            oc.ID,
		OrganizationName:   oc.OrganizationName,
		JobSummary:         oc.ServiceType,
		JobSummaryShort:    oc.JobSummaryShort,
		BuilderSummary:     builderSummary,
		City:               oc.LeadCity,
		Postcode4:          oc.LeadPostcode4,
		Buurtcode:          oc.LeadBuurtcode,
		ConstructionYear:   oc.LeadEnergyBouwjaar,
		ScopeAssessment:    scopeAssessment,
		UrgencyLevel:       oc.UrgencyLevel,
		VakmanPriceCents:   oc.VakmanPriceCents,
		PricingSource:      oc.PricingSource,
		Status:             oc.Status,
		RequiresInspection: oc.RequiresInspection,
		ExpiresAt:          oc.ExpiresAt,
		CreatedAt:          oc.CreatedAt,
		LeadContact:        mapPublicOfferLeadContact(oc),
		PartnerPrefill:     mapPublicOfferPartnerPrefill(oc),
		LineItems:          mapPublicOfferLineItems(items),
		Photos:             mapOfferPhotos(photos),
	}, nil
}

func (s *Service) GetPublicOfferTerms(ctx context.Context, publicToken string) (transport.PartnerOfferTermsResponse, error) {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return transport.PartnerOfferTermsResponse{}, err
	}

	terms, err := s.repo.GetActivePartnerOfferTerms(ctx, oc.OrganizationID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return transport.PartnerOfferTermsResponse{}, nil
		}
		return transport.PartnerOfferTermsResponse{}, err
	}

	return mapPartnerOfferTermsResponse(terms), nil
}

func (s *Service) GetOfferTerms(ctx context.Context, tenantID uuid.UUID) (transport.PartnerOfferTermsResponse, error) {
	terms, err := s.repo.GetActivePartnerOfferTerms(ctx, tenantID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return transport.PartnerOfferTermsResponse{}, nil
		}
		return transport.PartnerOfferTermsResponse{}, err
	}
	return mapPartnerOfferTermsResponse(terms), nil
}

func (s *Service) UpdateOfferTerms(ctx context.Context, tenantID, userID uuid.UUID, req transport.UpdatePartnerOfferTermsRequest) (transport.PartnerOfferTermsResponse, error) {
	terms, err := s.repo.UpsertPartnerOfferTerms(ctx, tenantID, req.Content, userID)
	if err != nil {
		return transport.PartnerOfferTermsResponse{}, err
	}
	return mapPartnerOfferTermsResponse(terms), nil
}

func (s *Service) ListOfferTermsHistory(ctx context.Context, tenantID uuid.UUID) (transport.PartnerOfferTermsHistoryResponse, error) {
	items, err := s.repo.ListPartnerOfferTermsHistory(ctx, tenantID)
	if err != nil {
		return transport.PartnerOfferTermsHistoryResponse{}, err
	}
	result := make([]transport.PartnerOfferTermsHistoryItem, 0, len(items))
	for _, item := range items {
		result = append(result, transport.PartnerOfferTermsHistoryItem{
			ID:              item.ID,
			Content:         item.Content,
			Version:         item.Version,
			CreatedAt:       item.CreatedAt,
			CreatedByUserID: item.CreatedByUserID,
		})
	}
	return transport.PartnerOfferTermsHistoryResponse{Items: result}, nil
}

func (s *Service) IsOfferPDFReady(ctx context.Context, publicToken string) (bool, error) {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return false, err
	}
	return oc.Status == "accepted" && oc.PDFFileKey != nil && strings.TrimSpace(*oc.PDFFileKey) != "", nil
}

func (s *Service) GetOfferPDFByToken(ctx context.Context, publicToken string) (string, io.ReadCloser, error) {
	if s.storage == nil || strings.TrimSpace(s.pdfBucket) == "" {
		return "", nil, apperr.NotFound("offer pdf not available")
	}

	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return "", nil, err
	}
	if oc.Status != "accepted" {
		return "", nil, apperr.Conflict("offer pdf is only available after acceptance")
	}
	if oc.PDFFileKey == nil || strings.TrimSpace(*oc.PDFFileKey) == "" {
		return "", nil, apperr.NotFound("offer pdf is not ready yet")
	}

	reader, err := s.storage.DownloadFile(ctx, s.pdfBucket, *oc.PDFFileKey)
	if err != nil {
		return "", nil, fmt.Errorf("download offer pdf: %w", err)
	}

	fileName := fmt.Sprintf("offer-%s-signed.pdf", oc.ID.String()[:8])
	return fileName, reader, nil
}

func (s *Service) GetOfferPDF(ctx context.Context, tenantID, offerID uuid.UUID) (string, io.ReadCloser, error) {
	if s.storage == nil || strings.TrimSpace(s.pdfBucket) == "" {
		return "", nil, apperr.NotFound("offer pdf not available")
	}

	oc, err := s.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return "", nil, err
	}
	if oc.Status != "accepted" {
		return "", nil, apperr.Conflict("offer pdf is only available after acceptance")
	}
	if oc.PDFFileKey == nil || strings.TrimSpace(*oc.PDFFileKey) == "" {
		return "", nil, apperr.NotFound("offer pdf is not ready yet")
	}

	reader, err := s.storage.DownloadFile(ctx, s.pdfBucket, *oc.PDFFileKey)
	if err != nil {
		return "", nil, fmt.Errorf("download offer pdf: %w", err)
	}

	fileName := fmt.Sprintf("offer-%s-signed.pdf", oc.ID.String()[:8])
	return fileName, reader, nil
}

// AcceptOffer processes a vakman's acceptance, locks the job via the unique index.
func (s *Service) AcceptOffer(ctx context.Context, publicToken string, req transport.AcceptOfferRequest) error {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return err
	}

	// Validation
	if time.Now().After(oc.ExpiresAt) {
		return apperr.Gone("this offer has expired")
	}
	if oc.Status != "pending" && oc.Status != "sent" {
		return apperr.Conflict("offer cannot be accepted in current state")
	}

	// When inspection is required we need at least one inspection slot
	if oc.RequiresInspection && len(req.InspectionSlots) == 0 {
		return apperr.Validation("at least one inspection slot is required")
	}

	inspectionJSON, jobJSON, err := marshalOfferSlots(req.InspectionSlots, req.JobSlots)
	if err != nil {
		return err
	}

	// Normalise optional signer fields
	signerName := nilIfEmpty(req.SignerFullName)
	signerBusiness := nilIfEmpty(req.SignerBusinessName)
	signerAddress := nilIfEmpty(req.SignerAddress)
	signatureData := nilIfEmpty(req.SignatureData)

	// Atomic update (unique index enforces exclusivity)
	if err := s.repo.AcceptOffer(ctx, repository.AcceptOfferParams{
		OfferID:            oc.ID,
		InspectionSlots:    inspectionJSON,
		JobSlots:           jobJSON,
		SignerName:         signerName,
		SignerBusinessName: signerBusiness,
		SignerAddress:      signerAddress,
		SignatureData:      signatureData,
	}); err != nil {
		return err
	}

	s.enqueueAcceptedOfferPDF(ctx, oc)
	s.publishAcceptedOfferEvent(ctx, oc)

	return nil
}

func marshalOfferSlots(inspectionSlots, jobSlots []transport.TimeSlot) ([]byte, []byte, error) {
	var inspectionJSON []byte
	var err error
	if len(inspectionSlots) > 0 {
		inspectionJSON, err = json.Marshal(inspectionSlots)
		if err != nil {
			return nil, nil, apperr.Validation("invalid inspection slots")
		}
	}

	var jobJSON []byte
	if len(jobSlots) > 0 {
		jobJSON, err = json.Marshal(jobSlots)
		if err != nil {
			return nil, nil, apperr.Validation("invalid job slots")
		}
	}

	return inspectionJSON, jobJSON, nil
}

func (s *Service) enqueueAcceptedOfferPDF(ctx context.Context, oc repository.PartnerOfferWithContext) {
	if s.pdfQueue == nil {
		return
	}
	if qErr := s.pdfQueue.EnqueuePartnerOfferPDF(ctx, scheduler.PartnerOfferPDFPayload{
		OfferID:  oc.ID.String(),
		TenantID: oc.OrganizationID.String(),
	}); qErr != nil {
		log.Printf("partners: failed to enqueue offer PDF generation for offer=%s tenant=%s: %v", oc.ID, oc.OrganizationID, qErr)
	}
}

func (s *Service) publishAcceptedOfferEvent(ctx context.Context, oc repository.PartnerOfferWithContext) {
	leadID, _ := s.repo.GetLeadIDForService(ctx, oc.LeadServiceID, oc.OrganizationID)

	var partnerEmail string
	var partnerPhone string
	var partnerWhatsAppOptedIn bool
	if partner, err := s.repo.GetByID(ctx, oc.PartnerID, oc.OrganizationID); err == nil {
		partnerEmail = partner.ContactEmail
		partnerPhone = partner.ContactPhone
		partnerWhatsAppOptedIn = partner.WhatsAppOptedIn
	}

	s.eventBus.Publish(ctx, events.PartnerOfferAccepted{
		BaseEvent:              events.NewBaseEvent(),
		OfferID:                oc.ID,
		OrganizationID:         oc.OrganizationID,
		PartnerID:              oc.PartnerID,
		LeadServiceID:          oc.LeadServiceID,
		LeadID:                 leadID,
		PartnerName:            oc.PartnerName,
		PartnerEmail:           partnerEmail,
		PartnerPhone:           partnerPhone,
		PartnerWhatsAppOptedIn: partnerWhatsAppOptedIn,
	})
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// RejectOffer processes a vakman's rejection of an offer.
func (s *Service) RejectOffer(ctx context.Context, publicToken string, req transport.RejectOfferRequest) error {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return err
	}

	if oc.Status != "pending" && oc.Status != "sent" {
		return apperr.Conflict("offer cannot be rejected in current state")
	}

	if err := s.repo.RejectOffer(ctx, oc.ID, req.Reason); err != nil {
		return err
	}

	// Resolve lead ID for timeline/notification handlers
	leadID, _ := s.repo.GetLeadIDForService(ctx, oc.LeadServiceID, oc.OrganizationID)

	s.eventBus.Publish(ctx, events.PartnerOfferRejected{
		BaseEvent:      events.NewBaseEvent(),
		OfferID:        oc.ID,
		OrganizationID: oc.OrganizationID,
		PartnerID:      oc.PartnerID,
		LeadServiceID:  oc.LeadServiceID,
		LeadID:         leadID,
		PartnerName:    oc.PartnerName,
		Reason:         req.Reason,
	})

	return nil
}

// GetOfferPreview returns the same vakman-facing view but requires authentication.
// This lets admin users preview what the vakman sees.
func (s *Service) GetOfferPreview(ctx context.Context, tenantID uuid.UUID, offerID uuid.UUID) (transport.PublicOfferResponse, error) {
	oc, err := s.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return transport.PublicOfferResponse{}, err
	}

	items, photos := s.resolveOfferViewData(ctx, oc)
	scopeAssessment := buildScopeAssessment(items)
	builderSummary := normalizeBuilderSummary(oc.BuilderSummary)

	return transport.PublicOfferResponse{
		OfferID:            oc.ID,
		OrganizationName:   oc.OrganizationName,
		JobSummary:         oc.ServiceType,
		JobSummaryShort:    oc.JobSummaryShort,
		BuilderSummary:     builderSummary,
		City:               oc.LeadCity,
		Postcode4:          oc.LeadPostcode4,
		Buurtcode:          oc.LeadBuurtcode,
		ConstructionYear:   oc.LeadEnergyBouwjaar,
		ScopeAssessment:    scopeAssessment,
		UrgencyLevel:       oc.UrgencyLevel,
		VakmanPriceCents:   oc.VakmanPriceCents,
		PricingSource:      oc.PricingSource,
		Status:             oc.Status,
		RequiresInspection: oc.RequiresInspection,
		ExpiresAt:          oc.ExpiresAt,
		CreatedAt:          oc.CreatedAt,
		LeadContact:        mapPublicOfferLeadContact(oc),
		PartnerPrefill:     mapPublicOfferPartnerPrefill(oc),
		LineItems:          mapPublicOfferLineItems(items),
		Photos:             mapOfferPhotos(photos),
	}, nil
}

func (s *Service) ResendOffer(ctx context.Context, tenantID, offerID uuid.UUID) error {
	oc, err := s.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return err
	}

	status := strings.ToLower(strings.TrimSpace(oc.Status))
	if status != "pending" && status != "sent" {
		return apperr.Conflict("offer cannot be resent").WithDetails(map[string]any{"status": oc.Status})
	}
	if time.Now().After(oc.ExpiresAt) {
		return apperr.Gone("this offer has expired")
	}

	partner, err := s.repo.GetByID(ctx, oc.PartnerID, tenantID)
	if err != nil {
		return err
	}

	leadID, err := s.repo.GetLeadIDForService(ctx, oc.LeadServiceID, tenantID)
	if err != nil {
		return err
	}

	organizationName, _ := s.repo.GetOrganizationName(ctx, tenantID)

	s.publishOfferCreated(ctx, offerCreatedParams{
		offerID:       oc.ID,
		tenantID:      tenantID,
		orgName:       organizationName,
		partnerID:     oc.PartnerID,
		leadServiceID: oc.LeadServiceID,
		leadID:        leadID,
		vakmanPrice:   oc.VakmanPriceCents,
		rawToken:      oc.PublicToken,
		partner:       partner,
	})

	return nil
}

func mapPublicOfferLeadContact(offer repository.PartnerOfferWithContext) *transport.PublicOfferLeadContact {
	if offer.Status != "accepted" {
		return nil
	}
	name := strings.TrimSpace(strings.TrimSpace(offer.LeadFirstName + " " + offer.LeadLastName))
	addressParts := []string{strings.TrimSpace(offer.LeadStreet), strings.TrimSpace(offer.LeadHouseNumber)}
	streetLine := strings.TrimSpace(strings.Join(addressParts, " "))
	cityLine := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(offer.LeadZipCode), strings.TrimSpace(offer.LeadCity)}, " "))
	address := strings.TrimSpace(strings.Join([]string{streetLine, cityLine}, ", "))
	if name == "" && strings.TrimSpace(offer.LeadPhone) == "" && strings.TrimSpace(offer.LeadEmail) == "" && address == "" {
		return nil
	}
	return &transport.PublicOfferLeadContact{
		Name:    name,
		Phone:   strings.TrimSpace(offer.LeadPhone),
		Email:   strings.TrimSpace(offer.LeadEmail),
		Address: address,
	}
}

func mapPublicOfferPartnerPrefill(offer repository.PartnerOfferWithContext) *transport.PublicOfferPartnerPrefill {
	fullName := strings.TrimSpace(offer.PartnerContactName)
	businessName := strings.TrimSpace(offer.PartnerName)
	addressParts := []string{
		strings.TrimSpace(strings.Join([]string{strings.TrimSpace(offer.PartnerAddressLine1), strings.TrimSpace(offer.PartnerHouseNumber)}, " ")),
		strings.TrimSpace(offer.PartnerAddressLine2),
		strings.TrimSpace(strings.Join([]string{strings.TrimSpace(offer.PartnerPostalCode), strings.TrimSpace(offer.PartnerCity)}, " ")),
	}
	filtered := make([]string, 0, len(addressParts))
	for _, part := range addressParts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	address := strings.TrimSpace(strings.Join(filtered, ", "))
	if fullName == "" && businessName == "" && address == "" {
		return nil
	}
	return &transport.PublicOfferPartnerPrefill{
		FullName:     fullName,
		BusinessName: businessName,
		Address:      address,
	}
}

func mapPartnerOfferTermsResponse(item repository.PartnerOfferTerms) transport.PartnerOfferTermsResponse {
	return transport.PartnerOfferTermsResponse{
		Content:         item.Content,
		Version:         item.Version,
		CreatedAt:       &item.CreatedAt,
		CreatedByUserID: item.CreatedByUserID,
	}
}

func (s *Service) GetOfferPhotoByToken(ctx context.Context, publicToken string, attachmentID uuid.UUID) (repository.PhotoAttachment, io.ReadCloser, error) {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return repository.PhotoAttachment{}, nil, err
	}

	return s.getOfferPhotoStream(ctx, oc.LeadServiceID, oc.OrganizationID, attachmentID)
}

func (s *Service) GetOfferPhotoPreview(ctx context.Context, tenantID, offerID, attachmentID uuid.UUID) (repository.PhotoAttachment, io.ReadCloser, error) {
	oc, err := s.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return repository.PhotoAttachment{}, nil, err
	}

	return s.getOfferPhotoStream(ctx, oc.LeadServiceID, oc.OrganizationID, attachmentID)
}

func buildBuilderSummary(items []repository.QuoteItemSummary, scopeAssessment *string, urgencyLevel *string, requiresInspection bool) *string {
	if len(items) == 0 {
		return nil
	}

	lines := buildSummaryHeader(scopeAssessment, urgencyLevel)
	if intro := buildSummaryIntro(items); intro != "" {
		lines = append(lines, intro, "")
	}
	if workItems := buildSummaryItems(items); len(workItems) > 0 {
		lines = append(lines, "### Werkzaamheden")
		lines = append(lines, workItems...)
	}
	if attentionItems := buildSummaryAttentionPoints(items, scopeAssessment, urgencyLevel, requiresInspection); len(attentionItems) > 0 {
		lines = append(lines, "", "### Let op")
		lines = append(lines, attentionItems...)
	}
	if len(lines) == 0 {
		return nil
	}

	result := strings.Join(lines, "\n")
	return &result
}

func (s *Service) resolveOfferContext(ctx context.Context, tenantID, leadServiceID uuid.UUID) (repository.LeadServiceSummaryContext, error) {
	if err := s.ensureLeadServiceExists(ctx, tenantID, leadServiceID); err != nil {
		return repository.LeadServiceSummaryContext{}, err
	}
	return s.repo.GetLeadServiceSummaryContext(ctx, leadServiceID, tenantID)
}

func (s *Service) ensureOfferAvailable(ctx context.Context, leadServiceID uuid.UUID) error {
	hasActive, err := s.repo.HasActiveOffer(ctx, leadServiceID)
	if err != nil {
		return err
	}
	if hasActive {
		return apperr.Conflict("an active offer already exists for this service")
	}
	return nil
}

func (s *Service) buildOfferSummaryPayload(offerID, tenantID, leadServiceID uuid.UUID, serviceCtx repository.LeadServiceSummaryContext, scopeAssessment *string, items []repository.QuoteItemSummary) (scheduler.PartnerOfferSummaryPayload, bool) {
	if s == nil || s.summaryQueue == nil || s.summaryGenerator == nil || len(items) == 0 {
		return scheduler.PartnerOfferSummaryPayload{}, false
	}

	payload := scheduler.PartnerOfferSummaryPayload{
		OfferID:       offerID.String(),
		TenantID:      tenantID.String(),
		LeadID:        serviceCtx.LeadID.String(),
		LeadServiceID: leadServiceID.String(),
		ServiceType:   serviceCtx.ServiceType,
		Scope:         scopeAssessment,
		UrgencyLevel:  serviceCtx.UrgencyLevel,
		Items:         make([]scheduler.PartnerOfferSummaryItemPayload, 0, len(items)),
	}

	for _, item := range items {
		payload.Items = append(payload.Items, scheduler.PartnerOfferSummaryItemPayload{
			Description: sanitizeSummaryText(item.Description),
			Quantity:    item.Quantity,
		})
	}

	return payload, true
}

func (s *Service) resolveOfferMarginBasisPoints(ctx context.Context, tenantID uuid.UUID, override *int) int {
	if override != nil {
		return clampMarginBasisPoints(*override)
	}
	if s != nil && s.settingsReader != nil {
		settings, err := s.settingsReader(ctx, tenantID)
		if err == nil {
			return clampMarginBasisPoints(settings.OfferMarginBasisPoints)
		}
	}
	return defaultOfferMarginBasisPoints
}

func (s *Service) resolveOfferViewData(ctx context.Context, oc repository.PartnerOfferWithContext) ([]repository.QuoteItemSummary, []repository.PhotoAttachment) {
	items := cloneQuoteItemsFromOffer(oc.OfferLineItems)
	if len(items) == 0 {
		latest, err := s.repo.GetLatestQuoteItemsForService(ctx, oc.LeadServiceID, oc.OrganizationID)
		if err == nil {
			items = latest
		}
	}

	photos, err := s.repo.GetLeadServiceImageAttachments(ctx, oc.LeadServiceID, oc.OrganizationID)
	if err != nil {
		photos = nil
	}

	return items, photos
}

func (s *Service) getOfferPhotoStream(ctx context.Context, leadServiceID, organizationID, attachmentID uuid.UUID) (repository.PhotoAttachment, io.ReadCloser, error) {
	if s == nil || s.storage == nil {
		return repository.PhotoAttachment{}, nil, apperr.Internal("photo storage is unavailable")
	}
	if strings.TrimSpace(s.attachmentsBucket) == "" {
		return repository.PhotoAttachment{}, nil, apperr.Internal("photo storage bucket is not configured")
	}

	attachment, err := s.repo.GetLeadServiceImageAttachmentByID(ctx, attachmentID, leadServiceID, organizationID)
	if err != nil {
		return repository.PhotoAttachment{}, nil, err
	}

	reader, err := s.storage.DownloadFile(ctx, s.attachmentsBucket, attachment.FileKey)
	if err != nil {
		return repository.PhotoAttachment{}, nil, apperr.Internal("failed to load offer photo")
	}

	return attachment, reader, nil
}

func sanitizeJobSummary(value string) *string {
	jobSummary := strings.TrimSpace(value)
	jobSummary = sanitize.Text(jobSummary)
	if jobSummary == "" {
		return nil
	}
	return &jobSummary
}

func validateQuoteForOffer(q repository.QuoteForOffer) error {
	if strings.TrimSpace(q.Status) != "Accepted" {
		return apperr.Conflict("quote must be Accepted")
	}
	if q.LeadServiceID == nil || *q.LeadServiceID == uuid.Nil {
		return apperr.Validation("quote must be linked to a lead service")
	}
	if q.TotalCents <= 0 {
		return apperr.Validation("quote total must be greater than 0")
	}
	return nil
}

type offerCreatedParams struct {
	offerID       uuid.UUID
	tenantID      uuid.UUID
	orgName       string
	partnerID     uuid.UUID
	leadServiceID uuid.UUID
	leadID        uuid.UUID
	vakmanPrice   int64
	rawToken      string
	partner       repository.Partner
}

func (s *Service) publishOfferCreated(ctx context.Context, params offerCreatedParams) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(ctx, events.PartnerOfferCreated{
		BaseEvent:        events.NewBaseEvent(),
		OfferID:          params.offerID,
		OrganizationID:   params.tenantID,
		OrganizationName: params.orgName,
		PartnerID:        params.partnerID,
		LeadServiceID:    params.leadServiceID,
		LeadID:           params.leadID,
		VakmanPriceCents: params.vakmanPrice,
		PublicToken:      params.rawToken,
		PartnerName:      params.partner.BusinessName,
		PartnerPhone:     params.partner.ContactPhone,
		PartnerEmail:     params.partner.ContactEmail,
	})
}

func buildSummaryHeader(scopeAssessment *string, urgencyLevel *string) []string {
	scopeLabel := mapScopeLabel(scopeAssessment)
	urgencyLabel := mapUrgencyLabel(urgencyLevel)
	if scopeLabel == "" && urgencyLabel == "" {
		return nil
	}
	if scopeLabel == "" {
		scopeLabel = "Onbekend"
	}
	if urgencyLabel == "" {
		urgencyLabel = "Onbekend"
	}

	return []string{
		fmt.Sprintf("**Omvang** %s  **Urgentie** %s", scopeLabel, urgencyLabel),
		"",
	}
}

func buildSummaryAttentionPoints(items []repository.QuoteItemSummary, scopeAssessment *string, urgencyLevel *string, requiresInspection bool) []string {
	points := make([]string, 0, 3)

	if requiresInspection {
		points = append(points, "- Plan eerst een schouw of eerste opname om de situatie, maatvoering en bereikbaarheid te controleren.")
	}

	if mapUrgencyLabel(urgencyLevel) == "Hoog" {
		points = append(points, "- Deze aanvraag staat als urgent gemarkeerd, dus snelle terugkoppeling en planning liggen voor de hand.")
	}

	if mapScopeLabel(scopeAssessment) == "Groot" || len(items) > 4 {
		points = append(points, "- Houd rekening met extra afstemming over materiaal, tijdsduur of fasering omdat dit geen kleine klus lijkt.")
	}

	if len(points) == 0 && len(items) > 0 {
		points = append(points, "- Controleer vooraf of de inbegrepen posten volledig aansluiten op wat u op locatie verwacht aan te treffen.")
	}

	if len(points) > 2 {
		return points[:2]
	}

	return points
}

func buildSummaryIntro(items []repository.QuoteItemSummary) string {
	if len(items) == 0 {
		return ""
	}

	main, _ := buildSummaryItem(items[0], false, 0)
	main = strings.TrimSpace(main)
	if main == "" {
		return ""
	}

	if len(items) == 1 {
		return fmt.Sprintf("Deze klus draait vooral om %s.", lowerFirst(main))
	}

	return fmt.Sprintf("Deze klus draait vooral om %s, met nog %d aanvullende werkzaamheden.", lowerFirst(main), len(items)-1)
}

func buildSummaryItems(items []repository.QuoteItemSummary) []string {
	const maxItems = 3
	remaining := len(items) - maxItems
	if remaining < 0 {
		remaining = 0
	}

	lines := make([]string, 0, len(items))
	for i, it := range items {
		if i >= maxItems {
			break
		}
		main, inclusions := buildSummaryItem(it, remaining > 0 && i == maxItems-1, remaining)
		if main == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s", main))
		for _, inc := range inclusions {
			lines = append(lines, "   - "+inc)
		}
	}

	return lines
}

func buildSummaryItem(item repository.QuoteItemSummary, isLast bool, remaining int) (string, []string) {
	quantity := strings.TrimSpace(item.Quantity)
	main, inclusions := splitInclusions(item.Description)
	main = sanitizeSummaryText(main)
	for index := range inclusions {
		inclusions[index] = sanitizeSummaryText(inclusions[index])
	}
	main = strings.TrimSpace(main)
	if main == "" {
		return "", nil
	}
	if quantity != "" {
		main = strings.TrimSpace(quantity + " " + main)
	}
	if len(inclusions) > 0 {
		main += " Inclusief:"
	}
	if isLast {
		main = fmt.Sprintf("%s (+%d)", main, remaining)
	}

	return main, inclusions
}

func sanitizeSummaryText(value string) string {
	clean := sanitize.Text(value)
	return strings.TrimSpace(strings.Join(strings.Fields(clean), " "))
}

func lowerFirst(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	runes := []rune(trimmed)
	runes[0] = []rune(strings.ToLower(string(runes[0])))[0]
	return string(runes)
}

// splitOfferSummary splits AI output into short + full parts separated by "---".
// If no separator is found, the entire text is returned as the full summary.
func splitOfferSummary(raw string) (short string, full string) {
	clean := strings.TrimSpace(sanitize.Text(raw))
	if clean == "" {
		return "", ""
	}

	parts := strings.SplitN(clean, "---", 2)
	if len(parts) == 2 {
		s := strings.TrimSpace(parts[0])
		f := strings.TrimSpace(parts[1])
		if f != "" {
			runes := []rune(s)
			if len(runes) > 200 {
				s = string(runes[:200])
			}
			return s, f
		}
	}

	return "", clean
}

func normalizeBuilderSummary(value *string) *string {
	if value == nil {
		return nil
	}
	clean := sanitize.Text(*value)
	clean = strings.ReplaceAll(clean, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")
	clean = strings.ReplaceAll(clean, "\u00a0", " ")
	clean = emptyMarkdownHeadingPattern.ReplaceAllString(clean, "")
	clean = markdownHeadingPrefixPattern.ReplaceAllString(clean, "")
	clean = markdownBulletPrefixPattern.ReplaceAllString(clean, "")
	clean = strings.ReplaceAll(clean, "**", "")
	clean = strings.ReplaceAll(clean, "__", "")
	clean = strings.ReplaceAll(clean, "`", "")
	clean = normalizeSummaryLines(clean)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return nil
	}
	return &clean
}

func normalizeSummaryLines(value string) string {
	lines := strings.Split(value, "\n")
	normalized := make([]string, 0, len(lines))
	previousBlank := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if previousBlank {
				continue
			}
			normalized = append(normalized, "")
			previousBlank = true
			continue
		}

		normalized = append(normalized, trimmed)
		previousBlank = false
	}

	return strings.TrimSpace(strings.Join(normalized, "\n"))
}

func buildScopeAssessment(items []repository.QuoteItemSummary) *string {
	count := len(items)
	if count == 0 {
		return nil
	}
	var scope string
	switch {
	case count <= 2:
		scope = "Small"
	case count <= 5:
		scope = "Medium"
	default:
		scope = "Large"
	}
	return &scope
}

func mapScopeLabel(scope *string) string {
	if scope == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(*scope)) {
	case "small":
		return "Klein"
	case "medium":
		return "Middel"
	case "large":
		return "Groot"
	default:
		return ""
	}
}

func mapUrgencyLabel(level *string) string {
	if level == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(*level)) {
	case "low":
		return "Laag"
	case "medium":
		return "Gemiddeld"
	case "high":
		return "Hoog"
	default:
		return ""
	}
}

func splitInclusions(description string) (string, []string) {
	parts := strings.SplitN(description, "Inclusief:", 2)
	main := strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		return main, nil
	}

	return main, parseInclusionList(parts[1])
}

func parseInclusionList(value string) []string {
	cleaned := strings.ReplaceAll(value, "\n\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, " - ", "\n- ")
	lines := strings.Split(cleaned, "\n")

	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		}
		if line != "" {
			items = append(items, line)
		}
	}

	return items
}

// ListOffers returns the global offers overview (admin view), paginated.
func (s *Service) ListOffers(ctx context.Context, tenantID uuid.UUID, req transport.ListOffersRequest) (transport.OfferListResponse, error) {
	var partnerID, leadServiceID, serviceTypeID uuid.UUID
	if req.PartnerID != "" {
		partnerID, _ = uuid.Parse(req.PartnerID)
	}
	if req.LeadServiceID != "" {
		leadServiceID, _ = uuid.Parse(req.LeadServiceID)
	}
	if req.ServiceTypeID != "" {
		serviceTypeID, _ = uuid.Parse(req.ServiceTypeID)
	}

	result, err := s.repo.ListOffers(ctx, repository.OfferListParams{
		OrganizationID: tenantID,
		Search:         req.Search,
		Status:         req.Status,
		PartnerID:      partnerID,
		LeadServiceID:  leadServiceID,
		ServiceTypeID:  serviceTypeID,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
		Page:           req.Page,
		PageSize:       req.PageSize,
	})
	if err != nil {
		return transport.OfferListResponse{}, err
	}

	items := make([]transport.OfferResponse, 0, len(result.Items))
	for _, o := range result.Items {
		items = append(items, mapOfferResponse(o))
	}

	return transport.OfferListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}, nil
}

// DeleteOffer deletes an offer only when it is not accepted and not rejected.
// We allow deletion for: pending, sent, expired.
func (s *Service) DeleteOffer(ctx context.Context, tenantID uuid.UUID, offerID uuid.UUID) error {
	offer, err := s.repo.GetOfferByID(ctx, offerID, tenantID)
	if err != nil {
		return err
	}

	status := strings.ToLower(strings.TrimSpace(offer.Status))
	if status != "pending" && status != "sent" && status != "expired" {
		return apperr.Conflict("offer cannot be deleted").WithDetails(map[string]any{"status": offer.Status})
	}

	if err := s.repo.DeleteOffer(ctx, offerID, tenantID); err != nil {
		return err
	}

	// Publish event so the orchestrator can reconcile the pipeline stage.
	leadID, _ := s.repo.GetLeadIDForService(ctx, offer.LeadServiceID, tenantID)
	s.eventBus.Publish(ctx, events.PartnerOfferDeleted{
		BaseEvent:      events.NewBaseEvent(),
		OfferID:        offerID,
		OrganizationID: tenantID,
		PartnerID:      offer.PartnerID,
		LeadServiceID:  offer.LeadServiceID,
		LeadID:         leadID,
	})

	return nil
}

// ListOffersByPartner returns all offers for a given partner (admin view).
func (s *Service) ListOffersByPartner(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID) (transport.ListOffersResponse, error) {
	if err := s.ensurePartnerExists(ctx, tenantID, partnerID); err != nil {
		return transport.ListOffersResponse{}, err
	}

	offers, err := s.repo.ListOffersByPartner(ctx, partnerID, tenantID)
	if err != nil {
		return transport.ListOffersResponse{}, err
	}

	items := make([]transport.OfferResponse, 0, len(offers))
	for _, o := range offers {
		items = append(items, mapOfferResponse(o))
	}

	return transport.ListOffersResponse{Items: items}, nil
}

// ListOffersForService returns all offers for a given lead service (admin view).
func (s *Service) ListOffersForService(ctx context.Context, tenantID uuid.UUID, leadServiceID uuid.UUID) (transport.ListOffersResponse, error) {
	offers, err := s.repo.ListOffersForService(ctx, leadServiceID, tenantID)
	if err != nil {
		return transport.ListOffersResponse{}, err
	}

	items := make([]transport.OfferResponse, 0, len(offers))
	for _, o := range offers {
		items = append(items, mapOfferResponse(o))
	}

	return transport.ListOffersResponse{Items: items}, nil
}

// ExpireOffers is called by a background job to expire stale offers.
func (s *Service) ExpireOffers(ctx context.Context) (int, error) {
	expired, err := s.repo.ExpireOffers(ctx)
	if err != nil {
		return 0, err
	}

	for _, o := range expired {
		// Resolve lead ID and partner name for timeline handlers
		leadID, _ := s.repo.GetLeadIDForService(ctx, o.LeadServiceID, o.OrganizationID)
		var partnerName string
		if p, err := s.repo.GetByID(ctx, o.PartnerID, o.OrganizationID); err == nil {
			partnerName = p.BusinessName
		}

		s.eventBus.Publish(ctx, events.PartnerOfferExpired{
			BaseEvent:      events.NewBaseEvent(),
			OfferID:        o.ID,
			OrganizationID: o.OrganizationID,
			PartnerID:      o.PartnerID,
			LeadServiceID:  o.LeadServiceID,
			LeadID:         leadID,
			PartnerName:    partnerName,
		})
	}

	return len(expired), nil
}

func mapOfferResponse(oc repository.PartnerOfferWithContext) transport.OfferResponse {
	resp := transport.OfferResponse{
		ID:                 oc.ID,
		PartnerID:          oc.PartnerID,
		PartnerName:        oc.PartnerName,
		LeadServiceID:      oc.LeadServiceID,
		PricingSource:      oc.PricingSource,
		CustomerPriceCents: oc.CustomerPriceCents,
		VakmanPriceCents:   oc.VakmanPriceCents,
		Status:             oc.Status,
		PublicToken:        oc.PublicToken,
		ExpiresAt:          oc.ExpiresAt,
		AcceptedAt:         oc.AcceptedAt,
		RejectedAt:         oc.RejectedAt,
		CreatedAt:          oc.CreatedAt,
	}
	if strings.TrimSpace(oc.ServiceType) != "" {
		resp.ServiceType = &oc.ServiceType
	}
	if strings.TrimSpace(oc.LeadCity) != "" {
		resp.LeadCity = &oc.LeadCity
	}

	if oc.ServiceTypeID != uuid.Nil {
		resp.ServiceTypeID = &oc.ServiceTypeID
	}

	if oc.RejectionReason != nil {
		resp.RejectionReason = *oc.RejectionReason
	}
	return resp
}

func calculateVakmanPrice(customerPriceCents int64) int64 {
	return resolveVakmanPrice(customerPriceCents, defaultOfferMarginBasisPoints, nil)
}

func resolveVakmanPrice(customerPriceCents int64, marginBasisPoints int, override *int64) int64 {
	if override != nil {
		if *override < 0 {
			return 0
		}
		return *override
	}

	margin := int64(clampMarginBasisPoints(marginBasisPoints))
	price := customerPriceCents - (customerPriceCents*margin)/10000
	if price < 0 {
		return 0
	}
	return price
}

func clampMarginBasisPoints(value int) int {
	if value < 0 {
		return 0
	}
	if value > 5000 {
		return 5000
	}
	return value
}

func calculateCustomerPrice(items []repository.QuoteItemSummary) int64 {
	var total int64
	for _, item := range items {
		total += item.LineTotalCents
	}
	return total
}

func buildOfferLineItems(items []repository.QuoteItemSummary) []repository.OfferLineItem {
	lineItems := make([]repository.OfferLineItem, 0, len(items))
	for _, item := range items {
		lineItems = append(lineItems, repository.OfferLineItem{
			QuoteItemID:    item.ID,
			Description:    item.Description,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
			LineTotalCents: item.LineTotalCents,
		})
	}
	return lineItems
}

func cloneQuoteItemsFromOffer(items []repository.OfferLineItem) []repository.QuoteItemSummary {
	if len(items) == 0 {
		return nil
	}

	result := make([]repository.QuoteItemSummary, 0, len(items))
	for _, item := range items {
		result = append(result, repository.QuoteItemSummary{
			ID:             item.QuoteItemID,
			Description:    item.Description,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
			LineTotalCents: item.LineTotalCents,
		})
	}
	return result
}

func selectOfferItems(items []repository.QuoteItemSummary, selectedItemIDs []uuid.UUID) []repository.QuoteItemSummary {
	if len(selectedItemIDs) == 0 {
		return items
	}

	selected := make(map[uuid.UUID]struct{}, len(selectedItemIDs))
	for _, id := range selectedItemIDs {
		selected[id] = struct{}{}
	}

	filtered := make([]repository.QuoteItemSummary, 0, len(items))
	for _, item := range items {
		if _, ok := selected[item.ID]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func mapPublicOfferLineItems(items []repository.QuoteItemSummary) []transport.PublicOfferLineItem {
	if len(items) == 0 {
		return nil
	}

	result := make([]transport.PublicOfferLineItem, 0, len(items))
	for _, item := range items {
		result = append(result, transport.PublicOfferLineItem{
			Description: item.Description,
			Quantity:    item.Quantity,
		})
	}
	return result
}

func mapOfferPhotos(photos []repository.PhotoAttachment) []transport.OfferPhotoRef {
	if len(photos) == 0 {
		return nil
	}

	result := make([]transport.OfferPhotoRef, 0, len(photos))
	for _, photo := range photos {
		result = append(result, transport.OfferPhotoRef{
			ID:          photo.ID,
			FileName:    photo.FileName,
			ContentType: photo.ContentType,
		})
	}
	return result
}

func resolveRequiresInspection(v *bool) bool {
	if v == nil {
		return true // default: inspection required
	}
	return *v
}

// WithPDFQueue attaches a PDF job queue to the service.
func (s *Service) WithPDFQueue(q OfferPDFJobQueue) {
	s.pdfQueue = q
}

// GetOfferDetail returns the full detail view of an offer for admins.
func (s *Service) GetOfferDetail(ctx context.Context, tenantID, offerID uuid.UUID) (transport.OfferDetailResponse, error) {
	oc, err := s.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return transport.OfferDetailResponse{}, err
	}

	items, photos := s.resolveOfferViewData(ctx, oc)

	var inspectionSlots []transport.TimeSlot
	if len(oc.InspectionAvailability) > 0 {
		_ = json.Unmarshal(oc.InspectionAvailability, &inspectionSlots)
	}

	var jobSlots []transport.TimeSlot
	if len(oc.JobAvailability) > 0 {
		_ = json.Unmarshal(oc.JobAvailability, &jobSlots)
	}

	lineItems := make([]transport.OfferDetailLineItem, 0, len(items))
	for _, item := range items {
		lineItems = append(lineItems, transport.OfferDetailLineItem{
			Description:    item.Description,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
			LineTotalCents: item.LineTotalCents,
		})
	}

	return transport.OfferDetailResponse{
		ID:                 oc.ID,
		PartnerID:          oc.PartnerID,
		PartnerName:        oc.PartnerName,
		LeadServiceID:      oc.LeadServiceID,
		ServiceType:        oc.ServiceType,
		LeadCity:           oc.LeadCity,
		Status:             oc.Status,
		RequiresInspection: oc.RequiresInspection,
		PublicToken:        oc.PublicToken,
		VakmanPriceCents:   oc.VakmanPriceCents,
		CustomerPriceCents: oc.CustomerPriceCents,
		JobSummaryShort:    oc.JobSummaryShort,
		BuilderSummary:     oc.BuilderSummary,
		LineItems:          lineItems,
		Photos:             mapOfferPhotos(photos),
		ExpiresAt:          oc.ExpiresAt,
		CreatedAt:          oc.CreatedAt,
		AcceptedAt:         oc.AcceptedAt,
		RejectedAt:         oc.RejectedAt,
		RejectionReason:    oc.RejectionReason,
		InspectionSlots:    inspectionSlots,
		JobSlots:           jobSlots,
		SignerName:         oc.SignerName,
		SignerBusinessName: oc.SignerBusinessName,
		SignerAddress:      oc.SignerAddress,
		PDFFileKey:         oc.PDFFileKey,
	}, nil
}

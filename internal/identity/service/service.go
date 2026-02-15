package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/transport"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	inviteTokenBytes         = 32
	inviteTTL                = 72 * time.Hour
	inviteNotFound           = "invite not found"
	organizationNotFound     = "organization not found"
	whatsappNotConfiguredMsg = "whatsapp service not configured"
)

type Service struct {
	repo              *repository.Repository
	eventBus          events.Bus
	storage           storage.StorageService
	logoBucket        string
	whatsapp          *whatsapp.Client
	smtpEncryptionKey []byte
}

func New(repo *repository.Repository, eventBus events.Bus, storageSvc storage.StorageService, logoBucket string, whatsappClient *whatsapp.Client) *Service {
	return &Service{repo: repo, eventBus: eventBus, storage: storageSvc, logoBucket: logoBucket, whatsapp: whatsappClient}
}

func (s *Service) GetUserOrganizationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.repo.GetUserOrganizationID(ctx, userID)
}

func (s *Service) CreateOrganizationForUser(ctx context.Context, q repository.DBTX, name string, userID uuid.UUID) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return uuid.UUID{}, apperr.Validation("organization name is required")
	}

	org, err := s.repo.CreateOrganization(ctx, q, trimmed, userID)
	if err != nil {
		return uuid.UUID{}, err
	}

	if s.eventBus != nil {
		if err := s.eventBus.PublishSync(ctx, events.OrganizationCreated{
			BaseEvent:      events.NewBaseEvent(),
			OrganizationID: org.ID,
			CreatedBy:      userID,
		}); err != nil {
			return uuid.UUID{}, err
		}
	}

	return org.ID, nil
}

func (s *Service) AddMember(ctx context.Context, q repository.DBTX, organizationID, userID uuid.UUID) error {
	return s.repo.AddMember(ctx, q, organizationID, userID)
}

func (s *Service) CreateInvite(ctx context.Context, organizationID uuid.UUID, email string, createdBy uuid.UUID) (string, time.Time, error) {
	rawToken, err := token.GenerateRandomToken(inviteTokenBytes)
	if err != nil {
		return "", time.Time{}, err
	}

	tokenHash := token.HashSHA256(rawToken)
	expiresAt := time.Now().Add(inviteTTL)

	if _, err := s.repo.CreateInvite(ctx, organizationID, email, tokenHash, expiresAt, createdBy); err != nil {
		return "", time.Time{}, err
	}

	// Publish event to send invite email
	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err == nil && s.eventBus != nil {
		s.eventBus.Publish(ctx, events.OrganizationInviteCreated{
			BaseEvent:        events.NewBaseEvent(),
			OrganizationID:   organizationID,
			OrganizationName: org.Name,
			Email:            email,
			InviteToken:      rawToken,
		})
	}

	return rawToken, expiresAt, nil
}

func (s *Service) GetOrganization(ctx context.Context, organizationID uuid.UUID) (repository.Organization, error) {
	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound(organizationNotFound)
		}
		return repository.Organization{}, err
	}
	return org, nil
}

func (s *Service) GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (repository.OrganizationSettings, error) {
	return s.repo.GetOrganizationSettings(ctx, organizationID)
}

func (s *Service) UpdateOrganizationSettings(
	ctx context.Context,
	organizationID uuid.UUID,
	update repository.OrganizationSettingsUpdate,
) (repository.OrganizationSettings, error) {
	return s.repo.UpsertOrganizationSettings(ctx, organizationID, update)
}

type ResolveLeadWorkflowInput struct {
	OrganizationID  uuid.UUID
	LeadID          uuid.UUID
	LeadSource      *string
	LeadServiceType *string
	PipelineStage   *string
}

type ResolveLeadWorkflowResult struct {
	Workflow         *repository.Workflow
	ResolutionSource string
	OverrideMode     *string
	MatchedRuleID    *uuid.UUID
}

func (s *Service) ListWorkflows(ctx context.Context, organizationID uuid.UUID) ([]repository.Workflow, error) {
	return s.repo.ListWorkflows(ctx, organizationID)
}

func (s *Service) ReplaceWorkflows(ctx context.Context, organizationID uuid.UUID, workflows []repository.WorkflowUpsert) ([]repository.Workflow, error) {
	normalized := normalizeWorkflowUpserts(workflows)
	return s.repo.ReplaceWorkflows(ctx, organizationID, normalized)
}

func normalizeWorkflowUpserts(workflows []repository.WorkflowUpsert) []repository.WorkflowUpsert {
	normalized := make([]repository.WorkflowUpsert, 0, len(workflows))
	for _, workflow := range workflows {
		normalized = append(normalized, workflow)
	}
	return normalized
}

func (s *Service) ListWorkflowAssignmentRules(ctx context.Context, organizationID uuid.UUID) ([]repository.WorkflowAssignmentRule, error) {
	return s.repo.ListWorkflowAssignmentRules(ctx, organizationID)
}

func (s *Service) ReplaceWorkflowAssignmentRules(ctx context.Context, organizationID uuid.UUID, rules []repository.WorkflowAssignmentRuleUpsert) ([]repository.WorkflowAssignmentRule, error) {
	return s.repo.ReplaceWorkflowAssignmentRules(ctx, organizationID, rules)
}

func (s *Service) GetLeadWorkflowOverride(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (repository.LeadWorkflowOverride, error) {
	override, err := s.repo.GetLeadWorkflowOverride(ctx, leadID, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.LeadWorkflowOverride{}, apperr.NotFound("lead workflow override not found")
		}
		return repository.LeadWorkflowOverride{}, err
	}
	return override, nil
}

func (s *Service) UpsertLeadWorkflowOverride(ctx context.Context, upsert repository.LeadWorkflowOverrideUpsert) (repository.LeadWorkflowOverride, error) {
	leadExists, err := s.repo.LeadExistsInOrganization(ctx, upsert.LeadID, upsert.OrganizationID)
	if err != nil {
		return repository.LeadWorkflowOverride{}, err
	}
	if !leadExists {
		return repository.LeadWorkflowOverride{}, apperr.NotFound("lead not found")
	}

	if upsert.WorkflowID != nil {
		workflowExists, err := s.repo.WorkflowExistsInOrganization(ctx, *upsert.WorkflowID, upsert.OrganizationID)
		if err != nil {
			return repository.LeadWorkflowOverride{}, err
		}
		if !workflowExists {
			return repository.LeadWorkflowOverride{}, apperr.NotFound("workflow not found")
		}
	}

	return s.repo.UpsertLeadWorkflowOverride(ctx, upsert)
}

func (s *Service) DeleteLeadWorkflowOverride(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) error {
	_, err := s.repo.GetLeadWorkflowOverride(ctx, leadID, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return apperr.NotFound("lead workflow override not found")
		}
		return err
	}

	return s.repo.DeleteLeadWorkflowOverride(ctx, leadID, organizationID)
}

func (s *Service) ResolveLeadWorkflow(ctx context.Context, input ResolveLeadWorkflowInput) (ResolveLeadWorkflowResult, error) {
	workflows, err := s.repo.ListWorkflows(ctx, input.OrganizationID)
	if err != nil {
		return ResolveLeadWorkflowResult{}, err
	}

	workflowMap := buildEnabledWorkflowMap(workflows)

	override, err := s.repo.GetLeadWorkflowOverride(ctx, input.LeadID, input.OrganizationID)
	if err != nil && err != repository.ErrNotFound {
		return ResolveLeadWorkflowResult{}, err
	}

	if err == nil {
		result, matched := resolveWorkflowFromOverride(override, workflowMap)
		if matched {
			return result, nil
		}
		return ResolveLeadWorkflowResult{
			ResolutionSource: "manual_override",
			OverrideMode:     &override.OverrideMode,
		}, nil
	}

	rules, err := s.repo.ListWorkflowAssignmentRules(ctx, input.OrganizationID)
	if err != nil {
		return ResolveLeadWorkflowResult{}, err
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !matchesAssignmentRule(rule, input) {
			continue
		}

		if workflow, ok := workflowMap[rule.WorkflowID]; ok {
			ruleID := rule.ID
			return ResolveLeadWorkflowResult{
				Workflow:         workflow,
				ResolutionSource: "auto_rule",
				MatchedRuleID:    &ruleID,
			}, nil
		}
	}

	return ResolveLeadWorkflowResult{ResolutionSource: "organization_default"}, nil
}

type WhatsAppStatus struct {
	State       string `json:"state"`
	Message     string `json:"message"`
	CanSend     bool   `json:"canSend"`
	NeedsReauth bool   `json:"needsReauth"`
}

func (s *Service) RegisterWhatsAppDevice(ctx context.Context, organizationID uuid.UUID) (string, error) {
	deviceID := fmt.Sprintf("org_%s", organizationID.String())
	if s.whatsapp == nil {
		return "", apperr.Internal(whatsappNotConfiguredMsg)
	}
	if err := s.whatsapp.CreateDevice(ctx, deviceID); err != nil {
		return "", apperr.Internal("failed to register device with provider: " + err.Error())
	}

	update := repository.OrganizationSettingsUpdate{
		WhatsAppDeviceID: &deviceID,
	}
	if _, err := s.repo.UpsertOrganizationSettings(ctx, organizationID, update); err != nil {
		return "", err
	}

	return deviceID, nil
}

func (s *Service) GetWhatsAppQR(ctx context.Context, organizationID uuid.UUID) ([]byte, error) {
	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if settings.WhatsAppDeviceID == nil || *settings.WhatsAppDeviceID == "" {
		return nil, apperr.Validation("no device registered for this organization")
	}
	if s.whatsapp == nil {
		return nil, apperr.Internal(whatsappNotConfiguredMsg)
	}

	return s.whatsapp.GetLoginQR(ctx, *settings.WhatsAppDeviceID)
}

func (s *Service) DisconnectWhatsAppDevice(ctx context.Context, organizationID uuid.UUID) error {
	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return err
	}

	if settings.WhatsAppDeviceID != nil && *settings.WhatsAppDeviceID != "" && s.whatsapp != nil {
		_ = s.whatsapp.DeleteDevice(ctx, *settings.WhatsAppDeviceID)
	}

	clear := ""
	update := repository.OrganizationSettingsUpdate{
		WhatsAppDeviceID: &clear,
	}
	_, err = s.repo.UpsertOrganizationSettings(ctx, organizationID, update)
	return err
}

func (s *Service) GetWhatsAppStatus(ctx context.Context, organizationID uuid.UUID) (WhatsAppStatus, error) {
	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return WhatsAppStatus{}, err
	}
	if settings.WhatsAppDeviceID == nil || *settings.WhatsAppDeviceID == "" {
		return WhatsAppStatus{State: "UNREGISTERED", Message: "No device linked", CanSend: false}, nil
	}
	if s.whatsapp == nil {
		return WhatsAppStatus{}, apperr.Internal(whatsappNotConfiguredMsg)
	}

	upstreamStatus, err := s.whatsapp.GetDeviceStatus(ctx, *settings.WhatsAppDeviceID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return WhatsAppStatus{
				State:       "ERROR",
				Message:     "Device configuration lost upstream. Please register again.",
				CanSend:     false,
				NeedsReauth: true,
			}, nil
		}
		return WhatsAppStatus{}, err
	}

	if upstreamStatus.IsLoggedIn {
		return WhatsAppStatus{State: "CONNECTED", Message: "Online", CanSend: true}, nil
	}

	msg := "Waiting for authentication"
	if upstreamStatus.IsConnected {
		msg = "Connected but not logged in"
	}

	return WhatsAppStatus{
		State:       "DISCONNECTED",
		Message:     msg,
		CanSend:     false,
		NeedsReauth: true,
	}, nil
}

func (s *Service) AttemptReconnect(ctx context.Context, organizationID uuid.UUID) error {
	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return err
	}
	if settings.WhatsAppDeviceID == nil || *settings.WhatsAppDeviceID == "" {
		return apperr.Validation("no device to reconnect")
	}
	if s.whatsapp == nil {
		return apperr.Internal(whatsappNotConfiguredMsg)
	}

	return s.whatsapp.ReconnectDevice(ctx, *settings.WhatsAppDeviceID)
}

func (s *Service) SendWhatsAppTestMessage(ctx context.Context, organizationID uuid.UUID) (string, error) {
	if s.whatsapp == nil {
		return "", apperr.Internal(whatsappNotConfiguredMsg)
	}

	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return "", apperr.NotFound(organizationNotFound)
		}
		return "", err
	}
	phoneNumber := ""
	if org.Phone != nil {
		phoneNumber = strings.TrimSpace(*org.Phone)
	}
	if phoneNumber == "" {
		return "", apperr.Validation("organization phone number is required for WhatsApp test")
	}

	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return "", err
	}
	if settings.WhatsAppDeviceID == nil || strings.TrimSpace(*settings.WhatsAppDeviceID) == "" {
		return "", apperr.Validation("no WhatsApp device linked for this organization")
	}

	message := fmt.Sprintf("Testbericht vanaf Portal op %s", time.Now().Format(time.RFC3339))
	if err := s.whatsapp.SendMessage(ctx, *settings.WhatsAppDeviceID, phoneNumber, message); err != nil {
		return "", apperr.Internal("failed to send WhatsApp test message: " + err.Error())
	}

	return phoneNumber, nil
}

type OrganizationProfileUpdate struct {
	Name         *string
	Email        *string
	Phone        *string
	VATNumber    *string
	KVKNumber    *string
	AddressLine1 *string
	AddressLine2 *string
	PostalCode   *string
	City         *string
	Country      *string
}

func (s *Service) UpdateOrganizationProfile(
	ctx context.Context,
	organizationID uuid.UUID,
	update OrganizationProfileUpdate,
) (repository.Organization, error) {
	update = normalizeOrganizationProfileUpdate(update)

	if update.Name != nil && *update.Name == "" {
		return repository.Organization{}, apperr.Validation("organization name is required")
	}
	if update.VATNumber != nil && !isValidNLVAT(*update.VATNumber) {
		return repository.Organization{}, apperr.Validation("invalid VAT number")
	}
	if update.KVKNumber != nil && !isValidKVK(*update.KVKNumber) {
		return repository.Organization{}, apperr.Validation("invalid KVK number")
	}

	org, err := s.repo.UpdateOrganizationProfile(
		ctx,
		organizationID,
		repository.OrganizationProfileUpdate{
			Name:         update.Name,
			Email:        update.Email,
			Phone:        update.Phone,
			VatNumber:    update.VATNumber,
			KvkNumber:    update.KVKNumber,
			AddressLine1: update.AddressLine1,
			AddressLine2: update.AddressLine2,
			PostalCode:   update.PostalCode,
			City:         update.City,
			Country:      update.Country,
		},
	)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound(organizationNotFound)
		}
		return repository.Organization{}, err
	}

	return org, nil
}

func normalizeOrganizationProfileUpdate(update OrganizationProfileUpdate) OrganizationProfileUpdate {
	update.Name = normalizeOptional(update.Name)
	update.Email = normalizeOptional(update.Email)
	update.Phone = normalizeOptional(update.Phone)
	update.VATNumber = normalizeOptional(update.VATNumber)
	update.KVKNumber = normalizeOptional(update.KVKNumber)
	update.AddressLine1 = normalizeOptional(update.AddressLine1)
	update.AddressLine2 = normalizeOptional(update.AddressLine2)
	update.PostalCode = normalizeOptional(update.PostalCode)
	update.City = normalizeOptional(update.City)
	update.Country = normalizeOptional(update.Country)
	return update
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

var nlVATPattern = regexp.MustCompile(`^NL[0-9]{9}B[0-9]{2}$`)
var kvkPattern = regexp.MustCompile(`^[0-9]{8}$`)

func isValidNLVAT(value string) bool {
	return nlVATPattern.MatchString(strings.ToUpper(strings.TrimSpace(value)))
}

func isValidKVK(value string) bool {
	return kvkPattern.MatchString(strings.TrimSpace(value))
}

func buildEnabledWorkflowMap(workflows []repository.Workflow) map[uuid.UUID]*repository.Workflow {
	result := make(map[uuid.UUID]*repository.Workflow, len(workflows))
	for i := range workflows {
		workflow := &workflows[i]
		if !workflow.Enabled {
			continue
		}
		result[workflow.ID] = workflow
	}
	return result
}

func resolveWorkflowFromOverride(
	override repository.LeadWorkflowOverride,
	workflowMap map[uuid.UUID]*repository.Workflow,
) (ResolveLeadWorkflowResult, bool) {
	mode := strings.TrimSpace(strings.ToLower(override.OverrideMode))
	if mode == "clear" {
		return ResolveLeadWorkflowResult{
			ResolutionSource: "manual_clear",
			OverrideMode:     &override.OverrideMode,
		}, true
	}

	if override.WorkflowID == nil {
		return ResolveLeadWorkflowResult{}, false
	}

	workflow, ok := workflowMap[*override.WorkflowID]
	if !ok {
		return ResolveLeadWorkflowResult{}, false
	}

	return ResolveLeadWorkflowResult{
		Workflow:         workflow,
		ResolutionSource: "manual_override",
		OverrideMode:     &override.OverrideMode,
	}, true
}

func matchesAssignmentRule(rule repository.WorkflowAssignmentRule, input ResolveLeadWorkflowInput) bool {
	if !matchesOptionalField(rule.LeadSource, input.LeadSource) {
		return false
	}
	if !matchesOptionalField(rule.LeadServiceType, input.LeadServiceType) {
		return false
	}
	if !matchesOptionalField(rule.PipelineStage, input.PipelineStage) {
		return false
	}
	return true
}

func matchesOptionalField(ruleValue *string, actualValue *string) bool {
	if ruleValue == nil {
		return true
	}
	ruleText := strings.TrimSpace(*ruleValue)
	if ruleText == "" {
		return true
	}
	if actualValue == nil {
		return false
	}
	actualText := strings.TrimSpace(*actualValue)
	if actualText == "" {
		return false
	}
	return strings.EqualFold(ruleText, actualText)
}

func (s *Service) ResolveInvite(ctx context.Context, rawToken string) (repository.Invite, error) {
	tokenHash := token.HashSHA256(rawToken)
	invite, err := s.repo.GetInviteByToken(ctx, tokenHash)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Invite{}, apperr.NotFound(inviteNotFound)
		}
		return repository.Invite{}, err
	}

	if invite.UsedAt != nil {
		return repository.Invite{}, apperr.Conflict("invite already used")
	}

	if time.Now().After(invite.ExpiresAt) {
		return repository.Invite{}, apperr.Forbidden("invite expired")
	}

	return invite, nil
}

func (s *Service) UseInvite(ctx context.Context, q repository.DBTX, inviteID, userID uuid.UUID) error {
	return s.repo.UseInvite(ctx, q, inviteID, userID)
}

func (s *Service) ListInvites(ctx context.Context, organizationID uuid.UUID) ([]repository.Invite, error) {
	return s.repo.ListInvites(ctx, organizationID)
}

func (s *Service) UpdateInvite(
	ctx context.Context,
	organizationID uuid.UUID,
	inviteID uuid.UUID,
	email *string,
	resend bool,
) (repository.Invite, *string, error) {
	email = normalizeOptional(email)

	if email == nil && !resend {
		return repository.Invite{}, nil, apperr.Validation("no updates provided")
	}

	resendData, err := buildInviteResendData(resend)
	if err != nil {
		return repository.Invite{}, nil, err
	}

	invite, err := s.repo.UpdateInvite(ctx, organizationID, inviteID, email, resendData.tokenHash, resendData.expiresAt)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Invite{}, nil, apperr.NotFound(inviteNotFound)
		}
		return repository.Invite{}, nil, err
	}

	// Publish event to send invite email when resending
	if resend && resendData.tokenValue != nil && s.eventBus != nil {
		s.publishInviteResend(ctx, organizationID, invite.Email, *resendData.tokenValue)
	}

	return invite, resendData.tokenValue, nil
}

type inviteResendData struct {
	tokenValue *string
	tokenHash  *string
	expiresAt  *time.Time
}

func buildInviteResendData(resend bool) (inviteResendData, error) {
	if !resend {
		return inviteResendData{}, nil
	}

	rawToken, err := token.GenerateRandomToken(inviteTokenBytes)
	if err != nil {
		return inviteResendData{}, err
	}

	hash := token.HashSHA256(rawToken)
	value := rawToken
	freshExpires := time.Now().Add(inviteTTL)

	return inviteResendData{
		tokenValue: &value,
		tokenHash:  &hash,
		expiresAt:  &freshExpires,
	}, nil
}

func (s *Service) publishInviteResend(
	ctx context.Context,
	organizationID uuid.UUID,
	email string,
	tokenValue string,
) {
	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err != nil {
		return
	}

	s.eventBus.Publish(ctx, events.OrganizationInviteCreated{
		BaseEvent:        events.NewBaseEvent(),
		OrganizationID:   organizationID,
		OrganizationName: org.Name,
		Email:            email,
		InviteToken:      tokenValue,
	})
}

func (s *Service) RevokeInvite(ctx context.Context, organizationID, inviteID uuid.UUID) (repository.Invite, error) {
	invite, err := s.repo.RevokeInvite(ctx, organizationID, inviteID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Invite{}, apperr.NotFound(inviteNotFound)
		}
		return repository.Invite{}, err
	}

	return invite, nil
}

// PresignLogoUpload generates a presigned URL for uploading an organization logo.
func (s *Service) PresignLogoUpload(ctx context.Context, organizationID uuid.UUID, req transport.OrgLogoPresignRequest) (transport.OrgLogoPresignResponse, error) {
	if !storage.IsImageContentType(req.ContentType) {
		return transport.OrgLogoPresignResponse{}, apperr.Validation("logo must be an image")
	}

	presigned, err := s.storage.GenerateUploadURL(
		ctx,
		s.logoBucket,
		logoFolder(organizationID),
		req.FileName,
		req.ContentType,
		req.SizeBytes,
	)
	if err != nil {
		return transport.OrgLogoPresignResponse{}, err
	}

	return transport.OrgLogoPresignResponse{
		UploadURL: presigned.URL,
		FileKey:   presigned.FileKey,
		ExpiresAt: presigned.ExpiresAt.Unix(),
	}, nil
}

// SetLogo stores logo metadata after the client has uploaded the file to MinIO.
func (s *Service) SetLogo(ctx context.Context, organizationID uuid.UUID, req transport.SetOrgLogoRequest) (repository.Organization, error) {
	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound(organizationNotFound)
		}
		return repository.Organization{}, err
	}

	if !storage.IsImageContentType(req.ContentType) {
		return repository.Organization{}, apperr.Validation("logo must be an image")
	}
	if err := s.storage.ValidateContentType(req.ContentType); err != nil {
		return repository.Organization{}, err
	}
	if err := s.storage.ValidateFileSize(req.SizeBytes); err != nil {
		return repository.Organization{}, err
	}
	if !strings.HasPrefix(req.FileKey, logoFolder(organizationID)+"/") {
		return repository.Organization{}, apperr.Validation("invalid logo file key")
	}

	// Delete old logo if it was a different file
	if org.LogoFileKey != nil && *org.LogoFileKey != req.FileKey {
		_ = s.storage.DeleteObject(ctx, s.logoBucket, *org.LogoFileKey)
	}

	updated, err := s.repo.UpdateOrganizationLogo(ctx, organizationID, repository.OrganizationLogo{
		FileKey:     req.FileKey,
		FileName:    req.FileName,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
	})
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound(organizationNotFound)
		}
		return repository.Organization{}, err
	}

	return updated, nil
}

// GetLogoDownloadURL generates a presigned download URL for the organization logo.
func (s *Service) GetLogoDownloadURL(ctx context.Context, organizationID uuid.UUID) (transport.OrgLogoDownloadResponse, error) {
	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return transport.OrgLogoDownloadResponse{}, apperr.NotFound(organizationNotFound)
		}
		return transport.OrgLogoDownloadResponse{}, err
	}

	if org.LogoFileKey == nil || *org.LogoFileKey == "" {
		return transport.OrgLogoDownloadResponse{}, apperr.NotFound("logo not found")
	}

	presigned, err := s.storage.GenerateDownloadURL(ctx, s.logoBucket, *org.LogoFileKey)
	if err != nil {
		return transport.OrgLogoDownloadResponse{}, err
	}

	return transport.OrgLogoDownloadResponse{
		DownloadURL: presigned.URL,
		ExpiresAt:   presigned.ExpiresAt.Unix(),
	}, nil
}

// DeleteLogo removes the organization logo from storage and clears the metadata.
func (s *Service) DeleteLogo(ctx context.Context, organizationID uuid.UUID) (repository.Organization, error) {
	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound(organizationNotFound)
		}
		return repository.Organization{}, err
	}

	if org.LogoFileKey != nil && *org.LogoFileKey != "" {
		_ = s.storage.DeleteObject(ctx, s.logoBucket, *org.LogoFileKey)
	}

	updated, err := s.repo.ClearOrganizationLogo(ctx, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound(organizationNotFound)
		}
		return repository.Organization{}, err
	}

	return updated, nil
}

func logoFolder(organizationID uuid.UUID) string {
	return "organizations/" + organizationID.String()
}

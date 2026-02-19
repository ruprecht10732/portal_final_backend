package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

const googleLeadSource = "google-leads"

// LeadCreator is the interface for creating leads. Satisfied by management.Service.
type LeadCreator interface {
	Create(ctx context.Context, req transport.CreateLeadRequest, tenantID uuid.UUID) (transport.LeadResponse, error)
}

// FormSubmission represents an inbound form submission via the webhook.
type FormSubmission struct {
	Fields       map[string]string // all form fields as key-value
	Files        []FormFile        // uploaded files
	SourceDomain string            // origin domain of the form
	APIKeyID     uuid.UUID         // the API key that authenticated this request
}

// FormFile represents an uploaded file within a form submission.
type FormFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Size        int64
	Reader      io.Reader
}

// FormSubmissionResponse is returned to the caller on success.
type FormSubmissionResponse struct {
	LeadID       uuid.UUID         `json:"leadId"`
	IsIncomplete bool              `json:"isIncomplete"`
	Extracted    map[string]string `json:"extractedFields"`
	Message      string            `json:"message"`
}

// Service handles inbound form submissions and API key management.
type Service struct {
	repo          *Repository
	leadCreator   LeadCreator
	storageSvc    storage.StorageService
	storageBucket string
	eventBus      events.Bus
	log           *logger.Logger
}

// NewService creates a new webhook service.
func NewService(repo *Repository, leadCreator LeadCreator, storageSvc storage.StorageService, storageBucket string, eventBus events.Bus, log *logger.Logger) *Service {
	return &Service{
		repo:          repo,
		leadCreator:   leadCreator,
		storageSvc:    storageSvc,
		storageBucket: storageBucket,
		eventBus:      eventBus,
		log:           log,
	}
}

// ProcessFormSubmission handles an inbound form submission: extract fields, create lead, upload files, store raw data.
func (s *Service) ProcessFormSubmission(ctx context.Context, sub FormSubmission, orgID uuid.UUID) (FormSubmissionResponse, error) {
	extracted := ExtractFields(sub.Fields)
	isIncomplete := extracted.IsIncomplete()
	requestedServiceType := strings.TrimSpace(extracted.ServiceType)

	// 1. Check for recent duplicate
	dupID, err := s.repo.FindRecentDuplicateLead(ctx, orgID, extracted.Email, extracted.Phone, 60*time.Second)
	if err != nil {
		s.log.Error("webhook: failed to check for duplicate lead", "error", err, "domain", sub.SourceDomain)
		// Continue anyway, better to have a duplicate than lose a lead
	} else if dupID != nil {
		s.log.Info("webhook: duplicate lead detected, skipping creation", "leadId", *dupID, "domain", sub.SourceDomain)

		extractedMap := buildExtractedMap(extracted)
		return FormSubmissionResponse{
			LeadID:       *dupID,
			IsIncomplete: isIncomplete,
			Extracted:    extractedMap,
			Message:      "Duplicate lead ignored",
		}, nil
	}

	createReq := buildCreateLeadRequest(extracted, sub.SourceDomain)
	applyLeadPlaceholders(&createReq)

	leadResp, err := s.leadCreator.Create(ctx, createReq, orgID)
	if err != nil {
		s.log.Error("webhook: failed to create lead from form submission", "error", err, "domain", sub.SourceDomain)
		return FormSubmissionResponse{}, err
	}

	// 4. Store the raw form data + webhook metadata on the lead
	rawData, _ := json.Marshal(sub.Fields)
	if err := s.repo.UpdateWebhookLeadData(ctx, leadResp.ID, orgID, rawData, sub.SourceDomain, isIncomplete); err != nil {
		s.log.Error("webhook: failed to store raw form data", "error", err, "leadId", leadResp.ID)
		// Non-fatal: don't fail the request
	}

	// 5. Upload files as lead service attachments
	if len(sub.Files) > 0 && len(leadResp.Services) > 0 {
		serviceID := leadResp.Services[0].ID
		s.uploadFiles(ctx, leadResp.ID, serviceID, orgID, sub.Files)
	}

	s.recordServiceTypeFallbackEvent(ctx, leadResp, orgID, requestedServiceType, string(createReq.ServiceType), sub.SourceDomain)

	// 6. Publish event
	s.eventBus.Publish(ctx, events.WebhookLeadCreated{
		BaseEvent:    events.NewBaseEvent(),
		LeadID:       leadResp.ID,
		TenantID:     orgID,
		SourceDomain: sub.SourceDomain,
		IsIncomplete: isIncomplete,
	})

	extractedMap := buildExtractedMap(extracted)
	msg := buildWebhookMessage(isIncomplete)

	return FormSubmissionResponse{
		LeadID:       leadResp.ID,
		IsIncomplete: isIncomplete,
		Extracted:    extractedMap,
		Message:      msg,
	}, nil
}

func (s *Service) recordServiceTypeFallbackEvent(ctx context.Context, leadResp transport.LeadResponse, orgID uuid.UUID, requestedServiceType string, normalizedRequestedType string, sourceDomain string) {
	appliedServiceType, serviceID := resolveAppliedServiceType(leadResp)
	if requestedServiceType != "" && strings.EqualFold(appliedServiceType, normalizedRequestedType) {
		return
	}

	summary := "Intake had geen of onbekend diensttype; standaard diensttype toegepast"
	err := s.repo.CreateTimelineEvent(ctx, createTimelineEventParams{
		LeadID:         leadResp.ID,
		ServiceID:      serviceID,
		OrganizationID: orgID,
		ActorType:      "AI",
		ActorName:      "Webhook",
		EventType:      "service_type_change",
		Title:          "Diensttype automatisch gekozen",
		Summary:        &summary,
		Metadata: map[string]any{
			"requestedServiceType":  requestedServiceType,
			"normalizedServiceType": normalizedRequestedType,
			"appliedServiceType":    appliedServiceType,
			"source":                sourceDomain,
			"autoFallback":          true,
		},
	})
	if err != nil {
		s.log.Error("webhook: failed to record service type fallback timeline event", "error", err, "leadId", leadResp.ID)
	}
}

func resolveAppliedServiceType(leadResp transport.LeadResponse) (string, *uuid.UUID) {
	if leadResp.CurrentService != nil {
		id := leadResp.CurrentService.ID
		return string(leadResp.CurrentService.ServiceType), &id
	}
	if len(leadResp.Services) > 0 {
		id := leadResp.Services[0].ID
		return string(leadResp.Services[0].ServiceType), &id
	}
	return "", nil
}

// GoogleLeadResult describes the result of processing a Google Lead Form webhook.
type GoogleLeadResult struct {
	LeadID      *uuid.UUID
	IsTest      bool
	IsDuplicate bool
}

// ProcessGoogleLeadWebhook validates and processes a Google Lead Form webhook payload.
func (s *Service) ProcessGoogleLeadWebhook(ctx context.Context, payload GoogleLeadPayload, config GoogleWebhookConfig) (GoogleLeadResult, error) {
	orgID := config.OrganizationID
	if payload.LeadID == "" {
		return GoogleLeadResult{}, errors.New("missing lead_id")
	}

	if payload.IsTest {
		_ = s.repo.StoreGoogleLeadID(ctx, payload.LeadID, orgID, nil, true)
		return GoogleLeadResult{IsTest: true}, nil
	}

	exists, err := s.repo.CheckGoogleLeadIDExists(ctx, payload.LeadID)
	if err != nil {
		return GoogleLeadResult{}, err
	}
	if exists {
		return GoogleLeadResult{IsDuplicate: true}, nil
	}

	fields := ExtractGoogleLeadFields(payload)
	serviceType := FindServiceTypeForCampaign(config, payload.CampaignID)
	if serviceType != "" {
		fields["serviceType"] = serviceType
	}

	extracted := ExtractFields(fields)
	if serviceType != "" {
		extracted.ServiceType = serviceType
	}

	createReq := buildCreateLeadRequest(extracted, googleLeadSource)
	applyLeadPlaceholders(&createReq)

	leadResp, err := s.leadCreator.Create(ctx, createReq, orgID)
	if err != nil {
		s.log.Error("webhook: failed to create lead from Google webhook", "error", err, "leadId", payload.LeadID)
		return GoogleLeadResult{}, err
	}

	// Store raw payload on lead for auditing
	if rawData, err := json.Marshal(payload); err == nil {
		_ = s.repo.UpdateWebhookLeadData(ctx, leadResp.ID, orgID, rawData, googleLeadSource, extracted.IsIncomplete())
	}

	// Store Google metadata and dedupe marker
	_ = s.repo.UpdateGoogleLeadMetadata(ctx, leadResp.ID, payload.CampaignID, payload.CreativeID, payload.AdGroupID, payload.FormID)
	_ = s.repo.StoreGoogleLeadID(ctx, payload.LeadID, orgID, &leadResp.ID, false)

	s.eventBus.Publish(ctx, events.WebhookLeadCreated{
		BaseEvent:    events.NewBaseEvent(),
		LeadID:       leadResp.ID,
		TenantID:     orgID,
		SourceDomain: googleLeadSource,
		IsIncomplete: extracted.IsIncomplete(),
	})

	return GoogleLeadResult{LeadID: &leadResp.ID}, nil
}

func buildCreateLeadRequest(extracted ExtractedFields, sourceDomain string) transport.CreateLeadRequest {
	return transport.CreateLeadRequest{
		FirstName:     normalizeName(extracted.FirstName),
		LastName:      normalizeName(extracted.LastName),
		Phone:         extracted.Phone,
		Email:         extracted.Email,
		ConsumerRole:  transport.ConsumerRoleOwner,
		Street:        extracted.Street,
		HouseNumber:   extracted.HouseNumber,
		ZipCode:       extracted.ZipCode,
		City:          extracted.City,
		ServiceType:   transport.ServiceType(resolveServiceType(extracted.ServiceType)),
		ConsumerNote:  extracted.Message,
		Source:        "webhook:" + sourceDomain,
		GCLID:         extracted.GCLID,
		UTMSource:     extracted.UTMSource,
		UTMMedium:     extracted.UTMMedium,
		UTMCampaign:   extracted.UTMCampaign,
		UTMContent:    extracted.UTMContent,
		UTMTerm:       extracted.UTMTerm,
		AdLandingPage: extracted.AdLandingPage,
		ReferrerURL:   extracted.ReferrerURL,
	}
}

func normalizeName(value string) string {
	if value != "" {
		return value
	}
	return "Onbekend"
}

func resolveServiceType(value string) string {
	if value != "" {
		return value
	}
	return "Algemeen"
}

func applyLeadPlaceholders(createReq *transport.CreateLeadRequest) {
	if createReq.Phone == "" {
		createReq.Phone = "+31000000000"
	}
	if createReq.Street == "" {
		createReq.Street = "Onbekend"
	}
	if createReq.HouseNumber == "" {
		createReq.HouseNumber = "-"
	}
	if createReq.ZipCode == "" {
		createReq.ZipCode = "0000AA"
	}
	if createReq.City == "" {
		createReq.City = "Onbekend"
	}
}

func buildExtractedMap(extracted ExtractedFields) map[string]string {
	result := map[string]string{}
	if extracted.FirstName != "" {
		result["firstName"] = extracted.FirstName
	}
	if extracted.LastName != "" {
		result["lastName"] = extracted.LastName
	}
	if extracted.Email != "" {
		result["email"] = extracted.Email
	}
	if extracted.Phone != "" {
		result["phone"] = extracted.Phone
	}
	if extracted.Street != "" {
		result["street"] = extracted.Street
	}
	if extracted.HouseNumber != "" {
		result["houseNumber"] = extracted.HouseNumber
	}
	if extracted.ZipCode != "" {
		result["zipCode"] = extracted.ZipCode
	}
	if extracted.City != "" {
		result["city"] = extracted.City
	}
	if extracted.ServiceType != "" {
		result["serviceType"] = extracted.ServiceType
	}
	return result
}

func buildWebhookMessage(isIncomplete bool) string {
	if isIncomplete {
		return "Lead created with incomplete data — manual review recommended"
	}
	return "Lead created successfully"
}

func (s *Service) uploadFiles(ctx context.Context, leadID, serviceID, orgID uuid.UUID, files []FormFile) {
	folder := strings.Join([]string{orgID.String(), leadID.String(), serviceID.String()}, "/")
	for _, f := range files {
		fileKey, err := s.storageSvc.UploadFile(ctx, s.storageBucket, folder, f.FileName, f.ContentType, f.Reader, f.Size)
		if err != nil {
			s.log.Error("webhook: failed to upload file",
				"error", err,
				"leadId", leadID,
				"fileName", f.FileName,
			)
			continue
		}

		// Publish attachment uploaded event so existing photo analysis pipelines kick in
		s.eventBus.Publish(ctx, events.AttachmentUploaded{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      orgID,
			AttachmentID:  uuid.New(),
			FileName:      f.FileName,
			FileKey:       fileKey,
			ContentType:   f.ContentType,
			SizeBytes:     f.Size,
		})

		s.log.Info("webhook: uploaded file", "leadId", leadID, "fileKey", fileKey)
	}
}

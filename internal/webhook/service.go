package webhook

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

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
	// 1. Extract recognized fields from the raw form data
	extracted := ExtractFields(sub.Fields)
	isIncomplete := extracted.IsIncomplete()

	// 2. Build the lead creation request with best-effort values
	firstName := extracted.FirstName
	if firstName == "" {
		firstName = "Onbekend"
	}
	lastName := extracted.LastName
	if lastName == "" {
		lastName = "Onbekend"
	}

	// Determine service type: use extracted value or fall back to "Algemeen"
	serviceType := extracted.ServiceType
	if serviceType == "" {
		serviceType = "Algemeen"
	}

	createReq := transport.CreateLeadRequest{
		FirstName:    firstName,
		LastName:     lastName,
		Phone:        extracted.Phone,
		Email:        extracted.Email,
		ConsumerRole: transport.ConsumerRoleOwner, // default
		Street:       extracted.Street,
		HouseNumber:  extracted.HouseNumber,
		ZipCode:      extracted.ZipCode,
		City:         extracted.City,
		ServiceType:  transport.ServiceType(serviceType),
		ConsumerNote: extracted.Message,
		Source:       "webhook:" + sub.SourceDomain,
	}

	// If phone is empty, set a placeholder so validation doesn't fail
	if createReq.Phone == "" {
		createReq.Phone = "+31000000000"
	}
	// If address fields are empty, set placeholders
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

	// 3. Create the lead via the existing lead management service
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

	// 6. Publish event
	s.eventBus.Publish(ctx, events.WebhookLeadCreated{
		BaseEvent:    events.NewBaseEvent(),
		LeadID:       leadResp.ID,
		TenantID:     orgID,
		SourceDomain: sub.SourceDomain,
		IsIncomplete: isIncomplete,
	})

	// 7. Build extracted fields map for response
	extractedMap := map[string]string{}
	if extracted.FirstName != "" {
		extractedMap["firstName"] = extracted.FirstName
	}
	if extracted.LastName != "" {
		extractedMap["lastName"] = extracted.LastName
	}
	if extracted.Email != "" {
		extractedMap["email"] = extracted.Email
	}
	if extracted.Phone != "" {
		extractedMap["phone"] = extracted.Phone
	}
	if extracted.Street != "" {
		extractedMap["street"] = extracted.Street
	}
	if extracted.HouseNumber != "" {
		extractedMap["houseNumber"] = extracted.HouseNumber
	}
	if extracted.ZipCode != "" {
		extractedMap["zipCode"] = extracted.ZipCode
	}
	if extracted.City != "" {
		extractedMap["city"] = extracted.City
	}
	if extracted.ServiceType != "" {
		extractedMap["serviceType"] = extracted.ServiceType
	}

	msg := "Lead created successfully"
	if isIncomplete {
		msg = "Lead created with incomplete data — manual review recommended"
	}

	return FormSubmissionResponse{
		LeadID:       leadResp.ID,
		IsIncomplete: isIncomplete,
		Extracted:    extractedMap,
		Message:      msg,
	}, nil
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

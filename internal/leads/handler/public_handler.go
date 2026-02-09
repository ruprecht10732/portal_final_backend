package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PublicHandler handles public (unauthenticated) lead portal endpoints.
type PublicHandler struct {
	repo        repository.LeadsRepository
	eventBus    events.Bus
	storage     storage.StorageService
	bucket      string
	val         *validator.Validator
	quoteViewer ports.QuotePublicViewer
	apptViewer  ports.AppointmentPublicViewer
}

const (
	publicMsgInvalidInput       = "Invalid input"
	publicMsgInvalidRequest     = "Invalid request"
	publicMsgLeadNotFound       = "Lead not found"
	publicMsgServiceUnavailable = "Service data unavailable"
)

// NewPublicHandler creates a new public handler for lead portal access.
func NewPublicHandler(repo repository.LeadsRepository, eventBus events.Bus, storageSvc storage.StorageService, bucket string, val *validator.Validator) *PublicHandler {
	return &PublicHandler{repo: repo, eventBus: eventBus, storage: storageSvc, bucket: bucket, val: val}
}

// SetPublicViewers injects external data viewers (quotes and appointments).
func (h *PublicHandler) SetPublicViewers(quoteViewer ports.QuotePublicViewer, apptViewer ports.AppointmentPublicViewer) {
	h.quoteViewer = quoteViewer
	h.apptViewer = apptViewer
}

// RegisterRoutes registers public lead portal routes under /public/leads.
func (h *PublicHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:token", h.GetTrackAndTrace)
	rg.POST("/:token/preferences", h.UpdatePreferences)
	rg.POST("/:token/info", h.AddCustomerInfo)
	rg.POST("/:token/attachments/presign", h.PresignUpload)
	rg.POST("/:token/attachments", h.ConfirmUpload)
	rg.DELETE("/:token/attachments/:attachmentId", h.DeleteAttachment)
}

type PublicPreferencesRequest struct {
	Budget       string `json:"budget" validate:"omitempty,max=200"`
	Timeframe    string `json:"timeframe" validate:"omitempty,max=200"`
	Availability string `json:"availability" validate:"omitempty,max=2000"`
	ExtraNotes   string `json:"extraNotes" validate:"omitempty,max=2000"`
}

// GetTrackAndTrace returns the public portal data for a lead based on a token.
func (h *PublicHandler) GetTrackAndTrace(c *gin.Context) {
	token := c.Param("token")
	lead, err := h.repo.GetByPublicToken(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, "Link expired or invalid", nil)
		return
	}

	svc, err := h.repo.GetCurrentLeadService(c.Request.Context(), lead.ID, lead.OrganizationID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, publicMsgServiceUnavailable, nil)
		return
	}

	var quote *ports.PublicQuoteSummary
	if h.quoteViewer != nil {
		quote, _ = h.quoteViewer.GetActiveQuote(c.Request.Context(), lead.ID, lead.OrganizationID)
	}

	var appt *ports.PublicAppointmentSummary
	if h.apptViewer != nil {
		appt, _ = h.apptViewer.GetUpcomingVisit(c.Request.Context(), lead.ID, lead.OrganizationID)
	}

	statusLabel, statusDescription, step := resolveCustomerStatus(svc.PipelineStage, quote, appt)

	prefs := svc.CustomerPreferences
	if len(prefs) == 0 {
		prefs = json.RawMessage(`{}`)
	}

	var quoteLink string
	var downloadLink string
	if quote != nil {
		quoteLink = fmt.Sprintf("/quote/%s", quote.PublicToken)
		if quote.Status == "Accepted" && quote.PublicToken != "" {
			downloadLink = fmt.Sprintf("/api/v1/public/quotes/%s/pdf", quote.PublicToken)
		}
	}

	attachments, err := h.repo.ListAttachmentsByService(c.Request.Context(), svc.ID, lead.OrganizationID)
	if err != nil {
		attachments = nil
	}

	attachmentItems := make([]transport.AttachmentResponse, 0, len(attachments))
	for _, att := range attachments {
		var downloadURL *string
		if presigned, err := h.storage.GenerateDownloadURL(c.Request.Context(), h.bucket, att.FileKey); err == nil {
			url := presigned.URL
			downloadURL = &url
		}
		attachmentItems = append(attachmentItems, toAttachmentResponse(att, downloadURL))
	}

	response := gin.H{
		"consumerName": fmt.Sprintf("%s %s", lead.ConsumerFirstName, lead.ConsumerLastName),
		"city":         lead.AddressCity,
		"serviceType":  svc.ServiceType,
		"createdAt":    lead.CreatedAt,
		"preferences":  prefs,
		"status": gin.H{
			"label":       statusLabel,
			"description": statusDescription,
			"step":        step,
		},
		"appointment": appt,
		"quote": gin.H{
			"available":    quote != nil,
			"status":       statusStatus(quote),
			"link":         quoteLink,
			"downloadLink": downloadLink,
		},
		"attachments": attachmentItems,
	}

	httpkit.OK(c, response)
}

// UpdatePreferences stores lead preferences and triggers AI refresh.
func (h *PublicHandler) UpdatePreferences(c *gin.Context) {
	token := c.Param("token")
	var req PublicPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidInput, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidInput, err.Error())
		return
	}

	lead, err := h.repo.GetByPublicToken(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, publicMsgLeadNotFound, nil)
		return
	}
	svc, err := h.repo.GetCurrentLeadService(c.Request.Context(), lead.ID, lead.OrganizationID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, publicMsgServiceUnavailable, nil)
		return
	}

	prefsJSON, err := json.Marshal(req)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidInput, nil)
		return
	}

	if err := h.repo.UpdateServicePreferences(c.Request.Context(), svc.ID, lead.OrganizationID, prefsJSON); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "Failed to save preferences", nil)
		return
	}

	summary := "Klant heeft voorkeuren bijgewerkt (Budget/Tijd/Beschikbaarheid)"
	_, _ = h.repo.CreateTimelineEvent(c.Request.Context(), repository.CreateTimelineEventParams{
		LeadID:         lead.ID,
		ServiceID:      &svc.ID,
		OrganizationID: lead.OrganizationID,
		ActorType:      "Lead",
		ActorName:      "Klant",
		EventType:      "preferences_updated",
		Title:          "Voorkeuren bijgewerkt",
		Summary:        &summary,
		Metadata: map[string]any{
			"budget":       req.Budget,
			"timeframe":    req.Timeframe,
			"availability": req.Availability,
			"extraNotes":   req.ExtraNotes,
		},
	})

	h.eventBus.Publish(c.Request.Context(), events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        lead.ID,
		LeadServiceID: svc.ID,
		TenantID:      lead.OrganizationID,
		Source:        "customer_preferences",
	})

	httpkit.OK(c, gin.H{"status": "updated"})
}

// AddCustomerInfo allows the lead to add extra context.
func (h *PublicHandler) AddCustomerInfo(c *gin.Context) {
	token := c.Param("token")
	var req struct {
		Text string `json:"text" validate:"required,min=5,max=2000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidInput, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidInput, err.Error())
		return
	}

	lead, err := h.repo.GetByPublicToken(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, publicMsgLeadNotFound, nil)
		return
	}

	svc, err := h.repo.GetCurrentLeadService(c.Request.Context(), lead.ID, lead.OrganizationID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, publicMsgServiceUnavailable, nil)
		return
	}

	summary := fmt.Sprintf("Klant heeft info toegevoegd via portaal: %s", req.Text)
	_, _ = h.repo.CreateTimelineEvent(c.Request.Context(), repository.CreateTimelineEventParams{
		LeadID:         lead.ID,
		ServiceID:      &svc.ID,
		OrganizationID: lead.OrganizationID,
		ActorType:      "Lead",
		ActorName:      "Klant",
		EventType:      "info_added",
		Title:          "Klant update",
		Summary:        &summary,
		Metadata: map[string]any{
			"text": req.Text,
		},
	})

	h.eventBus.Publish(c.Request.Context(), events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        lead.ID,
		LeadServiceID: svc.ID,
		TenantID:      lead.OrganizationID,
		Source:        "customer_portal_update",
	})

	httpkit.OK(c, gin.H{"status": "received"})
}

// PresignUpload handles file upload initialization for the public portal.
func (h *PublicHandler) PresignUpload(c *gin.Context) {
	token := c.Param("token")
	var req transport.PresignedUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidRequest, err.Error())
		return
	}

	lead, err := h.repo.GetByPublicToken(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, publicMsgLeadNotFound, nil)
		return
	}

	svc, err := h.repo.GetCurrentLeadService(c.Request.Context(), lead.ID, lead.OrganizationID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, publicMsgServiceUnavailable, nil)
		return
	}

	if err := h.storage.ValidateContentType(req.ContentType); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "Invalid file type", nil)
		return
	}
	if err := h.storage.ValidateFileSize(req.SizeBytes); err != nil {
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	folder := fmt.Sprintf("%s/%s/%s/customer_uploads", lead.OrganizationID, lead.ID, svc.ID)
	presigned, err := h.storage.GenerateUploadURL(c.Request.Context(), h.bucket, folder, req.FileName, req.ContentType, req.SizeBytes)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "Storage error", nil)
		return
	}

	httpkit.OK(c, transport.PresignedUploadResponse{
		UploadURL: presigned.URL,
		FileKey:   presigned.FileKey,
		ExpiresAt: presigned.ExpiresAt.Unix(),
	})
}

// ConfirmUpload finalizes the upload and triggers AI refresh.
func (h *PublicHandler) ConfirmUpload(c *gin.Context) {
	token := c.Param("token")
	var req transport.CreateAttachmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidRequest, err.Error())
		return
	}

	lead, err := h.repo.GetByPublicToken(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, publicMsgLeadNotFound, nil)
		return
	}

	svc, err := h.repo.GetCurrentLeadService(c.Request.Context(), lead.ID, lead.OrganizationID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, publicMsgServiceUnavailable, nil)
		return
	}

	_, err = h.repo.CreateAttachment(c.Request.Context(), repository.CreateAttachmentParams{
		LeadServiceID:  svc.ID,
		OrganizationID: lead.OrganizationID,
		FileKey:        req.FileKey,
		FileName:       req.FileName,
		ContentType:    req.ContentType,
		SizeBytes:      req.SizeBytes,
		UploadedBy:     nil,
	})
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "Failed to save attachment", nil)
		return
	}

	h.eventBus.Publish(c.Request.Context(), events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        lead.ID,
		LeadServiceID: svc.ID,
		TenantID:      lead.OrganizationID,
		Source:        "customer_portal_upload",
	})

	httpkit.OK(c, gin.H{"status": "ok"})
}

// DeleteAttachment removes a public-uploaded attachment.
func (h *PublicHandler) DeleteAttachment(c *gin.Context) {
	token := c.Param("token")
	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidRequest, nil)
		return
	}

	lead, err := h.repo.GetByPublicToken(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, publicMsgLeadNotFound, nil)
		return
	}

	svc, err := h.repo.GetCurrentLeadService(c.Request.Context(), lead.ID, lead.OrganizationID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, publicMsgServiceUnavailable, nil)
		return
	}

	att, err := h.repo.GetAttachmentByID(c.Request.Context(), attachmentID, lead.OrganizationID)
	if err != nil {
		if errors.Is(err, repository.ErrAttachmentNotFound) {
			httpkit.Error(c, http.StatusNotFound, "Attachment not found", nil)
			return
		}
		httpkit.Error(c, http.StatusInternalServerError, "Failed to load attachment", nil)
		return
	}
	if att.LeadServiceID != svc.ID {
		httpkit.Error(c, http.StatusNotFound, "Attachment not found", nil)
		return
	}

	if err := h.storage.DeleteObject(c.Request.Context(), h.bucket, att.FileKey); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "Failed to delete attachment", nil)
		return
	}
	if err := h.repo.DeleteAttachment(c.Request.Context(), attachmentID, lead.OrganizationID); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "Failed to delete attachment", nil)
		return
	}

	httpkit.OK(c, gin.H{"status": "deleted"})
}

func resolveCustomerStatus(stage string, quote *ports.PublicQuoteSummary, appt *ports.PublicAppointmentSummary) (string, string, int) {
	if quote != nil && quote.Status == "Accepted" {
		if stage == "Completed" {
			return "Afgerond", "De werkzaamheden zijn succesvol afgerond.", 4
		}
		return "In Planning", "Bedankt voor uw akkoord! We bereiden de uitvoering voor.", 3
	}

	if quote != nil && quote.Status == "Sent" {
		return "Offerte Klaar", "Er staat een offerte voor u klaar. Bekijk en onderteken deze digitaal.", 3
	}

	if appt != nil {
		apptDate := appt.StartTime.Format("02-01-2006 om 15:04")
		return "Afspraak Gepland", fmt.Sprintf("We komen langs op %s.", apptDate), 2
	}

	switch stage {
	case "Triage", "Nurturing", "Manual_Intervention":
		return "Aanvraag Ontvangen", "We hebben uw aanvraag ontvangen en controleren de details.", 1
	case "Ready_For_Estimator":
		return "In Beoordeling", "Onze experts maken een inschatting van de situatie.", 2
	case "Ready_For_Partner", "Partner_Matching", "Partner_Assigned":
		return "In Voorbereiding", "We kijken naar de beschikbaarheid in de planning.", 2
	case "Lost":
		return "Gesloten", "Deze aanvraag is gesloten.", 1
	default:
		return "In Behandeling", "We zijn met uw aanvraag bezig.", 1
	}
}

func statusStatus(quote *ports.PublicQuoteSummary) string {
	if quote == nil {
		return ""
	}
	return quote.Status
}

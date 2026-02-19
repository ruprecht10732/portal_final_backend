package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PublicHandler handles public (unauthenticated) lead portal endpoints.
type PublicHandler struct {
	repo        repository.LeadsRepository
	eventBus    events.Bus
	sse         *sse.Service
	storage     storage.StorageService
	bucket      string
	val         *validator.Validator
	quoteViewer ports.QuotePublicViewer
	apptViewer  ports.AppointmentPublicViewer
	slotViewer  ports.AppointmentSlotProvider
	orgViewer   ports.OrganizationPublicViewer
}

const (
	publicMsgInvalidInput       = "Invalid input"
	publicMsgInvalidRequest     = "Invalid request"
	publicMsgLeadNotFound       = "Lead not found"
	publicMsgServiceUnavailable = "Service data unavailable"
)

// NewPublicHandler creates a new public handler for lead portal access.
func NewPublicHandler(repo repository.LeadsRepository, eventBus events.Bus, sseService *sse.Service, storageSvc storage.StorageService, bucket string, val *validator.Validator) *PublicHandler {
	return &PublicHandler{repo: repo, eventBus: eventBus, sse: sseService, storage: storageSvc, bucket: bucket, val: val}
}

// SetPublicViewers injects external data viewers (quotes and appointments).
func (h *PublicHandler) SetPublicViewers(quoteViewer ports.QuotePublicViewer, apptViewer ports.AppointmentPublicViewer, slotViewer ports.AppointmentSlotProvider) {
	h.quoteViewer = quoteViewer
	h.apptViewer = apptViewer
	h.slotViewer = slotViewer
}

// SetPublicOrgViewer injects the organization viewer for the public portal.
func (h *PublicHandler) SetPublicOrgViewer(orgViewer ports.OrganizationPublicViewer) {
	h.orgViewer = orgViewer
}

// RegisterRoutes registers public lead portal routes under /public/leads.
func (h *PublicHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:token", h.GetTrackAndTrace)
	if h.sse != nil {
		rg.GET(":token/events", h.sse.PublicLeadHandler(h.resolveLeadID))
	}
	rg.POST("/:token/preferences", h.UpdatePreferences)
	rg.POST("/:token/info", h.AddCustomerInfo)
	rg.GET("/:token/availability/slots", h.GetAvailabilitySlots)
	rg.POST("/:token/appointments/request", h.RequestAppointment)
	rg.POST("/:token/attachments/presign", h.PresignUpload)
	rg.POST("/:token/attachments", h.ConfirmUpload)
	rg.DELETE("/:token/attachments/:attachmentId", h.DeleteAttachment)
}

func (h *PublicHandler) resolveLeadID(token string) (uuid.UUID, error) {
	ctx := context.Background()
	lead, err := h.repo.GetByPublicToken(ctx, token)
	if err != nil {
		return uuid.UUID{}, err
	}
	return lead.ID, nil
}

type PublicPreferencesRequest struct {
	Budget       string `json:"budget" validate:"omitempty,max=200"`
	Timeframe    string `json:"timeframe" validate:"omitempty,max=200"`
	Availability string `json:"availability" validate:"omitempty,max=2000"`
	ExtraNotes   string `json:"extraNotes" validate:"omitempty,max=2000"`
}

type PublicAvailabilitySlotsQuery struct {
	StartDate    string `form:"startDate" validate:"required"`
	EndDate      string `form:"endDate" validate:"required"`
	SlotDuration int    `form:"slotDuration"`
}

type PublicAppointmentRequest struct {
	UserID    uuid.UUID `json:"userId" validate:"required"`
	StartTime time.Time `json:"startTime" validate:"required"`
	EndTime   time.Time `json:"endTime" validate:"required,gtfield=StartTime"`
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

	appt, pendingAppt := h.resolvePublicAppointments(c.Request.Context(), lead.ID, lead.OrganizationID)
	appointments := h.resolvePublicAppointmentsList(c.Request.Context(), lead.ID, lead.OrganizationID)
	slotsAvailable := h.resolveSlotsAvailable(c.Request.Context(), &lead)

	statusLabel, statusDescription, step := resolveCustomerStatus(svc.PipelineStage, quote, appt)

	prefs := normalizePreferences(svc.CustomerPreferences)
	quoteLink, downloadLink := buildQuoteLinks(quote)

	attachments, err := h.repo.ListAttachmentsByService(c.Request.Context(), svc.ID, lead.OrganizationID)
	if err != nil {
		attachments = nil
	}

	attachmentItems := buildAttachmentItems(c.Request.Context(), h.storage, h.bucket, attachments)

	orgPhone := ""
	if h.orgViewer != nil {
		phone, err := h.orgViewer.GetPublicPhone(c.Request.Context(), lead.OrganizationID)
		if err == nil {
			orgPhone = phone
		}
	}

	response := gin.H{
		"consumerName": strings.TrimSpace(lead.ConsumerFirstName),
		"city":         lead.AddressCity,
		"serviceType":  svc.ServiceType,
		"createdAt":    lead.CreatedAt,
		"preferences":  prefs,
		"status": gin.H{
			"label":       statusLabel,
			"description": statusDescription,
			"step":        step,
		},
		"appointment":        appt,
		"appointmentRequest": pendingAppt,
		"appointments":       appointments,
		"organizationPhone":  orgPhone,
		"slotsAvailable":     slotsAvailable,
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
		ActorType:      repository.ActorTypeLead,
		ActorName:      repository.ActorNameKlant,
		EventType:      repository.EventTypePreferencesUpdated,
		Title:          repository.EventTitlePreferencesUpdated,
		Summary:        &summary,
		Metadata: repository.PreferencesMetadata{
			Budget:       req.Budget,
			Timeframe:    req.Timeframe,
			Availability: req.Availability,
			ExtraNotes:   req.ExtraNotes,
		}.ToMap(),
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
		ActorType:      repository.ActorTypeLead,
		ActorName:      repository.ActorNameKlant,
		EventType:      repository.EventTypeInfoAdded,
		Title:          repository.EventTitleCustomerInfo,
		Summary:        &summary,
		Metadata: repository.CustomerInfoMetadata{
			Text: req.Text,
		}.ToMap(),
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

// GetAvailabilitySlots returns available inspection slots for the organization.
func (h *PublicHandler) GetAvailabilitySlots(c *gin.Context) {
	token := c.Param("token")
	var req PublicAvailabilitySlotsQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, publicMsgInvalidRequest, err.Error())
		return
	}
	if req.SlotDuration == 0 {
		req.SlotDuration = 60
	}

	lead, err := h.repo.GetByPublicToken(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, publicMsgLeadNotFound, nil)
		return
	}

	if h.slotViewer == nil {
		httpkit.OK(c, ports.PublicAvailableSlotsResponse{Days: []ports.PublicDaySlots{}})
		return
	}

	resp, err := h.slotViewer.GetAvailableSlots(c.Request.Context(), lead.OrganizationID, req.StartDate, req.EndDate, req.SlotDuration)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, publicMsgServiceUnavailable, nil)
		return
	}

	httpkit.OK(c, resp)
}

// RequestAppointment creates a requested inspection appointment for the lead.
func (h *PublicHandler) RequestAppointment(c *gin.Context) {
	token := c.Param("token")
	var req PublicAppointmentRequest
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

	if h.slotViewer == nil {
		httpkit.Error(c, http.StatusBadRequest, "Planning niet beschikbaar", nil)
		return
	}

	appointment, err := h.slotViewer.CreateRequestedAppointment(c.Request.Context(), req.UserID, lead.OrganizationID, lead.ID, svc.ID, req.StartTime, req.EndTime)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "Afspraak aanvragen mislukt", nil)
		return
	}

	startLabel := req.StartTime.Format("02-01-2006 om 15:04")
	summary := fmt.Sprintf("Klant heeft een inspectie aangevraagd voor %s", startLabel)
	_, _ = h.repo.CreateTimelineEvent(c.Request.Context(), repository.CreateTimelineEventParams{
		LeadID:         lead.ID,
		ServiceID:      &svc.ID,
		OrganizationID: lead.OrganizationID,
		ActorType:      repository.ActorTypeLead,
		ActorName:      repository.ActorNameKlant,
		EventType:      repository.EventTypeAppointmentRequested,
		Title:          repository.EventTitleAppointmentRequested,
		Summary:        &summary,
		Metadata: repository.AppointmentRequestMetadata{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		}.ToMap(),
	})

	h.eventBus.Publish(c.Request.Context(), events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        lead.ID,
		LeadServiceID: svc.ID,
		TenantID:      lead.OrganizationID,
		Source:        "appointment_request",
	})

	httpkit.OK(c, gin.H{"status": "requested", "appointment": appointment})
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

	att, err := h.repo.CreateAttachment(c.Request.Context(), repository.CreateAttachmentParams{
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

	h.eventBus.Publish(c.Request.Context(), events.AttachmentUploaded{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        lead.ID,
		LeadServiceID: svc.ID,
		TenantID:      lead.OrganizationID,
		AttachmentID:  att.ID,
		FileName:      req.FileName,
		ContentType:   req.ContentType,
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

	h.eventBus.Publish(c.Request.Context(), events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        lead.ID,
		LeadServiceID: svc.ID,
		TenantID:      lead.OrganizationID,
		Source:        "customer_portal_delete",
	})

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
		return "In planning", fmt.Sprintf("We hebben een moment gereserveerd op %s.", apptDate), 2
	}

	switch stage {
	case "Triage", "Nurturing", "Manual_Intervention":
		return "Aanvraag Ontvangen", "We hebben uw aanvraag ontvangen en controleren de details.", 1
	case "Estimation":
		return "In Beoordeling", "Onze experts maken een inschatting van de situatie.", 2
	case "Proposal":
		return "Offerte Verstuurd", "We wachten op uw reactie op de offerte.", 2
	case "Fulfillment":
		return "In planning", "We plannen een moment voor de volgende stap.", 2
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

func normalizePreferences(prefs json.RawMessage) json.RawMessage {
	if len(prefs) == 0 {
		return json.RawMessage(`{}`)
	}
	return prefs
}

func buildQuoteLinks(quote *ports.PublicQuoteSummary) (string, string) {
	if quote == nil {
		return "", ""
	}
	quoteLink := fmt.Sprintf("/quote/%s", quote.PublicToken)
	if quote.Status == "Accepted" && quote.PublicToken != "" {
		return quoteLink, fmt.Sprintf("/api/v1/public/quotes/%s/pdf", quote.PublicToken)
	}
	return quoteLink, ""
}

func buildAttachmentItems(ctx context.Context, storageSvc storage.StorageService, bucket string, attachments []repository.Attachment) []transport.AttachmentResponse {
	items := make([]transport.AttachmentResponse, 0, len(attachments))
	for _, att := range attachments {
		var downloadURL *string
		if presigned, err := storageSvc.GenerateDownloadURL(ctx, bucket, att.FileKey); err == nil {
			url := presigned.URL
			downloadURL = &url
		}
		items = append(items, toAttachmentResponse(att, downloadURL))
	}
	return items
}

func (h *PublicHandler) resolvePublicAppointments(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*ports.PublicAppointmentSummary, *ports.PublicAppointmentSummary) {
	if h.apptViewer == nil {
		return nil, nil
	}
	appt, _ := h.apptViewer.GetUpcomingVisit(ctx, leadID, organizationID)
	pending, _ := h.apptViewer.GetPendingVisit(ctx, leadID, organizationID)
	return appt, pending
}

func (h *PublicHandler) resolvePublicAppointmentsList(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) []ports.PublicAppointmentSummary {
	if h.apptViewer == nil {
		return []ports.PublicAppointmentSummary{}
	}
	items, err := h.apptViewer.ListVisits(ctx, leadID, organizationID)
	if err != nil {
		return []ports.PublicAppointmentSummary{}
	}
	return items
}

func (h *PublicHandler) resolveSlotsAvailable(ctx context.Context, lead *repository.Lead) bool {
	if h.slotViewer == nil {
		return false
	}
	available, _ := h.slotViewer.HasAvailabilityRules(ctx, lead.OrganizationID)
	return available
}

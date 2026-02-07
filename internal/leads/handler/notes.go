package handler

import (
	"net/http"
	"strings"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// NotesHandler handles HTTP requests for lead notes.
// This is separate from the main Handler to allow independent wiring.
type NotesHandler struct {
	svc      *notes.Service
	repo     repository.LeadsRepository
	eventBus events.Bus
	val      *validator.Validator
}

// NewNotesHandler creates a new notes handler.
func NewNotesHandler(svc *notes.Service, repo repository.LeadsRepository, eventBus events.Bus, val *validator.Validator) *NotesHandler {
	return &NotesHandler{svc: svc, repo: repo, eventBus: eventBus, val: val}
}

func (h *NotesHandler) ListNotes(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	notesList, err := h.svc.List(c.Request.Context(), id, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, notesList)
}

func (h *NotesHandler) AddNote(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.CreateLeadNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	created, err := h.svc.Add(c.Request.Context(), id, identity.UserID(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	serviceID, ok := h.getCurrentServiceID(c, id, tenantID)
	if ok {
		_, _ = h.repo.CreateTimelineEvent(c.Request.Context(), repository.CreateTimelineEventParams{
			LeadID:         id,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      "User",
			ActorName:      created.AuthorEmail,
			EventType:      "note",
			Title:          "Note added",
			Summary:        toSummaryPointer(created.Body, 400),
			Metadata: map[string]any{
				"noteId":   created.ID,
				"noteType": created.Type,
			},
		})

		h.eventBus.Publish(c.Request.Context(), events.LeadDataChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        id,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			Source:        "note",
		})
	}

	httpkit.JSON(c, http.StatusCreated, created)
}

func (h *NotesHandler) getCurrentServiceID(c *gin.Context, leadID, tenantID uuid.UUID) (uuid.UUID, bool) {
	svc, err := h.repo.GetCurrentLeadService(c.Request.Context(), leadID, tenantID)
	if err != nil {
		return uuid.UUID{}, false
	}
	return svc.ID, true
}

func toSummaryPointer(text string, maxLen int) *string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > maxLen {
		trimmed = trimmed[:maxLen] + "..."
	}
	return &trimmed
}

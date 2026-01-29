package handler

import (
	"net/http"

	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// NotesHandler handles HTTP requests for lead notes.
// This is separate from the main Handler to allow independent wiring.
type NotesHandler struct {
	svc *notes.Service
}

// NewNotesHandler creates a new notes handler.
func NewNotesHandler(svc *notes.Service) *NotesHandler {
	return &NotesHandler{svc: svc}
}

func (h *NotesHandler) ListNotes(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	notesList, err := h.svc.List(c.Request.Context(), id)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, notesList)
}

func (h *NotesHandler) AddNote(c *gin.Context) {
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
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	actorIDValue, ok := c.Get(httpkit.ContextUserIDKey)
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	authorID := actorIDValue.(uuid.UUID)
	created, err := h.svc.Add(c.Request.Context(), id, authorID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, created)
}

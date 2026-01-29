package handler

import (
	"net/http"

	"portal_final_backend/internal/leads/service"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) ListNotes(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	notes, err := h.svc.ListNotes(c.Request.Context(), id)
	if err != nil {
		if err == service.ErrLeadNotFound {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, notes)
}

func (h *Handler) AddNote(c *gin.Context) {
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
	created, err := h.svc.AddNote(c.Request.Context(), id, authorID, req)
	if err != nil {
		switch err {
		case service.ErrLeadNotFound:
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
		case service.ErrInvalidNote:
			httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		default:
			httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		}
		return
	}

	httpkit.JSON(c, http.StatusCreated, created)
}

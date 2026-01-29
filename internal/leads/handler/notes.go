package handler

import (
	"net/http"

	"portal_final_backend/internal/auth/validator"
	"portal_final_backend/internal/http/middleware"
	"portal_final_backend/internal/http/response"
	"portal_final_backend/internal/leads/service"
	"portal_final_backend/internal/leads/transport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) ListNotes(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	notes, err := h.svc.ListNotes(c.Request.Context(), id)
	if err != nil {
		if err == service.ErrLeadNotFound {
			response.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.OK(c, notes)
}

func (h *Handler) AddNote(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.CreateLeadNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	actorIDValue, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	authorID := actorIDValue.(uuid.UUID)
	created, err := h.svc.AddNote(c.Request.Context(), id, authorID, req)
	if err != nil {
		switch err {
		case service.ErrLeadNotFound:
			response.Error(c, http.StatusNotFound, err.Error(), nil)
		case service.ErrInvalidNote:
			response.Error(c, http.StatusBadRequest, err.Error(), nil)
		default:
			response.Error(c, http.StatusBadRequest, err.Error(), nil)
		}
		return
	}

	response.JSON(c, http.StatusCreated, created)
}

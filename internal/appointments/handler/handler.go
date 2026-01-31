package handler

import (
	"net/http"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

// Handler handles HTTP requests for appointments
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

// New creates a new appointments handler
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// RegisterRoutes registers the appointment routes
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/:id", h.GetByID)
	rg.PUT("/:id", h.Update)
	rg.DELETE("/:id", h.Delete)
	rg.PATCH("/:id/status", h.UpdateStatus)
}

// List handles GET /api/appointments
func (h *Handler) List(c *gin.Context) {
	var req transport.ListAppointmentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.List(c.Request.Context(), identity.UserID(), isAdmin, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Create handles POST /api/appointments
func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateAppointmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	result, err := h.svc.Create(c.Request.Context(), identity.UserID(), req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// GetByID handles GET /api/appointments/:id
func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	result, err := h.svc.GetByID(c.Request.Context(), id)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Update handles PUT /api/appointments/:id
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateAppointmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.Update(c.Request.Context(), id, identity.UserID(), isAdmin, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Delete handles DELETE /api/appointments/:id
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	isAdmin := containsRole(identity.Roles(), "admin")
	if err := h.svc.Delete(c.Request.Context(), id, identity.UserID(), isAdmin); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "appointment deleted"})
}

// UpdateStatus handles PATCH /api/appointments/:id/status
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateAppointmentStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.UpdateStatus(c.Request.Context(), id, identity.UserID(), isAdmin, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

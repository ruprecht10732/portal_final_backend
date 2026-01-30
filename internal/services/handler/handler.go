package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"portal_final_backend/internal/services/service"
	"portal_final_backend/internal/services/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"
)

// Handler handles HTTP requests for service types.
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
	msgInvalidID        = "invalid service type ID"
)

// New creates a new service types handler.
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// List retrieves all service types (admin only).
// GET /api/v1/admin/service-types
func (h *Handler) List(c *gin.Context) {
	result, err := h.svc.List(c.Request.Context())
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// ListActive retrieves only active service types (public).
// GET /api/v1/service-types
func (h *Handler) ListActive(c *gin.Context) {
	result, err := h.svc.ListActive(c.Request.Context())
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// GetByID retrieves a service type by ID.
// GET /api/v1/service-types/:id
func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	result, err := h.svc.GetByID(c.Request.Context(), id)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// GetBySlug retrieves a service type by slug.
// GET /api/v1/service-types/slug/:slug
func (h *Handler) GetBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		httpkit.Error(c, http.StatusBadRequest, "slug is required", nil)
		return
	}

	result, err := h.svc.GetBySlug(c.Request.Context(), slug)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// Create creates a new service type.
// POST /api/v1/admin/service-types
func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateServiceTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.Create(c.Request.Context(), req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.JSON(c, http.StatusCreated, result)
}

// Update updates an existing service type.
// PUT /api/v1/admin/service-types/:id
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.UpdateServiceTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.Update(c.Request.Context(), id, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// Delete removes a service type.
// DELETE /api/v1/admin/service-types/:id
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); httpkit.HandleError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// ToggleActive toggles the is_active flag for a service type.
// PATCH /api/v1/admin/service-types/:id/toggle-active
func (h *Handler) ToggleActive(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	result, err := h.svc.ToggleActive(c.Request.Context(), id)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// Reorder updates the display order of multiple service types.
// PUT /api/v1/admin/service-types/reorder
func (h *Handler) Reorder(c *gin.Context) {
	var req transport.ReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.Reorder(c.Request.Context(), req); httpkit.HandleError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

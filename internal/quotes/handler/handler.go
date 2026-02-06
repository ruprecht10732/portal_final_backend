package handler

import (
	"net/http"

	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

// Handler handles HTTP requests for quotes
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

// New creates a new quotes handler
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// RegisterRoutes registers the quote routes
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.POST("/calculate", h.PreviewCalculation)
	rg.GET("/:id", h.GetByID)
	rg.PUT("/:id", h.Update)
	rg.PATCH("/:id/status", h.UpdateStatus)
	rg.DELETE("/:id", h.Delete)
}

// List handles GET /api/v1/quotes
func (h *Handler) List(c *gin.Context) {
	var req transport.ListQuotesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.List(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Create handles POST /api/v1/quotes
func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)
	result, err := h.svc.Create(c.Request.Context(), tenantID, identity.UserID(), req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// GetByID handles GET /api/v1/quotes/:id
func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.GetByID(c.Request.Context(), id, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Update handles PUT /api/v1/quotes/:id
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.Update(c.Request.Context(), id, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// UpdateStatus handles PATCH /api/v1/quotes/:id/status
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateQuoteStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)
	result, err := h.svc.UpdateStatus(c.Request.Context(), id, tenantID, identity.UserID(), req.Status)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Delete handles DELETE /api/v1/quotes/:id
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id, tenantID); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "quote deleted"})
}

// PreviewCalculation handles POST /api/v1/quotes/calculate
// Returns calculated totals without persisting anything.
func (h *Handler) PreviewCalculation(c *gin.Context) {
	var req transport.QuoteCalculationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result := service.CalculateQuote(req)
	httpkit.OK(c, result)
}

// mustGetTenantID extracts the tenant ID from identity.
func mustGetTenantID(c *gin.Context) (uuid.UUID, bool) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return uuid.UUID{}, false
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant ID is required", nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}

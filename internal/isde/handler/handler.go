package handler

import (
	"net/http"

	"portal_final_backend/internal/isde/service"
	"portal_final_backend/internal/isde/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

// Handler handles HTTP requests for ISDE calculations.
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

// New creates a new ISDE handler.
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// RegisterRoutes registers ISDE routes.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/calculate", h.Calculate)
}

// Calculate handles POST /api/v1/isde/calculate.
func (h *Handler) Calculate(c *gin.Context) {
	var req transport.ISDECalculationRequest
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
	tenantID, ok := httpkit.RequireTenant(c)
	if !ok {
		return
	}

	result, err := h.svc.Calculate(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}


package handler

import (
	"net/http"

	"portal_final_backend/internal/search/service"
	"portal_final_backend/internal/search/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

type Handler struct {
	svc *service.Service
	val *validator.Validator
}

func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.GlobalSearch)
}

func (h *Handler) GlobalSearch(c *gin.Context) {
	var req transport.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
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
	tenantIDPtr := identity.TenantID()
	if tenantIDPtr == nil {
		// MustGetIdentity should already guard this for protected routes.
		httpkit.Error(c, http.StatusForbidden, "organization required", nil)
		return
	}
	tenantID := *tenantIDPtr

	isAdmin := identity.HasRole("admin")
	result, err := h.svc.GlobalSearch(c.Request.Context(), tenantID, req, isAdmin)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

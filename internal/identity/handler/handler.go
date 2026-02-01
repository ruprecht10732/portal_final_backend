package handler

import (
	"net/http"

	"portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/identity/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *service.Service
	val *validator.Validator
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/organizations/me", h.GetOrganization)
	rg.PATCH("/organizations/me", h.UpdateOrganization)
	rg.POST("/organizations/invites", h.CreateInvite)
}

func (h *Handler) CreateInvite(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant not set", nil)
		return
	}

	var req transport.CreateInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	token, expiresAt, err := h.svc.CreateInvite(c.Request.Context(), *tenantID, req.Email, identity.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, transport.CreateInviteResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	})
}

func (h *Handler) GetOrganization(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant not set", nil)
		return
	}

	org, err := h.svc.GetOrganization(c.Request.Context(), *tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.OrganizationResponse{
		ID:   org.ID.String(),
		Name: org.Name,
	})
}

func (h *Handler) UpdateOrganization(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant not set", nil)
		return
	}

	var req transport.UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	org, err := h.svc.UpdateOrganizationName(c.Request.Context(), *tenantID, req.Name)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.OrganizationResponse{
		ID:   org.ID.String(),
		Name: org.Name,
	})
}

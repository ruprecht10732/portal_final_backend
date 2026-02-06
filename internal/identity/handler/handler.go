package handler

import (
	"net/http"

	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/identity/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	svc *service.Service
	val *validator.Validator
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
	msgTenantNotSet     = "tenant not set"
)

func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/organizations/me", h.GetOrganization)
	rg.PATCH("/organizations/me", h.UpdateOrganization)
	rg.POST("/organizations/me/logo/presign", h.PresignLogo)
	rg.POST("/organizations/me/logo", h.SetLogo)
	rg.GET("/organizations/me/logo/download", h.GetLogoDownload)
	rg.DELETE("/organizations/me/logo", h.DeleteLogo)
	rg.POST("/organizations/invites", h.CreateInvite)
	rg.GET("/organizations/invites", h.ListInvites)
	rg.PATCH("/organizations/invites/:inviteID", h.UpdateInvite)
	rg.DELETE("/organizations/invites/:inviteID", h.RevokeInvite)
}

func (h *Handler) CreateInvite(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
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
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	org, err := h.svc.GetOrganization(c.Request.Context(), *tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.OrganizationResponse{
		ID:              org.ID.String(),
		Name:            org.Name,
		Email:           org.Email,
		Phone:           org.Phone,
		VatNumber:       org.VatNumber,
		KvkNumber:       org.KvkNumber,
		AddressLine1:    org.AddressLine1,
		AddressLine2:    org.AddressLine2,
		PostalCode:      org.PostalCode,
		City:            org.City,
		Country:         org.Country,
		LogoFileKey:     org.LogoFileKey,
		LogoFileName:    org.LogoFileName,
		LogoContentType: org.LogoContentType,
		LogoSizeBytes:   org.LogoSizeBytes,
	})
}

func (h *Handler) UpdateOrganization(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
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

	org, err := h.svc.UpdateOrganizationProfile(
		c.Request.Context(),
		*tenantID,
		service.OrganizationProfileUpdate{
			Name:         req.Name,
			Email:        req.Email,
			Phone:        req.Phone,
			VATNumber:    req.VatNumber,
			KVKNumber:    req.KvkNumber,
			AddressLine1: req.AddressLine1,
			AddressLine2: req.AddressLine2,
			PostalCode:   req.PostalCode,
			City:         req.City,
			Country:      req.Country,
		},
	)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.OrganizationResponse{
		ID:              org.ID.String(),
		Name:            org.Name,
		Email:           org.Email,
		Phone:           org.Phone,
		VatNumber:       org.VatNumber,
		KvkNumber:       org.KvkNumber,
		AddressLine1:    org.AddressLine1,
		AddressLine2:    org.AddressLine2,
		PostalCode:      org.PostalCode,
		City:            org.City,
		Country:         org.Country,
		LogoFileKey:     org.LogoFileKey,
		LogoFileName:    org.LogoFileName,
		LogoContentType: org.LogoContentType,
		LogoSizeBytes:   org.LogoSizeBytes,
	})
}

func (h *Handler) ListInvites(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	invites, err := h.svc.ListInvites(c.Request.Context(), *tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	responses := make([]transport.InviteResponse, 0, len(invites))
	for _, invite := range invites {
		responses = append(responses, mapInviteResponse(invite))
	}

	httpkit.OK(c, transport.ListInvitesResponse{Invites: responses})
}

func (h *Handler) UpdateInvite(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	inviteID, err := uuid.Parse(c.Param("inviteID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	invite, tokenValue, err := h.svc.UpdateInvite(c.Request.Context(), *tenantID, inviteID, req.Email, req.Resend)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.UpdateInviteResponse{
		Invite: mapInviteResponse(invite),
		Token:  tokenValue,
	})
}

func (h *Handler) RevokeInvite(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	inviteID, err := uuid.Parse(c.Param("inviteID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	invite, err := h.svc.RevokeInvite(c.Request.Context(), *tenantID, inviteID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, mapInviteResponse(invite))
}

func mapInviteResponse(invite repository.Invite) transport.InviteResponse {
	return transport.InviteResponse{
		ID:        invite.ID.String(),
		Email:     invite.Email,
		ExpiresAt: invite.ExpiresAt,
		CreatedAt: invite.CreatedAt,
		UsedAt:    invite.UsedAt,
	}
}

func mapOrgResponse(org repository.Organization) transport.OrganizationResponse {
	return transport.OrganizationResponse{
		ID:              org.ID.String(),
		Name:            org.Name,
		Email:           org.Email,
		Phone:           org.Phone,
		VatNumber:       org.VatNumber,
		KvkNumber:       org.KvkNumber,
		AddressLine1:    org.AddressLine1,
		AddressLine2:    org.AddressLine2,
		PostalCode:      org.PostalCode,
		City:            org.City,
		Country:         org.Country,
		LogoFileKey:     org.LogoFileKey,
		LogoFileName:    org.LogoFileName,
		LogoContentType: org.LogoContentType,
		LogoSizeBytes:   org.LogoSizeBytes,
	}
}

func (h *Handler) PresignLogo(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	var req transport.OrgLogoPresignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.PresignLogoUpload(c.Request.Context(), *tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) SetLogo(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	var req transport.SetOrgLogoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	org, err := h.svc.SetLogo(c.Request.Context(), *tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, mapOrgResponse(org))
}

func (h *Handler) GetLogoDownload(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	result, err := h.svc.GetLogoDownloadURL(c.Request.Context(), *tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) DeleteLogo(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	org, err := h.svc.DeleteLogo(c.Request.Context(), *tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, mapOrgResponse(org))
}

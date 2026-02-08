package handler

import (
	"net/http"

	"portal_final_backend/internal/partners/service"
	"portal_final_backend/internal/partners/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

// Handler handles HTTP requests for partners.
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

// New creates a new partners handler.
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// RegisterRoutes registers partner routes.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/:id", h.GetByID)
	rg.PUT("/:id", h.Update)
	rg.DELETE("/:id", h.Delete)

	rg.POST("/:id/logo/presign", h.PresignLogo)
	rg.POST("/:id/logo", h.SetLogo)
	rg.GET("/:id/logo/download", h.GetLogoDownload)
	rg.DELETE("/:id/logo", h.DeleteLogo)

	rg.GET("/:id/leads", h.ListLeads)
	rg.POST("/:id/leads", h.LinkLead)
	rg.DELETE("/:id/leads/:leadId", h.UnlinkLead)

	rg.GET("/:id/invites", h.ListInvites)
	rg.POST("/:id/invites", h.CreateInvite)
	rg.DELETE("/:id/invites/:inviteId", h.RevokeInvite)

	// Offer routes (authenticated / admin)
	rg.POST("/offers", h.CreateOffer)
	rg.GET("/services/:serviceId/offers", h.ListServiceOffers)
	rg.GET("/offers/:offerId/preview", h.PreviewOffer)
}

func (h *Handler) List(c *gin.Context) {
	var req transport.ListPartnersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.List(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	var req transport.CreatePartnerRequest
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.Create(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.GetByID(c.Request.Context(), tenantID, id)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdatePartnerRequest
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.Update(c.Request.Context(), tenantID, id, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), tenantID, id); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "partner deleted"})
}

func (h *Handler) PresignLogo(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.PartnerLogoPresignRequest
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.PresignLogoUpload(c.Request.Context(), tenantID, partnerID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) SetLogo(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.SetPartnerLogoRequest
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.SetLogo(c.Request.Context(), tenantID, partnerID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) GetLogoDownload(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.GetLogoDownloadURL(c.Request.Context(), tenantID, partnerID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) DeleteLogo(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.DeleteLogo(c.Request.Context(), tenantID, partnerID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) ListLeads(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.ListLeads(c.Request.Context(), tenantID, partnerID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) LinkLead(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.LinkLeadRequest
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	if err := h.svc.LinkLead(c.Request.Context(), tenantID, partnerID, req.LeadID); httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, gin.H{"message": "lead linked"})
}

func (h *Handler) UnlinkLead(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	leadID, err := uuid.Parse(c.Param("leadId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	if err := h.svc.UnlinkLead(c.Request.Context(), tenantID, partnerID, leadID); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "lead unlinked"})
}

func (h *Handler) CreateInvite(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.CreatePartnerInviteRequest
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.CreateInvite(c.Request.Context(), tenantID, partnerID, identity.UserID(), req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

func (h *Handler) ListInvites(c *gin.Context) {
	partnerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.ListInvites(c.Request.Context(), tenantID, partnerID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) RevokeInvite(c *gin.Context) {
	inviteID, err := uuid.Parse(c.Param("inviteId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.RevokeInvite(c.Request.Context(), tenantID, inviteID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) CreateOffer(c *gin.Context) {
	var req transport.CreateOfferRequest
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
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.CreateOffer(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

func (h *Handler) PreviewOffer(c *gin.Context) {
	offerID, err := uuid.Parse(c.Param("offerId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.GetOfferPreview(c.Request.Context(), tenantID, offerID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) ListServiceOffers(c *gin.Context) {
	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.ListOffersForService(c.Request.Context(), tenantID, serviceID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func mustGetTenantID(c *gin.Context, identity httpkit.Identity) (uuid.UUID, bool) {
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant ID is required", nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}

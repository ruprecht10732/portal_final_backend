package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"portal_final_backend/internal/productflows/service"
	"portal_final_backend/internal/productflows/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"
)

// Handler handles HTTP requests for product flows.
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

// New creates a new product-flows handler.
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// GetFlow returns the active flow definition for a product group.
// GET /api/v1/product-flows/:productGroupId
func (h *Handler) GetFlow(c *gin.Context) {
	productGroupID := c.Param("productGroupId")
	if productGroupID == "" {
		httpkit.Error(c, http.StatusBadRequest, "productGroupId is required", nil)
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

	result, err := h.svc.GetFlow(c.Request.Context(), tenantID, productGroupID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// List returns all active flows visible to the tenant.
// GET /api/v1/admin/product-flows
func (h *Handler) List(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.ListAll(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Create inserts a new product flow.
// POST /api/v1/admin/product-flows
func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateProductFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid request body", nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "validation failed", err.Error())
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

	c.JSON(http.StatusCreated, result)
}

// Update replaces the definition for an existing product flow.
// PUT /api/v1/admin/product-flows/:id
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid flow ID", nil)
		return
	}

	var req transport.UpdateProductFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid request body", nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "validation failed", err.Error())
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

func mustGetTenantID(c *gin.Context, identity httpkit.Identity) (uuid.UUID, bool) {
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant ID is required", nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}

package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/catalog/service"
	"portal_final_backend/internal/catalog/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"
)

// Handler handles HTTP requests for catalog.
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
	msgInvalidID        = "invalid catalog id"
)

// New creates a new catalog handler.
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// ListVatRates retrieves VAT rates.
// GET /api/v1/catalog/vat-rates
func (h *Handler) ListVatRates(c *gin.Context) {
	var req transport.ListVatRatesRequest
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

	result, err := h.svc.ListVatRatesWithFilters(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// GetVatRateByID retrieves a VAT rate by ID.
// GET /api/v1/catalog/vat-rates/:id
func (h *Handler) GetVatRateByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
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

	result, err := h.svc.GetVatRateByID(c.Request.Context(), tenantID, id)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// CreateVatRate creates a new VAT rate.
// POST /api/v1/admin/catalog/vat-rates
func (h *Handler) CreateVatRate(c *gin.Context) {
	var req transport.CreateVatRateRequest
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

	result, err := h.svc.CreateVatRate(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.JSON(c, http.StatusCreated, result)
}

// UpdateVatRate updates a VAT rate.
// PUT /api/v1/admin/catalog/vat-rates/:id
func (h *Handler) UpdateVatRate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.UpdateVatRateRequest
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

	result, err := h.svc.UpdateVatRate(c.Request.Context(), tenantID, id, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// DeleteVatRate deletes a VAT rate.
// DELETE /api/v1/admin/catalog/vat-rates/:id
func (h *Handler) DeleteVatRate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
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

	if err := h.svc.DeleteVatRate(c.Request.Context(), tenantID, id); httpkit.HandleError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// ListProducts retrieves catalog products.
// GET /api/v1/catalog/products
func (h *Handler) ListProducts(c *gin.Context) {
	var req transport.ListProductsRequest
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

	var vatRateID *uuid.UUID
	if req.VatRateID != "" {
		parsed, err := uuid.Parse(req.VatRateID)
		if err != nil {
			httpkit.Error(c, http.StatusBadRequest, "invalid vatRateId", nil)
			return
		}
		vatRateID = &parsed
	}

	result, err := h.svc.ListProductsWithFilters(c.Request.Context(), tenantID, req, vatRateID)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// GetProductByID retrieves a product by ID.
// GET /api/v1/catalog/products/:id
func (h *Handler) GetProductByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
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

	result, err := h.svc.GetProductByID(c.Request.Context(), tenantID, id)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// CreateProduct creates a product.
// POST /api/v1/admin/catalog/products
func (h *Handler) CreateProduct(c *gin.Context) {
	var req transport.CreateProductRequest
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

	result, err := h.svc.CreateProduct(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.JSON(c, http.StatusCreated, result)
}

// GetNextProductReference returns the next auto-generated product reference.
// GET /api/v1/admin/catalog/products/next-reference
func (h *Handler) GetNextProductReference(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	result, err := h.svc.GetNextProductReference(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// UpdateProduct updates a product.
// PUT /api/v1/admin/catalog/products/:id
func (h *Handler) UpdateProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.UpdateProductRequest
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

	result, err := h.svc.UpdateProduct(c.Request.Context(), tenantID, id, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// DeleteProduct deletes a product.
// DELETE /api/v1/admin/catalog/products/:id
func (h *Handler) DeleteProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
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

	if err := h.svc.DeleteProduct(c.Request.Context(), tenantID, id); httpkit.HandleError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// ListProductMaterials lists materials linked to a product.
// GET /api/v1/catalog/products/:id/materials
func (h *Handler) ListProductMaterials(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
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

	result, err := h.svc.ListProductMaterials(c.Request.Context(), tenantID, id)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, result)
}

// AddProductMaterials adds materials to a product.
// POST /api/v1/admin/catalog/products/:id/materials
func (h *Handler) AddProductMaterials(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.ProductMaterialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	links := make([]repository.ProductMaterialLink, 0)
	if len(req.Materials) > 0 {
		for _, item := range req.Materials {
			links = append(links, repository.ProductMaterialLink{
				MaterialID:  item.MaterialID,
				PricingMode: item.PricingMode,
			})
		}
	} else {
		for _, materialID := range req.MaterialIDs {
			links = append(links, repository.ProductMaterialLink{
				MaterialID:  materialID,
				PricingMode: "additional",
			})
		}
	}

	if len(links) == 0 {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, "materialIds or materials is required")
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

	if err := h.svc.AddProductMaterials(c.Request.Context(), tenantID, id, links); httpkit.HandleError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// RemoveProductMaterials removes materials from a product.
// DELETE /api/v1/admin/catalog/products/:id/materials
func (h *Handler) RemoveProductMaterials(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.ProductMaterialsRequest
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

	if err := h.svc.RemoveProductMaterials(c.Request.Context(), tenantID, id, req.MaterialIDs); httpkit.HandleError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// GetCatalogAssetPresign generates a presigned URL for uploading a catalog asset.
// POST /api/v1/admin/catalog/products/:id/assets/presign
func (h *Handler) GetCatalogAssetPresign(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.PresignCatalogAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.GetCatalogAssetPresign(c.Request.Context(), tenantID, productID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// CreateCatalogAsset creates a catalog asset after upload to MinIO.
// POST /api/v1/admin/catalog/products/:id/assets
func (h *Handler) CreateCatalogAsset(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.CreateCatalogAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.CreateCatalogAsset(c.Request.Context(), tenantID, productID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// CreateCatalogURLAsset creates a URL-based catalog asset (terms URL).
// POST /api/v1/admin/catalog/products/:id/assets/url
func (h *Handler) CreateCatalogURLAsset(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var req transport.CreateCatalogURLAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.CreateCatalogURLAsset(c.Request.Context(), tenantID, productID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// ListCatalogAssets lists assets for a product.
// GET /api/v1/catalog/products/:id/assets
func (h *Handler) ListCatalogAssets(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	var assetType *string
	if queryType := strings.TrimSpace(c.Query("type")); queryType != "" {
		assetType = &queryType
	}

	result, err := h.svc.ListCatalogAssets(c.Request.Context(), tenantID, productID, assetType)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// GetCatalogAssetDownloadURL returns a presigned download URL or external URL.
// GET /api/v1/catalog/products/:id/assets/:assetId/download
func (h *Handler) GetCatalogAssetDownloadURL(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	assetID, err := uuid.Parse(c.Param("assetId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	result, err := h.svc.GetCatalogAssetDownloadURL(c.Request.Context(), tenantID, productID, assetID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// DeleteCatalogAsset deletes a catalog asset and removes it from storage when applicable.
// DELETE /api/v1/admin/catalog/products/:id/assets/:assetId
func (h *Handler) DeleteCatalogAsset(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidID, nil)
		return
	}

	assetID, err := uuid.Parse(c.Param("assetId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if err := h.svc.DeleteCatalogAsset(c.Request.Context(), tenantID, productID, assetID); httpkit.HandleError(c, err) {
		return
	}

	c.Status(http.StatusNoContent)
}

// SearchProductsForAutocomplete handles GET /api/v1/catalog/products/search
// Lightweight search endpoint optimized for quote line autocomplete.
func (h *Handler) SearchProductsForAutocomplete(c *gin.Context) {
	var req transport.AutocompleteSearchRequest
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

	result, err := h.svc.SearchForAutocomplete(c.Request.Context(), tenantID, req)
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

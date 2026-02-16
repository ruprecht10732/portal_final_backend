package transport

import "github.com/google/uuid"

// VAT Rates

type CreateVatRateRequest struct {
	Name    string `json:"name" validate:"required,min=1,max=100"`
	RateBps *int   `json:"rateBps" validate:"required,min=0,max=10000"`
}

type UpdateVatRateRequest struct {
	Name    *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	RateBps *int    `json:"rateBps,omitempty" validate:"omitempty,min=0,max=10000"`
}

type ListVatRatesRequest struct {
	Search    string `form:"search" validate:"max=100"`
	Page      int    `form:"page" validate:"omitempty,min=1"`
	PageSize  int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
	SortBy    string `form:"sortBy" validate:"omitempty,oneof=name rateBps createdAt updatedAt"`
	SortOrder string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
}

type VatRateResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	RateBps   int       `json:"rateBps"`
	CreatedAt string    `json:"createdAt"`
	UpdatedAt string    `json:"updatedAt"`
}

type VatRateListResponse struct {
	Items      []VatRateResponse `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

// Products

type CreateProductRequest struct {
	Title          string    `json:"title" validate:"required,min=1,max=200"`
	Reference      string    `json:"reference,omitempty" validate:"omitempty,min=1,max=100"`
	Description    *string   `json:"description,omitempty" validate:"omitempty,max=1000"`
	PriceCents     int64     `json:"priceCents" validate:"min=0"`
	UnitPriceCents int64     `json:"unitPriceCents,omitempty" validate:"min=0"`
	UnitLabel      *string   `json:"unitLabel,omitempty" validate:"omitempty,max=50"`
	LaborTimeText  *string   `json:"laborTimeText,omitempty" validate:"omitempty,max=100"`
	VatRateID      uuid.UUID `json:"vatRateId" validate:"required"`
	Type           string    `json:"type" validate:"required,oneof=digital_service service product material"`
	PeriodCount    *int      `json:"periodCount,omitempty" validate:"omitempty,min=1"`
	PeriodUnit     *string   `json:"periodUnit,omitempty" validate:"omitempty,oneof=day week month quarter year"`
}

type UpdateProductRequest struct {
	Title          *string    `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Reference      *string    `json:"reference,omitempty" validate:"omitempty,min=1,max=100"`
	Description    *string    `json:"description,omitempty" validate:"omitempty,max=1000"`
	PriceCents     *int64     `json:"priceCents,omitempty" validate:"omitempty,min=0"`
	UnitPriceCents *int64     `json:"unitPriceCents,omitempty" validate:"omitempty,min=0"`
	UnitLabel      *string    `json:"unitLabel,omitempty" validate:"omitempty,max=50"`
	LaborTimeText  *string    `json:"laborTimeText,omitempty" validate:"omitempty,max=100"`
	VatRateID      *uuid.UUID `json:"vatRateId,omitempty" validate:"omitempty"`
	Type           *string    `json:"type,omitempty" validate:"omitempty,oneof=digital_service service product material"`
	PeriodCount    *int       `json:"periodCount,omitempty" validate:"omitempty,min=1"`
	PeriodUnit     *string    `json:"periodUnit,omitempty" validate:"omitempty,oneof=day week month quarter year"`
}

type ListProductsRequest struct {
	Search        string `form:"search" validate:"max=100"`
	Title         string `form:"title" validate:"omitempty,max=200"`
	Reference     string `form:"reference" validate:"omitempty,max=100"`
	Type          string `form:"type" validate:"omitempty,oneof=digital_service service product material"`
	VatRateID     string `form:"vatRateId" validate:"omitempty"`
	CreatedAtFrom string `form:"createdAtFrom" validate:"omitempty,max=50"`
	CreatedAtTo   string `form:"createdAtTo" validate:"omitempty,max=50"`
	UpdatedAtFrom string `form:"updatedAtFrom" validate:"omitempty,max=50"`
	UpdatedAtTo   string `form:"updatedAtTo" validate:"omitempty,max=50"`
	Page          int    `form:"page" validate:"omitempty,min=1"`
	PageSize      int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
	SortBy        string `form:"sortBy" validate:"omitempty,oneof=title reference priceCents type vatRateId createdAt updatedAt"`
	SortOrder     string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
}

type ProductResponse struct {
	ID             uuid.UUID `json:"id"`
	VatRateID      uuid.UUID `json:"vatRateId"`
	Title          string    `json:"title"`
	Reference      string    `json:"reference"`
	Description    *string   `json:"description,omitempty"`
	PriceCents     int64     `json:"priceCents"`
	UnitPriceCents int64     `json:"unitPriceCents"`
	UnitLabel      *string   `json:"unitLabel,omitempty"`
	LaborTimeText  *string   `json:"laborTimeText,omitempty"`
	Type           string    `json:"type"`
	PricingMode    *string   `json:"pricingMode,omitempty"`
	PeriodCount    *int      `json:"periodCount,omitempty"`
	PeriodUnit     *string   `json:"periodUnit,omitempty"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
}

type ProductListResponse struct {
	Items      []ProductResponse `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

type NextProductReferenceResponse struct {
	Reference string `json:"reference"`
}

// Assets

type PresignCatalogAssetRequest struct {
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=255"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
	AssetType   string `json:"assetType" validate:"required,oneof=image document"`
}

type PresignedUploadResponse struct {
	UploadURL string `json:"uploadUrl"`
	FileKey   string `json:"fileKey"`
	ExpiresAt int64  `json:"expiresAt"`
}

type CreateCatalogAssetRequest struct {
	AssetType   string `json:"assetType" validate:"required,oneof=image document"`
	FileKey     string `json:"fileKey" validate:"required,min=1"`
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=255"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

type CreateCatalogURLAssetRequest struct {
	AssetType string  `json:"assetType" validate:"required,oneof=terms_url"`
	URL       string  `json:"url" validate:"required,url,max=2048"`
	Label     *string `json:"label,omitempty" validate:"omitempty,max=255"`
}

type CatalogAssetResponse struct {
	ID          uuid.UUID `json:"id"`
	ProductID   uuid.UUID `json:"productId"`
	AssetType   string    `json:"assetType"`
	FileKey     *string   `json:"fileKey,omitempty"`
	FileName    *string   `json:"fileName,omitempty"`
	ContentType *string   `json:"contentType,omitempty"`
	SizeBytes   *int64    `json:"sizeBytes,omitempty"`
	URL         *string   `json:"url,omitempty"`
	CreatedAt   string    `json:"createdAt"`
}

type CatalogAssetListResponse struct {
	Items []CatalogAssetResponse `json:"items"`
}

type PresignedDownloadResponse struct {
	DownloadURL string `json:"downloadUrl"`
	ExpiresAt   *int64 `json:"expiresAt,omitempty"`
}

// ── Autocomplete Search ──────────────────────────────────────────────────────

type AutocompleteSearchRequest struct {
	Query string `form:"q" validate:"required,min=1,max=200"`
	Limit int    `form:"limit" validate:"omitempty,min=1,max=20"`
}

type AutocompleteDocumentResponse struct {
	ID       uuid.UUID `json:"id"`
	Filename string    `json:"filename"`
	FileKey  string    `json:"fileKey"`
}

type AutocompleteURLResponse struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type AutocompleteItemResponse struct {
	ID             uuid.UUID                      `json:"id"`
	Title          string                         `json:"title"`
	Description    *string                        `json:"description,omitempty"`
	PriceCents     int64                          `json:"priceCents"`
	UnitPriceCents int64                          `json:"unitPriceCents"`
	UnitLabel      *string                        `json:"unitLabel,omitempty"`
	VatRateID      uuid.UUID                      `json:"vatRateId"`
	VatRateBps     int                            `json:"vatRateBps"`
	Documents      []AutocompleteDocumentResponse `json:"documents"`
	URLs           []AutocompleteURLResponse      `json:"urls"`
}

// Materials

type ProductMaterialsRequest struct {
	MaterialIDs []uuid.UUID                `json:"materialIds,omitempty" validate:"omitempty,min=1,dive,required"`
	Materials   []ProductMaterialLinkInput `json:"materials,omitempty" validate:"omitempty,min=1,dive"`
}

type ProductMaterialLinkInput struct {
	MaterialID  uuid.UUID `json:"materialId" validate:"required"`
	PricingMode string    `json:"pricingMode" validate:"required,oneof=included additional optional"`
}

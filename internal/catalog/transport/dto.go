package transport

import "github.com/google/uuid"

// ─── VAT Rates ──────────────────────────────────────────────────────────────

// CreateVatRateRequest defines the payload for creating a VAT rate.
type CreateVatRateRequest struct {
	Name    string `json:"name" validate:"required,min=1,max=100"`
	RateBps *int   `json:"rateBps" validate:"required,min=0,max=10000"`
}

// UpdateVatRateRequest defines the payload for patching a VAT rate.
type UpdateVatRateRequest struct {
	Name    *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	RateBps *int    `json:"rateBps,omitempty" validate:"omitempty,min=0,max=10000"`
}

// ListVatRatesRequest handles query parameters for VAT rate listing.
// Reordered for 64-bit alignment (strings/slices first, then ints).
type ListVatRatesRequest struct {
	Search    string `form:"search" validate:"omitempty,max=100"`
	SortBy    string `form:"sortBy" validate:"omitempty,oneof=name rateBps createdAt updatedAt"`
	SortOrder string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
	Page      int    `form:"page" validate:"omitempty,min=1"`
	PageSize  int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
}

// VatRateResponse represents a VAT rate in the API.
type VatRateResponse struct {
	ID        uuid.UUID `json:"id"`        // 16 bytes
	Name      string    `json:"name"`      // 16 bytes
	CreatedAt string    `json:"createdAt"` // 16 bytes
	UpdatedAt string    `json:"updatedAt"` // 16 bytes
	RateBps   int       `json:"rateBps"`   // 8 bytes
}

// VatRateListResponse provides a paginated list of VAT rates.
type VatRateListResponse struct {
	Items      []VatRateResponse `json:"items"` // 24 bytes
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

// ─── Products ───────────────────────────────────────────────────────────────

// CreateProductRequest defines the payload for creating a catalog item.
// Packed to minimize padding bytes (O(1) space optimization).
type CreateProductRequest struct {
	Title          string    `json:"title" validate:"required,min=1,max=200"`
	Type           string    `json:"type" validate:"required,oneof=digital_service service product material"`
	Reference      string    `json:"reference,omitempty" validate:"omitempty,min=1,max=100"`
	VatRateID      uuid.UUID `json:"vatRateId" validate:"required"`
	Description    *string   `json:"description,omitempty" validate:"omitempty,max=1000"`
	UnitLabel      *string   `json:"unitLabel,omitempty" validate:"omitempty,max=50"`
	LaborTimeText  *string   `json:"laborTimeText,omitempty" validate:"omitempty,max=100"`
	PeriodUnit     *string   `json:"periodUnit,omitempty" validate:"omitempty,oneof=day week month quarter year"`
	PriceCents     int64     `json:"priceCents" validate:"min=0"`
	UnitPriceCents int64     `json:"unitPriceCents,omitempty" validate:"min=0"`
	PeriodCount    *int      `json:"periodCount,omitempty" validate:"omitempty,min=1"`
	IsDraft        *bool     `json:"isDraft,omitempty" validate:"omitempty"`
}

// UpdateProductRequest defines the payload for updating a catalog item.
type UpdateProductRequest struct {
	Title          *string    `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Reference      *string    `json:"reference,omitempty" validate:"omitempty,min=1,max=100"`
	Description    *string    `json:"description,omitempty" validate:"omitempty,max=1000"`
	UnitLabel      *string    `json:"unitLabel,omitempty" validate:"omitempty,max=50"`
	LaborTimeText  *string    `json:"laborTimeText,omitempty" validate:"omitempty,max=100"`
	Type           *string    `json:"type,omitempty" validate:"omitempty,oneof=digital_service service product material"`
	PeriodUnit     *string    `json:"periodUnit,omitempty" validate:"omitempty,oneof=day week month quarter year"`
	VatRateID      *uuid.UUID `json:"vatRateId,omitempty" validate:"omitempty"`
	PriceCents     *int64     `json:"priceCents,omitempty" validate:"omitempty,min=0"`
	UnitPriceCents *int64     `json:"unitPriceCents,omitempty" validate:"omitempty,min=0"`
	PeriodCount    *int       `json:"periodCount,omitempty" validate:"omitempty,min=1"`
	IsDraft        *bool      `json:"isDraft,omitempty" validate:"omitempty"`
}

// ListProductsRequest handles query parameters for product listing.
type ListProductsRequest struct {
	Search        string `form:"search" validate:"omitempty,max=100"`
	Title         string `form:"title" validate:"omitempty,max=200"`
	Reference     string `form:"reference" validate:"omitempty,max=100"`
	Type          string `form:"type" validate:"omitempty,oneof=digital_service service product material"`
	VatRateID     string `form:"vatRateId" validate:"omitempty"` // Note: kept as string for flexibility
	CreatedAtFrom string `form:"createdAtFrom" validate:"omitempty,max=50"`
	CreatedAtTo   string `form:"createdAtTo" validate:"omitempty,max=50"`
	UpdatedAtFrom string `form:"updatedAtFrom" validate:"omitempty,max=50"`
	UpdatedAtTo   string `form:"updatedAtTo" validate:"omitempty,max=50"`
	SortBy        string `form:"sortBy" validate:"omitempty,oneof=title reference priceCents type isDraft vatRateId createdAt updatedAt"`
	SortOrder     string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
	IsDraft       *bool  `form:"isDraft" validate:"omitempty"`
	Page          int    `form:"page" validate:"omitempty,min=1"`
	PageSize      int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
}

// ProductResponse represents a detailed product view.
type ProductResponse struct {
	ID             uuid.UUID `json:"id"`
	VatRateID      uuid.UUID `json:"vatRateId"`
	Title          string    `json:"title"`
	Reference      string    `json:"reference"`
	Type           string    `json:"type"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
	Description    *string   `json:"description,omitempty"`
	UnitLabel      *string   `json:"unitLabel,omitempty"`
	LaborTimeText  *string   `json:"laborTimeText,omitempty"`
	PricingMode    *string   `json:"pricingMode,omitempty"`
	PeriodUnit     *string   `json:"periodUnit,omitempty"`
	PriceCents     int64     `json:"priceCents"`
	UnitPriceCents int64     `json:"unitPriceCents"`
	PeriodCount    *int      `json:"periodCount,omitempty"`
	IsDraft        bool      `json:"isDraft"`
}

// ProductListResponse provides a paginated list of products.
type ProductListResponse struct {
	Items      []ProductResponse `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

// NextProductReferenceResponse contains the next generated SKU.
type NextProductReferenceResponse struct {
	Reference string `json:"reference"`
}

// ─── Assets ─────────────────────────────────────────────────────────────────

type PresignCatalogAssetRequest struct {
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=255"`
	AssetType   string `json:"assetType" validate:"required,oneof=image document"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
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
	CreatedAt   string    `json:"createdAt"`
	FileKey     *string   `json:"fileKey,omitempty"`
	FileName    *string   `json:"fileName,omitempty"`
	ContentType *string   `json:"contentType,omitempty"`
	URL         *string   `json:"url,omitempty"`
	SizeBytes   *int64    `json:"sizeBytes,omitempty"`
}

type CatalogAssetListResponse struct {
	Items []CatalogAssetResponse `json:"items"`
}

type PresignedDownloadResponse struct {
	DownloadURL string `json:"downloadUrl"`
	ExpiresAt   *int64 `json:"expiresAt,omitempty"`
}

// ─── Autocomplete Search ─────────────────────────────────────────────────────

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

// AutocompleteItemResponse represents a hit from semantic or fuzzy search.
type AutocompleteItemResponse struct {
	ID               string                         `json:"id"`
	Title            string                         `json:"title"`
	SourceType       string                         `json:"sourceType"`
	SourceCollection string                         `json:"sourceCollection,omitempty"`
	SourceLabel      string                         `json:"sourceLabel,omitempty"`
	CatalogProductID *uuid.UUID                     `json:"catalogProductId,omitempty"`
	VatRateID        *uuid.UUID                     `json:"vatRateId,omitempty"`
	Description      *string                        `json:"description,omitempty"`
	UnitLabel        *string                        `json:"unitLabel,omitempty"`
	SourceURL        *string                        `json:"sourceUrl,omitempty"`
	Documents        []AutocompleteDocumentResponse `json:"documents"`
	URLs             []AutocompleteURLResponse      `json:"urls"`
	PriceCents       int64                          `json:"priceCents"`
	UnitPriceCents   int64                          `json:"unitPriceCents"`
	Score            *float64                       `json:"score,omitempty"`
	VatRateBps       int                            `json:"vatRateBps"`
}

// ─── Materials ──────────────────────────────────────────────────────────────

type ProductMaterialsRequest struct {
	MaterialIDs []uuid.UUID                `json:"materialIds,omitempty" validate:"omitempty,min=1,dive,required"`
	Materials   []ProductMaterialLinkInput `json:"materials,omitempty" validate:"omitempty,min=1,dive"`
}

type ProductMaterialLinkInput struct {
	MaterialID  uuid.UUID `json:"materialId" validate:"required"`
	PricingMode string    `json:"pricingMode" validate:"required,oneof=included additional optional"`
}

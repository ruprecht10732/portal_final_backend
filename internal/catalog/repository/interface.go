package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// VatRate represents a VAT rate for catalog pricing.
// Fields ordered by byte size to prevent padding waste.
type VatRate struct {
	ID             uuid.UUID `db:"id"`              // 16 bytes
	OrganizationID uuid.UUID `db:"organization_id"` // 16 bytes
	Name           string    `db:"name"`            // 16 bytes
	CreatedAt      string    `db:"created_at"`      // 16 bytes (Tech Debt: Should be time.Time)
	UpdatedAt      string    `db:"updated_at"`      // 16 bytes
	RateBps        int       `db:"rate_bps"`        // 8 bytes
}

// Product represents a catalog product or service.
// Struct packed: 16-byte blocks -> 8-byte blocks -> 1-byte blocks.
// This O(1) space optimization drastically reduces the memory footprint when fetching large catalogs.
type Product struct {
	ID             uuid.UUID `db:"id"`
	OrganizationID uuid.UUID `db:"organization_id"`
	VatRateID      uuid.UUID `db:"vat_rate_id"`
	Title          string    `db:"title"`
	Reference      string    `db:"reference"`
	Type           string    `db:"type"`
	CreatedAt      string    `db:"created_at"`
	UpdatedAt      string    `db:"updated_at"`
	PriceCents     int64     `db:"price_cents"`
	UnitPriceCents int64     `db:"unit_price_cents"`
	Description    *string   `db:"description"`
	UnitLabel      *string   `db:"unit_label"`
	LaborTimeText  *string   `db:"labor_time_text"`
	PricingMode    *string   `db:"pricing_mode"`
	PeriodCount    *int      `db:"period_count"`
	PeriodUnit     *string   `db:"period_unit"`
	IsDraft        bool      `db:"is_draft"`
}

type ProductMaterialLink struct {
	MaterialID  uuid.UUID
	PricingMode string
}

// ProductAsset represents an asset linked to a catalog product.
type ProductAsset struct {
	ID             uuid.UUID `db:"id"`
	OrganizationID uuid.UUID `db:"organization_id"`
	ProductID      uuid.UUID `db:"product_id"`
	AssetType      string    `db:"asset_type"`
	CreatedAt      string    `db:"created_at"`
	FileKey        *string   `db:"file_key"`
	FileName       *string   `db:"file_name"`
	ContentType    *string   `db:"content_type"`
	SizeBytes      *int64    `db:"size_bytes"`
	URL            *string   `db:"url"`
}

// CreateVatRateParams contains data for creating a VAT rate.
type CreateVatRateParams struct {
	OrganizationID uuid.UUID
	Name           string
	RateBps        int
}

// UpdateVatRateParams contains data for updating a VAT rate.
type UpdateVatRateParams struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           *string
	RateBps        *int
}

// ListVatRatesParams defines filters for listing VAT rates.
type ListVatRatesParams struct {
	OrganizationID uuid.UUID
	Search         string
	SortBy         string
	SortOrder      string
	Offset         int
	Limit          int
}

// CreateProductParams contains data for creating a product.
type CreateProductParams struct {
	OrganizationID uuid.UUID
	VatRateID      uuid.UUID
	Title          string
	Reference      string
	Type           string
	Description    *string
	UnitLabel      *string
	LaborTimeText  *string
	PeriodUnit     *string
	PriceCents     int64
	UnitPriceCents int64
	PeriodCount    *int
	IsDraft        bool
}

// UpdateProductParams contains data for updating a product.
type UpdateProductParams struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	VatRateID      *uuid.UUID
	Title          *string
	Reference      *string
	Description    *string
	UnitLabel      *string
	LaborTimeText  *string
	Type           *string
	PeriodUnit     *string
	PriceCents     *int64
	UnitPriceCents *int64
	PeriodCount    *int
	IsDraft        *bool
}

// CreateProductAssetParams contains data for creating a product asset.
type CreateProductAssetParams struct {
	OrganizationID uuid.UUID
	ProductID      uuid.UUID
	AssetType      string
	FileKey        *string
	FileName       *string
	ContentType    *string
	SizeBytes      *int64
	URL            *string
}

// ListProductAssetsParams defines filters for listing product assets.
// Tech Debt (Security): Missing Limit and Offset. This allows O(N) unbounded queries.
type ListProductAssetsParams struct {
	OrganizationID uuid.UUID
	ProductID      uuid.UUID
	AssetType      *string
}

// ListProductsParams defines filters for listing products.
// Packed for memory alignment.
type ListProductsParams struct {
	CreatedAtFrom  *time.Time
	CreatedAtTo    *time.Time
	UpdatedAtFrom  *time.Time
	UpdatedAtTo    *time.Time
	OrganizationID uuid.UUID
	Search         string
	Title          string
	Reference      string
	Type           string
	SortBy         string
	SortOrder      string
	VatRateID      *uuid.UUID
	Offset         int
	Limit          int
	IsDraft        *bool
}

// Repository defines catalog storage operations.
type Repository interface {
	CreateVatRate(ctx context.Context, params CreateVatRateParams) (VatRate, error)
	UpdateVatRate(ctx context.Context, params UpdateVatRateParams) (VatRate, error)
	DeleteVatRate(ctx context.Context, organizationID, id uuid.UUID) error
	GetVatRateByID(ctx context.Context, organizationID, id uuid.UUID) (VatRate, error)
	ListVatRates(ctx context.Context, params ListVatRatesParams) ([]VatRate, int, error)
	HasProductsWithVatRate(ctx context.Context, organizationID, id uuid.UUID) (bool, error)

	CreateProduct(ctx context.Context, params CreateProductParams) (Product, error)
	NextProductReference(ctx context.Context, organizationID uuid.UUID) (string, error)
	UpdateProduct(ctx context.Context, params UpdateProductParams) (Product, error)
	DeleteProduct(ctx context.Context, organizationID, id uuid.UUID) error
	GetProductByID(ctx context.Context, organizationID, id uuid.UUID) (Product, error)
	ListProducts(ctx context.Context, params ListProductsParams) ([]Product, int, error)

	// GetProductsByIDs must execute in O(1) network roundtrips via SQL IN clause.
	// Implementing this via a loop of GetProductByID is strictly forbidden.
	GetProductsByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]Product, error)

	CreateProductAsset(ctx context.Context, params CreateProductAssetParams) (ProductAsset, error)
	GetProductAssetByID(ctx context.Context, organizationID, id uuid.UUID) (ProductAsset, error)

	// ListProductAssets is currently unbounded O(N). Migrate to pagination.
	ListProductAssets(ctx context.Context, params ListProductAssetsParams) ([]ProductAsset, error)
	DeleteProductAsset(ctx context.Context, organizationID, id uuid.UUID) error

	AddProductMaterials(ctx context.Context, organizationID, productID uuid.UUID, links []ProductMaterialLink) error
	RemoveProductMaterials(ctx context.Context, organizationID, productID uuid.UUID, materialIDs []uuid.UUID) error
	ListProductMaterials(ctx context.Context, organizationID, productID uuid.UUID) ([]Product, error)
	HasProductMaterials(ctx context.Context, organizationID, productID uuid.UUID) (bool, error)
}

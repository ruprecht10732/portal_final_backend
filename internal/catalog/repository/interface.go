package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// VatRate represents a VAT rate for catalog pricing.
type VatRate struct {
	ID             uuid.UUID `db:"id"`
	OrganizationID uuid.UUID `db:"organization_id"`
	Name           string    `db:"name"`
	RateBps        int       `db:"rate_bps"`
	CreatedAt      string    `db:"created_at"`
	UpdatedAt      string    `db:"updated_at"`
}

// Product represents a catalog product or service.
type Product struct {
	ID             uuid.UUID `db:"id"`
	OrganizationID uuid.UUID `db:"organization_id"`
	VatRateID      uuid.UUID `db:"vat_rate_id"`
	IsDraft        bool      `db:"is_draft"`
	Title          string    `db:"title"`
	Reference      string    `db:"reference"`
	Description    *string   `db:"description"`
	PriceCents     int64     `db:"price_cents"`
	UnitPriceCents int64     `db:"unit_price_cents"`
	UnitLabel      *string   `db:"unit_label"`
	LaborTimeText  *string   `db:"labor_time_text"`
	Type           string    `db:"type"`
	PricingMode    *string   `db:"pricing_mode"`
	PeriodCount    *int      `db:"period_count"`
	PeriodUnit     *string   `db:"period_unit"`
	CreatedAt      string    `db:"created_at"`
	UpdatedAt      string    `db:"updated_at"`
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
	FileKey        *string   `db:"file_key"`
	FileName       *string   `db:"file_name"`
	ContentType    *string   `db:"content_type"`
	SizeBytes      *int64    `db:"size_bytes"`
	URL            *string   `db:"url"`
	CreatedAt      string    `db:"created_at"`
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
	Offset         int
	Limit          int
	SortBy         string
	SortOrder      string
}

// CreateProductParams contains data for creating a product.
type CreateProductParams struct {
	OrganizationID uuid.UUID
	VatRateID      uuid.UUID
	IsDraft        bool
	Title          string
	Reference      string
	Description    *string
	PriceCents     int64
	UnitPriceCents int64
	UnitLabel      *string
	LaborTimeText  *string
	Type           string
	PeriodCount    *int
	PeriodUnit     *string
}

// UpdateProductParams contains data for updating a product.
type UpdateProductParams struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	VatRateID      *uuid.UUID
	IsDraft        *bool
	Title          *string
	Reference      *string
	Description    *string
	PriceCents     *int64
	UnitPriceCents *int64
	UnitLabel      *string
	LaborTimeText  *string
	Type           *string
	PeriodCount    *int
	PeriodUnit     *string
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
type ListProductAssetsParams struct {
	OrganizationID uuid.UUID
	ProductID      uuid.UUID
	AssetType      *string
}

// ListProductsParams defines filters for listing products.
type ListProductsParams struct {
	OrganizationID uuid.UUID
	Search         string
	Title          string
	Reference      string
	Type           string
	IsDraft        *bool
	VatRateID      *uuid.UUID
	CreatedAtFrom  *time.Time
	CreatedAtTo    *time.Time
	UpdatedAtFrom  *time.Time
	UpdatedAtTo    *time.Time
	Offset         int
	Limit          int
	SortBy         string
	SortOrder      string
}

// Repository defines catalog storage operations.
type Repository interface {
	CreateVatRate(ctx context.Context, params CreateVatRateParams) (VatRate, error)
	UpdateVatRate(ctx context.Context, params UpdateVatRateParams) (VatRate, error)
	DeleteVatRate(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error
	GetVatRateByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (VatRate, error)
	ListVatRates(ctx context.Context, params ListVatRatesParams) ([]VatRate, int, error)
	HasProductsWithVatRate(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error)

	CreateProduct(ctx context.Context, params CreateProductParams) (Product, error)
	NextProductReference(ctx context.Context, organizationID uuid.UUID) (string, error)
	UpdateProduct(ctx context.Context, params UpdateProductParams) (Product, error)
	DeleteProduct(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error
	GetProductByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (Product, error)
	ListProducts(ctx context.Context, params ListProductsParams) ([]Product, int, error)
	GetProductsByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]Product, error)

	CreateProductAsset(ctx context.Context, params CreateProductAssetParams) (ProductAsset, error)
	GetProductAssetByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (ProductAsset, error)
	ListProductAssets(ctx context.Context, params ListProductAssetsParams) ([]ProductAsset, error)
	DeleteProductAsset(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error

	AddProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID, links []ProductMaterialLink) error
	RemoveProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID, materialIDs []uuid.UUID) error
	ListProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) ([]Product, error)
	HasProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) (bool, error)
}

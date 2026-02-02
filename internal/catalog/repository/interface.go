package repository

import (
	"context"

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
	Title          string    `db:"title"`
	Reference      string    `db:"reference"`
	Description    *string   `db:"description"`
	PriceCents     int64     `db:"price_cents"`
	Type           string    `db:"type"`
	PeriodCount    *int      `db:"period_count"`
	PeriodUnit     *string   `db:"period_unit"`
	CreatedAt      string    `db:"created_at"`
	UpdatedAt      string    `db:"updated_at"`
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
	Title          string
	Reference      string
	Description    *string
	PriceCents     int64
	Type           string
	PeriodCount    *int
	PeriodUnit     *string
}

// UpdateProductParams contains data for updating a product.
type UpdateProductParams struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	VatRateID      *uuid.UUID
	Title          *string
	Reference      *string
	Description    *string
	PriceCents     *int64
	Type           *string
	PeriodCount    *int
	PeriodUnit     *string
}

// ListProductsParams defines filters for listing products.
type ListProductsParams struct {
	OrganizationID uuid.UUID
	Search         string
	Type           string
	VatRateID      *uuid.UUID
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
	UpdateProduct(ctx context.Context, params UpdateProductParams) (Product, error)
	DeleteProduct(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error
	GetProductByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (Product, error)
	ListProducts(ctx context.Context, params ListProductsParams) ([]Product, int, error)
	GetProductsByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]Product, error)

	AddProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID, materialIDs []uuid.UUID) error
	RemoveProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID, materialIDs []uuid.UUID) error
	ListProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) ([]Product, error)
	HasProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) (bool, error)
}

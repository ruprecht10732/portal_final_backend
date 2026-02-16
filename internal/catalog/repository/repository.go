package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/apperr"
)

const (
	vatRateNotFoundMessage = "vat rate not found"
	productNotFoundMessage = "product not found"
)

// productSortFields maps API field names to allowed database sort columns.
var productSortFields = map[string]string{
	"title":      "title",
	"reference":  "reference",
	"priceCents": "price_cents",
	"type":       "type",
	"vatRateId":  "vat_rate_id",
	"createdAt":  "created_at",
	"updatedAt":  "updated_at",
}

// mapProductSortColumn returns the validated database sort column.
func mapProductSortColumn(sortBy string) (string, error) {
	if sortBy == "" {
		return "created_at", nil
	}
	if column, ok := productSortFields[sortBy]; ok {
		return column, nil
	}
	return "", apperr.BadRequest("invalid sort field")
}

// mapVatRateSortColumn returns the validated database sort column.
func mapVatRateSortColumn(sortBy string) (string, error) {
	if sortBy == "" {
		return "name", nil
	}
	switch sortBy {
	case "name":
		return "name", nil
	case "rateBps":
		return "rate_bps", nil
	case "createdAt":
		return "created_at", nil
	case "updatedAt":
		return "updated_at", nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

// mapSortOrder returns validated sort order key.
func mapSortOrder(sortOrder string) (string, error) {
	if sortOrder == "" {
		return "desc", nil
	}
	switch sortOrder {
	case "asc", "desc":
		return sortOrder, nil
	default:
		return "", apperr.BadRequest("invalid sort order")
	}
}

// Repo implements the catalog repository.
type Repo struct {
	pool *pgxpool.Pool
}

// New creates a new catalog repository.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Compile-time check that Repo implements Repository.
var _ Repository = (*Repo)(nil)

// CreateVatRate creates a VAT rate.
func (r *Repo) CreateVatRate(ctx context.Context, params CreateVatRateParams) (VatRate, error) {
	query := `
		INSERT INTO RAC_catalog_vat_rates (organization_id, name, rate_bps)
		VALUES ($1, $2, $3)
		RETURNING id, organization_id, name, rate_bps, created_at, updated_at`

	var rate VatRate
	var createdAt, updatedAt time.Time
	if err := r.pool.QueryRow(ctx, query, params.OrganizationID, params.Name, params.RateBps).Scan(
		&rate.ID, &rate.OrganizationID, &rate.Name, &rate.RateBps, &createdAt, &updatedAt,
	); err != nil {
		return VatRate{}, fmt.Errorf("create vat rate: %w", err)
	}

	rate.CreatedAt = createdAt.Format(time.RFC3339)
	rate.UpdatedAt = updatedAt.Format(time.RFC3339)
	return rate, nil
}

// UpdateVatRate updates a VAT rate.
func (r *Repo) UpdateVatRate(ctx context.Context, params UpdateVatRateParams) (VatRate, error) {
	query := `
		UPDATE RAC_catalog_vat_rates
		SET name = COALESCE($3, name),
			rate_bps = COALESCE($4, rate_bps),
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, name, rate_bps, created_at, updated_at`

	var rate VatRate
	var createdAt, updatedAt time.Time
	if err := r.pool.QueryRow(ctx, query,
		params.ID, params.OrganizationID, params.Name, params.RateBps,
	).Scan(&rate.ID, &rate.OrganizationID, &rate.Name, &rate.RateBps, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VatRate{}, apperr.NotFound(vatRateNotFoundMessage)
		}
		return VatRate{}, fmt.Errorf("update vat rate: %w", err)
	}

	rate.CreatedAt = createdAt.Format(time.RFC3339)
	rate.UpdatedAt = updatedAt.Format(time.RFC3339)
	return rate, nil
}

// DeleteVatRate deletes a VAT rate.
func (r *Repo) DeleteVatRate(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error {
	query := `DELETE FROM RAC_catalog_vat_rates WHERE id = $1 AND organization_id = $2`
	result, err := r.pool.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf("delete vat rate: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(vatRateNotFoundMessage)
	}
	return nil
}

// GetVatRateByID retrieves a VAT rate by ID.
func (r *Repo) GetVatRateByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (VatRate, error) {
	query := `
		SELECT id, organization_id, name, rate_bps, created_at, updated_at
		FROM RAC_catalog_vat_rates
		WHERE id = $1 AND organization_id = $2`

	var rate VatRate
	var createdAt, updatedAt time.Time
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(
		&rate.ID, &rate.OrganizationID, &rate.Name, &rate.RateBps, &createdAt, &updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VatRate{}, apperr.NotFound(vatRateNotFoundMessage)
		}
		return VatRate{}, fmt.Errorf("get vat rate by id: %w", err)
	}

	rate.CreatedAt = createdAt.Format(time.RFC3339)
	rate.UpdatedAt = updatedAt.Format(time.RFC3339)
	return rate, nil
}

// ListVatRates lists VAT rates with filters and pagination.
func (r *Repo) ListVatRates(ctx context.Context, params ListVatRatesParams) ([]VatRate, int, error) {
	var searchParam interface{}
	if params.Search != "" {
		searchParam = "%" + params.Search + "%"
	}

	sortBy, err := mapVatRateSortColumn(params.SortBy)
	if err != nil {
		return nil, 0, err
	}
	sortOrder, err := mapSortOrder(params.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	countQuery := `
		SELECT COUNT(*)
		FROM RAC_catalog_vat_rates
		WHERE organization_id = $1
			AND ($2::text IS NULL OR name ILIKE $2)
	`

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, params.OrganizationID, searchParam).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count vat rates: %w", err)
	}

	orderBy := fmt.Sprintf("%s %s, name ASC", sortBy, sortOrder)
	query := fmt.Sprintf(`
		SELECT id, organization_id, name, rate_bps, created_at, updated_at
		FROM RAC_catalog_vat_rates
		WHERE organization_id = $1
			AND ($2::text IS NULL OR name ILIKE $2)
		ORDER BY %s
		LIMIT $3 OFFSET $4
	`, orderBy)

	rows, err := r.pool.Query(ctx, query, params.OrganizationID, searchParam, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list vat rates: %w", err)
	}
	defer rows.Close()

	items := make([]VatRate, 0)
	for rows.Next() {
		var rate VatRate
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&rate.ID, &rate.OrganizationID, &rate.Name, &rate.RateBps, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan vat rate: %w", err)
		}
		rate.CreatedAt = createdAt.Format(time.RFC3339)
		rate.UpdatedAt = updatedAt.Format(time.RFC3339)
		items = append(items, rate)
	}
	if rows.Err() != nil {
		return nil, 0, fmt.Errorf("iterate vat rates: %w", rows.Err())
	}

	return items, total, nil
}

// HasProductsWithVatRate checks if any products reference a VAT rate.
func (r *Repo) HasProductsWithVatRate(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM RAC_catalog_products WHERE vat_rate_id = $1 AND organization_id = $2)`
	var exists bool
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check vat rate usage: %w", err)
	}
	return exists, nil
}

// CreateProduct creates a product.
func (r *Repo) CreateProduct(ctx context.Context, params CreateProductParams) (Product, error) {
	query := `
		INSERT INTO RAC_catalog_products (
			organization_id, vat_rate_id, title, reference, description, price_cents, unit_price_cents, unit_label, labor_time_text, type, period_count, period_unit
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, organization_id, vat_rate_id, title, reference, description, price_cents, unit_price_cents, unit_label, labor_time_text, type, period_count, period_unit, created_at, updated_at`

	var product Product
	var createdAt, updatedAt time.Time
	if err := r.pool.QueryRow(ctx, query,
		params.OrganizationID, params.VatRateID, params.Title, params.Reference, params.Description,
		params.PriceCents, params.UnitPriceCents, params.UnitLabel, params.LaborTimeText, params.Type, params.PeriodCount, params.PeriodUnit,
	).Scan(
		&product.ID, &product.OrganizationID, &product.VatRateID, &product.Title, &product.Reference,
		&product.Description, &product.PriceCents, &product.UnitPriceCents, &product.UnitLabel, &product.LaborTimeText, &product.Type, &product.PeriodCount, &product.PeriodUnit,
		&createdAt, &updatedAt,
	); err != nil {
		return Product{}, fmt.Errorf("create product: %w", err)
	}

	product.CreatedAt = createdAt.Format(time.RFC3339)
	product.UpdatedAt = updatedAt.Format(time.RFC3339)
	return product, nil
}

// NextProductReference atomically generates the next product reference for an organization.
func (r *Repo) NextProductReference(ctx context.Context, organizationID uuid.UUID) (string, error) {
	var nextNum int
	query := `
		INSERT INTO RAC_catalog_product_counters (organization_id, last_number)
		VALUES ($1, 1)
		ON CONFLICT (organization_id) DO UPDATE SET last_number = RAC_catalog_product_counters.last_number + 1
		RETURNING last_number`

	if err := r.pool.QueryRow(ctx, query, organizationID).Scan(&nextNum); err != nil {
		return "", fmt.Errorf("generate next product reference: %w", err)
	}

	year := time.Now().Year()
	return fmt.Sprintf("SKU-%d-%04d", year, nextNum), nil
}

// UpdateProduct updates a product.
func (r *Repo) UpdateProduct(ctx context.Context, params UpdateProductParams) (Product, error) {
	query := `
		UPDATE RAC_catalog_products
		SET
			vat_rate_id = COALESCE($3, vat_rate_id),
			title = COALESCE($4, title),
			reference = COALESCE($5, reference),
			description = COALESCE($6, description),
			price_cents = COALESCE($7, price_cents),
			unit_price_cents = COALESCE($8, unit_price_cents),
			unit_label = COALESCE($9, unit_label),
			labor_time_text = COALESCE($10, labor_time_text),
			type = COALESCE($11, type),
			period_count = COALESCE($12, period_count),
			period_unit = COALESCE($13, period_unit),
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, vat_rate_id, title, reference, description, price_cents, unit_price_cents, unit_label, labor_time_text, type, period_count, period_unit, created_at, updated_at`

	var product Product
	var createdAt, updatedAt time.Time
	if err := r.pool.QueryRow(ctx, query,
		params.ID, params.OrganizationID, params.VatRateID, params.Title, params.Reference, params.Description,
		params.PriceCents, params.UnitPriceCents, params.UnitLabel, params.LaborTimeText, params.Type, params.PeriodCount, params.PeriodUnit,
	).Scan(
		&product.ID, &product.OrganizationID, &product.VatRateID, &product.Title, &product.Reference,
		&product.Description, &product.PriceCents, &product.UnitPriceCents, &product.UnitLabel, &product.LaborTimeText, &product.Type, &product.PeriodCount, &product.PeriodUnit,
		&createdAt, &updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, apperr.NotFound(productNotFoundMessage)
		}
		return Product{}, fmt.Errorf("update product: %w", err)
	}

	product.CreatedAt = createdAt.Format(time.RFC3339)
	product.UpdatedAt = updatedAt.Format(time.RFC3339)
	return product, nil
}

// DeleteProduct deletes a product.
func (r *Repo) DeleteProduct(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error {
	query := `DELETE FROM RAC_catalog_products WHERE id = $1 AND organization_id = $2`
	result, err := r.pool.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(productNotFoundMessage)
	}
	return nil
}

// GetProductByID retrieves a product by ID.
func (r *Repo) GetProductByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (Product, error) {
	query := `
		SELECT id, organization_id, vat_rate_id, title, reference, description, price_cents, unit_price_cents, unit_label, labor_time_text, type, period_count, period_unit, created_at, updated_at
		FROM RAC_catalog_products
		WHERE id = $1 AND organization_id = $2`

	var product Product
	var createdAt, updatedAt time.Time
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(
		&product.ID, &product.OrganizationID, &product.VatRateID, &product.Title, &product.Reference,
		&product.Description, &product.PriceCents, &product.UnitPriceCents, &product.UnitLabel, &product.LaborTimeText, &product.Type, &product.PeriodCount, &product.PeriodUnit,
		&createdAt, &updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, apperr.NotFound(productNotFoundMessage)
		}
		return Product{}, fmt.Errorf("get product by id: %w", err)
	}

	product.CreatedAt = createdAt.Format(time.RFC3339)
	product.UpdatedAt = updatedAt.Format(time.RFC3339)
	return product, nil
}

// ListProducts lists products with filters and pagination.
func (r *Repo) ListProducts(ctx context.Context, params ListProductsParams) ([]Product, int, error) {
	var searchParam interface{}
	if params.Search != "" {
		searchParam = "%" + params.Search + "%"
	}
	var titleParam interface{}
	if params.Title != "" {
		titleParam = "%" + params.Title + "%"
	}
	var referenceParam interface{}
	if params.Reference != "" {
		referenceParam = "%" + params.Reference + "%"
	}
	var typeParam interface{}
	if params.Type != "" {
		typeParam = params.Type
	}
	var vatRateParam interface{}
	if params.VatRateID != nil {
		vatRateParam = *params.VatRateID
	}
	var createdAtFrom interface{}
	if params.CreatedAtFrom != nil {
		createdAtFrom = *params.CreatedAtFrom
	}
	var createdAtTo interface{}
	if params.CreatedAtTo != nil {
		createdAtTo = *params.CreatedAtTo
	}
	var updatedAtFrom interface{}
	if params.UpdatedAtFrom != nil {
		updatedAtFrom = *params.UpdatedAtFrom
	}
	var updatedAtTo interface{}
	if params.UpdatedAtTo != nil {
		updatedAtTo = *params.UpdatedAtTo
	}

	sortBy, err := mapProductSortColumn(params.SortBy)
	if err != nil {
		return nil, 0, err
	}
	sortOrder, err := mapSortOrder(params.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	countQuery := `
		SELECT COUNT(*)
		FROM RAC_catalog_products
		WHERE organization_id = $1
			AND ($2::text IS NULL OR (title ILIKE $2 OR reference ILIKE $2))
			AND ($3::text IS NULL OR title ILIKE $3)
			AND ($4::text IS NULL OR reference ILIKE $4)
			AND ($5::text IS NULL OR type = $5)
			AND ($6::uuid IS NULL OR vat_rate_id = $6)
			AND ($7::timestamptz IS NULL OR created_at >= $7)
			AND ($8::timestamptz IS NULL OR created_at <= $8)
			AND ($9::timestamptz IS NULL OR updated_at >= $9)
			AND ($10::timestamptz IS NULL OR updated_at <= $10)
	`

	var total int
	if err := r.pool.QueryRow(ctx, countQuery,
		params.OrganizationID,
		searchParam,
		titleParam,
		referenceParam,
		typeParam,
		vatRateParam,
		createdAtFrom,
		createdAtTo,
		updatedAtFrom,
		updatedAtTo,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	orderBy := fmt.Sprintf("%s %s, created_at DESC", sortBy, sortOrder)
	query := fmt.Sprintf(`
		SELECT id, organization_id, vat_rate_id, title, reference, description, price_cents, unit_price_cents, unit_label, labor_time_text, type, period_count, period_unit, created_at, updated_at
		FROM RAC_catalog_products
		WHERE organization_id = $1
			AND ($2::text IS NULL OR (title ILIKE $2 OR reference ILIKE $2))
			AND ($3::text IS NULL OR title ILIKE $3)
			AND ($4::text IS NULL OR reference ILIKE $4)
			AND ($5::text IS NULL OR type = $5)
			AND ($6::uuid IS NULL OR vat_rate_id = $6)
			AND ($7::timestamptz IS NULL OR created_at >= $7)
			AND ($8::timestamptz IS NULL OR created_at <= $8)
			AND ($9::timestamptz IS NULL OR updated_at >= $9)
			AND ($10::timestamptz IS NULL OR updated_at <= $10)
		ORDER BY %s
		LIMIT $11 OFFSET $12
	`, orderBy)

	rows, err := r.pool.Query(ctx, query,
		params.OrganizationID,
		searchParam,
		titleParam,
		referenceParam,
		typeParam,
		vatRateParam,
		createdAtFrom,
		createdAtTo,
		updatedAtFrom,
		updatedAtTo,
		params.Limit,
		params.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	items, err := scanProducts(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func scanProducts(rows pgx.Rows) ([]Product, error) {
	items := make([]Product, 0)
	for rows.Next() {
		var product Product
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&product.ID, &product.OrganizationID, &product.VatRateID, &product.Title, &product.Reference,
			&product.Description, &product.PriceCents, &product.UnitPriceCents, &product.UnitLabel, &product.LaborTimeText, &product.Type, &product.PeriodCount, &product.PeriodUnit,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		product.CreatedAt = createdAt.Format(time.RFC3339)
		product.UpdatedAt = updatedAt.Format(time.RFC3339)
		items = append(items, product)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate products: %w", rows.Err())
	}
	return items, nil
}

// GetProductsByIDs retrieves products by IDs within an organization.
func (r *Repo) GetProductsByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]Product, error) {
	query := `
		SELECT id, organization_id, vat_rate_id, title, reference, description, price_cents, unit_price_cents, unit_label, labor_time_text, type, period_count, period_unit, created_at, updated_at
		FROM RAC_catalog_products
		WHERE organization_id = $1 AND id = ANY($2)
	`

	rows, err := r.pool.Query(ctx, query, organizationID, ids)
	if err != nil {
		return nil, fmt.Errorf("get products by ids: %w", err)
	}
	defer rows.Close()

	items := make([]Product, 0)
	for rows.Next() {
		var product Product
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&product.ID, &product.OrganizationID, &product.VatRateID, &product.Title, &product.Reference,
			&product.Description, &product.PriceCents, &product.UnitPriceCents, &product.UnitLabel, &product.LaborTimeText, &product.Type, &product.PeriodCount, &product.PeriodUnit,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		product.CreatedAt = createdAt.Format(time.RFC3339)
		product.UpdatedAt = updatedAt.Format(time.RFC3339)
		items = append(items, product)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate products by ids: %w", rows.Err())
	}

	return items, nil
}

// AddProductMaterials adds materials to a product.
func (r *Repo) AddProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID, links []ProductMaterialLink) error {
	if len(links) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin add product materials tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	query := `
		INSERT INTO RAC_catalog_product_materials (organization_id, product_id, material_id, pricing_mode)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, product_id, material_id)
		DO UPDATE SET pricing_mode = EXCLUDED.pricing_mode`

	for _, link := range links {
		if _, err := tx.Exec(ctx, query, organizationID, productID, link.MaterialID, link.PricingMode); err != nil {
			return fmt.Errorf("add product material: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit add product materials tx: %w", err)
	}

	return nil
}

// RemoveProductMaterials removes materials from a product.
func (r *Repo) RemoveProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID, materialIDs []uuid.UUID) error {
	query := `
		DELETE FROM RAC_catalog_product_materials
		WHERE organization_id = $1 AND product_id = $2 AND material_id = ANY($3::uuid[])`

	if _, err := r.pool.Exec(ctx, query, organizationID, productID, materialIDs); err != nil {
		return fmt.Errorf("remove product materials: %w", err)
	}
	return nil
}

// ListProductMaterials lists materials for a product.
func (r *Repo) ListProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) ([]Product, error) {
	query := `
		SELECT p.id, p.organization_id, p.vat_rate_id, p.title, p.reference, p.description, p.price_cents, p.unit_price_cents, p.unit_label, p.labor_time_text, p.type, pm.pricing_mode, p.period_count, p.period_unit, p.created_at, p.updated_at
		FROM RAC_catalog_products p
		JOIN RAC_catalog_product_materials pm
		  ON pm.material_id = p.id AND pm.organization_id = p.organization_id
		WHERE pm.organization_id = $1 AND pm.product_id = $2
		ORDER BY p.title ASC`

	rows, err := r.pool.Query(ctx, query, organizationID, productID)
	if err != nil {
		return nil, fmt.Errorf("list product materials: %w", err)
	}
	defer rows.Close()

	items := make([]Product, 0)
	for rows.Next() {
		var product Product
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&product.ID, &product.OrganizationID, &product.VatRateID, &product.Title, &product.Reference,
			&product.Description, &product.PriceCents, &product.UnitPriceCents, &product.UnitLabel, &product.LaborTimeText, &product.Type, &product.PricingMode, &product.PeriodCount, &product.PeriodUnit,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan product material: %w", err)
		}
		product.CreatedAt = createdAt.Format(time.RFC3339)
		product.UpdatedAt = updatedAt.Format(time.RFC3339)
		items = append(items, product)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate product materials: %w", rows.Err())
	}

	return items, nil
}

// HasProductMaterials checks if a product has any materials linked.
func (r *Repo) HasProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM RAC_catalog_product_materials WHERE organization_id = $1 AND product_id = $2)`
	var exists bool
	if err := r.pool.QueryRow(ctx, query, organizationID, productID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check product materials: %w", err)
	}
	return exists, nil
}

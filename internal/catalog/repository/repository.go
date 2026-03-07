package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	catalogdb "portal_final_backend/internal/catalog/db"
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
	"isDraft":    "is_draft",
	"vatRateId":  "vat_rate_id",
	"createdAt":  "created_at",
	"updatedAt":  "updated_at",
}

// mapProductSortColumn returns the validated database sort column key.
func mapProductSortColumn(sortBy string) (string, error) {
	if sortBy == "" {
		return "createdAt", nil
	}
	if _, ok := productSortFields[sortBy]; ok {
		return sortBy, nil
	}
	return "", apperr.BadRequest("invalid sort field")
}

// mapVatRateSortColumn returns the validated database sort column key.
func mapVatRateSortColumn(sortBy string) (string, error) {
	if sortBy == "" {
		return "name", nil
	}
	switch sortBy {
	case "name", "rateBps", "createdAt", "updatedAt":
		return sortBy, nil
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
	pool    *pgxpool.Pool
	queries *catalogdb.Queries
}

// New creates a new catalog repository.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool, queries: catalogdb.New(pool)}
}

// Compile-time check that Repo implements Repository.
var _ Repository = (*Repo)(nil)

type catalogProductFields struct {
	ID             pgtype.UUID
	OrganizationID pgtype.UUID
	VatRateID      pgtype.UUID
	IsDraft        bool
	Title          string
	Reference      string
	Description    pgtype.Text
	PriceCents     int64
	UnitPriceCents int64
	UnitLabel      pgtype.Text
	LaborTimeText  pgtype.Text
	Type           string
	PricingMode    *string
	PeriodCount    pgtype.Int4
	PeriodUnit     pgtype.Text
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

// CreateVatRate creates a VAT rate.
func (r *Repo) CreateVatRate(ctx context.Context, params CreateVatRateParams) (VatRate, error) {
	row, err := r.queries.CreateVatRate(ctx, catalogdb.CreateVatRateParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		Name:           params.Name,
		RateBps:        int32(params.RateBps),
	})
	if err != nil {
		return VatRate{}, fmt.Errorf("create vat rate: %w", err)
	}
	return vatRateFromRow(row), nil
}

// UpdateVatRate updates a VAT rate.
func (r *Repo) UpdateVatRate(ctx context.Context, params UpdateVatRateParams) (VatRate, error) {
	row, err := r.queries.UpdateVatRate(ctx, catalogdb.UpdateVatRateParams{
		Name:           toPgText(params.Name),
		Ratebps:        toPgInt4(params.RateBps),
		ID:             toPgUUID(params.ID),
		Organizationid: toPgUUID(params.OrganizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return VatRate{}, apperr.NotFound(vatRateNotFoundMessage)
	}
	if err != nil {
		return VatRate{}, fmt.Errorf("update vat rate: %w", err)
	}
	return vatRateFromRow(row), nil
}

// DeleteVatRate deletes a VAT rate.
func (r *Repo) DeleteVatRate(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error {
	rowsAffected, err := r.queries.DeleteVatRate(ctx, catalogdb.DeleteVatRateParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return fmt.Errorf("delete vat rate: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(vatRateNotFoundMessage)
	}
	return nil
}

// GetVatRateByID retrieves a VAT rate by ID.
func (r *Repo) GetVatRateByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (VatRate, error) {
	row, err := r.queries.GetVatRateByID(ctx, catalogdb.GetVatRateByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return VatRate{}, apperr.NotFound(vatRateNotFoundMessage)
	}
	if err != nil {
		return VatRate{}, fmt.Errorf("get vat rate by id: %w", err)
	}
	return vatRateFromRow(row), nil
}

// ListVatRates lists VAT rates with filters and pagination.
func (r *Repo) ListVatRates(ctx context.Context, params ListVatRatesParams) ([]VatRate, int, error) {
	searchPattern := likePattern(params.Search)
	sortBy, err := mapVatRateSortColumn(params.SortBy)
	if err != nil {
		return nil, 0, err
	}
	sortOrder, err := mapSortOrder(params.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.queries.CountVatRates(ctx, catalogdb.CountVatRatesParams{
		Organizationid: toPgUUID(params.OrganizationID),
		Searchpattern:  searchPattern,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count vat rates: %w", err)
	}

	rows, err := r.queries.ListVatRates(ctx, catalogdb.ListVatRatesParams{
		Organizationid: toPgUUID(params.OrganizationID),
		Searchpattern:  searchPattern,
		Sortby:         sortBy,
		Sortorder:      sortOrder,
		Offsetcount:    int32(params.Offset),
		Limitcount:     int32(params.Limit),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list vat rates: %w", err)
	}

	items := make([]VatRate, 0, len(rows))
	for _, row := range rows {
		items = append(items, vatRateFromRow(row))
	}
	return items, int(total), nil
}

// HasProductsWithVatRate checks if any products reference a VAT rate.
func (r *Repo) HasProductsWithVatRate(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error) {
	exists, err := r.queries.HasProductsWithVatRate(ctx, catalogdb.HasProductsWithVatRateParams{
		VatRateID:      toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return false, fmt.Errorf("check vat rate usage: %w", err)
	}
	return exists, nil
}

// CreateProduct creates a product.
func (r *Repo) CreateProduct(ctx context.Context, params CreateProductParams) (Product, error) {
	row, err := r.queries.CreateProduct(ctx, catalogdb.CreateProductParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		VatRateID:      toPgUUID(params.VatRateID),
		IsDraft:        params.IsDraft,
		Title:          params.Title,
		Reference:      params.Reference,
		Description:    toPgText(params.Description),
		PriceCents:     params.PriceCents,
		UnitPriceCents: params.UnitPriceCents,
		UnitLabel:      toPgText(params.UnitLabel),
		LaborTimeText:  toPgText(params.LaborTimeText),
		Type:           params.Type,
		PeriodCount:    toPgInt4(params.PeriodCount),
		PeriodUnit:     toPgText(params.PeriodUnit),
	})
	if err != nil {
		return Product{}, fmt.Errorf("create product: %w", err)
	}
	return productFromFields(catalogProductFields{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		VatRateID:      row.VatRateID,
		IsDraft:        row.IsDraft,
		Title:          row.Title,
		Reference:      row.Reference,
		Description:    row.Description,
		PriceCents:     row.PriceCents,
		UnitPriceCents: row.UnitPriceCents,
		UnitLabel:      row.UnitLabel,
		LaborTimeText:  row.LaborTimeText,
		Type:           row.Type,
		PeriodCount:    row.PeriodCount,
		PeriodUnit:     row.PeriodUnit,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}), nil
}

// NextProductReference atomically generates the next product reference for an organization.
func (r *Repo) NextProductReference(ctx context.Context, organizationID uuid.UUID) (string, error) {
	nextNum, err := r.queries.GetNextProductCounter(ctx, toPgUUID(organizationID))
	if err != nil {
		return "", fmt.Errorf("generate next product reference: %w", err)
	}
	return fmt.Sprintf("SKU-%d-%04d", time.Now().Year(), nextNum), nil
}

// UpdateProduct updates a product.
func (r *Repo) UpdateProduct(ctx context.Context, params UpdateProductParams) (Product, error) {
	row, err := r.queries.UpdateProduct(ctx, catalogdb.UpdateProductParams{
		Vatrateid:      toPgUUIDPtr(params.VatRateID),
		Isdraft:        toPgBool(params.IsDraft),
		Title:          toPgText(params.Title),
		Reference:      toPgText(params.Reference),
		Description:    toPgText(params.Description),
		Pricecents:     toPgInt8(params.PriceCents),
		Unitpricecents: toPgInt8(params.UnitPriceCents),
		Unitlabel:      toPgText(params.UnitLabel),
		Labortimetext:  toPgText(params.LaborTimeText),
		Type:           toPgText(params.Type),
		Periodcount:    toPgInt4(params.PeriodCount),
		Periodunit:     toPgText(params.PeriodUnit),
		ID:             toPgUUID(params.ID),
		Organizationid: toPgUUID(params.OrganizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, apperr.NotFound(productNotFoundMessage)
	}
	if err != nil {
		return Product{}, fmt.Errorf("update product: %w", err)
	}
	return productFromFields(catalogProductFields{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		VatRateID:      row.VatRateID,
		IsDraft:        row.IsDraft,
		Title:          row.Title,
		Reference:      row.Reference,
		Description:    row.Description,
		PriceCents:     row.PriceCents,
		UnitPriceCents: row.UnitPriceCents,
		UnitLabel:      row.UnitLabel,
		LaborTimeText:  row.LaborTimeText,
		Type:           row.Type,
		PeriodCount:    row.PeriodCount,
		PeriodUnit:     row.PeriodUnit,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}), nil
}

// DeleteProduct deletes a product.
func (r *Repo) DeleteProduct(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error {
	rowsAffected, err := r.queries.DeleteProduct(ctx, catalogdb.DeleteProductParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(productNotFoundMessage)
	}
	return nil
}

// GetProductByID retrieves a product by ID.
func (r *Repo) GetProductByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (Product, error) {
	row, err := r.queries.GetProductByID(ctx, catalogdb.GetProductByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, apperr.NotFound(productNotFoundMessage)
	}
	if err != nil {
		return Product{}, fmt.Errorf("get product by id: %w", err)
	}
	return productFromFields(catalogProductFields{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		VatRateID:      row.VatRateID,
		IsDraft:        row.IsDraft,
		Title:          row.Title,
		Reference:      row.Reference,
		Description:    row.Description,
		PriceCents:     row.PriceCents,
		UnitPriceCents: row.UnitPriceCents,
		UnitLabel:      row.UnitLabel,
		LaborTimeText:  row.LaborTimeText,
		Type:           row.Type,
		PeriodCount:    row.PeriodCount,
		PeriodUnit:     row.PeriodUnit,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}), nil
}

// ListProducts lists products with filters and pagination.
func (r *Repo) ListProducts(ctx context.Context, params ListProductsParams) ([]Product, int, error) {
	sortBy, err := mapProductSortColumn(params.SortBy)
	if err != nil {
		return nil, 0, err
	}
	sortOrder, err := mapSortOrder(params.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	queryParams := catalogdb.ListProductsParams{
		Organizationid:   toPgUUID(params.OrganizationID),
		Searchpattern:    likePattern(params.Search),
		Titlepattern:     likePattern(params.Title),
		Referencepattern: likePattern(params.Reference),
		Producttype:      toPgText(nonEmptyPtr(params.Type)),
		Isdraft:          toPgBool(params.IsDraft),
		Vatrateid:        toPgUUIDPtr(params.VatRateID),
		Createdatfrom:    toPgTimestampPtr(params.CreatedAtFrom),
		Createdatto:      toPgTimestampPtr(params.CreatedAtTo),
		Updatedatfrom:    toPgTimestampPtr(params.UpdatedAtFrom),
		Updatedatto:      toPgTimestampPtr(params.UpdatedAtTo),
		Sortby:           sortBy,
		Sortorder:        sortOrder,
		Offsetcount:      int32(params.Offset),
		Limitcount:       int32(params.Limit),
	}

	total, err := r.queries.CountProducts(ctx, catalogdb.CountProductsParams{
		Organizationid:   queryParams.Organizationid,
		Searchpattern:    queryParams.Searchpattern,
		Titlepattern:     queryParams.Titlepattern,
		Referencepattern: queryParams.Referencepattern,
		Producttype:      queryParams.Producttype,
		Isdraft:          queryParams.Isdraft,
		Vatrateid:        queryParams.Vatrateid,
		Createdatfrom:    queryParams.Createdatfrom,
		Createdatto:      queryParams.Createdatto,
		Updatedatfrom:    queryParams.Updatedatfrom,
		Updatedatto:      queryParams.Updatedatto,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	rows, err := r.queries.ListProducts(ctx, queryParams)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}

	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		items = append(items, productFromFields(catalogProductFields{
			ID:             row.ID,
			OrganizationID: row.OrganizationID,
			VatRateID:      row.VatRateID,
			IsDraft:        row.IsDraft,
			Title:          row.Title,
			Reference:      row.Reference,
			Description:    row.Description,
			PriceCents:     row.PriceCents,
			UnitPriceCents: row.UnitPriceCents,
			UnitLabel:      row.UnitLabel,
			LaborTimeText:  row.LaborTimeText,
			Type:           row.Type,
			PeriodCount:    row.PeriodCount,
			PeriodUnit:     row.PeriodUnit,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}))
	}
	return items, int(total), nil
}

// GetProductsByIDs retrieves products by IDs within an organization.
func (r *Repo) GetProductsByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]Product, error) {
	rows, err := r.queries.GetProductsByIDs(ctx, catalogdb.GetProductsByIDsParams{
		Organizationid: toPgUUID(organizationID),
		Productids:     toPgUUIDSlice(ids),
	})
	if err != nil {
		return nil, fmt.Errorf("get products by ids: %w", err)
	}

	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		items = append(items, productFromFields(catalogProductFields{
			ID:             row.ID,
			OrganizationID: row.OrganizationID,
			VatRateID:      row.VatRateID,
			IsDraft:        row.IsDraft,
			Title:          row.Title,
			Reference:      row.Reference,
			Description:    row.Description,
			PriceCents:     row.PriceCents,
			UnitPriceCents: row.UnitPriceCents,
			UnitLabel:      row.UnitLabel,
			LaborTimeText:  row.LaborTimeText,
			Type:           row.Type,
			PeriodCount:    row.PeriodCount,
			PeriodUnit:     row.PeriodUnit,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}))
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

	queries := r.queries.WithTx(tx)
	for _, link := range links {
		if err := queries.UpsertProductMaterial(ctx, catalogdb.UpsertProductMaterialParams{
			OrganizationID: toPgUUID(organizationID),
			ProductID:      toPgUUID(productID),
			MaterialID:     toPgUUID(link.MaterialID),
			PricingMode:    link.PricingMode,
		}); err != nil {
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
	if err := r.queries.RemoveProductMaterials(ctx, catalogdb.RemoveProductMaterialsParams{
		OrganizationID: toPgUUID(organizationID),
		ProductID:      toPgUUID(productID),
		Column3:        toPgUUIDSlice(materialIDs),
	}); err != nil {
		return fmt.Errorf("remove product materials: %w", err)
	}
	return nil
}

// ListProductMaterials lists materials for a product.
func (r *Repo) ListProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) ([]Product, error) {
	rows, err := r.queries.ListProductMaterials(ctx, catalogdb.ListProductMaterialsParams{
		OrganizationID: toPgUUID(organizationID),
		ProductID:      toPgUUID(productID),
	})
	if err != nil {
		return nil, fmt.Errorf("list product materials: %w", err)
	}

	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		pricingMode := row.PricingMode
		items = append(items, productFromFields(catalogProductFields{
			ID:             row.ID,
			OrganizationID: row.OrganizationID,
			VatRateID:      row.VatRateID,
			IsDraft:        row.IsDraft,
			Title:          row.Title,
			Reference:      row.Reference,
			Description:    row.Description,
			PriceCents:     row.PriceCents,
			UnitPriceCents: row.UnitPriceCents,
			UnitLabel:      row.UnitLabel,
			LaborTimeText:  row.LaborTimeText,
			Type:           row.Type,
			PricingMode:    &pricingMode,
			PeriodCount:    row.PeriodCount,
			PeriodUnit:     row.PeriodUnit,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}))
	}
	return items, nil
}

// HasProductMaterials checks if a product has any materials linked.
func (r *Repo) HasProductMaterials(ctx context.Context, organizationID uuid.UUID, productID uuid.UUID) (bool, error) {
	exists, err := r.queries.HasProductMaterials(ctx, catalogdb.HasProductMaterialsParams{
		OrganizationID: toPgUUID(organizationID),
		ProductID:      toPgUUID(productID),
	})
	if err != nil {
		return false, fmt.Errorf("check product materials: %w", err)
	}
	return exists, nil
}

func vatRateFromRow(row catalogdb.RacCatalogVatRate) VatRate {
	return VatRate{
		ID:             row.ID.Bytes,
		OrganizationID: row.OrganizationID.Bytes,
		Name:           row.Name,
		RateBps:        int(row.RateBps),
		CreatedAt:      row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:      row.UpdatedAt.Time.Format(time.RFC3339),
	}
}

func productFromFields(fields catalogProductFields) Product {
	return Product{
		ID:             fields.ID.Bytes,
		OrganizationID: fields.OrganizationID.Bytes,
		VatRateID:      fields.VatRateID.Bytes,
		IsDraft:        fields.IsDraft,
		Title:          fields.Title,
		Reference:      fields.Reference,
		Description:    optionalString(fields.Description),
		PriceCents:     fields.PriceCents,
		UnitPriceCents: fields.UnitPriceCents,
		UnitLabel:      optionalString(fields.UnitLabel),
		LaborTimeText:  optionalString(fields.LaborTimeText),
		Type:           fields.Type,
		PricingMode:    fields.PricingMode,
		PeriodCount:    optionalInt(fields.PeriodCount),
		PeriodUnit:     optionalString(fields.PeriodUnit),
		CreatedAt:      fields.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:      fields.UpdatedAt.Time.Format(time.RFC3339),
	}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgUUIDSlice(ids []uuid.UUID) []pgtype.UUID {
	result := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		result = append(result, toPgUUID(id))
	}
	return result
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgInt4(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}

func toPgInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func toPgBool(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}

func toPgTimestampPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func likePattern(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + value + "%", Valid: true}
}

func nonEmptyPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func optionalInt(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int32)
	return &result
}

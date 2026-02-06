package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Domain Models ─────────────────────────────────────────────────────────────

// Quote is the database model for a quote header
type Quote struct {
	ID                  uuid.UUID  `db:"id"`
	OrganizationID      uuid.UUID  `db:"organization_id"`
	LeadID              uuid.UUID  `db:"lead_id"`
	LeadServiceID       *uuid.UUID `db:"lead_service_id"`
	QuoteNumber         string     `db:"quote_number"`
	Status              string     `db:"status"`
	PricingMode         string     `db:"pricing_mode"`
	DiscountType        string     `db:"discount_type"`
	DiscountValue       int64      `db:"discount_value"`
	SubtotalCents       int64      `db:"subtotal_cents"`
	DiscountAmountCents int64      `db:"discount_amount_cents"`
	TaxTotalCents       int64      `db:"tax_total_cents"`
	TotalCents          int64      `db:"total_cents"`
	ValidUntil          *time.Time `db:"valid_until"`
	Notes               *string    `db:"notes"`
	CreatedAt           time.Time  `db:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at"`
}

// QuoteItem is the database model for a quote line item
type QuoteItem struct {
	ID              uuid.UUID `db:"id"`
	QuoteID         uuid.UUID `db:"quote_id"`
	OrganizationID  uuid.UUID `db:"organization_id"`
	Description     string    `db:"description"`
	Quantity        string    `db:"quantity"`
	QuantityNumeric float64   `db:"quantity_numeric"`
	UnitPriceCents  int64     `db:"unit_price_cents"`
	TaxRateBps      int       `db:"tax_rate"`
	IsOptional      bool      `db:"is_optional"`
	SortOrder       int       `db:"sort_order"`
	CreatedAt       time.Time `db:"created_at"`
}

// ListParams contains parameters for listing quotes
type ListParams struct {
	OrganizationID uuid.UUID
	LeadID         *uuid.UUID
	Status         *string
	Search         string
	SortBy         string
	SortOrder      string
	Page           int
	PageSize       int
}

// ListResult contains the paginated result of listing quotes
type ListResult struct {
	Items      []Quote
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

// ── Repository ────────────────────────────────────────────────────────────────

const quoteNotFoundMsg = "quote not found"

// Repository provides database operations for quotes
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new quotes repository
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// NextQuoteNumber atomically generates the next quote number for an organization
func (r *Repository) NextQuoteNumber(ctx context.Context, orgID uuid.UUID) (string, error) {
	var nextNum int
	query := `
		INSERT INTO RAC_quote_counters (organization_id, last_number)
		VALUES ($1, 1)
		ON CONFLICT (organization_id) DO UPDATE SET last_number = RAC_quote_counters.last_number + 1
		RETURNING last_number`

	if err := r.pool.QueryRow(ctx, query, orgID).Scan(&nextNum); err != nil {
		return "", fmt.Errorf("failed to generate quote number: %w", err)
	}

	year := time.Now().Year()
	return fmt.Sprintf("OFF-%d-%04d", year, nextNum), nil
}

// CreateWithItems inserts a quote and its line items in a single transaction
func (r *Repository) CreateWithItems(ctx context.Context, quote *Quote, items []QuoteItem) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	quoteQuery := `
		INSERT INTO RAC_quotes (
			id, organization_id, lead_id, lead_service_id, quote_number, status,
			pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	if _, err := tx.Exec(ctx, quoteQuery,
		quote.ID, quote.OrganizationID, quote.LeadID, quote.LeadServiceID,
		quote.QuoteNumber, quote.Status, quote.PricingMode, quote.DiscountType, quote.DiscountValue,
		quote.SubtotalCents, quote.DiscountAmountCents, quote.TaxTotalCents, quote.TotalCents,
		quote.ValidUntil, quote.Notes, quote.CreatedAt, quote.UpdatedAt,
	); err != nil {
		return fmt.Errorf("failed to insert quote: %w", err)
	}

	if err := r.insertItems(ctx, tx, items); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateWithItems updates a quote and optionally replaces its line items
func (r *Repository) UpdateWithItems(ctx context.Context, quote *Quote, items []QuoteItem, replaceItems bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	updateQuery := `
		UPDATE RAC_quotes SET
			pricing_mode = $2, discount_type = $3, discount_value = $4,
			subtotal_cents = $5, discount_amount_cents = $6, tax_total_cents = $7, total_cents = $8,
			valid_until = $9, notes = $10, updated_at = $11
		WHERE id = $1 AND organization_id = $12`

	result, err := tx.Exec(ctx, updateQuery,
		quote.ID, quote.PricingMode, quote.DiscountType, quote.DiscountValue,
		quote.SubtotalCents, quote.DiscountAmountCents, quote.TaxTotalCents, quote.TotalCents,
		quote.ValidUntil, quote.Notes, quote.UpdatedAt, quote.OrganizationID,
	)
	if err != nil {
		return fmt.Errorf("failed to update quote: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}

	if replaceItems {
		// Delete existing items and insert new ones
		if _, err := tx.Exec(ctx, `DELETE FROM RAC_quote_items WHERE quote_id = $1 AND organization_id = $2`, quote.ID, quote.OrganizationID); err != nil {
			return fmt.Errorf("failed to delete old quote items: %w", err)
		}
		if err := r.insertItems(ctx, tx, items); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) insertItems(ctx context.Context, tx pgx.Tx, items []QuoteItem) error {
	itemQuery := `
		INSERT INTO RAC_quote_items (
			id, quote_id, organization_id, description, quantity, quantity_numeric,
			unit_price_cents, tax_rate, is_optional, sort_order, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	for _, item := range items {
		if _, err := tx.Exec(ctx, itemQuery,
			item.ID, item.QuoteID, item.OrganizationID,
			item.Description, item.Quantity, item.QuantityNumeric,
			item.UnitPriceCents, item.TaxRateBps, item.IsOptional, item.SortOrder, item.CreatedAt,
		); err != nil {
			return fmt.Errorf("failed to insert quote item: %w", err)
		}
	}
	return nil
}

// GetByID retrieves a quote by its ID scoped to organization
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*Quote, error) {
	var q Quote
	query := `
		SELECT id, organization_id, lead_id, lead_service_id, quote_number, status,
			pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, created_at, updated_at
		FROM RAC_quotes WHERE id = $1 AND organization_id = $2`

	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&q.ID, &q.OrganizationID, &q.LeadID, &q.LeadServiceID, &q.QuoteNumber, &q.Status,
		&q.PricingMode, &q.DiscountType, &q.DiscountValue,
		&q.SubtotalCents, &q.DiscountAmountCents, &q.TaxTotalCents, &q.TotalCents,
		&q.ValidUntil, &q.Notes, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteNotFoundMsg)
		}
		return nil, fmt.Errorf("failed to get quote: %w", err)
	}
	return &q, nil
}

// GetItemsByQuoteID retrieves all items for a quote
func (r *Repository) GetItemsByQuoteID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]QuoteItem, error) {
	query := `
		SELECT id, quote_id, organization_id, description, quantity, quantity_numeric,
			unit_price_cents, tax_rate, is_optional, sort_order, created_at
		FROM RAC_quote_items WHERE quote_id = $1 AND organization_id = $2
		ORDER BY sort_order ASC`

	rows, err := r.pool.Query(ctx, query, quoteID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query quote items: %w", err)
	}
	defer rows.Close()

	var items []QuoteItem
	for rows.Next() {
		var it QuoteItem
		if err := rows.Scan(
			&it.ID, &it.QuoteID, &it.OrganizationID,
			&it.Description, &it.Quantity, &it.QuantityNumeric,
			&it.UnitPriceCents, &it.TaxRateBps, &it.IsOptional, &it.SortOrder, &it.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan quote item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate quote items: %w", err)
	}
	return items, nil
}

// UpdateStatus updates the status of a quote
func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, orgID uuid.UUID, status string) error {
	query := `UPDATE RAC_quotes SET status = $3, updated_at = $4 WHERE id = $1 AND organization_id = $2`
	result, err := r.pool.Exec(ctx, query, id, orgID, status, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update quote status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}
	return nil
}

// Delete removes a quote (cascade deletes items)
func (r *Repository) Delete(ctx context.Context, id uuid.UUID, orgID uuid.UUID) error {
	query := `DELETE FROM RAC_quotes WHERE id = $1 AND organization_id = $2`
	result, err := r.pool.Exec(ctx, query, id, orgID)
	if err != nil {
		return fmt.Errorf("failed to delete quote: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}
	return nil
}

// List retrieves quotes with filtering and pagination
func (r *Repository) List(ctx context.Context, params ListParams) (*ListResult, error) {
	sortBy, err := resolveSortBy(params.SortBy)
	if err != nil {
		return nil, err
	}
	sortOrder, err := resolveSortOrder(params.SortOrder)
	if err != nil {
		return nil, err
	}

	var searchParam interface{}
	if params.Search != "" {
		searchParam = "%" + params.Search + "%"
	}

	var statusParam interface{}
	if params.Status != nil {
		statusParam = *params.Status
	}

	var leadParam interface{}
	if params.LeadID != nil {
		leadParam = *params.LeadID
	}

	baseQuery := `
		FROM RAC_quotes
		WHERE organization_id = $1
			AND ($2::uuid IS NULL OR lead_id = $2)
			AND ($3::text IS NULL OR status::text = $3)
			AND ($4::text IS NULL OR quote_number ILIKE $4 OR notes ILIKE $4)
	`
	args := []interface{}{params.OrganizationID, leadParam, statusParam, searchParam}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count quotes: %w", err)
	}

	totalPages := (total + params.PageSize - 1) / params.PageSize
	offset := (params.Page - 1) * params.PageSize

	selectQuery := `
		SELECT id, organization_id, lead_id, lead_service_id, quote_number, status,
			pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, created_at, updated_at
		` + baseQuery + `
		ORDER BY
			CASE WHEN $5 = 'quoteNumber' AND $6 = 'asc' THEN quote_number END ASC,
			CASE WHEN $5 = 'quoteNumber' AND $6 = 'desc' THEN quote_number END DESC,
			CASE WHEN $5 = 'status' AND $6 = 'asc' THEN status::text END ASC,
			CASE WHEN $5 = 'status' AND $6 = 'desc' THEN status::text END DESC,
			CASE WHEN $5 = 'total' AND $6 = 'asc' THEN total_cents END ASC,
			CASE WHEN $5 = 'total' AND $6 = 'desc' THEN total_cents END DESC,
			CASE WHEN $5 = 'createdAt' AND $6 = 'asc' THEN created_at END ASC,
			CASE WHEN $5 = 'createdAt' AND $6 = 'desc' THEN created_at END DESC,
			CASE WHEN $5 = 'updatedAt' AND $6 = 'asc' THEN updated_at END ASC,
			CASE WHEN $5 = 'updatedAt' AND $6 = 'desc' THEN updated_at END DESC,
			created_at DESC
		LIMIT $7 OFFSET $8`

	args = append(args, sortBy, sortOrder, params.PageSize, offset)

	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list quotes: %w", err)
	}
	defer rows.Close()

	var items []Quote
	for rows.Next() {
		var q Quote
		if err := rows.Scan(
			&q.ID, &q.OrganizationID, &q.LeadID, &q.LeadServiceID, &q.QuoteNumber, &q.Status,
			&q.PricingMode, &q.DiscountType, &q.DiscountValue,
			&q.SubtotalCents, &q.DiscountAmountCents, &q.TaxTotalCents, &q.TotalCents,
			&q.ValidUntil, &q.Notes, &q.CreatedAt, &q.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan quote: %w", err)
		}
		items = append(items, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate quotes: %w", err)
	}

	return &ListResult{
		Items:      items,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

func resolveSortBy(sortBy string) (string, error) {
	if sortBy == "" {
		return "createdAt", nil
	}
	switch sortBy {
	case "quoteNumber", "status", "total", "createdAt", "updatedAt":
		return sortBy, nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

func resolveSortOrder(sortOrder string) (string, error) {
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

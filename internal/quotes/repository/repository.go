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
	ID                         uuid.UUID  `db:"id"`
	OrganizationID             uuid.UUID  `db:"organization_id"`
	LeadID                     uuid.UUID  `db:"lead_id"`
	LeadServiceID              *uuid.UUID `db:"lead_service_id"`
	CreatedByID                *uuid.UUID `db:"created_by_id"`
	CreatedByFirstName         *string    `db:"created_by_first_name"`
	CreatedByLastName          *string    `db:"created_by_last_name"`
	CreatedByEmail             *string    `db:"created_by_email"`
	CustomerFirstName          *string    `db:"consumer_first_name"`
	CustomerLastName           *string    `db:"consumer_last_name"`
	CustomerPhone              *string    `db:"consumer_phone"`
	CustomerEmail              *string    `db:"consumer_email"`
	CustomerAddressStreet      *string    `db:"address_street"`
	CustomerAddressHouseNumber *string    `db:"address_house_number"`
	CustomerAddressZipCode     *string    `db:"address_zip_code"`
	CustomerAddressCity        *string    `db:"address_city"`
	QuoteNumber                string     `db:"quote_number"`
	Status                     string     `db:"status"`
	PricingMode                string     `db:"pricing_mode"`
	DiscountType               string     `db:"discount_type"`
	DiscountValue              int64      `db:"discount_value"`
	SubtotalCents              int64      `db:"subtotal_cents"`
	DiscountAmountCents        int64      `db:"discount_amount_cents"`
	TaxTotalCents              int64      `db:"tax_total_cents"`
	TotalCents                 int64      `db:"total_cents"`
	ValidUntil                 *time.Time `db:"valid_until"`
	Notes                      *string    `db:"notes"`
	PublicToken                *string    `db:"public_token"`
	PublicTokenExpAt           *time.Time `db:"public_token_expires_at"`
	PreviewToken               *string    `db:"preview_token"`
	PreviewTokenExpAt          *time.Time `db:"preview_token_expires_at"`
	ViewedAt                   *time.Time `db:"viewed_at"`
	AcceptedAt                 *time.Time `db:"accepted_at"`
	RejectedAt                 *time.Time `db:"rejected_at"`
	RejectionReason            *string    `db:"rejection_reason"`
	SignatureName              *string    `db:"signature_name"`
	SignatureData              *string    `db:"signature_data"`
	SignatureIP                *string    `db:"signature_ip"`
	PDFFileKey                 *string    `db:"pdf_file_key"`
	CreatedAt                  time.Time  `db:"created_at"`
	UpdatedAt                  time.Time  `db:"updated_at"`
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
	IsSelected      bool      `db:"is_selected"`
	SortOrder       int       `db:"sort_order"`
	CreatedAt       time.Time `db:"created_at"`
}

// QuoteAnnotation is the database model for a quote line item annotation
type QuoteAnnotation struct {
	ID             uuid.UUID  `db:"id"`
	QuoteItemID    uuid.UUID  `db:"quote_item_id"`
	OrganizationID uuid.UUID  `db:"organization_id"`
	AuthorType     string     `db:"author_type"`
	AuthorID       *uuid.UUID `db:"author_id"`
	Text           string     `db:"text"`
	IsResolved     bool       `db:"is_resolved"`
	CreatedAt      time.Time  `db:"created_at"`
}

// ListParams contains parameters for listing quotes
type ListParams struct {
	OrganizationID uuid.UUID
	LeadID         *uuid.UUID
	Status         *string
	Search         string
	CreatedAtFrom  *time.Time
	CreatedAtTo    *time.Time
	ValidUntilFrom *time.Time
	ValidUntilTo   *time.Time
	TotalFrom      *int64
	TotalTo        *int64
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

type quoteListFilters struct {
	leadID         interface{}
	status         interface{}
	search         interface{}
	createdAtFrom  interface{}
	createdAtTo    interface{}
	validUntilFrom interface{}
	validUntilTo   interface{}
	totalFrom      interface{}
	totalTo        interface{}
}

func buildQuoteListFilters(params ListParams) quoteListFilters {
	return quoteListFilters{
		leadID:         nullable(params.LeadID),
		status:         nullable(params.Status),
		search:         optionalSearchParam(params.Search),
		createdAtFrom:  nullable(params.CreatedAtFrom),
		createdAtTo:    nullable(params.CreatedAtTo),
		validUntilFrom: nullable(params.ValidUntilFrom),
		validUntilTo:   nullable(params.ValidUntilTo),
		totalFrom:      nullable(params.TotalFrom),
		totalTo:        nullable(params.TotalTo),
	}
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
			id, organization_id, lead_id, lead_service_id, created_by_id, quote_number, status,
			pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, created_at, updated_at,
			public_token, public_token_expires_at, preview_token, preview_token_expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`

	if _, err := tx.Exec(ctx, quoteQuery,
		quote.ID, quote.OrganizationID, quote.LeadID, quote.LeadServiceID, quote.CreatedByID,
		quote.QuoteNumber, quote.Status, quote.PricingMode, quote.DiscountType, quote.DiscountValue,
		quote.SubtotalCents, quote.DiscountAmountCents, quote.TaxTotalCents, quote.TotalCents,
		quote.ValidUntil, quote.Notes, quote.CreatedAt, quote.UpdatedAt,
		quote.PublicToken, quote.PublicTokenExpAt, quote.PreviewToken, quote.PreviewTokenExpAt,
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
			unit_price_cents, tax_rate, is_optional, is_selected, sort_order, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	for _, item := range items {
		if _, err := tx.Exec(ctx, itemQuery,
			item.ID, item.QuoteID, item.OrganizationID,
			item.Description, item.Quantity, item.QuantityNumeric,
			item.UnitPriceCents, item.TaxRateBps, item.IsOptional, item.IsSelected, item.SortOrder, item.CreatedAt,
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
		SELECT q.id, q.organization_id, q.lead_id, q.lead_service_id, q.created_by_id,
			u.first_name, u.last_name, u.email,
			q.quote_number, q.status,
			pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, q.created_at, q.updated_at,
			public_token, public_token_expires_at, preview_token, preview_token_expires_at,
			viewed_at, accepted_at, rejected_at,
			rejection_reason, signature_name, signature_data, signature_ip, pdf_file_key
		FROM RAC_quotes q
		LEFT JOIN RAC_users u ON u.id = q.created_by_id
		WHERE q.id = $1 AND q.organization_id = $2`

	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&q.ID, &q.OrganizationID, &q.LeadID, &q.LeadServiceID, &q.CreatedByID,
		&q.CreatedByFirstName, &q.CreatedByLastName, &q.CreatedByEmail,
		&q.QuoteNumber, &q.Status,
		&q.PricingMode, &q.DiscountType, &q.DiscountValue,
		&q.SubtotalCents, &q.DiscountAmountCents, &q.TaxTotalCents, &q.TotalCents,
		&q.ValidUntil, &q.Notes, &q.CreatedAt, &q.UpdatedAt,
		&q.PublicToken, &q.PublicTokenExpAt, &q.PreviewToken, &q.PreviewTokenExpAt,
		&q.ViewedAt, &q.AcceptedAt, &q.RejectedAt,
		&q.RejectionReason, &q.SignatureName, &q.SignatureData, &q.SignatureIP, &q.PDFFileKey,
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
			unit_price_cents, tax_rate, is_optional, is_selected, sort_order, created_at
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
			&it.UnitPriceCents, &it.TaxRateBps, &it.IsOptional, &it.IsSelected, &it.SortOrder, &it.CreatedAt,
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

	filters := buildQuoteListFilters(params)

	baseQuery := `
		FROM RAC_quotes q
		LEFT JOIN RAC_leads l ON l.id = q.lead_id AND l.organization_id = q.organization_id
		LEFT JOIN RAC_users u ON u.id = q.created_by_id
		WHERE q.organization_id = $1
			AND ($2::uuid IS NULL OR q.lead_id = $2)
			AND ($3::text IS NULL OR q.status::text = $3)
			AND ($4::text IS NULL OR (
				q.quote_number ILIKE $4 OR q.notes ILIKE $4
				OR l.consumer_first_name ILIKE $4 OR l.consumer_last_name ILIKE $4
				OR l.consumer_phone ILIKE $4 OR l.consumer_email ILIKE $4
				OR l.address_street ILIKE $4 OR l.address_house_number ILIKE $4
				OR l.address_zip_code ILIKE $4 OR l.address_city ILIKE $4
				OR u.first_name ILIKE $4 OR u.last_name ILIKE $4 OR u.email ILIKE $4
				OR EXISTS (
					SELECT 1
					FROM RAC_quote_items qi
					WHERE qi.quote_id = q.id AND qi.organization_id = q.organization_id
						AND qi.description ILIKE $4
				)
			))
			AND ($5::timestamptz IS NULL OR q.created_at >= $5)
			AND ($6::timestamptz IS NULL OR q.created_at < $6)
			AND ($7::timestamptz IS NULL OR q.valid_until >= $7)
			AND ($8::timestamptz IS NULL OR q.valid_until < $8)
			AND ($9::bigint IS NULL OR q.total_cents >= $9)
			AND ($10::bigint IS NULL OR q.total_cents <= $10)
	`
	args := []interface{}{
		params.OrganizationID,
		filters.leadID,
		filters.status,
		filters.search,
		filters.createdAtFrom,
		filters.createdAtTo,
		filters.validUntilFrom,
		filters.validUntilTo,
		filters.totalFrom,
		filters.totalTo,
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(DISTINCT q.id) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count quotes: %w", err)
	}

	totalPages := (total + params.PageSize - 1) / params.PageSize
	offset := (params.Page - 1) * params.PageSize

	selectQuery := `
		SELECT q.id, q.organization_id, q.lead_id, q.lead_service_id,
			q.created_by_id, u.first_name, u.last_name, u.email,
			l.consumer_first_name, l.consumer_last_name, l.consumer_phone, l.consumer_email,
			l.address_street, l.address_house_number, l.address_zip_code, l.address_city,
			q.quote_number, q.status, q.pricing_mode, q.discount_type, q.discount_value,
			q.subtotal_cents, q.discount_amount_cents, q.tax_total_cents, q.total_cents,
			q.valid_until, q.notes, q.created_at, q.updated_at,
			q.public_token, q.public_token_expires_at, q.preview_token, q.preview_token_expires_at,
			q.viewed_at, q.accepted_at, q.rejected_at,
			q.rejection_reason, q.signature_name, q.signature_data, q.signature_ip, q.pdf_file_key
		` + baseQuery + `
		ORDER BY
			CASE WHEN $11 = 'quoteNumber' AND $12 = 'asc' THEN q.quote_number END ASC,
			CASE WHEN $11 = 'quoteNumber' AND $12 = 'desc' THEN q.quote_number END DESC,
			CASE WHEN $11 = 'status' AND $12 = 'asc' THEN q.status::text END ASC,
			CASE WHEN $11 = 'status' AND $12 = 'desc' THEN q.status::text END DESC,
			CASE WHEN $11 = 'total' AND $12 = 'asc' THEN q.total_cents END ASC,
			CASE WHEN $11 = 'total' AND $12 = 'desc' THEN q.total_cents END DESC,
			CASE WHEN $11 = 'validUntil' AND $12 = 'asc' THEN q.valid_until END ASC,
			CASE WHEN $11 = 'validUntil' AND $12 = 'desc' THEN q.valid_until END DESC,
			CASE WHEN $11 = 'customerName' AND $12 = 'asc' THEN l.consumer_last_name END ASC,
			CASE WHEN $11 = 'customerName' AND $12 = 'desc' THEN l.consumer_last_name END DESC,
			CASE WHEN $11 = 'customerPhone' AND $12 = 'asc' THEN l.consumer_phone END ASC,
			CASE WHEN $11 = 'customerPhone' AND $12 = 'desc' THEN l.consumer_phone END DESC,
			CASE WHEN $11 = 'customerAddress' AND $12 = 'asc' THEN l.address_city END ASC,
			CASE WHEN $11 = 'customerAddress' AND $12 = 'desc' THEN l.address_city END DESC,
			CASE WHEN $11 = 'createdBy' AND $12 = 'asc' THEN u.last_name END ASC,
			CASE WHEN $11 = 'createdBy' AND $12 = 'desc' THEN u.last_name END DESC,
			CASE WHEN $11 = 'createdAt' AND $12 = 'asc' THEN q.created_at END ASC,
			CASE WHEN $11 = 'createdAt' AND $12 = 'desc' THEN q.created_at END DESC,
			CASE WHEN $11 = 'updatedAt' AND $12 = 'asc' THEN q.updated_at END ASC,
			CASE WHEN $11 = 'updatedAt' AND $12 = 'desc' THEN q.updated_at END DESC,
			q.created_at DESC
		LIMIT $13 OFFSET $14`

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
			&q.ID, &q.OrganizationID, &q.LeadID, &q.LeadServiceID,
			&q.CreatedByID, &q.CreatedByFirstName, &q.CreatedByLastName, &q.CreatedByEmail,
			&q.CustomerFirstName, &q.CustomerLastName, &q.CustomerPhone, &q.CustomerEmail,
			&q.CustomerAddressStreet, &q.CustomerAddressHouseNumber, &q.CustomerAddressZipCode, &q.CustomerAddressCity,
			&q.QuoteNumber, &q.Status, &q.PricingMode, &q.DiscountType, &q.DiscountValue,
			&q.SubtotalCents, &q.DiscountAmountCents, &q.TaxTotalCents, &q.TotalCents,
			&q.ValidUntil, &q.Notes, &q.CreatedAt, &q.UpdatedAt,
			&q.PublicToken, &q.PublicTokenExpAt, &q.PreviewToken, &q.PreviewTokenExpAt,
			&q.ViewedAt, &q.AcceptedAt, &q.RejectedAt,
			&q.RejectionReason, &q.SignatureName, &q.SignatureData, &q.SignatureIP, &q.PDFFileKey,
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

// ── Public Access Methods ─────────────────────────────────────────────────────

// GetByPublicToken retrieves a quote by its public token (no org scoping needed).
func (r *Repository) GetByPublicToken(ctx context.Context, token string) (*Quote, error) {
	var q Quote
	query := `
		SELECT id, organization_id, lead_id, lead_service_id, quote_number, status,
			pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, created_at, updated_at,
			public_token, public_token_expires_at, preview_token, preview_token_expires_at,
			viewed_at, accepted_at, rejected_at,
			rejection_reason, signature_name, signature_data, signature_ip, pdf_file_key
		FROM RAC_quotes WHERE public_token = $1`

	err := r.pool.QueryRow(ctx, query, token).Scan(
		&q.ID, &q.OrganizationID, &q.LeadID, &q.LeadServiceID, &q.QuoteNumber, &q.Status,
		&q.PricingMode, &q.DiscountType, &q.DiscountValue,
		&q.SubtotalCents, &q.DiscountAmountCents, &q.TaxTotalCents, &q.TotalCents,
		&q.ValidUntil, &q.Notes, &q.CreatedAt, &q.UpdatedAt,
		&q.PublicToken, &q.PublicTokenExpAt, &q.PreviewToken, &q.PreviewTokenExpAt,
		&q.ViewedAt, &q.AcceptedAt, &q.RejectedAt,
		&q.RejectionReason, &q.SignatureName, &q.SignatureData, &q.SignatureIP, &q.PDFFileKey,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteNotFoundMsg)
		}
		return nil, fmt.Errorf("failed to get quote by token: %w", err)
	}
	return &q, nil
}

// TokenKind describes which token matched a quote lookup.
type TokenKind string

const (
	TokenKindPublic  TokenKind = "public"
	TokenKindPreview TokenKind = "preview"
)

// GetByToken retrieves a quote by either public or preview token.
func (r *Repository) GetByToken(ctx context.Context, token string) (*Quote, TokenKind, error) {
	var q Quote
	var kind string
	query := `
		SELECT id, organization_id, lead_id, lead_service_id, quote_number, status,
			pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, created_at, updated_at,
			public_token, public_token_expires_at, preview_token, preview_token_expires_at,
			viewed_at, accepted_at, rejected_at,
			rejection_reason, signature_name, signature_data, signature_ip, pdf_file_key,
			CASE WHEN public_token = $1 THEN 'public' ELSE 'preview' END AS token_kind
		FROM RAC_quotes
		WHERE public_token = $1 OR preview_token = $1`

	err := r.pool.QueryRow(ctx, query, token).Scan(
		&q.ID, &q.OrganizationID, &q.LeadID, &q.LeadServiceID, &q.QuoteNumber, &q.Status,
		&q.PricingMode, &q.DiscountType, &q.DiscountValue,
		&q.SubtotalCents, &q.DiscountAmountCents, &q.TaxTotalCents, &q.TotalCents,
		&q.ValidUntil, &q.Notes, &q.CreatedAt, &q.UpdatedAt,
		&q.PublicToken, &q.PublicTokenExpAt, &q.PreviewToken, &q.PreviewTokenExpAt,
		&q.ViewedAt, &q.AcceptedAt, &q.RejectedAt,
		&q.RejectionReason, &q.SignatureName, &q.SignatureData, &q.SignatureIP, &q.PDFFileKey,
		&kind,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", apperr.NotFound(quoteNotFoundMsg)
		}
		return nil, "", fmt.Errorf("failed to get quote by token: %w", err)
	}

	if kind == string(TokenKindPublic) {
		return &q, TokenKindPublic, nil
	}
	return &q, TokenKindPreview, nil
}

// SetPublicToken sets the public access token and expiry on a quote.
func (r *Repository) SetPublicToken(ctx context.Context, quoteID, orgID uuid.UUID, token string, expiresAt time.Time) error {
	query := `UPDATE RAC_quotes SET public_token = $3, public_token_expires_at = $4, updated_at = $5
		WHERE id = $1 AND organization_id = $2`
	result, err := r.pool.Exec(ctx, query, quoteID, orgID, token, expiresAt, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set public token: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}
	return nil
}

// SetPreviewToken sets the read-only preview token and expiry on a quote.
func (r *Repository) SetPreviewToken(ctx context.Context, quoteID, orgID uuid.UUID, token string, expiresAt time.Time) error {
	query := `UPDATE RAC_quotes SET preview_token = $3, preview_token_expires_at = $4, updated_at = $5
		WHERE id = $1 AND organization_id = $2`
	result, err := r.pool.Exec(ctx, query, quoteID, orgID, token, expiresAt, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set preview token: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}
	return nil
}

// SetViewedAt sets the viewed_at timestamp if it's currently NULL (first view).
func (r *Repository) SetViewedAt(ctx context.Context, quoteID uuid.UUID) error {
	query := `UPDATE RAC_quotes SET viewed_at = $2 WHERE id = $1 AND viewed_at IS NULL`
	_, err := r.pool.Exec(ctx, query, quoteID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set viewed_at: %w", err)
	}
	return nil
}

// UpdateItemSelection updates the is_selected flag on a quote item.
func (r *Repository) UpdateItemSelection(ctx context.Context, itemID, quoteID uuid.UUID, isSelected bool) error {
	query := `UPDATE RAC_quote_items SET is_selected = $3 WHERE id = $1 AND quote_id = $2`
	result, err := r.pool.Exec(ctx, query, itemID, quoteID, isSelected)
	if err != nil {
		return fmt.Errorf("failed to update item selection: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound("quote item not found")
	}
	return nil
}

// UpdateQuoteTotals updates only the calculated totals on a quote.
func (r *Repository) UpdateQuoteTotals(ctx context.Context, quoteID uuid.UUID, subtotal, discount, tax, total int64) error {
	query := `UPDATE RAC_quotes SET subtotal_cents = $2, discount_amount_cents = $3, tax_total_cents = $4, total_cents = $5, updated_at = $6
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, quoteID, subtotal, discount, tax, total, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update quote totals: %w", err)
	}
	return nil
}

// AcceptQuote sets the quote to Accepted status with signature data.
func (r *Repository) AcceptQuote(ctx context.Context, quoteID uuid.UUID, signatureName, signatureData, signatureIP string) error {
	now := time.Now()
	query := `UPDATE RAC_quotes SET status = 'Accepted', accepted_at = $2, signature_name = $3, signature_data = $4, signature_ip = $5, updated_at = $2
		WHERE id = $1 AND status = 'Sent'`
	result, err := r.pool.Exec(ctx, query, quoteID, now, signatureName, signatureData, signatureIP)
	if err != nil {
		return fmt.Errorf("failed to accept quote: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.Conflict("quote cannot be accepted in its current state")
	}
	return nil
}

// RejectQuote sets the quote to Rejected status with an optional reason.
func (r *Repository) RejectQuote(ctx context.Context, quoteID uuid.UUID, reason *string) error {
	now := time.Now()
	query := `UPDATE RAC_quotes SET status = 'Rejected', rejected_at = $2, rejection_reason = $3, updated_at = $2
		WHERE id = $1 AND status = 'Sent'`
	result, err := r.pool.Exec(ctx, query, quoteID, now, reason)
	if err != nil {
		return fmt.Errorf("failed to reject quote: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.Conflict("quote cannot be rejected in its current state")
	}
	return nil
}

// SetPDFFileKey stores the MinIO reference for the generated PDF.
func (r *Repository) SetPDFFileKey(ctx context.Context, quoteID uuid.UUID, fileKey string) error {
	query := `UPDATE RAC_quotes SET pdf_file_key = $2, updated_at = $3 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, quoteID, fileKey, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set PDF file key: %w", err)
	}
	return nil
}

// GetPDFFileKeyByQuoteID returns the PDF file key for a quote (no org scoping).
func (r *Repository) GetPDFFileKeyByQuoteID(ctx context.Context, quoteID uuid.UUID) (string, error) {
	var fileKey *string
	query := `SELECT pdf_file_key FROM RAC_quotes WHERE id = $1`
	err := r.pool.QueryRow(ctx, query, quoteID).Scan(&fileKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperr.NotFound(quoteNotFoundMsg)
		}
		return "", fmt.Errorf("failed to get PDF file key: %w", err)
	}
	if fileKey == nil {
		return "", nil
	}
	return *fileKey, nil
}

// GetOrganizationIDByQuoteID returns the organization ID for a quote (no org scoping).
func (r *Repository) GetOrganizationIDByQuoteID(ctx context.Context, quoteID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	query := `SELECT organization_id FROM RAC_quotes WHERE id = $1`
	err := r.pool.QueryRow(ctx, query, quoteID).Scan(&orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, apperr.NotFound(quoteNotFoundMsg)
		}
		return uuid.Nil, fmt.Errorf("failed to get organization ID: %w", err)
	}
	return orgID, nil
}

// GetItemByID retrieves a single quote item by its ID and quote ID.
func (r *Repository) GetItemByID(ctx context.Context, itemID, quoteID uuid.UUID) (*QuoteItem, error) {
	var it QuoteItem
	query := `
		SELECT id, quote_id, organization_id, description, quantity, quantity_numeric,
			unit_price_cents, tax_rate, is_optional, is_selected, sort_order, created_at
		FROM RAC_quote_items WHERE id = $1 AND quote_id = $2`
	err := r.pool.QueryRow(ctx, query, itemID, quoteID).Scan(
		&it.ID, &it.QuoteID, &it.OrganizationID,
		&it.Description, &it.Quantity, &it.QuantityNumeric,
		&it.UnitPriceCents, &it.TaxRateBps, &it.IsOptional, &it.IsSelected, &it.SortOrder, &it.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("quote item not found")
		}
		return nil, fmt.Errorf("failed to get quote item: %w", err)
	}
	return &it, nil
}

// GetItemsByQuoteIDNoOrg retrieves all items for a quote without org scoping (for public access).
func (r *Repository) GetItemsByQuoteIDNoOrg(ctx context.Context, quoteID uuid.UUID) ([]QuoteItem, error) {
	query := `
		SELECT id, quote_id, organization_id, description, quantity, quantity_numeric,
			unit_price_cents, tax_rate, is_optional, is_selected, sort_order, created_at
		FROM RAC_quote_items WHERE quote_id = $1
		ORDER BY sort_order ASC`
	rows, err := r.pool.Query(ctx, query, quoteID)
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
			&it.UnitPriceCents, &it.TaxRateBps, &it.IsOptional, &it.IsSelected, &it.SortOrder, &it.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan quote item: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// ── Annotation Methods ────────────────────────────────────────────────────────

// CreateAnnotation inserts a new annotation on a quote item.
func (r *Repository) CreateAnnotation(ctx context.Context, a *QuoteAnnotation) error {
	query := `INSERT INTO RAC_quote_annotations (id, quote_item_id, organization_id, author_type, author_id, text, is_resolved, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.pool.Exec(ctx, query, a.ID, a.QuoteItemID, a.OrganizationID, a.AuthorType, a.AuthorID, a.Text, a.IsResolved, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create annotation: %w", err)
	}
	return nil
}

// ListAnnotationsByQuoteID retrieves all annotations for items belonging to a quote.
func (r *Repository) ListAnnotationsByQuoteID(ctx context.Context, quoteID uuid.UUID) ([]QuoteAnnotation, error) {
	query := `
		SELECT a.id, a.quote_item_id, a.organization_id, a.author_type, a.author_id, a.text, a.is_resolved, a.created_at
		FROM RAC_quote_annotations a
		JOIN RAC_quote_items qi ON qi.id = a.quote_item_id
		WHERE qi.quote_id = $1
		ORDER BY a.created_at ASC`
	rows, err := r.pool.Query(ctx, query, quoteID)
	if err != nil {
		return nil, fmt.Errorf("failed to list annotations: %w", err)
	}
	defer rows.Close()
	var annotations []QuoteAnnotation
	for rows.Next() {
		var a QuoteAnnotation
		if err := rows.Scan(&a.ID, &a.QuoteItemID, &a.OrganizationID, &a.AuthorType, &a.AuthorID, &a.Text, &a.IsResolved, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan annotation: %w", err)
		}
		annotations = append(annotations, a)
	}
	return annotations, rows.Err()
}

// UpdateAnnotationText updates the text for a single annotation (scoped to item and author type).
func (r *Repository) UpdateAnnotationText(ctx context.Context, annotationID, itemID uuid.UUID, authorType, text string) (*QuoteAnnotation, error) {
	query := `
		UPDATE RAC_quote_annotations
		SET text = $1
		WHERE id = $2 AND quote_item_id = $3 AND author_type = $4
		RETURNING id, quote_item_id, organization_id, author_type, author_id, text, is_resolved, created_at`

	var a QuoteAnnotation
	if err := r.pool.QueryRow(ctx, query, text, annotationID, itemID, authorType).Scan(
		&a.ID, &a.QuoteItemID, &a.OrganizationID, &a.AuthorType, &a.AuthorID, &a.Text, &a.IsResolved, &a.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("annotation not found")
		}
		return nil, fmt.Errorf("failed to update annotation: %w", err)
	}
	return &a, nil
}

// DeleteAnnotation removes an annotation scoped to item and author type.
func (r *Repository) DeleteAnnotation(ctx context.Context, annotationID, itemID uuid.UUID, authorType string) error {
	query := `DELETE FROM RAC_quote_annotations WHERE id = $1 AND quote_item_id = $2 AND author_type = $3`
	result, err := r.pool.Exec(ctx, query, annotationID, itemID, authorType)
	if err != nil {
		return fmt.Errorf("failed to delete annotation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound("annotation not found")
	}
	return nil
}

// ── Activity Methods ──────────────────────────────────────────────────────────

// QuoteActivity is the database model for a quote activity log entry
type QuoteActivity struct {
	ID             uuid.UUID `db:"id"`
	QuoteID        uuid.UUID `db:"quote_id"`
	OrganizationID uuid.UUID `db:"organization_id"`
	EventType      string    `db:"event_type"`
	Message        string    `db:"message"`
	Metadata       []byte    `db:"metadata"`
	CreatedAt      time.Time `db:"created_at"`
}

// CreateActivity inserts a new activity log entry for a quote.
func (r *Repository) CreateActivity(ctx context.Context, a *QuoteActivity) error {
	query := `INSERT INTO RAC_quote_activity (id, quote_id, organization_id, event_type, message, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query, a.ID, a.QuoteID, a.OrganizationID, a.EventType, a.Message, a.Metadata, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create quote activity: %w", err)
	}
	return nil
}

// ListActivities retrieves all activity log entries for a quote, newest first.
func (r *Repository) ListActivities(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]QuoteActivity, error) {
	query := `
		SELECT id, quote_id, organization_id, event_type, message, metadata, created_at
		FROM RAC_quote_activity
		WHERE quote_id = $1 AND organization_id = $2
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, quoteID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list quote activities: %w", err)
	}
	defer rows.Close()
	var activities []QuoteActivity
	for rows.Next() {
		var a QuoteActivity
		if err := rows.Scan(&a.ID, &a.QuoteID, &a.OrganizationID, &a.EventType, &a.Message, &a.Metadata, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan quote activity: %w", err)
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

// ── Sorting Helpers ───────────────────────────────────────────────────────────

func resolveSortBy(sortBy string) (string, error) {
	if sortBy == "" {
		return "createdAt", nil
	}
	switch sortBy {
	case "quoteNumber", "status", "total", "validUntil", "customerName", "customerPhone", "customerAddress", "createdBy", "createdAt", "updatedAt":
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

func nullable[T any](value *T) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func optionalSearchParam(value string) interface{} {
	if value == "" {
		return nil
	}
	return "%" + value + "%"
}

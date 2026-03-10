package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Quote is the database model for a quote header
type Quote struct {
	ID                         uuid.UUID  `db:"id"`
	OrganizationID             uuid.UUID  `db:"organization_id"`
	LeadID                     uuid.UUID  `db:"lead_id"`
	LeadServiceID              *uuid.UUID `db:"lead_service_id"`
	DuplicatedFromQuoteID      *uuid.UUID `db:"duplicated_from_quote_id"`
	PreviousVersionQuoteID     *uuid.UUID `db:"previous_version_quote_id"`
	VersionRootQuoteID         *uuid.UUID `db:"version_root_quote_id"`
	VersionNumber              int        `db:"version_number"`
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
	FinancingDisclaimer        bool       `db:"financing_disclaimer"`
	CreatedAt                  time.Time  `db:"created_at"`
	UpdatedAt                  time.Time  `db:"updated_at"`
}

// QuoteItem is the database model for a quote line item
type QuoteItem struct {
	ID               uuid.UUID  `db:"id"`
	QuoteID          uuid.UUID  `db:"quote_id"`
	OrganizationID   uuid.UUID  `db:"organization_id"`
	Title            string     `db:"title"`
	Description      string     `db:"description"`
	Quantity         string     `db:"quantity"`
	QuantityNumeric  float64    `db:"quantity_numeric"`
	UnitPriceCents   int64      `db:"unit_price_cents"`
	TaxRateBps       int        `db:"tax_rate"`
	IsOptional       bool       `db:"is_optional"`
	IsSelected       bool       `db:"is_selected"`
	SortOrder        int        `db:"sort_order"`
	CatalogProductID *uuid.UUID `db:"catalog_product_id"`
	CreatedAt        time.Time  `db:"created_at"`
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

// PendingApprovalItem is a lightweight row for dashboard draft approvals.
type PendingApprovalItem struct {
	QuoteID           uuid.UUID
	LeadID            uuid.UUID
	QuoteNumber       string
	ConsumerFirstName *string
	ConsumerLastName  *string
	TotalCents        int64
	LeadScore         *int
	CreatedAt         time.Time
}

// PendingApprovalsResult contains paginated draft approvals.
type PendingApprovalsResult struct {
	Items      []PendingApprovalItem
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

const (
	quoteNotFoundMsg            = "quote not found"
	quoteGenerateJobNotFoundMsg = "quote generate job not found"
	errScanQuoteItemFmt         = "failed to scan quote item: %w"
	errBeginTransactionFmt      = "failed to begin transaction: %w"
)

// Repository provides database operations for quotes
type Repository struct {
	pool    *pgxpool.Pool
	queries *quotesdb.Queries
}

// New creates a new quotes repository
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: quotesdb.New(pool)}
}

// NextQuoteNumber atomically generates the next quote number for an organization
func (r *Repository) NextQuoteNumber(ctx context.Context, orgID uuid.UUID) (string, error) {
	nextNum, err := r.queries.NextQuoteNumber(ctx, toPgUUID(orgID))
	if err != nil {
		return "", fmt.Errorf("failed to generate quote number: %w", err)
	}

	year := time.Now().Year()
	return fmt.Sprintf("OFF-%d-%04d", year, nextNum), nil
}

// CreateWithItems inserts a quote and its line items in a single transaction.
func (r *Repository) CreateWithItems(ctx context.Context, quote *Quote, items []QuoteItem, pricingSnapshot *QuotePricingSnapshot) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(errBeginTransactionFmt, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	if err := r.insertQuote(ctx, tx, quote); err != nil {
		return fmt.Errorf("failed to insert quote: %w", err)
	}

	if err := r.insertItems(ctx, qtx, items); err != nil {
		return err
	}
	if err := r.insertPricingSnapshot(ctx, qtx, quote, items, pricingSnapshot); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateWithItems updates a quote and optionally replaces its line items.
func (r *Repository) UpdateWithItems(ctx context.Context, quote *Quote, items []QuoteItem, replaceItems bool, pricingSnapshot *QuotePricingSnapshot) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(errBeginTransactionFmt, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	previousPricingSnapshot, err := r.getLatestPricingSnapshot(ctx, qtx, quote.ID, quote.OrganizationID)
	if err != nil {
		return err
	}
	rowsAffected, err := qtx.UpdateQuoteWithItems(ctx, quotesdb.UpdateQuoteWithItemsParams{
		ID:                  toPgUUID(quote.ID),
		PricingMode:         quote.PricingMode,
		DiscountType:        quote.DiscountType,
		DiscountValue:       quote.DiscountValue,
		SubtotalCents:       quote.SubtotalCents,
		DiscountAmountCents: quote.DiscountAmountCents,
		TaxTotalCents:       quote.TaxTotalCents,
		TotalCents:          quote.TotalCents,
		ValidUntil:          toPgTimestampPtr(quote.ValidUntil),
		Notes:               toPgTextPtr(quote.Notes),
		FinancingDisclaimer: quote.FinancingDisclaimer,
		UpdatedAt:           toPgTimestamp(quote.UpdatedAt),
		OrganizationID:      toPgUUID(quote.OrganizationID),
	})
	if err != nil {
		return fmt.Errorf("failed to update quote: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}

	if replaceItems {
		if err := qtx.DeleteQuoteItemsByQuote(ctx, quotesdb.DeleteQuoteItemsByQuoteParams{
			QuoteID:        toPgUUID(quote.ID),
			OrganizationID: toPgUUID(quote.OrganizationID),
		}); err != nil {
			return fmt.Errorf("failed to delete old quote items: %w", err)
		}
		if err := r.insertItems(ctx, qtx, items); err != nil {
			return err
		}
	}
	if err := r.insertPricingSnapshot(ctx, qtx, quote, items, pricingSnapshot); err != nil {
		return err
	}
	if err := r.insertPricingCorrections(ctx, qtx, quote, items, previousPricingSnapshot, pricingSnapshot); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) insertItems(ctx context.Context, queries *quotesdb.Queries, items []QuoteItem) error {
	for _, item := range items {
		if err := queries.CreateQuoteItem(ctx, quotesdb.CreateQuoteItemParams{
			ID:               toPgUUID(item.ID),
			QuoteID:          toPgUUID(item.QuoteID),
			OrganizationID:   toPgUUID(item.OrganizationID),
			Title:            item.Title,
			Description:      item.Description,
			Quantity:         item.Quantity,
			QuantityNumeric:  toPgNumericValue(item.QuantityNumeric),
			UnitPriceCents:   item.UnitPriceCents,
			TaxRate:          int32(item.TaxRateBps),
			IsOptional:       item.IsOptional,
			IsSelected:       item.IsSelected,
			SortOrder:        int32(item.SortOrder),
			CatalogProductID: toPgUUIDPtr(item.CatalogProductID),
			CreatedAt:        toPgTimestamp(item.CreatedAt),
		}); err != nil {
			return fmt.Errorf("failed to insert quote item: %w", err)
		}
	}
	return nil
}

// GetByID retrieves a quote by its ID scoped to organization
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*Quote, error) {
	row, err := r.queries.GetQuoteByID(ctx, quotesdb.GetQuoteByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(orgID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteNotFoundMsg)
		}
		return nil, fmt.Errorf("failed to get quote: %w", err)
	}
	quote := quoteFromGetByIDRow(row)
	if err := r.loadQuoteLineage(ctx, &quote); err != nil {
		return nil, err
	}
	return &quote, nil
}

func (r *Repository) GetQuoteNumberByID(ctx context.Context, id, orgID uuid.UUID) (*string, error) {
	var quoteNumber string
	err := r.pool.QueryRow(ctx, `
		SELECT quote_number
		FROM RAC_quotes
		WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&quoteNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get quote number: %w", err)
	}
	return &quoteNumber, nil
}

func (r *Repository) NextQuoteVersionNumber(ctx context.Context, orgID, rootQuoteID uuid.UUID) (int, error) {
	var nextVersion int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_number), 1) + 1
		FROM RAC_quotes
		WHERE organization_id = $1
		  AND (id = $2 OR version_root_quote_id = $2)
	`, orgID, rootQuoteID).Scan(&nextVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to compute next quote version number: %w", err)
	}
	return nextVersion, nil
}

// GetLatestNonDraftByLead returns the most recent non-draft quote for a lead.
func (r *Repository) GetLatestNonDraftByLead(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID) (*Quote, error) {
	row, err := r.queries.GetLatestNonDraftByLead(ctx, quotesdb.GetLatestNonDraftByLeadParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest non-draft quote: %w", err)
	}
	quote := quoteFromLatestNonDraftRow(row)
	return &quote, nil
}

// GetItemsByQuoteID retrieves all items for a quote
func (r *Repository) GetItemsByQuoteID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]QuoteItem, error) {
	rows, err := r.queries.ListQuoteItemsByQuoteID(ctx, quotesdb.ListQuoteItemsByQuoteIDParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query quote items: %w", err)
	}

	items := make([]QuoteItem, 0, len(rows))
	for _, row := range rows {
		item, mapErr := quoteItemFromListRow(row)
		if mapErr != nil {
			return nil, fmt.Errorf(errScanQuoteItemFmt, mapErr)
		}
		items = append(items, item)
	}
	return items, nil
}

// GetItemsByQuoteIDs retrieves all items for the provided quotes in a single query,
// grouped by quote_id and sorted by sort_order.
func (r *Repository) GetItemsByQuoteIDs(ctx context.Context, orgID uuid.UUID, quoteIDs []uuid.UUID) (map[uuid.UUID][]QuoteItem, error) {
	result := make(map[uuid.UUID][]QuoteItem, len(quoteIDs))
	if len(quoteIDs) == 0 {
		return result, nil
	}

	rows, err := r.queries.ListQuoteItemsByQuoteIDs(ctx, quotesdb.ListQuoteItemsByQuoteIDsParams{
		OrganizationID: toPgUUID(orgID),
		QuoteIds:       toPgUUIDSlice(quoteIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query quote items by quote ids: %w", err)
	}

	for _, row := range rows {
		item, mapErr := quoteItemFromListIDsRow(row)
		if mapErr != nil {
			return nil, fmt.Errorf(errScanQuoteItemFmt, mapErr)
		}
		result[item.QuoteID] = append(result[item.QuoteID], item)
	}

	return result, nil
}

// UpdateStatus updates the status of a quote
func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, orgID uuid.UUID, status string) error {
	rowsAffected, err := r.queries.UpdateQuoteStatus(ctx, quotesdb.UpdateQuoteStatusParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(orgID),
		Status:         quotesdb.QuoteStatus(status),
		UpdatedAt:      toPgTimestamp(time.Now()),
	})
	if err != nil {
		return fmt.Errorf("failed to update quote status: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}
	return nil
}

// SetLeadServiceID sets the lead_service_id for a quote, but only if the provided lead service
// belongs to the same lead as the quote (and same tenant).
func (r *Repository) SetLeadServiceID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID, leadServiceID uuid.UUID) error {
	rowsAffected, err := r.queries.SetQuoteLeadServiceID(ctx, quotesdb.SetQuoteLeadServiceIDParams{
		ID:             toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
		LeadServiceID:  toPgUUID(leadServiceID),
		UpdatedAt:      toPgTimestamp(time.Now()),
	})
	if err != nil {
		return fmt.Errorf("failed to set quote lead service: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.Validation("leadServiceId does not belong to this quote")
	}
	return nil
}

// ValidateLeadServiceID checks whether the provided lead service belongs to the
// same lead and organization as the target quote.
func (r *Repository) ValidateLeadServiceID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID, leadServiceID uuid.UUID) error {
	exists, err := r.queries.ValidateQuoteLeadServiceID(ctx, quotesdb.ValidateQuoteLeadServiceIDParams{
		ID:             toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
		ID_2:           toPgUUID(leadServiceID),
	})
	if err != nil {
		return fmt.Errorf("failed to validate quote lead service: %w", err)
	}
	if !exists {
		return apperr.Validation("leadServiceId does not belong to this quote")
	}
	return nil
}

// Delete removes a quote (cascade deletes items)
func (r *Repository) Delete(ctx context.Context, id uuid.UUID, orgID uuid.UUID) error {
	rowsAffected, err := r.queries.DeleteQuote(ctx, quotesdb.DeleteQuoteParams{ID: toPgUUID(id), OrganizationID: toPgUUID(orgID)})
	if err != nil {
		return fmt.Errorf("failed to delete quote: %w", err)
	}
	if rowsAffected == 0 {
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

	countParams := quotesdb.CountQuotesParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		LeadID:         toPgUUIDPtr(params.LeadID),
		Status:         toPgTextPtr(params.Status),
		Search:         searchText(params.Search),
		CreatedAtFrom:  toPgTimestampPtr(params.CreatedAtFrom),
		CreatedAtTo:    toPgTimestampPtr(params.CreatedAtTo),
		ValidUntilFrom: toPgTimestampPtr(params.ValidUntilFrom),
		ValidUntilTo:   toPgTimestampPtr(params.ValidUntilTo),
		TotalFrom:      toPgInt8Ptr(params.TotalFrom),
		TotalTo:        toPgInt8Ptr(params.TotalTo),
	}

	total, err := r.queries.CountQuotes(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("failed to count quotes: %w", err)
	}

	totalCount := int(total)
	totalPages := 0
	if params.PageSize > 0 {
		totalPages = (totalCount + params.PageSize - 1) / params.PageSize
	}
	offset := (params.Page - 1) * params.PageSize

	rows, err := r.queries.ListQuotes(ctx, quotesdb.ListQuotesParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		LeadID:         toPgUUIDPtr(params.LeadID),
		Status:         toPgTextPtr(params.Status),
		Search:         searchText(params.Search),
		CreatedAtFrom:  toPgTimestampPtr(params.CreatedAtFrom),
		CreatedAtTo:    toPgTimestampPtr(params.CreatedAtTo),
		ValidUntilFrom: toPgTimestampPtr(params.ValidUntilFrom),
		ValidUntilTo:   toPgTimestampPtr(params.ValidUntilTo),
		TotalFrom:      toPgInt8Ptr(params.TotalFrom),
		TotalTo:        toPgInt8Ptr(params.TotalTo),
		SortBy:         sortBy,
		SortOrder:      sortOrder,
		OffsetCount:    int32(offset),
		LimitCount:     int32(params.PageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list quotes: %w", err)
	}

	items := make([]Quote, 0, len(rows))
	for _, row := range rows {
		items = append(items, quoteFromListRow(row))
	}

	return &ListResult{
		Items:      items,
		Total:      totalCount,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

// ListPendingApprovals lists draft quotes for dashboard review queue.
func (r *Repository) ListPendingApprovals(ctx context.Context, orgID uuid.UUID, page int, pageSize int) (*PendingApprovalsResult, error) {
	total, err := r.queries.CountPendingApprovals(ctx, toPgUUID(orgID))
	if err != nil {
		return nil, fmt.Errorf("failed to count pending approvals: %w", err)
	}

	totalCount := int(total)
	totalPages := 0
	if totalCount > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}
	offset := (page - 1) * pageSize

	rows, err := r.queries.ListPendingApprovals(ctx, quotesdb.ListPendingApprovalsParams{
		OrganizationID: toPgUUID(orgID),
		Limit:          int32(pageSize),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pending approvals: %w", err)
	}

	items := make([]PendingApprovalItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PendingApprovalItem{
			QuoteID:           uuid.UUID(row.ID.Bytes),
			LeadID:            uuid.UUID(row.LeadID.Bytes),
			QuoteNumber:       row.QuoteNumber,
			ConsumerFirstName: optionalString(row.ConsumerFirstName),
			ConsumerLastName:  optionalString(row.ConsumerLastName),
			TotalCents:        row.TotalCents,
			LeadScore:         optionalInt(row.LeadScore),
			CreatedAt:         timeFromPg(row.CreatedAt),
		})
	}

	return &PendingApprovalsResult{
		Items:      items,
		Total:      totalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// TokenKind describes which token matched a quote lookup.
type TokenKind string

const (
	TokenKindPublic  TokenKind = "public"
	TokenKindPreview TokenKind = "preview"
)

// GetByPublicToken retrieves a quote by its public token (no org scoping needed).
func (r *Repository) GetByPublicToken(ctx context.Context, token string) (*Quote, error) {
	row, err := r.queries.GetQuoteByPublicToken(ctx, pgtype.Text{String: token, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteNotFoundMsg)
		}
		return nil, fmt.Errorf("failed to get quote by token: %w", err)
	}
	quote := quoteFromPublicTokenRow(row)
	return &quote, nil
}

// GetByToken retrieves a quote by either public or preview token.
func (r *Repository) GetByToken(ctx context.Context, token string) (*Quote, TokenKind, error) {
	row, err := r.queries.GetQuoteByToken(ctx, pgtype.Text{String: token, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", apperr.NotFound(quoteNotFoundMsg)
		}
		return nil, "", fmt.Errorf("failed to get quote by token: %w", err)
	}

	quote := quoteFromTokenRow(row)
	if row.TokenKind == string(TokenKindPublic) {
		return &quote, TokenKindPublic, nil
	}
	return &quote, TokenKindPreview, nil
}

// SetPublicToken sets the public access token and expiry on a quote.
func (r *Repository) SetPublicToken(ctx context.Context, quoteID, orgID uuid.UUID, token string, expiresAt time.Time) error {
	rowsAffected, err := r.queries.SetQuotePublicToken(ctx, quotesdb.SetQuotePublicTokenParams{
		ID:                   toPgUUID(quoteID),
		OrganizationID:       toPgUUID(orgID),
		PublicToken:          pgtype.Text{String: token, Valid: true},
		PublicTokenExpiresAt: toPgTimestamp(expiresAt),
		UpdatedAt:            toPgTimestamp(time.Now()),
	})
	if err != nil {
		return fmt.Errorf("failed to set public token: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}
	return nil
}

// SetPreviewToken sets the read-only preview token and expiry on a quote.
func (r *Repository) SetPreviewToken(ctx context.Context, quoteID, orgID uuid.UUID, token string, expiresAt time.Time) error {
	rowsAffected, err := r.queries.SetQuotePreviewToken(ctx, quotesdb.SetQuotePreviewTokenParams{
		ID:                    toPgUUID(quoteID),
		OrganizationID:        toPgUUID(orgID),
		PreviewToken:          pgtype.Text{String: token, Valid: true},
		PreviewTokenExpiresAt: toPgTimestamp(expiresAt),
		UpdatedAt:             toPgTimestamp(time.Now()),
	})
	if err != nil {
		return fmt.Errorf("failed to set preview token: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(quoteNotFoundMsg)
	}
	return nil
}

// SetViewedAt sets the viewed_at timestamp if it's currently NULL (first view).
func (r *Repository) SetViewedAt(ctx context.Context, quoteID uuid.UUID) error {
	if err := r.queries.SetQuoteViewedAt(ctx, quotesdb.SetQuoteViewedAtParams{
		ID:       toPgUUID(quoteID),
		ViewedAt: toPgTimestamp(time.Now()),
	}); err != nil {
		return fmt.Errorf("failed to set viewed_at: %w", err)
	}
	return nil
}

// UpdateItemSelection updates the is_selected flag on a quote item.
func (r *Repository) UpdateItemSelection(ctx context.Context, itemID, quoteID uuid.UUID, isSelected bool) error {
	rowsAffected, err := r.queries.UpdateQuoteItemSelection(ctx, quotesdb.UpdateQuoteItemSelectionParams{
		ID:         toPgUUID(itemID),
		QuoteID:    toPgUUID(quoteID),
		IsSelected: isSelected,
	})
	if err != nil {
		return fmt.Errorf("failed to update item selection: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound("quote item not found")
	}
	return nil
}

// UpdateQuoteTotals updates only the calculated totals on a quote.
func (r *Repository) UpdateQuoteTotals(ctx context.Context, quoteID uuid.UUID, subtotal, discount, tax, total int64) error {
	if err := r.queries.UpdateQuoteTotals(ctx, quotesdb.UpdateQuoteTotalsParams{
		ID:                  toPgUUID(quoteID),
		SubtotalCents:       subtotal,
		DiscountAmountCents: discount,
		TaxTotalCents:       tax,
		TotalCents:          total,
		UpdatedAt:           toPgTimestamp(time.Now()),
	}); err != nil {
		return fmt.Errorf("failed to update quote totals: %w", err)
	}
	return nil
}

// AcceptQuote sets the quote to Accepted status with signature data and records the pricing outcome.
func (r *Repository) AcceptQuote(ctx context.Context, quote *Quote, signatureName, signatureData, signatureIP string) error {
	now := time.Now()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(errBeginTransactionFmt, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	rowsAffected, err := qtx.AcceptQuote(ctx, quotesdb.AcceptQuoteParams{
		ID:            toPgUUID(quote.ID),
		AcceptedAt:    toPgTimestamp(now),
		SignatureName: pgtype.Text{String: signatureName, Valid: true},
		SignatureData: pgtype.Text{String: signatureData, Valid: true},
		SignatureIp:   pgtype.Text{String: signatureIP, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to accept quote: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.Conflict("quote cannot be accepted in its current state")
	}
	if err := r.insertPricingOutcome(ctx, qtx, quote, quotePricingOutcomeParams{
		OutcomeType:        "accepted",
		AcceptedTotalCents: &quote.TotalCents,
		FinalTotalCents:    &quote.TotalCents,
		OutcomeAt:          now,
		Metadata: map[string]any{
			"signatureName": signatureName,
			"signatureIP":   signatureIP,
		},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RejectQuote sets the quote to Rejected status with an optional reason and records the pricing outcome.
func (r *Repository) RejectQuote(ctx context.Context, quote *Quote, reason *string) error {
	now := time.Now()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(errBeginTransactionFmt, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	rowsAffected, err := qtx.RejectQuote(ctx, quotesdb.RejectQuoteParams{
		ID:              toPgUUID(quote.ID),
		RejectedAt:      toPgTimestamp(now),
		RejectionReason: toPgTextPtr(reason),
	})
	if err != nil {
		return fmt.Errorf("failed to reject quote: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.Conflict("quote cannot be rejected in its current state")
	}
	if err := r.insertPricingOutcome(ctx, qtx, quote, quotePricingOutcomeParams{
		OutcomeType:     "rejected",
		RejectionReason: reason,
		FinalTotalCents: &quote.TotalCents,
		OutcomeAt:       now,
		Metadata: map[string]any{
			"rejectionReason": reason,
		},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// SetPDFFileKey stores the MinIO reference for the generated PDF.
func (r *Repository) SetPDFFileKey(ctx context.Context, quoteID uuid.UUID, fileKey string) error {
	if err := r.queries.SetQuotePDFFileKey(ctx, quotesdb.SetQuotePDFFileKeyParams{
		ID:         toPgUUID(quoteID),
		PdfFileKey: pgtype.Text{String: fileKey, Valid: true},
		UpdatedAt:  toPgTimestamp(time.Now()),
	}); err != nil {
		return fmt.Errorf("failed to set PDF file key: %w", err)
	}
	return nil
}

// GetItemByID retrieves a single quote item by its ID and quote ID.
func (r *Repository) GetItemByID(ctx context.Context, itemID, quoteID uuid.UUID) (*QuoteItem, error) {
	row, err := r.queries.GetQuoteItemByID(ctx, quotesdb.GetQuoteItemByIDParams{ID: toPgUUID(itemID), QuoteID: toPgUUID(quoteID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("quote item not found")
		}
		return nil, fmt.Errorf("failed to get quote item: %w", err)
	}
	item, mapErr := quoteItemFromGetRow(row)
	if mapErr != nil {
		return nil, fmt.Errorf("failed to get quote item: %w", mapErr)
	}
	return &item, nil
}

// GetItemsByQuoteIDNoOrg retrieves all items for a quote without org scoping (for public access).
func (r *Repository) GetItemsByQuoteIDNoOrg(ctx context.Context, quoteID uuid.UUID) ([]QuoteItem, error) {
	rows, err := r.queries.ListQuoteItemsByQuoteIDNoOrg(ctx, toPgUUID(quoteID))
	if err != nil {
		return nil, fmt.Errorf("failed to query quote items: %w", err)
	}

	items := make([]QuoteItem, 0, len(rows))
	for _, row := range rows {
		item, mapErr := quoteItemFromNoOrgRow(row)
		if mapErr != nil {
			return nil, fmt.Errorf(errScanQuoteItemFmt, mapErr)
		}
		items = append(items, item)
	}
	return items, nil
}

// CreateAnnotation inserts a new annotation on a quote item.
func (r *Repository) CreateAnnotation(ctx context.Context, a *QuoteAnnotation) error {
	if err := r.queries.CreateQuoteAnnotation(ctx, quotesdb.CreateQuoteAnnotationParams{
		ID:             toPgUUID(a.ID),
		QuoteItemID:    toPgUUID(a.QuoteItemID),
		OrganizationID: toPgUUID(a.OrganizationID),
		AuthorType:     a.AuthorType,
		AuthorID:       toPgUUIDPtr(a.AuthorID),
		Text:           a.Text,
		IsResolved:     a.IsResolved,
		CreatedAt:      toPgTimestamp(a.CreatedAt),
	}); err != nil {
		return fmt.Errorf("failed to create annotation: %w", err)
	}
	return nil
}

// ListAnnotationsByQuoteID retrieves all annotations for items belonging to a quote.
func (r *Repository) ListAnnotationsByQuoteID(ctx context.Context, quoteID uuid.UUID) ([]QuoteAnnotation, error) {
	rows, err := r.queries.ListQuoteAnnotationsByQuoteID(ctx, toPgUUID(quoteID))
	if err != nil {
		return nil, fmt.Errorf("failed to list annotations: %w", err)
	}

	annotations := make([]QuoteAnnotation, 0, len(rows))
	for _, row := range rows {
		annotations = append(annotations, quoteAnnotationFromModel(row))
	}
	return annotations, nil
}

// UpdateAnnotationText updates the text for a single annotation (scoped to item and author type).
func (r *Repository) UpdateAnnotationText(ctx context.Context, annotationID, itemID uuid.UUID, authorType, text string) (*QuoteAnnotation, error) {
	row, err := r.queries.UpdateQuoteAnnotationText(ctx, quotesdb.UpdateQuoteAnnotationTextParams{
		Text:        text,
		ID:          toPgUUID(annotationID),
		QuoteItemID: toPgUUID(itemID),
		AuthorType:  authorType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("annotation not found")
		}
		return nil, fmt.Errorf("failed to update annotation: %w", err)
	}
	annotation := quoteAnnotationFromModel(row)
	return &annotation, nil
}

// DeleteAnnotation removes an annotation scoped to item and author type.
func (r *Repository) DeleteAnnotation(ctx context.Context, annotationID, itemID uuid.UUID, authorType string) error {
	rowsAffected, err := r.queries.DeleteQuoteAnnotation(ctx, quotesdb.DeleteQuoteAnnotationParams{
		ID:          toPgUUID(annotationID),
		QuoteItemID: toPgUUID(itemID),
		AuthorType:  authorType,
	})
	if err != nil {
		return fmt.Errorf("failed to delete annotation: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound("annotation not found")
	}
	return nil
}

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
	if err := r.queries.CreateQuoteActivity(ctx, quotesdb.CreateQuoteActivityParams{
		ID:             toPgUUID(a.ID),
		QuoteID:        toPgUUID(a.QuoteID),
		OrganizationID: toPgUUID(a.OrganizationID),
		EventType:      a.EventType,
		Message:        a.Message,
		Metadata:       a.Metadata,
		CreatedAt:      toPgTimestamp(a.CreatedAt),
	}); err != nil {
		return fmt.Errorf("failed to create quote activity: %w", err)
	}
	return nil
}

// ListActivities retrieves all activity log entries for a quote, newest first.
func (r *Repository) ListActivities(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]QuoteActivity, error) {
	rows, err := r.queries.ListQuoteActivities(ctx, quotesdb.ListQuoteActivitiesParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list quote activities: %w", err)
	}

	activities := make([]QuoteActivity, 0, len(rows))
	for _, row := range rows {
		activities = append(activities, quoteActivityFromModel(row))
	}
	return activities, nil
}

// QuoteAttachment is the database model for a quote document attachment.
type QuoteAttachment struct {
	ID               uuid.UUID  `db:"id"`
	QuoteID          uuid.UUID  `db:"quote_id"`
	OrganizationID   uuid.UUID  `db:"organization_id"`
	Filename         string     `db:"filename"`
	FileKey          string     `db:"file_key"`
	Source           string     `db:"source"`
	CatalogProductID *uuid.UUID `db:"catalog_product_id"`
	Enabled          bool       `db:"enabled"`
	SortOrder        int        `db:"sort_order"`
	CreatedAt        time.Time  `db:"created_at"`
}

// QuoteURL is the database model for a quote URL attachment.
type QuoteURL struct {
	ID               uuid.UUID  `db:"id"`
	QuoteID          uuid.UUID  `db:"quote_id"`
	OrganizationID   uuid.UUID  `db:"organization_id"`
	Label            string     `db:"label"`
	Href             string     `db:"href"`
	Accepted         bool       `db:"accepted"`
	CatalogProductID *uuid.UUID `db:"catalog_product_id"`
	CreatedAt        time.Time  `db:"created_at"`
}

// GenerateQuoteJob is the database model for async quote generation jobs.
type GenerateQuoteJob struct {
	ID                  uuid.UUID  `db:"id"`
	OrganizationID      uuid.UUID  `db:"organization_id"`
	UserID              uuid.UUID  `db:"user_id"`
	LeadID              uuid.UUID  `db:"lead_id"`
	LeadServiceID       uuid.UUID  `db:"lead_service_id"`
	Status              string     `db:"status"`
	Step                string     `db:"step"`
	ProgressPercent     int        `db:"progress_percent"`
	Error               *string    `db:"error"`
	QuoteID             *uuid.UUID `db:"quote_id"`
	QuoteNumber         *string    `db:"quote_number"`
	ItemCount           *int       `db:"item_count"`
	FeedbackRating      *int       `db:"feedback_rating"`
	FeedbackComment     *string    `db:"feedback_comment"`
	FeedbackSubmittedAt *time.Time `db:"feedback_submitted_at"`
	CancellationReason  *string    `db:"cancellation_reason"`
	ViewedAt            *time.Time `db:"viewed_at"`
	StartedAt           time.Time  `db:"started_at"`
	UpdatedAt           time.Time  `db:"updated_at"`
	FinishedAt          *time.Time `db:"finished_at"`
}

// CreateGenerateQuoteJob inserts a new async quote generation job row.
func (r *Repository) CreateGenerateQuoteJob(ctx context.Context, job *GenerateQuoteJob) error {
	if err := r.queries.CreateGenerateQuoteJob(ctx, quotesdb.CreateGenerateQuoteJobParams{
		ID:                  toPgUUID(job.ID),
		OrganizationID:      toPgUUID(job.OrganizationID),
		UserID:              toPgUUID(job.UserID),
		LeadID:              toPgUUID(job.LeadID),
		LeadServiceID:       toPgUUID(job.LeadServiceID),
		Status:              job.Status,
		Step:                job.Step,
		ProgressPercent:     int32(job.ProgressPercent),
		Error:               toPgTextPtr(job.Error),
		QuoteID:             toPgUUIDPtr(job.QuoteID),
		QuoteNumber:         toPgTextPtr(job.QuoteNumber),
		ItemCount:           toPgInt4Ptr(job.ItemCount),
		StartedAt:           toPgTimestamp(job.StartedAt),
		UpdatedAt:           toPgTimestamp(job.UpdatedAt),
		FinishedAt:          toPgTimestampPtr(job.FinishedAt),
		FeedbackRating:      toPgInt4Ptr(job.FeedbackRating),
		FeedbackComment:     toPgTextPtr(job.FeedbackComment),
		FeedbackSubmittedAt: toPgTimestampPtr(job.FeedbackSubmittedAt),
		CancellationReason:  toPgTextPtr(job.CancellationReason),
		ViewedAt:            toPgTimestampPtr(job.ViewedAt),
	}); err != nil {
		return fmt.Errorf("create generate quote job: %w", err)
	}
	return nil
}

// GetGenerateQuoteJob retrieves a job for a specific tenant + user.
func (r *Repository) GetGenerateQuoteJob(ctx context.Context, orgID, userID, jobID uuid.UUID) (*GenerateQuoteJob, error) {
	row, err := r.queries.GetGenerateQuoteJob(ctx, quotesdb.GetGenerateQuoteJobParams{
		ID:             toPgUUID(jobID),
		OrganizationID: toPgUUID(orgID),
		UserID:         toPgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteGenerateJobNotFoundMsg)
		}
		return nil, fmt.Errorf("get generate quote job: %w", err)
	}
	job := generateQuoteJobFromModel(row)
	return &job, nil
}

// ListGenerateQuoteJobs lists jobs for a tenant + user (newest first).
func (r *Repository) ListGenerateQuoteJobs(ctx context.Context, orgID, userID uuid.UUID, limit, offset int) ([]GenerateQuoteJob, int, error) {
	if orgID == uuid.Nil || userID == uuid.Nil {
		return nil, 0, apperr.Validation("organizationId and userId are required")
	}

	total, err := r.queries.CountGenerateQuoteJobs(ctx, quotesdb.CountGenerateQuoteJobsParams{
		OrganizationID: toPgUUID(orgID),
		UserID:         toPgUUID(userID),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count generate quote jobs: %w", err)
	}

	rows, err := r.queries.ListGenerateQuoteJobs(ctx, quotesdb.ListGenerateQuoteJobsParams{
		OrganizationID: toPgUUID(orgID),
		UserID:         toPgUUID(userID),
		Limit:          int32(limit),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list generate quote jobs: %w", err)
	}

	items := make([]GenerateQuoteJob, 0, len(rows))
	for _, row := range rows {
		items = append(items, generateQuoteJobFromModel(row))
	}

	return items, int(total), nil
}

// DeleteGenerateQuoteJob deletes a finished job (completed/failed/cancelled) for a tenant + user.
func (r *Repository) DeleteGenerateQuoteJob(ctx context.Context, orgID, userID, jobID uuid.UUID) error {
	if orgID == uuid.Nil || userID == uuid.Nil || jobID == uuid.Nil {
		return apperr.Validation("organizationId, userId and jobId are required")
	}

	rowsAffected, err := r.queries.DeleteGenerateQuoteJob(ctx, quotesdb.DeleteGenerateQuoteJobParams{
		ID:             toPgUUID(jobID),
		OrganizationID: toPgUUID(orgID),
		UserID:         toPgUUID(userID),
	})
	if err != nil {
		return fmt.Errorf("delete generate quote job: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(quoteGenerateJobNotFoundMsg)
	}
	return nil
}

// CancelGenerateQuoteJob transitions an active job to cancelled for a tenant + user.
func (r *Repository) CancelGenerateQuoteJob(ctx context.Context, orgID, userID, jobID uuid.UUID, updatedAt time.Time, finishedAt time.Time, cancellationReason *string) (*GenerateQuoteJob, error) {
	if orgID == uuid.Nil || userID == uuid.Nil || jobID == uuid.Nil {
		return nil, apperr.Validation("organizationId, userId and jobId are required")
	}

	row, err := r.queries.CancelGenerateQuoteJob(ctx, quotesdb.CancelGenerateQuoteJobParams{
		ID:                 toPgUUID(jobID),
		OrganizationID:     toPgUUID(orgID),
		UserID:             toPgUUID(userID),
		UpdatedAt:          toPgTimestamp(updatedAt),
		FinishedAt:         toPgTimestamp(finishedAt),
		CancellationReason: toPgTextPtr(cancellationReason),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteGenerateJobNotFoundMsg)
		}
		return nil, fmt.Errorf("cancel generate quote job: %w", err)
	}
	job := generateQuoteJobFromModel(row)
	return &job, nil
}

func (r *Repository) SubmitGenerateQuoteJobFeedback(ctx context.Context, orgID, userID, jobID uuid.UUID, rating int, comment *string, submittedAt time.Time) (*GenerateQuoteJob, error) {
	ratingValue := rating
	row, err := r.queries.SubmitGenerateQuoteJobFeedback(ctx, quotesdb.SubmitGenerateQuoteJobFeedbackParams{
		ID:                  toPgUUID(jobID),
		OrganizationID:      toPgUUID(orgID),
		UserID:              toPgUUID(userID),
		FeedbackRating:      toPgInt4Ptr(&ratingValue),
		FeedbackComment:     toPgTextPtr(comment),
		FeedbackSubmittedAt: toPgTimestamp(submittedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteGenerateJobNotFoundMsg)
		}
		return nil, fmt.Errorf("submit generate quote job feedback: %w", err)
	}
	job := generateQuoteJobFromModel(row)
	return &job, nil
}

func (r *Repository) MarkGenerateQuoteJobViewed(ctx context.Context, orgID, userID, jobID uuid.UUID, viewedAt time.Time) (*GenerateQuoteJob, error) {
	row, err := r.queries.MarkGenerateQuoteJobViewed(ctx, quotesdb.MarkGenerateQuoteJobViewedParams{
		ID:             toPgUUID(jobID),
		OrganizationID: toPgUUID(orgID),
		UserID:         toPgUUID(userID),
		ViewedAt:       toPgTimestamp(viewedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteGenerateJobNotFoundMsg)
		}
		return nil, fmt.Errorf("mark generate quote job viewed: %w", err)
	}
	job := generateQuoteJobFromModel(row)
	return &job, nil
}

// DeleteCompletedGenerateQuoteJobs deletes completed jobs for a tenant + user.
func (r *Repository) DeleteCompletedGenerateQuoteJobs(ctx context.Context, orgID, userID uuid.UUID) (int64, error) {
	if orgID == uuid.Nil || userID == uuid.Nil {
		return 0, apperr.Validation("organizationId and userId are required")
	}

	rowsAffected, err := r.queries.DeleteCompletedGenerateQuoteJobs(ctx, quotesdb.DeleteCompletedGenerateQuoteJobsParams{
		OrganizationID: toPgUUID(orgID),
		UserID:         toPgUUID(userID),
	})
	if err != nil {
		return 0, fmt.Errorf("delete completed generate quote jobs: %w", err)
	}
	return rowsAffected, nil
}

// GetGenerateQuoteJobByID retrieves a job by id without user scoping (worker use).
func (r *Repository) GetGenerateQuoteJobByID(ctx context.Context, jobID uuid.UUID) (*GenerateQuoteJob, error) {
	row, err := r.queries.GetGenerateQuoteJobByID(ctx, toPgUUID(jobID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(quoteGenerateJobNotFoundMsg)
		}
		return nil, fmt.Errorf("get generate quote job by id: %w", err)
	}
	job := generateQuoteJobFromModel(row)
	return &job, nil
}

// ClaimGenerateQuoteJob atomically transitions a pending job to running.
// Returns nil,nil when the job cannot be claimed (already claimed/finished).
func (r *Repository) ClaimGenerateQuoteJob(ctx context.Context, jobID uuid.UUID, step string, progressPercent int, updatedAt time.Time) (*GenerateQuoteJob, error) {
	row, err := r.queries.ClaimGenerateQuoteJob(ctx, quotesdb.ClaimGenerateQuoteJobParams{
		ID:              toPgUUID(jobID),
		Step:            step,
		ProgressPercent: int32(progressPercent),
		UpdatedAt:       toPgTimestamp(updatedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim generate quote job: %w", err)
	}
	job := generateQuoteJobFromModel(row)
	return &job, nil
}

// UpdateGenerateQuoteJob updates mutable job fields.
func (r *Repository) UpdateGenerateQuoteJob(ctx context.Context, job *GenerateQuoteJob) error {
	rowsAffected, err := r.queries.UpdateGenerateQuoteJob(ctx, quotesdb.UpdateGenerateQuoteJobParams{
		ID:              toPgUUID(job.ID),
		Status:          job.Status,
		Step:            job.Step,
		ProgressPercent: int32(job.ProgressPercent),
		Error:           toPgTextPtr(job.Error),
		QuoteID:         toPgUUIDPtr(job.QuoteID),
		QuoteNumber:     toPgTextPtr(job.QuoteNumber),
		ItemCount:       toPgInt4Ptr(job.ItemCount),
		UpdatedAt:       toPgTimestamp(job.UpdatedAt),
		FinishedAt:      toPgTimestampPtr(job.FinishedAt),
	})
	if err != nil {
		return fmt.Errorf("update generate quote job: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(quoteGenerateJobNotFoundMsg)
	}
	return nil
}

// DeleteFinishedGenerateQuoteJobsBefore deletes completed/failed jobs older than retention cutoffs.
func (r *Repository) DeleteFinishedGenerateQuoteJobsBefore(ctx context.Context, completedBefore, failedBefore time.Time) (int64, error) {
	rowsAffected, err := r.queries.DeleteFinishedGenerateQuoteJobsBefore(ctx, quotesdb.DeleteFinishedGenerateQuoteJobsBeforeParams{
		FinishedAt:   toPgTimestamp(completedBefore),
		FinishedAt_2: toPgTimestamp(failedBefore),
	})
	if err != nil {
		return 0, fmt.Errorf("delete finished generate quote jobs: %w", err)
	}
	return rowsAffected, nil
}

// ReplaceAttachments atomically replaces all attachments for a quote (delete + insert).
func (r *Repository) ReplaceAttachments(ctx context.Context, quoteID, orgID uuid.UUID, attachments []QuoteAttachment) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	if err := qtx.DeleteQuoteAttachmentsByQuote(ctx, quotesdb.DeleteQuoteAttachmentsByQuoteParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	}); err != nil {
		return fmt.Errorf("delete old attachments: %w", err)
	}

	for _, attachment := range attachments {
		if err := qtx.CreateQuoteAttachment(ctx, quotesdb.CreateQuoteAttachmentParams{
			ID:               toPgUUID(attachment.ID),
			QuoteID:          toPgUUID(attachment.QuoteID),
			OrganizationID:   toPgUUID(attachment.OrganizationID),
			Filename:         attachment.Filename,
			FileKey:          attachment.FileKey,
			Source:           quotesdb.RacQuoteAttachmentSource(attachment.Source),
			CatalogProductID: toPgUUIDPtr(attachment.CatalogProductID),
			Enabled:          attachment.Enabled,
			SortOrder:        int32(attachment.SortOrder),
			CreatedAt:        toPgTimestamp(attachment.CreatedAt),
		}); err != nil {
			return fmt.Errorf("insert attachment: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) insertQuote(ctx context.Context, tx pgx.Tx, quote *Quote) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO RAC_quotes (
			id, organization_id, lead_id, lead_service_id, duplicated_from_quote_id,
			previous_version_quote_id, version_root_quote_id, version_number,
			created_by_id, quote_number, status, pricing_mode, discount_type, discount_value,
			subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
			valid_until, notes, financing_disclaimer, created_at, updated_at,
			public_token, public_token_expires_at, preview_token, preview_token_expires_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18,
			$19, $20, $21, $22, $23,
			$24, $25, $26, $27
		)
	`,
		toPgUUID(quote.ID),
		toPgUUID(quote.OrganizationID),
		toPgUUID(quote.LeadID),
		toPgUUIDPtr(quote.LeadServiceID),
		toPgUUIDPtr(quote.DuplicatedFromQuoteID),
		toPgUUIDPtr(quote.PreviousVersionQuoteID),
		toPgUUIDPtr(quote.VersionRootQuoteID),
		quote.VersionNumber,
		toPgUUIDPtr(quote.CreatedByID),
		quote.QuoteNumber,
		quote.Status,
		quote.PricingMode,
		quote.DiscountType,
		quote.DiscountValue,
		quote.SubtotalCents,
		quote.DiscountAmountCents,
		quote.TaxTotalCents,
		quote.TotalCents,
		toPgTimestampPtr(quote.ValidUntil),
		toPgTextPtr(quote.Notes),
		quote.FinancingDisclaimer,
		toPgTimestamp(quote.CreatedAt),
		toPgTimestamp(quote.UpdatedAt),
		toPgTextPtr(quote.PublicToken),
		toPgTimestampPtr(quote.PublicTokenExpAt),
		toPgTextPtr(quote.PreviewToken),
		toPgTimestampPtr(quote.PreviewTokenExpAt),
	)
	return err
}

func (r *Repository) loadQuoteLineage(ctx context.Context, quote *Quote) error {
	var duplicatedFrom pgtype.UUID
	var previousVersion pgtype.UUID
	var versionRoot pgtype.UUID
	var versionNumber int32

	err := r.pool.QueryRow(ctx, `
		SELECT duplicated_from_quote_id, previous_version_quote_id, version_root_quote_id, version_number
		FROM RAC_quotes
		WHERE id = $1
	`, quote.ID).Scan(&duplicatedFrom, &previousVersion, &versionRoot, &versionNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound(quoteNotFoundMsg)
		}
		return fmt.Errorf("failed to load quote lineage: %w", err)
	}

	quote.DuplicatedFromQuoteID = optionalUUID(duplicatedFrom)
	quote.PreviousVersionQuoteID = optionalUUID(previousVersion)
	quote.VersionRootQuoteID = optionalUUID(versionRoot)
	quote.VersionNumber = int(versionNumber)
	if quote.VersionNumber < 1 {
		quote.VersionNumber = 1
	}
	return nil
}

// ReplaceURLs atomically replaces all URLs for a quote (delete + insert).
func (r *Repository) ReplaceURLs(ctx context.Context, quoteID, orgID uuid.UUID, urls []QuoteURL) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	if err := qtx.DeleteQuoteURLsByQuote(ctx, quotesdb.DeleteQuoteURLsByQuoteParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	}); err != nil {
		return fmt.Errorf("delete old urls: %w", err)
	}

	for _, quoteURL := range urls {
		if err := qtx.CreateQuoteURL(ctx, quotesdb.CreateQuoteURLParams{
			ID:               toPgUUID(quoteURL.ID),
			QuoteID:          toPgUUID(quoteURL.QuoteID),
			OrganizationID:   toPgUUID(quoteURL.OrganizationID),
			Label:            quoteURL.Label,
			Href:             quoteURL.Href,
			Accepted:         quoteURL.Accepted,
			CatalogProductID: toPgUUIDPtr(quoteURL.CatalogProductID),
			CreatedAt:        toPgTimestamp(quoteURL.CreatedAt),
		}); err != nil {
			return fmt.Errorf("insert url: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// GetAttachmentsByQuoteID retrieves all attachments for a quote ordered by sort_order.
func (r *Repository) GetAttachmentsByQuoteID(ctx context.Context, quoteID, orgID uuid.UUID) ([]QuoteAttachment, error) {
	rows, err := r.queries.ListQuoteAttachmentsByQuoteID(ctx, quotesdb.ListQuoteAttachmentsByQuoteIDParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}

	result := make([]QuoteAttachment, 0, len(rows))
	for _, row := range rows {
		result = append(result, quoteAttachmentFromModel(row))
	}
	return result, nil
}

// GetURLsByQuoteID retrieves all URLs for a quote.
func (r *Repository) GetURLsByQuoteID(ctx context.Context, quoteID, orgID uuid.UUID) ([]QuoteURL, error) {
	rows, err := r.queries.ListQuoteURLsByQuoteID(ctx, quotesdb.ListQuoteURLsByQuoteIDParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		return nil, fmt.Errorf("query urls: %w", err)
	}

	result := make([]QuoteURL, 0, len(rows))
	for _, row := range rows {
		result = append(result, quoteURLFromModel(row))
	}
	return result, nil
}

// GetAttachmentsByQuoteIDNoOrg retrieves all attachments for a quote without org scoping (for public/PDF access).
func (r *Repository) GetAttachmentsByQuoteIDNoOrg(ctx context.Context, quoteID uuid.UUID) ([]QuoteAttachment, error) {
	rows, err := r.queries.ListQuoteAttachmentsByQuoteIDNoOrg(ctx, toPgUUID(quoteID))
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}

	result := make([]QuoteAttachment, 0, len(rows))
	for _, row := range rows {
		result = append(result, quoteAttachmentFromModel(row))
	}
	return result, nil
}

// GetURLsByQuoteIDNoOrg retrieves all URLs for a quote without org scoping (for public access).
func (r *Repository) GetURLsByQuoteIDNoOrg(ctx context.Context, quoteID uuid.UUID) ([]QuoteURL, error) {
	rows, err := r.queries.ListQuoteURLsByQuoteIDNoOrg(ctx, toPgUUID(quoteID))
	if err != nil {
		return nil, fmt.Errorf("query urls: %w", err)
	}

	result := make([]QuoteURL, 0, len(rows))
	for _, row := range rows {
		result = append(result, quoteURLFromModel(row))
	}
	return result, nil
}

// GetAttachmentByID returns a single attachment by ID, scoped to quote + org.
func (r *Repository) GetAttachmentByID(ctx context.Context, attachmentID, quoteID, orgID uuid.UUID) (*QuoteAttachment, error) {
	row, err := r.queries.GetQuoteAttachmentByID(ctx, quotesdb.GetQuoteAttachmentByIDParams{
		ID:             toPgUUID(attachmentID),
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("attachment not found")
		}
		return nil, fmt.Errorf("get attachment by id: %w", err)
	}
	attachment := quoteAttachmentFromModel(row)
	return &attachment, nil
}

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

func searchText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + value + "%", Valid: true}
}

func quoteFromGetByIDRow(row quotesdb.GetQuoteByIDRow) Quote {
	return Quote{
		ID:                         uuid.UUID(row.ID.Bytes),
		OrganizationID:             uuid.UUID(row.OrganizationID.Bytes),
		LeadID:                     uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:              optionalUUID(row.LeadServiceID),
		VersionNumber:              1,
		CreatedByID:                optionalUUID(row.CreatedByID),
		CreatedByFirstName:         optionalString(row.FirstName),
		CreatedByLastName:          optionalString(row.LastName),
		CreatedByEmail:             optionalString(row.Email),
		CustomerFirstName:          optionalString(row.ConsumerFirstName),
		CustomerLastName:           optionalString(row.ConsumerLastName),
		CustomerPhone:              optionalString(row.ConsumerPhone),
		CustomerEmail:              optionalString(row.ConsumerEmail),
		CustomerAddressStreet:      optionalString(row.AddressStreet),
		CustomerAddressHouseNumber: optionalString(row.AddressHouseNumber),
		CustomerAddressZipCode:     optionalString(row.AddressZipCode),
		CustomerAddressCity:        optionalString(row.AddressCity),
		QuoteNumber:                row.QuoteNumber,
		Status:                     string(row.Status),
		PricingMode:                row.PricingMode,
		DiscountType:               row.DiscountType,
		DiscountValue:              row.DiscountValue,
		SubtotalCents:              row.SubtotalCents,
		DiscountAmountCents:        row.DiscountAmountCents,
		TaxTotalCents:              row.TaxTotalCents,
		TotalCents:                 row.TotalCents,
		ValidUntil:                 optionalTime(row.ValidUntil),
		Notes:                      optionalString(row.Notes),
		PublicToken:                optionalString(row.PublicToken),
		PublicTokenExpAt:           optionalTime(row.PublicTokenExpiresAt),
		PreviewToken:               optionalString(row.PreviewToken),
		PreviewTokenExpAt:          optionalTime(row.PreviewTokenExpiresAt),
		ViewedAt:                   optionalTime(row.ViewedAt),
		AcceptedAt:                 optionalTime(row.AcceptedAt),
		RejectedAt:                 optionalTime(row.RejectedAt),
		RejectionReason:            optionalString(row.RejectionReason),
		SignatureName:              optionalString(row.SignatureName),
		SignatureData:              optionalString(row.SignatureData),
		SignatureIP:                optionalString(row.SignatureIp),
		PDFFileKey:                 optionalString(row.PdfFileKey),
		FinancingDisclaimer:        row.FinancingDisclaimer,
		CreatedAt:                  timeFromPg(row.CreatedAt),
		UpdatedAt:                  timeFromPg(row.UpdatedAt),
	}
}

func quoteFromLatestNonDraftRow(row quotesdb.GetLatestNonDraftByLeadRow) Quote {
	return Quote{
		ID:             uuid.UUID(row.ID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		LeadID:         uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:  optionalUUID(row.LeadServiceID),
		VersionNumber:  1,
		QuoteNumber:    row.QuoteNumber,
		Status:         string(row.Status),
		TotalCents:     row.TotalCents,
		PublicToken:    optionalString(row.PublicToken),
		PDFFileKey:     optionalString(row.PdfFileKey),
	}
}

func quoteFromPublicTokenRow(row quotesdb.GetQuoteByPublicTokenRow) Quote {
	return Quote{
		ID:                  uuid.UUID(row.ID.Bytes),
		OrganizationID:      uuid.UUID(row.OrganizationID.Bytes),
		LeadID:              uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:       optionalUUID(row.LeadServiceID),
		VersionNumber:       1,
		QuoteNumber:         row.QuoteNumber,
		Status:              string(row.Status),
		PricingMode:         row.PricingMode,
		DiscountType:        row.DiscountType,
		DiscountValue:       row.DiscountValue,
		SubtotalCents:       row.SubtotalCents,
		DiscountAmountCents: row.DiscountAmountCents,
		TaxTotalCents:       row.TaxTotalCents,
		TotalCents:          row.TotalCents,
		ValidUntil:          optionalTime(row.ValidUntil),
		Notes:               optionalString(row.Notes),
		PublicToken:         optionalString(row.PublicToken),
		PublicTokenExpAt:    optionalTime(row.PublicTokenExpiresAt),
		PreviewToken:        optionalString(row.PreviewToken),
		PreviewTokenExpAt:   optionalTime(row.PreviewTokenExpiresAt),
		ViewedAt:            optionalTime(row.ViewedAt),
		AcceptedAt:          optionalTime(row.AcceptedAt),
		RejectedAt:          optionalTime(row.RejectedAt),
		RejectionReason:     optionalString(row.RejectionReason),
		SignatureName:       optionalString(row.SignatureName),
		SignatureData:       optionalString(row.SignatureData),
		SignatureIP:         optionalString(row.SignatureIp),
		PDFFileKey:          optionalString(row.PdfFileKey),
		FinancingDisclaimer: row.FinancingDisclaimer,
		CreatedAt:           timeFromPg(row.CreatedAt),
		UpdatedAt:           timeFromPg(row.UpdatedAt),
	}
}

func quoteFromTokenRow(row quotesdb.GetQuoteByTokenRow) Quote {
	return Quote{
		ID:                  uuid.UUID(row.ID.Bytes),
		OrganizationID:      uuid.UUID(row.OrganizationID.Bytes),
		LeadID:              uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:       optionalUUID(row.LeadServiceID),
		VersionNumber:       1,
		QuoteNumber:         row.QuoteNumber,
		Status:              string(row.Status),
		PricingMode:         row.PricingMode,
		DiscountType:        row.DiscountType,
		DiscountValue:       row.DiscountValue,
		SubtotalCents:       row.SubtotalCents,
		DiscountAmountCents: row.DiscountAmountCents,
		TaxTotalCents:       row.TaxTotalCents,
		TotalCents:          row.TotalCents,
		ValidUntil:          optionalTime(row.ValidUntil),
		Notes:               optionalString(row.Notes),
		PublicToken:         optionalString(row.PublicToken),
		PublicTokenExpAt:    optionalTime(row.PublicTokenExpiresAt),
		PreviewToken:        optionalString(row.PreviewToken),
		PreviewTokenExpAt:   optionalTime(row.PreviewTokenExpiresAt),
		ViewedAt:            optionalTime(row.ViewedAt),
		AcceptedAt:          optionalTime(row.AcceptedAt),
		RejectedAt:          optionalTime(row.RejectedAt),
		RejectionReason:     optionalString(row.RejectionReason),
		SignatureName:       optionalString(row.SignatureName),
		SignatureData:       optionalString(row.SignatureData),
		SignatureIP:         optionalString(row.SignatureIp),
		PDFFileKey:          optionalString(row.PdfFileKey),
		FinancingDisclaimer: row.FinancingDisclaimer,
		CreatedAt:           timeFromPg(row.CreatedAt),
		UpdatedAt:           timeFromPg(row.UpdatedAt),
	}
}

func quoteFromListRow(row quotesdb.ListQuotesRow) Quote {
	return Quote{
		ID:                         uuid.UUID(row.ID.Bytes),
		OrganizationID:             uuid.UUID(row.OrganizationID.Bytes),
		LeadID:                     uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:              optionalUUID(row.LeadServiceID),
		VersionNumber:              1,
		CreatedByID:                optionalUUID(row.CreatedByID),
		CreatedByFirstName:         optionalString(row.FirstName),
		CreatedByLastName:          optionalString(row.LastName),
		CreatedByEmail:             optionalString(row.Email),
		CustomerFirstName:          optionalString(row.ConsumerFirstName),
		CustomerLastName:           optionalString(row.ConsumerLastName),
		CustomerPhone:              optionalString(row.ConsumerPhone),
		CustomerEmail:              optionalString(row.ConsumerEmail),
		CustomerAddressStreet:      optionalString(row.AddressStreet),
		CustomerAddressHouseNumber: optionalString(row.AddressHouseNumber),
		CustomerAddressZipCode:     optionalString(row.AddressZipCode),
		CustomerAddressCity:        optionalString(row.AddressCity),
		QuoteNumber:                row.QuoteNumber,
		Status:                     string(row.Status),
		PricingMode:                row.PricingMode,
		DiscountType:               row.DiscountType,
		DiscountValue:              row.DiscountValue,
		SubtotalCents:              row.SubtotalCents,
		DiscountAmountCents:        row.DiscountAmountCents,
		TaxTotalCents:              row.TaxTotalCents,
		TotalCents:                 row.TotalCents,
		ValidUntil:                 optionalTime(row.ValidUntil),
		Notes:                      optionalString(row.Notes),
		PublicToken:                optionalString(row.PublicToken),
		PublicTokenExpAt:           optionalTime(row.PublicTokenExpiresAt),
		PreviewToken:               optionalString(row.PreviewToken),
		PreviewTokenExpAt:          optionalTime(row.PreviewTokenExpiresAt),
		ViewedAt:                   optionalTime(row.ViewedAt),
		AcceptedAt:                 optionalTime(row.AcceptedAt),
		RejectedAt:                 optionalTime(row.RejectedAt),
		RejectionReason:            optionalString(row.RejectionReason),
		SignatureName:              optionalString(row.SignatureName),
		SignatureData:              optionalString(row.SignatureData),
		SignatureIP:                optionalString(row.SignatureIp),
		PDFFileKey:                 optionalString(row.PdfFileKey),
		FinancingDisclaimer:        row.FinancingDisclaimer,
		CreatedAt:                  timeFromPg(row.CreatedAt),
		UpdatedAt:                  timeFromPg(row.UpdatedAt),
	}
}

func quoteItemFromListRow(row quotesdb.ListQuoteItemsByQuoteIDRow) (QuoteItem, error) {
	return quoteItemSnapshot{
		id:               row.ID,
		quoteID:          row.QuoteID,
		organizationID:   row.OrganizationID,
		title:            row.Title,
		description:      row.Description,
		quantity:         row.Quantity,
		quantityNumeric:  row.QuantityNumeric,
		unitPriceCents:   row.UnitPriceCents,
		taxRate:          row.TaxRate,
		isOptional:       row.IsOptional,
		isSelected:       row.IsSelected,
		sortOrder:        row.SortOrder,
		catalogProductID: row.CatalogProductID,
		createdAt:        row.CreatedAt,
	}.toModel()
}

func quoteItemFromListIDsRow(row quotesdb.ListQuoteItemsByQuoteIDsRow) (QuoteItem, error) {
	return quoteItemSnapshot{
		id:               row.ID,
		quoteID:          row.QuoteID,
		organizationID:   row.OrganizationID,
		title:            row.Title,
		description:      row.Description,
		quantity:         row.Quantity,
		quantityNumeric:  row.QuantityNumeric,
		unitPriceCents:   row.UnitPriceCents,
		taxRate:          row.TaxRate,
		isOptional:       row.IsOptional,
		isSelected:       row.IsSelected,
		sortOrder:        row.SortOrder,
		catalogProductID: row.CatalogProductID,
		createdAt:        row.CreatedAt,
	}.toModel()
}

func quoteItemFromGetRow(row quotesdb.GetQuoteItemByIDRow) (QuoteItem, error) {
	return quoteItemSnapshot{
		id:               row.ID,
		quoteID:          row.QuoteID,
		organizationID:   row.OrganizationID,
		title:            row.Title,
		description:      row.Description,
		quantity:         row.Quantity,
		quantityNumeric:  row.QuantityNumeric,
		unitPriceCents:   row.UnitPriceCents,
		taxRate:          row.TaxRate,
		isOptional:       row.IsOptional,
		isSelected:       row.IsSelected,
		sortOrder:        row.SortOrder,
		catalogProductID: row.CatalogProductID,
		createdAt:        row.CreatedAt,
	}.toModel()
}

func quoteItemFromNoOrgRow(row quotesdb.ListQuoteItemsByQuoteIDNoOrgRow) (QuoteItem, error) {
	return quoteItemSnapshot{
		id:               row.ID,
		quoteID:          row.QuoteID,
		organizationID:   row.OrganizationID,
		title:            row.Title,
		description:      row.Description,
		quantity:         row.Quantity,
		quantityNumeric:  row.QuantityNumeric,
		unitPriceCents:   row.UnitPriceCents,
		taxRate:          row.TaxRate,
		isOptional:       row.IsOptional,
		isSelected:       row.IsSelected,
		sortOrder:        row.SortOrder,
		catalogProductID: row.CatalogProductID,
		createdAt:        row.CreatedAt,
	}.toModel()
}

type quoteItemSnapshot struct {
	id               pgtype.UUID
	quoteID          pgtype.UUID
	organizationID   pgtype.UUID
	title            string
	description      string
	quantity         string
	quantityNumeric  pgtype.Numeric
	unitPriceCents   int64
	taxRate          int32
	isOptional       bool
	isSelected       bool
	sortOrder        int32
	catalogProductID pgtype.UUID
	createdAt        pgtype.Timestamptz
}

func (snapshot quoteItemSnapshot) toModel() (QuoteItem, error) {
	quantityValue, err := numericFloat64(snapshot.quantityNumeric)
	if err != nil {
		return QuoteItem{}, err
	}

	return QuoteItem{
		ID:               uuid.UUID(snapshot.id.Bytes),
		QuoteID:          uuid.UUID(snapshot.quoteID.Bytes),
		OrganizationID:   uuid.UUID(snapshot.organizationID.Bytes),
		Title:            snapshot.title,
		Description:      snapshot.description,
		Quantity:         snapshot.quantity,
		QuantityNumeric:  quantityValue,
		UnitPriceCents:   snapshot.unitPriceCents,
		TaxRateBps:       int(snapshot.taxRate),
		IsOptional:       snapshot.isOptional,
		IsSelected:       snapshot.isSelected,
		SortOrder:        int(snapshot.sortOrder),
		CatalogProductID: optionalUUID(snapshot.catalogProductID),
		CreatedAt:        timeFromPg(snapshot.createdAt),
	}, nil
}

func quoteAnnotationFromModel(row quotesdb.RacQuoteAnnotation) QuoteAnnotation {
	return QuoteAnnotation{
		ID:             uuid.UUID(row.ID.Bytes),
		QuoteItemID:    uuid.UUID(row.QuoteItemID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		AuthorType:     row.AuthorType,
		AuthorID:       optionalUUID(row.AuthorID),
		Text:           row.Text,
		IsResolved:     row.IsResolved,
		CreatedAt:      timeFromPg(row.CreatedAt),
	}
}

func quoteActivityFromModel(row quotesdb.RacQuoteActivity) QuoteActivity {
	return QuoteActivity{
		ID:             uuid.UUID(row.ID.Bytes),
		QuoteID:        uuid.UUID(row.QuoteID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		EventType:      row.EventType,
		Message:        row.Message,
		Metadata:       row.Metadata,
		CreatedAt:      timeFromPg(row.CreatedAt),
	}
}

func quoteAttachmentFromModel(row quotesdb.RacQuoteAttachment) QuoteAttachment {
	return QuoteAttachment{
		ID:               uuid.UUID(row.ID.Bytes),
		QuoteID:          uuid.UUID(row.QuoteID.Bytes),
		OrganizationID:   uuid.UUID(row.OrganizationID.Bytes),
		Filename:         row.Filename,
		FileKey:          row.FileKey,
		Source:           string(row.Source),
		CatalogProductID: optionalUUID(row.CatalogProductID),
		Enabled:          row.Enabled,
		SortOrder:        int(row.SortOrder),
		CreatedAt:        timeFromPg(row.CreatedAt),
	}
}

func quoteURLFromModel(row quotesdb.RacQuoteUrl) QuoteURL {
	return QuoteURL{
		ID:               uuid.UUID(row.ID.Bytes),
		QuoteID:          uuid.UUID(row.QuoteID.Bytes),
		OrganizationID:   uuid.UUID(row.OrganizationID.Bytes),
		Label:            row.Label,
		Href:             row.Href,
		Accepted:         row.Accepted,
		CatalogProductID: optionalUUID(row.CatalogProductID),
		CreatedAt:        timeFromPg(row.CreatedAt),
	}
}

func generateQuoteJobFromModel(row quotesdb.RacAiQuoteJob) GenerateQuoteJob {
	return GenerateQuoteJob{
		ID:                  uuid.UUID(row.ID.Bytes),
		OrganizationID:      uuid.UUID(row.OrganizationID.Bytes),
		UserID:              uuid.UUID(row.UserID.Bytes),
		LeadID:              uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:       uuid.UUID(row.LeadServiceID.Bytes),
		Status:              row.Status,
		Step:                row.Step,
		ProgressPercent:     int(row.ProgressPercent),
		Error:               optionalString(row.Error),
		QuoteID:             optionalUUID(row.QuoteID),
		QuoteNumber:         optionalString(row.QuoteNumber),
		ItemCount:           optionalInt(row.ItemCount),
		FeedbackRating:      optionalInt(row.FeedbackRating),
		FeedbackComment:     optionalString(row.FeedbackComment),
		FeedbackSubmittedAt: optionalTime(row.FeedbackSubmittedAt),
		CancellationReason:  optionalString(row.CancellationReason),
		ViewedAt:            optionalTime(row.ViewedAt),
		StartedAt:           timeFromPg(row.StartedAt),
		UpdatedAt:           timeFromPg(row.UpdatedAt),
		FinishedAt:          optionalTime(row.FinishedAt),
	}
}

func toPgUUIDSlice(values []uuid.UUID) []pgtype.UUID {
	result := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		result = append(result, toPgUUID(value))
	}
	return result
}

func toPgInt4Ptr(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}

func toPgInt8Ptr(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func toPgNumericValue(value float64) pgtype.Numeric {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		value = 1
	}
	formatted := strconv.FormatFloat(value, 'f', -1, 64)
	var numeric pgtype.Numeric
	if err := numeric.Scan(formatted); err == nil && numeric.Valid {
		return numeric
	}
	return pgtype.Numeric{Int: big.NewInt(1), Exp: 0, Valid: true}
}

func optionalInt(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	number := int(value.Int32)
	return &number
}

func numericFloat64(value pgtype.Numeric) (float64, error) {
	if !value.Valid {
		return 0, nil
	}
	floatValue, err := value.Float64Value()
	if err != nil {
		return 0, err
	}
	if !floatValue.Valid {
		return 0, nil
	}
	return floatValue.Float64, nil
}

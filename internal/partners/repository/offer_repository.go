package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PartnerOffer represents a job offer to a vakman partner.
type PartnerOffer struct {
	ID                     uuid.UUID
	OrganizationID         uuid.UUID
	PartnerID              uuid.UUID
	LeadServiceID          uuid.UUID
	PublicToken            string
	ExpiresAt              time.Time
	PricingSource          string
	CustomerPriceCents     int64
	VakmanPriceCents       int64
	JobSummaryShort        *string
	BuilderSummary         *string
	Status                 string
	AcceptedAt             *time.Time
	RejectedAt             *time.Time
	RejectionReason        *string
	InspectionAvailability []byte // Raw JSONB
	JobAvailability        []byte // Raw JSONB
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// PartnerOfferWithContext enriches a PartnerOffer with display information.
type PartnerOfferWithContext struct {
	PartnerOffer
	PartnerName        string
	OrganizationName   string
	LeadCity           string
	ServiceType        string
	ServiceTypeID      uuid.UUID
	LeadPostcode4      *string
	LeadBuurtcode      *string
	LeadEnergyBouwjaar *int
	UrgencyLevel       *string
}

// QuoteItemSummary is a minimal view of a quote line item for summary generation.
type QuoteItemSummary struct {
	Description string
	Quantity    string
}

// LeadServiceSummaryContext captures non-PII fields for summary generation.
type LeadServiceSummaryContext struct {
	LeadID       uuid.UUID
	ServiceType  string
	UrgencyLevel *string
}

// QuoteForOffer is the minimal quote header data needed to validate and create an offer.
// Kept local to the partners context to avoid cross-module dependencies.
type QuoteForOffer struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	LeadID         uuid.UUID
	LeadServiceID  *uuid.UUID
	Status         string
	TotalCents     int64
}

const offerNotFoundMsg = "offer not found"

var deletableOfferStatuses = []string{"pending", "sent", "expired"}

// CreateOffer inserts a new partner offer.
func (r *Repository) CreateOffer(ctx context.Context, offer PartnerOffer) (PartnerOffer, error) {
	query := `
		INSERT INTO RAC_partner_offers (
			organization_id, partner_id, lead_service_id, public_token, expires_at,
			pricing_source, customer_price_cents, vakman_price_cents, job_summary_short, builder_summary, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending')
		RETURNING id, status, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		offer.OrganizationID, offer.PartnerID, offer.LeadServiceID,
		offer.PublicToken, offer.ExpiresAt,
		offer.PricingSource, offer.CustomerPriceCents, offer.VakmanPriceCents, offer.JobSummaryShort, offer.BuilderSummary,
	).Scan(&offer.ID, &offer.Status, &offer.CreatedAt, &offer.UpdatedAt)
	if err != nil {
		return PartnerOffer{}, fmt.Errorf("create partner offer: %w", err)
	}

	return offer, nil
}

// GetOfferByToken retrieves an offer by its public token with context info.
func (r *Repository) GetOfferByToken(ctx context.Context, token string) (PartnerOfferWithContext, error) {
	query := `
		SELECT o.id, o.organization_id, o.partner_id, o.lead_service_id,
		       o.public_token, o.expires_at,
		       o.pricing_source, o.customer_price_cents, o.vakman_price_cents,
		       o.job_summary_short,
		       o.builder_summary,
		       o.status, o.accepted_at, o.rejected_at, o.rejection_reason,
		       o.inspection_availability, o.job_availability,
		       o.created_at, o.updated_at,
		       p.business_name,
		       org.name,
		       l.address_city,
		       st.name AS service_type,
		       l.lead_enrichment_postcode4,
		       l.lead_enrichment_buurtcode,
		       l.energy_bouwjaar,
		       ai.urgency_level
		FROM RAC_partner_offers o
		JOIN RAC_partners p ON p.id = o.partner_id
		JOIN RAC_organizations org ON org.id = o.organization_id
		JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
		JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
		JOIN RAC_leads l ON l.id = ls.lead_id
		LEFT JOIN LATERAL (
			SELECT urgency_level
			FROM RAC_lead_ai_analysis
			WHERE lead_service_id = ls.id
			ORDER BY created_at DESC
			LIMIT 1
		) ai ON true
		WHERE o.public_token = $1`

	var oc PartnerOfferWithContext
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&oc.ID, &oc.OrganizationID, &oc.PartnerID, &oc.LeadServiceID,
		&oc.PublicToken, &oc.ExpiresAt,
		&oc.PricingSource, &oc.CustomerPriceCents, &oc.VakmanPriceCents,
		&oc.JobSummaryShort,
		&oc.BuilderSummary,
		&oc.Status, &oc.AcceptedAt, &oc.RejectedAt, &oc.RejectionReason,
		&oc.InspectionAvailability, &oc.JobAvailability,
		&oc.CreatedAt, &oc.UpdatedAt,
		&oc.PartnerName,
		&oc.OrganizationName,
		&oc.LeadCity,
		&oc.ServiceType,
		&oc.LeadPostcode4,
		&oc.LeadBuurtcode,
		&oc.LeadEnergyBouwjaar,
		&oc.UrgencyLevel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartnerOfferWithContext{}, apperr.NotFound(offerNotFoundMsg)
	}
	if err != nil {
		return PartnerOfferWithContext{}, fmt.Errorf("get offer by token: %w", err)
	}

	return oc, nil
}

// GetOfferByID retrieves an offer by its ID within a tenant.
func (r *Repository) GetOfferByID(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) (PartnerOffer, error) {
	query := `
		SELECT id, organization_id, partner_id, lead_service_id,
		       public_token, expires_at,
		       pricing_source, customer_price_cents, vakman_price_cents,
		       job_summary_short,
		       builder_summary,
		       status, accepted_at, rejected_at, rejection_reason,
		       created_at, updated_at
		FROM RAC_partner_offers
		WHERE id = $1 AND organization_id = $2`

	var o PartnerOffer
	err := r.pool.QueryRow(ctx, query, offerID, organizationID).Scan(
		&o.ID, &o.OrganizationID, &o.PartnerID, &o.LeadServiceID,
		&o.PublicToken, &o.ExpiresAt,
		&o.PricingSource, &o.CustomerPriceCents, &o.VakmanPriceCents,
		&o.JobSummaryShort,
		&o.BuilderSummary,
		&o.Status, &o.AcceptedAt, &o.RejectedAt, &o.RejectionReason,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartnerOffer{}, apperr.NotFound(offerNotFoundMsg)
	}
	if err != nil {
		return PartnerOffer{}, fmt.Errorf("get offer by id: %w", err)
	}

	return o, nil
}

// DeleteOffer deletes an offer within a tenant if it is still in a deletable state.
// Accepted and rejected offers are intentionally not deletable.
func (r *Repository) DeleteOffer(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) error {
	query := `
		DELETE FROM RAC_partner_offers
		WHERE id = $1
		  AND organization_id = $2
		  AND status = ANY($3::text[])`

	cmd, err := r.pool.Exec(ctx, query, offerID, organizationID, deletableOfferStatuses)
	if err != nil {
		return fmt.Errorf("delete offer: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		// Caller should have checked existence/status; this is a safety net for races.
		return apperr.Conflict("offer cannot be deleted")
	}

	return nil
}

// GetLeadServiceSummaryContext fetches non-PII data used to build offer summaries.
func (r *Repository) GetLeadServiceSummaryContext(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) (LeadServiceSummaryContext, error) {
	query := `
		SELECT ls.lead_id,
		       st.name AS service_type,
		       ai.urgency_level
		FROM RAC_lead_services ls
		JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
		LEFT JOIN LATERAL (
			SELECT urgency_level
			FROM RAC_lead_ai_analysis
			WHERE lead_service_id = ls.id
			ORDER BY created_at DESC
			LIMIT 1
		) ai ON true
		WHERE ls.id = $1 AND ls.organization_id = $2`

	var ctxData LeadServiceSummaryContext
	if err := r.pool.QueryRow(ctx, query, leadServiceID, organizationID).Scan(&ctxData.LeadID, &ctxData.ServiceType, &ctxData.UrgencyLevel); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LeadServiceSummaryContext{}, apperr.NotFound("lead service not found")
		}
		return LeadServiceSummaryContext{}, fmt.Errorf("get lead service summary context: %w", err)
	}

	return ctxData, nil
}

// GetOfferByIDWithContext retrieves an offer by ID with display context (partner name, city, etc.).
func (r *Repository) GetOfferByIDWithContext(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) (PartnerOfferWithContext, error) {
	query := `
		SELECT o.id, o.organization_id, o.partner_id, o.lead_service_id,
		       o.public_token, o.expires_at,
		       o.pricing_source, o.customer_price_cents, o.vakman_price_cents,
		       o.job_summary_short,
		       o.builder_summary,
		       o.status, o.accepted_at, o.rejected_at, o.rejection_reason,
		       o.inspection_availability, o.job_availability,
		       o.created_at, o.updated_at,
		       p.business_name,
		       org.name,
		       l.address_city,
		       st.name AS service_type,
		       l.lead_enrichment_postcode4,
		       l.lead_enrichment_buurtcode,
		       l.energy_bouwjaar,
		       ai.urgency_level
		FROM RAC_partner_offers o
		JOIN RAC_partners p ON p.id = o.partner_id
		JOIN RAC_organizations org ON org.id = o.organization_id
		JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
		JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
		JOIN RAC_leads l ON l.id = ls.lead_id
		LEFT JOIN LATERAL (
			SELECT urgency_level
			FROM RAC_lead_ai_analysis
			WHERE lead_service_id = ls.id
			ORDER BY created_at DESC
			LIMIT 1
		) ai ON true
		WHERE o.id = $1 AND o.organization_id = $2`

	var oc PartnerOfferWithContext
	err := r.pool.QueryRow(ctx, query, offerID, organizationID).Scan(
		&oc.ID, &oc.OrganizationID, &oc.PartnerID, &oc.LeadServiceID,
		&oc.PublicToken, &oc.ExpiresAt,
		&oc.PricingSource, &oc.CustomerPriceCents, &oc.VakmanPriceCents,
		&oc.JobSummaryShort,
		&oc.BuilderSummary,
		&oc.Status, &oc.AcceptedAt, &oc.RejectedAt, &oc.RejectionReason,
		&oc.InspectionAvailability, &oc.JobAvailability,
		&oc.CreatedAt, &oc.UpdatedAt,
		&oc.PartnerName,
		&oc.OrganizationName,
		&oc.LeadCity,
		&oc.ServiceType,
		&oc.LeadPostcode4,
		&oc.LeadBuurtcode,
		&oc.LeadEnergyBouwjaar,
		&oc.UrgencyLevel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartnerOfferWithContext{}, apperr.NotFound(offerNotFoundMsg)
	}
	if err != nil {
		return PartnerOfferWithContext{}, fmt.Errorf("get offer by id with context: %w", err)
	}

	return oc, nil
}

// GetLatestQuoteItemsForService returns line items from the latest non-draft quote for a lead service.
func (r *Repository) GetLatestQuoteItemsForService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]QuoteItemSummary, error) {
	query := `
		WITH latest_quote AS (
			SELECT id
			FROM RAC_quotes
			WHERE lead_service_id = $1 AND organization_id = $2 AND status != 'Draft'
			ORDER BY created_at DESC
			LIMIT 1
		)
		SELECT qi.description, qi.quantity
		FROM RAC_quote_items qi
		JOIN latest_quote lq ON lq.id = qi.quote_id
		WHERE qi.is_optional = false OR qi.is_selected = true
		ORDER BY qi.sort_order ASC`

	rows, err := r.pool.Query(ctx, query, leadServiceID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("query quote items for service: %w", err)
	}
	defer rows.Close()

	var items []QuoteItemSummary
	for rows.Next() {
		var it QuoteItemSummary
		if err := rows.Scan(&it.Description, &it.Quantity); err != nil {
			return nil, fmt.Errorf("scan quote item summary: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quote item summaries: %w", err)
	}

	return items, nil
}

// GetQuoteForOffer retrieves the quote header needed for offer creation.
func (r *Repository) GetQuoteForOffer(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID) (QuoteForOffer, error) {
	query := `
		SELECT id, organization_id, lead_id, lead_service_id, status, total_cents
		FROM RAC_quotes
		WHERE id = $1 AND organization_id = $2`

	var q QuoteForOffer
	err := r.pool.QueryRow(ctx, query, quoteID, organizationID).Scan(
		&q.ID,
		&q.OrganizationID,
		&q.LeadID,
		&q.LeadServiceID,
		&q.Status,
		&q.TotalCents,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteForOffer{}, apperr.NotFound("quote not found")
	}
	if err != nil {
		return QuoteForOffer{}, fmt.Errorf("get quote for offer: %w", err)
	}

	return q, nil
}

// GetQuoteItemsForQuote returns selected/non-optional line items for a specific quote.
func (r *Repository) GetQuoteItemsForQuote(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID) ([]QuoteItemSummary, error) {
	query := `
		SELECT qi.description, qi.quantity
		FROM RAC_quote_items qi
		WHERE qi.quote_id = $1 AND qi.organization_id = $2
			AND (qi.is_optional = false OR qi.is_selected = true)
		ORDER BY qi.sort_order ASC`

	rows, err := r.pool.Query(ctx, query, quoteID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("query quote items for quote: %w", err)
	}
	defer rows.Close()

	var items []QuoteItemSummary
	for rows.Next() {
		var it QuoteItemSummary
		if err := rows.Scan(&it.Description, &it.Quantity); err != nil {
			return nil, fmt.Errorf("scan quote item summary: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quote item summaries: %w", err)
	}

	return items, nil
}

// ListOffersForService returns all offers for a given lead service.
func (r *Repository) ListOffersForService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]PartnerOfferWithContext, error) {
	query := `
		SELECT o.id, o.organization_id, o.partner_id, o.lead_service_id,
		       o.public_token, o.expires_at,
		       o.pricing_source, o.customer_price_cents, o.vakman_price_cents,
		       o.status, o.accepted_at, o.rejected_at, o.rejection_reason,
		       o.inspection_availability, o.job_availability,
		       o.created_at, o.updated_at,
		       p.business_name
		FROM RAC_partner_offers o
		JOIN RAC_partners p ON p.id = o.partner_id
		WHERE o.lead_service_id = $1 AND o.organization_id = $2
		ORDER BY o.created_at DESC`

	rows, err := r.pool.Query(ctx, query, leadServiceID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list offers for service: %w", err)
	}
	defer rows.Close()

	var offers []PartnerOfferWithContext
	for rows.Next() {
		var oc PartnerOfferWithContext
		if err := rows.Scan(
			&oc.ID, &oc.OrganizationID, &oc.PartnerID, &oc.LeadServiceID,
			&oc.PublicToken, &oc.ExpiresAt,
			&oc.PricingSource, &oc.CustomerPriceCents, &oc.VakmanPriceCents,
			&oc.Status, &oc.AcceptedAt, &oc.RejectedAt, &oc.RejectionReason,
			&oc.InspectionAvailability, &oc.JobAvailability,
			&oc.CreatedAt, &oc.UpdatedAt,
			&oc.PartnerName,
		); err != nil {
			return nil, fmt.Errorf("scan offer: %w", err)
		}
		offers = append(offers, oc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate offers: %w", err)
	}

	return offers, nil
}

// ListOffersByPartner returns all offers for a given partner within a tenant.
func (r *Repository) ListOffersByPartner(ctx context.Context, partnerID uuid.UUID, organizationID uuid.UUID) ([]PartnerOfferWithContext, error) {
	query := `
		SELECT o.id, o.organization_id, o.partner_id, o.lead_service_id,
		       o.public_token, o.expires_at,
		       o.pricing_source, o.customer_price_cents, o.vakman_price_cents,
		       o.status, o.accepted_at, o.rejected_at, o.rejection_reason,
		       o.inspection_availability, o.job_availability,
		       o.created_at, o.updated_at,
		       p.business_name,
		       org.name,
		       l.address_city,
		       st.name AS service_type,
		       ls.service_type_id
		FROM RAC_partner_offers o
		JOIN RAC_partners p ON p.id = o.partner_id
		JOIN RAC_organizations org ON org.id = o.organization_id
		JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
		JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
		JOIN RAC_leads l ON l.id = ls.lead_id
		WHERE o.partner_id = $1 AND o.organization_id = $2
		ORDER BY o.created_at DESC`

	rows, err := r.pool.Query(ctx, query, partnerID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list offers by partner: %w", err)
	}
	defer rows.Close()

	var offers []PartnerOfferWithContext
	for rows.Next() {
		var oc PartnerOfferWithContext
		if err := rows.Scan(
			&oc.ID, &oc.OrganizationID, &oc.PartnerID, &oc.LeadServiceID,
			&oc.PublicToken, &oc.ExpiresAt,
			&oc.PricingSource, &oc.CustomerPriceCents, &oc.VakmanPriceCents,
			&oc.Status, &oc.AcceptedAt, &oc.RejectedAt, &oc.RejectionReason,
			&oc.InspectionAvailability, &oc.JobAvailability,
			&oc.CreatedAt, &oc.UpdatedAt,
			&oc.PartnerName,
			&oc.OrganizationName,
			&oc.LeadCity,
			&oc.ServiceType,
			&oc.ServiceTypeID,
		); err != nil {
			return nil, fmt.Errorf("scan partner offer: %w", err)
		}
		offers = append(offers, oc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partner offers: %w", err)
	}

	return offers, nil
}

// HasActiveOffer returns true if there is already a pending/sent offer for the lead service.
func (r *Repository) HasActiveOffer(ctx context.Context, leadServiceID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(
		SELECT 1 FROM RAC_partner_offers
		WHERE lead_service_id = $1 AND status IN ('pending', 'sent')
	)`
	if err := r.pool.QueryRow(ctx, query, leadServiceID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check active offer: %w", err)
	}
	return exists, nil
}

// AcceptOffer atomically accepts an offer and records availability.
// The unique index idx_partner_offers_exclusive_acceptance prevents double-acceptance.
func (r *Repository) AcceptOffer(ctx context.Context, offerID uuid.UUID, inspectionSlots []byte, jobSlots []byte) error {
	query := `
		UPDATE RAC_partner_offers
		SET status = 'accepted',
		    accepted_at = now(),
		    inspection_availability = $2,
		    job_availability = $3,
		    updated_at = now()
		WHERE id = $1 AND status IN ('pending', 'sent')`

	tag, err := r.pool.Exec(ctx, query, offerID, inspectionSlots, jobSlots)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "idx_partner_offers_exclusive_acceptance") {
			return apperr.Conflict("job already assigned to another partner")
		}
		return fmt.Errorf("accept offer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("offer is not in a valid state to be accepted")
	}

	return nil
}

// RejectOffer marks an offer as rejected with an optional reason.
func (r *Repository) RejectOffer(ctx context.Context, offerID uuid.UUID, reason string) error {
	query := `
		UPDATE RAC_partner_offers
		SET status = 'rejected',
		    rejected_at = now(),
		    rejection_reason = $2,
		    updated_at = now()
		WHERE id = $1 AND status IN ('pending', 'sent')`

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	tag, err := r.pool.Exec(ctx, query, offerID, reasonPtr)
	if err != nil {
		return fmt.Errorf("reject offer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("offer is not in a valid state to be rejected")
	}

	return nil
}

// ExpireOffers marks all pending/sent offers past their expiry as expired.
// Returns the expired offers for event publishing.
func (r *Repository) ExpireOffers(ctx context.Context) ([]PartnerOffer, error) {
	query := `
		UPDATE RAC_partner_offers
		SET status = 'expired', updated_at = now()
		WHERE status IN ('pending', 'sent') AND expires_at < now()
		RETURNING id, organization_id, partner_id, lead_service_id`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("expire offers: %w", err)
	}
	defer rows.Close()

	var expired []PartnerOffer
	for rows.Next() {
		var o PartnerOffer
		if err := rows.Scan(&o.ID, &o.OrganizationID, &o.PartnerID, &o.LeadServiceID); err != nil {
			return nil, fmt.Errorf("scan expired offer: %w", err)
		}
		expired = append(expired, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired offers: %w", err)
	}

	return expired, nil
}

// OfferListParams defines filters/sort/paging for the global offers overview.
// NOTE: SortBy/SortOrder are validated at the transport layer, but we still
// resolve them here to safe SQL fragments.
type OfferListParams struct {
	OrganizationID uuid.UUID
	Search         string
	Status         string
	PartnerID      uuid.UUID
	LeadServiceID  uuid.UUID
	ServiceTypeID  uuid.UUID
	SortBy         string
	SortOrder      string
	Page           int
	PageSize       int
}

type OfferListResult struct {
	Items      []PartnerOfferWithContext
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

func normalizeOfferListPaging(page int, pageSize int) (normalizedPage int, normalizedPageSize int, offset int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

func calcTotalPages(total int, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}

func buildOfferListWhere(params OfferListParams) (whereSQL string, args []interface{}, nextArg int) {
	where := []string{"o.organization_id = $1"}
	args = []interface{}{params.OrganizationID}
	nextArg = 2

	if strings.TrimSpace(params.Search) != "" {
		search := "%" + strings.TrimSpace(params.Search) + "%"
		where = append(where, fmt.Sprintf("(p.business_name ILIKE $%d OR st.name ILIKE $%d OR l.address_city ILIKE $%d)", nextArg, nextArg, nextArg))
		args = append(args, search)
		nextArg++
	}
	if strings.TrimSpace(params.Status) != "" {
		where = append(where, fmt.Sprintf("o.status = $%d", nextArg))
		args = append(args, strings.TrimSpace(params.Status))
		nextArg++
	}
	if params.PartnerID != uuid.Nil {
		where = append(where, fmt.Sprintf("o.partner_id = $%d", nextArg))
		args = append(args, params.PartnerID)
		nextArg++
	}
	if params.LeadServiceID != uuid.Nil {
		where = append(where, fmt.Sprintf("o.lead_service_id = $%d", nextArg))
		args = append(args, params.LeadServiceID)
		nextArg++
	}
	if params.ServiceTypeID != uuid.Nil {
		where = append(where, fmt.Sprintf("ls.service_type_id = $%d", nextArg))
		args = append(args, params.ServiceTypeID)
		nextArg++
	}

	return "WHERE " + strings.Join(where, " AND "), args, nextArg
}

// ListOffers returns a paginated list of offers across all partners in a tenant.
func (r *Repository) ListOffers(ctx context.Context, params OfferListParams) (OfferListResult, error) {
	sortCol, err := resolveOfferSortBy(params.SortBy)
	if err != nil {
		return OfferListResult{}, err
	}
	orderBy, err := resolveOfferSortOrder(params.SortOrder)
	if err != nil {
		return OfferListResult{}, err
	}

	page, pageSize, offset := normalizeOfferListPaging(params.Page, params.PageSize)

	baseFrom := `
FROM RAC_partner_offers o
JOIN RAC_partners p ON p.id = o.partner_id
JOIN RAC_organizations org ON org.id = o.organization_id
JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
JOIN RAC_leads l ON l.id = ls.lead_id
`

	whereSQL, args, argN := buildOfferListWhere(params)

	var total int
	countQuery := "SELECT COUNT(*) " + baseFrom + " " + whereSQL
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return OfferListResult{}, fmt.Errorf("count offers: %w", err)
	}

	totalPages := calcTotalPages(total, pageSize)

	selectQuery := `
SELECT o.id, o.organization_id, o.partner_id, o.lead_service_id,
       o.public_token, o.expires_at,
       o.pricing_source, o.customer_price_cents, o.vakman_price_cents,
       o.job_summary_short,
       o.builder_summary,
       o.status, o.accepted_at, o.rejected_at, o.rejection_reason,
       o.inspection_availability, o.job_availability,
       o.created_at, o.updated_at,
       p.business_name,
       org.name,
       l.address_city,
	st.name AS service_type,
	ls.service_type_id
` + baseFrom + " " + whereSQL + "\n" +
		"ORDER BY " + sortCol + " " + orderBy + ", o.created_at DESC\n" +
		fmt.Sprintf("LIMIT $%d OFFSET $%d", argN, argN+1)

	args = append(args, pageSize, offset)
	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return OfferListResult{}, fmt.Errorf("list offers: %w", err)
	}
	defer rows.Close()

	offers := make([]PartnerOfferWithContext, 0)
	for rows.Next() {
		var oc PartnerOfferWithContext
		if err := rows.Scan(
			&oc.ID, &oc.OrganizationID, &oc.PartnerID, &oc.LeadServiceID,
			&oc.PublicToken, &oc.ExpiresAt,
			&oc.PricingSource, &oc.CustomerPriceCents, &oc.VakmanPriceCents,
			&oc.JobSummaryShort,
			&oc.BuilderSummary,
			&oc.Status, &oc.AcceptedAt, &oc.RejectedAt, &oc.RejectionReason,
			&oc.InspectionAvailability, &oc.JobAvailability,
			&oc.CreatedAt, &oc.UpdatedAt,
			&oc.PartnerName,
			&oc.OrganizationName,
			&oc.LeadCity,
			&oc.ServiceType,
			&oc.ServiceTypeID,
		); err != nil {
			return OfferListResult{}, fmt.Errorf("scan offer: %w", err)
		}
		offers = append(offers, oc)
	}
	if err := rows.Err(); err != nil {
		return OfferListResult{}, fmt.Errorf("iterate offers: %w", err)
	}

	return OfferListResult{Items: offers, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func resolveOfferSortBy(value string) (string, error) {
	if value == "" {
		return "o.created_at", nil
	}
	switch value {
	case "createdAt":
		return "o.created_at", nil
	case "expiresAt":
		return "o.expires_at", nil
	case "status":
		return "o.status", nil
	case "partnerName":
		return "p.business_name", nil
	case "serviceType":
		return "st.name", nil
	case "vakmanPriceCents":
		return "o.vakman_price_cents", nil
	case "customerPriceCents":
		return "o.customer_price_cents", nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

func resolveOfferSortOrder(value string) (string, error) {
	if value == "" {
		return "desc", nil
	}
	switch value {
	case "asc", "desc":
		return value, nil
	default:
		return "", apperr.BadRequest("invalid sort order")
	}
}

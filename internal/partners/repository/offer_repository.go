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
	PartnerName      string
	OrganizationName string
	LeadCity         string
	ServiceType      string
}

const offerNotFoundMsg = "offer not found"

// CreateOffer inserts a new partner offer.
func (r *Repository) CreateOffer(ctx context.Context, offer PartnerOffer) (PartnerOffer, error) {
	query := `
		INSERT INTO RAC_partner_offers (
			organization_id, partner_id, lead_service_id, public_token, expires_at,
			pricing_source, customer_price_cents, vakman_price_cents, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
		RETURNING id, status, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		offer.OrganizationID, offer.PartnerID, offer.LeadServiceID,
		offer.PublicToken, offer.ExpiresAt,
		offer.PricingSource, offer.CustomerPriceCents, offer.VakmanPriceCents,
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
		       o.status, o.accepted_at, o.rejected_at, o.rejection_reason,
		       o.inspection_availability, o.job_availability,
		       o.created_at, o.updated_at,
		       p.business_name,
		       org.name,
		       l.address_city,
		       ls.service_type
		FROM RAC_partner_offers o
		JOIN RAC_partners p ON p.id = o.partner_id
		JOIN RAC_organizations org ON org.id = o.organization_id
		JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
		JOIN RAC_leads l ON l.id = ls.lead_id
		WHERE o.public_token = $1`

	var oc PartnerOfferWithContext
	err := r.pool.QueryRow(ctx, query, token).Scan(
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
		       status, accepted_at, rejected_at, rejection_reason,
		       created_at, updated_at
		FROM RAC_partner_offers
		WHERE id = $1 AND organization_id = $2`

	var o PartnerOffer
	err := r.pool.QueryRow(ctx, query, offerID, organizationID).Scan(
		&o.ID, &o.OrganizationID, &o.PartnerID, &o.LeadServiceID,
		&o.PublicToken, &o.ExpiresAt,
		&o.PricingSource, &o.CustomerPriceCents, &o.VakmanPriceCents,
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

// GetOfferByIDWithContext retrieves an offer by ID with display context (partner name, city, etc.).
func (r *Repository) GetOfferByIDWithContext(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) (PartnerOfferWithContext, error) {
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
		       ls.service_type
		FROM RAC_partner_offers o
		JOIN RAC_partners p ON p.id = o.partner_id
		JOIN RAC_organizations org ON org.id = o.organization_id
		JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
		JOIN RAC_leads l ON l.id = ls.lead_id
		WHERE o.id = $1 AND o.organization_id = $2`

	var oc PartnerOfferWithContext
	err := r.pool.QueryRow(ctx, query, offerID, organizationID).Scan(
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
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartnerOfferWithContext{}, apperr.NotFound(offerNotFoundMsg)
	}
	if err != nil {
		return PartnerOfferWithContext{}, fmt.Errorf("get offer by id with context: %w", err)
	}

	return oc, nil
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
		       ls.service_type
		FROM RAC_partner_offers o
		JOIN RAC_partners p ON p.id = o.partner_id
		JOIN RAC_organizations org ON org.id = o.organization_id
		JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
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

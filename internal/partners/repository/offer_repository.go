package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	partnersdb "portal_final_backend/internal/partners/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	MarginBasisPoints      int
	OfferLineItems         []OfferLineItem
	JobSummaryShort        *string
	BuilderSummary         *string
	Status                 string
	RequiresInspection     bool
	AcceptedAt             *time.Time
	RejectedAt             *time.Time
	RejectionReason        *string
	InspectionAvailability []byte
	JobAvailability        []byte
	SignerName             *string
	SignerBusinessName     *string
	SignerAddress          *string
	SignatureData          *string
	PDFFileKey             *string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// PartnerOfferWithContext enriches a PartnerOffer with display information.
type PartnerOfferWithContext struct {
	PartnerOffer
	PartnerName         string
	PartnerContactName  string
	PartnerAddressLine1 string
	PartnerAddressLine2 string
	PartnerHouseNumber  string
	PartnerPostalCode   string
	PartnerCity         string
	OrganizationName    string
	LeadCity            string
	LeadFirstName       string
	LeadLastName        string
	LeadPhone           string
	LeadEmail           string
	LeadStreet          string
	LeadHouseNumber     string
	LeadZipCode         string
	ServiceType         string
	ServiceTypeID       uuid.UUID
	LeadPostcode4       *string
	LeadBuurtcode       *string
	LeadEnergyBouwjaar  *int
	UrgencyLevel        *string
}

type PartnerOfferTerms struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	Content         string
	Version         int
	CreatedAt       time.Time
	CreatedByUserID *uuid.UUID
	IsActive        bool
}

// QuoteItemSummary is a minimal view of a quote line item for summary generation.
type QuoteItemSummary struct {
	ID             uuid.UUID
	Description    string
	Quantity       string
	UnitPriceCents int64
	LineTotalCents int64
}

type OfferLineItem struct {
	QuoteItemID    uuid.UUID `json:"quoteItemId"`
	Description    string    `json:"description"`
	Quantity       string    `json:"quantity"`
	UnitPriceCents int64     `json:"unitPriceCents"`
	LineTotalCents int64     `json:"lineTotalCents"`
}

type PhotoAttachment struct {
	ID            uuid.UUID
	LeadServiceID uuid.UUID
	FileKey       string
	FileName      string
	ContentType   string
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

func optionalInt(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int32)
	return &n
}

func optionalNonEmptyString(value string) *string {
	if value == "" {
		return nil
	}
	text := value
	return &text
}

func optionalUnknownString(value interface{}) *string {
	switch typed := value.(type) {
	case string:
		return optionalNonEmptyString(typed)
	case []byte:
		return optionalNonEmptyString(string(typed))
	default:
		return nil
	}
}

func optionalFilterText(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + trimmed + "%", Valid: true}
}

func optionalExactText(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}

func optionalFilterUUID(value uuid.UUID) pgtype.UUID {
	if value == uuid.Nil {
		return pgtype.UUID{}
	}
	return toPgUUID(value)
}

type offerSnapshot struct {
	ID                     pgtype.UUID
	OrganizationID         pgtype.UUID
	PartnerID              pgtype.UUID
	LeadServiceID          pgtype.UUID
	PublicToken            string
	ExpiresAt              pgtype.Timestamptz
	PricingSource          string
	CustomerPriceCents     int64
	VakmanPriceCents       int64
	MarginBasisPoints      int32
	OfferLineItems         []byte
	JobSummaryShort        pgtype.Text
	BuilderSummary         pgtype.Text
	Status                 string
	RequiresInspection     bool
	AcceptedAt             pgtype.Timestamptz
	RejectedAt             pgtype.Timestamptz
	RejectionReason        pgtype.Text
	InspectionAvailability []byte
	JobAvailability        []byte
	SignerName             pgtype.Text
	SignerBusinessName     pgtype.Text
	SignerAddress          pgtype.Text
	SignatureData          pgtype.Text
	PDFFileKey             pgtype.Text
	CreatedAt              pgtype.Timestamptz
	UpdatedAt              pgtype.Timestamptz
}

func offerFromSnapshot(data offerSnapshot) PartnerOffer {
	return PartnerOffer{
		ID:                     uuid.UUID(data.ID.Bytes),
		OrganizationID:         uuid.UUID(data.OrganizationID.Bytes),
		PartnerID:              uuid.UUID(data.PartnerID.Bytes),
		LeadServiceID:          uuid.UUID(data.LeadServiceID.Bytes),
		PublicToken:            data.PublicToken,
		ExpiresAt:              data.ExpiresAt.Time,
		PricingSource:          data.PricingSource,
		CustomerPriceCents:     data.CustomerPriceCents,
		VakmanPriceCents:       data.VakmanPriceCents,
		MarginBasisPoints:      int(data.MarginBasisPoints),
		OfferLineItems:         unmarshalOfferLineItems(data.OfferLineItems),
		JobSummaryShort:        optionalString(data.JobSummaryShort),
		BuilderSummary:         optionalString(data.BuilderSummary),
		Status:                 data.Status,
		RequiresInspection:     data.RequiresInspection,
		AcceptedAt:             optionalTime(data.AcceptedAt),
		RejectedAt:             optionalTime(data.RejectedAt),
		RejectionReason:        optionalString(data.RejectionReason),
		InspectionAvailability: data.InspectionAvailability,
		JobAvailability:        data.JobAvailability,
		SignerName:             optionalString(data.SignerName),
		SignerBusinessName:     optionalString(data.SignerBusinessName),
		SignerAddress:          optionalString(data.SignerAddress),
		SignatureData:          optionalString(data.SignatureData),
		PDFFileKey:             optionalString(data.PDFFileKey),
		CreatedAt:              data.CreatedAt.Time,
		UpdatedAt:              data.UpdatedAt.Time,
	}
}

func offerFromGetPartnerOfferByIDRow(row partnersdb.GetPartnerOfferByIDRow) PartnerOffer {
	return offerFromSnapshot(offerSnapshot{
		ID:                     row.ID,
		OrganizationID:         row.OrganizationID,
		PartnerID:              row.PartnerID,
		LeadServiceID:          row.LeadServiceID,
		PublicToken:            row.PublicToken,
		ExpiresAt:              row.ExpiresAt,
		PricingSource:          row.PricingSource,
		CustomerPriceCents:     row.CustomerPriceCents,
		VakmanPriceCents:       row.VakmanPriceCents,
		MarginBasisPoints:      row.MarginBasisPoints,
		OfferLineItems:         row.OfferLineItems,
		JobSummaryShort:        row.JobSummaryShort,
		BuilderSummary:         row.BuilderSummary,
		Status:                 row.Status,
		RequiresInspection:     row.RequiresInspection,
		AcceptedAt:             row.AcceptedAt,
		RejectedAt:             row.RejectedAt,
		RejectionReason:        row.RejectionReason,
		InspectionAvailability: row.InspectionAvailability,
		JobAvailability:        row.JobAvailability,
		SignerName:             row.SignerName,
		SignerBusinessName:     row.SignerBusinessName,
		SignerAddress:          row.SignerAddress,
		SignatureData:          row.SignatureData,
		PDFFileKey:             row.PdfFileKey,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	})
}

func offerFromCreatePartnerOfferRow(row partnersdb.CreatePartnerOfferRow) PartnerOffer {
	return offerFromSnapshot(offerSnapshot{
		ID:                     row.ID,
		OrganizationID:         row.OrganizationID,
		PartnerID:              row.PartnerID,
		LeadServiceID:          row.LeadServiceID,
		PublicToken:            row.PublicToken,
		ExpiresAt:              row.ExpiresAt,
		PricingSource:          row.PricingSource,
		CustomerPriceCents:     row.CustomerPriceCents,
		VakmanPriceCents:       row.VakmanPriceCents,
		MarginBasisPoints:      row.MarginBasisPoints,
		OfferLineItems:         row.OfferLineItems,
		JobSummaryShort:        row.JobSummaryShort,
		BuilderSummary:         row.BuilderSummary,
		Status:                 row.Status,
		RequiresInspection:     row.RequiresInspection,
		AcceptedAt:             row.AcceptedAt,
		RejectedAt:             row.RejectedAt,
		RejectionReason:        row.RejectionReason,
		InspectionAvailability: row.InspectionAvailability,
		JobAvailability:        row.JobAvailability,
		SignerName:             row.SignerName,
		SignerBusinessName:     row.SignerBusinessName,
		SignerAddress:          row.SignerAddress,
		SignatureData:          row.SignatureData,
		PDFFileKey:             row.PdfFileKey,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	})
}

type offerContext struct {
	Offer               PartnerOffer
	PartnerName         string
	PartnerContactName  string
	PartnerAddressLine1 string
	PartnerAddressLine2 string
	PartnerHouseNumber  string
	PartnerPostalCode   string
	PartnerCity         string
	OrganizationName    string
	LeadCity            string
	LeadFirstName       string
	LeadLastName        string
	LeadPhone           string
	LeadEmail           string
	LeadStreet          string
	LeadHouseNumber     string
	LeadZipCode         string
	ServiceType         string
	ServiceTypeID       pgtype.UUID
	LeadPostcode4       pgtype.Text
	LeadBuurtcode       pgtype.Text
	LeadEnergyBouwjaar  pgtype.Int4
	UrgencyLevel        interface{}
}

func offerWithContext(data offerContext) PartnerOfferWithContext {
	result := PartnerOfferWithContext{
		PartnerOffer:        data.Offer,
		PartnerName:         data.PartnerName,
		PartnerContactName:  data.PartnerContactName,
		PartnerAddressLine1: data.PartnerAddressLine1,
		PartnerAddressLine2: data.PartnerAddressLine2,
		PartnerHouseNumber:  data.PartnerHouseNumber,
		PartnerPostalCode:   data.PartnerPostalCode,
		PartnerCity:         data.PartnerCity,
		OrganizationName:    data.OrganizationName,
		LeadCity:            data.LeadCity,
		LeadFirstName:       data.LeadFirstName,
		LeadLastName:        data.LeadLastName,
		LeadPhone:           data.LeadPhone,
		LeadEmail:           data.LeadEmail,
		LeadStreet:          data.LeadStreet,
		LeadHouseNumber:     data.LeadHouseNumber,
		LeadZipCode:         data.LeadZipCode,
		ServiceType:         data.ServiceType,
		LeadPostcode4:       optionalString(data.LeadPostcode4),
		LeadBuurtcode:       optionalString(data.LeadBuurtcode),
		LeadEnergyBouwjaar:  optionalInt(data.LeadEnergyBouwjaar),
		UrgencyLevel:        optionalUnknownString(data.UrgencyLevel),
	}
	if data.ServiceTypeID.Valid {
		result.ServiceTypeID = uuid.UUID(data.ServiceTypeID.Bytes)
	}
	return result
}

// CreateOffer inserts a new partner offer.
func (r *Repository) CreateOffer(ctx context.Context, offer PartnerOffer) (PartnerOffer, error) {
	offerLineItems, err := json.Marshal(offer.OfferLineItems)
	if err != nil {
		return PartnerOffer{}, fmt.Errorf("marshal offer line items: %w", err)
	}
	created, err := r.queries.CreatePartnerOffer(ctx, partnersdb.CreatePartnerOfferParams{
		OrganizationID:     toPgUUID(offer.OrganizationID),
		PartnerID:          toPgUUID(offer.PartnerID),
		LeadServiceID:      toPgUUID(offer.LeadServiceID),
		PublicToken:        offer.PublicToken,
		ExpiresAt:          pgtype.Timestamptz{Time: offer.ExpiresAt, Valid: true},
		PricingSource:      partnersdb.PricingSource(offer.PricingSource),
		CustomerPriceCents: offer.CustomerPriceCents,
		VakmanPriceCents:   offer.VakmanPriceCents,
		MarginBasisPoints:  int32(offer.MarginBasisPoints),
		OfferLineItems:     offerLineItems,
		JobSummaryShort:    toPgText(offer.JobSummaryShort),
		BuilderSummary:     toPgText(offer.BuilderSummary),
		RequiresInspection: offer.RequiresInspection,
	})
	if err != nil {
		return PartnerOffer{}, fmt.Errorf("create partner offer: %w", err)
	}
	return offerFromCreatePartnerOfferRow(created), nil
}

// GetOfferByToken retrieves an offer by its public token with context info.
func (r *Repository) GetOfferByToken(ctx context.Context, token string) (PartnerOfferWithContext, error) {
	const query = `
		SELECT o.id,
			o.organization_id,
			o.partner_id,
			o.lead_service_id,
			o.public_token,
			o.expires_at,
			o.pricing_source::text AS pricing_source,
			o.customer_price_cents,
			o.vakman_price_cents,
			o.margin_basis_points,
			o.offer_line_items,
			o.job_summary_short,
			o.builder_summary,
			o.status::text AS status,
			o.requires_inspection,
			o.accepted_at,
			o.rejected_at,
			o.rejection_reason,
			o.inspection_availability,
			o.job_availability,
			o.signer_name,
			o.signer_business_name,
			o.signer_address,
			o.signature_data,
			o.pdf_file_key,
			o.created_at,
			o.updated_at,
			p.business_name,
			COALESCE(p.contact_name, '')::text,
			COALESCE(p.address_line1, '')::text,
			COALESCE(p.address_line2, '')::text,
			COALESCE(p.house_number, '')::text,
			COALESCE(p.postal_code, '')::text,
			COALESCE(p.city, '')::text,
			org.name,
			l.address_city,
			l.consumer_first_name,
			l.consumer_last_name,
			COALESCE(l.consumer_phone, '')::text,
			COALESCE(l.consumer_email, '')::text,
			COALESCE(l.address_street, '')::text,
			COALESCE(l.address_house_number, '')::text,
			COALESCE(l.address_zip_code, '')::text,
			st.name AS service_type,
			st.id AS service_type_id,
			l.lead_enrichment_postcode4,
			l.lead_enrichment_buurtcode,
			l.energy_bouwjaar,
			COALESCE(ai.urgency_level::text, '') AS urgency_level
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
		WHERE o.public_token = $1::text`
	return r.getOfferWithContext(ctx, query, token)
}

// GetOfferByID retrieves an offer by its ID within a tenant.
func (r *Repository) GetOfferByID(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) (PartnerOffer, error) {
	row, err := r.queries.GetPartnerOfferByID(ctx, partnersdb.GetPartnerOfferByIDParams{
		OfferID:        toPgUUID(offerID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PartnerOffer{}, apperr.NotFound(offerNotFoundMsg)
	}
	if err != nil {
		return PartnerOffer{}, fmt.Errorf("get offer by id: %w", err)
	}
	return offerFromGetPartnerOfferByIDRow(row), nil
}

// DeleteOffer deletes an offer within a tenant if it is still in a deletable state.
// Accepted and rejected offers are intentionally not deletable.
func (r *Repository) DeleteOffer(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) error {
	rowsAffected, err := r.queries.DeletePartnerOffer(ctx, partnersdb.DeletePartnerOfferParams{
		OfferID:        toPgUUID(offerID),
		OrganizationID: toPgUUID(organizationID),
		Statuses:       deletableOfferStatuses,
	})
	if err != nil {
		return fmt.Errorf("delete offer: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.Conflict("offer cannot be deleted")
	}

	return nil
}

// GetLeadServiceSummaryContext fetches non-PII data used to build offer summaries.
func (r *Repository) GetLeadServiceSummaryContext(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) (LeadServiceSummaryContext, error) {
	row, err := r.queries.GetLeadServiceSummaryContext(ctx, partnersdb.GetLeadServiceSummaryContextParams{
		LeadServiceID:  toPgUUID(leadServiceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LeadServiceSummaryContext{}, apperr.NotFound("lead service not found")
		}
		return LeadServiceSummaryContext{}, fmt.Errorf("get lead service summary context: %w", err)
	}

	return LeadServiceSummaryContext{
		LeadID:       uuid.UUID(row.LeadID.Bytes),
		ServiceType:  row.ServiceType,
		UrgencyLevel: optionalUnknownString(row.UrgencyLevel),
	}, nil
}

// GetOfferByIDWithContext retrieves an offer by ID with display context.
func (r *Repository) GetOfferByIDWithContext(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) (PartnerOfferWithContext, error) {
	const query = `
		SELECT o.id,
			o.organization_id,
			o.partner_id,
			o.lead_service_id,
			o.public_token,
			o.expires_at,
			o.pricing_source::text AS pricing_source,
			o.customer_price_cents,
			o.vakman_price_cents,
			o.margin_basis_points,
			o.offer_line_items,
			o.job_summary_short,
			o.builder_summary,
			o.status::text AS status,
			o.requires_inspection,
			o.accepted_at,
			o.rejected_at,
			o.rejection_reason,
			o.inspection_availability,
			o.job_availability,
			o.signer_name,
			o.signer_business_name,
			o.signer_address,
			o.signature_data,
			o.pdf_file_key,
			o.created_at,
			o.updated_at,
			p.business_name,
			COALESCE(p.contact_name, '')::text,
			COALESCE(p.address_line1, '')::text,
			COALESCE(p.address_line2, '')::text,
			COALESCE(p.house_number, '')::text,
			COALESCE(p.postal_code, '')::text,
			COALESCE(p.city, '')::text,
			org.name,
			l.address_city,
			l.consumer_first_name,
			l.consumer_last_name,
			COALESCE(l.consumer_phone, '')::text,
			COALESCE(l.consumer_email, '')::text,
			COALESCE(l.address_street, '')::text,
			COALESCE(l.address_house_number, '')::text,
			COALESCE(l.address_zip_code, '')::text,
			st.name AS service_type,
			st.id AS service_type_id,
			l.lead_enrichment_postcode4,
			l.lead_enrichment_buurtcode,
			l.energy_bouwjaar,
			COALESCE(ai.urgency_level::text, '') AS urgency_level
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
		WHERE o.id = $1::uuid
		  AND o.organization_id = $2::uuid`
	return r.getOfferWithContext(ctx, query, offerID, organizationID)
}

func (r *Repository) getOfferWithContext(ctx context.Context, query string, args ...any) (PartnerOfferWithContext, error) {
	var snapshot offerSnapshot
	var contextRow offerContext
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&snapshot.ID,
		&snapshot.OrganizationID,
		&snapshot.PartnerID,
		&snapshot.LeadServiceID,
		&snapshot.PublicToken,
		&snapshot.ExpiresAt,
		&snapshot.PricingSource,
		&snapshot.CustomerPriceCents,
		&snapshot.VakmanPriceCents,
		&snapshot.MarginBasisPoints,
		&snapshot.OfferLineItems,
		&snapshot.JobSummaryShort,
		&snapshot.BuilderSummary,
		&snapshot.Status,
		&snapshot.RequiresInspection,
		&snapshot.AcceptedAt,
		&snapshot.RejectedAt,
		&snapshot.RejectionReason,
		&snapshot.InspectionAvailability,
		&snapshot.JobAvailability,
		&snapshot.SignerName,
		&snapshot.SignerBusinessName,
		&snapshot.SignerAddress,
		&snapshot.SignatureData,
		&snapshot.PDFFileKey,
		&snapshot.CreatedAt,
		&snapshot.UpdatedAt,
		&contextRow.PartnerName,
		&contextRow.PartnerContactName,
		&contextRow.PartnerAddressLine1,
		&contextRow.PartnerAddressLine2,
		&contextRow.PartnerHouseNumber,
		&contextRow.PartnerPostalCode,
		&contextRow.PartnerCity,
		&contextRow.OrganizationName,
		&contextRow.LeadCity,
		&contextRow.LeadFirstName,
		&contextRow.LeadLastName,
		&contextRow.LeadPhone,
		&contextRow.LeadEmail,
		&contextRow.LeadStreet,
		&contextRow.LeadHouseNumber,
		&contextRow.LeadZipCode,
		&contextRow.ServiceType,
		&contextRow.ServiceTypeID,
		&contextRow.LeadPostcode4,
		&contextRow.LeadBuurtcode,
		&contextRow.LeadEnergyBouwjaar,
		&contextRow.UrgencyLevel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartnerOfferWithContext{}, apperr.NotFound(offerNotFoundMsg)
	}
	if err != nil {
		return PartnerOfferWithContext{}, err
	}
	contextRow.Offer = offerFromSnapshot(snapshot)
	return offerWithContext(contextRow), nil
}

func (r *Repository) GetActivePartnerOfferTerms(ctx context.Context, organizationID uuid.UUID) (PartnerOfferTerms, error) {
	const query = `
		SELECT id, organization_id, content, version, created_by, created_at, is_active
		FROM RAC_partner_offer_terms
		WHERE organization_id = $1
		  AND is_active = TRUE
		ORDER BY version DESC
		LIMIT 1`

	var item PartnerOfferTerms
	var createdBy pgtype.UUID
	err := r.pool.QueryRow(ctx, query, organizationID).Scan(
		&item.ID,
		&item.OrganizationID,
		&item.Content,
		&item.Version,
		&createdBy,
		&item.CreatedAt,
		&item.IsActive,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartnerOfferTerms{}, apperr.NotFound("partner offer terms not found")
	}
	if err != nil {
		return PartnerOfferTerms{}, fmt.Errorf("get active partner offer terms: %w", err)
	}
	item.CreatedByUserID = optionalTermsUUID(createdBy)
	return item, nil
}

func (r *Repository) UpsertPartnerOfferTerms(ctx context.Context, organizationID uuid.UUID, content string, createdByUserID uuid.UUID) (PartnerOfferTerms, error) {
	const deactivateQuery = `
		UPDATE RAC_partner_offer_terms
		SET is_active = FALSE
		WHERE organization_id = $1
		  AND is_active = TRUE`
	const insertQuery = `
		INSERT INTO RAC_partner_offer_terms (organization_id, content, version, created_by, is_active)
		VALUES (
			$1,
			$2,
			COALESCE((SELECT MAX(version) FROM RAC_partner_offer_terms WHERE organization_id = $1), 0) + 1,
			$3,
			TRUE
		)
		RETURNING id, organization_id, content, version, created_by, created_at, is_active`

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PartnerOfferTerms{}, fmt.Errorf("begin partner offer terms tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, deactivateQuery, organizationID); err != nil {
		return PartnerOfferTerms{}, fmt.Errorf("deactivate partner offer terms: %w", err)
	}

	var item PartnerOfferTerms
	var createdBy pgtype.UUID
	var createdByArg any
	if createdByUserID != uuid.Nil {
		createdByArg = createdByUserID
	}
	if err := tx.QueryRow(ctx, insertQuery, organizationID, strings.TrimSpace(content), createdByArg).Scan(
		&item.ID,
		&item.OrganizationID,
		&item.Content,
		&item.Version,
		&createdBy,
		&item.CreatedAt,
		&item.IsActive,
	); err != nil {
		return PartnerOfferTerms{}, fmt.Errorf("insert partner offer terms: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return PartnerOfferTerms{}, fmt.Errorf("commit partner offer terms tx: %w", err)
	}

	item.CreatedByUserID = optionalTermsUUID(createdBy)
	return item, nil
}

func (r *Repository) ListPartnerOfferTermsHistory(ctx context.Context, organizationID uuid.UUID) ([]PartnerOfferTerms, error) {
	const query = `
		SELECT id, organization_id, content, version, created_by, created_at, is_active
		FROM RAC_partner_offer_terms
		WHERE organization_id = $1
		ORDER BY version DESC, created_at DESC`

	rows, err := r.pool.Query(ctx, query, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list partner offer terms history: %w", err)
	}
	defer rows.Close()

	items := make([]PartnerOfferTerms, 0)
	for rows.Next() {
		var item PartnerOfferTerms
		var createdBy pgtype.UUID
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Content, &item.Version, &createdBy, &item.CreatedAt, &item.IsActive); err != nil {
			return nil, fmt.Errorf("scan partner offer terms history: %w", err)
		}
		item.CreatedByUserID = optionalTermsUUID(createdBy)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partner offer terms history: %w", err)
	}
	return items, nil
}

func optionalTermsUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	parsed := uuid.UUID(value.Bytes)
	return &parsed
}

func (r *Repository) UpdateOfferBuilderSummaryIfEmpty(ctx context.Context, offerID, organizationID uuid.UUID, summary string) error {
	if r == nil || r.pool == nil {
		return nil
	}

	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return nil
	}

	_, err := r.queries.UpdatePartnerOfferBuilderSummaryIfEmpty(ctx, partnersdb.UpdatePartnerOfferBuilderSummaryIfEmptyParams{
		Summary:        trimmed,
		OfferID:        toPgUUID(offerID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return fmt.Errorf("update offer builder summary: %w", err)
	}

	return nil
}

func (r *Repository) UpdateOfferJobSummaryShortIfEmpty(ctx context.Context, offerID, organizationID uuid.UUID, summary string) error {
	if r == nil || r.pool == nil {
		return nil
	}

	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return nil
	}

	_, err := r.queries.UpdatePartnerOfferJobSummaryShortIfEmpty(ctx, partnersdb.UpdatePartnerOfferJobSummaryShortIfEmptyParams{
		Summary:        trimmed,
		OfferID:        toPgUUID(offerID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return fmt.Errorf("update offer job summary short: %w", err)
	}

	return nil
}

// GetLatestQuoteItemsForService returns line items from the latest non-draft quote for a lead service.
func (r *Repository) GetLatestQuoteItemsForService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]QuoteItemSummary, error) {
	rows, err := r.queries.ListLatestQuoteItemsForService(ctx, partnersdb.ListLatestQuoteItemsForServiceParams{
		LeadServiceID:  toPgUUID(leadServiceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("query quote items for service: %w", err)
	}

	items := make([]QuoteItemSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, QuoteItemSummary{ID: uuid.UUID(row.ID.Bytes), Description: row.Description, Quantity: row.Quantity, UnitPriceCents: row.UnitPriceCents, LineTotalCents: row.LineTotalCents})
	}
	return items, nil
}

// GetQuoteForOffer retrieves the quote header needed for offer creation.
func (r *Repository) GetQuoteForOffer(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID) (QuoteForOffer, error) {
	row, err := r.queries.GetQuoteForPartnerOffer(ctx, partnersdb.GetQuoteForPartnerOfferParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteForOffer{}, apperr.NotFound("quote not found")
	}
	if err != nil {
		return QuoteForOffer{}, fmt.Errorf("get quote for offer: %w", err)
	}

	return QuoteForOffer{
		ID:             uuid.UUID(row.ID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		LeadID:         uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:  optionalUUID(row.LeadServiceID),
		Status:         row.Status,
		TotalCents:     row.TotalCents,
	}, nil
}

// GetQuoteItemsForQuote returns selected/non-optional line items for a specific quote.
func (r *Repository) GetQuoteItemsForQuote(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID) ([]QuoteItemSummary, error) {
	rows, err := r.queries.ListQuoteItemsForQuote(ctx, partnersdb.ListQuoteItemsForQuoteParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("query quote items for quote: %w", err)
	}

	items := make([]QuoteItemSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, QuoteItemSummary{ID: uuid.UUID(row.ID.Bytes), Description: row.Description, Quantity: row.Quantity, UnitPriceCents: row.UnitPriceCents, LineTotalCents: row.LineTotalCents})
	}
	return items, nil
}

func (r *Repository) GetLeadServiceImageAttachments(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]PhotoAttachment, error) {
	rows, err := r.queries.GetLeadServiceImageAttachments(ctx, partnersdb.GetLeadServiceImageAttachmentsParams{
		LeadServiceID:  toPgUUID(leadServiceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("query lead service image attachments: %w", err)
	}

	attachments := make([]PhotoAttachment, 0, len(rows))
	for _, row := range rows {
		contentType := ""
		if value := optionalString(row.ContentType); value != nil {
			contentType = *value
		}
		attachments = append(attachments, PhotoAttachment{
			ID:            uuid.UUID(row.ID.Bytes),
			LeadServiceID: uuid.UUID(row.LeadServiceID.Bytes),
			FileKey:       row.FileKey,
			FileName:      row.FileName,
			ContentType:   contentType,
		})
	}

	return attachments, nil
}

func (r *Repository) GetLeadServiceImageAttachmentByID(ctx context.Context, attachmentID, leadServiceID, organizationID uuid.UUID) (PhotoAttachment, error) {
	row, err := r.queries.GetLeadServiceImageAttachmentByID(ctx, partnersdb.GetLeadServiceImageAttachmentByIDParams{
		AttachmentID:   toPgUUID(attachmentID),
		LeadServiceID:  toPgUUID(leadServiceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PhotoAttachment{}, apperr.NotFound("attachment not found")
	}
	if err != nil {
		return PhotoAttachment{}, fmt.Errorf("get lead service image attachment: %w", err)
	}

	contentType := ""
	if value := optionalString(row.ContentType); value != nil {
		contentType = *value
	}

	return PhotoAttachment{
		ID:            uuid.UUID(row.ID.Bytes),
		LeadServiceID: uuid.UUID(row.LeadServiceID.Bytes),
		FileKey:       row.FileKey,
		FileName:      row.FileName,
		ContentType:   contentType,
	}, nil
}

// ListOffersForService returns all offers for a given lead service.
func (r *Repository) ListOffersForService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]PartnerOfferWithContext, error) {
	rows, err := r.queries.ListPartnerOffersForService(ctx, partnersdb.ListPartnerOffersForServiceParams{
		LeadServiceID:  toPgUUID(leadServiceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("list offers for service: %w", err)
	}

	offers := make([]PartnerOfferWithContext, 0, len(rows))
	for _, row := range rows {
		offer := offerFromSnapshot(offerSnapshot{ID: row.ID, OrganizationID: row.OrganizationID, PartnerID: row.PartnerID, LeadServiceID: row.LeadServiceID, PublicToken: row.PublicToken, ExpiresAt: row.ExpiresAt, PricingSource: row.PricingSource, CustomerPriceCents: row.CustomerPriceCents, VakmanPriceCents: row.VakmanPriceCents, MarginBasisPoints: row.MarginBasisPoints, OfferLineItems: row.OfferLineItems, Status: row.Status, AcceptedAt: row.AcceptedAt, RejectedAt: row.RejectedAt, RejectionReason: row.RejectionReason, InspectionAvailability: row.InspectionAvailability, JobAvailability: row.JobAvailability, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
		offers = append(offers, PartnerOfferWithContext{PartnerOffer: offer, PartnerName: row.BusinessName})
	}

	return offers, nil
}

// ListOffersByPartner returns all offers for a given partner within a tenant.
func (r *Repository) ListOffersByPartner(ctx context.Context, partnerID uuid.UUID, organizationID uuid.UUID) ([]PartnerOfferWithContext, error) {
	rows, err := r.queries.ListPartnerOffersByPartner(ctx, partnersdb.ListPartnerOffersByPartnerParams{
		PartnerID:      toPgUUID(partnerID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("list offers by partner: %w", err)
	}

	offers := make([]PartnerOfferWithContext, 0, len(rows))
	for _, row := range rows {
		offer := offerFromSnapshot(offerSnapshot{ID: row.ID, OrganizationID: row.OrganizationID, PartnerID: row.PartnerID, LeadServiceID: row.LeadServiceID, PublicToken: row.PublicToken, ExpiresAt: row.ExpiresAt, PricingSource: row.PricingSource, CustomerPriceCents: row.CustomerPriceCents, VakmanPriceCents: row.VakmanPriceCents, MarginBasisPoints: row.MarginBasisPoints, OfferLineItems: row.OfferLineItems, Status: row.Status, AcceptedAt: row.AcceptedAt, RejectedAt: row.RejectedAt, RejectionReason: row.RejectionReason, InspectionAvailability: row.InspectionAvailability, JobAvailability: row.JobAvailability, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
		offers = append(offers, offerWithContext(offerContext{Offer: offer, PartnerName: row.BusinessName, OrganizationName: row.Name, LeadCity: row.AddressCity, ServiceType: row.ServiceType, ServiceTypeID: row.ServiceTypeID}))
	}

	return offers, nil
}

// HasActiveOffer returns true if there is already a pending/sent offer for the lead service.
func (r *Repository) HasActiveOffer(ctx context.Context, leadServiceID uuid.UUID) (bool, error) {
	exists, err := r.queries.HasActivePartnerOffer(ctx, toPgUUID(leadServiceID))
	if err != nil {
		return false, fmt.Errorf("check active offer: %w", err)
	}
	return exists, nil
}

// AcceptOfferParams groups the acceptance data to avoid a long parameter list.
type AcceptOfferParams struct {
	OfferID            uuid.UUID
	InspectionSlots    []byte
	JobSlots           []byte
	SignerName         *string
	SignerBusinessName *string
	SignerAddress      *string
	SignatureData      *string
}

// AcceptOffer atomically accepts an offer and records availability + signer data.
func (r *Repository) AcceptOffer(ctx context.Context, p AcceptOfferParams) error {
	rowsAffected, err := r.queries.AcceptPartnerOffer(ctx, partnersdb.AcceptPartnerOfferParams{
		InspectionSlots:    p.InspectionSlots,
		JobSlots:           p.JobSlots,
		SignerName:         toPgText(p.SignerName),
		SignerBusinessName: toPgText(p.SignerBusinessName),
		SignerAddress:      toPgText(p.SignerAddress),
		SignatureData:      toPgText(p.SignatureData),
		OfferID:            toPgUUID(p.OfferID),
	})
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "idx_partner_offers_exclusive_acceptance") {
			return apperr.Conflict("job already assigned to another partner")
		}
		return fmt.Errorf("accept offer: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.Conflict("offer is not in a valid state to be accepted")
	}

	return nil
}

// SetOfferPDFFileKey persists the generated PDF file key on the offer record.
func (r *Repository) SetOfferPDFFileKey(ctx context.Context, offerID uuid.UUID, fileKey string) error {
	_, err := r.queries.SetPartnerOfferPDFFileKey(ctx, partnersdb.SetPartnerOfferPDFFileKeyParams{
		FileKey: fileKey,
		OfferID: toPgUUID(offerID),
	})
	if err != nil {
		return fmt.Errorf("set offer pdf file key: %w", err)
	}
	return nil
}

// RejectOffer marks an offer as rejected with an optional reason.
func (r *Repository) RejectOffer(ctx context.Context, offerID uuid.UUID, reason string) error {
	rowsAffected, err := r.queries.RejectPartnerOffer(ctx, partnersdb.RejectPartnerOfferParams{
		RejectionReason: optionalExactText(reason),
		OfferID:         toPgUUID(offerID),
	})
	if err != nil {
		return fmt.Errorf("reject offer: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.Conflict("offer is not in a valid state to be rejected")
	}

	return nil
}

// ExpireOffers marks all pending/sent offers past their expiry as expired.
func (r *Repository) ExpireOffers(ctx context.Context) ([]PartnerOffer, error) {
	rows, err := r.queries.ExpirePartnerOffers(ctx)
	if err != nil {
		return nil, fmt.Errorf("expire offers: %w", err)
	}

	expired := make([]PartnerOffer, 0, len(rows))
	for _, row := range rows {
		expired = append(expired, PartnerOffer{
			ID:             uuid.UUID(row.ID.Bytes),
			OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
			PartnerID:      uuid.UUID(row.PartnerID.Bytes),
			LeadServiceID:  uuid.UUID(row.LeadServiceID.Bytes),
		})
	}

	return expired, nil
}

// OfferListParams defines filters/sort/paging for the global offers overview.
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

	count, err := r.queries.CountPartnerOffers(ctx, partnersdb.CountPartnerOffersParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		Search:         optionalFilterText(params.Search),
		Status:         optionalExactText(params.Status),
		PartnerID:      optionalFilterUUID(params.PartnerID),
		LeadServiceID:  optionalFilterUUID(params.LeadServiceID),
		ServiceTypeID:  optionalFilterUUID(params.ServiceTypeID),
	})
	if err != nil {
		return OfferListResult{}, fmt.Errorf("count offers: %w", err)
	}

	total := int(count)
	totalPages := calcTotalPages(total, pageSize)

	rows, err := r.queries.ListPartnerOffers(ctx, partnersdb.ListPartnerOffersParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		Search:         optionalFilterText(params.Search),
		Status:         optionalExactText(params.Status),
		PartnerID:      optionalFilterUUID(params.PartnerID),
		LeadServiceID:  optionalFilterUUID(params.LeadServiceID),
		ServiceTypeID:  optionalFilterUUID(params.ServiceTypeID),
		SortBy:         sortCol,
		SortOrder:      orderBy,
		OffsetCount:    int32(offset),
		LimitCount:     int32(pageSize),
	})
	if err != nil {
		return OfferListResult{}, fmt.Errorf("list offers: %w", err)
	}

	offers := make([]PartnerOfferWithContext, 0, len(rows))
	for _, row := range rows {
		offer := offerFromSnapshot(offerSnapshot{ID: row.ID, OrganizationID: row.OrganizationID, PartnerID: row.PartnerID, LeadServiceID: row.LeadServiceID, PublicToken: row.PublicToken, ExpiresAt: row.ExpiresAt, PricingSource: row.PricingSource, CustomerPriceCents: row.CustomerPriceCents, VakmanPriceCents: row.VakmanPriceCents, MarginBasisPoints: row.MarginBasisPoints, OfferLineItems: row.OfferLineItems, JobSummaryShort: row.JobSummaryShort, BuilderSummary: row.BuilderSummary, Status: row.Status, AcceptedAt: row.AcceptedAt, RejectedAt: row.RejectedAt, RejectionReason: row.RejectionReason, InspectionAvailability: row.InspectionAvailability, JobAvailability: row.JobAvailability, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
		offers = append(offers, offerWithContext(offerContext{Offer: offer, PartnerName: row.BusinessName, OrganizationName: row.Name, LeadCity: row.AddressCity, ServiceType: row.ServiceType, ServiceTypeID: row.ServiceTypeID}))
	}

	return OfferListResult{Items: offers, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func resolveOfferSortBy(value string) (string, error) {
	if value == "" {
		return "createdAt", nil
	}
	switch value {
	case "createdAt", "expiresAt", "status", "partnerName", "serviceType", "vakmanPriceCents", "customerPriceCents":
		return value, nil
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

func unmarshalOfferLineItems(raw []byte) []OfferLineItem {
	if len(raw) == 0 {
		return nil
	}

	items := make([]OfferLineItem, 0)
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	return items
}

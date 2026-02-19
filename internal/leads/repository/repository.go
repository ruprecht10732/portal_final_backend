package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/apperr"
)

var ErrNotFound = errors.New("lead not found")

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListActiveServiceTypes returns active service types with intake guidelines for AI context.
func (r *Repository) ListActiveServiceTypes(ctx context.Context, organizationID uuid.UUID) ([]ServiceContextDefinition, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT name, description, intake_guidelines, estimation_guidelines
		FROM RAC_service_types
		WHERE organization_id = $1 AND is_active = true
		ORDER BY name ASC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ServiceContextDefinition, 0)
	for rows.Next() {
		var item ServiceContextDefinition
		if err := rows.Scan(&item.Name, &item.Description, &item.IntakeGuidelines, &item.EstimationGuidelines); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return items, nil
}

type Lead struct {
	ID                                      uuid.UUID
	OrganizationID                          uuid.UUID
	ConsumerFirstName                       string
	ConsumerLastName                        string
	ConsumerPhone                           string
	ConsumerEmail                           *string
	ConsumerRole                            string
	AddressStreet                           string
	AddressHouseNumber                      string
	AddressZipCode                          string
	AddressCity                             string
	Latitude                                *float64
	Longitude                               *float64
	AssignedAgentID                         *uuid.UUID
	Source                                  *string
	WhatsAppOptedIn                         bool
	EnergyClass                             *string
	EnergyIndex                             *float64
	EnergyBouwjaar                          *int
	EnergyGebouwtype                        *string
	EnergyLabelValidUntil                   *time.Time
	EnergyLabelRegisteredAt                 *time.Time
	EnergyPrimairFossiel                    *float64
	EnergyBAGVerblijfsobjectID              *string
	EnergyLabelFetchedAt                    *time.Time
	LeadEnrichmentSource                    *string
	LeadEnrichmentPostcode6                 *string
	LeadEnrichmentPostcode4                 *string
	LeadEnrichmentBuurtcode                 *string
	LeadEnrichmentDataYear                  *int
	LeadEnrichmentGemAardgasverbruik        *float64
	LeadEnrichmentGemElektriciteitsverbruik *float64
	LeadEnrichmentHuishoudenGrootte         *float64
	LeadEnrichmentKoopwoningenPct           *float64
	LeadEnrichmentBouwjaarVanaf2000Pct      *float64
	LeadEnrichmentWOZWaarde                 *float64
	LeadEnrichmentMediaanVermogenX1000      *float64
	LeadEnrichmentGemInkomen                *float64
	LeadEnrichmentPctHoogInkomen            *float64
	LeadEnrichmentPctLaagInkomen            *float64
	LeadEnrichmentHuishoudensMetKinderenPct *float64
	LeadEnrichmentStedelijkheid             *int
	LeadEnrichmentConfidence                *float64
	LeadEnrichmentFetchedAt                 *time.Time
	LeadScore                               *int
	LeadScorePreAI                          *int
	LeadScoreFactors                        []byte
	LeadScoreVersion                        *string
	LeadScoreUpdatedAt                      *time.Time
	ViewedByID                              *uuid.UUID
	ViewedAt                                *time.Time
	CreatedAt                               time.Time
	UpdatedAt                               time.Time
}

// LeadSummary is a lightweight lead representation for returning customer detection
type LeadSummary struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	ConsumerName    string
	ConsumerPhone   string
	ConsumerEmail   *string
	AddressCity     string
	ServiceCount    int
	LastServiceType *string
	LastStatus      *string
	CreatedAt       time.Time
}

type CreateLeadParams struct {
	OrganizationID     uuid.UUID
	ConsumerFirstName  string
	ConsumerLastName   string
	ConsumerPhone      string
	ConsumerEmail      *string
	ConsumerRole       string
	AddressStreet      string
	AddressHouseNumber string
	AddressZipCode     string
	AddressCity        string
	Latitude           *float64
	Longitude          *float64
	AssignedAgentID    *uuid.UUID
	Source             *string
	GCLID              *string
	UTMSource          *string
	UTMMedium          *string
	UTMCampaign        *string
	UTMContent         *string
	UTMTerm            *string
	AdLandingPage      *string
	ReferrerURL        *string
	WhatsAppOptedIn    bool
}

func (r *Repository) Create(ctx context.Context, params CreateLeadParams) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_leads (
			organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source,
			gclid, utm_source, utm_medium, utm_campaign, utm_content, utm_term, ad_landing_page, referrer_url,
			whatsapp_opted_in
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		RETURNING id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, whatsapp_opted_in, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
			energy_label_valid_until, energy_label_registered_at, energy_primair_fossiel, energy_bag_verblijfsobject_id,
			energy_label_fetched_at,
			lead_enrichment_source, lead_enrichment_postcode6, lead_enrichment_postcode4, lead_enrichment_buurtcode, lead_enrichment_data_year,
			lead_enrichment_gem_aardgasverbruik, lead_enrichment_gem_elektriciteitsverbruik, lead_enrichment_huishouden_grootte,
			lead_enrichment_koopwoningen_pct, lead_enrichment_bouwjaar_vanaf2000_pct, lead_enrichment_woz_waarde,
			lead_enrichment_mediaan_vermogen_x1000, lead_enrichment_gem_inkomen, lead_enrichment_pct_hoog_inkomen, lead_enrichment_pct_laag_inkomen,
			lead_enrichment_huishoudens_met_kinderen_pct, lead_enrichment_stedelijkheid, lead_enrichment_confidence, lead_enrichment_fetched_at,
			lead_score, lead_score_pre_ai, lead_score_factors, lead_score_version, lead_score_updated_at,
			viewed_by_id, viewed_at, created_at, updated_at
	`,
		params.OrganizationID, params.ConsumerFirstName, params.ConsumerLastName, params.ConsumerPhone, params.ConsumerEmail, params.ConsumerRole,
		params.AddressStreet, params.AddressHouseNumber, params.AddressZipCode, params.AddressCity, params.Latitude, params.Longitude,
		params.AssignedAgentID, params.Source,
		params.GCLID, params.UTMSource, params.UTMMedium, params.UTMCampaign, params.UTMContent, params.UTMTerm, params.AdLandingPage, params.ReferrerURL,
		params.WhatsAppOptedIn,
	).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.WhatsAppOptedIn, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
		&lead.EnergyLabelValidUntil, &lead.EnergyLabelRegisteredAt, &lead.EnergyPrimairFossiel, &lead.EnergyBAGVerblijfsobjectID,
		&lead.EnergyLabelFetchedAt,
		&lead.LeadEnrichmentSource, &lead.LeadEnrichmentPostcode6, &lead.LeadEnrichmentPostcode4, &lead.LeadEnrichmentBuurtcode, &lead.LeadEnrichmentDataYear,
		&lead.LeadEnrichmentGemAardgasverbruik, &lead.LeadEnrichmentGemElektriciteitsverbruik, &lead.LeadEnrichmentHuishoudenGrootte,
		&lead.LeadEnrichmentKoopwoningenPct, &lead.LeadEnrichmentBouwjaarVanaf2000Pct, &lead.LeadEnrichmentWOZWaarde,
		&lead.LeadEnrichmentMediaanVermogenX1000, &lead.LeadEnrichmentGemInkomen, &lead.LeadEnrichmentPctHoogInkomen, &lead.LeadEnrichmentPctLaagInkomen,
		&lead.LeadEnrichmentHuishoudensMetKinderenPct, &lead.LeadEnrichmentStedelijkheid, &lead.LeadEnrichmentConfidence, &lead.LeadEnrichmentFetchedAt,
		&lead.LeadScore, &lead.LeadScorePreAI, &lead.LeadScoreFactors, &lead.LeadScoreVersion, &lead.LeadScoreUpdatedAt,
		&lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return Lead{}, err
	}

	return lead, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, whatsapp_opted_in, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
			energy_label_valid_until, energy_label_registered_at, energy_primair_fossiel, energy_bag_verblijfsobject_id,
			energy_label_fetched_at,
			lead_enrichment_source, lead_enrichment_postcode6, lead_enrichment_postcode4, lead_enrichment_buurtcode, lead_enrichment_data_year,
			lead_enrichment_gem_aardgasverbruik, lead_enrichment_gem_elektriciteitsverbruik, lead_enrichment_huishouden_grootte,
			lead_enrichment_koopwoningen_pct, lead_enrichment_bouwjaar_vanaf2000_pct, lead_enrichment_woz_waarde,
			lead_enrichment_mediaan_vermogen_x1000, lead_enrichment_gem_inkomen, lead_enrichment_pct_hoog_inkomen, lead_enrichment_pct_laag_inkomen,
			lead_enrichment_huishoudens_met_kinderen_pct, lead_enrichment_stedelijkheid, lead_enrichment_confidence, lead_enrichment_fetched_at,
			lead_score, lead_score_pre_ai, lead_score_factors, lead_score_version, lead_score_updated_at,
			viewed_by_id, viewed_at, created_at, updated_at
		FROM RAC_leads WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, organizationID).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.WhatsAppOptedIn, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
		&lead.EnergyLabelValidUntil, &lead.EnergyLabelRegisteredAt, &lead.EnergyPrimairFossiel, &lead.EnergyBAGVerblijfsobjectID,
		&lead.EnergyLabelFetchedAt,
		&lead.LeadEnrichmentSource, &lead.LeadEnrichmentPostcode6, &lead.LeadEnrichmentPostcode4, &lead.LeadEnrichmentBuurtcode, &lead.LeadEnrichmentDataYear,
		&lead.LeadEnrichmentGemAardgasverbruik, &lead.LeadEnrichmentGemElektriciteitsverbruik, &lead.LeadEnrichmentHuishoudenGrootte,
		&lead.LeadEnrichmentKoopwoningenPct, &lead.LeadEnrichmentBouwjaarVanaf2000Pct, &lead.LeadEnrichmentWOZWaarde,
		&lead.LeadEnrichmentMediaanVermogenX1000, &lead.LeadEnrichmentGemInkomen, &lead.LeadEnrichmentPctHoogInkomen, &lead.LeadEnrichmentPctLaagInkomen,
		&lead.LeadEnrichmentHuishoudensMetKinderenPct, &lead.LeadEnrichmentStedelijkheid, &lead.LeadEnrichmentConfidence, &lead.LeadEnrichmentFetchedAt,
		&lead.LeadScore, &lead.LeadScorePreAI, &lead.LeadScoreFactors, &lead.LeadScoreVersion, &lead.LeadScoreUpdatedAt,
		&lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

// GetByIDWithServices returns a lead with all its services populated
func (r *Repository) GetByIDWithServices(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, []LeadService, error) {
	lead, err := r.GetByID(ctx, id, organizationID)
	if err != nil {
		return Lead{}, nil, err
	}

	services, err := r.ListLeadServices(ctx, id, organizationID)
	if err != nil {
		return Lead{}, nil, err
	}

	return lead, services, nil
}

func (r *Repository) GetByPhone(ctx context.Context, phone string, organizationID uuid.UUID) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, whatsapp_opted_in, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
			energy_label_valid_until, energy_label_registered_at, energy_primair_fossiel, energy_bag_verblijfsobject_id,
			energy_label_fetched_at,
			lead_enrichment_source, lead_enrichment_postcode6, lead_enrichment_postcode4, lead_enrichment_buurtcode, lead_enrichment_data_year,
			lead_enrichment_gem_aardgasverbruik, lead_enrichment_gem_elektriciteitsverbruik, lead_enrichment_huishouden_grootte,
			lead_enrichment_koopwoningen_pct, lead_enrichment_bouwjaar_vanaf2000_pct, lead_enrichment_woz_waarde,
			lead_enrichment_mediaan_vermogen_x1000, lead_enrichment_gem_inkomen, lead_enrichment_pct_hoog_inkomen, lead_enrichment_pct_laag_inkomen,
			lead_enrichment_huishoudens_met_kinderen_pct, lead_enrichment_stedelijkheid, lead_enrichment_confidence, lead_enrichment_fetched_at,
			lead_score, lead_score_pre_ai, lead_score_factors, lead_score_version, lead_score_updated_at,
			viewed_by_id, viewed_at, created_at, updated_at
		FROM RAC_leads WHERE consumer_phone = $1 AND organization_id = $2 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, phone, organizationID).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.WhatsAppOptedIn, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
		&lead.EnergyLabelValidUntil, &lead.EnergyLabelRegisteredAt, &lead.EnergyPrimairFossiel, &lead.EnergyBAGVerblijfsobjectID,
		&lead.EnergyLabelFetchedAt,
		&lead.LeadEnrichmentSource, &lead.LeadEnrichmentPostcode6, &lead.LeadEnrichmentPostcode4, &lead.LeadEnrichmentBuurtcode, &lead.LeadEnrichmentDataYear,
		&lead.LeadEnrichmentGemAardgasverbruik, &lead.LeadEnrichmentGemElektriciteitsverbruik, &lead.LeadEnrichmentHuishoudenGrootte,
		&lead.LeadEnrichmentKoopwoningenPct, &lead.LeadEnrichmentBouwjaarVanaf2000Pct, &lead.LeadEnrichmentWOZWaarde,
		&lead.LeadEnrichmentMediaanVermogenX1000, &lead.LeadEnrichmentGemInkomen, &lead.LeadEnrichmentPctHoogInkomen, &lead.LeadEnrichmentPctLaagInkomen,
		&lead.LeadEnrichmentHuishoudensMetKinderenPct, &lead.LeadEnrichmentStedelijkheid, &lead.LeadEnrichmentConfidence, &lead.LeadEnrichmentFetchedAt,
		&lead.LeadScore, &lead.LeadScorePreAI, &lead.LeadScoreFactors, &lead.LeadScoreVersion, &lead.LeadScoreUpdatedAt,
		&lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) IsWhatsAppOptedIn(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	var optedIn bool
	err := r.pool.QueryRow(ctx, `
		SELECT whatsapp_opted_in
		FROM RAC_leads
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, organizationID).Scan(&optedIn)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return optedIn, err
}

// GetByPhoneOrEmail finds a lead matching the given phone or email for returning customer detection.
// Returns the first matching lead with its services, or nil if not found.
func (r *Repository) GetByPhoneOrEmail(ctx context.Context, phone string, email string, organizationID uuid.UUID) (*LeadSummary, []LeadService, error) {
	if phone == "" && email == "" {
		return nil, nil, nil
	}

	var summary LeadSummary
	err := r.pool.QueryRow(ctx, `
		SELECT 
			l.id,
			l.organization_id,
			l.consumer_first_name || ' ' || l.consumer_last_name AS consumer_name,
			l.consumer_phone,
			l.consumer_email,
			l.address_city,
			COUNT(ls.id) AS service_count,
			(SELECT st.name FROM RAC_lead_services ls2 
			 JOIN RAC_service_types st ON st.id = ls2.service_type_id AND st.organization_id = l.organization_id
			 WHERE ls2.lead_id = l.id ORDER BY ls2.created_at DESC LIMIT 1) AS last_service_type,
			(SELECT ls2.status FROM RAC_lead_services ls2 
			 WHERE ls2.lead_id = l.id ORDER BY ls2.created_at DESC LIMIT 1) AS last_status,
			l.created_at
		FROM RAC_leads l
		LEFT JOIN RAC_lead_services ls ON ls.lead_id = l.id
		WHERE l.deleted_at IS NULL 
		  AND l.organization_id = $3
		  AND (($1 != '' AND l.consumer_phone = $1) OR ($2 != '' AND l.consumer_email = $2))
		GROUP BY l.id
		ORDER BY l.created_at DESC
		LIMIT 1
	`, phone, email, organizationID).Scan(
		&summary.ID, &summary.OrganizationID, &summary.ConsumerName, &summary.ConsumerPhone, &summary.ConsumerEmail,
		&summary.AddressCity, &summary.ServiceCount, &summary.LastServiceType, &summary.LastStatus,
		&summary.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	// Fetch services for the found lead
	services, err := r.ListLeadServices(ctx, summary.ID, organizationID)
	if err != nil {
		return nil, nil, err
	}

	return &summary, services, nil
}

// GetLatestAcceptedQuoteIDForService returns the most recent Accepted quote ID for a lead service.
// This is used by agent tooling to create partner offers in quote-only mode.
func (r *Repository) GetLatestAcceptedQuoteIDForService(ctx context.Context, serviceID, organizationID uuid.UUID) (uuid.UUID, error) {
	var quoteID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id
		FROM RAC_quotes
		WHERE lead_service_id = $1
			AND organization_id = $2
			AND status = 'Accepted'
			AND total_cents > 0
		ORDER BY created_at DESC
		LIMIT 1
	`, serviceID, organizationID).Scan(&quoteID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, apperr.NotFound("accepted quote not found")
	}
	if err != nil {
		return uuid.Nil, err
	}
	return quoteID, nil
}

func (r *Repository) HasNonDraftQuote(ctx context.Context, serviceID, organizationID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM RAC_quotes
			WHERE lead_service_id = $1
			  AND organization_id = $2
			  AND status <> 'Draft'
		)
	`, serviceID, organizationID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) GetLatestDraftQuoteID(ctx context.Context, serviceID, organizationID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id
		FROM RAC_quotes
		WHERE lead_service_id = $1
		  AND organization_id = $2
		  AND status = 'Draft'
		ORDER BY created_at DESC
		LIMIT 1
	`, serviceID, organizationID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

type UpdateLeadParams struct {
	ConsumerFirstName  *string
	ConsumerLastName   *string
	ConsumerPhone      *string
	ConsumerEmail      *string
	ConsumerRole       *string
	AddressStreet      *string
	AddressHouseNumber *string
	AddressZipCode     *string
	AddressCity        *string
	Latitude           *float64
	Longitude          *float64
	AssignedAgentID    *uuid.UUID
	AssignedAgentIDSet bool
	WhatsAppOptedIn    *bool
	WhatsAppOptedInSet bool
}

type UpdateEnergyLabelParams struct {
	Class          *string
	Index          *float64
	Bouwjaar       *int
	Gebouwtype     *string
	ValidUntil     *time.Time
	RegisteredAt   *time.Time
	PrimairFossiel *float64
	BAGObjectID    *string
	FetchedAt      time.Time
}

type UpdateLeadEnrichmentParams struct {
	Source                    *string
	Postcode6                 *string
	Postcode4                 *string
	Buurtcode                 *string
	DataYear                  *int
	GemAardgasverbruik        *float64
	GemElektriciteitsverbruik *float64
	HuishoudenGrootte         *float64
	KoopwoningenPct           *float64
	BouwjaarVanaf2000Pct      *float64
	WOZWaarde                 *float64
	MediaanVermogenX1000      *float64
	GemInkomen                *float64
	PctHoogInkomen            *float64
	PctLaagInkomen            *float64
	HuishoudensMetKinderenPct *float64
	Stedelijkheid             *int
	Confidence                *float64
	FetchedAt                 time.Time
	Score                     *int
	ScorePreAI                *int
	ScoreFactors              []byte
	ScoreVersion              *string
	ScoreUpdatedAt            *time.Time
}

type UpdateLeadScoreParams struct {
	Score          *int
	ScorePreAI     *int
	ScoreFactors   []byte
	ScoreVersion   *string
	ScoreUpdatedAt time.Time
}

func nullable[T any](value *T) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadParams) (Lead, error) {
	hasUpdates := params.ConsumerFirstName != nil ||
		params.ConsumerLastName != nil ||
		params.ConsumerPhone != nil ||
		params.ConsumerEmail != nil ||
		params.ConsumerRole != nil ||
		params.AddressStreet != nil ||
		params.AddressHouseNumber != nil ||
		params.AddressZipCode != nil ||
		params.AddressCity != nil ||
		params.Latitude != nil ||
		params.Longitude != nil ||
		params.AssignedAgentIDSet ||
		params.WhatsAppOptedInSet

	if !hasUpdates {
		return r.GetByID(ctx, id, organizationID)
	}

	query := `
		UPDATE RAC_leads
		SET
			consumer_first_name = COALESCE($3, consumer_first_name),
			consumer_last_name = COALESCE($4, consumer_last_name),
			consumer_phone = COALESCE($5, consumer_phone),
			consumer_email = COALESCE($6, consumer_email),
			consumer_role = COALESCE($7, consumer_role),
			address_street = COALESCE($8, address_street),
			address_house_number = COALESCE($9, address_house_number),
			address_zip_code = COALESCE($10, address_zip_code),
			address_city = COALESCE($11, address_city),
			latitude = COALESCE($12, latitude),
			longitude = COALESCE($13, longitude),
			assigned_agent_id = CASE WHEN $15 THEN $14 ELSE assigned_agent_id END,
			whatsapp_opted_in = CASE WHEN $17 THEN $16 ELSE whatsapp_opted_in END,
			updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		RETURNING id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, whatsapp_opted_in, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
			energy_label_valid_until, energy_label_registered_at, energy_primair_fossiel, energy_bag_verblijfsobject_id,
			energy_label_fetched_at,
			lead_enrichment_source, lead_enrichment_postcode6, lead_enrichment_postcode4, lead_enrichment_buurtcode, lead_enrichment_data_year,
			lead_enrichment_gem_aardgasverbruik, lead_enrichment_gem_elektriciteitsverbruik, lead_enrichment_huishouden_grootte,
			lead_enrichment_koopwoningen_pct, lead_enrichment_bouwjaar_vanaf2000_pct, lead_enrichment_woz_waarde,
			lead_enrichment_mediaan_vermogen_x1000, lead_enrichment_gem_inkomen, lead_enrichment_pct_hoog_inkomen, lead_enrichment_pct_laag_inkomen,
			lead_enrichment_huishoudens_met_kinderen_pct, lead_enrichment_stedelijkheid, lead_enrichment_confidence, lead_enrichment_fetched_at,
			lead_score, lead_score_pre_ai, lead_score_factors, lead_score_version, lead_score_updated_at,
			viewed_by_id, viewed_at, created_at, updated_at
	`

	var assignedAgentParam interface{}
	if params.AssignedAgentIDSet {
		assignedAgentParam = params.AssignedAgentID
	}

	var lead Lead
	err := r.pool.QueryRow(
		ctx,
		query,
		id,
		organizationID,
		nullable(params.ConsumerFirstName),
		nullable(params.ConsumerLastName),
		nullable(params.ConsumerPhone),
		nullable(params.ConsumerEmail),
		nullable(params.ConsumerRole),
		nullable(params.AddressStreet),
		nullable(params.AddressHouseNumber),
		nullable(params.AddressZipCode),
		nullable(params.AddressCity),
		nullable(params.Latitude),
		nullable(params.Longitude),
		assignedAgentParam,
		params.AssignedAgentIDSet,
		nullable(params.WhatsAppOptedIn),
		params.WhatsAppOptedInSet,
	).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.WhatsAppOptedIn, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
		&lead.EnergyLabelValidUntil, &lead.EnergyLabelRegisteredAt, &lead.EnergyPrimairFossiel, &lead.EnergyBAGVerblijfsobjectID,
		&lead.EnergyLabelFetchedAt,
		&lead.LeadEnrichmentSource, &lead.LeadEnrichmentPostcode6, &lead.LeadEnrichmentPostcode4, &lead.LeadEnrichmentBuurtcode, &lead.LeadEnrichmentDataYear,
		&lead.LeadEnrichmentGemAardgasverbruik, &lead.LeadEnrichmentGemElektriciteitsverbruik, &lead.LeadEnrichmentHuishoudenGrootte,
		&lead.LeadEnrichmentKoopwoningenPct, &lead.LeadEnrichmentBouwjaarVanaf2000Pct, &lead.LeadEnrichmentWOZWaarde,
		&lead.LeadEnrichmentMediaanVermogenX1000, &lead.LeadEnrichmentGemInkomen, &lead.LeadEnrichmentPctHoogInkomen, &lead.LeadEnrichmentPctLaagInkomen,
		&lead.LeadEnrichmentHuishoudensMetKinderenPct, &lead.LeadEnrichmentStedelijkheid, &lead.LeadEnrichmentConfidence, &lead.LeadEnrichmentFetchedAt,
		&lead.LeadScore, &lead.LeadScorePreAI, &lead.LeadScoreFactors, &lead.LeadScoreVersion, &lead.LeadScoreUpdatedAt,
		&lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) UpdateEnergyLabel(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateEnergyLabelParams) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE RAC_leads
		SET energy_class = $3,
			energy_index = $4,
			energy_bouwjaar = $5,
			energy_gebouwtype = $6,
			energy_label_valid_until = $7,
			energy_label_registered_at = $8,
			energy_primair_fossiel = $9,
			energy_bag_verblijfsobject_id = $10,
			energy_label_fetched_at = $11,
			updated_at = $12
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`,
		id,
		organizationID,
		nullable(params.Class),
		nullable(params.Index),
		nullable(params.Bouwjaar),
		nullable(params.Gebouwtype),
		nullable(params.ValidUntil),
		nullable(params.RegisteredAt),
		nullable(params.PrimairFossiel),
		nullable(params.BAGObjectID),
		params.FetchedAt,
		params.FetchedAt,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateLeadEnrichment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadEnrichmentParams) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE RAC_leads
		SET lead_enrichment_source = $3,
			lead_enrichment_postcode6 = $4,
			lead_enrichment_postcode4 = $5,
			lead_enrichment_buurtcode = $6,
			lead_enrichment_data_year = $7,
			lead_enrichment_gem_aardgasverbruik = $8,
			lead_enrichment_gem_elektriciteitsverbruik = $9,
			lead_enrichment_huishouden_grootte = $10,
			lead_enrichment_koopwoningen_pct = $11,
			lead_enrichment_bouwjaar_vanaf2000_pct = $12,
			lead_enrichment_woz_waarde = $13,
			lead_enrichment_mediaan_vermogen_x1000 = $14,
			lead_enrichment_gem_inkomen = $15,
			lead_enrichment_pct_hoog_inkomen = $16,
			lead_enrichment_pct_laag_inkomen = $17,
			lead_enrichment_huishoudens_met_kinderen_pct = $18,
			lead_enrichment_stedelijkheid = $19,
			lead_enrichment_confidence = $20,
			lead_enrichment_fetched_at = $21,
			lead_score = $22,
			lead_score_pre_ai = $23,
			lead_score_factors = $24,
			lead_score_version = $25,
			lead_score_updated_at = $26,
			updated_at = $27
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`,
		id,
		organizationID,
		nullable(params.Source),
		nullable(params.Postcode6),
		nullable(params.Postcode4),
		nullable(params.Buurtcode),
		nullable(params.DataYear),
		nullable(params.GemAardgasverbruik),
		nullable(params.GemElektriciteitsverbruik),
		nullable(params.HuishoudenGrootte),
		nullable(params.KoopwoningenPct),
		nullable(params.BouwjaarVanaf2000Pct),
		nullable(params.WOZWaarde),
		nullable(params.MediaanVermogenX1000),
		nullable(params.GemInkomen),
		nullable(params.PctHoogInkomen),
		nullable(params.PctLaagInkomen),
		nullable(params.HuishoudensMetKinderenPct),
		nullable(params.Stedelijkheid),
		nullable(params.Confidence),
		params.FetchedAt,
		nullable(params.Score),
		nullable(params.ScorePreAI),
		params.ScoreFactors,
		nullable(params.ScoreVersion),
		nullable(params.ScoreUpdatedAt),
		params.FetchedAt,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateLeadScore(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadScoreParams) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE RAC_leads
		SET lead_score = $3,
			lead_score_pre_ai = $4,
			lead_score_factors = $5,
			lead_score_version = $6,
			lead_score_updated_at = $7,
			updated_at = $8
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`,
		id,
		organizationID,
		nullable(params.Score),
		nullable(params.ScorePreAI),
		params.ScoreFactors,
		nullable(params.ScoreVersion),
		params.ScoreUpdatedAt,
		params.ScoreUpdatedAt,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateProjectedValueCents(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, projectedValueCents int64) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE RAC_leads
		SET projected_value_cents = $3, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, organizationID, projectedValueCents)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SetViewedBy(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_leads SET viewed_by_id = $3, viewed_at = now(), updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, organizationID, userID)
	return err
}

func (r *Repository) AddActivity(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID, userID uuid.UUID, action string, meta map[string]interface{}) error {
	var metaJSON []byte
	if meta != nil {
		encoded, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		metaJSON = encoded
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO RAC_lead_activity (lead_id, organization_id, user_id, action, meta)
		VALUES ($1, $2, $3, $4, $5)
	`, leadID, organizationID, userID, action, metaJSON)
	return err
}

type ListParams struct {
	OrganizationID  uuid.UUID
	Status          *string
	ServiceType     *string
	Search          string
	FirstName       *string
	LastName        *string
	Phone           *string
	Email           *string
	Role            *string
	Street          *string
	HouseNumber     *string
	ZipCode         *string
	City            *string
	AssignedAgentID *uuid.UUID
	CreatedAtFrom   *time.Time
	CreatedAtTo     *time.Time
	Offset          int
	Limit           int
	SortBy          string
	SortOrder       string
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]Lead, int, error) {
	filters := buildLeadListFilters(params)

	sortBy, err := resolveLeadSortBy(params.SortBy)
	if err != nil {
		return nil, 0, err
	}

	sortOrder, err := resolveLeadSortOrder(params.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	baseQuery := `
		FROM RAC_leads l
		LEFT JOIN LATERAL (
			SELECT ls.id, ls.status, ls.service_type_id
			FROM RAC_lead_services ls
			WHERE ls.lead_id = l.id AND ls.pipeline_stage NOT IN ('Completed', 'Lost') AND ls.status != 'Disqualified'
			ORDER BY ls.created_at DESC
			LIMIT 1
		) cs ON true
		LEFT JOIN RAC_service_types st ON st.id = cs.service_type_id AND st.organization_id = l.organization_id
		WHERE l.organization_id = $1
			AND l.deleted_at IS NULL
			AND ($2::text IS NULL OR cs.status = $2)
			AND ($3::text IS NULL OR st.name = $3)
			AND ($4::text IS NULL OR (
				l.consumer_first_name ILIKE $4 OR l.consumer_last_name ILIKE $4 OR l.consumer_phone ILIKE $4 OR l.consumer_email ILIKE $4 OR l.address_city ILIKE $4
			))
			AND ($5::text IS NULL OR l.consumer_first_name ILIKE $5)
			AND ($6::text IS NULL OR l.consumer_last_name ILIKE $6)
			AND ($7::text IS NULL OR l.consumer_phone ILIKE $7)
			AND ($8::text IS NULL OR l.consumer_email ILIKE $8)
			AND ($9::text IS NULL OR l.consumer_role = $9)
			AND ($10::text IS NULL OR l.address_street ILIKE $10)
			AND ($11::text IS NULL OR l.address_house_number ILIKE $11)
			AND ($12::text IS NULL OR l.address_zip_code ILIKE $12)
			AND ($13::text IS NULL OR l.address_city ILIKE $13)
			AND ($14::uuid IS NULL OR l.assigned_agent_id = $14)
			AND ($15::timestamptz IS NULL OR l.created_at >= $15)
			AND ($16::timestamptz IS NULL OR l.created_at < $16)
	`

	args := []interface{}{
		params.OrganizationID,
		filters.status,
		filters.serviceType,
		filters.search,
		filters.firstName,
		filters.lastName,
		filters.phone,
		filters.email,
		filters.role,
		filters.street,
		filters.houseNumber,
		filters.zipCode,
		filters.city,
		filters.assignedAgentID,
		filters.createdAtFrom,
		filters.createdAtTo,
	}

	var total int
	countQuery := "SELECT COUNT(DISTINCT l.id) " + baseQuery
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	innerQuery := `
		SELECT DISTINCT l.id, l.organization_id, l.consumer_first_name, l.consumer_last_name, l.consumer_phone, l.consumer_email, l.consumer_role,
			l.address_street, l.address_house_number, l.address_zip_code, l.address_city, l.latitude, l.longitude,
			l.assigned_agent_id, l.source, l.whatsapp_opted_in, l.energy_class, l.energy_index, l.energy_bouwjaar, l.energy_gebouwtype,
			l.energy_label_valid_until, l.energy_label_registered_at, l.energy_primair_fossiel, l.energy_bag_verblijfsobject_id,
			l.energy_label_fetched_at,
			l.lead_enrichment_source, l.lead_enrichment_postcode6, l.lead_enrichment_postcode4, l.lead_enrichment_buurtcode, l.lead_enrichment_data_year,
			l.lead_enrichment_gem_aardgasverbruik, l.lead_enrichment_gem_elektriciteitsverbruik, l.lead_enrichment_huishouden_grootte,
			l.lead_enrichment_koopwoningen_pct, l.lead_enrichment_bouwjaar_vanaf2000_pct, l.lead_enrichment_woz_waarde,
			l.lead_enrichment_mediaan_vermogen_x1000, l.lead_enrichment_gem_inkomen, l.lead_enrichment_pct_hoog_inkomen, l.lead_enrichment_pct_laag_inkomen,
			l.lead_enrichment_huishoudens_met_kinderen_pct, l.lead_enrichment_stedelijkheid, l.lead_enrichment_confidence, l.lead_enrichment_fetched_at,
			l.lead_score, l.lead_score_pre_ai, l.lead_score_factors, l.lead_score_version, l.lead_score_updated_at,
			l.viewed_by_id, l.viewed_at, l.created_at, l.updated_at
		` + baseQuery + `
	`

	query := `
		SELECT * FROM (
			` + innerQuery + `
		) leads
		ORDER BY
			CASE WHEN $17 = 'createdAt' AND $18 = 'asc' THEN leads.created_at END ASC,
			CASE WHEN $17 = 'createdAt' AND $18 = 'desc' THEN leads.created_at END DESC,
			CASE WHEN $17 = 'firstName' AND $18 = 'asc' THEN leads.consumer_first_name END ASC,
			CASE WHEN $17 = 'firstName' AND $18 = 'desc' THEN leads.consumer_first_name END DESC,
			CASE WHEN $17 = 'lastName' AND $18 = 'asc' THEN leads.consumer_last_name END ASC,
			CASE WHEN $17 = 'lastName' AND $18 = 'desc' THEN leads.consumer_last_name END DESC,
			CASE WHEN $17 = 'phone' AND $18 = 'asc' THEN leads.consumer_phone END ASC,
			CASE WHEN $17 = 'phone' AND $18 = 'desc' THEN leads.consumer_phone END DESC,
			CASE WHEN $17 = 'email' AND $18 = 'asc' THEN leads.consumer_email END ASC,
			CASE WHEN $17 = 'email' AND $18 = 'desc' THEN leads.consumer_email END DESC,
			CASE WHEN $17 = 'role' AND $18 = 'asc' THEN leads.consumer_role END ASC,
			CASE WHEN $17 = 'role' AND $18 = 'desc' THEN leads.consumer_role END DESC,
			CASE WHEN $17 = 'street' AND $18 = 'asc' THEN leads.address_street END ASC,
			CASE WHEN $17 = 'street' AND $18 = 'desc' THEN leads.address_street END DESC,
			CASE WHEN $17 = 'houseNumber' AND $18 = 'asc' THEN leads.address_house_number END ASC,
			CASE WHEN $17 = 'houseNumber' AND $18 = 'desc' THEN leads.address_house_number END DESC,
			CASE WHEN $17 = 'zipCode' AND $18 = 'asc' THEN leads.address_zip_code END ASC,
			CASE WHEN $17 = 'zipCode' AND $18 = 'desc' THEN leads.address_zip_code END DESC,
			CASE WHEN $17 = 'city' AND $18 = 'asc' THEN leads.address_city END ASC,
			CASE WHEN $17 = 'city' AND $18 = 'desc' THEN leads.address_city END DESC,
			CASE WHEN $17 = 'assignedAgentId' AND $18 = 'asc' THEN leads.assigned_agent_id END ASC,
			CASE WHEN $17 = 'assignedAgentId' AND $18 = 'desc' THEN leads.assigned_agent_id END DESC,
			leads.created_at DESC
		LIMIT $19 OFFSET $20
	`

	args = append(args, sortBy, sortOrder, params.Limit, params.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	leads := make([]Lead, 0)
	for rows.Next() {
		var lead Lead
		if err := rows.Scan(
			&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
			&lead.AssignedAgentID, &lead.Source, &lead.WhatsAppOptedIn, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
			&lead.EnergyLabelValidUntil, &lead.EnergyLabelRegisteredAt, &lead.EnergyPrimairFossiel, &lead.EnergyBAGVerblijfsobjectID,
			&lead.EnergyLabelFetchedAt,
			&lead.LeadEnrichmentSource, &lead.LeadEnrichmentPostcode6, &lead.LeadEnrichmentPostcode4, &lead.LeadEnrichmentBuurtcode, &lead.LeadEnrichmentDataYear,
			&lead.LeadEnrichmentGemAardgasverbruik, &lead.LeadEnrichmentGemElektriciteitsverbruik, &lead.LeadEnrichmentHuishoudenGrootte,
			&lead.LeadEnrichmentKoopwoningenPct, &lead.LeadEnrichmentBouwjaarVanaf2000Pct, &lead.LeadEnrichmentWOZWaarde,
			&lead.LeadEnrichmentMediaanVermogenX1000, &lead.LeadEnrichmentGemInkomen, &lead.LeadEnrichmentPctHoogInkomen, &lead.LeadEnrichmentPctLaagInkomen,
			&lead.LeadEnrichmentHuishoudensMetKinderenPct, &lead.LeadEnrichmentStedelijkheid, &lead.LeadEnrichmentConfidence, &lead.LeadEnrichmentFetchedAt,
			&lead.LeadScore, &lead.LeadScorePreAI, &lead.LeadScoreFactors, &lead.LeadScoreVersion, &lead.LeadScoreUpdatedAt,
			&lead.ViewedByID, &lead.ViewedAt,
			&lead.CreatedAt, &lead.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		leads = append(leads, lead)
	}

	if rows.Err() != nil {
		return nil, 0, rows.Err()
	}

	return leads, total, nil
}

type leadListFilters struct {
	status          interface{}
	serviceType     interface{}
	search          interface{}
	firstName       interface{}
	lastName        interface{}
	phone           interface{}
	email           interface{}
	role            interface{}
	street          interface{}
	houseNumber     interface{}
	zipCode         interface{}
	city            interface{}
	assignedAgentID interface{}
	createdAtFrom   interface{}
	createdAtTo     interface{}
}

func buildLeadListFilters(params ListParams) leadListFilters {
	return leadListFilters{
		status:          nullable(params.Status),
		serviceType:     nullable(params.ServiceType),
		search:          optionalSearchParam(params.Search),
		firstName:       optionalLikeParam(params.FirstName),
		lastName:        optionalLikeParam(params.LastName),
		phone:           optionalLikeParam(params.Phone),
		email:           optionalLikeParam(params.Email),
		role:            nullable(params.Role),
		street:          optionalLikeParam(params.Street),
		houseNumber:     optionalLikeParam(params.HouseNumber),
		zipCode:         optionalLikeParam(params.ZipCode),
		city:            optionalLikeParam(params.City),
		assignedAgentID: nullable(params.AssignedAgentID),
		createdAtFrom:   nullable(params.CreatedAtFrom),
		createdAtTo:     nullable(params.CreatedAtTo),
	}
}

func optionalLikeParam(value *string) interface{} {
	if value == nil {
		return nil
	}
	return "%" + *value + "%"
}

func optionalSearchParam(value string) interface{} {
	if value == "" {
		return nil
	}
	return "%" + value + "%"
}

func resolveLeadSortBy(sortBy string) (string, error) {
	if sortBy == "" {
		return "createdAt", nil
	}
	switch sortBy {
	case "createdAt", "firstName", "lastName", "phone", "email", "role", "street", "houseNumber", "zipCode", "city", "assignedAgentId":
		return sortBy, nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

func resolveLeadSortOrder(sortOrder string) (string, error) {
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

type HeatmapPoint struct {
	Latitude  float64
	Longitude float64
}

func (r *Repository) ListHeatmapPoints(ctx context.Context, organizationID uuid.UUID, startDate *time.Time, endDate *time.Time) ([]HeatmapPoint, error) {
	var startParam interface{}
	if startDate != nil {
		startParam = *startDate
	}
	var endParam interface{}
	if endDate != nil {
		endParam = *endDate
	}

	query := `
		SELECT latitude, longitude
		FROM RAC_leads
		WHERE organization_id = $1
			AND deleted_at IS NULL
			AND latitude IS NOT NULL
			AND longitude IS NOT NULL
			AND ($2::timestamptz IS NULL OR created_at >= $2)
			AND ($3::timestamptz IS NULL OR created_at < $3)
	`

	rows, err := r.pool.Query(ctx, query, organizationID, startParam, endParam)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]HeatmapPoint, 0)
	for rows.Next() {
		var point HeatmapPoint
		if err := rows.Scan(&point.Latitude, &point.Longitude); err != nil {
			return nil, err
		}
		points = append(points, point)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return points, nil
}

type ActionItem struct {
	ID            uuid.UUID
	FirstName     string
	LastName      string
	UrgencyLevel  *string
	UrgencyReason *string
	CreatedAt     time.Time
}

type ActionItemListResult struct {
	Items []ActionItem
	Total int
}

func (r *Repository) ListActionItems(ctx context.Context, organizationID uuid.UUID, newLeadDays int, limit int, offset int) (ActionItemListResult, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM RAC_leads l
		LEFT JOIN (
			SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
			FROM RAC_lead_ai_analysis
			ORDER BY lead_id, created_at DESC
		) ai ON ai.lead_id = l.id
		WHERE l.organization_id = $1
			AND l.deleted_at IS NULL
			AND (ai.urgency_level = 'High' OR l.created_at >= now() - ($2::int || ' days')::interval)
	`

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, organizationID, newLeadDays).Scan(&total); err != nil {
		return ActionItemListResult{}, err
	}

	query := `
		SELECT l.id, l.consumer_first_name, l.consumer_last_name, ai.urgency_level, ai.urgency_reason, l.created_at
		FROM RAC_leads l
		LEFT JOIN (
			SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
			FROM RAC_lead_ai_analysis
			ORDER BY lead_id, created_at DESC
		) ai ON ai.lead_id = l.id
		WHERE l.organization_id = $1
			AND l.deleted_at IS NULL
			AND (ai.urgency_level = 'High' OR l.created_at >= now() - ($2::int || ' days')::interval)
		ORDER BY
			CASE WHEN ai.urgency_level = 'High' THEN 0 ELSE 1 END,
			l.created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(ctx, query, organizationID, newLeadDays, limit, offset)
	if err != nil {
		return ActionItemListResult{}, err
	}
	defer rows.Close()

	items := make([]ActionItem, 0)
	for rows.Next() {
		var item ActionItem
		if err := rows.Scan(&item.ID, &item.FirstName, &item.LastName, &item.UrgencyLevel, &item.UrgencyReason, &item.CreatedAt); err != nil {
			return ActionItemListResult{}, err
		}
		items = append(items, item)
	}

	if rows.Err() != nil {
		return ActionItemListResult{}, rows.Err()
	}

	return ActionItemListResult{Items: items, Total: total}, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, "UPDATE RAC_leads SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL", id, organizationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRecentActivity returns the most recent org-wide activity by unioning
// lead activity, quote activity, and recent appointments.
// Events are clustered: sequential events with the same lead, event type, and category
// within a 15-minute window are grouped into a single row with a group_count.
func (r *Repository) ListRecentActivity(ctx context.Context, organizationID uuid.UUID, limit int, offset int) ([]ActivityFeedEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		WITH unified AS (
			-- Lead activity
			SELECT
				la.id,
				'leads' AS category,
				la.action AS event_type,
				la.action AS title,
				'' AS description,
				la.lead_id AS entity_id,
				COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
				COALESCE(l.consumer_phone, '') AS phone,
				COALESCE(l.consumer_email, '') AS email,
				COALESCE(svc.status, '') AS lead_status,
				COALESCE(svc.name, '') AS service_type,
				l.lead_score,
				NULL::text AS address,
				NULL::double precision AS latitude,
				NULL::double precision AS longitude,
				NULL::timestamptz AS scheduled_at,
				la.created_at,
				COALESCE(NULLIF(trim(concat_ws(' ', u.first_name, u.last_name)), ''), 'Systeem') AS actor_name,
				la.meta AS raw_metadata,
				NULL::uuid AS service_id
			FROM RAC_lead_activity la
			LEFT JOIN RAC_leads l ON l.id = la.lead_id AND l.organization_id = la.organization_id
			LEFT JOIN RAC_users u ON u.id = la.user_id
			LEFT JOIN LATERAL (
				SELECT ls.status, st.name
				FROM RAC_lead_services ls
				LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
				WHERE ls.lead_id = l.id
				ORDER BY ls.created_at DESC
				LIMIT 1
			) svc ON true
			WHERE la.organization_id = $1
			  AND la.action != 'lead_viewed'

			UNION ALL

			-- Quote activity
			SELECT
				qa.id,
				'quotes' AS category,
				qa.event_type,
				qa.message AS title,
				'' AS description,
				qa.quote_id AS entity_id,
				COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
				COALESCE(l.consumer_phone, '') AS phone,
				COALESCE(l.consumer_email, '') AS email,
				COALESCE(ls.status, '') AS lead_status,
				COALESCE(st.name, '') AS service_type,
				l.lead_score,
				NULL::text AS address,
				NULL::double precision AS latitude,
				NULL::double precision AS longitude,
				NULL::timestamptz AS scheduled_at,
				qa.created_at,
				'Systeem' AS actor_name,
				qa.metadata AS raw_metadata,
				NULL::uuid AS service_id
			FROM RAC_quote_activity qa
			LEFT JOIN RAC_quotes q ON q.id = qa.quote_id
			LEFT JOIN RAC_lead_services ls ON ls.id = q.lead_service_id
			LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = qa.organization_id
			LEFT JOIN RAC_leads l ON l.id = ls.lead_id AND l.organization_id = qa.organization_id
			WHERE qa.organization_id = $1

			UNION ALL

			-- Appointment activity (recent creates/updates)
			SELECT
				a.id,
				'appointments' AS category,
				CASE
					WHEN a.created_at = a.updated_at THEN 'appointment_created'
					ELSE 'appointment_updated'
				END AS event_type,
				a.title,
				COALESCE(a.description, '') AS description,
				a.id AS entity_id,
				COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
				COALESCE(l.consumer_phone, '') AS phone,
				COALESCE(l.consumer_email, '') AS email,
				COALESCE(als.status, svc.status, '') AS lead_status,
				COALESCE(ast.name, svc.name, '') AS service_type,
				l.lead_score,
				COALESCE(
					NULLIF(a.location, ''),
					concat_ws(', ',
						concat_ws(' ', l.address_street, l.address_house_number),
						concat_ws(' ', l.address_zip_code, l.address_city)
					)
				) AS address,
				l.latitude,
				l.longitude,
				a.start_time AS scheduled_at,
				a.updated_at AS created_at,
				'Systeem' AS actor_name,
				NULL::jsonb AS raw_metadata,
				NULL::uuid AS service_id
			FROM RAC_appointments a
			LEFT JOIN RAC_leads l ON l.id = a.lead_id AND l.organization_id = a.organization_id
			LEFT JOIN RAC_lead_services als ON als.id = a.lead_service_id
			LEFT JOIN RAC_service_types ast ON ast.id = als.service_type_id AND ast.organization_id = a.organization_id
			LEFT JOIN LATERAL (
				SELECT ls.status, st.name
				FROM RAC_lead_services ls
				LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
				WHERE ls.lead_id = l.id
				ORDER BY ls.created_at DESC
				LIMIT 1
			) svc ON true
			WHERE a.organization_id = $1

			UNION ALL

			-- AI timeline activity
			SELECT
				te.id,
				'ai' AS category,
				te.event_type,
				te.title,
				COALESCE(te.summary, '') AS description,
				te.lead_id AS entity_id,
				COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
				COALESCE(l.consumer_phone, '') AS phone,
				COALESCE(l.consumer_email, '') AS email,
				COALESCE(svc.status, '') AS lead_status,
				COALESCE(svc.name, '') AS service_type,
				l.lead_score,
				NULL::text AS address,
				NULL::double precision AS latitude,
				NULL::double precision AS longitude,
				NULL::timestamptz AS scheduled_at,
				te.created_at,
				COALESCE(te.actor_name, 'AI') AS actor_name,
				te.metadata AS raw_metadata,
				te.service_id
			FROM lead_timeline_events te
			LEFT JOIN RAC_leads l ON l.id = te.lead_id AND l.organization_id = te.organization_id
			LEFT JOIN LATERAL (
				SELECT ls.status, st.name
				FROM RAC_lead_services ls
				LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
				WHERE ls.lead_id = l.id
				ORDER BY ls.created_at DESC
				LIMIT 1
			) svc ON true
			WHERE te.organization_id = $1
				AND te.event_type IN ('ai', 'photo_analysis_completed')
		),
		-- Step 1: compute the time gap from the previous event in the same partition.
		with_gap AS (
			SELECT *,
				CASE
					WHEN created_at - LAG(created_at) OVER (
						PARTITION BY entity_id, event_type, category
						ORDER BY created_at
					) <= interval '15 minutes' THEN 0
					ELSE 1
				END AS is_new_cluster
			FROM unified
		),
		-- Step 2: running SUM over the gap flag to assign a cluster_id.
		clustered AS (
			SELECT *,
				SUM(is_new_cluster) OVER (
					PARTITION BY entity_id, event_type, category
					ORDER BY created_at
				) AS cluster_id
			FROM with_gap
		),
		-- Attach per-cluster count to every row, then pick the latest row per cluster.
		with_count AS (
			SELECT *,
				COUNT(*) OVER (
					PARTITION BY entity_id, event_type, category, cluster_id
				)::int AS group_count
			FROM clustered
		),
		deduped AS (
			SELECT DISTINCT ON (entity_id, event_type, category, cluster_id)
			       id, category, event_type, title, description, entity_id,
			       service_id,
			       lead_name, phone, email, lead_status, service_type, lead_score,
			       COALESCE(address, '') AS address, latitude, longitude,
			       scheduled_at, created_at, 0 AS priority,
			       group_count, COALESCE(actor_name, '') AS actor_name, raw_metadata
			FROM with_count
			ORDER BY entity_id, event_type, category, cluster_id, created_at DESC
		)
		SELECT * FROM deduped
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, organizationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]ActivityFeedEntry, 0, limit)
	for rows.Next() {
		var e ActivityFeedEntry
		if err := rows.Scan(
			&e.ID,
			&e.Category,
			&e.EventType,
			&e.Title,
			&e.Description,
			&e.EntityID,
			&e.ServiceID,
			&e.LeadName,
			&e.Phone,
			&e.Email,
			&e.LeadStatus,
			&e.ServiceType,
			&e.LeadScore,
			&e.Address,
			&e.Latitude,
			&e.Longitude,
			&e.ScheduledAt,
			&e.CreatedAt,
			&e.Priority,
			&e.GroupCount,
			&e.ActorName,
			&e.RawMetadata,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return entries, nil
}

// ListUpcomingAppointments returns soon upcoming scheduled appointments for the org.
func (r *Repository) ListUpcomingAppointments(ctx context.Context, organizationID uuid.UUID, limit int) ([]ActivityFeedEntry, error) {
	if limit <= 0 {
		limit = 5
	}

	query := `
		SELECT
			a.id,
			'appointments' AS category,
			'appointment_upcoming' AS event_type,
			a.title,
			COALESCE(a.description, '') AS description,
			a.id AS entity_id,
			COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
			COALESCE(l.consumer_phone, '') AS phone,
			COALESCE(l.consumer_email, '') AS email,
			COALESCE(als.status, svc.status, '') AS lead_status,
			COALESCE(ast.name, svc.name, '') AS service_type,
			l.lead_score,
			COALESCE(
				NULLIF(a.location, ''),
				concat_ws(', ',
					concat_ws(' ', l.address_street, l.address_house_number),
					concat_ws(' ', l.address_zip_code, l.address_city)
				)
			) AS address,
			l.latitude,
			l.longitude,
			a.start_time AS scheduled_at,
			now() AS created_at,
			2 AS priority
		FROM RAC_appointments a
		LEFT JOIN RAC_leads l ON l.id = a.lead_id AND l.organization_id = a.organization_id
		LEFT JOIN RAC_lead_services als ON als.id = a.lead_service_id
		LEFT JOIN RAC_service_types ast ON ast.id = als.service_type_id AND ast.organization_id = a.organization_id
		LEFT JOIN LATERAL (
			SELECT ls.status, st.name
			FROM RAC_lead_services ls
			LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
			WHERE ls.lead_id = l.id
			ORDER BY ls.created_at DESC
			LIMIT 1
		) svc ON true
		WHERE a.organization_id = $1
			AND a.status = 'scheduled'
			AND a.start_time > now()
			AND a.start_time <= now() + interval '48 hours'
		ORDER BY a.start_time ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]ActivityFeedEntry, 0, limit)
	for rows.Next() {
		var e ActivityFeedEntry
		if err := rows.Scan(
			&e.ID,
			&e.Category,
			&e.EventType,
			&e.Title,
			&e.Description,
			&e.EntityID,
			&e.LeadName,
			&e.Phone,
			&e.Email,
			&e.LeadStatus,
			&e.ServiceType,
			&e.LeadScore,
			&e.Address,
			&e.Latitude,
			&e.Longitude,
			&e.ScheduledAt,
			&e.CreatedAt,
			&e.Priority,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return entries, nil
}

func (r *Repository) BulkDelete(ctx context.Context, ids []uuid.UUID, organizationID uuid.UUID) (int, error) {
	result, err := r.pool.Exec(ctx, "UPDATE RAC_leads SET deleted_at = now(), updated_at = now() WHERE id = ANY($1) AND organization_id = $2 AND deleted_at IS NULL", ids, organizationID)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

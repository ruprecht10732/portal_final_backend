package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
		SELECT name, description, intake_guidelines
		FROM RAC_service_types
		WHERE organization_id = $1 AND is_active = true
		ORDER BY display_order ASC, name ASC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ServiceContextDefinition, 0)
	for rows.Next() {
		var item ServiceContextDefinition
		if err := rows.Scan(&item.Name, &item.Description, &item.IntakeGuidelines); err != nil {
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
}

func (r *Repository) Create(ctx context.Context, params CreateLeadParams) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_leads (
			organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
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
	).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
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
			assigned_agent_id, source, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
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
		&lead.AssignedAgentID, &lead.Source, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
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
			assigned_agent_id, source, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
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
		&lead.AssignedAgentID, &lead.Source, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
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

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func nullable[T any](value *T) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadParams) (Lead, error) {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	fields := []struct {
		enabled bool
		column  string
		value   interface{}
	}{
		{params.ConsumerFirstName != nil, "consumer_first_name", derefString(params.ConsumerFirstName)},
		{params.ConsumerLastName != nil, "consumer_last_name", derefString(params.ConsumerLastName)},
		{params.ConsumerPhone != nil, "consumer_phone", derefString(params.ConsumerPhone)},
		{params.ConsumerEmail != nil, "consumer_email", derefString(params.ConsumerEmail)},
		{params.ConsumerRole != nil, "consumer_role", derefString(params.ConsumerRole)},
		{params.AddressStreet != nil, "address_street", derefString(params.AddressStreet)},
		{params.AddressHouseNumber != nil, "address_house_number", derefString(params.AddressHouseNumber)},
		{params.AddressZipCode != nil, "address_zip_code", derefString(params.AddressZipCode)},
		{params.AddressCity != nil, "address_city", derefString(params.AddressCity)},
		{params.Latitude != nil, "latitude", derefFloat(params.Latitude)},
		{params.Longitude != nil, "longitude", derefFloat(params.Longitude)},
		{params.AssignedAgentIDSet, "assigned_agent_id", params.AssignedAgentID},
	}

	for _, field := range fields {
		if !field.enabled {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field.column, argIdx))
		args = append(args, field.value)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id, organizationID)
	}

	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, id, organizationID)

	query := fmt.Sprintf(`
		UPDATE RAC_leads SET %s
		WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL
		RETURNING id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, energy_class, energy_index, energy_bouwjaar, energy_gebouwtype,
			energy_label_valid_until, energy_label_registered_at, energy_primair_fossiel, energy_bag_verblijfsobject_id,
			energy_label_fetched_at,
			lead_enrichment_source, lead_enrichment_postcode6, lead_enrichment_postcode4, lead_enrichment_buurtcode, lead_enrichment_data_year,
			lead_enrichment_gem_aardgasverbruik, lead_enrichment_gem_elektriciteitsverbruik, lead_enrichment_huishouden_grootte,
			lead_enrichment_koopwoningen_pct, lead_enrichment_bouwjaar_vanaf2000_pct, lead_enrichment_woz_waarde,
			lead_enrichment_mediaan_vermogen_x1000, lead_enrichment_gem_inkomen, lead_enrichment_pct_hoog_inkomen, lead_enrichment_pct_laag_inkomen,
			lead_enrichment_huishoudens_met_kinderen_pct, lead_enrichment_stedelijkheid, lead_enrichment_confidence, lead_enrichment_fetched_at,
			lead_score, lead_score_pre_ai, lead_score_factors, lead_score_version, lead_score_updated_at,
			viewed_by_id, viewed_at, created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx, argIdx+1)

	var lead Lead
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
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
	whereClause, joinClause, args, argIdx := buildLeadListWhere(params)

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(DISTINCT l.id) FROM RAC_leads l %s WHERE %s", joinClause, whereClause)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortColumn, err := mapLeadSortColumn(params.SortBy)
	if err != nil {
		return nil, 0, err
	}
	sortOrder := "DESC"
	if params.SortOrder != "" {
		switch params.SortOrder {
		case "asc":
			sortOrder = "ASC"
		case "desc":
			sortOrder = "DESC"
		default:
			return nil, 0, apperr.BadRequest("invalid sort order")
		}
	}

	args = append(args, params.Limit, params.Offset)

	query := fmt.Sprintf(`
		SELECT DISTINCT l.id, l.organization_id, l.consumer_first_name, l.consumer_last_name, l.consumer_phone, l.consumer_email, l.consumer_role,
			l.address_street, l.address_house_number, l.address_zip_code, l.address_city, l.latitude, l.longitude,
			l.assigned_agent_id, l.source, l.energy_class, l.energy_index, l.energy_bouwjaar, l.energy_gebouwtype,
			l.energy_label_valid_until, l.energy_label_registered_at, l.energy_primair_fossiel, l.energy_bag_verblijfsobject_id,
			l.energy_label_fetched_at,
			l.lead_enrichment_source, l.lead_enrichment_postcode6, l.lead_enrichment_postcode4, l.lead_enrichment_buurtcode, l.lead_enrichment_data_year,
			l.lead_enrichment_gem_aardgasverbruik, l.lead_enrichment_gem_elektriciteitsverbruik, l.lead_enrichment_huishouden_grootte,
			l.lead_enrichment_koopwoningen_pct, l.lead_enrichment_bouwjaar_vanaf2000_pct, l.lead_enrichment_woz_waarde,
			l.lead_enrichment_mediaan_vermogen_x1000, l.lead_enrichment_gem_inkomen, l.lead_enrichment_pct_hoog_inkomen, l.lead_enrichment_pct_laag_inkomen,
			l.lead_enrichment_huishoudens_met_kinderen_pct, l.lead_enrichment_stedelijkheid, l.lead_enrichment_confidence, l.lead_enrichment_fetched_at,
			l.lead_score, l.lead_score_pre_ai, l.lead_score_factors, l.lead_score_version, l.lead_score_updated_at,
			l.viewed_by_id, l.viewed_at, l.created_at, l.updated_at
		FROM RAC_leads l
		%s
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, joinClause, whereClause, sortColumn, sortOrder, argIdx, argIdx+1)

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
			&lead.AssignedAgentID, &lead.Source, &lead.EnergyClass, &lead.EnergyIndex, &lead.EnergyBouwjaar, &lead.EnergyGebouwtype,
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

type leadListWhereBuilder struct {
	whereClauses     []string
	args             []interface{}
	argIdx           int
	needsServiceJoin bool
}

func newLeadListWhereBuilder(organizationID uuid.UUID) *leadListWhereBuilder {
	return &leadListWhereBuilder{
		whereClauses: []string{"l.organization_id = $1", "l.deleted_at IS NULL"},
		args:         []interface{}{organizationID},
		argIdx:       2,
	}
}

func (b *leadListWhereBuilder) addEquals(column string, value interface{}) {
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("%s = $%d", column, b.argIdx))
	b.args = append(b.args, value)
	b.argIdx++
}

func (b *leadListWhereBuilder) addILike(column string, value string) {
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("%s ILIKE $%d", column, b.argIdx))
	b.args = append(b.args, "%"+value+"%")
	b.argIdx++
}

func (b *leadListWhereBuilder) addStatus(status *string) {
	if status == nil {
		return
	}
	b.needsServiceJoin = true
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("cs.status = $%d", b.argIdx))
	b.args = append(b.args, *status)
	b.argIdx++
}

func (b *leadListWhereBuilder) addServiceType(serviceType *string) {
	if serviceType == nil {
		return
	}
	b.needsServiceJoin = true
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("st.name = $%d", b.argIdx))
	b.args = append(b.args, *serviceType)
	b.argIdx++
}

func (b *leadListWhereBuilder) addSearch(search string) {
	if search == "" {
		return
	}
	searchPattern := "%" + search + "%"
	b.whereClauses = append(b.whereClauses, fmt.Sprintf(
		"(l.consumer_first_name ILIKE $%d OR l.consumer_last_name ILIKE $%d OR l.consumer_phone ILIKE $%d OR l.consumer_email ILIKE $%d OR l.address_city ILIKE $%d)",
		b.argIdx, b.argIdx, b.argIdx, b.argIdx, b.argIdx,
	))
	b.args = append(b.args, searchPattern)
	b.argIdx++
}

func (b *leadListWhereBuilder) addOptionalILike(column string, value *string) {
	if value == nil {
		return
	}
	b.addILike(column, *value)
}

func (b *leadListWhereBuilder) addOptionalEquals(column string, value *string) {
	if value == nil {
		return
	}
	b.addEquals(column, *value)
}

func (b *leadListWhereBuilder) addOptionalUUIDEquals(column string, value *uuid.UUID) {
	if value == nil {
		return
	}
	b.addEquals(column, *value)
}

func (b *leadListWhereBuilder) addCreatedAtFrom(value *time.Time) {
	if value == nil {
		return
	}
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("l.created_at >= $%d", b.argIdx))
	b.args = append(b.args, *value)
	b.argIdx++
}

func (b *leadListWhereBuilder) addCreatedAtTo(value *time.Time) {
	if value == nil {
		return
	}
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("l.created_at < $%d", b.argIdx))
	b.args = append(b.args, *value)
	b.argIdx++
}

func (b *leadListWhereBuilder) joinClause() string {
	if !b.needsServiceJoin {
		return ""
	}
	return `
		LEFT JOIN LATERAL (
			SELECT ls.id, ls.status, ls.service_type_id
			FROM RAC_lead_services ls
			WHERE ls.lead_id = l.id AND ls.status NOT IN ('Closed', 'Bad_Lead', 'Surveyed')
			ORDER BY ls.created_at DESC
			LIMIT 1
		) cs ON true
		LEFT JOIN RAC_service_types st ON st.id = cs.service_type_id AND st.organization_id = l.organization_id`
}

func buildLeadListWhere(params ListParams) (string, string, []interface{}, int) {
	builder := newLeadListWhereBuilder(params.OrganizationID)
	builder.addStatus(params.Status)
	builder.addServiceType(params.ServiceType)
	builder.addSearch(params.Search)
	builder.addOptionalILike("l.consumer_first_name", params.FirstName)
	builder.addOptionalILike("l.consumer_last_name", params.LastName)
	builder.addOptionalILike("l.consumer_phone", params.Phone)
	builder.addOptionalILike("l.consumer_email", params.Email)
	builder.addOptionalEquals("l.consumer_role", params.Role)
	builder.addOptionalILike("l.address_street", params.Street)
	builder.addOptionalILike("l.address_house_number", params.HouseNumber)
	builder.addOptionalILike("l.address_zip_code", params.ZipCode)
	builder.addOptionalILike("l.address_city", params.City)
	builder.addOptionalUUIDEquals("l.assigned_agent_id", params.AssignedAgentID)
	builder.addCreatedAtFrom(params.CreatedAtFrom)
	builder.addCreatedAtTo(params.CreatedAtTo)

	return strings.Join(builder.whereClauses, " AND "), builder.joinClause(), builder.args, builder.argIdx
}

func mapLeadSortColumn(sortBy string) (string, error) {
	sortColumn := "l.created_at"
	if sortBy == "" {
		return sortColumn, nil
	}

	switch sortBy {
	case "createdAt":
		return "l.created_at", nil
	case "firstName":
		return "l.consumer_first_name", nil
	case "lastName":
		return "l.consumer_last_name", nil
	case "phone":
		return "l.consumer_phone", nil
	case "email":
		return "l.consumer_email", nil
	case "role":
		return "l.consumer_role", nil
	case "street":
		return "l.address_street", nil
	case "houseNumber":
		return "l.address_house_number", nil
	case "zipCode":
		return "l.address_zip_code", nil
	case "city":
		return "l.address_city", nil
	case "assignedAgentId":
		return "l.assigned_agent_id", nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

type HeatmapPoint struct {
	Latitude  float64
	Longitude float64
}

func (r *Repository) ListHeatmapPoints(ctx context.Context, organizationID uuid.UUID, startDate *time.Time, endDate *time.Time) ([]HeatmapPoint, error) {
	whereClauses := []string{"organization_id = $1", "deleted_at IS NULL", "latitude IS NOT NULL", "longitude IS NOT NULL"}
	args := []interface{}{organizationID}
	argIdx := 2

	if startDate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *startDate)
		argIdx++
	}
	if endDate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, *endDate)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	query := fmt.Sprintf(`
		SELECT latitude, longitude
		FROM RAC_leads
		WHERE %s
	`, whereClause)

	rows, err := r.pool.Query(ctx, query, args...)
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
	whereClauses := []string{"l.organization_id = $1", "l.deleted_at IS NULL"}
	args := []interface{}{organizationID, newLeadDays}
	argIdx := 3

	whereClauses = append(whereClauses, "(ai.urgency_level = 'High' OR l.created_at >= now() - ($2::int || ' days')::interval)")

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM RAC_leads l
		LEFT JOIN (
			SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
			FROM RAC_lead_ai_analysis
			ORDER BY lead_id, created_at DESC
		) ai ON ai.lead_id = l.id
		WHERE %s
	`, whereClause)

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return ActionItemListResult{}, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT l.id, l.consumer_first_name, l.consumer_last_name, ai.urgency_level, ai.urgency_reason, l.created_at
		FROM RAC_leads l
		LEFT JOIN (
			SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
			FROM RAC_lead_ai_analysis
			ORDER BY lead_id, created_at DESC
		) ai ON ai.lead_id = l.id
		WHERE %s
		ORDER BY
			CASE WHEN ai.urgency_level = 'High' THEN 0 ELSE 1 END,
			l.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
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

func (r *Repository) BulkDelete(ctx context.Context, ids []uuid.UUID, organizationID uuid.UUID) (int, error) {
	result, err := r.pool.Exec(ctx, "UPDATE RAC_leads SET deleted_at = now(), updated_at = now() WHERE id = ANY($1) AND organization_id = $2 AND deleted_at IS NULL", ids, organizationID)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

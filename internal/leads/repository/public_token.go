package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetByPublicToken(ctx context.Context, token string) (Lead, error) {
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
        FROM RAC_leads
        WHERE public_token = $1
            AND deleted_at IS NULL
            AND (public_token_expires_at IS NULL OR public_token_expires_at > now())
    `, token).Scan(
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

func (r *Repository) SetPublicToken(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, token string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
        UPDATE RAC_leads
        SET public_token = $3, public_token_expires_at = $4, updated_at = now()
        WHERE id = $1 AND organization_id = $2
    `, id, organizationID, token, expiresAt)
	return err
}

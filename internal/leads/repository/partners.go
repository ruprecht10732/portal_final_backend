package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PartnerMatch struct {
	ID           uuid.UUID
	BusinessName string
	Email        string
	DistanceKm   float64
}

func (r *Repository) FindMatchingPartners(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID, serviceType string, zipCode string, radiusKm int, excludePartnerIDs []uuid.UUID) ([]PartnerMatch, error) {
	lat, lon, ok, err := r.lookupLeadCoordinates(ctx, organizationID, leadID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Fallback: zip-based heuristic (kept for backward compatibility / older leads)
		lat, lon, ok, err = r.lookupZipCoordinates(ctx, organizationID, zipCode)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []PartnerMatch{}, nil
		}
	}

	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.business_name, p.contact_email,
			earth_distance(ll_to_earth($2, $1), ll_to_earth(p.latitude, p.longitude)) / 1000.0 AS dist_km
		FROM RAC_partners p
		JOIN RAC_partner_service_types pst ON pst.partner_id = p.id
		JOIN RAC_service_types st ON st.id = pst.service_type_id AND st.organization_id = p.organization_id
		WHERE p.organization_id = $3
			AND st.is_active = true
			AND (st.name = $4 OR st.slug = $4)
			AND p.latitude IS NOT NULL AND p.longitude IS NOT NULL
			AND earth_distance(ll_to_earth($2, $1), ll_to_earth(p.latitude, p.longitude)) <= ($5 * 1000.0)
			AND (CARDINALITY($6::uuid[]) = 0 OR p.id != ALL($6::uuid[]))
		ORDER BY dist_km ASC
		LIMIT 5
	`, lon, lat, organizationID, serviceType, radiusKm, excludePartnerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := make([]PartnerMatch, 0)
	for rows.Next() {
		var match PartnerMatch
		if err := rows.Scan(&match.ID, &match.BusinessName, &match.Email, &match.DistanceKm); err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return matches, nil
}

func (r *Repository) lookupLeadCoordinates(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID) (float64, float64, bool, error) {
	var lat float64
	var lon float64

	err := r.pool.QueryRow(ctx, `
		SELECT latitude, longitude
		FROM RAC_leads
		WHERE organization_id = $1
			AND id = $2
			AND latitude IS NOT NULL
			AND longitude IS NOT NULL
		LIMIT 1
	`, organizationID, leadID).Scan(&lat, &lon)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}

	return lat, lon, true, nil
}

// GetInvitedPartnerIDs returns IDs of partners who have already received an offer for this service.
func (r *Repository) GetInvitedPartnerIDs(ctx context.Context, serviceID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT partner_id
		FROM RAC_partner_offers
		WHERE lead_service_id = $1
			AND status IN ('rejected', 'expired', 'sent', 'pending')
	`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return ids, nil
}

func (r *Repository) lookupZipCoordinates(ctx context.Context, organizationID uuid.UUID, zipCode string) (float64, float64, bool, error) {
	var lat float64
	var lon float64

	err := r.pool.QueryRow(ctx, `
		SELECT latitude, longitude
		FROM RAC_leads
		WHERE organization_id = $1
			AND address_zip_code = $2
			AND latitude IS NOT NULL
			AND longitude IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, organizationID, zipCode).Scan(&lat, &lon)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}

	return lat, lon, true, nil
}

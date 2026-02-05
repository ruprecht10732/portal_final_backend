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

func (r *Repository) FindMatchingPartners(ctx context.Context, organizationID uuid.UUID, serviceType string, zipCode string, radiusKm int) ([]PartnerMatch, error) {
	lat, lon, ok, err := r.lookupZipCoordinates(ctx, organizationID, zipCode)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []PartnerMatch{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.business_name, p.contact_email,
			(point(p.longitude, p.latitude) <@> point($1, $2)) * 1.60934 AS dist_km
		FROM RAC_partners p
		JOIN RAC_partner_service_types pst ON pst.partner_id = p.id
		JOIN RAC_service_types st ON st.id = pst.service_type_id AND st.organization_id = p.organization_id
		WHERE p.organization_id = $3
			AND st.is_active = true
			AND (st.name = $4 OR st.slug = $4)
			AND p.latitude IS NOT NULL AND p.longitude IS NOT NULL
			AND (point(p.longitude, p.latitude) <@> point($1, $2)) < ($5 / 1.60934)
		ORDER BY dist_km ASC
		LIMIT 5
	`, lon, lat, organizationID, serviceType, radiusKm)
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

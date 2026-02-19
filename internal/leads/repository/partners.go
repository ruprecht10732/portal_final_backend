package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PartnerMatch struct {
	ID           uuid.UUID
	BusinessName string
	Email        string
	DistanceKm   float64
}

// PartnerOfferStats provides recent offer outcome counts for a partner.
// These are coarse signals used by the Dispatcher to avoid repeatedly selecting
// partners that frequently reject offers.
type PartnerOfferStats struct {
	Rejected int
	Accepted int
	Open     int // pending + sent
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
			// Final fallback: when we have no coordinates at all, do not claim "0 partners".
			// Instead, return a best-effort shortlist based on service type and (if present) lead city.
			return r.findPartnersWithoutAnchor(ctx, organizationID, leadID, serviceType, excludePartnerIDs)
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

// GetPartnerOfferStatsSince returns recent offer outcome counts per partner since the given time.
func (r *Repository) GetPartnerOfferStatsSince(ctx context.Context, organizationID uuid.UUID, partnerIDs []uuid.UUID, sinceTime time.Time) (map[uuid.UUID]PartnerOfferStats, error) {
	if len(partnerIDs) == 0 {
		return map[uuid.UUID]PartnerOfferStats{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT partner_id,
			COUNT(*) FILTER (WHERE status = 'rejected') AS rejected_count,
			COUNT(*) FILTER (WHERE status = 'accepted') AS accepted_count,
			COUNT(*) FILTER (WHERE status IN ('pending', 'sent')) AS open_count
		FROM RAC_partner_offers
		WHERE organization_id = $1
			AND partner_id = ANY($2::uuid[])
			AND created_at >= $3
		GROUP BY partner_id
	`, organizationID, partnerIDs, sinceTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[uuid.UUID]PartnerOfferStats, len(partnerIDs))
	for rows.Next() {
		var pid uuid.UUID
		var rejected int
		var accepted int
		var open int
		if err := rows.Scan(&pid, &rejected, &accepted, &open); err != nil {
			return nil, err
		}
		stats[pid] = PartnerOfferStats{Rejected: rejected, Accepted: accepted, Open: open}
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	// Ensure all requested partners have an entry (default 0 counts) to simplify callers.
	for _, pid := range partnerIDs {
		if _, ok := stats[pid]; !ok {
			stats[pid] = PartnerOfferStats{}
		}
	}

	return stats, nil
}

func (r *Repository) findPartnersWithoutAnchor(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID, serviceType string, excludePartnerIDs []uuid.UUID) ([]PartnerMatch, error) {
	// Prefer matching by lead city to keep results locally relevant.
	city, ok, err := r.lookupLeadCity(ctx, organizationID, leadID)
	if err != nil {
		return nil, err
	}
	if ok {
		matches, err := r.findPartnersByServiceTypeAndCity(ctx, organizationID, serviceType, city, excludePartnerIDs)
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			return matches, nil
		}
	}

	// As a last resort, match by service type only.
	return r.findPartnersByServiceType(ctx, organizationID, serviceType, excludePartnerIDs)
}

func (r *Repository) lookupLeadCity(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID) (string, bool, error) {
	var city string
	err := r.pool.QueryRow(ctx, `
		SELECT address_city
		FROM RAC_leads
		WHERE organization_id = $1
			AND id = $2
		LIMIT 1
	`, organizationID, leadID).Scan(&city)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if city == "" {
		return "", false, nil
	}
	return city, true, nil
}

func (r *Repository) findPartnersByServiceTypeAndCity(ctx context.Context, organizationID uuid.UUID, serviceType string, city string, excludePartnerIDs []uuid.UUID) ([]PartnerMatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.business_name, p.contact_email,
			0.0::double precision AS dist_km
		FROM RAC_partners p
		JOIN RAC_partner_service_types pst ON pst.partner_id = p.id
		JOIN RAC_service_types st ON st.id = pst.service_type_id AND st.organization_id = p.organization_id
		WHERE p.organization_id = $1
			AND st.is_active = true
			AND (st.name = $2 OR st.slug = $2)
			AND lower(p.city) = lower($3)
			AND (CARDINALITY($4::uuid[]) = 0 OR p.id != ALL($4::uuid[]))
		ORDER BY p.updated_at DESC
		LIMIT 5
	`, organizationID, serviceType, city, excludePartnerIDs)
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

func (r *Repository) findPartnersByServiceType(ctx context.Context, organizationID uuid.UUID, serviceType string, excludePartnerIDs []uuid.UUID) ([]PartnerMatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.business_name, p.contact_email,
			0.0::double precision AS dist_km
		FROM RAC_partners p
		JOIN RAC_partner_service_types pst ON pst.partner_id = p.id
		JOIN RAC_service_types st ON st.id = pst.service_type_id AND st.organization_id = p.organization_id
		WHERE p.organization_id = $1
			AND st.is_active = true
			AND (st.name = $2 OR st.slug = $2)
			AND (CARDINALITY($3::uuid[]) = 0 OR p.id != ALL($3::uuid[]))
		ORDER BY p.updated_at DESC
		LIMIT 5
	`, organizationID, serviceType, excludePartnerIDs)
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

// HasLinkedPartners reports whether this lead has at least one manually linked partner.
// This is used as a guard to avoid re-running the dispatcher when a human already did the matching.
func (r *Repository) HasLinkedPartners(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM RAC_partner_leads
			WHERE organization_id = $1
				AND lead_id = $2
		)
	`, organizationID, leadID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
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

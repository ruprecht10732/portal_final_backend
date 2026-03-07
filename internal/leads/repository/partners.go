package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
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

	rows, err := r.queries.FindMatchingPartnersByCoordinates(ctx, leadsdb.FindMatchingPartnersByCoordinatesParams{
		LlToEarth:      lon,
		LlToEarth_2:    lat,
		OrganizationID: toPgUUID(organizationID),
		Name:           serviceType,
		Column5:        radiusKm,
		Column6:        toPgUUIDSlice(excludePartnerIDs),
	})
	if err != nil {
		return nil, err
	}

	matches := make([]PartnerMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, partnerMatchFromDistanceRow(row.ID, row.BusinessName, row.ContactEmail, row.DistKm))
	}

	return matches, nil
}

// GetPartnerOfferStatsSince returns recent offer outcome counts per partner since the given time.
func (r *Repository) GetPartnerOfferStatsSince(ctx context.Context, organizationID uuid.UUID, partnerIDs []uuid.UUID, sinceTime time.Time) (map[uuid.UUID]PartnerOfferStats, error) {
	if len(partnerIDs) == 0 {
		return map[uuid.UUID]PartnerOfferStats{}, nil
	}

	rows, err := r.queries.GetPartnerOfferStatsSince(ctx, leadsdb.GetPartnerOfferStatsSinceParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        toPgUUIDSlice(partnerIDs),
		CreatedAt:      toPgTimestamp(sinceTime),
	})
	if err != nil {
		return nil, err
	}

	stats := make(map[uuid.UUID]PartnerOfferStats, len(partnerIDs))
	for _, row := range rows {
		pid := uuid.UUID(row.PartnerID.Bytes)
		stats[pid] = PartnerOfferStats{Rejected: int(row.RejectedCount), Accepted: int(row.AcceptedCount), Open: int(row.OpenCount)}
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
	city, err := r.queries.GetLeadCity(ctx, leadsdb.GetLeadCityParams{
		OrganizationID: toPgUUID(organizationID),
		ID:             toPgUUID(leadID),
	})
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
	rows, err := r.queries.FindPartnersByServiceTypeAndCity(ctx, leadsdb.FindPartnersByServiceTypeAndCityParams{
		OrganizationID: toPgUUID(organizationID),
		Name:           serviceType,
		Lower:          city,
		Column4:        toPgUUIDSlice(excludePartnerIDs),
	})
	if err != nil {
		return nil, err
	}

	matches := make([]PartnerMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, partnerMatchFromDistanceRow(row.ID, row.BusinessName, row.ContactEmail, row.DistKm))
	}

	return matches, nil
}

func (r *Repository) findPartnersByServiceType(ctx context.Context, organizationID uuid.UUID, serviceType string, excludePartnerIDs []uuid.UUID) ([]PartnerMatch, error) {
	rows, err := r.queries.FindPartnersByServiceType(ctx, leadsdb.FindPartnersByServiceTypeParams{
		OrganizationID: toPgUUID(organizationID),
		Name:           serviceType,
		Column3:        toPgUUIDSlice(excludePartnerIDs),
	})
	if err != nil {
		return nil, err
	}

	matches := make([]PartnerMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, partnerMatchFromDistanceRow(row.ID, row.BusinessName, row.ContactEmail, row.DistKm))
	}

	return matches, nil
}

func (r *Repository) lookupLeadCoordinates(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID) (float64, float64, bool, error) {
	row, err := r.queries.GetLeadCoordinates(ctx, leadsdb.GetLeadCoordinatesParams{
		OrganizationID: toPgUUID(organizationID),
		ID:             toPgUUID(leadID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}

	return row.Latitude.Float64, row.Longitude.Float64, true, nil
}

// GetInvitedPartnerIDs returns IDs of partners who have already received an offer for this service.
func (r *Repository) GetInvitedPartnerIDs(ctx context.Context, serviceID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.queries.ListInvitedPartnerIDs(ctx, toPgUUID(serviceID))
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, id := range rows {
		ids = append(ids, uuid.UUID(id.Bytes))
	}

	return ids, nil
}

// HasLinkedPartners reports whether this lead has at least one manually linked partner.
// This is used as a guard to avoid re-running the dispatcher when a human already did the matching.
func (r *Repository) HasLinkedPartners(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID) (bool, error) {
	return r.queries.HasLinkedPartners(ctx, leadsdb.HasLinkedPartnersParams{
		OrganizationID: toPgUUID(organizationID),
		LeadID:         toPgUUID(leadID),
	})
}

func (r *Repository) lookupZipCoordinates(ctx context.Context, organizationID uuid.UUID, zipCode string) (float64, float64, bool, error) {
	row, err := r.queries.GetZipCoordinates(ctx, leadsdb.GetZipCoordinatesParams{
		OrganizationID: toPgUUID(organizationID),
		AddressZipCode: zipCode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}

	return row.Latitude.Float64, row.Longitude.Float64, true, nil
}

func partnerMatchFromDistanceRow(id pgtype.UUID, businessName, email string, distanceKm float64) PartnerMatch {
	return PartnerMatch{
		ID:           uuid.UUID(id.Bytes),
		BusinessName: businessName,
		Email:        email,
		DistanceKm:   distanceKm,
	}
}

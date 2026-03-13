package repository

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

// FuzzyLeadMatch is the result of a trigram similarity lead search.
// It includes the most recently active service details when available.
type FuzzyLeadMatch struct {
	LeadID        uuid.UUID
	FirstName     string
	LastName      string
	Phone         string
	Email         *string
	City          string
	ServiceID     *uuid.UUID
	ServiceType   string
	ServiceStatus string
	CreatedAt     time.Time
}

// fuzzySearchLeadsQuery is the raw SQL for a pg_trgm word_similarity search.
// It is intentionally NOT managed via sqlc because it returns a computed
// match_score column that sqlc cannot currently represent in the generated model.
//
// Requires the pg_trgm extension (migration 153).
const fuzzySearchLeadsQuery = `
SELECT
	l.id,
	l.consumer_first_name,
	l.consumer_last_name,
	l.consumer_phone,
	l.consumer_email,
	l.address_city,
	l.created_at,
	cs.id AS service_id,
	COALESCE(st.name, '') AS service_type,
	COALESCE(cs.status, '') AS service_status
FROM RAC_leads l
LEFT JOIN LATERAL (
	SELECT ls.id, ls.status, ls.service_type_id
	FROM RAC_lead_services ls
	WHERE ls.lead_id = l.id
		AND ls.pipeline_stage NOT IN ('Completed', 'Lost')
		AND ls.status != 'Disqualified'
	ORDER BY ls.created_at DESC
	LIMIT 1
) cs ON true
LEFT JOIN RAC_service_types st
	ON st.id = cs.service_type_id AND st.organization_id = l.organization_id
WHERE l.organization_id = $1
	AND l.deleted_at IS NULL
	AND GREATEST(
		word_similarity($2, l.consumer_first_name),
		word_similarity($2, l.consumer_last_name),
		word_similarity($2, l.consumer_first_name || ' ' || l.consumer_last_name),
		word_similarity($2, l.consumer_phone)
	) > 0.22
ORDER BY GREATEST(
	word_similarity($2, l.consumer_first_name),
	word_similarity($2, l.consumer_last_name),
	word_similarity($2, l.consumer_first_name || ' ' || l.consumer_last_name),
	word_similarity($2, l.consumer_phone)
) DESC
LIMIT $3
`

// FuzzySearchLeads returns leads whose name or phone closely matches the query
// string using PostgreSQL pg_trgm word similarity scoring.
// Falls back gracefully (returns nil slice, no error) when pg_trgm is not installed.
func (r *Repository) FuzzySearchLeads(ctx context.Context, organizationID uuid.UUID, query string, limit int) ([]FuzzyLeadMatch, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.pool.Query(ctx, fuzzySearchLeadsQuery, toPgUUID(organizationID), query, int32(limit))
	if err != nil {
		// If pg_trgm is not installed yet the function word_similarity won't exist.
		// Return an empty result instead of propagating the error so the caller can
		// fall back to the standard ILIKE search.
		log.Printf("waagent: FuzzySearchLeads SQL error org=%s query=%q: %v", organizationID, query, err)
		return nil, nil //nolint:nilerr
	}
	defer rows.Close()

	var results []FuzzyLeadMatch
	for rows.Next() {
		var m FuzzyLeadMatch
		var pgServiceID *[16]byte
		if err := rows.Scan(
			&m.LeadID,
			&m.FirstName,
			&m.LastName,
			&m.Phone,
			&m.Email,
			&m.City,
			&m.CreatedAt,
			&pgServiceID,
			&m.ServiceType,
			&m.ServiceStatus,
		); err != nil {
			return nil, err
		}
		if pgServiceID != nil {
			id := uuid.UUID(*pgServiceID)
			m.ServiceID = &id
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// quoteBasedLeadSearchQuery finds leads via their associated quotes.
// It intentionally does NOT filter on l.deleted_at because the lead may have
// been soft-deleted while still having active quotes referencing it.
const quoteBasedLeadSearchQuery = `
SELECT DISTINCT ON (l.id)
	l.id,
	l.consumer_first_name,
	l.consumer_last_name,
	l.consumer_phone,
	l.consumer_email,
	l.address_city,
	l.created_at,
	cs.id AS service_id,
	COALESCE(st.name, '') AS service_type,
	COALESCE(cs.status, '') AS service_status
FROM RAC_quotes q
JOIN RAC_leads l ON l.id = q.lead_id
LEFT JOIN LATERAL (
	SELECT ls.id, ls.status, ls.service_type_id
	FROM RAC_lead_services ls
	WHERE ls.lead_id = l.id
	ORDER BY ls.created_at DESC
	LIMIT 1
) cs ON true
LEFT JOIN RAC_service_types st
	ON st.id = cs.service_type_id AND st.organization_id = l.organization_id
WHERE q.organization_id = $1
	AND (
		l.consumer_first_name ILIKE '%' || $2 || '%'
		OR l.consumer_last_name ILIKE '%' || $2 || '%'
		OR (l.consumer_first_name || ' ' || l.consumer_last_name) ILIKE '%' || $2 || '%'
		OR l.consumer_phone ILIKE '%' || $2 || '%'
		OR q.quote_number ILIKE '%' || $2 || '%'
	)
ORDER BY l.id, l.created_at DESC
LIMIT $3
`

// QuoteBasedLeadSearch finds leads through their associated quotes.
// This is the last-resort fallback that catches soft-deleted leads and leads
// that are only discoverable through quote data (e.g. by quote number).
func (r *Repository) QuoteBasedLeadSearch(ctx context.Context, organizationID uuid.UUID, query string, limit int) ([]FuzzyLeadMatch, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.pool.Query(ctx, quoteBasedLeadSearchQuery, toPgUUID(organizationID), query, int32(limit))
	if err != nil {
		log.Printf("waagent: QuoteBasedLeadSearch SQL error org=%s query=%q: %v", organizationID, query, err)
		return nil, nil //nolint:nilerr
	}
	defer rows.Close()

	var results []FuzzyLeadMatch
	for rows.Next() {
		var m FuzzyLeadMatch
		var pgServiceID *[16]byte
		if err := rows.Scan(
			&m.LeadID,
			&m.FirstName,
			&m.LastName,
			&m.Phone,
			&m.Email,
			&m.City,
			&m.CreatedAt,
			&pgServiceID,
			&m.ServiceType,
			&m.ServiceStatus,
		); err != nil {
			return nil, err
		}
		if pgServiceID != nil {
			id := uuid.UUID(*pgServiceID)
			m.ServiceID = &id
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

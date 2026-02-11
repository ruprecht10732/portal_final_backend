package exports

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAPIKeyNotFound = errors.New("export API key not found")

const apiKeyPrefix = "gads_"

// APIKey represents an export API key stored in the database.
type APIKey struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	KeyHash        string
	KeyPrefix      string
	IsActive       bool
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastUsedAt     *time.Time
}

// ConversionEvent represents a lead service event used for conversion exports.
type ConversionEvent struct {
	EventID             uuid.UUID
	OrganizationID      uuid.UUID
	LeadID              uuid.UUID
	LeadServiceID       uuid.UUID
	EventType           string
	Status              *string
	PipelineStage       *string
	OccurredAt          time.Time
	GCLID               string
	ConsumerEmail       *string
	ConsumerPhone       string
	ProjectedValueCents int64
}

// Repository provides data access for export operations.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GenerateAPIKey creates a new random API key and returns the plaintext key and its hash.
func GenerateAPIKey() (plaintext string, hash string, prefix string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", err
	}
	plaintext = apiKeyPrefix + hex.EncodeToString(bytes)
	h := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(h[:])
	prefix = plaintext[:12]
	return plaintext, hash, prefix, nil
}

// HashKey hashes a plaintext API key for lookup.
func HashKey(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// CreateAPIKey creates a new export API key record.
func (r *Repository) CreateAPIKey(ctx context.Context, orgID uuid.UUID, name string, keyHash string, keyPrefix string, createdBy *uuid.UUID) (APIKey, error) {
	var key APIKey
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_export_api_keys (organization_id, name, key_hash, key_prefix, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, name, key_hash, key_prefix, is_active, created_by, created_at, updated_at, last_used_at
	`, orgID, name, keyHash, keyPrefix, createdBy).Scan(
		&key.ID, &key.OrganizationID, &key.Name, &key.KeyHash, &key.KeyPrefix, &key.IsActive, &key.CreatedBy, &key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
	)
	return key, err
}

// GetAPIKeyByHash retrieves an active API key by its hash.
func (r *Repository) GetAPIKeyByHash(ctx context.Context, keyHash string) (APIKey, error) {
	var key APIKey
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, key_hash, key_prefix, is_active, created_by, created_at, updated_at, last_used_at
		FROM RAC_export_api_keys
		WHERE key_hash = $1 AND is_active = true
	`, keyHash).Scan(
		&key.ID, &key.OrganizationID, &key.Name, &key.KeyHash, &key.KeyPrefix, &key.IsActive, &key.CreatedBy, &key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrAPIKeyNotFound
	}
	return key, err
}

// ListAPIKeys returns all export API keys for an organization.
func (r *Repository) ListAPIKeys(ctx context.Context, orgID uuid.UUID) ([]APIKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, key_hash, key_prefix, is_active, created_by, created_at, updated_at, last_used_at
		FROM RAC_export_api_keys
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]APIKey, 0)
	for rows.Next() {
		var key APIKey
		if err := rows.Scan(
			&key.ID, &key.OrganizationID, &key.Name, &key.KeyHash, &key.KeyPrefix, &key.IsActive, &key.CreatedBy, &key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
		); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// RevokeAPIKey deactivates an export API key.
func (r *Repository) RevokeAPIKey(ctx context.Context, keyID uuid.UUID, orgID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE RAC_export_api_keys SET is_active = false, updated_at = now()
		WHERE id = $1 AND organization_id = $2
	`, keyID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// TouchAPIKey updates the last_used_at timestamp for the key.
func (r *Repository) TouchAPIKey(ctx context.Context, keyID uuid.UUID) {
	_, _ = r.pool.Exec(ctx, `
		UPDATE RAC_export_api_keys SET last_used_at = now(), updated_at = now()
		WHERE id = $1
	`, keyID)
}

// ListConversionEvents returns conversion-relevant lead service events.
func (r *Repository) ListConversionEvents(ctx context.Context, orgID uuid.UUID, from time.Time, to time.Time, limit int) ([]ConversionEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.organization_id, e.lead_id, e.lead_service_id, e.event_type, e.status, e.pipeline_stage, e.occurred_at,
			l.gclid, l.consumer_email, l.consumer_phone, l.projected_value_cents
		FROM RAC_lead_service_events e
		JOIN RAC_leads l ON l.id = e.lead_id AND l.organization_id = e.organization_id
		WHERE e.organization_id = $1
			AND l.deleted_at IS NULL
			AND l.gclid IS NOT NULL
			AND e.occurred_at >= $2 AND e.occurred_at <= $3
		ORDER BY e.occurred_at ASC
		LIMIT $4
	`, orgID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ConversionEvent, 0)
	for rows.Next() {
		var item ConversionEvent
		if err := rows.Scan(
			&item.EventID,
			&item.OrganizationID,
			&item.LeadID,
			&item.LeadServiceID,
			&item.EventType,
			&item.Status,
			&item.PipelineStage,
			&item.OccurredAt,
			&item.GCLID,
			&item.ConsumerEmail,
			&item.ConsumerPhone,
			&item.ProjectedValueCents,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListExportedKeys returns a set of order_id + conversion_name that have been exported.
func (r *Repository) ListExportedKeys(ctx context.Context, orgID uuid.UUID, orderIDs []string) (map[string]struct{}, error) {
	if len(orderIDs) == 0 {
		return map[string]struct{}{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT order_id, conversion_name
		FROM RAC_google_ads_exports
		WHERE organization_id = $1 AND order_id = ANY($2)
	`, orgID, orderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]struct{})
	for rows.Next() {
		var orderID string
		var conversionName string
		if err := rows.Scan(&orderID, &conversionName); err != nil {
			return nil, err
		}
		result[orderID+"::"+conversionName] = struct{}{}
	}
	return result, rows.Err()
}

// RecordExports stores export rows to prevent duplicates.
func (r *Repository) RecordExports(ctx context.Context, orgID uuid.UUID, rows []ExportRecord) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, row := range rows {
		batch.Queue(`
			INSERT INTO RAC_google_ads_exports (
				organization_id, lead_id, lead_service_id, conversion_name, conversion_time,
				conversion_value, gclid, order_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (organization_id, order_id, conversion_name) DO NOTHING
		`, orgID, row.LeadID, row.LeadServiceID, row.ConversionName, row.ConversionTime, row.ConversionValue, row.GCLID, row.OrderID)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ExportRecord represents a conversion row to persist.
type ExportRecord struct {
	LeadID          uuid.UUID
	LeadServiceID   uuid.UUID
	ConversionName  string
	ConversionTime  time.Time
	ConversionValue float64
	GCLID           string
	OrderID         string
}

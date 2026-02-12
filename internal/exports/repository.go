package exports

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCredentialNotFound = errors.New("google ads export credential not found")

const (
	credentialUsernamePrefix = "gadsu_"
	credentialPasswordPrefix = "gadsp_"
)

// ExportCredential represents Google Ads export credentials stored in the database.
type ExportCredential struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Username       string
	PasswordHash   string
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

// GenerateCredential generates random username/password for Google Ads HTTPS Basic Auth.
func GenerateCredential() (username string, password string, err error) {
	usernameBytes := make([]byte, 12)
	if _, err := rand.Read(usernameBytes); err != nil {
		return "", "", err
	}
	passwordBytes := make([]byte, 24)
	if _, err := rand.Read(passwordBytes); err != nil {
		return "", "", err
	}

	username = credentialUsernamePrefix + hex.EncodeToString(usernameBytes)
	password = credentialPasswordPrefix + hex.EncodeToString(passwordBytes)
	return username, password, nil
}

// UpsertCredential creates or rotates an organization credential.
func (r *Repository) UpsertCredential(ctx context.Context, orgID uuid.UUID, username string, passwordHash string, createdBy *uuid.UUID) (ExportCredential, error) {
	var credential ExportCredential
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_google_ads_export_credentials (organization_id, username, password_hash, created_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id)
		DO UPDATE SET username = EXCLUDED.username, password_hash = EXCLUDED.password_hash, created_by = EXCLUDED.created_by, updated_at = now(), last_used_at = NULL
		RETURNING id, organization_id, username, password_hash, created_by, created_at, updated_at, last_used_at
	`, orgID, username, passwordHash, createdBy).Scan(
		&credential.ID,
		&credential.OrganizationID,
		&credential.Username,
		&credential.PasswordHash,
		&credential.CreatedBy,
		&credential.CreatedAt,
		&credential.UpdatedAt,
		&credential.LastUsedAt,
	)
	return credential, err
}

// GetCredentialByUsername retrieves Google Ads export credential by username.
func (r *Repository) GetCredentialByUsername(ctx context.Context, username string) (ExportCredential, error) {
	var credential ExportCredential
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, username, password_hash, created_by, created_at, updated_at, last_used_at
		FROM RAC_google_ads_export_credentials
		WHERE username = $1
	`, username).Scan(
		&credential.ID,
		&credential.OrganizationID,
		&credential.Username,
		&credential.PasswordHash,
		&credential.CreatedBy,
		&credential.CreatedAt,
		&credential.UpdatedAt,
		&credential.LastUsedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExportCredential{}, ErrCredentialNotFound
	}
	return credential, err
}

// GetCredentialByOrganization retrieves credential metadata for an organization.
func (r *Repository) GetCredentialByOrganization(ctx context.Context, orgID uuid.UUID) (ExportCredential, error) {
	var credential ExportCredential
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, username, password_hash, created_by, created_at, updated_at, last_used_at
		FROM RAC_google_ads_export_credentials
		WHERE organization_id = $1
	`, orgID).Scan(
		&credential.ID,
		&credential.OrganizationID,
		&credential.Username,
		&credential.PasswordHash,
		&credential.CreatedBy,
		&credential.CreatedAt,
		&credential.UpdatedAt,
		&credential.LastUsedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExportCredential{}, ErrCredentialNotFound
	}
	return credential, err
}

// DeleteCredential removes an organization's credential.
func (r *Repository) DeleteCredential(ctx context.Context, orgID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_google_ads_export_credentials
		WHERE organization_id = $1
	`, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// TouchCredential updates the last_used_at timestamp for the credential.
func (r *Repository) TouchCredential(ctx context.Context, credentialID uuid.UUID) {
	_, _ = r.pool.Exec(ctx, `
		UPDATE RAC_google_ads_export_credentials SET last_used_at = now(), updated_at = now()
		WHERE id = $1
	`, credentialID)
}

// ListConversionEvents returns conversion-relevant lead service events.
func (r *Repository) ListConversionEvents(ctx context.Context, orgID uuid.UUID, from time.Time, to time.Time, limit int) ([]ConversionEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.organization_id, e.lead_id, e.lead_service_id, e.event_type, e.status, e.pipeline_stage, e.occurred_at,
			COALESCE(l.gclid, ''), l.consumer_email, COALESCE(l.consumer_phone, ''), l.projected_value_cents
		FROM RAC_lead_service_events e
		JOIN RAC_leads l ON l.id = e.lead_id AND l.organization_id = e.organization_id
		WHERE e.organization_id = $1
			AND l.deleted_at IS NULL
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
	defer func() { _ = results.Close() }()
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

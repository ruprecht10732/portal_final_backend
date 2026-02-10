// Package webhook provides the webhook/form capture bounded context.
// It handles API key management and inbound form submissions from external websites.
package webhook

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

var ErrAPIKeyNotFound = errors.New("webhook API key not found")

// APIKey represents a webhook API key stored in the database.
type APIKey struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	KeyHash        string
	KeyPrefix      string
	AllowedDomains []string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Repository provides data access for webhook API keys.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new webhook repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GenerateAPIKey creates a new random API key and returns the plaintext key and its hash.
// The plaintext key is returned only once; only the hash is stored.
func GenerateAPIKey() (plaintext string, hash string, prefix string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", err
	}
	plaintext = "whk_" + hex.EncodeToString(bytes)
	h := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(h[:])
	prefix = plaintext[:12] // "whk_" + 8 hex chars
	return plaintext, hash, prefix, nil
}

// HashKey hashes a plaintext API key for lookup.
func HashKey(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// Create creates a new API key record.
func (r *Repository) Create(ctx context.Context, orgID uuid.UUID, name string, keyHash string, keyPrefix string, allowedDomains []string) (APIKey, error) {
	var key APIKey
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_webhook_api_keys (organization_id, name, key_hash, key_prefix, allowed_domains)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, name, key_hash, key_prefix, allowed_domains, is_active, created_at, updated_at
	`, orgID, name, keyHash, keyPrefix, allowedDomains).Scan(
		&key.ID, &key.OrganizationID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.AllowedDomains, &key.IsActive, &key.CreatedAt, &key.UpdatedAt,
	)
	return key, err
}

// GetByHash retrieves an active API key by its hash.
func (r *Repository) GetByHash(ctx context.Context, keyHash string) (APIKey, error) {
	var key APIKey
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, key_hash, key_prefix, allowed_domains, is_active, created_at, updated_at
		FROM RAC_webhook_api_keys
		WHERE key_hash = $1 AND is_active = true
	`, keyHash).Scan(
		&key.ID, &key.OrganizationID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.AllowedDomains, &key.IsActive, &key.CreatedAt, &key.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrAPIKeyNotFound
	}
	return key, err
}

// ListByOrganization returns all API keys for an organization.
func (r *Repository) ListByOrganization(ctx context.Context, orgID uuid.UUID) ([]APIKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, key_hash, key_prefix, allowed_domains, is_active, created_at, updated_at
		FROM RAC_webhook_api_keys
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var key APIKey
		if err := rows.Scan(
			&key.ID, &key.OrganizationID, &key.Name, &key.KeyHash, &key.KeyPrefix,
			&key.AllowedDomains, &key.IsActive, &key.CreatedAt, &key.UpdatedAt,
		); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// Revoke deactivates an API key.
func (r *Repository) Revoke(ctx context.Context, keyID uuid.UUID, orgID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE RAC_webhook_api_keys SET is_active = false, updated_at = now()
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

// UpdateWebhookLeadData sets webhook-specific columns on a lead (raw_form_data, source domain, is_incomplete).
func (r *Repository) UpdateWebhookLeadData(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID, rawFormData []byte, sourceDomain string, isIncomplete bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_leads
		SET raw_form_data = $3, webhook_source_domain = $4, is_incomplete = $5, updated_at = now()
		WHERE id = $1 AND organization_id = $2
	`, leadID, orgID, rawFormData, sourceDomain, isIncomplete)
	return err
}

package webhook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAPIKeyNotFound = errors.New("webhook API key not found")
var ErrGoogleConfigNotFound = errors.New("google webhook config not found")
var ErrDuplicateGoogleLeadID = errors.New("google lead ID already processed")

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

type createTimelineEventParams struct {
	LeadID         uuid.UUID
	ServiceID      *uuid.UUID
	OrganizationID uuid.UUID
	ActorType      string
	ActorName      string
	EventType      string
	Title          string
	Summary        *string
	Metadata       map[string]any
}

// NewRepository creates a new webhook repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateTimelineEvent(ctx context.Context, params createTimelineEventParams) error {
	metadataJSON, err := json.Marshal(params.Metadata)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO lead_timeline_events (
			lead_id,
			service_id,
			organization_id,
			actor_type,
			actor_name,
			event_type,
			title,
			summary,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, params.LeadID, params.ServiceID, params.OrganizationID, params.ActorType, params.ActorName, params.EventType, params.Title, params.Summary, metadataJSON)
	return err
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

// ---- Google Lead Form Webhooks ----

// GenerateGoogleKey creates a new random Google webhook key and returns plaintext + hash.
func GenerateGoogleKey() (plaintext string, hash string, prefix string, err error) {
	// 22 bytes -> 44 hex chars; with "glk_" prefix totals 48 chars (<= 50).
	bytes := make([]byte, 22)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", err
	}
	plaintext = "glk_" + hex.EncodeToString(bytes)
	h := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(h[:])
	prefix = plaintext[:12]
	return plaintext, hash, prefix, nil
}

// CreateGoogleConfig creates a new Google webhook configuration.
func (r *Repository) CreateGoogleConfig(ctx context.Context, orgID uuid.UUID, name string, googleKeyHash string, googleKeyPrefix string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_google_webhook_configs (organization_id, name, google_key_hash, google_key_prefix, campaign_mappings)
		VALUES ($1, $2, $3, $4, '{}'::jsonb)
		RETURNING id
	`, orgID, name, googleKeyHash, googleKeyPrefix).Scan(&id)
	return id, err
}

// GetGoogleConfigByKey looks up a Google webhook config by its hashed key.
func (r *Repository) GetGoogleConfigByKey(ctx context.Context, googleKeyHash string) (GoogleWebhookConfig, error) {
	var cfg GoogleWebhookConfig
	var mappingsJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, google_key_hash, google_key_prefix, campaign_mappings, is_active, created_at, updated_at
		FROM RAC_google_webhook_configs
		WHERE google_key_hash = $1 AND is_active = true
	`, googleKeyHash).Scan(
		&cfg.ID, &cfg.OrganizationID, &cfg.Name, &cfg.GoogleKeyHash, &cfg.GoogleKeyPrefix, &mappingsJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoogleWebhookConfig{}, ErrGoogleConfigNotFound
	}
	if err != nil {
		return GoogleWebhookConfig{}, err
	}

	// Parse campaign mappings from JSONB
	if len(mappingsJSON) > 0 {
		var mappings map[string]string
		if err := json.Unmarshal(mappingsJSON, &mappings); err == nil {
			cfg.CampaignMappings = mappings
		}
	}

	return cfg, nil
}

// ListGoogleConfigs returns all Google webhook configs for an organization.
func (r *Repository) ListGoogleConfigs(ctx context.Context, orgID uuid.UUID) ([]GoogleWebhookConfig, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, google_key_hash, google_key_prefix, campaign_mappings, is_active, created_at, updated_at
		FROM RAC_google_webhook_configs
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []GoogleWebhookConfig
	for rows.Next() {
		var cfg GoogleWebhookConfig
		var mappingsJSON []byte

		if err := rows.Scan(
			&cfg.ID, &cfg.OrganizationID, &cfg.Name, &cfg.GoogleKeyHash, &cfg.GoogleKeyPrefix, &mappingsJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt,
		); err != nil {
			return nil, err
		}

		// Parse campaign mappings
		if len(mappingsJSON) > 0 {
			var mappings map[string]string
			if err := json.Unmarshal(mappingsJSON, &mappings); err == nil {
				cfg.CampaignMappings = mappings
			}
		}

		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// UpdateGoogleCampaignMapping adds or updates a campaign → service mapping.
func (r *Repository) UpdateGoogleCampaignMapping(ctx context.Context, configID uuid.UUID, orgID uuid.UUID, campaignID int64, serviceType string) error {
	campaignKey := strconv.FormatInt(campaignID, 10)
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_google_webhook_configs
		SET campaign_mappings = jsonb_set(campaign_mappings, ARRAY[$3], to_jsonb($4::text), true),
		updated_at = now()
		WHERE id = $1 AND organization_id = $2
	`, configID, orgID, campaignKey, serviceType)
	return err
}

// DeleteGoogleCampaignMapping removes a campaign mapping from a config.
func (r *Repository) DeleteGoogleCampaignMapping(ctx context.Context, configID uuid.UUID, orgID uuid.UUID, campaignID int64) error {
	campaignKey := strconv.FormatInt(campaignID, 10)
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_google_webhook_configs
		SET campaign_mappings = campaign_mappings - $3,
		updated_at = now()
		WHERE id = $1 AND organization_id = $2
	`, configID, orgID, campaignKey)
	return err
}

// DeleteGoogleConfig deletes a Google webhook configuration.
func (r *Repository) DeleteGoogleConfig(ctx context.Context, configID uuid.UUID, orgID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_google_webhook_configs
		WHERE id = $1 AND organization_id = $2
	`, configID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGoogleConfigNotFound
	}
	return nil
}

// CheckGoogleLeadIDExists checks if a Google lead ID has already been processed.
func (r *Repository) CheckGoogleLeadIDExists(ctx context.Context, leadID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM RAC_google_lead_ids WHERE lead_id = $1)
	`, leadID).Scan(&exists)
	return exists, err
}

// StoreGoogleLeadID stores a Google lead ID to prevent duplicate processing.
func (r *Repository) StoreGoogleLeadID(ctx context.Context, leadID string, orgID uuid.UUID, leadUUID *uuid.UUID, isTest bool) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO RAC_google_lead_ids (lead_id, organization_id, lead_uuid, is_test)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (lead_id) DO NOTHING
	`, leadID, orgID, leadUUID, isTest)
	return err
}

// UpdateGoogleLeadMetadata sets Google-specific metadata on a lead.
func (r *Repository) UpdateGoogleLeadMetadata(ctx context.Context, leadID uuid.UUID, campaignID, creativeID, adGroupID, formID int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_leads
		SET google_campaign_id = $2, google_creative_id = $3, google_adgroup_id = $4, google_form_id = $5, updated_at = now()
		WHERE id = $1
	`, leadID, campaignID, creativeID, adGroupID, formID)
	return err
}

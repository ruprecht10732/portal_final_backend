package webhook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	webhookdb "portal_final_backend/internal/webhook/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAPIKeyNotFound = errors.New("webhook API key not found")
var ErrGoogleConfigNotFound = errors.New("google webhook config not found")
var ErrDuplicateGoogleLeadID = errors.New("google lead ID already processed")
var ErrWhatsAppDeviceNotFound = errors.New("whatsapp device not found")

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
	pool    *pgxpool.Pool
	queries *webhookdb.Queries
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
	return &Repository{pool: pool, queries: webhookdb.New(pool)}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgTextValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func toPgInt8Value(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

func apiKeyFromModel(model webhookdb.RacWebhookApiKey) APIKey {
	return APIKey{
		ID:             uuid.UUID(model.ID.Bytes),
		OrganizationID: uuid.UUID(model.OrganizationID.Bytes),
		Name:           model.Name,
		KeyHash:        model.KeyHash,
		KeyPrefix:      model.KeyPrefix,
		AllowedDomains: model.AllowedDomains,
		IsActive:       model.IsActive,
		CreatedAt:      model.CreatedAt.Time,
		UpdatedAt:      model.UpdatedAt.Time,
	}
}

func googleWebhookConfigFromModel(model webhookdb.RacGoogleWebhookConfig) GoogleWebhookConfig {
	config := GoogleWebhookConfig{
		ID:              uuid.UUID(model.ID.Bytes),
		OrganizationID:  uuid.UUID(model.OrganizationID.Bytes),
		Name:            model.Name,
		GoogleKeyHash:   model.GoogleKeyHash,
		GoogleKeyPrefix: model.GoogleKeyPrefix,
		IsActive:        model.IsActive,
		CreatedAt:       model.CreatedAt.Time,
		UpdatedAt:       model.UpdatedAt.Time,
	}
	if len(model.CampaignMappings) > 0 {
		var mappings map[string]string
		if err := json.Unmarshal(model.CampaignMappings, &mappings); err == nil {
			config.CampaignMappings = mappings
		}
	}
	return config
}

func (r *Repository) CreateTimelineEvent(ctx context.Context, params createTimelineEventParams) error {
	metadataJSON, err := json.Marshal(params.Metadata)
	if err != nil {
		return err
	}

	err = r.queries.CreateWebhookTimelineEvent(ctx, webhookdb.CreateWebhookTimelineEventParams{
		LeadID:         toPgUUID(params.LeadID),
		ServiceID:      toPgUUIDPtr(params.ServiceID),
		OrganizationID: toPgUUID(params.OrganizationID),
		ActorType:      params.ActorType,
		ActorName:      params.ActorName,
		EventType:      params.EventType,
		Title:          params.Title,
		Summary:        toPgText(params.Summary),
		Metadata:       metadataJSON,
	})
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
	row, err := r.queries.CreateWebhookAPIKey(ctx, webhookdb.CreateWebhookAPIKeyParams{
		OrganizationID: toPgUUID(orgID),
		Name:           name,
		KeyHash:        keyHash,
		KeyPrefix:      keyPrefix,
		AllowedDomains: allowedDomains,
	})
	if err != nil {
		return key, err
	}
	return apiKeyFromModel(row), nil
}

// GetByHash retrieves an active API key by its hash.
func (r *Repository) GetByHash(ctx context.Context, keyHash string) (APIKey, error) {
	row, err := r.queries.GetWebhookAPIKeyByHash(ctx, keyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrAPIKeyNotFound
	}
	if err != nil {
		return APIKey{}, err
	}
	return apiKeyFromModel(row), nil
}

func (r *Repository) GetOrganizationIDByWhatsAppDeviceID(ctx context.Context, deviceID string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(deviceID)
	if trimmed == "" {
		return uuid.UUID{}, ErrWhatsAppDeviceNotFound
	}

	const query = `
		SELECT organization_id
		FROM RAC_organization_settings
		WHERE whatsapp_device_id = $1
		   OR whatsapp_account_jid = $1
		LIMIT 1`

	var organizationID uuid.UUID
	if err := r.pool.QueryRow(ctx, query, trimmed).Scan(&organizationID); errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, ErrWhatsAppDeviceNotFound
	} else if err != nil {
		return uuid.UUID{}, err
	}

	return organizationID, nil
}

// ListByOrganization returns all API keys for an organization.
func (r *Repository) ListByOrganization(ctx context.Context, orgID uuid.UUID) ([]APIKey, error) {
	rows, err := r.queries.ListWebhookAPIKeysByOrganization(ctx, toPgUUID(orgID))
	if err != nil {
		return nil, err
	}

	keys := make([]APIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, apiKeyFromModel(row))
	}
	return keys, nil
}

// Revoke deactivates an API key.
func (r *Repository) Revoke(ctx context.Context, keyID uuid.UUID, orgID uuid.UUID) error {
	tag, err := r.queries.RevokeWebhookAPIKey(ctx, webhookdb.RevokeWebhookAPIKeyParams{ID: toPgUUID(keyID), OrganizationID: toPgUUID(orgID)})
	if err != nil {
		return err
	}
	if tag == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

func (r *Repository) Rotate(ctx context.Context, keyID uuid.UUID, orgID uuid.UUID, name string, keyHash string, keyPrefix string, allowedDomains []string) (APIKey, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return APIKey{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const selectExisting = `
		SELECT id, organization_id, name, key_hash, key_prefix, allowed_domains, is_active, created_at, updated_at
		FROM RAC_webhook_api_keys
		WHERE id = $1 AND organization_id = $2
		FOR UPDATE`

	var current APIKey
	if err := tx.QueryRow(ctx, selectExisting, keyID, orgID).Scan(
		&current.ID,
		&current.OrganizationID,
		&current.Name,
		&current.KeyHash,
		&current.KeyPrefix,
		&current.AllowedDomains,
		&current.IsActive,
		&current.CreatedAt,
		&current.UpdatedAt,
	); errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrAPIKeyNotFound
	} else if err != nil {
		return APIKey{}, err
	}
	if !current.IsActive {
		return APIKey{}, ErrAPIKeyNotFound
	}

	queries := webhookdb.New(tx)
	created, err := queries.CreateWebhookAPIKey(ctx, webhookdb.CreateWebhookAPIKeyParams{
		OrganizationID: toPgUUID(orgID),
		Name:           name,
		KeyHash:        keyHash,
		KeyPrefix:      keyPrefix,
		AllowedDomains: allowedDomains,
	})
	if err != nil {
		return APIKey{}, err
	}

	tag, err := queries.RevokeWebhookAPIKey(ctx, webhookdb.RevokeWebhookAPIKeyParams{ID: toPgUUID(keyID), OrganizationID: toPgUUID(orgID)})
	if err != nil {
		return APIKey{}, err
	}
	if tag == 0 {
		return APIKey{}, ErrAPIKeyNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return APIKey{}, err
	}

	return apiKeyFromModel(created), nil
}

// UpdateWebhookLeadData sets webhook-specific columns on a lead (raw_form_data, source domain, is_incomplete).
func (r *Repository) UpdateWebhookLeadData(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID, rawFormData []byte, sourceDomain string, isIncomplete bool) error {
	return r.queries.UpdateWebhookLeadData(ctx, webhookdb.UpdateWebhookLeadDataParams{
		ID:                  toPgUUID(leadID),
		OrganizationID:      toPgUUID(orgID),
		RawFormData:         rawFormData,
		WebhookSourceDomain: toPgTextValue(sourceDomain),
		IsIncomplete:        isIncomplete,
	})
}

// FindRecentDuplicateLead checks if a lead with the same email and phone was created recently.
func (r *Repository) FindRecentDuplicateLead(ctx context.Context, orgID uuid.UUID, email, phone string, window time.Duration) (*uuid.UUID, error) {
	if email == "" && phone == "" {
		return nil, nil
	}

	row, err := r.queries.FindRecentDuplicateLead(ctx, webhookdb.FindRecentDuplicateLeadParams{
		OrganizationID: toPgUUID(orgID),
		Secs:           window.Seconds(),
		Column3:        email,
		Column4:        phone,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	leadID := uuid.UUID(row.Bytes)
	return &leadID, nil
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
	id, err := r.queries.CreateGoogleWebhookConfig(ctx, webhookdb.CreateGoogleWebhookConfigParams{
		OrganizationID:  toPgUUID(orgID),
		Name:            name,
		GoogleKeyHash:   googleKeyHash,
		GoogleKeyPrefix: googleKeyPrefix,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.UUID(id.Bytes), nil
}

// GetGoogleConfigByKey looks up a Google webhook config by its hashed key.
func (r *Repository) GetGoogleConfigByKey(ctx context.Context, googleKeyHash string) (GoogleWebhookConfig, error) {
	row, err := r.queries.GetGoogleWebhookConfigByHash(ctx, googleKeyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoogleWebhookConfig{}, ErrGoogleConfigNotFound
	}
	if err != nil {
		return GoogleWebhookConfig{}, err
	}
	return googleWebhookConfigFromModel(row), nil
}

// ListGoogleConfigs returns all Google webhook configs for an organization.
func (r *Repository) ListGoogleConfigs(ctx context.Context, orgID uuid.UUID) ([]GoogleWebhookConfig, error) {
	rows, err := r.queries.ListGoogleWebhookConfigs(ctx, toPgUUID(orgID))
	if err != nil {
		return nil, err
	}

	configs := make([]GoogleWebhookConfig, 0, len(rows))
	for _, row := range rows {
		configs = append(configs, googleWebhookConfigFromModel(row))
	}
	return configs, nil
}

// UpdateGoogleCampaignMapping adds or updates a campaign → service mapping.
func (r *Repository) UpdateGoogleCampaignMapping(ctx context.Context, configID uuid.UUID, orgID uuid.UUID, campaignID int64, serviceType string) error {
	campaignKey := strconv.FormatInt(campaignID, 10)
	return r.queries.UpsertGoogleCampaignMapping(ctx, webhookdb.UpsertGoogleCampaignMappingParams{
		ID:             toPgUUID(configID),
		OrganizationID: toPgUUID(orgID),
		Column3:        campaignKey,
		Column4:        serviceType,
	})
}

// DeleteGoogleCampaignMapping removes a campaign mapping from a config.
func (r *Repository) DeleteGoogleCampaignMapping(ctx context.Context, configID uuid.UUID, orgID uuid.UUID, campaignID int64) error {
	campaignKey := strconv.FormatInt(campaignID, 10)
	return r.queries.DeleteGoogleCampaignMapping(ctx, webhookdb.DeleteGoogleCampaignMappingParams{
		ID:             toPgUUID(configID),
		OrganizationID: toPgUUID(orgID),
		Column3:        campaignKey,
	})
}

// DeleteGoogleConfig deletes a Google webhook configuration.
func (r *Repository) DeleteGoogleConfig(ctx context.Context, configID uuid.UUID, orgID uuid.UUID) error {
	tag, err := r.queries.DeleteGoogleWebhookConfig(ctx, webhookdb.DeleteGoogleWebhookConfigParams{ID: toPgUUID(configID), OrganizationID: toPgUUID(orgID)})
	if err != nil {
		return err
	}
	if tag == 0 {
		return ErrGoogleConfigNotFound
	}
	return nil
}

// CheckGoogleLeadIDExists checks if a Google lead ID has already been processed.
func (r *Repository) CheckGoogleLeadIDExists(ctx context.Context, leadID string) (bool, error) {
	return r.queries.GoogleLeadIDExists(ctx, leadID)
}

// StoreGoogleLeadID stores a Google lead ID to prevent duplicate processing.
func (r *Repository) StoreGoogleLeadID(ctx context.Context, leadID string, orgID uuid.UUID, leadUUID *uuid.UUID, isTest bool) error {
	return r.queries.StoreGoogleLeadID(ctx, webhookdb.StoreGoogleLeadIDParams{
		LeadID:         leadID,
		OrganizationID: toPgUUID(orgID),
		LeadUuid:       toPgUUIDPtr(leadUUID),
		IsTest:         isTest,
	})
}

// UpdateGoogleLeadMetadata sets Google-specific metadata on a lead.
func (r *Repository) UpdateGoogleLeadMetadata(ctx context.Context, leadID uuid.UUID, campaignID, creativeID, adGroupID, formID int64) error {
	return r.queries.UpdateGoogleLeadMetadata(ctx, webhookdb.UpdateGoogleLeadMetadataParams{
		ID:               toPgUUID(leadID),
		GoogleCampaignID: toPgInt8Value(campaignID),
		GoogleCreativeID: toPgInt8Value(creativeID),
		GoogleAdgroupID:  toPgInt8Value(adGroupID),
		GoogleFormID:     toPgInt8Value(formID),
	})
}

// ---- GTM Config (Webhook SDK) ----

// GetGTMContainerID returns the GTM container ID for an organization (or nil if not set).
func (r *Repository) GetGTMContainerID(ctx context.Context, orgID uuid.UUID) (*string, error) {
	containerID, err := r.queries.GetOrganizationGTMContainerID(ctx, toPgUUID(orgID))
	if err != nil {
		return nil, err
	}
	if !containerID.Valid {
		return nil, nil
	}
	trimmed := strings.TrimSpace(containerID.String)
	if trimmed == "" {
		return nil, nil
	}
	return &trimmed, nil
}

// SetGTMContainerID sets the GTM container ID for an organization.
func (r *Repository) SetGTMContainerID(ctx context.Context, orgID uuid.UUID, containerID string) error {
	return r.queries.SetOrganizationGTMContainerID(ctx, webhookdb.SetOrganizationGTMContainerIDParams{
		ID:             toPgUUID(orgID),
		GtmContainerID: toPgTextValue(containerID),
	})
}

// ClearGTMContainerID clears the GTM container ID for an organization.
func (r *Repository) ClearGTMContainerID(ctx context.Context, orgID uuid.UUID) error {
	return r.queries.ClearOrganizationGTMContainerID(ctx, toPgUUID(orgID))
}

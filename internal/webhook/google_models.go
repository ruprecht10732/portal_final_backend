package webhook

import (
	"time"

	"github.com/google/uuid"
)

// Google Lead Form webhook payload structures based on:
// https://developers.google.com/google-ads/webhook/docs/implementation

// GoogleLeadPayload represents the webhook payload from Google Ads Lead Forms.
type GoogleLeadPayload struct {
	GoogleKey      string             `json:"google_key"`       // Authentication key
	LeadID         string             `json:"lead_id"`          // Unique lead identifier
	CampaignID     int64              `json:"campaign_id"`      // Google Ads campaign ID
	FormID         int64              `json:"form_id"`          // Lead form ID
	AdGroupID      int64              `json:"adgroup_id"`       // Ad group ID
	CreativeID     int64              `json:"creative_id"`      // Creative/ad ID
	GCLID          string             `json:"gclid"`            // Google Click ID
	UserColumnData []GoogleColumnData `json:"user_column_data"` // Form field data
	IsTest         bool               `json:"is_test"`          // Test lead flag
	APIVersion     string             `json:"api_version"`      // Google API version
	GCLIDURL       string             `json:"gclidurl"`         // Landing page URL
	AdGroupName    string             `json:"adgroup_name"`     // Ad group name (optional)
	CampaignName   string             `json:"campaign_name"`    // Campaign name (optional)
	FormName       string             `json:"form_name"`        // Form name (optional)
}

// GoogleColumnData represents a single form field from Google Lead Form.
type GoogleColumnData struct {
	ColumnID    string `json:"column_id"`    // Field identifier
	StringValue string `json:"string_value"` // Field value
	ColumnName  string `json:"column_name"`  // Human-readable field name
}

// GoogleWebhookConfig represents a stored Google webhook configuration.
type GoogleWebhookConfig struct {
	ID               uuid.UUID         `json:"id"`
	OrganizationID   uuid.UUID         `json:"organizationId"`
	Name             string            `json:"name"`
	GoogleKey        string            `json:"googleKey"` // Plaintext key (returned on creation only)
	GoogleKeyHash    string            `json:"-"`         // Stored hash
	GoogleKeyPrefix  string            `json:"googleKeyPrefix"`
	CampaignMappings map[string]string `json:"campaignMappings"` // Campaign ID → Service type
	IsActive         bool              `json:"isActive"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

// CreateGoogleWebhookConfigRequest is the admin API request to create a new config.
type CreateGoogleWebhookConfigRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

// CreateGoogleWebhookConfigResponse returns the new config with plaintext key.
type CreateGoogleWebhookConfigResponse struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	GoogleKey       string    `json:"googleKey"` // Plaintext key shown only once
	GoogleKeyPrefix string    `json:"googleKeyPrefix"`
	WebhookURL      string    `json:"webhookUrl"` // Full webhook endpoint URL
	CreatedAt       string    `json:"createdAt"`
}

// GoogleWebhookConfigResponse is the response for list/get operations.
type GoogleWebhookConfigResponse struct {
	ID               uuid.UUID         `json:"id"`
	Name             string            `json:"name"`
	GoogleKeyPrefix  string            `json:"googleKeyPrefix"`
	CampaignMappings map[string]string `json:"campaignMappings"`
	IsActive         bool              `json:"isActive"`
	CreatedAt        string            `json:"createdAt"`
	UpdatedAt        string            `json:"updatedAt"`
}

// UpdateCampaignMappingRequest updates or adds a campaign → service mapping.
type UpdateCampaignMappingRequest struct {
	CampaignID  string `json:"campaignId" validate:"required"`
	ServiceType string `json:"serviceType" validate:"required"`
}

// DeleteCampaignMappingRequest deletes a campaign → service mapping.
type DeleteCampaignMappingRequest struct {
	CampaignID string `json:"campaignId" validate:"required"`
}

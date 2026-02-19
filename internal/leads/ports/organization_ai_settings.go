package ports

import (
	"context"

	"github.com/google/uuid"
)

// OrganizationAISettings are tenant-scoped toggles and heuristics that control
// autonomous AI behavior and catalog improvement signals.
//
// These settings are persisted in the identity bounded context
// (RAC_organization_settings) but are consumed by other domains.
type OrganizationAISettings struct {
	AIAutoDisqualifyJunk   bool
	AIAutoDispatch         bool
	AIAutoEstimate         bool
	CatalogGapThreshold    int
	CatalogGapLookbackDays int
}

// DefaultOrganizationAISettings must match the identity repository defaults.
func DefaultOrganizationAISettings() OrganizationAISettings {
	return OrganizationAISettings{
		AIAutoDisqualifyJunk:   true,
		AIAutoDispatch:         false,
		AIAutoEstimate:         true,
		CatalogGapThreshold:    3,
		CatalogGapLookbackDays: 30,
	}
}

// OrganizationAISettingsReader loads the latest AI settings for a tenant.
//
// Returning an error should be treated as "unknown settings" by callers; most
// autonomous actions should fail-safe (skip) when settings cannot be loaded.
type OrganizationAISettingsReader func(ctx context.Context, organizationID uuid.UUID) (OrganizationAISettings, error)

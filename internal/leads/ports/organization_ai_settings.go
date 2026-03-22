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
	AIAutoDisqualifyJunk                              bool
	AIAutoDispatch                                    bool
	AIAutoEstimate                                    bool
	AIConfidenceGateEnabled                           bool
	AIAdaptiveReasoning                               bool
	AIExperienceMemory                                bool
	AICouncilMode                                     bool
	AICouncilConsensusMode                            string
	WhatsAppToneOfVoice                               string
	WhatsAppDefaultReplyScenario                      ReplySuggestionScenario
	EmailDefaultReplyScenario                         ReplySuggestionScenario
	QuoteRelatedReplyScenario                         ReplySuggestionScenario
	AppointmentRelatedReplyScenario                   ReplySuggestionScenario
	CatalogGapThreshold                               int
	CatalogGapLookbackDays                            int
	PhotoAnalysisPreprocessingEnabled                 bool
	PhotoAnalysisOCRAssistEnabled                     bool
	PhotoAnalysisOCRAssistServiceTypes                []string
	PhotoAnalysisLensCorrectionEnabled                bool
	PhotoAnalysisLensCorrectionServiceTypes           []string
	PhotoAnalysisPerspectiveNormalizationEnabled      bool
	PhotoAnalysisPerspectiveNormalizationServiceTypes []string
	DailyDigestEnabled                                bool
}

// DefaultOrganizationAISettings must match the identity repository defaults.
func DefaultOrganizationAISettings() OrganizationAISettings {
	return OrganizationAISettings{
		AIAutoDisqualifyJunk:                              true,
		AIAutoDispatch:                                    false,
		AIAutoEstimate:                                    true,
		AIConfidenceGateEnabled:                           false,
		AIAdaptiveReasoning:                               true,
		AIExperienceMemory:                                true,
		AICouncilMode:                                     true,
		AICouncilConsensusMode:                            "weighted",
		WhatsAppToneOfVoice:                               "warm, practical, and professional",
		WhatsAppDefaultReplyScenario:                      ReplySuggestionScenarioGeneric,
		EmailDefaultReplyScenario:                         ReplySuggestionScenarioGeneric,
		QuoteRelatedReplyScenario:                         ReplySuggestionScenarioQuoteReminder,
		AppointmentRelatedReplyScenario:                   ReplySuggestionScenarioAppointmentReminder,
		CatalogGapThreshold:                               3,
		CatalogGapLookbackDays:                            30,
		PhotoAnalysisPreprocessingEnabled:                 true,
		PhotoAnalysisOCRAssistEnabled:                     false,
		PhotoAnalysisOCRAssistServiceTypes:                []string{},
		PhotoAnalysisLensCorrectionEnabled:                false,
		PhotoAnalysisLensCorrectionServiceTypes:           []string{},
		PhotoAnalysisPerspectiveNormalizationEnabled:      false,
		PhotoAnalysisPerspectiveNormalizationServiceTypes: []string{},
		DailyDigestEnabled:                                true,
	}
}

// OrganizationAISettingsReader loads the latest AI settings for a tenant.
//
// Returning an error should be treated as "unknown settings" by callers; most
// autonomous actions should fail-safe (skip) when settings cannot be loaded.
type OrganizationAISettingsReader func(ctx context.Context, organizationID uuid.UUID) (OrganizationAISettings, error)

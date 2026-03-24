package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/internal/leads/agent"
	leadsdb "portal_final_backend/internal/leads/db"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/platform/logger"
)

const staleReEngagementCooldownHours = 24

// StaleLeadReEngagementService orchestrates AI-powered re-engagement
// suggestion generation for stale lead services. It checks the feature flag,
// applies a cooldown window to avoid regenerating recent suggestions, calls
// the LLM agent, and persists the result.
type StaleLeadReEngagementService struct {
	pool     *pgxpool.Pool
	queries  *leadsdb.Queries
	agent    *agent.StaleReEngagementAgent
	aiReader ports.OrganizationAISettingsReader
	log      *logger.Logger
}

// NewStaleLeadReEngagementService creates a new service. If the agent is nil
// the service gracefully no-ops.
func NewStaleLeadReEngagementService(
	pool *pgxpool.Pool,
	reengagementAgent *agent.StaleReEngagementAgent,
	aiReader ports.OrganizationAISettingsReader,
	log *logger.Logger,
) *StaleLeadReEngagementService {
	return &StaleLeadReEngagementService{
		pool:     pool,
		queries:  leadsdb.New(pool),
		agent:    reengagementAgent,
		aiReader: aiReader,
		log:      log,
	}
}

// SetOrganizationAISettingsReader injects the tenant-scoped settings reader
// after construction, following the same pattern as other lead agents.
func (s *StaleLeadReEngagementService) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	if s == nil {
		return
	}
	s.aiReader = reader
}

// ProcessReEngagement generates (or refreshes) a re-engagement suggestion for
// a single stale lead service. It implements the
// scheduler.StaleLeadReEngageProcessor interface.
func (s *StaleLeadReEngagementService) ProcessReEngagement(
	ctx context.Context,
	orgID, leadID, serviceID uuid.UUID,
	staleReason string,
) error {
	if s == nil || s.agent == nil {
		return nil
	}

	// Check feature flag.
	if !s.isEnabled(ctx, orgID) {
		return nil
	}

	// Cooldown: skip if a suggestion was generated recently.
	if s.hasRecentSuggestion(ctx, serviceID, orgID) {
		return nil
	}

	result, err := s.agent.GenerateSuggestion(ctx, orgID, leadID, serviceID, staleReason)
	if err != nil {
		return fmt.Errorf("stale reengagement: generate suggestion for service %s: %w", serviceID, err)
	}

	if err := s.persistSuggestion(ctx, orgID, leadID, serviceID, staleReason, result); err != nil {
		return fmt.Errorf("stale reengagement: persist suggestion for service %s: %w", serviceID, err)
	}

	if s.log != nil {
		s.log.Info("stale reengagement: suggestion generated",
			"orgId", orgID,
			"leadId", leadID,
			"serviceId", serviceID,
			"action", result.RecommendedAction,
			"channel", result.PreferredContactChannel,
		)
	}
	return nil
}

func (s *StaleLeadReEngagementService) isEnabled(ctx context.Context, orgID uuid.UUID) bool {
	if s.aiReader == nil {
		return false
	}
	settings, err := s.aiReader(ctx, orgID)
	if err != nil {
		if s.log != nil {
			s.log.Warn("stale reengagement: failed to read AI settings", "orgId", orgID, "error", err)
		}
		return false
	}
	return settings.AIStaleLeadReEngagementEnabled
}

func (s *StaleLeadReEngagementService) hasRecentSuggestion(ctx context.Context, serviceID, orgID uuid.UUID) bool {
	row, err := s.queries.GetStaleLeadSuggestion(ctx, leadsdb.GetStaleLeadSuggestionParams{
		LeadServiceID:  pgtype.UUID{Bytes: serviceID, Valid: true},
		OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true},
	})
	if err != nil {
		// No existing suggestion — not recent.
		return false
	}
	if !row.CreatedAt.Valid {
		return false
	}
	return time.Since(row.CreatedAt.Time) < staleReEngagementCooldownHours*time.Hour
}

func (s *StaleLeadReEngagementService) persistSuggestion(
	ctx context.Context,
	orgID, leadID, serviceID uuid.UUID,
	staleReason string,
	result agent.StaleReEngagementResult,
) error {
	return s.queries.UpsertStaleLeadSuggestion(ctx, leadsdb.UpsertStaleLeadSuggestionParams{
		LeadID:                  pgtype.UUID{Bytes: leadID, Valid: true},
		LeadServiceID:           pgtype.UUID{Bytes: serviceID, Valid: true},
		OrganizationID:          pgtype.UUID{Bytes: orgID, Valid: true},
		StaleReason:             staleReason,
		RecommendedAction:       result.RecommendedAction,
		SuggestedContactMessage: result.SuggestedContactMessage,
		PreferredContactChannel: result.PreferredContactChannel,
		Summary:                 result.Summary,
	})
}

// SuggestionLookup is a read-only snapshot of AI suggestions keyed by service ID.
type SuggestionLookup struct {
	RecommendedAction       string
	SuggestedContactMessage string
	PreferredContactChannel string
	Summary                 string
}

// ListSuggestionsByOrg returns a map of service-ID → SuggestionLookup for the
// given organization. The handler uses this to enrich stale-lead API responses.
func (s *StaleLeadReEngagementService) ListSuggestionsByOrg(ctx context.Context, orgID uuid.UUID) (map[uuid.UUID]SuggestionLookup, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.queries.ListStaleLeadSuggestionsByOrg(ctx, pgtype.UUID{Bytes: orgID, Valid: true})
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]SuggestionLookup, len(rows))
	for _, r := range rows {
		if !r.LeadServiceID.Valid {
			continue
		}
		m[uuid.UUID(r.LeadServiceID.Bytes)] = SuggestionLookup{
			RecommendedAction:       r.RecommendedAction,
			SuggestedContactMessage: r.SuggestedContactMessage,
			PreferredContactChannel: r.PreferredContactChannel,
			Summary:                 r.Summary,
		}
	}
	return m, nil
}

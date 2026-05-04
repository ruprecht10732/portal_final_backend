package agent

import (
	"fmt"
	"log"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
	apptools "portal_final_backend/internal/tools"
)

func createFindMatchingPartnersTool() (tool.Tool, error) {
	return apptools.NewFindMatchingPartnersTool(withDeps(handleFindMatchingPartners))
}

func createCreatePartnerOfferTool() (tool.Tool, error) {
	return apptools.NewCreatePartnerOfferTool(withDeps(func(ctx tool.Context, deps *ToolDependencies, input CreatePartnerOfferInput) (CreatePartnerOfferOutput, error) {
		if deps.OfferCreator == nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Offer creation not configured"}, fmt.Errorf("offer creator not configured")
		}

		tenantID, serviceID, partnerID, hours, contextMessage, err := resolveOfferContext(deps, input.PartnerID, input.ExpirationHours)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: contextMessage}, err
		}

		quoteID, err := deps.Repo.GetLatestAcceptedQuoteIDForService(ctx, serviceID, tenantID)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Accepted quote not found for service"}, err
		}

		summary := truncateRunes(strings.TrimSpace(input.JobSummaryShort), 200)
		result, err := deps.OfferCreator.CreateOfferFromQuote(ctx, tenantID, ports.CreateOfferFromQuoteParams{
			PartnerID:         partnerID,
			QuoteID:           quoteID,
			ExpiresInHours:    hours,
			JobSummaryShort:   summary,
			MarginBasisPoints: input.MarginBasisPoints,
			VakmanPriceCents:  input.VakmanPriceCents,
		})
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkOfferCreated()
		return CreatePartnerOfferOutput{Success: true, Message: "Offer created", OfferID: result.OfferID.String(), PublicToken: result.PublicToken}, nil
	}))
}

func handleFindMatchingPartners(ctx tool.Context, deps *ToolDependencies, input FindMatchingPartnersInput) (FindMatchingPartnersOutput, error) {
	tenantID, err := getTenantID(deps)
	if err != nil {
		return FindMatchingPartnersOutput{Matches: nil}, err
	}

	excludeUUIDs := parsePartnerExclusions(input.ExcludePartnerIDs)

	leadID, serviceID, err := getLeadContext(deps)
	if err != nil {
		return FindMatchingPartnersOutput{Matches: nil}, err
	}

	matches, err := deps.Repo.FindMatchingPartners(ctx, tenantID, leadID, input.ServiceType, input.ZipCode, input.RadiusKm, excludeUUIDs)
	if err != nil {
		return FindMatchingPartnersOutput{Matches: nil}, err
	}

	statsByPartner := lookupPartnerOfferStats(ctx, deps, tenantID, matches)
	recordPartnerSearchTimelineEvent(ctx, deps, tenantID, leadID, serviceID, input, len(matches))
	log.Printf("dispatcher FindMatchingPartners: run=%s lead=%s service=%s matches=%d", deps.GetRunID(), leadID, serviceID, len(matches))

	return FindMatchingPartnersOutput{Matches: buildPartnerMatchOutput(matches, statsByPartner)}, nil
}

func parsePartnerExclusions(rawIDs []string) []uuid.UUID {
	excludeUUIDs := make([]uuid.UUID, 0, len(rawIDs))
	for _, idStr := range rawIDs {
		if uid, err := uuid.Parse(idStr); err == nil {
			excludeUUIDs = append(excludeUUIDs, uid)
		}
	}
	return excludeUUIDs
}

func lookupPartnerOfferStats(ctx tool.Context, deps *ToolDependencies, tenantID uuid.UUID, matches []repository.PartnerMatch) map[uuid.UUID]repository.PartnerOfferStats {
	partnerIDs := make([]uuid.UUID, 0, len(matches))
	for _, m := range matches {
		partnerIDs = append(partnerIDs, m.ID)
	}
	if len(partnerIDs) == 0 {
		return map[uuid.UUID]repository.PartnerOfferStats{}
	}

	since := time.Now().AddDate(0, 0, -30)
	statsByPartner, statsErr := deps.Repo.GetPartnerOfferStatsSince(ctx, tenantID, partnerIDs, since)
	if statsErr != nil {
		// Non-fatal: if stats query fails, fall back to distance-only selection.
		log.Printf("FindMatchingPartners: offer stats lookup failed: %v", statsErr)
		return map[uuid.UUID]repository.PartnerOfferStats{}
	}
	return statsByPartner
}

func recordPartnerSearchTimelineEvent(ctx tool.Context, deps *ToolDependencies, tenantID, leadID, serviceID uuid.UUID, input FindMatchingPartnersInput, matchCount int) {
	actorType, actorName := deps.GetActor()
	summary := fmt.Sprintf("Found %d partner(s)", matchCount)
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypePartnerSearch,
		Title:          repository.EventTitlePartnerSearch,
		Summary:        &summary,
		Metadata: repository.PartnerSearchMetadata{
			ServiceType: input.ServiceType,
			ZipCode:     input.ZipCode,
			RadiusKm:    input.RadiusKm,
			MatchCount:  matchCount,
		}.ToMap(),
	})
}

func buildPartnerMatchOutput(matches []repository.PartnerMatch, statsByPartner map[uuid.UUID]repository.PartnerOfferStats) []PartnerMatch {
	output := make([]PartnerMatch, 0, len(matches))
	for _, match := range matches {
		stats := statsByPartner[match.ID]
		output = append(output, PartnerMatch{
			PartnerID:         match.ID.String(),
			BusinessName:      match.BusinessName,
			Email:             match.Email,
			DistanceKm:        match.DistanceKm,
			RejectedOffers30d: stats.Rejected,
			AcceptedOffers30d: stats.Accepted,
			OpenOffers30d:     stats.Open,
		})
	}
	return output
}

func resolveOfferContext(deps *ToolDependencies, partnerIDRaw string, expirationHours int) (uuid.UUID, uuid.UUID, uuid.UUID, int, string, error) {
	tenantID, err := getTenantID(deps)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, 0, missingTenantContextMessage, err
	}

	_, serviceID, err := getLeadContext(deps)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, 0, missingLeadContextMessage, err
	}

	partnerID, err := uuid.Parse(partnerIDRaw)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, 0, "Invalid partner ID", err
	}

	hours := expirationHours
	if hours <= 0 {
		hours = 12
	}
	if hours > 72 {
		hours = 72
	}

	return tenantID, serviceID, partnerID, hours, "", nil
}

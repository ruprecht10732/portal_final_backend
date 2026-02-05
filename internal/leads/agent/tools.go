package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
)

const (
	invalidLeadIDMessage        = "Invalid lead ID"
	invalidLeadServiceIDMessage = "Invalid lead service ID"
	missingTenantContextMessage = "Missing tenant context"
)

// normalizeUrgencyLevel converts various urgency level formats to the required values: High, Medium, Low
func normalizeUrgencyLevel(level string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(level))

	switch normalized {
	case "high", "hoog", "urgent", "spoed", "spoedeisend", "critical":
		return "High", nil
	case "medium", "mid", "moderate", "matig", "gemiddeld", "normal":
		return "Medium", nil
	case "low", "laag", "non-urgent", "niet-urgent", "minor":
		return "Low", nil
	default:
		// If unrecognized, default to Medium but log it
		log.Printf("Unrecognized urgency level '%s', defaulting to Medium", level)
		return "Medium", nil
	}
}

// normalizeLeadQuality converts various lead quality formats to the required values: Junk, Low, Potential, High, Urgent
func normalizeLeadQuality(quality string) string {
	normalized := strings.ToLower(strings.TrimSpace(quality))

	switch normalized {
	case "junk", "spam", "rommel", "onzin", "fake":
		return "Junk"
	case "low", "laag":
		return "Low"
	case "potential", "potentieel", "medium", "gemiddeld", "moderate", "mid":
		return "Potential"
	case "high", "hoog", "good", "goed":
		return "High"
	case "urgent", "spoed", "critical", "kritiek":
		return "Urgent"
	default:
		log.Printf("Unrecognized lead quality '%s', defaulting to Potential", quality)
		return "Potential"
	}
}

// ToolDependencies contains the dependencies needed by tools
type ToolDependencies struct {
	Repo          repository.LeadsRepository
	DraftedEmails map[uuid.UUID]EmailDraft
	Scorer        *scoring.Service
	EventBus      events.Bus
	mu            sync.RWMutex
	tenantID      *uuid.UUID
	leadID        *uuid.UUID
	serviceID     *uuid.UUID
	actorType     string
	actorName     string
}

func (d *ToolDependencies) SetTenantID(tenantID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenantID = &tenantID
}

func (d *ToolDependencies) GetTenantID() (*uuid.UUID, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tenantID == nil {
		return nil, false
	}
	return d.tenantID, true
}

func (d *ToolDependencies) SetLeadContext(leadID, serviceID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.leadID = &leadID
	d.serviceID = &serviceID
}

func (d *ToolDependencies) GetLeadContext() (uuid.UUID, uuid.UUID, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.leadID == nil || d.serviceID == nil {
		return uuid.UUID{}, uuid.UUID{}, false
	}
	return *d.leadID, *d.serviceID, true
}

func (d *ToolDependencies) SetActor(actorType, actorName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actorType = actorType
	d.actorName = actorName
}

func (d *ToolDependencies) GetActor() (string, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.actorType == "" {
		return "AI", "Agent"
	}
	return d.actorType, d.actorName
}

func parseUUID(value string, invalidMessage string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, errors.New(invalidMessage)
	}
	return parsed, nil
}

func getTenantID(deps *ToolDependencies) (uuid.UUID, error) {
	tenantID, ok := deps.GetTenantID()
	if !ok {
		return uuid.UUID{}, fmt.Errorf("missing tenant context")
	}
	return *tenantID, nil
}

func getLeadContext(deps *ToolDependencies) (uuid.UUID, uuid.UUID, error) {
	leadID, serviceID, ok := deps.GetLeadContext()
	if !ok {
		return uuid.UUID{}, uuid.UUID{}, fmt.Errorf("missing lead context")
	}
	return leadID, serviceID, nil
}

func normalizeContactChannel(channel string) (string, error) {
	clean := strings.TrimSpace(channel)
	normalized := strings.ToLower(clean)

	// WhatsApp variations
	if strings.Contains(normalized, "whatsapp") || normalized == "wa" {
		return "WhatsApp", nil
	}

	// Email variations
	if strings.Contains(normalized, "email") || strings.Contains(normalized, "e-mail") || normalized == "mail" {
		return "Email", nil
	}

	// Phone/call variations - map to WhatsApp since it's our phone-based channel
	if strings.Contains(normalized, "phone") || strings.Contains(normalized, "telefoon") ||
		strings.Contains(normalized, "call") || strings.Contains(normalized, "bel") ||
		normalized == "tel" || normalized == "sms" {
		return "WhatsApp", nil
	}

	// If unrecognized, default to Email and log
	log.Printf("Unrecognized contact channel '%s', defaulting to Email", channel)
	return "Email", nil
}

func resolvePreferredChannel(inputChannel string, lead repository.Lead) (string, error) {
	_, err := normalizeContactChannel(inputChannel)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(lead.ConsumerPhone) != "" {
		return "WhatsApp", nil
	}
	return "Email", nil
}

func parseLeadServiceID(value string) (uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return uuid.UUID{}, fmt.Errorf("missing lead service ID")
	}
	return parseUUID(value, invalidLeadServiceIDMessage)
}

func handleSaveAnalysis(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: invalidLeadIDMessage}, err
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadServiceID, err := parseLeadServiceID(input.LeadServiceID)
	if err != nil {
		message := err.Error()
		if err.Error() == invalidLeadServiceIDMessage {
			message = invalidLeadServiceIDMessage
		}
		return SaveAnalysisOutput{Success: false, Message: message}, err
	}

	urgencyLevel, err := normalizeUrgencyLevel(input.UrgencyLevel)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	var urgencyReason *string
	if input.UrgencyReason != "" {
		urgencyReason = &input.UrgencyReason
	}

	lead, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: "Lead not found"}, err
	}

	channel, err := resolvePreferredChannel(input.PreferredContactChannel, lead)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: "Invalid preferred contact channel"}, err
	}

	// Normalize lead quality to valid enum value
	leadQuality := normalizeLeadQuality(input.LeadQuality)

	_, err = deps.Repo.CreateAIAnalysis(context.Background(), repository.CreateAIAnalysisParams{
		LeadID:                  leadID,
		OrganizationID:          tenantID,
		LeadServiceID:           leadServiceID,
		UrgencyLevel:            urgencyLevel,
		UrgencyReason:           urgencyReason,
		LeadQuality:             leadQuality,
		RecommendedAction:       input.RecommendedAction,
		MissingInformation:      input.MissingInformation,
		PreferredContactChannel: channel,
		SuggestedContactMessage: input.SuggestedContactMessage,
		Summary:                 input.Summary,
	})
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	actorType, actorName := deps.GetActor()
	if len(input.MissingInformation) > 0 {
		summary := buildMissingInfoSummary(input.MissingInformation)
		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &leadServiceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      "analysis",
			Title:          "Ontbrekende informatie",
			Summary:        &summary,
			Metadata: map[string]any{
				"missingInformation": input.MissingInformation,
			},
		})
	}

	if deps.Scorer != nil {
		if scoreResult, scoreErr := deps.Scorer.Recalculate(ctx, leadID, &leadServiceID, tenantID, true); scoreErr == nil {
			_ = deps.Repo.UpdateLeadScore(ctx, leadID, tenantID, repository.UpdateLeadScoreParams{
				Score:          &scoreResult.Score,
				ScorePreAI:     &scoreResult.ScorePreAI,
				ScoreFactors:   scoreResult.FactorsJSON,
				ScoreVersion:   &scoreResult.Version,
				ScoreUpdatedAt: scoreResult.UpdatedAt,
			})

			summary := buildLeadScoreSummary(scoreResult)
			_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
				LeadID:         leadID,
				ServiceID:      &leadServiceID,
				OrganizationID: tenantID,
				ActorType:      actorType,
				ActorName:      actorName,
				EventType:      "analysis",
				Title:          "Leadscore bijgewerkt",
				Summary:        &summary,
				Metadata: map[string]any{
					"leadScore":        scoreResult.Score,
					"leadScorePreAI":   scoreResult.ScorePreAI,
					"leadScoreVersion": scoreResult.Version,
				},
			})
		}
	}

	log.Printf(
		"gatekeeper SaveAnalysis: leadId=%s serviceId=%s urgency=%s quality=%s action=%s missing=%d",
		leadID,
		leadServiceID,
		urgencyLevel,
		leadQuality,
		input.RecommendedAction,
		len(input.MissingInformation),
	)

	return SaveAnalysisOutput{Success: true, Message: "Analysis saved successfully"}, nil
}

func buildMissingInfoSummary(items []string) string {
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return "Ontbrekende informatie bijgewerkt"
	}

	limit := 4
	if len(cleaned) < limit {
		limit = len(cleaned)
	}
	preview := strings.Join(cleaned[:limit], "; ")
	if len(cleaned) > limit {
		return fmt.Sprintf("Ontbrekende informatie: %s (+%d)", preview, len(cleaned)-limit)
	}
	return fmt.Sprintf("Ontbrekende informatie: %s", preview)
}

func buildLeadScoreSummary(result *scoring.Result) string {
	return fmt.Sprintf("Leadscore %d (pre-AI %d)", result.Score, result.ScorePreAI)
}

func handleUpdateLeadServiceType(ctx tool.Context, deps *ToolDependencies, input UpdateLeadServiceTypeInput) (UpdateLeadServiceTypeOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: invalidLeadIDMessage}, err
	}
	leadServiceID, err := parseUUID(input.LeadServiceID, invalidLeadServiceIDMessage)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: invalidLeadServiceIDMessage}, err
	}
	serviceType := strings.TrimSpace(input.ServiceType)
	if serviceType == "" {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Missing service type"}, fmt.Errorf("missing service type")
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadService, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Lead service not found"}, err
	}
	if leadService.LeadID != leadID {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Lead service does not belong to lead"}, fmt.Errorf("lead service mismatch")
	}

	_, err = deps.Repo.UpdateLeadServiceType(ctx, leadServiceID, tenantID, serviceType)
	if err != nil {
		if errors.Is(err, repository.ErrServiceTypeNotFound) {
			return UpdateLeadServiceTypeOutput{Success: false, Message: "Service type not found or inactive"}, nil
		}
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Failed to update service type"}, err
	}

	log.Printf(
		"gatekeeper UpdateLeadServiceType: leadId=%s serviceId=%s from=%s to=%s",
		leadID,
		leadServiceID,
		leadService.ServiceType,
		serviceType,
	)

	return UpdateLeadServiceTypeOutput{Success: true, Message: "Service type updated"}, nil
}

// createSaveAnalysisTool creates the SaveAnalysis tool
func createSaveAnalysisTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SaveAnalysis",
		Description: "Saves the gatekeeper triage analysis to the database. Call this ONCE after completing your full analysis. Include urgency, lead quality, recommended action, missing information, preferred contact channel, message, and summary.",
	}, func(ctx tool.Context, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
		return handleSaveAnalysis(ctx, deps, input)
	})
}

// createUpdateLeadServiceTypeTool creates the UpdateLeadServiceType tool
func createUpdateLeadServiceTypeTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "UpdateLeadServiceType",
		Description: "Updates the service type for a lead service when there is a confident mismatch. The service type must match an active service type name or slug.",
	}, func(ctx tool.Context, input UpdateLeadServiceTypeInput) (UpdateLeadServiceTypeOutput, error) {
		return handleUpdateLeadServiceType(ctx, deps, input)
	})
}

// createDraftEmailTool creates the DraftFollowUpEmail tool
func createDraftEmailTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "DraftFollowUpEmail",
		Description: "Creates a draft follow-up email to send to the customer. Use this when you need more information from the customer before providing a quote, or to confirm details. The email will be saved as a draft for the sales advisor to review and send.",
	}, func(ctx tool.Context, input DraftEmailInput) (DraftEmailOutput, error) {
		leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
		if err != nil {
			return DraftEmailOutput{Success: false, Message: invalidLeadIDMessage}, err
		}

		draftID := uuid.New()
		draft := EmailDraft{
			ID:          draftID,
			LeadID:      leadID,
			Subject:     input.Subject,
			Body:        input.Body,
			Purpose:     input.Purpose,
			MissingInfo: input.MissingInfo,
			CreatedAt:   time.Now(),
		}

		deps.DraftedEmails[draftID] = draft
		log.Printf("Email draft created for lead %s: %s", leadID, input.Subject)

		return DraftEmailOutput{
			Success: true,
			Message: fmt.Sprintf("Email draft created: '%s'. Saved for review.", input.Subject),
			DraftID: draftID.String(),
		}, nil
	})
}

// createGetPricingTool creates the GetServicePricing tool
func createGetPricingTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "GetServicePricing",
		Description: "Retrieves typical pricing information for home services. Use this to help with objection handling around pricing, or to give customers realistic expectations. Returns price ranges, typical duration, and what's included.",
	}, func(ctx tool.Context, input GetPricingInput) (GetPricingOutput, error) {
		return getServicePricing(input.ServiceCategory, input.ServiceType, input.Urgency), nil
	})
}

// createSuggestSpecialistTool creates the SuggestSpecialist tool
func createSuggestSpecialistTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SuggestSpecialist",
		Description: "Analyzes a problem description and recommends the most appropriate type of specialist (plumber, electrician, HVAC technician, carpenter, handyman, etc.). Use this when the customer's problem spans multiple trades or when it's unclear which specialist they need.",
	}, func(ctx tool.Context, input SuggestSpecialistInput) (SuggestSpecialistOutput, error) {
		return suggestSpecialist(input.ProblemDescription, input.ServiceCategory), nil
	})
}

var validPipelineStages = map[string]bool{
	"Triage":              true,
	"Nurturing":           true,
	"Ready_For_Estimator": true,
	"Ready_For_Partner":   true,
	"Partner_Matching":    true,
	"Partner_Assigned":    true,
	"Manual_Intervention": true,
	"Completed":           true,
	"Lost":                true,
}

func createUpdatePipelineStageTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "UpdatePipelineStage",
		Description: "Updates the pipeline stage for the lead service and records a timeline event.",
	}, func(ctx tool.Context, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
		if !validPipelineStages[input.Stage] {
			return UpdatePipelineStageOutput{Success: false, Message: "Invalid pipeline stage"}, fmt.Errorf("invalid pipeline stage: %s", input.Stage)
		}

		tenantID, err := getTenantID(deps)
		if err != nil {
			return UpdatePipelineStageOutput{Success: false, Message: missingTenantContextMessage}, err
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return UpdatePipelineStageOutput{Success: false, Message: "Missing lead context"}, err
		}

		svc, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
		if err != nil {
			return UpdatePipelineStageOutput{Success: false, Message: "Lead service not found"}, err
		}
		oldStage := svc.PipelineStage

		_, err = deps.Repo.UpdatePipelineStage(ctx, serviceID, tenantID, input.Stage)
		if err != nil {
			return UpdatePipelineStageOutput{Success: false, Message: "Failed to update pipeline stage"}, err
		}

		actorType, actorName := deps.GetActor()
		reason := strings.TrimSpace(input.Reason)
		var summary *string
		if reason != "" {
			summary = &reason
		}

		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      "stage_change",
			Title:          "Stage Updated",
			Summary:        summary,
			Metadata: map[string]any{
				"oldStage": oldStage,
				"newStage": input.Stage,
			},
		})

		if deps.EventBus != nil {
			deps.EventBus.Publish(ctx, events.PipelineStageChanged{
				BaseEvent:     events.NewBaseEvent(),
				LeadID:        leadID,
				LeadServiceID: serviceID,
				TenantID:      tenantID,
				OldStage:      oldStage,
				NewStage:      input.Stage,
			})
		}

		if reason == "" {
			reason = "(no reason provided)"
		}
		log.Printf(
			"gatekeeper UpdatePipelineStage: leadId=%s serviceId=%s from=%s to=%s reason=%s",
			leadID,
			serviceID,
			oldStage,
			input.Stage,
			reason,
		)

		return UpdatePipelineStageOutput{Success: true, Message: "Pipeline stage updated"}, nil
	})
}

func createFindMatchingPartnersTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "FindMatchingPartners",
		Description: "Finds partner matches by service type and distance radius.",
	}, func(ctx tool.Context, input FindMatchingPartnersInput) (FindMatchingPartnersOutput, error) {
		tenantID, err := getTenantID(deps)
		if err != nil {
			return FindMatchingPartnersOutput{Matches: nil}, err
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return FindMatchingPartnersOutput{Matches: nil}, err
		}

		matches, err := deps.Repo.FindMatchingPartners(ctx, tenantID, input.ServiceType, input.ZipCode, input.RadiusKm)
		if err != nil {
			return FindMatchingPartnersOutput{Matches: nil}, err
		}

		actorType, actorName := deps.GetActor()
		summary := fmt.Sprintf("Found %d partner(s)", len(matches))
		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      "partner_search",
			Title:          "Partner search",
			Summary:        &summary,
			Metadata: map[string]any{
				"serviceType": input.ServiceType,
				"zipCode":     input.ZipCode,
				"radiusKm":    input.RadiusKm,
				"matches":     matches,
			},
		})

		output := make([]PartnerMatch, 0, len(matches))
		for _, match := range matches {
			output = append(output, PartnerMatch{
				PartnerID:    match.ID.String(),
				BusinessName: match.BusinessName,
				Email:        match.Email,
				DistanceKm:   match.DistanceKm,
			})
		}

		return FindMatchingPartnersOutput{Matches: output}, nil
	})
}

func createSaveEstimationTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SaveEstimation",
		Description: "Saves estimation metadata (scope and price range) to the lead timeline.",
	}, func(ctx tool.Context, input SaveEstimationInput) (SaveEstimationOutput, error) {
		tenantID, err := getTenantID(deps)
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: missingTenantContextMessage}, err
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: "Missing lead context"}, err
		}

		actorType, actorName := deps.GetActor()
		summary := strings.TrimSpace(input.Summary)
		var summaryPtr *string
		if summary != "" {
			summaryPtr = &summary
		}

		_, err = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      "analysis",
			Title:          "Estimation saved",
			Summary:        summaryPtr,
			Metadata: map[string]any{
				"scope":      input.Scope,
				"priceRange": input.PriceRange,
				"notes":      input.Notes,
			},
		})
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: "Failed to save estimation"}, err
		}

		return SaveEstimationOutput{Success: true, Message: "Estimation saved"}, nil
	})
}

// buildTools creates all tools for the LeadAdvisor agent
func buildTools(deps *ToolDependencies) ([]tool.Tool, error) {
	var tools []tool.Tool
	var errs []error

	saveAnalysisTool, err := createSaveAnalysisTool(deps)
	if err != nil {
		errs = append(errs, fmt.Errorf("SaveAnalysis tool: %w", err))
	} else {
		tools = append(tools, saveAnalysisTool)
	}

	updateLeadServiceTypeTool, err := createUpdateLeadServiceTypeTool(deps)
	if err != nil {
		errs = append(errs, fmt.Errorf("UpdateLeadServiceType tool: %w", err))
	} else {
		tools = append(tools, updateLeadServiceTypeTool)
	}

	draftEmailTool, err := createDraftEmailTool(deps)
	if err != nil {
		errs = append(errs, fmt.Errorf("DraftFollowUpEmail tool: %w", err))
	} else {
		tools = append(tools, draftEmailTool)
	}

	getPricingTool, err := createGetPricingTool()
	if err != nil {
		errs = append(errs, fmt.Errorf("GetServicePricing tool: %w", err))
	} else {
		tools = append(tools, getPricingTool)
	}

	suggestSpecialistTool, err := createSuggestSpecialistTool()
	if err != nil {
		errs = append(errs, fmt.Errorf("SuggestSpecialist tool: %w", err))
	} else {
		tools = append(tools, suggestSpecialistTool)
	}

	if len(errs) > 0 {
		return tools, fmt.Errorf("failed to create some tools: %v", errs)
	}

	return tools, nil
}

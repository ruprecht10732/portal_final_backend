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

	"portal_final_backend/internal/leads/repository"
)

const (
	invalidLeadIDMessage        = "Invalid lead ID"
	invalidLeadServiceIDMessage = "Invalid lead service ID"
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
	mu            sync.RWMutex
	tenantID      *uuid.UUID
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
		return SaveAnalysisOutput{Success: false, Message: "Missing tenant context"}, err
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

	return SaveAnalysisOutput{Success: true, Message: "Analysis saved successfully"}, nil
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
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Missing tenant context"}, err
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

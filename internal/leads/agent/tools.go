package agent

import (
	"context"
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

// createSaveAnalysisTool creates the SaveAnalysis tool
func createSaveAnalysisTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SaveAnalysis",
		Description: "Saves the complete lead analysis to the database. Call this ONCE after completing your full analysis. Include all sections: urgency (must be exactly 'High', 'Medium', or 'Low'), talking points, objections, upsells, and summary.",
	}, func(ctx tool.Context, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
		leadID, err := uuid.Parse(input.LeadID)
		if err != nil {
			return SaveAnalysisOutput{Success: false, Message: "Invalid lead ID"}, err
		}

		tenantID, ok := deps.GetTenantID()
		if !ok {
			return SaveAnalysisOutput{Success: false, Message: "Missing tenant context"}, fmt.Errorf("missing tenant context")
		}

		// Parse service ID if provided
		var leadServiceID *uuid.UUID
		if input.LeadServiceID != "" {
			parsed, err := uuid.Parse(input.LeadServiceID)
			if err != nil {
				return SaveAnalysisOutput{Success: false, Message: "Invalid lead service ID"}, err
			}
			leadServiceID = &parsed
		}

		// Normalize urgency level to valid database value
		urgencyLevel, err := normalizeUrgencyLevel(input.UrgencyLevel)
		if err != nil {
			return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
		}

		objections := make([]repository.ObjectionResponse, len(input.ObjectionHandling))
		for i, o := range input.ObjectionHandling {
			objections[i] = repository.ObjectionResponse{
				Objection: o.Objection,
				Response:  o.Response,
			}
		}

		var urgencyReason *string
		if input.UrgencyReason != "" {
			urgencyReason = &input.UrgencyReason
		}

		var whatsappMessage *string
		if input.SuggestedWhatsAppMessage != "" {
			whatsappMessage = &input.SuggestedWhatsAppMessage
		}

		_, err = deps.Repo.CreateAIAnalysis(context.Background(), repository.CreateAIAnalysisParams{
			LeadID:                   leadID,
			OrganizationID:           *tenantID,
			LeadServiceID:            leadServiceID,
			UrgencyLevel:             urgencyLevel,
			UrgencyReason:            urgencyReason,
			TalkingPoints:            input.TalkingPoints,
			ObjectionHandling:        objections,
			UpsellOpportunities:      input.UpsellOpportunities,
			SuggestedWhatsAppMessage: whatsappMessage,
			Summary:                  input.Summary,
		})
		if err != nil {
			return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
		}

		return SaveAnalysisOutput{Success: true, Message: "Analysis saved successfully"}, nil
	})
}

// createDraftEmailTool creates the DraftFollowUpEmail tool
func createDraftEmailTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "DraftFollowUpEmail",
		Description: "Creates a draft follow-up email to send to the customer. Use this when you need more information from the customer before providing a quote, or to confirm details. The email will be saved as a draft for the sales advisor to review and send.",
	}, func(ctx tool.Context, input DraftEmailInput) (DraftEmailOutput, error) {
		leadID, err := uuid.Parse(input.LeadID)
		if err != nil {
			return DraftEmailOutput{Success: false, Message: "Invalid lead ID"}, err
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

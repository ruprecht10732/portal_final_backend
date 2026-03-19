package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
)

const subsidyAnalyzerAppName = "subsidy-analyzer"

// SubsidyAnalyzer is the agent for analyzing quotes and suggesting subsidy parameters.
type SubsidyAnalyzer struct {
	runner         *runner.Runner
	sessionService session.Service
	modelConfig    moonshot.Config
	repo           repository.LeadsRepository
	appName        string
}

// SubsidyAnalyzerConfig holds dependencies for subsidy analyzer agent.
type SubsidyAnalyzerConfig struct {
	APIKey string
	Model  string
	Repo   repository.LeadsRepository
}

// NewSubsidyAnalyzerAgent creates a new subsidy analyzer agent.
func NewSubsidyAnalyzerAgent(cfg SubsidyAnalyzerConfig) (*SubsidyAnalyzer, error) {
	modelConfig := newMoonshotModelConfig(cfg.APIKey, cfg.Model)
	kimi := moonshot.NewModel(modelConfig)

	// Create ADK agent with LLM (no tools for now, agent will respond with structured text)
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "SubsidyAnalyzer",
		Model:       kimi,
		Description: "Analyzes quote line items and suggests relevant subsidy measures and installations.",
		Instruction: subsidyAnalyzerSystemPrompt(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subsidy analyzer agent: %w", err)
	}

	sessionService := session.InMemoryService()

	r, err := runner.New(runner.Config{
		AppName:        subsidyAnalyzerAppName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subsidy analyzer runner: %w", err)
	}

	return &SubsidyAnalyzer{
		runner:         r,
		sessionService: sessionService,
		modelConfig:    modelConfig,
		repo:           cfg.Repo,
		appName:        subsidyAnalyzerAppName,
	}, nil
}

// Run executes the agent for a given quote.
func (a *SubsidyAnalyzer) Run(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID, quoteContext string) (map[string]interface{}, error) {
	// Build the prompt for the agent
	prompt := fmt.Sprintf("Analyze this quote context and suggest appropriate subsidy measures and installations:\n\n%s", quoteContext)

	sessionID := quoteID.String()
	userID := "subsidy-analyzer-" + organizationID.String()

	// Run the agent prompt through the session-based runner
	outputText, err := runPromptTextSession(ctx, promptRunRequest{
		SessionService:       a.sessionService,
		Runner:               a.runner,
		AppName:              a.appName,
		UserID:               userID,
		SessionID:            sessionID,
		CreateSessionMessage: "subsidy analysis: create session",
		RunFailureMessage:    "subsidy analysis: run failed",
		TraceLabel:           "subsidy-analyzer",
	}, prompt)
	if err != nil {
		return nil, fmt.Errorf("subsidy analyzer run failed: %w", err)
	}

	// Parse the agent's response to extract structured suggestion
	result := a.parseAnalysisResponse(outputText)
	return result, nil
}

// parseAnalysisResponse parses the agent's text response into structured data.
func (a *SubsidyAnalyzer) parseAnalysisResponse(response string) map[string]interface{} {
	if strings.TrimSpace(response) == "" {
		return map[string]interface{}{}
	}

	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		candidate := response[jsonStart : jsonEnd+1]
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			normalizeMeasureTypeIDs(parsed)
			return parsed
		}
	}

	return map[string]interface{}{
		"confidence": "low",
		"reasoning":  strings.TrimSpace(response),
	}
}

func normalizeMeasureTypeIDs(parsed map[string]interface{}) {
	if parsed == nil {
		return
	}

	if normalized, ok := extractMeasureTypeIDs(parsed["measure_type_ids"]); ok {
		parsed["measure_type_ids"] = normalized
		setPrimaryMeasureTypeID(parsed, normalized)
		return
	}

	if measureTypeID, ok := parsed["measure_type_id"].(string); ok && strings.TrimSpace(measureTypeID) != "" {
		parsed["measure_type_ids"] = []string{measureTypeID}
	}
}

func extractMeasureTypeIDs(raw interface{}) ([]string, bool) {
	switch values := raw.(type) {
	case []interface{}:
		return normalizeMeasureTypeIDInterfaces(values), true
	case []string:
		return values, true
	default:
		return nil, false
	}
}

func normalizeMeasureTypeIDInterfaces(values []interface{}) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			normalized = append(normalized, text)
		}
	}
	return normalized
}

func setPrimaryMeasureTypeID(parsed map[string]interface{}, measureTypeIDs []string) {
	if len(measureTypeIDs) == 0 {
		return
	}

	parsed["measure_type_id"] = measureTypeIDs[0]
}

// subsidyAnalyzerSystemPrompt returns the system prompt for the subsidy analyzer agent.
func subsidyAnalyzerSystemPrompt() string {
	return `You are an expert subsidy and energy efficiency consultant. Your task is to analyze quote line items and suggest:

1. All relevant ISDE subsidy measure type IDs based on the services/products listed, in the same order as the quote lines
2. The appropriate installation meldcode ID if applicable

Use only these measure_type_id values when relevant:
- roof
- attic
- facade
- cavity_wall
- floor
- crawl_space
- hr_plus_plus
- triple_glass
- vacuum_glass
- glass_panel_low
- glass_panel_high
- insulated_door_low
- insulated_door_high

Respond in this exact JSON format:
{
	"measure_type_ids": ["zero or more allowed IDs from the list above, in quote line order"],
	"measure_type_id": "the first relevant ID from measure_type_ids or null",
	"installation_meldcode_id": "meldcode string or null", 
  "confidence": "high|medium|low",
  "reasoning": "Dutch explanation of why these measures were selected"
}

Include every relevant measure exactly once per matching quote line. If no subsidy measure applies, return an empty array and null for measure_type_id.

Only return valid JSON, nothing else.`
}

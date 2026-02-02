package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
)

// toolCallTracker tracks which tools have been called during a run
type toolCallTracker struct {
	mu                 sync.Mutex
	saveAnalysisCalled bool
}

func (t *toolCallTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.saveAnalysisCalled = false
}

func (t *toolCallTracker) markSaveAnalysisCalled() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.saveAnalysisCalled = true
}

func (t *toolCallTracker) wasSaveAnalysisCalled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.saveAnalysisCalled
}

// LeadAdvisor provides AI-powered lead analysis using the ADK framework
type LeadAdvisor struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	// Store drafted emails temporarily (in production, this would be persisted to DB)
	draftedEmails map[uuid.UUID]EmailDraft
	// Track tool calls during runs
	toolTracker *toolCallTracker
	toolDeps    *ToolDependencies
	runMu       sync.Mutex
}

// NewLeadAdvisor builds the AI Advisor agent with Kimi model
// Returns an error if the agent or runner cannot be initialized
func NewLeadAdvisor(apiKey string, repo repository.LeadsRepository) (*LeadAdvisor, error) {
	// Use kimi-k2.5 with thinking disabled for more reliable tool calling
	// Thinking mode restricts tool_choice to only "auto" or "none"
	// Non-thinking mode may allow more flexibility
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          apiKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	tracker := &toolCallTracker{}

	advisor := &LeadAdvisor{
		repo:          repo,
		draftedEmails: make(map[uuid.UUID]EmailDraft),
		appName:       "lead_advisor",
		toolTracker:   tracker,
	}

	// Create tool dependencies
	toolDeps := &ToolDependencies{
		Repo:          repo,
		DraftedEmails: advisor.draftedEmails,
	}
	advisor.toolDeps = toolDeps

	// Build all tools
	tools, err := buildTools(toolDeps)
	if err != nil {
		log.Printf("warning: some tools failed to initialize: %v", err)
		// Continue with partial tools rather than failing completely
	}

	// Create AfterToolCallback to track SaveAnalysis calls
	// Signature: func(ctx tool.Context, tool tool.Tool, args, result map[string]any, err error) (map[string]any, error)
	afterToolCallback := func(ctx tool.Context, t tool.Tool, args, result map[string]any, toolErr error) (map[string]any, error) {
		if t.Name() == "SaveAnalysis" {
			log.Printf("SaveAnalysis tool was called with result: %v, error: %v", result, toolErr)
			if toolErr == nil {
				tracker.markSaveAnalysisCalled()
			}
		}
		return nil, nil // Return nil to use original result
	}

	// Create the ADK agent with tool tracking callback
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:               "LeadAdvisor",
		Model:              kimi,
		Description:        "Expert AI Sales Advisor for home services marketplace (plumbing, HVAC, electrical, carpentry) that analyzes leads and provides personalized, actionable sales guidance.",
		Instruction:        getSystemPrompt(),
		Tools:              tools,
		AfterToolCallbacks: []llmagent.AfterToolCallback{afterToolCallback},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ADK agent: %w", err)
	}

	// Create session service for conversation management
	sessionService := session.InMemoryService()

	// Create the runner
	r, err := runner.New(runner.Config{
		AppName:        advisor.appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ADK runner: %w", err)
	}

	advisor.agent = adkAgent
	advisor.runner = r
	advisor.sessionService = sessionService

	return advisor, nil
}

// Analyze runs the AI advisor on a specific lead (legacy async method)
func (la *LeadAdvisor) Analyze(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, tenantID uuid.UUID) error {
	_, err := la.AnalyzeAndReturn(ctx, leadID, serviceID, false, tenantID)
	return err
}

// AnalyzeAndReturn runs the AI advisor and returns the result directly
// If serviceID is nil, it will use the current (most recent non-terminal) service
// If force is true, it will regenerate even if no changes are detected
func (la *LeadAdvisor) AnalyzeAndReturn(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, force bool, tenantID uuid.UUID) (*AnalyzeResponse, error) {
	la.runMu.Lock()
	defer la.runMu.Unlock()
	if la.toolDeps != nil {
		la.toolDeps.SetTenantID(tenantID)
	}

	if err := la.ensureInitialized(); err != nil {
		return nil, err
	}

	lead, targetService, meaningfulNotes, err := la.fetchLeadAndServiceNotes(ctx, leadID, serviceID, tenantID)
	if err != nil {
		return nil, err
	}

	// Require a service to analyze
	if targetService == nil {
		return &AnalyzeResponse{
			Status:  "error",
			Message: "No service found to analyze",
		}, nil
	}

	existingAnalysis, hasExisting, err := la.getExistingAnalysis(ctx, targetService.ID, tenantID)
	if err != nil {
		return nil, err
	}

	if response := la.buildNoChangeResponse(lead, targetService, meaningfulNotes, existingAnalysis, hasExisting, force); response != nil {
		return response, nil
	}

	serviceContextList, err := la.buildServiceContextString(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to build service context: %w", err)
	}

	userMessage := la.buildUserMessage(lead, targetService, meaningfulNotes, serviceContextList)
	userID := "advisor-" + leadID.String() + "-" + targetService.ID.String()
	sessionID := uuid.New().String()

	cleanupSession, err := la.createSession(ctx, userID, sessionID, leadID)
	if err != nil {
		return nil, err
	}
	defer cleanupSession()

	// Reset tool tracker before run
	la.toolTracker.reset()

	output, err := la.runAdvisor(ctx, userID, sessionID, userMessage)
	if err != nil {
		return nil, err
	}

	log.Printf("Lead advisor finished for lead %s. Output: %s", leadID, output)

	// Check if SaveAnalysis was called using the tracker
	if la.toolTracker.wasSaveAnalysisCalled() {
		log.Printf("SaveAnalysis was called for lead %s service %s, fetching new analysis", leadID, targetService.ID)
		newAnalysis, err := la.repo.GetLatestAIAnalysis(ctx, targetService.ID, tenantID)
		if err != nil {
			return &AnalyzeResponse{
				Status:  "error",
				Message: "SaveAnalysis was called but failed to save. Please try again.",
			}, nil
		}
		result := la.analysisToResult(newAnalysis)
		return &AnalyzeResponse{
			Status:   "created",
			Message:  "New analysis generated",
			Analysis: result,
		}, nil
	}

	// SaveAnalysis was NOT called - retry with Moonshot's recommended prompt
	log.Printf("WARNING: AI did not call SaveAnalysis for lead %s. Retrying with tool selection prompt.", leadID)

	retryMessage := la.buildRetryMessage()
	retrySessionID := uuid.New().String()
	cleanupRetry, err := la.createSession(ctx, userID, retrySessionID, leadID)
	if err != nil {
		return nil, err
	}
	defer cleanupRetry()

	// Reset tracker for retry
	la.toolTracker.reset()

	retryOutput, err := la.runAdvisor(ctx, userID, retrySessionID, retryMessage)
	if err != nil {
		return nil, err
	}
	log.Printf("Lead advisor retry finished for lead %s. Output: %s", leadID, retryOutput)

	// Check if retry succeeded
	if la.toolTracker.wasSaveAnalysisCalled() {
		log.Printf("SaveAnalysis was called on retry for lead %s service %s", leadID, targetService.ID)
		newAnalysis, err := la.repo.GetLatestAIAnalysis(ctx, targetService.ID, tenantID)
		if err != nil {
			return &AnalyzeResponse{
				Status:  "error",
				Message: "SaveAnalysis was called but failed to save. Please try again.",
			}, nil
		}
		result := la.analysisToResult(newAnalysis)
		return &AnalyzeResponse{
			Status:   "created",
			Message:  "New analysis generated",
			Analysis: result,
		}, nil
	}

	// Retry also failed
	log.Printf("ERROR: AI did not call SaveAnalysis for lead %s service %s even after retry.", leadID, targetService.ID)
	return &AnalyzeResponse{
		Status:  "error",
		Message: "AI kon geen analyse opslaan. Probeer het opnieuw.",
	}, nil
}

func (la *LeadAdvisor) ensureInitialized() error {
	if la.runner == nil {
		return fmt.Errorf("lead advisor runner is not initialized")
	}
	if la.sessionService == nil {
		return fmt.Errorf("lead advisor session service is not initialized")
	}
	return nil
}

func (la *LeadAdvisor) fetchLeadAndServiceNotes(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, tenantID uuid.UUID) (repository.Lead, *repository.LeadService, []repository.LeadNote, error) {
	lead, err := la.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return repository.Lead{}, nil, nil, fmt.Errorf("failed to get lead: %w", err)
	}

	// Determine target service
	var targetService *repository.LeadService
	if serviceID != nil {
		// Fetch specific service
		service, err := la.repo.GetLeadServiceByID(ctx, *serviceID, tenantID)
		if err != nil && err != repository.ErrServiceNotFound {
			log.Printf("failed to fetch service %s: %v", *serviceID, err)
		}
		if err == nil {
			targetService = &service
		}
	} else {
		// Fetch current service (most recent non-terminal service)
		currentService, err := la.repo.GetCurrentLeadService(ctx, leadID, tenantID)
		if err != nil && err != repository.ErrServiceNotFound {
			log.Printf("failed to fetch current service for lead %s: %v", leadID, err)
		}
		if err == nil {
			targetService = &currentService
		}
	}

	notes, err := la.repo.ListLeadNotes(ctx, leadID, tenantID)
	if err != nil {
		log.Printf("failed to fetch notes for lead %s: %v", leadID, err)
		notes = nil
	}

	meaningfulNotes := filterMeaningfulNotes(notes)
	return lead, targetService, meaningfulNotes, nil
}

func (la *LeadAdvisor) getExistingAnalysis(ctx context.Context, serviceID uuid.UUID, tenantID uuid.UUID) (*repository.AIAnalysis, bool, error) {
	existingAnalysis, err := la.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil && err != repository.ErrNotFound {
		return nil, false, fmt.Errorf("failed to check existing analysis: %w", err)
	}
	if err == repository.ErrNotFound {
		return nil, false, nil
	}
	return &existingAnalysis, true, nil
}

func (la *LeadAdvisor) buildNoChangeResponse(lead repository.Lead, currentService *repository.LeadService, notes []repository.LeadNote, existingAnalysis *repository.AIAnalysis, hasExisting bool, force bool) *AnalyzeResponse {
	if hasExisting && !force && existingAnalysis != nil {
		if shouldSkipRegeneration(lead, currentService, notes, *existingAnalysis) {
			result := la.analysisToResult(*existingAnalysis)
			return &AnalyzeResponse{
				Status:   "no_change",
				Message:  "Geen nieuwe informatie sinds laatste analyse",
				Analysis: result,
			}
		}
	}

	return nil
}

func (la *LeadAdvisor) buildUserMessage(lead repository.Lead, currentService *repository.LeadService, notes []repository.LeadNote, serviceContextList string) *genai.Content {
	prompt := buildAnalysisPrompt(lead, currentService, notes, serviceContextList)
	return &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: prompt},
		},
	}
}

// buildServiceContextString builds the service list with intake guidelines for the prompt.
func (la *LeadAdvisor) buildServiceContextString(ctx context.Context, tenantID uuid.UUID) (string, error) {
	services, err := la.repo.ListActiveServiceTypes(ctx, tenantID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, s := range services {
		sb.WriteString(fmt.Sprintf("### %s\n", s.Name))
		if s.Description != nil && *s.Description != "" {
			sb.WriteString(fmt.Sprintf("Omschrijving: %s\n", *s.Description))
		}
		if s.IntakeGuidelines != nil && *s.IntakeGuidelines != "" {
			sb.WriteString(fmt.Sprintf("HARDE EISEN (Intake Requirements): %s\n", *s.IntakeGuidelines))
		} else {
			sb.WriteString("EISEN: Gebruik je algemene kennis over wat nodig is voor een offerte hiervoor.\n")
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// buildRetryMessage creates a retry message using Moonshot's recommended approach
// for forcing tool selection when tool_choice=required is not supported
func (la *LeadAdvisor) buildRetryMessage() *genai.Content {
	// Moonshot's recommended prompt for forcing tool selection
	// See: https://platform.moonshot.cn/docs/guide/migrating-from-openai-to-kimi
	return &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: "请选择一个工具（tool）来处理当前的问题。You MUST call the SaveAnalysis tool now with your complete analysis."},
		},
	}
}

func (la *LeadAdvisor) createSession(ctx context.Context, userID string, sessionID string, leadID uuid.UUID) (func(), error) {
	_, err := la.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   la.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	log.Printf("Created advisor session %s for lead %s", sessionID, leadID)

	cleanup := func() {
		deleteReq := &session.DeleteRequest{
			AppName:   la.appName,
			UserID:    userID,
			SessionID: sessionID,
		}
		if deleteErr := la.sessionService.Delete(ctx, deleteReq); deleteErr != nil {
			log.Printf("warning: failed to delete session %s: %v", sessionID, deleteErr)
		}
	}

	return cleanup, nil
}

func (la *LeadAdvisor) runAdvisor(ctx context.Context, userID string, sessionID string, userMessage *genai.Content) (string, error) {
	var output string
	runConfig := agent.RunConfig{
		StreamingMode: agent.StreamingModeNone,
	}

	for event, err := range la.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		if err != nil {
			return "", fmt.Errorf("advisor run failed: %w", err)
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				output += part.Text
			}
		}
	}

	return output, nil
}

// GetLatestAnalysis retrieves the most recent analysis for a lead service
func (la *LeadAdvisor) GetLatestAnalysis(ctx context.Context, serviceID uuid.UUID, tenantID uuid.UUID) (*AnalysisResult, error) {
	analysis, err := la.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		return nil, err
	}
	return la.analysisToResult(analysis), nil
}

// GetLatestOrDefault retrieves the most recent analysis, or returns a default "contact lead" response
func (la *LeadAdvisor) GetLatestOrDefault(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, tenantID uuid.UUID) (*AnalysisResult, bool, error) {
	analysis, err := la.GetLatestAnalysis(ctx, serviceID, tenantID)
	if err == nil {
		return analysis, true, nil
	}

	// If no analysis found, return a default response
	if err == repository.ErrNotFound {
		return getDefaultAnalysis(leadID, serviceID), false, nil
	}

	return nil, false, err
}

// ListAnalyses retrieves all analyses for a lead service
func (la *LeadAdvisor) ListAnalyses(ctx context.Context, serviceID uuid.UUID, tenantID uuid.UUID) ([]AnalysisResult, error) {
	analyses, err := la.repo.ListAIAnalyses(ctx, serviceID, tenantID)
	if err != nil {
		return nil, err
	}

	results := make([]AnalysisResult, len(analyses))
	for i, analysis := range analyses {
		results[i] = *la.analysisToResult(analysis)
	}

	return results, nil
}

// GetDraftedEmail retrieves a drafted email by ID
func (la *LeadAdvisor) GetDraftedEmail(draftID uuid.UUID) (*EmailDraft, bool) {
	draft, ok := la.draftedEmails[draftID]
	if !ok {
		return nil, false
	}
	return &draft, true
}

// GetDraftedEmailsForLead retrieves all drafted emails for a specific lead
func (la *LeadAdvisor) GetDraftedEmailsForLead(leadID uuid.UUID) []EmailDraft {
	var drafts []EmailDraft
	for _, draft := range la.draftedEmails {
		if draft.LeadID == leadID {
			drafts = append(drafts, draft)
		}
	}
	return drafts
}

// DeleteDraftedEmail removes a drafted email (e.g., after it's been sent)
func (la *LeadAdvisor) DeleteDraftedEmail(draftID uuid.UUID) {
	delete(la.draftedEmails, draftID)
}

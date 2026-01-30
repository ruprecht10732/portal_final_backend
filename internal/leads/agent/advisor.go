package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
)

// LeadAdvisor provides AI-powered lead analysis using the ADK framework
type LeadAdvisor struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	// Store drafted emails temporarily (in production, this would be persisted to DB)
	draftedEmails map[uuid.UUID]EmailDraft
}

// NewLeadAdvisor builds the AI Advisor agent with Kimi model
// Returns an error if the agent or runner cannot be initialized
func NewLeadAdvisor(apiKey string, repo repository.LeadsRepository) (*LeadAdvisor, error) {
	kimi := moonshot.NewModel(moonshot.Config{APIKey: apiKey})

	advisor := &LeadAdvisor{
		repo:          repo,
		draftedEmails: make(map[uuid.UUID]EmailDraft),
		appName:       "lead_advisor",
	}

	// Create tool dependencies
	toolDeps := &ToolDependencies{
		Repo:          repo,
		DraftedEmails: advisor.draftedEmails,
	}

	// Build all tools
	tools, err := buildTools(toolDeps)
	if err != nil {
		log.Printf("warning: some tools failed to initialize: %v", err)
		// Continue with partial tools rather than failing completely
	}

	// Create the ADK agent
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "LeadAdvisor",
		Model:       kimi,
		Description: "Expert AI Sales Advisor for home services marketplace (plumbing, HVAC, electrical, carpentry) that analyzes leads and provides personalized, actionable sales guidance.",
		Instruction: getSystemPrompt(),
		Tools:       tools,
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
func (la *LeadAdvisor) Analyze(ctx context.Context, leadID uuid.UUID) error {
	_, err := la.AnalyzeAndReturn(ctx, leadID, false)
	return err
}

// AnalyzeAndReturn runs the AI advisor and returns the result directly
// If force is true, it will regenerate even if no changes are detected
func (la *LeadAdvisor) AnalyzeAndReturn(ctx context.Context, leadID uuid.UUID, force bool) (*AnalyzeResponse, error) {
	if err := la.ensureInitialized(); err != nil {
		return nil, err
	}

	lead, meaningfulNotes, err := la.fetchLeadAndMeaningfulNotes(ctx, leadID)
	if err != nil {
		return nil, err
	}

	existingAnalysis, hasExisting, err := la.getExistingAnalysis(ctx, leadID)
	if err != nil {
		return nil, err
	}

	if response := la.buildNoChangeResponse(lead, meaningfulNotes, existingAnalysis, hasExisting, force); response != nil {
		return response, nil
	}

	userMessage := la.buildUserMessage(lead, meaningfulNotes)
	userID := "advisor-" + leadID.String()
	sessionID := uuid.New().String()

	cleanupSession, err := la.createSession(ctx, userID, sessionID, leadID)
	if err != nil {
		return nil, err
	}
	defer cleanupSession()

	analysisStartTime := time.Now()
	output, err := la.runAdvisor(ctx, userID, sessionID, userMessage)
	if err != nil {
		return nil, err
	}

	log.Printf("Lead advisor finished for lead %s. Output: %s", leadID, output)

	newAnalysis, errorResponse, err := la.getLatestAnalysisOrErrorResponse(ctx, leadID)
	if err != nil {
		return nil, err
	}
	if errorResponse != nil {
		return errorResponse, nil
	}

	return la.buildPostRunResponse(*newAnalysis, analysisStartTime, leadID), nil
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

func (la *LeadAdvisor) fetchLeadAndMeaningfulNotes(ctx context.Context, leadID uuid.UUID) (repository.Lead, []repository.LeadNote, error) {
	lead, err := la.repo.GetByID(ctx, leadID)
	if err != nil {
		return repository.Lead{}, nil, fmt.Errorf("failed to get lead: %w", err)
	}

	notes, err := la.repo.ListLeadNotes(ctx, leadID)
	if err != nil {
		log.Printf("failed to fetch notes for lead %s: %v", leadID, err)
		notes = nil
	}

	meaningfulNotes := filterMeaningfulNotes(notes)
	return lead, meaningfulNotes, nil
}

func (la *LeadAdvisor) getExistingAnalysis(ctx context.Context, leadID uuid.UUID) (*repository.AIAnalysis, bool, error) {
	existingAnalysis, err := la.repo.GetLatestAIAnalysis(ctx, leadID)
	if err != nil && err != repository.ErrNotFound {
		return nil, false, fmt.Errorf("failed to check existing analysis: %w", err)
	}
	if err == repository.ErrNotFound {
		return nil, false, nil
	}
	return &existingAnalysis, true, nil
}

func (la *LeadAdvisor) buildNoChangeResponse(lead repository.Lead, notes []repository.LeadNote, existingAnalysis *repository.AIAnalysis, hasExisting bool, force bool) *AnalyzeResponse {
	if hasExisting && !force && existingAnalysis != nil {
		if shouldSkipRegeneration(lead, notes, *existingAnalysis) {
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

func (la *LeadAdvisor) buildUserMessage(lead repository.Lead, notes []repository.LeadNote) *genai.Content {
	prompt := buildAnalysisPrompt(lead, notes)
	return &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: prompt},
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

func (la *LeadAdvisor) getLatestAnalysisOrErrorResponse(ctx context.Context, leadID uuid.UUID) (*repository.AIAnalysis, *AnalyzeResponse, error) {
	newAnalysis, err := la.repo.GetLatestAIAnalysis(ctx, leadID)
	if err != nil {
		return nil, &AnalyzeResponse{
			Status:  "error",
			Message: "AI did not generate an analysis. Please try again.",
		}, nil
	}
	return &newAnalysis, nil, nil
}

func (la *LeadAdvisor) buildPostRunResponse(newAnalysis repository.AIAnalysis, analysisStartTime time.Time, leadID uuid.UUID) *AnalyzeResponse {
	if newAnalysis.CreatedAt.Before(analysisStartTime) {
		log.Printf("WARNING: AI did not call SaveAnalysis tool for lead %s. Returning old analysis.", leadID)
		result := la.analysisToResult(newAnalysis)
		return &AnalyzeResponse{
			Status:   "no_change",
			Message:  "AI kon geen nieuwe analyse genereren. Bestaande analyse getoond.",
			Analysis: result,
		}
	}

	result := la.analysisToResult(newAnalysis)
	return &AnalyzeResponse{
		Status:   "created",
		Message:  "New analysis generated",
		Analysis: result,
	}
}

// GetLatestAnalysis retrieves the most recent analysis for a lead
func (la *LeadAdvisor) GetLatestAnalysis(ctx context.Context, leadID uuid.UUID) (*AnalysisResult, error) {
	analysis, err := la.repo.GetLatestAIAnalysis(ctx, leadID)
	if err != nil {
		return nil, err
	}
	return la.analysisToResult(analysis), nil
}

// GetLatestOrDefault retrieves the most recent analysis, or returns a default "contact lead" response
func (la *LeadAdvisor) GetLatestOrDefault(ctx context.Context, leadID uuid.UUID) (*AnalysisResult, bool, error) {
	analysis, err := la.GetLatestAnalysis(ctx, leadID)
	if err == nil {
		return analysis, true, nil
	}

	// If no analysis found, return a default response
	if err == repository.ErrNotFound {
		return getDefaultAnalysis(leadID), false, nil
	}

	return nil, false, err
}

// ListAnalyses retrieves all analyses for a lead
func (la *LeadAdvisor) ListAnalyses(ctx context.Context, leadID uuid.UUID) ([]AnalysisResult, error) {
	analyses, err := la.repo.ListAIAnalyses(ctx, leadID)
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

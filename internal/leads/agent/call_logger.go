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
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
	"portal_final_backend/platform/apperr"
)

// Error messages
const (
	errMsgMissingContext       = "Missing context"
	errMsgBookingNotConfigured = "Appointment booking not configured"
	errBookerNotConfigured     = "booker not configured"
)

// Date/time layout used for short human-readable timestamps
const dateTimeShortLayout = "2006-01-02 15:04"

// Sentinel errors
var (
	errMissingContext = errors.New("missing context")
)

// CallLogResult represents the result of processing a call summary
type CallLogResult struct {
	NoteCreated                   bool       `json:"noteCreated"`
	NoteBody                      string     `json:"noteBody,omitempty"`
	AuthorEmail                   string     `json:"authorEmail,omitempty"`
	CallOutcome                   *string    `json:"callOutcome,omitempty"`
	StatusUpdated                 *string    `json:"statusUpdated,omitempty"`
	PipelineStageUpdated          *string    `json:"pipelineStageUpdated,omitempty"`
	AppointmentBooked             *time.Time `json:"appointmentBooked,omitempty"`
	AppointmentRescheduled        *time.Time `json:"appointmentRescheduled,omitempty"`
	AppointmentRescheduleFallback bool       `json:"appointmentRescheduleFallback,omitempty"`
	AppointmentCancelled          bool       `json:"appointmentCancelled,omitempty"`
	Message                       string     `json:"message"`
}

// CallLogger processes post-call summaries into structured actions
type CallLogger struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	booker         ports.AppointmentBooker
	toolDeps       *CallLoggerToolDeps
	runMu          sync.Mutex
}

// SetAppointmentBooker sets the appointment booker after initialization.
// This is needed to break circular dependencies during module initialization.
func (c *CallLogger) SetAppointmentBooker(booker ports.AppointmentBooker) {
	c.booker = booker
	c.toolDeps.Booker = booker
}

// CallLoggerToolDeps contains the dependencies needed by CallLogger tools
type CallLoggerToolDeps struct {
	Repo     repository.LeadsRepository
	Booker   ports.AppointmentBooker
	EventBus events.Bus
	mu       sync.RWMutex

	// Context for the current run
	tenantID  *uuid.UUID
	userID    *uuid.UUID
	leadID    *uuid.UUID
	serviceID *uuid.UUID

	// Track results during the run
	result CallLogResult

	// Drafted note content (persisted after the run finishes)
	noteDraftBody string
	noteDrafted   bool
}

func (d *CallLoggerToolDeps) SetContext(tenantID, userID, leadID, serviceID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenantID = &tenantID
	d.userID = &userID
	d.leadID = &leadID
	d.serviceID = &serviceID
	d.result = CallLogResult{} // Reset result
	d.noteDraftBody = ""
	d.noteDrafted = false
}

func (d *CallLoggerToolDeps) GetContext() (tenantID, userID, leadID, serviceID uuid.UUID, ok bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tenantID == nil || d.userID == nil || d.leadID == nil || d.serviceID == nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, false
	}
	return *d.tenantID, *d.userID, *d.leadID, *d.serviceID, true
}

func (d *CallLoggerToolDeps) MarkNoteCreated() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.NoteCreated = true
}

func (d *CallLoggerToolDeps) SetNoteDraft(body string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.noteDraftBody = body
	d.noteDrafted = true
}

func (d *CallLoggerToolDeps) GetNoteDraft() (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.noteDraftBody, d.noteDrafted
}

func (d *CallLoggerToolDeps) SetNoteDetails(body, authorEmail string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.NoteBody = body
	d.result.AuthorEmail = authorEmail
}

func (d *CallLoggerToolDeps) MarkStatusUpdated(status string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.StatusUpdated = &status
}

func (d *CallLoggerToolDeps) MarkAppointmentBooked(startTime time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.AppointmentBooked = &startTime
}

func (d *CallLoggerToolDeps) MarkAppointmentRescheduled(startTime time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.AppointmentRescheduled = &startTime
}

func (d *CallLoggerToolDeps) MarkRescheduleFallback() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.AppointmentRescheduleFallback = true
}

func (d *CallLoggerToolDeps) MarkAppointmentCancelled() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.AppointmentCancelled = true
}

func (d *CallLoggerToolDeps) SetCallOutcome(outcome string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.CallOutcome = &outcome
}

func (d *CallLoggerToolDeps) MarkPipelineStageUpdated(stage string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.PipelineStageUpdated = &stage
}

func (d *CallLoggerToolDeps) GetResult() CallLogResult {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.result
}

// NewCallLogger creates a new CallLogger agent
func NewCallLogger(apiKey string, repo repository.LeadsRepository, booker ports.AppointmentBooker, eventBus events.Bus) (*CallLogger, error) {
	// Use kimi-k2.5 with thinking disabled for reliable tool calling
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          apiKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	logger := &CallLogger{
		repo:           repo,
		booker:         booker,
		appName:        "call_logger",
		sessionService: session.InMemoryService(),
		toolDeps: &CallLoggerToolDeps{
			Repo:     repo,
			Booker:   booker,
			EventBus: eventBus,
		},
	}

	// Build tools
	tools, err := buildCallLoggerTools(logger.toolDeps)
	if err != nil {
		return nil, fmt.Errorf("failed to build call logger tools: %w", err)
	}

	// Create the ADK agent
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "CallLogger",
		Model:       kimi,
		Description: "Post-call processing assistant that converts natural language call summaries into structured database updates (Notes, Status changes, Appointments).",
		Instruction: getCallLoggerSystemPrompt(),
		Tools:       tools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create call logger agent: %w", err)
	}

	// Create the runner
	r, err := runner.New(runner.Config{
		AppName:        logger.appName,
		Agent:          adkAgent,
		SessionService: logger.sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create call logger runner: %w", err)
	}

	logger.agent = adkAgent
	logger.runner = r

	return logger, nil
}

// resolveExistingAppointment checks whether the lead already has a booked visit
// and returns a human-readable timestamp or "None".
func (c *CallLogger) resolveExistingAppointment(ctx context.Context, tenantID, serviceID, userID uuid.UUID) string {
	if c.booker == nil {
		return "None"
	}

	visit, err := c.booker.GetLeadVisitByService(ctx, tenantID, serviceID, userID)
	if err == nil && visit != nil {
		return visit.StartTime.Format(dateTimeShortLayout)
	}
	if err != nil && !apperr.Is(err, apperr.KindNotFound) {
		log.Printf("CallLogger warning: failed to check existing appointment: %v", err)
	}
	return "None"
}

// executeAgentRun creates an ephemeral session, runs the agent, and returns the
// concatenated text output.
func (c *CallLogger) executeAgentRun(ctx context.Context, userIDStr, sessionID, promptText string) (string, error) {
	_, err := c.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   c.appName,
		UserID:    userIDStr,
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer func() {
		if deleteErr := c.sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   c.appName,
			UserID:    userIDStr,
			SessionID: sessionID,
		}); deleteErr != nil {
			log.Printf("warning: failed to delete call logger session: %v", deleteErr)
		}
	}()

	userMessage := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: promptText},
		},
	}

	runConfig := agent.RunConfig{
		StreamingMode: agent.StreamingModeNone,
	}

	var outputText string
	for event, err := range c.runner.Run(ctx, userIDStr, sessionID, userMessage, runConfig) {
		if err != nil {
			return "", fmt.Errorf("call logger run failed: %w", err)
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				outputText += part.Text
			}
		}
	}

	return outputText, nil
}

// ProcessSummary is the main entry point for processing a call summary
func (c *CallLogger) ProcessSummary(ctx context.Context, leadID, serviceID, userID, tenantID uuid.UUID, summary string) (*CallLogResult, error) {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	// Set context for tools
	c.toolDeps.SetContext(tenantID, userID, leadID, serviceID)

	existingAppointment := c.resolveExistingAppointment(ctx, tenantID, serviceID, userID)

	// Construct the prompt with context
	promptText := fmt.Sprintf(`Analysis Context:
- Current Time: %s
- Lead ID: %s
- Service ID: %s
- Agent User ID: %s
- Existing Appointment: %s

The agent provided this post-call summary:
"%s"

Task:
1. Analyze the summary to determine the call outcome and any appointment changes.
2. ALWAYS save a clean, professional Dutch call note.
	- Draft the note, then run it through 'NormalizeCallNote'.
	- Save the normalized version using 'SaveNote'.
	- Write a short, readable note (no raw input block).
	- If the input is sparse, infer structure (do NOT invent facts).
	- Use 24-hour time format (e.g., 09:00).
	- Preferred structure:
	  "Afspraak: ..."
	  "Werkzaamheden: ..."
	  "Materiaal: ..."
	  "Locatie: ..." (if provided)
	  "Vragen: ..." (if any)
3. If an appointment was scheduled (e.g., "booked next tuesday at 9", "scheduled for friday 2pm"):
   - Calculate the exact date based on Current Time
   - Use 'ScheduleVisit' to book the appointment
   - Assume 1 hour duration unless specified otherwise
4. If an existing appointment must be rescheduled and a new time is provided:
   - Use 'RescheduleVisit' with the new start/end time
	- Only reschedule if Existing Appointment is not "None"; otherwise schedule a new appointment and write "Nieuwe afspraak ingepland"
5. If the appointment is cancelled:
   - Use 'CancelVisit'
6. Set a call outcome using 'SetCallOutcome' (short label, e.g., Scheduled, Attempted_Contact, Bad_Lead, Needs_Rescheduling).
7. Update the status using 'UpdateStatus' if the outcome implies a status change:
   - "booked", "scheduled", "appointment set" → Scheduled
   - "not interested", "no need", "declined" → Bad_Lead
   - "voicemail", "no answer", "callback" → Attempted_Contact
   - "completed survey", "finished inspection" → Surveyed
   - "needs to reschedule", "postponed" → Needs_Rescheduling
8. Update the pipeline stage with 'UpdatePipelineStage' if the summary explicitly indicates a stage change.

Execute the appropriate tools now.`,
		time.Now().Format(time.RFC3339),
		leadID.String(),
		serviceID.String(),
		userID.String(),
		existingAppointment,
		summary,
	)

	sessionID := uuid.New().String()
	userIDStr := userID.String()

	outputText, err := c.executeAgentRun(ctx, userIDStr, sessionID, promptText)
	if err != nil {
		return nil, err
	}

	log.Printf("CallLogger finished. Output: %s", outputText)

	if err := c.persistDraftedNote(ctx); err != nil {
		return nil, err
	}

	// Get the result and build message
	result := c.toolDeps.GetResult()
	result.Message = buildResultMessage(result)

	return &result, nil
}

func (c *CallLogger) persistDraftedNote(ctx context.Context) error {
	body, ok := c.toolDeps.GetNoteDraft()
	if !ok {
		return nil
	}

	result := c.toolDeps.GetResult()
	finalBody := body
	if result.AppointmentRescheduleFallback && result.AppointmentBooked != nil {
		finalBody = appendRescheduleFallbackNote(finalBody, *result.AppointmentBooked)
	}

	tenantID, userID, leadID, _, ok := c.toolDeps.GetContext()
	if !ok {
		return errMissingContext
	}

	note, err := c.repo.CreateLeadNote(ctx, repository.CreateLeadNoteParams{
		LeadID:         leadID,
		OrganizationID: tenantID,
		AuthorID:       userID,
		Type:           "call",
		Body:           finalBody,
	})
	if err != nil {
		return err
	}

	c.toolDeps.MarkNoteCreated()
	c.toolDeps.SetNoteDetails(finalBody, note.AuthorEmail)
	return nil
}

// buildResultMessage constructs a human-readable message from the call log result
func buildResultMessage(result CallLogResult) string {
	var messages []string
	if result.NoteCreated {
		messages = append(messages, "Note saved")
	}
	if result.CallOutcome != nil {
		messages = append(messages, fmt.Sprintf("Call outcome set to %s", *result.CallOutcome))
	}
	if result.StatusUpdated != nil {
		messages = append(messages, fmt.Sprintf("Status updated to %s", *result.StatusUpdated))
	}
	if result.PipelineStageUpdated != nil {
		messages = append(messages, fmt.Sprintf("Pipeline stage updated to %s", *result.PipelineStageUpdated))
	}
	if result.AppointmentBooked != nil {
		messages = append(messages, fmt.Sprintf("Appointment booked for %s", result.AppointmentBooked.Format(dateTimeShortLayout)))
	}
	if result.AppointmentRescheduled != nil {
		messages = append(messages, fmt.Sprintf("Appointment rescheduled for %s", result.AppointmentRescheduled.Format(dateTimeShortLayout)))
	}
	if result.AppointmentCancelled {
		messages = append(messages, "Appointment cancelled")
	}
	if len(messages) == 0 {
		return "No actions taken"
	}
	return strings.Join(messages, ". ")
}

func getCallLoggerSystemPrompt() string {
	return `You are a Post-Call Processing Assistant for a home services sales team.

Your job is to read a rough summary of a sales/qualification call and execute the necessary database updates using the available tools.

IMPORTANT RULES:
1. Draft a clean, professional Dutch note and pass it through NormalizeCallNote.
	- Do NOT include the raw input text.
	- Structure when possible (Afspraak, Werkzaamheden, Materiaal, Locatie, Vragen).
	- Do NOT invent details that were not stated.
2. ALWAYS call SaveNote with the normalized note.
3. Parse dates relative to the Current Time provided in the context:
   - "next Tuesday" = the coming Tuesday from Current Time
   - "tomorrow" = Current Time + 1 day
   - "this Friday" = the Friday of the current week
   - "on the 15th" = the 15th of the current or next month
4. Default appointment duration is 1 hour unless explicitly stated.
5. Set a call outcome using SetCallOutcome (short label like Scheduled, Attempted_Contact, Bad_Lead, Needs_Rescheduling).
6. If the context says Existing Appointment is None, do NOT say "verplaatst". Schedule a new appointment and write "Nieuwe afspraak ingepland" in the note.
7. Status mapping:
   - Appointment scheduled/booked → "Scheduled"
   - No answer/voicemail/try again → "Attempted_Contact"  
   - Not interested/declined/bad fit → "Bad_Lead"
   - Survey/inspection completed → "Surveyed"
   - Needs to reschedule/postponed → "Needs_Rescheduling"
8. When booking RAC_appointments, also update status to "Scheduled".
9. Use 24-hour time format (e.g., 09:00, 14:30).
10. Do NOT make up information. Only act on what is explicitly stated in the summary.
11. Email confirmation behavior for RAC_appointments:
   - By default, sendConfirmationEmail should be TRUE (send email)
   - Only set sendConfirmationEmail to FALSE if the call notes explicitly mention:
     - "no email", "don't send email", "skip email", "no confirmation email"
     - "they'll confirm differently", "will contact them separately"
   - If unclear, default to TRUE to send confirmation

Available tools:
- NormalizeCallNote: Cleans and formats a call note draft (use before SaveNote)
- SaveNote: Saves the call note (ALWAYS use this)
- SetCallOutcome: Stores a short outcome label for the call
- UpdateStatus: Updates the lead service status
- UpdatePipelineStage: Updates the pipeline stage when explicitly indicated
- ScheduleVisit: Books an inspection/visit appointment (includes sendConfirmationEmail option)
- RescheduleVisit: Reschedules an existing appointment
- CancelVisit: Cancels the existing appointment
`
}

// Tool input/output types for CallLogger

type SaveNoteInput struct {
	Body string `json:"body"` // The note text to save
}

type SaveNoteOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type NormalizeCallNoteInput struct {
	Body string `json:"body"` // Draft note to normalize
}

type NormalizeCallNoteOutput struct {
	Body string `json:"body"`
}

type SetCallOutcomeInput struct {
	Outcome string `json:"outcome"` // Short outcome label
	Notes   string `json:"notes,omitempty"`
}

type SetCallOutcomeOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type UpdateStatusInput struct {
	Status string `json:"status"` // New status: New, Attempted_Contact, Scheduled, Surveyed, Bad_Lead, Needs_Rescheduling, Closed
}

type UpdateStatusOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ScheduleVisitInput struct {
	StartTime             string `json:"startTime"`             // ISO 8601 format (e.g., "2026-02-10T09:00:00Z")
	EndTime               string `json:"endTime"`               // ISO 8601 format (e.g., "2026-02-10T10:00:00Z")
	Title                 string `json:"title"`                 // Appointment title (e.g., "Inspection visit")
	SendConfirmationEmail *bool  `json:"sendConfirmationEmail"` // Whether to send email to lead (default: true)
}

type ScheduleVisitOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type RescheduleVisitInput struct {
	StartTime   string `json:"startTime"` // ISO 8601 format (e.g., "2026-02-10T09:00:00Z")
	EndTime     string `json:"endTime"`   // ISO 8601 format (e.g., "2026-02-10T10:00:00Z")
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type RescheduleVisitOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type CancelVisitInput struct {
	Reason string `json:"reason,omitempty"`
}

type CancelVisitOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func buildCallLoggerTools(deps *CallLoggerToolDeps) ([]tool.Tool, error) {
	normalizeCallNoteTool, err := buildNormalizeCallNoteTool(deps)
	if err != nil {
		return nil, err
	}

	saveNoteTool, err := buildSaveNoteTool(deps)
	if err != nil {
		return nil, err
	}

	setCallOutcomeTool, err := buildSetCallOutcomeTool(deps)
	if err != nil {
		return nil, err
	}

	updateStatusTool, err := buildUpdateStatusTool(deps)
	if err != nil {
		return nil, err
	}

	updatePipelineStageTool, err := buildCallLoggerUpdatePipelineStageTool(deps)
	if err != nil {
		return nil, err
	}

	scheduleVisitTool, err := buildScheduleVisitTool(deps)
	if err != nil {
		return nil, err
	}

	rescheduleVisitTool, err := buildRescheduleVisitTool(deps)
	if err != nil {
		return nil, err
	}

	cancelVisitTool, err := buildCancelVisitTool(deps)
	if err != nil {
		return nil, err
	}

	return []tool.Tool{
		normalizeCallNoteTool,
		saveNoteTool,
		setCallOutcomeTool,
		updateStatusTool,
		updatePipelineStageTool,
		scheduleVisitTool,
		rescheduleVisitTool,
		cancelVisitTool,
	}, nil
}

func buildSaveNoteTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SaveNote",
		Description: "Saves the call summary as a note on the lead. ALWAYS call this tool to record the call outcome.",
	}, func(ctx tool.Context, input SaveNoteInput) (SaveNoteOutput, error) {
		if _, _, _, _, ok := deps.GetContext(); !ok {
			return SaveNoteOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}

		deps.SetNoteDraft(input.Body)
		return SaveNoteOutput{Success: true, Message: "Note drafted"}, nil
	})
}

func normalizeCallNoteBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	cleaned := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		plain := strings.TrimSpace(line)
		lower := strings.ToLower(plain)
		if strings.Contains(lower, "originele input") {
			continue
		}
		if plain == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			cleaned = append(cleaned, "")
			continue
		}
		lastBlank = false
		cleaned = append(cleaned, strings.TrimRight(line, " \t"))
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func appendRescheduleFallbackNote(body string, startTime time.Time) string {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "geen bestaande afspraak") || strings.Contains(lower, "nieuwe afspraak") {
		return body
	}

	correction := fmt.Sprintf("Let op: er was geen bestaande afspraak. Nieuwe afspraak ingepland op %s.", startTime.Format(dateTimeShortLayout))
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return correction
	}
	return strings.TrimRight(body, "\n") + "\n\n" + correction
}

func buildNormalizeCallNoteTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	_ = deps
	return functiontool.New(functiontool.Config{
		Name:        "NormalizeCallNote",
		Description: "Cleans and normalizes a drafted call note. Use before SaveNote.",
	}, func(ctx tool.Context, input NormalizeCallNoteInput) (NormalizeCallNoteOutput, error) {
		normalized := normalizeCallNoteBody(input.Body)
		return NormalizeCallNoteOutput{Body: normalized}, nil
	})
}

func buildSetCallOutcomeTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SetCallOutcome",
		Description: "Stores a short call outcome label on the timeline.",
	}, func(ctx tool.Context, input SetCallOutcomeInput) (SetCallOutcomeOutput, error) {
		tenantID, userID, leadID, serviceID, ok := deps.GetContext()
		if !ok {
			return SetCallOutcomeOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}

		outcome := strings.TrimSpace(input.Outcome)
		if outcome == "" {
			return SetCallOutcomeOutput{Success: false, Message: "Missing outcome"}, fmt.Errorf("missing outcome")
		}

		actorName := userID.String()
		summary := outcome
		if strings.TrimSpace(input.Notes) != "" {
			summary = fmt.Sprintf("%s - %s", outcome, strings.TrimSpace(input.Notes))
		}

		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      "User",
			ActorName:      actorName,
			EventType:      "call_outcome",
			Title:          "Call outcome",
			Summary:        &summary,
			Metadata: map[string]any{
				"outcome": outcome,
				"notes":   strings.TrimSpace(input.Notes),
			},
		})

		deps.SetCallOutcome(outcome)
		return SetCallOutcomeOutput{Success: true, Message: "Call outcome set"}, nil
	})
}

// validLeadStatuses defines the allowed status values for RAC_leads
var validLeadStatuses = map[string]bool{
	"New":                true,
	"Attempted_Contact":  true,
	"Scheduled":          true,
	"Surveyed":           true,
	"Bad_Lead":           true,
	"Needs_Rescheduling": true,
	"Closed":             true,
}

func buildUpdateStatusTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "UpdateStatus",
		Description: "Updates the status of the lead service. Valid statuses: New, Attempted_Contact, Scheduled, Surveyed, Bad_Lead, Needs_Rescheduling, Closed",
	}, func(ctx tool.Context, input UpdateStatusInput) (UpdateStatusOutput, error) {
		tenantID, _, _, serviceID, ok := deps.GetContext()
		if !ok {
			return UpdateStatusOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}

		if !validLeadStatuses[input.Status] {
			return UpdateStatusOutput{Success: false, Message: "Invalid status"}, fmt.Errorf("invalid status: %s", input.Status)
		}

		_, err := deps.Repo.UpdateServiceStatus(context.Background(), serviceID, tenantID, input.Status)
		if err != nil {
			return UpdateStatusOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkStatusUpdated(input.Status)
		return UpdateStatusOutput{Success: true, Message: fmt.Sprintf("Status updated to %s", input.Status)}, nil
	})
}

func buildScheduleVisitTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "ScheduleVisit",
		Description: "Books an inspection/visit appointment for the lead. Provide start and end times in ISO 8601 format. Set sendConfirmationEmail to false if the call notes mention not sending email; otherwise it defaults to true.",
	}, func(ctx tool.Context, input ScheduleVisitInput) (ScheduleVisitOutput, error) {
		return executeScheduleVisit(deps, input)
	})
}

func buildRescheduleVisitTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "RescheduleVisit",
		Description: "Reschedules an existing lead visit appointment. Provide start and end times in ISO 8601 format.",
	}, func(ctx tool.Context, input RescheduleVisitInput) (RescheduleVisitOutput, error) {
		return executeRescheduleVisit(deps, input)
	})
}

func buildCancelVisitTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "CancelVisit",
		Description: "Cancels the existing lead visit appointment.",
	}, func(ctx tool.Context, input CancelVisitInput) (CancelVisitOutput, error) {
		return executeCancelVisit(deps, input)
	})
}

func executeScheduleVisit(deps *CallLoggerToolDeps, input ScheduleVisitInput) (ScheduleVisitOutput, error) {
	tenantID, userID, leadID, serviceID, ok := deps.GetContext()
	if !ok {
		return ScheduleVisitOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
	}

	if deps.Booker == nil {
		return ScheduleVisitOutput{Success: false, Message: errMsgBookingNotConfigured}, fmt.Errorf(errBookerNotConfigured)
	}

	startTime, err := time.Parse(time.RFC3339, input.StartTime)
	if err != nil {
		return ScheduleVisitOutput{Success: false, Message: "Invalid start time format"}, err
	}

	endTime, err := time.Parse(time.RFC3339, input.EndTime)
	if err != nil {
		return ScheduleVisitOutput{Success: false, Message: "Invalid end time format"}, err
	}

	title := input.Title
	if title == "" {
		title = "Lead visit"
	}

	// Default to sending confirmation email if not specified
	sendEmail := true
	if input.SendConfirmationEmail != nil {
		sendEmail = *input.SendConfirmationEmail
	}

	err = deps.Booker.BookLeadVisit(context.Background(), ports.BookVisitParams{
		TenantID:              tenantID,
		UserID:                userID,
		LeadID:                leadID,
		LeadServiceID:         serviceID,
		StartTime:             startTime,
		EndTime:               endTime,
		Title:                 title,
		Description:           "Scheduled via Call Logger",
		SendConfirmationEmail: sendEmail,
	})
	if err != nil {
		return ScheduleVisitOutput{Success: false, Message: err.Error()}, err
	}

	deps.MarkAppointmentBooked(startTime)
	return ScheduleVisitOutput{Success: true, Message: "Appointment booked"}, nil
}

func buildCallLoggerUpdatePipelineStageTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "UpdatePipelineStage",
		Description: "Updates the pipeline stage for the lead service and records a timeline event.",
	}, func(ctx tool.Context, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
		if !validPipelineStages[input.Stage] {
			return UpdatePipelineStageOutput{Success: false, Message: "Invalid pipeline stage"}, fmt.Errorf("invalid pipeline stage: %s", input.Stage)
		}

		tenantID, userID, leadID, serviceID, ok := deps.GetContext()
		if !ok {
			return UpdatePipelineStageOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
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

		reason := strings.TrimSpace(input.Reason)
		var summary *string
		if reason != "" {
			summary = &reason
		}

		actorName := userID.String()
		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      "User",
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

		deps.MarkPipelineStageUpdated(input.Stage)
		return UpdatePipelineStageOutput{Success: true, Message: "Pipeline stage updated"}, nil
	})
}

func executeRescheduleVisit(deps *CallLoggerToolDeps, input RescheduleVisitInput) (RescheduleVisitOutput, error) {
	tenantID, userID, _, serviceID, ok := deps.GetContext()
	if !ok {
		return RescheduleVisitOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
	}

	if deps.Booker == nil {
		return RescheduleVisitOutput{Success: false, Message: errMsgBookingNotConfigured}, fmt.Errorf(errBookerNotConfigured)
	}

	if _, err := deps.Booker.GetLeadVisitByService(context.Background(), tenantID, serviceID, userID); err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			deps.MarkRescheduleFallback()
			scheduled, scheduleErr := executeScheduleVisit(deps, ScheduleVisitInput{
				StartTime: input.StartTime,
				EndTime:   input.EndTime,
				Title:     input.Title,
			})
			if scheduleErr != nil {
				return RescheduleVisitOutput{Success: false, Message: scheduled.Message}, scheduleErr
			}
			return RescheduleVisitOutput{Success: true, Message: "Appointment scheduled"}, nil
		}
		return RescheduleVisitOutput{Success: false, Message: err.Error()}, err
	}

	startTime, err := time.Parse(time.RFC3339, input.StartTime)
	if err != nil {
		return RescheduleVisitOutput{Success: false, Message: "Invalid start time format"}, err
	}

	endTime, err := time.Parse(time.RFC3339, input.EndTime)
	if err != nil {
		return RescheduleVisitOutput{Success: false, Message: "Invalid end time format"}, err
	}

	var title *string
	if strings.TrimSpace(input.Title) != "" {
		value := strings.TrimSpace(input.Title)
		title = &value
	}

	var description *string
	if strings.TrimSpace(input.Description) != "" {
		value := strings.TrimSpace(input.Description)
		description = &value
	}

	err = deps.Booker.RescheduleLeadVisit(context.Background(), ports.RescheduleVisitParams{
		TenantID:      tenantID,
		UserID:        userID,
		LeadServiceID: serviceID,
		StartTime:     startTime,
		EndTime:       endTime,
		Title:         title,
		Description:   description,
	})
	if err != nil {
		return RescheduleVisitOutput{Success: false, Message: err.Error()}, err
	}

	deps.MarkAppointmentRescheduled(startTime)
	return RescheduleVisitOutput{Success: true, Message: "Appointment rescheduled"}, nil
}

func executeCancelVisit(deps *CallLoggerToolDeps, input CancelVisitInput) (CancelVisitOutput, error) {
	_ = input
	tenantID, userID, _, serviceID, ok := deps.GetContext()
	if !ok {
		return CancelVisitOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
	}

	if deps.Booker == nil {
		return CancelVisitOutput{Success: false, Message: errMsgBookingNotConfigured}, fmt.Errorf(errBookerNotConfigured)
	}

	err := deps.Booker.CancelLeadVisit(context.Background(), ports.CancelVisitParams{
		TenantID:      tenantID,
		UserID:        userID,
		LeadServiceID: serviceID,
	})
	if err != nil {
		return CancelVisitOutput{Success: false, Message: err.Error()}, err
	}

	deps.MarkAppointmentCancelled()
	return CancelVisitOutput{Success: true, Message: "Appointment cancelled"}, nil
}

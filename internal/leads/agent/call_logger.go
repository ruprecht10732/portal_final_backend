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

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
)

// Error messages
const (
	errMsgMissingContext = "Missing context"
)

// Sentinel errors
var (
	errMissingContext = errors.New("missing context")
)

// CallLogResult represents the result of processing a call summary
type CallLogResult struct {
	NoteCreated       bool       `json:"noteCreated"`
	StatusUpdated     *string    `json:"statusUpdated,omitempty"`
	AppointmentBooked *time.Time `json:"appointmentBooked,omitempty"`
	Message           string     `json:"message"`
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
	Repo   repository.LeadsRepository
	Booker ports.AppointmentBooker
	mu     sync.RWMutex

	// Context for the current run
	tenantID  *uuid.UUID
	userID    *uuid.UUID
	leadID    *uuid.UUID
	serviceID *uuid.UUID

	// Track results during the run
	result CallLogResult
}

func (d *CallLoggerToolDeps) SetContext(tenantID, userID, leadID, serviceID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenantID = &tenantID
	d.userID = &userID
	d.leadID = &leadID
	d.serviceID = &serviceID
	d.result = CallLogResult{} // Reset result
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

func (d *CallLoggerToolDeps) GetResult() CallLogResult {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.result
}

// NewCallLogger creates a new CallLogger agent
func NewCallLogger(apiKey string, repo repository.LeadsRepository, booker ports.AppointmentBooker) (*CallLogger, error) {
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
			Repo:   repo,
			Booker: booker,
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

// ProcessSummary is the main entry point for processing a call summary
func (c *CallLogger) ProcessSummary(ctx context.Context, leadID, serviceID, userID, tenantID uuid.UUID, summary string) (*CallLogResult, error) {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	// Set context for tools
	c.toolDeps.SetContext(tenantID, userID, leadID, serviceID)

	// Construct the prompt with context
	promptText := fmt.Sprintf(`Analysis Context:
- Current Time: %s
- Lead ID: %s
- Service ID: %s
- Agent User ID: %s

The agent provided this post-call summary:
"%s"

Task:
1. Analyze the summary to determine the call outcome.
2. ALWAYS save the summary as a note using 'SaveNote' tool.
3. If an appointment was scheduled (e.g., "booked next tuesday at 9", "scheduled for friday 2pm"):
   - Calculate the exact date based on Current Time
   - Use 'ScheduleVisit' to book the appointment
   - Assume 1 hour duration unless specified otherwise
4. Update the status using 'UpdateStatus' if the outcome implies a status change:
   - "booked", "scheduled", "appointment set" → Scheduled
   - "not interested", "no need", "declined" → Bad_Lead
   - "voicemail", "no answer", "callback" → Attempted_Contact
   - "completed survey", "finished inspection" → Surveyed
   - "needs to reschedule", "postponed" → Needs_Rescheduling

Execute the appropriate tools now.`,
		time.Now().Format(time.RFC3339),
		leadID.String(),
		serviceID.String(),
		userID.String(),
		summary,
	)

	// Create ephemeral session
	sessionID := uuid.New().String()
	userIDStr := userID.String()

	_, err := c.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   c.appName,
		UserID:    userIDStr,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
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

	// Run the agent
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
			return nil, fmt.Errorf("call logger run failed: %w", err)
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				outputText += part.Text
			}
		}
	}

	log.Printf("CallLogger finished. Output: %s", outputText)

	// Get the result and build message
	result := c.toolDeps.GetResult()
	result.Message = buildResultMessage(result)

	return &result, nil
}

// buildResultMessage constructs a human-readable message from the call log result
func buildResultMessage(result CallLogResult) string {
	var messages []string
	if result.NoteCreated {
		messages = append(messages, "Note saved")
	}
	if result.StatusUpdated != nil {
		messages = append(messages, fmt.Sprintf("Status updated to %s", *result.StatusUpdated))
	}
	if result.AppointmentBooked != nil {
		messages = append(messages, fmt.Sprintf("Appointment booked for %s", result.AppointmentBooked.Format("2006-01-02 15:04")))
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
1. ALWAYS call SaveNote first to save the raw summary text as a note.
2. Parse dates relative to the Current Time provided in the context:
   - "next Tuesday" = the coming Tuesday from Current Time
   - "tomorrow" = Current Time + 1 day
   - "this Friday" = the Friday of the current week
   - "on the 15th" = the 15th of the current or next month
3. Default appointment duration is 1 hour unless explicitly stated.
4. Status mapping:
   - Appointment scheduled/booked → "Scheduled"
   - No answer/voicemail/try again → "Attempted_Contact"  
   - Not interested/declined/bad fit → "Bad_Lead"
   - Survey/inspection completed → "Surveyed"
   - Needs to reschedule/postponed → "Needs_Rescheduling"
5. When booking appointments, also update status to "Scheduled".
6. Use 24-hour time format (e.g., 09:00, 14:30).
7. Do NOT make up information. Only act on what is explicitly stated in the summary.
8. Email confirmation behavior for appointments:
   - By default, sendConfirmationEmail should be TRUE (send email)
   - Only set sendConfirmationEmail to FALSE if the call notes explicitly mention:
     - "no email", "don't send email", "skip email", "no confirmation email"
     - "they'll confirm differently", "will contact them separately"
   - If unclear, default to TRUE to send confirmation

Available tools:
- SaveNote: Saves the call summary as a note (ALWAYS use this)
- UpdateStatus: Updates the lead service status
- ScheduleVisit: Books an inspection/visit appointment (includes sendConfirmationEmail option)`
}

// Tool input/output types for CallLogger

type SaveNoteInput struct {
	Body string `json:"body"` // The note text to save
}

type SaveNoteOutput struct {
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

func buildCallLoggerTools(deps *CallLoggerToolDeps) ([]tool.Tool, error) {
	saveNoteTool, err := buildSaveNoteTool(deps)
	if err != nil {
		return nil, err
	}

	updateStatusTool, err := buildUpdateStatusTool(deps)
	if err != nil {
		return nil, err
	}

	scheduleVisitTool, err := buildScheduleVisitTool(deps)
	if err != nil {
		return nil, err
	}

	return []tool.Tool{saveNoteTool, updateStatusTool, scheduleVisitTool}, nil
}

func buildSaveNoteTool(deps *CallLoggerToolDeps) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SaveNote",
		Description: "Saves the call summary as a note on the lead. ALWAYS call this tool to record the call outcome.",
	}, func(ctx tool.Context, input SaveNoteInput) (SaveNoteOutput, error) {
		tenantID, userID, leadID, _, ok := deps.GetContext()
		if !ok {
			return SaveNoteOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}

		_, err := deps.Repo.CreateLeadNote(context.Background(), repository.CreateLeadNoteParams{
			LeadID:         leadID,
			OrganizationID: tenantID,
			AuthorID:       userID,
			Type:           "call",
			Body:           input.Body,
		})
		if err != nil {
			return SaveNoteOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkNoteCreated()
		return SaveNoteOutput{Success: true, Message: "Note saved"}, nil
	})
}

// validLeadStatuses defines the allowed status values for leads
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

func executeScheduleVisit(deps *CallLoggerToolDeps, input ScheduleVisitInput) (ScheduleVisitOutput, error) {
	tenantID, userID, leadID, serviceID, ok := deps.GetContext()
	if !ok {
		return ScheduleVisitOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
	}

	if deps.Booker == nil {
		return ScheduleVisitOutput{Success: false, Message: "Appointment booking not configured"}, fmt.Errorf("booker not configured")
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

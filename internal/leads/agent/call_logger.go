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

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/orchestration"
	apptools "portal_final_backend/internal/tools"
	"portal_final_backend/platform/adk/confirmation"
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/phone"
)

// Error messages
const (
	errMsgMissingContext       = "Missing context"
	errMsgBookingNotConfigured = "Appointment booking not configured"
	errMsgAppointmentRequired  = "Appointment must be booked or confirmed before setting Appointment_Scheduled"
	errMsgLeadUpdaterMissing   = "Lead update not configured"
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
	LeadUpdatedFields             []string   `json:"leadUpdatedFields,omitempty"`
	CallOutcome                   *string    `json:"callOutcome,omitempty"`
	StatusUpdated                 *string    `json:"statusUpdated,omitempty"`
	PipelineStageUpdated          *string    `json:"pipelineStageUpdated,omitempty"`
	AppointmentBooked             *time.Time `json:"appointmentBooked,omitempty"`
	AppointmentRescheduled        *time.Time `json:"appointmentRescheduled,omitempty"`
	AppointmentRescheduleFallback bool       `json:"appointmentRescheduleFallback,omitempty"`
	AppointmentCancelled          bool       `json:"appointmentCancelled,omitempty"`
	Warning                       string     `json:"warning,omitempty"`
	Message                       string     `json:"message"`
}

type CallLoggerLeadUpdater interface {
	Update(ctx context.Context, id uuid.UUID, req transport.UpdateLeadRequest, actorID uuid.UUID, tenantID uuid.UUID, actorRoles []string) (transport.LeadResponse, error)
}

// CallLogger processes post-call summaries into structured actions
type CallLogger struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	booker         ports.AppointmentBooker
	leadUpdater    CallLoggerLeadUpdater
	toolDeps       *CallLoggerToolDeps
}

// SetAppointmentBooker sets the appointment booker after initialization.
// This is needed to break circular dependencies during module initialization.
func (c *CallLogger) SetAppointmentBooker(booker ports.AppointmentBooker) {
	c.booker = booker
	if c.toolDeps != nil {
		c.toolDeps.Booker = booker
	}
}

func (c *CallLogger) SetLeadUpdater(updater CallLoggerLeadUpdater) {
	c.leadUpdater = updater
	if c.toolDeps != nil {
		c.toolDeps.LeadUpdater = updater
	}
}

func (c *CallLogger) HasAppointmentBooker() bool {
	return c != nil && c.booker != nil
}

func (c *CallLogger) HasLeadUpdater() bool {
	return c != nil && c.leadUpdater != nil
}

// CallLoggerToolDeps contains the dependencies needed by CallLogger tools
type CallLoggerToolDeps struct {
	Repo        repository.LeadsRepository
	Booker      ports.AppointmentBooker
	LeadUpdater CallLoggerLeadUpdater
	EventBus    events.Bus
	mu          sync.RWMutex

	// Context for the current run
	tenantID  *uuid.UUID
	userID    *uuid.UUID
	leadID    *uuid.UUID
	serviceID *uuid.UUID

	appointmentAvailable bool

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
	d.appointmentAvailable = false
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

func (d *CallLoggerToolDeps) MarkLeadUpdated(fields []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.LeadUpdatedFields = append([]string(nil), fields...)
}

func (d *CallLoggerToolDeps) MarkStatusUpdated(status string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.StatusUpdated = &status
}

func (d *CallLoggerToolDeps) MarkAppointmentBooked(startTime time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appointmentAvailable = true
	d.result.AppointmentBooked = &startTime
}

func (d *CallLoggerToolDeps) MarkAppointmentRescheduled(startTime time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appointmentAvailable = true
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
	d.appointmentAvailable = false
	d.result.AppointmentCancelled = true
}

func (d *CallLoggerToolDeps) SetAppointmentAvailable(available bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appointmentAvailable = available
}

func (d *CallLoggerToolDeps) HasAppointmentAvailable() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.appointmentAvailable
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

func (d *CallLoggerToolDeps) NewRequestDeps() *CallLoggerToolDeps {
	return &CallLoggerToolDeps{
		Repo:        d.Repo,
		Booker:      d.Booker,
		LeadUpdater: d.LeadUpdater,
		EventBus:    d.EventBus,
	}
}

// NewCallLogger creates a new CallLogger agent
func NewCallLogger(modelCfg openaicompat.Config, repo repository.LeadsRepository, booker ports.AppointmentBooker, eventBus events.Bus, sessionService session.Service) (*CallLogger, error) {
	kimi := openaicompat.NewModel(modelCfg)
	workspace, err := orchestration.LoadAgentWorkspace("call-logger")
	if err != nil {
		return nil, fmt.Errorf("failed to load call logger workspace context: %w", err)
	}

	logger := &CallLogger{
		repo:           repo,
		booker:         booker,
		appName:        "call_logger",
		sessionService: sessionService,
		toolDeps: &CallLoggerToolDeps{
			Repo:     repo,
			Booker:   booker,
			EventBus: eventBus,
		},
	}

	// Build tools
	tools, err := buildCallLoggerTools()
	if err != nil {
		return nil, fmt.Errorf("failed to build call logger tools: %w", err)
	}
	toolsets := orchestration.BuildWorkspaceToolsets(workspace, "call_logger_tools", tools)

	// Create the ADK agent
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "CallLogger",
		Model:       kimi,
		Description: "Post-call processing assistant that converts natural language call summaries into structured database updates (Notes, Status changes, Appointments).",
		Instruction: workspace.Instruction,
		Toolsets:    toolsets,
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
func (c *CallLogger) resolveExistingAppointment(ctx context.Context, tenantID, serviceID, userID uuid.UUID) (string, bool) {
	if c.booker == nil {
		return "None", false
	}

	visit, err := c.booker.GetLeadVisitByService(ctx, tenantID, serviceID, userID)
	if err == nil && visit != nil {
		return visit.StartTime.Format(dateTimeShortLayout), true
	}
	if err != nil && !apperr.Is(err, apperr.KindNotFound) {
		log.Printf("CallLogger warning: failed to check existing appointment: %v", err)
	}
	return "None", false
}

// executeAgentRun creates an ephemeral session, runs the agent, and returns the
// concatenated text output.
func (c *CallLogger) executeAgentRun(ctx context.Context, userIDStr, sessionID, promptText string) (string, error) {
	return runPromptTextSession(ctx, promptRunRequest{
		SessionService:       c.sessionService,
		Runner:               c.runner,
		AppName:              c.appName,
		UserID:               userIDStr,
		SessionID:            sessionID,
		CreateSessionMessage: "failed to create session",
		RunFailureMessage:    "call logger run failed",
		TraceLabel:           "call-logger",
	}, promptText)
}

// ProcessSummary is the main entry point for processing a call summary
func (c *CallLogger) ProcessSummary(ctx context.Context, leadID, serviceID, userID, tenantID uuid.UUID, summary string) (*CallLogResult, error) {
	reqDeps := c.toolDeps.NewRequestDeps()
	reqDeps.SetContext(tenantID, userID, leadID, serviceID)
	ctx = confirmation.WithTenantID(ctx, tenantID)
	ctx = WithCallLoggerDeps(ctx, reqDeps)

	existingAppointment, hasExistingAppointment := c.resolveExistingAppointment(ctx, tenantID, serviceID, userID)
	reqDeps.SetAppointmentAvailable(hasExistingAppointment)

	// Construct the prompt with context
	analysisContext := wrapReferenceBlock(fmt.Sprintf(`- Current Time: %s
- Lead ID: %s
- Existing Appointment: %s`,
		time.Now().Format(time.RFC3339),
		leadID.String(),
		existingAppointment,
	))
	summaryBlock := wrapReferenceBlock(sanitizeUserInput(summary, maxNoteLength))

	promptText := fmt.Sprintf(`Analysis Context (reference data):
%s

The agent provided this post-call summary (reference data):
%s

Execute the appropriate tools now. Respond ONLY with tool calls.`,
		analysisContext,
		summaryBlock,
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
	callDeps, err := GetCallLoggerDeps(ctx)
	if err != nil {
		return nil, err
	}
	result := callDeps.GetResult()
	result = enforceAppointmentSchedulingConsistency(result, reqDeps.HasAppointmentAvailable())
	result.Message = buildResultMessage(result)

	// Write the call-log timeline event here so the HTTP handler stays
	// free of persistence concerns (Issue: handler-level timeline writes).
	actorName := result.AuthorEmail
	if actorName == "" {
		actorName = userID.String()
	}
	summaryText := summary
	if result.NoteBody != "" {
		summaryText = result.NoteBody
	}
	_, _ = c.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      repository.ActorTypeUser,
		ActorName:      actorName,
		EventType:      repository.EventTypeCallLog,
		Title:          repository.EventTitleCallLog,
		Summary:        repository.TruncateSummary(summaryText, repository.TimelineSummaryMaxLen),
		Metadata: repository.CallLogMetadata{
			CallOutcome:            result.CallOutcome,
			NoteCreated:            result.NoteCreated,
			StatusUpdated:          result.StatusUpdated,
			PipelineStageUpdated:   result.PipelineStageUpdated,
			AppointmentBooked:      result.AppointmentBooked,
			AppointmentRescheduled: result.AppointmentRescheduled,
			AppointmentCancelled:   result.AppointmentCancelled,
		}.ToMap(),
	})

	return &result, nil
}

func (c *CallLogger) persistDraftedNote(ctx context.Context) error {
	reqDeps, err := GetCallLoggerDeps(ctx)
	if err != nil {
		return err
	}
	body, ok := reqDeps.GetNoteDraft()
	if !ok {
		return nil
	}

	result := reqDeps.GetResult()
	finalBody := body
	if result.AppointmentRescheduleFallback && result.AppointmentBooked != nil {
		finalBody = appendRescheduleFallbackNote(finalBody, *result.AppointmentBooked)
	}

	tenantID, userID, leadID, _, ok := reqDeps.GetContext()
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

	reqDeps.MarkNoteCreated()
	reqDeps.SetNoteDetails(finalBody, note.AuthorEmail)
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
	if len(result.LeadUpdatedFields) > 0 {
		messages = append(messages, fmt.Sprintf("Lead details updated: %s", strings.Join(result.LeadUpdatedFields, ", ")))
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
	if warning := strings.TrimSpace(result.Warning); warning != "" {
		messages = append(messages, warning)
	}
	if len(messages) == 0 {
		return "No actions taken"
	}
	return strings.Join(messages, ". ")
}

// Tool input/output types for CallLogger

type SaveNoteInput struct {
	Body string `json:"body"` // The note text to save
}

type SaveNoteOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
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
	Status string `json:"status"` // New status: New, Pending, In_Progress, Attempted_Contact, Appointment_Scheduled, Needs_Rescheduling, Disqualified
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

func buildCallLoggerTools() ([]tool.Tool, error) {
	saveNoteTool, err := buildSaveNoteTool()
	if err != nil {
		return nil, err
	}

	updateLeadDetailsTool, err := buildUpdateLeadDetailsTool()
	if err != nil {
		return nil, err
	}

	setCallOutcomeTool, err := buildSetCallOutcomeTool()
	if err != nil {
		return nil, err
	}

	updateStatusTool, err := buildUpdateStatusTool()
	if err != nil {
		return nil, err
	}

	updatePipelineStageTool, err := buildCallLoggerUpdatePipelineStageTool()
	if err != nil {
		return nil, err
	}

	scheduleVisitTool, err := buildScheduleVisitTool()
	if err != nil {
		return nil, err
	}

	rescheduleVisitTool, err := buildRescheduleVisitTool()
	if err != nil {
		return nil, err
	}

	cancelVisitTool, err := buildCancelVisitTool()
	if err != nil {
		return nil, err
	}

	return []tool.Tool{
		saveNoteTool,
		updateLeadDetailsTool,
		setCallOutcomeTool,
		updateStatusTool,
		updatePipelineStageTool,
		scheduleVisitTool,
		rescheduleVisitTool,
		cancelVisitTool,
	}, nil
}

func buildSaveNoteTool() (tool.Tool, error) {
	return apptools.NewSaveNoteTool(func(ctx tool.Context, input SaveNoteInput) (SaveNoteOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return SaveNoteOutput{Success: false, Message: errMsgMissingContext}, err
		}
		if _, _, _, _, ok := deps.GetContext(); !ok {
			return SaveNoteOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}

		deps.SetNoteDraft(normalizeCallNoteBody(input.Body))
		return SaveNoteOutput{Success: true, Message: "Note drafted"}, nil
	})
}

func buildUpdateLeadDetailsTool() (tool.Tool, error) {
	return apptools.NewUpdateLeadDetailsTool("Updates lead profile fields such as name, phone, email, address, consumer role, and WhatsApp preference when the caller provides corrections.", func(ctx tool.Context, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return UpdateLeadDetailsOutput{Success: false, Message: errMsgMissingContext}, err
		}
		tenantID, userID, leadID, _, ok := deps.GetContext()
		if !ok {
			return UpdateLeadDetailsOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}
		if deps.LeadUpdater == nil {
			return UpdateLeadDetailsOutput{Success: false, Message: errMsgLeadUpdaterMissing}, errors.New("lead update not configured")
		}

		req, updatedFields, err := buildLeadUpdateRequest(input)
		if err != nil {
			return UpdateLeadDetailsOutput{Success: false, Message: err.Error()}, err
		}

		_, err = deps.LeadUpdater.Update(ctx, leadID, req, userID, tenantID, nil)
		if err != nil {
			return UpdateLeadDetailsOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkLeadUpdated(updatedFields)
		return UpdateLeadDetailsOutput{
			Success:       true,
			Message:       fmt.Sprintf("Lead details updated: %s", strings.Join(updatedFields, ", ")),
			UpdatedFields: updatedFields,
		}, nil
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

func applyLeadUpdateString(fieldName string, value *string, target **string, updatedFields *[]string) {
	if value == nil {
		return
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return
	}
	*target = &trimmed
	*updatedFields = append(*updatedFields, fieldName)
}

func applyLeadUpdatePhone(value *string, req *transport.UpdateLeadRequest, updatedFields *[]string) error {
	if value == nil {
		return nil
	}
	normalizedPhone := phone.NormalizeE164(strings.TrimSpace(*value))
	if normalizedPhone == "" {
		return errors.New("invalid phone")
	}
	req.Phone = &normalizedPhone
	*updatedFields = append(*updatedFields, "phone")
	return nil
}

func applyLeadUpdateConsumerRole(value *string, req *transport.UpdateLeadRequest, updatedFields *[]string) error {
	if value == nil {
		return nil
	}
	normalizedRole, err := normalizeConsumerRole(*value)
	if err != nil {
		return err
	}
	role := transport.ConsumerRole(normalizedRole)
	req.ConsumerRole = &role
	*updatedFields = append(*updatedFields, "consumerRole")
	return nil
}

func applyLeadUpdateAssignee(value *string, req *transport.UpdateLeadRequest, updatedFields *[]string) error {
	if value == nil {
		return nil
	}
	trimmedAssigneeID := strings.TrimSpace(*value)
	if trimmedAssigneeID == "" {
		return errors.New("invalid assigneeId")
	}
	parsedAssigneeID, err := uuid.Parse(trimmedAssigneeID)
	if err != nil {
		return errors.New("invalid assigneeId")
	}
	req.AssigneeID = transport.OptionalUUID{Value: &parsedAssigneeID, Set: true}
	*updatedFields = append(*updatedFields, "assigneeId")
	return nil
}

func applyLeadUpdateCoordinate(fieldName string, value *float64, min float64, max float64, target **float64, updatedFields *[]string) error {
	if value == nil {
		return nil
	}
	if *value < min || *value > max {
		return fmt.Errorf("invalid %s", fieldName)
	}
	coordinate := *value
	*target = &coordinate
	*updatedFields = append(*updatedFields, fieldName)
	return nil
}

func applyLeadUpdateWhatsAppOptIn(value *bool, req *transport.UpdateLeadRequest, updatedFields *[]string) {
	if value == nil {
		return
	}
	whatsAppOptedIn := *value
	req.WhatsAppOptedIn = &whatsAppOptedIn
	*updatedFields = append(*updatedFields, "whatsAppOptedIn")
}

func buildLeadUpdateRequest(input UpdateLeadDetailsInput) (transport.UpdateLeadRequest, []string, error) {
	req := transport.UpdateLeadRequest{}
	updatedFields := make([]string, 0, 10)

	applyLeadUpdateString("firstName", input.FirstName, &req.FirstName, &updatedFields)
	applyLeadUpdateString("lastName", input.LastName, &req.LastName, &updatedFields)
	applyLeadUpdateString("email", input.Email, &req.Email, &updatedFields)
	applyLeadUpdateString("street", input.Street, &req.Street, &updatedFields)
	applyLeadUpdateString("houseNumber", input.HouseNumber, &req.HouseNumber, &updatedFields)
	applyLeadUpdateString("zipCode", input.ZipCode, &req.ZipCode, &updatedFields)
	applyLeadUpdateString("city", input.City, &req.City, &updatedFields)

	if err := applyLeadUpdatePhone(input.Phone, &req, &updatedFields); err != nil {
		return transport.UpdateLeadRequest{}, nil, err
	}
	if err := applyLeadUpdateConsumerRole(input.ConsumerRole, &req, &updatedFields); err != nil {
		return transport.UpdateLeadRequest{}, nil, err
	}
	if err := applyLeadUpdateAssignee(input.AssigneeID, &req, &updatedFields); err != nil {
		return transport.UpdateLeadRequest{}, nil, err
	}
	if err := applyLeadUpdateCoordinate("latitude", input.Latitude, -90, 90, &req.Latitude, &updatedFields); err != nil {
		return transport.UpdateLeadRequest{}, nil, err
	}
	if err := applyLeadUpdateCoordinate("longitude", input.Longitude, -180, 180, &req.Longitude, &updatedFields); err != nil {
		return transport.UpdateLeadRequest{}, nil, err
	}
	applyLeadUpdateWhatsAppOptIn(input.WhatsAppOptedIn, &req, &updatedFields)

	if len(updatedFields) == 0 {
		return transport.UpdateLeadRequest{}, nil, errors.New("no lead fields provided")
	}

	return req, updatedFields, nil
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

func requiresAppointmentAvailability(value string) bool {
	return strings.TrimSpace(value) == domain.LeadStatusAppointmentScheduled
}

func validateAppointmentAvailability(value string, appointmentAvailable bool) error {
	if !requiresAppointmentAvailability(value) || appointmentAvailable {
		return nil
	}
	return errors.New(strings.ToLower(errMsgAppointmentRequired[:1]) + errMsgAppointmentRequired[1:])
}

func enforceAppointmentSchedulingConsistency(result CallLogResult, appointmentAvailable bool) CallLogResult {
	if appointmentAvailable {
		return result
	}

	var normalized bool
	if result.CallOutcome != nil && requiresAppointmentAvailability(*result.CallOutcome) {
		result.CallOutcome = nil
		normalized = true
	}
	if result.StatusUpdated != nil && requiresAppointmentAvailability(*result.StatusUpdated) {
		result.StatusUpdated = nil
		normalized = true
	}
	if !normalized {
		return result
	}

	result.Warning = "Appointment could not be confirmed automatically; manual follow-up required"
	return result
}

func buildSetCallOutcomeTool() (tool.Tool, error) {
	return apptools.NewSetCallOutcomeTool(func(ctx tool.Context, input SetCallOutcomeInput) (SetCallOutcomeOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return SetCallOutcomeOutput{Success: false, Message: errMsgMissingContext}, err
		}
		tenantID, userID, leadID, serviceID, ok := deps.GetContext()
		if !ok {
			return SetCallOutcomeOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}

		outcome := strings.TrimSpace(input.Outcome)
		if outcome == "" {
			return SetCallOutcomeOutput{Success: false, Message: "Missing outcome"}, fmt.Errorf("missing outcome")
		}
		if err := validateAppointmentAvailability(outcome, deps.HasAppointmentAvailable()); err != nil {
			return SetCallOutcomeOutput{Success: false, Message: err.Error()}, err
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
			ActorType:      repository.ActorTypeUser,
			ActorName:      actorName,
			EventType:      repository.EventTypeCallOutcome,
			Title:          repository.EventTitleCallOutcome,
			Summary:        &summary,
			Metadata: repository.CallOutcomeMetadata{
				Outcome: outcome,
				Notes:   strings.TrimSpace(input.Notes),
			}.ToMap(),
		})

		deps.SetCallOutcome(outcome)
		return SetCallOutcomeOutput{Success: true, Message: "Call outcome set"}, nil
	})
}

// validLeadStatuses defines the allowed status values for RAC_leads
var validLeadStatuses = map[string]bool{
	"New":                   true,
	"Pending":               true,
	"In_Progress":           true,
	"Attempted_Contact":     true,
	"Appointment_Scheduled": true,
	"Needs_Rescheduling":    true,
	"Disqualified":          true,
}

func buildUpdateStatusTool() (tool.Tool, error) {
	return apptools.NewUpdateStatusTool(func(ctx tool.Context, input UpdateStatusInput) (UpdateStatusOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return UpdateStatusOutput{Success: false, Message: errMsgMissingContext}, err
		}
		tenantID, _, _, serviceID, ok := deps.GetContext()
		if !ok {
			return UpdateStatusOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
		}

		if !validLeadStatuses[input.Status] {
			return UpdateStatusOutput{Success: false, Message: "Invalid status"}, fmt.Errorf("invalid status: %s", input.Status)
		}
		if err := validateAppointmentAvailability(input.Status, deps.HasAppointmentAvailable()); err != nil {
			return UpdateStatusOutput{Success: false, Message: err.Error()}, err
		}

		svc, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
		if err != nil {
			return UpdateStatusOutput{Success: false, Message: "Lead service not found"}, err
		}

		if domain.IsTerminal(svc.Status, svc.PipelineStage) {
			return UpdateStatusOutput{Success: false, Message: "Cannot update status for a service in terminal state"}, fmt.Errorf("service %s is terminal", serviceID)
		}

		if reason := domain.ValidateStateCombination(input.Status, svc.PipelineStage); reason != "" {
			return UpdateStatusOutput{Success: false, Message: reason}, fmt.Errorf("invalid state combination: %s", reason)
		}

		_, err = deps.Repo.UpdateServiceStatus(context.Background(), serviceID, tenantID, input.Status)
		if err != nil {
			return UpdateStatusOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkStatusUpdated(input.Status)
		return UpdateStatusOutput{Success: true, Message: fmt.Sprintf("Status updated to %s", input.Status)}, nil
	})
}

func buildScheduleVisitTool() (tool.Tool, error) {
	return apptools.NewScheduleVisitTool(func(ctx tool.Context, input ScheduleVisitInput) (ScheduleVisitOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return ScheduleVisitOutput{}, err
		}
		return executeScheduleVisit(deps, input)
	})
}

func buildRescheduleVisitTool() (tool.Tool, error) {
	return apptools.NewRescheduleVisitTool(func(ctx tool.Context, input RescheduleVisitInput) (RescheduleVisitOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return RescheduleVisitOutput{}, err
		}
		return executeRescheduleVisit(deps, input)
	})
}

func buildCancelVisitTool() (tool.Tool, error) {
	return apptools.NewCancelVisitTool(func(ctx tool.Context, input CancelVisitInput) (CancelVisitOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return CancelVisitOutput{}, err
		}
		return executeCancelVisit(deps, input)
	})
}

func executeScheduleVisit(deps *CallLoggerToolDeps, input ScheduleVisitInput) (ScheduleVisitOutput, error) {
	tenantID, userID, leadID, serviceID, ok := deps.GetContext()
	if !ok {
		return ScheduleVisitOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
	}

	if deps.Booker == nil {
		return ScheduleVisitOutput{Success: false, Message: errMsgBookingNotConfigured}, errors.New(errBookerNotConfigured)
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

func validateCallLoggerPipelineStage(stage string) error {
	if !domain.IsKnownPipelineStage(stage) {
		return fmt.Errorf("invalid pipeline stage: %s", stage)
	}
	return nil
}

func resolveCallLoggerStageContext(deps *CallLoggerToolDeps, ctx tool.Context) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, string, string, error) {
	tenantID, userID, leadID, serviceID, ok := deps.GetContext()
	if !ok {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, "", "", errMissingContext
	}

	svc, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, "", "", err
	}

	return tenantID, userID, leadID, serviceID, svc.PipelineStage, svc.Status, nil
}

func guardCallLoggerQuoteSentTransition(deps *CallLoggerToolDeps, ctx tool.Context, tenantID, serviceID uuid.UUID, newStage string) error {
	if newStage != domain.PipelineStageProposal {
		return nil
	}

	hasNonDraftQuote, err := deps.Repo.HasNonDraftQuote(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}
	if !hasNonDraftQuote {
		return fmt.Errorf("quote state guard blocked Proposal for service %s", serviceID)
	}
	return nil
}

type callLoggerStageTimelineEventInput struct {
	TenantID  uuid.UUID
	UserID    uuid.UUID
	LeadID    uuid.UUID
	ServiceID uuid.UUID
	OldStage  string
	NewStage  string
	Reason    string
}

func createCallLoggerStageTimelineEvent(deps *CallLoggerToolDeps, ctx tool.Context, input callLoggerStageTimelineEventInput) {
	trimmedReason := strings.TrimSpace(input.Reason)
	var summary *string
	if trimmedReason != "" {
		summary = &trimmedReason
	}

	actorName := input.UserID.String()
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         input.LeadID,
		ServiceID:      &input.ServiceID,
		OrganizationID: input.TenantID,
		ActorType:      repository.ActorTypeUser,
		ActorName:      actorName,
		EventType:      repository.EventTypeStageChange,
		Title:          repository.EventTitleStageUpdated,
		Summary:        summary,
		Metadata: repository.StageChangeMetadata{
			OldStage: input.OldStage,
			NewStage: input.NewStage,
		}.ToMap(),
	})
}

func publishCallLoggerStageChanged(deps *CallLoggerToolDeps, ctx tool.Context, tenantID, leadID, serviceID uuid.UUID, oldStage, newStage string) {
	if deps.EventBus == nil {
		return
	}

	deps.EventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        leadID,
		LeadServiceID: serviceID,
		TenantID:      tenantID,
		OldStage:      oldStage,
		NewStage:      newStage,
	})
}

func buildCallLoggerUpdatePipelineStageTool() (tool.Tool, error) {
	return apptools.NewUpdatePipelineStageTool(func(ctx tool.Context, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
		deps, err := GetCallLoggerDeps(ctx)
		if err != nil {
			return UpdatePipelineStageOutput{}, err
		}
		return handleCallLoggerUpdatePipelineStage(ctx, deps, input)
	})
}

type callLoggerStageContext struct {
	tenantID       uuid.UUID
	userID         uuid.UUID
	leadID         uuid.UUID
	serviceID      uuid.UUID
	oldStage       string
	currentStatus  string
	requestedStage string
}

func handleCallLoggerUpdatePipelineStage(ctx tool.Context, deps *CallLoggerToolDeps, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	stageCtx, out, err := validateAndResolveCallLoggerStageContext(deps, ctx, input)
	if err != nil {
		return out, err
	}

	if err := updateCallLoggerPipelineStage(deps, ctx, stageCtx); err != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Failed to update pipeline stage"}, err
	}

	createCallLoggerStageTimelineEvent(deps, ctx, callLoggerStageTimelineEventInput{
		TenantID:  stageCtx.tenantID,
		UserID:    stageCtx.userID,
		LeadID:    stageCtx.leadID,
		ServiceID: stageCtx.serviceID,
		OldStage:  stageCtx.oldStage,
		NewStage:  stageCtx.requestedStage,
		Reason:    input.Reason,
	})
	publishCallLoggerStageChanged(deps, ctx, stageCtx.tenantID, stageCtx.leadID, stageCtx.serviceID, stageCtx.oldStage, stageCtx.requestedStage)

	deps.MarkPipelineStageUpdated(stageCtx.requestedStage)
	return UpdatePipelineStageOutput{Success: true, Message: "Pipeline stage updated"}, nil
}

func validateAndResolveCallLoggerStageContext(deps *CallLoggerToolDeps, ctx tool.Context, input UpdatePipelineStageInput) (callLoggerStageContext, UpdatePipelineStageOutput, error) {
	if err := validateCallLoggerPipelineStage(input.Stage); err != nil {
		return callLoggerStageContext{}, UpdatePipelineStageOutput{Success: false, Message: "Invalid pipeline stage"}, err
	}

	tenantID, userID, leadID, serviceID, oldStage, currentStatus, err := resolveCallLoggerStageContext(deps, ctx)
	if err != nil {
		if errors.Is(err, errMissingContext) {
			return callLoggerStageContext{}, UpdatePipelineStageOutput{Success: false, Message: errMsgMissingContext}, err
		}
		return callLoggerStageContext{}, UpdatePipelineStageOutput{Success: false, Message: "Lead service not found"}, err
	}

	if domain.IsTerminal(currentStatus, oldStage) {
		return callLoggerStageContext{}, UpdatePipelineStageOutput{Success: false, Message: "Cannot update pipeline stage for a service in terminal state"}, fmt.Errorf("service %s is terminal", serviceID)
	}

	if reason := domain.ValidateStateCombination(currentStatus, input.Stage); reason != "" {
		return callLoggerStageContext{}, UpdatePipelineStageOutput{Success: false, Message: reason}, fmt.Errorf("invalid state combination: %s", reason)
	}

	if err := guardCallLoggerQuoteSentTransition(deps, ctx, tenantID, serviceID, input.Stage); err != nil {
		if strings.Contains(err.Error(), "quote state guard blocked") {
			return callLoggerStageContext{}, UpdatePipelineStageOutput{Success: false, Message: "Cannot set Proposal while quote is still draft"}, err
		}
		return callLoggerStageContext{}, UpdatePipelineStageOutput{Success: false, Message: "Failed to validate quote state"}, err
	}

	return callLoggerStageContext{
		tenantID:       tenantID,
		userID:         userID,
		leadID:         leadID,
		serviceID:      serviceID,
		oldStage:       oldStage,
		currentStatus:  currentStatus,
		requestedStage: input.Stage,
	}, UpdatePipelineStageOutput{}, nil
}

func updateCallLoggerPipelineStage(deps *CallLoggerToolDeps, ctx tool.Context, stageCtx callLoggerStageContext) error {
	_, err := deps.Repo.UpdatePipelineStage(ctx, stageCtx.serviceID, stageCtx.tenantID, stageCtx.requestedStage)
	return err
}

func executeRescheduleVisit(deps *CallLoggerToolDeps, input RescheduleVisitInput) (RescheduleVisitOutput, error) {
	tenantID, userID, _, serviceID, ok := deps.GetContext()
	if !ok {
		return RescheduleVisitOutput{Success: false, Message: errMsgMissingContext}, errMissingContext
	}

	if deps.Booker == nil {
		return RescheduleVisitOutput{Success: false, Message: errMsgBookingNotConfigured}, errors.New(errBookerNotConfigured)
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
		return CancelVisitOutput{Success: false, Message: errMsgBookingNotConfigured}, errors.New(errBookerNotConfigured)
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

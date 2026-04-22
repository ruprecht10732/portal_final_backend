package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	leadrepo "portal_final_backend/internal/leads/repository"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	repo               *Repository
	notificationOutbox *notificationoutbox.Repository
	reminderScheduler  scheduler.TaskReminderScheduler
	timeline           leadrepo.TimelineEventStore
	log                *logger.Logger
}

func NewService(repo *Repository, reminderScheduler scheduler.TaskReminderScheduler, timeline leadrepo.TimelineEventStore, log *logger.Logger) *Service {
	return &Service{
		repo:               repo,
		notificationOutbox: notificationoutbox.New(repo.pool),
		reminderScheduler:  reminderScheduler,
		timeline:           timeline,
		log:                log,
	}
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, req ListTasksRequest) ([]TaskRecord, error) {
	filter, err := parseListFilter(req)
	if err != nil {
		return nil, err
	}
	return s.repo.listTasks(ctx, tenantID, filter)
}

func (s *Service) Get(ctx context.Context, tenantID, taskID uuid.UUID) (TaskRecord, error) {
	return s.repo.getTask(ctx, tenantID, taskID)
}

func (s *Service) Create(ctx context.Context, tenantID, actorID uuid.UUID, req CreateTaskRequest) (TaskRecord, error) {
	task, reminderCfg, err := s.prepareCreateTask(tenantID, actorID, req)
	if err != nil {
		return TaskRecord{}, err
	}
	tx, err := s.repo.pool.Begin(ctx)
	if err != nil {
		return TaskRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.insertTaskWithReminder(ctx, tx, tenantID, task, reminderCfg)
	if err != nil {
		return TaskRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TaskRecord{}, err
	}

	created, err := s.repo.getTask(ctx, tenantID, id)
	if err != nil {
		return TaskRecord{}, err
	}
	if created.Reminder != nil {
		s.scheduleReminderBestEffort(ctx, *created.Reminder)
	}
	s.writeTimelineBestEffort(ctx, created, leadrepo.EventTitleCustomerInfo, "Taak aangemaakt")
	return created, nil
}

func (s *Service) Update(ctx context.Context, tenantID, taskID uuid.UUID, req UpdateTaskRequest) (TaskRecord, error) {
	tx, err := s.repo.pool.Begin(ctx)
	if err != nil {
		return TaskRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	setClauses, args, err := s.buildUpdateMutation(ctx, tx, tenantID, taskID, req)
	if err != nil {
		return TaskRecord{}, err
	}
	if err := s.applyTaskFieldUpdates(ctx, tx, setClauses, args); err != nil {
		return TaskRecord{}, err
	}
	if err := s.applyReminderUpdate(ctx, tx, tenantID, taskID, req); err != nil {
		return TaskRecord{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return TaskRecord{}, err
	}
	updated, err := s.repo.getTask(ctx, tenantID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if updated.Reminder != nil {
		s.scheduleReminderBestEffort(ctx, *updated.Reminder)
	}
	return updated, nil
}

func (s *Service) Assign(ctx context.Context, tenantID, taskID, assignedUserID uuid.UUID) (TaskRecord, error) {
	requestID := assignedUserID.String()
	return s.Update(ctx, tenantID, taskID, UpdateTaskRequest{AssignedUserID: &requestID})
}

func (s *Service) Complete(ctx context.Context, tenantID, taskID uuid.UUID) (TaskRecord, error) {
	if _, err := s.repo.pool.Exec(ctx, `UPDATE RAC_tasks SET status = $3, completed_at = now(), updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenantID, taskID, StatusCompleted); err != nil {
		return TaskRecord{}, err
	}
	_, _ = s.repo.pool.Exec(ctx, `UPDATE RAC_task_reminders SET enabled = false, next_run_at = NULL, updated_at = now() WHERE tenant_id = $1 AND task_id = $2`, tenantID, taskID)
	task, err := s.repo.getTask(ctx, tenantID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	s.writeTimelineBestEffort(ctx, task, leadrepo.EventTitleLeadDetailsUpdated, "Taak afgerond")
	return task, nil
}

func (s *Service) Cancel(ctx context.Context, tenantID, taskID uuid.UUID) (TaskRecord, error) {
	if _, err := s.repo.pool.Exec(ctx, `UPDATE RAC_tasks SET status = $3, cancelled_at = now(), updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenantID, taskID, StatusCancelled); err != nil {
		return TaskRecord{}, err
	}
	_, _ = s.repo.pool.Exec(ctx, `UPDATE RAC_task_reminders SET enabled = false, next_run_at = NULL, updated_at = now() WHERE tenant_id = $1 AND task_id = $2`, tenantID, taskID)
	task, err := s.repo.getTask(ctx, tenantID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	s.writeTimelineBestEffort(ctx, task, leadrepo.EventTitleLeadDetailsUpdated, "Taak geannuleerd")
	return task, nil
}

func (s *Service) Reopen(ctx context.Context, tenantID, taskID uuid.UUID) (TaskRecord, error) {
	if _, err := s.repo.pool.Exec(ctx, `UPDATE RAC_tasks SET status = $3, completed_at = NULL, cancelled_at = NULL, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenantID, taskID, StatusOpen); err != nil {
		return TaskRecord{}, err
	}
	task, err := s.repo.getTask(ctx, tenantID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	s.writeTimelineBestEffort(ctx, task, leadrepo.EventTitleLeadDetailsUpdated, "Taak heropend")
	return task, nil
}

func (s *Service) Delete(ctx context.Context, tenantID, taskID uuid.UUID) error {
	task, err := s.repo.getTask(ctx, tenantID, taskID)
	if err != nil {
		return err
	}
	if _, err := s.repo.pool.Exec(ctx, `DELETE FROM RAC_tasks WHERE tenant_id = $1 AND id = $2`, tenantID, taskID); err != nil {
		return err
	}
	s.writeTimelineBestEffort(ctx, task, leadrepo.EventTitleLeadDetailsUpdated, "Taak verwijderd")
	return nil
}

func (s *Service) ProcessTaskReminder(ctx context.Context, reminderID uuid.UUID, scheduledFor time.Time) error {
	rec, err := s.loadProcessableReminder(ctx, reminderID, scheduledFor)
	if err != nil || rec == nil {
		return err
	}
	nextRunAt := nextReminderRunAt(rec.Reminder)
	if err := s.deliverTaskReminder(ctx, *rec); err != nil {
		return err
	}
	if err := s.repo.advanceReminderAfterTrigger(ctx, reminderID, scheduledFor.UTC(), nextRunAt, nil); err != nil {
		return err
	}
	if nextRunAt != nil {
		s.scheduleReminderBestEffort(ctx, ReminderRecord{ID: reminderID, NextRunAt: nextRunAt, Enabled: true})
	}
	return nil
}

func (s *Service) scheduleReminderBestEffort(ctx context.Context, reminder ReminderRecord) {
	if !reminder.Enabled || reminder.NextRunAt == nil {
		return
	}
	if s.reminderScheduler == nil {
		return
	}
	if err := s.reminderScheduler.ScheduleTaskReminder(ctx, scheduler.TaskReminderPayload{
		ReminderID:   reminder.ID.String(),
		ScheduledFor: reminder.NextRunAt.UTC().Format(time.RFC3339Nano),
	}, reminder.NextRunAt.UTC()); err != nil {
		if s.log != nil {
			s.log.Warn("failed to schedule task reminder", "reminderId", reminder.ID.String(), "error", err)
		}
		_ = s.repo.markReminderError(ctx, reminder.ID, err.Error())
	}
}

func (s *Service) writeTimelineBestEffort(ctx context.Context, task TaskRecord, title, summary string) {
	if s.timeline == nil || task.LeadID == nil || task.LeadServiceID == nil {
		return
	}
	_, _ = s.timeline.CreateTimelineEvent(ctx, leadrepo.CreateTimelineEventParams{
		LeadID:         *task.LeadID,
		ServiceID:      task.LeadServiceID,
		OrganizationID: task.TenantID,
		ActorType:      leadrepo.ActorTypeSystem,
		ActorName:      "Tasks",
		EventType:      leadrepo.EventTypeLeadUpdate,
		Title:          title,
		Summary:        leadrepo.TruncateSummary(summary+": "+task.Title, leadrepo.TimelineSummaryMaxLen),
		Metadata: map[string]any{
			"taskId": task.ID.String(),
		},
		Visibility: leadrepo.TimelineVisibilityInternal,
	})
}

func normalizeReminderConfig(cfg *ReminderConfig) (*ReminderConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	if !cfg.Enabled {
		return &ReminderConfig{Enabled: false}, nil
	}
	if cfg.RunAt == nil || cfg.RunAt.IsZero() {
		return nil, fmt.Errorf("reminder runAt is required when reminder is enabled")
	}
	if !cfg.SendEmail && !cfg.SendWhatsApp {
		return nil, fmt.Errorf("reminder must send email, whatsapp, or both")
	}
	normalized := &ReminderConfig{
		Enabled:      true,
		RunAt:        ptrTime(cfg.RunAt.UTC()),
		RepeatDaily:  cfg.RepeatDaily,
		SendEmail:    cfg.SendEmail,
		SendWhatsApp: cfg.SendWhatsApp,
	}
	return normalized, nil
}

func parseListFilter(req ListTasksRequest) (listTasksFilter, error) {
	filter := listTasksFilter{ScopeType: strings.TrimSpace(req.ScopeType), Status: strings.TrimSpace(req.Status)}
	if strings.TrimSpace(req.AssignedUserID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(req.AssignedUserID))
		if err != nil {
			return listTasksFilter{}, err
		}
		filter.AssignedUserID = &parsed
	}
	if strings.TrimSpace(req.LeadID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(req.LeadID))
		if err != nil {
			return listTasksFilter{}, err
		}
		filter.LeadID = &parsed
	}
	if strings.TrimSpace(req.LeadServiceID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(req.LeadServiceID))
		if err != nil {
			return listTasksFilter{}, err
		}
		filter.LeadServiceID = &parsed
	}
	if strings.TrimSpace(req.DueFrom) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.DueFrom))
		if err != nil {
			return listTasksFilter{}, err
		}
		filter.DueFrom = &parsed
	}
	if strings.TrimSpace(req.DueTo) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.DueTo))
		if err != nil {
			return listTasksFilter{}, err
		}
		filter.DueTo = &parsed
	}
	if req.Limit > 0 {
		filter.Limit = req.Limit
	}
	return filter, nil
}

func parseRequiredUUID(value *string, name string) (uuid.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return uuid.Nil, fmt.Errorf("%s is required", name)
	}
	return uuid.Parse(strings.TrimSpace(*value))
}

func normalizePriority(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return PriorityNormal
	}
	return trimmed
}

func trimmedString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func ptrTime(value time.Time) *time.Time {
	v := value.UTC()
	return &v
}

func nullableUUIDString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func buildReminderSubject(task TaskRecord) string {
	return "Taakherinnering: " + task.Title
}

func buildReminderEmailHTML(task TaskRecord) string {
	var builder strings.Builder
	builder.WriteString("<p>Je hebt een taakherinnering.</p>")
	builder.WriteString("<p><strong>")
	builder.WriteString(task.Title)
	builder.WriteString("</strong></p>")
	if strings.TrimSpace(task.Description) != "" {
		builder.WriteString("<p>")
		builder.WriteString(task.Description)
		builder.WriteString("</p>")
	}
	builder.WriteString("<p><a href=\"")
	builder.WriteString("/app/tasks?taskId=" + task.ID.String())
	builder.WriteString("\">Open taakoverzicht</a></p>")
	return builder.String()
}

func buildReminderWhatsAppMessage(task TaskRecord) string {
	message := "Taakherinnering: " + task.Title
	if strings.TrimSpace(task.Description) != "" {
		message += "\n" + task.Description
	}
	message += "\nOpen in portal: /app/tasks?taskId=" + task.ID.String()
	return message
}

func (s *Service) prepareCreateTask(tenantID, actorID uuid.UUID, req CreateTaskRequest) (TaskRecord, *ReminderConfig, error) {
	reminderCfg, err := normalizeReminderConfig(req.Reminder)
	if err != nil {
		return TaskRecord{}, nil, err
	}
	assignedUserID, err := uuid.Parse(req.AssignedUserID)
	if err != nil {
		return TaskRecord{}, nil, err
	}
	task := TaskRecord{
		TenantID:        tenantID,
		ScopeType:       req.ScopeType,
		AssignedUserID:  assignedUserID,
		CreatedByUserID: actorID,
		Title:           strings.TrimSpace(req.Title),
		Description:     trimmedString(req.Description),
		Status:          StatusOpen,
		Priority:        normalizePriority(req.Priority),
		DueAt:           req.DueAt,
	}
	if err := populateLeadServiceScope(&task, req); err != nil {
		return TaskRecord{}, nil, err
	}
	return task, reminderCfg, nil
}

func populateLeadServiceScope(task *TaskRecord, req CreateTaskRequest) error {
	if req.ScopeType != ScopeLeadService {
		return nil
	}
	leadID, err := parseRequiredUUID(req.LeadID, "leadId")
	if err != nil {
		return err
	}
	serviceID, err := parseRequiredUUID(req.LeadServiceID, "leadServiceId")
	if err != nil {
		return err
	}
	task.LeadID = &leadID
	task.LeadServiceID = &serviceID
	return nil
}

func (s *Service) insertTaskWithReminder(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, task TaskRecord, reminderCfg *ReminderConfig) (uuid.UUID, error) {
	if _, err := s.repo.requireAssignee(ctx, tx, tenantID, task.AssignedUserID); err != nil {
		return uuid.Nil, err
	}
	if task.ScopeType == ScopeLeadService && task.LeadID != nil && task.LeadServiceID != nil {
		if err := s.repo.validateLeadServiceScope(ctx, tx, tenantID, *task.LeadID, *task.LeadServiceID); err != nil {
			return uuid.Nil, err
		}
	}
	id, err := s.repo.insertTask(ctx, tx, task)
	if err != nil {
		return uuid.Nil, err
	}
	if reminderCfg == nil {
		return id, nil
	}
	if _, err := s.repo.upsertReminder(ctx, tx, tenantID, id, *reminderCfg); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (s *Service) buildUpdateMutation(ctx context.Context, tx pgx.Tx, tenantID, taskID uuid.UUID, req UpdateTaskRequest) ([]string, []any, error) {
	setClauses := make([]string, 0, 6)
	args := []any{tenantID, taskID}
	argIndex := 3
	appendStringField := func(column string, value *string, transform func(string) string) {
		if value == nil {
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, argIndex))
		args = append(args, transform(*value))
		argIndex++
	}
	appendStringField("title", req.Title, strings.TrimSpace)
	appendStringField("description", req.Description, strings.TrimSpace)
	if req.Priority != nil {
		appendStringField("priority", req.Priority, normalizePriority)
	}
	if req.ClearDueAt {
		setClauses = append(setClauses, "due_at = NULL")
	} else if req.DueAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("due_at = $%d", argIndex))
		args = append(args, *req.DueAt)
		argIndex++
	}
	if err := s.appendAssignedUserMutation(ctx, tx, tenantID, req.AssignedUserID, &setClauses, &args, &argIndex); err != nil {
		return nil, nil, err
	}
	return setClauses, args, nil
}

func (s *Service) appendAssignedUserMutation(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, assignedUserIDRaw *string, setClauses *[]string, args *[]any, argIndex *int) error {
	if assignedUserIDRaw == nil {
		return nil
	}
	assignedUserID, err := uuid.Parse(strings.TrimSpace(*assignedUserIDRaw))
	if err != nil {
		return err
	}
	if _, err := s.repo.requireAssignee(ctx, tx, tenantID, assignedUserID); err != nil {
		return err
	}
	*setClauses = append(*setClauses, fmt.Sprintf("assigned_user_id = $%d", *argIndex))
	*args = append(*args, assignedUserID)
	*argIndex++
	return nil
}

func (s *Service) applyTaskFieldUpdates(ctx context.Context, tx pgx.Tx, setClauses []string, args []any) error {
	if len(setClauses) == 0 {
		return nil
	}
	return s.repo.updateTaskFields(ctx, tx, setClauses, args)
}

func (s *Service) applyReminderUpdate(ctx context.Context, tx pgx.Tx, tenantID, taskID uuid.UUID, req UpdateTaskRequest) error {
	if req.ClearReminder {
		return s.repo.disableReminder(ctx, tx, tenantID, taskID)
	}
	if req.Reminder == nil {
		return nil
	}
	reminderCfg, err := normalizeReminderConfig(req.Reminder)
	if err != nil {
		return err
	}
	if reminderCfg == nil {
		return s.repo.disableReminder(ctx, tx, tenantID, taskID)
	}
	_, err = s.repo.upsertReminder(ctx, tx, tenantID, taskID, *reminderCfg)
	return err
}

func (s *Service) loadProcessableReminder(ctx context.Context, reminderID uuid.UUID, scheduledFor time.Time) (*reminderProcessRecord, error) {
	rec, err := s.repo.getReminderForProcessing(ctx, reminderID)
	if err != nil {
		if err == errReminderNotFound {
			return nil, nil
		}
		return nil, err
	}
	if rec.Task.Status != StatusOpen || !rec.Reminder.Enabled || rec.Reminder.NextRunAt == nil {
		return nil, nil
	}
	if !rec.Reminder.NextRunAt.UTC().Equal(scheduledFor.UTC()) {
		return nil, nil
	}
	return &rec, nil
}

func nextReminderRunAt(reminder ReminderRecord) *time.Time {
	if !reminder.RepeatDaily || reminder.NextRunAt == nil {
		return nil
	}
	value := reminder.NextRunAt.Add(24 * time.Hour).UTC()
	return &value
}

func (s *Service) deliverTaskReminder(ctx context.Context, rec reminderProcessRecord) error {
	if err := s.insertTaskReminderEmail(ctx, rec); err != nil {
		return err
	}
	return s.insertTaskReminderWhatsApp(ctx, rec)
}

func (s *Service) insertTaskReminderEmail(ctx context.Context, rec reminderProcessRecord) error {
	if !rec.Reminder.SendEmail || strings.TrimSpace(rec.Task.AssigneeEmail) == "" {
		return nil
	}
	_, err := s.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
		TenantID:  rec.Task.TenantID,
		LeadID:    rec.Task.LeadID,
		ServiceID: rec.Task.LeadServiceID,
		Kind:      "email",
		Template:  "email_send",
		Payload: map[string]any{
			"orgId":    rec.Task.TenantID.String(),
			"toEmail":  rec.Task.AssigneeEmail,
			"subject":  buildReminderSubject(rec.Task),
			"bodyHtml": buildReminderEmailHTML(rec.Task),
		},
		RunAt: time.Now().UTC(),
	})
	return err
}

func (s *Service) insertTaskReminderWhatsApp(ctx context.Context, rec reminderProcessRecord) error {
	if !rec.Reminder.SendWhatsApp || rec.AssigneePhone == nil || strings.TrimSpace(*rec.AssigneePhone) == "" {
		return nil
	}
	_, err := s.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
		TenantID:  rec.Task.TenantID,
		LeadID:    rec.Task.LeadID,
		ServiceID: rec.Task.LeadServiceID,
		Kind:      "whatsapp",
		Template:  "whatsapp_send",
		Payload: map[string]any{
			"orgId":       rec.Task.TenantID.String(),
			"leadId":      nullableUUIDString(rec.Task.LeadID),
			"serviceId":   nullableUUIDString(rec.Task.LeadServiceID),
			"phoneNumber": phone.NormalizeE164(*rec.AssigneePhone),
			"message":     buildReminderWhatsAppMessage(rec.Task),
			"category":    "task_reminder",
			"audience":    "internal",
			"summary":     rec.Task.Title,
			"actorType":   "System",
			"actorName":   "Tasks",
		},
		RunAt: time.Now().UTC(),
	})
	return err
}

var _ scheduler.TaskReminderProcessor = (*Service)(nil)

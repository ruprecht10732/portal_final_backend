package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errTaskNotFound = errors.New("task not found")
var errReminderNotFound = errors.New("task reminder not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type assigneeContact struct {
	UserID     uuid.UUID
	Email      string
	Phone      *string
	FirstName  *string
	LastName   *string
}

type taskScanResult struct {
	Task         TaskRecord
	AssigneePhone *string
}

type taskScanFields struct {
	leadID                pgtype.UUID
	leadServiceID         pgtype.UUID
	dueAt                 pgtype.Timestamptz
	completedAt           pgtype.Timestamptz
	cancelledAt           pgtype.Timestamptz
	assigneeFirstName     sql.NullString
	assigneeLastName      sql.NullString
	reminderID            pgtype.UUID
	reminderEnabled       sql.NullBool
	reminderSendEmail     sql.NullBool
	reminderSendWhatsApp  sql.NullBool
	reminderNextRunAt     pgtype.Timestamptz
	reminderRepeatDaily   sql.NullBool
	reminderLastSentAt    pgtype.Timestamptz
	reminderLastTriggeredAt pgtype.Timestamptz
	reminderLastError     sql.NullString
	reminderCreatedAt     pgtype.Timestamptz
	reminderUpdatedAt     pgtype.Timestamptz
	phone                 sql.NullString
}

func (r *Repository) requireAssignee(ctx context.Context, tx pgx.Tx, tenantID, userID uuid.UUID) (assigneeContact, error) {
	const query = `
		SELECT u.id, u.email, u.phone, u.first_name, u.last_name
		FROM RAC_organization_members om
		JOIN RAC_users u ON u.id = om.user_id
		WHERE om.organization_id = $1 AND om.user_id = $2`

	var phone sql.NullString
	var firstName sql.NullString
	var lastName sql.NullString
	var contact assigneeContact
	if err := tx.QueryRow(ctx, query, tenantID, userID).Scan(&contact.UserID, &contact.Email, &phone, &firstName, &lastName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return assigneeContact{}, fmt.Errorf("assigned user is not a member of the organization")
		}
		return assigneeContact{}, err
	}
	if phone.Valid {
		contact.Phone = ptrString(phone.String)
	}
	if firstName.Valid {
		contact.FirstName = ptrString(firstName.String)
	}
	if lastName.Valid {
		contact.LastName = ptrString(lastName.String)
	}
	return contact, nil
}

func (r *Repository) validateLeadServiceScope(ctx context.Context, tx pgx.Tx, tenantID, leadID, leadServiceID uuid.UUID) error {
	const query = `
		SELECT 1
		FROM RAC_leads l
		JOIN RAC_lead_services s ON s.id = $3 AND s.lead_id = l.id
		WHERE l.organization_id = $1 AND l.id = $2`
		
	var marker int
	if err := tx.QueryRow(ctx, query, tenantID, leadID, leadServiceID).Scan(&marker); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lead service scope is invalid for the organization")
		}
		return err
	}
	return nil
}

func (r *Repository) insertTask(ctx context.Context, tx pgx.Tx, task TaskRecord) (uuid.UUID, error) {
	const query = `
		INSERT INTO RAC_tasks (
			tenant_id, scope_type, lead_id, lead_service_id, assigned_user_id, created_by_user_id,
			title, description, status, priority, due_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`

	var id uuid.UUID
	if err := tx.QueryRow(
		ctx,
		query,
		task.TenantID,
		task.ScopeType,
		task.LeadID,
		task.LeadServiceID,
		task.AssignedUserID,
		task.CreatedByUserID,
		task.Title,
		task.Description,
		task.Status,
		task.Priority,
		task.DueAt,
	).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *Repository) updateTaskFields(ctx context.Context, tx pgx.Tx, setClauses []string, args []any) error {
	query := `UPDATE RAC_tasks SET ` + strings.Join(setClauses, ", ") + `, updated_at = now() WHERE tenant_id = $1 AND id = $2`
	commandTag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errTaskNotFound
	}
	return nil
}

func (r *Repository) upsertReminder(ctx context.Context, tx pgx.Tx, tenantID, taskID uuid.UUID, cfg ReminderConfig) (uuid.UUID, error) {
	const query = `
		INSERT INTO RAC_task_reminders (
			task_id, tenant_id, enabled, send_email, send_whatsapp, next_run_at, repeat_daily, last_error, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULL, now())
		ON CONFLICT (task_id) DO UPDATE
		SET enabled = EXCLUDED.enabled,
		    send_email = EXCLUDED.send_email,
		    send_whatsapp = EXCLUDED.send_whatsapp,
		    next_run_at = EXCLUDED.next_run_at,
		    repeat_daily = EXCLUDED.repeat_daily,
		    last_error = NULL,
		    updated_at = now()
		RETURNING id`

	var id uuid.UUID
	if err := tx.QueryRow(ctx, query, taskID, tenantID, cfg.Enabled, cfg.SendEmail, cfg.SendWhatsApp, cfg.RunAt, cfg.RepeatDaily).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *Repository) disableReminder(ctx context.Context, tx pgx.Tx, tenantID, taskID uuid.UUID) error {
	const query = `
		UPDATE RAC_task_reminders
		SET enabled = false, next_run_at = NULL, updated_at = now()
		WHERE tenant_id = $1 AND task_id = $2`
	_, err := tx.Exec(ctx, query, tenantID, taskID)
	return err
}

func (r *Repository) markReminderError(ctx context.Context, reminderID uuid.UUID, message string) error {
	if r == nil || r.pool == nil {
		return nil
	}
	_, err := r.pool.Exec(ctx, `UPDATE RAC_task_reminders SET last_error = $2, updated_at = now() WHERE id = $1`, reminderID, message)
	return err
}

func (r *Repository) scanTask(row pgx.Row) (TaskRecord, error) {
	result, err := scanTaskResult(row, errTaskNotFound, false)
	if err != nil {
		return TaskRecord{}, err
	}
	return result.Task, nil
}

func (r *Repository) getTask(ctx context.Context, tenantID, taskID uuid.UUID) (TaskRecord, error) {
	const query = `
		SELECT
			t.id, t.tenant_id, t.scope_type, t.lead_id, t.lead_service_id, t.assigned_user_id,
			t.created_by_user_id, t.title, t.description, t.status, t.priority, t.due_at,
			t.completed_at, t.cancelled_at, t.created_at, t.updated_at,
			u.email, u.first_name, u.last_name,
			r.id, r.enabled, r.send_email, r.send_whatsapp, r.next_run_at, r.repeat_daily,
			r.last_sent_at, r.last_triggered_at, r.last_error, r.created_at, r.updated_at
		FROM RAC_tasks t
		JOIN RAC_users u ON u.id = t.assigned_user_id
		LEFT JOIN RAC_task_reminders r ON r.task_id = t.id
		WHERE t.tenant_id = $1 AND t.id = $2`
	return r.scanTask(r.pool.QueryRow(ctx, query, tenantID, taskID))
}

func (r *Repository) listTasks(ctx context.Context, tenantID uuid.UUID, filter listTasksFilter) ([]TaskRecord, error) {
	base := `
		SELECT
			t.id, t.tenant_id, t.scope_type, t.lead_id, t.lead_service_id, t.assigned_user_id,
			t.created_by_user_id, t.title, t.description, t.status, t.priority, t.due_at,
			t.completed_at, t.cancelled_at, t.created_at, t.updated_at,
			u.email, u.first_name, u.last_name,
			r.id, r.enabled, r.send_email, r.send_whatsapp, r.next_run_at, r.repeat_daily,
			r.last_sent_at, r.last_triggered_at, r.last_error, r.created_at, r.updated_at
		FROM RAC_tasks t
		JOIN RAC_users u ON u.id = t.assigned_user_id
		LEFT JOIN RAC_task_reminders r ON r.task_id = t.id
		WHERE t.tenant_id = $1`
	args := []any{tenantID}
	index := 2
	if filter.ScopeType != "" {
		base += fmt.Sprintf(" AND t.scope_type = $%d", index)
		args = append(args, filter.ScopeType)
		index++
	}
	if filter.Status != "" {
		base += fmt.Sprintf(" AND t.status = $%d", index)
		args = append(args, filter.Status)
		index++
	}
	if filter.AssignedUserID != nil {
		base += fmt.Sprintf(" AND t.assigned_user_id = $%d", index)
		args = append(args, *filter.AssignedUserID)
		index++
	}
	if filter.LeadID != nil {
		base += fmt.Sprintf(" AND t.lead_id = $%d", index)
		args = append(args, *filter.LeadID)
		index++
	}
	if filter.LeadServiceID != nil {
		base += fmt.Sprintf(" AND t.lead_service_id = $%d", index)
		args = append(args, *filter.LeadServiceID)
		index++
	}
	if filter.DueFrom != nil {
		base += fmt.Sprintf(" AND t.due_at >= $%d", index)
		args = append(args, *filter.DueFrom)
		index++
	}
	if filter.DueTo != nil {
		base += fmt.Sprintf(" AND t.due_at <= $%d", index)
		args = append(args, *filter.DueTo)
		index++
	}
	base += ` ORDER BY t.due_at NULLS LAST, t.created_at DESC`

	rows, err := r.pool.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TaskRecord, 0)
	for rows.Next() {
		item, scanErr := r.scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) getReminderForProcessing(ctx context.Context, reminderID uuid.UUID) (reminderProcessRecord, error) {
	const query = `
		SELECT
			t.id, t.tenant_id, t.scope_type, t.lead_id, t.lead_service_id, t.assigned_user_id,
			t.created_by_user_id, t.title, t.description, t.status, t.priority, t.due_at,
			t.completed_at, t.cancelled_at, t.created_at, t.updated_at,
			u.email, u.first_name, u.last_name,
			r.id, r.enabled, r.send_email, r.send_whatsapp, r.next_run_at, r.repeat_daily,
			r.last_sent_at, r.last_triggered_at, r.last_error, r.created_at, r.updated_at,
			u.phone
		FROM RAC_task_reminders r
		JOIN RAC_tasks t ON t.id = r.task_id
		JOIN RAC_users u ON u.id = t.assigned_user_id
		WHERE r.id = $1`

	row := r.pool.QueryRow(ctx, query, reminderID)
	result, err := scanTaskResult(row, errReminderNotFound, true)
	if err != nil {
		return reminderProcessRecord{}, err
	}
	rec := reminderProcessRecord{Task: result.Task, AssigneePhone: result.AssigneePhone}
	if result.Task.Reminder != nil {
		rec.Reminder = *result.Task.Reminder
	}
	return rec, nil
}

func (r *Repository) advanceReminderAfterTrigger(ctx context.Context, reminderID uuid.UUID, scheduledFor time.Time, nextRunAt *time.Time, lastError *string) error {
	const query = `
		UPDATE RAC_task_reminders
		SET last_sent_at = $2,
		    last_triggered_at = now(),
		    next_run_at = $3,
		    enabled = CASE WHEN $3::timestamptz IS NULL THEN false ELSE enabled END,
		    last_error = $4,
		    updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, reminderID, scheduledFor, nextRunAt, lastError)
	return err
}

func ptrString(value string) *string {
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func scanTaskResult(row pgx.Row, notFoundErr error, includePhone bool) (taskScanResult, error) {
	result := taskScanResult{}
	fields := taskScanFields{}
	if err := row.Scan(fields.scanArgs(&result.Task, includePhone)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return taskScanResult{}, notFoundErr
		}
		return taskScanResult{}, err
	}
	fields.apply(&result.Task)
	result.AssigneePhone = nullableStringPtr(fields.phone)
	return result, nil
}

func (f *taskScanFields) scanArgs(task *TaskRecord, includePhone bool) []any {
	args := []any{
		&task.ID,
		&task.TenantID,
		&task.ScopeType,
		&f.leadID,
		&f.leadServiceID,
		&task.AssignedUserID,
		&task.CreatedByUserID,
		&task.Title,
		&task.Description,
		&task.Status,
		&task.Priority,
		&f.dueAt,
		&f.completedAt,
		&f.cancelledAt,
		&task.CreatedAt,
		&task.UpdatedAt,
		&task.AssigneeEmail,
		&f.assigneeFirstName,
		&f.assigneeLastName,
		&f.reminderID,
		&f.reminderEnabled,
		&f.reminderSendEmail,
		&f.reminderSendWhatsApp,
		&f.reminderNextRunAt,
		&f.reminderRepeatDaily,
		&f.reminderLastSentAt,
		&f.reminderLastTriggeredAt,
		&f.reminderLastError,
		&f.reminderCreatedAt,
		&f.reminderUpdatedAt,
	}
	if includePhone {
		args = append(args, &f.phone)
	}
	return args
}

func (f *taskScanFields) apply(task *TaskRecord) {
	task.LeadID = nullableUUIDPtr(f.leadID)
	task.LeadServiceID = nullableUUIDPtr(f.leadServiceID)
	task.DueAt = nullableTimestampPtr(f.dueAt)
	task.CompletedAt = nullableTimestampPtr(f.completedAt)
	task.CancelledAt = nullableTimestampPtr(f.cancelledAt)
	task.AssigneeFirstName = nullableStringPtr(f.assigneeFirstName)
	task.AssigneeLastName = nullableStringPtr(f.assigneeLastName)
	task.Reminder = f.reminderForTask(*task)
}

func (f *taskScanFields) reminderForTask(task TaskRecord) *ReminderRecord {
	if !f.reminderID.Valid {
		return nil
	}
	return &ReminderRecord{
		ID:              uuid.UUID(f.reminderID.Bytes),
		TaskID:          task.ID,
		TenantID:        task.TenantID,
		Enabled:         f.reminderEnabled.Valid && f.reminderEnabled.Bool,
		SendEmail:       f.reminderSendEmail.Valid && f.reminderSendEmail.Bool,
		SendWhatsApp:    f.reminderSendWhatsApp.Valid && f.reminderSendWhatsApp.Bool,
		NextRunAt:       nullableTimestampPtr(f.reminderNextRunAt),
		RepeatDaily:     f.reminderRepeatDaily.Valid && f.reminderRepeatDaily.Bool,
		LastSentAt:      nullableTimestampPtr(f.reminderLastSentAt),
		LastTriggeredAt: nullableTimestampPtr(f.reminderLastTriggeredAt),
		LastError:       nullableStringPtr(f.reminderLastError),
		CreatedAt:       nullableTimestampValue(f.reminderCreatedAt),
		UpdatedAt:       nullableTimestampValue(f.reminderUpdatedAt),
	}
}

func nullableUUIDPtr(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	parsed := uuid.UUID(value.Bytes)
	return &parsed
}

func nullableTimestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed := value.Time.UTC()
	return &parsed
}

func nullableTimestampValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func nullableStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return ptrString(value.String)
}
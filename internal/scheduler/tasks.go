package scheduler

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TaskAppointmentReminder = "appointments.reminder"

const TaskNotificationOutboxDue = "notification.outbox.due"

type AppointmentReminderPayload struct {
	AppointmentID  string `json:"appointmentId"`
	OrganizationID string `json:"organizationId"`
}

type NotificationOutboxDuePayload struct {
	OutboxID string `json:"outboxId"`
	TenantID string `json:"tenantId"`
}

func NewAppointmentReminderTask(payload AppointmentReminderPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAppointmentReminder, data), nil
}

func ParseAppointmentReminderPayload(task *asynq.Task) (AppointmentReminderPayload, error) {
	var payload AppointmentReminderPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return AppointmentReminderPayload{}, err
	}
	return payload, nil
}

func NewNotificationOutboxDueTask(payload NotificationOutboxDuePayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskNotificationOutboxDue, data), nil
}

func ParseNotificationOutboxDuePayload(task *asynq.Task) (NotificationOutboxDuePayload, error) {
	var payload NotificationOutboxDuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return NotificationOutboxDuePayload{}, err
	}
	return payload, nil
}

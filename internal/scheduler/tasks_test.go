package scheduler

import "testing"

const testTaskReminderID = "reminder-1"
const testTaskReminderScheduledFor = "2026-03-17T09:30:00Z"

func TestNewTaskReminderTaskRoundTripsPayload(t *testing.T) {
	task, err := NewTaskReminderTask(TaskReminderPayload{
		ReminderID:   testTaskReminderID,
		ScheduledFor: testTaskReminderScheduledFor,
	})
	if err != nil {
		t.Fatalf("expected task reminder task to marshal, got %v", err)
	}

	payload, err := ParseTaskReminderPayload(task)
	if err != nil {
		t.Fatalf("expected task reminder payload to parse, got %v", err)
	}

	if payload.ReminderID != testTaskReminderID || payload.ScheduledFor != testTaskReminderScheduledFor {
		t.Fatalf("unexpected task reminder payload: %#v", payload)
	}
}

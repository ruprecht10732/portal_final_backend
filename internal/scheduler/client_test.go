package scheduler

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

func TestNormalizeEnqueueErrorIgnoresDuplicateTask(t *testing.T) {
	t.Parallel()

	if err := normalizeEnqueueError(asynq.ErrDuplicateTask); err != nil {
		t.Fatalf("expected duplicate task error to be ignored, got %v", err)
	}
}

func TestNormalizeEnqueueErrorPreservesOtherFailures(t *testing.T) {
	t.Parallel()

	expected := errors.New("redis unavailable")
	if err := normalizeEnqueueError(expected); !errors.Is(err, expected) {
		t.Fatalf("expected original error to be preserved, got %v", err)
	}
}

func TestWAAgentVoiceTranscriptionTaskRoundTrip(t *testing.T) {
	t.Parallel()

	payload := WAAgentVoiceTranscriptionPayload{
		OrganizationID:    "org-1",
		PhoneNumber:       "+31612345678",
		ExternalMessageID: "msg-42",
		RequestID:         "req-1",
		TraceID:           "trace-1",
	}
	task, err := NewWAAgentVoiceTranscriptionTask(payload)
	if err != nil {
		t.Fatalf("NewWAAgentVoiceTranscriptionTask returned error: %v", err)
	}
	if task.Type() != TaskWAAgentVoiceTranscription {
		t.Fatalf("expected task type %q, got %q", TaskWAAgentVoiceTranscription, task.Type())
	}

	parsed, err := ParseWAAgentVoiceTranscriptionPayload(task)
	if err != nil {
		t.Fatalf("ParseWAAgentVoiceTranscriptionPayload returned error: %v", err)
	}
	if parsed != payload {
		raw, _ := json.Marshal(parsed)
		t.Fatalf("unexpected parsed payload: %s", raw)
	}
}

package scheduler

import (
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

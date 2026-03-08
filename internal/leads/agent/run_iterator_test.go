package agent

import (
	"errors"
	"iter"
	"strings"
	"testing"
)

func TestConsumeRunEventsReturnsWrappedIteratorError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("moonshot unauthorized")
	err := consumeRunEvents(iter.Seq2[int, error](func(yield func(int, error) bool) {
		yield(0, sentinel)
	}), "gatekeeper run failed", nil)
	if err == nil {
		t.Fatal("expected iterator error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
	if !strings.Contains(err.Error(), "gatekeeper run failed") {
		t.Fatalf("expected wrapped error to contain context, got %q", err.Error())
	}
}

func TestConsumeRunEventsProcessesEventsUntilIteratorError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("moonshot timeout")
	handled := make([]int, 0, 2)
	err := consumeRunEvents(iter.Seq2[int, error](func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		if !yield(2, nil) {
			return
		}
		yield(0, sentinel)
	}), "dispatcher run failed", func(event int) {
		handled = append(handled, event)
	})
	if err == nil {
		t.Fatal("expected iterator error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
	if len(handled) != 2 || handled[0] != 1 || handled[1] != 2 {
		t.Fatalf("expected processed events [1 2], got %v", handled)
	}
}

func TestConsumeRunEventsAllowsNilHandler(t *testing.T) {
	t.Parallel()

	err := consumeRunEvents(iter.Seq2[int, error](func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		yield(2, nil)
	}), "auditor run failed", nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

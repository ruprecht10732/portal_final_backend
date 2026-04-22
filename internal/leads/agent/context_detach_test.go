package agent

import (
	"context"
	"testing"
	"time"
)

func TestDetachedTimeout_ParentDeadlineCancelsChild(t *testing.T) {
	// Simulate an already-expired parent context (e.g. after a long LLM call).
	parent, parentCancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer parentCancel()

	if parent.Err() == nil {
		t.Fatal("parent context should already be expired")
	}

	child, childCancel := detachedTimeout(parent, 5*time.Second)
	defer childCancel()

	// The child should be cancelled promptly when the parent expires,
	// so the timer is released and does not leak.
	select {
	case <-child.Done():
		if child.Err() != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", child.Err())
		}
	case <-time.After(1 * time.Second):
		t.Fatal("detached child context should have been cancelled after parent deadline exceeded")
	}
}

func TestDetachedTimeout_ExplicitCancelPropagates(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	child, childCancel := detachedTimeout(parent, 5*time.Second)
	defer childCancel()

	// Explicitly cancel the parent.
	parentCancel()

	// The child should be cancelled shortly after.
	select {
	case <-child.Done():
		if child.Err() != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", child.Err())
		}
	case <-time.After(1 * time.Second):
		t.Fatal("detached child context should have been cancelled after parent was explicitly cancelled")
	}
}

func TestDetachedTimeout_OwnDeadlineApplies(t *testing.T) {
	parent := context.Background()
	child, childCancel := detachedTimeout(parent, 50*time.Millisecond)
	defer childCancel()

	select {
	case <-child.Done():
		if child.Err() != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", child.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("detached child context should have expired from its own timeout")
	}
}

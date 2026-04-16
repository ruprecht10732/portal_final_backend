package agent

import (
	"context"
	"time"
)

// detachedTimeout creates a context with its own timeout that is independent of the
// parent's deadline. This prevents an already-expired parent deadline (e.g. from a
// long-running LLM call consuming the time budget) from immediately cancelling
// downstream I/O operations that have their own time budget.
//
// Explicit cancellation (context.Canceled) from the parent is still propagated so
// that deliberate shutdowns stop the child operation.
func detachedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	child, cancel := context.WithTimeout(context.Background(), timeout)
	go func() {
		select {
		case <-parent.Done():
			if parent.Err() == context.Canceled {
				cancel()
			}
		case <-child.Done():
		}
	}()
	return child, cancel
}

package client

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestIsEOFErrorRecognizesWrappedEOF(t *testing.T) {
	t.Parallel()

	err := errors.Join(io.EOF, errors.New("read failed"))
	if !isEOFError(err) {
		t.Fatal("expected wrapped EOF to be detected")
	}
}

func TestIsTransientIMAPErrorRecognizesConnectionReset(t *testing.T) {
	t.Parallel()

	err := &net.OpError{Err: syscall.ECONNRESET}
	if !isTransientIMAPError(err) {
		t.Fatal("expected connection reset to be transient")
	}
}

func TestIsTransientIMAPErrorRecognizesTimeout(t *testing.T) {
	t.Parallel()

	err := &net.DNSError{IsTimeout: true}
	if !isTransientIMAPError(err) {
		t.Fatal("expected timeout to be transient")
	}
}

func TestIsTransientIMAPErrorDoesNotTreatCancellationAsTransient(t *testing.T) {
	t.Parallel()

	if isTransientIMAPError(context.Canceled) {
		t.Fatal("expected context cancellation not to be transient")
	}
	if isTransientIMAPError(context.DeadlineExceeded) {
		t.Fatal("expected deadline exceeded not to be transient")
	}
}

func TestWaitForRetryReturnsContextErrorWhenCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForRetry(ctx, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestContextErrorAllowsNilContext(t *testing.T) {
	t.Parallel()

	if err := contextError(nil); err != nil {
		t.Fatalf("expected nil context to be allowed, got %v", err)
	}
}

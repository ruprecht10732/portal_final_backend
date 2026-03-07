package service

import (
	"context"
	"errors"
	"testing"

	"portal_final_backend/internal/imap/repository"
	"portal_final_backend/internal/scheduler"

	"github.com/google/uuid"
)

type testIMAPScheduler struct {
	payloads []scheduler.IMAPSyncAccountPayload
	err      error
}

func (s *testIMAPScheduler) EnqueueIMAPSyncAccount(_ context.Context, payload scheduler.IMAPSyncAccountPayload) error {
	if s.err != nil {
		return s.err
	}
	s.payloads = append(s.payloads, payload)
	return nil
}

func TestEnqueueAccountSyncUsesSchedulerPayload(t *testing.T) {
	t.Parallel()

	account := repository.Account{ID: uuid.New(), UserID: uuid.New()}
	scheduler := &testIMAPScheduler{}
	svc := &Service{scheduler: scheduler}

	enqueued := svc.enqueueAccountSync(context.Background(), account)
	if !enqueued {
		t.Fatal("expected account sync to be enqueued")
	}
	if len(scheduler.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(scheduler.payloads))
	}
	if scheduler.payloads[0].AccountID != account.ID.String() {
		t.Fatalf("expected account id %q, got %q", account.ID.String(), scheduler.payloads[0].AccountID)
	}
	if scheduler.payloads[0].UserID != account.UserID.String() {
		t.Fatalf("expected user id %q, got %q", account.UserID.String(), scheduler.payloads[0].UserID)
	}
}

func TestEnqueueAccountSyncFallsBackWhenSchedulerFails(t *testing.T) {
	t.Parallel()

	account := repository.Account{ID: uuid.New(), UserID: uuid.New()}
	svc := &Service{scheduler: &testIMAPScheduler{err: errors.New("queue unavailable")}}

	enqueued := svc.enqueueAccountSync(context.Background(), account)
	if enqueued {
		t.Fatal("expected enqueueAccountSync to report fallback when scheduler fails")
	}
}

func TestSyncAccountSkipsWhenAccountAlreadyLocked(t *testing.T) {
	t.Parallel()

	account := repository.Account{ID: uuid.New(), UserID: uuid.New(), FolderName: "INBOX"}
	svc := &Service{}
	if !svc.tryLock(account.ID) {
		t.Fatal("expected initial lock acquisition to succeed")
	}
	defer svc.unlock(account.ID)

	if err := svc.syncAccount(context.Background(), account); err != nil {
		t.Fatalf("syncAccount returned error for overlapping sync: %v", err)
	}
}

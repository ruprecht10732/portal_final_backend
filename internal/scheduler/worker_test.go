package scheduler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type testIMAPProcessor struct {
	executedAccountID uuid.UUID
	executedUserID    uuid.UUID
	executeCalls      int
	syncCalls         int
	sweepCalls        int
}

func (p *testIMAPProcessor) SyncAccount(context.Context, uuid.UUID, uuid.UUID) error {
	p.syncCalls++
	return nil
}

func (p *testIMAPProcessor) ExecuteAccountSync(_ context.Context, userID uuid.UUID, accountID uuid.UUID) error {
	p.executeCalls++
	p.executedUserID = userID
	p.executedAccountID = accountID
	return nil
}

func (p *testIMAPProcessor) SyncEligibleAccounts(context.Context) error {
	p.sweepCalls++
	return nil
}

func TestHandleIMAPSyncAccountExecutesDirectSyncPath(t *testing.T) {
	t.Parallel()

	processor := &testIMAPProcessor{}
	worker := &Worker{imap: processor}
	accountID := uuid.New()
	userID := uuid.New()
	task, err := NewIMAPSyncAccountTask(IMAPSyncAccountPayload{
		AccountID: accountID.String(),
		UserID:    userID.String(),
	})
	if err != nil {
		t.Fatalf("NewIMAPSyncAccountTask returned error: %v", err)
	}

	if err := worker.handleIMAPSyncAccount(context.Background(), task); err != nil {
		t.Fatalf("handleIMAPSyncAccount returned error: %v", err)
	}
	if processor.executeCalls != 1 {
		t.Fatalf("expected execute path to be called once, got %d", processor.executeCalls)
	}
	if processor.syncCalls != 0 {
		t.Fatalf("expected public sync path not to be used, got %d calls", processor.syncCalls)
	}
	if processor.executedAccountID != accountID {
		t.Fatalf("expected account id %q, got %q", accountID, processor.executedAccountID)
	}
	if processor.executedUserID != userID {
		t.Fatalf("expected user id %q, got %q", userID, processor.executedUserID)
	}
}

func TestHandleIMAPSyncSweepCallsSweepProcessor(t *testing.T) {
	t.Parallel()

	processor := &testIMAPProcessor{}
	worker := &Worker{imap: processor}

	if err := worker.handleIMAPSyncSweep(context.Background(), asynq.NewTask(TaskIMAPSyncSweep, nil)); err != nil {
		t.Fatalf("handleIMAPSyncSweep returned error: %v", err)
	}
	if processor.sweepCalls != 1 {
		t.Fatalf("expected sweep path to be called once, got %d", processor.sweepCalls)
	}
}

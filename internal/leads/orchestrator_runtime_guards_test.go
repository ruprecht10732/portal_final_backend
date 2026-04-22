package leads

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"portal_final_backend/internal/events"
	"portal_final_backend/platform/logger"
)

const miniredisStartFailedMsg = "failed to start miniredis: %v"

func newGuardOnlyOrchestrator(locker orchestratorRunLocker) *Orchestrator {
	if locker == nil {
		locker = newInMemoryOrchestratorRunLocker()
	}

	return &Orchestrator{
		runLocker:         locker,
		recentStageEvents: make(map[string]time.Time),
		orgSettingsCache:  make(map[uuid.UUID]cachedOrgAISettings),
		log:               logger.New("development"),
	}
}

func TestMarkReconciliationRunning(t *testing.T) {
	o := newGuardOnlyOrchestrator(nil)
	serviceID := uuid.New()

	ctx := context.Background()
	if !o.markReconciliationRunning(ctx, serviceID) {
		t.Fatalf("expected first reconciliation lock acquisition to succeed")
	}
	if o.markReconciliationRunning(ctx, serviceID) {
		t.Fatalf("expected second reconciliation lock acquisition to fail")
	}

	o.markReconciliationComplete(ctx, serviceID)

	if !o.markReconciliationRunning(ctx, serviceID) {
		t.Fatalf("expected lock acquisition to succeed after completion")
	}
}

func TestShouldSkipDuplicateStageEvent(t *testing.T) {
	o := newGuardOnlyOrchestrator(nil)
	serviceID := uuid.New()
	evt := events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        uuid.New(),
		LeadServiceID: serviceID,
		TenantID:      uuid.New(),
		OldStage:      "Triage",
		NewStage:      "Estimation",
	}

	if o.shouldSkipDuplicateStageEvent(evt) {
		t.Fatalf("expected first stage event to be accepted")
	}
	if !o.shouldSkipDuplicateStageEvent(evt) {
		t.Fatalf("expected immediate duplicate stage event to be skipped")
	}

	key := evt.LeadServiceID.String() + ":" + evt.OldStage + "->" + evt.NewStage
	o.recentStageEvents[key] = time.Now().Add(-stageEventDedupWindow - time.Second)

	if o.shouldSkipDuplicateStageEvent(evt) {
		t.Fatalf("expected stage event to be accepted after dedupe window elapsed")
	}
}

func TestRedisReconciliationLocksAreSharedAcrossOrchestrators(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(miniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()

	o1 := newGuardOnlyOrchestrator(newRedisOrchestratorRunLocker(redisClient, reconciliationLockTimeout))
	o2 := newGuardOnlyOrchestrator(newRedisOrchestratorRunLocker(redisClient, reconciliationLockTimeout))
	serviceID := uuid.New()

	ctx := context.Background()
	if !o1.markReconciliationRunning(ctx, serviceID) {
		t.Fatalf("expected first reconciliation lock acquisition to succeed")
	}
	if o2.markReconciliationRunning(ctx, serviceID) {
		t.Fatalf("expected second orchestrator to observe the shared reconciliation lock")
	}

	reconcileKey := reconciliationLockKey(serviceID)
	if ttl := redisClient.TTL(ctx, reconcileKey).Val(); ttl <= 0 || ttl > reconciliationLockTimeout {
		t.Fatalf("expected redis reconciliation lock TTL within (0,%s], got %s", reconciliationLockTimeout, ttl)
	}

	o1.markReconciliationComplete(ctx, serviceID)

	if exists := redisClient.Exists(ctx, reconcileKey).Val(); exists != 0 {
		t.Fatalf("expected redis lock keys to be released, found %d keys", exists)
	}
	if !o2.markReconciliationRunning(ctx, serviceID) {
		t.Fatalf("expected reconciliation lock acquisition to succeed after redis release")
	}
}

func TestRedisReconciliationLockReleaseDoesNotDeleteDifferentOwner(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(miniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()

	ctx := context.Background()
	serviceID := uuid.New()
	key := reconciliationLockKey(serviceID)
	locker := newRedisOrchestratorRunLocker(redisClient, time.Second).(*redisOrchestratorRunLocker)

	ok, err := locker.TryAcquireReconciliation(ctx, serviceID)
	if err != nil {
		t.Fatalf("expected redis lock acquisition without error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected redis lock acquisition to succeed")
	}

	if err := redisClient.Set(ctx, key, "different-owner", time.Second).Err(); err != nil {
		t.Fatalf("expected test overwrite to succeed, got %v", err)
	}
	if err := locker.ReleaseReconciliation(ctx, serviceID); err != nil {
		t.Fatalf("expected release to succeed, got %v", err)
	}

	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("expected replacement owner value to remain, got error %v", err)
	}
	if value != "different-owner" {
		t.Fatalf("expected replacement owner value to remain, got %q", value)
	}
}

func TestRedisReconciliationLockHeartbeatKeepsKeyAlivePastOriginalTTL(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(miniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()

	ctx := context.Background()
	ttl := 300 * time.Millisecond
	serviceID := uuid.New()
	key := reconciliationLockKey(serviceID)
	locker := newRedisOrchestratorRunLocker(redisClient, ttl).(*redisOrchestratorRunLocker)

	ok, err := locker.TryAcquireReconciliation(ctx, serviceID)
	if err != nil {
		t.Fatalf("expected redis lock acquisition without error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected redis lock acquisition to succeed")
	}

	time.Sleep(ttl + 250*time.Millisecond)

	if exists := redisClient.Exists(ctx, key).Val(); exists != 1 {
		t.Fatalf("expected heartbeat to keep redis lock alive, found %d keys", exists)
	}
	if remaining := redisClient.PTTL(ctx, key).Val(); remaining <= 0 {
		t.Fatalf("expected redis lock to have positive remaining TTL, got %s", remaining)
	}

	if err := locker.ReleaseReconciliation(ctx, serviceID); err != nil {
		t.Fatalf("expected redis lock release without error, got %v", err)
	}
}

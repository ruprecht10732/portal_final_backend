package leads

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"portal_final_backend/platform/logger"
)

const orchestratorLockPrefix = "leads:orchestrator"

type orchestratorRunLocker interface {
	TryAcquireAgentRun(agentName string, serviceID uuid.UUID) (bool, error)
	ReleaseAgentRun(agentName string, serviceID uuid.UUID) error
	TryAcquireReconciliation(serviceID uuid.UUID) (bool, error)
	ReleaseReconciliation(serviceID uuid.UUID) error
}

func newOrchestratorRunLocker(redisClient *redis.Client, log *logger.Logger) orchestratorRunLocker {
	if redisClient == nil {
		if log != nil {
			log.Warn("orchestrator: redis unavailable, using in-memory run locks")
		}
		return newInMemoryOrchestratorRunLocker()
	}

	if log != nil {
		log.Info("orchestrator: redis-backed run locks enabled", "ttl", agentRunTimeout)
	}
	return newRedisOrchestratorRunLocker(redisClient, agentRunTimeout)
}

func agentRunLockKey(agentName string, serviceID uuid.UUID) string {
	return fmt.Sprintf("%s:agent:%s:%s", orchestratorLockPrefix, agentName, serviceID)
}

func reconciliationLockKey(serviceID uuid.UUID) string {
	return fmt.Sprintf("%s:reconcile:%s", orchestratorLockPrefix, serviceID)
}

type inMemoryOrchestratorRunLocker struct {
	mu                    sync.Mutex
	activeRuns            map[string]bool
	activeReconciliations map[uuid.UUID]bool
}

func newInMemoryOrchestratorRunLocker() orchestratorRunLocker {
	return &inMemoryOrchestratorRunLocker{
		activeRuns:            make(map[string]bool),
		activeReconciliations: make(map[uuid.UUID]bool),
	}
}

func (l *inMemoryOrchestratorRunLocker) TryAcquireAgentRun(agentName string, serviceID uuid.UUID) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := agentRunLockKey(agentName, serviceID)
	if l.activeRuns[key] {
		return false, nil
	}
	l.activeRuns[key] = true
	return true, nil
}

func (l *inMemoryOrchestratorRunLocker) ReleaseAgentRun(agentName string, serviceID uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.activeRuns, agentRunLockKey(agentName, serviceID))
	return nil
}

func (l *inMemoryOrchestratorRunLocker) TryAcquireReconciliation(serviceID uuid.UUID) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.activeReconciliations[serviceID] {
		return false, nil
	}
	l.activeReconciliations[serviceID] = true
	return true, nil
}

func (l *inMemoryOrchestratorRunLocker) ReleaseReconciliation(serviceID uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.activeReconciliations, serviceID)
	return nil
}

type redisOrchestratorRunLocker struct {
	client *redis.Client
	ttl    time.Duration
	owners sync.Map
}

func newRedisOrchestratorRunLocker(client *redis.Client, ttl time.Duration) orchestratorRunLocker {
	return &redisOrchestratorRunLocker{
		client: client,
		ttl:    ttl,
	}
}

func (l *redisOrchestratorRunLocker) TryAcquireAgentRun(agentName string, serviceID uuid.UUID) (bool, error) {
	return l.tryAcquire(agentRunLockKey(agentName, serviceID))
}

func (l *redisOrchestratorRunLocker) ReleaseAgentRun(agentName string, serviceID uuid.UUID) error {
	return l.release(agentRunLockKey(agentName, serviceID))
}

func (l *redisOrchestratorRunLocker) TryAcquireReconciliation(serviceID uuid.UUID) (bool, error) {
	return l.tryAcquire(reconciliationLockKey(serviceID))
}

func (l *redisOrchestratorRunLocker) ReleaseReconciliation(serviceID uuid.UUID) error {
	return l.release(reconciliationLockKey(serviceID))
}

func (l *redisOrchestratorRunLocker) tryAcquire(key string) (bool, error) {
	token := uuid.NewString()
	ok, err := l.client.SetNX(context.Background(), key, token, l.ttl).Result()
	if !ok || err != nil {
		return ok, err
	}
	l.owners.Store(key, token)
	return true, nil
}

func (l *redisOrchestratorRunLocker) release(key string) error {
	l.owners.Delete(key)
	return l.client.Del(context.Background(), key).Err()
}

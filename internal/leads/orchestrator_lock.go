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

const minimumRedisLockRenewInterval = 100 * time.Millisecond

var compareAndDeleteRedisLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

var compareAndExpireRedisLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

type orchestratorRunLocker interface {
	TryAcquireReconciliation(ctx context.Context, serviceID uuid.UUID) (bool, error)
	ReleaseReconciliation(ctx context.Context, serviceID uuid.UUID) error
}

const reconciliationLockTimeout = 5 * time.Minute

func newOrchestratorRunLocker(redisClient *redis.Client, log *logger.Logger) orchestratorRunLocker {
	if redisClient == nil {
		if log != nil {
			log.Warn("orchestrator: redis unavailable, using in-memory reconciliation locks")
		}
		return newInMemoryOrchestratorRunLocker()
	}

	if log != nil {
		log.Info("orchestrator: redis-backed reconciliation locks enabled", "ttl", reconciliationLockTimeout)
	}
	return newRedisOrchestratorRunLocker(redisClient, reconciliationLockTimeout, log)
}

func reconciliationLockKey(serviceID uuid.UUID) string {
	return fmt.Sprintf("%s:reconcile:%s", orchestratorLockPrefix, serviceID)
}

type inMemoryOrchestratorRunLocker struct {
	mu                    sync.Mutex
	activeReconciliations map[uuid.UUID]bool
}

func newInMemoryOrchestratorRunLocker() orchestratorRunLocker {
	return &inMemoryOrchestratorRunLocker{
		activeReconciliations: make(map[uuid.UUID]bool),
	}
}

func (l *inMemoryOrchestratorRunLocker) TryAcquireReconciliation(ctx context.Context, serviceID uuid.UUID) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.activeReconciliations[serviceID] {
		return false, nil
	}
	l.activeReconciliations[serviceID] = true
	return true, nil
}

func (l *inMemoryOrchestratorRunLocker) ReleaseReconciliation(ctx context.Context, serviceID uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.activeReconciliations, serviceID)
	return nil
}

type redisOrchestratorRunLocker struct {
	client *redis.Client
	ttl    time.Duration
	log    *logger.Logger
	owners sync.Map
}

type redisLockOwner struct {
	token  string
	cancel context.CancelFunc
}

func newRedisOrchestratorRunLocker(client *redis.Client, ttl time.Duration, logs ...*logger.Logger) orchestratorRunLocker {
	var log *logger.Logger
	if len(logs) > 0 {
		log = logs[0]
	}

	return &redisOrchestratorRunLocker{
		client: client,
		ttl:    ttl,
		log:    log,
	}
}

func (l *redisOrchestratorRunLocker) TryAcquireReconciliation(ctx context.Context, serviceID uuid.UUID) (bool, error) {
	return l.tryAcquire(ctx, reconciliationLockKey(serviceID))
}

func (l *redisOrchestratorRunLocker) ReleaseReconciliation(ctx context.Context, serviceID uuid.UUID) error {
	return l.release(ctx, reconciliationLockKey(serviceID))
}

func (l *redisOrchestratorRunLocker) tryAcquire(ctx context.Context, key string) (bool, error) {
	token := uuid.NewString()
	ok, err := l.client.SetNX(ctx, key, token, l.ttl).Result()
	if !ok || err != nil {
		return ok, err
	}
	l.owners.Store(key, redisLockOwner{
		token:  token,
		cancel: l.startRenewal(key, token),
	})
	return true, nil
}

func (l *redisOrchestratorRunLocker) release(ctx context.Context, key string) error {
	ownerValue, ok := l.owners.LoadAndDelete(key)
	if !ok {
		return nil
	}

	owner, ok := ownerValue.(redisLockOwner)
	if !ok {
		return nil
	}
	owner.cancel()

	_, err := compareAndDeleteRedisLockScript.Run(ctx, l.client, []string{key}, owner.token).Result()
	return err
}

func (l *redisOrchestratorRunLocker) startRenewal(key, token string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	interval := l.renewInterval()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !l.renewKey(key, token) {
					return
				}
			}
		}
	}()

	return cancel
}

func (l *redisOrchestratorRunLocker) renewInterval() time.Duration {
	interval := l.ttl / 3
	if interval < minimumRedisLockRenewInterval {
		return minimumRedisLockRenewInterval
	}
	return interval
}

func (l *redisOrchestratorRunLocker) renewKey(key, token string) bool {
	result, err := compareAndExpireRedisLockScript.Run(
		context.Background(),
		l.client,
		[]string{key},
		token,
		l.ttl.Milliseconds(),
	).Int64()
	if err != nil {
		l.logRenewalError(key, err)
		return true
	}
	if result != 0 {
		return true
	}

	l.owners.Delete(key)
	l.logRenewalOwnershipLoss(key)
	return false
}

func (l *redisOrchestratorRunLocker) logRenewalError(key string, err error) {
	if l.log != nil {
		l.log.Warn("orchestrator: failed to renew redis lock", "error", err, "key", key)
	}
}

func (l *redisOrchestratorRunLocker) logRenewalOwnershipLoss(key string) {
	if l.log != nil {
		l.log.Warn("orchestrator: redis lock renewal stopped because ownership was lost", "key", key)
	}
}

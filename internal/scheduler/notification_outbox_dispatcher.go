package scheduler

import (
	"context"
	"fmt"
	"time"

	"portal_final_backend/internal/notification/outbox"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationOutboxDispatcher struct {
	client *asynq.Client
	queue  string
	repo   *outbox.Repository
	log    *logger.Logger
}

func NewNotificationOutboxDispatcher(cfg config.SchedulerConfig, pool *pgxpool.Pool, log *logger.Logger) (*NotificationOutboxDispatcher, error) {
	redisURL := cfg.GetRedisURL()
	if redisURL == "" {
		return nil, fmt.Errorf("redis url not configured")
	}

	opt, err := redisClientOpt(redisURL, cfg.GetRedisTLSInsecure())
	if err != nil {
		return nil, err
	}

	queue := cfg.GetAsynqQueueName()
	if queue == "" {
		queue = "default"
	}

	return &NotificationOutboxDispatcher{
		client: asynq.NewClient(opt),
		queue:  queue,
		repo:   outbox.New(pool),
		log:    log,
	}, nil
}

func (d *NotificationOutboxDispatcher) Close() error {
	if d == nil || d.client == nil {
		return nil
	}
	return d.client.Close()
}

func (d *NotificationOutboxDispatcher) Run(ctx context.Context) {
	if d == nil || d.client == nil || d.repo == nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		records, err := d.repo.ClaimPending(ctx, 50)
		if err != nil {
			d.log.Warn("outbox claim failed", "error", err)
			continue
		}
		if len(records) == 0 {
			continue
		}

		for _, rec := range records {
			task, err := NewNotificationOutboxDueTask(NotificationOutboxDuePayload{
				OutboxID: rec.ID.String(),
				TenantID: rec.TenantID.String(),
			})
			if err != nil {
				msg := err.Error()
				_ = d.repo.MarkPending(ctx, rec.ID, &msg)
				continue
			}

			_, err = d.client.EnqueueContext(ctx, task, asynq.ProcessAt(rec.RunAt), asynq.Queue(d.queue))
			if err != nil {
				msg := err.Error()
				_ = d.repo.MarkPending(ctx, rec.ID, &msg)
				continue
			}
		}
	}
}

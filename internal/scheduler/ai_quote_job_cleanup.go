package scheduler

import (
	"context"
	"time"

	quotesrepo "portal_final_backend/internal/quotes/repository"
	"portal_final_backend/platform/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultAIQuoteJobCleanupInterval = time.Hour
	defaultCompletedJobRetention     = 14 * 24 * time.Hour
	defaultFailedJobRetention        = 30 * 24 * time.Hour
)

// AIQuoteJobCleanup periodically removes old finished AI quote jobs.
type AIQuoteJobCleanup struct {
	repo               *quotesrepo.Repository
	log                *logger.Logger
	interval           time.Duration
	completedRetention time.Duration
	failedRetention    time.Duration
}

func NewAIQuoteJobCleanup(pool *pgxpool.Pool, log *logger.Logger, interval, completedRetention, failedRetention time.Duration) *AIQuoteJobCleanup {
	if interval <= 0 {
		interval = defaultAIQuoteJobCleanupInterval
	}
	if completedRetention <= 0 {
		completedRetention = defaultCompletedJobRetention
	}
	if failedRetention <= 0 {
		failedRetention = defaultFailedJobRetention
	}

	return &AIQuoteJobCleanup{
		repo:               quotesrepo.New(pool),
		log:                log,
		interval:           interval,
		completedRetention: completedRetention,
		failedRetention:    failedRetention,
	}
}

func (c *AIQuoteJobCleanup) Run(ctx context.Context) {
	if c == nil || c.repo == nil {
		return
	}

	c.cleanup(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *AIQuoteJobCleanup) cleanup(ctx context.Context) {
	now := time.Now()
	completedBefore := now.Add(-c.completedRetention)
	failedBefore := now.Add(-c.failedRetention)

	deleted, err := c.repo.DeleteFinishedGenerateQuoteJobsBefore(ctx, completedBefore, failedBefore)
	if err != nil {
		c.log.Warn("ai quote job cleanup failed", "error", err)
		return
	}

	if deleted > 0 {
		c.log.Info("ai quote job cleanup deleted finished jobs", "deleted", deleted)
	}
}

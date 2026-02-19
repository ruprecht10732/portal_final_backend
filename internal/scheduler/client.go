package scheduler

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"portal_final_backend/platform/config"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	client *asynq.Client
	queue  string
}

type ReminderScheduler interface {
	ScheduleAppointmentReminder(ctx context.Context, payload AppointmentReminderPayload, runAt time.Time) error
}

type QuoteJobScheduler interface {
	EnqueueGenerateQuoteJob(ctx context.Context, payload GenerateQuoteJobPayload) error
}

type QuoteJobRunner interface {
	EnqueueGenerateQuoteJobRequest(ctx context.Context, req GenerateQuoteJobRequest) error
}

// GenerateQuoteJobRequest groups parameters for enqueueing a quote generation job.
// This keeps the scheduler API ergonomic while avoiding long parameter lists.
type GenerateQuoteJobRequest struct {
	JobID         uuid.UUID
	TenantID      uuid.UUID
	UserID        uuid.UUID
	LeadID        uuid.UUID
	LeadServiceID uuid.UUID
	Prompt        string
	QuoteID       *uuid.UUID
}

func NewClient(cfg config.SchedulerConfig) (*Client, error) {
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

	return &Client{
		client: asynq.NewClient(opt),
		queue:  queue,
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *Client) ScheduleAppointmentReminder(ctx context.Context, payload AppointmentReminderPayload, runAt time.Time) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewAppointmentReminderTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, asynq.ProcessAt(runAt), asynq.Queue(c.queue))
	return err
}

func (c *Client) EnqueueGenerateQuoteJob(ctx context.Context, payload GenerateQuoteJobPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewGenerateQuoteJobTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue(c.queue))
	return err
}

func (c *Client) EnqueueGenerateQuoteJobRequest(ctx context.Context, req GenerateQuoteJobRequest) error {
	var quoteIDStr *string
	if req.QuoteID != nil {
		value := req.QuoteID.String()
		quoteIDStr = &value
	}

	return c.EnqueueGenerateQuoteJob(ctx, GenerateQuoteJobPayload{
		JobID:         req.JobID.String(),
		TenantID:      req.TenantID.String(),
		UserID:        req.UserID.String(),
		LeadID:        req.LeadID.String(),
		LeadServiceID: req.LeadServiceID.String(),
		Prompt:        req.Prompt,
		QuoteID:       quoteIDStr,
	})
}

func redisClientOpt(redisURL string, tlsInsecure bool) (asynq.RedisClientOpt, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, err
	}

	var tlsConfig *tls.Config
	if opt.TLSConfig != nil {
		clone := opt.TLSConfig.Clone()
		if tlsInsecure {
			clone.InsecureSkipVerify = true
		}
		tlsConfig = clone
	} else if tlsInsecure {
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return asynq.RedisClientOpt{
		Addr:      opt.Addr,
		Password:  opt.Password,
		DB:        opt.DB,
		TLSConfig: tlsConfig,
	}, nil
}

package scheduler

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/platform/config"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const (
	imapSyncAccountTaskTimeout   = 90 * time.Second
	imapSyncAccountTaskUniqueTTL = 2 * time.Minute
	imapSyncAccountTaskMaxRetry  = 3
	imapSyncSweepTaskTimeout     = 5 * time.Minute
	imapSyncSweepTaskUniqueTTL   = 9 * time.Minute
	imapSyncSweepTaskMaxRetry    = 1
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

type IMAPSyncScheduler interface {
	EnqueueIMAPSyncAccount(ctx context.Context, payload IMAPSyncAccountPayload) error
	EnqueueIMAPSyncSweep(ctx context.Context) error
}

type HumanFeedbackMemoryScheduler interface {
	EnqueueApplyHumanFeedbackMemory(ctx context.Context, payload ApplyHumanFeedbackMemoryPayload) error
}

type CallLogScheduler interface {
	EnqueueLogCall(ctx context.Context, payload LogCallPayload) error
}

type QuoteJobRunner interface {
	EnqueueGenerateQuoteJobRequest(ctx context.Context, req GenerateQuoteJobRequest) error
}

type QuoteAcceptedPDFRunner interface {
	EnqueueGenerateAcceptedQuotePDFRequest(ctx context.Context, req GenerateAcceptedQuotePDFRequest) error
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
	Force         bool
}

type GenerateAcceptedQuotePDFRequest struct {
	QuoteID       uuid.UUID
	TenantID      uuid.UUID
	OrgName       string
	CustomerName  string
	SignatureName string
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

func (c *Client) EnqueueGenerateAcceptedQuotePDF(ctx context.Context, payload GenerateAcceptedQuotePDFPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewGenerateAcceptedQuotePDFTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue(c.queue))
	return err
}

func (c *Client) EnqueueLogCall(ctx context.Context, payload LogCallPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewLogCallTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue(c.queue))
	return err
}

func (c *Client) EnqueueIMAPSyncAccount(ctx context.Context, payload IMAPSyncAccountPayload) error {
	if c == nil || c.client == nil {
		return nil
	}
	task, err := NewIMAPSyncAccountTask(payload)
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, imapSyncAccountTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueueIMAPSyncSweep(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	task, err := NewIMAPSyncSweepTask(IMAPSyncSweepPayload{})
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, imapSyncSweepTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func imapSyncAccountTaskOptions(queue string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(imapSyncAccountTaskMaxRetry),
		asynq.Timeout(imapSyncAccountTaskTimeout),
		asynq.Unique(imapSyncAccountTaskUniqueTTL),
	}
}

func imapSyncSweepTaskOptions(queue string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(imapSyncSweepTaskMaxRetry),
		asynq.Timeout(imapSyncSweepTaskTimeout),
		asynq.Unique(imapSyncSweepTaskUniqueTTL),
	}
}

func normalizeEnqueueError(err error) error {
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (c *Client) EnqueueApplyHumanFeedbackMemory(ctx context.Context, payload ApplyHumanFeedbackMemoryPayload) error {
	if c == nil || c.client == nil {
		return nil
	}
	task, err := NewApplyHumanFeedbackMemoryTask(payload)
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
		Force:         req.Force,
	})
}

func (c *Client) EnqueueGenerateAcceptedQuotePDFRequest(ctx context.Context, req GenerateAcceptedQuotePDFRequest) error {
	return c.EnqueueGenerateAcceptedQuotePDF(ctx, GenerateAcceptedQuotePDFPayload{
		QuoteID:       req.QuoteID.String(),
		TenantID:      req.TenantID.String(),
		OrgName:       req.OrgName,
		CustomerName:  req.CustomerName,
		SignatureName: req.SignatureName,
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

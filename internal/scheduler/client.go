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
	imapSyncAccountTaskTimeout     = 90 * time.Second
	imapSyncAccountTaskUniqueTTL   = 2 * time.Minute
	imapSyncAccountTaskMaxRetry    = 3
	imapSyncSweepTaskTimeout       = 5 * time.Minute
	imapSyncSweepTaskUniqueTTL     = 9 * time.Minute
	imapSyncSweepTaskMaxRetry      = 1
	offerSummaryTaskTimeout        = 2 * time.Minute
	offerSummaryTaskUniqueTTL      = 10 * time.Minute
	offerSummaryTaskMaxRetry       = 3
	leadAutomationTaskTimeout      = 5 * time.Minute
	leadAutomationTaskUniqueTTL    = leadAutomationTaskTimeout
	gatekeeperTaskUniqueTTL        = 45 * time.Second
	leadAutomationTaskMaxRetry     = 3
	autoPhotoAnalysisDelay         = 30 * time.Second
	estimatorTaskTimeout           = 10 * time.Minute
	estimatorTaskUniqueTTL         = estimatorTaskTimeout
	waAgentVoiceTaskTimeout        = 5 * time.Minute
	waAgentVoiceTaskUniqueTTL      = 15 * time.Minute
	waAgentVoiceTaskMaxRetry       = 3
	staleLeadNotifyTaskUniqueTTL   = 24 * time.Hour
	staleLeadReEngageTaskTimeout   = 3 * time.Minute
	staleLeadReEngageTaskUniqueTTL = 24 * time.Hour
	staleLeadReEngageTaskMaxRetry  = 2
)

type Client struct {
	client *asynq.Client
	queue  string
}

type ReminderScheduler interface {
	ScheduleAppointmentReminder(ctx context.Context, payload AppointmentReminderPayload, runAt time.Time) error
}

type TaskReminderScheduler interface {
	ScheduleTaskReminder(ctx context.Context, payload TaskReminderPayload, runAt time.Time) error
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

type PartnerOfferSummaryScheduler interface {
	EnqueuePartnerOfferSummary(ctx context.Context, payload PartnerOfferSummaryPayload) error
}

type PartnerOfferPDFScheduler interface {
	EnqueuePartnerOfferPDF(ctx context.Context, payload PartnerOfferPDFPayload) error
}

type GatekeeperScheduler interface {
	EnqueueGatekeeperRun(ctx context.Context, payload GatekeeperRunPayload) error
}

type EstimatorScheduler interface {
	EnqueueEstimatorRun(ctx context.Context, payload EstimatorRunPayload) error
}

type DispatcherScheduler interface {
	EnqueueDispatcherRun(ctx context.Context, payload DispatcherRunPayload) error
}

type PhotoAnalysisScheduler interface {
	EnqueuePhotoAnalysis(ctx context.Context, payload PhotoAnalysisPayload) error
	EnqueuePhotoAnalysisIn(ctx context.Context, payload PhotoAnalysisPayload, delay time.Duration) error
}

type AuditorScheduler interface {
	EnqueueAuditVisitReport(ctx context.Context, payload AuditVisitReportPayload) error
	EnqueueAuditCallLog(ctx context.Context, payload AuditCallLogPayload) error
}

type WAAgentVoiceTranscriptionScheduler interface {
	EnqueueWAAgentVoiceTranscription(ctx context.Context, payload WAAgentVoiceTranscriptionPayload) error
}

type StaleLeadNotifyScheduler interface {
	EnqueueStaleLeadNotify(ctx context.Context, payload StaleLeadNotifyPayload) error
}

type StaleLeadReEngageScheduler interface {
	EnqueueStaleLeadReEngage(ctx context.Context, payload StaleLeadReEngagePayload) error
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

func (c *Client) ScheduleTaskReminder(ctx context.Context, payload TaskReminderPayload, runAt time.Time) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewTaskReminderTask(payload)
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

	_, err = c.client.EnqueueContext(ctx, task, estimatorTaskOptions(c.queue)...)
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

func (c *Client) EnqueueSubsidyAnalyzerJob(ctx context.Context, payload SubsidyAnalyzerJobPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewSubsidyAnalyzerJobTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue(c.queue))
	return err
}

func (c *Client) EnqueuePartnerOfferSummary(ctx context.Context, payload PartnerOfferSummaryPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewPartnerOfferSummaryTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, offerSummaryTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueuePartnerOfferPDF(ctx context.Context, payload PartnerOfferPDFPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewPartnerOfferPDFTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue(c.queue))
	return normalizeEnqueueError(err)
}

// ErrDuplicateTask is returned when a task is a duplicate of an existing task
// in the queue (within the unique TTL window). Callers can use errors.Is to check
// for this specifically when they need to distinguish "already queued" from other errors.
var ErrDuplicateTask = asynq.ErrDuplicateTask

func (c *Client) EnqueueGatekeeperRun(ctx context.Context, payload GatekeeperRunPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewGatekeeperRunTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, gatekeeperTaskOptions(c.queue)...)
	// Intentionally NOT using normalizeEnqueueError here so that callers can
	// distinguish duplicate tasks from successful enqueues.
	return err
}

func (c *Client) EnqueueEstimatorRun(ctx context.Context, payload EstimatorRunPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewEstimatorRunTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, estimatorTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueueDispatcherRun(ctx context.Context, payload DispatcherRunPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewDispatcherRunTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, leadAutomationTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueuePhotoAnalysis(ctx context.Context, payload PhotoAnalysisPayload) error {
	return c.EnqueuePhotoAnalysisIn(ctx, payload, 0)
}

func (c *Client) EnqueuePhotoAnalysisIn(ctx context.Context, payload PhotoAnalysisPayload, delay time.Duration) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewPhotoAnalysisTask(payload)
	if err != nil {
		return err
	}

	options := leadAutomationTaskOptions(c.queue)
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = c.client.EnqueueContext(ctx, task, options...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueueAuditVisitReport(ctx context.Context, payload AuditVisitReportPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewAuditVisitReportTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, leadAutomationTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueueAuditCallLog(ctx context.Context, payload AuditCallLogPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewAuditCallLogTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, leadAutomationTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueueWAAgentVoiceTranscription(ctx context.Context, payload WAAgentVoiceTranscriptionPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewWAAgentVoiceTranscriptionTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task, waAgentVoiceTaskOptions(c.queue)...)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueueStaleLeadNotify(ctx context.Context, payload StaleLeadNotifyPayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewStaleLeadNotifyTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task,
		asynq.Queue(c.queue),
		asynq.MaxRetry(2),
		asynq.Unique(staleLeadNotifyTaskUniqueTTL),
	)
	return normalizeEnqueueError(err)
}

func (c *Client) EnqueueStaleLeadReEngage(ctx context.Context, payload StaleLeadReEngagePayload) error {
	if c == nil || c.client == nil {
		return nil
	}

	task, err := NewStaleLeadReEngageTask(payload)
	if err != nil {
		return err
	}

	_, err = c.client.EnqueueContext(ctx, task,
		asynq.Queue(c.queue),
		asynq.MaxRetry(staleLeadReEngageTaskMaxRetry),
		asynq.Timeout(staleLeadReEngageTaskTimeout),
		asynq.Unique(staleLeadReEngageTaskUniqueTTL),
	)
	return normalizeEnqueueError(err)
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

func offerSummaryTaskOptions(queue string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(offerSummaryTaskMaxRetry),
		asynq.Timeout(offerSummaryTaskTimeout),
		asynq.Unique(offerSummaryTaskUniqueTTL),
	}
}

func gatekeeperTaskOptions(queue string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(leadAutomationTaskMaxRetry),
		asynq.Timeout(leadAutomationTaskTimeout),
		asynq.Unique(gatekeeperTaskUniqueTTL),
	}
}

func leadAutomationTaskOptions(queue string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(leadAutomationTaskMaxRetry),
		asynq.Timeout(leadAutomationTaskTimeout),
		asynq.Unique(leadAutomationTaskUniqueTTL),
	}
}

func estimatorTaskOptions(queue string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(leadAutomationTaskMaxRetry),
		asynq.Timeout(estimatorTaskTimeout),
		asynq.Unique(estimatorTaskUniqueTTL),
	}
}

func waAgentVoiceTaskOptions(queue string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(waAgentVoiceTaskMaxRetry),
		asynq.Timeout(waAgentVoiceTaskTimeout),
		asynq.Unique(waAgentVoiceTaskUniqueTTL),
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

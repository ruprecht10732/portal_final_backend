package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/internal/appointments/repository"
	"portal_final_backend/internal/events"
	leadrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/qdrant"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	repo   *repository.Repository
	leads  *leadrepo.Repository
	bus    events.Bus
	log    *logger.Logger
	quotes QuoteJobProcessor
	pdf    QuoteAcceptedPDFProcessor
	call   CallLogProcessor
	imap   IMAPSyncProcessor
	embed  *embeddings.Client
	qdrant *qdrant.Client
}

type QuoteJobProcessor interface {
	ProcessGenerateQuoteJob(ctx context.Context, jobID uuid.UUID, prompt string, existingQuoteID *uuid.UUID, force bool) error
}

type IMAPSyncProcessor interface {
	SyncAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) error
	SyncEligibleAccounts(ctx context.Context) error
}

type QuoteAcceptedPDFProcessor interface {
	GenerateAndStorePDF(ctx context.Context, quoteID, organizationID uuid.UUID, orgName, customerName, signatureName string) (string, []byte, error)
}

type CallLogProcessor interface {
	ProcessLogCallJob(ctx context.Context, leadID, serviceID, userID, tenantID uuid.UUID, summary string) error
}

func NewWorker(cfg config.SchedulerConfig, pool *pgxpool.Pool, bus events.Bus, log *logger.Logger) (*Worker, error) {
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

	concurrency := cfg.GetAsynqConcurrency()
	if concurrency < 1 {
		concurrency = 10
	}

	server := asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			queue: 1,
		},
	})

	mux := asynq.NewServeMux()
	w := &Worker{
		server: server,
		mux:    mux,
		repo:   repository.New(pool),
		leads:  leadrepo.New(pool),
		bus:    bus,
		log:    log,
	}

	if embeddingCfg, ok := any(cfg).(interface {
		IsEmbeddingEnabled() bool
		GetEmbeddingAPIURL() string
		GetEmbeddingAPIKey() string
	}); ok && embeddingCfg.IsEmbeddingEnabled() {
		w.embed = embeddings.NewClient(embeddings.Config{
			BaseURL: embeddingCfg.GetEmbeddingAPIURL(),
			APIKey:  embeddingCfg.GetEmbeddingAPIKey(),
		})
	}

	if qdrantCfg, ok := any(cfg).(interface {
		IsQdrantEnabled() bool
		GetQdrantURL() string
		GetQdrantAPIKey() string
		GetQdrantCollection() string
	}); ok && qdrantCfg.IsQdrantEnabled() {
		w.qdrant = qdrant.NewClient(qdrant.Config{
			BaseURL:    qdrantCfg.GetQdrantURL(),
			APIKey:     qdrantCfg.GetQdrantAPIKey(),
			Collection: qdrantCfg.GetQdrantCollection(),
		})
	}

	mux.HandleFunc(TaskAppointmentReminder, w.handleAppointmentReminder)
	mux.HandleFunc(TaskNotificationOutboxDue, w.handleNotificationOutboxDue)
	mux.HandleFunc(TaskGenerateQuoteJob, w.handleGenerateQuoteJob)
	mux.HandleFunc(TaskGenerateAcceptedQuotePDF, w.handleGenerateAcceptedQuotePDF)
	mux.HandleFunc(TaskLogCall, w.handleLogCall)
	mux.HandleFunc(TaskIMAPSyncAccount, w.handleIMAPSyncAccount)
	mux.HandleFunc(TaskIMAPSyncSweep, w.handleIMAPSyncSweep)
	mux.HandleFunc(TaskApplyHumanFeedbackMemory, w.handleApplyHumanFeedbackMemory)

	return w, nil
}

func (w *Worker) SetQuoteJobProcessor(processor QuoteJobProcessor) {
	w.quotes = processor
}

func (w *Worker) SetAcceptedQuotePDFProcessor(processor QuoteAcceptedPDFProcessor) {
	w.pdf = processor
}

func (w *Worker) SetIMAPSyncProcessor(processor IMAPSyncProcessor) {
	w.imap = processor
}

func (w *Worker) SetCallLogProcessor(processor CallLogProcessor) {
	w.call = processor
}

func (w *Worker) handleNotificationOutboxDue(ctx context.Context, task *asynq.Task) error {
	if w.bus == nil {
		return nil
	}

	payload, err := ParseNotificationOutboxDuePayload(task)
	if err != nil {
		return err
	}

	outboxID, err := uuid.Parse(payload.OutboxID)
	if err != nil {
		return err
	}

	tenantID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		return err
	}

	return w.bus.PublishSync(ctx, events.NotificationOutboxDue{
		BaseEvent: events.NewBaseEvent(),
		OutboxID:  outboxID,
		TenantID:  tenantID,
	})
}

func (w *Worker) handleApplyHumanFeedbackMemory(ctx context.Context, task *asynq.Task) error {
	if w.leads == nil {
		return nil
	}

	payload, err := ParseApplyHumanFeedbackMemoryPayload(task)
	if err != nil {
		return err
	}

	tenantID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		return err
	}
	feedbackID, err := uuid.Parse(payload.FeedbackID)
	if err != nil {
		return err
	}

	feedback, err := w.leads.GetHumanFeedbackByID(ctx, feedbackID, tenantID)
	if err != nil {
		if errors.Is(err, leadrepo.ErrNotFound) {
			return nil
		}
		return err
	}
	if feedback.AppliedToMemory {
		return nil
	}
	if w.embed == nil || w.qdrant == nil {
		return fmt.Errorf("human feedback memory dependencies not configured for feedbackId=%s tenantId=%s", feedbackID, tenantID)
	}

	text := buildHumanFeedbackMemoryDocument(feedback)
	vector, err := w.embed.Embed(ctx, text)
	if err != nil {
		return err
	}

	pointID := "hf:" + feedback.ID.String()
	point := qdrant.Point{
		ID:     pointID,
		Vector: vector,
		Payload: map[string]any{
			"type":             "human_feedback",
			"organization_id":  feedback.OrganizationID.String(),
			"quote_id":         feedback.QuoteID.String(),
			"field_changed":    feedback.FieldChanged,
			"delta_percentage": feedback.DeltaPercentage,
			"created_at":       feedback.CreatedAt.Format(time.RFC3339),
			"memory_text":      text,
			"ai_value":         feedback.AIValue,
			"human_value":      feedback.HumanValue,
		},
	}
	if feedback.LeadServiceID != nil {
		point.Payload["lead_service_id"] = feedback.LeadServiceID.String()
	}

	if err := w.qdrant.UpsertPoint(ctx, point); err != nil {
		return err
	}

	embeddingRef := w.qdrant.CollectionName() + "/" + pointID
	if _, err := w.leads.MarkHumanFeedbackApplied(ctx, feedback.ID, tenantID, &embeddingRef); err != nil {
		return err
	}

	w.log.Info("human feedback memory applied", "feedbackId", feedback.ID, "tenantId", tenantID, "embeddingId", embeddingRef)
	return nil
}

func buildHumanFeedbackMemoryDocument(feedback leadrepo.HumanFeedback) string {
	aiJSON, _ := json.Marshal(feedback.AIValue)
	humanJSON, _ := json.Marshal(feedback.HumanValue)

	var sb strings.Builder
	sb.WriteString("Human feedback correction\n")
	sb.WriteString("field_changed: ")
	sb.WriteString(feedback.FieldChanged)
	sb.WriteString("\n")
	if feedback.DeltaPercentage != nil {
		sb.WriteString(fmt.Sprintf("delta_percentage: %.2f\n", *feedback.DeltaPercentage))
	}
	sb.WriteString("ai_value: ")
	sb.Write(aiJSON)
	sb.WriteString("\n")
	sb.WriteString("human_value: ")
	sb.Write(humanJSON)
	sb.WriteString("\n")

	return sb.String()
}

func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.server == nil {
		return
	}

	go func() {
		<-ctx.Done()
		w.server.Shutdown()
	}()

	if err := w.server.Run(w.mux); err != nil {
		w.log.Error("scheduler worker stopped", "error", err)
	}
}

func (w *Worker) handleAppointmentReminder(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseAppointmentReminderPayload(task)
	if err != nil {
		return err
	}

	apptID, err := uuid.Parse(payload.AppointmentID)
	if err != nil {
		return err
	}

	orgID, err := uuid.Parse(payload.OrganizationID)
	if err != nil {
		return err
	}

	appt, err := w.repo.GetByID(ctx, apptID, orgID)
	if err != nil {
		return err
	}

	if appt.Status != "scheduled" || appt.Type != "lead_visit" {
		return nil
	}

	if appt.LeadID == nil {
		return nil
	}

	leadInfo, err := w.repo.GetLeadInfo(ctx, *appt.LeadID, orgID)
	if err != nil {
		return err
	}
	if leadInfo == nil || leadInfo.Phone == "" {
		return nil
	}

	consumerEmail, err := w.repo.GetLeadEmail(ctx, *appt.LeadID, orgID)
	if err != nil {
		consumerEmail = ""
	}

	consumerName := strings.TrimSpace(fmt.Sprintf("%s %s", leadInfo.FirstName, leadInfo.LastName))
	if consumerName == "" {
		consumerName = "klant"
	}

	if w.bus == nil {
		return nil
	}

	w.bus.Publish(ctx, events.AppointmentReminderDue{
		BaseEvent:      events.NewBaseEvent(),
		AppointmentID:  appt.ID,
		OrganizationID: appt.OrganizationID,
		LeadID:         appt.LeadID,
		LeadServiceID:  appt.LeadServiceID,
		UserID:         appt.UserID,
		Type:           appt.Type,
		Title:          appt.Title,
		StartTime:      appt.StartTime,
		EndTime:        appt.EndTime,
		ConsumerName:   consumerName,
		ConsumerPhone:  leadInfo.Phone,
		ConsumerEmail:  consumerEmail,
		Location:       getOptionalString(appt.Location),
	})

	return nil
}

func (w *Worker) handleGenerateQuoteJob(ctx context.Context, task *asynq.Task) error {
	if w.quotes == nil {
		return fmt.Errorf("quote job processor is not configured")
	}

	payload, err := ParseGenerateQuoteJobPayload(task)
	if err != nil {
		return err
	}

	jobID, err := uuid.Parse(payload.JobID)
	if err != nil {
		return err
	}

	var existingQuoteID *uuid.UUID
	if payload.QuoteID != nil && *payload.QuoteID != "" {
		parsed, parseErr := uuid.Parse(*payload.QuoteID)
		if parseErr != nil {
			return parseErr
		}
		existingQuoteID = &parsed
	}

	start := time.Now()
	w.log.Info(
		"scheduler: starting quote generation job",
		"jobId", jobID,
		"tenantId", payload.TenantID,
		"userId", payload.UserID,
		"leadId", payload.LeadID,
		"leadServiceId", payload.LeadServiceID,
		"hasExistingQuote", existingQuoteID != nil,
	)

	err = w.quotes.ProcessGenerateQuoteJob(ctx, jobID, payload.Prompt, existingQuoteID, payload.Force)
	if err != nil {
		w.log.Error(
			"scheduler: quote generation job failed",
			"jobId", jobID,
			"durationMs", time.Since(start).Milliseconds(),
			"error", err,
		)
		return err
	}

	w.log.Info(
		"scheduler: quote generation job completed",
		"jobId", jobID,
		"durationMs", time.Since(start).Milliseconds(),
	)
	return nil
}

func (w *Worker) handleGenerateAcceptedQuotePDF(ctx context.Context, task *asynq.Task) error {
	if w.pdf == nil {
		return fmt.Errorf("accepted quote PDF processor is not configured")
	}

	payload, err := ParseGenerateAcceptedQuotePDFPayload(task)
	if err != nil {
		return err
	}

	quoteID, err := uuid.Parse(payload.QuoteID)
	if err != nil {
		return err
	}

	tenantID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		return err
	}

	start := time.Now()
	_, _, err = w.pdf.GenerateAndStorePDF(ctx, quoteID, tenantID, payload.OrgName, payload.CustomerName, payload.SignatureName)
	if err != nil {
		w.log.Error(
			"scheduler: accepted quote PDF generation failed",
			"quoteId", quoteID,
			"tenantId", tenantID,
			"durationMs", time.Since(start).Milliseconds(),
			"error", err,
		)
		return err
	}

	w.log.Info(
		"scheduler: accepted quote PDF generation completed",
		"quoteId", quoteID,
		"tenantId", tenantID,
		"durationMs", time.Since(start).Milliseconds(),
	)

	return nil
}

func (w *Worker) handleLogCall(ctx context.Context, task *asynq.Task) error {
	if w.call == nil {
		return fmt.Errorf("call log processor is not configured")
	}

	payload, err := ParseLogCallPayload(task)
	if err != nil {
		return err
	}

	leadID, err := uuid.Parse(payload.LeadID)
	if err != nil {
		return err
	}

	serviceID, err := uuid.Parse(payload.ServiceID)
	if err != nil {
		return err
	}

	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return err
	}

	tenantID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		return err
	}

	start := time.Now()
	w.log.Info(
		"scheduler: starting async call log processing",
		"leadId", leadID,
		"serviceId", serviceID,
		"tenantId", tenantID,
		"userId", userID,
	)

	err = w.call.ProcessLogCallJob(ctx, leadID, serviceID, userID, tenantID, payload.Summary)
	if err != nil {
		w.log.Error(
			"scheduler: async call log processing failed",
			"leadId", leadID,
			"serviceId", serviceID,
			"tenantId", tenantID,
			"durationMs", time.Since(start).Milliseconds(),
			"error", err,
		)
		return err
	}

	w.log.Info(
		"scheduler: async call log processing completed",
		"leadId", leadID,
		"serviceId", serviceID,
		"tenantId", tenantID,
		"durationMs", time.Since(start).Milliseconds(),
	)

	return nil
}

func (w *Worker) handleIMAPSyncAccount(ctx context.Context, task *asynq.Task) error {
	if w.imap == nil {
		return fmt.Errorf("imap sync processor is not configured")
	}
	payload, err := ParseIMAPSyncAccountPayload(task)
	if err != nil {
		return err
	}
	accountID, err := uuid.Parse(payload.AccountID)
	if err != nil {
		return err
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return err
	}
	return w.imap.SyncAccount(ctx, userID, accountID)
}

func (w *Worker) handleIMAPSyncSweep(ctx context.Context, _ *asynq.Task) error {
	if w.imap == nil {
		return nil
	}
	return w.imap.SyncEligibleAccounts(ctx)
}

func getOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

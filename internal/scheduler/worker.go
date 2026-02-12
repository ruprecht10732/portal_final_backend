package scheduler

import (
	"context"
	"fmt"
	"strings"

	"portal_final_backend/internal/appointments/repository"
	"portal_final_backend/internal/events"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	repo   *repository.Repository
	bus    events.Bus
	log    *logger.Logger
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
		bus:    bus,
		log:    log,
	}

	mux.HandleFunc(TaskAppointmentReminder, w.handleAppointmentReminder)
	mux.HandleFunc(TaskNotificationOutboxDue, w.handleNotificationOutboxDue)

	return w, nil
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
		Location:       getOptionalString(appt.Location),
	})

	return nil
}

func getOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

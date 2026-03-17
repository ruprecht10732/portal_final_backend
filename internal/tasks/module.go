package tasks

import (
	apphttp "portal_final_backend/internal/http"
	leadrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *Handler
	svc     *Service
}

func NewModule(pool *pgxpool.Pool, val *validator.Validator, reminderScheduler scheduler.TaskReminderScheduler, timeline leadrepo.TimelineEventStore, log *logger.Logger) *Module {
	repo := NewRepository(pool)
	svc := NewService(repo, reminderScheduler, timeline, log)
	handler := NewHandler(svc, val)
	return &Module{handler: handler, svc: svc}
}

func (m *Module) Name() string {
	return "tasks"
}

func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	group := ctx.Protected.Group("/tasks")
	m.handler.RegisterRoutes(group)
}

func (m *Module) Service() *Service {
	return m.svc
}

var _ apphttp.Module = (*Module)(nil)
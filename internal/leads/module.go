// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"context"

	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scheduling"
	"portal_final_backend/internal/maps"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the leads bounded context module implementing http.Module.
type Module struct {
	handler    *handler.Handler
	management *management.Service
	scheduling *scheduling.Service
	notes      *notes.Service
	advisor    *agent.LeadAdvisor
}

// NewModule creates and initializes the leads module with all its dependencies.
func NewModule(pool *pgxpool.Pool, eventBus events.Bus, val *validator.Validator, cfg *config.Config, log *logger.Logger) (*Module, error) {
	// Create shared repository
	repo := repository.New(pool)

	// AI Advisor for lead analysis
	advisor, err := agent.NewLeadAdvisor(cfg.MoonshotAPIKey, repo)
	if err != nil {
		return nil, err
	}

	// Subscribe to LeadCreated events to auto-analyze new leads
	eventBus.Subscribe(events.LeadCreated{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadCreated)
		if !ok {
			return nil
		}

		go func() {
			if err := advisor.Analyze(context.Background(), e.LeadID); err != nil {
				log.Error("lead advisor analysis failed", "error", err, "leadId", e.LeadID)
			}
		}()

		return nil
	}))

	// Create focused services (vertical slices)
	mapsSvc := maps.NewService(log)
	mgmtSvc := management.New(repo, eventBus, mapsSvc)
	schedulingSvc := scheduling.New(repo, eventBus)
	notesSvc := notes.New(repo)

	// Create handlers
	notesHandler := handler.NewNotesHandler(notesSvc, val)
	h := handler.New(mgmtSvc, schedulingSvc, notesHandler, advisor, val)

	return &Module{
		handler:    h,
		management: mgmtSvc,
		scheduling: schedulingSvc,
		notes:      notesSvc,
		advisor:    advisor,
	}, nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "leads"
}

// ManagementService returns the lead management service for external use.
func (m *Module) ManagementService() *management.Service {
	return m.management
}

// SchedulingService returns the lead scheduling service for external use.
func (m *Module) SchedulingService() *scheduling.Service {
	return m.scheduling
}

// NotesService returns the lead notes service for external use.
func (m *Module) NotesService() *notes.Service {
	return m.notes
}

// RegisterRoutes mounts leads routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// All leads routes require authentication
	leadsGroup := ctx.Protected.Group("/leads")
	m.handler.RegisterRoutes(leadsGroup)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)

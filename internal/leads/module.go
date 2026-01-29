// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scheduling"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the leads bounded context module implementing http.Module.
type Module struct {
	handler    *handler.Handler
	management *management.Service
	scheduling *scheduling.Service
	notes      *notes.Service
}

// NewModule creates and initializes the leads module with all its dependencies.
func NewModule(pool *pgxpool.Pool, eventBus events.Bus) *Module {
	// Create shared repository
	repo := repository.New(pool)

	// Create focused services (vertical slices)
	mgmtSvc := management.New(repo)
	schedulingSvc := scheduling.New(repo, eventBus)
	notesSvc := notes.New(repo)

	// Create handlers
	notesHandler := handler.NewNotesHandler(notesSvc)
	h := handler.New(mgmtSvc, schedulingSvc, notesHandler)

	return &Module{
		handler:    h,
		management: mgmtSvc,
		scheduling: schedulingSvc,
		notes:      notesSvc,
	}
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

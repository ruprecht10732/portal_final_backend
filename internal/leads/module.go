// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"context"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/maps"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the leads bounded context module implementing http.Module.
type Module struct {
	handler            *handler.Handler
	attachmentsHandler *handler.AttachmentsHandler
	management         *management.Service
	notes              *notes.Service
	advisor            *agent.LeadAdvisor
	callLogger         *agent.CallLogger
}

// NewModule creates and initializes the leads module with all its dependencies.
func NewModule(pool *pgxpool.Pool, eventBus events.Bus, storageSvc storage.StorageService, val *validator.Validator, cfg *config.Config, log *logger.Logger) (*Module, error) {
	// Create shared repository
	repo := repository.New(pool)

	// AI Advisor for lead analysis
	advisor, err := agent.NewLeadAdvisor(cfg.MoonshotAPIKey, repo)
	if err != nil {
		return nil, err
	}

	// CallLogger for post-call processing (booker will be set later to break circular dependency)
	callLogger, err := agent.NewCallLogger(cfg.MoonshotAPIKey, repo, nil)
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
			// Pass nil for serviceID to analyze the most recent (current) service
			if err := advisor.Analyze(context.Background(), e.LeadID, nil, e.TenantID); err != nil {
				log.Error("lead advisor analysis failed", "error", err, "leadId", e.LeadID)
			}
		}()

		return nil
	}))

	// Create focused services (vertical slices)
	mapsSvc := maps.NewService(log)
	mgmtSvc := management.New(repo, eventBus, mapsSvc)
	notesSvc := notes.New(repo)

	// Create handlers
	notesHandler := handler.NewNotesHandler(notesSvc, val)
	attachmentsHandler := handler.NewAttachmentsHandler(repo, storageSvc, cfg.GetMinioBucketLeadServiceAttachments(), val)
	h := handler.New(mgmtSvc, notesHandler, advisor, callLogger, val)

	return &Module{
		handler:            h,
		attachmentsHandler: attachmentsHandler,
		management:         mgmtSvc,
		notes:              notesSvc,
		advisor:            advisor,
		callLogger:         callLogger,
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

// NotesService returns the lead notes service for external use.
func (m *Module) NotesService() *notes.Service {
	return m.notes
}

// CallLogger returns the call logger agent for external use.
func (m *Module) CallLogger() *agent.CallLogger {
	return m.callLogger
}

// SetAppointmentBooker sets the appointment booker on the CallLogger.
// This is called after module initialization to break circular dependencies.
func (m *Module) SetAppointmentBooker(booker ports.AppointmentBooker) {
	m.callLogger.SetAppointmentBooker(booker)
}

// RegisterRoutes mounts leads routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// All leads routes require authentication
	leadsGroup := ctx.Protected.Group("/leads")
	m.handler.RegisterRoutes(leadsGroup)

	// Attachment routes: /leads/:id/services/:serviceId/attachments
	attachmentsGroup := leadsGroup.Group("/:id/services/:serviceId/attachments")
	m.attachmentsHandler.RegisterRoutes(attachmentsGroup)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)

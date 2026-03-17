package whatsappagent

import (
	apphttp "portal_final_backend/internal/http"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/internal/whatsappagent/engine"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CurrentInboundMessage = engine.CurrentInboundMessage
type ConversationMessage = engine.ConversationMessage
type AgentRunResult = engine.AgentRunResult
type AudioTranscriptionInput = engine.AudioTranscriptionInput
type AudioTranscriptionResult = engine.AudioTranscriptionResult
type AudioTranscriber = engine.AudioTranscriber

type Module struct {
	inner         *engine.Module
	deviceHandler *DeviceHandler
	service       *Service
}

func NewModule(pool *pgxpool.Pool, cfg ModuleConfig, deps ModuleDependencies) (*Module, error) {
	inner, err := engine.NewModule(pool, engine.ModuleConfig{
		MoonshotAPIKey: cfg.MoonshotAPIKey,
		LLMModel:       cfg.LLMModel,
		WebhookSecret:  cfg.WebhookSecret,
	}, engine.ModuleDependencies{
		WhatsAppClient:               deps.WhatsAppClient,
		QuotesReader:                 adaptQuotesReader(deps.QuotesReader),
		AppointmentsReader:           adaptAppointmentsReader(deps.AppointmentsReader),
		LeadSearchReader:             adaptLeadSearchReader(deps.LeadSearchReader),
		LeadDetailsReader:            adaptLeadDetailsReader(deps.LeadDetailsReader),
		NavigationLinkReader:         adaptNavigationLinkReader(deps.NavigationLinkReader),
		CatalogSearchReader:          adaptCatalogSearchReader(deps.CatalogSearchReader),
		LeadMutationWriter:           adaptLeadMutationWriter(deps.LeadMutationWriter),
		TaskWriter:                   adaptTaskWriter(deps.TaskWriter),
		QuoteWorkflowWriter:          adaptQuoteWorkflowWriter(deps.QuoteWorkflowWriter),
		CurrentInboundPhotoAttacher:  adaptCurrentInboundPhotoAttacher(deps.CurrentInboundPhotoAttacher),
		Storage:                      deps.Storage,
		AttachmentBucket:             deps.AttachmentBucket,
		TranscriptionScheduler:       deps.TranscriptionScheduler,
		AudioTranscriber:             deps.AudioTranscriber,
		InboxMessageSync:             deps.InboxMessageSync,
		VisitSlotReader:              adaptVisitSlotReader(deps.VisitSlotReader),
		VisitMutationWriter:          adaptVisitMutationWriter(deps.VisitMutationWriter),
		PartnerPhoneReader:           adaptPartnerPhoneReader(deps.PartnerPhoneReader),
		PartnerJobReader:             adaptPartnerJobReader(deps.PartnerJobReader),
		AppointmentVisitReportWriter: adaptAppointmentVisitReportWriter(deps.AppointmentVisitReportWriter),
		AppointmentStatusWriter:      adaptAppointmentStatusWriter(deps.AppointmentStatusWriter),
		RedisClient:                  deps.RedisClient,
		InboxWriter:                  deps.InboxWriter,
		Logger:                       deps.Logger,
	})
	if err != nil {
		return nil, err
	}
	module := &Module{inner: inner}
	if deps.WhatsAppClient != nil && pool != nil {
		module.deviceHandler = newDeviceHandler(whatsappagentdb.New(pool), deps.WhatsAppClient, cfg.WebhookSecret)
	}
	module.service = newService(pool, deps, inner.Service())
	return module, nil
}

func (m *Module) Name() string {
	if m == nil || m.inner == nil {
		return "whatsappagent"
	}
	return m.inner.Name()
}

func (m *Module) Service() *Service {
	if m == nil {
		return nil
	}
	return m.service
}

func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	if m == nil || ctx == nil {
		return
	}
	if m.deviceHandler != nil {
		m.deviceHandler.RegisterRoutes(ctx)
	}
	if m.inner != nil {
		m.inner.RegisterAdminRoutes(ctx)
	}
}

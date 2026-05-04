package agent

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/adk/session"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/qdrant"
)

// Runtime creates agents on demand and routes tasks to the correct workspace.
// It holds shared dependencies injected at module init time.
type Runtime struct {
	sessionSvc session.Service

	gatekeeperModelCfg openaicompat.Config
	calculatorModelCfg openaicompat.Config
	matchmakerModelCfg openaicompat.Config
	auditorModelCfg    openaicompat.Config

	repo                repository.LeadsRepository
	scorer              *scoring.Service
	eventBus            events.Bus
	catalogReader       ports.CatalogReader
	pricingIntelligence ports.PricingIntelligenceReader

	embeddingClient      *embeddings.Client
	qdrantClient         *qdrant.Client
	bouwmaatQdrantClient *qdrant.Client
	catalogQdrantClient  *qdrant.Client

	orgSettingsReader ports.OrganizationAISettingsReader
	quoteDrafter      ports.QuoteDrafter
	offerCreator      ports.PartnerOfferCreator
}

// NewRuntime creates a runtime with shared dependencies.
func NewRuntime(
	sessionSvc session.Service,
	gatekeeperModelCfg openaicompat.Config,
	calculatorModelCfg openaicompat.Config,
	matchmakerModelCfg openaicompat.Config,
	auditorModelCfg openaicompat.Config,
	repo repository.LeadsRepository,
	scorer *scoring.Service,
	eventBus events.Bus,
) *Runtime {
	return &Runtime{
		sessionSvc:         sessionSvc,
		gatekeeperModelCfg: gatekeeperModelCfg,
		calculatorModelCfg: calculatorModelCfg,
		matchmakerModelCfg: matchmakerModelCfg,
		auditorModelCfg:    auditorModelCfg,
		repo:               repo,
		scorer:             scorer,
		eventBus:           eventBus,
	}
}

// SetCatalogReader injects the catalog reader.
func (r *Runtime) SetCatalogReader(cr ports.CatalogReader) { r.catalogReader = cr }

// SetQuoteDrafter injects the quote drafter.
func (r *Runtime) SetQuoteDrafter(qd ports.QuoteDrafter) { r.quoteDrafter = qd }

// SetPricingIntelligenceReader injects pricing intelligence.
func (r *Runtime) SetPricingIntelligenceReader(reader ports.PricingIntelligenceReader) {
	r.pricingIntelligence = reader
}

// SetOfferCreator injects the partner offer creator.
func (r *Runtime) SetOfferCreator(creator ports.PartnerOfferCreator) { r.offerCreator = creator }

// SetOrganizationAISettingsReader injects org AI settings.
func (r *Runtime) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	r.orgSettingsReader = reader
}

// SetEmbeddingClient injects the embedding client.
func (r *Runtime) SetEmbeddingClient(client *embeddings.Client) { r.embeddingClient = client }

// SetQdrantClients injects the Qdrant clients.
func (r *Runtime) SetQdrantClients(main, bouwmaat, catalog *qdrant.Client) {
	r.qdrantClient = main
	r.bouwmaatQdrantClient = bouwmaat
	r.catalogQdrantClient = catalog
}

// Run executes the agent for the given payload, routing to the correct workspace.
func (r *Runtime) Run(ctx context.Context, payload AgentTaskPayload) error {
	switch payload.Workspace {
	case "gatekeeper":
		return r.runGatekeeper(ctx, payload)
	case "calculator":
		return r.runCalculator(ctx, payload)
	case "matchmaker":
		return r.runMatchmaker(ctx, payload)
	case "auditor":
		return r.runAuditor(ctx, payload)
	default:
		return fmt.Errorf("unsupported workspace %q", payload.Workspace)
	}
}

func (r *Runtime) runGatekeeper(ctx context.Context, payload AgentTaskPayload) error {
	llm := BuildLLM(r.gatekeeperModelCfg)
	gk, err := newGatekeeper(llm, r.repo, r.eventBus, r.scorer, r.sessionSvc)
	if err != nil {
		return err
	}
	if r.orgSettingsReader != nil {
		gk.SetOrganizationAISettingsReader(r.orgSettingsReader)
	}
	return gk.Run(ctx, payload.LeadID, payload.ServiceID, payload.TenantID)
}

func (r *Runtime) runCalculator(ctx context.Context, payload AgentTaskPayload) error {
	cfg := QuotingAgentConfig{
		ModelConfig:          r.calculatorModelCfg,
		Repo:                 r.repo,
		EventBus:             r.eventBus,
		EmbeddingClient:      r.embeddingClient,
		QdrantClient:         r.qdrantClient,
		BouwmaatQdrantClient: r.bouwmaatQdrantClient,
		CatalogQdrantClient:  r.catalogQdrantClient,
		CatalogReader:        r.catalogReader,
		QuoteDrafter:         r.quoteDrafter,
		PricingIntelligence:  r.pricingIntelligence,
	}

	mode := quotingAgentModeEstimator
	if payload.Mode == "quote-generator" {
		mode = quotingAgentModeQuoteGenerator
	}

	qa, err := newQuotingAgent(cfg, mode, r.sessionSvc)
	if err != nil {
		return err
	}
	if r.orgSettingsReader != nil {
		qa.SetOrganizationAISettingsReader(r.orgSettingsReader)
	}

	if mode == quotingAgentModeEstimator {
		return qa.Execute(ctx, payload.LeadID, payload.ServiceID, payload.TenantID, payload.Force)
	}
	_, err = qa.Generate(ctx, payload.LeadID, payload.ServiceID, payload.TenantID, "", nil, payload.Force)
	return err
}

// Generate implements the QuoteGenerator interface by running the calculator
// workspace in quote-generator mode.
func (r *Runtime) Generate(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, userPrompt string, existingQuoteID *uuid.UUID, force bool) (*GenerateResult, error) {
	cfg := QuotingAgentConfig{
		ModelConfig:          r.calculatorModelCfg,
		Repo:                 r.repo,
		EventBus:             r.eventBus,
		EmbeddingClient:      r.embeddingClient,
		QdrantClient:         r.qdrantClient,
		BouwmaatQdrantClient: r.bouwmaatQdrantClient,
		CatalogQdrantClient:  r.catalogQdrantClient,
		CatalogReader:        r.catalogReader,
		QuoteDrafter:         r.quoteDrafter,
		PricingIntelligence:  r.pricingIntelligence,
	}

	qa, err := newQuotingAgent(cfg, quotingAgentModeQuoteGenerator, r.sessionSvc)
	if err != nil {
		return nil, err
	}
	if r.orgSettingsReader != nil {
		qa.SetOrganizationAISettingsReader(r.orgSettingsReader)
	}

	return qa.Generate(ctx, leadID, serviceID, tenantID, userPrompt, existingQuoteID, force)
}

func (r *Runtime) runMatchmaker(ctx context.Context, payload AgentTaskPayload) error {
	d, err := newDispatcher(r.matchmakerModelCfg, r.repo, r.eventBus, r.sessionSvc)
	if err != nil {
		return err
	}
	if r.orgSettingsReader != nil {
		d.SetOrganizationAISettingsReader(r.orgSettingsReader)
	}
	if r.offerCreator != nil {
		d.SetOfferCreator(r.offerCreator)
	}
	return d.Run(ctx, payload.LeadID, payload.ServiceID, payload.TenantID)
}

func (r *Runtime) runAuditor(ctx context.Context, payload AgentTaskPayload) error {
	a, err := newAuditor(r.auditorModelCfg, r.repo, r.eventBus, r.sessionSvc)
	if err != nil {
		return err
	}
	if payload.AppointmentID != uuid.Nil {
		return a.AuditVisitReport(ctx, payload.LeadID, payload.ServiceID, payload.TenantID, payload.AppointmentID)
	}
	return a.AuditCallLog(ctx, payload.LeadID, payload.ServiceID, payload.TenantID)
}

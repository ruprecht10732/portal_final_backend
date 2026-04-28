package leads

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/logger"
)

// Orchestrator routes pipeline events to specialized agents.
type Orchestrator struct {
	gatekeeper      *agent.Gatekeeper
	estimator       agent.Estimator
	dispatcher      *agent.Dispatcher
	auditor         *agent.Auditor
	repo            repository.LeadsRepository
	outbox          *notificationoutbox.Repository
	eventBus        events.Bus
	sse             *sse.Service
	log             *logger.Logger
	automationQueue AutomationScheduler

	orgSettingsReader ports.OrganizationAISettingsReader
	orgSettingsMu     sync.Mutex
	orgSettingsCache  map[uuid.UUID]cachedOrgAISettings
	runLocker         orchestratorRunLocker
	gatekeeperDeduper gatekeeperTriggerDeduper
	estimatorDeduper  triggerFingerprintDeduper
	dispatcherDeduper triggerFingerprintDeduper

	reconciliationEnabled bool

	// Dedup short-window duplicate stage-change events.
	recentStageEvents map[string]time.Time
	stageEventsMu     sync.Mutex // dedicated lock for recentStageEvents only
}

type cachedOrgAISettings struct {
	settings  ports.OrganizationAISettings
	fetchedAt time.Time
}

const (
	staleDraftDuration          = 30 * 24 * time.Hour
	stageEventDedupWindow       = 5 * time.Second
	minimumEstimationConfidence = 0.45
	orchestratorAutomationLog   = "orchestrator: automation decision"
)

type OrchestratorAgents struct {
	Gatekeeper *agent.Gatekeeper
	Estimator  agent.Estimator
	Dispatcher *agent.Dispatcher
	Auditor    *agent.Auditor
}

func NewOrchestrator(
	agents OrchestratorAgents,
	repo repository.LeadsRepository,
	outbox *notificationoutbox.Repository,
	eventBus events.Bus,
	sse *sse.Service,
	log *logger.Logger,
	runLocker orchestratorRunLocker,
) *Orchestrator {
	if runLocker == nil {
		runLocker = newInMemoryOrchestratorRunLocker()
	}

	return &Orchestrator{
		gatekeeper:            agents.Gatekeeper,
		estimator:             agents.Estimator,
		dispatcher:            agents.Dispatcher,
		auditor:               agents.Auditor,
		repo:                  repo,
		outbox:                outbox,
		eventBus:              eventBus,
		sse:                   sse,
		log:                   log,
		runLocker:             runLocker,
		reconciliationEnabled: true,
		recentStageEvents:     make(map[string]time.Time),
		orgSettingsCache:      make(map[uuid.UUID]cachedOrgAISettings),
	}
}

// SetOrganizationAISettingsReader injects the tenant settings reader.
// When unset, the orchestrator falls back to default behavior.
func (o *Orchestrator) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	o.orgSettingsMu.Lock()
	defer o.orgSettingsMu.Unlock()
	o.orgSettingsReader = reader
	// Keep cache; reader replacement is rare and cache TTL is short.
}

func (o *Orchestrator) SetAutomationScheduler(queue AutomationScheduler) {
	if queue == nil {
		if o.log != nil {
			o.log.Error("orchestrator: SetAutomationScheduler called with nil queue")
		}
		panic("orchestrator: automation scheduler is required")
	}
	o.automationQueue = queue
}

func (o *Orchestrator) loadOrgAISettings(ctx context.Context, tenantID uuid.UUID) (ports.OrganizationAISettings, error) {
	// Fast path: check cache with only a read lock.
	o.orgSettingsMu.Lock()
	if cached, ok := o.orgSettingsCache[tenantID]; ok {
		if time.Since(cached.fetchedAt) < 30*time.Second {
			o.orgSettingsMu.Unlock()
			return cached.settings, nil
		}
	}
	o.orgSettingsMu.Unlock()

	if o.orgSettingsReader == nil {
		settings := ports.DefaultOrganizationAISettings()
		o.orgSettingsMu.Lock()
		o.orgSettingsCache[tenantID] = cachedOrgAISettings{settings: settings, fetchedAt: time.Now()}
		o.orgSettingsMu.Unlock()
		return settings, nil
	}

	// Blocking I/O is performed WITHOUT the global mutex so that a slow
	// settings reader for one tenant cannot stall the entire orchestrator.
	settings, err := o.orgSettingsReader(ctx, tenantID)
	if err != nil {
		return ports.OrganizationAISettings{}, err
	}

	o.orgSettingsMu.Lock()
	o.orgSettingsCache[tenantID] = cachedOrgAISettings{settings: settings, fetchedAt: time.Now()}
	// Opportunistically evict stale entries to prevent unbounded growth.
	for id, cached := range o.orgSettingsCache {
		if id != tenantID && time.Since(cached.fetchedAt) > 2*time.Minute {
			delete(o.orgSettingsCache, id)
		}
	}
	o.orgSettingsMu.Unlock()
	return settings, nil
}


func (o *Orchestrator) SetReconciliationEnabled(enabled bool) {
	o.reconciliationEnabled = enabled
}



func (o *Orchestrator) cancelPendingWorkflows(ctx context.Context, tenantID, leadID uuid.UUID, trigger string) {
	if o.outbox == nil {
		return
	}

	cancelled, err := o.outbox.CancelPendingForLead(ctx, tenantID, leadID)
	if err != nil {
		o.log.Error("orchestrator: failed to cancel pending workflow outbox messages", "tenantId", tenantID, "leadId", leadID, "trigger", trigger, "error", err)
		return
	}

	if cancelled > 0 {
		o.log.Info("orchestrator: cancelled pending workflow outbox messages", "tenantId", tenantID, "leadId", leadID, "trigger", trigger, "count", cancelled)
	}
}

func (o *Orchestrator) markReconciliationRunning(ctx context.Context, serviceID uuid.UUID) bool {
	ok, err := o.runLocker.TryAcquireReconciliation(ctx, serviceID)
	if err != nil {
		o.log.Error("orchestrator: failed to acquire reconciliation lock", "error", err, "serviceId", serviceID)
		return false
	}
	return ok
}

func (o *Orchestrator) markReconciliationComplete(ctx context.Context, serviceID uuid.UUID) {
	if err := o.runLocker.ReleaseReconciliation(ctx, serviceID); err != nil {
		o.log.Warn("orchestrator: failed to release reconciliation lock", "error", err, "serviceId", serviceID)
	}
}

func (o *Orchestrator) shouldSkipDuplicateStageEvent(evt events.PipelineStageChanged) bool {
	o.stageEventsMu.Lock()
	defer o.stageEventsMu.Unlock()

	now := time.Now()
	key := evt.LeadServiceID.String() + ":" + evt.OldStage + "->" + evt.NewStage

	if ts, ok := o.recentStageEvents[key]; ok && now.Sub(ts) <= stageEventDedupWindow {
		return true
	}
	o.recentStageEvents[key] = now
	return false
}

// StartCleanupLoop runs a background goroutine that periodically evicts expired
// entries from recentStageEvents so the map does not grow unboundedly.
func (o *Orchestrator) StartCleanupLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				o.cleanupStageEvents()
			}
		}
	}()
}

func (o *Orchestrator) cleanupStageEvents() {
	o.stageEventsMu.Lock()
	defer o.stageEventsMu.Unlock()
	now := time.Now()
	for key, ts := range o.recentStageEvents {
		if now.Sub(ts) > stageEventDedupWindow {
			delete(o.recentStageEvents, key)
		}
	}
}


// ShouldRunAgent checks if a service is eligible for agent processing.
// Returns false if the service is in a terminal state.
func (o *Orchestrator) ShouldRunAgent(service repository.LeadService) bool {
	if domain.IsTerminal(service.Status, service.PipelineStage) {
		o.log.Info(orchestratorAutomationLog,
			"agent", "any",
			"decision", "skip",
			"reason", "terminal_service",
			"serviceId", service.ID,
			"status", service.Status,
			"pipelineStage", service.PipelineStage)
		return false
	}
	return true
}



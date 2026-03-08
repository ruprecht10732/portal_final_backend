package leads

import (
	"context"
	"testing"
	"time"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type fakeAutomationScheduler struct {
	gatekeeperPayloads []scheduler.GatekeeperRunPayload
	estimatorPayloads  []scheduler.EstimatorRunPayload
	dispatcherPayloads []scheduler.DispatcherRunPayload
	photoPayloads      []scheduler.PhotoAnalysisPayload
	photoDelays        []time.Duration
	auditVisitPayloads []scheduler.AuditVisitReportPayload
	auditCallPayloads  []scheduler.AuditCallLogPayload
}

func (f *fakeAutomationScheduler) EnqueueGatekeeperRun(_ context.Context, payload scheduler.GatekeeperRunPayload) error {
	f.gatekeeperPayloads = append(f.gatekeeperPayloads, payload)
	return nil
}

func (f *fakeAutomationScheduler) EnqueueEstimatorRun(_ context.Context, payload scheduler.EstimatorRunPayload) error {
	f.estimatorPayloads = append(f.estimatorPayloads, payload)
	return nil
}

func (f *fakeAutomationScheduler) EnqueueDispatcherRun(_ context.Context, payload scheduler.DispatcherRunPayload) error {
	f.dispatcherPayloads = append(f.dispatcherPayloads, payload)
	return nil
}

func (f *fakeAutomationScheduler) EnqueuePhotoAnalysis(_ context.Context, payload scheduler.PhotoAnalysisPayload) error {
	f.photoPayloads = append(f.photoPayloads, payload)
	f.photoDelays = append(f.photoDelays, 0)
	return nil
}

func (f *fakeAutomationScheduler) EnqueuePhotoAnalysisIn(_ context.Context, payload scheduler.PhotoAnalysisPayload, delay time.Duration) error {
	f.photoPayloads = append(f.photoPayloads, payload)
	f.photoDelays = append(f.photoDelays, delay)
	return nil
}

func (f *fakeAutomationScheduler) EnqueueAuditVisitReport(_ context.Context, payload scheduler.AuditVisitReportPayload) error {
	f.auditVisitPayloads = append(f.auditVisitPayloads, payload)
	return nil
}

func (f *fakeAutomationScheduler) EnqueueAuditCallLog(_ context.Context, payload scheduler.AuditCallLogPayload) error {
	f.auditCallPayloads = append(f.auditCallPayloads, payload)
	return nil
}

func TestHandleEstimationStageEnqueuesEstimatorWhenSchedulerConfigured(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	o := &Orchestrator{
		automationQueue:  queue,
		log:              logger.New("development"),
		orgSettingsCache: make(map[uuid.UUID]cachedOrgAISettings),
	}

	evt := eventsPipelineStageChanged(domain.PipelineStageEstimation)
	o.handleEstimationStage(evt)

	if len(queue.estimatorPayloads) != 1 {
		t.Fatalf("expected estimator job to be enqueued, got %d", len(queue.estimatorPayloads))
	}
	if queue.estimatorPayloads[0].LeadServiceID != evt.LeadServiceID.String() {
		t.Fatalf("unexpected lead service id: %#v", queue.estimatorPayloads[0])
	}
}

func TestHandleFulfillmentStageEnqueuesDispatcherWhenSchedulerConfigured(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	o := &Orchestrator{
		automationQueue:  queue,
		log:              logger.New("development"),
		orgSettingsCache: make(map[uuid.UUID]cachedOrgAISettings),
	}
	o.SetOrganizationAISettingsReader(func(context.Context, uuid.UUID) (ports.OrganizationAISettings, error) {
		settings := ports.DefaultOrganizationAISettings()
		settings.AIAutoDispatch = true
		return settings, nil
	})

	evt := eventsPipelineStageChanged(domain.PipelineStageFulfillment)
	o.handleFulfillmentStage(evt)

	if len(queue.dispatcherPayloads) != 1 {
		t.Fatalf("expected dispatcher job to be enqueued, got %d", len(queue.dispatcherPayloads))
	}
}

func TestMaybeRunAuditorForCallLogEnqueuesAuditWhenSchedulerConfigured(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	o := &Orchestrator{
		automationQueue: queue,
		auditor:         &agent.Auditor{},
		log:             logger.New("development"),
	}

	evt := eventsLeadDataChanged("call_log")
	o.maybeRunAuditorForCallLog(evt)

	if len(queue.auditCallPayloads) != 1 {
		t.Fatalf("expected call-log audit job to be enqueued, got %d", len(queue.auditCallPayloads))
	}
}

func TestMaybeRunGatekeeperForDataChangeEnqueuesGatekeeperWhenSchedulerConfigured(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	o := &Orchestrator{
		automationQueue: queue,
		log:             logger.New("development"),
	}
	svc := repositoryLeadService(domain.PipelineStageTriage)
	evt := eventsLeadDataChanged("user_update")

	o.maybeRunGatekeeperForDataChange(svc, evt)

	if len(queue.gatekeeperPayloads) != 1 {
		t.Fatalf("expected gatekeeper job to be enqueued, got %d", len(queue.gatekeeperPayloads))
	}
}

func TestMaybeRunGatekeeperForDataChangeSkipsManualIntervention(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	o := &Orchestrator{
		automationQueue: queue,
		log:             logger.New("development"),
	}
	svc := repositoryLeadService(domain.PipelineStageManualIntervention)
	evt := eventsLeadDataChanged("user_update")

	o.maybeRunGatekeeperForDataChange(svc, evt)

	if len(queue.gatekeeperPayloads) != 0 {
		t.Fatalf("expected no gatekeeper job to be enqueued while in manual intervention, got %d", len(queue.gatekeeperPayloads))
	}
}

func TestEstimatorBlockerLoopScenarioQueuesEstimatorThenGatekeeperReplyReview(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	o := &Orchestrator{
		automationQueue:  queue,
		log:              logger.New("development"),
		orgSettingsCache: make(map[uuid.UUID]cachedOrgAISettings),
	}

	estimationEvt := eventsPipelineStageChanged(domain.PipelineStageEstimation)
	o.handleEstimationStage(estimationEvt)

	if len(queue.estimatorPayloads) != 1 {
		t.Fatalf("expected estimator job to be enqueued once, got %d", len(queue.estimatorPayloads))
	}

	replyEvt := events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        estimationEvt.LeadID,
		LeadServiceID: estimationEvt.LeadServiceID,
		TenantID:      estimationEvt.TenantID,
		Source:        "note",
	}
	o.maybeRunGatekeeperForDataChange(repositoryLeadService(domain.PipelineStageNurturing), replyEvt)

	if len(queue.gatekeeperPayloads) != 1 {
		t.Fatalf("expected customer reply in Nurturing to queue one gatekeeper review, got %d", len(queue.gatekeeperPayloads))
	}
	if queue.gatekeeperPayloads[0].LeadServiceID != estimationEvt.LeadServiceID.String() {
		t.Fatalf("unexpected gatekeeper lead service id: %#v", queue.gatekeeperPayloads[0])
	}
}

func TestQueueOrRunPhotoAnalysisEnqueuesDelayedTaskWhenSchedulerConfigured(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	module := &Module{automationQueue: queue, photoAnalysisHandler: &handler.PhotoAnalysisHandler{}}
	leadID := uuid.New()
	serviceID := uuid.New()
	tenantID := uuid.New()

	queueOrRunPhotoAnalysis(context.Background(), module, logger.New("development"), leadID, serviceID, tenantID)

	if len(queue.photoPayloads) != 1 {
		t.Fatalf("expected photo analysis job to be enqueued, got %d", len(queue.photoPayloads))
	}
	if queue.photoDelays[0] != 30*time.Second {
		t.Fatalf("expected 30s enqueue delay, got %s", queue.photoDelays[0])
	}
	if queue.photoPayloads[0].LeadID != leadID.String() {
		t.Fatalf("unexpected photo payload: %#v", queue.photoPayloads[0])
	}
}

func eventsPipelineStageChanged(newStage string) events.PipelineStageChanged {
	return events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        uuid.New(),
		LeadServiceID: uuid.New(),
		TenantID:      uuid.New(),
		OldStage:      domain.PipelineStageTriage,
		NewStage:      newStage,
	}
}

func eventsLeadDataChanged(source string) events.LeadDataChanged {
	return events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        uuid.New(),
		LeadServiceID: uuid.New(),
		TenantID:      uuid.New(),
		Source:        source,
	}
}

func repositoryLeadService(stage string) repository.LeadService {
	return repository.LeadService{PipelineStage: stage}
}

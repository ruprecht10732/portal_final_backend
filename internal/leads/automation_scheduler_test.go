package leads

import (
	"context"
	"testing"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type fakeAutomationScheduler struct {
	agentTaskPayloads []scheduler.AgentTaskPayload
}

func (f *fakeAutomationScheduler) EnqueueAgentTask(_ context.Context, payload scheduler.AgentTaskPayload) error {
	f.agentTaskPayloads = append(f.agentTaskPayloads, payload)
	return nil
}

func (f *fakeAutomationScheduler) EnqueueLogCall(_ context.Context, _ scheduler.LogCallPayload) error {
	return nil
}

func (f *fakeAutomationScheduler) EnqueueStaleLeadReEngage(_ context.Context, _ scheduler.StaleLeadReEngagePayload) error {
	return nil
}

func (f *fakeAutomationScheduler) gatekeeperPayloads() []scheduler.AgentTaskPayload {
	var out []scheduler.AgentTaskPayload
	for _, p := range f.agentTaskPayloads {
		if p.Workspace == "gatekeeper" {
			out = append(out, p)
		}
	}
	return out
}

func (f *fakeAutomationScheduler) estimatorPayloads() []scheduler.AgentTaskPayload {
	var out []scheduler.AgentTaskPayload
	for _, p := range f.agentTaskPayloads {
		if p.Workspace == "calculator" && p.Mode == "estimator" {
			out = append(out, p)
		}
	}
	return out
}

func (f *fakeAutomationScheduler) dispatcherPayloads() []scheduler.AgentTaskPayload {
	var out []scheduler.AgentTaskPayload
	for _, p := range f.agentTaskPayloads {
		if p.Workspace == "matchmaker" {
			out = append(out, p)
		}
	}
	return out
}

func (f *fakeAutomationScheduler) auditCallPayloads() []scheduler.AgentTaskPayload {
	var out []scheduler.AgentTaskPayload
	for _, p := range f.agentTaskPayloads {
		if p.Workspace == "auditor" && p.AppointmentID == "" {
			out = append(out, p)
		}
	}
	return out
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

	if len(queue.estimatorPayloads()) != 1 {
		t.Fatalf("expected estimator job to be enqueued, got %d", len(queue.estimatorPayloads()))
	}
	if queue.estimatorPayloads()[0].LeadServiceID != evt.LeadServiceID.String() {
		t.Fatalf("unexpected lead service id: %#v", queue.estimatorPayloads()[0])
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

	if len(queue.dispatcherPayloads()) != 1 {
		t.Fatalf("expected dispatcher job to be enqueued, got %d", len(queue.dispatcherPayloads()))
	}
}

func TestMaybeRunAuditorForCallLogEnqueuesAuditWhenSchedulerConfigured(t *testing.T) {
	queue := &fakeAutomationScheduler{}
	o := &Orchestrator{
		automationQueue: queue,
		runtime:         &agent.Runtime{},
		log:             logger.New("development"),
	}

	evt := eventsLeadDataChanged("call_log")
	o.maybeRunAuditorForCallLog(evt)

	if len(queue.auditCallPayloads()) != 1 {
		t.Fatalf("expected call-log audit job to be enqueued, got %d", len(queue.auditCallPayloads()))
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

	if len(queue.gatekeeperPayloads()) != 1 {
		t.Fatalf("expected gatekeeper job to be enqueued, got %d", len(queue.gatekeeperPayloads()))
	}
}

func TestInitialGatekeeperBurstCollapsesWithFollowUpDataChange(t *testing.T) {
	leadID := uuid.New()
	serviceID := uuid.New()
	tenantID := uuid.New()
	queue := &queueUniqueGatekeeperScheduler{}
	repo := &gatekeeperFingerprintRepoStub{
		lead: repository.Lead{
			ID:                 leadID,
			ConsumerFirstName:  "Jane",
			ConsumerLastName:   "Doe",
			ConsumerPhone:      "+31612345678",
			AddressStreet:      "Voorbeeldstraat",
			AddressHouseNumber: "12",
			AddressZipCode:     "1234AB",
			AddressCity:        "Amsterdam",
			WhatsAppOptedIn:    true,
		},
		service: repository.LeadService{
			ID:             serviceID,
			LeadID:         leadID,
			OrganizationID: tenantID,
			PipelineStage:  domain.PipelineStageTriage,
			ServiceType:    "Algemeen",
		},
	}

	if !maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{
		ctx:       context.Background(),
		repo:      repo,
		queue:     queue,
		log:       logger.New("development"),
		leadID:    leadID,
		serviceID: serviceID,
		tenantID:  tenantID,
		source:    "lead created",
	}) {
		t.Fatalf("expected initial gatekeeper enqueue helper to handle lead-created trigger")
	}

	o := &Orchestrator{
		automationQueue: queue,
		log:             logger.New("development"),
	}
	repo.notes = []repository.LeadNote{{ID: uuid.New(), LeadID: leadID, OrganizationID: tenantID, Type: "note", Body: "Klant stuurde extra details"}}
	o.maybeRunGatekeeperForDataChange(repository.LeadService{PipelineStage: domain.PipelineStageTriage}, events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        leadID,
		LeadServiceID: serviceID,
		TenantID:      tenantID,
		Source:        "user_update",
	})

	if len(queue.gatekeeperPayloads()) != 1 {
		t.Fatalf("expected lead-created enqueue plus immediate data-change follow-up to collapse into one queue entry, got %d", len(queue.gatekeeperPayloads()))
	}
	if queue.gatekeeperPayloads()[0].LeadServiceID != serviceID.String() {
		t.Fatalf("unexpected gatekeeper payload after burst collapse: %#v", queue.gatekeeperPayloads()[0])
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

	if len(queue.gatekeeperPayloads()) != 0 {
		t.Fatalf("expected no gatekeeper job to be enqueued while in manual intervention, got %d", len(queue.gatekeeperPayloads()))
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

	if len(queue.estimatorPayloads()) != 1 {
		t.Fatalf("expected estimator job to be enqueued once, got %d", len(queue.estimatorPayloads()))
	}

	replyEvt := events.LeadDataChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        estimationEvt.LeadID,
		LeadServiceID: estimationEvt.LeadServiceID,
		TenantID:      estimationEvt.TenantID,
		Source:        "note",
	}
	o.maybeRunGatekeeperForDataChange(repositoryLeadService(domain.PipelineStageNurturing), replyEvt)

	if len(queue.gatekeeperPayloads()) != 1 {
		t.Fatalf("expected customer reply in Nurturing to queue one gatekeeper review, got %d", len(queue.gatekeeperPayloads()))
	}
	if queue.gatekeeperPayloads()[0].LeadServiceID != estimationEvt.LeadServiceID.String() {
		t.Fatalf("unexpected gatekeeper lead service id: %#v", queue.gatekeeperPayloads()[0])
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

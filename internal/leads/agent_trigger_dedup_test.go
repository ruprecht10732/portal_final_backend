package leads

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/logger"
)

type estimatorFingerprintRepoStub struct {
	lead     repository.Lead
	service  repository.LeadService
	notes    []repository.LeadNote
	analysis *repository.AIAnalysis
}

func (s *estimatorFingerprintRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.Lead, error) {
	return s.lead, nil
}

func (s *estimatorFingerprintRepoStub) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	return s.service, nil
}

func (s *estimatorFingerprintRepoStub) ListNotesByService(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID) ([]repository.LeadNote, error) {
	return append([]repository.LeadNote(nil), s.notes...), nil
}

func (s *estimatorFingerprintRepoStub) GetLatestAIAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.AIAnalysis, error) {
	if s.analysis == nil {
		return repository.AIAnalysis{}, repository.ErrNotFound
	}
	return *s.analysis, nil
}

type dispatcherFingerprintRepoStub struct {
	lead               repository.Lead
	service            repository.LeadService
	aggs               repository.ServiceStateAggregates
	excludedPartnerIDs []uuid.UUID
	linkedPartners     bool
}

func (s *dispatcherFingerprintRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.Lead, error) {
	return s.lead, nil
}

func (s *dispatcherFingerprintRepoStub) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	return s.service, nil
}

func (s *dispatcherFingerprintRepoStub) GetServiceStateAggregates(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.ServiceStateAggregates, error) {
	return s.aggs, nil
}

func (s *dispatcherFingerprintRepoStub) GetInvitedPartnerIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return append([]uuid.UUID(nil), s.excludedPartnerIDs...), nil
}

func (s *dispatcherFingerprintRepoStub) HasLinkedPartners(_ context.Context, _ uuid.UUID, _ uuid.UUID) (bool, error) {
	return s.linkedPartners, nil
}

func TestMaybeEnqueueEstimatorRunSkipsUnchangedFingerprint(t *testing.T) {
	ctx := context.Background()
	leadID := uuid.New()
	serviceID := uuid.New()
	tenantID := uuid.New()
	queue := &fakeAutomationScheduler{}
	repo := &estimatorFingerprintRepoStub{
		lead: repository.Lead{
			ID:                 leadID,
			ConsumerFirstName:  "Jane",
			ConsumerLastName:   "Doe",
			ConsumerPhone:      "+31612345678",
			AddressStreet:      "Voorbeeldstraat",
			AddressHouseNumber: "12",
			AddressZipCode:     "1234AB",
			AddressCity:        "Amsterdam",
		},
		service: repository.LeadService{
			ID:             serviceID,
			LeadID:         leadID,
			OrganizationID: tenantID,
			PipelineStage:  "Estimation",
			ServiceType:    "Isolatie",
		},
		analysis: &repository.AIAnalysis{
			LeadID:            leadID,
			LeadServiceID:     serviceID,
			OrganizationID:    tenantID,
			LeadQuality:       "Hot",
			RecommendedAction: "Estimate",
			Summary:           "Klaar voor offerte",
		},
	}
	deduper := newTriggerFingerprintDeduper(nil, time.Hour, estimatorTriggerFingerprintPrefix, logger.New("development"))

	if !maybeEnqueueEstimatorRun(estimatorEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, force: false, source: "pipeline_stage_change"}) {
		t.Fatalf("expected estimator enqueue helper to handle trigger")
	}
	if !maybeEnqueueEstimatorRun(estimatorEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, force: false, source: "pipeline_stage_change"}) {
		t.Fatalf("expected duplicate estimator trigger to be handled")
	}
	if len(queue.estimatorPayloads) != 1 {
		t.Fatalf("expected unchanged estimator rerun to be skipped, got %d enqueues", len(queue.estimatorPayloads))
	}
	if queue.estimatorPayloads[0].Fingerprint == "" {
		t.Fatalf("expected estimator fingerprint to be populated")
	}

	repo.analysis.Summary = "Klaar voor offerte met extra detail"
	maybeEnqueueEstimatorRun(estimatorEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, force: false, source: "analysis_updated"})
	if len(queue.estimatorPayloads) != 2 {
		t.Fatalf("expected material estimator change to enqueue a second run, got %d", len(queue.estimatorPayloads))
	}
}

func TestMaybeEnqueueEstimatorRunBypassesDedupeWhenForced(t *testing.T) {
	ctx := context.Background()
	leadID := uuid.New()
	serviceID := uuid.New()
	tenantID := uuid.New()
	queue := &fakeAutomationScheduler{}
	repo := &estimatorFingerprintRepoStub{
		lead:    repository.Lead{ID: leadID, AddressZipCode: "1234AB"},
		service: repository.LeadService{ID: serviceID, LeadID: leadID, OrganizationID: tenantID, PipelineStage: "Estimation", ServiceType: "Isolatie"},
	}
	deduper := newTriggerFingerprintDeduper(nil, time.Hour, estimatorTriggerFingerprintPrefix, logger.New("development"))

	maybeEnqueueEstimatorRun(estimatorEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, force: true, source: "manual_force"})
	maybeEnqueueEstimatorRun(estimatorEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, force: true, source: "manual_force"})

	if len(queue.estimatorPayloads) != 2 {
		t.Fatalf("expected forced estimator runs to bypass dedupe, got %d enqueues", len(queue.estimatorPayloads))
	}
	if queue.estimatorPayloads[0].Fingerprint != "" || queue.estimatorPayloads[1].Fingerprint != "" {
		t.Fatalf("expected forced estimator payloads to omit semantic fingerprint")
	}
}

func TestMaybeEnqueueDispatcherRunSkipsUnchangedFingerprint(t *testing.T) {
	ctx := context.Background()
	leadID := uuid.New()
	serviceID := uuid.New()
	tenantID := uuid.New()
	queue := &fakeAutomationScheduler{}
	repo := &dispatcherFingerprintRepoStub{
		lead: repository.Lead{
			ID:             leadID,
			AddressZipCode: "1234AB",
			AddressCity:    "Amsterdam",
		},
		service: repository.LeadService{
			ID:             serviceID,
			LeadID:         leadID,
			OrganizationID: tenantID,
			PipelineStage:  "Fulfillment",
			ServiceType:    "Isolatie",
		},
	}
	deduper := newTriggerFingerprintDeduper(nil, time.Hour, dispatcherTriggerFingerprintPrefix, logger.New("development"))

	if !maybeEnqueueDispatcherRun(dispatcherEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, source: "pipeline_stage_change"}) {
		t.Fatalf("expected dispatcher enqueue helper to handle trigger")
	}
	if !maybeEnqueueDispatcherRun(dispatcherEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, source: "partner_offer_expired"}) {
		t.Fatalf("expected duplicate dispatcher trigger to be handled")
	}
	if len(queue.dispatcherPayloads) != 1 {
		t.Fatalf("expected unchanged dispatcher rerun to be skipped, got %d enqueues", len(queue.dispatcherPayloads))
	}
	if queue.dispatcherPayloads[0].Fingerprint == "" {
		t.Fatalf("expected dispatcher fingerprint to be populated")
	}

	repo.excludedPartnerIDs = []uuid.UUID{uuid.New()}
	maybeEnqueueDispatcherRun(dispatcherEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, source: "partner_offer_rejected"})
	if len(queue.dispatcherPayloads) != 2 {
		t.Fatalf("expected changed dispatcher exclusions to enqueue a second run, got %d", len(queue.dispatcherPayloads))
	}
}

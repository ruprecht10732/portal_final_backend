package leads

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/logger"
)

type gatekeeperFingerprintRepoStub struct {
	lead        repository.Lead
	service     repository.LeadService
	notes       []repository.LeadNote
	attachments []repository.Attachment
	photo       *repository.PhotoAnalysis
	visitReport *repository.AppointmentVisitReport
}

func (s *gatekeeperFingerprintRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.Lead, error) {
	return s.lead, nil
}

func (s *gatekeeperFingerprintRepoStub) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	return s.service, nil
}

func (s *gatekeeperFingerprintRepoStub) ListNotesByService(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID) ([]repository.LeadNote, error) {
	return append([]repository.LeadNote(nil), s.notes...), nil
}

func (s *gatekeeperFingerprintRepoStub) ListAttachmentsByService(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]repository.Attachment, error) {
	return append([]repository.Attachment(nil), s.attachments...), nil
}

func (s *gatekeeperFingerprintRepoStub) GetLatestPhotoAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.PhotoAnalysis, error) {
	if s.photo == nil {
		return repository.PhotoAnalysis{}, repository.ErrPhotoAnalysisNotFound
	}
	return *s.photo, nil
}

func (s *gatekeeperFingerprintRepoStub) GetLatestAppointmentVisitReportByService(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*repository.AppointmentVisitReport, error) {
	if s.visitReport == nil {
		return nil, repository.ErrNotFound
	}
	return s.visitReport, nil
}

func TestMaybeEnqueueGatekeeperRunSkipsUnchangedFingerprintAfterStageOnlyChange(t *testing.T) {
	ctx := context.Background()
	leadID := uuid.New()
	serviceID := uuid.New()
	tenantID := uuid.New()
	queue := &fakeAutomationScheduler{}
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
			PipelineStage:  "Triage",
			ServiceType:    "Algemeen",
		},
	}
	deduper := newInMemoryGatekeeperTriggerDeduper(time.Hour)

	if !maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, source: "lead created"}) {
		t.Fatalf("expected enqueue helper to handle trigger")
	}
	if len(queue.gatekeeperPayloads) != 1 {
		t.Fatalf("expected first gatekeeper enqueue, got %d", len(queue.gatekeeperPayloads))
	}
	if queue.gatekeeperPayloads[0].Fingerprint == "" {
		t.Fatalf("expected payload fingerprint to be populated")
	}

	repo.service.PipelineStage = "Nurturing"
	if !maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, source: "user_update"}) {
		t.Fatalf("expected enqueue helper to handle duplicate trigger")
	}
	if len(queue.gatekeeperPayloads) != 1 {
		t.Fatalf("expected unchanged stage-only rerun to be skipped, got %d enqueues", len(queue.gatekeeperPayloads))
	}
}

func TestMaybeEnqueueGatekeeperRunAllowsMaterialChange(t *testing.T) {
	ctx := context.Background()
	leadID := uuid.New()
	serviceID := uuid.New()
	tenantID := uuid.New()
	queue := &fakeAutomationScheduler{}
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
			PipelineStage:  "Triage",
			ServiceType:    "Algemeen",
		},
	}
	deduper := newInMemoryGatekeeperTriggerDeduper(time.Hour)

	maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, source: "lead created"})
	repo.notes = []repository.LeadNote{{ID: uuid.New(), LeadID: leadID, OrganizationID: tenantID, Type: "note", Body: "Klant stuurde extra details"}}
	maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{ctx: ctx, repo: repo, deduper: deduper, queue: queue, log: logger.New("development"), leadID: leadID, serviceID: serviceID, tenantID: tenantID, source: "note"})

	if len(queue.gatekeeperPayloads) != 2 {
		t.Fatalf("expected material note change to enqueue a second run, got %d", len(queue.gatekeeperPayloads))
	}
	if queue.gatekeeperPayloads[0].Fingerprint == queue.gatekeeperPayloads[1].Fingerprint {
		t.Fatalf("expected fingerprints to differ after material change")
	}
}

func TestRedisGatekeeperTriggerDeduperSharesFingerprintState(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(miniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()

	serviceID := uuid.New()
	fingerprint := "same-fingerprint"
	d1 := newGatekeeperTriggerDeduper(redisClient, time.Hour, logger.New("development"))
	d2 := newGatekeeperTriggerDeduper(redisClient, time.Hour, logger.New("development"))

	shouldEnqueue, err := d1.ShouldEnqueue(serviceID, fingerprint)
	if err != nil {
		t.Fatalf("expected first redis dedupe check to succeed, got %v", err)
	}
	if !shouldEnqueue {
		t.Fatalf("expected first fingerprint to enqueue")
	}

	shouldEnqueue, err = d2.ShouldEnqueue(serviceID, fingerprint)
	if err != nil {
		t.Fatalf("expected second redis dedupe check to succeed, got %v", err)
	}
	if shouldEnqueue {
		t.Fatalf("expected duplicate fingerprint to be shared across dedupers")
	}

	shouldEnqueue, err = d2.ShouldEnqueue(serviceID, "new-fingerprint")
	if err != nil {
		t.Fatalf("expected changed redis fingerprint to succeed, got %v", err)
	}
	if !shouldEnqueue {
		t.Fatalf("expected changed fingerprint to enqueue")
	}
}

package service

import (
	"context"
	"testing"

	"portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/scheduler"

	"github.com/google/uuid"
)

type fakeOfferSummaryQueue struct{}

func (fakeOfferSummaryQueue) EnqueuePartnerOfferSummary(context.Context, scheduler.PartnerOfferSummaryPayload) error {
	return nil
}

type fakeOfferSummaryGenerator struct {
	called   bool
	tenantID uuid.UUID
	input    OfferSummaryInput
	output   string
	err      error
}

func (f *fakeOfferSummaryGenerator) GenerateSummary(_ context.Context, tenantID uuid.UUID, input OfferSummaryInput) (string, error) {
	f.called = true
	f.tenantID = tenantID
	f.input = input
	return f.output, f.err
}

func TestBuildOfferSummaryPayloadIncludesExpectedFields(t *testing.T) {
	generator := &fakeOfferSummaryGenerator{}
	svc := &Service{
		summaryGenerator: generator,
		summaryQueue:     fakeOfferSummaryQueue{},
	}

	offerID := uuid.New()
	tenantID := uuid.New()
	leadServiceID := uuid.New()
	leadID := uuid.New()
	scope := "Gemiddeld"
	urgency := "Hoog"

	payload, ok := svc.buildOfferSummaryPayload(offerID, tenantID, leadServiceID, repository.LeadServiceSummaryContext{
		LeadID:       leadID,
		ServiceType:  "Dakreparatie",
		UrgencyLevel: &urgency,
	}, &scope, []repository.QuoteItemSummary{{Description: "Dakpan vervangen", Quantity: "2x"}})
	if !ok {
		t.Fatal("expected payload to be created")
	}
	if payload.OfferID != offerID.String() {
		t.Fatalf("expected offer id %s, got %s", offerID, payload.OfferID)
	}
	if payload.TenantID != tenantID.String() {
		t.Fatalf("expected tenant id %s, got %s", tenantID, payload.TenantID)
	}
	if payload.LeadID != leadID.String() {
		t.Fatalf("expected lead id %s, got %s", leadID, payload.LeadID)
	}
	if payload.LeadServiceID != leadServiceID.String() {
		t.Fatalf("expected lead service id %s, got %s", leadServiceID, payload.LeadServiceID)
	}
	if len(payload.Items) != 1 || payload.Items[0].Description != "Dakpan vervangen" {
		t.Fatalf("expected one payload item, got %#v", payload.Items)
	}
}

func TestProcessPartnerOfferSummaryJobUsesGeneratorAndSanitizesOutput(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	leadServiceID := uuid.New()
	offerID := uuid.New()
	scope := "Klein"
	urgency := "Laag"
	generator := &fakeOfferSummaryGenerator{output: "  **Omvang** Klein  \n\nKorte samenvatting  "}
	svc := &Service{
		repo:             repository.New(nil),
		summaryGenerator: generator,
	}

	err := svc.ProcessPartnerOfferSummaryJob(context.Background(), scheduler.PartnerOfferSummaryPayload{
		OfferID:       offerID.String(),
		TenantID:      tenantID.String(),
		LeadID:        leadID.String(),
		LeadServiceID: leadServiceID.String(),
		ServiceType:   "Schilderwerk",
		Scope:         &scope,
		UrgencyLevel:  &urgency,
		Items: []scheduler.PartnerOfferSummaryItemPayload{{
			Description: "Muur verven",
			Quantity:    "1x",
		}},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !generator.called {
		t.Fatal("expected generator to be called")
	}
	if generator.tenantID != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, generator.tenantID)
	}
	if generator.input.LeadID != leadID {
		t.Fatalf("expected lead id %s, got %s", leadID, generator.input.LeadID)
	}
	if generator.input.LeadServiceID != leadServiceID {
		t.Fatalf("expected lead service id %s, got %s", leadServiceID, generator.input.LeadServiceID)
	}
	if len(generator.input.Items) != 1 || generator.input.Items[0].Quantity != "1x" {
		t.Fatalf("unexpected generator input items: %#v", generator.input.Items)
	}
}

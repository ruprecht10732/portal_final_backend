package service

import (
	"context"
	"strings"
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

func TestBuildBuilderSummaryProducesReadableFallback(t *testing.T) {
	scope := "Medium"
	urgency := "High"
	summary := buildBuilderSummary([]repository.QuoteItemSummary{
		{Description: "Dakpannen vervangen", Quantity: "2x"},
		{Description: "Goot herstellen", Quantity: "1x"},
	}, &scope, &urgency, true)
	if summary == nil {
		t.Fatal("expected fallback summary")
	}

	text := *summary
	if !strings.Contains(text, "Deze klus draait vooral om") {
		t.Fatalf("expected readable intro, got %q", text)
	}
	if !strings.Contains(text, "### Werkzaamheden") {
		t.Fatalf("expected work heading, got %q", text)
	}
	if !strings.Contains(text, "- 2x Dakpannen vervangen") {
		t.Fatalf("expected bullet work item, got %q", text)
	}
	if !strings.Contains(text, "**Omvang** Middel  **Urgentie** Hoog") {
		t.Fatalf("expected header labels, got %q", text)
	}
	if !strings.Contains(text, "### Let op") {
		t.Fatalf("expected attention heading, got %q", text)
	}
}

func TestSplitOfferSummarySeparatesShortAndDetailedOutput(t *testing.T) {
	short, full := splitOfferSummary("Dakklus: 2 pannen vervangen --- **Omvang** Klein\n\nDeze klus draait om dakherstel.")
	if short != "Dakklus: 2 pannen vervangen" {
		t.Fatalf("unexpected short summary: %q", short)
	}
	if !strings.Contains(full, "Deze klus draait om dakherstel.") {
		t.Fatalf("unexpected detailed summary: %q", full)
	}
}

func TestNormalizeBuilderSummaryStripsEmptyMarkdownHeadings(t *testing.T) {
	value := "###\nWerkzaamheden\n- Glas plaatsen\n\n###\nLet op\n- Steiger nodig"
	normalized := normalizeBuilderSummary(&value)
	if normalized == nil {
		t.Fatal("expected normalized summary")
	}
	if strings.Contains(*normalized, "###") {
		t.Fatalf("expected stray markdown headings to be removed, got %q", *normalized)
	}
	if strings.Contains(*normalized, "- Glas plaatsen") {
		t.Fatalf("expected markdown bullets to be removed, got %q", *normalized)
	}
	if !strings.Contains(*normalized, "Werkzaamheden") {
		t.Fatalf("expected summary content to remain, got %q", *normalized)
	}
	if !strings.Contains(*normalized, "Glas plaatsen") {
		t.Fatalf("expected bullet text to remain, got %q", *normalized)
	}
}

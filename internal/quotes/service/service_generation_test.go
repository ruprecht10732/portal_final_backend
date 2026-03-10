package service

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/quotes/transport"
)

func TestBuildDraftQuoteRepositoryModelInitializesFirstVersion(t *testing.T) {
	leadID := uuid.New()
	serviceID := uuid.New()
	orgID := uuid.New()
	createdByID := uuid.New()
	now := time.Date(2026, time.March, 10, 21, 19, 12, 0, time.UTC)
	validUntil := now.AddDate(0, 0, 14)

	quote := buildDraftQuoteRepositoryModel(
		DraftQuoteParams{
			LeadID:         leadID,
			LeadServiceID:  serviceID,
			OrganizationID: orgID,
			CreatedByID:    createdByID,
			Notes:          "Conceptofferte vanuit estimator",
		},
		"OFF-2026-0001",
		transport.QuoteCalculationResponse{
			SubtotalCents:       125000,
			DiscountAmountCents: 0,
			VatTotalCents:       26250,
			TotalCents:          151250,
		},
		now,
		&validUntil,
	)

	if quote.VersionNumber != 1 {
		t.Fatalf("expected version number 1, got %d", quote.VersionNumber)
	}
	if quote.LeadServiceID == nil || *quote.LeadServiceID != serviceID {
		t.Fatalf("expected lead service id %s, got %v", serviceID, quote.LeadServiceID)
	}
	if quote.CreatedByID == nil || *quote.CreatedByID != createdByID {
		t.Fatalf("expected created by id %s, got %v", createdByID, quote.CreatedByID)
	}
	if quote.ValidUntil == nil || !quote.ValidUntil.Equal(validUntil) {
		t.Fatalf("expected valid until %v, got %v", validUntil, quote.ValidUntil)
	}
	if quote.Notes == nil || *quote.Notes != "Conceptofferte vanuit estimator" {
		t.Fatalf("expected notes to be preserved, got %v", quote.Notes)
	}
	if quote.Status != string(transport.QuoteStatusDraft) {
		t.Fatalf("expected draft status, got %s", quote.Status)
	}
}

func TestBuildDraftQuoteRepositoryModelOmitsNilCreatedByForSystemDrafts(t *testing.T) {
	quote := buildDraftQuoteRepositoryModel(
		DraftQuoteParams{
			LeadID:         uuid.New(),
			LeadServiceID:  uuid.New(),
			OrganizationID: uuid.New(),
			CreatedByID:    uuid.Nil,
		},
		"OFF-2026-0002",
		transport.QuoteCalculationResponse{},
		time.Date(2026, time.March, 10, 21, 19, 17, 0, time.UTC),
		nil,
	)

	if quote.CreatedByID != nil {
		t.Fatalf("expected nil createdBy for system draft, got %v", quote.CreatedByID)
	}
	if quote.VersionNumber != 1 {
		t.Fatalf("expected version number 1, got %d", quote.VersionNumber)
	}
	if quote.ValidUntil != nil {
		t.Fatalf("expected nil valid until, got %v", quote.ValidUntil)
	}
	if quote.Notes != nil {
		t.Fatalf("expected nil notes for empty input, got %v", quote.Notes)
	}
}

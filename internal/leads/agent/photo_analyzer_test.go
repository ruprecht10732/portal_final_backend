package agent

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPhotoAnalyzerDepsSetResultClonesInput(t *testing.T) {
	t.Parallel()

	deps := &PhotoAnalyzerDeps{}
	result := &PhotoAnalysis{
		ID:                     uuid.New(),
		LeadID:                 uuid.New(),
		ServiceID:              uuid.New(),
		Summary:                "origineel",
		Observations:           []string{"obs-1"},
		SafetyConcerns:         []string{"veiligheid-1"},
		AdditionalInfo:         []string{"info-1"},
		Measurements:           []Measurement{{Description: "breedte", Value: 100, Unit: "cm"}},
		NeedsOnsiteMeasurement: []string{"maat ontbreekt"},
		Discrepancies:          []string{"verschil-1"},
		ExtractedText:          []string{"tekst-1"},
		SuggestedSearchTerms:   []string{"term-1"},
	}

	deps.SetResult(result)

	result.Observations[0] = "gewijzigd"
	result.SafetyConcerns[0] = "gewijzigd"
	result.AdditionalInfo[0] = "gewijzigd"
	result.Measurements[0].Description = "gewijzigd"
	result.NeedsOnsiteMeasurement[0] = "gewijzigd"
	result.Discrepancies[0] = "gewijzigd"
	result.ExtractedText[0] = "gewijzigd"
	result.SuggestedSearchTerms[0] = "gewijzigd"

	stored := deps.GetResult()
	if stored == nil {
		t.Fatal("expected stored result")
	}

	if got := stored.Observations[0]; got != "obs-1" {
		t.Fatalf("expected stored observations to stay isolated, got %q", got)
	}
	if got := stored.SafetyConcerns[0]; got != "veiligheid-1" {
		t.Fatalf("expected stored safety concerns to stay isolated, got %q", got)
	}
	if got := stored.AdditionalInfo[0]; got != "info-1" {
		t.Fatalf("expected stored additional info to stay isolated, got %q", got)
	}
	if got := stored.Measurements[0].Description; got != "breedte" {
		t.Fatalf("expected stored measurements to stay isolated, got %q", got)
	}
	if got := stored.NeedsOnsiteMeasurement[0]; got != "maat ontbreekt" {
		t.Fatalf("expected stored onsite flags to stay isolated, got %q", got)
	}
	if got := stored.Discrepancies[0]; got != "verschil-1" {
		t.Fatalf("expected stored discrepancies to stay isolated, got %q", got)
	}
	if got := stored.ExtractedText[0]; got != "tekst-1" {
		t.Fatalf("expected stored extracted text to stay isolated, got %q", got)
	}
	if got := stored.SuggestedSearchTerms[0]; got != "term-1" {
		t.Fatalf("expected stored search terms to stay isolated, got %q", got)
	}
}

func TestPhotoAnalyzerDepsGetResultReturnsClone(t *testing.T) {
	t.Parallel()

	deps := &PhotoAnalyzerDeps{}
	deps.SetResult(&PhotoAnalysis{
		ID:                     uuid.New(),
		LeadID:                 uuid.New(),
		ServiceID:              uuid.New(),
		Observations:           []string{"obs-1"},
		Measurements:           []Measurement{{Description: "hoogte", Value: 210, Unit: "cm"}},
		NeedsOnsiteMeasurement: []string{"diepte nodig"},
	})

	first := deps.GetResult()
	if first == nil {
		t.Fatal("expected result")
	}

	first.Observations[0] = "mutated"
	first.Measurements[0].Description = "mutated"
	first.NeedsOnsiteMeasurement[0] = "mutated"

	second := deps.GetResult()
	if second == nil {
		t.Fatal("expected second result")
	}

	if got := second.Observations[0]; got != "obs-1" {
		t.Fatalf("expected observations clone, got %q", got)
	}
	if got := second.Measurements[0].Description; got != "hoogte" {
		t.Fatalf("expected measurements clone, got %q", got)
	}
	if got := second.NeedsOnsiteMeasurement[0]; got != "diepte nodig" {
		t.Fatalf("expected onsite measurement clone, got %q", got)
	}
}

func TestPhotoAnalyzerDepsGetOnsiteFlagsReturnsClone(t *testing.T) {
	t.Parallel()

	deps := &PhotoAnalyzerDeps{}
	deps.AddOnsiteFlag("eerste reden")

	flags := deps.GetOnsiteFlags()
	flags[0] = "gewijzigd"

	stored := deps.GetOnsiteFlags()
	if len(stored) != 1 {
		t.Fatalf("expected one stored flag, got %d", len(stored))
	}
	if got := stored[0]; got != "eerste reden" {
		t.Fatalf("expected onsite flags clone, got %q", got)
	}
}

func TestMergeOnsiteFlagsUsesClonedFlags(t *testing.T) {
	t.Parallel()

	deps := &PhotoAnalyzerDeps{}
	deps.AddOnsiteFlag("exacte dagmaat nodig")
	ctx := WithPhotoAnalyzerDeps(context.Background(), deps)

	result := &PhotoAnalysis{
		NeedsOnsiteMeasurement: []string{"bestaande reden"},
	}

	pa := &PhotoAnalyzer{}
	pa.mergeOnsiteFlags(ctx, result)

	result.NeedsOnsiteMeasurement[1] = "gewijzigd"

	stored := deps.GetOnsiteFlags()
	if len(stored) != 1 {
		t.Fatalf("expected one stored flag, got %d", len(stored))
	}
	if got := stored[0]; got != "exacte dagmaat nodig" {
		t.Fatalf("expected deps flags to remain unchanged, got %q", got)
	}
}

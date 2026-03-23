package engine

import (
	"strings"
	"testing"
)

const testCurrencyEuro1500 = "€1.500"

func TestDetectGroundingIssueAllowsQuoteRepliesWithDatesWithoutAppointmentContext(t *testing.T) {
	t.Parallel()

	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-1234","created_at":"2026-03-12T09:00:00Z"},{"quote_number":"OFF-1235","created_at":"2026-03-04T08:30:00Z"}],"count":2}`,
		}},
	}

	reply := "Ik zie 2 open offertes:\n- Offerte: OFF-1234 van 12 maart 2026\n- Offerte: OFF-1235 van 4 maart 2026"
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected quote overview to pass grounding, got %#v", decision)
	}
}

func TestDetectGroundingIssueStillRequiresAppointmentToolsForAppointmentDates(t *testing.T) {
	t.Parallel()

	evidence := &replyGroundingEvidence{toolResponseNames: map[string]int{"GetQuotes": 1}}
	reply := "De afspraak staat op 12 maart 2026."
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "appointment_details_without_appointment_tool" {
		t.Fatalf("expected appointment grounding failure, got %#v", decision)
	}
	if len(decision.UnsupportedFacts) != 1 || decision.UnsupportedFacts[0] != "12 maart 2026" {
		t.Fatalf("expected unsupported appointment date, got %#v", decision.UnsupportedFacts)
	}
}

func TestDetectGroundingIssueAllowsQuoteListWithLongNumbersContainingZero(t *testing.T) {
	t.Parallel()

	// Reproduces the production bug: quote numbers like OFF-2026-0047 contain
	// "026-0047 -" which the phone regex used to match as a phone number.
	// The grounding checker then flagged "lead_details_without_lead_tool"
	// even though the data came from GetQuotes.
	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0047","consumer_name":"Klant A","status":"concept"},{"quote_number":"OFF-2026-0045","consumer_name":"Klant B","status":"sent"},{"quote_number":"OFF-2026-0044","consumer_name":"Klant C","status":"concept"},{"quote_number":"OFF-2026-0042","consumer_name":"Klant D","status":"accepted"},{"quote_number":"OFF-2026-0019","consumer_name":"Klant E","status":"sent"}],"count":5}`,
		}},
	}

	reply := "Hoi! Ik zie de volgende offertes:\n- OFF-2026-0047 - Klant A\n- OFF-2026-0045 - Klant B\n- OFF-2026-0044 - Klant C\n- OFF-2026-0042 - Klant D\n- OFF-2026-0019 - Klant E"
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected quote listing to pass grounding, got code=%q unsupported=%v", decision.Code, decision.UnsupportedFacts)
	}
}

func TestExtractLeadFactsIgnoresPhoneMatchesEmbeddedInQuoteNumbers(t *testing.T) {
	t.Parallel()

	reply := "OFF-2026-0047 - Klant A\nOFF-2026-0019 - Klant B"
	facts := extractLeadFacts(reply)
	for _, f := range facts {
		t.Errorf("unexpected lead fact extracted from quote listing: %q", f)
	}
}

func TestExtractLeadFactsStillMatchesRealPhoneNumbers(t *testing.T) {
	t.Parallel()

	reply := "Bel mij op 0612345678 of +31687654321."
	facts := extractLeadFacts(reply)
	if len(facts) != 2 {
		t.Fatalf("expected 2 phone facts, got %d: %v", len(facts), facts)
	}
}

func TestLeadFactVariantsIncludesStatusTranslations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fact     string
		contains string
	}{
		{"Concept", "Draft"},
		{"Draft", "Concept"},
		{"Verstuurd", "Sent"},
		{"Sent", "Verstuurd"},
		{"Geaccepteerd", "Accepted"},
		{"Accepted", "Geaccepteerd"},
		{"Afgewezen", "Rejected"},
		{"Rejected", "Afgewezen"},
		{"Verlopen", "Expired"},
		{"Expired", "Verlopen"},
	}
	for _, tt := range tests {
		var found bool
		for _, v := range leadFactVariants(tt.fact) {
			if v == tt.contains {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("leadFactVariants(%q) should contain %q, got %v", tt.fact, tt.contains, leadFactVariants(tt.fact))
		}
	}
}

func TestDetectGroundingIssuePassesDutchStatusFromEnglishPayload(t *testing.T) {
	t.Parallel()

	// GetQuotes returns English status "Draft" but the LLM replies in Dutch "Concept".
	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0001","status":"Draft","consumer_name":"Jan Jansen","total_cents":150000}],"count":1}`,
		}},
	}

	reply := "Ik zie 1 offerte:\n- OFF-2026-0001 – Jan Jansen – Status: Concept – €1.500,00"
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected Dutch status translation to pass grounding, got code=%q unsupported=%v", decision.Code, decision.UnsupportedFacts)
	}
}
func TestDetectGroundingIssueStripsStatusDetailsAndPunctuation(t *testing.T) {
        t.Parallel()

        evidence := &replyGroundingEvidence{
                toolResponseNames: map[string]int{"GetQuotes": 1},
                toolResponses: []toolResponseObservation{{
                        Name:    "GetQuotes",
                        Payload: `{"quotes":[{"quote_number":"OFF-2026-0001","status":"Sent","consumer_name":"Jan Jansen"}],"count":1}`,
                }},
        }

        reply := "Offerte 1, Status: verstuurd, nog niet geaccepteerd):"
        decision := detectGroundingIssue(reply, evidence)
        if decision.Code != "" {
                t.Fatalf("expected status to pass grounding, got code=%q unsupported=%v", decision.Code, decision.UnsupportedFacts)
        }
        
        reply2 := "Offerte 1, Status: verstuurd (nog niet geaccepteerd)"
        decision2 := detectGroundingIssue(reply2, evidence)
        if decision2.Code != "" {
                t.Fatalf("expected status 2 to pass grounding, got code=%q unsupported=%v", decision2.Code, decision2.UnsupportedFacts)
        }
}

func TestDetectGroundingIssueAllowsQuoteWithGeplandKeyword(t *testing.T) {
	t.Parallel()

	// "gepland" used to be an appointment keyword, triggering false-positive
	// appointment grounding on quote replies containing the word.
	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0010","status":"Sent","consumer_name":"Lisa Smit","created_at":"2026-03-15T10:00:00Z"}],"count":1}`,
		}},
	}

	reply := "De offerte OFF-2026-0010 is gepland verstuurd op 15 maart 2026 aan Lisa Smit."
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected quote reply with 'gepland' to pass grounding, got code=%q unsupported=%v", decision.Code, decision.UnsupportedFacts)
	}
}

func TestDetectGroundingIssueAllowsQuoteDatesExplainedByQuotePayload(t *testing.T) {
	t.Parallel()

	// When the reply mentions an appointment keyword AND dates from quote
	// metadata, the dates should be filtered out if they appear in quote
	// payloads — preventing false appointment grounding.
	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0020","status":"Draft","created_at":"2026-04-01T08:00:00Z","consumer_name":"Piet de Vries"}],"count":1}`,
		}},
	}

	// "afspraak" triggers appointment context; "2026-04-01" is a quote date
	reply := "Ik zie 1 offerte (OFF-2026-0020) van 2026-04-01. Wilt u ook een afspraak inplannen?"
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected quote date explained by payload to pass, got code=%q unsupported=%v", decision.Code, decision.UnsupportedFacts)
	}
}

func TestDetectGroundingIssueStillCatchesUnexplainedAppointmentDates(t *testing.T) {
	t.Parallel()

	// A date NOT in the quote payload should still trigger appointment grounding
	// when appointment keywords are present.
	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0020","status":"Draft","created_at":"2026-04-01T08:00:00Z"}],"count":1}`,
		}},
	}

	reply := "De afspraak staat op 15 april 2026 om 10:00."
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "appointment_details_without_appointment_tool" {
		t.Fatalf("expected appointment grounding failure for unexplained date, got %#v", decision)
	}
}

func TestStatusTranslationsVerzonden(t *testing.T) {
	t.Parallel()

	// "Verzonden" is a common Dutch synonym for "Verstuurd" (both mean "Sent").
	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0001","status":"Sent","consumer_name":"Jan Jansen"}],"count":1}`,
		}},
	}

	reply := "Offerte OFF-2026-0001, Status: verzonden"
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected 'verzonden' to match 'Sent' via translations, got code=%q unsupported=%v", decision.Code, decision.UnsupportedFacts)
	}
}

func TestStatusTranslationsGoedgekeurd(t *testing.T) {
	t.Parallel()

	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0002","status":"Accepted","consumer_name":"Lisa Smit"}],"count":1}`,
		}},
	}

	reply := "Offerte OFF-2026-0002, Status: goedgekeurd"
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected 'goedgekeurd' to match 'Accepted', got code=%q unsupported=%v", decision.Code, decision.UnsupportedFacts)
	}
}

func TestCurrencyVariantsWithoutCentsSuffix(t *testing.T) {
	t.Parallel()

	// When the LLM writes €1.500 without the ,00 cents suffix, the digit-only
	// form "1500" should still match "total_cents": 150000 via the "150000"
	// cent variant (digits + "00").
	evidence := &replyGroundingEvidence{
		toolResponseNames: map[string]int{"GetQuotes": 1},
		toolResponses: []toolResponseObservation{{
			Name:    "GetQuotes",
			Payload: `{"quotes":[{"quote_number":"OFF-2026-0003","total_cents":150000,"consumer_name":"Piet"}],"count":1}`,
		}},
	}

	reply := "Offerte: OFF-2026-0003 – Piet – " + testCurrencyEuro1500
	decision := detectGroundingIssue(reply, evidence)
	if decision.Code != "" {
		t.Fatalf("expected %s without cents to pass grounding, got code=%q unsupported=%v", testCurrencyEuro1500, decision.Code, decision.UnsupportedFacts)
	}
}

func TestCurrencyVariantsGeneratesCentForm(t *testing.T) {
	t.Parallel()

	variants := currencyFactVariants(testCurrencyEuro1500)
	wantCents := "150000"
	found := false
	for _, v := range variants {
		if v == wantCents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cent variant %q in currencyFactVariants(%q), got %v", wantCents, testCurrencyEuro1500, variants)
	}
}

func TestHasAppointmentContextRejectsGenericKeywords(t *testing.T) {
	t.Parallel()

	// These keywords were removed because they triggered false positives.
	genericPhrases := []string{
		"De offerte is gepland voor volgende week.",
		"Dit is de planning voor het project.",
		"Op deze locatie wordt gewerkt.",
		"De route naar de klant.",
		"Ik heb even de tijd nodig.",
		"Er is een slot beschikbaar.",
	}
	for _, reply := range genericPhrases {
		if hasAppointmentContext(reply) {
			t.Errorf("expected hasAppointmentContext to return false for %q", reply)
		}
	}
}

func TestHasAppointmentContextAcceptsRealKeywords(t *testing.T) {
	t.Parallel()

	realPhrases := []string{
		"De afspraak is op dinsdag.",
		"Ik heb het bezoek ingepland.",
		"De monteur komt morgen langs.",
		"Kunt u de inspectie inplannen?",
	}
	for _, reply := range realPhrases {
		if !hasAppointmentContext(reply) {
			t.Errorf("expected hasAppointmentContext to return true for %q", reply)
		}
	}
}

func TestShouldSkipReplayedMessageFiltersGroundingFallbacks(t *testing.T) {
	t.Parallel()

	fallbacks := []string{
		"Noem de klantnaam of het offertenummer, dan pak ik de juiste offerte erbij.",
		"Ik kan dat offertedetail zo niet bevestigen. Noem de klantnaam of het offertenummer, dan controleer ik het meteen.",
		"Noem de datum, periode of klant, dan pak ik de juiste afspraak erbij.",
		"Noem de klantnaam of het dossier waar het over gaat, dan controleer ik de gegevens.",
		"Noem even welk onderdeel ik moet controleren, dan pak ik het gericht erbij.",
	}
	for _, msg := range fallbacks {
		if !shouldSkipReplayedMessage("assistant", msg) {
			t.Errorf("expected shouldSkipReplayedMessage to skip %q", msg)
		}
	}
}

func TestBuildGroundingRepairDirective(t *testing.T) {
	t.Parallel()

	directive := buildGroundingRepairDirective("quote_details_without_quote_tool", []string{testCurrencyEuro1500})
	if directive == "" {
		t.Fatal("expected non-empty repair directive")
	}
	if !strings.Contains(directive, "GetQuotes") {
		t.Error("expected directive to mention GetQuotes")
	}
	if !strings.Contains(directive, testCurrencyEuro1500) {
		t.Error("expected directive to contain unsupported fact")
	}
}

func TestGroundingFallbackReplyPrefersDomainSpecific(t *testing.T) {
	t.Parallel()

	result := AgentRunResult{
		GroundingFailure:       "quote_details_without_quote_tool",
		GroundingFallbackReply: "Noem de klantnaam of het offertenummer, dan pak ik de juiste offerte erbij.",
	}
	got := groundingFallbackReply(result)
	if got != result.GroundingFallbackReply {
		t.Fatalf("expected domain-specific fallback, got %q", got)
	}
}

func TestGroundingFallbackReplyFallsBackToGeneric(t *testing.T) {
	t.Parallel()

	result := AgentRunResult{GroundingFailure: "unknown", GroundingFallbackReply: ""}
	got := groundingFallbackReply(result)
	if got != msgGroundingFallback {
		t.Fatalf("expected generic fallback, got %q", got)
	}
}

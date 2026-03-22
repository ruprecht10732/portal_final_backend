package engine

import "testing"

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

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

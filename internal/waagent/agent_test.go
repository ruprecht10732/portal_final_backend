package waagent

import (
	"strings"
	"testing"
)

func TestBuildLeadContextTextUsesRoutingHintOnly(t *testing.T) {
	t.Parallel()

	agent := &Agent{}
	hint := &ConversationLeadHint{
		LeadID:        testHintLeadID,
		LeadServiceID: "svc-123",
		CustomerName:  "Robin Example",
		PreloadedDetails: &LeadDetailsResult{
			CustomerName: "Robin Example",
			FullAddress:  "Kerkstraat 12, Alkmaar",
			Phone:        "+31610000000",
			Email:        "robin@example.com",
			Status:       "Intake",
		},
	}

	text := agent.buildLeadContextText(hint)

	for _, want := range []string{
		"Laatst besproken klant: Robin Example.",
		"dienstcontext",
		"Verifieer",
		"GetLeadDetails",
		"GetQuotes",
		"GetAppointments",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected hint text to contain %q, got %q", want, text)
		}
	}

	for _, forbidden := range []string{
		"Kerkstraat 12",
		"+31610000000",
		"robin@example.com",
		"Intake",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("expected hint text not to contain %q, got %q", forbidden, text)
		}
	}
}
package waagent

import (
	"strings"
	"testing"
	"time"
)

func TestBuildLeadContextTextUsesRoutingHintOnly(t *testing.T) {
	t.Parallel()

	agent := &Agent{}
	hint := &ConversationLeadHint{
		LeadID:        testHintLeadID,
		LeadServiceID: "svc-123",
		CustomerName:  "Robin Example",
		RecentQuotes: []RecentQuoteHint{{
			QuoteNumber: "OFF-2026-0021",
			ClientName:  "Joey Plomp",
			Summary:     "Kogellagerscharnier RVS",
		}},
		RecentAppointments: []RecentAppointmentHint{{
			AppointmentID: "appt-1",
			Title:         "Bezoek",
			StartTime:     "2026-03-16T09:00:00Z",
			Location:      "Alkmaar",
		}},
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
		"Laatst getoonde offertes in dit gesprek:",
		"Joey Plomp (OFF-2026-0021): Kogellagerscharnier RVS",
		"Laatst getoonde afspraken in dit gesprek:",
		"Bezoek op 2026-03-16T09:00:00Z in Alkmaar",
		"dienstcontext",
		"verifieer",
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

func TestBuildLeadContextTextUsesRecentListsWithoutLeadID(t *testing.T) {
	t.Parallel()

	agent := &Agent{}
	hint := &ConversationLeadHint{
		RecentQuotes: []RecentQuoteHint{{
			QuoteNumber: "OFF-2026-0021",
			ClientName:  "Joey Plomp",
		}},
	}

	text := agent.buildLeadContextText(hint)
	if !strings.Contains(text, "Laatst getoonde offertes in dit gesprek:") {
		t.Fatalf("expected recent quote context in hint text, got %q", text)
	}
	if strings.Contains(text, "Laatst besproken klant:") {
		t.Fatalf("expected no resolved customer label in hint text, got %q", text)
	}
}

func TestFormatConversationHistoryContentIncludesTimestampWhenPresent(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2026, time.March, 15, 8, 14, 0, 0, time.UTC)
	formatted := formatConversationHistoryContent(ConversationMessage{Role: "user", Content: "Die van Carola Dekker", SentAt: &sentAt})

	if !strings.Contains(formatted, "[Berichttijd: 2026-03-15T08:14:00Z]") {
		t.Fatalf("expected timestamp marker in formatted history, got %q", formatted)
	}
	if !strings.Contains(formatted, "Die van Carola Dekker") {
		t.Fatalf("expected original content in formatted history, got %q", formatted)
	}
}

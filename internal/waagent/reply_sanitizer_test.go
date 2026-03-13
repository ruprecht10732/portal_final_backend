package waagent

import (
	"strings"
	"testing"
)

func TestSanitizeWhatsAppReplyConvertsMarkdownTable(t *testing.T) {
	input := strings.TrimSpace(`Ik ga voor u opzoeken welke offertes er open staan.

## Openstaande offertes

| Offertenummer | Klant | Bedrag |
|:---|:---|---:|
| OFF-2026-0019 | Carola Dekker | € 5.969,30 |
| OFF-2026-0020 | Mark de Vries | € 1.379,40 |`)

	got := sanitizeWhatsAppReply(input)

	if strings.Contains(got, "##") {
		t.Fatalf("expected heading markers to be removed, got %q", got)
	}
	if strings.Contains(got, "|") {
		t.Fatalf("expected markdown table to be converted, got %q", got)
	}
	if !strings.Contains(got, "- Offertenummer: OFF-2026-0019, Klant: Carola Dekker, Bedrag: € 5.969,30") {
		t.Fatalf("expected first row to be converted to bullet list, got %q", got)
	}
	if strings.Contains(strings.ToLower(got), "ik ga voor u opzoeken") {
		t.Fatalf("expected meta lookup preamble to be removed, got %q", got)
	}
}

func TestSanitizeWhatsAppReplyRemovesInternalIDs(t *testing.T) {
	input := "Ik heb Carola gevonden (lead ID: lead_01JQA8B5QG7F5H2D9K3M7N4P8R). Lead UUID: 550e8400-e29b-41d4-a716-446655440000"

	got := sanitizeWhatsAppReply(input)

	if strings.Contains(strings.ToLower(got), "lead id") {
		t.Fatalf("expected lead id clause to be removed, got %q", got)
	}
	if strings.Contains(got, "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("expected UUID to be removed, got %q", got)
	}
}

func TestSanitizeWhatsAppReplyConvertsDoubleStarToSingle(t *testing.T) {
	input := "**Navigatielink voor Roy Band:**\n\nhttps://maps.google.com/test\n\n**Adres:**\n- Marga Klompeland 13"
	got := sanitizeWhatsAppReply(input)

	if strings.Contains(got, "**") {
		t.Fatalf("expected double-star markdown to be converted to single-star, got %q", got)
	}
	if !strings.Contains(got, "*Navigatielink voor Roy Band:*") {
		t.Fatalf("expected single-star bold, got %q", got)
	}
}

func TestSanitizeWhatsAppReplyRemovesEmptyBullets(t *testing.T) {
	input := "- Naam: Roy Band\n-\n-\n- Telefoon: +31652855717"
	got := sanitizeWhatsAppReply(input)

	lines := strings.Split(got, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "-" || trimmed == "*" {
			t.Fatalf("expected empty bullet points to be removed, got %q", got)
		}
	}
	if !strings.Contains(got, "Naam: Roy Band") {
		t.Fatalf("expected non-empty bullets to remain, got %q", got)
	}
}
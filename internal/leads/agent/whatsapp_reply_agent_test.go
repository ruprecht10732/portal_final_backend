package agent

import (
	"strings"
	"testing"
	"time"

	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

func TestBuildWhatsAppReplyPromptWithoutLeadOrServiceContext(t *testing.T) {
	prompt := buildWhatsAppReplyPrompt(ports.WhatsAppReplyInput{
		PhoneNumber: "+31612345678",
		DisplayName: "Robin",
		Messages: []ports.WhatsAppReplyMessage{{
			Direction: "inbound",
			Body:      "Kunnen jullie volgende week langskomen?",
		}},
	}, whatsAppReplyContext{}, "Behulpzaam en direct")

	for _, expected := range []string{
		"Lead ID: Niet opgegeven",
		"Service ID: Niet opgegeven",
		"WhatsApp display name: Robin",
		"Geen gekoppelde leadcontext beschikbaar",
		"Geen actieve dienstcontext beschikbaar",
		"Kunnen jullie volgende week langskomen?",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got %s", expected, prompt)
		}
	}
}

func TestWhatsAppReplySystemPromptUsesConfiguredTone(t *testing.T) {
	tone := "calm, precise, and reassuring"
	prompt := whatsappReplySystemPrompt(tone)

	if !strings.Contains(prompt, tone) {
		t.Fatalf("expected prompt to include configured tone %q, got %q", tone, prompt)
	}
	if !strings.Contains(prompt, "Tenant Tone Addendum") {
		t.Fatalf("expected whatsapp tone addendum marker in prompt, got %q", prompt)
	}
}

func TestWhatsAppReplySystemPromptFallsBackToDefaultTone(t *testing.T) {
	defaultTone := ports.DefaultOrganizationAISettings().WhatsAppToneOfVoice
	prompt := whatsappReplySystemPrompt("   ")

	if !strings.Contains(prompt, defaultTone) {
		t.Fatalf("expected prompt to include default tone %q, got %q", defaultTone, prompt)
	}
}

func TestBuildWhatsAppReplyPromptIncludesSelectedScenario(t *testing.T) {
	leadID := uuid.New()
	prompt := buildWhatsAppReplyPrompt(ports.WhatsAppReplyInput{
		LeadID:         &leadID,
		ConversationID: uuid.New(),
		Scenario:       ports.ReplySuggestionScenarioAppointmentReminder,
		ScenarioNotes:  "Noem de geplande tijd nogmaals.",
		Messages: []ports.WhatsAppReplyMessage{{
			Direction: "inbound",
			Body:      "Top, tot morgen",
			CreatedAt: time.Now().UTC(),
		}},
	}, whatsAppReplyContext{}, "Behulpzaam en direct")

	checks := []string{"Selected reply scenario", "afspraakherinnering", "Noem de geplande tijd nogmaals."}
	for _, expected := range checks {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got %q", expected, prompt)
		}
	}
}

func TestFormatWhatsAppTranscriptLimitsRecentMessages(t *testing.T) {
	base := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	messages := make([]ports.WhatsAppReplyMessage, 0, 8)
	for i := 0; i < 8; i++ {
		messages = append(messages, ports.WhatsAppReplyMessage{
			Direction: "inbound",
			Body:      "bericht " + string(rune('0'+i)),
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}

	formatted := formatWhatsAppTranscript(messages)

	if strings.Contains(formatted, "bericht 0") || strings.Contains(formatted, "bericht 1") {
		t.Fatalf("expected transcript to exclude older messages, got %q", formatted)
	}
	if !strings.Contains(formatted, "bericht 7") {
		t.Fatalf("expected transcript to include newest message, got %q", formatted)
	}
	if count := strings.Count(formatted, "Klant:"); count != maxWhatsAppTranscriptItems {
		t.Fatalf("expected %d transcript lines, got %d in %q", maxWhatsAppTranscriptItems, count, formatted)
	}
}

func TestFormatWhatsAppExamplesLimitsRecentExamples(t *testing.T) {
	base := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	examples := make([]ports.WhatsAppReplyExample, 0, 6)
	for i := 0; i < 6; i++ {
		examples = append(examples, ports.WhatsAppReplyExample{
			CustomerMessage: "vraag " + string(rune('0'+i)),
			Reply:           "antwoord " + string(rune('0'+i)),
			CreatedAt:       base.Add(time.Duration(i) * time.Minute),
		})
	}

	formatted := formatWhatsAppExamples(examples)

	if strings.Contains(formatted, "vraag 4") || strings.Contains(formatted, "antwoord 4") {
		t.Fatalf("expected example output to only include the first %d examples, got %q", maxWhatsAppExampleItems, formatted)
	}
	if !strings.Contains(formatted, "vraag 0") || !strings.Contains(formatted, "antwoord 3") {
		t.Fatalf("expected example output to include the earliest retained examples, got %q", formatted)
	}
	if count := strings.Count(formatted, "Klant:"); count != maxWhatsAppExampleItems {
		t.Fatalf("expected %d example customer lines, got %d in %q", maxWhatsAppExampleItems, count, formatted)
	}
}

func TestFormatWhatsAppFeedbackMemoryLimitsRecentCorrections(t *testing.T) {
	feedbackItems := make([]ports.WhatsAppReplyFeedback, 0, 6)
	for i := 0; i < 6; i++ {
		feedbackItems = append(feedbackItems, ports.WhatsAppReplyFeedback{
			AIReply:    "ai voorstel " + string(rune('0'+i)),
			HumanReply: "mens correctie " + string(rune('0'+i)),
		})
	}

	formatted := formatWhatsAppFeedbackMemory(feedbackItems)

	if strings.Contains(formatted, "ai voorstel 4") || strings.Contains(formatted, "mens correctie 4") {
		t.Fatalf("expected feedback output to only include the first %d corrections, got %q", maxWhatsAppExampleItems, formatted)
	}
	if !strings.Contains(formatted, "ai voorstel 0") || !strings.Contains(formatted, "mens correctie 3") {
		t.Fatalf("expected feedback output to include retained corrections, got %q", formatted)
	}
	if count := strings.Count(formatted, "AI-draft:"); count != maxWhatsAppExampleItems {
		t.Fatalf("expected %d feedback items, got %d in %q", maxWhatsAppExampleItems, count, formatted)
	}
}

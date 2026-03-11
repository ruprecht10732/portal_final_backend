package agent

import (
	"strings"
	"testing"

	"portal_final_backend/internal/leads/ports"
)

func TestEmailReplySystemPromptUsesConfiguredTone(t *testing.T) {
	tone := "calm, practical, and reassuring"
	prompt := emailReplySystemPrompt(tone)

	if !strings.Contains(prompt, tone) {
		t.Fatalf("expected prompt to include configured tone %q, got %q", tone, prompt)
	}
	if !strings.Contains(prompt, "Do not include a subject line") {
		t.Fatalf("expected email-specific instruction in prompt, got %q", prompt)
	}
}

func TestEmailReplySystemPromptFallsBackToDefaultTone(t *testing.T) {
	defaultTone := ports.DefaultOrganizationAISettings().WhatsAppToneOfVoice
	prompt := emailReplySystemPrompt("   ")

	if !strings.Contains(prompt, defaultTone) {
		t.Fatalf("expected prompt to include default tone %q, got %q", defaultTone, prompt)
	}
}

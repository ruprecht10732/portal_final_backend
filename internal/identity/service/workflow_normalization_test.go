package service

import (
	"testing"

	"portal_final_backend/internal/identity/repository"
)

func TestNormalizeWorkflowUpsertsDoesNotInjectTemplateBodyForSendMessage(t *testing.T) {
	workflows := []repository.WorkflowUpsert{
		{
			WorkflowKey: "default",
			Name:        "Default",
			Enabled:     true,
			Steps: []repository.WorkflowStepUpsert{
				{
					Trigger:      "lead_welcome",
					Channel:      "whatsapp",
					Audience:     "lead",
					Action:       "send_message",
					StepOrder:    1,
					DelayMinutes: 0,
					Enabled:      true,
				},
			},
		},
	}

	normalized := normalizeWorkflowUpserts(workflows)
	if len(normalized) != 1 || len(normalized[0].Steps) != 1 {
		t.Fatalf("expected normalized workflow/step to exist")
	}
	if normalized[0].Steps[0].TemplateBody != nil {
		t.Fatal("expected template body to remain unset when not provided")
	}
}

func TestNormalizeWorkflowUpsertsKeepsExistingTemplateBody(t *testing.T) {
	existing := "Test bericht {{links.track}}"
	workflows := []repository.WorkflowUpsert{
		{
			WorkflowKey: "default",
			Name:        "Default",
			Enabled:     true,
			Steps: []repository.WorkflowStepUpsert{
				{
					Trigger:      "lead_welcome",
					Channel:      "whatsapp",
					Audience:     "lead",
					Action:       "send_message",
					StepOrder:    1,
					DelayMinutes: 0,
					Enabled:      true,
					TemplateBody: &existing,
				},
			},
		},
	}

	normalized := normalizeWorkflowUpserts(workflows)
	got := normalized[0].Steps[0].TemplateBody
	if got == nil || *got != existing {
		t.Fatalf("expected existing template body to remain unchanged, got %v", got)
	}
}

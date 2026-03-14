package waagent

import "testing"

func TestValidateWriteExecutionPlanRejectsMissingInformation(t *testing.T) {
	t.Parallel()

	reply := validateWriteExecutionPlan(writeExecutionPlan{
		ToolName:           "GenerateQuote",
		SafeToExecute:      false,
		MissingInformation: []string{"het exacte adres"},
	})
	if reply == "" {
		t.Fatal("expected missing-information fallback reply")
	}
}

func TestValidateWriteExecutionPlanRejectsUnknownTool(t *testing.T) {
	t.Parallel()

	reply := validateWriteExecutionPlan(writeExecutionPlan{ToolName: "DeleteLead", SafeToExecute: true})
	if reply == "" {
		t.Fatal("expected invalid-tool fallback reply")
	}
}

func TestPreferLookupRetryResultPrefersGroundedRetry(t *testing.T) {
	t.Parallel()

	current := AgentRunResult{Reply: "eerste", GroundingFailure: "lead_details_without_lead_tool"}
	retry := AgentRunResult{Reply: "tweede"}
	chosen := preferLookupRetryResult(current, retry)
	if chosen.Reply != "tweede" {
		t.Fatalf("expected retry result, got %#v", chosen)
	}
}

func TestShouldRetryLookupRunWhenReplyAsksUnnecessaryConfirmation(t *testing.T) {
	t.Parallel()

	messages := []ConversationMessage{{Role: "user", Content: testLeadLookupQuestion}, {Role: "assistant", Content: "Ik kan dat opzoeken."}, {Role: "user", Content: testLookupCustomerName}}
	result := AgentRunResult{Reply: "Ik heb Carola Dekker gevonden. Wil je dat ik de volledige contactgegevens en adresdetails voor je ophaal?"}
	if !shouldRetryLookupRun(messages, nil, result) {
		t.Fatal("expected lookup retry for unnecessary confirmation reply")
	}
}

func TestBuildLookupRepairDirectivePrefersDirectAnswerAfterConfirmation(t *testing.T) {
	t.Parallel()

	messages := []ConversationMessage{{Role: "user", Content: testLeadLookupQuestion}, {Role: "user", Content: testLookupCustomerName}}
	directive := buildLookupRepairDirective(messages, nil, AgentRunResult{Reply: "Wil je dat ik de gegevens ophaal?"})
	if directive == "" || directive == "Vorige poging leverde geen bruikbaar antwoord op. Antwoord opnieuw, kort en direct, zonder procesnarratie." {
		t.Fatalf("expected targeted repair directive, got %q", directive)
	}
}

func TestFallbackReplyForEditorIssueUsesLookupFallback(t *testing.T) {
	t.Parallel()

	reply := fallbackReplyForEditorIssue(agentRunModeLookup, "too_indirect", "origineel")
	if reply != lookupModeFallback {
		t.Fatalf("unexpected lookup editor fallback %q", reply)
	}
}

func TestExtractJSONObjectStripsWrapperText(t *testing.T) {
	t.Parallel()

	raw := "Hier is de beslissing:\n{\"approved\":true,\"issue\":\"\"}"
	got := extractJSONObject(raw)
	if got != "{\"approved\":true,\"issue\":\"\"}" {
		t.Fatalf("unexpected json extraction %q", got)
	}
}

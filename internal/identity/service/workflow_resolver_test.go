package service

import (
	"testing"

	"portal_final_backend/internal/identity/repository"

	"github.com/google/uuid"
)

func TestMatchesOptionalField(t *testing.T) {
	rule := "quote_flow"
	actual := "Quote_Flow"
	other := "webform"
	empty := ""

	if !matchesOptionalField(nil, &actual) {
		t.Fatal("nil rule should match any actual value")
	}
	if !matchesOptionalField(&empty, &actual) {
		t.Fatal("empty rule should match any actual value")
	}
	if !matchesOptionalField(&rule, &actual) {
		t.Fatal("matching values should be accepted case-insensitively")
	}
	if matchesOptionalField(&rule, &other) {
		t.Fatal("different values should not match")
	}
	if matchesOptionalField(&rule, nil) {
		t.Fatal("missing actual value should not match non-empty rule")
	}
}

func TestMatchesAssignmentRule(t *testing.T) {
	source := "quote_flow"
	serviceType := "insulation"
	stage := "new"

	rule := repository.WorkflowAssignmentRule{
		LeadSource:      &source,
		LeadServiceType: &serviceType,
		PipelineStage:   &stage,
	}

	inputMatch := ResolveLeadWorkflowInput{
		LeadSource:      strPtr("QUOTE_FLOW"),
		LeadServiceType: strPtr("Insulation"),
		PipelineStage:   strPtr("NEW"),
	}
	if !matchesAssignmentRule(rule, inputMatch) {
		t.Fatal("expected assignment rule to match case-insensitive input")
	}

	inputNoMatch := ResolveLeadWorkflowInput{
		LeadSource:      strPtr("webform"),
		LeadServiceType: strPtr("Insulation"),
		PipelineStage:   strPtr("NEW"),
	}
	if matchesAssignmentRule(rule, inputNoMatch) {
		t.Fatal("expected assignment rule mismatch when source differs")
	}
}

func TestResolveWorkflowFromOverride(t *testing.T) {
	workflowID := uuid.New()
	workflow := repository.Workflow{ID: workflowID, WorkflowKey: "wf.quote", Enabled: true}
	workflowMap := map[uuid.UUID]*repository.Workflow{workflowID: &workflow}

	overrideClear := repository.LeadWorkflowOverride{OverrideMode: "clear"}
	result, ok := resolveWorkflowFromOverride(overrideClear, workflowMap)
	if !ok {
		t.Fatal("clear override should be matched")
	}
	if result.ResolutionSource != "manual_clear" {
		t.Fatalf("expected manual_clear resolution, got %s", result.ResolutionSource)
	}

	overrideManual := repository.LeadWorkflowOverride{OverrideMode: "manual", WorkflowID: &workflowID}
	result, ok = resolveWorkflowFromOverride(overrideManual, workflowMap)
	if !ok {
		t.Fatal("manual override should match when workflow exists")
	}
	if result.Workflow == nil || result.Workflow.ID != workflowID {
		t.Fatal("expected matched workflow from manual override")
	}

	missingID := uuid.New()
	overrideMissing := repository.LeadWorkflowOverride{OverrideMode: "manual", WorkflowID: &missingID}
	_, ok = resolveWorkflowFromOverride(overrideMissing, workflowMap)
	if ok {
		t.Fatal("manual override should not match when workflow is missing")
	}
}

func strPtr(v string) *string {
	return &v
}

package notification

import (
	"context"
	"testing"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type testNotificationConfig struct{}

func (testNotificationConfig) GetAppBaseURL() string { return "https://app.example.com" }

type testWorkflowResolver struct {
	result identityservice.ResolveLeadWorkflowResult
}

func (r testWorkflowResolver) ResolveLeadWorkflow(_ context.Context, _ identityservice.ResolveLeadWorkflowInput) (identityservice.ResolveLeadWorkflowResult, error) {
	return r.result, nil
}

type testSender struct {
	quoteProposalCalls         int
	quoteAcceptedCalls         int
	quoteAcceptedThankYouCalls int
}

const testLeadEmail = "lead@example.com"
const testOrgName = "Vakman Portal"

func (s *testSender) SendVerificationEmail(context.Context, string, string) error  { return nil }
func (s *testSender) SendPasswordResetEmail(context.Context, string, string) error { return nil }
func (s *testSender) SendVisitInviteEmail(context.Context, string, string, string, string) error {
	return nil
}
func (s *testSender) SendOrganizationInviteEmail(context.Context, string, string, string) error {
	return nil
}
func (s *testSender) SendPartnerInviteEmail(context.Context, string, string, string, string) error {
	return nil
}
func (s *testSender) SendQuoteProposalEmail(context.Context, string, string, string, string, string) error {
	s.quoteProposalCalls++
	return nil
}
func (s *testSender) SendQuoteAcceptedEmail(context.Context, string, string, string, string, int64) error {
	s.quoteAcceptedCalls++
	return nil
}
func (s *testSender) SendQuoteAcceptedThankYouEmail(context.Context, string, string, string, string, ...email.Attachment) error {
	s.quoteAcceptedThankYouCalls++
	return nil
}
func (s *testSender) SendPartnerOfferAcceptedEmail(context.Context, string, string, string) error {
	return nil
}
func (s *testSender) SendPartnerOfferAcceptedConfirmationEmail(context.Context, string, string) error {
	return nil
}
func (s *testSender) SendPartnerOfferRejectedEmail(context.Context, string, string, string, string) error {
	return nil
}
func (s *testSender) SendCustomEmail(context.Context, string, string, string) error { return nil }

func TestHandleQuoteSentDoesNotUseLegacySenderWithoutOutbox(t *testing.T) {
	sender := &testSender{}
	orgID := uuid.New()
	leadID := uuid.New()
	workflowID := uuid.New()
	stepID := uuid.New()

	m := New(nil, sender, testNotificationConfig{}, logger.New("development"))
	m.SetWorkflowResolver(testWorkflowResolver{result: identityservice.ResolveLeadWorkflowResult{
		Workflow: &identityrepo.Workflow{
			ID: workflowID,
			Steps: []identityrepo.WorkflowStep{{
				ID:           stepID,
				Trigger:      "quote_sent",
				Channel:      "email",
				Audience:     "lead",
				Enabled:      true,
				DelayMinutes: 0,
			}},
		},
		ResolutionSource: "manual_override",
	}})

	err := m.handleQuoteSent(context.Background(), events.QuoteSent{
		QuoteID:          uuid.New(),
		OrganizationID:   orgID,
		LeadID:           leadID,
		PublicToken:      "token-123",
		QuoteNumber:      "OFF-2026-0001",
		ConsumerEmail:    testLeadEmail,
		ConsumerName:     "Lead",
		OrganizationName: testOrgName,
	})
	if err != nil {
		t.Fatalf("handleQuoteSent returned error: %v", err)
	}
	if sender.quoteProposalCalls != 0 {
		t.Fatalf("expected strict workflow mode to avoid legacy quote proposal sender, got %d calls", sender.quoteProposalCalls)
	}
}

func TestHandleQuoteAcceptedDoesNotUseLegacySendersWithoutOutbox(t *testing.T) {
	sender := &testSender{}
	orgID := uuid.New()
	leadID := uuid.New()
	workflowID := uuid.New()
	stepID := uuid.New()

	m := New(nil, sender, testNotificationConfig{}, logger.New("development"))
	m.SetWorkflowResolver(testWorkflowResolver{result: identityservice.ResolveLeadWorkflowResult{
		Workflow: &identityrepo.Workflow{
			ID: workflowID,
			Steps: []identityrepo.WorkflowStep{{
				ID:           stepID,
				Trigger:      "quote_accepted",
				Channel:      "email",
				Audience:     "lead",
				Enabled:      true,
				DelayMinutes: 0,
			}},
		},
		ResolutionSource: "manual_override",
	}})

	err := m.handleQuoteAccepted(context.Background(), events.QuoteAccepted{
		QuoteID:          uuid.New(),
		OrganizationID:   orgID,
		LeadID:           leadID,
		SignatureName:    "Lead",
		TotalCents:       125000,
		QuoteNumber:      "OFF-2026-0002",
		ConsumerEmail:    testLeadEmail,
		ConsumerName:     "Lead",
		OrganizationName: testOrgName,
		AgentEmail:       "agent@example.com",
		AgentName:        "Agent",
	})
	if err != nil {
		t.Fatalf("handleQuoteAccepted returned error: %v", err)
	}
	if sender.quoteAcceptedThankYouCalls != 0 {
		t.Fatalf("expected strict workflow mode to avoid legacy customer thank-you sender, got %d calls", sender.quoteAcceptedThankYouCalls)
	}
	if sender.quoteAcceptedCalls != 0 {
		t.Fatalf("expected strict workflow mode to avoid legacy agent acceptance sender, got %d calls", sender.quoteAcceptedCalls)
	}
}

func TestRenderTemplateTextAcceptsFrontendSyntax(t *testing.T) {
	rendered, err := renderTemplateText("Test bericht {{lead.name}} {{links.track}}", map[string]any{
		"lead":  map[string]any{"name": "Robin"},
		"links": map[string]any{"track": "http://localhost/track/abc"},
	})
	if err != nil {
		t.Fatalf("expected frontend syntax to render, got error: %v", err)
	}
	if rendered != "Test bericht Robin http://localhost/track/abc" {
		t.Fatalf("unexpected rendered text: %q", rendered)
	}
}

func TestRenderTemplateTextRejectsLegacyDotSyntax(t *testing.T) {
	_, err := renderTemplateText("Test bericht {{.lead.name}}", map[string]any{
		"lead": map[string]any{"name": "Robin"},
	})
	if err == nil {
		t.Fatal("expected legacy dot syntax to be rejected")
	}
}

func TestBuildWorkflowStepVariablesContainsSafeNestedMaps(t *testing.T) {
	vars := buildWorkflowStepVariables(workflowStepExecutionContext{})

	rendered, err := renderTemplateText("Offerte {{quote.number}} link {{links.track}}", vars)
	if err != nil {
		t.Fatalf("expected missing quote/links values to render safely, got error: %v", err)
	}
	if rendered == "" {
		t.Fatal("expected rendered output to be non-empty")
	}
}

func TestDispatchQuoteEmailWorkflowSkipsWhenNoRecipients(t *testing.T) {
	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))

	ok := m.dispatchQuoteEmailWorkflow(context.Background(), dispatchQuoteEmailWorkflowParams{
		Rule: &workflowRule{
			Enabled:      true,
			DelayMinutes: 0,
		},
		OrgID:        uuid.New(),
		LeadID:       nil,
		ServiceID:    nil,
		LeadEmail:    "",
		PartnerEmail: "",
		Trigger:      "quote_accepted",
		TemplateVars: map[string]any{},
		Summary:      "summary",
		FallbackNote: "fallback",
	})

	if !ok {
		t.Fatal("expected dispatch to skip cleanly when no recipients are available")
	}
}

func TestRenderWorkflowTemplateSubjectAndBodyFromRule(t *testing.T) {
	subjectTpl := "Offerte {{quote.number}} voor {{lead.name}}"
	bodyTpl := "Hoi {{lead.name}}, bekijk {{links.track}}"
	rule := &workflowRule{
		Enabled:         true,
		DelayMinutes:    0,
		TemplateSubject: &subjectTpl,
		TemplateText:    &bodyTpl,
	}
	vars := map[string]any{
		"lead":  map[string]any{"name": "Robin"},
		"quote": map[string]any{"number": "OFF-2026-0042"},
		"links": map[string]any{"track": "https://app.example.com/track/abc"},
	}

	subject := renderWorkflowTemplateSubject(rule, vars)
	body := renderWorkflowTemplateText(rule, vars)

	if subject != "Offerte OFF-2026-0042 voor Robin" {
		t.Fatalf("unexpected rendered subject: %q", subject)
	}
	if body != "Hoi Robin, bekijk https://app.example.com/track/abc" {
		t.Fatalf("unexpected rendered body: %q", body)
	}
}

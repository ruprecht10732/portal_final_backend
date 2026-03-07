package notification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type testNotificationConfig struct{}

func (testNotificationConfig) GetAppBaseURL() string    { return "https://app.example.com" }
func (testNotificationConfig) GetPublicBaseURL() string { return "https://public.example.com" }

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
	customEmailCalls           int
	lastCustomAttachments      []email.Attachment
}

type testQuotePDFGenerator struct {
	fileKey string
	pdfData []byte
	err     error
	calls   int
}

func (g *testQuotePDFGenerator) RegeneratePDF(_ context.Context, _ uuid.UUID, _ uuid.UUID) (string, []byte, error) {
	g.calls++
	if g.err != nil {
		return "", nil, g.err
	}
	return g.fileKey, g.pdfData, nil
}

type testQuotePDFStorage struct {
	data  []byte
	err   error
	calls int
}

func (s *testQuotePDFStorage) DownloadFile(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return io.NopCloser(strings.NewReader(string(s.data))), nil
}

const testLeadEmail = "lead@example.com"
const testOrgName = "Vakman Portal"
const errUnexpectedRenderedText = "unexpected rendered text: %q"
const generatedPDFContent = "generated-pdf"

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
func (s *testSender) SendCustomEmail(_ context.Context, _ string, _ string, _ string, attachments ...email.Attachment) error {
	s.customEmailCalls++
	s.lastCustomAttachments = append([]email.Attachment(nil), attachments...)
	return nil
}

func TestNormalizeWhatsAppMessage_StripsHTMLAndEntities(t *testing.T) {
	input := "<p>Hallo&nbsp;Robin Oost,&nbsp;welkom&nbsp;bij&nbsp;ons team.&nbsp;We&nbsp;hebben&nbsp;je&nbsp;aanvraag&nbsp;ontvangen.</p><p></p><p>https://example.com/track/abc</p>"
	expected := "Hallo Robin Oost, welkom bij ons team. We hebben je aanvraag ontvangen.\n\nhttps://example.com/track/abc"

	got := normalizeWhatsAppMessage(input)
	if got != expected {
		t.Fatalf(errUnexpectedRenderedText, got)
	}
}

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
		t.Fatalf(errUnexpectedRenderedText, rendered)
	}
}

func TestRenderTemplateTextAcceptsUppercasePlaceholders(t *testing.T) {
	rendered, err := renderTemplateText("Beste {{LEAD.NAME}}, bekijk {{QUOTE.NUMBER}}", map[string]any{
		"lead":  map[string]any{"name": "Robin"},
		"quote": map[string]any{"number": "OFF-2026-0042"},
	})
	if err != nil {
		t.Fatalf("expected uppercase placeholders to render, got error: %v", err)
	}
	if rendered != "Beste Robin, bekijk OFF-2026-0042" {
		t.Fatalf(errUnexpectedRenderedText, rendered)
	}
}

func TestRenderTemplateTextAcceptsMixedCaseNestedKeys(t *testing.T) {
	rendered, err := renderTemplateText("Link: {{quote.previewurl}}", map[string]any{
		"quote": map[string]any{"previewUrl": "https://example.test/quote/abc"},
	})
	if err != nil {
		t.Fatalf("expected mixed-case placeholders to render, got error: %v", err)
	}
	if rendered != "Link: https://example.test/quote/abc" {
		t.Fatalf(errUnexpectedRenderedText, rendered)
	}
}

func TestRenderTemplateTextAcceptsLegacyDotSyntax(t *testing.T) {
	rendered, err := renderTemplateText("Test bericht {{.lead.name}}", map[string]any{
		"lead": map[string]any{"name": "Robin"},
	})
	if err != nil {
		t.Fatalf("expected legacy dot syntax to render, got error: %v", err)
	}
	if rendered != "Test bericht Robin" {
		t.Fatalf(errUnexpectedRenderedText, rendered)
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

func TestRenderWorkflowTemplateTextConvertsEscapedLineBreaks(t *testing.T) {
	bodyTpl := "Hallo {{lead.name}},\\n\\nJe offerte {{quote.number}} staat klaar."
	rule := &workflowRule{
		Enabled:      true,
		DelayMinutes: 0,
		TemplateText: &bodyTpl,
	}
	vars := map[string]any{
		"lead":  map[string]any{"name": "Robin Oost"},
		"quote": map[string]any{"number": "OFF-2026-0010"},
	}

	body := renderWorkflowTemplateText(rule, vars)

	if strings.Contains(body, "\\n") {
		t.Fatalf("expected escaped line breaks to be normalized, got: %q", body)
	}
	if !strings.Contains(body, "\n\n") {
		t.Fatalf("expected rendered body to contain real line breaks, got: %q", body)
	}
}

func TestProcessGenericEmailOutboxAttachesQuotePDFFromStorage(t *testing.T) {
	sender := &testSender{}
	storage := &testQuotePDFStorage{data: []byte("stored-pdf")}
	generator := &testQuotePDFGenerator{pdfData: []byte(generatedPDFContent)}
	orgID := uuid.New()
	quoteID := uuid.New().String()

	m := New(nil, sender, testNotificationConfig{}, logger.New("development"))
	m.SetQuotePDFStorage(storage, "quote-pdfs")
	m.SetQuotePDFGenerator(generator)

	payloadBytes, err := json.Marshal(emailSendOutboxPayload{
		OrgID:    orgID.String(),
		ToEmail:  testLeadEmail,
		Subject:  "Onderwerp",
		BodyHTML: "<p>Body</p>",
		Attachments: []emailSendAttachmentSpec{{
			Kind:     "quote_pdf",
			QuoteID:  &quoteID,
			FileKey:  "quotes/file.pdf",
			FileName: "offerte-test.pdf",
			MIMEType: "application/pdf",
		}},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	err = m.processGenericEmailOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{Payload: payloadBytes})
	if err != nil {
		t.Fatalf("processGenericEmailOutbox returned error: %v", err)
	}
	if sender.customEmailCalls != 1 {
		t.Fatalf("expected 1 custom email call, got %d", sender.customEmailCalls)
	}
	if len(sender.lastCustomAttachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(sender.lastCustomAttachments))
	}
	if string(sender.lastCustomAttachments[0].Content) != "stored-pdf" {
		t.Fatalf("expected stored pdf content, got %q", string(sender.lastCustomAttachments[0].Content))
	}
	if generator.calls != 0 {
		t.Fatalf("expected storage hit to avoid regeneration, got %d generator calls", generator.calls)
	}
}

func TestProcessGenericEmailOutboxRegeneratesQuotePDFAfterStorageFailure(t *testing.T) {
	sender := &testSender{}
	storage := &testQuotePDFStorage{err: errors.New("storage unavailable")}
	generator := &testQuotePDFGenerator{pdfData: []byte(generatedPDFContent)}
	orgID := uuid.New()
	quoteID := uuid.New().String()

	m := New(nil, sender, testNotificationConfig{}, logger.New("development"))
	m.SetQuotePDFStorage(storage, "quote-pdfs")
	m.SetQuotePDFGenerator(generator)

	payloadBytes, err := json.Marshal(emailSendOutboxPayload{
		OrgID:    orgID.String(),
		ToEmail:  testLeadEmail,
		Subject:  "Onderwerp",
		BodyHTML: "<p>Body</p>",
		Attachments: []emailSendAttachmentSpec{{
			Kind:    "quote_pdf",
			QuoteID: &quoteID,
		}},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	err = m.processGenericEmailOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{Payload: payloadBytes})
	if err != nil {
		t.Fatalf("processGenericEmailOutbox returned error: %v", err)
	}
	if len(sender.lastCustomAttachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(sender.lastCustomAttachments))
	}
	if string(sender.lastCustomAttachments[0].Content) != generatedPDFContent {
		t.Fatalf("expected generated pdf content, got %q", string(sender.lastCustomAttachments[0].Content))
	}
	if generator.calls != 1 {
		t.Fatalf("expected 1 generator call, got %d", generator.calls)
	}
}

func TestBuildEmailAttachmentSpecsIncludesQuoteAcceptedPDF(t *testing.T) {
	quoteID := uuid.New().String()
	dispatchCtx := workflowStepDispatchContext{
		Exec: workflowStepExecutionContext{
			Trigger: "quote_accepted",
			Variables: map[string]any{
				"quote": map[string]any{
					"id":         quoteID,
					"number":     "OFF-2026-0003",
					"pdfFileKey": "quotes/signed.pdf",
				},
			},
		},
	}

	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))
	attachments := m.buildEmailAttachmentSpecs(dispatchCtx)
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment spec, got %d", len(attachments))
	}
	if attachments[0].Kind != "quote_pdf" {
		t.Fatalf("expected quote_pdf kind, got %q", attachments[0].Kind)
	}
	if attachments[0].QuoteID == nil || *attachments[0].QuoteID != quoteID {
		t.Fatalf("expected quote id %q, got %#v", quoteID, attachments[0].QuoteID)
	}
	if attachments[0].FileKey != "quotes/signed.pdf" {
		t.Fatalf("expected signed file key, got %q", attachments[0].FileKey)
	}
}

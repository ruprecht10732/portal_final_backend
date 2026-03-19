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
	"portal_final_backend/internal/identity/smtpcrypto"
	leadrepo "portal_final_backend/internal/leads/repository"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/whatsapp"
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

type testOrganizationSettingsReader struct {
	settings identityrepo.OrganizationSettings
	err      error
}

func (r testOrganizationSettingsReader) GetOrganizationSettings(_ context.Context, _ uuid.UUID) (identityrepo.OrganizationSettings, error) {
	return r.settings, r.err
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

type testSubsidyPDFGenerator struct {
	pdfData []byte
	err     error
	calls   int
	last    *isdeSubsidyPDFAttachmentPayload
}

func (s *testQuotePDFStorage) DownloadFile(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return io.NopCloser(strings.NewReader(string(s.data))), nil
}

func (g *testSubsidyPDFGenerator) GenerateSubsidyPDF(data isdeSubsidyPDFAttachmentPayload) ([]byte, error) {
	g.calls++
	copyData := data
	g.last = &copyData
	if g.err != nil {
		return nil, g.err
	}
	return append([]byte(nil), g.pdfData...), nil
}

const testLeadEmail = "lead@example.com"
const testOrgName = "Vakman Portal"
const testSMTPFromEmail = "mailer@example.com"
const errUnexpectedRenderedText = "unexpected rendered text: %q"
const generatedPDFContent = "generated-pdf"
const errMarshalPayloadFmt = "marshal payload: %v"
const testEmailHTMLBody = "<p>Body</p>"
const testLeadAddress = "Hoofdstraat 1, 1234 AB Utrecht"
const testUnsignedQuotePDFKey = "quotes/unsigned.pdf"
const errProcessGenericEmailOutboxFmt = "processGenericEmailOutbox returned error: %v"
const errExpectedOneAttachmentFmt = "expected 1 attachment, got %d"
const testPDFMIMEType = "application/pdf"
const expectedLeadOptInLookupFmt = "expected one lead opt-in lookup, got %d"
const testWhatsAppPhoneNumber = "+31612345678"
const testQuoteNumber0042 = "OFF-2026-0042"

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
func (s *testSender) SendPartnerOfferAcceptedConfirmationEmail(context.Context, string, string, ...email.Attachment) error {
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

type testLeadWhatsAppReader struct {
	optedIn bool
	err     error
	calls   int
}

func (r *testLeadWhatsAppReader) IsWhatsAppOptedIn(_ context.Context, _ uuid.UUID, _ uuid.UUID) (bool, error) {
	r.calls++
	return r.optedIn, r.err
}

type testWhatsAppSender struct {
	calls int
	err   error
}

func (s *testWhatsAppSender) SendMessage(_ context.Context, _ string, _ string, _ string) (whatsapp.SendResult, error) {
	s.calls++
	return whatsapp.SendResult{}, s.err
}

func TestNormalizeWhatsAppMessageStripsHTMLAndEntities(t *testing.T) {
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

func TestResolveSenderUsesTenantSMTPWhenConfigured(t *testing.T) {
	defaultSender := &testSender{}
	host := "smtp.example.com"
	port := 587
	username := testSMTPFromEmail
	fromEmail := testSMTPFromEmail
	fromName := "Example Mailer"
	encryptionKey := []byte("12345678901234567890123456789012")
	encryptedPassword, err := smtpcrypto.Encrypt("super-secret", encryptionKey)
	if err != nil {
		t.Fatalf("encrypt smtp password: %v", err)
	}

	m := New(nil, defaultSender, testNotificationConfig{}, logger.New("development"))
	m.SetOrganizationSettingsReader(testOrganizationSettingsReader{settings: identityrepo.OrganizationSettings{
		SMTPHost:      &host,
		SMTPPort:      &port,
		SMTPUsername:  &username,
		SMTPPassword:  &encryptedPassword,
		SMTPFromEmail: &fromEmail,
		SMTPFromName:  &fromName,
	}})
	m.SetSMTPEncryptionKey(encryptionKey)

	resolved := m.resolveSender(context.Background(), uuid.New())
	if _, ok := resolved.(*email.SMTPSender); !ok {
		t.Fatalf("expected tenant smtp sender, got %T", resolved)
	}
}

func TestResolveSenderFallsBackToDefaultSenderWithoutSMTPKey(t *testing.T) {
	defaultSender := &testSender{}
	host := "smtp.example.com"
	port := 587
	username := testSMTPFromEmail
	fromEmail := testSMTPFromEmail
	fromName := "Example Mailer"
	encryptionKey := []byte("12345678901234567890123456789012")
	encryptedPassword, err := smtpcrypto.Encrypt("super-secret", encryptionKey)
	if err != nil {
		t.Fatalf("encrypt smtp password: %v", err)
	}

	m := New(nil, defaultSender, testNotificationConfig{}, logger.New("development"))
	m.SetOrganizationSettingsReader(testOrganizationSettingsReader{settings: identityrepo.OrganizationSettings{
		SMTPHost:      &host,
		SMTPPort:      &port,
		SMTPUsername:  &username,
		SMTPPassword:  &encryptedPassword,
		SMTPFromEmail: &fromEmail,
		SMTPFromName:  &fromName,
	}})

	resolved := m.resolveSender(context.Background(), uuid.New())
	if resolved != defaultSender {
		t.Fatalf("expected default sender fallback without smtp key, got %T", resolved)
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
		"quote": map[string]any{"number": testQuoteNumber0042},
	})
	if err != nil {
		t.Fatalf("expected uppercase placeholders to render, got error: %v", err)
	}
	if rendered != "Beste Robin, bekijk "+testQuoteNumber0042 {
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
		"quote": map[string]any{"number": testQuoteNumber0042},
		"links": map[string]any{"track": "https://app.example.com/track/abc"},
	}

	subject, err := renderWorkflowTemplateSubjectWithError(rule, vars)
	if err != nil {
		t.Fatalf("unexpected subject render error: %v", err)
	}
	body, err := renderWorkflowTemplateTextWithError(rule, vars)
	if err != nil {
		t.Fatalf("unexpected body render error: %v", err)
	}

	if subject != "Offerte "+testQuoteNumber0042+" voor Robin" {
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

	body, err := renderWorkflowTemplateTextWithError(rule, vars)
	if err != nil {
		t.Fatalf("unexpected body render error: %v", err)
	}

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
		BodyHTML: testEmailHTMLBody,
		Attachments: []emailSendAttachmentSpec{{
			Kind:     "quote_pdf",
			QuoteID:  &quoteID,
			FileKey:  "quotes/file.pdf",
			FileName: "offerte-test.pdf",
			MIMEType: testPDFMIMEType,
		}},
	})
	if err != nil {
		t.Fatalf(errMarshalPayloadFmt, err)
	}

	err = m.processGenericEmailOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{Payload: payloadBytes})
	if err != nil {
		t.Fatalf(errProcessGenericEmailOutboxFmt, err)
	}
	if sender.customEmailCalls != 1 {
		t.Fatalf("expected 1 custom email call, got %d", sender.customEmailCalls)
	}
	if len(sender.lastCustomAttachments) != 1 {
		t.Fatalf(errExpectedOneAttachmentFmt, len(sender.lastCustomAttachments))
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
		BodyHTML: testEmailHTMLBody,
		Attachments: []emailSendAttachmentSpec{{
			Kind:    "quote_pdf",
			QuoteID: &quoteID,
		}},
	})
	if err != nil {
		t.Fatalf(errMarshalPayloadFmt, err)
	}

	err = m.processGenericEmailOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{Payload: payloadBytes})
	if err != nil {
		t.Fatalf(errProcessGenericEmailOutboxFmt, err)
	}
	if len(sender.lastCustomAttachments) != 1 {
		t.Fatalf(errExpectedOneAttachmentFmt, len(sender.lastCustomAttachments))
	}
	if string(sender.lastCustomAttachments[0].Content) != generatedPDFContent {
		t.Fatalf("expected generated pdf content, got %q", string(sender.lastCustomAttachments[0].Content))
	}
	if generator.calls != 1 {
		t.Fatalf("expected 1 generator call, got %d", generator.calls)
	}
}

func TestProcessGenericEmailOutboxAttachesISDESubsidyPDF(t *testing.T) {
	sender := &testSender{}
	generator := &testSubsidyPDFGenerator{pdfData: []byte("subsidy-pdf")}
	orgID := uuid.New()

	m := New(nil, sender, testNotificationConfig{}, logger.New("development"))
	m.SetSubsidyPDFGenerator(generator)

	payloadBytes, err := json.Marshal(emailSendOutboxPayload{
		OrgID:    orgID.String(),
		ToEmail:  testLeadEmail,
		Subject:  "Onderwerp",
		BodyHTML: testEmailHTMLBody,
		Attachments: []emailSendAttachmentSpec{{
			Kind:     "isde_subsidy_pdf",
			FileName: "isde-subsidie-test.pdf",
			MIMEType: testPDFMIMEType,
			ISDESubsidy: &isdeSubsidyPDFAttachmentPayload{
				QuoteNumber:          testQuoteNumber0042,
				OrganizationName:     testOrgName,
				LeadName:             "Lead",
				LeadAddress:          testLeadAddress,
				TotalAmountCents:     1336000,
				IsDoubled:            true,
				EligibleMeasureCount: 2,
				GlassBreakdown: []isdeSubsidyPDFLineItem{{
					Description: "HR++ glas",
					AreaM2:      10,
					AmountCents: 460000,
				}},
				Installations: []isdeSubsidyPDFLineItem{{
					Description: "Warmtepomp KA00001",
					AmountCents: 876000,
				}},
			},
		}},
	})
	if err != nil {
		t.Fatalf(errMarshalPayloadFmt, err)
	}

	err = m.processGenericEmailOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{Payload: payloadBytes})
	if err != nil {
		t.Fatalf(errProcessGenericEmailOutboxFmt, err)
	}
	if len(sender.lastCustomAttachments) != 1 {
		t.Fatalf(errExpectedOneAttachmentFmt, len(sender.lastCustomAttachments))
	}
	if string(sender.lastCustomAttachments[0].Content) != "subsidy-pdf" {
		t.Fatalf("expected subsidy pdf content, got %q", string(sender.lastCustomAttachments[0].Content))
	}
	if generator.calls != 1 {
		t.Fatalf("expected 1 subsidy generator call, got %d", generator.calls)
	}
	if generator.last == nil || generator.last.TotalAmountCents != 1336000 {
		t.Fatalf("expected subsidy payload to be forwarded, got %#v", generator.last)
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

func TestBuildEmailAttachmentSpecsIncludesQuoteSentPDF(t *testing.T) {
	quoteID := uuid.New().String()
	dispatchCtx := workflowStepDispatchContext{
		Exec: workflowStepExecutionContext{
			Trigger: "quote_sent",
			Variables: map[string]any{
				"quote": map[string]any{
					"id":         quoteID,
					"number":     "OFF-2026-0004",
					"pdfFileKey": testUnsignedQuotePDFKey,
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
	if attachments[0].FileKey != testUnsignedQuotePDFKey {
		t.Fatalf("expected unsigned file key, got %q", attachments[0].FileKey)
	}
	if attachments[0].FileName != "offerte-OFF-2026-0004.pdf" {
		t.Fatalf("expected attachment filename to be derived from quote number, got %q", attachments[0].FileName)
	}
	if attachments[0].MIMEType != testPDFMIMEType {
		t.Fatalf("expected %s mime type, got %q", testPDFMIMEType, attachments[0].MIMEType)
	}
}

func TestBuildEmailAttachmentSpecsIncludesISDESubsidyPDFWhenPresent(t *testing.T) {
	quoteID := uuid.New().String()
	dispatchCtx := workflowStepDispatchContext{
		Exec: workflowStepExecutionContext{
			Trigger: "quote_sent",
			Variables: map[string]any{
				"lead": map[string]any{
					"name":    "Robin",
					"address": testLeadAddress,
				},
				"org": map[string]any{
					"name": testOrgName,
				},
				"quote": map[string]any{
					"id":         quoteID,
					"number":     testQuoteNumber0042,
					"pdfFileKey": testUnsignedQuotePDFKey,
				},
				"subsidy": map[string]any{
					"totalAmountCents":     250000,
					"eligibleMeasureCount": 1,
					"insulationBreakdown": []map[string]any{{
						"description": "Dakisolatie",
						"areaM2":      25,
						"amountCents": 250000,
					}},
				},
			},
		},
	}

	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))
	attachments := m.buildEmailAttachmentSpecs(dispatchCtx)
	if len(attachments) != 2 {
		t.Fatalf("expected 2 attachment specs, got %d", len(attachments))
	}
	if attachments[1].Kind != "isde_subsidy_pdf" {
		t.Fatalf("expected isde_subsidy_pdf kind, got %q", attachments[1].Kind)
	}
	if attachments[1].ISDESubsidy == nil {
		t.Fatal("expected isde subsidy payload to be included")
	}
	if attachments[1].ISDESubsidy.OrganizationName != testOrgName {
		t.Fatalf("expected org name %q, got %q", testOrgName, attachments[1].ISDESubsidy.OrganizationName)
	}
	if attachments[1].ISDESubsidy.LeadAddress != testLeadAddress {
		t.Fatalf("expected lead address to be propagated, got %q", attachments[1].ISDESubsidy.LeadAddress)
	}
	if attachments[1].FileName != "isde-subsidie-OFF-2026-0042.pdf" {
		t.Fatalf("expected subsidy attachment filename, got %q", attachments[1].FileName)
	}
}

func TestProcessGenericWhatsAppOutboxMissingLeadReturnsNilAndDoesNotSend(t *testing.T) {
	leadID := uuid.New()
	orgID := uuid.New()
	leadIDValue := leadID.String()
	reader := &testLeadWhatsAppReader{err: leadrepo.ErrNotFound}
	sender := &testWhatsAppSender{}

	payloadBytes, err := json.Marshal(whatsAppSendOutboxPayload{
		LeadID:      &leadIDValue,
		OrgID:       orgID.String(),
		PhoneNumber: testWhatsAppPhoneNumber,
		Message:     "Testbericht",
		Category:    "workflow",
	})
	if err != nil {
		t.Fatalf(errMarshalPayloadFmt, err)
	}

	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))
	m.SetLeadWhatsAppReader(reader)
	m.SetWhatsAppSender(sender)
	m.SetNotificationOutbox(&notificationoutbox.Repository{})

	err = m.processGenericWhatsAppOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{
		ID:      uuid.New(),
		Payload: payloadBytes,
	})
	if err != nil {
		t.Fatalf("expected nil error for missing lead, got %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf(expectedLeadOptInLookupFmt, reader.calls)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no whatsapp send for missing lead, got %d calls", sender.calls)
	}
}

func TestProcessGenericWhatsAppOutboxOptedOutLeadReturnsNilAndDoesNotSend(t *testing.T) {
	leadID := uuid.New()
	orgID := uuid.New()
	leadIDValue := leadID.String()
	reader := &testLeadWhatsAppReader{optedIn: false}
	sender := &testWhatsAppSender{}

	payloadBytes, err := json.Marshal(whatsAppSendOutboxPayload{
		LeadID:      &leadIDValue,
		OrgID:       orgID.String(),
		PhoneNumber: testWhatsAppPhoneNumber,
		Message:     "Testbericht",
		Category:    "workflow",
	})
	if err != nil {
		t.Fatalf(errMarshalPayloadFmt, err)
	}

	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))
	m.SetLeadWhatsAppReader(reader)
	m.SetWhatsAppSender(sender)
	m.SetNotificationOutbox(&notificationoutbox.Repository{})

	err = m.processGenericWhatsAppOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{
		ID:      uuid.New(),
		Payload: payloadBytes,
	})
	if err != nil {
		t.Fatalf("expected nil error for opted-out lead, got %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf(expectedLeadOptInLookupFmt, reader.calls)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no whatsapp send for opted-out lead, got %d calls", sender.calls)
	}
}

func TestProcessGenericWhatsAppOutboxTransientReaderErrorIsRetryable(t *testing.T) {
	leadID := uuid.New()
	orgID := uuid.New()
	leadIDValue := leadID.String()
	transientErr := errors.New("temporary lookup failure")
	reader := &testLeadWhatsAppReader{err: transientErr}
	sender := &testWhatsAppSender{}

	payloadBytes, err := json.Marshal(whatsAppSendOutboxPayload{
		LeadID:      &leadIDValue,
		OrgID:       orgID.String(),
		PhoneNumber: testWhatsAppPhoneNumber,
		Message:     "Testbericht",
		Category:    "workflow",
	})
	if err != nil {
		t.Fatalf(errMarshalPayloadFmt, err)
	}

	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))
	m.SetLeadWhatsAppReader(reader)
	m.SetWhatsAppSender(sender)
	m.SetNotificationOutbox(&notificationoutbox.Repository{})

	err = m.processGenericWhatsAppOutbox(context.Background(), events.NotificationOutboxDue{TenantID: orgID}, notificationoutbox.Record{
		ID:      uuid.New(),
		Payload: payloadBytes,
	})
	if !errors.Is(err, transientErr) {
		t.Fatalf("expected transient error to be returned, got %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf(expectedLeadOptInLookupFmt, reader.calls)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no whatsapp send when reader fails, got %d calls", sender.calls)
	}
}

func TestBuildQuoteAnnotationTemplateVarsIncludesPreviewAndAnnotation(t *testing.T) {
	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))
	quoteID := uuid.New()
	itemID := uuid.New()
	leadID := uuid.New()

	vars := m.buildQuoteAnnotationTemplateVars(context.Background(), events.QuoteAnnotated{
		QuoteID:          quoteID,
		OrganizationID:   uuid.New(),
		LeadID:           leadID,
		QuoteNumber:      testQuoteNumber0042,
		PublicToken:      "public-token-42",
		ItemID:           itemID,
		ItemDescription:  "Warmtepomp installatie",
		AuthorType:       "agent",
		Text:             "We plannen dit in week 14.",
		ConsumerEmail:    testLeadEmail,
		ConsumerName:     "Robin",
		ConsumerPhone:    testWhatsAppPhoneNumber,
		OrganizationName: testOrgName,
		CreatorEmail:     "agent@example.com",
		CreatorName:      "Agent Example",
	})

	quoteVars, ok := vars["quote"].(map[string]any)
	if !ok {
		t.Fatal("expected quote vars map")
	}
	if quoteVars["number"] != testQuoteNumber0042 {
		t.Fatalf("expected quote number, got %#v", quoteVars["number"])
	}
	if quoteVars["previewUrl"] != "https://public.example.com/quote/public-token-42" {
		t.Fatalf("expected preview url, got %#v", quoteVars["previewUrl"])
	}

	annotationVars, ok := vars["annotation"].(map[string]any)
	if !ok {
		t.Fatal("expected annotation vars map")
	}
	if annotationVars["text"] != "We plannen dit in week 14." {
		t.Fatalf("expected annotation text, got %#v", annotationVars["text"])
	}
	if annotationVars["itemDescription"] != "Warmtepomp installatie" {
		t.Fatalf("expected item description, got %#v", annotationVars["itemDescription"])
	}
}

func TestDispatchQuoteQuestionAskedPartnerWhatsAppWorkflowSkipsWithoutPhone(t *testing.T) {
	workflowID := uuid.New()
	stepID := uuid.New()
	m := New(nil, &testSender{}, testNotificationConfig{}, logger.New("development"))
	m.SetWorkflowResolver(testWorkflowResolver{result: identityservice.ResolveLeadWorkflowResult{
		Workflow: &identityrepo.Workflow{
			ID: workflowID,
			Steps: []identityrepo.WorkflowStep{{
				ID:           stepID,
				Trigger:      "quote_question_asked",
				Channel:      "whatsapp",
				Audience:     "partner",
				Enabled:      true,
				DelayMinutes: 0,
			}},
		},
		ResolutionSource: "manual_override",
	}})

	ok := m.dispatchQuoteQuestionAskedPartnerWhatsAppWorkflow(context.Background(), events.QuoteAnnotated{
		OrganizationID: uuid.New(),
		LeadID:         uuid.New(),
		QuoteNumber:    "OFF-2026-0099",
		ConsumerName:   "Robin",
		CreatorName:    "Agent Example",
		Text:           "Kan dit sneller?",
	})

	if !ok {
		t.Fatal("expected missing partner phone to skip cleanly")
	}
}

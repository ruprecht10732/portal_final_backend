package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/notification/inapp"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/pdf"
	"portal_final_backend/internal/scheduler"
	"strings"
	"time"

	"github.com/google/uuid"
)

// QuotePDFGenerator generates and stores an unsigned PDF for a quote.
type QuotePDFGenerator interface {
	RegeneratePDF(ctx context.Context, quoteID, organizationID uuid.UUID) (string, []byte, error)
}

// QuoteActivityWriter persists activity log entries for quotes.
type QuoteActivityWriter interface {
	CreateActivity(ctx context.Context, quoteID, orgID uuid.UUID, eventType, message string, metadata map[string]interface{}) error
}

type QuoteAcceptedPDFScheduler interface {
	EnqueueGenerateAcceptedQuotePDFRequest(ctx context.Context, req scheduler.GenerateAcceptedQuotePDFRequest) error
}

type QuotePDFFileStorage interface {
	DownloadFile(ctx context.Context, bucket, fileKey string) (io.ReadCloser, error)
}

type SubsidyPDFGenerator interface {
	GenerateSubsidyPDF(data isdeSubsidyPDFAttachmentPayload) ([]byte, error)
}

type subsidyPDFGeneratorFunc func(data isdeSubsidyPDFAttachmentPayload) ([]byte, error)

const pdfMIMEType = "application/pdf"

func (f subsidyPDFGeneratorFunc) GenerateSubsidyPDF(data isdeSubsidyPDFAttachmentPayload) ([]byte, error) {
	return f(data)
}

type isdeSubsidyPDFAttachmentPayload struct {
	QuoteNumber          string                   `json:"quoteNumber,omitempty"`
	OrganizationName     string                   `json:"organizationName,omitempty"`
	LeadName             string                   `json:"leadName,omitempty"`
	LeadAddress          string                   `json:"leadAddress,omitempty"`
	TotalAmountCents     int64                    `json:"totalAmountCents"`
	IsDoubled            bool                     `json:"isDoubled,omitempty"`
	EligibleMeasureCount int                      `json:"eligibleMeasureCount,omitempty"`
	InsulationBreakdown  []isdeSubsidyPDFLineItem `json:"insulationBreakdown,omitempty"`
	GlassBreakdown       []isdeSubsidyPDFLineItem `json:"glassBreakdown,omitempty"`
	Installations        []isdeSubsidyPDFLineItem `json:"installations,omitempty"`
	UnknownMeasureIDs    []string                 `json:"unknownMeasureIds,omitempty"`
	UnknownMeldcodes     []string                 `json:"unknownMeldcodes,omitempty"`
}

type isdeSubsidyPDFLineItem struct {
	Description string  `json:"description"`
	AreaM2      float64 `json:"areaM2,omitempty"`
	AmountCents int64   `json:"amountCents"`
}

func buildQuotePDFAttachmentFileName(quoteNumber string) string {
	trimmed := strings.TrimSpace(quoteNumber)
	if trimmed == "" {
		return "offerte.pdf"
	}
	return fmt.Sprintf("offerte-%s.pdf", trimmed)
}

func buildISDESubsidyPDFAttachmentFileName(quoteNumber string) string {
	trimmed := strings.TrimSpace(quoteNumber)
	if trimmed == "" {
		return "isde-subsidie.pdf"
	}
	return fmt.Sprintf("isde-subsidie-%s.pdf", trimmed)
}

func buildISDESubsidyAttachmentPayload(vars map[string]any, quoteMap map[string]any) *isdeSubsidyPDFAttachmentPayload {
	payload, ok := extractISDESubsidyAttachmentPayload(vars, quoteMap)
	if !ok || payload.TotalAmountCents <= 0 {
		return nil
	}

	if payload.QuoteNumber == "" {
		payload.QuoteNumber = strings.TrimSpace(stringFromMap(quoteMap, "number"))
	}
	if payload.OrganizationName == "" {
		payload.OrganizationName = strings.TrimSpace(stringFromNestedMap(vars, "org", "name"))
	}
	if payload.LeadName == "" {
		payload.LeadName = strings.TrimSpace(stringFromNestedMap(vars, "lead", "name"))
	}
	if payload.LeadAddress == "" {
		payload.LeadAddress = buildLeadAddressFromWorkflowVars(vars)
	}

	return &payload
}

func extractISDESubsidyAttachmentPayload(vars map[string]any, quoteMap map[string]any) (isdeSubsidyPDFAttachmentPayload, bool) {
	candidates := []any{
		vars["isdeSubsidy"],
		vars["subsidy"],
		vars["isde"],
		quoteMap["isdeSubsidy"],
		quoteMap["subsidy"],
		quoteMap["isde"],
	}

	for _, candidate := range candidates {
		payload, ok := decodeISDESubsidyAttachmentPayload(candidate)
		if ok {
			return payload, true
		}
	}

	return isdeSubsidyPDFAttachmentPayload{}, false
}

func decodeISDESubsidyAttachmentPayload(candidate any) (isdeSubsidyPDFAttachmentPayload, bool) {
	switch value := candidate.(type) {
	case nil:
		return isdeSubsidyPDFAttachmentPayload{}, false
	case isdeSubsidyPDFAttachmentPayload:
		return value, true
	case *isdeSubsidyPDFAttachmentPayload:
		if value == nil {
			return isdeSubsidyPDFAttachmentPayload{}, false
		}
		return *value, true
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return isdeSubsidyPDFAttachmentPayload{}, false
		}
		var payload isdeSubsidyPDFAttachmentPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return decodeQuoteSubsidySnapshotPayload(data)
		}
		if payload.TotalAmountCents <= 0 {
			return decodeQuoteSubsidySnapshotPayload(data)
		}
		return payload, true
	}
}

func decodeQuoteSubsidySnapshotPayload(data []byte) (isdeSubsidyPDFAttachmentPayload, bool) {
	var snapshot struct {
		Result *isdeSubsidyPDFAttachmentPayload `json:"result"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil || snapshot.Result == nil {
		return isdeSubsidyPDFAttachmentPayload{}, false
	}
	return *snapshot.Result, true
}

func injectQuoteSubsidyTemplateVars(templateVars map[string]any, subsidy map[string]any) {
	if len(subsidy) == 0 {
		return
	}
	templateVars["isdeSubsidy"] = subsidy
	quoteMap, ok := templateVars["quote"].(map[string]any)
	if !ok {
		return
	}
	quoteMap["isdeSubsidy"] = subsidy
}

func (m *Module) buildPublicQuotePDFURL(tokenValue string) string {
	base := strings.TrimRight(m.cfg.GetPublicAPIBaseURL(), "/")
	return fmt.Sprintf(quotePDFPathFmt, base, tokenValue)
}

func (m *Module) resolveQuotePDFAttachment(ctx context.Context, orgID uuid.UUID, spec emailSendAttachmentSpec) (email.Attachment, error) {
	if spec.QuoteID == nil || strings.TrimSpace(*spec.QuoteID) == "" {
		return email.Attachment{}, fmt.Errorf("%w: quote attachment missing quoteId", errInvalidOutboxPayload)
	}
	quoteID, err := uuid.Parse(strings.TrimSpace(*spec.QuoteID))
	if err != nil {
		return email.Attachment{}, fmt.Errorf("%w: invalid quoteId %q", errInvalidOutboxPayload, strings.TrimSpace(*spec.QuoteID))
	}

	fileName := strings.TrimSpace(spec.FileName)
	if fileName == "" {
		fileName = buildQuotePDFAttachmentFileName(quoteID.String())
	}
	mimeType := strings.TrimSpace(spec.MIMEType)
	if mimeType == "" {
		mimeType = pdfMIMEType
	}

	if m.quotePDFGen == nil {
		return email.Attachment{}, fmt.Errorf("quote pdf generator not configured")
	}

	_, data, err := m.quotePDFGen.RegeneratePDF(ctx, quoteID, orgID)
	if err != nil {
		return email.Attachment{}, fmt.Errorf("generate quote pdf attachment: %w", err)
	}
	return email.Attachment{Content: data, FileName: fileName, MIMEType: mimeType}, nil
}

func (m *Module) resolveISDESubsidyPDFAttachment(spec emailSendAttachmentSpec) (email.Attachment, error) {
	if spec.ISDESubsidy == nil {
		return email.Attachment{}, fmt.Errorf("%w: isde subsidy attachment missing payload", errInvalidOutboxPayload)
	}
	if m.subsidyPDFGen == nil {
		return email.Attachment{}, fmt.Errorf("isde subsidy pdf generator not configured")
	}

	fileName := strings.TrimSpace(spec.FileName)
	if fileName == "" {
		fileName = buildISDESubsidyPDFAttachmentFileName(spec.ISDESubsidy.QuoteNumber)
	}
	mimeType := strings.TrimSpace(spec.MIMEType)
	if mimeType == "" {
		mimeType = pdfMIMEType
	}

	pdfData, err := m.subsidyPDFGen.GenerateSubsidyPDF(*spec.ISDESubsidy)
	if err != nil {
		return email.Attachment{}, err
	}

	return email.Attachment{Content: pdfData, FileName: fileName, MIMEType: mimeType}, nil
}

func generateISDESubsidyPDF(data isdeSubsidyPDFAttachmentPayload) ([]byte, error) {
	return pdf.GenerateISDESummaryPDF(pdf.ISDESummaryPDFData{
		QuoteNumber:          data.QuoteNumber,
		OrganizationName:     data.OrganizationName,
		LeadName:             data.LeadName,
		LeadAddress:          data.LeadAddress,
		TotalAmountCents:     data.TotalAmountCents,
		IsDoubled:            data.IsDoubled,
		EligibleMeasureCount: data.EligibleMeasureCount,
		InsulationBreakdown:  toPDFISDELineItems(data.InsulationBreakdown),
		GlassBreakdown:       toPDFISDELineItems(data.GlassBreakdown),
		Installations:        toPDFISDELineItems(data.Installations),
		UnknownMeasureIDs:    append([]string(nil), data.UnknownMeasureIDs...),
		UnknownMeldcodes:     append([]string(nil), data.UnknownMeldcodes...),
	})
}

func toPDFISDELineItems(items []isdeSubsidyPDFLineItem) []pdf.ISDESummaryLineItem {
	result := make([]pdf.ISDESummaryLineItem, 0, len(items))
	for _, item := range items {
		result = append(result, pdf.ISDESummaryLineItem{
			Description: item.Description,
			AreaM2:      item.AreaM2,
			AmountCents: item.AmountCents,
		})
	}
	return result
}

func (m *Module) handleQuoteSent(ctx context.Context, e events.QuoteSent) error {
	m.publishQuoteSentEvents(e)
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_sent",
		"Offerte verstuurd naar "+e.ConsumerName,
		map[string]interface{}{"quoteNumber": e.QuoteNumber, "consumerEmail": e.ConsumerEmail})

	pdfFileKey := ""
	if m.quotePDFGen != nil {
		if fileKey, _, err := m.quotePDFGen.RegeneratePDF(ctx, e.QuoteID, e.OrganizationID); err != nil {
			m.log.Warn("failed to pre-generate quote PDF on send", "quoteId", e.QuoteID, "error", err)
		} else {
			pdfFileKey = strings.TrimSpace(fileKey)
		}
	}

	_ = m.dispatchQuoteSentLeadEmailWorkflow(ctx, e, pdfFileKey)
	_ = m.dispatchQuoteSentLeadWhatsAppWorkflow(ctx, e)

	m.log.Info("quote sent event processed", "quoteId", e.QuoteID)
	return nil
}

type dispatchQuoteEmailWorkflowParams struct {
	Rule         *workflowRule
	OrgID        uuid.UUID
	LeadID       *uuid.UUID
	ServiceID    *uuid.UUID
	LeadEmail    string
	PartnerEmail string
	Trigger      string
	TemplateVars map[string]any
	Summary      string
	FallbackNote string
}

func (m *Module) dispatchQuoteEmailWorkflow(ctx context.Context, p dispatchQuoteEmailWorkflowParams) bool {
	if p.Rule == nil {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_not_found")
		return false
	}
	if !p.Rule.Enabled {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_disabled")
		return true
	}
	if strings.TrimSpace(p.LeadEmail) == "" && strings.TrimSpace(p.PartnerEmail) == "" {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "no_recipients")
		return true
	}
	if m.notificationOutbox == nil {
		m.log.Warn("workflow email dispatch blocked", "orgId", p.OrgID, "trigger", p.Trigger, "reason", "outbox_repo_missing")
		return false
	}

	bodyText, bodyErr := renderWorkflowTemplateTextWithError(p.Rule, p.TemplateVars)
	subjectText, subjectErr := renderWorkflowTemplateSubjectWithError(p.Rule, p.TemplateVars)
	if bodyErr != nil || subjectErr != nil {
		m.log.Warn("workflow email template render failed",
			"orgId", p.OrgID,
			"trigger", p.Trigger,
			"bodyError", bodyErr,
			"subjectError", subjectErr,
		)
		return true
	}
	subject := strings.TrimSpace(subjectText)
	if subject == "" || strings.TrimSpace(bodyText) == "" {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "empty_subject_or_body", "hasSubject", subject != "", "hasBody", strings.TrimSpace(bodyText) != "")
		return true
	}
	bodyHTML := strings.ReplaceAll(bodyText, "\n", "<br/>")
	recipientConfig := map[string]any{
		"includeLeadContact": strings.TrimSpace(p.LeadEmail) != "",
		"includePartner":     strings.TrimSpace(p.PartnerEmail) != "",
	}
	steps := []repository.WorkflowStep{{
		Enabled:         true,
		Channel:         "email",
		Audience:        "lead",
		DelayMinutes:    p.Rule.DelayMinutes,
		TemplateSubject: &subject,
		TemplateBody:    &bodyHTML,
		RecipientConfig: recipientConfig,
	}}

	err := m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
		OrgID:          p.OrgID,
		LeadID:         p.LeadID,
		ServiceID:      p.ServiceID,
		LeadEmail:      p.LeadEmail,
		PartnerEmail:   p.PartnerEmail,
		Trigger:        p.Trigger,
		DefaultSummary: p.Summary,
		DefaultActor:   "System",
		DefaultOrigin:  workflowEngineActorName,
		Variables:      p.TemplateVars,
	})
	if err != nil {
		m.log.Warn(p.FallbackNote, "error", err, "orgId", p.OrgID)
		return false
	}

	return true
}

type dispatchQuoteWhatsAppWorkflowParams struct {
	Rule         *workflowRule
	OrgID        uuid.UUID
	LeadID       *uuid.UUID
	ServiceID    *uuid.UUID
	LeadPhone    string
	Trigger      string
	TemplateVars map[string]any
	Summary      string
	FallbackNote string
}

func (m *Module) dispatchQuoteWhatsAppWorkflow(ctx context.Context, p dispatchQuoteWhatsAppWorkflowParams) bool {
	if p.Rule == nil {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_not_found")
		return false
	}
	if !p.Rule.Enabled {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_disabled")
		return true
	}
	if strings.TrimSpace(p.LeadPhone) == "" {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "missing_phone")
		return true
	}
	if p.LeadID != nil && !m.isLeadWhatsAppOptedIn(ctx, *p.LeadID, p.OrgID) {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "leadId", *p.LeadID, "reason", "lead_opted_out_in_db")
		return true
	}
	if m.notificationOutbox == nil {
		m.log.Warn("workflow whatsapp dispatch blocked", "orgId", p.OrgID, "trigger", p.Trigger, "reason", "outbox_repo_missing")
		return false
	}

	messageText, err := renderWorkflowTemplateTextWithError(p.Rule, p.TemplateVars)
	if err != nil {
		m.log.Warn(msgWorkflowWhatsAppTemplateRenderFailed, "orgId", p.OrgID, "trigger", p.Trigger, "error", err)
		return true
	}
	messageText = normalizeWhatsAppMessage(messageText)
	if strings.TrimSpace(messageText) == "" {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "empty_message_body")
		return true
	}

	steps := []repository.WorkflowStep{{
		Enabled:      true,
		Channel:      "whatsapp",
		Audience:     "lead",
		DelayMinutes: p.Rule.DelayMinutes,
		TemplateBody: &messageText,
		RecipientConfig: map[string]any{
			"includeLeadContact": true,
		},
	}}
	enqueueErr := m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
		OrgID:          p.OrgID,
		LeadID:         p.LeadID,
		ServiceID:      p.ServiceID,
		LeadPhone:      p.LeadPhone,
		Trigger:        p.Trigger,
		DefaultSummary: p.Summary,
		DefaultActor:   "System",
		DefaultOrigin:  workflowEngineActorName,
	})
	if enqueueErr != nil {
		m.log.Warn(p.FallbackNote, "error", enqueueErr, "orgId", p.OrgID)
		return false
	}

	return true
}

func (m *Module) publishQuoteSentEvents(e events.QuoteSent) {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteSent, e.QuoteID, map[string]interface{}{
		"quoteNumber": e.QuoteNumber,
		"status":      "Sent",
	})
	if m.sse == nil {
		return
	}

	evt := sse.Event{
		Type:   sse.EventQuoteSent,
		LeadID: e.LeadID,
		Data: map[string]interface{}{
			"quoteId":     e.QuoteID,
			"quoteNumber": e.QuoteNumber,
			"status":      "Sent",
		},
	}
	if e.LeadServiceID != nil {
		evt.ServiceID = *e.LeadServiceID
	}
	m.sse.PublishToLead(e.LeadID, evt)
}

func (m *Module) dispatchQuoteSentLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteSent) bool {
	if strings.TrimSpace(e.ConsumerPhone) == "" {
		return true
	}
	if !m.isLeadWhatsAppOptedIn(ctx, e.LeadID, e.OrganizationID) {
		return true
	}

	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_sent", "whatsapp", "lead", nil)
	proposalURL := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/") + quotePublicPathPrefix + e.PublicToken
	downloadURL := m.buildPublicQuotePDFURL(e.PublicToken)
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"number": e.QuoteNumber, "previewUrl": proposalURL, "downloadUrl": downloadURL},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)

	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_sent",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("WhatsApp offerte verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_sent lead whatsapp workflow",
	})
}

func (m *Module) dispatchQuoteSentLeadEmailWorkflow(ctx context.Context, e events.QuoteSent, pdfFileKey string) bool {
	if strings.TrimSpace(e.ConsumerEmail) == "" {
		return true
	}

	proposalURL := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/") + quotePublicPathPrefix + e.PublicToken
	downloadURL := m.buildPublicQuotePDFURL(e.PublicToken)
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"id": e.QuoteID.String(), "number": e.QuoteNumber, "previewUrl": proposalURL, "downloadUrl": downloadURL, "pdfFileKey": strings.TrimSpace(pdfFileKey)},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	injectQuoteSubsidyTemplateVars(templateVars, e.ISDESubsidy)
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_sent", "email", "lead", nil)

	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_sent",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email offerte verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_sent lead email workflow",
	})
}

func (m *Module) handleQuoteViewed(ctx context.Context, e events.QuoteViewed) error {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteViewed, e.QuoteID, map[string]interface{}{
		"viewerIp": e.ViewerIP,
	})
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_viewed",
		"Klant heeft de offerte geopend",
		map[string]interface{}{"viewerIp": e.ViewerIP})

	if raw, ok := m.quoteViewedDebounce.Load(e.QuoteID); ok {
		if lastSentAt, ok := raw.(time.Time); ok && time.Since(lastSentAt) < 60*time.Minute {
			m.log.Info("quote viewed in-app notification debounced", "quoteId", e.QuoteID)
			m.log.Info("quote viewed event processed", "quoteId", e.QuoteID)
			return nil
		}
	}
	m.quoteViewedDebounce.Store(e.QuoteID, time.Now())

	quoteNumber := strings.TrimSpace(e.QuoteNumber)
	if quoteNumber == "" {
		quoteNumber = "onbekend"
	}
	m.sendToAgentOrAdmins(ctx, e.OrganizationID, e.LeadID, inapp.SendParams{
		Title:        "Offerte bekeken door klant",
		Content:      fmt.Sprintf("De klant bekijkt momenteel jouw offerte %s.", quoteNumber),
		ResourceID:   &e.QuoteID,
		ResourceType: "quote",
		Category:     "info",
	})

	m.log.Info("quote viewed event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) handleQuoteUpdatedByCustomer(ctx context.Context, e events.QuoteUpdatedByCustomer) error {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteItemToggled, e.QuoteID, map[string]interface{}{
		"itemId":          e.ItemID,
		"itemDescription": e.ItemDescription,
		"isSelected":      e.IsSelected,
		"newTotalCents":   e.NewTotalCents,
	})
	action := "uitgeschakeld"
	if e.IsSelected {
		action = "ingeschakeld"
	}
	desc := e.ItemDescription
	if desc == "" {
		desc = "een item"
	}
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_item_toggled",
		"Klant heeft '"+truncate(desc, 60)+"' "+action,
		map[string]interface{}{"itemId": e.ItemID.String(), "itemDescription": e.ItemDescription, "isSelected": e.IsSelected, "newTotalCents": e.NewTotalCents})
	m.log.Info("quote item toggled event processed", "quoteId", e.QuoteID, "itemId", e.ItemID)
	return nil
}

func (m *Module) handleQuoteAnnotated(ctx context.Context, e events.QuoteAnnotated) error {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteAnnotated, e.QuoteID, map[string]interface{}{
		"itemId":     e.ItemID,
		"authorType": e.AuthorType,
		"text":       e.Text,
	})
	activityMessage := "Nieuwe vraag: \"" + truncate(e.Text, 80) + "\""
	if strings.EqualFold(e.AuthorType, "agent") {
		activityMessage = "Nieuw antwoord: \"" + truncate(e.Text, 80) + "\""
	}
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_annotated",
		activityMessage,
		map[string]interface{}{"itemId": e.ItemID.String(), "authorType": e.AuthorType, "text": e.Text})
	if strings.EqualFold(e.AuthorType, "customer") {
		_ = m.dispatchQuoteQuestionAskedPartnerEmailWorkflow(ctx, e)
		_ = m.dispatchQuoteQuestionAskedPartnerWhatsAppWorkflow(ctx, e)
	} else if strings.EqualFold(e.AuthorType, "agent") {
		_ = m.dispatchQuoteQuestionAnsweredLeadEmailWorkflow(ctx, e)
		_ = m.dispatchQuoteQuestionAnsweredLeadWhatsAppWorkflow(ctx, e)
	}
	m.log.Info("quote annotated event processed", "quoteId", e.QuoteID, "itemId", e.ItemID)
	return nil
}

func (m *Module) buildQuoteAnnotationTemplateVars(ctx context.Context, e events.QuoteAnnotated) map[string]any {
	previewURL := ""
	if strings.TrimSpace(e.PublicToken) != "" {
		previewURL = strings.TrimRight(m.cfg.GetPublicBaseURL(), "/") + quotePublicPathPrefix + strings.TrimSpace(e.PublicToken)
	}

	leadName := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	partnerName := defaultName(strings.TrimSpace(e.CreatorName), "adviseur")
	templateVars := map[string]any{
		"lead": map[string]any{
			"name":  leadName,
			"phone": e.ConsumerPhone,
			"email": e.ConsumerEmail,
		},
		"partner": map[string]any{
			"name":  partnerName,
			"phone": e.CreatorPhone,
			"email": e.CreatorEmail,
		},
		"quote": map[string]any{
			"id":         e.QuoteID.String(),
			"number":     e.QuoteNumber,
			"previewUrl": previewURL,
		},
		"annotation": map[string]any{
			"text":            e.Text,
			"authorType":      e.AuthorType,
			"itemId":          e.ItemID.String(),
			"itemDescription": e.ItemDescription,
		},
		"links": map[string]any{
			"view": previewURL,
		},
		"org": map[string]any{
			"name": defaultName(strings.TrimSpace(e.OrganizationName), defaultOrgNameFallback),
		},
	}
	enrichLeadVars(templateVars, m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID))
	return templateVars
}

func (m *Module) dispatchQuoteQuestionAskedPartnerEmailWorkflow(ctx context.Context, e events.QuoteAnnotated) bool {
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_question_asked", "email", "partner", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		PartnerEmail: e.CreatorEmail,
		Trigger:      "quote_question_asked",
		TemplateVars: m.buildQuoteAnnotationTemplateVars(ctx, e),
		Summary:      fmt.Sprintf("Email offertevraag verstuurd naar %s", defaultName(strings.TrimSpace(e.CreatorName), "adviseur")),
		FallbackNote: "failed to enqueue quote_question_asked partner email workflow",
	})
}

func (m *Module) dispatchQuoteQuestionAskedPartnerWhatsAppWorkflow(ctx context.Context, e events.QuoteAnnotated) bool {
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_question_asked", "whatsapp", "partner", nil)
	if rule == nil {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", e.OrganizationID, "trigger", "quote_question_asked", "reason", "rule_not_found")
		return false
	}
	if !rule.Enabled {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", e.OrganizationID, "trigger", "quote_question_asked", "reason", "rule_disabled")
		return true
	}
	if strings.TrimSpace(e.CreatorPhone) == "" {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", e.OrganizationID, "trigger", "quote_question_asked", "reason", "missing_phone")
		return true
	}
	templateVars := m.buildQuoteAnnotationTemplateVars(ctx, e)
	messageText, err := renderWorkflowTemplateTextWithError(rule, templateVars)
	if err != nil {
		m.log.Warn(msgWorkflowWhatsAppTemplateRenderFailed, "orgId", e.OrganizationID, "trigger", "quote_question_asked", "audience", "partner", "error", err)
		return true
	}
	messageText = normalizeWhatsAppMessage(messageText)
	if strings.TrimSpace(messageText) == "" {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", e.OrganizationID, "trigger", "quote_question_asked", "reason", "empty_message_body")
		return true
	}
	steps := []repository.WorkflowStep{{
		Enabled:      true,
		Channel:      "whatsapp",
		Audience:     "partner",
		DelayMinutes: rule.DelayMinutes,
		TemplateBody: &messageText,
		RecipientConfig: map[string]any{
			"includePartner": true,
		},
	}}
	if err := m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
		OrgID:          e.OrganizationID,
		LeadID:         nil,
		ServiceID:      e.LeadServiceID,
		PartnerPhone:   e.CreatorPhone,
		PartnerEmail:   e.CreatorEmail,
		Trigger:        "quote_question_asked",
		DefaultSummary: fmt.Sprintf("WhatsApp offertevraag verstuurd naar %s", defaultName(strings.TrimSpace(e.CreatorName), "adviseur")),
		DefaultActor:   "System",
		DefaultOrigin:  workflowEngineActorName,
		Variables:      templateVars,
	}); err != nil {
		m.log.Warn("failed to enqueue quote_question_asked partner whatsapp workflow", "error", err, "orgId", e.OrganizationID)
		return false
	}
	return true
}

func (m *Module) dispatchQuoteQuestionAnsweredLeadEmailWorkflow(ctx context.Context, e events.QuoteAnnotated) bool {
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_question_answered", "email", "lead", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_question_answered",
		TemplateVars: m.buildQuoteAnnotationTemplateVars(ctx, e),
		Summary:      fmt.Sprintf("Email antwoord op offertevraag verstuurd naar %s", defaultName(strings.TrimSpace(e.ConsumerName), "klant")),
		FallbackNote: "failed to enqueue quote_question_answered lead email workflow",
	})
}

func (m *Module) dispatchQuoteQuestionAnsweredLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteAnnotated) bool {
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_question_answered", "whatsapp", "lead", nil)
	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_question_answered",
		TemplateVars: m.buildQuoteAnnotationTemplateVars(ctx, e),
		Summary:      fmt.Sprintf("WhatsApp antwoord op offertevraag verstuurd naar %s", defaultName(strings.TrimSpace(e.ConsumerName), "klant")),
		FallbackNote: "failed to enqueue quote_question_answered lead whatsapp workflow",
	})
}

func (m *Module) handleQuoteAccepted(ctx context.Context, e events.QuoteAccepted) error {

	queued := false
	pdfFileKey := ""
	if m.quotePDFScheduler != nil {
		err := m.quotePDFScheduler.EnqueueGenerateAcceptedQuotePDFRequest(ctx, scheduler.GenerateAcceptedQuotePDFRequest{
			QuoteID:       e.QuoteID,
			TenantID:      e.OrganizationID,
			OrgName:       e.OrganizationName,
			CustomerName:  e.ConsumerName,
			SignatureName: e.SignatureName,
		})
		if err != nil {
			m.log.Warn("failed to enqueue accepted quote PDF generation", "quoteId", e.QuoteID, "error", err)
		} else {
			queued = true
		}
	}

	if !queued && m.quotePDFGen != nil {
		if fileKey, _, err := m.quotePDFGen.RegeneratePDF(ctx, e.QuoteID, e.OrganizationID); err != nil {
			m.log.Warn("failed to regenerate quote PDF on acceptance", "quoteId", e.QuoteID, "error", err)
		} else {
			pdfFileKey = strings.TrimSpace(fileKey)
		}
	}

	_ = m.dispatchQuoteAcceptedLeadEmailWorkflow(ctx, e, pdfFileKey)
	_ = m.dispatchQuoteAcceptedAgentEmailWorkflow(ctx, e)
	_ = m.dispatchQuoteAcceptedLeadWhatsAppWorkflow(ctx, e)

	quoteNumber := strings.TrimSpace(e.QuoteNumber)
	if quoteNumber == "" {
		quoteNumber = "onbekend"
	}
	m.sendToAgentOrAdmins(ctx, e.OrganizationID, e.LeadID, inapp.SendParams{
		Title:        "Offerte geaccepteerd",
		Content:      fmt.Sprintf("Geweldig! %s heeft offerte %s geaccepteerd.", defaultName(strings.TrimSpace(e.ConsumerName), "Klant"), quoteNumber),
		ResourceID:   &e.QuoteID,
		ResourceType: "quote",
		Category:     "success",
	})
	m.publishQuoteAcceptedSSE(e)
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_accepted",
		"Offerte geaccepteerd door "+e.SignatureName,
		map[string]interface{}{"signatureName": e.SignatureName, "totalCents": e.TotalCents, "consumerName": e.ConsumerName})

	m.log.Info("quote accepted event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) dispatchQuoteAcceptedLeadEmailWorkflow(ctx context.Context, e events.QuoteAccepted, pdfFileKey string) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	baseURL := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/")
	downloadURL := m.buildPublicQuotePDFURL(e.PublicToken)
	viewURL := baseURL + quotePublicPathPrefix + e.PublicToken
	formattedPrice := formatCurrencyEURCents(e.TotalCents)
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"id": e.QuoteID.String(), "number": e.QuoteNumber, "totalCents": e.TotalCents, "total": formattedPrice, "totalFormatted": formattedPrice, "downloadUrl": downloadURL, "pdfFileKey": strings.TrimSpace(pdfFileKey)},
		"links": map[string]any{"view": viewURL, "download": downloadURL, "scheduling": m.buildSchedulingLink(details)},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	injectQuoteSubsidyTemplateVars(templateVars, e.ISDESubsidy)
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "email", "lead", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_accepted",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email bevestiging offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted lead email workflow",
	})
}

func (m *Module) dispatchQuoteAcceptedAgentEmailWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.AgentName), "adviseur")
	formattedPrice := formatCurrencyEURCents(e.TotalCents)
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"partner": map[string]any{"name": name, "email": e.AgentEmail},
		"lead":    map[string]any{"name": defaultName(strings.TrimSpace(e.ConsumerName), "de klant")},
		"quote":   map[string]any{"number": e.QuoteNumber, "totalCents": e.TotalCents, "total": formattedPrice, "totalFormatted": formattedPrice},
		"links":   map[string]any{"scheduling": m.buildSchedulingLink(details)},
		"org":     map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "email", "partner", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		PartnerEmail: e.AgentEmail,
		Trigger:      "quote_accepted",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted partner email workflow",
	})
}

func (m *Module) dispatchQuoteAcceptedLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	downloadURL := m.buildPublicQuotePDFURL(e.PublicToken)
	formattedPrice := formatCurrencyEURCents(e.TotalCents)
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"number": e.QuoteNumber, "totalCents": e.TotalCents, "total": formattedPrice, "totalFormatted": formattedPrice, "downloadUrl": downloadURL},
		"links": map[string]any{"download": downloadURL, "scheduling": m.buildSchedulingLink(details)},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "whatsapp", "lead", nil)
	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_accepted",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("WhatsApp offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted lead whatsapp workflow",
	})
}

func (m *Module) publishQuoteAcceptedSSE(e events.QuoteAccepted) {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteAccepted, e.QuoteID, map[string]interface{}{
		"signatureName": e.SignatureName,
		"totalCents":    e.TotalCents,
	})

	if m.sse == nil {
		return
	}

	evt := sse.Event{
		Type:   sse.EventQuoteAccepted,
		LeadID: e.LeadID,
		Data: map[string]interface{}{
			"quoteId":   e.QuoteID,
			"status":    "Accepted",
			"signature": e.SignatureName,
		},
	}
	if e.LeadServiceID != nil {
		evt.ServiceID = *e.LeadServiceID
	}
	m.sse.PublishToLead(e.LeadID, evt)
}

func (m *Module) handleQuoteRejected(ctx context.Context, e events.QuoteRejected) error {
	_ = m.dispatchQuoteRejectedLeadEmailWorkflow(ctx, e)
	_ = m.dispatchQuoteRejectedLeadWhatsAppWorkflow(ctx, e)
	quoteNumber := strings.TrimSpace(e.QuoteNumber)
	if quoteNumber == "" {
		quoteNumber = "onbekend"
	}
	m.sendToAgentOrAdmins(ctx, e.OrganizationID, e.LeadID, inapp.SendParams{
		Title:        "Offerte afgewezen",
		Content:      fmt.Sprintf("Offerte %s is afgewezen door %s.", quoteNumber, defaultName(strings.TrimSpace(e.ConsumerName), "Klant")),
		ResourceID:   &e.QuoteID,
		ResourceType: "quote",
		Category:     "warning",
	})

	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteRejected, e.QuoteID, map[string]interface{}{
		"reason": e.Reason,
	})

	if m.sse != nil {
		evt := sse.Event{
			Type:   sse.EventQuoteRejected,
			LeadID: e.LeadID,
			Data: map[string]interface{}{
				"quoteId": e.QuoteID,
				"status":  "Rejected",
				"reason":  e.Reason,
			},
		}
		if e.LeadServiceID != nil {
			evt.ServiceID = *e.LeadServiceID
		}
		m.sse.PublishToLead(e.LeadID, evt)
	}
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_rejected",
		"Offerte afgewezen door klant",
		map[string]interface{}{"reason": e.Reason})
	m.log.Info("quote rejected event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) dispatchQuoteRejectedLeadEmailWorkflow(ctx context.Context, e events.QuoteRejected) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead": map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{
			"reason": e.Reason,
		},
		"org": map[string]any{"name": defaultName(strings.TrimSpace(e.OrganizationName), defaultOrgNameFallback)},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_rejected", "email", "lead", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_rejected",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email offerteafwijzing bevestigd naar %s", name),
		FallbackNote: "failed to enqueue quote_rejected lead email workflow",
	})
}

func (m *Module) dispatchQuoteRejectedLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteRejected) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead": map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{
			"reason": e.Reason,
		},
		"org": map[string]any{"name": defaultName(strings.TrimSpace(e.OrganizationName), defaultOrgNameFallback)},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_rejected", "whatsapp", "lead", nil)
	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_rejected",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("WhatsApp offerteafwijzing bevestigd naar %s", name),
		FallbackNote: "failed to enqueue quote_rejected lead whatsapp workflow",
	})
}

// pushQuoteSSE broadcasts a quote event to all connected agents in the org via SSE.
func (m *Module) pushQuoteSSE(orgID uuid.UUID, eventType sse.EventType, quoteID uuid.UUID, data interface{}) {
	if m.sse == nil {
		return
	}
	m.sse.PublishQuoteEvent(orgID, eventType, quoteID, data)
}

// logQuoteActivity persists an activity record for a quote. Failures are logged but non-fatal.
func (m *Module) logQuoteActivity(ctx context.Context, quoteID, orgID uuid.UUID, eventType, message string, metadata map[string]interface{}) {
	if m.actWriter == nil {
		return
	}
	if err := m.actWriter.CreateActivity(ctx, quoteID, orgID, eventType, message, metadata); err != nil {
		m.log.Error("failed to persist quote activity",
			"quoteId", quoteID,
			"eventType", eventType,
			"error", err,
		)
	}
}

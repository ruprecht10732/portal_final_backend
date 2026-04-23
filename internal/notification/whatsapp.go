package notification

import (
	"context"
	"errors"
	"fmt"
	htmlstd "html"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/phone"
	"strings"
	"time"

	htmlnode "golang.org/x/net/html"

	"github.com/google/uuid"
)

// WhatsAppSender sends WhatsApp messages.
type WhatsAppSender interface {
	SendMessage(ctx context.Context, deviceID string, phoneNumber string, message string) (whatsapp.SendResult, error)
}

type WhatsAppInboxWriter interface {
	PersistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string) error
}

func (m *Module) SendLeadWhatsApp(ctx context.Context, params SendLeadWhatsAppParams) error {
	if m.whatsapp == nil {
		return apperr.Internal("WhatsApp is niet geconfigureerd")
	}

	phoneNumber := strings.TrimSpace(phone.NormalizeE164(params.PhoneNumber))
	if phoneNumber == "" {
		return apperr.Validation("telefoonnummer voor WhatsApp ontbreekt")
	}

	message := strings.TrimSpace(params.Message)
	if message == "" {
		return apperr.Validation("WhatsApp-bericht is leeg")
	}

	deviceID := strings.TrimSpace(m.resolveWhatsAppDeviceID(ctx, params.OrgID))
	if deviceID == "" {
		return apperr.Validation("er is geen verbonden WhatsApp-apparaat voor deze organisatie")
	}

	result, err := m.whatsapp.SendMessage(ctx, deviceID, phoneNumber, message)
	if err != nil {
		m.log.Warn("failed to send explicit timeline whatsapp", "error", err, "orgId", params.OrgID, "leadId", params.LeadID)
		if params.LeadID != uuid.Nil {
			m.writeWhatsAppFailureEvent(ctx, params.LeadID, params.ServiceID, params.OrgID, err.Error())
		}
		if errors.Is(err, whatsapp.ErrNoDevice) {
			return apperr.Validation("er is geen verbonden WhatsApp-apparaat voor deze organisatie")
		}
		return apperr.Internal("WhatsApp-bericht kon niet worden verstuurd")
	}

	metadata := buildMergedWhatsAppSentMetadata(params.Metadata, params.Category, params.Audience, phoneNumber, message)
	m.writeWhatsAppSentEventWithMetadata(whatsAppSentEventWithMetadataParams{
		Ctx:       ctx,
		LeadID:    params.LeadID,
		ServiceID: params.ServiceID,
		OrgID:     params.OrgID,
		ActorType: params.ActorType,
		ActorName: params.ActorName,
		Summary:   params.Summary,
		Metadata:  metadata,
	})
	if m.whatsAppInboxWriter != nil {
		if persistErr := m.whatsAppInboxWriter.PersistOutgoingWhatsAppMessage(ctx, params.OrgID, nilIfUUIDNil(params.LeadID), phoneNumber, message, nilIfEmptyString(result.MessageID)); persistErr != nil {
			m.log.Warn("failed to persist whatsapp inbox message", "error", persistErr, "orgId", params.OrgID, "leadId", params.LeadID)
		}
	}

	return nil
}

type whatsAppSendOutboxPayload struct {
	OrgID       string         `json:"orgId"`
	LeadID      *string        `json:"leadId,omitempty"`
	ServiceID   *string        `json:"serviceId,omitempty"`
	PhoneNumber string         `json:"phoneNumber"`
	Message     string         `json:"message"`
	Category    string         `json:"category"`
	Audience    string         `json:"audience"`
	Summary     string         `json:"summary"`
	ActorType   string         `json:"actorType"`
	ActorName   string         `json:"actorName"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func normalizeWhatsAppMessage(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}

	text = htmlToWhatsAppMarkdown(text)
	text = htmlstd.UnescapeString(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")

	var b strings.Builder
	b.Grow(len(text))

	writeWhatsAppNormalizedBytes(&b, text)

	return strings.TrimSpace(b.String())
}

func writeWhatsAppNormalizedBytes(b *strings.Builder, text string) {
	blankCount := 0
	inWord := false
	lineEmpty := true

	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\n' {
			blankCount, lineEmpty, inWord = handleWhatsAppNewline(b, blankCount, lineEmpty)
			continue
		}

		if c == ' ' || c == '\t' || c == '\r' {
			inWord = false
			continue
		}

		if !inWord && !lineEmpty {
			b.WriteByte(' ')
		}
		b.WriteByte(c)
		inWord = true
		lineEmpty = false
	}
}

func handleWhatsAppNewline(b *strings.Builder, blankCount int, lineEmpty bool) (int, bool, bool) {
	if lineEmpty {
		blankCount++
		if blankCount <= 1 && b.Len() > 0 {
			b.WriteByte('\n')
		}
	} else {
		blankCount = 0
		b.WriteByte('\n')
	}
	return blankCount, true, false
}

func htmlToWhatsAppMarkdown(input string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return ""
	}

	doc, err := htmlnode.Parse(strings.NewReader(raw))
	if err != nil {
		return htmlstd.UnescapeString(raw)
	}

	var b strings.Builder
	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		appendWhatsAppNode(&b, child)
	}

	return b.String()
}

func appendWhatsAppNode(b *strings.Builder, node *htmlnode.Node) {
	if node == nil {
		return
	}

	switch node.Type {
	case htmlnode.TextNode:
		b.WriteString(htmlstd.UnescapeString(node.Data))
	case htmlnode.ElementNode:
		tag := strings.ToLower(node.Data)
		switch tag {
		case "br":
			b.WriteString("\n")
		case "strong", "b":
			appendWrappedWhatsAppNode(b, node, "*")
		case "em", "i":
			appendWrappedWhatsAppNode(b, node, "_")
		case "del", "s", "strike":
			appendWrappedWhatsAppNode(b, node, "~")
		case "p", "div", "li":
			appendWhatsAppChildren(b, node)
			b.WriteString("\n")
		default:
			appendWhatsAppChildren(b, node)
		}
	default:
		appendWhatsAppChildren(b, node)
	}
}

func appendWhatsAppChildren(b *strings.Builder, node *htmlnode.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendWhatsAppNode(b, child)
	}
}

func appendWrappedWhatsAppNode(b *strings.Builder, node *htmlnode.Node, marker string) {
	var inner strings.Builder
	appendWhatsAppChildren(&inner, node)
	content := inner.String()
	if strings.TrimSpace(content) == "" {
		b.WriteString(content)
		return
	}
	b.WriteString(marker)
	b.WriteString(content)
	b.WriteString(marker)
}

type whatsAppBestEffortParams struct {
	Ctx         context.Context
	OrgID       uuid.UUID
	LeadID      *uuid.UUID
	ServiceID   *uuid.UUID
	PhoneNumber string
	Message     string
	Category    string
	Audience    string
	Summary     string
	ActorType   string
	ActorName   string
	Metadata    map[string]any
}

func (m *Module) sendWhatsAppBestEffort(params whatsAppBestEffortParams) error {
	if m.whatsapp == nil || params.PhoneNumber == "" {
		return nil
	}

	deviceID := m.resolveWhatsAppDeviceID(params.Ctx, params.OrgID)
	result, err := m.whatsapp.SendMessage(params.Ctx, deviceID, params.PhoneNumber, params.Message)
	if err != nil {
		if errors.Is(err, whatsapp.ErrNoDevice) {
			m.log.Debug("whatsapp skipped: no device configured", "orgId", params.OrgID)
			return nil
		}

		m.log.Warn("failed to send whatsapp", "error", err, "orgId", params.OrgID)
		if params.LeadID != nil {
			m.writeWhatsAppFailureEvent(params.Ctx, *params.LeadID, params.ServiceID, params.OrgID, err.Error())
		}
		return err
	}

	if params.LeadID == nil {
		return nil
	}

	metadata := params.Metadata
	if metadata == nil {
		metadata = buildWhatsAppSentMetadata(params.Category, params.Audience, params.PhoneNumber, params.Message)
	}

	m.writeWhatsAppSentEventWithMetadata(whatsAppSentEventWithMetadataParams{
		Ctx:       params.Ctx,
		LeadID:    *params.LeadID,
		ServiceID: params.ServiceID,
		OrgID:     params.OrgID,
		ActorType: params.ActorType,
		ActorName: params.ActorName,
		Summary:   params.Summary,
		Metadata:  metadata,
	})
	if m.whatsAppInboxWriter != nil {
		if persistErr := m.whatsAppInboxWriter.PersistOutgoingWhatsAppMessage(params.Ctx, params.OrgID, params.LeadID, params.PhoneNumber, params.Message, nilIfEmptyString(result.MessageID)); persistErr != nil {
			m.log.Warn("failed to persist workflow whatsapp inbox message", "error", persistErr, "orgId", params.OrgID, "leadId", params.LeadID)
		}
	}

	return nil
}

func (m *Module) resolveWhatsAppDeviceID(ctx context.Context, orgID uuid.UUID) string {
	if m.settingsReader == nil {
		return ""
	}

	settings, err := m.settingsReader.GetOrganizationSettings(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to fetch org settings for whatsapp", "error", err, "orgId", orgID)
		return ""
	}
	if settings.WhatsAppDeviceID == nil {
		return ""
	}
	return *settings.WhatsAppDeviceID
}

func (m *Module) writeWhatsAppFailureEvent(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, orgID uuid.UUID, errorMsg string) {
	if m.leadTimeline == nil {
		return
	}

	friendlyError := "Verzenden mislukt"
	msgLower := strings.ToLower(errorMsg)
	if strings.Contains(msgLower, "disconnected") || strings.Contains(msgLower, "not connected") {
		friendlyError = "Telefoon niet verbonden"
	}

	summary := fmt.Sprintf("WhatsApp niet verstuurd: %s", friendlyError)
	_ = m.leadTimeline.CreateTimelineEvent(ctx, LeadTimelineEventParams{
		LeadID:    leadID,
		ServiceID: serviceID,
		OrgID:     orgID,
		ActorType: "System",
		ActorName: "WhatsApp",
		EventType: "whatsapp_failed",
		Title:     "WhatsApp fout",
		Summary:   &summary,
		Metadata: map[string]any{
			"raw_error": errorMsg,
		},
		Visibility: "internal",
	})
}

func buildWhatsAppSentMetadata(category, audience, phoneNumber, message string) map[string]any {
	return map[string]any{
		"status":          "sent",
		"messageCategory": category,
		"messageAudience": audience,
		"messageLanguage": "nl",
		"phoneNumber":     phone.NormalizeE164(phoneNumber),
		"messageContent":  message,
		"sentAt":          time.Now().UTC().Format(time.RFC3339),
	}
}

func buildMergedWhatsAppSentMetadata(base map[string]any, category, audience, phoneNumber, message string) map[string]any {
	merged := make(map[string]any, len(base)+8)
	for key, value := range base {
		merged[key] = value
	}
	defaults := buildWhatsAppSentMetadata(category, audience, phoneNumber, message)
	for key, value := range defaults {
		merged[key] = value
	}
	return merged
}

type whatsAppSentEventWithMetadataParams struct {
	Ctx       context.Context
	LeadID    uuid.UUID
	ServiceID *uuid.UUID
	OrgID     uuid.UUID
	ActorType string
	ActorName string
	Summary   string
	Metadata  map[string]any
}

func (m *Module) writeWhatsAppSentEventWithMetadata(params whatsAppSentEventWithMetadataParams) {
	if m.leadTimeline == nil {
		return
	}

	if err := m.leadTimeline.CreateTimelineEvent(params.Ctx, LeadTimelineEventParams{
		LeadID:     params.LeadID,
		ServiceID:  params.ServiceID,
		OrgID:      params.OrgID,
		ActorType:  params.ActorType,
		ActorName:  params.ActorName,
		EventType:  "whatsapp_sent",
		Title:      "WhatsApp verstuurd",
		Summary:    &params.Summary,
		Metadata:   params.Metadata,
		Visibility: "internal",
	}); err != nil {
		m.log.Error("failed to write whatsapp timeline event", "error", err, "leadId", params.LeadID)
	}
}

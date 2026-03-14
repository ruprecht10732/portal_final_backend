package waagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

var allowedWriteTools = map[string]struct{}{
	"CreateLead":                 {},
	"DraftQuote":                 {},
	"GenerateQuote":              {},
	"SendQuotePDF":               {},
	"UpdateLeadDetails":          {},
	"AskCustomerClarification":   {},
	"SaveNote":                   {},
	"UpdateStatus":               {},
	"ScheduleVisit":              {},
	"RescheduleVisit":            {},
	"CancelVisit":                {},
	"AttachCurrentWhatsAppPhoto": {},
}

func (a *Agent) planWriteAction(ctx context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *ConversationLeadHint, inboundMessage *CurrentInboundMessage) (writeExecutionPlan, string, error) {
	prompt := buildWritePlanningPrompt(messages, leadHint, inboundMessage)
	raw, err := a.runPlainPrompt(ctx, a.plannerRuntime, orgID, phoneKey, prompt)
	if err != nil {
		return writeExecutionPlan{}, "", err
	}
	plan, err := parseWriteExecutionPlan(raw)
	if err != nil {
		a.logWarn(ctx, "waagent: write plan parse failed", "error", err)
		return writeExecutionPlan{}, fallbackReplyForWritePlanIssue("invalid_plan", nil), nil
	}
	if reply := validateWriteExecutionPlan(plan); reply != "" {
		return writeExecutionPlan{}, reply, nil
	}
	return plan, "", nil
}

func (a *Agent) runPlainPrompt(ctx context.Context, runtime agentRuntime, orgID uuid.UUID, phoneKey, prompt string) (string, error) {
	userID := "waagent-" + orgID.String()
	sessionID := uuid.New().String()
	_, err := a.sessionService.Create(ctx, &session.CreateRequest{AppName: runtime.appName, UserID: userID, SessionID: sessionID})
	if err != nil {
		return "", fmt.Errorf("waagent: create plain session: %w", err)
	}
	defer func() {
		_ = a.sessionService.Delete(ctx, &session.DeleteRequest{AppName: runtime.appName, UserID: userID, SessionID: sessionID})
	}()
	userMessage := &genai.Content{Role: "user", Parts: []*genai.Part{{Text: strings.TrimSpace(prompt)}}}
	return a.collectPlainText(ctx, runtime, userID, sessionID, userMessage)
}

func (a *Agent) collectPlainText(ctx context.Context, runtime agentRuntime, userID, sessionID string, userMessage *genai.Content) (string, error) {
	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	var lastFinalText string
	iterations := 0
	for event, err := range runtime.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		if err != nil {
			return "", fmt.Errorf("waagent: plain run failed: %w", err)
		}
		if event.IsFinalResponse() {
			if text := extractContentText(event.Content); text != "" {
				lastFinalText = text
			}
		}
		iterations++
		if iterations >= maxToolIterations {
			break
		}
	}
	return strings.TrimSpace(lastFinalText), nil
}

func buildWritePlanningPrompt(messages []ConversationMessage, leadHint *ConversationLeadHint, inboundMessage *CurrentInboundMessage) string {
	latest := messages[len(messages)-1].Content
	var b strings.Builder
	b.WriteString("Laatste klantvraag:\n")
	b.WriteString(strings.TrimSpace(latest))
	if len(messages) > 1 {
		b.WriteString("\n\nRecente context:\n")
		start := len(messages) - 4
		if start < 0 {
			start = 0
		}
		for _, msg := range messages[start : len(messages)-1] {
			b.WriteString(msg.Role)
			b.WriteString(": ")
			b.WriteString(strings.TrimSpace(msg.Content))
			b.WriteString("\n")
		}
	}
	if hasConversationRoutingContext(leadHint) {
		b.WriteString("\nLeadhint:\n")
		b.WriteString(strings.TrimSpace((&Agent{}).buildLeadContextText(leadHint)))
	}
	if inboundMessage != nil && strings.TrimSpace(inboundMessage.ExternalMessageID) != "" {
		b.WriteString("\n\nEr is een actuele inbound WhatsApp-boodschap beschikbaar voor eventuele media-acties.")
	}
	return b.String()
}

func parseWriteExecutionPlan(raw string) (writeExecutionPlan, error) {
	var plan writeExecutionPlan
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &plan); err != nil {
		return writeExecutionPlan{}, err
	}
	plan.ToolName = strings.TrimSpace(plan.ToolName)
	plan.Goal = strings.TrimSpace(plan.Goal)
	plan.Reason = strings.TrimSpace(plan.Reason)
	filtered := plan.MissingInformation[:0]
	for _, item := range plan.MissingInformation {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	plan.MissingInformation = filtered
	return plan, nil
}

func validateWriteExecutionPlan(plan writeExecutionPlan) string {
	if !plan.SafeToExecute {
		return fallbackReplyForWritePlanIssue("missing_information", plan.MissingInformation)
	}
	if _, ok := allowedWriteTools[plan.ToolName]; !ok || plan.ToolName == "" {
		return fallbackReplyForWritePlanIssue("invalid_tool", nil)
	}
	return ""
}

func fallbackReplyForWritePlanIssue(issue string, missing []string) string {
	switch issue {
	case "missing_information":
		if len(missing) > 0 {
			return fmt.Sprintf("Ik mis nog %s om dit veilig uit te voeren. Kunt u dat eerst sturen?", strings.ToLower(missing[0]))
		}
		return "Ik mis nog een belangrijk detail om dit veilig uit te voeren. Kunt u dat eerst sturen?"
	default:
		return "Ik kan dit nog niet veilig uitvoeren. Kunt u kort aangeven wat u precies wilt laten aanpassen of plannen?"
	}
}

func buildWriteExecutionDirective(plan writeExecutionPlan) string {
	return fmt.Sprintf("Gevalideerd uitvoeringsplan voor deze beurt: einddoel is %s. Doel: %s. Gebruik alleen noodzakelijke leestools als voorbereiding en voer daarna uitsluitend deze schrijfactie uit als slotactie.", plan.ToolName, plan.Goal)
}

func shouldRetryLookupRun(messages []ConversationMessage, leadHint *ConversationLeadHint, result AgentRunResult) bool {
	if strings.TrimSpace(result.Reply) == "" || result.GroundingFailure != "" {
		return true
	}
	return lookupReplyNeedsRepair(messages, leadHint, result.Reply)
}

func buildLookupRepairDirective(messages []ConversationMessage, leadHint *ConversationLeadHint, result AgentRunResult) string {
	if lookupReplyNeedsRepair(messages, leadHint, result.Reply) {
		return "De klant vroeg al om een concreet detail of bevestigde dat je dat detail mag ophalen. Stel geen extra bevestigingsvraag en geef direct het gevraagde resultaat op basis van de lookuptools."
	}
	if result.GroundingFailure != "" {
		return fmt.Sprintf("Vorige poging faalde op %s. Antwoord opnieuw, kort en direct, en gebruik alleen verifieerbare feiten uit de beschikbare lookuptools.", result.GroundingFailure)
	}
	return "Vorige poging leverde geen bruikbaar antwoord op. Antwoord opnieuw, kort en direct, zonder procesnarratie."
}

func lookupReplyNeedsRepair(messages []ConversationMessage, leadHint *ConversationLeadHint, reply string) bool {
	if len(messages) == 0 {
		return false
	}
	latest := strings.TrimSpace(messages[len(messages)-1].Content)
	intent := baseSimpleLookupIntent(latest)
	if intent.kind == "" && looksLikeSimpleCustomerReference(latest) {
		intent = inferLookupIntentFromConversation(messages)
		intent.subject = latest
	}
	if intent.kind == "" && isAffirmativeText(latest) && leadHint != nil && strings.TrimSpace(leadHint.LeadID) != "" {
		intent = inferLookupIntentFromConversation(messages)
		intent.useLeadHint = true
	}
	if intent.kind == "" {
		return false
	}
	if !intent.wantsAddress && !intent.wantsContact && !intent.wantsStatus && !intent.wantsQuote {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(reply))
	return strings.Contains(lower, "wil je dat ik") || strings.Contains(lower, "zal ik") || strings.Contains(lower, "kan ik de volledige contactgegevens") || strings.Contains(lower, "kan ik de gegevens")
}

func inferLookupIntentFromConversation(messages []ConversationMessage) simpleLookupIntent {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].Role) != "user" {
			continue
		}
		intent := baseSimpleLookupIntent(messages[i].Content)
		if intent.kind != "" {
			return intent
		}
	}
	return simpleLookupIntent{}
}

func preferLookupRetryResult(current AgentRunResult, retry AgentRunResult) AgentRunResult {
	if current.GroundingFailure != "" && retry.GroundingFailure == "" && strings.TrimSpace(retry.Reply) != "" {
		return retry
	}
	if strings.TrimSpace(current.Reply) == "" && strings.TrimSpace(retry.Reply) != "" {
		return retry
	}
	return current
}

func (a *Agent) applyFinalEditor(ctx context.Context, latestUserMessage string, mode agentRunMode, result AgentRunResult) AgentRunResult {
	if strings.TrimSpace(result.Reply) == "" {
		return result
	}
	prompt := buildReplyEditorPrompt(mode, latestUserMessage, result)
	raw, err := a.runPlainPrompt(ctx, a.editorRuntime, uuid.Nil, "", prompt)
	if err != nil {
		a.logWarn(ctx, "waagent: reply editor failed", "error", err)
		return result
	}
	decision, err := parseReplyEditorDecision(raw)
	if err != nil {
		a.logWarn(ctx, "waagent: reply editor parse failed", "error", err)
		return result
	}
	if decision.Approved {
		return result
	}
	result.Reply = fallbackReplyForEditorIssue(mode, decision.Issue, result.Reply)
	return result
}

func buildReplyEditorPrompt(mode agentRunMode, latestUserMessage string, result AgentRunResult) string {
	return strings.TrimSpace(fmt.Sprintf("Modus: %s\nGebruikersvraag: %s\nConceptantwoord: %s\nGroundingFailure: %s\nToolResponses: %s", mode, strings.TrimSpace(latestUserMessage), strings.TrimSpace(result.Reply), strings.TrimSpace(result.GroundingFailure), strings.Join(result.ToolResponseNames, ", ")))
}

func parseReplyEditorDecision(raw string) (replyEditorDecision, error) {
	var decision replyEditorDecision
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &decision); err != nil {
		return replyEditorDecision{}, err
	}
	decision.Issue = strings.TrimSpace(decision.Issue)
	return decision, nil
}

func fallbackReplyForEditorIssue(mode agentRunMode, issue string, original string) string {
	switch mode {
	case agentRunModeLookup:
		return lookupModeFallback
	case agentRunModeWrite:
		return "Ik mis nog een belangrijk detail om dit veilig af te ronden. Kunt u kort aangeven wat u precies wilt laten aanpassen of plannen?"
	default:
		if issue == "process_narration" {
			return strings.TrimSpace(original)
		}
		return "Kunt u kort aangeven welke klant, offerte of afspraak u bedoelt?"
	}
}

func extractJSONObject(raw string) string {
	trimmed := strings.TrimSpace(raw)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return trimmed[start : end+1]
	}
	return trimmed
}

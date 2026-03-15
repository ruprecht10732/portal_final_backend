package waagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/internal/scheduler"
	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Hardcoded Dutch messages — zero LLM cost for rate limiting and unknown users.
const (
	msgRateLimited       = "Je stuurt te veel berichten. Probeer het over een paar minuten opnieuw."
	msgUnknownPhone      = "Je telefoonnummer is niet gekoppeld aan een organisatie. Neem contact op met je beheerder."
	msgVoiceUnavailable  = "Ik kon je spraakbericht niet verwerken. Stuur je vraag als tekst, dan help ik je meteen."
	msgSystemUnavailable = "Ons systeem is tijdelijk niet beschikbaar. Probeer het later opnieuw."
	msgConversationReset = "Ik heb ons gesprek gewist. We beginnen opnieuw met een schone context."
	msgGroundingFallback = "Ik kan deze informatie op dit moment niet betrouwbaar bevestigen. Bekijk de actuele gegevens in het portaal."
	recentMessageLimit   = 100
	logAudioFallback     = "waagent: audio fallback"
)

// Service orchestrates the waagent flow: rate limit → phone→org → AI.
type Service struct {
	queries                waagentdb.Querier
	agent                  agentRunner
	sender                 *Sender
	rateLimiter            *RateLimiter
	leadHintStore          LeadHintStore
	leadDetailsReader      LeadDetailsReader
	storage                ObjectStorage
	attachmentBucket       string
	transcriptionScheduler AudioTranscriptionScheduler
	transcriber            AudioTranscriber
	inboxSync              InboxMessageSync
	log                    *logger.Logger
}

type agentRunner interface {
	Run(ctx context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *ConversationLeadHint, inboundMessage *CurrentInboundMessage, mode agentRunMode) (AgentRunResult, error)
}

type voiceTranscriptionContext struct {
	orgID             uuid.UUID
	phoneNumber       string
	phoneKey          string
	externalMessageID string
}

type storedVoiceNote struct {
	contentType string
	fileName    string
	fileKey     string
	data        []byte
}

// HandleIncomingMessage processes an incoming WhatsApp message from the global agent device.
// It resolves the organization from the sender's phone number.
// It is designed to be called in a goroutine with context.WithoutCancel.
func (s *Service) HandleIncomingMessage(ctx context.Context, inbound CurrentInboundMessage) {
	externalMessageID := strings.TrimSpace(inbound.ExternalMessageID)
	phone := inbound.PhoneNumber
	text := inbound.Body
	replyTarget := strings.TrimSpace(phone)
	lookupPhone := normalizeAgentPhoneKey(phone)
	if replyTarget == "" {
		replyTarget = lookupPhone
	}
	if lookupPhone == "" {
		lookupPhone = replyTarget
	}

	claimed, _ := s.claimInboundMessage(ctx, externalMessageID, lookupPhone)
	if !claimed {
		return
	}

	// Step 1: Rate limit check
	allowed := s.allowInboundMessage(ctx, lookupPhone, externalMessageID)
	if !allowed {
		s.sendHardcoded(ctx, uuid.Nil, lookupPhone, replyTarget, text, msgRateLimited)
		return
	}

	// Step 2: Phone → org lookup
	user, err := s.lookupAgentUser(ctx, phone)
	if err != nil {
		s.sendHardcoded(ctx, uuid.Nil, lookupPhone, replyTarget, text, msgUnknownPhone)
		return
	}

	orgID := uuidFromPgtype(user.OrganizationID)

	if shouldResetConversation(text) {
		s.resetConversation(ctx, orgID, lookupPhone, replyTarget)
		return
	}

	if isAudioInboundMessage(inbound) {
		s.handleIncomingAudioMessage(ctx, orgID, lookupPhone, replyTarget, inbound)
		return
	}

	s.handleAIMessage(ctx, orgID, lookupPhone, replyTarget, text, &inbound, user)
}

func (s *Service) handleAIMessage(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, text string, inbound *CurrentInboundMessage, agentUser waagentdb.GetAgentUserByPhoneRow) {
	if err := s.persistInboundMessage(ctx, orgID, phoneKey, text, inbound); err != nil {
		s.logError(ctx, "waagent: failed to persist user message", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
	}
	s.syncInboundInboxMessage(ctx, orgID, replyTarget, inbound)
	decision := s.selectAgentRunMode(ctx, orgID, phoneKey, text)
	if partnerID, ok := partnerIDFromAgentUser(agentUser); ok {
		decision.mode = agentRunModePartner
		decision.reason = "partner"
		decision.partnerID = &partnerID
	}
	s.logInfo(ctx, "waagent: agent mode selected",
		"phone", phoneKey,
		"organization_id", orgID.String(),
		"mode", string(decision.mode),
		"reason", decision.reason)
	s.runAgentReply(ctx, orgID, phoneKey, replyTarget, inbound, decision)
}

func (s *Service) handleIncomingAudioMessage(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string, inbound CurrentInboundMessage) {
	if strings.TrimSpace(inbound.ExternalMessageID) == "" {
		s.logWarn(ctx, logAudioFallback, "reason", "missing_external_id", "phone", phoneKey, "organization_id", orgID.String())
		s.sendAudioFallback(ctx, orgID, phoneKey, replyTarget, inbound)
		return
	}

	inbound.Body = voiceMessagePlaceholder
	inbound.Metadata = mergeVoiceTranscriptionMetadata(inbound.Metadata, voiceTranscriptionUpdate{Status: "pending"})
	if err := s.persistInboundMessage(ctx, orgID, phoneKey, inbound.Body, &inbound); err != nil {
		s.logError(ctx, "waagent: failed to persist pending audio message", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
	}
	s.syncInboundInboxMessage(ctx, orgID, replyTarget, &inbound)
	if err := s.queries.UpsertAgentVoiceTranscription(ctx, waagentdb.UpsertAgentVoiceTranscriptionParams{
		OrganizationID:    pgtypeUUID(orgID),
		ExternalMessageID: strings.TrimSpace(inbound.ExternalMessageID),
		PhoneNumber:       phoneKey,
		Status:            "pending",
		Provider:          optionalPgText(audioProviderName(s.transcriber)),
		ErrorMessage:      pgtype.Text{},
	}); err != nil {
		s.logError(ctx, "waagent: failed to create pending audio transcription row", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
	}
	if s.transcriptionScheduler == nil || s.transcriber == nil || s.storage == nil || strings.TrimSpace(s.attachmentBucket) == "" {
		s.logWarn(ctx, logAudioFallback,
			"reason", "deps_nil",
			"phone", phoneKey,
			"organization_id", orgID.String(),
			"has_scheduler", s.transcriptionScheduler != nil,
			"has_transcriber", s.transcriber != nil,
			"has_storage", s.storage != nil,
			"attachment_bucket", s.attachmentBucket)
		s.failVoiceTranscription(ctx, orgID, inbound.ExternalMessageID, "audio transcription is not configured", false)
		s.sendAudioFallback(ctx, orgID, phoneKey, replyTarget, inbound)
		return
	}
	if err := s.transcriptionScheduler.EnqueueWAAgentVoiceTranscription(ctx, scheduler.WAAgentVoiceTranscriptionPayload{
		OrganizationID:    orgID.String(),
		PhoneNumber:       replyTarget,
		ExternalMessageID: inbound.ExternalMessageID,
		RequestID:         stringContextValue(ctx, logger.RequestIDKey),
		TraceID:           stringContextValue(ctx, logger.TraceIDKey),
	}); err != nil {
		s.logWarn(ctx, logAudioFallback, "reason", "enqueue_failed", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		s.failVoiceTranscription(ctx, orgID, inbound.ExternalMessageID, err.Error(), false)
		s.sendAudioFallback(ctx, orgID, phoneKey, replyTarget, inbound)
	}
}

func (s *Service) runAgentReply(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string, inbound *CurrentInboundMessage, decision agentModeDecision) {

	// Load recent conversation history
	recent, err := s.queries.GetRecentAgentMessages(ctx, waagentdb.GetRecentAgentMessagesParams{
		PhoneNumber: phoneKey,
		Limit:       recentMessageLimit,
	})
	if err != nil {
		s.logWarn(ctx, "waagent: failed to load history", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		recent = nil
	}

	messages := make([]ConversationMessage, 0, len(recent))
	for i := len(recent) - 1; i >= 0; i-- {
		msg := recent[i]
		if shouldSkipReplayedMessage(msg.Role, msg.Content) {
			continue
		}
		if msg.Role == "user" && strings.TrimSpace(msg.Content) == voiceMessagePlaceholder {
			continue
		}
		messages = append(messages, ConversationMessage{Role: msg.Role, Content: msg.Content, SentAt: timestamptzPtr(msg.CreatedAt)})
	}

	if err := s.sender.SendChatPresence(ctx, replyTarget, whatsapp.ChatPresenceComposing); err != nil {
		s.logWarn(ctx, "waagent: send chat presence start error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
	}
	defer func() {
		if err := s.sender.SendChatPresence(context.Background(), replyTarget, whatsapp.ChatPresencePaused); err != nil {
			s.logWarn(context.Background(), "waagent: send chat presence stop error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
		}
	}()

	leadHint := s.resolveLeadHint(ctx, orgID, phoneKey)
	if decision.partnerID != nil {
		ctx = context.WithValue(ctx, partnerIDContextKey{}, *decision.partnerID)
	}

	runResult, err := s.agent.Run(ctx, orgID, phoneKey, messages, leadHint, inbound, decision.mode)
	if err != nil {
		s.logError(ctx, "waagent: agent run error", "phone", phoneKey, "organization_id", orgID.String(), "mode", string(decision.mode), "reason", decision.reason, "error", err)
		s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgSystemUnavailable)
		return
	}
	if runResult.GroundingFailure != "" {
		s.logWarn(ctx, "waagent: grounding failure blocked reply",
			"phone", phoneKey,
			"organization_id", orgID.String(),
			"mode", string(decision.mode),
			"decision_reason", decision.reason,
			"grounding_reason", runResult.GroundingFailure,
			"unsupported_facts", runResult.GroundingFacts,
			"tool_response_names", runResult.ToolResponseNames,
			"tool_response_count", runResult.ToolResponseCount)
		s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgGroundingFallback)
		return
	}

	reply := sanitizeWhatsAppReply(strings.TrimSpace(runResult.Reply))
	reply = s.applyReplyPolicy(ctx, orgID, phoneKey, decision, reply)
	if reply == "" {
		s.logWarn(ctx, "waagent: empty agent reply", "phone", phoneKey, "organization_id", orgID.String(), "mode", string(decision.mode), "reason", decision.reason)
		return
	}

	s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, reply)
}

func shouldResetConversation(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return normalized == "/clear" || normalized == "/reset"
}

func shouldSkipReplayedMessage(role, content string) bool {
	trimmed := strings.TrimSpace(content)
	if role == "assistant" {
		if trimmed == msgVoiceUnavailable || trimmed == msgSystemUnavailable || trimmed == msgGroundingFallback {
			return true
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "ik kan die klantgegevens nu niet betrouwbaar bevestigen") {
			return true
		}
	}
	return false
}

func (s *Service) resetConversation(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string) {
	if err := s.queries.DeleteAgentMessagesByPhone(ctx, waagentdb.DeleteAgentMessagesByPhoneParams{
		OrganizationID: pgtypeUUID(orgID),
		PhoneNumber:    phoneKey,
	}); err != nil {
		s.logError(ctx, "waagent: failed to reset conversation history", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgSystemUnavailable)
		return
	}
	if s.leadHintStore != nil {
		s.leadHintStore.Clear(orgID.String(), phoneKey)
	}
	s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgConversationReset)
}

// sendHardcoded persists messages and sends a hardcoded reply without invoking the LLM.
func (s *Service) sendHardcoded(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, incomingText, reply string) {
	if orgID != uuid.Nil {
		_ = s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
			OrganizationID:    pgtypeUUID(orgID),
			PhoneNumber:       phoneKey,
			Role:              "user",
			Content:           incomingText,
			ExternalMessageID: pgtype.Text{},
			Metadata:          nil,
		})
		_ = s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
			OrganizationID:    pgtypeUUID(orgID),
			PhoneNumber:       phoneKey,
			Role:              "assistant",
			Content:           reply,
			ExternalMessageID: pgtype.Text{},
			Metadata:          nil,
		})
	}

	if err := s.sender.SendReply(ctx, orgID, replyTarget, reply); err != nil {
		s.logError(ctx, "waagent: hardcoded send error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
	}
}

func (s *Service) ProcessVoiceTranscription(ctx context.Context, payload scheduler.WAAgentVoiceTranscriptionPayload) error {
	ctx = withCorrelationIDs(ctx, payload.RequestID, payload.TraceID)
	if s.transcriber == nil || s.storage == nil || s.sender == nil || s.sender.client == nil {
		s.logError(ctx, "waagent: voice transcription dependencies missing",
			"has_transcriber", s.transcriber != nil,
			"has_storage", s.storage != nil,
			"has_sender", s.sender != nil,
			"has_sender_client", s.sender != nil && s.sender.client != nil)
		return fmt.Errorf("waagent voice transcription dependencies are not configured")
	}
	voiceCtx, err := s.prepareVoiceTranscription(ctx, payload)
	if err != nil {
		return err
	}
	if voiceCtx == nil {
		return nil
	}

	note, err := s.downloadAndStoreVoiceNote(ctx, *voiceCtx)
	if err != nil || note == nil {
		return err
	}

	result, err := s.transcribeVoiceNote(ctx, *voiceCtx, *note)
	if err != nil {
		return err
	}

	return s.finalizeVoiceTranscription(ctx, *voiceCtx, *note, result)
}

func (s *Service) prepareVoiceTranscription(ctx context.Context, payload scheduler.WAAgentVoiceTranscriptionPayload) (*voiceTranscriptionContext, error) {
	orgID, err := uuid.Parse(strings.TrimSpace(payload.OrganizationID))
	if err != nil {
		return nil, err
	}
	voiceCtx := &voiceTranscriptionContext{
		orgID:             orgID,
		phoneNumber:       strings.TrimSpace(payload.PhoneNumber),
		externalMessageID: strings.TrimSpace(payload.ExternalMessageID),
	}
	if voiceCtx.phoneNumber == "" || voiceCtx.externalMessageID == "" {
		return nil, fmt.Errorf("waagent voice transcription payload is incomplete")
	}
	voiceCtx.phoneKey = normalizeAgentPhoneKey(voiceCtx.phoneNumber)

	transcriptionRow, err := s.queries.GetAgentVoiceTranscriptionByExternalID(ctx, waagentdb.GetAgentVoiceTranscriptionByExternalIDParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: voiceCtx.externalMessageID,
	})
	if err == nil && strings.EqualFold(strings.TrimSpace(transcriptionRow.Status), "completed") {
		return nil, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err := s.queries.MarkAgentVoiceTranscriptionProcessing(ctx, waagentdb.MarkAgentVoiceTranscriptionProcessingParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: voiceCtx.externalMessageID,
	}); err != nil {
		return nil, err
	}
	return voiceCtx, nil
}

func (s *Service) downloadAndStoreVoiceNote(ctx context.Context, voiceCtx voiceTranscriptionContext) (*storedVoiceNote, error) {
	config, err := s.sender.getAgentConfig(ctx)
	if err != nil {
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), false)
		return nil, nil
	}
	mediaFile, err := s.sender.client.DownloadMediaFile(ctx, config.DeviceID, voiceCtx.externalMessageID, voiceCtx.phoneNumber)
	if err != nil {
		s.logError(ctx, "waagent: voice download failed", "phone", voiceCtx.phoneKey, "external_message_id", voiceCtx.externalMessageID, "device_id", config.DeviceID, "error", err)
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), true)
		return nil, err
	}
	contentType := normalizeVoiceImportContentType(mediaFile.ContentType, mediaFile.MediaType)
	s.logInfo(ctx, "waagent: voice download ok", "phone", voiceCtx.phoneKey, "content_type", contentType, "raw_content_type", mediaFile.ContentType, "raw_media_type", mediaFile.MediaType, "size_bytes", len(mediaFile.Data))
	if err := s.validateVoiceNote(ctx, voiceCtx, contentType, mediaFile.Data); err != nil {
		return nil, nil
	}
	note := &storedVoiceNote{
		contentType: contentType,
		fileName:    chooseVoiceFileName(mediaFile.Filename, contentType),
		data:        mediaFile.Data,
	}
	folder := voiceStorageFolder(voiceCtx.orgID, voiceCtx.phoneKey, voiceCtx.externalMessageID)
	note.fileKey, err = s.storage.UploadFile(ctx, s.attachmentBucket, folder, note.fileName, note.contentType, bytes.NewReader(note.data), int64(len(note.data)))
	if err != nil {
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), true)
		return nil, err
	}
	if err := s.queries.UpdateAgentVoiceTranscriptionStorage(ctx, waagentdb.UpdateAgentVoiceTranscriptionStorageParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: voiceCtx.externalMessageID,
		StorageBucket:     pgtype.Text{String: s.attachmentBucket, Valid: true},
		StorageKey:        pgtype.Text{String: note.fileKey, Valid: true},
		ContentType:       pgtype.Text{String: note.contentType, Valid: true},
	}); err != nil {
		return nil, err
	}
	return note, nil
}

func (s *Service) validateVoiceNote(ctx context.Context, voiceCtx voiceTranscriptionContext, contentType string, data []byte) error {
	// Voice notes from WhatsApp may use MIME types (e.g. audio/opus, audio/aac)
	// that are not in the general-purpose storage upload whitelist. Since we already
	// verified this is an audio message upstream, accept any audio/* content type.
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "audio/") {
		if err := s.storage.ValidateContentType(contentType); err != nil {
			s.logWarn(ctx, logAudioFallback, "reason", "content_type_rejected", "phone", voiceCtx.phoneKey, "content_type", contentType)
			s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), false)
			s.sendAudioFallback(ctx, voiceCtx.orgID, voiceCtx.phoneKey, voiceCtx.phoneNumber, CurrentInboundMessage{ExternalMessageID: voiceCtx.externalMessageID})
			return err
		}
	}
	if err := s.storage.ValidateFileSize(int64(len(data))); err != nil {
		s.logWarn(ctx, logAudioFallback, "reason", "file_size_rejected", "phone", voiceCtx.phoneKey, "size_bytes", len(data))
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), false)
		s.sendAudioFallback(ctx, voiceCtx.orgID, voiceCtx.phoneKey, voiceCtx.phoneNumber, CurrentInboundMessage{ExternalMessageID: voiceCtx.externalMessageID})
		return err
	}
	return nil
}

func (s *Service) transcribeVoiceNote(ctx context.Context, voiceCtx voiceTranscriptionContext, note storedVoiceNote) (AudioTranscriptionResult, error) {
	result, err := s.transcriber.Transcribe(ctx, AudioTranscriptionInput{
		Filename:    note.fileName,
		ContentType: note.contentType,
		Data:        note.data,
	})
	if err != nil {
		s.logError(ctx, "waagent: voice transcribe failed", "phone", voiceCtx.phoneKey, "content_type", note.contentType, "size_bytes", len(note.data), "error", err)
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), true)
		return AudioTranscriptionResult{}, err
	}
	s.logInfo(ctx, "waagent: voice transcribe ok", "phone", voiceCtx.phoneKey, "language", result.Language, "text_preview", truncateForLog(result.Text, 80))
	return result, nil
}

func (s *Service) finalizeVoiceTranscription(ctx context.Context, voiceCtx voiceTranscriptionContext, note storedVoiceNote, result AudioTranscriptionResult) error {
	message, err := s.queries.GetAgentMessageByExternalID(ctx, waagentdb.GetAgentMessageByExternalIDParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: optionalPgText(voiceCtx.externalMessageID),
	})
	if err != nil {
		return err
	}
	transcriptText := formatVoiceTranscriptUserMessage(result.Text)
	metadata := mergeVoiceTranscriptionMetadata(message.Metadata, voiceTranscriptionUpdate{
		Status:        "completed",
		Provider:      audioProviderName(s.transcriber),
		StorageBucket: s.attachmentBucket,
		StorageKey:    note.fileKey,
		Language:      result.Language,
		Transcript:    result.Text,
		Confidence:    result.Confidence,
	})
	if err := s.queries.UpdateAgentMessageByExternalID(ctx, waagentdb.UpdateAgentMessageByExternalIDParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: optionalPgText(voiceCtx.externalMessageID),
		Content:           transcriptText,
		Metadata:          metadata,
	}); err != nil {
		return err
	}
	if err := s.updateInboundInboxMessage(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, transcriptText, metadata); err != nil {
		s.logWarn(ctx, "waagent: failed to update mirrored inbox message", "organization_id", voiceCtx.orgID.String(), "external_message_id", voiceCtx.externalMessageID, "error", err)
	}
	inbound := &CurrentInboundMessage{
		ExternalMessageID: voiceCtx.externalMessageID,
		PhoneNumber:       voiceCtx.phoneNumber,
		Body:              transcriptText,
		Metadata:          metadata,
	}
	s.runAgentReply(ctx, voiceCtx.orgID, voiceCtx.phoneKey, voiceCtx.phoneNumber, inbound, agentModeDecision{mode: agentRunModeDefault, reason: "voice_transcription"})
	return s.queries.MarkAgentVoiceTranscriptionCompleted(ctx, waagentdb.MarkAgentVoiceTranscriptionCompletedParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: voiceCtx.externalMessageID,
		TranscriptText:    pgtype.Text{String: strings.TrimSpace(result.Text), Valid: strings.TrimSpace(result.Text) != ""},
		Provider:          pgtype.Text{String: audioProviderName(s.transcriber), Valid: audioProviderName(s.transcriber) != ""},
		Language:          pgtype.Text{String: strings.TrimSpace(result.Language), Valid: strings.TrimSpace(result.Language) != ""},
		ConfidenceScore:   optionalPgFloat8(result.Confidence),
	})
}

// resolveLeadHint fetches the conversation lead hint for lead resolution.
// Concrete customer details must still be fetched via tools during the run.
func (s *Service) resolveLeadHint(ctx context.Context, orgID uuid.UUID, phoneKey string) *ConversationLeadHint {
	_ = ctx
	if s.leadHintStore == nil {
		return nil
	}
	hint, _ := s.leadHintStore.Get(orgID.String(), phoneKey)
	return hint
}

func (s *Service) lookupAgentUser(ctx context.Context, phone string) (waagentdb.GetAgentUserByPhoneRow, error) {
	var lastErr error
	for _, candidate := range agentPhoneCandidates(phone) {
		user, err := s.queries.GetAgentUserByPhone(ctx, candidate)
		if err == nil {
			return user, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			lastErr = err
			continue
		}
		return waagentdb.GetAgentUserByPhoneRow{}, err
	}
	if lastErr == nil {
		lastErr = pgx.ErrNoRows
	}
	return waagentdb.GetAgentUserByPhoneRow{}, lastErr
}

func pgtypeUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func uuidFromPgtype(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

func optionalPgText(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}

func optionalPgFloat8(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func inboundMessageID(inbound *CurrentInboundMessage) string {
	if inbound == nil {
		return ""
	}
	return inbound.ExternalMessageID
}

func inboundMetadata(inbound *CurrentInboundMessage) []byte {
	if inbound == nil || len(inbound.Metadata) == 0 {
		return nil
	}
	return inbound.Metadata
}

type agentModeDecision struct {
	mode      agentRunMode
	reason    string
	partnerID *uuid.UUID
}

func (s *Service) selectAgentRunMode(ctx context.Context, orgID uuid.UUID, phoneKey, message string) agentModeDecision {
	_ = ctx
	_ = orgID
	_ = phoneKey
	_ = message
	return agentModeDecision{mode: agentRunModeDefault, reason: "default"}
}

func (s *Service) applyReplyPolicy(ctx context.Context, orgID uuid.UUID, phoneKey string, decision agentModeDecision, reply string) string {
	_ = ctx
	_ = orgID
	_ = phoneKey
	_ = decision
	return strings.TrimSpace(reply)
}

func partnerIDFromAgentUser(user waagentdb.GetAgentUserByPhoneRow) (uuid.UUID, bool) {
	if strings.TrimSpace(user.UserType) != "partner" || !user.PartnerID.Valid {
		return uuid.Nil, false
	}
	return uuid.UUID(user.PartnerID.Bytes), true
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func (s *Service) claimInboundMessage(ctx context.Context, externalMessageID, phoneKey string) (bool, error) {
	if s.rateLimiter == nil {
		s.logWarn(ctx, "waagent: rate limiter unavailable; dedupe disabled", "phone", phoneKey, "external_message_id", externalMessageID)
		return true, nil
	}
	claimed, err := s.rateLimiter.ClaimMessage(ctx, externalMessageID)
	if err != nil {
		s.logWarn(ctx, "waagent: message dedupe unavailable; continuing fail-open", "phone", phoneKey, "external_message_id", externalMessageID, "error", err)
		return true, err
	}
	if !claimed {
		s.logInfo(ctx, "waagent: duplicate inbound message ignored", "phone", phoneKey, "external_message_id", externalMessageID)
	}
	return claimed, nil
}

func (s *Service) allowInboundMessage(ctx context.Context, phoneKey, externalMessageID string) bool {
	if s.rateLimiter == nil {
		s.logWarn(ctx, "waagent: rate limiter unavailable; throttling disabled", "phone", phoneKey, "external_message_id", externalMessageID)
		return true
	}
	allowed, err := s.rateLimiter.Allow(ctx, phoneKey)
	if err != nil {
		s.logWarn(ctx, "waagent: rate limiter unavailable; continuing fail-open", "phone", phoneKey, "external_message_id", externalMessageID, "error", err)
		return true
	}
	if !allowed {
		s.logInfo(ctx, "waagent: inbound message rate limited", "phone", phoneKey, "external_message_id", externalMessageID)
	}
	return allowed
}

func (s *Service) persistInboundMessage(ctx context.Context, orgID uuid.UUID, phoneKey, content string, inbound *CurrentInboundMessage) error {
	return s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
		OrganizationID:    pgtypeUUID(orgID),
		PhoneNumber:       phoneKey,
		Role:              "user",
		Content:           content,
		ExternalMessageID: optionalPgText(inboundMessageID(inbound)),
		Metadata:          inboundMetadata(inbound),
	})
}

func (s *Service) sendAssistantReply(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, reply string) {
	if err := s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
		OrganizationID:    pgtypeUUID(orgID),
		PhoneNumber:       phoneKey,
		Role:              "assistant",
		Content:           reply,
		ExternalMessageID: pgtype.Text{},
		Metadata:          nil,
	}); err != nil {
		s.logError(ctx, "waagent: failed to persist assistant message", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
	}
	if err := s.sender.SendReply(ctx, orgID, replyTarget, reply); err != nil {
		s.logError(ctx, "waagent: send reply error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
	}
}

func (s *Service) sendAudioFallback(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string, inbound CurrentInboundMessage) {
	s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgVoiceUnavailable)
	updated := mergeVoiceTranscriptionMetadata(inbound.Metadata, voiceTranscriptionUpdate{Status: "failed", ErrorMessage: msgVoiceUnavailable})
	if strings.TrimSpace(inbound.ExternalMessageID) != "" {
		if err := s.queries.UpdateAgentMessageByExternalID(ctx, waagentdb.UpdateAgentMessageByExternalIDParams{
			OrganizationID:    pgtypeUUID(orgID),
			ExternalMessageID: optionalPgText(inbound.ExternalMessageID),
			Content:           voiceMessagePlaceholder,
			Metadata:          updated,
		}); err != nil {
			s.logWarn(ctx, "waagent: failed to update failed audio metadata", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		}
		if err := s.updateInboundInboxMessage(ctx, orgID, inbound.ExternalMessageID, voiceMessagePlaceholder, updated); err != nil {
			s.logWarn(ctx, "waagent: failed to update failed mirrored inbox message", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		}
	}
}

func (s *Service) failVoiceTranscription(ctx context.Context, orgID uuid.UUID, externalMessageID string, reason string, retryable bool) {
	if strings.TrimSpace(externalMessageID) == "" {
		return
	}
	if err := s.queries.MarkAgentVoiceTranscriptionFailed(ctx, waagentdb.MarkAgentVoiceTranscriptionFailedParams{
		OrganizationID:    pgtypeUUID(orgID),
		ExternalMessageID: strings.TrimSpace(externalMessageID),
		ErrorMessage:      pgtype.Text{String: truncateReason(reason), Valid: strings.TrimSpace(reason) != ""},
	}); err != nil {
		s.logWarn(ctx, "waagent: failed to mark audio transcription failure", "organization_id", orgID.String(), "external_message_id", externalMessageID, "error", err)
	}
	if !retryable {
		message, err := s.queries.GetAgentMessageByExternalID(ctx, waagentdb.GetAgentMessageByExternalIDParams{
			OrganizationID:    pgtypeUUID(orgID),
			ExternalMessageID: optionalPgText(externalMessageID),
		})
		if err == nil {
			metadata := mergeVoiceTranscriptionMetadata(message.Metadata, voiceTranscriptionUpdate{Status: "failed", ErrorMessage: reason})
			_ = s.queries.UpdateAgentMessageByExternalID(ctx, waagentdb.UpdateAgentMessageByExternalIDParams{
				OrganizationID:    pgtypeUUID(orgID),
				ExternalMessageID: optionalPgText(externalMessageID),
				Content:           message.Content,
				Metadata:          metadata,
			})
		}
	}
}

func audioProviderName(transcriber AudioTranscriber) string {
	if transcriber == nil {
		return ""
	}
	return strings.TrimSpace(transcriber.Name())
}

func truncateReason(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 500 {
		return trimmed
	}
	return trimmed[:500]
}

func truncateForLog(value string, max int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "…"
}

func stringContextValue(ctx context.Context, key any) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(key).(string)
	return strings.TrimSpace(value)
}

func withCorrelationIDs(ctx context.Context, requestID, traceID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	traceID = strings.TrimSpace(traceID)
	if requestID == "" && traceID == "" {
		return ctx
	}
	if requestID != "" {
		ctx = context.WithValue(ctx, logger.RequestIDKey, requestID)
	}
	if traceID != "" {
		ctx = context.WithValue(ctx, logger.TraceIDKey, traceID)
	}
	return ctx
}

func (s *Service) loggerWithContext(ctx context.Context) *logger.Logger {
	if s == nil || s.log == nil {
		return nil
	}
	return s.log.WithContext(ctx)
}

func (s *Service) logInfo(ctx context.Context, message string, args ...any) {
	if log := s.loggerWithContext(ctx); log != nil {
		log.Info(message, args...)
	}
}

func (s *Service) logWarn(ctx context.Context, message string, args ...any) {
	if log := s.loggerWithContext(ctx); log != nil {
		log.Warn(message, args...)
	}
}

func (s *Service) logError(ctx context.Context, message string, args ...any) {
	if log := s.loggerWithContext(ctx); log != nil {
		log.Error(message, args...)
	}
}

func (s *Service) syncInboundInboxMessage(ctx context.Context, orgID uuid.UUID, phoneNumber string, inbound *CurrentInboundMessage) {
	if s.inboxSync == nil || inbound == nil {
		return
	}
	var externalMessageID *string
	if trimmed := strings.TrimSpace(inbound.ExternalMessageID); trimmed != "" {
		externalMessageID = &trimmed
	}
	if err := s.inboxSync.PersistIncomingWhatsAppMessage(ctx, orgID, phoneNumber, inbound.DisplayName, inbound.Body, externalMessageID, inbound.Metadata); err != nil {
		s.logWarn(ctx, "waagent: failed to mirror inbound inbox message", "organization_id", orgID.String(), "phone", phoneNumber, "error", err)
	}
}

func (s *Service) updateInboundInboxMessage(ctx context.Context, orgID uuid.UUID, externalMessageID, body string, metadata []byte) error {
	if s.inboxSync == nil || strings.TrimSpace(externalMessageID) == "" {
		return nil
	}
	return s.inboxSync.UpdateIncomingWhatsAppMessage(ctx, orgID, externalMessageID, body, metadata)
}

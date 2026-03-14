package waagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

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
	msgRateLimited      = "Je stuurt te veel berichten. Probeer het over een paar minuten opnieuw."
	msgUnknownPhone     = "Je telefoonnummer is niet gekoppeld aan een organisatie. Neem contact op met je beheerder."
	msgVoiceUnavailable = "Ik kon je spraakbericht niet verwerken. Stuur je vraag als tekst, dan help ik je meteen."
	recentMessageLimit  = 8
)

// Service orchestrates the waagent flow: rate limit → phone→org → AI.
type Service struct {
	queries                waagentdb.Querier
	agent                  *Agent
	sender                 *Sender
	rateLimiter            *RateLimiter
	leadHintStore          *ConversationLeadHintStore
	leadDetailsReader      LeadDetailsReader
	storage                ObjectStorage
	attachmentBucket       string
	transcriptionScheduler AudioTranscriptionScheduler
	transcriber            AudioTranscriber
	inboxSync              InboxMessageSync
	log                    *logger.Logger
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

	claimed, err := s.rateLimiter.ClaimMessage(ctx, strings.TrimSpace(externalMessageID))
	if err != nil {
		log.Printf("waagent: message dedupe error id=%s: %v", externalMessageID, err)
	}
	if !claimed {
		log.Printf("waagent: duplicate inbound message ignored id=%s phone=%s", externalMessageID, lookupPhone)
		return
	}

	// Step 1: Rate limit check
	allowed, err := s.rateLimiter.Allow(ctx, lookupPhone)
	if err != nil {
		log.Printf("waagent: rate limiter error phone=%s: %v", lookupPhone, err)
	}
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

	if isAudioInboundMessage(inbound) {
		s.handleIncomingAudioMessage(ctx, orgID, lookupPhone, replyTarget, inbound)
		return
	}

	s.handleAIMessage(ctx, orgID, lookupPhone, replyTarget, text, &inbound)
}

func (s *Service) handleAIMessage(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, text string, inbound *CurrentInboundMessage) {
	if err := s.persistInboundMessage(ctx, orgID, phoneKey, text, inbound); err != nil {
		log.Printf("waagent: failed to persist user message phone=%s: %v", phoneKey, err)
	}
	s.syncInboundInboxMessage(ctx, orgID, replyTarget, inbound)
	s.runAgentReply(ctx, orgID, phoneKey, replyTarget, inbound)
}

func (s *Service) handleIncomingAudioMessage(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string, inbound CurrentInboundMessage) {
	if strings.TrimSpace(inbound.ExternalMessageID) == "" {
		log.Printf("waagent: audio fallback reason=missing_external_id phone=%s org=%s", phoneKey, orgID)
		s.sendAudioFallback(ctx, orgID, phoneKey, replyTarget, inbound)
		return
	}

	inbound.Body = voiceMessagePlaceholder
	inbound.Metadata = mergeVoiceTranscriptionMetadata(inbound.Metadata, voiceTranscriptionUpdate{Status: "pending"})
	if err := s.persistInboundMessage(ctx, orgID, phoneKey, inbound.Body, &inbound); err != nil {
		log.Printf("waagent: failed to persist pending audio message phone=%s: %v", phoneKey, err)
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
		log.Printf("waagent: failed to create pending audio transcription row phone=%s: %v", phoneKey, err)
	}
	if s.transcriptionScheduler == nil || s.transcriber == nil || s.storage == nil || strings.TrimSpace(s.attachmentBucket) == "" {
		log.Printf("waagent: audio fallback reason=deps_nil phone=%s org=%s scheduler=%v transcriber=%v storage=%v bucket=%q",
			phoneKey, orgID, s.transcriptionScheduler != nil, s.transcriber != nil, s.storage != nil, s.attachmentBucket)
		s.failVoiceTranscription(ctx, orgID, inbound.ExternalMessageID, "audio transcription is not configured", false)
		s.sendAudioFallback(ctx, orgID, phoneKey, replyTarget, inbound)
		return
	}
	if err := s.transcriptionScheduler.EnqueueWAAgentVoiceTranscription(ctx, scheduler.WAAgentVoiceTranscriptionPayload{
		OrganizationID:    orgID.String(),
		PhoneNumber:       replyTarget,
		ExternalMessageID: inbound.ExternalMessageID,
	}); err != nil {
		log.Printf("waagent: audio fallback reason=enqueue_failed phone=%s org=%s: %v", phoneKey, orgID, err)
		s.failVoiceTranscription(ctx, orgID, inbound.ExternalMessageID, err.Error(), false)
		s.sendAudioFallback(ctx, orgID, phoneKey, replyTarget, inbound)
	}
}

func (s *Service) runAgentReply(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string, inbound *CurrentInboundMessage) {

	// Load recent conversation history
	recent, err := s.queries.GetRecentAgentMessages(ctx, waagentdb.GetRecentAgentMessagesParams{
		PhoneNumber: phoneKey,
		Limit:       recentMessageLimit,
	})
	if err != nil {
		log.Printf("waagent: failed to load history phone=%s: %v", phoneKey, err)
		recent = nil
	}

	// Reverse to chronological order (DB returns DESC)
	messages := make([]ConversationMessage, len(recent))
	for i, msg := range recent {
		messages[len(recent)-1-i] = ConversationMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	if err := s.sender.SendChatPresence(ctx, replyTarget, whatsapp.ChatPresenceComposing); err != nil {
		log.Printf("waagent: send chat presence start error phone=%s org=%s: %v", replyTarget, orgID, err)
	}
	defer func() {
		if err := s.sender.SendChatPresence(context.Background(), replyTarget, whatsapp.ChatPresencePaused); err != nil {
			log.Printf("waagent: send chat presence stop error phone=%s org=%s: %v", replyTarget, orgID, err)
		}
	}()

	// Run the AI agent
	leadHint := s.resolveLeadHint(ctx, orgID, phoneKey)

	reply, err := s.agent.Run(ctx, orgID, phoneKey, messages, leadHint, inbound)
	if err != nil {
		log.Printf("waagent: agent run error phone=%s org=%s: %v", phoneKey, orgID, err)
		return
	}

	reply = sanitizeWhatsAppReply(strings.TrimSpace(reply))
	if reply == "" {
		log.Printf("waagent: empty agent reply phone=%s org=%s", phoneKey, orgID)
		return
	}

	s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, reply)
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
		log.Printf("waagent: hardcoded send error phone=%s: %v", replyTarget, err)
	}
}

func (s *Service) ProcessVoiceTranscription(ctx context.Context, payload scheduler.WAAgentVoiceTranscriptionPayload) error {
	if s.transcriber == nil || s.storage == nil || s.sender == nil || s.sender.client == nil {
		log.Printf("waagent: voice transcription deps nil transcriber=%v storage=%v sender=%v client=%v",
			s.transcriber != nil, s.storage != nil, s.sender != nil, s.sender != nil && s.sender.client != nil)
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
		log.Printf("waagent: voice download failed phone=%s external=%s device=%s: %v", voiceCtx.phoneKey, voiceCtx.externalMessageID, config.DeviceID, err)
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), true)
		return nil, err
	}
	contentType := normalizeVoiceImportContentType(mediaFile.ContentType, mediaFile.MediaType)
	log.Printf("waagent: voice download ok phone=%s contentType=%q rawContentType=%q rawMediaType=%q size=%d",
		voiceCtx.phoneKey, contentType, mediaFile.ContentType, mediaFile.MediaType, len(mediaFile.Data))
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
			log.Printf("waagent: audio fallback reason=content_type_rejected phone=%s contentType=%q", voiceCtx.phoneKey, contentType)
			s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), false)
			s.sendAudioFallback(ctx, voiceCtx.orgID, voiceCtx.phoneKey, voiceCtx.phoneNumber, CurrentInboundMessage{ExternalMessageID: voiceCtx.externalMessageID})
			return err
		}
	}
	if err := s.storage.ValidateFileSize(int64(len(data))); err != nil {
		log.Printf("waagent: audio fallback reason=file_size_rejected phone=%s size=%d", voiceCtx.phoneKey, len(data))
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
		log.Printf("waagent: voice transcribe failed phone=%s contentType=%q size=%d: %v", voiceCtx.phoneKey, note.contentType, len(note.data), err)
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), true)
		return AudioTranscriptionResult{}, err
	}
	log.Printf("waagent: voice transcribe ok phone=%s lang=%s text=%q", voiceCtx.phoneKey, result.Language, truncateForLog(result.Text, 80))
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
		log.Printf("waagent: failed to update mirrored inbox message org=%s external=%s: %v", voiceCtx.orgID, voiceCtx.externalMessageID, err)
	}
	inbound := &CurrentInboundMessage{
		ExternalMessageID: voiceCtx.externalMessageID,
		PhoneNumber:       voiceCtx.phoneNumber,
		Body:              transcriptText,
		Metadata:          metadata,
	}
	s.runAgentReply(ctx, voiceCtx.orgID, voiceCtx.phoneKey, voiceCtx.phoneNumber, inbound)
	return s.queries.MarkAgentVoiceTranscriptionCompleted(ctx, waagentdb.MarkAgentVoiceTranscriptionCompletedParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: voiceCtx.externalMessageID,
		TranscriptText:    pgtype.Text{String: strings.TrimSpace(result.Text), Valid: strings.TrimSpace(result.Text) != ""},
		Provider:          pgtype.Text{String: audioProviderName(s.transcriber), Valid: audioProviderName(s.transcriber) != ""},
		Language:          pgtype.Text{String: strings.TrimSpace(result.Language), Valid: strings.TrimSpace(result.Language) != ""},
		ConfidenceScore:   optionalPgFloat8(result.Confidence),
	})
}

// resolveLeadHint fetches the conversation lead hint and auto-loads details.
func (s *Service) resolveLeadHint(ctx context.Context, orgID uuid.UUID, phoneKey string) *ConversationLeadHint {
	if s.leadHintStore == nil {
		return nil
	}
	hint, _ := s.leadHintStore.Get(orgID.String(), phoneKey)
	if hint != nil && strings.TrimSpace(hint.LeadID) != "" && s.leadDetailsReader != nil {
		if details, err := s.leadDetailsReader.GetLeadDetails(ctx, orgID, hint.LeadID); err == nil && details != nil {
			hint.PreloadedDetails = details
		}
	}
	return hint
}

func (s *Service) lookupAgentUser(ctx context.Context, phone string) (waagentdb.RacWhatsappAgentUser, error) {
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
		return waagentdb.RacWhatsappAgentUser{}, err
	}
	if lastErr == nil {
		lastErr = pgx.ErrNoRows
	}
	return waagentdb.RacWhatsappAgentUser{}, lastErr
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
		log.Printf("waagent: failed to persist assistant message phone=%s: %v", phoneKey, err)
	}
	if err := s.sender.SendReply(ctx, orgID, replyTarget, reply); err != nil {
		log.Printf("waagent: send reply error phone=%s org=%s: %v", replyTarget, orgID, err)
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
			log.Printf("waagent: failed to update failed audio metadata phone=%s: %v", phoneKey, err)
		}
		if err := s.updateInboundInboxMessage(ctx, orgID, inbound.ExternalMessageID, voiceMessagePlaceholder, updated); err != nil {
			log.Printf("waagent: failed to update failed mirrored inbox message phone=%s: %v", phoneKey, err)
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
		log.Printf("waagent: failed to mark audio transcription failure org=%s external=%s: %v", orgID, externalMessageID, err)
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

func (s *Service) syncInboundInboxMessage(ctx context.Context, orgID uuid.UUID, phoneNumber string, inbound *CurrentInboundMessage) {
	if s.inboxSync == nil || inbound == nil {
		return
	}
	var externalMessageID *string
	if trimmed := strings.TrimSpace(inbound.ExternalMessageID); trimmed != "" {
		externalMessageID = &trimmed
	}
	if err := s.inboxSync.PersistIncomingWhatsAppMessage(ctx, orgID, phoneNumber, inbound.DisplayName, inbound.Body, externalMessageID, inbound.Metadata); err != nil {
		log.Printf("waagent: failed to mirror inbound inbox message org=%s phone=%s: %v", orgID, phoneNumber, err)
	}
}

func (s *Service) updateInboundInboxMessage(ctx context.Context, orgID uuid.UUID, externalMessageID, body string, metadata []byte) error {
	if s.inboxSync == nil || strings.TrimSpace(externalMessageID) == "" {
		return nil
	}
	return s.inboxSync.UpdateIncomingWhatsAppMessage(ctx, orgID, externalMessageID, body, metadata)
}

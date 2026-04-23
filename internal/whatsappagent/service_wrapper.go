package whatsappagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/internal/whatsappagent/engine"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const msgRateLimited = "Je stuurt te veel berichten. Probeer het over een paar minuten opnieuw."
const msgUnknownPhone = "Je telefoonnummer is niet gekoppeld aan een organisatie. Neem contact op met je beheerder."
const msgVoiceUnavailable = "Ik kon je spraakbericht niet verwerken. Stuur je vraag als tekst, dan help ik je meteen."
const msgSystemUnavailable = "Ons systeem is tijdelijk niet beschikbaar. Probeer het later opnieuw."
const msgConversationReset = "Ik heb ons gesprek gewist. We beginnen opnieuw met een schone context."
const msgGroundingFallback = "Ik kan deze informatie op dit moment niet betrouwbaar bevestigen. Bekijk de actuele gegevens in het portaal."
const logAudioFallback = "whatsappagent: audio fallback"
const recentMessageLimit = 100

type voiceTranscriptionQuerier interface {
	DeleteAgentMessagesByPhone(ctx context.Context, arg whatsappagentdb.DeleteAgentMessagesByPhoneParams) error
	GetAgentMessageByExternalID(ctx context.Context, arg whatsappagentdb.GetAgentMessageByExternalIDParams) (whatsappagentdb.GetAgentMessageByExternalIDRow, error)
	GetRecentAgentMessages(ctx context.Context, arg whatsappagentdb.GetRecentAgentMessagesParams) ([]whatsappagentdb.GetRecentAgentMessagesRow, error)
	GetAgentUserByPhone(ctx context.Context, phoneNumber string) (whatsappagentdb.GetAgentUserByPhoneRow, error)
	GetAgentVoiceTranscriptionByExternalID(ctx context.Context, arg whatsappagentdb.GetAgentVoiceTranscriptionByExternalIDParams) (whatsappagentdb.RacWhatsappAgentVoiceTranscription, error)
	InsertAgentMessage(ctx context.Context, arg whatsappagentdb.InsertAgentMessageParams) error
	MarkAgentVoiceTranscriptionCompleted(ctx context.Context, arg whatsappagentdb.MarkAgentVoiceTranscriptionCompletedParams) error
	MarkAgentVoiceTranscriptionFailed(ctx context.Context, arg whatsappagentdb.MarkAgentVoiceTranscriptionFailedParams) error
	MarkAgentVoiceTranscriptionProcessing(ctx context.Context, arg whatsappagentdb.MarkAgentVoiceTranscriptionProcessingParams) error
	UpdateAgentMessageByExternalID(ctx context.Context, arg whatsappagentdb.UpdateAgentMessageByExternalIDParams) error
	UpdateAgentVoiceTranscriptionStorage(ctx context.Context, arg whatsappagentdb.UpdateAgentVoiceTranscriptionStorageParams) error
	UpsertAgentVoiceTranscription(ctx context.Context, arg whatsappagentdb.UpsertAgentVoiceTranscriptionParams) error
}

type legacyVoiceReplyRunner interface {
	RunPreparedDefaultAgent(ctx context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *engine.ConversationLeadHint, inbound *engine.CurrentInboundMessage) (AgentRunResult, error)
	RunPreparedPartnerAgent(ctx context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *engine.ConversationLeadHint, inbound *engine.CurrentInboundMessage, partnerID uuid.UUID) (AgentRunResult, error)
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

type Service struct {
	inner                  *engine.Service
	replyRunner            legacyVoiceReplyRunner
	queries                voiceTranscriptionQuerier
	sender                 *Sender
	storage                ObjectStorage
	attachmentBucket       string
	transcriptionScheduler AudioTranscriptionScheduler
	transcriber            AudioTranscriber
	inboxSync              InboxMessageSync
	rateLimiter            *engine.RateLimiter
	debouncer              *engine.MessageDebouncer
	leadHintStore          LeadHintStore
	log                    *logger.Logger

	conversationMu    sync.Mutex
	conversationLocks map[string]*sync.Mutex
}

func newService(pool *pgxpool.Pool, deps ModuleDependencies, inner *engine.Service) *Service {
	if inner == nil {
		return nil
	}
	service := &Service{
		inner:            inner,
		replyRunner:      inner,
		storage:          deps.Storage,
		attachmentBucket: deps.AttachmentBucket,
		transcriber:      deps.AudioTranscriber,
		inboxSync:        deps.InboxMessageSync,
		log:              deps.Logger,
	}
	if pool != nil {
		queries := whatsappagentdb.New(pool)
		service.queries = queries
		if deps.WhatsAppClient != nil {
			service.sender = &Sender{client: deps.WhatsAppClient, queries: queries, inboxWriter: deps.InboxWriter, log: deps.Logger}
		}
	}
	service.transcriptionScheduler = deps.TranscriptionScheduler
	service.rateLimiter = engine.NewRateLimiter(deps.RedisClient, deps.Logger)
	service.debouncer = engine.NewMessageDebouncer(deps.RedisClient, deps.Logger)
	service.leadHintStore = NewRedisConversationLeadHintStore(deps.RedisClient, deps.Logger)
	return service
}

func (s *Service) HandleIncomingMessage(ctx context.Context, inbound CurrentInboundMessage) {
	if s == nil || s.replyRunner == nil || s.queries == nil {
		return
	}

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

	if !s.allowInboundMessage(ctx, lookupPhone, externalMessageID) {
		s.sendHardcoded(ctx, uuid.Nil, lookupPhone, replyTarget, text, msgRateLimited)
		return
	}

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

	if err := s.persistInboundMessage(ctx, orgID, lookupPhone, text, &inbound); err != nil {
		s.logError(ctx, "whatsappagent: failed to persist user message", "phone", lookupPhone, "organization_id", orgID.String(), "error", err)
	}
	s.syncInboundInboxMessage(ctx, orgID, replyTarget, &inbound)

	// Debounce: when multiple messages arrive in quick succession only the
	// last handler runs the agent. Earlier handlers exit after the wait.
	nonce := s.debouncer.Claim(ctx, lookupPhone)
	if nonce != "" {
		s.debouncer.Wait()
		if !s.debouncer.ShouldProceed(ctx, lookupPhone, nonce) {
			return
		}
	}

	inboundCopy := inbound
	var partnerID *uuid.UUID
	if resolvedPartnerID, ok := partnerIDFromAgentUser(user); ok {
		partnerID = &resolvedPartnerID
	}
	s.runAgentReply(ctx, orgID, lookupPhone, replyTarget, &inboundCopy, partnerID, "default")
}

func (s *Service) ProcessVoiceTranscription(ctx context.Context, payload scheduler.WAAgentVoiceTranscriptionPayload) error {
	ctx = withCorrelationIDs(ctx, payload.RequestID, payload.TraceID)
	if s == nil || s.replyRunner == nil || s.transcriber == nil || s.storage == nil || s.sender == nil || s.sender.client == nil || s.queries == nil {
		s.logError(ctx, "whatsappagent: voice transcription dependencies missing",
			"has_transcriber", s != nil && s.transcriber != nil,
			"has_storage", s != nil && s.storage != nil,
			"has_sender", s != nil && s.sender != nil,
			"has_sender_client", s != nil && s.sender != nil && s.sender.client != nil,
			"has_queries", s != nil && s.queries != nil,
			"has_reply_runner", s != nil && s.replyRunner != nil)
		return fmt.Errorf("whatsappagent voice transcription dependencies are not configured")
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
		return nil, fmt.Errorf("whatsappagent voice transcription payload is incomplete")
	}
	voiceCtx.phoneKey = normalizeAgentPhoneKey(voiceCtx.phoneNumber)

	transcriptionRow, err := s.queries.GetAgentVoiceTranscriptionByExternalID(ctx, whatsappagentdb.GetAgentVoiceTranscriptionByExternalIDParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: voiceCtx.externalMessageID,
	})
	if err == nil && strings.EqualFold(strings.TrimSpace(transcriptionRow.Status), "completed") {
		return nil, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err := s.queries.MarkAgentVoiceTranscriptionProcessing(ctx, whatsappagentdb.MarkAgentVoiceTranscriptionProcessingParams{
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
		s.logError(ctx, "whatsappagent: voice download failed", "phone", voiceCtx.phoneKey, "external_message_id", voiceCtx.externalMessageID, "device_id", config.DeviceID, "error", err)
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), true)
		return nil, err
	}
	contentType := normalizeVoiceImportContentType(mediaFile.ContentType, mediaFile.MediaType)
	s.logInfo(ctx, "whatsappagent: voice download ok", "phone", voiceCtx.phoneKey, "content_type", contentType, "raw_content_type", mediaFile.ContentType, "raw_media_type", mediaFile.MediaType, "size_bytes", len(mediaFile.Data))
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
	if err := s.queries.UpdateAgentVoiceTranscriptionStorage(ctx, whatsappagentdb.UpdateAgentVoiceTranscriptionStorageParams{
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
		s.logError(ctx, "whatsappagent: voice transcribe failed", "phone", voiceCtx.phoneKey, "content_type", note.contentType, "size_bytes", len(note.data), "error", err)
		s.failVoiceTranscription(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, err.Error(), true)
		return AudioTranscriptionResult{}, err
	}
	s.logInfo(ctx, "whatsappagent: voice transcribe ok", "phone", voiceCtx.phoneKey, "language", result.Language, "text_preview", truncateForLog(result.Text, 80))
	return result, nil
}

func (s *Service) finalizeVoiceTranscription(ctx context.Context, voiceCtx voiceTranscriptionContext, note storedVoiceNote, result AudioTranscriptionResult) error {
	message, err := s.queries.GetAgentMessageByExternalID(ctx, whatsappagentdb.GetAgentMessageByExternalIDParams{
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
	if err := s.queries.UpdateAgentMessageByExternalID(ctx, whatsappagentdb.UpdateAgentMessageByExternalIDParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: optionalPgText(voiceCtx.externalMessageID),
		Content:           transcriptText,
		Metadata:          metadata,
	}); err != nil {
		return err
	}
	if err := s.updateInboundInboxMessage(ctx, voiceCtx.orgID, voiceCtx.externalMessageID, transcriptText, metadata); err != nil {
		s.logWarn(ctx, "whatsappagent: failed to update mirrored inbox message", "organization_id", voiceCtx.orgID.String(), "external_message_id", voiceCtx.externalMessageID, "error", err)
	}
	inbound := engine.CurrentInboundMessage{
		ExternalMessageID: voiceCtx.externalMessageID,
		PhoneNumber:       voiceCtx.phoneNumber,
		Body:              transcriptText,
		Metadata:          metadata,
	}
	inboundLocal := CurrentInboundMessage(inbound)
	s.runAgentReply(ctx, voiceCtx.orgID, voiceCtx.phoneKey, voiceCtx.phoneNumber, &inboundLocal, nil, "voice_transcription")
	provider := audioProviderName(s.transcriber)
	return s.queries.MarkAgentVoiceTranscriptionCompleted(ctx, whatsappagentdb.MarkAgentVoiceTranscriptionCompletedParams{
		OrganizationID:    pgtypeUUID(voiceCtx.orgID),
		ExternalMessageID: voiceCtx.externalMessageID,
		TranscriptText:    pgtype.Text{String: strings.TrimSpace(result.Text), Valid: strings.TrimSpace(result.Text) != ""},
		Provider:          pgtype.Text{String: provider, Valid: provider != ""},
		Language:          pgtype.Text{String: strings.TrimSpace(result.Language), Valid: strings.TrimSpace(result.Language) != ""},
		ConfidenceScore:   optionalPgFloat8(result.Confidence),
	})
}

func shouldResetConversation(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return normalized == "/clear" || normalized == "/reset"
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
		s.logError(ctx, "whatsappagent: failed to persist pending audio message", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
	}
	s.syncInboundInboxMessage(ctx, orgID, replyTarget, &inbound)
	if err := s.queries.UpsertAgentVoiceTranscription(ctx, whatsappagentdb.UpsertAgentVoiceTranscriptionParams{
		OrganizationID:    pgtypeUUID(orgID),
		ExternalMessageID: strings.TrimSpace(inbound.ExternalMessageID),
		PhoneNumber:       phoneKey,
		Status:            "pending",
		Provider:          optionalPgText(audioProviderName(s.transcriber)),
		ErrorMessage:      pgtype.Text{},
	}); err != nil {
		s.logError(ctx, "whatsappagent: failed to create pending audio transcription row", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
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

func (s *Service) lookupAgentUser(ctx context.Context, phone string) (whatsappagentdb.GetAgentUserByPhoneRow, error) {
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
		return whatsappagentdb.GetAgentUserByPhoneRow{}, err
	}
	if lastErr == nil {
		lastErr = pgx.ErrNoRows
	}
	return whatsappagentdb.GetAgentUserByPhoneRow{}, lastErr
}

func (s *Service) claimInboundMessage(ctx context.Context, externalMessageID, phoneKey string) (bool, error) {
	if s.rateLimiter == nil {
		s.logWarn(ctx, "whatsappagent: rate limiter unavailable; dedupe disabled", "phone", phoneKey, "external_message_id", externalMessageID)
		return true, nil
	}
	claimed, err := s.rateLimiter.ClaimMessage(ctx, externalMessageID)
	if err != nil {
		s.logWarn(ctx, "whatsappagent: message dedupe unavailable; continuing fail-open", "phone", phoneKey, "external_message_id", externalMessageID, "error", err)
		return true, err
	}
	if !claimed {
		s.logInfo(ctx, "whatsappagent: duplicate inbound message ignored", "phone", phoneKey, "external_message_id", externalMessageID)
	}
	return claimed, nil
}

func (s *Service) allowInboundMessage(ctx context.Context, phoneKey, externalMessageID string) bool {
	if s.rateLimiter == nil {
		s.logWarn(ctx, "whatsappagent: rate limiter unavailable; throttling disabled", "phone", phoneKey, "external_message_id", externalMessageID)
		return true
	}
	allowed, err := s.rateLimiter.Allow(ctx, phoneKey)
	if err != nil {
		s.logWarn(ctx, "whatsappagent: rate limiter unavailable; continuing fail-open", "phone", phoneKey, "external_message_id", externalMessageID, "error", err)
		return true
	}
	if !allowed {
		s.logInfo(ctx, "whatsappagent: inbound message rate limited", "phone", phoneKey, "external_message_id", externalMessageID)
	}
	return allowed
}

func (s *Service) persistInboundMessage(ctx context.Context, orgID uuid.UUID, phoneKey, content string, inbound *CurrentInboundMessage) error {
	return s.queries.InsertAgentMessage(ctx, whatsappagentdb.InsertAgentMessageParams{
		OrganizationID:    pgtypeUUID(orgID),
		PhoneNumber:       phoneKey,
		Role:              "user",
		Content:           content,
		ExternalMessageID: optionalPgText(inboundMessageID(inbound)),
		Metadata:          inboundMetadata(inbound),
	})
}

func (s *Service) resetConversation(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string) {
	if err := s.queries.DeleteAgentMessagesByPhone(ctx, whatsappagentdb.DeleteAgentMessagesByPhoneParams{
		OrganizationID: pgtypeUUID(orgID),
		PhoneNumber:    phoneKey,
	}); err != nil {
		s.logError(ctx, "whatsappagent: failed to reset conversation history", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgSystemUnavailable)
		return
	}
	if s.leadHintStore != nil {
		s.leadHintStore.Clear(ctx, orgID.String(), phoneKey)
	}
	s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgConversationReset)
}

func (s *Service) sendHardcoded(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, incomingText, reply string) {
	if orgID != uuid.Nil {
		_ = s.queries.InsertAgentMessage(ctx, whatsappagentdb.InsertAgentMessageParams{
			OrganizationID:    pgtypeUUID(orgID),
			PhoneNumber:       phoneKey,
			Role:              "user",
			Content:           incomingText,
			ExternalMessageID: pgtype.Text{},
			Metadata:          nil,
		})
		_ = s.queries.InsertAgentMessage(ctx, whatsappagentdb.InsertAgentMessageParams{
			OrganizationID:    pgtypeUUID(orgID),
			PhoneNumber:       phoneKey,
			Role:              "assistant",
			Content:           reply,
			ExternalMessageID: pgtype.Text{},
			Metadata:          nil,
		})
	}
	if err := s.sender.SendReply(ctx, orgID, replyTarget, reply); err != nil {
		s.logError(ctx, "whatsappagent: hardcoded send error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
	}
}

func (s *Service) sendAssistantReply(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, reply string) {
	if err := s.queries.InsertAgentMessage(ctx, whatsappagentdb.InsertAgentMessageParams{
		OrganizationID:    pgtypeUUID(orgID),
		PhoneNumber:       phoneKey,
		Role:              "assistant",
		Content:           reply,
		ExternalMessageID: pgtype.Text{},
		Metadata:          nil,
	}); err != nil {
		s.logError(ctx, "whatsappagent: failed to persist assistant message", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
	}
	if err := s.sender.SendReply(ctx, orgID, replyTarget, reply); err != nil {
		s.logError(ctx, "whatsappagent: send reply error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
	}
}

func (s *Service) sendAudioFallback(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string, inbound CurrentInboundMessage) {
	s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgVoiceUnavailable)
	updated := mergeVoiceTranscriptionMetadata(inbound.Metadata, voiceTranscriptionUpdate{Status: "failed", ErrorMessage: msgVoiceUnavailable})
	if strings.TrimSpace(inbound.ExternalMessageID) != "" {
		if err := s.queries.UpdateAgentMessageByExternalID(ctx, whatsappagentdb.UpdateAgentMessageByExternalIDParams{
			OrganizationID:    pgtypeUUID(orgID),
			ExternalMessageID: optionalPgText(inbound.ExternalMessageID),
			Content:           voiceMessagePlaceholder,
			Metadata:          updated,
		}); err != nil {
			s.logWarn(ctx, "whatsappagent: failed to update failed audio metadata", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		}
		if err := s.updateInboundInboxMessage(ctx, orgID, inbound.ExternalMessageID, voiceMessagePlaceholder, updated); err != nil {
			s.logWarn(ctx, "whatsappagent: failed to update failed mirrored inbox message", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
		}
	}
}

func (s *Service) failVoiceTranscription(ctx context.Context, orgID uuid.UUID, externalMessageID string, reason string, retryable bool) {
	if strings.TrimSpace(externalMessageID) == "" {
		return
	}
	if err := s.queries.MarkAgentVoiceTranscriptionFailed(ctx, whatsappagentdb.MarkAgentVoiceTranscriptionFailedParams{
		OrganizationID:    pgtypeUUID(orgID),
		ExternalMessageID: strings.TrimSpace(externalMessageID),
		ErrorMessage:      pgtype.Text{String: truncateReason(reason), Valid: strings.TrimSpace(reason) != ""},
	}); err != nil {
		s.logWarn(ctx, "whatsappagent: failed to mark audio transcription failure", "organization_id", orgID.String(), "external_message_id", externalMessageID, "error", err)
	}
	if !retryable {
		message, err := s.queries.GetAgentMessageByExternalID(ctx, whatsappagentdb.GetAgentMessageByExternalIDParams{
			OrganizationID:    pgtypeUUID(orgID),
			ExternalMessageID: optionalPgText(externalMessageID),
		})
		if err == nil {
			metadata := mergeVoiceTranscriptionMetadata(message.Metadata, voiceTranscriptionUpdate{Status: "failed", ErrorMessage: reason})
			_ = s.queries.UpdateAgentMessageByExternalID(ctx, whatsappagentdb.UpdateAgentMessageByExternalIDParams{
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
	return trimmed[:max] + "..."
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

func pgtypeUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
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

func (s *Service) updateInboundInboxMessage(ctx context.Context, orgID uuid.UUID, externalMessageID, body string, metadata []byte) error {
	if s.inboxSync == nil || strings.TrimSpace(externalMessageID) == "" {
		return nil
	}
	return s.inboxSync.UpdateIncomingWhatsAppMessage(ctx, orgID, externalMessageID, body, metadata)
}

func (s *Service) getConversationLock(phoneKey string) *sync.Mutex {
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()
	if s.conversationLocks == nil {
		s.conversationLocks = make(map[string]*sync.Mutex)
	}
	lock, ok := s.conversationLocks[phoneKey]
	if !ok {
		lock = &sync.Mutex{}
		s.conversationLocks[phoneKey] = lock
	}
	return lock
}

func (s *Service) runAgentReply(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget string, inbound *CurrentInboundMessage, partnerID *uuid.UUID, reason string) {
	// Serialize agent runs per phone number so that only one reply is generated
	// at a time for a given conversation. This prevents race conditions where a
	// slow earlier run finishes after a newer run and sends a stale reply.
	lock := s.getConversationLock(phoneKey)
	lock.Lock()
	defer lock.Unlock()

	recent, err := s.queries.GetRecentAgentMessages(ctx, whatsappagentdb.GetRecentAgentMessagesParams{
		OrganizationID: pgtypeUUID(orgID),
		PhoneNumber:    phoneKey,
		Limit:          recentMessageLimit,
	})
	if err != nil {
		s.logWarn(ctx, "whatsappagent: failed to load history", "phone", phoneKey, "organization_id", orgID.String(), "error", err)
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
	messages = engine.MergeTrailingUserMessages(messages)

	// Clear SentAt on all messages to prevent double timestamp formatting.
	// MergeTrailingUserMessages already adds timestamps to content when needed.
	for i := range messages {
		messages[i].SentAt = nil
	}

	// Ensure the inbound message is always included in the conversation.
	// There can be a race condition where the DB query doesn't see the message yet.
	inboundBody := strings.TrimSpace(inbound.Body)
	if inboundBody != "" && !messageContentExists(messages, inboundBody) {
		messages = append(messages, ConversationMessage{
			Role:    "user",
			Content: inboundBody,
			SentAt:  nil,
		})
	}

	if err := s.sender.SendChatPresence(ctx, replyTarget, whatsapp.ChatPresenceComposing); err != nil {
		s.logWarn(ctx, "whatsappagent: send chat presence start error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
	}
	defer func() {
		if err := s.sender.SendChatPresence(context.Background(), replyTarget, whatsapp.ChatPresencePaused); err != nil {
			s.logWarn(context.Background(), "whatsappagent: send chat presence stop error", "phone", replyTarget, "organization_id", orgID.String(), "error", err)
		}
	}()

	leadHint := s.resolveLeadHint(ctx, orgID, phoneKey)
	inboundBridge := engine.CurrentInboundMessage(*inbound)
	var runResult AgentRunResult
	if partnerID != nil {
		s.logInfo(ctx, "whatsappagent: agent mode selected", "phone", phoneKey, "organization_id", orgID.String(), "mode", "partner", "reason", "partner")
		runResult, err = s.replyRunner.RunPreparedPartnerAgent(ctx, orgID, phoneKey, messages, toLegacyConversationLeadHint(leadHint), &inboundBridge, *partnerID)
	} else {
		s.logInfo(ctx, "whatsappagent: agent mode selected", "phone", phoneKey, "organization_id", orgID.String(), "mode", "default", "reason", reason)
		runResult, err = s.replyRunner.RunPreparedDefaultAgent(ctx, orgID, phoneKey, messages, toLegacyConversationLeadHint(leadHint), &inboundBridge)
	}
	if err != nil {
		s.logError(ctx, "whatsappagent: agent run error", "phone", phoneKey, "organization_id", orgID.String(), "reason", reason, "error", err)
		s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, msgSystemUnavailable)
		return
	}

	reply := sanitizeWhatsAppReply(strings.TrimSpace(runResult.Reply))
	if reply == "" {
		s.logWarn(ctx, "whatsappagent: empty agent reply", "phone", phoneKey, "organization_id", orgID.String(), "reason", reason)
		return
	}
	s.sendAssistantReply(ctx, orgID, phoneKey, replyTarget, strings.TrimSpace(reply))
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
		s.logWarn(ctx, "whatsappagent: failed to mirror inbound inbox message", "organization_id", orgID.String(), "phone", phoneNumber, "error", err)
	}
}

func uuidFromPgtype(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

func partnerIDFromAgentUser(user whatsappagentdb.GetAgentUserByPhoneRow) (uuid.UUID, bool) {
	if strings.TrimSpace(user.UserType) != "partner" || !user.PartnerID.Valid {
		return uuid.Nil, false
	}
	return uuid.UUID(user.PartnerID.Bytes), true
}

func shouldSkipReplayedMessage(role, content string) bool {
	trimmed := strings.TrimSpace(content)
	if role == "assistant" {
		if trimmed == msgVoiceUnavailable || trimmed == msgSystemUnavailable || trimmed == msgGroundingFallback || trimmed == msgConversationReset {
			return true
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "ik kan die klantgegevens nu niet betrouwbaar bevestigen") {
			return true
		}
	}
	return false
}

func (s *Service) resolveLeadHint(ctx context.Context, orgID uuid.UUID, phoneKey string) *ConversationLeadHint {
	if s.leadHintStore == nil {
		return nil
	}
	hint, _ := s.leadHintStore.Get(ctx, orgID.String(), phoneKey)
	return hint
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

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func toLegacyConversationLeadHint(hint *ConversationLeadHint) *engine.ConversationLeadHint {
	if hint == nil {
		return nil
	}
	converted := &engine.ConversationLeadHint{
		LeadID:        hint.LeadID,
		LeadServiceID: hint.LeadServiceID,
		CustomerName:  hint.CustomerName,
		UpdatedAt:     hint.UpdatedAt,
	}
	if len(hint.RecentQuotes) > 0 {
		converted.RecentQuotes = make([]engine.RecentQuoteHint, 0, len(hint.RecentQuotes))
		for _, item := range hint.RecentQuotes {
			converted.RecentQuotes = append(converted.RecentQuotes, engine.RecentQuoteHint(item))
		}
	}
	if len(hint.RecentAppointments) > 0 {
		converted.RecentAppointments = make([]engine.RecentAppointmentHint, 0, len(hint.RecentAppointments))
		for _, item := range hint.RecentAppointments {
			converted.RecentAppointments = append(converted.RecentAppointments, engine.RecentAppointmentHint(item))
		}
	}
	if hint.PreloadedDetails != nil {
		details := engine.LeadDetailsResult(*hint.PreloadedDetails)
		converted.PreloadedDetails = &details
	}
	return converted
}

func agentPhoneCandidates(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, 4)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}

	add(trimmed)
	normalized := normalizeAgentPhoneKey(trimmed)
	add(normalized)
	add(strings.TrimPrefix(normalized, "+"))
	if local, _, ok := strings.Cut(trimmed, "@"); ok {
		add(local)
		add(normalizeAgentPhoneKey(local))
	}

	return result
}

// messageContentExists checks if a message with the given content already exists
// in the conversation history (case-insensitive).
func messageContentExists(messages []ConversationMessage, content string) bool {
	contentLower := strings.ToLower(strings.TrimSpace(content))
	for _, msg := range messages {
		if strings.ToLower(strings.TrimSpace(msg.Content)) == contentLower {
			return true
		}
	}
	return false
}

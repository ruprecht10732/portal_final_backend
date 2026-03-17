package whatsappagent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/internal/whatsappagent/engine"
	"portal_final_backend/platform/logger"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

const serviceWrapperTestPhone = "+31612345678"
const serviceWrapperMiniredisStartFailedMsg = "failed to start miniredis: %v"

type serviceWrapperTestReplyRunner struct {
	defaultCalled bool
	partnerCalled bool
	orgID         uuid.UUID
	phoneKey      string
	replyTarget   string
	inbound       *engine.CurrentInboundMessage
	partnerID     uuid.UUID
	messages      []ConversationMessage
	leadHint      *engine.ConversationLeadHint
	result        AgentRunResult
	err           error
}

func (r *serviceWrapperTestReplyRunner) RunPreparedDefaultAgent(_ context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *engine.ConversationLeadHint, inbound *engine.CurrentInboundMessage) (AgentRunResult, error) {
	r.orgID = orgID
	r.phoneKey = phoneKey
	r.messages = append([]ConversationMessage(nil), messages...)
	if leadHint != nil {
		copyHint := *leadHint
		r.leadHint = &copyHint
	}
	if inbound != nil {
		copyInbound := *inbound
		r.inbound = &copyInbound
	}
	r.defaultCalled = true
	return r.result, r.err
}

func (r *serviceWrapperTestReplyRunner) RunPreparedPartnerAgent(_ context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *engine.ConversationLeadHint, inbound *engine.CurrentInboundMessage, partnerID uuid.UUID) (AgentRunResult, error) {
	r.orgID = orgID
	r.phoneKey = phoneKey
	r.partnerID = partnerID
	r.messages = append([]ConversationMessage(nil), messages...)
	if leadHint != nil {
		copyHint := *leadHint
		r.leadHint = &copyHint
	}
	if inbound != nil {
		copyInbound := *inbound
		r.inbound = &copyInbound
	}
	r.partnerCalled = true
	return r.result, r.err
}

type serviceWrapperTestQueries struct {
	transcriptionLookupErr error
	lookupErr              error
	lookupUser             whatsappagentdb.GetAgentUserByPhoneRow
	messageRow             whatsappagentdb.GetAgentMessageByExternalIDRow
	processingCalls        int
	lookupCalls            int
	deleteCalls            int
	upsertVoiceCalls       int
	recent                 []whatsappagentdb.GetRecentAgentMessagesRow
	recentArgs             []whatsappagentdb.GetRecentAgentMessagesParams
	storageUpdates         []whatsappagentdb.UpdateAgentVoiceTranscriptionStorageParams
	messageUpdates         []whatsappagentdb.UpdateAgentMessageByExternalIDParams
	completed              []whatsappagentdb.MarkAgentVoiceTranscriptionCompletedParams
	failed                 []whatsappagentdb.MarkAgentVoiceTranscriptionFailedParams
	inserted               []whatsappagentdb.InsertAgentMessageParams
}

func (q *serviceWrapperTestQueries) DeleteAgentMessagesByPhone(context.Context, whatsappagentdb.DeleteAgentMessagesByPhoneParams) error {
	q.deleteCalls++
	return nil
}

func (q *serviceWrapperTestQueries) GetAgentMessageByExternalID(context.Context, whatsappagentdb.GetAgentMessageByExternalIDParams) (whatsappagentdb.GetAgentMessageByExternalIDRow, error) {
	return q.messageRow, nil
}

func (q *serviceWrapperTestQueries) GetAgentUserByPhone(context.Context, string) (whatsappagentdb.GetAgentUserByPhoneRow, error) {
	q.lookupCalls++
	if q.lookupErr != nil {
		return whatsappagentdb.GetAgentUserByPhoneRow{}, q.lookupErr
	}
	return q.lookupUser, nil
}

func (q *serviceWrapperTestQueries) GetRecentAgentMessages(_ context.Context, arg whatsappagentdb.GetRecentAgentMessagesParams) ([]whatsappagentdb.GetRecentAgentMessagesRow, error) {
	q.recentArgs = append(q.recentArgs, arg)
	return q.recent, nil
}

func (q *serviceWrapperTestQueries) GetAgentVoiceTranscriptionByExternalID(context.Context, whatsappagentdb.GetAgentVoiceTranscriptionByExternalIDParams) (whatsappagentdb.RacWhatsappAgentVoiceTranscription, error) {
	if q.transcriptionLookupErr != nil {
		return whatsappagentdb.RacWhatsappAgentVoiceTranscription{}, q.transcriptionLookupErr
	}
	return whatsappagentdb.RacWhatsappAgentVoiceTranscription{}, nil
}

func (q *serviceWrapperTestQueries) InsertAgentMessage(_ context.Context, arg whatsappagentdb.InsertAgentMessageParams) error {
	q.inserted = append(q.inserted, arg)
	return nil
}

func (q *serviceWrapperTestQueries) MarkAgentVoiceTranscriptionCompleted(_ context.Context, arg whatsappagentdb.MarkAgentVoiceTranscriptionCompletedParams) error {
	q.completed = append(q.completed, arg)
	return nil
}

func (q *serviceWrapperTestQueries) MarkAgentVoiceTranscriptionFailed(_ context.Context, arg whatsappagentdb.MarkAgentVoiceTranscriptionFailedParams) error {
	q.failed = append(q.failed, arg)
	return nil
}

func (q *serviceWrapperTestQueries) MarkAgentVoiceTranscriptionProcessing(context.Context, whatsappagentdb.MarkAgentVoiceTranscriptionProcessingParams) error {
	q.processingCalls++
	return nil
}

func (q *serviceWrapperTestQueries) UpdateAgentMessageByExternalID(_ context.Context, arg whatsappagentdb.UpdateAgentMessageByExternalIDParams) error {
	q.messageUpdates = append(q.messageUpdates, arg)
	return nil
}

func (q *serviceWrapperTestQueries) UpdateAgentVoiceTranscriptionStorage(_ context.Context, arg whatsappagentdb.UpdateAgentVoiceTranscriptionStorageParams) error {
	q.storageUpdates = append(q.storageUpdates, arg)
	return nil
}

func (q *serviceWrapperTestQueries) UpsertAgentVoiceTranscription(context.Context, whatsappagentdb.UpsertAgentVoiceTranscriptionParams) error {
	q.upsertVoiceCalls++
	return nil
}

type serviceWrapperTestTransport struct {
	downloadResult whatsapp.DownloadMediaFileResult
}

func (t *serviceWrapperTestTransport) SendMessage(context.Context, string, string, string) (whatsapp.SendResult, error) {
	return whatsapp.SendResult{}, nil
}

func (t *serviceWrapperTestTransport) SendChatPresence(context.Context, string, string, string) error {
	return nil
}

func (t *serviceWrapperTestTransport) SendFile(context.Context, string, whatsapp.SendFileInput) (whatsapp.SendResult, error) {
	return whatsapp.SendResult{}, nil
}

func (t *serviceWrapperTestTransport) DownloadMediaFile(context.Context, string, string, string, ...string) (whatsapp.DownloadMediaFileResult, error) {
	return t.downloadResult, nil
}

type serviceWrapperTestStorage struct {
	uploadedKey string
}

func (s serviceWrapperTestStorage) DownloadFile(context.Context, string, string) (io.ReadCloser, error) {
	return nil, nil
}

func (s serviceWrapperTestStorage) UploadFile(context.Context, string, string, string, string, io.Reader, int64) (string, error) {
	return s.uploadedKey, nil
}

func (s serviceWrapperTestStorage) ValidateContentType(string) error { return nil }
func (s serviceWrapperTestStorage) ValidateFileSize(int64) error     { return nil }

type serviceWrapperTestTranscriber struct{}

func (serviceWrapperTestTranscriber) Name() string { return "test-transcriber" }

func (serviceWrapperTestTranscriber) Transcribe(context.Context, AudioTranscriptionInput) (AudioTranscriptionResult, error) {
	confidence := 0.98
	return AudioTranscriptionResult{Text: "Hallo vanaf audio", Language: "nl", Confidence: &confidence}, nil
}

func TestServiceProcessVoiceTranscriptionRunsExtractedFlow(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceWrapperTestQueries{
		transcriptionLookupErr: pgx.ErrNoRows,
		messageRow: whatsappagentdb.GetAgentMessageByExternalIDRow{
			Content: "[Spraakbericht]",
		},
	}
	replyRunner := &serviceWrapperTestReplyRunner{}
	transport := &serviceWrapperTestTransport{downloadResult: whatsapp.DownloadMediaFileResult{
		DownloadMediaResult: whatsapp.DownloadMediaResult{Filename: "voice.ogg", MediaType: "audio"},
		ContentType:         "application/ogg",
		Data:                []byte("audio-bytes"),
	}}
	service := &Service{
		queries:          queries,
		replyRunner:      replyRunner,
		sender:           &Sender{client: transport, queries: senderTestConfigReader{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}, log: logger.New("development")},
		storage:          serviceWrapperTestStorage{uploadedKey: "voice/key.ogg"},
		attachmentBucket: "attachments",
		transcriber:      serviceWrapperTestTranscriber{},
		log:              logger.New("development"),
	}
	replyRunner.result = AgentRunResult{Reply: "Dank, ik heb je spraakbericht verwerkt."}

	err := service.ProcessVoiceTranscription(context.Background(), scheduler.WAAgentVoiceTranscriptionPayload{
		OrganizationID:    orgID.String(),
		PhoneNumber:       testSenderPhone,
		ExternalMessageID: "voice-msg-1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if queries.processingCalls != 1 {
		t.Fatalf("expected processing mark once, got %d", queries.processingCalls)
	}
	if len(queries.storageUpdates) != 1 {
		t.Fatalf("expected one storage update, got %d", len(queries.storageUpdates))
	}
	if len(queries.messageUpdates) != 1 {
		t.Fatalf("expected one message update, got %d", len(queries.messageUpdates))
	}
	if !strings.Contains(queries.messageUpdates[0].Content, "Spraakbericht: Hallo vanaf audio") {
		t.Fatalf("expected transcript content update, got %q", queries.messageUpdates[0].Content)
	}
	if len(queries.completed) != 1 {
		t.Fatalf("expected one completion mark, got %d", len(queries.completed))
	}
	if !replyRunner.defaultCalled {
		t.Fatal("expected prepared default agent runner to be called")
	}
	if replyRunner.orgID != orgID {
		t.Fatalf("expected org %q, got %q", orgID, replyRunner.orgID)
	}
	if replyRunner.phoneKey != testSenderPhone {
		t.Fatalf("expected normalized phone key %q, got %q", testSenderPhone, replyRunner.phoneKey)
	}
	if replyRunner.inbound == nil || !strings.Contains(replyRunner.inbound.Body, "Hallo vanaf audio") {
		t.Fatalf("expected bridged inbound transcript, got %#v", replyRunner.inbound)
	}
}

func TestServiceProcessVoiceTranscriptionReturnsErrorWhenDependenciesMissing(t *testing.T) {
	t.Parallel()

	service := &Service{}
	err := service.ProcessVoiceTranscription(context.Background(), scheduler.WAAgentVoiceTranscriptionPayload{})
	if err == nil || !strings.Contains(err.Error(), "voice transcription dependencies are not configured") {
		t.Fatalf("expected dependency error, got %v", err)
	}
}

func TestServiceHandleIncomingMessageIgnoresDuplicateBeforeLookup(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(serviceWrapperMiniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	queries := &serviceWrapperTestQueries{lookupErr: pgx.ErrNoRows}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	service := &Service{
		queries:     queries,
		replyRunner: &serviceWrapperTestReplyRunner{},
		sender:      newTestSender(transport, senderTestConfigReader{}, nil),
		rateLimiter: engine.NewRateLimiter(client, logger.New("development")),
		log:         logger.New("development"),
	}
	inbound := CurrentInboundMessage{ExternalMessageID: "msg-1", PhoneNumber: serviceWrapperTestPhone, Body: "hallo"}

	service.HandleIncomingMessage(context.Background(), inbound)
	firstLookups := queries.lookupCalls
	if firstLookups == 0 {
		t.Fatal("expected first inbound message to perform lookup")
	}

	service.HandleIncomingMessage(context.Background(), inbound)
	if queries.lookupCalls != firstLookups {
		t.Fatalf("expected duplicate inbound message to skip lookup, got %d calls after duplicate vs %d before", queries.lookupCalls, firstLookups)
	}
}

func TestServiceHandleIncomingMessageRateLimitedBeforeLookup(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(serviceWrapperMiniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	queries := &serviceWrapperTestQueries{lookupErr: pgx.ErrNoRows}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	service := &Service{
		queries:     queries,
		replyRunner: &serviceWrapperTestReplyRunner{},
		sender:      newTestSender(transport, senderTestConfigReader{}, nil),
		rateLimiter: engine.NewRateLimiter(client, logger.New("development")),
		log:         logger.New("development"),
	}
	ctx := context.Background()
	phoneKey := normalizeAgentPhoneKey(serviceWrapperTestPhone)
	for i := 0; i < 30; i++ {
		allowed, allowErr := service.rateLimiter.Allow(ctx, phoneKey)
		if allowErr != nil {
			t.Fatalf("preload allow returned error: %v", allowErr)
		}
		if !allowed {
			t.Fatalf("expected preload call %d to stay under limit", i+1)
		}
	}

	service.HandleIncomingMessage(ctx, CurrentInboundMessage{ExternalMessageID: "msg-2", PhoneNumber: serviceWrapperTestPhone, Body: "hallo"})
	if queries.lookupCalls != 0 {
		t.Fatalf("expected rate-limited message to skip lookup, got %d lookups", queries.lookupCalls)
	}
	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected hardcoded rate-limit reply to be sent")
	}
}

func TestServiceHandleIncomingMessageRegisteredSenderRunsTextBridge(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(serviceWrapperMiniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	orgID := uuid.New()
	queries := &serviceWrapperTestQueries{lookupUser: whatsappagentdb.GetAgentUserByPhoneRow{OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true}}}
	replyRunner := &serviceWrapperTestReplyRunner{}
	replyRunner.result = AgentRunResult{Reply: "Alles goed"}
	service := &Service{
		queries:     queries,
		replyRunner: replyRunner,
		sender:      newTestSender(&senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}, senderTestConfigReader{}, nil),
		rateLimiter: engine.NewRateLimiter(client, logger.New("development")),
		log:         logger.New("development"),
	}

	inbound := CurrentInboundMessage{ExternalMessageID: "msg-registered-default", PhoneNumber: serviceWrapperTestPhone, Body: "Kun je mij helpen?"}
	service.HandleIncomingMessage(context.Background(), inbound)

	if queries.lookupCalls == 0 {
		t.Fatal("expected registered sender to be looked up")
	}
	if !replyRunner.defaultCalled {
		t.Fatal("expected default reply bridge to be called")
	}
	if replyRunner.orgID != orgID {
		t.Fatalf("expected organization %q, got %q", orgID, replyRunner.orgID)
	}
	if replyRunner.phoneKey != serviceWrapperTestPhone {
		t.Fatalf("expected normalized phone key %q, got %q", serviceWrapperTestPhone, replyRunner.phoneKey)
	}
	if replyRunner.inbound == nil || replyRunner.inbound.ExternalMessageID != inbound.ExternalMessageID {
		t.Fatalf("expected bridged inbound payload, got %#v", replyRunner.inbound)
	}
	if len(queries.inserted) != 2 || queries.inserted[0].Content != inbound.Body || queries.inserted[1].Content != "Alles goed" {
		t.Fatalf("expected inbound message to be persisted before bridge, got %#v", queries.inserted)
	}
}

func TestServiceHandleIncomingMessageResetClearsConversation(t *testing.T) {
	t.Parallel()

	store := NewConversationLeadHintStore()
	orgID := uuid.New()
	phoneKey := normalizeAgentPhoneKey(serviceWrapperTestPhone)
	store.Set(orgID.String(), phoneKey, ConversationLeadHint{LeadID: uuid.New().String(), CustomerName: "Robin"})
	queries := &serviceWrapperTestQueries{lookupUser: whatsappagentdb.GetAgentUserByPhoneRow{OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true}}}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	service := &Service{
		queries:       queries,
		replyRunner:   &serviceWrapperTestReplyRunner{},
		sender:        newTestSender(transport, senderTestConfigReader{}, nil),
		leadHintStore: store,
		log:           logger.New("development"),
	}

	service.HandleIncomingMessage(context.Background(), CurrentInboundMessage{ExternalMessageID: "msg-reset", PhoneNumber: serviceWrapperTestPhone, Body: "/reset"})

	if queries.deleteCalls != 1 {
		t.Fatalf("expected one conversation delete, got %d", queries.deleteCalls)
	}
	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected reset reply to be sent")
	}
	if hint, ok := store.Get(orgID.String(), phoneKey); ok || hint != nil {
		t.Fatalf("expected lead hint to be cleared, got %#v", hint)
	}
}

func TestServiceHandleIncomingMessageContinuesWhenRedisUnavailable(t *testing.T) {
	t.Parallel()

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 5 * time.Millisecond, ReadTimeout: 5 * time.Millisecond, WriteTimeout: 5 * time.Millisecond})
	defer func() { _ = client.Close() }()

	orgID := uuid.New()
	queries := &serviceWrapperTestQueries{lookupUser: whatsappagentdb.GetAgentUserByPhoneRow{OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true}}}
	replyRunner := &serviceWrapperTestReplyRunner{}
	replyRunner.result = AgentRunResult{Reply: "Doorgaan"}
	service := &Service{queries: queries, replyRunner: replyRunner, sender: newTestSender(&senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}, senderTestConfigReader{}, nil), rateLimiter: engine.NewRateLimiter(client, logger.New("development")), log: logger.New("development")}

	service.HandleIncomingMessage(context.Background(), CurrentInboundMessage{ExternalMessageID: "msg-3", PhoneNumber: serviceWrapperTestPhone, Body: "hallo"})
	if queries.lookupCalls == 0 {
		t.Fatal("expected service to continue to phone lookup when redis is unavailable")
	}
	if !replyRunner.defaultCalled {
		t.Fatal("expected redis-unavailable path to still reach default reply bridge")
	}
}

func TestServiceHandleIncomingMessageRegisteredPartnerRunsPartnerBridge(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(serviceWrapperMiniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	orgID := uuid.New()
	partnerID := uuid.New()
	queries := &serviceWrapperTestQueries{lookupUser: whatsappagentdb.GetAgentUserByPhoneRow{
		OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true},
		UserType:       "partner",
		PartnerID:      pgtype.UUID{Bytes: partnerID, Valid: true},
	}}
	replyRunner := &serviceWrapperTestReplyRunner{}
	replyRunner.result = AgentRunResult{Reply: "Partner antwoord"}
	service := &Service{
		queries:     queries,
		replyRunner: replyRunner,
		sender:      newTestSender(&senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}, senderTestConfigReader{}, nil),
		rateLimiter: engine.NewRateLimiter(client, logger.New("development")),
		log:         logger.New("development"),
	}

	inbound := CurrentInboundMessage{ExternalMessageID: "msg-registered-partner", PhoneNumber: serviceWrapperTestPhone, Body: "Kun je mij helpen?"}
	service.HandleIncomingMessage(context.Background(), inbound)

	if !replyRunner.partnerCalled {
		t.Fatal("expected partner reply bridge to be called")
	}
	if replyRunner.partnerID != partnerID {
		t.Fatalf("expected partner id %q, got %q", partnerID, replyRunner.partnerID)
	}
	if len(queries.inserted) != 2 || queries.inserted[0].Content != inbound.Body || queries.inserted[1].Content != "Partner antwoord" {
		t.Fatalf("expected inbound message to be persisted before partner bridge, got %#v", queries.inserted)
	}
}

func TestServiceRunAgentReplySendsSystemFallbackWhenRunnerFails(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceWrapperTestQueries{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	replyRunner := &serviceWrapperTestReplyRunner{err: errors.New("runner timeout")}
	service := &Service{
		queries:     queries,
		replyRunner: replyRunner,
		sender:      newTestSender(transport, senderTestConfigReader{}, nil),
		log:         logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-error", PhoneNumber: serviceWrapperTestPhone, Body: "Welke afspraken zijn er?"}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(serviceWrapperTestPhone), serviceWrapperTestPhone, inbound, nil, "default")

	if len(queries.inserted) != 1 || queries.inserted[0].Content != msgSystemUnavailable {
		t.Fatalf("expected system fallback to be persisted, got %#v", queries.inserted)
	}
	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected system fallback to be sent")
	}
}

func TestServiceRunAgentReplySkipsPoisonedAssistantHistory(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceWrapperTestQueries{recent: []whatsappagentdb.GetRecentAgentMessagesRow{
		{Role: "assistant", Content: msgGroundingFallback},
		{Role: "user", Content: "Heeft u mijn offerte?"},
	}}
	replyRunner := &serviceWrapperTestReplyRunner{result: AgentRunResult{Reply: "Prima"}}
	service := &Service{
		queries:     queries,
		replyRunner: replyRunner,
		sender:      newTestSender(&senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}, senderTestConfigReader{}, nil),
		log:         logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-clean", PhoneNumber: serviceWrapperTestPhone, Body: "Kun je helpen?"}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(serviceWrapperTestPhone), serviceWrapperTestPhone, inbound, nil, "default")

	if len(replyRunner.messages) != 1 || replyRunner.messages[0].Content != "Heeft u mijn offerte?" {
		t.Fatalf("expected poisoned assistant reply to be filtered, got %#v", replyRunner.messages)
	}
}

func TestServiceRunAgentReplyPassesLeadHintToPreparedRunner(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	store := NewConversationLeadHintStore()
	phoneKey := normalizeAgentPhoneKey(serviceWrapperTestPhone)
	store.Set(orgID.String(), phoneKey, ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})
	replyRunner := &serviceWrapperTestReplyRunner{result: AgentRunResult{Reply: "Prima"}}
	service := &Service{
		queries:       &serviceWrapperTestQueries{},
		replyRunner:   replyRunner,
		sender:        newTestSender(&senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}, senderTestConfigReader{}, nil),
		leadHintStore: store,
		log:           logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-hint", PhoneNumber: serviceWrapperTestPhone, Body: "Zoek Robin"}

	service.runAgentReply(context.Background(), orgID, phoneKey, serviceWrapperTestPhone, inbound, nil, "default")

	if replyRunner.leadHint == nil || replyRunner.leadHint.LeadID != testHintLeadID {
		t.Fatalf("expected lead hint to reach prepared runner, got %#v", replyRunner.leadHint)
	}
}

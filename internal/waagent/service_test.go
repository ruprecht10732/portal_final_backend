package waagent

import (
	"context"
	"testing"
	"time"

	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/logger"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	waagentdb "portal_final_backend/internal/waagent/db"
)

const testAgentPhone = "+31612345678"

const (
	testExpectedAssistantPersistCountMsg = "expected one assistant message to be persisted, got %d"
	testQuoteRequestMessage              = "Heeft u mijn offerte?"
)

type serviceTestLeadDetailsReader struct {
	calls int
}

func (r *serviceTestLeadDetailsReader) GetLeadDetails(context.Context, uuid.UUID, string) (*LeadDetailsResult, error) {
	r.calls++
	return &LeadDetailsResult{LeadID: testHintLeadID, CustomerName: "Robin"}, nil
}

type serviceTestQuerier struct {
	lookupCalls int
	lookupErr   error
	lookupUser  waagentdb.RacWhatsappAgentUser
	recent      []waagentdb.GetRecentAgentMessagesRow
	inserted    []waagentdb.InsertAgentMessageParams
}

type serviceTestAgent struct {
	result           AgentRunResult
	err              error
	lastMessages     []ConversationMessage
	lastLeadHint     *ConversationLeadHint
	lastInbound      *CurrentInboundMessage
	lastPhoneKey     string
	lastOrganization uuid.UUID
}

func (a *serviceTestAgent) Run(_ context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *ConversationLeadHint, inboundMessage *CurrentInboundMessage) (AgentRunResult, error) {
	a.lastOrganization = orgID
	a.lastPhoneKey = phoneKey
	a.lastMessages = append([]ConversationMessage(nil), messages...)
	if leadHint != nil {
		copyHint := *leadHint
		a.lastLeadHint = &copyHint
	}
	if inboundMessage != nil {
		copyInbound := *inboundMessage
		a.lastInbound = &copyInbound
	}
	if a.err != nil {
		return AgentRunResult{}, a.err
	}
	return a.result, nil
}

func (q *serviceTestQuerier) CreateAgentUser(context.Context, waagentdb.CreateAgentUserParams) error {
	return nil
}
func (q *serviceTestQuerier) DeleteAgentConfig(context.Context) error { return nil }
func (q *serviceTestQuerier) DeleteAgentUser(context.Context, waagentdb.DeleteAgentUserParams) error {
	return nil
}
func (q *serviceTestQuerier) GetAgentConfig(context.Context) (waagentdb.RacWhatsappAgentConfig, error) {
	return waagentdb.RacWhatsappAgentConfig{}, nil
}
func (q *serviceTestQuerier) GetAgentConfigByDeviceID(context.Context, string) (waagentdb.RacWhatsappAgentConfig, error) {
	return waagentdb.RacWhatsappAgentConfig{}, nil
}
func (q *serviceTestQuerier) GetAgentMessageByExternalID(context.Context, waagentdb.GetAgentMessageByExternalIDParams) (waagentdb.GetAgentMessageByExternalIDRow, error) {
	return waagentdb.GetAgentMessageByExternalIDRow{}, pgx.ErrNoRows
}
func (q *serviceTestQuerier) GetAgentUserByPhone(context.Context, string) (waagentdb.RacWhatsappAgentUser, error) {
	q.lookupCalls++
	if q.lookupErr != nil {
		return waagentdb.RacWhatsappAgentUser{}, q.lookupErr
	}
	return q.lookupUser, nil
}
func (q *serviceTestQuerier) GetAgentVoiceTranscriptionByExternalID(context.Context, waagentdb.GetAgentVoiceTranscriptionByExternalIDParams) (waagentdb.RacWhatsappAgentVoiceTranscription, error) {
	return waagentdb.RacWhatsappAgentVoiceTranscription{}, pgx.ErrNoRows
}
func (q *serviceTestQuerier) GetRecentAgentMessages(context.Context, waagentdb.GetRecentAgentMessagesParams) ([]waagentdb.GetRecentAgentMessagesRow, error) {
	return q.recent, nil
}
func (q *serviceTestQuerier) GetRecentInboundAgentMessages(context.Context, waagentdb.GetRecentInboundAgentMessagesParams) ([]waagentdb.GetRecentInboundAgentMessagesRow, error) {
	return nil, nil
}
func (q *serviceTestQuerier) InsertAgentMessage(_ context.Context, params waagentdb.InsertAgentMessageParams) error {
	q.inserted = append(q.inserted, params)
	return nil
}
func (q *serviceTestQuerier) ListAgentUsersByOrganization(context.Context, pgtype.UUID) ([]waagentdb.RacWhatsappAgentUser, error) {
	return nil, nil
}
func (q *serviceTestQuerier) MarkAgentVoiceTranscriptionCompleted(context.Context, waagentdb.MarkAgentVoiceTranscriptionCompletedParams) error {
	return nil
}
func (q *serviceTestQuerier) MarkAgentVoiceTranscriptionFailed(context.Context, waagentdb.MarkAgentVoiceTranscriptionFailedParams) error {
	return nil
}
func (q *serviceTestQuerier) MarkAgentVoiceTranscriptionProcessing(context.Context, waagentdb.MarkAgentVoiceTranscriptionProcessingParams) error {
	return nil
}
func (q *serviceTestQuerier) UpdateAgentMessageByExternalID(context.Context, waagentdb.UpdateAgentMessageByExternalIDParams) error {
	return nil
}
func (q *serviceTestQuerier) UpdateAgentVoiceTranscriptionStorage(context.Context, waagentdb.UpdateAgentVoiceTranscriptionStorageParams) error {
	return nil
}
func (q *serviceTestQuerier) UpsertAgentConfig(context.Context, waagentdb.UpsertAgentConfigParams) (waagentdb.RacWhatsappAgentConfig, error) {
	return waagentdb.RacWhatsappAgentConfig{}, nil
}
func (q *serviceTestQuerier) UpsertAgentVoiceTranscription(context.Context, waagentdb.UpsertAgentVoiceTranscriptionParams) error {
	return nil
}

func newTestService(t *testing.T, redisClient *redis.Client, queries *serviceTestQuerier) *Service {
	t.Helper()
	return &Service{
		queries:     queries,
		sender:      &Sender{},
		rateLimiter: NewRateLimiter(redisClient, logger.New("development")),
		log:         logger.New("development"),
	}
}

type serviceTestConfigReader struct{}

func (serviceTestConfigReader) GetAgentConfig(context.Context) (waagentdb.RacWhatsappAgentConfig, error) {
	return waagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}, nil
}

func TestHandleIncomingMessageIgnoresDuplicateBeforeLookup(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	queries := &serviceTestQuerier{lookupErr: pgx.ErrNoRows}
	service := newTestService(t, client, queries)
	inbound := CurrentInboundMessage{ExternalMessageID: "msg-1", PhoneNumber: testAgentPhone, Body: "hallo"}

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

func TestHandleIncomingMessageRateLimitedBeforeLookup(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	queries := &serviceTestQuerier{lookupErr: pgx.ErrNoRows}
	service := newTestService(t, client, queries)
	ctx := context.Background()
	phoneKey := normalizeAgentPhoneKey(testAgentPhone)
	for i := 0; i < rateLimitMax; i++ {
		allowed, allowErr := service.rateLimiter.Allow(ctx, phoneKey)
		if allowErr != nil {
			t.Fatalf("preload allow returned error: %v", allowErr)
		}
		if !allowed {
			t.Fatalf("expected preload call %d to stay under limit", i+1)
		}
	}

	service.HandleIncomingMessage(ctx, CurrentInboundMessage{ExternalMessageID: "msg-2", PhoneNumber: testAgentPhone, Body: "hallo"})
	if queries.lookupCalls != 0 {
		t.Fatalf("expected rate-limited message to skip lookup, got %d lookups", queries.lookupCalls)
	}
}

func TestHandleIncomingMessageContinuesWhenRedisUnavailable(t *testing.T) {
	t.Parallel()

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 5 * time.Millisecond, ReadTimeout: 5 * time.Millisecond, WriteTimeout: 5 * time.Millisecond})
	defer func() { _ = client.Close() }()

	queries := &serviceTestQuerier{lookupErr: pgx.ErrNoRows, lookupUser: waagentdb.RacWhatsappAgentUser{OrganizationID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}}
	service := newTestService(t, client, queries)

	service.HandleIncomingMessage(context.Background(), CurrentInboundMessage{ExternalMessageID: "msg-3", PhoneNumber: testAgentPhone, Body: "hallo"})
	if queries.lookupCalls == 0 {
		t.Fatal("expected service to continue to phone lookup when redis is unavailable")
	}
}

func TestResolveLeadHintDoesNotAutoLoadDetails(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	store := newConversationLeadHintStore(time.Now, time.Hour, 10)
	store.Set(orgID.String(), testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})
	reader := &serviceTestLeadDetailsReader{}
	service := &Service{
		leadHintStore:     store,
		leadDetailsReader: reader,
	}

	hint := service.resolveLeadHint(context.Background(), orgID, testHintPhoneKey)
	if hint == nil {
		t.Fatal("expected hint to be returned")
	}
	if hint.PreloadedDetails != nil {
		t.Fatalf("expected no preloaded details on resolved hint, got %#v", hint.PreloadedDetails)
	}
	if reader.calls != 0 {
		t.Fatalf("expected resolveLeadHint not to load lead details, got %d calls", reader.calls)
	}
	if hint.CustomerName != "Robin" {
		t.Fatalf("expected stored customer name to remain intact, got %q", hint.CustomerName)
	}
}

func TestRunAgentReplyPersistsStatusFallbackWhenGroundingFails(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: "Ik kan die klantgegevens nu niet betrouwbaar bevestigen. Welke klant of welk dossier bedoelt u precies?", GroundingFailure: "lead_details_without_lead_tool"}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-status", PhoneNumber: testAgentPhone, Body: "Wat is mijn status?"}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound)

	if len(queries.inserted) != 1 {
		t.Fatalf(testExpectedAssistantPersistCountMsg, len(queries.inserted))
	}
	if queries.inserted[0].Content != "Ik kan die klantgegevens nu niet betrouwbaar bevestigen. Welke klant of welk dossier bedoelt u precies?" {
		t.Fatalf("unexpected persisted status fallback %q", queries.inserted[0].Content)
	}
	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected fallback reply to be sent")
	}
	if agent.lastInbound == nil || agent.lastInbound.Body != "Wat is mijn status?" {
		t.Fatalf("expected inbound message to be passed through, got %#v", agent.lastInbound)
	}
	if agent.lastPhoneKey != normalizeAgentPhoneKey(testAgentPhone) {
		t.Fatalf("unexpected phone key %q", agent.lastPhoneKey)
	}
	if agent.result.GroundingFailure != "lead_details_without_lead_tool" {
		t.Fatalf("expected grounding failure to remain visible, got %q", agent.result.GroundingFailure)
	}
}

func TestRunAgentReplyPersistsQuoteReplyFromValidatedAgentResult(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{recent: []waagentdb.GetRecentAgentMessagesRow{{Role: "user", Content: testQuoteRequestMessage}}}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: "*Offerte:* klaar\n*Bedrag:* € 125,00", ToolResponseNames: []string{"GetQuotes"}}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-quote", PhoneNumber: testAgentPhone, Body: testQuoteRequestMessage}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound)

	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected validated quote reply to be sent")
	}
	if len(agent.lastMessages) != 1 || agent.lastMessages[0].Content != testQuoteRequestMessage {
		t.Fatalf("expected latest conversation message to be passed to agent, got %#v", agent.lastMessages)
	}
	if len(queries.inserted) != 1 {
		t.Fatalf(testExpectedAssistantPersistCountMsg, len(queries.inserted))
	}
	if queries.inserted[0].Content != "*Offerte:* klaar\n*Bedrag:* € 125,00" {
		t.Fatalf("unexpected persisted quote reply %q", queries.inserted[0].Content)
	}
}

func TestRunAgentReplyPersistsAppointmentFallbackWhenGroundingFails(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: "Ik kan die afspraak nu niet betrouwbaar bevestigen. Over welke afspraak gaat het precies?", GroundingFailure: "appointment_details_without_appointment_tool"}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-visit", PhoneNumber: testAgentPhone, Body: "Wanneer is mijn afspraak?"}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound)

	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected appointment fallback to be sent")
	}
	if agent.err != nil {
		t.Fatalf("unexpected agent error: %v", agent.err)
	}
	if len(queries.inserted) != 1 {
		t.Fatalf(testExpectedAssistantPersistCountMsg, len(queries.inserted))
	}
	if queries.inserted[0].Content != "Ik kan die afspraak nu niet betrouwbaar bevestigen. Over welke afspraak gaat het precies?" {
		t.Fatalf("unexpected persisted appointment fallback %q", queries.inserted[0].Content)
	}
	if agent.lastOrganization != orgID {
		t.Fatalf("expected orgID %s, got %s", orgID, agent.lastOrganization)
	}
}

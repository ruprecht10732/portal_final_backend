package waagent

import (
	"context"
	"errors"
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
	testExpectedPersistedConversationMsg = "expected inbound and assistant message to be persisted, got %d entries"
	testMiniredisStartFailedMsg          = "failed to start miniredis: %v"
	testQuoteRequestMessage              = "Heeft u mijn offerte?"
	testLookupModeReply                  = "lookup-mode-reply"
	testDefaultModeReply                 = "default-mode-reply"
	testLookupCustomerName               = "Joey Plomp"
	testLeadLookupQuestion               = "Zoek adres van Joey plomp"
	testQuoteLookupQuestion              = "De offerte van Joey plomp"
	testGenericHelpQuestion              = "Kun je mij helpen?"
	testGroundingFallbackReply           = "Ik kan die klantgegevens nu niet betrouwbaar bevestigen. Welke klant of welk dossier bedoelt u precies?"
	testExpectedDefaultModeCallMsg       = "expected default-mode agent run, got %d calls"
	testExpectedDefaultModeMsg           = "expected default mode, got %q"
	testExpectedDefaultReasonMsg         = "expected default reason, got %q"
	testUnexpectedServiceReplyMsg        = "unexpected reply %q"
)

type serviceTestLeadDetailsReader struct {
	calls int
	lead  *LeadDetailsResult
}

func (r *serviceTestLeadDetailsReader) GetLeadDetails(context.Context, uuid.UUID, string) (*LeadDetailsResult, error) {
	r.calls++
	if r.lead != nil {
		copyLead := *r.lead
		return &copyLead, nil
	}
	return &LeadDetailsResult{LeadID: testHintLeadID, CustomerName: "Robin"}, nil
}

type serviceTestQuerier struct {
	lookupCalls int
	lookupErr   error
	lookupUser  waagentdb.GetAgentUserByPhoneRow
	recent      []waagentdb.GetRecentAgentMessagesRow
	inserted    []waagentdb.InsertAgentMessageParams
	deleted     []waagentdb.DeleteAgentMessagesByPhoneParams
}

type serviceTestAgent struct {
	calls            int
	result           AgentRunResult
	err              error
	lastMessages     []ConversationMessage
	lastLeadHint     *ConversationLeadHint
	lastInbound      *CurrentInboundMessage
	lastPhoneKey     string
	lastOrganization uuid.UUID
	lastMode         agentRunMode
}

func (a *serviceTestAgent) Run(_ context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *ConversationLeadHint, inboundMessage *CurrentInboundMessage, mode agentRunMode) (AgentRunResult, error) {
	a.calls++
	a.lastOrganization = orgID
	a.lastPhoneKey = phoneKey
	a.lastMode = mode
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
func (q *serviceTestQuerier) DeleteAgentMessagesByPhone(_ context.Context, params waagentdb.DeleteAgentMessagesByPhoneParams) error {
	q.deleted = append(q.deleted, params)
	q.recent = nil
	q.inserted = nil
	return nil
}
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
func (q *serviceTestQuerier) GetAgentUserByPhone(context.Context, string) (waagentdb.GetAgentUserByPhoneRow, error) {
	q.lookupCalls++
	if q.lookupErr != nil {
		return waagentdb.GetAgentUserByPhoneRow{}, q.lookupErr
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
func (q *serviceTestQuerier) ListAgentUsersByOrganization(context.Context, pgtype.UUID) ([]waagentdb.ListAgentUsersByOrganizationRow, error) {
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
		t.Fatalf(testMiniredisStartFailedMsg, err)
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
		t.Fatalf(testMiniredisStartFailedMsg, err)
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

	queries := &serviceTestQuerier{lookupErr: pgx.ErrNoRows, lookupUser: waagentdb.GetAgentUserByPhoneRow{OrganizationID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}}
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
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testGroundingFallbackReply, GroundingFailure: "lead_details_without_lead_tool"}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-status", PhoneNumber: testAgentPhone, Body: "Wat is mijn status?"}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound, agentModeDecision{mode: agentRunModeDefault, reason: "test_default"})

	if len(queries.inserted) != 1 {
		t.Fatalf(testExpectedAssistantPersistCountMsg, len(queries.inserted))
	}
	if queries.inserted[0].Content != testGroundingFallbackReply {
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
	if agent.lastMode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, agent.lastMode)
	}
}

func TestRunAgentReplyPersistsQuoteReplyFromValidatedAgentResult(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	sentAt := pgtype.Timestamptz{Time: time.Date(2026, time.March, 15, 8, 14, 0, 0, time.UTC), Valid: true}
	queries := &serviceTestQuerier{recent: []waagentdb.GetRecentAgentMessagesRow{{Role: "user", Content: testQuoteRequestMessage, CreatedAt: sentAt}}}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: "*Offerte:* klaar\n*Bedrag:* € 125,00", ToolResponseNames: []string{"GetQuotes"}}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-quote", PhoneNumber: testAgentPhone, Body: testQuoteRequestMessage}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound, agentModeDecision{mode: agentRunModeDefault, reason: "test_default"})

	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected validated quote reply to be sent")
	}
	if len(agent.lastMessages) != 1 || agent.lastMessages[0].Content != testQuoteRequestMessage {
		t.Fatalf("expected latest conversation message to be passed to agent, got %#v", agent.lastMessages)
	}
	if agent.lastMessages[0].SentAt == nil || !agent.lastMessages[0].SentAt.Equal(sentAt.Time) {
		t.Fatalf("expected message timestamp to be preserved, got %#v", agent.lastMessages[0].SentAt)
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
	agent := &serviceTestAgent{result: AgentRunResult{Reply: "Noem de datum, periode of klant, dan pak ik de juiste afspraak erbij.", GroundingFailure: "appointment_details_without_appointment_tool"}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-visit", PhoneNumber: testAgentPhone, Body: "Wanneer is mijn afspraak?"}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound, agentModeDecision{mode: agentRunModeDefault, reason: "test_default"})

	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected appointment fallback to be sent")
	}
	if agent.err != nil {
		t.Fatalf("unexpected agent error: %v", agent.err)
	}
	if len(queries.inserted) != 1 {
		t.Fatalf(testExpectedAssistantPersistCountMsg, len(queries.inserted))
	}
	if queries.inserted[0].Content != "Noem de datum, periode of klant, dan pak ik de juiste afspraak erbij." {
		t.Fatalf("unexpected persisted appointment fallback %q", queries.inserted[0].Content)
	}
	if agent.lastOrganization != orgID {
		t.Fatalf("expected orgID %s, got %s", orgID, agent.lastOrganization)
	}
}

func TestHandleAIMessageRoutesSimpleLeadLookupToLookupMode(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testLookupModeReply}}
	service := &Service{
		queries:       queries,
		agent:         agent,
		sender:        newTestSender(transport, serviceTestConfigReader{}, nil),
		leadHintStore: NewConversationLeadHintStore(),
		log:           logger.New("development"),
	}

	service.handleAIMessage(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, testLeadLookupQuestion, &CurrentInboundMessage{ExternalMessageID: "msg-lead", PhoneNumber: testAgentPhone, Body: testLeadLookupQuestion}, waagentdb.GetAgentUserByPhoneRow{})

	if agent.calls != 1 {
		t.Fatalf(testExpectedDefaultModeCallMsg, agent.calls)
	}
	if agent.lastMode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, agent.lastMode)
	}
	if decision := service.selectAgentRunMode(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testLeadLookupQuestion); decision.reason != "default" {
		t.Fatalf(testExpectedDefaultReasonMsg, decision.reason)
	}
	if len(queries.inserted) != 2 {
		t.Fatalf(testExpectedPersistedConversationMsg, len(queries.inserted))
	}
	if queries.inserted[1].Content != testLookupModeReply {
		t.Fatalf(testUnexpectedServiceReplyMsg, queries.inserted[1].Content)
	}
	if agent.lastInbound == nil || agent.lastInbound.Body != testLeadLookupQuestion {
		t.Fatalf("expected inbound message to reach lookup agent, got %#v", agent.lastInbound)
	}
}

func TestHandleAIMessageRoutesSimpleQuoteLookupToLookupMode(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testLookupModeReply}}
	service := &Service{
		queries:       queries,
		agent:         agent,
		sender:        newTestSender(transport, serviceTestConfigReader{}, nil),
		leadHintStore: NewConversationLeadHintStore(),
		log:           logger.New("development"),
	}

	service.handleAIMessage(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, testQuoteLookupQuestion, &CurrentInboundMessage{ExternalMessageID: "msg-quote-fast", PhoneNumber: testAgentPhone, Body: testQuoteLookupQuestion}, waagentdb.GetAgentUserByPhoneRow{})

	if agent.calls != 1 {
		t.Fatalf(testExpectedDefaultModeCallMsg, agent.calls)
	}
	if agent.lastMode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, agent.lastMode)
	}
	if decision := service.selectAgentRunMode(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testQuoteLookupQuestion); decision.reason != "default" {
		t.Fatalf(testExpectedDefaultReasonMsg, decision.reason)
	}
	if len(queries.inserted) != 2 {
		t.Fatalf(testExpectedPersistedConversationMsg, len(queries.inserted))
	}
	if queries.inserted[1].Content != testLookupModeReply {
		t.Fatalf(testUnexpectedServiceReplyMsg, queries.inserted[1].Content)
	}
}

func TestHandleAIMessageRoutesAmbiguousLookupToLookupMode(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testLookupModeReply}}
	service := &Service{
		queries:       queries,
		agent:         agent,
		sender:        newTestSender(transport, serviceTestConfigReader{}, nil),
		leadHintStore: NewConversationLeadHintStore(),
		log:           logger.New("development"),
	}

	service.handleAIMessage(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, "Zoek Joey plomp", &CurrentInboundMessage{ExternalMessageID: "msg-ambiguous", PhoneNumber: testAgentPhone, Body: "Zoek Joey plomp"}, waagentdb.GetAgentUserByPhoneRow{})

	if agent.calls != 1 {
		t.Fatalf(testExpectedDefaultModeCallMsg, agent.calls)
	}
	if agent.lastMode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, agent.lastMode)
	}
	if queries.inserted[1].Content != testLookupModeReply {
		t.Fatalf(testUnexpectedServiceReplyMsg, queries.inserted[1].Content)
	}
}

func TestHandleAIMessageKeepsDefaultModeForGenericMessages(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testDefaultModeReply}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}

	service.handleAIMessage(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, testGenericHelpQuestion, &CurrentInboundMessage{ExternalMessageID: "msg-default", PhoneNumber: testAgentPhone, Body: testGenericHelpQuestion}, waagentdb.GetAgentUserByPhoneRow{})

	if agent.calls != 1 {
		t.Fatalf("expected one agent run, got %d calls", agent.calls)
	}
	if agent.lastMode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, agent.lastMode)
	}
	if decision := service.selectAgentRunMode(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testGenericHelpQuestion); decision.reason != "default" {
		t.Fatalf(testExpectedDefaultReasonMsg, decision.reason)
	}
	if len(queries.inserted) != 2 {
		t.Fatalf(testExpectedPersistedConversationMsg, len(queries.inserted))
	}
	if queries.inserted[1].Content != testDefaultModeReply {
		t.Fatalf("unexpected default mode reply %q", queries.inserted[1].Content)
	}
}

func TestHandleAIMessageKeepsWriteRequestsOnDefaultMode(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testDefaultModeReply}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}

	writeQuestion := "Maak een offerte voor dakisolatie"
	service.handleAIMessage(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, writeQuestion, &CurrentInboundMessage{ExternalMessageID: "msg-write", PhoneNumber: testAgentPhone, Body: writeQuestion}, waagentdb.GetAgentUserByPhoneRow{})

	if agent.calls != 1 {
		t.Fatalf("expected one default-mode agent run, got %d calls", agent.calls)
	}
	if agent.lastMode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, agent.lastMode)
	}
	if decision := service.selectAgentRunMode(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), writeQuestion); decision.reason != "default" {
		t.Fatalf(testExpectedDefaultReasonMsg, decision.reason)
	}
}

func TestHandleAIMessageRoutesPartnerToPartnerMode(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	partnerID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testDefaultModeReply}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}

	service.handleAIMessage(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, testGenericHelpQuestion, &CurrentInboundMessage{ExternalMessageID: "msg-partner", PhoneNumber: testAgentPhone, Body: testGenericHelpQuestion}, waagentdb.GetAgentUserByPhoneRow{
		OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true},
		UserType:       "partner",
		PartnerID:      pgtype.UUID{Bytes: partnerID, Valid: true},
	})

	if agent.calls != 1 {
		t.Fatalf("expected one partner-mode agent run, got %d calls", agent.calls)
	}
	if agent.lastMode != agentRunModePartner {
		t.Fatalf("expected partner mode, got %q", agent.lastMode)
	}
}

func TestSelectAgentRunModeInheritsAddressIntentFromBareNameFollowUp(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	service := &Service{
		queries: &serviceTestQuerier{recent: []waagentdb.GetRecentAgentMessagesRow{{Role: "user", Content: testLeadLookupQuestion}, {Role: "assistant", Content: "Ik kan dat voor u opzoeken."}}},
		log:     logger.New("development"),
	}

	decision := service.selectAgentRunMode(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testLookupCustomerName)
	if decision.mode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, decision.mode)
	}
	if decision.reason != "default" {
		t.Fatalf(testExpectedDefaultReasonMsg, decision.reason)
	}
}

func TestRunAgentReplySendsFallbackWhenAgentRunFails(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{err: errors.New("runner timeout")}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-error", PhoneNumber: testAgentPhone, Body: "Welke afspraken zijn er?"}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound, agentModeDecision{mode: agentRunModeDefault, reason: "test_default"})

	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected fallback reply to be sent")
	}
	if len(queries.inserted) != 1 {
		t.Fatalf(testExpectedAssistantPersistCountMsg, len(queries.inserted))
	}
	if queries.inserted[0].Content != msgSystemUnavailable {
		t.Fatalf("unexpected persisted fallback %q", queries.inserted[0].Content)
	}
	if agent.calls != 1 {
		t.Fatalf("expected one failed agent run, got %d", agent.calls)
	}
}

func TestHandleIncomingMessageResetCommandClearsHistoryAndHints(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(testMiniredisStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	orgID := uuid.New()
	queries := &serviceTestQuerier{lookupUser: waagentdb.GetAgentUserByPhoneRow{OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true}}}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	store := NewConversationLeadHintStore()
	store.Set(orgID.String(), normalizeAgentPhoneKey(testAgentPhone), ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})
	service := &Service{
		queries:       queries,
		sender:        newTestSender(transport, serviceTestConfigReader{}, nil),
		rateLimiter:   NewRateLimiter(client, logger.New("development")),
		leadHintStore: store,
		log:           logger.New("development"),
	}

	service.HandleIncomingMessage(context.Background(), CurrentInboundMessage{ExternalMessageID: "msg-reset", PhoneNumber: testAgentPhone, Body: "/reset"})

	if len(queries.deleted) != 1 {
		t.Fatalf("expected one history delete call, got %d", len(queries.deleted))
	}
	if transport.lastSendMessagePhone == "" {
		t.Fatal("expected reset confirmation to be sent")
	}
	if len(queries.inserted) != 1 || queries.inserted[0].Content != msgConversationReset {
		t.Fatalf("expected only reset confirmation to be persisted, got %#v", queries.inserted)
	}
	if hint, ok := store.Get(orgID.String(), normalizeAgentPhoneKey(testAgentPhone)); ok || hint != nil {
		t.Fatalf("expected lead hint to be cleared, got %#v", hint)
	}
	if queries.lookupCalls == 0 {
		t.Fatal("expected phone lookup before reset handling")
	}
	if len(queries.inserted) != 1 {
		t.Fatalf(testExpectedAssistantPersistCountMsg, len(queries.inserted))
	}
}

func TestRunAgentReplySkipsPoisonedAssistantHistory(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	queries := &serviceTestQuerier{recent: []waagentdb.GetRecentAgentMessagesRow{
		{Role: "assistant", Content: testGroundingFallbackReply},
		{Role: "user", Content: testQuoteRequestMessage},
	}}
	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	agent := &serviceTestAgent{result: AgentRunResult{Reply: testDefaultModeReply}}
	service := &Service{
		queries: queries,
		agent:   agent,
		sender:  newTestSender(transport, serviceTestConfigReader{}, nil),
		log:     logger.New("development"),
	}
	inbound := &CurrentInboundMessage{ExternalMessageID: "msg-clean", PhoneNumber: testAgentPhone, Body: testGenericHelpQuestion}

	service.runAgentReply(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), testAgentPhone, inbound, agentModeDecision{mode: agentRunModeDefault, reason: "test_default"})

	if len(agent.lastMessages) != 1 {
		t.Fatalf("expected poisoned assistant reply to be filtered, got %#v", agent.lastMessages)
	}
	if agent.lastMessages[0].Role != "user" || agent.lastMessages[0].Content != testQuoteRequestMessage {
		t.Fatalf("unexpected replayed history %#v", agent.lastMessages)
	}
}

func TestSelectAgentRunModeUsesLookupForAffirmativeFollowUpWithLeadHint(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	store := NewConversationLeadHintStore()
	store.Set(orgID.String(), normalizeAgentPhoneKey(testAgentPhone), ConversationLeadHint{LeadID: testHintLeadID, CustomerName: testLookupCustomerName})
	service := &Service{
		queries:       &serviceTestQuerier{recent: []waagentdb.GetRecentAgentMessagesRow{{Role: "user", Content: testLeadLookupQuestion}, {Role: "assistant", Content: "Ik heb Carola Dekker gevonden. Wil je dat ik de volledige contactgegevens en adresdetails voor je ophaal?"}}},
		leadHintStore: store,
		log:           logger.New("development"),
	}

	decision := service.selectAgentRunMode(context.Background(), orgID, normalizeAgentPhoneKey(testAgentPhone), "Ja")
	if decision.mode != agentRunModeDefault {
		t.Fatalf(testExpectedDefaultModeMsg, decision.mode)
	}
	if decision.reason != "default" {
		t.Fatalf(testExpectedDefaultReasonMsg, decision.reason)
	}
}

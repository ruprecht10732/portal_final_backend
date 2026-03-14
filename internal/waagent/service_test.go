package waagent

import (
	"context"
	"testing"
	"time"

	"portal_final_backend/platform/logger"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	waagentdb "portal_final_backend/internal/waagent/db"
)

const testAgentPhone = "+31612345678"

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
	return nil, nil
}
func (q *serviceTestQuerier) GetRecentInboundAgentMessages(context.Context, waagentdb.GetRecentInboundAgentMessagesParams) ([]waagentdb.GetRecentInboundAgentMessagesRow, error) {
	return nil, nil
}
func (q *serviceTestQuerier) InsertAgentMessage(context.Context, waagentdb.InsertAgentMessageParams) error {
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

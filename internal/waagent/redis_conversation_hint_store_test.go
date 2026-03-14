package waagent

import (
	"context"
	"testing"
	"time"

	"portal_final_backend/platform/logger"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

const redisHintStoreStartFailedMsg = "failed to start miniredis: %v"

func TestRedisConversationLeadHintStorePersistsAndLoadsHint(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(redisHintStoreStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newRedisConversationLeadHintStore(client, logger.New("development"), time.Hour, func() time.Time { return now })
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID, LeadServiceID: "svc-1", CustomerName: "Robin", PreloadedDetails: &LeadDetailsResult{LeadID: testHintLeadID}})

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if !ok || hint == nil {
		t.Fatal("expected redis-backed hint to be returned")
	}
	if hint.LeadID != testHintLeadID || hint.LeadServiceID != "svc-1" || hint.CustomerName != "Robin" {
		t.Fatalf("unexpected hint payload: %#v", hint)
	}
	if hint.PreloadedDetails != nil {
		t.Fatal("expected preloaded details not to be persisted in redis")
	}
	if !hint.UpdatedAt.Equal(now) {
		t.Fatalf("expected updated_at %v, got %v", now, hint.UpdatedAt)
	}
}

func TestRedisConversationLeadHintStoreExpiresHintsByTTL(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(redisHintStoreStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	store := newRedisConversationLeadHintStore(client, logger.New("development"), time.Minute, time.Now)
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID})
	redisServer.FastForward(2 * time.Minute)

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if ok || hint != nil {
		t.Fatalf("expected redis hint to expire, got %#v", hint)
	}
}

func TestRedisConversationLeadHintStoreIgnoresInvalidKeysAndBlankLeadID(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(redisHintStoreStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	store := newRedisConversationLeadHintStore(client, logger.New("development"), time.Hour, time.Now)
	store.Set("", testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID})
	store.Set(testHintOrgID, "", ConversationLeadHint{LeadID: testHintLeadID})
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: "   "})

	keys, keysErr := client.Keys(context.Background(), conversationLeadHintRedisPrefix+"*").Result()
	if keysErr != nil {
		t.Fatalf("failed to read hint keys: %v", keysErr)
	}
	if len(keys) != 0 {
		t.Fatalf("expected no redis hint keys, got %#v", keys)
	}
}

func TestRedisConversationLeadHintStoreDropsCorruptPayloads(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(redisHintStoreStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	store := newRedisConversationLeadHintStore(client, logger.New("development"), time.Hour, time.Now)
	key := conversationLeadHintRedisPrefix + conversationLeadHintKey(testHintOrgID, testHintPhoneKey)
	if err := client.Set(context.Background(), key, "not-json", time.Hour).Err(); err != nil {
		t.Fatalf("failed to seed corrupt payload: %v", err)
	}

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if ok || hint != nil {
		t.Fatalf("expected corrupt redis hint to be ignored, got %#v", hint)
	}
	exists, existsErr := client.Exists(context.Background(), key).Result()
	if existsErr != nil {
		t.Fatalf("failed to inspect hint key: %v", existsErr)
	}
	if exists != 0 {
		t.Fatal("expected corrupt redis hint to be deleted")
	}
}

func TestRedisConversationLeadHintStorePersistsRecentQuoteAndAppointmentLists(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(redisHintStoreStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newRedisConversationLeadHintStore(client, logger.New("development"), time.Hour, func() time.Time { return now })
	store.RememberQuotes(testHintOrgID, testHintPhoneKey, []QuoteSummary{{
		QuoteID:     "quote-1",
		QuoteNumber: testQuoteNumber,
		ClientName:  testQuoteClientName,
		Summary:     "Kogellagerscharnier RVS",
	}})
	now = now.Add(10 * time.Minute)
	store.RememberAppointments(testHintOrgID, testHintPhoneKey, []AppointmentSummary{{
		AppointmentID: testAppointmentID,
		Title:         "Bezoek",
		StartTime:     "2026-03-16T09:00:00Z",
		Location:      "Alkmaar",
	}})

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if !ok || hint == nil {
		t.Fatal("expected redis-backed hint with recent lists to be returned")
	}
	if len(hint.RecentQuotes) != 1 || hint.RecentQuotes[0].QuoteNumber != testQuoteNumber {
		t.Fatalf("unexpected recent quotes %#v", hint.RecentQuotes)
	}
	if len(hint.RecentAppointments) != 1 || hint.RecentAppointments[0].AppointmentID != testAppointmentID {
		t.Fatalf("unexpected recent appointments %#v", hint.RecentAppointments)
	}
	if !hint.UpdatedAt.Equal(now) {
		t.Fatalf("expected updated_at %v, got %v", now, hint.UpdatedAt)
	}
}

func TestRedisConversationLeadHintStoreSetPreservesRecentQuotesWhenUpdatingLead(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf(redisHintStoreStartFailedMsg, err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newRedisConversationLeadHintStore(client, logger.New("development"), time.Hour, func() time.Time { return now })
	store.RememberQuotes(testHintOrgID, testHintPhoneKey, []QuoteSummary{{
		QuoteNumber: testQuoteNumber,
		ClientName:  testQuoteClientName,
	}})
	now = now.Add(time.Minute)
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if !ok || hint == nil {
		t.Fatal("expected merged redis hint to be returned")
	}
	if hint.LeadID != testHintLeadID || hint.CustomerName != "Robin" {
		t.Fatalf("unexpected lead hint payload %#v", hint)
	}
	if len(hint.RecentQuotes) != 1 || hint.RecentQuotes[0].QuoteNumber != testQuoteNumber {
		t.Fatalf("expected recent quote to be preserved, got %#v", hint.RecentQuotes)
	}
}

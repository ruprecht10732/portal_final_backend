package waagent

import (
	"strings"
	"testing"
	"time"
)

const (
	testHintOrgID        = "org-123"
	testHintPhoneKey     = "+31612345678"
	testHintLeadID       = "lead-123"
	testHintPhoneOne     = "+31610000001"
	testHintPhoneTwo     = "+31610000002"
	testHintPhoneTri     = "+31610000003"
	testHintUpdatedAtMsg = "expected updated timestamp %v, got %v"
	testQuoteNumber      = "OFF-2026-0021"
	testQuoteClientName  = "Joey Plomp"
	testAppointmentID    = "appt-1"
)

func TestConversationLeadHintStoreReturnsStoredHint(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newConversationLeadHintStore(func() time.Time { return now }, time.Hour, 10)
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if !ok || hint == nil {
		t.Fatal("expected stored hint to be returned")
	}
	if hint.LeadID != testHintLeadID || hint.CustomerName != "Robin" {
		t.Fatalf("unexpected hint payload: %#v", hint)
	}
	if !hint.UpdatedAt.Equal(now) {
		t.Fatalf(testHintUpdatedAtMsg, now, hint.UpdatedAt)
	}
}

func TestConversationLeadHintStoreExpiresOldHints(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newConversationLeadHintStore(func() time.Time { return now }, time.Hour, 10)
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID})

	now = now.Add(2 * time.Hour)
	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if ok || hint != nil {
		t.Fatalf("expected hint to expire, got %#v", hint)
	}
	if len(store.items) != 0 {
		t.Fatalf("expected expired hint to be pruned, got %d items", len(store.items))
	}
}

func TestConversationLeadHintStoreEvictsOldestWhenOverCapacity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newConversationLeadHintStore(func() time.Time { return now }, 24*time.Hour, 2)
	store.Set(testHintOrgID, testHintPhoneOne, ConversationLeadHint{LeadID: "lead-1"})
	now = now.Add(time.Minute)
	store.Set(testHintOrgID, testHintPhoneTwo, ConversationLeadHint{LeadID: "lead-2"})
	now = now.Add(time.Minute)
	store.Set(testHintOrgID, testHintPhoneTri, ConversationLeadHint{LeadID: "lead-3"})

	if _, ok := store.Get(testHintOrgID, testHintPhoneOne); ok {
		t.Fatal("expected oldest hint to be evicted")
	}
	if _, ok := store.Get(testHintOrgID, testHintPhoneTwo); !ok {
		t.Fatal("expected newer hint to remain")
	}
	if _, ok := store.Get(testHintOrgID, testHintPhoneTri); !ok {
		t.Fatal("expected latest hint to remain")
	}
}

func TestConversationLeadHintStoreIgnoresInvalidKeysAndBlankLeadID(t *testing.T) {
	t.Parallel()

	store := newConversationLeadHintStore(time.Now, time.Hour, 10)
	store.Set("", testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID})
	store.Set(testHintOrgID, "", ConversationLeadHint{LeadID: testHintLeadID})
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: "   "})

	if len(store.items) != 0 {
		t.Fatalf("expected invalid hints to be ignored, got %d items", len(store.items))
	}
}

func TestConversationLeadHintStorePrunesExpiredEntriesOnSet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newConversationLeadHintStore(func() time.Time { return now }, time.Hour, 10)
	store.Set(testHintOrgID, testHintPhoneOne, ConversationLeadHint{LeadID: "lead-1"})
	now = now.Add(2 * time.Hour)
	store.Set(testHintOrgID, testHintPhoneTwo, ConversationLeadHint{LeadID: "lead-2"})

	if len(store.items) != 1 {
		t.Fatalf("expected expired hint to be pruned during set, got %d items", len(store.items))
	}
	if _, ok := store.Get(testHintOrgID, testHintPhoneOne); ok {
		t.Fatal("expected expired hint to be gone after prune-on-set")
	}
}

func TestConversationLeadHintStoreRemembersRecentQuotesWithoutLeadID(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newConversationLeadHintStore(func() time.Time { return now }, time.Hour, 10)
	store.RememberQuotes(testHintOrgID, testHintPhoneKey, []QuoteSummary{{
		QuoteID:     "quote-1",
		QuoteNumber: testQuoteNumber,
		ClientName:  testQuoteClientName,
		LeadID:      "lead-joey",
		Summary:     "Kogellagerscharnier RVS",
	}})

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if !ok || hint == nil {
		t.Fatal("expected hint with recent quotes to be returned")
	}
	if len(hint.RecentQuotes) != 1 {
		t.Fatalf("expected 1 recent quote, got %#v", hint.RecentQuotes)
	}
	if hint.RecentQuotes[0].ClientName != testQuoteClientName || hint.RecentQuotes[0].QuoteNumber != testQuoteNumber {
		t.Fatalf("unexpected recent quote payload %#v", hint.RecentQuotes[0])
	}
	if strings.TrimSpace(hint.LeadID) != "" {
		t.Fatalf("expected lead id to remain empty, got %q", hint.LeadID)
	}
	if !hint.UpdatedAt.Equal(now) {
		t.Fatalf(testHintUpdatedAtMsg, now, hint.UpdatedAt)
	}
}

func TestConversationLeadHintStoreMergesRecentAppointmentsIntoExistingHint(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newConversationLeadHintStore(func() time.Time { return now }, time.Hour, 10)
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})
	now = now.Add(5 * time.Minute)
	store.RememberAppointments(testHintOrgID, testHintPhoneKey, []AppointmentSummary{{
		AppointmentID: testAppointmentID,
		Title:         "Bezoek",
		StartTime:     "2026-03-16T09:00:00Z",
		Location:      "Alkmaar",
	}})

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if !ok || hint == nil {
		t.Fatal("expected merged hint to be returned")
	}
	if hint.LeadID != testHintLeadID || hint.CustomerName != "Robin" {
		t.Fatalf("expected base hint to remain intact, got %#v", hint)
	}
	if len(hint.RecentAppointments) != 1 || hint.RecentAppointments[0].AppointmentID != testAppointmentID {
		t.Fatalf("unexpected recent appointments %#v", hint.RecentAppointments)
	}
	if !hint.UpdatedAt.Equal(now) {
		t.Fatalf(testHintUpdatedAtMsg, now, hint.UpdatedAt)
	}
}

func TestConversationLeadHintStoreSetPreservesRecentQuotesWhenUpdatingLead(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	store := newConversationLeadHintStore(func() time.Time { return now }, time.Hour, 10)
	store.RememberQuotes(testHintOrgID, testHintPhoneKey, []QuoteSummary{{
		QuoteNumber: testQuoteNumber,
		ClientName:  testQuoteClientName,
	}})
	now = now.Add(time.Minute)
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if !ok || hint == nil {
		t.Fatal("expected merged hint to be returned")
	}
	if hint.LeadID != testHintLeadID || hint.CustomerName != "Robin" {
		t.Fatalf("unexpected lead hint payload %#v", hint)
	}
	if len(hint.RecentQuotes) != 1 || hint.RecentQuotes[0].QuoteNumber != testQuoteNumber {
		t.Fatalf("expected recent quote to be preserved, got %#v", hint.RecentQuotes)
	}
}

func TestHasConversationRoutingContextAllowsRecentListsWithoutLeadID(t *testing.T) {
	t.Parallel()

	if hasConversationRoutingContext(nil) {
		t.Fatal("expected nil hint not to count as routing context")
	}
	if hasConversationRoutingContext(&ConversationLeadHint{}) {
		t.Fatal("expected empty hint not to count as routing context")
	}
	if !hasConversationRoutingContext(&ConversationLeadHint{RecentQuotes: []RecentQuoteHint{{QuoteNumber: testQuoteNumber}}}) {
		t.Fatal("expected recent quotes to count as routing context")
	}
	if !hasConversationRoutingContext(&ConversationLeadHint{RecentAppointments: []RecentAppointmentHint{{AppointmentID: testAppointmentID}}}) {
		t.Fatal("expected recent appointments to count as routing context")
	}
}

func TestConversationLeadHintStoreClearRemovesStoredHint(t *testing.T) {
	t.Parallel()

	store := newConversationLeadHintStore(time.Now, time.Hour, 10)
	store.Set(testHintOrgID, testHintPhoneKey, ConversationLeadHint{LeadID: testHintLeadID, CustomerName: "Robin"})
	store.Clear(testHintOrgID, testHintPhoneKey)

	hint, ok := store.Get(testHintOrgID, testHintPhoneKey)
	if ok || hint != nil {
		t.Fatalf("expected cleared hint to be removed, got %#v", hint)
	}
}

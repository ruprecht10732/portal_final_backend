package whatsappagent

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	conversationLeadHintTTL        = 24 * time.Hour
	conversationLeadHintMaxEntries = 1000
)

type conversationLeadHintClock func() time.Time

const (
	conversationHintRecentQuotesLimit       = 5
	conversationHintRecentAppointmentsLimit = 5
)

type RecentQuoteHint struct {
	QuoteID       string
	QuoteNumber   string
	LeadID        string
	LeadServiceID string
	ClientName    string
	Status        string
	Summary       string
}

type RecentAppointmentHint struct {
	AppointmentID string
	LeadID        string
	LeadServiceID string
	Title         string
	StartTime     string
	Status        string
	Location      string
}

type ConversationLeadHint struct {
	LeadID             string
	LeadServiceID      string
	CustomerName       string
	RecentQuotes       []RecentQuoteHint
	RecentAppointments []RecentAppointmentHint
	UpdatedAt          time.Time
	PreloadedDetails   *LeadDetailsResult
}

type LeadHintStore interface {
	Get(ctx context.Context, orgID, phoneKey string) (*ConversationLeadHint, bool)
	Set(ctx context.Context, orgID, phoneKey string, hint ConversationLeadHint)
	RememberQuotes(ctx context.Context, orgID, phoneKey string, quotes []QuoteSummary)
	RememberAppointments(ctx context.Context, orgID, phoneKey string, appointments []AppointmentSummary)
	Clear(ctx context.Context, orgID, phoneKey string)
}

type ConversationLeadHintStore struct {
	mu         sync.RWMutex
	items      map[string]ConversationLeadHint
	lru        *list.List
	entryRefs  map[string]*list.Element
	now        conversationLeadHintClock
	ttl        time.Duration
	maxEntries int
}

func NewConversationLeadHintStore() *ConversationLeadHintStore {
	return newConversationLeadHintStore(time.Now, conversationLeadHintTTL, conversationLeadHintMaxEntries)
}

func newConversationLeadHintStore(now conversationLeadHintClock, ttl time.Duration, maxEntries int) *ConversationLeadHintStore {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = conversationLeadHintTTL
	}
	if maxEntries <= 0 {
		maxEntries = conversationLeadHintMaxEntries
	}
	return &ConversationLeadHintStore{
		items:      make(map[string]ConversationLeadHint),
		lru:        list.New(),
		entryRefs:  make(map[string]*list.Element),
		now:        now,
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (s *ConversationLeadHintStore) Get(ctx context.Context, orgID, phoneKey string) (*ConversationLeadHint, bool) {
	if s == nil {
		return nil, false
	}
	key := conversationLeadHintKey(orgID, phoneKey)
	if key == "" {
		return nil, false
	}
	s.mu.RLock()
	hint, ok := s.items[key]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if s.isExpired(hint) {
		s.mu.Lock()
		delete(s.items, key)
		if elem := s.entryRefs[key]; elem != nil {
			s.lru.Remove(elem)
			delete(s.entryRefs, key)
		}
		s.mu.Unlock()
		return nil, false
	}
	s.mu.Lock()
	if elem := s.entryRefs[key]; elem != nil {
		s.lru.MoveToFront(elem)
	}
	s.mu.Unlock()
	copyHint := hint
	return &copyHint, true
}

func (s *ConversationLeadHintStore) Set(ctx context.Context, orgID, phoneKey string, hint ConversationLeadHint) {
	if s == nil {
		return
	}
	key := conversationLeadHintKey(orgID, phoneKey)
	if key == "" {
		return
	}
	s.mu.Lock()
	s.pruneExpiredLocked()
	existing := normalizeConversationLeadHint(s.items[key])
	hint = mergeConversationLeadHints(existing, hint)
	if hintIsEmpty(hint) {
		delete(s.items, key)
		if elem := s.entryRefs[key]; elem != nil {
			s.lru.Remove(elem)
			delete(s.entryRefs, key)
		}
		s.mu.Unlock()
		return
	}
	hint.UpdatedAt = s.currentTime()
	s.items[key] = hint
	if elem := s.entryRefs[key]; elem != nil {
		s.lru.MoveToFront(elem)
	} else {
		s.entryRefs[key] = s.lru.PushFront(key)
	}
	s.evictOverflowLocked()
	s.mu.Unlock()
}

func (s *ConversationLeadHintStore) RememberQuotes(ctx context.Context, orgID, phoneKey string, quotes []QuoteSummary) {
	s.remember(ctx, orgID, phoneKey, func(hint *ConversationLeadHint) {
		hint.RecentQuotes = summarizeRecentQuotes(quotes)
	})
}

func (s *ConversationLeadHintStore) RememberAppointments(ctx context.Context, orgID, phoneKey string, appointments []AppointmentSummary) {
	s.remember(ctx, orgID, phoneKey, func(hint *ConversationLeadHint) {
		hint.RecentAppointments = summarizeRecentAppointments(appointments)
	})
}

func (s *ConversationLeadHintStore) Clear(ctx context.Context, orgID, phoneKey string) {
	if s == nil {
		return
	}
	key := conversationLeadHintKey(orgID, phoneKey)
	if key == "" {
		return
	}
	s.mu.Lock()
	delete(s.items, key)
	if elem := s.entryRefs[key]; elem != nil {
		s.lru.Remove(elem)
		delete(s.entryRefs, key)
	}
	s.mu.Unlock()
}

func (s *ConversationLeadHintStore) remember(ctx context.Context, orgID, phoneKey string, mutate func(*ConversationLeadHint)) {
	if s == nil || mutate == nil {
		return
	}
	key := conversationLeadHintKey(orgID, phoneKey)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked()
	hint := normalizeConversationLeadHint(s.items[key])
	mutate(&hint)
	if hintIsEmpty(hint) {
		delete(s.items, key)
		if elem := s.entryRefs[key]; elem != nil {
			s.lru.Remove(elem)
			delete(s.entryRefs, key)
		}
		return
	}
	hint.UpdatedAt = s.currentTime()
	s.items[key] = hint
	if elem := s.entryRefs[key]; elem != nil {
		s.lru.MoveToFront(elem)
	} else {
		s.entryRefs[key] = s.lru.PushFront(key)
	}
	s.evictOverflowLocked()
}

func (s *ConversationLeadHintStore) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func (s *ConversationLeadHintStore) isExpired(hint ConversationLeadHint) bool {
	if s == nil {
		return false
	}
	return s.currentTime().Sub(hint.UpdatedAt) > s.ttl
}

func (s *ConversationLeadHintStore) pruneExpiredLocked() {
	for key, hint := range s.items {
		if s.isExpired(hint) {
			delete(s.items, key)
			if elem := s.entryRefs[key]; elem != nil {
				s.lru.Remove(elem)
				delete(s.entryRefs, key)
			}
		}
	}
}

func (s *ConversationLeadHintStore) evictOverflowLocked() {
	for len(s.items) > s.maxEntries {
		back := s.lru.Back()
		if back == nil {
			return
		}
		key := back.Value.(string)
		s.lru.Remove(back)
		delete(s.items, key)
		delete(s.entryRefs, key)
	}
}

func conversationLeadHintKey(orgID, phoneKey string) string {
	orgID = strings.TrimSpace(orgID)
	phoneKey = strings.TrimSpace(phoneKey)
	if orgID == "" || phoneKey == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s", orgID, phoneKey)
}

func summarizeRecentQuotes(quotes []QuoteSummary) []RecentQuoteHint {
	if len(quotes) == 0 {
		return nil
	}
	limit := len(quotes)
	if limit > conversationHintRecentQuotesLimit {
		limit = conversationHintRecentQuotesLimit
	}
	result := make([]RecentQuoteHint, 0, limit)
	for _, quote := range quotes[:limit] {
		result = append(result, RecentQuoteHint{
			QuoteID:       strings.TrimSpace(quote.QuoteID),
			QuoteNumber:   strings.TrimSpace(quote.QuoteNumber),
			LeadID:        strings.TrimSpace(quote.LeadID),
			LeadServiceID: strings.TrimSpace(quote.LeadServiceID),
			ClientName:    strings.TrimSpace(quote.ClientName),
			Status:        strings.TrimSpace(quote.Status),
			Summary:       strings.TrimSpace(quote.Summary),
		})
	}
	return result
}

func summarizeRecentAppointments(appointments []AppointmentSummary) []RecentAppointmentHint {
	if len(appointments) == 0 {
		return nil
	}
	limit := len(appointments)
	if limit > conversationHintRecentAppointmentsLimit {
		limit = conversationHintRecentAppointmentsLimit
	}
	result := make([]RecentAppointmentHint, 0, limit)
	for _, appointment := range appointments[:limit] {
		result = append(result, RecentAppointmentHint{
			AppointmentID: strings.TrimSpace(appointment.AppointmentID),
			LeadID:        strings.TrimSpace(appointment.LeadID),
			LeadServiceID: strings.TrimSpace(appointment.LeadServiceID),
			Title:         strings.TrimSpace(appointment.Title),
			StartTime:     strings.TrimSpace(appointment.StartTime),
			Status:        strings.TrimSpace(appointment.Status),
			Location:      strings.TrimSpace(appointment.Location),
		})
	}
	return result
}

func normalizeConversationLeadHint(hint ConversationLeadHint) ConversationLeadHint {
	hint.LeadID = strings.TrimSpace(hint.LeadID)
	hint.LeadServiceID = strings.TrimSpace(hint.LeadServiceID)
	hint.CustomerName = strings.TrimSpace(hint.CustomerName)
	hint.RecentQuotes = append([]RecentQuoteHint(nil), hint.RecentQuotes...)
	hint.RecentAppointments = append([]RecentAppointmentHint(nil), hint.RecentAppointments...)
	return hint
}

func mergeConversationLeadHints(existing, incoming ConversationLeadHint) ConversationLeadHint {
	merged := normalizeConversationLeadHint(incoming)
	existing = normalizeConversationLeadHint(existing)
	if merged.LeadID == "" {
		merged.LeadID = existing.LeadID
	}
	if merged.LeadServiceID == "" {
		merged.LeadServiceID = existing.LeadServiceID
	}
	if merged.CustomerName == "" {
		merged.CustomerName = existing.CustomerName
	}
	if len(merged.RecentQuotes) == 0 {
		merged.RecentQuotes = existing.RecentQuotes
	}
	if len(merged.RecentAppointments) == 0 {
		merged.RecentAppointments = existing.RecentAppointments
	}
	if merged.PreloadedDetails == nil {
		merged.PreloadedDetails = existing.PreloadedDetails
	}
	return merged
}

func hasConversationRoutingContext(hint *ConversationLeadHint) bool {
	if hint == nil {
		return false
	}
	return strings.TrimSpace(hint.LeadID) != "" || len(hint.RecentQuotes) > 0 || len(hint.RecentAppointments) > 0
}

func hintIsEmpty(hint ConversationLeadHint) bool {
	return strings.TrimSpace(hint.LeadID) == "" && len(hint.RecentQuotes) == 0 && len(hint.RecentAppointments) == 0
}

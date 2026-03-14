package waagent

import (
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

type ConversationLeadHint struct {
	LeadID           string
	LeadServiceID    string
	CustomerName     string
	UpdatedAt        time.Time
	PreloadedDetails *LeadDetailsResult // populated at runtime, never persisted
}

type ConversationLeadHintStore struct {
	mu         sync.RWMutex
	items      map[string]ConversationLeadHint
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
		now:        now,
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (s *ConversationLeadHintStore) Get(orgID, phoneKey string) (*ConversationLeadHint, bool) {
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
		s.mu.Unlock()
		return nil, false
	}
	copyHint := hint
	return &copyHint, true
}

func (s *ConversationLeadHintStore) Set(orgID, phoneKey string, hint ConversationLeadHint) {
	if s == nil {
		return
	}
	key := conversationLeadHintKey(orgID, phoneKey)
	if key == "" || strings.TrimSpace(hint.LeadID) == "" {
		return
	}
	hint.UpdatedAt = s.currentTime()
	s.mu.Lock()
	s.pruneExpiredLocked()
	s.items[key] = hint
	s.evictOverflowLocked()
	s.mu.Unlock()
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
		}
	}
}

func (s *ConversationLeadHintStore) evictOverflowLocked() {
	for len(s.items) > s.maxEntries {
		oldestKey := ""
		var oldestAt time.Time
		for key, hint := range s.items {
			if oldestKey == "" || hint.UpdatedAt.Before(oldestAt) {
				oldestKey = key
				oldestAt = hint.UpdatedAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.items, oldestKey)
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

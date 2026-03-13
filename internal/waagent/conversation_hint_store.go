package waagent

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const conversationLeadHintTTL = 24 * time.Hour

type ConversationLeadHint struct {
	LeadID           string
	LeadServiceID    string
	CustomerName     string
	UpdatedAt        time.Time
	PreloadedDetails *LeadDetailsResult // populated at runtime, never persisted
}

type ConversationLeadHintStore struct {
	mu    sync.RWMutex
	items map[string]ConversationLeadHint
}

func NewConversationLeadHintStore() *ConversationLeadHintStore {
	return &ConversationLeadHintStore{items: make(map[string]ConversationLeadHint)}
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
	if time.Since(hint.UpdatedAt) > conversationLeadHintTTL {
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
	hint.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.items[key] = hint
	s.mu.Unlock()
}

func conversationLeadHintKey(orgID, phoneKey string) string {
	orgID = strings.TrimSpace(orgID)
	phoneKey = strings.TrimSpace(phoneKey)
	if orgID == "" || phoneKey == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s", orgID, phoneKey)
}

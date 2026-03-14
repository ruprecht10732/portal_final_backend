package waagent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"portal_final_backend/platform/logger"

	"github.com/redis/go-redis/v9"
)

const conversationLeadHintRedisPrefix = "waagent:lead-hint:"

type redisConversationLeadHintRecord struct {
	LeadID             string                  `json:"lead_id,omitempty"`
	LeadServiceID      string                  `json:"lead_service_id,omitempty"`
	CustomerName       string                  `json:"customer_name,omitempty"`
	RecentQuotes       []RecentQuoteHint       `json:"recent_quotes,omitempty"`
	RecentAppointments []RecentAppointmentHint `json:"recent_appointments,omitempty"`
	UpdatedAt          time.Time               `json:"updated_at"`
}

type RedisConversationLeadHintStore struct {
	redis *redis.Client
	log   *logger.Logger
	ttl   time.Duration
	now   conversationLeadHintClock
}

func NewRedisConversationLeadHintStore(client *redis.Client, log *logger.Logger) LeadHintStore {
	if client == nil {
		return NewConversationLeadHintStore()
	}
	return &RedisConversationLeadHintStore{
		redis: client,
		log:   log,
		ttl:   conversationLeadHintTTL,
		now:   time.Now,
	}
}

func newRedisConversationLeadHintStore(client *redis.Client, log *logger.Logger, ttl time.Duration, now conversationLeadHintClock) *RedisConversationLeadHintStore {
	if ttl <= 0 {
		ttl = conversationLeadHintTTL
	}
	if now == nil {
		now = time.Now
	}
	return &RedisConversationLeadHintStore{redis: client, log: log, ttl: ttl, now: now}
}

func (s *RedisConversationLeadHintStore) Get(orgID, phoneKey string) (*ConversationLeadHint, bool) {
	if s == nil || s.redis == nil {
		return nil, false
	}
	key := s.redisKey(orgID, phoneKey)
	if key == "" {
		return nil, false
	}
	raw, err := s.redis.Get(context.Background(), key).Bytes()
	if err != nil {
		if err != redis.Nil {
			s.logWarn("waagent: failed to load redis conversation hint", "key", key, "error", err)
		}
		return nil, false
	}
	var record redisConversationLeadHintRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		_ = s.redis.Del(context.Background(), key).Err()
		s.logWarn("waagent: failed to decode redis conversation hint", "key", key, "error", err)
		return nil, false
	}
	hint := normalizeConversationLeadHint(ConversationLeadHint{
		LeadID:             strings.TrimSpace(record.LeadID),
		LeadServiceID:      strings.TrimSpace(record.LeadServiceID),
		CustomerName:       strings.TrimSpace(record.CustomerName),
		RecentQuotes:       record.RecentQuotes,
		RecentAppointments: record.RecentAppointments,
		UpdatedAt:          record.UpdatedAt,
	})
	if hintIsEmpty(hint) {
		_ = s.redis.Del(context.Background(), key).Err()
		return nil, false
	}
	return &hint, true
}

func (s *RedisConversationLeadHintStore) Set(orgID, phoneKey string, hint ConversationLeadHint) {
	if s == nil || s.redis == nil {
		return
	}
	key := s.redisKey(orgID, phoneKey)
	hint = normalizeConversationLeadHint(hint)
	if key == "" || hintIsEmpty(hint) {
		return
	}
	record := redisConversationLeadHintRecord{
		LeadID:             hint.LeadID,
		LeadServiceID:      hint.LeadServiceID,
		CustomerName:       hint.CustomerName,
		RecentQuotes:       hint.RecentQuotes,
		RecentAppointments: hint.RecentAppointments,
		UpdatedAt:          s.currentTime(),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		s.logWarn("waagent: failed to encode redis conversation hint", "key", key, "error", err)
		return
	}
	if err := s.redis.Set(context.Background(), key, payload, s.ttl).Err(); err != nil {
		s.logWarn("waagent: failed to store redis conversation hint", "key", key, "error", err)
	}
}

func (s *RedisConversationLeadHintStore) RememberQuotes(orgID, phoneKey string, quotes []QuoteSummary) {
	s.remember(orgID, phoneKey, func(hint *ConversationLeadHint) {
		hint.RecentQuotes = summarizeRecentQuotes(quotes)
	})
}

func (s *RedisConversationLeadHintStore) RememberAppointments(orgID, phoneKey string, appointments []AppointmentSummary) {
	s.remember(orgID, phoneKey, func(hint *ConversationLeadHint) {
		hint.RecentAppointments = summarizeRecentAppointments(appointments)
	})
}

func (s *RedisConversationLeadHintStore) remember(orgID, phoneKey string, mutate func(*ConversationLeadHint)) {
	if s == nil || s.redis == nil || mutate == nil {
		return
	}
	key := s.redisKey(orgID, phoneKey)
	if key == "" {
		return
	}
	hint, _ := s.Get(orgID, phoneKey)
	merged := ConversationLeadHint{}
	if hint != nil {
		merged = *hint
	}
	mutate(&merged)
	if hintIsEmpty(merged) {
		_ = s.redis.Del(context.Background(), key).Err()
		return
	}
	s.Set(orgID, phoneKey, merged)
}

func (s *RedisConversationLeadHintStore) redisKey(orgID, phoneKey string) string {
	key := conversationLeadHintKey(orgID, phoneKey)
	if key == "" {
		return ""
	}
	return conversationLeadHintRedisPrefix + key
}

func (s *RedisConversationLeadHintStore) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func (s *RedisConversationLeadHintStore) logWarn(message string, args ...any) {
	if s == nil || s.log == nil {
		return
	}
	s.log.Warn(message, args...)
}

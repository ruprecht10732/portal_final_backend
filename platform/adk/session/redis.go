// Package session provides persistent ADK session backends.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/adk/session"
)

// redisSession implements session.Session backed by Redis.
type redisSession struct {
	id            string
	appName       string
	userID        string
	state         map[string]any
	events        []*session.Event
	lastUpdateTime time.Time
}

func (s *redisSession) ID() string                { return s.id }
func (s *redisSession) AppName() string           { return s.appName }
func (s *redisSession) UserID() string            { return s.userID }
func (s *redisSession) State() session.State      { return &mapState{data: s.state} }
func (s *redisSession) Events() session.Events    { return &sliceEvents{events: s.events} }
func (s *redisSession) LastUpdateTime() time.Time { return s.lastUpdateTime }

// mapState implements session.State.
type mapState struct {
	data map[string]any
}

func (m *mapState) Get(key string) (any, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("state key %q not found", key)
	}
	return v, nil
}

func (m *mapState) Set(key string, value any) error {
	m.data[key] = value
	return nil
}

func (m *mapState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range m.data {
			if !yield(k, v) {
				return
			}
		}
	}
}

// sliceEvents implements session.Events.
type sliceEvents struct {
	events []*session.Event
}

func (s *sliceEvents) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, e := range s.events {
			if !yield(e) {
				return
			}
		}
	}
}

func (s *sliceEvents) Len() int             { return len(s.events) }
func (s *sliceEvents) At(i int) *session.Event { return s.events[i] }

// RedisService implements session.Service using Redis for persistence.
// Sessions survive process restarts and are shareable across replicas.
type RedisService struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

// NewRedisService creates a Redis-backed session service.
// prefix is prepended to all Redis keys (e.g., "adk:session:").
// ttl controls session expiration; zero means no expiration.
func NewRedisService(client *redis.Client, prefix string, ttl time.Duration) *RedisService {
	if prefix == "" {
		prefix = "adk:session:"
	}
	return &RedisService{client: client, prefix: prefix, ttl: ttl}
}

func (r *RedisService) sessionKey(appName, userID, sessionID string) string {
	return fmt.Sprintf("%s%s:%s:%s", r.prefix, appName, userID, sessionID)
}

func (r *RedisService) sessionIndexKey(appName, userID string) string {
	return fmt.Sprintf("%sindex:%s:%s", r.prefix, appName, userID)
}

// sessionData is the JSON-serializable representation of a session.
type sessionData struct {
	ID             string            `json:"id"`
	AppName        string            `json:"app_name"`
	UserID         string            `json:"user_id"`
	State          map[string]any    `json:"state"`
	Events         []*session.Event  `json:"events"`
	LastUpdateTime time.Time         `json:"last_update_time"`
}

func (r *RedisService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	now := time.Now().UTC()
	sid := req.SessionID
	if sid == "" {
		sid = fmt.Sprintf("%d", now.UnixNano())
	}

	state := req.State
	if state == nil {
		state = make(map[string]any)
	}

	sd := sessionData{
		ID:             sid,
		AppName:        req.AppName,
		UserID:         req.UserID,
		State:          state,
		Events:         make([]*session.Event, 0),
		LastUpdateTime: now,
	}

	key := r.sessionKey(req.AppName, req.UserID, sid)
	data, err := json.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("redis session: marshal: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, r.ttl)
	pipe.SAdd(ctx, r.sessionIndexKey(req.AppName, req.UserID), sid)
	if r.ttl > 0 {
		pipe.Expire(ctx, r.sessionIndexKey(req.AppName, req.UserID), r.ttl)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("redis session: create: %w", err)
	}

	s := &redisSession{
		id:             sid,
		appName:        req.AppName,
		userID:         req.UserID,
		state:          state,
		events:         sd.Events,
		lastUpdateTime: now,
	}
	return &session.CreateResponse{Session: s}, nil
}

func (r *RedisService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	key := r.sessionKey(req.AppName, req.UserID, req.SessionID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("redis session: session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("redis session: get: %w", err)
	}

	var sd sessionData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, fmt.Errorf("redis session: unmarshal: %w", err)
	}

	// Apply event filters
	events := sd.Events
	if req.After.After(time.Time{}) {
		filtered := make([]*session.Event, 0, len(events))
		for _, e := range events {
			if e != nil && e.Timestamp.After(req.After) {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}
	if req.NumRecentEvents > 0 && len(events) > req.NumRecentEvents {
		events = events[len(events)-req.NumRecentEvents:]
	}

	s := &redisSession{
		id:             sd.ID,
		appName:        sd.AppName,
		userID:         sd.UserID,
		state:          sd.State,
		events:         events,
		lastUpdateTime: sd.LastUpdateTime,
	}
	return &session.GetResponse{Session: s}, nil
}

func (r *RedisService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	indexKey := r.sessionIndexKey(req.AppName, req.UserID)
	ids, err := r.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("redis session: list: %w", err)
	}

	sessions := make([]session.Session, 0, len(ids))
	for _, id := range ids {
		resp, err := r.Get(ctx, &session.GetRequest{
			AppName:   req.AppName,
			UserID:    req.UserID,
			SessionID: id,
		})
		if err != nil {
			continue // skip stale entries
		}
		sessions = append(sessions, resp.Session)
	}
	return &session.ListResponse{Sessions: sessions}, nil
}

func (r *RedisService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	key := r.sessionKey(req.AppName, req.UserID, req.SessionID)
	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, r.sessionIndexKey(req.AppName, req.UserID), req.SessionID)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis session: delete: %w", err)
	}
	return nil
}

func (r *RedisService) AppendEvent(ctx context.Context, s session.Session, event *session.Event) error {
	if event == nil {
		return nil
	}
	key := r.sessionKey(s.AppName(), s.UserID(), s.ID())
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return fmt.Errorf("redis session: cannot append to missing session")
	}
	if err != nil {
		return fmt.Errorf("redis session: append get: %w", err)
	}

	var sd sessionData
	if err := json.Unmarshal(data, &sd); err != nil {
		return fmt.Errorf("redis session: append unmarshal: %w", err)
	}

	// Remove temporary state keys before persisting (per ADK contract)
	if len(event.Actions.StateDelta) > 0 {
		for k, v := range event.Actions.StateDelta {
			if strings.HasPrefix(k, "__tmp_") {
				delete(event.Actions.StateDelta, k)
				continue
			}
			sd.State[k] = v
		}
	}

	sd.Events = append(sd.Events, event)
	sd.LastUpdateTime = time.Now().UTC()

	updated, err := json.Marshal(sd)
	if err != nil {
		return fmt.Errorf("redis session: append marshal: %w", err)
	}

	if err := r.client.Set(ctx, key, updated, r.ttl).Err(); err != nil {
		return fmt.Errorf("redis session: append set: %w", err)
	}
	return nil
}

// Ensure RedisService implements session.Service.
var _ session.Service = (*RedisService)(nil)

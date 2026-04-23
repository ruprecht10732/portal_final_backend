// Package service provides business logic for energy label lookups.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"portal_final_backend/internal/energylabel/client"
	"portal_final_backend/internal/energylabel/transport"
	"portal_final_backend/platform/logger"
)

// cacheEntry holds a cached energy label result.
// Fields ordered by size (8-byte pointers/slices first) to optimize alignment.
type cacheEntry struct {
	expiresAt time.Time               // 24 bytes
	labels    []transport.EnergyLabel // 24 bytes
}

// Service handles energy label lookups with internal in-memory caching.
type Service struct {
	client   *client.Client
	log      *logger.Logger
	cache    map[string]cacheEntry
	cacheTTL time.Duration
	cacheMu  sync.RWMutex
}

// New creates a new energy label service.
func New(client *client.Client, log *logger.Logger) *Service {
	return &Service{
		client:   client,
		log:      log,
		cache:    make(map[string]cacheEntry),
		cacheTTL: 24 * time.Hour, // Energy labels are static data; long TTL is optimal.
	}
}

// GetByAddress fetches energy label by address with O(1) cache lookup performance.
func (s *Service) GetByAddress(ctx context.Context, postcode, huisnummer, huisletter, toevoeging, detail string) (*transport.EnergyLabel, error) {
	// Unique key schema prevents collisions between address and BAG lookups.
	key := fmt.Sprintf("addr:%s:%s:%s:%s:%s", postcode, huisnummer, huisletter, toevoeging, detail)

	return s.fetchWithCache(key, func() ([]transport.EnergyLabel, error) {
		return s.client.GetByAddress(ctx, postcode, huisnummer, huisletter, toevoeging, detail)
	})
}

// GetByBAGObjectID fetches energy label by BAG object ID with O(1) cache lookup performance.
func (s *Service) GetByBAGObjectID(ctx context.Context, objectID string) (*transport.EnergyLabel, error) {
	key := "bag:" + objectID

	return s.fetchWithCache(key, func() ([]transport.EnergyLabel, error) {
		return s.client.GetByBAGObjectID(ctx, objectID)
	})
}

// fetchWithCache implements the Cache-Aside pattern.
// It centralizes synchronization and error handling, reducing the surface area for bugs.
func (s *Service) fetchWithCache(key string, fetcher func() ([]transport.EnergyLabel, error)) (*transport.EnergyLabel, error) {
	// Attempt Cache Hit (Read-lock allows O(N) concurrent readers)
	if labels := s.getFromCache(key); labels != nil {
		if len(labels) > 0 {
			return &labels[0], nil
		}
		return nil, nil
	}

	// Cache Miss - Fetch from Upstream
	labels, err := fetcher()
	if err != nil {
		return nil, err
	}

	// Update Cache (Write-lock ensures atomicity)
	s.setCache(key, labels)

	if len(labels) == 0 {
		return nil, nil
	}
	return &labels[0], nil
}

// Ping checks if the upstream EP-Online API is reachable.
func (s *Service) Ping(ctx context.Context) error {
	return s.client.Ping(ctx)
}

// ClearCache flushes the internal map.
// O(1) pointer swap for the map, though old entries will be cleared by GC in O(N).
func (s *Service) ClearCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = make(map[string]cacheEntry)
}

// ─── Internal Cache Management ───────────────────────────────────────────────

func (s *Service) getFromCache(key string) []transport.EnergyLabel {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, ok := s.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.labels
}

func (s *Service) setCache(key string, labels []transport.EnergyLabel) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cache[key] = cacheEntry{
		labels:    labels,
		expiresAt: time.Now().Add(s.cacheTTL),
	}
}

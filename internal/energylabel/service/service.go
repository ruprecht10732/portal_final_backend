// Package service provides business logic for energy label lookups.
package service

import (
	"context"
	"sync"
	"time"

	"portal_final_backend/internal/energylabel/client"
	"portal_final_backend/internal/energylabel/transport"
	"portal_final_backend/platform/logger"
)

// cacheEntry holds a cached energy label with expiration.
type cacheEntry struct {
	labels    []transport.EnergyLabel
	expiresAt time.Time
}

// Service handles energy label lookups with caching.
type Service struct {
	client   *client.Client
	log      *logger.Logger
	cache    map[string]cacheEntry
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
}

// New creates a new energy label service.
func New(client *client.Client, log *logger.Logger) *Service {
	return &Service{
		client:   client,
		log:      log,
		cache:    make(map[string]cacheEntry),
		cacheTTL: 24 * time.Hour, // Energy labels don't change often
	}
}

// GetByAddress fetches energy label by address, using cache when available.
func (s *Service) GetByAddress(ctx context.Context, postcode, huisnummer, huisletter, toevoeging, detail string) (*transport.EnergyLabel, error) {
	cacheKey := buildAddressCacheKey(postcode, huisnummer, huisletter, toevoeging, detail)

	// Check cache first
	if labels := s.getFromCache(cacheKey); labels != nil {
		if len(labels) > 0 {
			return &labels[0], nil
		}
		return nil, nil
	}

	// Fetch from API
	labels, err := s.client.GetByAddress(ctx, postcode, huisnummer, huisletter, toevoeging, detail)
	if err != nil {
		return nil, err
	}

	// Cache result (even if empty to avoid repeated lookups for non-existent labels)
	s.setCache(cacheKey, labels)

	if len(labels) == 0 {
		return nil, nil
	}

	return &labels[0], nil
}

// GetByBAGObjectID fetches energy label by BAG object ID, using cache when available.
func (s *Service) GetByBAGObjectID(ctx context.Context, objectID string) (*transport.EnergyLabel, error) {
	cacheKey := "bag:" + objectID

	// Check cache first
	if labels := s.getFromCache(cacheKey); labels != nil {
		if len(labels) > 0 {
			return &labels[0], nil
		}
		return nil, nil
	}

	// Fetch from API
	labels, err := s.client.GetByBAGObjectID(ctx, objectID)
	if err != nil {
		return nil, err
	}

	// Cache result
	s.setCache(cacheKey, labels)

	if len(labels) == 0 {
		return nil, nil
	}

	return &labels[0], nil
}

// Ping checks if the EP-Online API is available.
func (s *Service) Ping(ctx context.Context) error {
	return s.client.Ping(ctx)
}

// ClearCache removes all cached entries.
func (s *Service) ClearCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = make(map[string]cacheEntry)
}

func (s *Service) getFromCache(key string) []transport.EnergyLabel {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, ok := s.cache[key]
	if !ok {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
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

func buildAddressCacheKey(postcode, huisnummer, huisletter, toevoeging, detail string) string {
	return "addr:" + postcode + ":" + huisnummer + ":" + huisletter + ":" + toevoeging + ":" + detail
}

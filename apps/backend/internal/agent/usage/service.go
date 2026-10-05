package usage

import (
	"context"
	"sync"
	"time"
)

const defaultDeferralThresholdPct = 90.0

// registration binds a client to its cache key.
type registration struct {
	client   ProviderUsageClient
	cacheKey string
}

// UsageService is the top-level service for subscription utilization tracking.
// Callers register a ProviderUsageClient per agent profile at startup; the
// service caches results and provides a rate-limited gate check.
type UsageService struct {
	cache     *UsageCache
	mu        sync.RWMutex
	clients   map[string]registration // profileID → registration
	threshold float64
}

// NewUsageService creates a UsageService with the default deferral threshold.
func NewUsageService() *UsageService {
	return &UsageService{
		cache:     NewUsageCache(),
		clients:   make(map[string]registration),
		threshold: defaultDeferralThresholdPct,
	}
}

// Register binds a ProviderUsageClient to the given agent profile ID.
// cacheKey should be built with CacheKey(provider, credentialPath).
func (s *UsageService) Register(profileID string, client ProviderUsageClient, cacheKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[profileID] = registration{client: client, cacheKey: cacheKey}
}

// SetRecorder installs a callback invoked after each real successful live
// fetch, with the same cache key Register stored. It is the seam that lets a
// consumer build measured history without a second provider poll.
func (s *UsageService) SetRecorder(fn func(key string, usage *ProviderUsage)) {
	s.cache.SetOnFetched(fn)
}

// CacheKeyFor returns the cache key registered for a profile, or false when
// the profile has no registration (no live quota client).
func (s *UsageService) CacheKeyFor(profileID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reg, ok := s.clients[profileID]
	if !ok {
		return "", false
	}
	return reg.cacheKey, true
}

// LastError returns the most recent fetch error recorded for a cache key, or
// nil. Callers use it to distinguish "credentials expired" from "missing"
// without a second provider probe.
func (s *UsageService) LastError(cacheKey string) error {
	return s.cache.LastError(cacheKey)
}

// Invalidate drops the cached value and error for a cache key, so the next
// GetUsage re-fetches. Callers use it right after a credential is saved.
func (s *UsageService) Invalidate(cacheKey string) {
	s.cache.Invalidate(cacheKey)
}

// InvalidateAll drops every cached value and error, so the next lookups
// re-fetch. Callers use it right after a credential is saved or refreshed.
func (s *UsageService) InvalidateAll() {
	s.cache.InvalidateAll()
}

// GetUsage returns fresh-or-cached utilization for the agent profile.
// Returns nil, nil if the profile has no registered client.
func (s *UsageService) GetUsage(ctx context.Context, profileID string) (*ProviderUsage, error) {
	s.mu.RLock()
	reg, ok := s.clients[profileID]
	s.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	return s.cache.GetOrFetch(ctx, reg.cacheKey, reg.client.FetchUsage)
}

// IsPotentiallyRateLimited returns true when any utilization window is at or
// above the configured threshold (default 90%). resetAt is the earliest reset
// time among windows that breached the threshold; zero if not limited.
func (s *UsageService) IsPotentiallyRateLimited(
	ctx context.Context, profileID string,
) (bool, time.Time, error) {
	usage, err := s.GetUsage(ctx, profileID)
	if err != nil {
		return false, time.Time{}, err
	}
	if usage == nil {
		return false, time.Time{}, nil
	}
	var earliest time.Time
	limited := false
	for _, w := range usage.Windows {
		if w.UtilizationPct >= s.threshold {
			limited = true
			if earliest.IsZero() || w.ResetAt.Before(earliest) {
				earliest = w.ResetAt
			}
		}
	}
	return limited, earliest, nil
}

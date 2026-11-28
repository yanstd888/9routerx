// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package middleware

import (
	"context"
	"sync"
	"time"
)

// cachedTokenUsageTTL is how long one key's lifetime spend is served from the
// cache. The quota read is a whole-lifetime SUM over the ledger (an OR across
// the key's two spellings) per request on the hot path; a few seconds of
// staleness near an exhausted quota costs nothing the fail-open semantics do
// not already tolerate, and the refusal message still reports a spend that is
// within one TTL of exact.
const cachedTokenUsageTTL = 3 * time.Second

// cachedTokenUsageEntry is one key's cached spend.
type cachedTokenUsageEntry struct {
	used      int
	fetchedAt time.Time
}

// CachedTokenUsage wraps a TokenUsageReader and serves a key's lifetime spend
// from a short-TTL cache, so an armed token quota stops paying the ledger SUM
// on every request. Errors pass through uncached: a transient storage blip
// must not pin a zero spend for the whole TTL, and the limiter fails open on
// read errors anyway.
type CachedTokenUsage struct {
	inner TokenUsageReader
	ttl   time.Duration

	mu      sync.Mutex
	entries map[string]cachedTokenUsageEntry
}

// NewCachedTokenUsage wraps inner with the default TTL.
func NewCachedTokenUsage(inner TokenUsageReader) *CachedTokenUsage {
	return &CachedTokenUsage{inner: inner, ttl: cachedTokenUsageTTL, entries: make(map[string]cachedTokenUsageEntry)}
}

// NewCachedTokenUsageWithTTL wraps inner with an explicit TTL; tests use it to
// observe expiry without waiting on the clock.
func NewCachedTokenUsageWithTTL(inner TokenUsageReader, ttl time.Duration) *CachedTokenUsage {
	return &CachedTokenUsage{inner: inner, ttl: ttl, entries: make(map[string]cachedTokenUsageEntry)}
}

// GetApiKeyTokenUsageByID serves the key's spend from the cache when the entry
// is fresh, reading through and refreshing otherwise.
func (c *CachedTokenUsage) GetApiKeyTokenUsageByID(ctx context.Context, keyID string) (int, error) {
	c.mu.Lock()
	if entry, ok := c.entries[keyID]; ok && time.Since(entry.fetchedAt) <= c.ttl {
		used := entry.used
		c.mu.Unlock()
		return used, nil
	}
	c.mu.Unlock()

	used, err := c.inner.GetApiKeyTokenUsageByID(ctx, keyID)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	if c.entries == nil || len(c.entries) >= cachedTokenUsageMaxEntries {
		c.entries = make(map[string]cachedTokenUsageEntry, cachedTokenUsageMaxEntries)
	}
	c.entries[keyID] = cachedTokenUsageEntry{used: used, fetchedAt: time.Now()}
	c.mu.Unlock()
	return used, nil
}

// cachedTokenUsageMaxEntries bounds memory. The dashboard and the limiter see
// the same keys; a bespoke client cycling thousands of keys is dropped
// wholesale and a miss is one SUM.
const cachedTokenUsageMaxEntries = 4096

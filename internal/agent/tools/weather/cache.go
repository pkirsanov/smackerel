// Cache is the in-process LRU used by the weather tool. Cache hits
// PRESERVE the original Forecast.RetrievedAt timestamp (design §5.2).
//
// The cache is keyed by (provider, location, forecast_window) so two
// providers may have independent entries for the same location without
// cross-contamination. TTL is set at construction time and re-checked
// on every Get; expired entries are evicted lazily.
//
// The implementation is intentionally minimal: a fixed-capacity ring
// of entries, O(n) Get/Put. v1 traffic is per-conversation and bounded
// by the assistant rate limit, so the O(n) lookup is dominated by the
// network round-trip it avoids.

package weather

import (
	"strings"
	"sync"
	"time"
)

// Cache is an LRU cache for Forecast values with a uniform TTL.
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	cap     int
	entries []cacheEntry
	now     func() time.Time
}

type cacheEntry struct {
	key        string
	forecast   Forecast
	insertedAt time.Time
}

// NewCache constructs a Cache with the given uniform TTL and capacity.
// Both MUST be > 0; New panics otherwise to fail loudly at startup.
func NewCache(ttl time.Duration, capacity int) *Cache {
	if ttl <= 0 {
		panic("weather.NewCache: ttl must be > 0")
	}
	if capacity <= 0 {
		panic("weather.NewCache: capacity must be > 0")
	}
	return &Cache{
		ttl:     ttl,
		cap:     capacity,
		entries: make([]cacheEntry, 0, capacity),
		now:     time.Now,
	}
}

// withClock is a test-only hook to inject a deterministic clock.
func (c *Cache) withClock(now func() time.Time) *Cache {
	c.now = now
	return c
}

func cacheKey(provider, location string, window ForecastWindow) string {
	return strings.ToLower(provider) + "|" + strings.ToLower(strings.TrimSpace(location)) + "|" + string(window)
}

// Get returns the cached Forecast for (provider, location, window) if
// it is still within TTL. The returned Forecast preserves the original
// upstream RetrievedAt — callers MUST NOT overwrite it with the
// current time.
func (c *Cache) Get(provider, location string, window ForecastWindow) (Forecast, bool) {
	key := cacheKey(provider, location, window)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for i, e := range c.entries {
		if e.key != key {
			continue
		}
		if now.Sub(e.insertedAt) > c.ttl {
			// expired — evict
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			return Forecast{}, false
		}
		// move-to-front (LRU)
		c.entries = append(c.entries[:i], c.entries[i+1:]...)
		c.entries = append([]cacheEntry{e}, c.entries...)
		return e.forecast, true
	}
	return Forecast{}, false
}

// Put inserts or refreshes (provider, location, window) → forecast.
// The Forecast.RetrievedAt is stored verbatim; subsequent Gets return
// it unchanged.
func (c *Cache) Put(provider, location string, window ForecastWindow, forecast Forecast) {
	key := cacheKey(provider, location, window)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	// If key exists, replace in place (and move-to-front).
	for i, e := range c.entries {
		if e.key == key {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			break
		}
	}
	c.entries = append([]cacheEntry{{
		key:        key,
		forecast:   forecast,
		insertedAt: now,
	}}, c.entries...)
	if len(c.entries) > c.cap {
		// Evict the oldest entry.
		c.entries = c.entries[:c.cap]
	}
}

// Len returns the number of entries currently in the cache. Test-only.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

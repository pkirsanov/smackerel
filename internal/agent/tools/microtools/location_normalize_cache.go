// Spec 065 SCOPE-2 — tool-local LRU cache for location_normalize
// envelopes. Keyed by (provider, normalized input). Cache hits return
// the cached Envelope verbatim — Source.RetrievedAt is preserved so
// the trace renderer surfaces the original upstream moment.
//
// Failure envelopes (status="failed") are intentionally NOT cached so
// transient provider errors recover on the next call. The handler in
// location_normalize.go enforces this by skipping Put on failures.

package microtools

import (
	"strings"
	"sync"
	"time"
)

// LocationCache is a small TTL-bounded LRU for location envelopes.
type LocationCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	cap     int
	entries []locCacheEntry
	now     func() time.Time
}

type locCacheEntry struct {
	key        string
	envelope   Envelope
	insertedAt time.Time
}

// NewLocationCache constructs a cache with the given TTL and
// capacity. Both MUST be > 0; the constructor panics otherwise so
// misconfiguration is caught at startup, not at the first call.
func NewLocationCache(ttl time.Duration, capacity int) *LocationCache {
	if ttl <= 0 {
		panic("microtools.NewLocationCache: ttl must be > 0")
	}
	if capacity <= 0 {
		panic("microtools.NewLocationCache: capacity must be > 0")
	}
	return &LocationCache{
		ttl:     ttl,
		cap:     capacity,
		entries: make([]locCacheEntry, 0, capacity),
		now:     time.Now,
	}
}

func locCacheKey(provider, normalized string) string {
	return strings.ToLower(provider) + "|" + strings.ToLower(strings.TrimSpace(normalized))
}

// Get returns the cached Envelope for (provider, normalized) if it
// is still within TTL.
func (c *LocationCache) Get(provider, normalized string) (Envelope, bool) {
	key := locCacheKey(provider, normalized)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for i, e := range c.entries {
		if e.key != key {
			continue
		}
		if now.Sub(e.insertedAt) > c.ttl {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			return Envelope{}, false
		}
		c.entries = append(c.entries[:i], c.entries[i+1:]...)
		c.entries = append([]locCacheEntry{e}, c.entries...)
		return e.envelope, true
	}
	return Envelope{}, false
}

// Put inserts or refreshes the envelope. Failure envelopes are
// silently ignored to honor the "no caching of transient errors"
// rule documented at the top of this file.
func (c *LocationCache) Put(provider, normalized string, env Envelope) {
	if env.Status == StatusFailed {
		return
	}
	key := locCacheKey(provider, normalized)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for i, e := range c.entries {
		if e.key == key {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			break
		}
	}
	c.entries = append([]locCacheEntry{{
		key:        key,
		envelope:   env,
		insertedAt: now,
	}}, c.entries...)
	if len(c.entries) > c.cap {
		c.entries = c.entries[:c.cap]
	}
}

// Len returns the current entry count. Test-only.
func (c *LocationCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Spec 072 SCOPE-3 — Inbound-delivery idempotency cache.
//
// Meta retries the same WhatsApp Business webhook with the same
// `messages[].id` (a `wamid.*` token) until the receiver returns a
// 2xx. The adapter MUST treat the message id as the idempotency key
// so the capability facade and the capture-as-fallback hook each run
// at most once per real inbound message, regardless of how many
// times Meta retries.
//
// Design constraints:
//
//   - The cache is in-process: a single core replica owns the
//     webhook ingress, and Meta retries within a small window
//     (minutes), so a process-local LRU is sufficient and avoids
//     a Postgres round-trip on every delivery.
//   - The cache is BOUNDED. A pathological flood of unique ids
//     MUST NOT grow without limit; the FIFO eviction trades
//     correctness for a much older id (which Meta will not retry
//     anyway) for memory safety.
//   - The cache stores ONLY the opaque TransportMessageID — no
//     payload, body, or PII (Principle 8 + spec 072 §8).
//
// The cache is NOT a deduplication store for facade-side replay
// protection (confirm refs, scenario idempotency keys, etc.).
// Those live inside the facade. This cache exists solely so the
// WhatsApp adapter can swallow Meta's transport-layer retries
// before they reach the facade.

package assistant_adapter

import (
	"container/list"
	"sync"
)

// IdempotencyCacheCapacity caps the per-process retained message-id
// set. At ~50 bytes per id (Meta `wamid.*` strings) this is well
// under 1 MiB at default capacity and bounds memory under flood.
const IdempotencyCacheCapacity = 16384

// idempotencyCache is a bounded FIFO set of seen TransportMessageIDs.
// Safe for concurrent use.
type idempotencyCache struct {
	mu       sync.Mutex
	capacity int
	order    *list.List               // front = newest, back = oldest
	index    map[string]*list.Element // id → element in order
}

func newIdempotencyCache(capacity int) *idempotencyCache {
	if capacity <= 0 {
		capacity = IdempotencyCacheCapacity
	}
	return &idempotencyCache{
		capacity: capacity,
		order:    list.New(),
		index:    make(map[string]*list.Element, capacity),
	}
}

// markSeen records id as seen and reports whether it was already
// present. Returns true when id is a duplicate (previously seen) and
// false when it was newly inserted. Empty ids are never recorded
// and always return false — the caller MUST upstream-reject empty
// TransportMessageIDs as malformed.
func (c *idempotencyCache) markSeen(id string) bool {
	if id == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.index[id]; ok {
		return true
	}
	elem := c.order.PushFront(id)
	c.index[id] = elem
	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			if s, ok := oldest.Value.(string); ok {
				delete(c.index, s)
			}
		}
	}
	return false
}

// size returns the current number of retained ids. Test-only.
func (c *idempotencyCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.index)
}

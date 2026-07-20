package httpadapter

import (
	"container/list"
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// HTTPTurnDedupCapacity bounds retained HTTP responses under a flood of unique
// transport message IDs. It mirrors the established bounded transport-cache
// posture used by the WhatsApp adapter.
const HTTPTurnDedupCapacity = 16384

var errTransportMessageIDConflict = errors.New("transport_message_id reused with a different request payload")
var errTurnDedupCapacity = errors.New("HTTP turn dedup cache is at capacity with in-flight requests")

type turnDedupKey struct {
	userDigest [sha256.Size]byte
	transport  string
	messageID  string
}

type turnDedupResult struct {
	status   int
	response TurnResponse
}

type turnDedupEntry struct {
	fingerprint [sha256.Size]byte
	ready       chan struct{}
	result      turnDedupResult
	expiresAt   time.Time
	completed   bool
	element     *list.Element
}

type turnResponseCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	now      func() time.Time
	order    *list.List
	entries  map[turnDedupKey]*turnDedupEntry
}

type turnDedupLease struct {
	cache *turnResponseCache
	key   turnDedupKey
	entry *turnDedupEntry
	owner bool
}

func newTurnResponseCache(capacity int, ttl time.Duration, now func() time.Time) (*turnResponseCache, error) {
	if capacity <= 0 {
		return nil, errors.New("httpadapter: turn dedup capacity must be positive")
	}
	if ttl <= 0 {
		return nil, errors.New("httpadapter: turn dedup TTL must be positive")
	}
	if now == nil {
		return nil, errors.New("httpadapter: turn dedup clock is required")
	}
	return &turnResponseCache{
		capacity: capacity,
		ttl:      ttl,
		now:      now,
		order:    list.New(),
		entries:  make(map[turnDedupKey]*turnDedupEntry, capacity),
	}, nil
}

func (c *turnResponseCache) begin(userID, messageID string, fingerprint [sha256.Size]byte) (*turnDedupLease, error) {
	key := turnDedupKey{
		userDigest: sha256.Sum256([]byte(userID)),
		transport:  TransportName,
		messageID:  messageID,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeExpiredLocked(c.now())
	if entry, ok := c.entries[key]; ok {
		if entry.fingerprint != fingerprint {
			return nil, errTransportMessageIDConflict
		}
		return &turnDedupLease{cache: c, key: key, entry: entry}, nil
	}
	if len(c.entries) >= c.capacity && !c.evictOldestCompletedLocked() {
		return nil, errTurnDedupCapacity
	}

	entry := &turnDedupEntry{
		fingerprint: fingerprint,
		ready:       make(chan struct{}),
	}
	entry.element = c.order.PushFront(key)
	c.entries[key] = entry
	c.evictCompletedLocked()
	return &turnDedupLease{cache: c, key: key, entry: entry, owner: true}, nil
}

func (l *turnDedupLease) wait(ctx context.Context) (turnDedupResult, bool) {
	select {
	case <-l.entry.ready:
		return l.entry.result, true
	case <-ctx.Done():
		return turnDedupResult{}, false
	}
}

func (l *turnDedupLease) complete(result turnDedupResult) {
	if !l.owner {
		return
	}
	l.cache.mu.Lock()
	defer l.cache.mu.Unlock()
	current, ok := l.cache.entries[l.key]
	if !ok || current != l.entry || current.completed {
		return
	}
	current.result = result
	current.expiresAt = l.cache.now().Add(l.cache.ttl)
	current.completed = true
	l.cache.order.MoveToFront(current.element)
	close(current.ready)
	l.cache.evictCompletedLocked()
}

func (c *turnResponseCache) removeExpiredLocked(now time.Time) {
	for key, entry := range c.entries {
		if !entry.completed || now.Before(entry.expiresAt) {
			continue
		}
		c.order.Remove(entry.element)
		delete(c.entries, key)
	}
}

func (c *turnResponseCache) evictCompletedLocked() {
	for len(c.entries) > c.capacity {
		if !c.evictOldestCompletedLocked() {
			return
		}
	}
}

func (c *turnResponseCache) evictOldestCompletedLocked() bool {
	for element := c.order.Back(); element != nil; element = element.Prev() {
		key, ok := element.Value.(turnDedupKey)
		if !ok || !c.entries[key].completed {
			continue
		}
		c.order.Remove(element)
		delete(c.entries, key)
		return true
	}
	return false
}

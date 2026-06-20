// BUG-001 fix — Per-user rate limiter for WhatsApp webhook ingress.
//
// The limiter uses a simple sliding-window approach: each user_id
// gets a bucket that tracks the count of requests in the current
// 60-second window. When the window expires, the bucket resets.
// This is simpler than a true sliding window or token bucket but
// sufficient for the per-user rate limiting requirement.
//
// The limiter is safe for concurrent use and bounded by an LRU-style
// eviction when the number of tracked users exceeds a cap (default
// 65536), ensuring memory stays bounded under flood.

package assistant_adapter

import (
	"container/list"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var webhookRateLimitExceeded = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "assistant_whatsapp_webhook_ratelimit_exceeded_total",
		Help: "WhatsApp webhook deliveries rejected for per-user rate limit exceeded.",
	},
)

func init() {
	prometheus.MustRegister(webhookRateLimitExceeded)
}

// PerUserLimiterCapacity caps the number of tracked user buckets.
// At ~100 bytes per bucket this is well under 10 MiB at default
// capacity and bounds memory under user-count explosion.
const PerUserLimiterCapacity = 65536

// perUserLimiter implements a simple fixed-window per-user rate
// limiter. Each user gets perMinute requests per 60-second window.
type perUserLimiter struct {
	mu        sync.Mutex
	perMinute int
	window    time.Duration
	capacity  int
	buckets   map[string]*userBucket
	order     *list.List               // front = most recently used
	index     map[string]*list.Element // user_id → element in order
	nowFunc   func() time.Time         // injectable for tests
}

type userBucket struct {
	count       int
	windowStart time.Time
}

func newPerUserLimiter(perMinute int) *perUserLimiter {
	if perMinute <= 0 {
		perMinute = 30 // defensive fallback, should never hit due to SST validation
	}
	return &perUserLimiter{
		perMinute: perMinute,
		window:    60 * time.Second,
		capacity:  PerUserLimiterCapacity,
		buckets:   make(map[string]*userBucket, 256),
		order:     list.New(),
		index:     make(map[string]*list.Element, 256),
		nowFunc:   time.Now,
	}
}

// Allow checks whether the user has budget remaining in the current
// window. Returns true if allowed (and increments count), false if
// rate limited. Empty user_id is always rejected.
func (l *perUserLimiter) Allow(userID string) bool {
	if userID == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowFunc()
	bucket, exists := l.buckets[userID]

	if exists {
		// Move to front (LRU)
		if elem, ok := l.index[userID]; ok {
			l.order.MoveToFront(elem)
		}
		// Check if window expired
		if now.Sub(bucket.windowStart) >= l.window {
			bucket.count = 1
			bucket.windowStart = now
			return true
		}
		// Within window — check limit
		if bucket.count >= l.perMinute {
			return false
		}
		bucket.count++
		return true
	}

	// New user — evict oldest if at capacity
	if len(l.buckets) >= l.capacity {
		oldest := l.order.Back()
		if oldest != nil {
			oldID := oldest.Value.(string)
			l.order.Remove(oldest)
			delete(l.index, oldID)
			delete(l.buckets, oldID)
		}
	}

	// Create new bucket
	l.buckets[userID] = &userBucket{count: 1, windowStart: now}
	elem := l.order.PushFront(userID)
	l.index[userID] = elem
	return true
}

// size returns the current number of tracked users. Test-only.
func (l *perUserLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// reset clears all tracked users. Test-only.
func (l *perUserLimiter) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = make(map[string]*userBucket, 256)
	l.order = list.New()
	l.index = make(map[string]*list.Element, 256)
}

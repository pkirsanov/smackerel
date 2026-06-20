// BUG-001 — Per-user rate limiter unit tests.
//
// TP-BUG001-01: Per-user limiter allows N then blocks N+1
// TP-BUG001-02: Limiter resets after window expiry

package assistant_adapter

import (
	"sync"
	"testing"
	"time"
)

func TestPerUserLimiter_AllowsThenBlocks(t *testing.T) {
	// TP-BUG001-01: Per-user limiter allows N then blocks N+1
	limiter := newPerUserLimiter(3) // 3 per minute

	userID := "user-001"

	// First 3 requests should succeed
	for i := 1; i <= 3; i++ {
		if !limiter.Allow(userID) {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	// 4th request should be blocked
	if limiter.Allow(userID) {
		t.Fatal("4th request should be rate limited")
	}

	// Different user should still be allowed
	if !limiter.Allow("user-002") {
		t.Fatal("different user should be allowed")
	}
}

func TestPerUserLimiter_ResetsAfterWindow(t *testing.T) {
	// TP-BUG001-02: Limiter resets after window expiry
	limiter := newPerUserLimiter(2)

	// Mock time
	now := time.Now()
	limiter.nowFunc = func() time.Time { return now }

	userID := "user-001"

	// Use up the quota
	if !limiter.Allow(userID) {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow(userID) {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow(userID) {
		t.Fatal("third request should be rate limited")
	}

	// Advance time past the window
	now = now.Add(61 * time.Second)

	// Should be allowed again
	if !limiter.Allow(userID) {
		t.Fatal("request after window reset should be allowed")
	}
}

func TestPerUserLimiter_RejectsEmptyUserID(t *testing.T) {
	limiter := newPerUserLimiter(10)

	if limiter.Allow("") {
		t.Fatal("empty user_id should be rejected")
	}
}

func TestPerUserLimiter_EvictsOldestOnCapacity(t *testing.T) {
	limiter := newPerUserLimiter(10)
	limiter.capacity = 3 // Small capacity for test

	// Fill to capacity
	limiter.Allow("user-a")
	limiter.Allow("user-b")
	limiter.Allow("user-c")

	if limiter.size() != 3 {
		t.Fatalf("expected 3 users, got %d", limiter.size())
	}

	// Add one more — should evict oldest (user-a)
	limiter.Allow("user-d")

	if limiter.size() != 3 {
		t.Fatalf("expected 3 users after eviction, got %d", limiter.size())
	}

	// user-a should be gone and treated as new (gets full quota)
	// user-d used 1, so it should have 9 left
	// We can verify user-a is gone by checking it gets a fresh window
	limiter.reset()
	limiter.capacity = 3
	limiter.Allow("user-a")
	limiter.Allow("user-b")
	limiter.Allow("user-c")
	limiter.Allow("user-d") // evicts user-a

	// Now user-a is evicted, should get fresh quota
	for i := 0; i < 10; i++ {
		if !limiter.Allow("user-a") {
			t.Fatalf("user-a request %d should be allowed (fresh after eviction)", i+1)
		}
	}
	// 11th should fail
	if limiter.Allow("user-a") {
		t.Fatal("user-a request 11 should be rate limited")
	}
}

func TestPerUserLimiter_ConcurrentSafety(t *testing.T) {
	limiter := newPerUserLimiter(100)
	userID := "concurrent-user"

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- limiter.Allow(userID)
		}()
	}

	wg.Wait()
	close(allowed)

	allowedCount := 0
	for ok := range allowed {
		if ok {
			allowedCount++
		}
	}

	// Exactly 100 should be allowed (the rate limit)
	if allowedCount != 100 {
		t.Fatalf("expected exactly 100 allowed, got %d", allowedCount)
	}
}

func TestPerUserLimiter_IndependentUsers(t *testing.T) {
	limiter := newPerUserLimiter(5)

	// Use up user-1's quota
	for i := 0; i < 5; i++ {
		if !limiter.Allow("user-1") {
			t.Fatalf("user-1 request %d should be allowed", i+1)
		}
	}
	if limiter.Allow("user-1") {
		t.Fatal("user-1 should be rate limited")
	}

	// user-2 should have independent quota
	for i := 0; i < 5; i++ {
		if !limiter.Allow("user-2") {
			t.Fatalf("user-2 request %d should be allowed (independent quota)", i+1)
		}
	}
	if limiter.Allow("user-2") {
		t.Fatal("user-2 should be rate limited")
	}
}

package surfacing

import (
	"sync"
	"time"
)

// BudgetTracker enforces the per-user daily nudge ceiling across every
// surfacing channel. The implementation is intentionally process-local
// and in-memory: smackerel is a single-user, single-process deployment
// today, and the cross-channel boundary lives inside this process. A
// future multi-process deployment would back this with Redis or a
// database table keyed on (user_id, day).
type BudgetTracker struct {
	mu         sync.Mutex
	dailyLimit int
	delivered  int
	overrides  int
	currentDay string
	clock      func() time.Time
}

// NewBudgetTracker constructs a tracker with the SST-supplied daily limit.
// limit MUST be > 0; the caller validates SST upstream.
func NewBudgetTracker(limit int) *BudgetTracker {
	return &BudgetTracker{dailyLimit: limit, clock: time.Now}
}

// rollover resets counters when the day changes. Caller MUST hold mu.
func (b *BudgetTracker) rollover() {
	today := b.clock().UTC().Format("2006-01-02")
	if today != b.currentDay {
		b.currentDay = today
		b.delivered = 0
		b.overrides = 0
	}
}

// Remaining returns the number of nudges still permitted today.
func (b *BudgetTracker) Remaining() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollover()
	rem := b.dailyLimit - b.delivered
	if rem < 0 {
		return 0
	}
	return rem
}

// TryConsume attempts to claim one slot in today's budget. Returns true
// when the slot was reserved; false when the budget is exhausted (caller
// then decides whether to invoke RecordOverride for an urgent escalation).
func (b *BudgetTracker) TryConsume() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollover()
	if b.delivered >= b.dailyLimit {
		return false
	}
	b.delivered++
	return true
}

// RecordOverride accounts for an urgent escalation that bypassed the
// budget. The override is still recorded so per-channel safety nets
// and the budget_overrides_total counter remain truthful.
func (b *BudgetTracker) RecordOverride() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollover()
	b.overrides++
}

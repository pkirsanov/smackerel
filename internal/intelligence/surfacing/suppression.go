package surfacing

import (
	"sync"
	"time"
)

// AckLookup is the read interface the suppression module needs from the
// annotation/alert subsystems. Implementations consult dismissed alerts
// (internal/intelligence.DismissAlert), "not useful" annotations
// (internal/annotation.InteractionType extensions in spec 027), or any
// equivalent acknowledgement signal. The interface keeps the surfacing
// package free of import cycles with internal/intelligence.
type AckLookup interface {
	// LastAcknowledged returns the most recent acknowledgement time for
	// contentKey, or the zero time if none. ok is false when no
	// acknowledgement has ever been recorded.
	LastAcknowledged(contentKey string) (when time.Time, ok bool)
}

// InMemoryAck is a process-local AckLookup suitable for the single-user
// MVP. Production wiring also calls Acknowledge() when DismissAlert /
// "not useful" annotations land, so cross-channel follow-ups are
// suppressed within the configured window.
type InMemoryAck struct {
	mu      sync.Mutex
	entries map[string]time.Time
	clock   func() time.Time

	// nextSweep mirrors DedupeIndex: zero value means "eligible now".
	nextSweep time.Time
	// sweeps counts completed GC sweeps, for deterministic assertion.
	sweeps int
}

// ackRetention bounds opportunistic GC for the in-memory ack registry.
// Suppression windows are owned by SuppressionWindow (typically hours);
// any entry older than this floor is unreachable by any plausible window.
const ackRetention = 30 * 24 * time.Hour

// ackSweepInterval rate-limits the opportunistic sweep so Acknowledge
// stays O(1) amortized. See DedupeIndex.Record for the full rationale.
const ackSweepInterval = 24 * time.Hour

// NewInMemoryAck returns an empty in-memory ack registry.
func NewInMemoryAck() *InMemoryAck {
	return &InMemoryAck{entries: make(map[string]time.Time), clock: time.Now}
}

// Acknowledge records that the user dismissed/acked contentKey.
func (a *InMemoryAck) Acknowledge(contentKey string) {
	if contentKey == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.clock()
	a.entries[contentKey] = now
	// Opportunistic GC, AMORTIZED — see DedupeIndex.Record for the full
	// rationale. Sweeping on every call is O(n) once the live set
	// exceeds the threshold, which makes a sustained burst quadratic.
	if len(a.entries) > gcSizeThreshold && !now.Before(a.nextSweep) {
		cutoff := now.Add(-ackRetention)
		for k, t := range a.entries {
			if t.Before(cutoff) {
				delete(a.entries, k)
			}
		}
		a.nextSweep = now.Add(ackSweepInterval)
		a.sweeps++
	}
}

// LastAcknowledged satisfies AckLookup.
func (a *InMemoryAck) LastAcknowledged(contentKey string) (time.Time, bool) {
	if contentKey == "" {
		return time.Time{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.entries[contentKey]
	return t, ok
}

// SuppressionWindow applies the configured suppression window to ack
// lookups. A candidate whose contentKey was acknowledged within the
// window is suppressed regardless of producer or channel.
type SuppressionWindow struct {
	window time.Duration
	lookup AckLookup
	clock  func() time.Time
}

// NewSuppressionWindow constructs a window helper. windowHours MUST be > 0;
// lookup MAY be nil, in which case suppression always returns false.
func NewSuppressionWindow(windowHours int, lookup AckLookup) *SuppressionWindow {
	return &SuppressionWindow{
		window: time.Duration(windowHours) * time.Hour,
		lookup: lookup,
		clock:  time.Now,
	}
}

// IsSuppressed returns true when contentKey was acknowledged within the
// configured suppression window.
func (s *SuppressionWindow) IsSuppressed(contentKey string) bool {
	if s == nil || s.lookup == nil || contentKey == "" {
		return false
	}
	last, ok := s.lookup.LastAcknowledged(contentKey)
	if !ok {
		return false
	}
	return s.clock().Sub(last) < s.window
}

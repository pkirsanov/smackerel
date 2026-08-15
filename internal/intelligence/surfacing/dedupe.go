package surfacing

import (
	"sync"
	"time"
)

// DedupeIndex tracks the most-recent delivery time per ContentKey so a
// candidate carrying the same key inside the dedupe window collapses to
// one delivery across all channels (SCN-021-017). Process-local memory
// is sufficient for the single-process MVP deployment.
type DedupeIndex struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]time.Time
	clock   func() time.Time

	// nextSweep is the earliest time the opportunistic GC below may run
	// again. Zero value means "eligible now", so the first sweep after
	// the size threshold is crossed is never delayed.
	nextSweep time.Time
	// sweeps counts completed GC sweeps. It exists so the regression
	// test can assert the sweep RATE deterministically instead of
	// timing the map scan, which would be flaky on a loaded host.
	sweeps int
}

// gcSizeThreshold is the entry count above which the opportunistic
// sweeps in this package become eligible to run. Shared by DedupeIndex
// and InMemoryAck so both bound memory on the same rule.
const gcSizeThreshold = 4096

// NewDedupeIndex constructs an index with the SST-supplied dedupe window
// (hours -> Duration). windowHours MUST be > 0; SST validation enforces.
func NewDedupeIndex(windowHours int) *DedupeIndex {
	return &DedupeIndex{
		window:  time.Duration(windowHours) * time.Hour,
		entries: make(map[string]time.Time),
		clock:   time.Now,
	}
}

// IsDuplicate returns true when contentKey was recorded as delivered
// within the configured dedupe window. Empty keys are never duplicates —
// the caller MUST set ContentKey for items that should dedupe.
func (d *DedupeIndex) IsDuplicate(contentKey string) bool {
	if contentKey == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	last, ok := d.entries[contentKey]
	if !ok {
		return false
	}
	return d.clock().Sub(last) < d.window
}

// Record marks contentKey as delivered now. Callers invoke Record after
// the controller verdict is Permit/Escalated so future candidates see
// the recent delivery.
func (d *DedupeIndex) Record(contentKey string) {
	if contentKey == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.clock()
	d.entries[contentKey] = now
	// Opportunistic GC, AMORTIZED.
	//
	// A sweep can only reclaim entries older than 2x window, so gating
	// it on size ALONE is pathological in exactly the regime that
	// matters: once more than gcSizeThreshold keys are live INSIDE the
	// window, no entry is evictable, yet every Record still paid a full
	// O(n) scan that deleted nothing — making a sustained burst of
	// distinct keys O(n^2) overall. Rate-limiting the sweep to once per
	// window keeps Record O(1) amortized while still bounding entry age
	// at ~3x window (2x window to expire, plus up to one window before
	// the next sweep observes it).
	if len(d.entries) > gcSizeThreshold && !now.Before(d.nextSweep) {
		cutoff := now.Add(-2 * d.window)
		for k, t := range d.entries {
			if t.Before(cutoff) {
				delete(d.entries, k)
			}
		}
		d.nextSweep = now.Add(d.window)
		d.sweeps++
	}
}

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
}

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
	d.entries[contentKey] = d.clock()
	// Opportunistic GC: drop entries older than 2× window to keep memory
	// bounded over long uptimes without paying a periodic-sweep tax.
	if len(d.entries) > 4096 {
		cutoff := d.clock().Add(-2 * d.window)
		for k, t := range d.entries {
			if t.Before(cutoff) {
				delete(d.entries, k)
			}
		}
	}
}

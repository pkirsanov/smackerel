package surfacing

import (
	"fmt"
	"testing"
	"time"
)

// Adversarial regression for the O(n^2) opportunistic sweep.
//
// The defect: the GC in DedupeIndex.Record / InMemoryAck.Acknowledge was
// gated on SIZE alone. Once more than gcSizeThreshold keys were live
// INSIDE the retention window, no entry was evictable, yet every single
// subsequent call still paid a full O(n) map scan that deleted nothing.
// A sustained burst of distinct keys was therefore quadratic, which is
// what pushed the spec-107 card-projection hot path over its 5 ms p99
// ceiling (tests/stress/proactive) at 50k iterations.
//
// These tests assert the sweep RATE, not elapsed time. A timing bound
// would be flaky on a loaded host — the same class of defect being
// fixed here — whereas the sweep counter is exact and deterministic.

func TestDedupeGC_SustainedDistinctKeysDoNotResweep(t *testing.T) {
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	d := NewDedupeIndex(6)
	d.clock = func() time.Time { return now }

	const burst = 20000
	for i := 0; i < burst; i++ {
		d.Record(fmt.Sprintf("fresh-%d", i))
	}

	// Every key is inside the window, so a sweep can reclaim nothing.
	if got := len(d.entries); got != burst {
		t.Fatalf("entries=%d want %d — nothing is evictable here", got, burst)
	}

	// Size-only gating would sweep on EVERY Record past the threshold:
	// burst-gcSizeThreshold = 15904 full scans that free nothing.
	if d.sweeps > 1 {
		t.Fatalf("dedupe swept %d times over a fresh burst of %d; want <=1 "+
			"(size-only gating is O(n^2))", d.sweeps, burst)
	}
}

func TestDedupeGC_StaleEntriesStillReclaimedAfterInterval(t *testing.T) {
	base := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	clock := base
	d := NewDedupeIndex(1) // window=1h => GC cutoff = 2h
	d.clock = func() time.Time { return clock }

	for i := 0; i < gcSizeThreshold+1; i++ {
		d.entries[fmt.Sprintf("stale-%d", i)] = base.Add(-10 * time.Hour)
	}
	// nextSweep zero value => the first sweep is never delayed.
	d.Record("trigger-1")
	if got := len(d.entries); got != 1 {
		t.Fatalf("after first sweep entries=%d want 1", got)
	}

	// Refill with stale entries and prove a LATER sweep still reclaims,
	// so amortizing rate-limited the GC without switching it off.
	for i := 0; i < gcSizeThreshold+1; i++ {
		d.entries[fmt.Sprintf("stale2-%d", i)] = clock.Add(-10 * time.Hour)
	}
	clock = clock.Add(2 * time.Hour) // past nextSweep (= base+1h)
	d.Record("trigger-2")
	if got := len(d.entries); got > 2 {
		t.Fatalf("after second sweep entries=%d want <=2 — stale entries "+
			"must still be reclaimed once the interval elapses", got)
	}
}

func TestAckGC_SustainedDistinctKeysDoNotResweep(t *testing.T) {
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	a := NewInMemoryAck()
	a.clock = func() time.Time { return now }

	const burst = 20000
	for i := 0; i < burst; i++ {
		a.Acknowledge(fmt.Sprintf("ack-%d", i))
	}

	if got := len(a.entries); got != burst {
		t.Fatalf("entries=%d want %d — nothing is evictable here", got, burst)
	}
	if a.sweeps > 1 {
		t.Fatalf("ack swept %d times over a fresh burst of %d; want <=1 "+
			"(size-only gating is O(n^2))", a.sweeps, burst)
	}
}

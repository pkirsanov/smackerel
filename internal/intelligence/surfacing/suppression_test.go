package surfacing

import (
	"fmt"
	"testing"
	"time"
)

// STAB-078-S03 — InMemoryAck opportunistic GC keeps map bounded.
func TestInMemoryAck_OpportunisticEvictionDropsExpiredEntries(t *testing.T) {
	now := time.Now()
	ack := NewInMemoryAck()
	ack.clock = func() time.Time { return now.Add(-2 * ackRetention) }

	// Seed > GC threshold (4096) with entries timestamped well beyond the
	// retention floor so the sweep on the next Acknowledge evicts them.
	for i := 0; i < 4100; i++ {
		ack.Acknowledge(fmt.Sprintf("old-%d", i))
	}
	if got := len(ack.entries); got < 4100 {
		t.Fatalf("seed: want >=4100 entries, got %d", got)
	}

	// Advance clock past retention horizon and add one fresh entry — the
	// GC branch fires because len(entries) > 4096.
	ack.clock = func() time.Time { return now }
	ack.Acknowledge("fresh")

	if _, ok := ack.entries["fresh"]; !ok {
		t.Fatalf("fresh entry missing after GC")
	}
	if got := len(ack.entries); got != 1 {
		t.Fatalf("expired entries not evicted: want 1 entry, got %d", got)
	}
}

func TestInMemoryAck_GCKeepsRecentEntriesWithinRetention(t *testing.T) {
	now := time.Now()
	ack := NewInMemoryAck()
	ack.clock = func() time.Time { return now }

	for i := 0; i < 4100; i++ {
		ack.Acknowledge(fmt.Sprintf("recent-%d", i))
	}

	// All entries are within retention; GC must not evict any.
	if got := len(ack.entries); got != 4100 {
		t.Fatalf("recent entries should be retained: want 4100, got %d", got)
	}
}

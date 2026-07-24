package proactive

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

func TestNudgeRegistry_MintResolveConsume(t *testing.T) {
	reg := NewNudgeRegistry(6 * time.Hour)

	ref := reg.Mint("artifact-42", surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")
	if ref == "" {
		t.Fatalf("Mint returned empty ref")
	}
	if len(ref) != 26 {
		t.Errorf("ref %q length = %d, want 26 (ULID-shaped)", ref, len(ref))
	}
	if strings.Contains(string(ref), "artifact-42") {
		t.Fatalf("ref leaks content_key: %q", ref)
	}

	// Peek is non-consuming and returns the resolved routing view.
	resolved, status := reg.Peek(ref)
	if status != ResolveOK {
		t.Fatalf("Peek status = %v, want ResolveOK", status)
	}
	if resolved.ContentKey != "artifact-42" || resolved.Principal != "user-1" {
		t.Errorf("Peek resolved = %+v, want content_key=artifact-42 principal=user-1", resolved)
	}
	if resolved.Channel != surfacing.ChannelTelegram || resolved.Producer != surfacing.ProducerAlerts {
		t.Errorf("Peek resolved channel/producer = %v/%v", resolved.Channel, resolved.Producer)
	}

	// First Consume succeeds; second is already-handled (idempotent).
	if _, s := reg.Consume(ref); s != ResolveOK {
		t.Fatalf("first Consume = %v, want ResolveOK", s)
	}
	if _, s := reg.Consume(ref); s != ResolveAlreadyHandled {
		t.Fatalf("second Consume = %v, want ResolveAlreadyHandled", s)
	}
	// Peek after consume also reports already-handled.
	if _, s := reg.Peek(ref); s != ResolveAlreadyHandled {
		t.Fatalf("Peek after consume = %v, want ResolveAlreadyHandled", s)
	}
}

func TestNudgeRegistry_MintProducesDistinctRefs(t *testing.T) {
	reg := NewNudgeRegistry(6 * time.Hour)
	seen := make(map[NudgeRef]bool)
	for i := 0; i < 1000; i++ {
		ref := reg.Mint("k", surfacing.ProducerDigest, surfacing.ChannelWebPush, "u")
		if seen[ref] {
			t.Fatalf("duplicate ref minted: %q", ref)
		}
		seen[ref] = true
	}
}

func TestNudgeRegistry_UnknownRefIsExpired(t *testing.T) {
	reg := NewNudgeRegistry(6 * time.Hour)
	if _, s := reg.Peek("never-minted"); s != ResolveExpired {
		t.Fatalf("Peek(unknown) = %v, want ResolveExpired", s)
	}
	if _, s := reg.Consume("never-minted"); s != ResolveExpired {
		t.Fatalf("Consume(unknown) = %v, want ResolveExpired", s)
	}
}

// TestNudgeRegistry_TTLExpiry proves a ref past its TTL resolves to
// ResolveExpired using an injected clock (a late tap on any channel then renders
// an honest expired state).
func TestNudgeRegistry_TTLExpiry(t *testing.T) {
	reg := NewNudgeRegistry(6 * time.Hour)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	reg.clock = func() time.Time { return now }

	ref := reg.Mint("artifact-late", surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")
	if _, s := reg.Peek(ref); s != ResolveOK {
		t.Fatalf("fresh ref Peek = %v, want ResolveOK", s)
	}

	// Advance to exactly the TTL boundary: expired (>= ttl).
	now = now.Add(6 * time.Hour)
	if _, s := reg.Peek(ref); s != ResolveExpired {
		t.Fatalf("ref at TTL boundary = %v, want ResolveExpired", s)
	}
}

// TestNudgeRegistry_GCEvictsExpired proves opportunistic GC bounds memory: after
// the map grows past the GC threshold with expired entries, a new Mint sweeps
// them.
func TestNudgeRegistry_GCEvictsExpired(t *testing.T) {
	reg := NewNudgeRegistry(1 * time.Hour)
	base := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	reg.clock = func() time.Time { return base }

	// Fill past the GC threshold with entries that will all expire.
	for i := 0; i < nudgeRegistryGCThreshold+1; i++ {
		reg.Mint("k", surfacing.ProducerDigest, surfacing.ChannelNtfy, "u")
	}
	if got := len(reg.entries); got <= nudgeRegistryGCThreshold {
		t.Fatalf("pre-GC size = %d, want > %d", got, nudgeRegistryGCThreshold)
	}

	// Advance past TTL and mint once more to trigger the opportunistic sweep.
	reg.clock = func() time.Time { return base.Add(2 * time.Hour) }
	reg.Mint("fresh", surfacing.ProducerDigest, surfacing.ChannelNtfy, "u")
	if got := len(reg.entries); got != 1 {
		t.Fatalf("post-GC size = %d, want 1 (only the fresh entry survives)", got)
	}
}

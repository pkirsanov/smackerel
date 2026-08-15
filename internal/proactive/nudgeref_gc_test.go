package proactive

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// TestNudgeRegistry_GCDoesNotResweepOnEveryMint is the adversarial
// regression for the O(n^2) opportunistic sweep in gcLocked.
//
// The defect: gcLocked was gated on SIZE alone and Mint calls it on
// every ref. Once more than nudgeRegistryGCThreshold refs were live
// INSIDE the ttl, nothing was evictable, yet every Mint still paid a
// full O(n) map scan that deleted nothing. Because Mint sits on the
// card-projection hot path, a sustained burst was quadratic and broke
// the NFR-107-001 5 ms p99 ceiling at 50k iterations
// (tests/stress/proactive/proactive_hotpath_test.go).
//
// This asserts the sweep RATE, not elapsed time: a timing bound would
// be flaky on a loaded host, which is the very failure mode being fixed.
func TestNudgeRegistry_GCDoesNotResweepOnEveryMint(t *testing.T) {
	base := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	reg := NewNudgeRegistry(6 * time.Hour)
	reg.clock = func() time.Time { return base }

	const burst = 20000
	for i := 0; i < burst; i++ {
		reg.Mint("k", surfacing.ProducerDigest, surfacing.ChannelNtfy, "u")
	}

	// Every ref was minted at base and ttl is 6h, so a sweep can
	// reclaim nothing here.
	if got := len(reg.entries); got != burst {
		t.Fatalf("entries=%d want %d — nothing is evictable inside ttl", got, burst)
	}

	// Size-only gating would sweep on EVERY Mint past the threshold:
	// burst-nudgeRegistryGCThreshold = 15904 full scans freeing nothing.
	if reg.sweeps > 1 {
		t.Fatalf("registry swept %d times over a fresh burst of %d; want <=1 "+
			"(size-only gating is O(n^2) on the card-projection hot path)",
			reg.sweeps, burst)
	}
}

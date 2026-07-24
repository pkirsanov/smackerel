package proactive

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// TestNFR107001_CardProjectionHotPathP99 proves the NFR-107-001 hot-path budget:
// building a ProactiveCardModel (controller Propose -> Mint -> ProjectCard) adds
// no I/O and stays far under the <5ms p99 ceiling. It runs in the fast unit lane
// (pure in-memory, no datastore) so the property is guarded on every commit; the
// stress lane carries the same assertion under the live-stack profile.
func TestNFR107001_CardProjectionHotPathP99(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        1_000_000_000, // effectively unbounded so every op projects a card
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, ack, nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	reg := NewNudgeRegistry(6 * time.Hour)

	const iters = 20000
	// Warm up.
	for i := 0; i < 2000; i++ {
		cand := hotCandidate(i)
		dec, _ := ctrl.Propose(ctx, cand)
		ref := reg.Mint(cand.ContentKey, cand.Producer, cand.Channel, "u")
		_, _ = ProjectCard(dec, cand, ref, "t")
	}

	samples := make([]time.Duration, 0, iters)
	for i := 0; i < iters; i++ {
		cand := hotCandidate(i + 1_000_000)
		start := time.Now()
		dec, perr := ctrl.Propose(ctx, cand)
		ref := reg.Mint(cand.ContentKey, cand.Producer, cand.Channel, "u")
		card, ok := ProjectCard(dec, cand, ref, "Renewal due")
		elapsed := time.Since(start)
		if perr != nil || !ok || !card.State.IsCard() {
			t.Fatalf("hot-path op %d did not project a card: err=%v ok=%t state=%q", i, perr, ok, card.State)
		}
		samples = append(samples, elapsed)
	}

	sort.Slice(samples, func(a, b int) bool { return samples[a] < samples[b] })
	p99 := samples[int(float64(len(samples))*0.99)]
	if p99 > 5*time.Millisecond {
		t.Fatalf("card-projection hot path p99 = %s, want <= 5ms (NFR-107-001)", p99)
	}
	t.Logf("card-projection hot-path p99 = %s over %d ops (ceiling 5ms)", p99, iters)
}

func hotCandidate(i int) surfacing.SurfacingCandidate {
	return surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    surfacing.ChannelTelegram,
		ContentKey: "hot-" + itoa(i),
		Priority:   2,
	}
}

// itoa avoids an fmt import in the hot loop.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

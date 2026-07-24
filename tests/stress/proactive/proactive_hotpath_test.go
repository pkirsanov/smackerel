//go:build stress

// Spec 107 SCOPE-01 — NFR-107-001 stress: the proactive card-projection hot path
// (controller Propose -> NudgeRef Mint -> ProjectCard) preserves the controller
// <5ms p99 budget under a sustained burst. Pure in-memory composition over the
// real controller; it adds no I/O to the Propose hot path. The same assertion
// runs in the fast unit lane (internal/proactive/hotpath_test.go) so the budget
// is guarded on every commit; this is the live-lane profile.
package proactive_stress

import (
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

func TestSCN107Hotpath_CardProjectionP99Live(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        1_000_000_000,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, ack, nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	reg := proactive.NewNudgeRegistry(6 * time.Hour)

	const iters = 50000
	for i := 0; i < 5000; i++ {
		cand := stressCandidate(i)
		dec, _ := ctrl.Propose(ctx, cand)
		ref := reg.Mint(cand.ContentKey, cand.Producer, cand.Channel, "u")
		_, _ = proactive.ProjectCard(dec, cand, ref, "t")
	}

	samples := make([]time.Duration, 0, iters)
	for i := 0; i < iters; i++ {
		cand := stressCandidate(i + 2_000_000)
		start := time.Now()
		dec, perr := ctrl.Propose(ctx, cand)
		ref := reg.Mint(cand.ContentKey, cand.Producer, cand.Channel, "u")
		card, ok := proactive.ProjectCard(dec, cand, ref, "Renewal due")
		elapsed := time.Since(start)
		if perr != nil || !ok || !card.State.IsCard() {
			t.Fatalf("op %d did not project a card: err=%v ok=%t", i, perr, ok)
		}
		samples = append(samples, elapsed)
	}

	sort.Slice(samples, func(a, b int) bool { return samples[a] < samples[b] })
	p99 := samples[int(float64(len(samples))*0.99)]
	if p99 > 5*time.Millisecond {
		t.Fatalf("card-projection hot-path p99 = %s under burst, want <= 5ms (NFR-107-001)", p99)
	}
	t.Logf("card-projection hot-path p99 = %s over %d ops (ceiling 5ms)", p99, iters)
}

func stressCandidate(i int) surfacing.SurfacingCandidate {
	return surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    surfacing.ChannelTelegram,
		ContentKey: "stress-" + strconv.Itoa(i),
		Priority:   2,
	}
}

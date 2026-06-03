package surfacing

import (
	"context"
	"fmt"
	"testing"
)

// Benchmarks for GAP-078-G02: empirically measure Controller.Propose hot-path
// latency to validate the NFR p99 < 5ms claim from spec 078. Bench mean is
// not a true p99, but if mean is orders of magnitude below 5ms the NFR is
// safely satisfied on this hardware class; prod Prometheus histogram remains
// the source of truth for SLO observation.

func benchConfig() Config {
	return Config{
		DailyNudgeBudget:        1_000_000_000,
		SuppressionWindowHours:  1,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}
}

// BenchmarkProposeHotPath exercises a realistic mixed workload: varied
// producers, varied channels, unique content keys (no dedupe collapse),
// budget always available. Models the steady-state permit path.
func BenchmarkProposeHotPath(b *testing.B) {
	c, err := NewController(benchConfig(), nil, noopMetrics{})
	if err != nil {
		b.Fatalf("NewController: %v", err)
	}
	producers := []Producer{
		ProducerAlerts,
		ProducerDigest,
		ProducerResurfacing,
		ProducerWeeklySynthesis,
		ProducerPreMeetingBriefs,
	}
	channels := []Channel{
		ChannelTelegram,
		ChannelWebPush,
		ChannelNtfy,
		ChannelEmailOut,
		ChannelDigest,
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cand := SurfacingCandidate{
			Producer:   producers[i%len(producers)],
			Channel:    channels[i%len(channels)],
			ContentKey: fmt.Sprintf("hot-%d", i),
			Priority:   1 + (i % 3),
		}
		if _, err := c.Propose(ctx, cand); err != nil {
			b.Fatalf("Propose: %v", err)
		}
	}
}

// BenchmarkProposeDeduped hits the dedupe fast-path: the same ContentKey
// is proposed repeatedly, so every call after the first short-circuits
// at the dedupe check.
func BenchmarkProposeDeduped(b *testing.B) {
	c, err := NewController(benchConfig(), nil, noopMetrics{})
	if err != nil {
		b.Fatalf("NewController: %v", err)
	}
	ctx := context.Background()
	cand := SurfacingCandidate{
		Producer:   ProducerAlerts,
		Channel:    ChannelTelegram,
		ContentKey: "dedupe-key",
		Priority:   2,
	}
	if _, err := c.Propose(ctx, cand); err != nil {
		b.Fatalf("seed Propose: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Propose(ctx, cand); err != nil {
			b.Fatalf("Propose: %v", err)
		}
	}
}

// BenchmarkProposeBudgetExhausted measures the post-budget defer path:
// budget=1 is consumed by a seed call, then every benchmark iteration
// is a non-urgent candidate with a unique key, hitting the defer branch.
func BenchmarkProposeBudgetExhausted(b *testing.B) {
	cfg := benchConfig()
	cfg.DailyNudgeBudget = 1
	c, err := NewController(cfg, nil, noopMetrics{})
	if err != nil {
		b.Fatalf("NewController: %v", err)
	}
	ctx := context.Background()
	if _, err := c.Propose(ctx, SurfacingCandidate{
		Producer:   ProducerAlerts,
		Channel:    ChannelTelegram,
		ContentKey: "seed",
		Priority:   2,
	}); err != nil {
		b.Fatalf("seed Propose: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cand := SurfacingCandidate{
			Producer:   ProducerDigest,
			Channel:    ChannelWebPush,
			ContentKey: fmt.Sprintf("defer-%d", i),
			Priority:   2,
		}
		if _, err := c.Propose(ctx, cand); err != nil {
			b.Fatalf("Propose: %v", err)
		}
	}
}

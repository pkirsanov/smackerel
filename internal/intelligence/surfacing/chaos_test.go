//go:build chaos

package surfacing

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// chaosMetrics is an atomic-counter MetricsSink suitable for high-concurrency
// chaos runs — the production noop sink would hide truthfulness bugs.
type chaosMetrics struct {
	delivered, deduped, suppressed, overrides, deferred int64
}

func (m *chaosMetrics) IncDelivered(Producer, Channel)      { atomic.AddInt64(&m.delivered, 1) }
func (m *chaosMetrics) IncDeduped(Producer)                 { atomic.AddInt64(&m.deduped, 1) }
func (m *chaosMetrics) IncSuppressed(string)                { atomic.AddInt64(&m.suppressed, 1) }
func (m *chaosMetrics) IncBudgetOverride(string)            { atomic.AddInt64(&m.overrides, 1) }
func (m *chaosMetrics) IncDeferredBudgetExhausted(Producer) { atomic.AddInt64(&m.deferred, 1) }
func (m *chaosMetrics) SetBudgetRemaining(int)              {}

var (
	producers = []Producer{ProducerAlerts, ProducerDigest, ProducerResurfacing, ProducerWeeklySynthesis, ProducerMonthlyReport, ProducerPreMeetingBriefs, ProducerFrequentLookups}
	channels  = []Channel{ChannelTelegram, ChannelWebPush, ChannelNtfy, ChannelEmailOut, ChannelDigest}
)

// TestChaos_ConcurrentPropose hammers Propose() from many goroutines with
// mixed producers/channels/content-keys and verifies decision-count
// conservation and absence of races (run with -race).
func TestChaos_ConcurrentPropose(t *testing.T) {
	const (
		goroutines  = 128
		perG        = 50
		budget      = 200
		distinctKey = 64
	)
	cfg := Config{DailyNudgeBudget: budget, SuppressionWindowHours: 12, DedupeWindowHours: 24, UrgentEscalationEnabled: true}
	m := &chaosMetrics{}
	ack := NewInMemoryAck()
	c, err := NewController(cfg, ack, m)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(seed int64) {
			defer wg.Done()
			r := rand.New(rand.NewSource(seed))
			for i := 0; i < perG; i++ {
				cand := SurfacingCandidate{
					Producer:     producers[r.Intn(len(producers))],
					Channel:      channels[r.Intn(len(channels))],
					ContentKey:   fmt.Sprintf("ck-%d", r.Intn(distinctKey)),
					Priority:     1 + r.Intn(3),
					TimeCritical: r.Intn(4) == 0,
					ProposedAt:   time.Now(),
				}
				if _, err := c.Propose(context.Background(), cand); err != nil {
					t.Errorf("Propose: %v", err)
					return
				}
			}
		}(int64(g) + 1)
	}
	wg.Wait()
	dur := time.Since(start)

	total := int64(goroutines * perG)
	sum := atomic.LoadInt64(&m.delivered) + atomic.LoadInt64(&m.deduped) +
		atomic.LoadInt64(&m.suppressed) + atomic.LoadInt64(&m.overrides) +
		atomic.LoadInt64(&m.deferred)
	// Note: IncDelivered fires for both permit and escalation paths, so
	// overrides is a SUBSET of delivered. Conservation = delivered + deduped + suppressed + deferred.
	conserved := atomic.LoadInt64(&m.delivered) + atomic.LoadInt64(&m.deduped) +
		atomic.LoadInt64(&m.suppressed) + atomic.LoadInt64(&m.deferred)
	t.Logf("chaos/concurrent: proposals=%d delivered=%d deduped=%d suppressed=%d overrides=%d deferred=%d sum_incl_overrides=%d conserved=%d duration=%s",
		total, m.delivered, m.deduped, m.suppressed, m.overrides, m.deferred, sum, conserved, dur)
	if conserved != total {
		t.Fatalf("decision conservation violated: want %d got %d", total, conserved)
	}
	if m.delivered > int64(budget)+m.overrides {
		t.Fatalf("budget violated: delivered=%d budget=%d overrides=%d", m.delivered, budget, m.overrides)
	}
}

// TestChaos_BudgetExhaustion feeds non-urgent unique-key candidates past
// the budget and verifies deferred == total - budget.
func TestChaos_BudgetExhaustion(t *testing.T) {
	const budget = 50
	const total = 500
	cfg := Config{DailyNudgeBudget: budget, SuppressionWindowHours: 12, DedupeWindowHours: 24, UrgentEscalationEnabled: true}
	m := &chaosMetrics{}
	c, err := NewController(cfg, NewInMemoryAck(), m)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	r := rand.New(rand.NewSource(42))
	for i := 0; i < total; i++ {
		cand := SurfacingCandidate{
			Producer:     producers[r.Intn(len(producers))],
			Channel:      channels[r.Intn(len(channels))],
			ContentKey:   fmt.Sprintf("ke-%d", i), // unique => no dedupe
			Priority:     2 + r.Intn(2),           // never P1 => no escalation
			TimeCritical: false,
			ProposedAt:   time.Now(),
		}
		if _, err := c.Propose(context.Background(), cand); err != nil {
			t.Fatalf("Propose: %v", err)
		}
	}
	t.Logf("chaos/budget: total=%d budget=%d delivered=%d deferred=%d overrides=%d",
		total, budget, m.delivered, m.deferred, m.overrides)
	if m.delivered != int64(budget) {
		t.Fatalf("delivered want %d got %d", budget, m.delivered)
	}
	if m.deferred != int64(total-budget) {
		t.Fatalf("deferred want %d got %d", total-budget, m.deferred)
	}
	if m.overrides != 0 {
		t.Fatalf("overrides want 0 got %d", m.overrides)
	}
}

// TestChaos_DedupeWindowTiming inserts and re-inserts the same content-key
// at random intervals across the window boundary and verifies dedupe fires.
func TestChaos_DedupeWindowTiming(t *testing.T) {
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	clock := now
	d := NewDedupeIndex(1) // 1-hour window
	d.clock = func() time.Time { return clock }

	const key = "dedupe-key"
	d.Record(key)
	if !d.IsDuplicate(key) {
		t.Fatalf("expected duplicate immediately after Record")
	}
	// Within window
	clock = now.Add(30 * time.Minute)
	if !d.IsDuplicate(key) {
		t.Fatalf("expected duplicate at 30m (window=1h)")
	}
	// Outside window
	clock = now.Add(2 * time.Hour)
	if d.IsDuplicate(key) {
		t.Fatalf("expected NOT duplicate at 2h (window=1h)")
	}
	t.Logf("chaos/dedupe: window=1h boundary respected at 30m (dup) and 2h (not dup)")
}

// TestChaos_SuppressionAfterAck acks random keys then re-proposes them
// and verifies suppression fires.
func TestChaos_SuppressionAfterAck(t *testing.T) {
	cfg := Config{DailyNudgeBudget: 1000, SuppressionWindowHours: 12, DedupeWindowHours: 24, UrgentEscalationEnabled: false}
	m := &chaosMetrics{}
	ack := NewInMemoryAck()
	c, err := NewController(cfg, ack, m)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	const n = 100
	r := rand.New(rand.NewSource(7))
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = fmt.Sprintf("ack-%d", i)
		ack.Acknowledge(keys[i])
	}
	for i := 0; i < n; i++ {
		dec, err := c.Propose(context.Background(), SurfacingCandidate{
			Producer:   producers[r.Intn(len(producers))],
			Channel:    channels[r.Intn(len(channels))],
			ContentKey: keys[i],
		})
		if err != nil {
			t.Fatalf("Propose: %v", err)
		}
		if dec.Kind != DecisionSuppressed {
			t.Fatalf("expected suppressed for acked key %s, got %s", keys[i], dec.Kind)
		}
	}
	t.Logf("chaos/suppression: n=%d all acked keys suppressed; counter=%d", n, m.suppressed)
}

// TestChaos_OpportunisticGC pushes the DedupeIndex and InMemoryAck past
// the 4096 threshold with stale entries and verifies GC bounds memory.
func TestChaos_OpportunisticGC(t *testing.T) {
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	clock := now
	d := NewDedupeIndex(1) // window=1h, GC cutoff=2h
	d.clock = func() time.Time { return clock }
	// Insert 4096 stale entries
	for i := 0; i < 4096; i++ {
		d.entries[fmt.Sprintf("stale-%d", i)] = now.Add(-10 * time.Hour)
	}
	// Advance clock; trigger GC by Record-ing one more (>4096 entries)
	clock = now.Add(5 * time.Hour)
	d.Record("fresh-trigger")
	if got := len(d.entries); got > 1 {
		// Stale entries (recorded at now-10h, cutoff = (now+5h)-2h = now+3h)
		// all stale entries are before cutoff => should be GC'd.
		t.Fatalf("dedupe GC failed: entries=%d (expected ~1 fresh)", got)
	}
	t.Logf("chaos/gc: dedupe size after GC=%d", len(d.entries))

	a := NewInMemoryAck()
	a.clock = func() time.Time { return clock }
	for i := 0; i < 4096; i++ {
		a.entries[fmt.Sprintf("ack-stale-%d", i)] = now.Add(-60 * 24 * time.Hour)
	}
	a.Acknowledge("fresh-ack")
	if got := len(a.entries); got > 1 {
		t.Fatalf("ack GC failed: entries=%d", got)
	}
	t.Logf("chaos/gc: ack size after GC=%d", len(a.entries))
}

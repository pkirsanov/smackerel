package surfacing

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeMetrics captures the controller's metric emissions so unit tests
// can assert on the bounded counters that Scope 4 requires.
type fakeMetrics struct {
	mu                sync.Mutex
	delivered         map[string]int
	deduped           map[Producer]int
	suppressed        map[string]int
	budgetOverrides   map[string]int
	deferredExhausted map[Producer]int
	budgetRemaining   int
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{
		delivered:         map[string]int{},
		deduped:           map[Producer]int{},
		suppressed:        map[string]int{},
		budgetOverrides:   map[string]int{},
		deferredExhausted: map[Producer]int{},
	}
}

func (f *fakeMetrics) IncDelivered(p Producer, c Channel) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered[string(p)+"|"+string(c)]++
}
func (f *fakeMetrics) IncDeduped(p Producer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deduped[p]++
}
func (f *fakeMetrics) IncSuppressed(reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.suppressed[reason]++
}
func (f *fakeMetrics) IncBudgetOverride(reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.budgetOverrides[reason]++
}
func (f *fakeMetrics) IncDeferredBudgetExhausted(p Producer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deferredExhausted[p]++
}
func (f *fakeMetrics) SetBudgetRemaining(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.budgetRemaining = n
}

func newTestController(t *testing.T, cfg Config, ack AckLookup) (*Controller, *fakeMetrics) {
	t.Helper()
	m := newFakeMetrics()
	c, err := NewController(cfg, ack, m)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	return c, m
}

// SCN-021-016 — Daily nudge budget enforced across all surfaces.
func TestController_BudgetExhaustionDefersNonUrgentCandidates(t *testing.T) {
	c, m := newTestController(t, Config{
		DailyNudgeBudget:        5,
		SuppressionWindowHours:  1,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, nil)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		d, err := c.Propose(ctx, SurfacingCandidate{
			Producer:   ProducerAlerts,
			Channel:    ChannelTelegram,
			ContentKey: "key-permit-" + string(rune('a'+i)),
			Priority:   2,
		})
		if err != nil || d.Kind != DecisionPermit {
			t.Fatalf("candidate %d: want Permit, got %+v err=%v", i, d, err)
		}
	}

	d, err := c.Propose(ctx, SurfacingCandidate{
		Producer:   ProducerDigest,
		Channel:    ChannelTelegram,
		ContentKey: "key-deferred",
		Priority:   2,
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if d.Kind != DecisionDeferredBudgetExhausted {
		t.Fatalf("want deferred-budget-exhausted; got %s (%s)", d.Kind, d.Reason)
	}
	if m.budgetRemaining != 0 {
		t.Fatalf("budgetRemaining: want 0, got %d", m.budgetRemaining)
	}
	if m.deferredExhausted[ProducerDigest] != 1 {
		t.Fatalf("deferredExhausted[digest]: want 1, got %d", m.deferredExhausted[ProducerDigest])
	}
}

// SCN-021-017 — Duplicate content deduped across surfaces.
func TestController_DuplicateContentKeyDedupedAcrossChannels(t *testing.T) {
	c, m := newTestController(t, Config{
		DailyNudgeBudget:        10,
		SuppressionWindowHours:  1,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, nil)

	ctx := context.Background()
	d, err := c.Propose(ctx, SurfacingCandidate{
		Producer:   ProducerAlerts,
		Channel:    ChannelTelegram,
		ContentKey: "artifact-789",
		Priority:   2,
	})
	if err != nil || d.Kind != DecisionPermit {
		t.Fatalf("first: want Permit, got %+v err=%v", d, err)
	}

	d, err = c.Propose(ctx, SurfacingCandidate{
		Producer:   ProducerDigest,
		Channel:    ChannelDigest,
		ContentKey: "artifact-789",
		Priority:   3,
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if d.Kind != DecisionDeduped {
		t.Fatalf("want deduped; got %s (%s)", d.Kind, d.Reason)
	}
	if m.deduped[ProducerDigest] != 1 {
		t.Fatalf("deduped[digest]: want 1, got %d", m.deduped[ProducerDigest])
	}
}

// SCN-021-018 — Acknowledged item suppresses follow-up nudges.
func TestController_AcknowledgedContentSuppressesFollowups(t *testing.T) {
	ack := NewInMemoryAck()
	ack.Acknowledge("insight-42")

	c, m := newTestController(t, Config{
		DailyNudgeBudget:        10,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, ack)

	d, err := c.Propose(context.Background(), SurfacingCandidate{
		Producer:   ProducerAlerts,
		Channel:    ChannelTelegram,
		ContentKey: "insight-42",
		Priority:   2,
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if d.Kind != DecisionSuppressed {
		t.Fatalf("want suppressed; got %s (%s)", d.Kind, d.Reason)
	}
	if d.Reason != "acknowledged-by-user" {
		t.Fatalf("reason: want acknowledged-by-user, got %s", d.Reason)
	}
	if m.suppressed["acknowledged-by-user"] != 1 {
		t.Fatalf("suppressed counter not incremented: %+v", m.suppressed)
	}
}

// SCN-021-019 — Urgent event escalates past exhausted budget.
func TestController_UrgentEscalationBypassesExhaustedBudget(t *testing.T) {
	c, m := newTestController(t, Config{
		DailyNudgeBudget:        1,
		SuppressionWindowHours:  1,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, nil)

	ctx := context.Background()
	if d, err := c.Propose(ctx, SurfacingCandidate{
		Producer:   ProducerDigest,
		Channel:    ChannelTelegram,
		ContentKey: "warmup",
		Priority:   3,
	}); err != nil || d.Kind != DecisionPermit {
		t.Fatalf("warmup: want Permit, got %+v err=%v", d, err)
	}

	d, err := c.Propose(ctx, SurfacingCandidate{
		Producer:     ProducerAlerts,
		Channel:      ChannelTelegram,
		ContentKey:   "urgent-bill",
		Priority:     1,
		TimeCritical: true,
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if d.Kind != DecisionEscalated || d.Reason != "urgent_escalation" {
		t.Fatalf("want escalated/urgent_escalation; got %s (%s)", d.Kind, d.Reason)
	}
	if m.budgetOverrides["urgent_escalation"] != 1 {
		t.Fatalf("budget_overrides_total{reason=urgent_escalation}: want 1, got %d",
			m.budgetOverrides["urgent_escalation"])
	}
}

// Escalation MUST be disabled when SST flag is off.
func TestController_UrgentEscalationDisabledHoldsCandidate(t *testing.T) {
	c, _ := newTestController(t, Config{
		DailyNudgeBudget:        1,
		SuppressionWindowHours:  1,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: false,
	}, nil)

	ctx := context.Background()
	_, _ = c.Propose(ctx, SurfacingCandidate{
		Producer:   ProducerDigest,
		Channel:    ChannelTelegram,
		ContentKey: "warmup",
		Priority:   3,
	})
	d, err := c.Propose(ctx, SurfacingCandidate{
		Producer:     ProducerAlerts,
		Channel:      ChannelTelegram,
		ContentKey:   "urgent-but-flag-off",
		Priority:     1,
		TimeCritical: true,
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if d.Kind != DecisionDeferredBudgetExhausted {
		t.Fatalf("want deferred-budget-exhausted; got %s (%s)", d.Kind, d.Reason)
	}
}

func TestNewController_FailsLoudOnZeroSST(t *testing.T) {
	cases := []Config{
		{DailyNudgeBudget: 0, SuppressionWindowHours: 1, DedupeWindowHours: 1},
		{DailyNudgeBudget: 1, SuppressionWindowHours: 0, DedupeWindowHours: 1},
		{DailyNudgeBudget: 1, SuppressionWindowHours: 1, DedupeWindowHours: 0},
	}
	for i, cfg := range cases {
		if _, err := NewController(cfg, nil, nil); err == nil {
			t.Fatalf("case %d: want error, got nil for %+v", i, cfg)
		}
	}
}

func TestDedupeIndex_WindowExpiry(t *testing.T) {
	d := NewDedupeIndex(1)
	now := time.Now()
	d.clock = func() time.Time { return now }
	d.Record("k")
	if !d.IsDuplicate("k") {
		t.Fatalf("expected duplicate immediately after record")
	}
	d.clock = func() time.Time { return now.Add(2 * time.Hour) }
	if d.IsDuplicate("k") {
		t.Fatalf("expected expiry after window")
	}
}

func TestBudgetTracker_DailyRollover(t *testing.T) {
	b := NewBudgetTracker(1)
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	b.clock = func() time.Time { return now }
	if !b.TryConsume() {
		t.Fatalf("first consume must succeed")
	}
	if b.TryConsume() {
		t.Fatalf("second consume same day must fail")
	}
	b.clock = func() time.Time { return now.Add(24 * time.Hour) }
	if !b.TryConsume() {
		t.Fatalf("rollover must reset budget")
	}
}

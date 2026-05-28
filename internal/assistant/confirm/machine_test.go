// Spec 061 SCOPE-08 — Machine unit tests.
//
// Exercises Propose / Confirm / Discard / SweepTimeouts against an
// in-memory store + in-memory audit writer. No PG. The PG-backed
// integration test lives in machine_pg_integration_test.go (build
// tag `integration`).

package confirm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
)

// memStore is a minimal in-memory assistantctx.Store for unit tests.
type memStore struct {
	mu   sync.Mutex
	rows map[string]assistantctx.Conversation
}

func newMemStore() *memStore {
	return &memStore{rows: map[string]assistantctx.Conversation{}}
}

func (m *memStore) key(uid, tr string) string { return uid + "|" + tr }

func (m *memStore) Load(_ context.Context, userID, transport string) (assistantctx.Conversation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.rows[m.key(userID, transport)]
	if !ok {
		return assistantctx.Conversation{UserID: userID, Transport: transport}, false, nil
	}
	return c, true, nil
}

func (m *memStore) Persist(_ context.Context, conv assistantctx.Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[m.key(conv.UserID, conv.Transport)] = conv
	return nil
}

func (m *memStore) DeleteByKey(_ context.Context, userID, transport string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, m.key(userID, transport))
	return nil
}

func (m *memStore) SweepIdle(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func (m *memStore) CountActiveByTransport(_ context.Context) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	counts := map[string]int{}
	for _, conv := range m.rows {
		counts[conv.Transport]++
	}
	return counts, nil
}

// memWriter captures audit rows for assertion.
type memWriter struct {
	mu   sync.Mutex
	rows []ProposalArtifact
}

func (w *memWriter) WriteProposalArtifact(_ context.Context, p ProposalArtifact) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rows = append(w.rows, p)
	return nil
}

func (w *memWriter) snapshot() []ProposalArtifact {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]ProposalArtifact, len(w.rows))
	copy(out, w.rows)
	return out
}

func newFixture(t *testing.T) (*Machine, *memStore, *memWriter) {
	t.Helper()
	store := newMemStore()
	writer := &memWriter{}
	return NewMachine(store, writer), store, writer
}

func freshProposal() ProposalInput {
	return ProposalInput{
		UserID:         "u1",
		Transport:      "telegram",
		ScenarioID:     "notification_schedule",
		ConfirmRef:     "cr-1",
		ProposedAction: "Remind you to call mom at 6pm",
		Payload:        []byte(`{"what":"call mom","when_utc":"2026-01-01T18:00:00Z","user_id":"u1","transport":"telegram"}`),
		ExpiresAt:      time.Date(2026, 1, 1, 17, 50, 0, 0, time.UTC),
	}
}

func TestMachine_Propose_PersistsPending(t *testing.T) {
	m, store, _ := newFixture(t)
	in := freshProposal()
	now := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(context.Background(), in, now); err != nil {
		t.Fatalf("Propose: unexpected error: %v", err)
	}
	conv, ok, err := store.Load(context.Background(), in.UserID, in.Transport)
	if err != nil || !ok {
		t.Fatalf("Load after Propose: ok=%v err=%v", ok, err)
	}
	if conv.PendingConfirm == nil {
		t.Fatal("PendingConfirm not persisted")
	}
	if conv.PendingConfirm.ConfirmRef != in.ConfirmRef {
		t.Errorf("ConfirmRef: got %q want %q", conv.PendingConfirm.ConfirmRef, in.ConfirmRef)
	}
	if !conv.LastActivityAt.Equal(now) {
		t.Errorf("LastActivityAt: got %v want %v", conv.LastActivityAt, now)
	}
	if string(conv.PendingConfirm.Payload) != string(in.Payload) {
		t.Errorf("Payload: got %q want %q", conv.PendingConfirm.Payload, in.Payload)
	}
}

func TestMachine_Propose_Validation(t *testing.T) {
	m, _, _ := newFixture(t)
	now := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	cases := []struct {
		name  string
		mut   func(*ProposalInput)
		match string
	}{
		{"no user", func(p *ProposalInput) { p.UserID = "" }, "UserID required"},
		{"no transport", func(p *ProposalInput) { p.Transport = "" }, "Transport required"},
		{"no scenario", func(p *ProposalInput) { p.ScenarioID = "" }, "ScenarioID required"},
		{"no ref", func(p *ProposalInput) { p.ConfirmRef = "" }, "ConfirmRef required"},
		{"no action", func(p *ProposalInput) { p.ProposedAction = "" }, "ProposedAction required"},
		{"no payload", func(p *ProposalInput) { p.Payload = nil }, "Payload required"},
		{"no expiry", func(p *ProposalInput) { p.ExpiresAt = time.Time{} }, "ExpiresAt required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			p := freshProposal()
			tc.mut(&p)
			err := m.Propose(context.Background(), p, now)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tc.match) {
				t.Errorf("error %q does not contain %q", got, tc.match)
			}
		})
	}
}

func TestMachine_Confirm_HappyPath_WritesAuditAndClearsPending(t *testing.T) {
	m, store, writer := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now0 := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	now1 := now0.Add(30 * time.Second)
	res, err := m.Confirm(ctx, ConfirmInput{
		UserID: in.UserID, Transport: in.Transport,
		ConfirmRef: in.ConfirmRef, ScheduledJobID: "job-42",
	}, now1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if string(res.Payload) != string(in.Payload) {
		t.Errorf("Confirm.Payload: got %q want %q", res.Payload, in.Payload)
	}
	conv, _, _ := store.Load(ctx, in.UserID, in.Transport)
	if conv.PendingConfirm != nil {
		t.Errorf("PendingConfirm should be cleared, got %+v", conv.PendingConfirm)
	}
	if !conv.LastActivityAt.Equal(now1) {
		t.Errorf("LastActivityAt: got %v want %v", conv.LastActivityAt, now1)
	}
	rows := writer.snapshot()
	if len(rows) != 1 {
		t.Fatalf("audit rows: got %d want 1", len(rows))
	}
	got := rows[0]
	if got.Outcome != OutcomeConfirmed {
		t.Errorf("Outcome: got %q want %q", got.Outcome, OutcomeConfirmed)
	}
	if got.ScheduledJobID != "job-42" {
		t.Errorf("ScheduledJobID: got %q want %q", got.ScheduledJobID, "job-42")
	}
	if got.ConfirmRef != in.ConfirmRef {
		t.Errorf("ConfirmRef: got %q want %q", got.ConfirmRef, in.ConfirmRef)
	}
}

func TestMachine_Confirm_SingleFlight_SecondCallReturnsNotFound(t *testing.T) {
	m, _, writer := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	ci := ConfirmInput{UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef, ScheduledJobID: "job-1"}
	if _, err := m.Confirm(ctx, ci, now.Add(time.Second)); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	_, err := m.Confirm(ctx, ci, now.Add(2*time.Second))
	if !errors.Is(err, ErrPendingNotFound) {
		t.Errorf("second Confirm: got %v want ErrPendingNotFound", err)
	}
	if len(writer.snapshot()) != 1 {
		t.Errorf("audit rows: got %d want 1 (second call MUST NOT write a duplicate audit row)", len(writer.snapshot()))
	}
}

func TestMachine_Confirm_WrongRef_ReturnsNotFound(t *testing.T) {
	m, _, _ := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	_, err := m.Confirm(ctx, ConfirmInput{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: "different-ref",
	}, now.Add(time.Second))
	if !errors.Is(err, ErrPendingNotFound) {
		t.Errorf("got %v want ErrPendingNotFound", err)
	}
}

func TestMachine_Discard_WritesDiscardedUserAudit(t *testing.T) {
	m, store, writer := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now0 := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	now1 := now0.Add(45 * time.Second)
	err := m.Discard(ctx, DiscardInput{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef,
	}, now1)
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	conv, _, _ := store.Load(ctx, in.UserID, in.Transport)
	if conv.PendingConfirm != nil {
		t.Error("PendingConfirm should be cleared")
	}
	rows := writer.snapshot()
	if len(rows) != 1 || rows[0].Outcome != OutcomeDiscardedUser {
		t.Fatalf("audit: got %+v want one discarded_user row", rows)
	}
}

func TestMachine_SweepTimeouts_WritesTimeoutAudit(t *testing.T) {
	m, _, writer := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now0 := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	now1 := in.ExpiresAt.Add(time.Minute)
	res, err := m.SweepTimeouts(ctx, []ExpiredPending{{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef,
	}}, now1)
	if err != nil {
		t.Fatalf("SweepTimeouts: %v", err)
	}
	if res.Expired != 1 {
		t.Errorf("Expired count: got %d want 1", res.Expired)
	}
	rows := writer.snapshot()
	if len(rows) != 1 || rows[0].Outcome != OutcomeDiscardedTimeout {
		t.Fatalf("audit: got %+v want one discarded_timeout row", rows)
	}
}

func TestMachine_SweepTimeouts_SkipsRacedConfirms(t *testing.T) {
	m, _, writer := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now0 := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	// User confirms BEFORE the sweep observes the row.
	if _, err := m.Confirm(ctx, ConfirmInput{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef, ScheduledJobID: "job-7",
	}, now0.Add(time.Second)); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	// Now the sweep fires with a stale view.
	res, err := m.SweepTimeouts(ctx, []ExpiredPending{{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef,
	}}, in.ExpiresAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("SweepTimeouts: %v", err)
	}
	if res.Expired != 0 {
		t.Errorf("Expired count: got %d want 0 (raced confirm wins)", res.Expired)
	}
	rows := writer.snapshot()
	if len(rows) != 1 || rows[0].Outcome != OutcomeConfirmed {
		t.Fatalf("audit: got %+v want one confirmed row only (no duplicate timeout)", rows)
	}
}

// contains is a tiny strings.Contains shim that doesn't pull the
// stdlib `strings` package into this test file's import block; keeps
// the file's intent narrowly focused on test logic.
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// ---- Spec 061 SCOPE-09 — metric emission tests ---------------------
//
// Pin the Prometheus emissions added by SCOPE-09 to the confirm
// machine. The tests use the package-private metric vectors as the
// observation surface (read via dto.Metric) so a regression that
// removes an emission OR mutates the label vocabulary fails loudly.

func metricCounterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("counter.Write: %v", err)
	}
	if m.Counter == nil {
		t.Fatalf("counter.Write produced nil dto.Counter")
	}
	return m.Counter.GetValue()
}

func TestMachine_Confirm_EmitsConfirmedMetric(t *testing.T) {
	m, _, _ := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now0 := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	counter, err := assistantmetrics.ConfirmCardOutcomesTotal.GetMetricWithLabelValues(
		in.ScenarioID,
		assistantmetrics.ConfirmOutcomeConfirmed,
		assistantmetrics.TransportTelegram,
	)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	before := metricCounterValue(t, counter)

	if _, err := m.Confirm(ctx, ConfirmInput{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef, ScheduledJobID: "job-9",
	}, now0.Add(time.Second)); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	after := metricCounterValue(t, counter)
	if after-before != 1 {
		t.Errorf("ConfirmCardOutcomesTotal{confirmed} delta = %.0f, want 1", after-before)
	}
}

func TestMachine_Discard_EmitsDiscardedUserMetricAndCaptureFallback(t *testing.T) {
	m, _, _ := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now0 := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}

	confirmCounter, err := assistantmetrics.ConfirmCardOutcomesTotal.GetMetricWithLabelValues(
		in.ScenarioID,
		assistantmetrics.ConfirmOutcomeDiscardedUser,
		assistantmetrics.TransportTelegram,
	)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues confirm: %v", err)
	}
	fallbackCounter, err := assistantmetrics.CaptureFallbackTotal.GetMetricWithLabelValues(
		assistantmetrics.CauseConfirmDiscarded,
		assistantmetrics.TransportTelegram,
	)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues fallback: %v", err)
	}
	confirmBefore := metricCounterValue(t, confirmCounter)
	fallbackBefore := metricCounterValue(t, fallbackCounter)

	if err := m.Discard(ctx, DiscardInput{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef,
	}, now0.Add(45*time.Second)); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	if got := metricCounterValue(t, confirmCounter) - confirmBefore; got != 1 {
		t.Errorf("ConfirmCardOutcomesTotal{discarded_user} delta = %.0f, want 1", got)
	}
	if got := metricCounterValue(t, fallbackCounter) - fallbackBefore; got != 1 {
		t.Errorf("CaptureFallbackTotal{confirm_discarded} delta = %.0f, want 1", got)
	}
}

func TestMachine_SweepTimeouts_EmitsDiscardedTimeoutMetric(t *testing.T) {
	m, _, _ := newFixture(t)
	ctx := context.Background()
	in := freshProposal()
	now0 := time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}

	confirmCounter, err := assistantmetrics.ConfirmCardOutcomesTotal.GetMetricWithLabelValues(
		in.ScenarioID,
		assistantmetrics.ConfirmOutcomeDiscardedTimeout,
		assistantmetrics.TransportTelegram,
	)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues confirm: %v", err)
	}
	fallbackCounter, err := assistantmetrics.CaptureFallbackTotal.GetMetricWithLabelValues(
		assistantmetrics.CauseConfirmTimeout,
		assistantmetrics.TransportTelegram,
	)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues fallback: %v", err)
	}
	confirmBefore := metricCounterValue(t, confirmCounter)
	fallbackBefore := metricCounterValue(t, fallbackCounter)

	if _, err := m.SweepTimeouts(ctx, []ExpiredPending{{
		UserID: in.UserID, Transport: in.Transport, ConfirmRef: in.ConfirmRef,
	}}, in.ExpiresAt.Add(time.Minute)); err != nil {
		t.Fatalf("SweepTimeouts: %v", err)
	}

	if got := metricCounterValue(t, confirmCounter) - confirmBefore; got != 1 {
		t.Errorf("ConfirmCardOutcomesTotal{discarded_timeout} delta = %.0f, want 1", got)
	}
	if got := metricCounterValue(t, fallbackCounter) - fallbackBefore; got != 1 {
		t.Errorf("CaptureFallbackTotal{confirm_timeout} delta = %.0f, want 1", got)
	}
}

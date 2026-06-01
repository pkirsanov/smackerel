//go:build integration

// Spec 068 Scope 3 — Confirmation contracts canary.
//
// The intent-compiler insertion at the facade introduces a new gate
// BEFORE the existing confirm.Machine. This canary proves the
// pre-existing confirm-card lifecycle (pending-state persistence and
// replay protection) still holds end-to-end: a Propose persists a
// pending row; a Confirm clears it and writes one audit row; a
// replay of the same ConfirmRef MUST NOT double-write.

package assistant_integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/confirm"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
)

type canaryStore struct {
	mu   sync.Mutex
	rows map[string]assistantctx.Conversation
}

func (m *canaryStore) key(uid, tr string) string { return uid + "|" + tr }

func (m *canaryStore) Load(_ context.Context, userID, transport string) (assistantctx.Conversation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.rows[m.key(userID, transport)]
	if !ok {
		return assistantctx.Conversation{UserID: userID, Transport: transport}, false, nil
	}
	return c, true, nil
}
func (m *canaryStore) Persist(_ context.Context, conv assistantctx.Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[m.key(conv.UserID, conv.Transport)] = conv
	return nil
}
func (m *canaryStore) DeleteByKey(_ context.Context, userID, transport string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, m.key(userID, transport))
	return nil
}
func (m *canaryStore) SweepIdle(context.Context, time.Duration) (int64, error) { return 0, nil }
func (m *canaryStore) CountActiveByTransport(context.Context) (map[string]int, error) {
	return map[string]int{}, nil
}

type canaryWriter struct {
	mu   sync.Mutex
	rows []confirm.ProposalArtifact
}

func (w *canaryWriter) WriteProposalArtifact(_ context.Context, p confirm.ProposalArtifact) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rows = append(w.rows, p)
	return nil
}

// TestConfirmationCanary_PendingStateAndReplayProtectionStillHold
// SCN-068-A03, SCN-068-A09 canary.
//
//  1. Propose persists PendingConfirm.
//  2. Confirm clears PendingConfirm and writes exactly one audit row.
//  3. A second Confirm with the same ConfirmRef returns an error and
//     does NOT write a second audit row (replay protection).
func TestConfirmationCanary_PendingStateAndReplayProtectionStillHold(t *testing.T) {
	store := &canaryStore{rows: map[string]assistantctx.Conversation{}}
	writer := &canaryWriter{}
	m := confirm.NewMachine(store, writer)
	ctx := context.Background()

	in := confirm.ProposalInput{
		UserID:         "u-canary",
		Transport:      "telegram",
		ScenarioID:     "notification_schedule",
		ConfirmRef:     "cr-canary-1",
		ProposedAction: "Remind you to confirm later",
		Payload:        []byte(`{"what":"x","when_utc":"2026-06-01T18:00:00Z"}`),
		ExpiresAt:      time.Date(2026, 6, 1, 17, 50, 0, 0, time.UTC),
	}
	now0 := time.Date(2026, 6, 1, 17, 40, 0, 0, time.UTC)
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	conv, ok, _ := store.Load(ctx, in.UserID, in.Transport)
	if !ok || conv.PendingConfirm == nil {
		t.Fatalf("after Propose: PendingConfirm not persisted (ok=%v, pending=%v)", ok, conv.PendingConfirm)
	}

	now1 := now0.Add(30 * time.Second)
	if _, err := m.Confirm(ctx, confirm.ConfirmInput{
		UserID: in.UserID, Transport: in.Transport,
		ConfirmRef: in.ConfirmRef, ScheduledJobID: "job-1",
	}, now1); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	conv, _, _ = store.Load(ctx, in.UserID, in.Transport)
	if conv.PendingConfirm != nil {
		t.Fatalf("after Confirm: PendingConfirm must be cleared; got %+v", conv.PendingConfirm)
	}
	writer.mu.Lock()
	first := len(writer.rows)
	writer.mu.Unlock()
	if first != 1 {
		t.Fatalf("audit rows after Confirm = %d, want 1", first)
	}

	// Replay protection: a second Confirm for the same ref MUST NOT
	// double-write. The machine clears pending BEFORE writing, so
	// the replay sees "no pending" and short-circuits.
	now2 := now1.Add(5 * time.Second)
	_, err := m.Confirm(ctx, confirm.ConfirmInput{
		UserID: in.UserID, Transport: in.Transport,
		ConfirmRef: in.ConfirmRef, ScheduledJobID: "job-1-replay",
	}, now2)
	if err == nil {
		t.Fatalf("replay Confirm: expected error, got nil (replay protection broken)")
	}
	writer.mu.Lock()
	after := len(writer.rows)
	writer.mu.Unlock()
	if after != first {
		t.Fatalf("audit rows after replay = %d, want %d (replay must not double-write)", after, first)
	}
}

//go:build integration

// Spec 061 SCOPE-08 — Machine integration test against live PostgreSQL.
//
// Run with:
//   DATABASE_URL=postgres://... go test -tags integration \
//       ./internal/assistant/confirm/ -run TestMachinePg -count=1 -v
//
// Covers two of the three terminal outcomes (confirmed + discarded_user)
// end-to-end: PgStore writes pending_confirm, PgWriter writes the
// assistant_proposal artifact row with the additive
// assistant_proposal_payload JSONB (migration 042).

package confirm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/db"
)

func newPgFixture(t *testing.T) (*Machine, *pgxpool.Pool, string) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	prefix := "confirm-int-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
	store := assistantctx.NewPgStore(pool)
	writer := NewPgWriter(pool)
	return NewMachine(store, writer), pool, prefix
}

func TestMachinePgConfirmRoundTripWritesAuditRow(t *testing.T) {
	m, pool, prefix := newPgFixture(t)
	ctx := context.Background()
	userID := prefix + "-u1"
	transport := "telegram"
	confirmRef := prefix + "-cr-1"
	scheduledJobID := prefix + "-job-1"

	now0 := time.Now().UTC().Round(time.Microsecond)
	in := ProposalInput{
		UserID:         userID,
		Transport:      transport,
		ScenarioID:     "notification_schedule",
		ConfirmRef:     confirmRef,
		ProposedAction: "Remind you to call mom at 6pm",
		Payload:        []byte(`{"what":"call mom","when_utc":"2026-01-01T18:00:00Z"}`),
		ExpiresAt:      now0.Add(10 * time.Minute),
	}
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}

	// Verify pending row persisted via assistantctx.
	store := assistantctx.NewPgStore(pool)
	conv, ok, err := store.Load(ctx, userID, transport)
	if err != nil || !ok {
		t.Fatalf("Load after Propose: ok=%v err=%v", ok, err)
	}
	if conv.PendingConfirm == nil || conv.PendingConfirm.ConfirmRef != confirmRef {
		t.Fatalf("PendingConfirm not persisted: %+v", conv.PendingConfirm)
	}

	now1 := now0.Add(30 * time.Second)
	res, err := m.Confirm(ctx, ConfirmInput{
		UserID: userID, Transport: transport, ConfirmRef: confirmRef, ScheduledJobID: scheduledJobID,
	}, now1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if string(res.Payload) != string(in.Payload) {
		t.Errorf("Confirm.Payload: got %q want %q", res.Payload, in.Payload)
	}

	// Pending row should be cleared.
	conv2, _, err := store.Load(ctx, userID, transport)
	if err != nil {
		t.Fatalf("Load after Confirm: %v", err)
	}
	if conv2.PendingConfirm != nil {
		t.Errorf("PendingConfirm should be cleared after Confirm, got %+v", conv2.PendingConfirm)
	}

	// Audit row should exist with payload + outcome.
	const q = `
SELECT title, source_id, assistant_proposal_payload
  FROM artifacts
 WHERE artifact_type = 'assistant_proposal'
   AND assistant_proposal_payload->>'confirm_ref' = $1
   AND assistant_proposal_payload->>'outcome' = $2
`
	var title, sourceID string
	var payloadRaw []byte
	row := pool.QueryRow(ctx, q, confirmRef, string(OutcomeConfirmed))
	if err := row.Scan(&title, &sourceID, &payloadRaw); err != nil {
		t.Fatalf("audit row query: %v", err)
	}
	if sourceID != SystemSourceID {
		t.Errorf("source_id: got %q want %q", sourceID, SystemSourceID)
	}
	if !strings.Contains(title, confirmRef) {
		t.Errorf("title: got %q does not contain confirm_ref %q", title, confirmRef)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["outcome"] != string(OutcomeConfirmed) {
		t.Errorf("payload.outcome: got %v want %q", payload["outcome"], OutcomeConfirmed)
	}
	if payload["scheduled_job_id"] != scheduledJobID {
		t.Errorf("payload.scheduled_job_id: got %v want %q", payload["scheduled_job_id"], scheduledJobID)
	}
	if payload["user_id"] != userID {
		t.Errorf("payload.user_id: got %v want %q", payload["user_id"], userID)
	}
}

func TestMachinePgDiscardWritesAuditRow(t *testing.T) {
	m, pool, prefix := newPgFixture(t)
	ctx := context.Background()
	userID := prefix + "-u2"
	transport := "telegram"
	confirmRef := prefix + "-cr-2"

	now0 := time.Now().UTC().Round(time.Microsecond)
	in := ProposalInput{
		UserID:         userID,
		Transport:      transport,
		ScenarioID:     "notification_schedule",
		ConfirmRef:     confirmRef,
		ProposedAction: "Remind you to walk dog at 8pm",
		Payload:        []byte(`{"what":"walk dog","when_utc":"2026-01-01T20:00:00Z"}`),
		ExpiresAt:      now0.Add(10 * time.Minute),
	}
	if err := m.Propose(ctx, in, now0); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if err := m.Discard(ctx, DiscardInput{
		UserID: userID, Transport: transport, ConfirmRef: confirmRef,
	}, now0.Add(time.Minute)); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	const q = `
SELECT assistant_proposal_payload->>'outcome'
  FROM artifacts
 WHERE artifact_type = 'assistant_proposal'
   AND assistant_proposal_payload->>'confirm_ref' = $1
`
	var outcome string
	if err := pool.QueryRow(ctx, q, confirmRef).Scan(&outcome); err != nil {
		t.Fatalf("audit row query: %v", err)
	}
	if outcome != string(OutcomeDiscardedUser) {
		t.Errorf("outcome: got %q want %q", outcome, OutcomeDiscardedUser)
	}

	// Second Discard MUST return ErrPendingNotFound and MUST NOT write a duplicate audit row.
	err := m.Discard(ctx, DiscardInput{
		UserID: userID, Transport: transport, ConfirmRef: confirmRef,
	}, now0.Add(2*time.Minute))
	if !errors.Is(err, ErrPendingNotFound) {
		t.Errorf("second Discard: got %v want ErrPendingNotFound", err)
	}

	const countQ = `
SELECT COUNT(*)
  FROM artifacts
 WHERE artifact_type = 'assistant_proposal'
   AND assistant_proposal_payload->>'confirm_ref' = $1
`
	var count int
	if err := pool.QueryRow(ctx, countQ, confirmRef).Scan(&count); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if count != 1 {
		t.Errorf("audit row count: got %d want 1 (single-flight invariant)", count)
	}
}

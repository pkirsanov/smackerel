//go:build integration

// Spec 061 SCOPE-04 — PgStore integration test against live PostgreSQL.
//
// Run with:
//   DATABASE_URL=postgres://... go test -tags integration \
//       ./internal/assistant/context/ -run TestPgStore -count=1 -v
//
// All conversations are namespaced with a per-test prefix
// (assistant-int-<RFC3339Nano>) so parallel test runs do not collide.

package assistantctx

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
)

func newIntegrationStore(t *testing.T) (*PgStore, *pgxpool.Pool) {
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
	return NewPgStore(pool), pool
}

func integrationPrefix(t *testing.T) string {
	t.Helper()
	return "assistant-int-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
}

func TestPgStoreLoadReturnsEmptyWhenRowAbsent(t *testing.T) {
	store, _ := newIntegrationStore(t)
	prefix := integrationPrefix(t)

	conv, found, err := store.Load(context.Background(), prefix+"-u1", "telegram")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if found {
		t.Fatalf("Load reported found=true for unknown (userID, transport)")
	}
	if conv.UserID != prefix+"-u1" || conv.Transport != "telegram" {
		t.Errorf("Load returned wrong primary-key on miss: %+v", conv)
	}
	if len(conv.WorkingContext.Turns) != 0 {
		t.Errorf("WorkingContext.Turns = %d on miss; want 0", len(conv.WorkingContext.Turns))
	}
}

func TestPgStorePersistRoundTrip(t *testing.T) {
	store, _ := newIntegrationStore(t)
	prefix := integrationPrefix(t)
	ctx := context.Background()
	now := time.Now().UTC().Round(time.Microsecond) // truncate to PG's precision

	want := Conversation{
		UserID:    prefix + "-u-roundtrip",
		Transport: "telegram",
		WorkingContext: WorkingContext{Turns: []ContextTurn{
			{UserText: "weather", Body: "sunny", SourceIDs: []string{"art-1"}, EmittedAt: now},
			{UserText: "and tomorrow?", Body: "rainy", SourceIDs: []string{"art-2", "art-3"}, EmittedAt: now.Add(time.Second)},
		}},
		PendingConfirm: &PendingConfirm{
			ConfirmRef: "ref-1", ScenarioID: "notification_schedule",
			ProposedAction: "remind tomorrow", Payload: []byte(`{"hint":"x"}`),
			ExpiresAt: now.Add(time.Hour),
		},
		PendingDisambig: &PendingDisambig{
			DisambiguationRef: "dref-1",
			Choices:           []DisambigChoiceID{{Number: 1, ID: "weather_query"}, {Number: 2, ID: "save_as_note"}},
			ExpiresAt:         now.Add(time.Minute),
		},
		LastActivityAt: now,
		SchemaVersion:  1,
	}
	if err := store.Persist(ctx, want); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteByKey(context.Background(), want.UserID, want.Transport) })

	got, found, err := store.Load(ctx, want.UserID, want.Transport)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !found {
		t.Fatalf("Load did not find persisted row")
	}
	if len(got.WorkingContext.Turns) != 2 {
		t.Fatalf("WorkingContext.Turns = %d; want 2", len(got.WorkingContext.Turns))
	}
	if got.WorkingContext.Turns[0].Body != "sunny" {
		t.Errorf("Turn[0].Body = %q; want sunny", got.WorkingContext.Turns[0].Body)
	}
	if got.WorkingContext.Turns[1].SourceIDs[1] != "art-3" {
		t.Errorf("Turn[1].SourceIDs[1] = %q; want art-3", got.WorkingContext.Turns[1].SourceIDs[1])
	}
	if got.PendingConfirm == nil || got.PendingConfirm.ConfirmRef != "ref-1" {
		t.Errorf("PendingConfirm not round-tripped: %+v", got.PendingConfirm)
	}
	if got.PendingDisambig == nil || got.PendingDisambig.DisambiguationRef != "dref-1" {
		t.Errorf("PendingDisambig not round-tripped: %+v", got.PendingDisambig)
	}
	if !got.LastActivityAt.Equal(want.LastActivityAt) {
		t.Errorf("LastActivityAt = %v; want %v", got.LastActivityAt, want.LastActivityAt)
	}
}

func TestPgStorePersistUpsertOnConflict(t *testing.T) {
	store, _ := newIntegrationStore(t)
	prefix := integrationPrefix(t)
	ctx := context.Background()
	now := time.Now().UTC().Round(time.Microsecond)

	key := Conversation{
		UserID: prefix + "-u-upsert", Transport: "telegram",
		WorkingContext: WorkingContext{Turns: []ContextTurn{{UserText: "v1", Body: "first"}}},
		LastActivityAt: now, SchemaVersion: 1,
	}
	if err := store.Persist(ctx, key); err != nil {
		t.Fatalf("Persist v1: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteByKey(context.Background(), key.UserID, key.Transport) })

	// Persist again with same PK but different body — MUST upsert.
	key.WorkingContext.Turns[0].Body = "second"
	key.LastActivityAt = now.Add(10 * time.Second)
	if err := store.Persist(ctx, key); err != nil {
		t.Fatalf("Persist v2: %v", err)
	}
	got, _, err := store.Load(ctx, key.UserID, key.Transport)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.WorkingContext.Turns[0].Body != "second" {
		t.Errorf("Body = %q; want second (upsert overwrites)", got.WorkingContext.Turns[0].Body)
	}
}

func TestPgStoreDeleteByKey(t *testing.T) {
	store, _ := newIntegrationStore(t)
	prefix := integrationPrefix(t)
	ctx := context.Background()
	now := time.Now().UTC().Round(time.Microsecond)

	conv := Conversation{
		UserID: prefix + "-u-delete", Transport: "telegram",
		LastActivityAt: now, SchemaVersion: 1,
	}
	if err := store.Persist(ctx, conv); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if err := store.DeleteByKey(ctx, conv.UserID, conv.Transport); err != nil {
		t.Fatalf("DeleteByKey: %v", err)
	}
	if _, found, err := store.Load(ctx, conv.UserID, conv.Transport); err != nil {
		t.Fatalf("Load after delete: %v", err)
	} else if found {
		t.Errorf("row still present after DeleteByKey")
	}
	// Idempotent — second delete must not error.
	if err := store.DeleteByKey(ctx, conv.UserID, conv.Transport); err != nil {
		t.Errorf("DeleteByKey on missing row should be idempotent; got %v", err)
	}
}

func TestPgStoreSweepIdleRemovesStaleRows(t *testing.T) {
	store, _ := newIntegrationStore(t)
	prefix := integrationPrefix(t)
	ctx := context.Background()

	// Row 1 — 2 hours old, should be swept with 1h TTL.
	stale := Conversation{
		UserID: prefix + "-u-stale", Transport: "telegram",
		LastActivityAt: time.Now().UTC().Add(-2 * time.Hour),
		SchemaVersion:  1,
	}
	// Row 2 — fresh, must remain.
	fresh := Conversation{
		UserID: prefix + "-u-fresh", Transport: "telegram",
		LastActivityAt: time.Now().UTC(),
		SchemaVersion:  1,
	}
	if err := store.Persist(ctx, stale); err != nil {
		t.Fatalf("Persist stale: %v", err)
	}
	if err := store.Persist(ctx, fresh); err != nil {
		t.Fatalf("Persist fresh: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteByKey(context.Background(), stale.UserID, stale.Transport)
		_ = store.DeleteByKey(context.Background(), fresh.UserID, fresh.Transport)
	})

	removed, err := store.SweepIdle(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("SweepIdle: %v", err)
	}
	if removed < 1 {
		t.Errorf("SweepIdle removed = %d; want >= 1 (the stale row)", removed)
	}

	// Fresh row MUST survive.
	if _, found, err := store.Load(ctx, fresh.UserID, fresh.Transport); err != nil {
		t.Fatalf("Load fresh: %v", err)
	} else if !found {
		t.Errorf("SweepIdle removed the fresh row")
	}
	// Stale row MUST be gone.
	if _, found, err := store.Load(ctx, stale.UserID, stale.Transport); err != nil {
		t.Fatalf("Load stale: %v", err)
	} else if found {
		t.Errorf("SweepIdle did not remove the stale row")
	}
}

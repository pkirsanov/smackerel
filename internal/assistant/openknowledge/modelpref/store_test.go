//go:build integration

// Spec 089 (Fork B) SCOPE-02 — modelpref.PostgresStore integration test
// against live PostgreSQL (the test stack via DATABASE_URL).
//
// Run with:
//
//	DATABASE_URL=postgres://... go test -tags integration \
//	    ./internal/assistant/openknowledge/modelpref/ -run TestModelPrefStore -count=1 -v
//
// db.Migrate applies every embedded migration (incl. the new
// 059_user_model_preferences.sql) before the test body, so the real SQL —
// the ON CONFLICT (actor_user_id) DO UPDATE upsert, the WHERE
// actor_user_id = $1 claim-binding, the idempotent DELETE — is exercised
// against a real table. Every userID is namespaced with a per-test unique
// prefix so parallel runs (and the two-user claim-bound case) never collide,
// and each test cleans up its own rows. SCN-089-A02 / A04 / A03.

package modelpref

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
)

func newModelPrefStore(t *testing.T) (*PostgresStore, *pgxpool.Pool) {
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
	return NewPostgresStore(pool), pool
}

// uniqueUser returns a per-test, per-suffix unique claim-bound user id, and
// registers cleanup that deletes that user's row after the test.
func uniqueUser(t *testing.T, pool *pgxpool.Pool, suffix string) string {
	t.Helper()
	id := "modelpref-int-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-") + "-" + suffix
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cctx, `DELETE FROM user_model_preferences WHERE actor_user_id = $1`, id); err != nil {
			t.Logf("cleanup user_model_preferences %s: %v", id, err)
		}
	})
	return id
}

// TestModelPrefStore_GetAfterSet_PersistsAcrossReads_Spec089 — SCN-089-A02.
// Set once, then two separate Get calls both return the sticky model with
// ok=true — the "no flag repeated" guarantee at the store layer.
func TestModelPrefStore_GetAfterSet_PersistsAcrossReads_Spec089(t *testing.T) {
	store, pool := newModelPrefStore(t)
	ctx := context.Background()
	userA := uniqueUser(t, pool, "a")

	// No row yet ⇒ ok=false ⇒ caller inherits the SST default.
	if _, ok, err := store.Get(ctx, userA); err != nil || ok {
		t.Fatalf("Get before Set: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}

	if err := store.Set(ctx, userA, "deepseek-r1:7b"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	for i := 0; i < 2; i++ {
		pref, ok, err := store.Get(ctx, userA)
		if err != nil {
			t.Fatalf("Get #%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("Get #%d: want ok=true (sticky persists across reads), got ok=false", i)
		}
		if pref.SynthesisModel != "deepseek-r1:7b" {
			t.Fatalf("Get #%d: want sticky deepseek-r1:7b, got %q", i, pref.SynthesisModel)
		}
	}
}

// TestModelPrefStore_Set_UpsertOnConflict_Spec089 — SCN-089-A02. A second Set
// overwrites the same row (one row per user), Get returns the latest, and
// updated_at advances.
func TestModelPrefStore_Set_UpsertOnConflict_Spec089(t *testing.T) {
	store, pool := newModelPrefStore(t)
	ctx := context.Background()
	userA := uniqueUser(t, pool, "a")

	t0 := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	store.WithNow(func() time.Time { return t0 })
	if err := store.Set(ctx, userA, "deepseek-r1:7b"); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	first, _, err := store.Get(ctx, userA)
	if err != nil {
		t.Fatalf("Get after first Set: %v", err)
	}

	t1 := t0.Add(1 * time.Hour)
	store.WithNow(func() time.Time { return t1 })
	if err := store.Set(ctx, userA, "gemma4:26b"); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	second, ok, err := store.Get(ctx, userA)
	if err != nil || !ok {
		t.Fatalf("Get after second Set: ok=%v err=%v", ok, err)
	}
	if second.SynthesisModel != "gemma4:26b" {
		t.Fatalf("want overwritten gemma4:26b, got %q", second.SynthesisModel)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("updated_at must advance on upsert: first=%v second=%v", first.UpdatedAt, second.UpdatedAt)
	}

	// Exactly one row for the user (no duplicate from the upsert).
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_model_preferences WHERE actor_user_id = $1`, userA).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("upsert must keep exactly one row per user, got %d", count)
	}
}

// TestModelPrefStore_ClaimBound_UserBNeverReadsUserA_Spec089 (ADVERSARIAL) —
// SCN-089-A04 store arm. After user A sets a sticky model, user B's Get
// returns ok=false (inherits default), and a Set for B never mutates A's row.
// This fails if the store ever leaks a key across users (e.g. a missing/
// wrong WHERE actor_user_id binding).
func TestModelPrefStore_ClaimBound_UserBNeverReadsUserA_Spec089(t *testing.T) {
	store, pool := newModelPrefStore(t)
	ctx := context.Background()
	userA := uniqueUser(t, pool, "a")
	userB := uniqueUser(t, pool, "b")

	if err := store.Set(ctx, userA, "deepseek-r1:7b"); err != nil {
		t.Fatalf("Set A: %v", err)
	}

	// User B has set nothing ⇒ MUST inherit the default (ok=false), NOT see A's.
	prefB, okB, err := store.Get(ctx, userB)
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if okB {
		t.Fatalf("CLAIM-BINDING BREACH: user B read a preference (%q) when B set none — A's row leaked across the actor key", prefB.SynthesisModel)
	}

	// B sets its own sticky — A's row MUST be untouched.
	if err := store.Set(ctx, userB, "gemma4:26b"); err != nil {
		t.Fatalf("Set B: %v", err)
	}
	prefA, okA, err := store.Get(ctx, userA)
	if err != nil || !okA {
		t.Fatalf("Get A after B Set: ok=%v err=%v", okA, err)
	}
	if prefA.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("CLAIM-BINDING BREACH: user B's Set mutated user A's row (A now %q)", prefA.SynthesisModel)
	}
}

// TestModelPrefStore_Clear_IdempotentDelete_Spec089 — SCN-089-A03 reset. Clear
// deletes the row; a subsequent Get ⇒ ok=false; a second Clear on an absent
// row is a no-op (no error).
func TestModelPrefStore_Clear_IdempotentDelete_Spec089(t *testing.T) {
	store, pool := newModelPrefStore(t)
	ctx := context.Background()
	userA := uniqueUser(t, pool, "a")

	if err := store.Set(ctx, userA, "deepseek-r1:7b"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Clear(ctx, userA); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok, err := store.Get(ctx, userA); err != nil || ok {
		t.Fatalf("Get after Clear: want ok=false err=nil (inherits default), got ok=%v err=%v", ok, err)
	}
	// Second Clear on an absent row MUST be a no-op (idempotent reset).
	if err := store.Clear(ctx, userA); err != nil {
		t.Fatalf("second Clear on absent row must be a no-op, got: %v", err)
	}
}

// TestModelPrefStore_GatherModelColumnReservedUnread_Spec089 — SCN-089-A04
// reserved-column. The gather_model column exists (migration 059 shape) but
// Get never surfaces it (the Preference type has no gather field), so a sticky
// gather can never be silently activated. Fails if Get starts reading it.
func TestModelPrefStore_GatherModelColumnReservedUnread_Spec089(t *testing.T) {
	store, pool := newModelPrefStore(t)
	ctx := context.Background()
	userA := uniqueUser(t, pool, "a")

	// The reserved column exists in the live schema.
	var colCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'user_model_preferences' AND column_name = 'gather_model'`).Scan(&colCount); err != nil {
		t.Fatalf("inspect columns: %v", err)
	}
	if colCount != 1 {
		t.Fatalf("migration 059 must define the reserved gather_model column, found %d", colCount)
	}

	// Raw-insert a row WITH gather_model populated; Get must surface ONLY the
	// synthesis model — the reserved gather column is unread by the resolver.
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_model_preferences (actor_user_id, synthesis_model, gather_model, updated_at)
		VALUES ($1, $2, $3, now())`, userA, "deepseek-r1:7b", "llama3.1:8b"); err != nil {
		t.Fatalf("raw insert with gather_model: %v", err)
	}
	pref, ok, err := store.Get(ctx, userA)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if pref.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("want synthesis deepseek-r1:7b, got %q", pref.SynthesisModel)
	}
	// The Preference type structurally cannot carry a gather model (no field);
	// this test pins that contract at the schema + read level.
}

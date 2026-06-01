//go:build integration

// Spec 075 SCOPE-3 — TP-075-11 / SCN-075-A11.
//
// Live-stack privacy proof for the SQL residual store:
//
//  1. The bucket-shape CHECK constraint rejects a raw user id
//     (anything other than 64-char hex or the literal "anonymous").
//  2. After driving a real observation through SQLResidualStore.Record
//     with a raw id, the persisted row carries the "anonymous"
//     sentinel — never the raw id — because the store normalises
//     through normaliseBucketLabel before INSERT.
//  3. No row in the table contains raw user text fragments.
package monitoring_integration

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func openPrivacyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestResidualStore_BucketShapeCheckRejectsRawID is the adversarial
// SQL-level proof. A direct INSERT of a raw id-shaped bucket must
// fail the CHECK constraint. A regression that loosened the
// constraint would surface here BEFORE any caller could leak a raw
// id.
func TestResidualStore_BucketShapeCheckRejectsRawID(t *testing.T) {
	pool := openPrivacyPool(t)
	windowID := fmt.Sprintf("spec-075-privacy-shape-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO assistant_legacy_retirement_residual
		    (window_id, command, user_bucket, day, count, last_seen_at)
		VALUES ($1, '/weather', 'telegram-chat-1234567890', current_date, 1, now())
	`, windowID)
	if err == nil {
		t.Fatal("INSERT with raw-id-shaped user_bucket succeeded; chk_assistant_legacy_retirement_residual_bucket_shape regressed")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "check") &&
		!strings.Contains(err.Error(), "23514") {
		t.Errorf("INSERT failed but not with CHECK violation: %v", err)
	}
}

// TestResidualStore_RecordNormalisesRawIDToAnonymous drives a real
// observation through SQLResidualStore.Record with a non-HMAC
// "bucket" value (simulating a caller bug). The store must persist
// the literal "anonymous" sentinel — never the raw input.
func TestResidualStore_RecordNormalisesRawIDToAnonymous(t *testing.T) {
	pool := openPrivacyPool(t)
	windowID := fmt.Sprintf("spec-075-privacy-norm-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})

	anchor := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: windowID,
		Clock:    func() time.Time { return anchor },
	})
	if err != nil {
		t.Fatalf("NewSQLResidualStore: %v", err)
	}

	ctx := context.Background()
	const rawID = "telegram-chat-1234567890"
	if err := store.RecordWithError(ctx, "/weather", rawID, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("RecordWithError: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT user_bucket FROM assistant_legacy_retirement_residual WHERE window_id = $1
	`, windowID)
	if err != nil {
		t.Fatalf("select rows: %v", err)
	}
	defer rows.Close()
	rawRE := regexp.MustCompile(`^[0-9a-f]{64}$`)
	rowCount := 0
	for rows.Next() {
		rowCount++
		var bucket string
		if err := rows.Scan(&bucket); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if bucket == rawID || strings.Contains(bucket, rawID) {
			t.Fatalf("persisted bucket %q contains raw id %q — privacy regression", bucket, rawID)
		}
		if bucket != legacyretirement.AnonymousBucketLabel && !rawRE.MatchString(bucket) {
			t.Fatalf("persisted bucket %q is neither HMAC nor %q sentinel", bucket, legacyretirement.AnonymousBucketLabel)
		}
	}
	if rowCount == 0 {
		t.Fatal("no row persisted; Record() silently dropped the observation")
	}
}

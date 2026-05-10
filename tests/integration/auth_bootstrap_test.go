//go:build integration

// Spec 044 Scope 01 — T1-08 bootstrap integration test.
//
// Validates the full bootstrap path against a live test DB:
//
//  1. Apply the auth migration (it ships in
//     internal/db/migrations/033_auth_per_user_bearer.sql and is picked
//     up by db.Migrate when the test stack starts).
//  2. Confirm a freshly-migrated DB has zero enrolled users.
//  3. Call BearerStore.Enroll + IssueToken + HashToken + PersistToken
//     to simulate the bootstrap subcommand WITHOUT shelling out to the
//     CLI.
//  4. Round-trip the issued token through VerifyAndParse to prove the
//     operator-facing wire token is usable.
//  5. Adversarial: attempt a duplicate enrollment and confirm the
//     UNIQUE constraint surfaces a duplicate-user error.
//
// SCN-AUTH-008 evidence: the test runs against a DB that started empty
// and ends with exactly one auth_users row + one auth_tokens row whose
// hashed_token matches the HMAC of the wire token under the test
// hashing key.
//
// No `t.Skip()` — when DATABASE_URL is unset, this test fails with a
// clear message because spec 043 set the no-skip precedent for live-
// stack tests.
package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/db"
)

// authTestPool opens a pgx pool against the live test stack DATABASE_URL
// and applies migrations. Fails fast (NOT skip) when env is missing so
// the auth integration tests do not silently turn into no-ops.
func authTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("auth integration test requires DATABASE_URL — run via `./smackerel.sh test integration` which brings up the live test stack and exports DATABASE_URL")
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping DATABASE_URL: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// resetAuthTables drops every row from the spec 044 tables so each test
// starts from a clean slate. Test isolation policy (live test stack is
// shared but disposable; rows do not leak across runs).
func resetAuthTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, sql := range []string{
		`DELETE FROM auth_revocations`,
		`DELETE FROM auth_tokens`,
		`DELETE FROM auth_users`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("reset %q: %v", sql, err)
		}
	}
}

// TestAuthBootstrap_FreshProduction_EnrollsFirstUser proves the
// bootstrap path yields a usable per-user token.
func TestAuthBootstrap_FreshProduction_EnrollsFirstUser(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()
	resetAuthTables(t, pool)

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	count, err := store.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 0 {
		t.Fatalf("auth_users not empty before bootstrap: count=%d", count)
	}

	priv, pub := auth.GenerateSigningKeypair()
	hashKey := "test-hashing-key-9b2f4c8a-do-not-reuse-in-production"
	keyID := "key-test-2026-05"

	enrolledBy := "bootstrap@integration-test"
	if err := store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     "user-bootstrap-001",
		EnrolledBy: enrolledBy,
		Notes:      "spec 044 T1-08 bootstrap integration",
	}); err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	now := time.Now()
	clock := func() time.Time { return now }
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "user-bootstrap-001",
		TokenID:    "tok-bootstrap-001",
		SigningKey: priv,
		KeyID:      keyID,
		TTL:        24 * time.Hour,
		Issuer:     "smackerel-test",
		Now:        clock,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	hashed, err := auth.HashToken(issued.WireToken, hashKey)
	if err != nil {
		t.Fatalf("HashToken: %v", err)
	}

	if err := store.PersistToken(context.Background(), auth.PersistTokenParams{
		TokenID:      "tok-bootstrap-001",
		UserID:       "user-bootstrap-001",
		KeyID:        keyID,
		IssuedAt:     issued.IssuedAt,
		ExpiresAt:    issued.ExpiresAt,
		HashedToken:  hashed,
		IssuedBy:     enrolledBy,
		IssuedSource: "bootstrap",
	}); err != nil {
		t.Fatalf("PersistToken: %v", err)
	}

	parsed, err := auth.VerifyAndParse(issued.WireToken, auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        keyID,
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                clock,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse round-trip: %v", err)
	}
	if parsed.UserID != "user-bootstrap-001" {
		t.Errorf("UserID round-trip mismatch: want user-bootstrap-001 got %q", parsed.UserID)
	}
	if parsed.TokenID != "tok-bootstrap-001" {
		t.Errorf("TokenID round-trip mismatch: got %q", parsed.TokenID)
	}

	count, err = store.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers post-bootstrap: %v", err)
	}
	if count != 1 {
		t.Errorf("auth_users count: want 1 got %d", count)
	}

	var dbHash string
	if err := pool.QueryRow(context.Background(),
		`SELECT hashed_token FROM auth_tokens WHERE token_id = $1`,
		"tok-bootstrap-001").Scan(&dbHash); err != nil {
		t.Fatalf("query hashed_token: %v", err)
	}
	if dbHash != hashed {
		t.Errorf("DB hashed_token does not match HashToken output:\n want=%s\n  got=%s", hashed, dbHash)
	}

	// Adversarial: a SECOND bootstrap call (simulating an operator who
	// runs `bootstrap` twice) MUST surface as a duplicate enrollment
	// error against auth_users.user_id UNIQUE.
	err = store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     "user-bootstrap-001",
		EnrolledBy: enrolledBy,
	})
	if err == nil {
		t.Fatal("second Enroll of same user_id MUST fail (unique constraint)")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
		!strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Errorf("error should indicate a uniqueness violation, got: %v", err)
	}
}

// TestAuthBootstrap_PublicHexDerivation proves PublicHexFromSecretHex
// is deterministic — the public hex derived from a private hex MUST
// match the public hex returned by GenerateSigningKeypair when both
// halves come from the same secret. Adversarial: attempting derivation
// on a malformed hex string MUST fail loudly.
func TestAuthBootstrap_PublicHexDerivation(t *testing.T) {
	priv, expectedPub := auth.GenerateSigningKeypair()

	derivedPub, err := auth.PublicHexFromSecretHex(priv)
	if err != nil {
		t.Fatalf("PublicHexFromSecretHex: %v", err)
	}
	if derivedPub != expectedPub {
		t.Errorf("derived public hex does not match GenerateSigningKeypair pair:\n want=%s\n  got=%s", expectedPub, derivedPub)
	}

	// Adversarial: malformed input MUST fail loudly, not silently
	// produce a zero key.
	if _, err := auth.PublicHexFromSecretHex("not-hex"); err == nil {
		t.Error("PublicHexFromSecretHex MUST reject malformed hex; got nil error")
	}
	if _, err := auth.PublicHexFromSecretHex(""); err == nil {
		t.Error("PublicHexFromSecretHex MUST reject empty input; got nil error")
	}
}

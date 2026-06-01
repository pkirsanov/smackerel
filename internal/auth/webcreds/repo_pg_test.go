//go:build integration

// Spec 070 SCOPE-1 — PostgresRepo integration tests against a live
// ephemeral test Postgres (./smackerel.sh test integration).
//
// Each test namespaces usernames with a per-test prefix to keep
// parallel runs collision-free against the shared web_user_credentials
// table.

package webcreds

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
)

func newIntegrationRepo(t *testing.T) (*PostgresRepo, *pgxpool.Pool) {
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
	repo, err := NewPostgresRepo(pool)
	if err != nil {
		t.Fatalf("NewPostgresRepo: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort cleanup of namespaced rows; ignore errors.
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM web_user_credentials WHERE username LIKE 'wc-int-%'`)
	})
	return repo, pool
}

func integrationUsername(t *testing.T, tag string) string {
	t.Helper()
	stamp := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
	return "wc-int-" + tag + "-" + stamp
}

func TestPostgresRepo_UpsertInsertsNewRow(t *testing.T) {
	repo, pool := newIntegrationRepo(t)
	ctx := context.Background()
	user := integrationUsername(t, "insert")

	if err := repo.UpsertPassword(ctx, user, "correct-horse-battery-staple", true); err != nil {
		t.Fatalf("UpsertPassword(create=true) err: %v", err)
	}

	var hash string
	var created time.Time
	if err := pool.QueryRow(ctx,
		`SELECT password_hash, created_at FROM web_user_credentials WHERE username = $1`,
		user,
	).Scan(&hash, &created); err != nil {
		t.Fatalf("select after insert: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("stored hash is not an argon2id PHC string: %q", hash)
	}
	if created.IsZero() {
		t.Errorf("created_at not populated")
	}
}

func TestPostgresRepo_UpsertCreateRejectsExisting(t *testing.T) {
	repo, _ := newIntegrationRepo(t)
	ctx := context.Background()
	user := integrationUsername(t, "dup")

	if err := repo.UpsertPassword(ctx, user, "correct-horse-battery-staple", true); err != nil {
		t.Fatalf("first UpsertPassword: %v", err)
	}
	err := repo.UpsertPassword(ctx, user, "another-long-enough-pw", true)
	if err != ErrUserExists {
		t.Fatalf("second UpsertPassword(create=true) expected ErrUserExists, got %v", err)
	}
}

func TestPostgresRepo_UpsertRotatePreservesCreatedAt(t *testing.T) {
	repo, pool := newIntegrationRepo(t)
	ctx := context.Background()
	user := integrationUsername(t, "rotate")

	if err := repo.UpsertPassword(ctx, user, "first-password-here", true); err != nil {
		t.Fatalf("create: %v", err)
	}
	var firstCreated time.Time
	var firstHash string
	if err := pool.QueryRow(ctx,
		`SELECT created_at, password_hash FROM web_user_credentials WHERE username = $1`,
		user,
	).Scan(&firstCreated, &firstHash); err != nil {
		t.Fatalf("select after create: %v", err)
	}

	if err := repo.UpsertPassword(ctx, user, "second-password-here", false); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	var secondCreated time.Time
	var secondHash string
	if err := pool.QueryRow(ctx,
		`SELECT created_at, password_hash FROM web_user_credentials WHERE username = $1`,
		user,
	).Scan(&secondCreated, &secondHash); err != nil {
		t.Fatalf("select after rotate: %v", err)
	}
	if !secondCreated.Equal(firstCreated) {
		t.Errorf("created_at changed across rotate: %v -> %v", firstCreated, secondCreated)
	}
	if secondHash == firstHash {
		t.Errorf("password_hash unchanged after rotate")
	}
}

func TestPostgresRepo_UpsertRotateRejectsMissing(t *testing.T) {
	repo, _ := newIntegrationRepo(t)
	ctx := context.Background()
	user := integrationUsername(t, "ghost")

	err := repo.UpsertPassword(ctx, user, "any-long-enough-pw", false)
	if err != ErrUserNotFound {
		t.Fatalf("UpsertPassword(create=false) on missing user: expected ErrUserNotFound, got %v", err)
	}
}

func TestPostgresRepo_VerifyAndTouchHappyPath(t *testing.T) {
	repo, pool := newIntegrationRepo(t)
	ctx := context.Background()
	user := integrationUsername(t, "verify-ok")
	pw := "correct-horse-battery-staple"

	if err := repo.UpsertPassword(ctx, user, pw, true); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Pre-condition: last_login_at is NULL on a fresh row.
	var pre *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_login_at FROM web_user_credentials WHERE username = $1`,
		user,
	).Scan(&pre); err != nil {
		t.Fatalf("select pre: %v", err)
	}
	if pre != nil {
		t.Fatalf("last_login_at should start NULL, got %v", pre)
	}

	if err := repo.VerifyAndTouch(ctx, user, pw); err != nil {
		t.Fatalf("VerifyAndTouch happy path: %v", err)
	}

	var post *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_login_at FROM web_user_credentials WHERE username = $1`,
		user,
	).Scan(&post); err != nil {
		t.Fatalf("select post: %v", err)
	}
	if post == nil {
		t.Errorf("last_login_at not bumped after successful verify")
	}
}

func TestPostgresRepo_VerifyAndTouchWrongPasswordKeepsLastLoginUnchanged(t *testing.T) {
	repo, pool := newIntegrationRepo(t)
	ctx := context.Background()
	user := integrationUsername(t, "verify-wrong")

	if err := repo.UpsertPassword(ctx, user, "correct-horse-battery-staple", true); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// First, do a successful verify so last_login_at has a value to compare.
	if err := repo.VerifyAndTouch(ctx, user, "correct-horse-battery-staple"); err != nil {
		t.Fatalf("seed verify: %v", err)
	}
	var baseline *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_login_at FROM web_user_credentials WHERE username = $1`,
		user,
	).Scan(&baseline); err != nil {
		t.Fatalf("select baseline: %v", err)
	}
	if baseline == nil {
		t.Fatalf("baseline last_login_at unexpectedly NULL")
	}

	if err := repo.VerifyAndTouch(ctx, user, "wrong-password-here"); err != ErrInvalidCredentials {
		t.Fatalf("wrong-password verify expected ErrInvalidCredentials, got %v", err)
	}

	var after *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_login_at FROM web_user_credentials WHERE username = $1`,
		user,
	).Scan(&after); err != nil {
		t.Fatalf("select after: %v", err)
	}
	if after == nil || !after.Equal(*baseline) {
		t.Errorf("last_login_at changed on failed verify: baseline=%v after=%v", baseline, after)
	}
}

func TestPostgresRepo_VerifyAndTouchUnknownUser(t *testing.T) {
	repo, _ := newIntegrationRepo(t)
	ctx := context.Background()
	user := integrationUsername(t, "verify-ghost")

	err := repo.VerifyAndTouch(ctx, user, "any-long-enough-pw")
	if err != ErrInvalidCredentials {
		t.Fatalf("unknown-user verify expected ErrInvalidCredentials, got %v", err)
	}
}

func TestPostgresRepo_ListReturnsSeededRows(t *testing.T) {
	repo, _ := newIntegrationRepo(t)
	ctx := context.Background()
	tag := integrationUsername(t, "list")

	if err := repo.UpsertPassword(ctx, tag+"-a", "correct-horse-battery-staple", true); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := repo.UpsertPassword(ctx, tag+"-b", "correct-horse-battery-staple", true); err != nil {
		t.Fatalf("seed b: %v", err)
	}

	rows, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var foundA, foundB bool
	for _, r := range rows {
		if r.Username == tag+"-a" {
			foundA = true
			if r.CreatedAt.IsZero() {
				t.Errorf("List row %q has zero CreatedAt", r.Username)
			}
		}
		if r.Username == tag+"-b" {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Errorf("List did not return both seeded users (a=%v b=%v); rows=%d", foundA, foundB, len(rows))
	}
}

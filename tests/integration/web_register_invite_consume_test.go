//go:build integration

// Spec 093 SCOPE-02 — live-DB integration tests for the widened
// /v1/web/register OR-gate. Driven by the curated go-integration lane
// (./smackerel.sh test integration). They construct api.Dependencies with the
// REAL webcreds + webinvite Postgres repos and drive HandleWebRegister via
// httptest against the live ephemeral DB, proving the DB-invite consume+create
// path end-to-end (account created + invite atomically marked used in one tx),
// single-use reuse rejection, duplicate-username rollback, and that the static
// secret path consumes NO invite. Shares the inviteTag / testPool helpers from
// web_registration_invite_test.go (same package + build tag).

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth/webcreds"
	"github.com/smackerel/smackerel/internal/auth/webinvite"
	"github.com/smackerel/smackerel/internal/db"
)

// The user-facing banner contract (bound verbatim by spec.md / design.md). The
// in-package constants are unexported, so the integration package asserts the
// literal strings — which is exactly the non-enumeration contract under test.
const (
	registerGenericBanner   = "Registration is not available or the invite is invalid."
	registerDuplicateBanner = "That username is taken."
	registerRedirect        = "/login?registered=1"
)

func registerInviteDeps(t *testing.T, staticToken string) (*api.Dependencies, *webinvite.PostgresRepo, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate (058): %v", err)
	}
	creds, err := webcreds.NewPostgresRepo(pool)
	if err != nil {
		t.Fatalf("webcreds.NewPostgresRepo: %v", err)
	}
	inv, err := webinvite.NewPostgresRepo(pool)
	if err != nil {
		t.Fatalf("webinvite.NewPostgresRepo: %v", err)
	}
	deps := &api.Dependencies{
		Environment:                "development",
		AuthToken:                  "shared-token-unused-by-register",
		WebCredentials:             creds,
		WebInvites:                 inv,
		WebRegistrationInviteToken: staticToken,
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(c, `DELETE FROM web_registration_invites WHERE created_by LIKE 'wi-int-%'`)
		_, _ = pool.Exec(c, `DELETE FROM web_user_credentials WHERE username LIKE 'wi-int-%'`)
	})
	return deps, inv, pool
}

func postRegister(t *testing.T, deps *api.Dependencies, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	body := form.Encode()
	req := httptest.NewRequest(http.MethodPost, "/v1/web/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	deps.HandleWebRegister(rec, req)
	return rec
}

// TestWebRegisterIntegration_DBInviteConsumes — a new person registers once with
// a live DB invite; the account is created (argon2id) AND the invite is marked
// used in the SAME tx; 303 → /login?registered=1 with NO Set-Cookie (SCN-093-08).
func TestWebRegisterIntegration_DBInviteConsumes(t *testing.T) {
	deps, inv, pool := registerInviteDeps(t, "") // DB-invite only, no static
	ctx := context.Background()
	createdBy := inviteTag(t, "reg-by")
	username := inviteTag(t, "reg-newcomer")

	plaintext, err := inv.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	rec := postRegister(t, deps, url.Values{
		"invite-token":     {plaintext},
		"username":         {username},
		"password":         {"correct-horse-battery-staple"},
		"confirm-password": {"correct-horse-battery-staple"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, registerRedirect) {
		t.Errorf("Location=%q want prefix %q", loc, registerRedirect)
	}
	if sc := rec.Header().Get("Set-Cookie"); sc != "" {
		t.Errorf("register MUST NOT set a cookie; got Set-Cookie=%q", sc)
	}

	// Account row created with an argon2id hash.
	var hash string
	if err := pool.QueryRow(ctx,
		`SELECT password_hash FROM web_user_credentials WHERE username = $1`, username).Scan(&hash); err != nil {
		t.Fatalf("account row not found: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("stored hash is not argon2id: %q", hash)
	}
	// Invite atomically marked used.
	var usedAt *time.Time
	var usedBy *string
	if err := pool.QueryRow(ctx,
		`SELECT used_at, used_by FROM web_registration_invites WHERE token_hash = $1`,
		webinvite.HashToken(plaintext)).Scan(&usedAt, &usedBy); err != nil {
		t.Fatalf("select invite used_*: %v", err)
	}
	if usedAt == nil || usedBy == nil || *usedBy != username {
		t.Fatalf("invite not marked used by %q in the same tx: at=%v by=%v", username, usedAt, usedBy)
	}
}

// TestWebRegisterIntegration_ReusedInviteRejected — a second register with an
// already-consumed invite is rejected with the generic banner; no second
// account; used_* unchanged (SCN-093-09 / single-use).
func TestWebRegisterIntegration_ReusedInviteRejected(t *testing.T) {
	deps, inv, pool := registerInviteDeps(t, "")
	ctx := context.Background()
	createdBy := inviteTag(t, "reuse-by")
	user1 := inviteTag(t, "reuse-u1")
	user2 := inviteTag(t, "reuse-u2")

	plaintext, err := inv.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// First registration consumes the invite.
	rec1 := postRegister(t, deps, url.Values{
		"invite-token":     {plaintext},
		"username":         {user1},
		"password":         {"correct-horse-battery-staple"},
		"confirm-password": {"correct-horse-battery-staple"},
	})
	if rec1.Code != http.StatusSeeOther {
		t.Fatalf("first register status=%d want 303; body=%s", rec1.Code, rec1.Body.String())
	}

	// Second registration with the SAME token + a different username is rejected.
	rec2 := postRegister(t, deps, url.Values{
		"invite-token":     {plaintext},
		"username":         {user2},
		"password":         {"correct-horse-battery-staple"},
		"confirm-password": {"correct-horse-battery-staple"},
	})
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("reuse status=%d want 401; body=%s", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), registerGenericBanner) {
		t.Errorf("reuse body missing generic banner: %s", rec2.Body.String())
	}
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM web_user_credentials WHERE username = $1`, user2).Scan(&n); err != nil {
		t.Fatalf("count user2: %v", err)
	}
	if n != 0 {
		t.Fatalf("reused invite created a second account for %q", user2)
	}
	var usedBy *string
	if err := pool.QueryRow(ctx,
		`SELECT used_by FROM web_registration_invites WHERE token_hash = $1`,
		webinvite.HashToken(plaintext)).Scan(&usedBy); err != nil {
		t.Fatalf("select used_by: %v", err)
	}
	if usedBy == nil || *usedBy != user1 {
		t.Fatalf("used_by changed on the rejected reuse: %v (want %q)", usedBy, user1)
	}
}

// TestWebRegisterIntegration_DuplicateUsernameRollsBack — a duplicate username on
// the DB-invite path returns 409 and the invite is NOT consumed (rollback);
// it can be retried (SCN-093-12).
func TestWebRegisterIntegration_DuplicateUsernameRollsBack(t *testing.T) {
	deps, inv, pool := registerInviteDeps(t, "")
	ctx := context.Background()
	createdBy := inviteTag(t, "dupreg-by")
	taken := inviteTag(t, "dupreg-taken")

	// Seed an existing account "taken".
	if _, err := pool.Exec(ctx,
		`INSERT INTO web_user_credentials (username, password_hash) VALUES ($1, $2)`,
		taken, "$argon2id$v=19$m=65536,t=1,p=4$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaA",
	); err != nil {
		t.Fatalf("seed taken account: %v", err)
	}
	plaintext, err := inv.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	rec := postRegister(t, deps, url.Values{
		"invite-token":     {plaintext},
		"username":         {taken},
		"password":         {"correct-horse-battery-staple"},
		"confirm-password": {"correct-horse-battery-staple"},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d want 409; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), registerDuplicateBanner) {
		t.Errorf("duplicate body missing %q: %s", registerDuplicateBanner, rec.Body.String())
	}
	// Invite NOT consumed (rollback) — used_at stays NULL.
	var usedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT used_at FROM web_registration_invites WHERE token_hash = $1`,
		webinvite.HashToken(plaintext)).Scan(&usedAt); err != nil {
		t.Fatalf("select used_at: %v", err)
	}
	if usedAt != nil {
		t.Fatalf("invite was consumed on a duplicate-username rollback: used_at=%v", usedAt)
	}
	// Retriable: registering with a fresh username on the SAME invite succeeds.
	retryUser := inviteTag(t, "dupreg-retry")
	recRetry := postRegister(t, deps, url.Values{
		"invite-token":     {plaintext},
		"username":         {retryUser},
		"password":         {"correct-horse-battery-staple"},
		"confirm-password": {"correct-horse-battery-staple"},
	})
	if recRetry.Code != http.StatusSeeOther {
		t.Fatalf("retry status=%d want 303 (invite must still be live); body=%s", recRetry.Code, recRetry.Body.String())
	}
}

// TestWebRegisterIntegration_StaticSecretConsumesNothing — the static secret
// still registers an account and marks NO invite used (reusable bootstrap)
// (SCN-093-10).
func TestWebRegisterIntegration_StaticSecretConsumesNothing(t *testing.T) {
	const staticToken = "wi-int-static-bootstrap-secret"
	deps, inv, pool := registerInviteDeps(t, staticToken)
	ctx := context.Background()

	// An outstanding DB invite exists that the static path must NOT touch.
	createdBy := inviteTag(t, "static-untouched-by")
	untouched, err := inv.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate untouched invite: %v", err)
	}
	username := inviteTag(t, "static-newcomer")

	rec := postRegister(t, deps, url.Values{
		"invite-token":     {staticToken},
		"username":         {username},
		"password":         {"correct-horse-battery-staple"},
		"confirm-password": {"correct-horse-battery-staple"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("static register status=%d want 303; body=%s", rec.Code, rec.Body.String())
	}
	// Account created via the static (UpsertPassword) path.
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM web_user_credentials WHERE username = $1`, username).Scan(&n); err != nil {
		t.Fatalf("count account: %v", err)
	}
	if n != 1 {
		t.Fatalf("static path did not create the account for %q", username)
	}
	// The outstanding invite is UNTOUCHED (still live, used_at NULL).
	var usedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT used_at FROM web_registration_invites WHERE token_hash = $1`,
		webinvite.HashToken(untouched)).Scan(&usedAt); err != nil {
		t.Fatalf("select untouched invite: %v", err)
	}
	if usedAt != nil {
		t.Fatalf("the static-secret path consumed a DB invite (used_at=%v); it must consume NOTHING", usedAt)
	}
	if live, err := inv.IsLive(ctx, webinvite.HashToken(untouched)); err != nil || !live {
		t.Fatalf("the untouched invite is no longer live after a static registration (live=%v err=%v)", live, err)
	}
}

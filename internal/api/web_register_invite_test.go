// Spec 093 SCOPE-02 — unit coverage for the widened HandleWebRegister OR-gate.
//
// Drives the handler via httptest with the spec-091 in-memory webcreds fakeRepo
// PLUS a controllable webinvite fake (fakeInviteRepo). These prove the GATE
// ROUTING + non-enumeration without a database: which branch the handler takes
// (static-first / DB-second / disabled) and that every gate failure yields the
// byte-identical 401 + shared banner. The REAL atomic DB-invite consume+create
// (onClaimed → webcreds.HashAndInsertTx on a real tx) is proven against a live
// Postgres in tests/integration/web_registration_invite_test.go.
package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/auth/webcreds"
	"github.com/smackerel/smackerel/internal/auth/webinvite"
)

// fakeInviteRepo implements webinvite.Repo with controllable IsLive +
// ConsumeAndCreate, recording the consume call so a unit test can assert the
// handler routed to the DB branch. ConsumeAndCreate does NOT run onClaimed: the
// handler only consumes the returned outcome, and the real callback path is
// integration-tested.
type fakeInviteRepo struct {
	live            map[string]bool
	consumeOutcome  webinvite.ConsumeOutcome
	consumeErr      error
	consumeCalls    int
	lastConsumeHash string
	lastConsumeUser string
}

func (r *fakeInviteRepo) Generate(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", nil
}

func (r *fakeInviteRepo) IsLive(_ context.Context, tokenHash string) (bool, error) {
	return r.live[tokenHash], nil
}

func (r *fakeInviteRepo) ConsumeAndCreate(_ context.Context, tokenHash, usedBy string,
	_ func(context.Context, pgx.Tx) error) (webinvite.ConsumeOutcome, error) {
	r.consumeCalls++
	r.lastConsumeHash = tokenHash
	r.lastConsumeUser = usedBy
	return r.consumeOutcome, r.consumeErr
}

func (r *fakeInviteRepo) List(_ context.Context) ([]webinvite.InviteRow, error) { return nil, nil }
func (r *fakeInviteRepo) Revoke(_ context.Context, _ string) (webinvite.RevokeOutcome, error) {
	return webinvite.RevokeNoop, nil
}

func newRegisterInviteDeps(inviteToken string, creds webcreds.Repo, invites webinvite.Repo) *Dependencies {
	return &Dependencies{
		Environment:                "development",
		AuthToken:                  "shared-token-unused-by-register",
		WebCredentials:             creds,
		WebRegistrationInviteToken: inviteToken,
		WebInvites:                 invites,
	}
}

func validRegisterForm(invite, username string) url.Values {
	return url.Values{
		"invite-token":     {invite},
		"username":         {username},
		"password":         {"correct-horse-battery"},
		"confirm-password": {"correct-horse-battery"},
	}
}

// TestWebRegister_OrGate — the static-first / DB-second / disabled branch
// selection (SCN-093-08/10 routing half). DB-path persistence is integration.
func TestWebRegister_OrGate(t *testing.T) {
	t.Run("static-first-consumes-nothing", func(t *testing.T) {
		creds := &fakeRepo{creds: map[string]string{}}
		invites := &fakeInviteRepo{live: map[string]bool{}}
		deps := newRegisterInviteDeps("the-static-secret", creds, invites)

		rec := postWebRegisterForm(t, deps, validRegisterForm("the-static-secret", "boot-op"))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("static path status=%d want 303; body=%s", rec.Code, rec.Body.String())
		}
		if _, ok := creds.creds["boot-op"]; !ok {
			t.Errorf("static path did not create the account via UpsertPassword; creds=%v", creds.creds)
		}
		if invites.consumeCalls != 0 {
			t.Errorf("static path consumed an invite (%d ConsumeAndCreate calls); it must consume NOTHING", invites.consumeCalls)
		}
	})

	t.Run("db-second-when-no-static-configured", func(t *testing.T) {
		creds := &fakeRepo{creds: map[string]string{}}
		hash := webinvite.HashToken("inv_live_one")
		invites := &fakeInviteRepo{live: map[string]bool{hash: true}, consumeOutcome: webinvite.ConsumeCreated}
		deps := newRegisterInviteDeps("", creds, invites) // no static secret

		rec := postWebRegisterForm(t, deps, validRegisterForm("inv_live_one", "newcomer"))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("db path status=%d want 303; body=%s", rec.Code, rec.Body.String())
		}
		if invites.consumeCalls != 1 {
			t.Fatalf("db path ConsumeAndCreate calls=%d want 1", invites.consumeCalls)
		}
		if invites.lastConsumeHash != hash {
			t.Errorf("ConsumeAndCreate hash=%q want HashToken(inv_live_one)=%q", invites.lastConsumeHash, hash)
		}
		if invites.lastConsumeUser != "newcomer" {
			t.Errorf("ConsumeAndCreate usedBy=%q want newcomer", invites.lastConsumeUser)
		}
		if _, ok := creds.creds["newcomer"]; ok {
			t.Errorf("db path wrongly used the static UpsertPassword store; creds=%v", creds.creds)
		}
	})

	t.Run("db-second-when-static-mismatch", func(t *testing.T) {
		creds := &fakeRepo{creds: map[string]string{}}
		hash := webinvite.HashToken("inv_live_two")
		invites := &fakeInviteRepo{live: map[string]bool{hash: true}, consumeOutcome: webinvite.ConsumeCreated}
		deps := newRegisterInviteDeps("the-static-secret", creds, invites) // static configured but NOT submitted

		rec := postWebRegisterForm(t, deps, validRegisterForm("inv_live_two", "analyst"))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("db path (static configured) status=%d want 303; body=%s", rec.Code, rec.Body.String())
		}
		if invites.consumeCalls != 1 {
			t.Fatalf("db-second did not run when the submitted token wasn't the static secret (calls=%d)", invites.consumeCalls)
		}
	})

	t.Run("disabled-nil-credentials-store", func(t *testing.T) {
		invites := &fakeInviteRepo{live: map[string]bool{}}
		deps := newRegisterInviteDeps("the-static-secret", nil, invites)
		rec := postWebRegisterForm(t, deps, validRegisterForm("the-static-secret", "x"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("nil creds store status=%d want 401", rec.Code)
		}
	})

	t.Run("disabled-empty-static-and-no-invite-store", func(t *testing.T) {
		creds := &fakeRepo{creds: map[string]string{}}
		deps := newRegisterInviteDeps("", creds, nil) // no static, no invite repo
		rec := postWebRegisterForm(t, deps, validRegisterForm("anything", "x"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("disabled status=%d want 401", rec.Code)
		}
		if _, ok := creds.creds["x"]; ok {
			t.Error("disabled registration created an account")
		}
	})
}

// TestWebRegister_NonEnumerating — every gate failure (DB-invalid, static-wrong
// with and without an invite store, disabled) returns the BYTE-IDENTICAL 401 +
// shared banner + blank-secret re-render (SCN-093-11 / AC-7).
func TestWebRegister_NonEnumerating(t *testing.T) {
	bodyOf := func(t *testing.T, configured string, invites webinvite.Repo, submitted string) string {
		t.Helper()
		creds := &fakeRepo{creds: map[string]string{}}
		deps := newRegisterInviteDeps(configured, creds, invites)
		rec := postWebRegisterForm(t, deps, validRegisterForm(submitted, "should-not-exist"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("gate-reject status=%d want 401; body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !containsBanner(body) {
			t.Fatalf("gate-reject body missing shared banner: %s", body)
		}
		if _, ok := creds.creds["should-not-exist"]; ok {
			t.Error("gate-reject created an account")
		}
		return body
	}

	// (a) DB-invalid: no static; an invite store present but the token is not live.
	dbInvalid := bodyOf(t, "", &fakeInviteRepo{live: map[string]bool{}}, "inv_unknown")
	// (b) static-wrong, invite store present (but token not live there either).
	staticWrongWithStore := bodyOf(t, "the-static-secret", &fakeInviteRepo{live: map[string]bool{}}, "WRONG")
	// (c) static-wrong, NO invite store.
	staticWrongNoStore := bodyOf(t, "the-static-secret", nil, "WRONG")
	// (d) disabled entirely.
	disabled := bodyOf(t, "", nil, "anything")

	for name, body := range map[string]string{
		"static-wrong-with-store": staticWrongWithStore,
		"static-wrong-no-store":   staticWrongNoStore,
		"disabled":                disabled,
	} {
		if body != dbInvalid {
			t.Errorf("non-enumeration broken: %q body differs from the DB-invalid body (response shape leaks the failure mode)", name)
		}
	}
}

// containsBanner reports whether the rendered page carries the shared
// non-enumerating gate banner.
func containsBanner(body string) bool {
	return strings.Contains(body, registerGateBanner)
}

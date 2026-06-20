// Spec 093 SCOPE-03 — Go render/behaviour unit coverage for the admin invites
// handlers (AdminInvitesPage / AdminInviteGenerate / AdminInviteRevoke). These
// drive the handlers via httptest with a controllable webinvite fake and assert
// the rendered HTML: metadata-only list (no token, no hash), the one-time reveal
// (token present exactly once), the value-safe error path (no token echoed),
// the 503-when-nil guard, the PRG revoke + ?notice=race, and CSP-cleanliness
// (no inline <script> / event handlers). The live-stack flow + the real
// webAuthMiddleware anonymous-block are covered by the e2e-ui spec
// (web/pwa/tests/cardrewards_invites.spec.ts, SCN-093-16); the in-package
// TestAdminInvites_AnonymousBlocked below uses a faithful group-gate because
// internal/web imports internal/api, so an in-package api test would
// import-cycle.
package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/auth/webinvite"
)

// fakeInviteRepo is a controllable webinvite.Repo for handler render tests.
type fakeInviteRepo struct {
	rows       []webinvite.InviteRow
	genPlain   string
	genErr     error
	revokeOut  webinvite.RevokeOutcome
	lastRevoke string
}

func (r *fakeInviteRepo) Generate(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return r.genPlain, r.genErr
}
func (r *fakeInviteRepo) IsLive(_ context.Context, _ string) (bool, error) { return false, nil }
func (r *fakeInviteRepo) ConsumeAndCreate(_ context.Context, _, _ string,
	_ func(context.Context, pgx.Tx) error) (webinvite.ConsumeOutcome, error) {
	return webinvite.ConsumeInvalid, nil
}
func (r *fakeInviteRepo) List(_ context.Context) ([]webinvite.InviteRow, error) { return r.rows, nil }
func (r *fakeInviteRepo) Revoke(_ context.Context, id string) (webinvite.RevokeOutcome, error) {
	r.lastRevoke = id
	return r.revokeOut, nil
}

func assertCSPClean(t *testing.T, body string) {
	t.Helper()
	for _, bad := range []string{"<script", "onclick=", "onsubmit=", "onload=", "javascript:"} {
		if strings.Contains(strings.ToLower(body), bad) {
			t.Errorf("CSP-unclean: rendered body contains %q", bad)
		}
	}
}

func TestAdminInvitesPage(t *testing.T) {
	when := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	label := "for the analyst"
	usedBy := "newcomer-x"
	rows := []webinvite.InviteRow{
		{ID: "id-out", Label: &label, CreatedBy: "operator", CreatedAt: when, Status: webinvite.StatusOutstanding},
		{ID: "id-used", CreatedBy: "operator", CreatedAt: when, UsedAt: &when, UsedBy: &usedBy, Status: webinvite.StatusUsed},
		{ID: "id-rev", CreatedBy: "operator", CreatedAt: when, RevokedAt: &when, Status: webinvite.StatusRevoked},
	}

	t.Run("503-when-nil", func(t *testing.T) {
		h := NewCardRewardsWebHandler(nil) // Invites left nil
		rec := httptest.NewRecorder()
		h.AdminInvitesPage(rec, httptest.NewRequest(http.MethodGet, "/cards/admin/invites", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil invites status=%d want 503", rec.Code)
		}
	})

	t.Run("metadata-only-render", func(t *testing.T) {
		h := NewCardRewardsWebHandler(nil)
		h.SetInvites(&fakeInviteRepo{rows: rows})
		rec := httptest.NewRecorder()
		h.AdminInvitesPage(rec, httptest.NewRequest(http.MethodGet, "/cards/admin/invites", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()

		for _, want := range []string{
			`data-invite-id="id-out"`, `data-invite-status="outstanding"`, "for the analyst", "Outstanding",
			`data-invite-status="used"`, "Used", "by newcomer-x",
			`data-invite-status="revoked"`, "Revoked",
			`href="/cards/admin"`,           // back-to-admin link
			`action="/cards/admin/invites"`, // generate form
			`data-action="generate"`,        // generate hook
		} {
			if !strings.Contains(body, want) {
				t.Errorf("list render missing %q", want)
			}
		}
		// NO token / hash may ever appear in the list view.
		if strings.Contains(body, "inv_") {
			t.Errorf("list view leaked a token (contains inv_): %s", body)
		}
		// Only the OUTSTANDING row carries a revoke form (used/revoked do not).
		if n := strings.Count(body, `data-action="revoke"`); n != 1 {
			t.Errorf("revoke form count=%d want exactly 1 (only the outstanding row)", n)
		}
		assertCSPClean(t, body)
	})

	t.Run("empty-state", func(t *testing.T) {
		h := NewCardRewardsWebHandler(nil)
		h.SetInvites(&fakeInviteRepo{rows: nil})
		rec := httptest.NewRecorder()
		h.AdminInvitesPage(rec, httptest.NewRequest(http.MethodGet, "/cards/admin/invites", nil))
		if !strings.Contains(rec.Body.String(), `data-empty="invites"`) {
			t.Errorf("empty list missing empty-state; body=%s", rec.Body.String())
		}
	})

	t.Run("race-notice", func(t *testing.T) {
		h := NewCardRewardsWebHandler(nil)
		h.SetInvites(&fakeInviteRepo{rows: nil})
		rec := httptest.NewRecorder()
		h.AdminInvitesPage(rec, httptest.NewRequest(http.MethodGet, "/cards/admin/invites?notice=race", nil))
		if !strings.Contains(rec.Body.String(), `data-notice="race"`) {
			t.Errorf("?notice=race did not render the warning banner; body=%s", rec.Body.String())
		}
	})
}

func TestAdminInviteGenerate(t *testing.T) {
	t.Run("200-one-time-reveal", func(t *testing.T) {
		const token = "inv_THE_ONE_TIME_TOKEN_value"
		h := NewCardRewardsWebHandler(nil)
		h.SetInvites(&fakeInviteRepo{genPlain: token})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/cards/admin/invites",
			strings.NewReader(url.Values{"label": {"for the analyst"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.AdminInviteGenerate(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("generate status=%d want 200 (render-once, NOT a redirect); body=%s", rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Errorf("generate must NOT redirect (token must not travel via Location); got %q", loc)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-onetime-token-reveal`) {
			t.Error("missing the one-time reveal callout")
		}
		// The token appears EXACTLY once, in a readonly field.
		if c := strings.Count(body, token); c != 1 {
			t.Errorf("one-time token appears %d times, want exactly 1", c)
		}
		if !strings.Contains(body, `readonly value="`+token+`"`) {
			t.Errorf("token not rendered in a readonly field; body=%s", body)
		}
		if !strings.Contains(body, `data-onetime-token`) {
			t.Error("token field missing data-onetime-token hook")
		}
		assertCSPClean(t, body)
	})

	t.Run("value-safe-error", func(t *testing.T) {
		const token = "inv_SHOULD_NEVER_APPEAR"
		h := NewCardRewardsWebHandler(nil)
		// Generate fails — even though a token string is set on the fake, the
		// error path returns ("", err) so the handler has nothing to echo.
		h.SetInvites(&fakeInviteRepo{genPlain: "", genErr: errors.New("boom")})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/cards/admin/invites",
			strings.NewReader(url.Values{"label": {"x"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.AdminInviteGenerate(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("generate-error status=%d want 500; body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if strings.Contains(body, token) || strings.Contains(body, "inv_") {
			t.Errorf("value-safe violation: error re-render echoed a token; body=%s", body)
		}
		if !strings.Contains(body, inviteGenerateError) {
			t.Errorf("error re-render missing the value-safe banner; body=%s", body)
		}
		assertCSPClean(t, body)
	})

	t.Run("503-when-nil", func(t *testing.T) {
		h := NewCardRewardsWebHandler(nil)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/cards/admin/invites", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.AdminInviteGenerate(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil invites generate status=%d want 503", rec.Code)
		}
	})
}

func TestAdminInviteRevoke(t *testing.T) {
	newRouter := func(fake *fakeInviteRepo) *chi.Mux {
		h := NewCardRewardsWebHandler(nil)
		h.SetInvites(fake)
		r := chi.NewRouter()
		r.Post("/cards/admin/invites/{id}/revoke", h.AdminInviteRevoke)
		return r
	}

	t.Run("done-303-prg", func(t *testing.T) {
		fake := &fakeInviteRepo{revokeOut: webinvite.RevokeDone}
		r := newRouter(fake)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/cards/admin/invites/abc-123/revoke", nil))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("revoke status=%d want 303; body=%s", rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != "/cards/admin/invites" {
			t.Errorf("revoke Location=%q want /cards/admin/invites", loc)
		}
		if fake.lastRevoke != "abc-123" {
			t.Errorf("revoke id=%q want abc-123 (chi URL param)", fake.lastRevoke)
		}
	})

	t.Run("noop-303-with-race-notice", func(t *testing.T) {
		fake := &fakeInviteRepo{revokeOut: webinvite.RevokeNoop}
		r := newRouter(fake)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/cards/admin/invites/stale-id/revoke", nil))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("revoke-noop status=%d want 303", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/cards/admin/invites?notice=race" {
			t.Errorf("revoke-noop Location=%q want /cards/admin/invites?notice=race", loc)
		}
	})

	t.Run("503-when-nil", func(t *testing.T) {
		h := NewCardRewardsWebHandler(nil)
		r := chi.NewRouter()
		r.Post("/cards/admin/invites/{id}/revoke", h.AdminInviteRevoke)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/cards/admin/invites/x/revoke", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil invites revoke status=%d want 503", rec.Code)
		}
	})
}

// webAuthLike mirrors internal/api.webAuthMiddleware's contract (reject when a
// token is configured and the request carries neither a matching Bearer header
// nor a matching auth_token cookie). The REAL webAuthMiddleware — which
// internal/api/router.go wraps CardRewardsWebHandler.RegisterRoutes with — is
// the SAME contract and is exercised end-to-end by the e2e-ui anonymous-blocked
// scenario against the live stack (cardrewards_invites.spec.ts).
func webAuthLike(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			if c, err := r.Cookie("auth_token"); err == nil && c.Value == token {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}

// TestAdminInvites_AnonymousBlocked proves the three invite routes are
// registered INSIDE RegisterRoutes such that the group's auth middleware gates
// all of them: an anonymous GET/POST is 401; a cookie-authenticated request
// passes the gate and reaches the handler (503 on nil Invites, NOT 401). This
// catches a mis-registration that would mount the routes outside the gated
// group. The REAL webAuthMiddleware is verified live by the e2e-ui spec.
func TestAdminInvites_AnonymousBlocked(t *testing.T) {
	const token = "operator-secret-token"
	h := NewCardRewardsWebHandler(nil) // Invites nil; the middleware fires first
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(webAuthLike(token))
		h.RegisterRoutes(r)
	})

	cases := []struct{ name, method, path string }{
		{"get-list", http.MethodGet, "/cards/admin/invites"},
		{"post-generate", http.MethodPost, "/cards/admin/invites"},
		{"post-revoke", http.MethodPost, "/cards/admin/invites/some-id/revoke"},
	}
	for _, tc := range cases {
		t.Run("anonymous-"+tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous %s %s status=%d want 401 (group auth gate)", tc.method, tc.path, rec.Code)
			}
		})
	}

	t.Run("authenticated-reaches-handler", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/cards/admin/invites", nil)
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusUnauthorized {
			t.Fatal("authenticated request was wrongly blocked by the group gate")
		}
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("authenticated status=%d want 503 (reached handler, nil Invites)", rec.Code)
		}
	})
}

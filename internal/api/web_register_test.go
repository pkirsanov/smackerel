// Spec 091 SCOPE-03 — unit coverage for HandleWebRegister (POST /v1/web/register).
//
// Drives the handler directly via httptest with the established in-memory
// fakeRepo double (the spec-070 pattern in web_login_credential_test.go) plus
// real argon2id semantics — no internal mocks. Proves the security-critical
// invariants: invite-token gate FIRST (constant-time), shared non-enumerating
// 401 for wrong/missing/empty-configured/nil-store, no overwrite on duplicate,
// exact field-validation strings, 303 -> /login?registered=1 with NO cookie on
// success, and value-safe logging.
package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/auth/webcreds"
)

func newRegisterDeps(inviteToken string, repo webcreds.Repo) *Dependencies {
	return &Dependencies{
		Environment:                "development",
		AuthToken:                  "shared-token-unused-by-register",
		WebCredentials:             repo,
		WebRegistrationInviteToken: inviteToken,
	}
}

func postWebRegisterForm(t *testing.T, deps *Dependencies, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	body := form.Encode()
	req := httptest.NewRequest(http.MethodPost, "/v1/web/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	deps.HandleWebRegister(rec, req)
	return rec
}

// TestWebRegister_Success — valid token + new user + matching ≥12-char
// passwords → 303 to /login?registered=1 (carrying the sanitised next), NO
// auth_token cookie, a row created. A second distinct user also succeeds,
// proving the invite token is repeatable (not consumed). UC-1 / AC-2 / AC-8.
func TestWebRegister_Success(t *testing.T) {
	repo := &fakeRepo{creds: map[string]string{}}
	deps := newRegisterDeps("the-operator-invite", repo)

	rec := postWebRegisterForm(t, deps, url.Values{
		"invite-token":     {"the-operator-invite"},
		"username":         {"operator2"},
		"password":         {"correct-horse-battery"},
		"confirm-password": {"correct-horse-battery"},
		"next":             {"/cards"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?registered=1") {
		t.Errorf("Location=%q want prefix /login?registered=1", loc)
	}
	if !strings.Contains(loc, "next=%2Fcards") {
		t.Errorf("Location=%q must carry the sanitised next=/cards (url-escaped)", loc)
	}
	if cookieByName(rec, "auth_token") != nil {
		t.Error("register MUST NOT set the auth_token cookie (no session minted on register)")
	}
	if _, ok := repo.creds["operator2"]; !ok {
		t.Errorf("operator2 row was not created; repo=%v", repo.creds)
	}

	// Repeatable: a SECOND distinct user also succeeds (token not consumed).
	rec2 := postWebRegisterForm(t, deps, url.Values{
		"invite-token":     {"the-operator-invite"},
		"username":         {"operator3"},
		"password":         {"another-good-password"},
		"confirm-password": {"another-good-password"},
	})
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("second registration status=%d want 303 (invite token must remain valid); body=%s", rec2.Code, rec2.Body.String())
	}
	if _, ok := repo.creds["operator3"]; !ok {
		t.Errorf("operator3 row was not created on the second registration; repo=%v", repo.creds)
	}
}

// TestWebRegister_Success_HostileNextDropped — a SUCCESSFUL registration that
// carries a hostile ?next (open-redirect probe) still creates the account, but
// the 303 Location is the bare /login?registered=1 with the hostile next
// DROPPED. The success branch runs the supplied next through sanitizeNext and
// only appends &next= when the result is a safe in-origin path; a hostile
// "//evil/" sanitises to "/" (== sanitizeNextDefault) and is therefore NOT
// appended.
//
// This adversarially LOCKS the POST-success open-redirect defence. The
// happy-path TestWebRegister_Success above uses a SAFE next (/cards) that a
// non-sanitising (vulnerable) success branch would carry byte-identically, so
// it cannot distinguish sanitizeNext(nextRaw) from a raw passthrough — only a
// HOSTILE next exposes that gap. It is the POST-redirect twin of the GET-page
// lock TestRegisterPage_NextSanitized, closing the asymmetry where the GET
// hidden-field sanitisation was tested but the POST success redirect was not.
// (Verified non-tautological via mutate-prove-revert: replacing the success
// branch's sanitizeNext(nextRaw) with a raw `if nextRaw != "" { … nextRaw }`
// passthrough turns this RED while TestWebRegister_Success stays GREEN.)
// AC-10 value-safe / spec-057 open-redirect protection reused at POST time.
func TestWebRegister_Success_HostileNextDropped(t *testing.T) {
	repo := &fakeRepo{creds: map[string]string{}}
	deps := newRegisterDeps("the-operator-invite", repo)

	rec := postWebRegisterForm(t, deps, url.Values{
		"invite-token":     {"the-operator-invite"},
		"username":         {"operator9"},
		"password":         {"correct-horse-battery"},
		"confirm-password": {"correct-horse-battery"},
		"next":             {"//evil.example.com/"},
	})

	// Registration still SUCCEEDS — the hostile next neutralises the redirect
	// target, it does not reject account creation.
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303; body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := repo.creds["operator9"]; !ok {
		t.Errorf("operator9 row was not created; repo=%v", repo.creds)
	}

	loc := rec.Header().Get("Location")
	// The hostile next is DROPPED: Location is the bare success destination
	// with no &next= appended (sanitizeNext("//evil.example.com/") == "/").
	if loc != registerRedirectPath {
		t.Errorf("Location=%q want exactly %q (hostile next must be dropped, not carried)", loc, registerRedirectPath)
	}
	if strings.Contains(loc, "evil.example.com") {
		t.Errorf("OPEN REDIRECT: hostile next leaked into the 303 Location: %q", loc)
	}
	if strings.Contains(loc, "next=") {
		t.Errorf("hostile next was carried as a next= param (must be dropped): %q", loc)
	}
	if cookieByName(rec, "auth_token") != nil {
		t.Error("register MUST NOT set the auth_token cookie")
	}
}

// TestWebRegister_Gate — the invite-token gate (tokenless). Wrong token,
// missing token, empty-configured token, and a nil store ALL return 401 with
// the shared banner, create NO row, and do not panic. UC-2 / UC-3 / AC-4 / AC-5.
func TestWebRegister_Gate(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		nilStore   bool
		form       url.Values
	}{
		{
			name:       "wrong-token",
			configured: "the-real-invite",
			form: url.Values{
				"invite-token":     {"WRONG"},
				"username":         {"should-not-exist"},
				"password":         {"a-valid-password-12"},
				"confirm-password": {"a-valid-password-12"},
			},
		},
		{
			name:       "missing-token",
			configured: "the-real-invite",
			form: url.Values{
				// invite-token field omitted entirely
				"username":         {"should-not-exist"},
				"password":         {"a-valid-password-12"},
				"confirm-password": {"a-valid-password-12"},
			},
		},
		{
			name:       "empty-configured",
			configured: "", // registration disabled
			form: url.Values{
				"invite-token":     {"anything-the-attacker-types"},
				"username":         {"should-not-exist"},
				"password":         {"a-valid-password-12"},
				"confirm-password": {"a-valid-password-12"},
			},
		},
		{
			name:       "empty-configured-empty-submitted",
			configured: "", // the open-signup trap: empty == empty must NOT match
			form: url.Values{
				"invite-token":     {""},
				"username":         {"should-not-exist"},
				"password":         {"a-valid-password-12"},
				"confirm-password": {"a-valid-password-12"},
			},
		},
		{
			name:       "nil-store",
			configured: "the-real-invite",
			nilStore:   true,
			form: url.Values{
				"invite-token":     {"the-real-invite"}, // even the correct token is refused when the store is nil
				"username":         {"should-not-exist"},
				"password":         {"a-valid-password-12"},
				"confirm-password": {"a-valid-password-12"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var repo *fakeRepo
			var deps *Dependencies
			if tc.nilStore {
				deps = newRegisterDeps(tc.configured, nil) // untyped nil interface
			} else {
				repo = &fakeRepo{creds: map[string]string{}}
				deps = newRegisterDeps(tc.configured, repo)
			}

			rec := postWebRegisterForm(t, deps, tc.form)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want 401; body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), registerGateBanner) {
				t.Errorf("missing shared gate banner %q; body=%s", registerGateBanner, rec.Body.String())
			}
			if cookieByName(rec, "auth_token") != nil {
				t.Error("gate reject MUST NOT set a cookie")
			}
			if repo != nil {
				if _, ok := repo.creds["should-not-exist"]; ok {
					t.Errorf("a row was created on a gate-rejected request; repo=%v", repo.creds)
				}
			}
		})
	}
}

// TestWebRegister_Duplicate — a duplicate username (valid token, valid fields)
// returns 409 "That username is taken." and the existing hash is UNCHANGED
// (create=true no-overwrite). UC-4 / AC-3.
func TestWebRegister_Duplicate(t *testing.T) {
	const existing = "EXISTING-HASH-SENTINEL"
	repo := &fakeRepo{creds: map[string]string{"operator": existing}}
	deps := newRegisterDeps("the-invite", repo)

	rec := postWebRegisterForm(t, deps, url.Values{
		"invite-token":     {"the-invite"},
		"username":         {"operator"},
		"password":         {"a-brand-new-password"},
		"confirm-password": {"a-brand-new-password"},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), registerDuplicateUserMsg) {
		t.Errorf("missing duplicate banner %q; body=%s", registerDuplicateUserMsg, rec.Body.String())
	}
	if repo.creds["operator"] != existing {
		t.Errorf("existing hash was OVERWRITTEN: got %q want %q (create=true must not overwrite)", repo.creds["operator"], existing)
	}
	if cookieByName(rec, "auth_token") != nil {
		t.Error("duplicate reject MUST NOT set a cookie")
	}
}

// TestWebRegister_FieldValidation — with a VALID token, each field violation
// returns the exact catalog string + 400 and creates no row. UC-5 / AC-6.
func TestWebRegister_FieldValidation(t *testing.T) {
	cases := []struct {
		name    string
		form    url.Values
		wantMsg string
	}{
		{
			name: "password-mismatch",
			form: url.Values{
				"invite-token":     {"the-invite"},
				"username":         {"newuser"},
				"password":         {"correct-horse-battery"},
				"confirm-password": {"wrong-horse-battery-x"},
			},
			wantMsg: registerPasswordMismatch,
		},
		{
			name: "password-too-short",
			form: url.Values{
				"invite-token":     {"the-invite"},
				"username":         {"newuser"},
				"password":         {"short"},
				"confirm-password": {"short"},
			},
			wantMsg: registerPasswordTooShort,
		},
		{
			name: "missing-username",
			form: url.Values{
				"invite-token":     {"the-invite"},
				"password":         {"a-valid-password-12"},
				"confirm-password": {"a-valid-password-12"},
			},
			wantMsg: registerMissingFieldMsg,
		},
		{
			name: "missing-password",
			form: url.Values{
				"invite-token":     {"the-invite"},
				"username":         {"newuser"},
				"confirm-password": {"a-valid-password-12"},
			},
			wantMsg: registerMissingFieldMsg,
		},
		{
			name: "invalid-username-too-long",
			form: url.Values{
				"invite-token":     {"the-invite"},
				"username":         {strings.Repeat("a", 65)}, // > MaxUsernameLength (64)
				"password":         {"a-valid-password-12"},
				"confirm-password": {"a-valid-password-12"},
			},
			wantMsg: registerInvalidUsernameMsg,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{creds: map[string]string{}}
			deps := newRegisterDeps("the-invite", repo)
			rec := postWebRegisterForm(t, deps, tc.form)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want 400; body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantMsg) {
				t.Errorf("missing expected banner %q; body=%s", tc.wantMsg, rec.Body.String())
			}
			if len(repo.creds) != 0 {
				t.Errorf("a row was created on a field-rejected request; repo=%v", repo.creds)
			}
			if cookieByName(rec, "auth_token") != nil {
				t.Error("field reject MUST NOT set a cookie")
			}
		})
	}
}

// TestWebRegister_NonEnumeration — a wrong-token request and a disabled-gate
// (empty-configured) request submitting the SAME form produce byte-identical
// responses (status + body); the invite token never appears in the body,
// headers, or Location; and the gate reject does NOT echo the submitted
// username. AC-10 / UC-2.
func TestWebRegister_NonEnumeration(t *testing.T) {
	form := url.Values{
		"invite-token":     {"WRONG-INVITE-VALUE"},
		"username":         {"probe-user"},
		"password":         {"some-password-here"},
		"confirm-password": {"some-password-here"},
		"next":             {"/cards"},
	}

	repoWrong := &fakeRepo{creds: map[string]string{}}
	recWrong := postWebRegisterForm(t, newRegisterDeps("the-real-invite", repoWrong), form)

	repoDisabled := &fakeRepo{creds: map[string]string{}}
	recDisabled := postWebRegisterForm(t, newRegisterDeps("", repoDisabled), form) // empty configured = disabled

	if recWrong.Code != http.StatusUnauthorized || recDisabled.Code != http.StatusUnauthorized {
		t.Fatalf("status wrong=%d disabled=%d want both 401", recWrong.Code, recDisabled.Code)
	}
	if recWrong.Body.String() != recDisabled.Body.String() {
		t.Errorf("ENUMERATION LEAK: wrong-token and disabled-gate bodies differ\n--- wrong ---\n%s\n--- disabled ---\n%s",
			recWrong.Body.String(), recDisabled.Body.String())
	}

	body := recWrong.Body.String()
	for _, secret := range []string{"WRONG-INVITE-VALUE", "the-real-invite"} {
		if strings.Contains(body, secret) {
			t.Errorf("invite token %q leaked into the response body: %s", secret, body)
		}
		if strings.Contains(recWrong.Header().Get("Location"), secret) {
			t.Errorf("invite token %q leaked into Location header: %s", secret, recWrong.Header().Get("Location"))
		}
	}
	// The shared gate reject re-renders with a BLANK username (so wrong-token
	// and disabled are byte-identical) — the submitted username must be absent.
	if strings.Contains(body, "probe-user") {
		t.Errorf("gate reject echoed the submitted username (breaks byte-identical non-enumeration): %s", body)
	}
	if len(repoWrong.creds) != 0 || len(repoDisabled.creds) != 0 {
		t.Errorf("a row was created on a gate-rejected request; wrong=%v disabled=%v", repoWrong.creds, repoDisabled.creds)
	}
}

// TestWebRegister_ValueSafeLog — the structured reject log line records only
// remote_addr, username_len, and a coarse reason enum; it MUST NOT contain the
// invite-token value, the configured secret, the username value, or any
// password. AC-10.
func TestWebRegister_ValueSafeLog(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(prev)

	const (
		inviteCanary = "WRONG-INVITE-LEAKCANARY"
		configCanary = "CONFIGURED-INVITE-LEAKCANARY"
		userCanary   = "USERNAMELEAKCANARY"
		pwCanary     = "PASSWORDLEAKCANARY1234"
	)
	repo := &fakeRepo{creds: map[string]string{}}
	deps := newRegisterDeps(configCanary, repo)
	rec := postWebRegisterForm(t, deps, url.Values{
		"invite-token":     {inviteCanary},
		"username":         {userCanary},
		"password":         {pwCanary},
		"confirm-password": {pwCanary},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401; body=%s", rec.Code, rec.Body.String())
	}

	logOut := buf.String()
	for _, secret := range []string{inviteCanary, configCanary, userCanary, pwCanary} {
		if strings.Contains(logOut, secret) {
			t.Errorf("value-safe log LEAKED %q:\n%s", secret, logOut)
		}
	}
	// A reject log line WAS emitted with the value-safe structured fields.
	for _, field := range []string{"web_register_fail", "reason", "gate", "username_len"} {
		if !strings.Contains(logOut, field) {
			t.Errorf("reject log missing structured field %q:\n%s", field, logOut)
		}
	}
}

// TestWebRegister_MethodGuard — a non-POST (GET) request returns 405.
func TestWebRegister_MethodGuard(t *testing.T) {
	deps := newRegisterDeps("the-invite", &fakeRepo{creds: map[string]string{}})
	req := httptest.NewRequest(http.MethodGet, "/v1/web/register", nil)
	rec := httptest.NewRecorder()
	deps.HandleWebRegister(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status=%d want 405", rec.Code)
	}
}

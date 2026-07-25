package graphapi

// activation_test.go — real unit tests for the BUG-080-001 fail-soft
// runtime-disable core. These are hermetic (t.Setenv + httptest only):
// no datastore, no router, no live stack. They prove the operator's
// fail-soft contract:
//
//   - empty OR missing cursor secret  -> typed 503 capability_disabled
//     (NOT a silent 404, NOT an opaque 500, NOT a panic, NOT absent);
//   - present cursor secret           -> the operating path runs and its
//     typed errors flow through unchanged;
//   - single global-corpus grant matrix: operator and grant-holder are
//     authorized against the SAME global rows; an ungranted identity gets
//     a leak-free 403; no per-user/tenant row isolation is introduced.
//
// The adversarial test at the bottom fails if the code ever reverts to
// the original silent-absence (404) / opaque-500 behavior on an empty
// secret. `containsCaseInsensitive` is shared from errors_test.go.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	// unitSecretEnv is a hermetic env-var name used only by these
	// unit tests; it is never a real deployment secret name.
	unitSecretEnv = "KNOWLEDGE_GRAPH_API_UNIT_SECRET"
	// unitUnsetEnv is a name that these tests deliberately never set,
	// so os.LookupEnv returns ok=false (the "missing" branch).
	unitUnsetEnv = "KNOWLEDGE_GRAPH_API_UNIT_UNSET_ENV_DO_NOT_SET"
)

// operatingProbe is a sentinel "operating path" handler. It records
// whether it was invoked and writes a 200 marker, so a test can prove
// the Guard delegated (ENABLED) or short-circuited (DISABLED).
type operatingProbe struct{ called bool }

func (p *operatingProbe) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	p.called = true
	writeJSON(w, http.StatusOK, map[string]string{"operating": "true"})
}

// getThroughGuard drives one GET request for a known graph path through
// the capability Guard wrapping the supplied operating handler.
func getThroughGuard(t *testing.T, cap *GraphCapability, operating http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/topics/", nil)
	cap.Guard(operating).ServeHTTP(rec, req)
	return rec
}

// TestResolveActivation_EmptySecretIsTypedDisabled — SCN-080-001 fail-soft
// core: an EMPTY cursor secret resolves to the typed disabled state and
// answers every graph path with 503 capability_disabled (never 404/500).
func TestResolveActivation_EmptySecretIsTypedDisabled(t *testing.T) {
	t.Setenv(unitSecretEnv, "")
	cfg := Config{CursorSecretEnv: unitSecretEnv}

	act := ResolveActivation(cfg)
	if act.State != ActivationDisabled {
		t.Fatalf("State = %q; want %q", act.State, ActivationDisabled)
	}
	if act.SecretPresence != SecretEmpty {
		t.Errorf("SecretPresence = %q; want %q", act.SecretPresence, SecretEmpty)
	}
	if act.Code != CodeCursorSecretEmpty {
		t.Errorf("Code = %q; want %q", act.Code, CodeCursorSecretEmpty)
	}
	if !act.Disabled() {
		t.Error("Disabled() = false; want true for an empty secret")
	}

	probe := &operatingProbe{}
	rec := getThroughGuard(t, NewGraphCapability(cfg), probe)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503 (capability_disabled)", rec.Code)
	}
	if rec.Code == http.StatusNotFound {
		t.Fatal("status = 404: fail-soft reverted to the silent-absence bug")
	}
	if probe.called {
		t.Error("operating handler was invoked while capability is disabled")
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("disabled body is not a typed envelope: %v (body=%q)", err, rec.Body.String())
	}
	if env.Error.Code != CodeCapabilityDisabled {
		t.Errorf("code = %q; want %q", env.Error.Code, CodeCapabilityDisabled)
	}
	if env.Error.Message == "" {
		t.Error("disabled envelope carries no message; the state must be honest, not silent")
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Error("disabled response has no Content-Type; want application/json")
	}
}

// TestResolveActivation_MissingSecretIsTypedDisabled — a MISSING secret
// (either no env name configured, or the named env var unset) resolves to
// the same typed disabled state. Both branches must fail soft, not panic.
func TestResolveActivation_MissingSecretIsTypedDisabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "no env name configured", cfg: Config{CursorSecretEnv: ""}},
		{name: "named env var is unset", cfg: Config{CursorSecretEnv: unitUnsetEnv}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			act := ResolveActivation(tc.cfg)
			if act.State != ActivationDisabled {
				t.Fatalf("State = %q; want %q", act.State, ActivationDisabled)
			}
			if act.SecretPresence != SecretMissing {
				t.Errorf("SecretPresence = %q; want %q", act.SecretPresence, SecretMissing)
			}
			if act.Code != CodeCursorSecretMissing {
				t.Errorf("Code = %q; want %q", act.Code, CodeCursorSecretMissing)
			}

			probe := &operatingProbe{}
			rec := getThroughGuard(t, NewGraphCapability(tc.cfg), probe)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d; want 503 capability_disabled", rec.Code)
			}
			if probe.called {
				t.Error("operating handler invoked while capability disabled")
			}
			var env ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("body not typed envelope: %v", err)
			}
			if env.Error.Code != CodeCapabilityDisabled {
				t.Errorf("code = %q; want %q", env.Error.Code, CodeCapabilityDisabled)
			}
		})
	}
}

// TestResolveActivation_PresentSecretOperates — a PRESENT secret enables
// the capability and the Guard delegates to the operating path.
func TestResolveActivation_PresentSecretOperates(t *testing.T) {
	t.Setenv(unitSecretEnv, "unit-secret-value")
	cfg := Config{CursorSecretEnv: unitSecretEnv}

	act := ResolveActivation(cfg)
	if act.State != ActivationEnabled {
		t.Fatalf("State = %q; want %q", act.State, ActivationEnabled)
	}
	if act.SecretPresence != SecretPresent {
		t.Errorf("SecretPresence = %q; want %q", act.SecretPresence, SecretPresent)
	}
	if act.Code != CodeActivationOK {
		t.Errorf("Code = %q; want %q", act.Code, CodeActivationOK)
	}
	if act.Disabled() {
		t.Error("Disabled() = true; want false for a present secret")
	}

	probe := &operatingProbe{}
	rec := getThroughGuard(t, NewGraphCapability(cfg), probe)

	if !probe.called {
		t.Fatal("operating handler was NOT invoked while capability enabled")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 from the operating handler", rec.Code)
	}
	var env ErrorEnvelope
	if json.Unmarshal(rec.Body.Bytes(), &env) == nil && env.Error.Code == CodeCapabilityDisabled {
		t.Error("enabled path emitted capability_disabled; the operating path must run")
	}
}

// TestGuard_OperatingPathTypedErrorsFlowThrough — when enabled, the Guard
// must be transparent: a typed error from the operating handler reaches
// the client unchanged (it is not swallowed or rewritten to disabled).
func TestGuard_OperatingPathTypedErrorsFlowThrough(t *testing.T) {
	t.Setenv(unitSecretEnv, "unit-secret-value")
	cfg := Config{CursorSecretEnv: unitSecretEnv}

	operating := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteAPIError(w, ErrUnauthenticated) // typed 401 on the operating path
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/topics/", nil)
	NewGraphCapability(cfg).Guard(operating).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401 (typed error passed through)", rec.Code)
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Code != CodeUnauthenticated {
		t.Errorf("code = %q; want %q (Guard must not rewrite operating errors)", env.Error.Code, CodeUnauthenticated)
	}
}

// TestGraphReadGrantMatrix_GlobalCorpus — GRAPH-ACT-005 / GRAPH-ACT-011.
// Operator and grant-holder are authorized against the SAME single global
// corpus; an ungranted authenticated identity and an unauthenticated
// caller are denied. The classifier reads only the operator flag and the
// grant set, so no per-user/tenant row predicate can exist.
func TestGraphReadGrantMatrix_GlobalCorpus(t *testing.T) {
	cases := []struct {
		name      string
		id        GraphIdentity
		wantGrant GraphGrant
		wantAllow bool
	}{
		{
			name:      "operator reads all private content",
			id:        GraphIdentity{Authenticated: true, Operator: true, Grants: nil},
			wantGrant: GrantOperator,
			wantAllow: true,
		},
		{
			name:      "grant-holder reads authorized global projection",
			id:        GraphIdentity{Authenticated: true, Operator: false, Grants: []string{GraphReadScope}},
			wantGrant: GrantHolder,
			wantAllow: true,
		},
		{
			name:      "ungranted authenticated identity is denied",
			id:        GraphIdentity{Authenticated: true, Operator: false, Grants: []string{"annotation:edit"}},
			wantGrant: GrantNone,
			wantAllow: false,
		},
		{
			name:      "unauthenticated caller is denied",
			id:        GraphIdentity{Authenticated: false, Operator: false, Grants: []string{GraphReadScope}},
			wantGrant: GrantNone,
			wantAllow: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotGrant := ClassifyGraphGrant(tc.id)
			if gotGrant != tc.wantGrant {
				t.Fatalf("grant = %q; want %q", gotGrant, tc.wantGrant)
			}
			err := AuthorizeGraphRead(gotGrant)
			if tc.wantAllow && err != nil {
				t.Fatalf("AuthorizeGraphRead(%q) = %v; want allowed (nil)", gotGrant, err)
			}
			if !tc.wantAllow {
				if err == nil {
					t.Fatalf("AuthorizeGraphRead(%q) = nil; want leak-free denial", gotGrant)
				}
				if err.Status != http.StatusForbidden || err.Code != CodeForbidden {
					t.Errorf("denial = status %d code %q; want 403 %q", err.Status, err.Code, CodeForbidden)
				}
			}
		})
	}

	// No row isolation: two DISTINCT grant-holders with the same grant
	// classify identically. GraphIdentity carries no user/tenant id, so
	// the projection differs by grant, never by a per-identity partition.
	a := GraphIdentity{Authenticated: true, Grants: []string{GraphReadScope}}
	b := GraphIdentity{Authenticated: true, Grants: []string{GraphReadScope}}
	if ClassifyGraphGrant(a) != ClassifyGraphGrant(b) {
		t.Error("two grant-holders classified differently; a per-identity row predicate leaked in")
	}
}

// TestUngrantedDenialIsLeakFree — the ungranted denial is the generic
// 403 forbidden with no graph content, counts, or existence hints.
func TestUngrantedDenialIsLeakFree(t *testing.T) {
	err := AuthorizeGraphRead(GrantNone)
	if err == nil {
		t.Fatal("AuthorizeGraphRead(GrantNone) = nil; want denial")
	}
	if err != ErrMissingScope {
		t.Errorf("denial = %v; want the canonical ErrMissingScope", err)
	}
	rec := httptest.NewRecorder()
	WriteAPIError(rec, err)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403", rec.Code)
	}

	// Leak-free proof: the denial body is byte-identical to the canonical
	// static forbidden envelope, so NO dynamic graph content (labels, ids,
	// node/edge data, counts, source titles) can have been interpolated.
	// Naming the required knowledge-graph:read scope is the same generic
	// message auth.RequireScope already returns and is not graph content.
	// (A naive substring scan is wrong here: "edge" is a substring of the
	// scope name "knowledge-graph:read".)
	wantBody, mErr := json.Marshal(ErrorEnvelope{
		Error: ErrorBody{Code: CodeForbidden, Message: ErrMissingScope.Message},
	})
	if mErr != nil {
		t.Fatalf("marshal expected envelope: %v", mErr)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != string(wantBody) {
		t.Errorf("denial body = %q; want the static canonical envelope %q (dynamic content would indicate a leak)", got, wantBody)
	}
	// No count leak: the static denial carries no digits, so any injected
	// aggregate count would trip this.
	if strings.ContainsAny(rec.Body.String(), "0123456789") {
		t.Errorf("403 denial contains digits (possible count leak): %q", rec.Body.String())
	}
}

// TestActivationDiagnosticsNeverLeakSecret — GRAPH-ACT-002 / SCN-080-001-07
// at unit level: the value-safe Activation outcome and the disabled
// response never contain the secret bytes.
func TestActivationDiagnosticsNeverLeakSecret(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-DO-NOT-LEAK-9f3a"
	t.Setenv(unitSecretEnv, sentinel)
	cfg := Config{CursorSecretEnv: unitSecretEnv}

	act := ResolveActivation(cfg)
	for _, field := range []string{string(act.State), string(act.SecretPresence), act.Code} {
		if containsCaseInsensitive(field, sentinel) {
			t.Errorf("Activation field %q leaked the secret value", field)
		}
	}

	// The disabled response is a constant envelope; prove it holds no
	// secret material even when the env happens to carry the sentinel.
	rec := httptest.NewRecorder()
	NewGraphCapability(Config{CursorSecretEnv: ""}).WriteDisabled(rec)
	if containsCaseInsensitive(rec.Body.String(), sentinel) {
		t.Errorf("disabled response leaked secret material: %q", rec.Body.String())
	}
}

// TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500 — the
// anti-fabrication adversarial. It FAILS if the fail-soft core ever
// reverts to the ORIGINAL bug (empty secret -> nil handler -> Chi 404
// silent absence), or degrades to an opaque 500, or panics. The contrast
// leg drives the SAME path through an unguarded mux (the old nil-handler
// router shape) to prove the two outcomes are genuinely distinct.
func TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500(t *testing.T) {
	t.Setenv(unitSecretEnv, "")
	cfg := Config{CursorSecretEnv: unitSecretEnv}

	// RED shape: the pre-fix router omitted the route for a nil handler,
	// so a bare mux 404s the exact same path.
	bareMux := http.NewServeMux()
	redRec := httptest.NewRecorder()
	bareMux.ServeHTTP(redRec, httptest.NewRequest(http.MethodGet, "/api/topics/", nil))
	if redRec.Code != http.StatusNotFound {
		t.Fatalf("contrast precondition: unguarded mux status = %d; want 404 (the silent-absence bug)", redRec.Code)
	}

	// GREEN: the fail-soft capability answers the same path with a typed
	// 503 capability_disabled instead of the silent 404.
	probe := &operatingProbe{}
	rec := getThroughGuard(t, NewGraphCapability(cfg), probe)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503 capability_disabled", rec.Code)
	}
	if rec.Code == http.StatusNotFound {
		t.Fatal("REGRESSION: empty secret reverted to a silent 404 absence")
	}
	if rec.Code == http.StatusInternalServerError {
		t.Fatal("REGRESSION: empty secret degraded to an opaque 500")
	}
	if rec.Code == http.StatusOK {
		t.Fatal("REGRESSION: empty secret served the operating path as if enabled")
	}
	if probe.called {
		t.Fatal("REGRESSION: operating handler ran under an empty secret")
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("disabled body is not a typed envelope: %v", err)
	}
	if env.Error.Code != CodeCapabilityDisabled {
		t.Fatalf("code = %q; want %q — the disabled state must be typed and honest", env.Error.Code, CodeCapabilityDisabled)
	}
	if redRec.Code == rec.Code {
		t.Fatal("fail-soft 503 is indistinguishable from the silent-absence 404")
	}
}

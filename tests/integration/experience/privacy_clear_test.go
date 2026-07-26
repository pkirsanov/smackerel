//go:build integration

// Spec 106 SCOPE-106-03 — XP106-03-P (shared-infrastructure canary).
//
// TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail
// is the shared-state PRIVACY-CLEAR + 403-DENIAL + REDACTION canary for the
// spec-106 shared presentation primitives. It exercises the REAL
// experience.AuthenticatedRequestAdapter and the REAL state/mutation presenters
// with NO mock, NO stub, and NO interception, and proves the shared-infra
// contract that protects every high-fan-out consumer:
//
//   - a unified 401 (session ended) synchronously CLEARS the exact closed set of
//     five protected presentation targets, retains NO session, and offers a safe
//     re-authentication path;
//   - a 403 (access denied) RETAINS the valid session, clears nothing, shows the
//     access-denied state, and never loops through login;
//   - the emitted presentation directive — the single SOURCE every renderer,
//     accessibility tree, log, metric, trace, and storage surface is derived
//     from — carries NO sensitive detail: every emitted string is drawn ONLY
//     from the closed presentation vocabularies, so a stack frame, secret, token,
//     query, URL, or PII value structurally CANNOT ride along;
//   - a redaction-violating owner outcome on any presenter fails CLOSED and
//     leaks no value on the failure path.
//
// This is the shared-infrastructure PRIMITIVE canary (pure Go, integration-tag,
// same convention as shell_rollback_test.go in this package). The presenters are
// not yet wired into the live server/PWA routes — that cutover is SCOPE-106-04/05
// — so the LIVE cross-renderer surface propagation (real PWA 401 clear, real
// HTMX / Card-PRG mutation canaries against the live DOM) is intentionally
// coupled forward. What this canary proves is the shared SOURCE directive is
// redaction-clean and privacy-clearing by construction, before any consumer
// adopts it.
package integrationexperience

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/experience"
)

func TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail(t *testing.T) {
	auth := experience.AuthenticatedRequestAdapter{}
	sp := experience.ExperienceStatePresenter{}
	mp := experience.MutationFeedbackPresenter{}

	// sensitive is a saturation set of shapes that MUST NEVER ride a presented
	// value: a stack frame, a file:line, a secret, a bearer token, a PII email,
	// a SQL query, and a reset URL with a token.
	sensitive := []string{
		"panic: runtime error: index out of range",
		"/app/internal/web/handler.go:412",
		"AGE-SECRET-KEY-1EXAMPLEONLY",
		"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6",
		"person@example.com",
		"SELECT token FROM sessions WHERE user_id=",
		"https://host.example/reset?token=abc123",
	}
	assertNoSensitive := func(t *testing.T, label, blob string) {
		t.Helper()
		for _, s := range sensitive {
			if strings.Contains(blob, s) {
				t.Fatalf("%s leaked sensitive detail %q in %q", label, s, blob)
			}
		}
	}
	// closedVocabulary is every legitimate string an AuthPresentation may emit:
	// the closed ViewState values and the closed protected-target values.
	closedVocabulary := map[string]bool{
		"": true,
		string(experience.ViewUnauthorized): true, string(experience.ViewAccessDenied): true,
		string(experience.TargetProtectedDOM): true, string(experience.TargetAccessibleLabels): true,
		string(experience.TargetInMemoryBusinessState): true, string(experience.TargetPendingWork): true,
		string(experience.TargetGraphPixels): true,
	}

	// ── 401 privacy clear ─────────────────────────────────────────────────────
	t.Run("401_session_loss_clears_all_protected_presentation_and_retains_no_session", func(t *testing.T) {
		p, err := auth.Present(experience.AuthSessionEnded)
		if err != nil {
			t.Fatalf("Present(session_ended): %v", err)
		}
		if p.ViewState != experience.ViewUnauthorized {
			t.Fatalf("401 ViewState = %q; want unauthorized", p.ViewState)
		}
		if p.RetainSession {
			t.Fatal("401 retained a session; a session loss must retain NOTHING")
		}
		if !p.OffersSafeReauthentication {
			t.Fatal("401 did not offer a safe re-authentication path")
		}
		wantCleared := map[experience.ProtectedPresentationTarget]bool{
			experience.TargetProtectedDOM: false, experience.TargetAccessibleLabels: false,
			experience.TargetInMemoryBusinessState: false, experience.TargetPendingWork: false,
			experience.TargetGraphPixels: false,
		}
		for _, got := range p.ClearedTargets {
			if _, ok := wantCleared[got]; !ok {
				t.Fatalf("401 cleared an unexpected target %q", got)
			}
			wantCleared[got] = true
		}
		for target, seen := range wantCleared {
			if !seen {
				t.Fatalf("401 did NOT clear protected target %q (privacy-clear must be total)", target)
			}
		}
	})

	// ── 403 denial retains session, no login loop ─────────────────────────────
	t.Run("403_access_denial_retains_session_and_never_loops_login", func(t *testing.T) {
		p, err := auth.Present(experience.AuthAccessDenied)
		if err != nil {
			t.Fatalf("Present(access_denied): %v", err)
		}
		if p.ViewState != experience.ViewAccessDenied {
			t.Fatalf("403 ViewState = %q; want access_denied", p.ViewState)
		}
		if !p.RetainSession {
			t.Fatal("403 dropped the valid session; access-denial must RETAIN it")
		}
		if len(p.ClearedTargets) != 0 {
			t.Fatalf("403 cleared %d protected targets; a valid-session denial must clear NONE", len(p.ClearedTargets))
		}
		if p.OffersSafeReauthentication {
			t.Fatal("403 offered re-authentication; a valid-session denial must NOT loop through login")
		}
	})

	// ── Redaction: the emitted directive carries only closed-vocabulary values ─
	t.Run("emitted_auth_presentation_exposes_only_closed_vocabulary_no_sensitive_detail", func(t *testing.T) {
		for _, o := range []experience.AuthOutcome{experience.AuthAuthorized, experience.AuthSessionEnded, experience.AuthAccessDenied} {
			p, err := auth.Present(o)
			if err != nil {
				t.Fatalf("Present(%q): %v", o, err)
			}
			blob, err := json.Marshal(p)
			if err != nil {
				t.Fatalf("marshal presentation(%q): %v", o, err)
			}
			assertNoSensitive(t, fmt.Sprintf("auth presentation(%q)", o), string(blob))

			// Every string VALUE the directive emits must be a member of the
			// closed vocabulary — so no free-form detail can ever appear.
			var generic map[string]any
			if err := json.Unmarshal(blob, &generic); err != nil {
				t.Fatalf("unmarshal presentation(%q): %v", o, err)
			}
			var check func(v any)
			check = func(v any) {
				switch x := v.(type) {
				case string:
					if !closedVocabulary[x] {
						t.Fatalf("auth presentation(%q) emitted a non-vocabulary string %q (possible free-form detail)", o, x)
					}
				case []any:
					for _, e := range x {
						check(e)
					}
				case map[string]any:
					for _, e := range x {
						check(e)
					}
				}
			}
			check(generic)
		}
	})

	// ── Fail-closed: an unclassified outcome errors and leaks no protected value
	t.Run("unclassified_auth_outcome_fails_closed_and_leaks_no_protected_value", func(t *testing.T) {
		p, err := auth.Present(experience.AuthOutcome("unclassified"))
		if err == nil {
			t.Fatal("Present(unclassified) returned no error; auth adapter must fail closed")
		}
		var fpe *experience.F106PresentationError
		if !errors.As(err, &fpe) || fpe.Surface != "auth" {
			t.Fatalf("Present(unclassified) error is not an auth *F106PresentationError: %v", err)
		}
		// The failure path emits a ZERO-value presentation: no session retained,
		// no protected content, no re-auth offer — nothing to leak.
		if p.RetainSession || len(p.ClearedTargets) != 0 || p.OffersSafeReauthentication || p.ViewState != "" {
			t.Fatalf("failed auth presentation is not zero-value (leak risk): %+v", p)
		}
	})

	// ── Cross-presenter redaction: a raw error in a code field fails closed ────
	//
	// The privacy contract is not auth-only: a state or mutation owner outcome
	// carrying raw (non content-free) detail must fail closed and emit no value,
	// so a stack frame / secret / query can never reach a presented state.
	t.Run("state_and_mutation_presenters_reject_raw_detail_and_leak_no_value", func(t *testing.T) {
		badRead := experience.OwnerReadOutcome{
			Kind: experience.ReadFailed, Owner: "search",
			ActionCode: "https://host.example/reset?token=abc123",
		}
		gotView, err := sp.PresentRead(badRead)
		if err == nil {
			t.Fatal("PresentRead(raw url in action code) returned no error; must fail closed")
		}
		if gotView != "" {
			t.Fatalf("PresentRead leaked a ViewState %q while rejecting raw detail", gotView)
		}
		assertNoSensitive(t, "state presenter error", err.Error())

		badMutation := experience.OwnerMutationOutcome{
			Kind: experience.MutationOwnerCorePlusPending, Owner: "lists",
			OutstandingDependencyCode: "panic: runtime error: index out of range /app/internal/web/handler.go:412",
		}
		gotState, err := mp.Present(badMutation)
		if err == nil {
			t.Fatal("Present(raw stack in dependency code) returned no error; must fail closed")
		}
		if gotState != "" {
			t.Fatalf("Present leaked a MutationState %q while rejecting raw detail", gotState)
		}
		assertNoSensitive(t, "mutation presenter error", err.Error())
	})
}

package main

// Spec 060 Scope 3 — unit tests for the CLI scope-flag validation and
// rotation-scope-resolution helpers. These tests run pure-logic paths
// that do NOT require a DB connection, NATS, or SST env loading; the
// production callers (`runAuthEnroll`, `runAuthRotate`) call these
// helpers BEFORE any DB connect so an invalid invocation exits 2
// without touching the store. That structural choice is what makes
// these tests possible — flipping the order back (DB connect, then
// validate) would re-introduce the bug class spec 060 BS-005/BS-006
// guards against.
//
// Coverage matrix:
//   - SCN-060-013 (BS-005): invalid scope name → exit 2
//   - SCN-060-014 (BS-006): unknown surface w/o escape → exit 2;
//                            with --allow-unknown-surface → accept
//   - SCN-060-015 (BS-008): rotation preserve without --prior-token
//                            with no --scope → exit 2 (refuse at-source)
//   - SCN-060-016 (BS-009): demote sentinel (--scope "") → nil scopes;
//                            mixed --scope "" with non-empty → exit 2
//   - Repeatable --scope flag accumulates; embedded `,` NOT split
//
// The passthrough-wrapper smoke test (SCN-060-018) requires the live
// docker stack and is intentionally NOT included here; it is covered
// out-of-band by `./smackerel.sh test integration` once the test
// stack is up. The scopes.md DoD records an Uncertainty Declaration
// for that integration-only coverage.

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// TestValidateScopeFlags_EmptySliceAccepted is the no-`--scope`
// invocation (legacy enroll). Returns (nil, 0, "") — the CLI proceeds
// to mint a legacy spec-044-shape token.
func TestValidateScopeFlags_EmptySliceAccepted(t *testing.T) {
	got, exit, msg := validateScopeFlags(nil, false)
	if exit != 0 || msg != "" || got != nil {
		t.Fatalf("expected (nil, 0, \"\"), got (%v, %d, %q)", got, exit, msg)
	}
}

// TestValidateScopeFlags_RejectsInvalidScopeName proves SCN-060-013 /
// BS-005: an invalid scope-name shape exits 2 with a stderr message
// naming the offending value. The adversarial assertion is that the
// exit code is EXACTLY 2 (invocation error), NOT 1 (command failure)
// — operators tooling distinguishes the two for CI gating.
func TestValidateScopeFlags_RejectsInvalidScopeName(t *testing.T) {
	cases := []string{
		"ExtensionBookmarks",          // uppercase, no `:`
		"extension",                   // no `:`
		":bookmarks",                  // empty surface
		"extension:",                  // empty capability
		"extension:Bookmarks",         // uppercase capability
		"extension:bookmarks history", // space
		"",                            // empty
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			got, exit, msg := validateScopeFlags([]string{c}, false)
			if exit != 2 {
				t.Fatalf("expected exit=2 for %q, got %d (msg=%q)", c, exit, msg)
			}
			if got != nil {
				t.Fatalf("expected nil scopes on rejection, got %v", got)
			}
			if !strings.Contains(msg, "invalid scope name") {
				t.Fatalf("expected stderr to contain 'invalid scope name', got %q", msg)
			}
		})
	}
}

// TestValidateScopeFlags_RejectsUnknownSurfaceWithoutEscape proves
// SCN-060-014 / BS-006: an unknown surface exits 2 unless the operator
// supplies `--allow-unknown-surface`.
func TestValidateScopeFlags_RejectsUnknownSurfaceWithoutEscape(t *testing.T) {
	got, exit, msg := validateScopeFlags([]string{"future:capability"}, false)
	if exit != 2 {
		t.Fatalf("expected exit=2, got %d (msg=%q)", exit, msg)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on rejection, got %v", got)
	}
	if !strings.Contains(msg, "unknown scope surface") {
		t.Fatalf("expected stderr to contain 'unknown scope surface', got %q", msg)
	}
	if !strings.Contains(msg, "future") {
		t.Fatalf("expected stderr to name the offending surface, got %q", msg)
	}
	if !strings.Contains(msg, "--allow-unknown-surface") {
		t.Fatalf("expected stderr to name the escape-hatch flag, got %q", msg)
	}
}

// TestValidateScopeFlags_AcceptsUnknownSurfaceWithEscape proves
// SCN-060-014 / BS-006 escape path: with `--allow-unknown-surface=true`
// the validator accepts the scope and proceeds. The structured WARN
// log is emitted as a side effect; capturing slog output is out of
// scope for this pure unit (the WARN is verified at the integration
// layer once the live stack is up).
func TestValidateScopeFlags_AcceptsUnknownSurfaceWithEscape(t *testing.T) {
	got, exit, msg := validateScopeFlags([]string{"future:capability"}, true)
	if exit != 0 {
		t.Fatalf("expected exit=0 with escape hatch, got %d (msg=%q)", exit, msg)
	}
	if len(got) != 1 || got[0] != "future:capability" {
		t.Fatalf("expected scope preserved verbatim, got %v", got)
	}
}

// TestValidateScopeFlags_AcceptsRegisteredSurface confirms that the
// known `extension` surface (the only spec 060 initial entry) passes
// validation without the escape hatch.
func TestValidateScopeFlags_AcceptsRegisteredSurface(t *testing.T) {
	got, exit, msg := validateScopeFlags([]string{"extension:bookmarks,history"}, false)
	if exit != 0 {
		t.Fatalf("expected exit=0, got %d (msg=%q)", exit, msg)
	}
	if len(got) != 1 || got[0] != "extension:bookmarks,history" {
		t.Fatalf("expected scope preserved verbatim (embedded `,` NOT split), got %v", got)
	}
}

// TestValidateScopeFlags_AccumulatesMultipleEntries proves the
// repeatable-flag semantics: callers append each `--scope` occurrence
// to the slice, and the validator passes the slice through verbatim
// when all entries pass. The embedded `,` in any single value is
// NEVER split — that is the headline adversarial guarantee for the
// spec 058 wire `extension:bookmarks,history` value.
func TestValidateScopeFlags_AccumulatesMultipleEntries(t *testing.T) {
	in := []string{"extension:bookmarks,history", "extension:other"}
	got, exit, msg := validateScopeFlags(in, false)
	if exit != 0 {
		t.Fatalf("expected exit=0, got %d (msg=%q)", exit, msg)
	}
	if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
		t.Fatalf("expected slice preserved verbatim, got %v", got)
	}
}

// TestResolveRotationScopes_RefusesPreserveWithoutPriorToken proves
// SCN-060-015 / BS-008 refuse path: no `--scope`, no `--prior-token`
// → exit 2 at-source, no minting attempt.
func TestResolveRotationScopes_RefusesPreserveWithoutPriorToken(t *testing.T) {
	got, exit, msg := resolveRotationScopes(nil, "", false, auth.VerifyOptions{})
	if exit != 2 {
		t.Fatalf("expected exit=2, got %d (msg=%q)", exit, msg)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on refuse, got %v", got)
	}
	if !strings.Contains(msg, "--prior-token") || !strings.Contains(msg, "--scope") {
		t.Fatalf("expected stderr to name both --prior-token and --scope, got %q", msg)
	}
}

// TestResolveRotationScopes_DemotesOnEmptySentinel proves SCN-060-016
// / BS-009 demote path: a single `--scope ""` returns nil scopes so
// the rotation mints a legacy spec-044-shape token (no `scope` claim).
func TestResolveRotationScopes_DemotesOnEmptySentinel(t *testing.T) {
	got, exit, msg := resolveRotationScopes([]string{""}, "", false, auth.VerifyOptions{})
	if exit != 0 {
		t.Fatalf("expected exit=0, got %d (msg=%q)", exit, msg)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on demote, got %v", got)
	}
}

// TestResolveRotationScopes_RejectsEmptySentinelMixedWithNonEmpty
// proves SCN-060-016 / BS-009 mixed-rejection path: combining the
// demote sentinel with any non-empty scope exits 2. This is the
// adversarial guard against an operator typo that would silently
// either demote the token (losing the explicit scopes) or accept the
// scopes (silently dropping the demote intent) — both behaviors are
// data-integrity bugs; the only safe outcome is exit 2.
func TestResolveRotationScopes_RejectsEmptySentinelMixedWithNonEmpty(t *testing.T) {
	got, exit, msg := resolveRotationScopes(
		[]string{"", "extension:bookmarks,history"}, "", false, auth.VerifyOptions{})
	if exit != 2 {
		t.Fatalf("expected exit=2, got %d (msg=%q)", exit, msg)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on mixed-rejection, got %v", got)
	}
	if !strings.Contains(msg, `--scope ""`) {
		t.Fatalf("expected stderr to name `--scope \"\"`, got %q", msg)
	}
}

// TestResolveRotationScopes_AcceptsExplicitReplacement proves the
// explicit-replace path: `--scope <new>` without `--prior-token` is
// the explicit replace mode. Validation goes through validateScopeFlags
// (regex + registry) — invalid input still exits 2, valid input
// returns the scope slice verbatim.
func TestResolveRotationScopes_AcceptsExplicitReplacement(t *testing.T) {
	got, exit, msg := resolveRotationScopes(
		[]string{"extension:bookmarks,history"}, "", false, auth.VerifyOptions{})
	if exit != 0 {
		t.Fatalf("expected exit=0, got %d (msg=%q)", exit, msg)
	}
	if len(got) != 1 || got[0] != "extension:bookmarks,history" {
		t.Fatalf("expected scope preserved verbatim, got %v", got)
	}
}

// TestResolveRotationScopes_RejectsInvalidExplicitReplacement proves
// the explicit-replace path still threads through validateScopeFlags —
// an invalid scope-name shape exits 2 even on the rotation path.
// Adversarial guard: a regression that bypassed validation on the
// rotation path would silently accept malformed scopes (the headline
// spec 060 BS-002 anti-pattern at a different surface).
func TestResolveRotationScopes_RejectsInvalidExplicitReplacement(t *testing.T) {
	got, exit, msg := resolveRotationScopes(
		[]string{"BadlyFormatted"}, "", false, auth.VerifyOptions{})
	if exit != 2 {
		t.Fatalf("expected exit=2, got %d (msg=%q)", exit, msg)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on rejection, got %v", got)
	}
	if !strings.Contains(msg, "invalid scope name") {
		t.Fatalf("expected stderr to contain 'invalid scope name', got %q", msg)
	}
}

// TestResolveRotationScopes_PreservePathParsesPriorToken proves
// SCN-060-015 / BS-008 preserve path end-to-end at the helper layer:
// given a freshly minted prior token with a known scope claim, the
// helper returns the SAME scopes parsed back out of the wire form.
// The PASETO mint here uses `auth.IssueToken` directly so the test
// stays in-process (no DB, no SST env).
func TestResolveRotationScopes_PreservePathParsesPriorToken(t *testing.T) {
	priv, pub := auth.GenerateSigningKeypair()
	const keyID = "test-key-1"
	const wantScope = "extension:bookmarks,history"

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-1",
		SigningKey: priv,
		KeyID:      keyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
		Scopes:     []string{wantScope},
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	verifyOpts := auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        keyID,
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Minute,
		Now:                time.Now,
	}

	got, exit, msg := resolveRotationScopes(nil, issued.WireToken, false, verifyOpts)
	if exit != 0 {
		t.Fatalf("expected exit=0, got %d (msg=%q)", exit, msg)
	}
	if len(got) != 1 || got[0] != wantScope {
		t.Fatalf("expected scope preserved from prior token, got %v", got)
	}
}

// TestResolveRotationScopes_PreservePathHandlesLegacyPriorToken
// proves the preserve path is safe when the prior token is a legacy
// spec-044 token with no `scope` claim — the helper returns nil
// scopes (legacy → legacy roundtrip), NOT a wildcard fallback. This
// is the rotation-surface mirror of the spec 060 BS-002 anti-pattern
// guard.
func TestResolveRotationScopes_PreservePathHandlesLegacyPriorToken(t *testing.T) {
	priv, pub := auth.GenerateSigningKeypair()
	const keyID = "test-key-1"

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-1",
		SigningKey: priv,
		KeyID:      keyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
		// Scopes intentionally nil — legacy spec-044 shape.
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	verifyOpts := auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        keyID,
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Minute,
		Now:                time.Now,
	}

	got, exit, msg := resolveRotationScopes(nil, issued.WireToken, false, verifyOpts)
	if exit != 0 {
		t.Fatalf("expected exit=0 for legacy prior token, got %d (msg=%q)", exit, msg)
	}
	if got != nil {
		t.Fatalf("expected nil scopes from legacy prior token (NEVER a wildcard), got %v", got)
	}
}

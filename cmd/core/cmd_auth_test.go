package main

// Spec 060 Scope 3 + spec 108 §10.9 — unit tests for the CLI
// scope-flag validation, the rotation-scope-resolution helper, and the
// `auth list-users` GRANTS column renderer. These tests run pure-logic
// paths that need no DB connection, NATS, or SST env loading.
//
// One structural note, because it is load-bearing rather than
// incidental: every rotation mode EXCEPT record-preserve still
// resolves before `runAuthRotate` opens a pool, so an invalid
// invocation exits 2 without touching the store. Spec 108 added one
// mode that genuinely needs a read — preserve-from-record — and
// `runAuthRotate` opens the store ahead of resolution for that mode
// alone. TestResolveRotationScopes_NonPreserveModesNeverReadTheRecord
// pins the boundary so a refactor cannot quietly move the read up and
// re-introduce the bug class spec 060 BS-005/BS-006 guard against.
//
// Coverage matrix:
//   - SCN-060-013 (BS-005): invalid scope name → exit 2
//   - SCN-060-014 (BS-006): unknown surface w/o escape → exit 2;
//                            with --allow-unknown-surface → accept
//   - SCN-060-016 (BS-009): demote sentinel (--scope "") → nil scopes;
//                            mixed --scope "" with non-empty → exit 2
//   - Repeatable --scope flag accumulates; embedded `,` NOT split
//   - Spec 108 §10.9: preserve from the recorded set; refuse (naming
//                     the principal) on NULL, on no standing token,
//                     and on a reader error; GRANTS column renders
//                     four distinct non-empty states
//
// SCN-060-015 (BS-008) is deliberately SUPERSEDED: the no-`--scope` /
// no-`--prior-token` combination is no longer an unconditional exit 2,
// because spec 108 §10.9 makes it preserve from the recorded set. The
// invariant it protected — this mode never guesses — is now carried by
// TestResolveRotationScopes_RefusesWhenRecordedGrantsAreUnknown.
//
// The passthrough-wrapper smoke test (SCN-060-018) requires the live
// docker stack and is intentionally NOT included here; it is covered
// out-of-band by `./smackerel.sh test integration` once the test
// stack is up. The scopes.md DoD records an Uncertainty Declaration
// for that integration-only coverage. The same applies to the SQL in
// internal/auth/principal_grants.go: these tests exercise the type
// contract and the query STRUCTURE, not query execution.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// fakeGrantReader is the test double for the spec 108 design.md §10
// reader primitive. It records the call count so a test can prove the
// non-preserve rotation modes never touch the database.
type fakeGrantReader struct {
	grants    auth.RecordedGrants
	err       error
	calls     int
	gotUserID string
}

func (f *fakeGrantReader) GrantsForPrincipal(_ context.Context, userID string) (auth.RecordedGrants, error) {
	f.calls++
	f.gotUserID = userID
	return f.grants, f.err
}

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

// TestResolveRotationScopes_RefusesPreserveWithoutReader proves the
// no-`--scope` / no-`--prior-token` combination refuses when no grant
// reader was constructed.
//
// Spec 060 BS-008 made this combination an unconditional exit 2
// because preserving required a wire token. Spec 108 design.md §10.9
// deliberately SUPERSEDES that: the combination now preserves from the
// recorded grant set, and the tests below cover each of its outcomes.
// What survives from BS-008 is the underlying invariant — this mode
// never guesses a scope set.
func TestResolveRotationScopes_RefusesPreserveWithoutReader(t *testing.T) {
	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		UserID: "alice",
	})
	if exit == 0 {
		t.Fatalf("expected a refusal with no reader, got exit=0 scopes=%v", got)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on refuse, got %v", got)
	}
	if !strings.Contains(msg, "alice") {
		t.Fatalf("expected the refusal to name the principal, got %q", msg)
	}
}

// TestResolveRotationScopes_DemotesOnEmptySentinel proves SCN-060-016
// / BS-009 demote path: a single `--scope ""` returns nil scopes so
// the rotation mints a legacy spec-044-shape token (no `scope` claim).
func TestResolveRotationScopes_DemotesOnEmptySentinel(t *testing.T) {
	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		Scopes: []string{""},
	})
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
	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		Scopes: []string{"", "extension:bookmarks,history"},
	})
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
	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		Scopes: []string{"extension:bookmarks,history"},
	})
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
	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		Scopes: []string{"BadlyFormatted"},
	})
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

	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		PriorToken: issued.WireToken,
		UserID:     "alice",
		VerifyOpts: verifyOpts,
	})
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

	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		PriorToken: issued.WireToken,
		UserID:     "alice",
		VerifyOpts: verifyOpts,
	})
	if exit != 0 {
		t.Fatalf("expected exit=0 for legacy prior token, got %d (msg=%q)", exit, msg)
	}
	if got != nil {
		t.Fatalf("expected nil scopes from legacy prior token (NEVER a wildcard), got %v", got)
	}
}

// ---------------------------------------------------------------------
// Spec 108 design.md §10.9 — operator surface.
// ---------------------------------------------------------------------

// TestResolveRotationScopes_PreservesFromRecordedSet is the headline
// spec 108 §10.9 behavior: preserve mode now answers from the recorded
// grant set, so the operator never needs the wire token they were told
// at mint time they would never see again.
func TestResolveRotationScopes_PreservesFromRecordedSet(t *testing.T) {
	reader := &fakeGrantReader{grants: auth.RecordedGrants{
		TokenID:  "tok-standing",
		Scopes:   []string{"corpus:read", "annotation:edit"},
		Recorded: true,
	}}

	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		UserID: "alice",
		Reader: reader,
	})
	if exit != 0 {
		t.Fatalf("expected exit=0, got %d (msg=%q)", exit, msg)
	}
	if strings.Join(got, ",") != "corpus:read,annotation:edit" {
		t.Fatalf("expected the recorded set preserved verbatim, got %v", got)
	}
	if reader.gotUserID != "alice" {
		t.Errorf("reader queried user %q, want the principal being rotated (\"alice\")", reader.gotUserID)
	}
}

// TestResolveRotationScopes_RefusesWhenRecordedGrantsAreUnknown proves
// the NULL refusal required by spec 108 §10.9: when granted_scopes IS
// NULL the grants are unknown and MUST NOT be guessed.
//
// This is the pair-half of TestResolveRotationScopes_PreservesRecordedAsNone
// below. Both inputs carry ZERO scopes, so an implementation that
// branched on `len(Scopes) == 0` instead of on `Recorded` would fail
// one of the two whichever way it guessed. Neither test alone proves
// the distinction is honored.
func TestResolveRotationScopes_RefusesWhenRecordedGrantsAreUnknown(t *testing.T) {
	reader := &fakeGrantReader{grants: auth.RecordedGrants{
		TokenID:  "tok-pre-063",
		Recorded: false, // granted_scopes IS NULL
	}}

	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		UserID: "alice",
		Reader: reader,
	})
	if exit == 0 {
		t.Fatalf("ADVERSARIAL FAILURE: unknown grants resolved to scopes=%v with exit=0 — spec 108 §10.9 requires a refusal, never a guess", got)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on refuse, got %v", got)
	}
	if !strings.Contains(msg, "alice") {
		t.Errorf("expected the refusal to NAME the principal, got %q", msg)
	}
	if !strings.Contains(msg, "tok-pre-063") {
		t.Errorf("expected the refusal to name the specific token needing rotation, got %q", msg)
	}
}

// TestResolveRotationScopes_PreservesRecordedAsNone proves '{}' is
// preserved rather than refused. A deliberately unscoped token is a
// determinate answer; refusing it would treat "recorded as none" as if
// it were "unknown", which is the conflation migration 063 exists to
// prevent.
func TestResolveRotationScopes_PreservesRecordedAsNone(t *testing.T) {
	reader := &fakeGrantReader{grants: auth.RecordedGrants{
		TokenID:  "tok-none",
		Scopes:   []string{},
		Recorded: true, // granted_scopes = '{}'
	}}

	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		UserID: "alice",
		Reader: reader,
	})
	if exit != 0 {
		t.Fatalf("ADVERSARIAL FAILURE: recorded-as-none was refused (exit=%d msg=%q) — '{}' is a recording and MUST preserve, unlike NULL", exit, msg)
	}
	if len(got) != 0 {
		t.Fatalf("expected zero preserved scopes, got %v", got)
	}
}

// TestResolveRotationScopes_RefusesWhenPrincipalNotProvisioned proves
// the no-standing-token case refuses and names the principal, and that
// it is recognized through errors.Is rather than string matching.
func TestResolveRotationScopes_RefusesWhenPrincipalNotProvisioned(t *testing.T) {
	reader := &fakeGrantReader{
		err: fmt.Errorf("auth: grants for principal %q: %w", "alice", auth.ErrPrincipalNotProvisioned),
	}

	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		UserID: "alice",
		Reader: reader,
	})
	if exit == 0 {
		t.Fatalf("expected a refusal for an unprovisioned principal, got exit=0 scopes=%v", got)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on refuse, got %v", got)
	}
	if !strings.Contains(msg, "alice") {
		t.Errorf("expected the refusal to name the principal, got %q", msg)
	}
	if !strings.Contains(msg, "no active unexpired token") {
		t.Errorf("expected the unprovisioned diagnostic to be distinct from the unknown-grants one, got %q", msg)
	}
}

// TestResolveRotationScopes_FailsClosedOnReaderError proves a reader
// error surfaces as a refusal, never as an empty or permissive set.
// A read failure means authority could not be determined; minting
// anything at that moment would substitute silence for an answer.
func TestResolveRotationScopes_FailsClosedOnReaderError(t *testing.T) {
	reader := &fakeGrantReader{err: errors.New("connection refused")}

	got, exit, msg := resolveRotationScopes(context.Background(), rotationScopeInput{
		UserID: "alice",
		Reader: reader,
	})
	if exit == 0 {
		t.Fatalf("ADVERSARIAL FAILURE: reader error resolved to scopes=%v with exit=0 — a failed read MUST NOT mint anything", got)
	}
	if got != nil {
		t.Fatalf("expected nil scopes on reader error, got %v", got)
	}
	if !strings.Contains(msg, "alice") || !strings.Contains(msg, "connection refused") {
		t.Errorf("expected the refusal to name the principal and surface the cause, got %q", msg)
	}
}

// TestResolveRotationScopes_NonPreserveModesNeverReadTheRecord proves
// the structural property runAuthRotate depends on: every mode except
// record-preserve resolves without a database read, which is what lets
// a malformed invocation still exit 2 before a pool is opened. A
// regression that moved the read to the top of the function would pass
// every behavioral test above and fail only this one.
func TestResolveRotationScopes_NonPreserveModesNeverReadTheRecord(t *testing.T) {
	cases := []struct {
		name  string
		input rotationScopeInput
	}{
		{"demote sentinel", rotationScopeInput{UserID: "alice", Scopes: []string{""}}},
		{"explicit replace", rotationScopeInput{UserID: "alice", Scopes: []string{"corpus:read"}}},
		{"invalid scope name", rotationScopeInput{UserID: "alice", Scopes: []string{"BadlyFormatted"}}},
		{"mixed sentinel", rotationScopeInput{UserID: "alice", Scopes: []string{"", "corpus:read"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeGrantReader{}
			in := tc.input
			in.Reader = reader
			resolveRotationScopes(context.Background(), in)
			if reader.calls != 0 {
				t.Errorf("reader consulted %d time(s); the %s mode MUST resolve without a DB read", reader.calls, tc.name)
			}
		})
	}
}

// TestFormatGrantsColumn_RendersFourDistinctNonEmptyStates covers the
// `auth list-users` GRANTS column.
//
// The NULL row is the one that matters. A test covering only the
// populated case would pass while NULL rendered as an empty cell —
// and an empty cell in a GRANTS column reads as "this principal has no
// grants", silently fabricating an authority claim about a principal
// whose grants are merely unrecorded. So every state is asserted, and
// every state is asserted to be non-empty and pairwise distinct.
func TestFormatGrantsColumn_RendersFourDistinctNonEmptyStates(t *testing.T) {
	cases := []struct {
		name string
		in   auth.EnrolledUserGrants
		want string
	}{
		{
			name: "granted_scopes IS NULL renders the literal unknown",
			in: auth.EnrolledUserGrants{
				HasStandingToken: true,
				Grants:           auth.RecordedGrants{TokenID: "tok-pre-063", Recorded: false},
			},
			want: "unknown",
		},
		{
			name: "granted_scopes = '{}' renders recorded-as-none, NOT unknown",
			in: auth.EnrolledUserGrants{
				HasStandingToken: true,
				Grants:           auth.RecordedGrants{TokenID: "tok-none", Scopes: []string{}, Recorded: true},
			},
			want: "(none)",
		},
		{
			name: "recorded set renders the scopes",
			in: auth.EnrolledUserGrants{
				HasStandingToken: true,
				Grants: auth.RecordedGrants{
					TokenID:  "tok-set",
					Scopes:   []string{"corpus:read", "annotation:edit"},
					Recorded: true,
				},
			},
			want: "corpus:read,annotation:edit",
		},
		{
			name: "no standing token is determinate and distinct from unknown",
			in:   auth.EnrolledUserGrants{HasStandingToken: false},
			want: "(no active token)",
		},
	}

	seen := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatGrantsColumn(tc.in)
			if got != tc.want {
				t.Errorf("formatGrantsColumn() = %q, want %q", got, tc.want)
			}
			if strings.TrimSpace(got) == "" {
				t.Errorf("ADVERSARIAL FAILURE: rendered an empty GRANTS cell — an empty cell reads as \"no grants\" and would fabricate authority state (spec.md S7)")
			}
			if prior, dup := seen[got]; dup {
				t.Errorf("ADVERSARIAL FAILURE: state %q renders as %q, identical to state %q — the states MUST stay distinguishable", tc.name, got, prior)
			}
			seen[got] = tc.name
		})
	}
}

// TestGrantsColumnLiterals_CannotCollideWithAScopeName proves the
// sentinel literals are unmistakable for real data: every scope name
// must satisfy auth.ValidateScopeName, and none of the sentinels do.
// Without this, a future scope vocabulary change could make "unknown"
// ambiguous between "unrecorded" and "a scope literally named unknown".
func TestGrantsColumnLiterals_CannotCollideWithAScopeName(t *testing.T) {
	for _, literal := range []string{
		grantsColumnUnknown,
		grantsColumnRecordedNone,
		grantsColumnNoStandingToken,
	} {
		if err := auth.ValidateScopeName(literal); err == nil {
			t.Errorf("grants-column sentinel %q is a VALID scope name — it could be mistaken for recorded data", literal)
		}
	}
}

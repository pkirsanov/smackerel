// Spec 108 Scope 04 — T2 subset property for the Telegram delegation ceiling.
//
// design.md §10.10 layer T2: for a table of recorded sets `R` (including
// sets with and without `corpus:read`, empty, and unknown), assert
// `derived(R) ⊆ R`. The ceiling narrows and never confers.
//
// WHY THIS IS THE PROPERTY THAT MATTERS. `TelegramBridgeDelegableGrants` is
// documented as "a CEILING, not a grant list" — it may only ever WITHHOLD
// authority the principal already holds. That distinction is the whole of
// spec.md §18 decision 3: authority is defined at the principal, never at
// the minter. Read as prose it is a promise; asserted as `derived ⊆ recorded`
// over an exhaustive table it is a property. An implementation that confers
// — one that unions the ceiling into the result, or returns the ceiling when
// the recorded set is empty — fails the rows below.
//
// The single most load-bearing row is the one WITHOUT `corpus:read`: it is
// the negative case SCN-108-E04 turns into a 403. A test that only checks
// "the bridge still works" passes against a hardcoded list and proves
// nothing, which is why absence is asserted explicitly rather than inferred
// from a length check.
//
// # Anti-vacuity
//
// The subset assertion is itself a claim that must be falsifiable, so the
// subset checker is factored out and pointed at a deliberately conferring
// implementation in `TestDeriveTelegramBridgeGrants_SubsetCheckerIsNotVacuous`.
// §10.10 requires T2 to be "shown to fail against a union implementation";
// that demonstration is executable here rather than a one-off manual capture,
// so it re-runs on every commit instead of aging out.
//
// References:
//   - specs/108-corpus-grant-enforcement/design.md §10.7, §10.10
//   - specs/108-corpus-grant-enforcement/scopes.md Scope 04 (SCN-108-E01/E04)
//   - internal/telegram/scope_literal_guard_test.go (layer T1)
package auth

import (
	"slices"
	"strings"
	"testing"
)

// unrelatedGrant is a grant outside the delegation ceiling. Held by the
// principal, never delegable to the bridge — it exercises the "recorded but
// not delegable" direction, which is the ceiling doing its narrowing job.
const unrelatedGrant = "billing:admin"

// subsetViolations returns the elements of got that are absent from
// universe. Empty result means got ⊆ universe.
//
// Factored out rather than inlined so the same checker can be aimed at a
// conferring implementation below. A subset assertion that has never been
// observed to fail is indistinguishable from no assertion at all.
func subsetViolations(got, universe []string) []string {
	violations := make([]string, 0)
	for _, element := range got {
		if !slices.Contains(universe, element) {
			violations = append(violations, element)
		}
	}
	return violations
}

// TestTelegramBridgeDelegableGrants_IsTheDocumentedCeiling pins the ceiling's
// membership.
//
// This is a change-detector on purpose: widening the set widens what a
// credential-less principal may be delegated, which design.md §10.7 requires
// to be a deliberate, reviewed act rather than a drive-by append.
func TestTelegramBridgeDelegableGrants_IsTheDocumentedCeiling(t *testing.T) {
	want := []string{GrantGlobalCorpusRead, GrantAnnotationEdit}
	if !slices.Equal(TelegramBridgeDelegableGrants, want) {
		t.Errorf("TelegramBridgeDelegableGrants = %v, want %v — widening the ceiling widens what a credential-less principal may be delegated (design.md §10.7); update the doc comment and the route enumeration in the same change",
			TelegramBridgeDelegableGrants, want)
	}

	if slices.Contains(TelegramBridgeDelegableGrants, wildcardGrant) {
		t.Errorf("ceiling contains the wildcard sentinel %q — a bridge token must never carry a blanket grant", wildcardGrant)
	}

	seen := make(map[string]bool, len(TelegramBridgeDelegableGrants))
	for _, grant := range TelegramBridgeDelegableGrants {
		if seen[grant] {
			t.Errorf("ceiling contains duplicate %q — duplicates would be emitted twice by DeriveTelegramBridgeGrants", grant)
		}
		seen[grant] = true
	}
}

// TestDeriveTelegramBridgeGrants_SubsetProperty is T2.
//
// Every row asserts BOTH invariants the doc comment claims hold "by
// construction rather than by review":
//
//	derived ⊆ recorded                        — never confers
//	derived ⊆ TelegramBridgeDelegableGrants   — never exceeds the ceiling
//
// plus the exact expected intersection, so a implementation that merely
// returns something small still fails.
func TestDeriveTelegramBridgeGrants_SubsetProperty(t *testing.T) {
	cases := []struct {
		name         string
		recorded     []string
		want         []string
		wantAbsent   []string
		significance string
	}{
		{
			name:         "principal holds both delegable grants",
			recorded:     []string{GrantGlobalCorpusRead, GrantAnnotationEdit},
			want:         []string{GrantGlobalCorpusRead, GrantAnnotationEdit},
			significance: "SCN-108-E01 — the corpus command succeeds because the principal holds the grant, not because the minter named it",
		},
		{
			name:       "principal WITHOUT corpus:read",
			recorded:   []string{GrantAnnotationEdit},
			want:       []string{GrantAnnotationEdit},
			wantAbsent: []string{GrantGlobalCorpusRead},
			significance: "SCN-108-E04, the decisive negative row — a conferring implementation returns corpus:read here " +
				"and the bridge silently grants corpus access to every mapped chat",
		},
		{
			name:         "principal holds ONLY corpus:read",
			recorded:     []string{GrantGlobalCorpusRead},
			want:         []string{GrantGlobalCorpusRead},
			wantAbsent:   []string{GrantAnnotationEdit},
			significance: "the mirror of the row above — narrowing must be per-grant, not all-or-nothing",
		},
		{
			name:         "recorded as none (granted_scopes = '{}')",
			recorded:     []string{},
			want:         []string{},
			wantAbsent:   []string{GrantGlobalCorpusRead, GrantAnnotationEdit},
			significance: "a principal recorded as holding nothing is delegated nothing; the caller must refuse the mint",
		},
		{
			name:         "unknown (granted_scopes IS NULL, reader returns nil)",
			recorded:     nil,
			want:         []string{},
			wantAbsent:   []string{GrantGlobalCorpusRead, GrantAnnotationEdit},
			significance: "absent grant data denies rather than defaults — nil must not read as permissive",
		},
		{
			name:         "principal holds only grants outside the ceiling",
			recorded:     []string{unrelatedGrant},
			want:         []string{},
			wantAbsent:   []string{GrantGlobalCorpusRead, GrantAnnotationEdit},
			significance: "the ceiling withholds a recorded grant the bridge may not delegate",
		},
		{
			name:         "delegable and non-delegable grants together",
			recorded:     []string{unrelatedGrant, GrantAnnotationEdit},
			want:         []string{GrantAnnotationEdit},
			wantAbsent:   []string{GrantGlobalCorpusRead, unrelatedGrant},
			significance: "narrowing keeps the delegable grant and drops the rest — derived ⊆ ceiling as well as ⊆ recorded",
		},
		{
			name:         "recorded wildcard confers nothing",
			recorded:     []string{wildcardGrant},
			want:         []string{},
			wantAbsent:   []string{GrantGlobalCorpusRead, GrantAnnotationEdit},
			significance: "mirrors the RequireScope invariant — a recorded '*' must not expand into concrete grants",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			derived := DeriveTelegramBridgeGrants(tc.recorded)

			// T2 core: derivation NARROWS and never confers.
			if violations := subsetViolations(derived, tc.recorded); len(violations) > 0 {
				t.Errorf("SUBSET VIOLATION — derived ⊄ recorded: %v conferred but not held.\n  recorded = %v\n  derived  = %v\n  why it matters: %s",
					violations, tc.recorded, derived, tc.significance)
			}

			// Second documented invariant: never exceeds the ceiling.
			if violations := subsetViolations(derived, TelegramBridgeDelegableGrants); len(violations) > 0 {
				t.Errorf("CEILING VIOLATION — derived ⊄ TelegramBridgeDelegableGrants: %v.\n  derived = %v\n  ceiling = %v",
					violations, derived, TelegramBridgeDelegableGrants)
			}

			if !slices.Equal(derived, tc.want) {
				t.Errorf("DeriveTelegramBridgeGrants(%v) = %v, want %v — %s",
					tc.recorded, derived, tc.want, tc.significance)
			}

			// Absence asserted explicitly. A length check would pass for
			// an implementation that returned the right COUNT of the
			// wrong grants.
			for _, absent := range tc.wantAbsent {
				if slices.Contains(derived, absent) {
					t.Errorf("GRANT CONFERRED — %q present in derived %v but not delegable from recorded %v.\n  why it matters: %s",
						absent, derived, tc.recorded, tc.significance)
				}
			}
		})
	}
}

// TestDeriveTelegramBridgeGrants_ExhaustiveSubsetProperty runs the subset
// property over every subset of a small universe.
//
// The table above is chosen by hand and therefore only proves the property
// for the cases someone thought of. The vocabulary is small enough to
// enumerate exhaustively, so the property can be proven for ALL of it —
// which is the difference between "the examples pass" and "the property
// holds".
func TestDeriveTelegramBridgeGrants_ExhaustiveSubsetProperty(t *testing.T) {
	universe := []string{GrantGlobalCorpusRead, GrantAnnotationEdit, unrelatedGrant, wildcardGrant}

	for mask := 0; mask < 1<<len(universe); mask++ {
		recorded := make([]string, 0, len(universe))
		for bit, grant := range universe {
			if mask&(1<<bit) != 0 {
				recorded = append(recorded, grant)
			}
		}

		derived := DeriveTelegramBridgeGrants(recorded)

		if violations := subsetViolations(derived, recorded); len(violations) > 0 {
			t.Errorf("subset %v: derived %v confers %v", recorded, derived, violations)
		}
		if violations := subsetViolations(derived, TelegramBridgeDelegableGrants); len(violations) > 0 {
			t.Errorf("subset %v: derived %v exceeds ceiling by %v", recorded, derived, violations)
		}

		// The result must be exactly the intersection: subset alone would
		// be satisfied by an implementation that always returns empty.
		want := make([]string, 0, len(TelegramBridgeDelegableGrants))
		for _, grant := range TelegramBridgeDelegableGrants {
			if grant != wildcardGrant && slices.Contains(recorded, grant) {
				want = append(want, grant)
			}
		}
		if !slices.Equal(derived, want) {
			t.Errorf("subset %v: derived %v, want intersection %v", recorded, derived, want)
		}
	}
}

// TestDeriveTelegramBridgeGrants_SubsetCheckerIsNotVacuous is the §10.10
// "shown to fail against a union implementation" demonstration.
//
// Without this, every subset assertion above would pass identically if
// subsetViolations always returned empty — the tests would be green and
// blind. This aims the real checker at a deliberately conferring
// implementation and requires it to object.
func TestDeriveTelegramBridgeGrants_SubsetCheckerIsNotVacuous(t *testing.T) {
	// The rejected shortcut, written out: a minter-side list that hands the
	// full ceiling to every mapped chat regardless of what the principal
	// actually holds. This is what per_user_token.go:201 used to do.
	conferring := func(recorded []string) []string {
		union := slices.Clone(recorded)
		for _, grant := range TelegramBridgeDelegableGrants {
			if !slices.Contains(union, grant) {
				union = append(union, grant)
			}
		}
		return union
	}

	// The load-bearing negative row: a principal WITHOUT corpus:read.
	recorded := []string{GrantAnnotationEdit}

	conferred := conferring(recorded)
	violations := subsetViolations(conferred, recorded)
	if len(violations) == 0 {
		t.Fatalf("subsetViolations is VACUOUS: a union implementation returned %v for recorded %v and the checker reported no violation. Every subset assertion in this file is therefore decorative.",
			conferred, recorded)
	}
	if !slices.Contains(violations, GrantGlobalCorpusRead) {
		t.Errorf("checker reported %v but missed %q — the conferred grant that turns SCN-108-E04 from a 403 into a silent success",
			violations, GrantGlobalCorpusRead)
	}

	// And the real implementation must pass the same input the conferring
	// one fails, so the demonstration discriminates between them rather
	// than flagging everything.
	derived := DeriveTelegramBridgeGrants(recorded)
	if len(subsetViolations(derived, recorded)) != 0 {
		t.Errorf("real implementation confers on the same input the union implementation fails: derived %v from recorded %v", derived, recorded)
	}
	if slices.Contains(derived, GrantGlobalCorpusRead) {
		t.Errorf("real implementation conferred %q to a principal that does not hold it — derived %v", GrantGlobalCorpusRead, derived)
	}

	t.Logf("checker discriminates: union impl → violations %v; real impl → derived %v (clean)", violations, derived)
}

// TestDeriveTelegramBridgeGrants_OrderFollowsCeilingNotRecorded pins the
// determinism the doc comment claims.
//
// It matters operationally: the recorded set arrives from a database column
// with no ordering guarantee, and the derived list becomes a token claim.
// Order-dependent output would make otherwise-identical tokens differ, which
// breaks claim comparison and makes fixtures flaky for reasons unrelated to
// authority.
func TestDeriveTelegramBridgeGrants_OrderFollowsCeilingNotRecorded(t *testing.T) {
	forward := DeriveTelegramBridgeGrants([]string{GrantGlobalCorpusRead, GrantAnnotationEdit})
	reversed := DeriveTelegramBridgeGrants([]string{GrantAnnotationEdit, GrantGlobalCorpusRead})

	if !slices.Equal(forward, reversed) {
		t.Errorf("output depends on input order: %v vs %v — the recorded set has no ordering guarantee, so the claim must not inherit one",
			forward, reversed)
	}
	if !slices.Equal(forward, TelegramBridgeDelegableGrants) {
		t.Errorf("output order = %v, want ceiling order %v", forward, TelegramBridgeDelegableGrants)
	}
}

// TestDeriveTelegramBridgeGrants_ReturnsEmptySliceNotNil pins the empty-result
// contract.
//
// The doc comment states an empty result "is returned as an empty slice,
// never as a permissive one", and callers MUST refuse the mint on it. A nil
// return would serialize differently (JSON `null` rather than `[]`) and
// invites a `len(x) == 0 → use defaults` misread downstream.
func TestDeriveTelegramBridgeGrants_ReturnsEmptySliceNotNil(t *testing.T) {
	for _, recorded := range [][]string{nil, {}, {unrelatedGrant}, {wildcardGrant}} {
		derived := DeriveTelegramBridgeGrants(recorded)
		if derived == nil {
			t.Errorf("DeriveTelegramBridgeGrants(%v) returned nil, want an empty slice — callers must read this as 'no delegable authority', not as 'unset'", recorded)
		}
		if len(derived) != 0 {
			t.Errorf("DeriveTelegramBridgeGrants(%v) = %v, want empty", recorded, derived)
		}
	}
}

// TestDeriveTelegramBridgeGrants_DoesNotMutateRecorded guards the caller's
// input.
//
// The recorded set is read from the principal's standing token and may be
// reused by the caller for diagnostics or an audit line. Mutating or
// aliasing it would let a later append to the derived slice silently rewrite
// what the audit trail reports the principal held.
func TestDeriveTelegramBridgeGrants_DoesNotMutateRecorded(t *testing.T) {
	recorded := []string{GrantGlobalCorpusRead, GrantAnnotationEdit, unrelatedGrant}
	before := slices.Clone(recorded)

	derived := DeriveTelegramBridgeGrants(recorded)
	if !slices.Equal(recorded, before) {
		t.Fatalf("recorded set mutated: %v, was %v", recorded, before)
	}

	// Growing the result must not reach back into the input's backing
	// array. Appending to a slice that shared storage with recorded would
	// overwrite the next element in place.
	grown := append(derived, unrelatedGrant)
	if !slices.Equal(recorded, before) {
		t.Errorf("appending to the derived slice mutated the recorded set: %v, was %v — the result aliases the caller's input (grown = %v)",
			recorded, before, grown)
	}
}

// TestDeriveTelegramBridgeGrants_FailureMessagesNameTheRemedy is a guard on
// the guards.
//
// A subset violation surfaces to whoever broke it. This asserts the shared
// vocabulary those messages rely on still exists, so a rename cannot leave
// the failure text pointing at a symbol that no longer exists.
func TestDeriveTelegramBridgeGrants_FailureMessagesNameTheRemedy(t *testing.T) {
	if GrantGlobalCorpusRead == "" || GrantAnnotationEdit == "" {
		t.Fatal("grant constants are empty — the T1 guard in internal/telegram references these symbolically and would silently forbid nothing")
	}
	if !strings.Contains(GrantGlobalCorpusRead, ":") || !strings.Contains(GrantAnnotationEdit, ":") {
		t.Errorf("grant constants no longer carry the scope shape (%q, %q) — internal/telegram/scope_literal_guard_test.go matches on that shape",
			GrantGlobalCorpusRead, GrantAnnotationEdit)
	}
}

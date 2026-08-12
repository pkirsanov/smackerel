// Spec 108 Scope 04 — TP-04-08 second half: what actually lands in the minted
// PASETO `scope` claim.
//
// WHY THIS FILE EXISTS. `scope_literal_guard_test.go` (T1) proves the
// hardcoded `Scopes: []string{"annotation:edit"}` is GONE from the package
// source. That is a statement about what the source may CONTAIN. It says
// nothing about what the minter EMITS. Before this file,
// `per_user_token_test.go` never parsed `scope` out of the wire token at all
// — a change that removed the literal and then minted the wrong claim (an
// empty set, the full ceiling regardless of the principal, a set read from the
// wrong field) would leave every test in this package green. The Consumer
// Impact Sweep recorded exactly that absence: "no test asserts the minted
// scope claim today."
//
// EQUALITY, NOT CONTAINMENT. Each case asserts the parsed claim EQUALS the
// expected derived set. A `contains` assertion is the wrong tool here: it
// detects under-granting (a capability silently lost) but is blind to
// OVER-granting (a capability silently gained), and over-granting is the
// failure that matters — it is authority the principal does not hold, arriving
// at a gate that will honor it.
//
// WHY MORE THAN ONE FIXTURE. A single recorded set cannot distinguish
// derivation from a constant: any hardcoded expectation that happened to equal
// that one set would pass. The table below drives FIVE distinct recorded sets
// producing THREE distinct claims, and `TestMintedScopeClaim_TableIsNotVacuous`
// asserts mechanically that the table yields more than one distinct expected
// claim — so the file cannot silently collapse into a single-fixture test that
// a constant would satisfy.
//
// The table also covers the NARROWING direction (a principal holding grants
// ABOVE the ceiling gets the ceiling, not its full set), which is the property
// that keeps a mapped operator's chat from minting `operator:admin` to serve a
// `/find`.
//
// This file writes no scope literal; every fixture is built from `auth.*`
// constants, per the T1 guard.
package telegram

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

const (
	scopeClaimChatID int64 = 5150
	scopeClaimUser         = "tg-scope-claim-principal"
	scopeClaimKeyID        = "scope04-claim-k1" //gitleaks:allow — PASETO key *identifier* (kid), not a credential; the signing key is generated per-test
)

// mintedScopeClaimCase is one recorded grant set and the EXACT claim the
// minted token must carry for it.
type mintedScopeClaimCase struct {
	name string
	// recorded is what GrantsForPrincipal answers — the sole authority
	// source (§18 decision 3).
	recorded []string
	// want is the exact expected `scope` claim: recorded ∩ ceiling.
	want []string
	why  string
}

func mintedScopeClaimCases() []mintedScopeClaimCase {
	return []mintedScopeClaimCase{
		{
			name:     "principal holds the whole ceiling",
			recorded: []string{auth.GrantGlobalCorpusRead, auth.GrantAnnotationEdit},
			want:     []string{auth.GrantGlobalCorpusRead, auth.GrantAnnotationEdit},
			why:      "both delegable grants are recorded, so both are carried",
		},
		{
			name:     "principal holds annotation only",
			recorded: []string{auth.GrantAnnotationEdit},
			want:     []string{auth.GrantAnnotationEdit},
			why:      "the corpus grant is NOT recorded, so the bridge must not carry it — this is the row a reintroduced literal fails",
		},
		{
			name:     "principal holds corpus only",
			recorded: []string{auth.GrantGlobalCorpusRead},
			want:     []string{auth.GrantGlobalCorpusRead},
			why:      "the mirror image; a literal fixed at the annotation grant fails this row instead",
		},
		{
			name:     "principal holds grants ABOVE the ceiling",
			recorded: []string{auth.GrantOperatorAdmin, auth.GrantGlobalCorpusRead, auth.GrantAssistantTurn, auth.GrantAnnotationEdit},
			want:     []string{auth.GrantGlobalCorpusRead, auth.GrantAnnotationEdit},
			why:      "the ceiling WITHHOLDS: a mapped operator chat must not mint operator authority just because the principal holds it",
		},
		{
			name:     "principal holds one delegable grant plus non-delegable ones",
			recorded: []string{auth.GrantKnowledgeGraphRead, auth.GrantAnnotationEdit, auth.GrantOperatorModelPicker},
			want:     []string{auth.GrantAnnotationEdit},
			why:      "narrowing and per-capability tracking at once",
		},
	}
}

// newScopeClaimMinter builds a production minter over a reader that answers
// with `recorded`, and returns the public key needed to parse the mint.
func newScopeClaimMinter(t *testing.T, recorded []string) (*PerUserTokenMinter, string) {
	t.Helper()
	priv, pub := auth.GenerateSigningKeypair()
	m, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot: &Bot{environment: "production", userMapping: map[int64]string{scopeClaimChatID: scopeClaimUser}},
		PrincipalGrants: &fakePrincipalGrantReader{grants: auth.RecordedGrants{
			TokenID:  "tok-scope-claim",
			Scopes:   append([]string(nil), recorded...),
			Recorded: true,
		}},
		SigningKey: priv,
		KeyID:      scopeClaimKeyID,
		Issuer:     "smackerel",
		TTL:        2 * time.Minute,
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	return m, pub
}

// parseMintedScopes verifies the wire token and returns its `scope` claim.
// Verification is real (`auth.VerifyAndParse`, the same call the middleware
// makes), so a claim that only "looks right" in a struct field but never
// survived signing and parsing cannot pass.
func parseMintedScopes(t *testing.T, m *PerUserTokenMinter, pub, wire string) []string {
	t.Helper()
	parsed, err := auth.VerifyAndParse(wire, auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        scopeClaimKeyID,
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Minute,
		Now:                m.now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}
	return parsed.Scopes
}

// assertScopeClaimEquals compares the parsed claim to the expected set as
// SETS, and reports the two directions separately: a missing member is lost
// capability, an extra member is authority the principal never held. Naming
// them apart is what makes a failure diagnosable instead of just red.
func assertScopeClaimEquals(t *testing.T, label string, got, want []string, why string) {
	t.Helper()
	gotSorted := append([]string(nil), got...)
	wantSorted := append([]string(nil), want...)
	slices.Sort(gotSorted)
	slices.Sort(wantSorted)

	if slices.Equal(gotSorted, wantSorted) {
		return
	}
	var missing, extra []string
	for _, w := range wantSorted {
		if !slices.Contains(gotSorted, w) {
			missing = append(missing, w)
		}
	}
	for _, g := range gotSorted {
		if !slices.Contains(wantSorted, g) {
			extra = append(extra, g)
		}
	}
	t.Errorf("%s: minted scope claim = %v, want EXACTLY %v (missing=%v, EXTRA=%v). %s\n"+
		"An EXTRA member is authority the principal does not hold arriving at a gate that will honor it — the minter-side list §18 decision 3 rejects. "+
		"A MISSING member means derivation dropped a recorded capability.",
		label, gotSorted, wantSorted, missing, extra, why)
}

// TestMintForUser_ScopeClaimEqualsDerivedGrantSet is TP-04-08's second half on
// the direct entry point.
func TestMintForUser_ScopeClaimEqualsDerivedGrantSet(t *testing.T) {
	for _, tc := range mintedScopeClaimCases() {
		t.Run(tc.name, func(t *testing.T) {
			m, pub := newScopeClaimMinter(t, tc.recorded)

			tok, err := m.MintForUser(context.Background(), scopeClaimChatID, scopeClaimUser)
			if err != nil {
				t.Fatalf("MintForUser: %v", err)
			}
			if tok.WireToken == "" {
				t.Fatal("WireToken empty — nothing to parse, so the claim assertion would be vacuous")
			}
			assertScopeClaimEquals(t, "MintForUser/"+tc.name, parseMintedScopes(t, m, pub, tok.WireToken), tc.want, tc.why)
		})
	}
}

// TestMintForChat_ScopeClaimEqualsDerivedGrantSet drives the same matrix
// through the PRODUCTION entry point, so the claim is proven on the path the
// bot actually calls rather than only on the test-friendly helper.
func TestMintForChat_ScopeClaimEqualsDerivedGrantSet(t *testing.T) {
	for _, tc := range mintedScopeClaimCases() {
		t.Run(tc.name, func(t *testing.T) {
			m, pub := newScopeClaimMinter(t, tc.recorded)

			tok, err := m.MintForChat(context.Background(), scopeClaimChatID)
			if err != nil {
				t.Fatalf("MintForChat: %v", err)
			}
			assertScopeClaimEquals(t, "MintForChat/"+tc.name, parseMintedScopes(t, m, pub, tok.WireToken), tc.want, tc.why)
		})
	}
}

// TestMintedScopeClaim_TableIsNotVacuous guards the guard.
//
// The whole argument for the two tests above is that no CONSTANT claim can
// satisfy every row. That argument evaporates the moment the table degenerates
// to rows sharing one expected claim — which is exactly what happens when a
// later author "simplifies" the fixtures. So the multiplicity is asserted
// rather than assumed: at least three distinct recorded sets producing at
// least two distinct expected claims, with at least one row that expects the
// corpus grant ABSENT and one that expects it PRESENT.
func TestMintedScopeClaim_TableIsNotVacuous(t *testing.T) {
	cases := mintedScopeClaimCases()
	if len(cases) < 3 {
		t.Fatalf("table has %d rows; need at least 3 distinct recorded sets", len(cases))
	}

	distinct := make(map[string]struct{}, len(cases))
	var sawCorpusPresent, sawCorpusAbsent bool
	for _, tc := range cases {
		key := append([]string(nil), tc.want...)
		slices.Sort(key)
		distinct[strings.Join(key, "|")] = struct{}{}

		if slices.Contains(tc.want, auth.GrantGlobalCorpusRead) {
			sawCorpusPresent = true
		} else {
			sawCorpusAbsent = true
		}
	}

	if len(distinct) < 2 {
		t.Errorf("all rows expect the same claim (%d distinct) — a hardcoded constant would satisfy the entire table, which is the regression these tests exist to catch", len(distinct))
	}
	if !sawCorpusPresent || !sawCorpusAbsent {
		t.Errorf("table must contain a row expecting %s PRESENT and a row expecting it ABSENT (present=%v absent=%v); without both, a constant list either always or never carries it and is never contradicted",
			auth.GrantGlobalCorpusRead, sawCorpusPresent, sawCorpusAbsent)
	}
}

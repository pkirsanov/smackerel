// Spec 108 SCOPE-01 — corpus scope-surface registration prerequisite.
//
// This file is the TP-01-01 / TP-01-02 home named by
// specs/108-corpus-grant-enforcement/scopes.md. It sits next to
// browser_session_policy.go because SCOPE-01 is precisely the join between the
// scope-name REGISTRY (scopes.go) and the GRANT VOCABULARY defined here: the
// `corpus` surface has to be registered before the already-defined
// GrantGlobalCorpusRead constant can be carried by a real minted token.
//
// The adversarial half matters more than the positive half. Registering the
// surface makes `corpus:read` GRANTABLE; it must not make it GRANTED. The
// owner directive for spec 108 is explicit — "Enforce the grant AS DESIGNED. Do
// NOT widen the daily default set" — and design.md "Resolved Decisions" states
// that an ungranted daily user is SUPPOSED to be denied. A suite that only
// proved the grant can be issued would still pass if `dailyUserGrants` were
// widened, which is the exact regression that would silently delete the
// boundary this feature exists to build. TestCorpusSurfaceRegistrationDoesNot
// WidenDefaultGrants therefore fails on that widening.
package auth

import (
	"slices"
	"testing"
)

// TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant is TP-01-01
// (SCN-108-P01): the `corpus` surface is registered, and it is the surface of
// the EXISTING GrantGlobalCorpusRead constant rather than a new parallel grant.
// Spec 108 §11 decision 3 is explicit that no new grant identifier is
// introduced, so the mapping is asserted by deriving the surface FROM the grant
// constant instead of restating the literal "corpus" on both sides — a
// hand-copied literal would keep passing if the constant were ever renamed.
func TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant(t *testing.T) {
	if !slices.Contains(RegisteredScopeSurfaces, "corpus") {
		t.Fatalf("RegisteredScopeSurfaces missing 'corpus' (spec 108 SCOPE-01, F-108-SURFACE-01): %v", RegisteredScopeSurfaces)
	}
	if !IsRegisteredScopeSurface("corpus") {
		t.Errorf("IsRegisteredScopeSurface('corpus') = false; expected true")
	}

	// The registered surface is the surface of the existing grant constant.
	surface := ExtractScopeSurface(GrantGlobalCorpusRead)
	if surface != "corpus" {
		t.Fatalf("ExtractScopeSurface(GrantGlobalCorpusRead=%q) = %q, want %q", GrantGlobalCorpusRead, surface, "corpus")
	}
	if !IsRegisteredScopeSurface(surface) {
		t.Fatalf("the surface of GrantGlobalCorpusRead (%q) is not registered: %v", surface, RegisteredScopeSurfaces)
	}

	// Adversarial: a near-miss surface must NOT be admitted. This fails if the
	// registry is ever widened by prefix/substring matching rather than by the
	// closed-set membership the spec 060 allowlist promises.
	for _, nearMiss := range []string{"corpu", "corpuss", "Corpus", "corpus:read"} {
		if IsRegisteredScopeSurface(nearMiss) {
			t.Errorf("IsRegisteredScopeSurface(%q) = true; the surface allowlist must be an exact closed set", nearMiss)
		}
	}
}

// TestCorpusReadScopeClaimValidatesAndAuthorizes is TP-01-02 (SCN-108-P02): a
// scope claim containing `corpus:read` validates without an unknown-surface
// error, and AuthorizeGrant returns authorized for a session that holds it.
func TestCorpusReadScopeClaimValidatesAndAuthorizes(t *testing.T) {
	if err := ValidateScopeName(GrantGlobalCorpusRead); err != nil {
		t.Fatalf("ValidateScopeName(%q) unexpected err: %v", GrantGlobalCorpusRead, err)
	}
	if !IsRegisteredScopeSurface(ExtractScopeSurface(GrantGlobalCorpusRead)) {
		t.Fatalf("%q would require the --allow-unknown-surface escape hatch; the surface must be registered", GrantGlobalCorpusRead)
	}

	// The operator holds the grant by default...
	operator := SessionWithRole("op-1", "jti-op", RoleOperator)
	if d := AuthorizeGrant(operator, GrantGlobalCorpusRead); !d.Allowed {
		t.Errorf("operator should hold %s, got %+v", GrantGlobalCorpusRead, d)
	}

	// ...and a daily user holds it ONLY when it is granted explicitly.
	grantedDaily := SessionWithRole("granted-1", "jti-granted", RoleDailyUser, GrantGlobalCorpusRead)
	if d := AuthorizeGrant(grantedDaily, GrantGlobalCorpusRead); !d.Allowed {
		t.Errorf("specifically-granted daily user should hold %s, got %+v", GrantGlobalCorpusRead, d)
	}
	if !GateGlobalCorpusRead(grantedDaily).Allowed {
		t.Errorf("specifically-granted daily user should pass GateGlobalCorpusRead")
	}
}

// TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants is the ADVERSARIAL
// case for SCOPE-01, and it is the one that protects the feature.
//
// Registering the surface changes what an operator may MINT. It must not change
// what any principal HOLDS. This test fails if `dailyUserGrants` is widened to
// include corpus:read, if GrantsForRole starts synthesizing a grant, or if
// AuthorizeGrant/GateGlobalCorpusRead ever admit an ungranted daily user. Each
// of those is a silent deletion of the boundary spec 108 exists to enforce, and
// each would leave the positive tests above still green.
func TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants(t *testing.T) {
	dailyUser := SessionWithRole("daily-1", "jti-daily", RoleDailyUser)

	if slices.Contains(dailyUser.Scopes, GrantGlobalCorpusRead) {
		t.Fatalf("daily-user default grant snapshot must NOT contain %s (spec 108 owner directive: do not widen the daily set); got %v", GrantGlobalCorpusRead, dailyUser.Scopes)
	}
	if slices.Contains(GrantsForRole(RoleDailyUser), GrantGlobalCorpusRead) {
		t.Fatalf("GrantsForRole(RoleDailyUser) must NOT contain %s; got %v", GrantGlobalCorpusRead, GrantsForRole(RoleDailyUser))
	}
	if d := AuthorizeGrant(dailyUser, GrantGlobalCorpusRead); d.Allowed {
		t.Fatalf("ungranted daily user must be DENIED %s; registering the surface makes the grant grantable, not granted", GrantGlobalCorpusRead)
	} else if d.Reason != "grant_absent" {
		t.Errorf("denial reason = %q, want %q", d.Reason, "grant_absent")
	}
	if GateGlobalCorpusRead(dailyUser).Allowed {
		t.Fatalf("ungranted daily user must NOT pass GateGlobalCorpusRead")
	}

	// A bare authenticated session holds nothing: authority never follows from
	// merely possessing a valid credential.
	if GateGlobalCorpusRead(Session{UserID: "bare", TokenID: "jti-bare", Source: SessionSourcePerUserToken}).Allowed {
		t.Errorf("a bare valid session must hold no corpus grant")
	}
	// A wildcard is never honored, even now that the surface is registered.
	if GateGlobalCorpusRead(Session{Scopes: []string{"*"}}).Allowed {
		t.Errorf("wildcard must never open the corpus gate")
	}

	// operatorGrants already carried corpus:read before this scope; SCOPE-01
	// must leave it exactly as it was.
	operatorSet := GrantsForRole(RoleOperator)
	wantOperator := []string{
		GrantAssistantTurn,
		GrantKnowledgeGraphRead,
		GrantGlobalCorpusRead,
		GrantOperatorAdmin,
		GrantOperatorModelPicker,
	}
	if !slices.Equal(operatorSet, wantOperator) {
		t.Errorf("operatorGrants changed by SCOPE-01: got %v want %v", operatorSet, wantOperator)
	}
	wantDaily := []string{GrantAssistantTurn, GrantKnowledgeGraphRead}
	if !slices.Equal(GrantsForRole(RoleDailyUser), wantDaily) {
		t.Errorf("dailyUserGrants changed by SCOPE-01: got %v want %v", GrantsForRole(RoleDailyUser), wantDaily)
	}
}

//go:build e2e

// TP-01-04 — spec 108 SCOPE-01 regression E2E (SCN-108-P01, SCN-108-P02).
//
// This is the PERMANENT guard for the scope-registration prerequisite. Every
// later scope in spec 108 assumes the `corpus` surface exists and maps to
// `corpus:read`; if that registration is ever removed or renamed, the gate
// mounted in Scope 03 silently stops gating anything it was built to gate,
// and the failure would surface as "the corpus is readable by everyone"
// rather than as a compile error.
//
// It runs in the e2e lane, against the live stack, so the registration is
// asserted in the same shape a deployed binary carries rather than only in a
// unit-test process.

package auth_e2e

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// TestE2E_Spec108_CorpusSurfaceStaysRegistered_TP_01_04 — SCN-108-P01.
func TestE2E_Spec108_CorpusSurfaceStaysRegistered_TP_01_04(t *testing.T) {
	// Prove the stack is actually up, so this cannot pass as a pure unit
	// assertion in a lane whose stack failed to start.
	base := e2eBaseURL(t)
	resp, err := newNoRedirectClient().Do(mustGet(t, base+"/api/health"))
	if err != nil {
		t.Fatalf("live stack health probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live stack health = %d, want 200; the rest of this regression would be asserting against a dead stack", resp.StatusCode)
	}

	if !slices.Contains(auth.RegisteredScopeSurfaces, "corpus") {
		t.Fatalf("RegisteredScopeSurfaces = %v; the `corpus` surface is NOT registered. Every spec 108 scope assumes it, and without it `corpus:read` becomes an unknown-surface scope that cannot be minted", auth.RegisteredScopeSurfaces)
	}

	// The surface must still map to this exact grant string. A rename would
	// leave the surface registered while every gate consulting the constant
	// stops matching the tokens already in the field.
	if auth.GrantGlobalCorpusRead != "corpus:read" {
		t.Errorf("GrantGlobalCorpusRead = %q, want %q; tokens already issued carry the old value and would stop authorizing", auth.GrantGlobalCorpusRead, "corpus:read")
	}
	if err := auth.ValidateScopeName(auth.GrantGlobalCorpusRead); err != nil {
		t.Errorf("ValidateScopeName(%q) = %v; the grant must validate against the closed surface registry without the unknown-surface escape hatch", auth.GrantGlobalCorpusRead, err)
	}
}

// TestE2E_Spec108_CorpusGrantIsNotInTheDailyDefaultSet_TP_01_04 pins the
// invariant that makes the whole spec meaningful. If `corpus:read` were ever
// added to the daily default grant set, every daily user would hold it, the
// Scope 03 gate would allow everyone, and no test asserting "a granted
// principal is allowed" would notice.
func TestE2E_Spec108_CorpusGrantIsNotInTheDailyDefaultSet_TP_01_04(t *testing.T) {
	daily := auth.GrantsForRole(auth.RoleDailyUser)
	if slices.Contains(daily, auth.GrantGlobalCorpusRead) {
		t.Errorf("daily default grants = %v and now include %q. The corpus grant must be issued deliberately per principal (a token rotation), never handed to every daily user by default — that would make the Scope 03 gate a no-op", daily, auth.GrantGlobalCorpusRead)
	}

	// The operator set is the control: if this also failed, the assertion
	// above would be passing because the grant vanished entirely rather than
	// because it is correctly withheld.
	operator := auth.GrantsForRole(auth.RoleOperator)
	if !slices.Contains(operator, auth.GrantGlobalCorpusRead) {
		t.Errorf("operator default grants = %v and no longer include %q; the operator owns the corpus and must retain read access", operator, auth.GrantGlobalCorpusRead)
	}
}

// TestE2E_Spec108_CorpusTokenStillMintsAndAuthorizes_TP_01_04 — SCN-108-P02.
// Exercises issuance → verify → session → gate for a granted principal, with
// an ungranted principal as the adversarial control.
func TestE2E_Spec108_CorpusTokenStillMintsAndAuthorizes_TP_01_04(t *testing.T) {
	priv, pub := auth.GenerateSigningKeypair()
	const kid = "tp-01-04-kid"

	mint := func(t *testing.T, userID string, scopes []string) auth.Session {
		t.Helper()
		tokenID, err := auth.GenerateTokenID()
		if err != nil {
			t.Fatalf("GenerateTokenID: %v", err)
		}
		issued, err := auth.IssueToken(auth.IssueOptions{
			UserID:     userID,
			TokenID:    tokenID,
			SigningKey: priv,
			KeyID:      kid,
			TTL:        time.Hour,
			Issuer:     "smackerel",
			Now:        time.Now,
			Scopes:     scopes,
		})
		if err != nil {
			t.Fatalf("IssueToken(%s): %v", userID, err)
		}
		parsed, err := auth.VerifyAndParse(issued.WireToken, auth.VerifyOptions{
			ActivePublicKey: pub,
			ActiveKeyID:     kid,
			Issuer:          "smackerel",
			Now:             time.Now,
		})
		if err != nil {
			t.Fatalf("VerifyAndParse(%s): %v", userID, err)
		}
		return auth.Session{
			UserID: parsed.UserID,
			Scopes: parsed.Scopes,
			Source: auth.SessionSourcePerUserToken,
		}
	}

	granted := mint(t, "tp0104-granted", []string{auth.GrantGlobalCorpusRead})
	if !auth.GateGlobalCorpusRead(granted).Allowed {
		t.Errorf("a freshly minted %q token does not authorize a corpus read; the mint→verify→gate path is broken", auth.GrantGlobalCorpusRead)
	}

	ungranted := mint(t, "tp0104-ungranted", nil)
	if auth.GateGlobalCorpusRead(ungranted).Allowed {
		t.Errorf("a token carrying NO scope claim authorizes a corpus read. If this fails while the granted case passes, the gate is authorizing unconditionally and the corpus is open")
	}

	// A wildcard is never a grant (spec 060 BS-002). Asserted here because a
	// wildcard-honouring regression would pass both cases above.
	wild := mint(t, "tp0104-wildcard", []string{"*"})
	if auth.GateGlobalCorpusRead(wild).Allowed {
		t.Errorf("a wildcard scope authorized a corpus read; a wildcard MUST NEVER be honored as a grant")
	}
}

func mustGet(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", url, err)
	}
	return req
}

// Spec 060 scope 1 — PASETO `scope` claim roundtrip + legacy + malformed.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
)

func TestIssueToken_SetsScopeClaim(t *testing.T) {
	priv, _ := GenerateSigningKeypair()
	pub := publicHexFromPrivateHexT(t, priv)
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	issued, err := IssueToken(IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-scope-1",
		SigningKey: priv,
		KeyID:      "k1",
		TTL:        time.Hour,
		Issuer:     "smackerel-test",
		Now:        clock,
		Scopes:     []string{"extension:bookmarks,history"},
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	parsed, err := VerifyAndParse(issued.WireToken, VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "k1",
		Issuer: "smackerel-test", Now: clock,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}
	if len(parsed.Scopes) != 1 || parsed.Scopes[0] != "extension:bookmarks,history" {
		t.Errorf("Scopes round-trip mismatch: %v", parsed.Scopes)
	}
}

func TestVerifyAndParse_NilScopesForLegacyToken(t *testing.T) {
	priv, _ := GenerateSigningKeypair()
	pub := publicHexFromPrivateHexT(t, priv)
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	// Mint legacy-shape token (no Scopes).
	issued, err := IssueToken(IssueOptions{
		UserID:     "bob",
		TokenID:    "tok-legacy",
		SigningKey: priv,
		KeyID:      "k1",
		TTL:        time.Hour,
		Issuer:     "smackerel-test",
		Now:        clock,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	parsed, err := VerifyAndParse(issued.WireToken, VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "k1",
		Issuer: "smackerel-test", Now: clock,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}
	if parsed.Scopes != nil {
		t.Errorf("expected nil Scopes for legacy token, got %v", parsed.Scopes)
	}
}

// TestVerifyAndParse_MalformedScopeClaimFallsBackToNil mints a token
// directly via the paseto API with a malformed `scope` claim element
// and verifies that parsing yields Scopes: nil (legacy treatment),
// NEVER a wildcard sentinel. Spec 060 BS-002 adversarial regression.
func TestVerifyAndParse_MalformedScopeClaimFallsBackToNil(t *testing.T) {
	priv, _ := GenerateSigningKeypair()
	pub := publicHexFromPrivateHexT(t, priv)
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	secret, err := paseto.NewV4AsymmetricSecretKeyFromHex(priv)
	if err != nil {
		t.Fatalf("parse priv: %v", err)
	}

	tok := paseto.NewToken()
	tok.SetIssuer("smackerel-test")
	tok.SetSubject("mallory")
	tok.SetJti("tok-bad")
	tok.SetIssuedAt(now)
	tok.SetNotBefore(now)
	tok.SetExpiration(now.Add(time.Hour))
	tok.SetFooter([]byte(`{"kid":"k1"}`))
	if err := tok.Set("scope", []string{"BadlyFormatted"}); err != nil {
		t.Fatalf("set scope: %v", err)
	}
	wire := tok.V4Sign(secret, nil)

	parsed, err := VerifyAndParse(wire, VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "k1",
		Issuer: "smackerel-test",
		Now:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}
	if parsed.Scopes != nil {
		t.Fatalf("malformed scope claim MUST yield Scopes: nil (NEVER a wildcard); got %v", parsed.Scopes)
	}

	// Sanity: getScopeClaim itself returns ErrScopeClaimMalformed for
	// the same shape so future refactors cannot silently change the
	// defense-in-depth contract.
	innerTok := paseto.NewToken()
	if err := innerTok.Set("scope", []string{"BadlyFormatted"}); err != nil {
		t.Fatal(err)
	}
	if _, err := getScopeClaim(&innerTok); !errors.Is(err, ErrScopeClaimMalformed) {
		t.Errorf("getScopeClaim expected ErrScopeClaimMalformed, got %v", err)
	}

	// Print the malformed claim shape for debug visibility.
	if b, _ := json.Marshal(parsed); len(b) == 0 {
		t.Fatal(fmt.Sprintf("unexpected empty parsed: %v", parsed))
	}
}

func TestGetScopeClaim_AbsentReturnsNilNil(t *testing.T) {
	tok := paseto.NewToken()
	scopes, err := getScopeClaim(&tok)
	if err != nil || scopes != nil {
		t.Fatalf("expected (nil, nil) for absent claim; got (%v, %v)", scopes, err)
	}
}

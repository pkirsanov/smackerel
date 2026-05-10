package auth

import (
	"strings"
	"testing"
	"time"
)

// T1-04 — IssueToken produces a valid PASETO v4.public wire token whose
// claims (sub, jti, iat, exp, iss) round-trip through VerifyAndParse
// under the same key material. Failure of this round-trip would mean
// the issuer and verifier disagree on the wire format and would block
// every per-user request once the middleware lands in Scope 02.
func TestIssueToken_RoundTripWithVerify(t *testing.T) {
	priv, _ := GenerateSigningKeypair()

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	issued, err := IssueToken(IssueOptions{
		UserID:     "user-alice",
		TokenID:    "tok-2026-05-10-alice-001",
		SigningKey: priv,
		KeyID:      "key-2026-05",
		TTL:        24 * time.Hour,
		Issuer:     "smackerel-test",
		Now:        clock,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if issued.WireToken == "" {
		t.Fatal("WireToken is empty")
	}
	if !strings.HasPrefix(issued.WireToken, "v4.public.") {
		t.Errorf("expected v4.public prefix, got: %q", issued.WireToken[:min(len(issued.WireToken), 20)])
	}
	if !issued.IssuedAt.Equal(now) {
		t.Errorf("IssuedAt mismatch: want %v got %v", now, issued.IssuedAt)
	}
	if !issued.ExpiresAt.Equal(now.Add(24 * time.Hour)) {
		t.Errorf("ExpiresAt mismatch: want %v got %v", now.Add(24*time.Hour), issued.ExpiresAt)
	}

	// Round-trip — derive the public key from the same hex private key.
	pub := publicHexFromPrivateHexT(t, priv)
	parsed, err := VerifyAndParse(issued.WireToken, VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "key-2026-05",
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                clock,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse round-trip: %v", err)
	}
	if parsed.UserID != "user-alice" {
		t.Errorf("UserID round-trip mismatch: want user-alice got %q", parsed.UserID)
	}
	if parsed.TokenID != "tok-2026-05-10-alice-001" {
		t.Errorf("TokenID round-trip mismatch: got %q", parsed.TokenID)
	}
	if parsed.KeyID != "key-2026-05" {
		t.Errorf("KeyID round-trip mismatch: got %q", parsed.KeyID)
	}
}

// IssueToken refuses partial input (every required field is checked).
func TestIssueToken_RejectsMissingFields(t *testing.T) {
	priv, _ := GenerateSigningKeypair()
	now := func() time.Time { return time.Now() }

	cases := []struct {
		name   string
		opts   IssueOptions
		expect string
	}{
		{
			name:   "no-user-id",
			opts:   IssueOptions{TokenID: "t", SigningKey: priv, KeyID: "k", TTL: time.Hour, Issuer: "iss", Now: now},
			expect: "UserID",
		},
		{
			name:   "no-token-id",
			opts:   IssueOptions{UserID: "u", SigningKey: priv, KeyID: "k", TTL: time.Hour, Issuer: "iss", Now: now},
			expect: "TokenID",
		},
		{
			name:   "no-signing-key",
			opts:   IssueOptions{UserID: "u", TokenID: "t", KeyID: "k", TTL: time.Hour, Issuer: "iss", Now: now},
			expect: "SigningKey",
		},
		{
			name:   "no-key-id",
			opts:   IssueOptions{UserID: "u", TokenID: "t", SigningKey: priv, TTL: time.Hour, Issuer: "iss", Now: now},
			expect: "KeyID",
		},
		{
			name:   "no-issuer",
			opts:   IssueOptions{UserID: "u", TokenID: "t", SigningKey: priv, KeyID: "k", TTL: time.Hour, Now: now},
			expect: "Issuer",
		},
		{
			name:   "zero-ttl",
			opts:   IssueOptions{UserID: "u", TokenID: "t", SigningKey: priv, KeyID: "k", Issuer: "iss", Now: now},
			expect: "TTL",
		},
		{
			name:   "no-clock",
			opts:   IssueOptions{UserID: "u", TokenID: "t", SigningKey: priv, KeyID: "k", TTL: time.Hour, Issuer: "iss"},
			expect: "Now",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := IssueToken(tc.opts)
			if err == nil {
				t.Fatalf("expected error naming %q, got nil", tc.expect)
			}
			if !strings.Contains(err.Error(), tc.expect) {
				t.Errorf("expected error to name %q, got: %v", tc.expect, err)
			}
		})
	}
}

// publicHexFromPrivateHexT derives the V4 public key hex from a
// private key hex by reusing the go-paseto secret-key API. Used by the
// unit tests so they do not depend on storing the public half
// separately.
func publicHexFromPrivateHexT(t *testing.T, privHex string) string {
	t.Helper()
	// Local import-cycle-free derivation — the issue.go uses
	// aidanwoods.dev/go-paseto so we replicate the conversion here.
	return derivePublicHexT(t, privHex)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

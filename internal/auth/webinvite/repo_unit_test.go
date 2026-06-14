// Spec 093 SCOPE-01 — pure-unit coverage for the webinvite token format and
// HashToken determinism. These assertions need NO database: they exercise the
// token-mint format (inv_ + base64url(32 random bytes)) and the lowercase-hex
// SHA-256 at-rest hash. The DB-backed Generate/IsLive/ConsumeAndCreate/List/
// Revoke behaviours are covered honestly in the integration tier
// (repo_pg_test.go, //go:build integration) because they require a real
// Postgres — they cannot and must not be faked.
package webinvite

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// TestWebInvite_GenerateTokenShapeAndHash asserts the minted token format and
// the at-rest hash invariants (SCN-093-01, the database-independent half).
func TestWebInvite_GenerateTokenShapeAndHash(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		tok, err := newPlaintextToken()
		if err != nil {
			t.Fatalf("newPlaintextToken: %v", err)
		}
		// Prefix is the operator-recognizable cosmetic marker.
		if !strings.HasPrefix(tok, "inv_") {
			t.Fatalf("token %q does not start with inv_", tok)
		}
		// 32 random bytes → 43 RawURLEncoding chars; 4 (prefix) + 43 = 47.
		if len(tok) != 47 {
			t.Fatalf("token %q has length %d, want 47", tok, len(tok))
		}
		// The body must be valid unpadded URL-safe base64 of exactly 32 bytes.
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(tok, "inv_"))
		if err != nil {
			t.Fatalf("token body is not RawURLEncoding base64: %v (token=%q)", err, tok)
		}
		if len(raw) != 32 {
			t.Fatalf("decoded %d random bytes, want 32 (token=%q)", len(raw), tok)
		}
		// No padding chars must leak into the URL-safe token.
		if strings.ContainsAny(tok, "=+/") {
			t.Fatalf("token %q contains a non-URL-safe / padding character", tok)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("token %q repeated — generator is not random", tok)
		}
		seen[tok] = struct{}{}
	}
}

// TestHashToken_Deterministic asserts HashToken is a deterministic,
// lowercase-hex SHA-256 that covers the WHOLE string (including the inv_
// prefix) and separates distinct inputs.
func TestHashToken_Deterministic(t *testing.T) {
	const sample = "inv_AbCdEf0123456789-_xyz"

	h1 := HashToken(sample)
	h2 := HashToken(sample)
	if h1 != h2 {
		t.Fatalf("HashToken not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("HashToken length = %d, want 64 (hex SHA-256)", len(h1))
	}
	if h1 != strings.ToLower(h1) {
		t.Fatalf("HashToken is not lowercase hex: %q", h1)
	}
	if _, err := hex.DecodeString(h1); err != nil {
		t.Fatalf("HashToken output is not valid hex: %v", err)
	}
	// Distinct inputs (incl. only the prefix differing) must hash differently.
	if HashToken("inv_"+sample) == h1 {
		t.Fatalf("HashToken collided on prefixed vs unprefixed input")
	}
	if HashToken(strings.TrimPrefix(sample, "inv_")) == h1 {
		t.Fatalf("HashToken ignored the inv_ prefix (must hash the whole string)")
	}
}

// TestNewPostgresRepo_NilGuard asserts the constructor refuses a nil pool
// (mirrors webcreds.NewPostgresRepo — no silent dev no-op).
func TestNewPostgresRepo_NilGuard(t *testing.T) {
	if _, err := NewPostgresRepo(nil); err == nil {
		t.Fatal("NewPostgresRepo(nil) returned nil error; want a refusal")
	}
}

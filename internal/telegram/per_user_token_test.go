// Spec 044 Scope 03 — unit coverage for PerUserTokenMinter.
package telegram

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// minterTestKeypair returns a fresh PASETO v4.public keypair so each
// test run is independent.
func minterTestKeypair(t *testing.T) (priv, pub string) {
	t.Helper()
	priv, pub = auth.GenerateSigningKeypair()
	if priv == "" || pub == "" {
		t.Fatal("GenerateSigningKeypair returned empty keys")
	}
	return priv, pub
}

func newProductionMinter(t *testing.T, mapping map[int64]string) (*PerUserTokenMinter, string) {
	t.Helper()
	priv, pub := minterTestKeypair(t)
	bot := &Bot{environment: "production", userMapping: mapping}
	m, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      "scope03-mint-key",
		Issuer:     "smackerel",
		TTL:        2 * time.Minute,
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	return m, pub
}

func TestNewPerUserTokenMinter_Validates(t *testing.T) {
	priv, _ := auth.GenerateSigningKeypair()
	cases := []struct {
		name    string
		opts    PerUserTokenMinterOptions
		wantErr string
	}{
		{"missing bot", PerUserTokenMinterOptions{SigningKey: priv, KeyID: "k1"}, "non-nil Bot"},
		{"missing signing key", PerUserTokenMinterOptions{Bot: &Bot{}, SigningKey: "", KeyID: "k1"}, "non-empty SigningKey"},
		{"missing key id", PerUserTokenMinterOptions{Bot: &Bot{}, SigningKey: priv, KeyID: ""}, "non-empty KeyID"},
		{"whitespace key id", PerUserTokenMinterOptions{Bot: &Bot{}, SigningKey: priv, KeyID: "   "}, "non-empty KeyID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewPerUserTokenMinter(tc.opts)
			if err == nil {
				t.Fatalf("want error containing %q, got nil minter=%v", tc.wantErr, m)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestNewPerUserTokenMinter_DefaultsAppliedWhenZero(t *testing.T) {
	priv, _ := auth.GenerateSigningKeypair()
	m, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        &Bot{environment: "production", userMapping: map[int64]string{1: "u"}},
		SigningKey: priv,
		KeyID:      "k1",
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	if m.issuer != "smackerel" {
		t.Errorf("issuer default=%q want smackerel", m.issuer)
	}
	if m.ttl != 5*time.Minute {
		t.Errorf("ttl default=%v want 5m", m.ttl)
	}
	if m.now == nil {
		t.Errorf("now must default to time.Now")
	}
}

func TestMintForChat_Production_MappedChat_ProducesVerifiableToken(t *testing.T) {
	mapping := map[int64]string{12345: "alice"}
	m, pub := newProductionMinter(t, mapping)

	tok, err := m.MintForChat(12345)
	if err != nil {
		t.Fatalf("MintForChat: %v", err)
	}
	if tok.WireToken == "" {
		t.Fatal("WireToken empty")
	}
	if tok.UserID != "alice" {
		t.Errorf("UserID=%q want alice", tok.UserID)
	}
	if tok.ChatID != 12345 {
		t.Errorf("ChatID=%d want 12345", tok.ChatID)
	}
	if !strings.HasPrefix(tok.TokenID, "tg-12345-") {
		t.Errorf("TokenID=%q want tg-12345- prefix", tok.TokenID)
	}

	parsed, err := auth.VerifyAndParse(tok.WireToken, auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "scope03-mint-key",
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Minute,
		Now:                m.now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse round-trip: %v", err)
	}
	if parsed.UserID != "alice" {
		t.Errorf("PASETO sub=%q want alice (claim-binding MUST come from mapping)", parsed.UserID)
	}
	if parsed.TokenID != tok.TokenID {
		t.Errorf("PASETO jti=%q want %q", parsed.TokenID, tok.TokenID)
	}
}

func TestMintForChat_Production_UnmappedChat_ReturnsError(t *testing.T) {
	m, _ := newProductionMinter(t, map[int64]string{12345: "alice"})
	tok, err := m.MintForChat(99999)
	if !errors.Is(err, ErrNoUserMappingForChat) {
		t.Fatalf("err=%v want %v", err, ErrNoUserMappingForChat)
	}
	if tok.WireToken != "" {
		t.Errorf("unmapped MUST NOT mint a token; WireToken=%q", tok.WireToken)
	}
}

func TestMintForChat_Production_EmptyMapping_RejectsAll(t *testing.T) {
	m, _ := newProductionMinter(t, nil)
	_, err := m.MintForChat(12345)
	if !errors.Is(err, ErrNoUserMappingForChat) {
		t.Fatalf("err=%v want %v (production with empty mapping MUST reject every chat)", err, ErrNoUserMappingForChat)
	}
}

func TestMintForChat_Dev_UnmappedChat_ReturnsZeroAndNil(t *testing.T) {
	priv, _ := auth.GenerateSigningKeypair()
	bot := &Bot{environment: "development", userMapping: map[int64]string{12345: "alice"}}
	m, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      "k1",
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	tok, err := m.MintForChat(99999)
	if err != nil {
		t.Fatalf("dev unmapped MUST NOT error; err=%v", err)
	}
	if tok.WireToken != "" {
		t.Errorf("dev unmapped MUST NOT mint a token; WireToken=%q", tok.WireToken)
	}
	if tok.UserID != "" {
		t.Errorf("dev unmapped UserID=%q want empty", tok.UserID)
	}
}

func TestMintForChat_Dev_MappedChat_StillMintsForCorrectness(t *testing.T) {
	priv, pub := auth.GenerateSigningKeypair()
	bot := &Bot{environment: "test", userMapping: map[int64]string{42: "carol"}}
	m, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      "kdev",
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	tok, err := m.MintForChat(42)
	if err != nil {
		t.Fatalf("dev mapped err=%v want nil", err)
	}
	if tok.UserID != "carol" {
		t.Fatalf("dev mapped UserID=%q want carol", tok.UserID)
	}
	parsed, err := auth.VerifyAndParse(tok.WireToken, auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "kdev",
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Minute,
		Now:                m.now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}
	if parsed.UserID != "carol" {
		t.Errorf("parsed UserID=%q want carol", parsed.UserID)
	}
}

func TestMintForUser_RejectsEmptyUserID(t *testing.T) {
	m, _ := newProductionMinter(t, map[int64]string{12345: "alice"})
	for _, name := range []string{"", "   "} {
		_, err := m.MintForUser(12345, name)
		if err == nil || !strings.Contains(err.Error(), "non-empty user_id") {
			t.Errorf("name=%q err=%v want non-empty user_id", name, err)
		}
	}
}

// TestMintForChat_AdversarialNoBodyTrust proves that no Telegram
// message field other than chat_id can influence the minted UserID.
// The minter API surface only takes (chatID) — there is no field to
// pass message text, sender id, or any other client-controlled value.
// This test pins that contract: even if the test harness wanted to
// inject an attacker-claimed user_id alongside the chat id, the
// minter would NOT accept it.
func TestMintForChat_AdversarialNoBodyTrust(t *testing.T) {
	mapping := map[int64]string{12345: "alice"}
	m, pub := newProductionMinter(t, mapping)
	tok, err := m.MintForChat(12345)
	if err != nil {
		t.Fatalf("MintForChat: %v", err)
	}
	parsed, err := auth.VerifyAndParse(tok.WireToken, auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "scope03-mint-key",
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Minute,
		Now:                m.now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}
	if parsed.UserID != "alice" {
		t.Errorf("PASETO sub=%q want alice (mapping is the ONLY source of truth; no body field was consulted)", parsed.UserID)
	}
}

func TestMintForChat_FreshTokenIDPerCall(t *testing.T) {
	mapping := map[int64]string{12345: "alice"}
	m, _ := newProductionMinter(t, mapping)
	seen := make(map[string]struct{}, 10)
	for i := 0; i < 10; i++ {
		tok, err := m.MintForChat(12345)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if _, dup := seen[tok.TokenID]; dup {
			t.Fatalf("token id %q reused across calls — every mint MUST be fresh", tok.TokenID)
		}
		seen[tok.TokenID] = struct{}{}
	}
}

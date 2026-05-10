// Spec 044 Scope 04 — F02 closure unit test.
//
// Proves Bot.bearerForChat returns the correct bearer per the decision
// matrix in bot.go::bearerForChat (lines 200-238). This is the
// in-package counterpart to tests/integration/auth_telegram_f02_wiring_test.go,
// which exercises the same wiring via SetPerUserTokenMinter through a
// live HTTP router. Both tests are part of the same DoD evidence
// chain for Scope 04 → "F02 wiring landed".
package telegram

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// generateTestSigningKeypair is a tiny in-package wrapper around
// auth.GenerateSigningKeypair so the tests below read clearly.
func generateTestSigningKeypair(t *testing.T) (privHex, pubHex string) {
	t.Helper()
	priv, pub := auth.GenerateSigningKeypair()
	if priv == "" || pub == "" {
		t.Fatal("GenerateSigningKeypair returned empty keys")
	}
	return priv, pub
}

// newTestRequest returns a minimal *http.Request usable as a target
// for setBearerHeader. The URL/method are unimportant for the header
// assertions; we just need a valid request value.
func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	return req
}

// TestBot_bearerForChat_NilMinter_FallsBackToSharedToken proves the
// dev/test path: when no PerUserTokenMinter is wired (legacy single-
// user dev workflow), bearerForChat returns the shared b.authToken
// unchanged. This preserves backward compatibility for solo dev runs.
func TestBot_bearerForChat_NilMinter_FallsBackToSharedToken(t *testing.T) {
	bot := NewBotForTest("dev", map[int64]string{12345: "tg-user-alice"})
	bot.authToken = "legacy-shared-bearer-xyz"

	got, err := bot.bearerForChat(12345)
	if err != nil {
		t.Fatalf("bearerForChat: unexpected err=%v", err)
	}
	if got != "legacy-shared-bearer-xyz" {
		t.Fatalf("bearer=%q want %q", got, "legacy-shared-bearer-xyz")
	}
}

// TestBot_bearerForChat_NilMinter_EmptyAuthToken_ReturnsEmpty proves
// the dev empty-token bypass: when authToken is empty AND no minter
// is wired, bearerForChat returns ("", nil) so the caller knows to
// skip the Authorization header. Auth-disabled dev runs rely on this.
func TestBot_bearerForChat_NilMinter_EmptyAuthToken_ReturnsEmpty(t *testing.T) {
	bot := NewBotForTest("dev", nil)
	// authToken intentionally left at zero value.

	got, err := bot.bearerForChat(99)
	if err != nil {
		t.Fatalf("bearerForChat: unexpected err=%v", err)
	}
	if got != "" {
		t.Fatalf("bearer=%q want empty", got)
	}
}

// TestBot_bearerForChat_WithMinter_MappedChat_ReturnsPerUserPASETO
// proves the production happy path: when a minter is wired AND the
// chat is mapped to a known user, bearerForChat returns a freshly
// minted per-user PASETO (not the shared authToken).
//
// We verify "fresh per-user PASETO" by inspecting that the wire token
// is non-empty AND distinct from any shared b.authToken value (we set
// authToken to a sentinel so the wrong-branch bug would surface).
func TestBot_bearerForChat_WithMinter_MappedChat_ReturnsPerUserPASETO(t *testing.T) {
	bot := NewBotForTest("production", map[int64]string{
		12345: "tg-user-alice",
	})
	// Sentinel — if bearerForChat ever returns this in the mapped
	// + minter-wired path, the bug is "minter ignored, fell through
	// to shared token".
	bot.authToken = "WRONG-shared-bearer-DO-NOT-USE"

	priv, _ := generateTestSigningKeypair(t)
	minter, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      "test-kid-bearerForChat",
		Issuer:     "smackerel",
		TTL:        5 * time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	bot.SetPerUserTokenMinter(minter)

	got, err := bot.bearerForChat(12345)
	if err != nil {
		t.Fatalf("bearerForChat: unexpected err=%v", err)
	}
	if got == "" {
		t.Fatalf("bearer is empty; want a fresh PASETO")
	}
	if got == bot.authToken {
		t.Fatalf("bearer fell back to shared authToken; want fresh PASETO")
	}
	// PASETO v4.public tokens are prefixed with "v4.public.".
	if want := "v4.public."; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("bearer=%q does not look like a v4.public PASETO", got)
	}
}

// TestBot_bearerForChat_WithMinter_DevUnmappedChat_FallsBackToShared
// proves the dev/test fallback: in non-production environments, an
// unmapped chat causes the minter to return (zero, nil) — bearerForChat
// then falls through to the shared b.authToken so the dev flow keeps
// working. This is the behavior contract documented in
// per_user_token.go::MintForChat (lines 161-172).
func TestBot_bearerForChat_WithMinter_DevUnmappedChat_FallsBackToShared(t *testing.T) {
	bot := NewBotForTest("dev", map[int64]string{
		12345: "tg-user-alice",
	})
	bot.authToken = "legacy-shared-bearer-xyz"

	priv, _ := generateTestSigningKeypair(t)
	minter, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      "test-kid-dev-unmapped",
		Issuer:     "smackerel",
		TTL:        5 * time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	bot.SetPerUserTokenMinter(minter)

	got, err := bot.bearerForChat(99999) // unmapped
	if err != nil {
		t.Fatalf("bearerForChat: unexpected err=%v", err)
	}
	if got != "legacy-shared-bearer-xyz" {
		t.Fatalf("bearer=%q want shared authToken (dev fallback)", got)
	}
}

// TestBot_bearerForChat_WithMinter_ProdUnmappedChat_PropagatesError
// proves the production safety contract: in production, an unmapped
// chat MUST error out so the caller drops the request rather than
// attribute the capture to the wrong (or no) user.
//
// This is the F02 closure proof — without it, a Telegram update from
// an unknown chat could silently fall back to the shared bearer and
// land an artifact with an empty session.UserID, defeating the entire
// point of spec 044 claim binding.
func TestBot_bearerForChat_WithMinter_ProdUnmappedChat_PropagatesError(t *testing.T) {
	bot := NewBotForTest("production", map[int64]string{
		12345: "tg-user-alice",
	})
	bot.authToken = "WRONG-shared-bearer-DO-NOT-USE"

	priv, _ := generateTestSigningKeypair(t)
	minter, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      "test-kid-prod-unmapped",
		Issuer:     "smackerel",
		TTL:        5 * time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	bot.SetPerUserTokenMinter(minter)

	got, err := bot.bearerForChat(99999) // unmapped
	if err == nil {
		t.Fatalf("bearerForChat returned bearer=%q for unmapped prod chat; want error", got)
	}
	if !errors.Is(err, ErrNoUserMappingForChat) {
		t.Fatalf("err=%v want ErrNoUserMappingForChat", err)
	}
	if got != "" {
		t.Fatalf("bearer=%q want empty (error path)", got)
	}
}

// TestBot_setBearerHeader_NilMinter_AppliesSharedToken proves the
// helper applied to outbound HTTP requests in the dev/legacy path.
// When no minter is wired, setBearerHeader sets Authorization to
// "Bearer <authToken>" — the historic single-user dev contract.
func TestBot_setBearerHeader_NilMinter_AppliesSharedToken(t *testing.T) {
	bot := NewBotForTest("dev", nil)
	bot.authToken = "legacy-shared-bearer-xyz"

	req := newTestRequest(t)
	if err := bot.setBearerHeader(req, 12345); err != nil {
		t.Fatalf("setBearerHeader: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer legacy-shared-bearer-xyz" {
		t.Fatalf("Authorization=%q want %q", got, "Bearer legacy-shared-bearer-xyz")
	}
}

// TestBot_setBearerHeader_EmptyToken_LeavesHeaderUnset proves the
// dev empty-token bypass: when bearerForChat returns "", setBearerHeader
// MUST NOT set the Authorization header at all (otherwise downstream
// bearerAuthMiddleware would 400 on malformed bearer).
func TestBot_setBearerHeader_EmptyToken_LeavesHeaderUnset(t *testing.T) {
	bot := NewBotForTest("dev", nil)
	// authToken is empty.

	req := newTestRequest(t)
	if err := bot.setBearerHeader(req, 12345); err != nil {
		t.Fatalf("setBearerHeader: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization=%q want unset", got)
	}
}

// TestBot_setBearerHeader_ProdUnmappedChat_PropagatesError proves
// the production safety chain at the helper level: when bearerForChat
// errors (production unmapped chat), setBearerHeader propagates the
// error so the Telegram-bridge caller refuses the outbound HTTP call.
func TestBot_setBearerHeader_ProdUnmappedChat_PropagatesError(t *testing.T) {
	bot := NewBotForTest("production", map[int64]string{
		12345: "tg-user-alice",
	})
	priv, _ := generateTestSigningKeypair(t)
	minter, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      "test-kid-setBearerHeader-err",
		Issuer:     "smackerel",
		TTL:        5 * time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	bot.SetPerUserTokenMinter(minter)

	req := newTestRequest(t)
	if err := bot.setBearerHeader(req, 99999); err == nil {
		t.Fatalf("setBearerHeader: want error for prod unmapped chat; got nil; Authorization=%q",
			req.Header.Get("Authorization"))
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization=%q want unset on error", got)
	}
}

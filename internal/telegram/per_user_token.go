// Spec 044 Scope 03 — Telegram per-user PASETO minter.
//
// The Telegram bot sits behind a single shared bearer token in dev/test
// for legacy reasons (`TELEGRAM_BOT_TOKEN` plus the shared
// `SMACKEREL_AUTH_TOKEN` for internal API calls). In production, that
// model violates the spec 044 claim-binding contract: every captured
// artifact would carry an empty session.UserID, defeating the purpose
// of per-user bearer auth.
//
// `PerUserTokenMinter` closes the residual segment by issuing a real
// PASETO v4.public bearer for the *mapped* user behind a Telegram
// `chat_id`. The bot's HTTP wrapper code can call `MintForChat(chatID)`
// to obtain a freshly-minted bearer, then attach `Authorization: Bearer
// <wire>` on the internal API call. The verifying middleware
// (`bearerAuthMiddleware`) parses the token, attaches an
// `auth.Session{UserID: <mapped>, Source: SessionSourcePerUserToken}`,
// and downstream handlers (capture / annotation) derive `actor_id` and
// `actor_source` from THAT session. A malicious Telegram update payload
// claiming a different actor_id never reaches the persisted artifact
// because:
//
//   1. The chat → user lookup is done by `Bot.resolveActorUserID` on
//      the chat ID alone (no body field is consulted).
//   2. The annotation handler defensively rejects body `actor_source`
//      / `actor_id` smuggling in production (Scope 02 work).
//
// The minter is intentionally minimal:
//
//   - It does NOT cache tokens — every internal call mints a fresh
//     short-lived bearer, eliminating multi-tenant leakage classes
//     (a stale cached token can never be reused for the wrong chat).
//   - It does NOT touch the database — minting is pure crypto and
//     hits the auth signing key in memory.
//   - It does NOT call `Bot.resolveActorUserID` directly to keep the
//     dependency surface small; callers pass the resolved user_id in.
//     The companion `MintForChat` helper performs the resolve+mint in
//     one step for ergonomic use.
package telegram

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// PerUserTokenMinter issues short-lived per-user PASETO bearers on
// behalf of a Telegram-mapped user. Construct via
// `NewPerUserTokenMinter`. The minter is safe for concurrent use; it
// holds only immutable signing material.
type PerUserTokenMinter struct {
	bot        *Bot
	signingKey string
	keyID      string
	issuer     string
	ttl        time.Duration
	now        func() time.Time
}

// PerUserTokenMinterOptions configures a `PerUserTokenMinter`.
type PerUserTokenMinterOptions struct {
	// Bot supplies the chat → user mapping + environment via
	// `Bot.resolveActorUserID`. Required.
	Bot *Bot

	// SigningKey is the active PASETO v4.public private key (hex form).
	// Sourced from `auth.AuthConfig.SigningActivePrivateKey` in
	// production wiring. Required.
	SigningKey string

	// KeyID is the active key identifier embedded in the PASETO
	// footer, allowing the verifier to pick the right public key
	// during rotation. Sourced from `auth.AuthConfig.SigningActiveKeyID`.
	// Required.
	KeyID string

	// Issuer is the claim-`iss` value attached to each minted token.
	// Defaults to "smackerel" when empty.
	Issuer string

	// TTL is the per-token lifetime; short values (e.g. 5–15 minutes)
	// minimize replay risk on the message-handling hot path.
	// Defaults to 5 minutes when zero.
	TTL time.Duration

	// Now is the clock; tests inject a deterministic clock. Defaults
	// to `time.Now` when nil.
	Now func() time.Time
}

// NewPerUserTokenMinter constructs a per-user PASETO minter wired to
// the bot's user mapping. Returns an error when required fields are
// missing.
func NewPerUserTokenMinter(opts PerUserTokenMinterOptions) (*PerUserTokenMinter, error) {
	if opts.Bot == nil {
		return nil, fmt.Errorf("telegram: PerUserTokenMinter requires a non-nil Bot")
	}
	if strings.TrimSpace(opts.SigningKey) == "" {
		return nil, fmt.Errorf("telegram: PerUserTokenMinter requires a non-empty SigningKey")
	}
	if strings.TrimSpace(opts.KeyID) == "" {
		return nil, fmt.Errorf("telegram: PerUserTokenMinter requires a non-empty KeyID")
	}

	issuer := opts.Issuer
	if issuer == "" {
		issuer = "smackerel"
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &PerUserTokenMinter{
		bot:        opts.Bot,
		signingKey: opts.SigningKey,
		keyID:      opts.KeyID,
		issuer:     issuer,
		ttl:        ttl,
		now:        now,
	}, nil
}

// MintedTelegramToken bundles the wire token + claims metadata so the
// caller can attach the `Authorization` header AND log the token id
// for audit traceability. The wire token MUST be transient — the
// bot does not persist it; once the internal API call returns the
// token is forgotten and a fresh one is minted on the next call.
type MintedTelegramToken struct {
	WireToken string    // raw PASETO v4.public token; attach via Authorization: Bearer <WireToken>
	UserID    string    // resolved mapped user_id (PASETO sub claim)
	TokenID   string    // PASETO jti claim — opaque per-mint identifier (audit-only)
	IssuedAt  time.Time // token NotBefore
	ExpiresAt time.Time // token Expiration
	ChatID    int64     // chat the token was minted on behalf of
}

// MintForChat resolves the chat → user mapping then mints a per-user
// PASETO bearer for that user.
//
// Returns:
//   - production + mapped chat → fresh `MintedTelegramToken`, nil
//   - production + UN-mapped chat → zero, `ErrNoUserMappingForChat`
//     (the caller MUST drop the message — same contract as
//     `Bot.resolveActorUserID`)
//   - dev/test + mapped chat → fresh token bound to the mapped user
//   - dev/test + UN-mapped chat → zero, nil — the dev workflow
//     continues to use the shared `SMACKEREL_AUTH_TOKEN` (the
//     caller falls back to that bearer rather than this minter)
//
// In production, an unmapped chat MUST NOT mint a token; downgrading
// to a "synthetic" actor would defeat the spec 044 claim-binding.
func (m *PerUserTokenMinter) MintForChat(chatID int64) (MintedTelegramToken, error) {
	userID, err := m.bot.resolveActorUserID(chatID)
	if err != nil {
		return MintedTelegramToken{}, err
	}
	if userID == "" {
		// Dev/test unmapped chat — caller falls back to the legacy
		// shared bearer. We return (zero, nil) so the caller can
		// distinguish "no per-user surface" from a genuine mint
		// failure.
		return MintedTelegramToken{}, nil
	}
	return m.MintForUser(chatID, userID)
}

// MintForUser issues a short-lived PASETO bearer for an already-
// resolved user_id. Useful for tests that want to control the chat
// → user binding directly. Production callers should prefer
// `MintForChat` so the resolve step runs through the bot's mapping.
func (m *PerUserTokenMinter) MintForUser(chatID int64, userID string) (MintedTelegramToken, error) {
	if strings.TrimSpace(userID) == "" {
		return MintedTelegramToken{}, fmt.Errorf("telegram: MintForUser requires a non-empty user_id")
	}
	tokenID, err := newTelegramTokenID(chatID)
	if err != nil {
		return MintedTelegramToken{}, fmt.Errorf("telegram: generate token id: %w", err)
	}
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    tokenID,
		SigningKey: m.signingKey,
		KeyID:      m.keyID,
		TTL:        m.ttl,
		Issuer:     m.issuer,
		Now:        m.now,
	})
	if err != nil {
		return MintedTelegramToken{}, fmt.Errorf("telegram: mint per-user PASETO: %w", err)
	}
	return MintedTelegramToken{
		WireToken: issued.WireToken,
		UserID:    userID,
		TokenID:   tokenID,
		IssuedAt:  issued.IssuedAt,
		ExpiresAt: issued.ExpiresAt,
		ChatID:    chatID,
	}, nil
}

// newTelegramTokenID returns a fresh opaque PASETO jti for a Telegram
// chat. The shape `tg-<hex>` makes audit-log scans for Telegram-
// originated tokens trivial without leaking any chat content.
func newTelegramTokenID(chatID int64) (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	// chatID is included for audit traceability; it is NOT a secret
	// (Telegram chat ids are routinely shared by operators).
	return fmt.Sprintf("tg-%d-%s", chatID, hex.EncodeToString(raw[:])), nil
}

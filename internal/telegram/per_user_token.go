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
//  1. The chat → user lookup is done by `Bot.resolveActorUserID` on
//     the chat ID alone (no body field is consulted).
//  2. The annotation handler defensively rejects body `actor_source`
//     / `actor_id` smuggling in production (Scope 02 work).
//
// The minter is intentionally minimal:
//
//   - It does NOT cache tokens — every internal call mints a fresh
//     short-lived bearer, eliminating multi-tenant leakage classes
//     (a stale cached token can never be reused for the wrong chat).
//   - It does NOT call `Bot.resolveActorUserID` directly to keep the
//     dependency surface small; callers pass the resolved user_id in.
//     The companion `MintForChat` helper performs the resolve+mint in
//     one step for ergonomic use.
//
// Spec 108 §18 decision 3 (permanent) changed where the minted scope
// claim comes from. It used to be a literal in this file. It is now
// DERIVED from the mapped principal's persisted grant set, narrowed by
// the delegation ceiling `auth.TelegramBridgeDelegableGrants`. That
// makes the mint a database read (one per message, under a 5-minute
// TTL), which is why the mint path carries a context. It also means
// this package holds no scope literal at all — a structural guard
// (`scope_literal_guard_test.go`) fails the build if one reappears.
package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/metrics"
)

// ErrPrincipalGrantsUnrecorded is returned when the mapped principal's
// standing token predates grant recording (`granted_scopes IS NULL`).
// Distinct from "recorded as none" because the operator remedy differs:
// this one is fixed by rotating the principal's token so its grants are
// recorded, not by issuing a different grant set. Spec 108 design.md
// §10.8.
var ErrPrincipalGrantsUnrecorded = errors.New("telegram: mapped principal's grants are unrecorded; rotate the token to record them")

// ErrNoDelegableGrant is returned when the principal's grants ARE
// recorded but none of them is delegable to the bridge — the
// deliberately-demoted ('{}') case and the recorded ∩ ceiling = ∅ case.
// Both deny, and both are permanent operator-actionable conditions
// rather than transient failures, so the reply site MUST NOT render
// them as "try again". Spec 108 design.md §10.8.
var ErrNoDelegableGrant = errors.New("telegram: mapped principal holds no grant the bridge may delegate")

// PrincipalGrantReader reads a principal's recorded grant set without
// possessing its wire token. Satisfied by *auth.BearerStore.
//
// A consumer-side interface rather than a concrete dependency so the
// derivation contract is unit-testable without a database, and so a nil
// reader is a representable, refusable state rather than a panic on the
// message hot path.
type PrincipalGrantReader interface {
	GrantsForPrincipal(ctx context.Context, userID string) (auth.RecordedGrants, error)
}

// PerUserTokenMinter issues short-lived per-user PASETO bearers on
// behalf of a Telegram-mapped user. Construct via
// `NewPerUserTokenMinter`. The minter is safe for concurrent use; it
// holds only immutable signing material.
type PerUserTokenMinter struct {
	bot        *Bot
	grants     PrincipalGrantReader
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

	// PrincipalGrants supplies the mapped principal's persisted grant
	// set, which is the sole authority source for the minted scope
	// claim (spec 108 §18 decision 3). Required — a nil reader fails
	// construction rather than degrading to a literal, because a minter
	// that cannot read grants has no authority to delegate.
	PrincipalGrants PrincipalGrantReader

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
	if opts.PrincipalGrants == nil {
		return nil, fmt.Errorf("telegram: PerUserTokenMinter requires a non-nil PrincipalGrants reader")
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
		grants:     opts.PrincipalGrants,
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
//
// The dev/test unmapped case returns BEFORE any grant read, so the
// legacy single-user workflow never depends on grant readability.
func (m *PerUserTokenMinter) MintForChat(ctx context.Context, chatID int64) (MintedTelegramToken, error) {
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
	return m.MintForUser(ctx, chatID, userID)
}

// deriveGrants resolves the scope claim for a mapped principal.
//
// Spec 108 §18 decision 3: the authority is the PRINCIPAL's persisted
// grant set, never a list held here. `auth.DeriveTelegramBridgeGrants`
// narrows that set to the bridge's delegation ceiling; it can only
// withhold, never confer, so derived ⊆ recorded holds for every input.
//
// Every failure returns (nil, error) so the caller aborts the mint.
// None of them returns a partial or defaulted set: absent grant data
// denies. Spec 108 design.md §10.8.
func (m *PerUserTokenMinter) deriveGrants(ctx context.Context, userID string) ([]string, error) {
	recorded, err := m.grants.GrantsForPrincipal(ctx, userID)
	if err != nil {
		// Wraps auth.ErrPrincipalNotProvisioned and every read error
		// alike; errors.Is at the reply site separates them.
		return nil, fmt.Errorf("telegram: read grants for principal %q: %w", userID, err)
	}
	if !recorded.Recorded {
		return nil, fmt.Errorf("telegram: principal %q (token %q): %w", userID, recorded.TokenID, ErrPrincipalGrantsUnrecorded)
	}
	derived := auth.DeriveTelegramBridgeGrants(recorded.Scopes)
	if len(derived) == 0 {
		return nil, fmt.Errorf("telegram: principal %q (token %q): %w", userID, recorded.TokenID, ErrNoDelegableGrant)
	}
	return derived, nil
}

// MintForUser issues a short-lived PASETO bearer for an already-
// resolved user_id. Useful for tests that want to control the chat
// → user binding directly. Production callers should prefer
// `MintForChat` so the resolve step runs through the bot's mapping.
//
// The scope claim is DERIVED, never defaulted. When derivation fails
// the return is a ZERO MintedTelegramToken plus an error — never a
// scopeless-but-valid token, which would be a credential that passes
// authentication and silently fails every gated call.
func (m *PerUserTokenMinter) MintForUser(ctx context.Context, chatID int64, userID string) (MintedTelegramToken, error) {
	if strings.TrimSpace(userID) == "" {
		return MintedTelegramToken{}, fmt.Errorf("telegram: MintForUser requires a non-empty user_id")
	}
	scopes, err := m.deriveGrants(ctx, userID)
	if err != nil {
		return MintedTelegramToken{}, err
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
		// Derived from the mapped principal's persisted grants. The
		// token is transient and is NEVER written back as the
		// principal's recorded set — IssueToken, not
		// IssueAndPersistToken — because persisting a narrowed set
		// would ratchet the principal's authority down on every
		// message. Spec 108 design.md §10.10 T5.
		Scopes: scopes,
	})
	if err != nil {
		return MintedTelegramToken{}, fmt.Errorf("telegram: mint per-user PASETO: %w", err)
	}

	// Spec 044 Scope 04 — telemetry emission. Telegram-originated
	// per-user tokens are the third issuance source (alongside
	// admin_api and bootstrap_cli); operators monitor the rate to
	// detect Telegram-bridge anomalies.
	metrics.AuthIssuance.WithLabelValues("telegram_bridge").Inc()

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

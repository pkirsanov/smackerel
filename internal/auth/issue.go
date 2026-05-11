package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"aidanwoods.dev/go-paseto"
)

// IssueOptions configures a single token issuance call. Every field is
// REQUIRED; IssueToken refuses to mint a token from a partial spec
// rather than risk silently filling in a Go default that masks an
// upstream configuration bug.
type IssueOptions struct {
	// UserID is the stable principal identifier (auth_users.user_id).
	UserID string

	// TokenID is the auth_tokens.token_id PRIMARY KEY value. Callers
	// generate this (typically a UUIDv7) and persist the row before or
	// after Issue depending on the rotation flow.
	TokenID string

	// SigningKey is the hex-encoded V4 asymmetric secret key (64 bytes
	// raw → 128 hex chars). Generated via GenerateSigningKeypair().
	SigningKey string

	// KeyID is the short identifier embedded in the PASETO footer so
	// that VerifyAndParse can route validation to the active or prior
	// public key during a rotation grace window.
	KeyID string

	// TTL bounds the gap between IssuedAt and ExpiresAt. MUST be > 0.
	TTL time.Duration

	// Issuer is the expected `iss` claim (typically "smackerel"). Set
	// once at runtime construction and threaded through every Issue
	// call so VerifyAndParse can reject foreign-issued tokens.
	Issuer string

	// Now is the injectable clock used to compute IssuedAt/ExpiresAt.
	// Tests pass a deterministic fake; production passes time.Now.
	Now func() time.Time
}

// IssueResult is the PASETO v4.public wire token plus the IssuedAt /
// ExpiresAt timestamps the caller persists into auth_tokens.
type IssueResult struct {
	WireToken string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// IssueToken mints a PASETO v4.public token for the supplied user and
// token id under the configured signing key. The kid is written into
// the footer (JSON object {"kid":"<KeyID>"}) so VerifyAndParse can pick
// the right public key without trusting unauthenticated material.
//
// Spec 044 design.md §5.1 — Issuance flow.
func IssueToken(opts IssueOptions) (IssueResult, error) {
	if opts.UserID == "" {
		return IssueResult{}, errors.New("auth: IssueToken requires UserID")
	}
	if opts.TokenID == "" {
		return IssueResult{}, errors.New("auth: IssueToken requires TokenID")
	}
	if opts.SigningKey == "" {
		return IssueResult{}, errors.New("auth: IssueToken requires SigningKey")
	}
	if opts.KeyID == "" {
		return IssueResult{}, errors.New("auth: IssueToken requires KeyID")
	}
	if opts.Issuer == "" {
		return IssueResult{}, errors.New("auth: IssueToken requires Issuer")
	}
	if opts.TTL <= 0 {
		return IssueResult{}, fmt.Errorf("auth: IssueToken requires positive TTL (got %v)", opts.TTL)
	}
	if opts.Now == nil {
		return IssueResult{}, errors.New("auth: IssueToken requires Now (use time.Now in production)")
	}

	secret, err := paseto.NewV4AsymmetricSecretKeyFromHex(opts.SigningKey)
	if err != nil {
		return IssueResult{}, fmt.Errorf("auth: parse signing key: %w", err)
	}

	now := opts.Now().UTC()
	exp := now.Add(opts.TTL)

	token := paseto.NewToken()
	token.SetIssuer(opts.Issuer)
	token.SetSubject(opts.UserID)
	token.SetJti(opts.TokenID)
	token.SetIssuedAt(now)
	token.SetNotBefore(now)
	token.SetExpiration(exp)
	token.SetFooter([]byte(fmt.Sprintf(`{"kid":%q}`, opts.KeyID)))

	wire := token.V4Sign(secret, nil)
	return IssueResult{
		WireToken: wire,
		IssuedAt:  now,
		ExpiresAt: exp,
	}, nil
}

// GenerateSigningKeypair creates a fresh V4 asymmetric keypair and
// returns the hex-encoded private and public halves. Callers (operator
// CLI: `./smackerel.sh auth keygen`) write the private hex into
// auth.signing.active_private_key and the public hex into
// auth.signing.active_public_key on the operator side. The function
// has no side effects; rotation orchestration lives in cmd_auth.go.
func GenerateSigningKeypair() (privateHex, publicHex string) {
	secret := paseto.NewV4AsymmetricSecretKey()
	return secret.ExportHex(), secret.Public().ExportHex()
}

// PublicHexFromSecretHex derives the V4 asymmetric public-key hex from
// the corresponding hex-encoded private key. Used by tests and by the
// admin admin-rotate flow when only the private hex is in scope (e.g.
// when re-deriving the active public key after loading the SST value).
// Returns an error when the supplied hex does not parse as a V4 secret.
func PublicHexFromSecretHex(privateHex string) (string, error) {
	secret, err := paseto.NewV4AsymmetricSecretKeyFromHex(privateHex)
	if err != nil {
		return "", fmt.Errorf("auth: parse secret key hex: %w", err)
	}
	return secret.Public().ExportHex(), nil
}

// GenerateTokenID produces a 128-bit random token id, hex-encoded.
// PASETO embeds it in the jti claim and the auth_tokens row uses it as
// the PRIMARY KEY. Used by the operator CLI (cmd/core/cmd_auth.go) and
// the admin HTTP handlers (internal/api/auth_handlers.go) so the
// id-generation contract has exactly one definition.
func GenerateTokenID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("auth: rand read: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// IssueAndPersistOptions configures a single mint-and-persist call
// against a BearerStore. Spec 044 — issuance happens from two
// surfaces (the operator CLI in cmd/core/cmd_auth.go and the admin
// HTTP endpoints in internal/api/auth_handlers.go); both flow through
// this helper so the IssueToken + HashToken + PersistToken sequence
// has exactly one definition.
type IssueAndPersistOptions struct {
	// UserID is the principal the token is being issued for.
	UserID string

	// SigningPrivateKey / SigningKeyID are the active PASETO v4.public
	// signing material (auth.signing.active_private_key and
	// auth.signing.active_key_id from SST).
	SigningPrivateKey string
	SigningKeyID      string

	// AtRestHashingKey is auth.at_rest_hashing_key from SST. The
	// minted wire token is HMAC-hashed under this key before being
	// written to auth_tokens.hashed_token.
	AtRestHashingKey string

	// TTL is how long the issued token should remain valid.
	TTL time.Duration

	// Issuer is the `iss` claim (typically "smackerel").
	Issuer string

	// Now is the injectable clock; tests pass a deterministic fake.
	Now func() time.Time

	// IssuedBy is the operator identity recorded in auth_tokens
	// (CLI host, admin user_id, or "bootstrap").
	IssuedBy string

	// IssuedSource is the channel that minted the token — "cli" for
	// the operator subcommand, "admin_api" for the HTTP admin
	// handlers, "bootstrap" for the one-shot first-user enrollment.
	IssuedSource string

	// RotatedFromTokenID is the prior token id when this is a
	// rotation; empty for fresh enrollments.
	RotatedFromTokenID string
}

// IssueAndPersistResult is the wire token (only displayed once at
// mint time) plus the persisted-row metadata callers need to surface
// in their response payload.
type IssueAndPersistResult struct {
	WireToken string
	TokenID   string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// IssueAndPersistToken combines GenerateTokenID + IssueToken +
// HashToken + PersistToken into a single audited operation. Returns
// the wire token and the persisted-row metadata. Validation that
// belongs to each composed operation (positive TTL, non-empty
// SigningKey, etc.) is enforced inside the composed helpers; this
// wrapper validates only the inputs that have no natural home in the
// composed helpers (signing material presence and at-rest hashing
// key presence — the same fail-loud contract spec 044 enforces at
// the SST-loader boundary, repeated here as defense-in-depth).
func IssueAndPersistToken(ctx context.Context, store *BearerStore, opts IssueAndPersistOptions) (IssueAndPersistResult, error) {
	if store == nil {
		return IssueAndPersistResult{}, errors.New("auth: IssueAndPersistToken requires non-nil BearerStore")
	}
	if opts.SigningPrivateKey == "" || opts.SigningKeyID == "" {
		return IssueAndPersistResult{}, errors.New("auth.signing.active_private_key and active_key_id MUST be set to issue tokens")
	}
	if opts.AtRestHashingKey == "" {
		return IssueAndPersistResult{}, errors.New("auth.at_rest_hashing_key MUST be set to persist tokens at rest")
	}

	tokenID, err := GenerateTokenID()
	if err != nil {
		return IssueAndPersistResult{}, err
	}

	issued, err := IssueToken(IssueOptions{
		UserID:     opts.UserID,
		TokenID:    tokenID,
		SigningKey: opts.SigningPrivateKey,
		KeyID:      opts.SigningKeyID,
		TTL:        opts.TTL,
		Issuer:     opts.Issuer,
		Now:        opts.Now,
	})
	if err != nil {
		return IssueAndPersistResult{}, fmt.Errorf("issue token: %w", err)
	}

	hashed, err := HashToken(issued.WireToken, opts.AtRestHashingKey)
	if err != nil {
		return IssueAndPersistResult{}, fmt.Errorf("hash token: %w", err)
	}

	if err := store.PersistToken(ctx, PersistTokenParams{
		TokenID:            tokenID,
		UserID:             opts.UserID,
		KeyID:              opts.SigningKeyID,
		IssuedAt:           issued.IssuedAt,
		ExpiresAt:          issued.ExpiresAt,
		HashedToken:        hashed,
		IssuedBy:           opts.IssuedBy,
		IssuedSource:       opts.IssuedSource,
		RotatedFromTokenID: opts.RotatedFromTokenID,
	}); err != nil {
		return IssueAndPersistResult{}, fmt.Errorf("persist token: %w", err)
	}

	return IssueAndPersistResult{
		WireToken: issued.WireToken,
		TokenID:   tokenID,
		IssuedAt:  issued.IssuedAt,
		ExpiresAt: issued.ExpiresAt,
	}, nil
}

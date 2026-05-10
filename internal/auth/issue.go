package auth

import (
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

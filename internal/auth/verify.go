package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"aidanwoods.dev/go-paseto"
)

// VerifyOptions configures the hot-path verifier. Every non-prior field
// is REQUIRED; the prior fields are paired (both empty or both set)
// because a half-rotation state is a configuration bug.
type VerifyOptions struct {
	// ActivePublicKey is the hex-encoded V4 asymmetric public key that
	// signs newly issued tokens.
	ActivePublicKey string

	// ActiveKeyID is the kid the active key advertises in token footers.
	ActiveKeyID string

	// PriorPublicKey + PriorKeyID may be empty (no rotation in flight).
	// When set they validate tokens minted by the immediately previous
	// signing key during the rotation grace window.
	PriorPublicKey string
	PriorKeyID     string

	// Issuer is the expected `iss` claim value.
	Issuer string

	// ClockSkewTolerance widens iat / nbf / exp acceptance to absorb
	// minor desync between the issuer host and the verifier host.
	// Bounded by NFR-AUTH-005 to ≤ 60 s.
	ClockSkewTolerance time.Duration

	// Now is the injectable clock used to evaluate iat/nbf/exp. Tests
	// pass a deterministic fake; production passes time.Now.
	Now func() time.Time
}

// ParsedToken is the verified claim payload extracted by VerifyAndParse.
// Callers use it to populate a Session before pushing it onto the
// request context.
type ParsedToken struct {
	UserID    string
	TokenID   string
	KeyID     string
	IssuedAt  time.Time
	ExpiresAt time.Time

	// Scopes is the parsed PASETO `scope` claim (spec 060). Nil when
	// the claim is absent (legacy spec-044 token) OR when the claim
	// failed parse-time validation (defense-in-depth — see
	// `getScopeClaim`). NEVER set to a wildcard sentinel.
	Scopes []string
}

// ErrUnknownKeyID is returned when the footer kid does not match either
// the active or prior key id. The verifier refuses to fall back to an
// implicit "try every key" search because a malicious client could
// otherwise enumerate prior keys at low cost.
var ErrUnknownKeyID = errors.New("auth: token kid does not match active or prior signing key")

// ErrTokenExpired is returned when ExpiresAt is in the past (after
// applying ClockSkewTolerance). Distinct from ErrTokenNotYetValid so
// telemetry can label expiry vs clock-skew bugs separately.
var ErrTokenExpired = errors.New("auth: token expired")

// ErrTokenNotYetValid is returned when NotBefore (nbf) is in the future
// (after applying ClockSkewTolerance).
var ErrTokenNotYetValid = errors.New("auth: token not yet valid")

// ErrIssuerMismatch is returned when the iss claim does not equal
// VerifyOptions.Issuer.
var ErrIssuerMismatch = errors.New("auth: token issuer does not match expected value")

// ErrScopeClaimMalformed is returned by getScopeClaim when any
// element of the PASETO `scope` claim fails ScopeNameRegex. Callers
// MUST treat this as the "legacy / no scopes" outcome — NEVER fall
// back to a wildcard. Spec 060 BS-002 adversarial regression.
var ErrScopeClaimMalformed = errors.New("auth: scope claim contains malformed element")

// getScopeClaim reads the PASETO `scope` claim as a []string.
// Behavior:
//
//   - claim absent          → (nil, nil)
//   - claim wrong shape     → (nil, ErrScopeClaimMalformed)
//   - any element bad regex → (nil, ErrScopeClaimMalformed)
//   - all elements valid    → ([]string{...}, nil)
//
// Spec 060 SCN-060-003 — parse-time defense-in-depth.
func getScopeClaim(token *paseto.Token) ([]string, error) {
	var scopes []string
	if err := token.Get("scope", &scopes); err != nil {
		// Distinguish absent claim from malformed shape via the
		// underlying error string. The go-paseto Get returns
		// "value for key `scope' not present in claims" when the
		// claim is missing — that is the legacy-token case.
		if msg := err.Error(); msg != "" && (containsLower(msg, "not present")) {
			return nil, nil
		}
		return nil, ErrScopeClaimMalformed
	}
	for _, s := range scopes {
		if ValidateScopeName(s) != nil {
			return nil, ErrScopeClaimMalformed
		}
	}
	return scopes, nil
}

// containsLower is a small case-insensitive substring helper kept
// local to avoid pulling `strings` into the hot path import set
// (verify.go already lives in the per-request hot path).
func containsLower(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// VerifyAndParse validates a PASETO v4.public wire token and returns
// the parsed claims. Validation steps:
//
//  1. Parse the footer (no signature checking) to extract kid.
//  2. Route to the active or prior public key based on kid.
//  3. Use paseto.NewParserWithoutExpiryCheck so we apply our own
//     ClockSkewTolerance to iat/nbf/exp.
//  4. Verify signature via ParseV4Public (constant-time inside go-paseto).
//  5. Pull iss/sub/jti/iat/exp claims out of the verified token.
//
// Spec 044 design.md §6.1 — Hot-path verifier anatomy.
func VerifyAndParse(wireToken string, opts VerifyOptions) (ParsedToken, error) {
	if opts.ActivePublicKey == "" {
		return ParsedToken{}, errors.New("auth: VerifyAndParse requires ActivePublicKey")
	}
	if opts.ActiveKeyID == "" {
		return ParsedToken{}, errors.New("auth: VerifyAndParse requires ActiveKeyID")
	}
	if opts.Issuer == "" {
		return ParsedToken{}, errors.New("auth: VerifyAndParse requires Issuer")
	}
	if opts.Now == nil {
		return ParsedToken{}, errors.New("auth: VerifyAndParse requires Now (use time.Now in production)")
	}
	if (opts.PriorPublicKey == "") != (opts.PriorKeyID == "") {
		return ParsedToken{}, errors.New("auth: PriorPublicKey and PriorKeyID must both be set or both be empty")
	}

	// Step 1 — extract kid from footer without signature verification.
	// UnsafeParseFooter does not verify the signature; we use the kid
	// only to pick which public key to verify with. The signature check
	// happens in step 4, so a forged footer cannot pass validation.
	parser := paseto.NewParser()
	rawFooter, err := parser.UnsafeParseFooter(paseto.V4Public, wireToken)
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: parse footer: %w", err)
	}
	var footer struct {
		KID string `json:"kid"`
	}
	if len(rawFooter) > 0 {
		if jerr := json.Unmarshal(rawFooter, &footer); jerr != nil {
			return ParsedToken{}, fmt.Errorf("auth: footer is not valid JSON: %w", jerr)
		}
	}
	if footer.KID == "" {
		return ParsedToken{}, errors.New("auth: token footer is missing kid")
	}

	// Step 2 — route to active or prior public key.
	var publicKeyHex string
	switch footer.KID {
	case opts.ActiveKeyID:
		publicKeyHex = opts.ActivePublicKey
	case opts.PriorKeyID:
		if opts.PriorPublicKey == "" {
			return ParsedToken{}, ErrUnknownKeyID
		}
		publicKeyHex = opts.PriorPublicKey
	default:
		return ParsedToken{}, ErrUnknownKeyID
	}

	publicKey, err := paseto.NewV4AsymmetricPublicKeyFromHex(publicKeyHex)
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: parse public key: %w", err)
	}

	// Step 3 — disable built-in expiry checks; we apply our own clock
	// skew tolerance.
	verifier := paseto.NewParserWithoutExpiryCheck()
	token, err := verifier.ParseV4Public(publicKey, wireToken, nil)
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: signature verification failed: %w", err)
	}

	// Step 5 — pull and validate claims.
	issuer, err := token.GetIssuer()
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: read iss: %w", err)
	}
	if issuer != opts.Issuer {
		return ParsedToken{}, ErrIssuerMismatch
	}

	subject, err := token.GetSubject()
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: read sub: %w", err)
	}

	jti, err := token.GetJti()
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: read jti: %w", err)
	}

	iat, err := token.GetIssuedAt()
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: read iat: %w", err)
	}

	nbf, err := token.GetNotBefore()
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: read nbf: %w", err)
	}

	exp, err := token.GetExpiration()
	if err != nil {
		return ParsedToken{}, fmt.Errorf("auth: read exp: %w", err)
	}

	now := opts.Now().UTC()
	tol := opts.ClockSkewTolerance
	if tol < 0 {
		tol = 0
	}

	if exp.Add(tol).Before(now) {
		return ParsedToken{}, ErrTokenExpired
	}
	if nbf.Add(-tol).After(now) {
		return ParsedToken{}, ErrTokenNotYetValid
	}

	// Spec 060 — read optional `scope` claim. Malformed claims are
	// treated as legacy (Scopes: nil) so a forged token can NEVER
	// upgrade itself into a scoped session.
	scopes, scopeErr := getScopeClaim(token)
	if scopeErr != nil {
		slog.Warn("auth: scope claim malformed; treating as legacy token",
			"token_id", jti,
			"key_id", footer.KID)
		scopes = nil
	}

	return ParsedToken{
		UserID:    subject,
		TokenID:   jti,
		KeyID:     footer.KID,
		IssuedAt:  iat,
		ExpiresAt: exp,
		Scopes:    scopes,
	}, nil
}

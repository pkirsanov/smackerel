package qfdecisions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Spec 041 Scope 7 — personal-context consent-token issuance, validation,
// atomic reads_used accounting, and revocation. Tokens are persisted in
// qf_personal_context_consent_tokens (migration 037) so the 5-read cap
// and the ≤ 15 min TTL survive a connector restart mid-window.
//
// Vocabulary (SCN-SM-041-025/026/027):
//   - max_sensitivity_tier ∈ {"low", "medium", "high"}.
//   - expires_at ≤ issued_at + 15m (PersonalContextConsentMaxTTL).
//   - reads_used ≤ PersonalContextConsentMaxReads (= 5).
//   - revoked_at NULL while valid, set when explicitly revoked.

const (
	// PersonalContextConsentMaxTTL is the maximum lifetime of a
	// personal-context consent token (SCN-SM-041-025). Issuance MUST
	// refuse TTLs above this ceiling.
	PersonalContextConsentMaxTTL = 15 * time.Minute

	// PersonalContextConsentMaxReads is the per-token read cap
	// (SCN-SM-041-027). The 6th read attempt is rate-limited.
	PersonalContextConsentMaxReads = 5

	// PersonalContextTierLow / Medium / High are the documented
	// sensitivity-tier vocabulary (SCN-SM-041-026).
	PersonalContextTierLow    = "low"
	PersonalContextTierMedium = "medium"
	PersonalContextTierHigh   = "high"
)

// PersonalContextValidationError carries the documented QF error code
// returned by the route handler for consent-token validation failures.
type PersonalContextValidationError struct {
	Code    string
	Message string
}

func (e PersonalContextValidationError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// Documented QF error codes for the Scope 7 read path
// (SCN-SM-041-026/027).
const (
	PersonalContextErrConsentScopeViolation = "PERSONAL_CONTEXT_CONSENT_SCOPE_VIOLATION"
	PersonalContextErrConsentExpired        = "PERSONAL_CONTEXT_CONSENT_EXPIRED"
	PersonalContextErrConsentCeilingRaised  = "PERSONAL_CONTEXT_CONSENT_CEILING_RAISED"
	PersonalContextErrRateLimitExceeded     = "PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED"
	PersonalContextErrDisabledByCapability  = "PERSONAL_CONTEXT_DISABLED_BY_CAPABILITY"
	PersonalContextErrTokenNotFound         = "PERSONAL_CONTEXT_CONSENT_TOKEN_NOT_FOUND"
	PersonalContextErrTokenRevoked          = "PERSONAL_CONTEXT_CONSENT_REVOKED"
)

// PersonalContextConsentToken is a hydrated token record returned by the
// store after issuance or by AtomicConsumeRead after the reads_used
// increment.
type PersonalContextConsentToken struct {
	TokenID            string
	EntityRef          string
	MaxSensitivityTier string
	RequesterID        string
	IssuedAt           time.Time
	ExpiresAt          time.Time
	ReadsUsed          int
	RevokedAt          *time.Time
}

// PersonalContextConsentTokenStore persists personal-context consent
// tokens in qf_personal_context_consent_tokens (migration 037).
type PersonalContextConsentTokenStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPersonalContextConsentTokenStore returns a store backed by pool. now
// is injected to make TTL assertions deterministic in tests; callers in
// production wire time.Now.
func NewPersonalContextConsentTokenStore(pool *pgxpool.Pool, now func() time.Time) *PersonalContextConsentTokenStore {
	if now == nil {
		now = time.Now
	}
	return &PersonalContextConsentTokenStore{pool: pool, now: now}
}

// PersonalContextConsentIssueRequest is the issuance input.
type PersonalContextConsentIssueRequest struct {
	EntityRef          string
	MaxSensitivityTier string
	RequesterID        string
	TTL                time.Duration
}

// Issue persists a fresh token bound to (entity_ref, max_sensitivity_tier,
// requester_id) and returns the hydrated record. TTLs above
// PersonalContextConsentMaxTTL are refused.
func (s *PersonalContextConsentTokenStore) Issue(ctx context.Context, req PersonalContextConsentIssueRequest) (PersonalContextConsentToken, error) {
	if s == nil || s.pool == nil {
		return PersonalContextConsentToken{}, errors.New("personal-context consent store is not configured")
	}
	entityRef := strings.TrimSpace(req.EntityRef)
	if entityRef == "" {
		return PersonalContextConsentToken{}, errors.New("entity_ref is required")
	}
	tier := strings.TrimSpace(req.MaxSensitivityTier)
	if !isValidPersonalContextTier(tier) {
		return PersonalContextConsentToken{}, fmt.Errorf("max_sensitivity_tier %q is not one of low|medium|high", tier)
	}
	requesterID := strings.TrimSpace(req.RequesterID)
	if requesterID == "" {
		return PersonalContextConsentToken{}, errors.New("requester_id is required")
	}
	ttl := req.TTL
	if ttl <= 0 {
		return PersonalContextConsentToken{}, errors.New("ttl must be positive")
	}
	if ttl > PersonalContextConsentMaxTTL {
		return PersonalContextConsentToken{}, fmt.Errorf("ttl %s exceeds maximum %s", ttl, PersonalContextConsentMaxTTL)
	}
	tokenID, err := newPersonalContextConsentTokenID()
	if err != nil {
		return PersonalContextConsentToken{}, fmt.Errorf("generate token_id: %w", err)
	}
	issuedAt := s.now().UTC()
	expiresAt := issuedAt.Add(ttl)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO qf_personal_context_consent_tokens (
			token_id, entity_ref, max_sensitivity_tier, requester_id,
			issued_at, expires_at, reads_used, revoked_at
		) VALUES ($1, $2, $3, $4, $5, $6, 0, NULL)
	`, tokenID, entityRef, tier, requesterID, issuedAt, expiresAt)
	if err != nil {
		return PersonalContextConsentToken{}, fmt.Errorf("persist consent token: %w", err)
	}
	return PersonalContextConsentToken{
		TokenID:            tokenID,
		EntityRef:          entityRef,
		MaxSensitivityTier: tier,
		RequesterID:        requesterID,
		IssuedAt:           issuedAt,
		ExpiresAt:          expiresAt,
		ReadsUsed:          0,
	}, nil
}

// PersonalContextConsentValidateRequest is the read-time validation input.
type PersonalContextConsentValidateRequest struct {
	TokenID                    string
	EntityRef                  string
	RequestedSensitivityTier   string
	NowSkewToleranceForTesting time.Duration
}

// AtomicConsumeRead validates the token against the request, atomically
// increments reads_used regardless of outcome (so the 5-read cap is
// honored under concurrent reads — SCN-SM-041-027), and returns the
// hydrated token record reflecting the post-increment reads_used. The
// caller decides the HTTP outcome from (token, err):
//
//   - err == nil and token.ReadsUsed <= MaxReads → proceed with read.
//   - err is *PersonalContextValidationError(PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED)
//     when reads_used had already reached MaxReads before this attempt
//     (the increment is still recorded).
//   - err is *PersonalContextValidationError(scope|ceiling|expired|revoked|notfound)
//     for the documented failure matrix; the increment is still recorded
//     so the audit envelope per attempt invariant holds.
func (s *PersonalContextConsentTokenStore) AtomicConsumeRead(ctx context.Context, req PersonalContextConsentValidateRequest) (PersonalContextConsentToken, error) {
	if s == nil || s.pool == nil {
		return PersonalContextConsentToken{}, errors.New("personal-context consent store is not configured")
	}
	tokenID := strings.TrimSpace(req.TokenID)
	if tokenID == "" {
		return PersonalContextConsentToken{}, &PersonalContextValidationError{
			Code:    PersonalContextErrTokenNotFound,
			Message: "consent_token is required",
		}
	}
	entityRef := strings.TrimSpace(req.EntityRef)
	requestedTier := strings.TrimSpace(req.RequestedSensitivityTier)
	if !isValidPersonalContextTier(requestedTier) {
		return PersonalContextConsentToken{}, &PersonalContextValidationError{
			Code:    PersonalContextErrConsentScopeViolation,
			Message: fmt.Sprintf("max_sensitivity %q is not one of low|medium|high", requestedTier),
		}
	}

	// Atomically increment reads_used and return the post-increment row
	// in a single statement so the cap is concurrency-safe. The UPDATE
	// targets every row regardless of TTL/scope so even an expired or
	// scope-mismatched attempt is counted (SCN-SM-041-027 explicitly
	// states the rate-limit counter is bound to the token and EVERY
	// attempt is recorded).
	row := s.pool.QueryRow(ctx, `
		UPDATE qf_personal_context_consent_tokens
		   SET reads_used = reads_used + 1
		 WHERE token_id = $1
		 RETURNING token_id, entity_ref, max_sensitivity_tier, requester_id,
		           issued_at, expires_at, reads_used, revoked_at
	`, tokenID)
	var token PersonalContextConsentToken
	var revokedAt *time.Time
	if err := row.Scan(
		&token.TokenID,
		&token.EntityRef,
		&token.MaxSensitivityTier,
		&token.RequesterID,
		&token.IssuedAt,
		&token.ExpiresAt,
		&token.ReadsUsed,
		&revokedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PersonalContextConsentToken{}, &PersonalContextValidationError{
				Code:    PersonalContextErrTokenNotFound,
				Message: "consent_token is not recognized",
			}
		}
		return PersonalContextConsentToken{}, fmt.Errorf("consume consent_token: %w", err)
	}
	token.RevokedAt = revokedAt

	now := s.now().UTC()

	// Validation order is deterministic so the failure matrix is
	// stable across concurrent paths:
	//   1) revoked → PERSONAL_CONTEXT_CONSENT_REVOKED
	//   2) expired → PERSONAL_CONTEXT_CONSENT_EXPIRED
	//   3) entity_ref mismatch → PERSONAL_CONTEXT_CONSENT_SCOPE_VIOLATION
	//   4) ceiling raised → PERSONAL_CONTEXT_CONSENT_CEILING_RAISED
	//   5) reads_used > MaxReads after the increment → PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED
	if token.RevokedAt != nil {
		return token, &PersonalContextValidationError{
			Code:    PersonalContextErrTokenRevoked,
			Message: "consent_token has been revoked",
		}
	}
	if !token.ExpiresAt.After(now) {
		return token, &PersonalContextValidationError{
			Code:    PersonalContextErrConsentExpired,
			Message: "consent_token has expired",
		}
	}
	if entityRef != "" && entityRef != token.EntityRef {
		return token, &PersonalContextValidationError{
			Code:    PersonalContextErrConsentScopeViolation,
			Message: "entity_ref does not match the consent_token binding",
		}
	}
	if !personalContextTierAtOrBelow(requestedTier, token.MaxSensitivityTier) {
		return token, &PersonalContextValidationError{
			Code:    PersonalContextErrConsentCeilingRaised,
			Message: fmt.Sprintf("requested max_sensitivity %q exceeds consent ceiling %q", requestedTier, token.MaxSensitivityTier),
		}
	}
	if token.ReadsUsed > PersonalContextConsentMaxReads {
		return token, &PersonalContextValidationError{
			Code:    PersonalContextErrRateLimitExceeded,
			Message: fmt.Sprintf("consent_token has exceeded the %d-read cap", PersonalContextConsentMaxReads),
		}
	}
	return token, nil
}

// Revoke marks an outstanding token revoked.
func (s *PersonalContextConsentTokenStore) Revoke(ctx context.Context, tokenID string) error {
	if s == nil || s.pool == nil {
		return errors.New("personal-context consent store is not configured")
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return errors.New("token_id is required")
	}
	now := s.now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE qf_personal_context_consent_tokens
		   SET revoked_at = COALESCE(revoked_at, $2)
		 WHERE token_id = $1
	`, tokenID, now)
	if err != nil {
		return fmt.Errorf("revoke consent_token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &PersonalContextValidationError{
			Code:    PersonalContextErrTokenNotFound,
			Message: "consent_token is not recognized",
		}
	}
	return nil
}

// isValidPersonalContextTier reports whether tier is in the documented
// vocabulary.
func isValidPersonalContextTier(tier string) bool {
	switch tier {
	case PersonalContextTierLow, PersonalContextTierMedium, PersonalContextTierHigh:
		return true
	}
	return false
}

// personalContextTierRank maps the documented tier vocabulary to an
// integer ordering where low < medium < high. It is referenced by the
// consent-store ceiling check and the knowledge-graph sensitivity
// filter so both sides agree on ordering.
func personalContextTierRank(tier string) int {
	switch tier {
	case PersonalContextTierLow:
		return 1
	case PersonalContextTierMedium:
		return 2
	case PersonalContextTierHigh:
		return 3
	}
	return 0
}

// personalContextTierAtOrBelow reports whether candidate is at-or-below
// ceiling in the documented ordering. Both inputs MUST be validated by
// isValidPersonalContextTier first; an invalid tier always returns false.
func personalContextTierAtOrBelow(candidate, ceiling string) bool {
	c := personalContextTierRank(candidate)
	r := personalContextTierRank(ceiling)
	if c == 0 || r == 0 {
		return false
	}
	return c <= r
}

// PersonalContextTierLessOrEqual is the exported alias of
// personalContextTierAtOrBelow consumed by the knowledge-graph
// sensitivity filter and by API handler code that needs the documented
// ordering without re-deriving it.
func PersonalContextTierLessOrEqual(candidate, ceiling string) bool {
	return personalContextTierAtOrBelow(candidate, ceiling)
}

// PersonalContextTierMinimum returns the lower of a and b in the
// documented ordering. Invalid inputs are treated as the most-restrictive
// tier ("low") so a misconfigured ceiling can never grant access above
// what the system actually permits.
func PersonalContextTierMinimum(a, b string) string {
	ra := personalContextTierRank(a)
	rb := personalContextTierRank(b)
	if ra == 0 {
		ra = 1
	}
	if rb == 0 {
		rb = 1
	}
	if ra <= rb {
		return tierForRank(ra)
	}
	return tierForRank(rb)
}

func tierForRank(rank int) string {
	switch rank {
	case 1:
		return PersonalContextTierLow
	case 2:
		return PersonalContextTierMedium
	case 3:
		return PersonalContextTierHigh
	}
	return PersonalContextTierLow
}

// newPersonalContextConsentTokenID generates a 32-byte cryptographically
// random token id encoded as hex. The token_id is the bearer secret
// presented by QF in the `consent_token` query parameter; callers MUST
// transmit it over TLS only.
func newPersonalContextConsentTokenID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "pct_" + hex.EncodeToString(buf), nil
}

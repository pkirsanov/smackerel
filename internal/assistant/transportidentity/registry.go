// Package transportidentity implements the generic
// TransportIdentityRegistry capability foundation introduced in
// spec 072 SCOPE-1 (design §3 "Capability Foundation" + §6 "Data
// Model"). It maps a hashed external transport subject — e.g. the
// HMAC of a WhatsApp E.164 phone number — to a canonical Smackerel
// user_id without ever persisting the raw subject.
//
// The package is transport-neutral: WhatsApp uses
// external_subject_type="phone_e164_hmac" today; future Signal/
// Matrix/RCS adapters reuse the same table with their own subject
// type token.
package transportidentity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Registry is the boundary the assistant transport adapters use to
// resolve an inbound external subject to a canonical Smackerel
// user_id. Returns ErrUnknownSubject on miss; the adapter is
// expected to refuse the inbound message before the facade.
type Registry interface {
	Resolve(ctx context.Context, transport, externalSubjectHash string) (userID string, err error)
}

// ErrUnknownSubject is the closed-vocabulary "no active mapping"
// error. Transport adapters MUST refuse the message before facade
// invocation when Resolve returns ErrUnknownSubject.
var ErrUnknownSubject = errors.New("transportidentity: unknown subject")

// HashPhoneE164 returns the canonical
// external_subject_hash for a WhatsApp inbound: lowercase hex of
// HMAC-SHA256(identityHashKey, normalizedE164). The phone is
// normalized to strict E.164 form (leading '+' followed by digits
// only) before hashing so case/whitespace drift produces the same
// hash. Raw `phone` is NOT logged anywhere.
func HashPhoneE164(identityHashKey, phone string) (string, error) {
	if identityHashKey == "" {
		return "", errors.New("transportidentity: identityHashKey is required")
	}
	normalized, err := normalizeE164(phone)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(identityHashKey))
	_, _ = mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

var e164Re = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

func normalizeE164(phone string) (string, error) {
	trimmed := strings.TrimSpace(phone)
	if !strings.HasPrefix(trimmed, "+") {
		trimmed = "+" + trimmed
	}
	if !e164Re.MatchString(trimmed) {
		// Do NOT echo the raw phone in the error message — leak
		// avoidance per design §8 "Security/Compliance".
		return "", errors.New("transportidentity: phone is not a well-formed E.164 number")
	}
	return trimmed, nil
}

// PgRegistry implements Registry against PostgreSQL.
type PgRegistry struct {
	pool *pgxpool.Pool
}

// NewPgRegistry returns a PgRegistry that uses the supplied pool.
func NewPgRegistry(pool *pgxpool.Pool) *PgRegistry {
	if pool == nil {
		panic("transportidentity: NewPgRegistry requires a non-nil pool")
	}
	return &PgRegistry{pool: pool}
}

// Resolve returns the canonical user_id for an active mapping or
// ErrUnknownSubject. Disabled mappings are treated as unknown to
// avoid leaking provisioning state to a public webhook caller.
func (r *PgRegistry) Resolve(ctx context.Context, transport, externalSubjectHash string) (string, error) {
	if transport == "" || externalSubjectHash == "" {
		return "", errors.New("transportidentity: Resolve requires non-empty transport and externalSubjectHash")
	}
	const q = `
SELECT user_id
  FROM assistant_transport_identities
 WHERE transport = $1
   AND external_subject_hash = $2
   AND status = 'active'
`
	var userID string
	err := r.pool.QueryRow(ctx, q, transport, externalSubjectHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnknownSubject
	}
	if err != nil {
		return "", fmt.Errorf("transportidentity: Resolve: %w", err)
	}
	if userID == "" {
		return "", ErrUnknownSubject
	}
	return userID, nil
}

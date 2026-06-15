// Package modelpref — Spec 089 (Fork B): the per-user STICKY model
// preference store for the open-knowledge /ask agent.
//
// This is the first GENERAL per-user preference store in the product. A user
// sets a sticky synthesis model once (Telegram /model <id> or HTTP
// PUT /v1/agent/model); it then applies to that user's subsequent /ask
// invocations (precedence: per-request override > THIS sticky > the SST
// persistent default) until changed or reset.
//
// CLAIM-BOUND (spec 044 / OWASP A01): every method keys ONLY on the
// authenticated actor_user_id the caller threads — the Telegram
// Bot.resolveActorUserID(chatID) subject or the HTTP PASETO bearer subject
// (auth.UserIDFromContext). A request-body user id can never reach the key.
// One user can never read or write another user's preference.
//
// The store is a pure leaf (pgx only, no agent/facade import) so it can be
// reached by both surfaces (Telegram facade + HTTP api) without an import
// cycle, mirroring the spec-088 modelswitch + the intenttrace.PostgresStore
// shape (internal/assistant/intenttrace/recorder.go).
package modelpref

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Preference is a user's sticky open-knowledge /ask synthesis selection. The
// gather_model column (migration 059) is RESERVED for F-STICKY-GATHER and is
// deliberately NOT surfaced here — the spec-089 resolver does not read it, so
// no sticky gather can be silently activated.
type Preference struct {
	SynthesisModel string
	UpdatedAt      time.Time
}

// Store is the claim-bound per-user sticky-preference capability consumed
// identically by both surfaces. The caller MUST pass the authenticated
// principal as userID; the store never derives identity from a request body.
type Store interface {
	// Get returns the user's sticky preference. ok=false ⇒ no row ⇒ the caller
	// inherits the SST persistent default (NEVER a hardcoded fallback).
	Get(ctx context.Context, userID string) (Preference, bool, error)
	// Set upserts the user's sticky synthesis model (one row per user, the
	// ON CONFLICT (actor_user_id) DO UPDATE path).
	Set(ctx context.Context, userID, synthesisModel string) error
	// Clear deletes the user's preference. Idempotent — an absent row is a
	// no-op (no error), so /model default is safe to repeat.
	Clear(ctx context.Context, userID string) error
}

// PostgresStore implements Store against user_model_preferences (migration
// 059). The single-PK Get is O(1) on the /ask hot path — negligible against a
// multi-minute /ask, so no cache (don't over-engineer).
type PostgresStore struct {
	Pool *pgxpool.Pool
	now  func() time.Time
}

// NewPostgresStore constructs a PostgresStore using the wall clock.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{Pool: pool, now: time.Now}
}

// WithNow overrides the clock (tests use a fixed instant to assert
// updated_at advances on upsert). Returns the receiver for chaining.
func (s *PostgresStore) WithNow(now func() time.Time) *PostgresStore {
	s.now = now
	return s
}

const getSQL = `
SELECT synthesis_model, updated_at
FROM user_model_preferences
WHERE actor_user_id = $1
`

// Get implements Store. The WHERE actor_user_id = $1 binding is the
// claim-binding boundary: it can only ever return THIS user's row.
func (s *PostgresStore) Get(ctx context.Context, userID string) (Preference, bool, error) {
	if s == nil || s.Pool == nil {
		return Preference{}, false, errors.New("modelpref: PostgresStore requires a non-nil Pool")
	}
	if userID == "" {
		return Preference{}, false, errors.New("modelpref: Get requires a non-empty claim-bound userID")
	}
	var p Preference
	err := s.Pool.QueryRow(ctx, getSQL, userID).Scan(&p.SynthesisModel, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preference{}, false, nil
	}
	if err != nil {
		return Preference{}, false, fmt.Errorf("modelpref: get preference for user: %w", err)
	}
	return p, true, nil
}

const setSQL = `
INSERT INTO user_model_preferences (actor_user_id, synthesis_model, updated_at)
VALUES ($1, $2, $3)
ON CONFLICT (actor_user_id) DO UPDATE
SET synthesis_model = EXCLUDED.synthesis_model,
    updated_at      = EXCLUDED.updated_at
`

// Set implements Store. Upsert keyed on actor_user_id → exactly one row per
// user; a second Set overwrites (no duplicate, no cross-user write).
func (s *PostgresStore) Set(ctx context.Context, userID, synthesisModel string) error {
	if s == nil || s.Pool == nil {
		return errors.New("modelpref: PostgresStore requires a non-nil Pool")
	}
	if userID == "" {
		return errors.New("modelpref: Set requires a non-empty claim-bound userID")
	}
	if synthesisModel == "" {
		return errors.New("modelpref: Set requires a non-empty synthesisModel (no silent default)")
	}
	if _, err := s.Pool.Exec(ctx, setSQL, userID, synthesisModel, s.now().UTC()); err != nil {
		return fmt.Errorf("modelpref: set preference for user: %w", err)
	}
	return nil
}

const clearSQL = `DELETE FROM user_model_preferences WHERE actor_user_id = $1`

// Clear implements Store. Idempotent: deleting an absent row affects zero rows
// and returns no error, so /model default is safe to repeat.
func (s *PostgresStore) Clear(ctx context.Context, userID string) error {
	if s == nil || s.Pool == nil {
		return errors.New("modelpref: PostgresStore requires a non-nil Pool")
	}
	if userID == "" {
		return errors.New("modelpref: Clear requires a non-empty claim-bound userID")
	}
	if _, err := s.Pool.Exec(ctx, clearSQL, userID); err != nil {
		return fmt.Errorf("modelpref: clear preference for user: %w", err)
	}
	return nil
}

// Static assertion that PostgresStore satisfies Store.
var _ Store = (*PostgresStore)(nil)

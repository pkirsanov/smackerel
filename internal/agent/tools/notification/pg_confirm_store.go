// Spec 061 SCOPE-08 — minimal PostgreSQL-backed notification.ConfirmStore.
//
// The notification tool's ConfirmStore (defined in services.go) is
// keyed by `ref` only (not user_id/transport) — it holds the opaque
// payload between the propose-roundtrip and the eventual
// notification_execute call. This is intentionally separate from the
// facade-side `assistant_conversations.pending_confirm` row owned by
// `internal/assistant/confirm.Machine`; the two storage paths serve
// different lookup patterns per design §5.4 + §6.3.
//
// This implementation uses a tiny `assistant_confirm_pending` table
// (migration 043) so payload survival is durable across core
// restarts. TTL is enforced by ExpiresAt + a SELECT-time filter; an
// out-of-band sweep MAY periodically DELETE expired rows but is not
// required for correctness because lookups already filter.
//
// The file is placed alongside the rest of the notification package
// (NOT under internal/assistant/) because it implements an interface
// declared in that package.

package notification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgConfirmStore implements ConfirmStore against PostgreSQL.
type PgConfirmStore struct {
	pool *pgxpool.Pool
}

// NewPgConfirmStore constructs a PG-backed ConfirmStore. Panics on
// nil pool.
func NewPgConfirmStore(pool *pgxpool.Pool) *PgConfirmStore {
	if pool == nil {
		panic("notification: NewPgConfirmStore requires non-nil pool")
	}
	return &PgConfirmStore{pool: pool}
}

// Put inserts or replaces the pending-confirm payload for ref. TTL
// is converted to an absolute ExpiresAt at write-time so server-side
// expiry checks are deterministic regardless of caller clock drift.
func (s *PgConfirmStore) Put(ctx context.Context, ref string, payload string, ttl time.Duration) error {
	if ref == "" {
		return errors.New("notification.PgConfirmStore.Put: ref required")
	}
	if payload == "" {
		return errors.New("notification.PgConfirmStore.Put: payload required")
	}
	if ttl <= 0 {
		return errors.New("notification.PgConfirmStore.Put: ttl must be > 0")
	}
	const q = `
INSERT INTO assistant_confirm_pending (confirm_ref, payload, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (confirm_ref) DO UPDATE
   SET payload    = EXCLUDED.payload,
       expires_at = EXCLUDED.expires_at
`
	expiresAt := time.Now().UTC().Add(ttl)
	if _, err := s.pool.Exec(ctx, q, ref, payload, expiresAt); err != nil {
		return fmt.Errorf("notification.PgConfirmStore.Put: %w", err)
	}
	return nil
}

// Get returns the stored payload for ref if present and not expired.
// Per the ConfirmStore contract, returns ("", false, nil) on miss or
// expiry.
func (s *PgConfirmStore) Get(ctx context.Context, ref string) (string, bool, error) {
	if ref == "" {
		return "", false, errors.New("notification.PgConfirmStore.Get: ref required")
	}
	const q = `
SELECT payload
  FROM assistant_confirm_pending
 WHERE confirm_ref = $1
   AND expires_at > NOW()
`
	var payload string
	err := s.pool.QueryRow(ctx, q, ref).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("notification.PgConfirmStore.Get: %w", err)
	}
	return payload, true, nil
}

// Delete removes the pending row regardless of expiry. Used by
// notification_execute after successfully scheduling so the same ref
// cannot drive a second schedule.
func (s *PgConfirmStore) Delete(ctx context.Context, ref string) error {
	if ref == "" {
		return errors.New("notification.PgConfirmStore.Delete: ref required")
	}
	const q = `DELETE FROM assistant_confirm_pending WHERE confirm_ref = $1`
	if _, err := s.pool.Exec(ctx, q, ref); err != nil {
		return fmt.Errorf("notification.PgConfirmStore.Delete: %w", err)
	}
	return nil
}

package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BearerStore is the DB-backed accessor for the spec 044 per-user bearer
// auth tables (auth_users, auth_tokens, auth_revocations). It is
// deliberately small in Scope 01 — only the operations the CLI
// (cmd_auth.go), the admin HTTP handlers (auth_handlers.go), and the
// revocation cache bootstrap need. Hot-path verification does NOT use
// this store directly; it consults the in-process revocation cache
// (internal/auth/revocation) instead.
type BearerStore struct {
	pool *pgxpool.Pool
}

// NewBearerStore constructs a BearerStore around the supplied pool.
// Returns an error when pool is nil to refuse silent dev-mode no-ops.
func NewBearerStore(pool *pgxpool.Pool) (*BearerStore, error) {
	if pool == nil {
		return nil, errors.New("auth: NewBearerStore requires non-nil *pgxpool.Pool")
	}
	return &BearerStore{pool: pool}, nil
}

// EnrolledUser is the row shape returned by ListUsers and CountUsers'
// detail variant. Token lifecycle data is intentionally NOT joined in
// here because the admin list view in Scope 01 only needs principal-
// level metadata; per-token detail comes from a future scope.
type EnrolledUser struct {
	UserID     string
	EnrolledAt time.Time
	EnrolledBy string
	Status     string
	Notes      string
}

// EnrollUserParams is the input for Enroll. user_id is the caller-
// chosen stable identifier; enrolled_by is the principal that
// authorized the enrollment (admin user_id or "bootstrap").
type EnrollUserParams struct {
	UserID     string
	EnrolledBy string
	Notes      string
}

// Enroll inserts a new auth_users row. Returns an error wrapping
// pgx.ErrNoRows-style sentinel when user_id collides — callers should
// check for the unique-violation prefix in the error string.
func (s *BearerStore) Enroll(ctx context.Context, p EnrollUserParams) error {
	if p.UserID == "" {
		return errors.New("auth: Enroll requires UserID")
	}
	if p.EnrolledBy == "" {
		return errors.New("auth: Enroll requires EnrolledBy")
	}
	_, err := s.pool.Exec(ctx, `
        INSERT INTO auth_users (user_id, enrolled_by, notes)
        VALUES ($1, $2, $3)
    `, p.UserID, p.EnrolledBy, p.Notes)
	if err != nil {
		return fmt.Errorf("auth: enroll user %q: %w", p.UserID, err)
	}
	return nil
}

// CountUsers returns the number of rows in auth_users. Used by the
// startup validator (cmd/core/wiring.go) to decide whether the
// production-mode bootstrap-token check applies.
func (s *BearerStore) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM auth_users`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("auth: count users: %w", err)
	}
	return n, nil
}

// ListUsers returns every enrolled user ordered by enrolled_at ascending.
func (s *BearerStore) ListUsers(ctx context.Context) ([]EnrolledUser, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT user_id, enrolled_at, enrolled_by, status, notes
        FROM auth_users
        ORDER BY enrolled_at ASC, user_id ASC
    `)
	if err != nil {
		return nil, fmt.Errorf("auth: list users: %w", err)
	}
	defer rows.Close()

	var out []EnrolledUser
	for rows.Next() {
		var u EnrolledUser
		if scanErr := rows.Scan(&u.UserID, &u.EnrolledAt, &u.EnrolledBy, &u.Status, &u.Notes); scanErr != nil {
			return nil, fmt.Errorf("auth: scan user row: %w", scanErr)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterate users: %w", err)
	}
	return out, nil
}

// PersistTokenParams is the input for PersistToken. The hashed_token
// value is the HMAC-SHA-256 of the wire token under
// auth.at_rest_hashing_key (computed by HashToken before this call).
// IssuedSource is the channel that minted the token — "cli" for the
// operator subcommand, "admin_api" for the HTTP admin handlers, and
// "bootstrap" for the one-shot first-user enrollment.
type PersistTokenParams struct {
	TokenID            string
	UserID             string
	KeyID              string
	IssuedAt           time.Time
	ExpiresAt          time.Time
	HashedToken        string
	IssuedBy           string
	IssuedSource       string
	RotatedFromTokenID string
}

// PersistToken inserts a new auth_tokens row.
func (s *BearerStore) PersistToken(ctx context.Context, p PersistTokenParams) error {
	if p.TokenID == "" || p.UserID == "" || p.KeyID == "" || p.HashedToken == "" || p.IssuedBy == "" || p.IssuedSource == "" {
		return errors.New("auth: PersistToken requires TokenID, UserID, KeyID, HashedToken, IssuedBy, IssuedSource")
	}
	if p.IssuedAt.IsZero() || p.ExpiresAt.IsZero() {
		return errors.New("auth: PersistToken requires non-zero IssuedAt and ExpiresAt")
	}
	if !p.ExpiresAt.After(p.IssuedAt) {
		return fmt.Errorf("auth: PersistToken requires ExpiresAt > IssuedAt (got iat=%v exp=%v)", p.IssuedAt, p.ExpiresAt)
	}

	var rotatedFrom interface{}
	if p.RotatedFromTokenID != "" {
		rotatedFrom = p.RotatedFromTokenID
	} else {
		rotatedFrom = nil
	}

	_, err := s.pool.Exec(ctx, `
        INSERT INTO auth_tokens (
            token_id, user_id, key_id, issued_at, expires_at,
            hashed_token, issued_by, issued_source, rotated_from_token_id
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `, p.TokenID, p.UserID, p.KeyID, p.IssuedAt, p.ExpiresAt,
		p.HashedToken, p.IssuedBy, p.IssuedSource, rotatedFrom)
	if err != nil {
		return fmt.Errorf("auth: persist token %q: %w", p.TokenID, err)
	}
	return nil
}

// MarkTokenRotated transitions auth_tokens.status to 'rotated' for the
// supplied token_id. Used by the rotate flow once a fresh token has
// been minted; the prior token continues to validate until exp inside
// the rotation grace window.
func (s *BearerStore) MarkTokenRotated(ctx context.Context, tokenID string) error {
	if tokenID == "" {
		return errors.New("auth: MarkTokenRotated requires tokenID")
	}
	res, err := s.pool.Exec(ctx, `
        UPDATE auth_tokens SET status = 'rotated'
        WHERE token_id = $1 AND status = 'active'
    `, tokenID)
	if err != nil {
		return fmt.Errorf("auth: mark token rotated: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("auth: no active token with id %q to rotate", tokenID)
	}
	return nil
}

// ErrTokenNotFound is returned by RevokeToken when no auth_tokens row
// matches the supplied token_id. Distinct from the "already revoked"
// no-op so Scope 02 follow-up adversarial tests can assert each shape
// independently and so admin-API callers can surface a clean 404 rather
// than misclassifying a missing token as a permission failure.
var ErrTokenNotFound = errors.New("auth: token id not found")

// RevokeToken atomically writes both the auth_revocations row and the
// auth_tokens.status='revoked' transition inside a single transaction
// so the bootstrap cache cannot observe a half-applied revocation.
//
// Contract refinement (spec 044 Scope 02 follow-up): the function
// distinguishes three outcomes for the same input space the prior
// implementation collapsed:
//
//  1. The token does not exist → returns ErrTokenNotFound (wrapped with
//     the token id for log clarity).
//  2. The token exists and is already 'revoked' → returns nil
//     (idempotent — repeated revoke calls are a no-op so operator
//     retries and crash-restart loops never error out a second time).
//  3. The token exists and is 'active' or 'rotated' → updates the
//     status row, inserts the auth_revocations audit row, commits.
//
// The (1)/(2) split is enforced via a SELECT ... FOR UPDATE inside the
// transaction so a concurrent revoker cannot race a status check past
// the commit boundary.
func (s *BearerStore) RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	if tokenID == "" || revokedBy == "" {
		return errors.New("auth: RevokeToken requires tokenID and revokedBy")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("auth: begin revoke tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the row (or surface ErrTokenNotFound when absent) so the
	// (not-found / already-revoked / active) decision is made under
	// transactional isolation.
	var status string
	err = tx.QueryRow(ctx, `
        SELECT status FROM auth_tokens
        WHERE token_id = $1
        FOR UPDATE
    `, tokenID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("auth: revoke %q: %w", tokenID, ErrTokenNotFound)
		}
		return fmt.Errorf("auth: revoke status lookup: %w", err)
	}

	if status == "revoked" {
		// Idempotent — the canonical revocation is already in place.
		// Commit the (empty) transaction so the FOR UPDATE lock
		// releases cleanly.
		if cerr := tx.Commit(ctx); cerr != nil {
			return fmt.Errorf("auth: commit idempotent revoke tx: %w", cerr)
		}
		return nil
	}

	if _, err := tx.Exec(ctx, `
        UPDATE auth_tokens SET status = 'revoked'
        WHERE token_id = $1
    `, tokenID); err != nil {
		return fmt.Errorf("auth: revoke status update: %w", err)
	}

	if _, err := tx.Exec(ctx, `
        INSERT INTO auth_revocations (token_id, revoked_by, reason)
        VALUES ($1, $2, $3)
        ON CONFLICT (token_id) DO NOTHING
    `, tokenID, revokedBy, reason); err != nil {
		return fmt.Errorf("auth: insert revocation row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("auth: commit revoke tx: %w", err)
	}
	return nil
}

// LoadRevokedTokenIDs returns every token_id currently considered
// revoked. Used by the in-process revocation cache to bootstrap on
// startup and to refresh periodically as the failure-mode backstop
// when the NATS broadcast channel is down. The query unions over
// auth_revocations AND auth_tokens.status='revoked' so a row that
// landed in either side first is still included.
func (s *BearerStore) LoadRevokedTokenIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT token_id FROM auth_revocations
        UNION
        SELECT token_id FROM auth_tokens WHERE status = 'revoked'
    `)
	if err != nil {
		return nil, fmt.Errorf("auth: load revoked token ids: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("auth: scan revoked id: %w", scanErr)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterate revoked ids: %w", err)
	}
	return out, nil
}

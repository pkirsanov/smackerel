// Package webinvite implements the spec 093 admin-generated, single-use,
// DB-backed registration-invite layer: SHA-256 hashing of high-entropy
// invite tokens + a Postgres-backed repo for the web_registration_invites
// table (generate / is-live gate read / atomic consume-and-create / list /
// revoke).
//
// It mirrors the internal/auth/webcreds shape (interface + PostgresRepo +
// NewPostgresRepo(nil)→error nil-guard + a hash-excluded projection) but is
// deliberately decoupled from webcreds: the account-create step is injected
// into ConsumeAndCreate as a func(ctx, pgx.Tx) error callback, so the two
// packages never import each other and the atomic boundary stays owned by
// the repo that owns the pool.
//
// SECURITY: the plaintext invite leaves the process exactly ONCE — as the
// return value of Generate. Only its lowercase-hex SHA-256 (token_hash) is
// stored; List never selects or exposes token_hash; nothing here logs the
// plaintext. Single-use is enforced by a guarded UPDATE ... RETURNING id
// (the TOCTOU authority — mirrors migration 032), not a check-then-act read.
package webinvite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InviteStatus is the derived lifecycle state (never stored — computed in Go
// from used_at / revoked_at / expires_at against now()).
type InviteStatus string

const (
	StatusOutstanding InviteStatus = "outstanding"
	StatusUsed        InviteStatus = "used"
	StatusExpired     InviteStatus = "expired"
	StatusRevoked     InviteStatus = "revoked"
)

// InviteRow is the metadata-only projection for List — NO token_hash, NO
// plaintext. Mirrors webcreds.UserRow excluding password_hash.
type InviteRow struct {
	ID        string
	Label     *string
	CreatedBy string
	CreatedAt time.Time
	ExpiresAt *time.Time
	UsedAt    *time.Time
	UsedBy    *string
	RevokedAt *time.Time
	Status    InviteStatus // derived at scan time from now()
}

// ConsumeOutcome is the result of ConsumeAndCreate.
type ConsumeOutcome int

const (
	// ConsumeInvalid — unknown/used/expired/revoked (incl. a lost race): the
	// guarded UPDATE claimed nothing, the invite is untouched.
	ConsumeInvalid ConsumeOutcome = iota
	// ConsumeCreated — the invite was claimed AND onClaimed committed: success.
	ConsumeCreated
	// ConsumeRolledBack — a valid invite was claimed in-tx but onClaimed failed,
	// so the whole tx (claim + account insert) was rolled back; see the err.
	ConsumeRolledBack
)

// RevokeOutcome distinguishes a real transition from a no-op (stale-page race).
type RevokeOutcome int

const (
	// RevokeNoop — already used/revoked (or unknown id): nothing to do.
	RevokeNoop RevokeOutcome = iota
	// RevokeDone — OUTSTANDING → REVOKED.
	RevokeDone
)

// Repo is the storage interface for registration invites.
type Repo interface {
	// Generate mints a high-entropy token, stores ONLY its SHA-256 (hex) +
	// metadata, and returns the one-time PLAINTEXT to the caller. ttl>0 ⇒
	// expires_at=now()+ttl; ttl<=0 ⇒ NULL (never expires).
	Generate(ctx context.Context, createdBy, label string, ttl time.Duration) (plaintext string, err error)

	// IsLive reports whether tokenHash names an OUTSTANDING invite
	// (non-mutating gate read). Not the single-use authority.
	IsLive(ctx context.Context, tokenHash string) (bool, error)

	// ConsumeAndCreate atomically claims the invite (guarded UPDATE ...
	// RETURNING id) and runs onClaimed within the SAME tx. Commit ⇔ both
	// succeed. See ConsumeOutcome.
	ConsumeAndCreate(ctx context.Context, tokenHash, usedBy string,
		onClaimed func(ctx context.Context, tx pgx.Tx) error) (ConsumeOutcome, error)

	// List returns metadata-only rows (newest first). NEVER token_hash, NEVER
	// plaintext.
	List(ctx context.Context) ([]InviteRow, error)

	// Revoke sets revoked_at on an OUTSTANDING invite. Guarded so a
	// used/revoked/unknown id is a no-op.
	Revoke(ctx context.Context, id string) (RevokeOutcome, error)
}

// HashToken returns the lowercase-hex SHA-256 of the plaintext (the at-rest
// identifier). Exported so internal/api's /register gate can hash a submitted
// token for IsLive / ConsumeAndCreate. The hash covers the WHOLE string
// (including the inv_ prefix).
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// newPlaintextToken mints a high-entropy invite token: "inv_" followed by the
// URL-safe, unpadded base64 of 32 cryptographically-random bytes (≥256 bits).
// Factored out so the token format is unit-testable without a database.
func newPlaintextToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("webinvite: read random token: %w", err)
	}
	return "inv_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

// PostgresRepo is the pgx-backed Repo.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// Compile-time assertion that PostgresRepo satisfies Repo.
var _ Repo = (*PostgresRepo)(nil)

// NewPostgresRepo constructs a PostgresRepo. Returns an error when pool is nil
// to refuse silent no-op dev behavior (mirrors webcreds.NewPostgresRepo).
func NewPostgresRepo(pool *pgxpool.Pool) (*PostgresRepo, error) {
	if pool == nil {
		return nil, errors.New("webinvite: NewPostgresRepo requires non-nil *pgxpool.Pool")
	}
	return &PostgresRepo{pool: pool}, nil
}

// Generate implements Repo. The plaintext is returned to the caller and is the
// ONLY place it ever leaves the process; the DB stores only its hash.
func (r *PostgresRepo) Generate(ctx context.Context, createdBy, label string, ttl time.Duration) (string, error) {
	if createdBy == "" {
		return "", errors.New("webinvite: Generate requires a non-empty createdBy")
	}
	var expires *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expires = &t
	}
	var labelArg any
	if label != "" {
		labelArg = label
	}

	// Up to two attempts to dodge the ~never token_hash collision: regenerate
	// once on a unique violation, then surface the error.
	for attempt := 0; attempt < 2; attempt++ {
		plaintext, err := newPlaintextToken()
		if err != nil {
			return "", err
		}
		hash := HashToken(plaintext)
		_, err = r.pool.Exec(ctx,
			`INSERT INTO web_registration_invites (token_hash, label, created_by, expires_at)
			 VALUES ($1, $2, $3, $4)`,
			hash, labelArg, createdBy, expires,
		)
		if err == nil {
			return plaintext, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue // astronomically rare hash collision — regenerate once
		}
		return "", fmt.Errorf("webinvite: insert invite: %w", err)
	}
	return "", errors.New("webinvite: could not generate a unique invite token after retry")
}

// IsLive implements Repo (non-mutating gate read).
func (r *PostgresRepo) IsLive(ctx context.Context, tokenHash string) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx,
		`SELECT 1 FROM web_registration_invites
		  WHERE token_hash = $1
		    AND used_at    IS NULL
		    AND revoked_at IS NULL
		    AND (expires_at IS NULL OR expires_at > now())`,
		tokenHash,
	).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("webinvite: is-live check: %w", err)
	}
	return true, nil
}

// ConsumeAndCreate implements Repo. The guarded UPDATE ... RETURNING id is the
// single-use authority; onClaimed runs on the SAME tx, so the account insert
// and the invite claim commit or roll back together.
func (r *PostgresRepo) ConsumeAndCreate(ctx context.Context, tokenHash, usedBy string,
	onClaimed func(ctx context.Context, tx pgx.Tx) error) (ConsumeOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ConsumeInvalid, fmt.Errorf("webinvite: begin tx: %w", err)
	}
	// Deferred rollback is a no-op after a successful Commit; on every error
	// path it unwinds both the claim and the account insert.
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx,
		`UPDATE web_registration_invites
		    SET used_at = now(), used_by = $2
		  WHERE token_hash = $1
		    AND used_at    IS NULL
		    AND revoked_at IS NULL
		    AND (expires_at IS NULL OR expires_at > now())
		 RETURNING id`,
		tokenHash, usedBy,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// Unknown / used / expired / revoked, or lost the race after IsLive.
		return ConsumeInvalid, nil
	}
	if err != nil {
		return ConsumeInvalid, fmt.Errorf("webinvite: claim invite: %w", err)
	}

	if err := onClaimed(ctx, tx); err != nil {
		// Valid invite claimed in-tx but the account create failed — roll the
		// WHOLE tx back (the deferred rollback fires), so the invite stays
		// unconsumed and can be retried.
		return ConsumeRolledBack, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ConsumeRolledBack, fmt.Errorf("webinvite: commit consume: %w", err)
	}
	return ConsumeCreated, nil
}

// List implements Repo (metadata only; token_hash is NEVER selected).
func (r *PostgresRepo) List(ctx context.Context) ([]InviteRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, label, created_by, created_at, expires_at, used_at, used_by, revoked_at
		   FROM web_registration_invites
		  ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("webinvite: list invites: %w", err)
	}
	defer rows.Close()
	now := time.Now()
	var out []InviteRow
	for rows.Next() {
		var iv InviteRow
		if err := rows.Scan(
			&iv.ID, &iv.Label, &iv.CreatedBy, &iv.CreatedAt,
			&iv.ExpiresAt, &iv.UsedAt, &iv.UsedBy, &iv.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("webinvite: scan invite row: %w", err)
		}
		iv.Status = deriveStatus(now, iv.ExpiresAt, iv.UsedAt, iv.RevokedAt)
		out = append(out, iv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("webinvite: iterate invites: %w", err)
	}
	return out, nil
}

// Revoke implements Repo. Guarded so a used/revoked/unknown id is a no-op.
func (r *PostgresRepo) Revoke(ctx context.Context, id string) (RevokeOutcome, error) {
	var revokedID string
	err := r.pool.QueryRow(ctx,
		`UPDATE web_registration_invites
		    SET revoked_at = now()
		  WHERE id = $1
		    AND used_at    IS NULL
		    AND revoked_at IS NULL
		 RETURNING id`,
		id,
	).Scan(&revokedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RevokeNoop, nil
	}
	if err != nil {
		return RevokeNoop, fmt.Errorf("webinvite: revoke invite: %w", err)
	}
	return RevokeDone, nil
}

// deriveStatus computes the lifecycle state from the nullable timestamps,
// matching the design's precedence: revoked > used > expired > outstanding.
func deriveStatus(now time.Time, expiresAt, usedAt, revokedAt *time.Time) InviteStatus {
	switch {
	case revokedAt != nil:
		return StatusRevoked
	case usedAt != nil:
		return StatusUsed
	case expiresAt != nil && !expiresAt.After(now):
		return StatusExpired
	default:
		return StatusOutstanding
	}
}

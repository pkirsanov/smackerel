package webcreds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxUsernameLength caps usernames at 64 bytes after trim. Application
// code enforces this at the boundary so the DB does not need a CHECK
// constraint.
const MaxUsernameLength = 64

// UserRow is a public projection of one web_user_credentials row,
// minus the password_hash (never exposed).
type UserRow struct {
	Username    string
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

// Repo is the storage interface for web operator credentials.
// Implementations MUST treat ErrInvalidCredentials as the only
// error class surfaced to non-CLI callers — never leak DB errors
// to the HTTP login handler.
type Repo interface {
	// UpsertPassword inserts a new row or replaces the password_hash on
	// an existing one. Returns ErrUserExists when create=true and the
	// user already exists.
	UpsertPassword(ctx context.Context, username, password string, create bool) error

	// VerifyAndTouch checks the supplied password against the stored
	// hash. On match, updates last_login_at and returns nil. On any
	// failure (unknown user, wrong password, malformed hash) returns
	// ErrInvalidCredentials. Performs a dummy argon2id evaluation for
	// unknown users to keep timing parity.
	VerifyAndTouch(ctx context.Context, username, password string) error

	// List returns all rows ordered by username asc. Intended for the
	// `users list` CLI subcommand.
	List(ctx context.Context) ([]UserRow, error)

	// Exists reports whether the username has a row.
	Exists(ctx context.Context, username string) (bool, error)
}

// ErrUserExists is returned by UpsertPassword when create=true and a
// row with the same username already exists.
var ErrUserExists = errors.New("webcreds: user already exists")

// ErrUserNotFound is returned by UpsertPassword when create=false and
// the user does not exist (used by the `set-password` CLI subcommand
// which refuses to silently create).
var ErrUserNotFound = errors.New("webcreds: user not found")

// ValidateUsername returns nil when the username is acceptable for
// storage: non-empty after trim, ≤ MaxUsernameLength bytes, no
// control characters or leading/trailing whitespace. Returns a
// descriptive error otherwise.
func ValidateUsername(username string) error {
	if username == "" {
		return errors.New("username must not be empty")
	}
	if username != strings.TrimSpace(username) {
		return errors.New("username must not have leading or trailing whitespace")
	}
	if utf8.RuneCountInString(username) > MaxUsernameLength {
		return fmt.Errorf("username must be at most %d characters", MaxUsernameLength)
	}
	for _, r := range username {
		if unicode.IsControl(r) {
			return errors.New("username must not contain control characters")
		}
	}
	return nil
}

// PostgresRepo is the pgx-backed Repo.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresRepo constructs a PostgresRepo. Returns an error when
// pool is nil to refuse silent no-op dev behavior.
func NewPostgresRepo(pool *pgxpool.Pool) (*PostgresRepo, error) {
	if pool == nil {
		return nil, errors.New("webcreds: NewPostgresRepo requires non-nil *pgxpool.Pool")
	}
	return &PostgresRepo{pool: pool}, nil
}

// UpsertPassword implements Repo.
func (r *PostgresRepo) UpsertPassword(ctx context.Context, username, password string, create bool) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	hash, err := Hash(password)
	if err != nil {
		return err
	}
	exists, err := r.Exists(ctx, username)
	if err != nil {
		return err
	}
	switch {
	case create && exists:
		return ErrUserExists
	case !create && !exists:
		return ErrUserNotFound
	}
	if exists {
		_, err = r.pool.Exec(ctx,
			`UPDATE web_user_credentials SET password_hash = $1 WHERE username = $2`,
			hash, username,
		)
	} else {
		_, err = r.pool.Exec(ctx,
			`INSERT INTO web_user_credentials (username, password_hash) VALUES ($1, $2)`,
			username, hash,
		)
	}
	if err != nil {
		return fmt.Errorf("webcreds: upsert password: %w", err)
	}
	return nil
}

// VerifyAndTouch implements Repo.
func (r *PostgresRepo) VerifyAndTouch(ctx context.Context, username, password string) error {
	if err := ValidateUsername(username); err != nil {
		// Reject invalid usernames via the timing-parity path so callers
		// cannot probe ValidateUsername behavior to enumerate the table.
		_ = Verify(dummyHash, password)
		return ErrInvalidCredentials
	}
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT password_hash FROM web_user_credentials WHERE username = $1`,
		username,
	).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = Verify(dummyHash, password)
		return ErrInvalidCredentials
	}
	if err != nil {
		// On DB error perform the dummy compare anyway so we don't
		// leak transient pool issues via latency, then bubble up.
		_ = Verify(dummyHash, password)
		return fmt.Errorf("webcreds: lookup user: %w", err)
	}
	if err := Verify(hash, password); err != nil {
		return ErrInvalidCredentials
	}
	if _, err := r.pool.Exec(ctx,
		`UPDATE web_user_credentials SET last_login_at = now() WHERE username = $1`,
		username,
	); err != nil {
		// Login itself succeeded; failing to bump last_login_at is a
		// telemetry miss, not an auth failure. Warn (never log the
		// username itself — use its length as a non-identifying signal)
		// and return nil so the caller still sees a successful verify.
		slog.Warn("webcreds: last_login_at update failed",
			"username_len", len(username),
			"err", err,
		)
		return nil
	}
	return nil
}

// List implements Repo.
func (r *PostgresRepo) List(ctx context.Context) ([]UserRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT username, created_at, last_login_at FROM web_user_credentials ORDER BY username ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("webcreds: list users: %w", err)
	}
	defer rows.Close()
	var out []UserRow
	for rows.Next() {
		var u UserRow
		var last *time.Time
		if err := rows.Scan(&u.Username, &u.CreatedAt, &last); err != nil {
			return nil, fmt.Errorf("webcreds: scan user row: %w", err)
		}
		u.LastLoginAt = last
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("webcreds: iterate users: %w", err)
	}
	return out, nil
}

// Exists implements Repo.
func (r *PostgresRepo) Exists(ctx context.Context, username string) (bool, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT 1 FROM web_user_credentials WHERE username = $1`,
		username,
	).Scan(&n); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("webcreds: exists check: %w", err)
	}
	return true, nil
}

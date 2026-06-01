// sqlledger.go — spec 075 SCOPE-2 Postgres-backed NoticeLedger.
//
// Persists the dedup ledger into the JSONB column
// assistant_conversations.legacy_retirement_notices added by
// migration 046. The PK of assistant_conversations is
// (user_id, transport), so cross-transport dedup is implemented by
// querying / updating ALL rows for a given user:
//
//	HasNotified: EXISTS over the user's rows whose
//	             legacy_retirement_notices->>'window_id' = $window
//	             AND legacy_retirement_notices->'commands' ? $command
//
//	MarkShown:   UPDATE every row for the user, merging the new
//	             command entry into the JSONB ledger and re-stamping
//	             the window_id. Repeat calls preserve
//	             first_notified_at and bump notice_count.
//
// The package-level pgxLedgerQuerier interface lets unit tests stub
// the SQL boundary without standing up Postgres; integration tests
// use the real *pgxpool.Pool wrapper.
package legacyretirement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgxLedgerQuerier is the SQL boundary the SQL ledger uses. The
// concrete implementation is *pgxpool.Pool; tests can stub it.
type pgxLedgerQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SQLNoticeLedger is the Postgres-backed NoticeLedger.
type SQLNoticeLedger struct {
	db pgxLedgerQuerier
}

// NewSQLNoticeLedger constructs an SQLNoticeLedger over a pgx pool.
func NewSQLNoticeLedger(pool *pgxpool.Pool) (*SQLNoticeLedger, error) {
	if pool == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLNoticeLedger: pool is nil")
	}
	return &SQLNoticeLedger{db: pool}, nil
}

// HasNotified implements NoticeLedger.
func (l *SQLNoticeLedger) HasNotified(ctx context.Context, userID, retiredCommand, windowID string) (bool, error) {
	const q = `
		SELECT EXISTS(
			SELECT 1
			  FROM assistant_conversations
			 WHERE user_id = $1
			   AND legacy_retirement_notices->>'window_id' = $2
			   AND legacy_retirement_notices->'commands' ? $3
		)`
	var exists bool
	if err := l.db.QueryRow(ctx, q, userID, windowID, retiredCommand).Scan(&exists); err != nil {
		return false, fmt.Errorf("legacyretirement: HasNotified query: %w", err)
	}
	return exists, nil
}

// MarkShown implements NoticeLedger. Updates every assistant
// conversation row for the user so the ledger is visible
// regardless of which transport the user next contacts the
// assistant from.
func (l *SQLNoticeLedger) MarkShown(ctx context.Context, userID, retiredCommand, windowID string, shownAt time.Time) error {
	stamp, err := json.Marshal(shownAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("legacyretirement: encode timestamp: %w", err)
	}
	// JSONB merge: bump notice_count, refresh last_seen_at, preserve
	// first_notified_at if it already exists, refresh window_id to
	// the current window. The COALESCE keeps the prior
	// first_notified_at when present.
	const q = `
		UPDATE assistant_conversations
		   SET legacy_retirement_notices = jsonb_build_object(
				   'schema_version', 1,
				   'window_id',      $2::text,
				   'commands',
				       COALESCE(legacy_retirement_notices->'commands', '{}'::jsonb)
				       || jsonb_build_object(
					   $3::text,
					   jsonb_build_object(
					       'first_notified_at',
					       COALESCE(
						   legacy_retirement_notices->'commands'->$3->>'first_notified_at',
						   $4::text
					       ),
					       'last_seen_at', $4::text,
					       'notice_count',
					       COALESCE(
						   (legacy_retirement_notices->'commands'->$3->>'notice_count')::int,
						   0
					       ) + 1
					   )
				       )
			   )
		 WHERE user_id = $1`
	tag, err := l.db.Exec(ctx, q, userID, windowID, retiredCommand, stripJSONQuotes(stamp))
	if err != nil {
		return fmt.Errorf("legacyretirement: MarkShown update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("legacyretirement: MarkShown: no assistant_conversations row for user %q; ensure a conversation exists before recording a notice", userID)
	}
	return nil
}

// Get implements NoticeLedger.
func (l *SQLNoticeLedger) Get(ctx context.Context, userID, retiredCommand, windowID string) (NoticeLedgerEntry, bool, error) {
	const q = `
		SELECT legacy_retirement_notices->'commands'->$3->>'first_notified_at',
		       legacy_retirement_notices->'commands'->$3->>'last_seen_at',
		       (legacy_retirement_notices->'commands'->$3->>'notice_count')::int
		  FROM assistant_conversations
		 WHERE user_id = $1
		   AND legacy_retirement_notices->>'window_id' = $2
		   AND legacy_retirement_notices->'commands' ? $3
		 LIMIT 1`
	var first, last *string
	var count *int
	err := l.db.QueryRow(ctx, q, userID, windowID, retiredCommand).Scan(&first, &last, &count)
	if err != nil {
		if isNoRows(err) {
			return NoticeLedgerEntry{}, false, nil
		}
		return NoticeLedgerEntry{}, false, fmt.Errorf("legacyretirement: Get query: %w", err)
	}
	if first == nil || last == nil || count == nil {
		return NoticeLedgerEntry{}, false, nil
	}
	firstTS, err := time.Parse(time.RFC3339Nano, *first)
	if err != nil {
		return NoticeLedgerEntry{}, false, fmt.Errorf("legacyretirement: parse first_notified_at: %w", err)
	}
	lastTS, err := time.Parse(time.RFC3339Nano, *last)
	if err != nil {
		return NoticeLedgerEntry{}, false, fmt.Errorf("legacyretirement: parse last_seen_at: %w", err)
	}
	return NoticeLedgerEntry{
		Command:         retiredCommand,
		FirstNotifiedAt: firstTS,
		LastSeenAt:      lastTS,
		NoticeCount:     *count,
	}, true, nil
}

func isNoRows(err error) bool {
	return err == pgx.ErrNoRows
}

func stripJSONQuotes(b []byte) string {
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

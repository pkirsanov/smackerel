package web

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/smackerel/smackerel/internal/digest"
)

// DigestReader is the narrow consumer seam the server-rendered Digest page
// depends on (BUG-002-007). Production injects the existing *digest.Generator
// (whose GetLatest already scans DATE/TIMESTAMPTZ into typed time.Time and
// wraps pgx.ErrNoRows with %w); focused unit tests inject a fake that returns a
// preset row or error. It is an observation seam only — no runtime parameter,
// header, cookie, route, or UI control selects a test path. An empty date reads
// the latest digest; a validated YYYY-MM-DD reads that exact calendar date,
// exactly as the existing GET /api/digest caller does.
type DigestReader interface {
	GetLatest(ctx context.Context, date string) (*digest.Digest, error)
}

// DigestViewState is the closed, mutually-exclusive vocabulary for the
// server-rendered Digest outcome. Exactly one state is produced per read.
// `loading`, `retrying`, and session/auth recovery are HTTP/browser transitions
// around this server model, not storage states, and are never claimed here.
type DigestViewState string

const (
	// DigestCurrent is a successfully read, non-quiet, within-threshold row.
	DigestCurrent DigestViewState = "current"
	// DigestQuiet is a successfully read row deliberately marked quiet; it is a
	// real digest, never an empty state.
	DigestQuiet DigestViewState = "quiet"
	// DigestStale is a successfully read row older than the explicit freshness
	// SST contract. Stored prose and last-success metadata remain visible; it is
	// degraded, never empty.
	DigestStale DigestViewState = "stale"
	// DigestFirstUseEmpty is the ONLY true first-use empty: the latest read
	// completed successfully with wrapped pgx.ErrNoRows and no date was selected.
	DigestFirstUseEmpty DigestViewState = "first_use_empty"
	// DigestSelectedDateEmpty is a validated selected date that has no digest
	// while history exists; distinct from first-use so the two are never merged.
	DigestSelectedDateEmpty DigestViewState = "selected_date_empty"
	// DigestReadError is any query/scan/decode/connection fault. It is never
	// rendered as empty and never substitutes today's date or stored prose.
	DigestReadError DigestViewState = "read_error"
)

// DigestReadErrorKind is a closed, non-sensitive classification of a failed
// read used only for a safe UX label and bounded telemetry. It never carries
// SQL, schema, connection, or raw driver detail. `scan_failed`/`decode_failed`
// are members of the closed vocabulary exercised by the deferred real-PostgreSQL
// fault profile (DIGEST-FP-DB-SCAN-001); the unit classifier conservatively maps
// unknown failures to query_failed, never to empty.
type DigestReadErrorKind string

const (
	DigestReadErrorQuery               DigestReadErrorKind = "query_failed"
	DigestReadErrorScan                DigestReadErrorKind = "scan_failed"
	DigestReadErrorDecode              DigestReadErrorKind = "decode_failed"
	DigestReadErrorDatabaseUnavailable DigestReadErrorKind = "database_unavailable"
)

// DigestPageModel is the single typed projection rendered by digest.html.
// Exactly one State is set; digest-derived fields (Date, Text, WordCount,
// GeneratedAtUTC) are cleared on any failure so a read error can never present
// stored or substituted content.
type DigestPageModel struct {
	Title          string
	State          DigestViewState
	Date           string
	GeneratedAtUTC string
	Text           string
	WordCount      int
	IsQuiet        bool
	AgeDays        int
	ErrorKind      DigestReadErrorKind
	ErrorReference string
}

// errDigestReaderUnavailable is the sentinel for a Digest route mounted without
// a reader. It is surfaced as an honest read_error (HTTP 500), never a
// false-empty first-use state.
var errDigestReaderUnavailable = errors.New("digest reader unavailable")

// classifyDigest is the pure, deterministic core of the false-empty repair. It
// maps exactly one (row, error) read result to exactly one DigestPageModel:
//
//   - Only wrapped pgx.ErrNoRows yields an empty state (first-use when no date
//     was selected, selected-date-empty otherwise). EVERY other error is a
//     read_error with cleared digest-derived fields — never empty, never a
//     today's-date substitution. This is precisely the collapse the old
//     internal/web/handler.go duplicate-SQL path performed and this repair ends.
//   - A successfully read row is NEVER empty: it is current, quiet, or stale.
//   - Stale is produced only when the freshness contract is configured
//     (staleAfter > 0). Until the DIGEST_STALE_AFTER_HOURS SST value is wired
//     (BUG-002-007 Scope 01, deferred to the concurrent config work), staleAfter
//     is zero and a non-quiet row renders current rather than being arbitrarily
//     called stale — matching design.md "a row can be current/quiet but must not
//     be arbitrarily called stale" until the threshold is explicit.
func classifyDigest(d *digest.Digest, readErr error, selectedDate string, now time.Time, staleAfter time.Duration) DigestPageModel {
	m := DigestPageModel{Title: "Daily Digest"}

	if readErr != nil {
		if errors.Is(readErr, pgx.ErrNoRows) {
			if selectedDate != "" {
				m.State = DigestSelectedDateEmpty
				m.Date = selectedDate
			} else {
				m.State = DigestFirstUseEmpty
			}
			return m
		}
		m.State = DigestReadError
		m.ErrorKind = classifyReadErrorKind(readErr)
		return m
	}

	if d == nil {
		// A nil row with a nil error cannot represent a real read result; treat
		// it as an honest read error rather than an empty state.
		m.State = DigestReadError
		m.ErrorKind = DigestReadErrorQuery
		return m
	}

	// A stored row round-trips exactly; the PostgreSQL DATE is the authoritative
	// calendar date rendered without viewer/host timezone conversion, and
	// created_at is displayed in UTC.
	m.Date = d.DigestDate.Format("2006-01-02")
	m.GeneratedAtUTC = d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	m.Text = d.DigestText
	m.WordCount = d.WordCount
	m.IsQuiet = d.IsQuiet

	if d.IsQuiet {
		m.State = DigestQuiet
		return m
	}

	if staleAfter > 0 {
		if age := now.Sub(d.CreatedAt); age > staleAfter {
			m.State = DigestStale
			m.AgeDays = int(age.Hours() / 24)
			return m
		}
	}

	m.State = DigestCurrent
	return m
}

// classifyReadErrorKind maps a non-no-row read failure to a closed, safe kind
// using typed inspection (context sentinels and the pgconn SQLSTATE class), not
// error-message string matching. Unknown failures map to query_failed — never
// to empty — honoring the design contract that string matching may not drive
// empty/security behavior.
func classifyReadErrorKind(err error) DigestReadErrorKind {
	if errors.Is(err, errDigestReaderUnavailable) {
		return DigestReadErrorDatabaseUnavailable
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return DigestReadErrorDatabaseUnavailable
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// SQLSTATE class 08 is the standard "connection exception" class.
		if strings.HasPrefix(pgErr.Code, "08") {
			return DigestReadErrorDatabaseUnavailable
		}
		return DigestReadErrorQuery
	}
	return DigestReadErrorQuery
}

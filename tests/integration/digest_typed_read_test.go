//go:build integration

// BUG-002-007 — live-PostgreSQL proof of the truthful Digest read repair.
//
// These tests exercise the REAL read path end to end against the disposable
// integration Postgres: real migrations, a real row inserted through the real
// `digests` schema (DATE digest_date + TIMESTAMPTZ created_at), the real
// canonical reader (`*digest.Generator`) injected as the web `DigestReader`,
// the real `web.Handler.DigestPage`, and the real `html/template` render. There
// is NO database mock, NO reader fake, and NO HTTP interception — a canned
// response or mock reader cannot satisfy any row here.
//
// Adversarial red-to-green property (DIGEST-S02-T02 / SCN-002-007-01,02): the
// removed handler scanned the DATE `digest_date` column into a Go `string` and
// mapped EVERY error to `No digest generated yet.` + `time.Now()`. A real
// DATE-typed row therefore rendered false-empty. These tests insert exactly such
// a real DATE row and assert it renders `current` with the exact stored calendar
// date and prose; if the string-scan + catch-all were reintroduced the reader
// would error, the page would render `first_use_empty`, and the
// `data-digest-state="current"` assertion would FAIL.
//
// Truthful-state matrix (DIGEST-S03-T02 / SCN-002-007-04,05,06 + read-error):
// only a real no-row read is empty; a real quiet row is `quiet`, a real old row
// under the configured freshness threshold is `stale`, a validated selected date
// with no row is `selected_date_empty` (distinct from first-use), and a real
// connection fault (a closed pool) is a `read_error` at HTTP 500 with every
// digest-derived field cleared — never a false empty and never a substituted
// today's date.

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/web"
)

// digestReadTestPool opens a pgx pool against the live integration-stack
// DATABASE_URL and applies migrations. It FAILS FAST (not Skip) when the env is
// missing so a live Digest row can never silently degrade into a no-op.
func digestReadTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("digest integration test requires DATABASE_URL — run via `./smackerel.sh test integration` which brings up the disposable test stack and exports DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping DATABASE_URL: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// cleanDigests removes every digest row so "latest" is deterministic for the
// current test. The integration package runs serially (-p 1, no t.Parallel), so
// this test owns the shared disposable digests table for its duration.
func cleanDigests(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "DELETE FROM digests"); err != nil {
		t.Fatalf("clean digests: %v", err)
	}
}

// insertDigest writes one row through the REAL `digests` schema. digestDate is
// passed as a text literal cast to ::date so the stored column is a genuine
// PostgreSQL DATE (the exact type the removed string scan rejected). createdAt
// is set explicitly so freshness/staleness is deterministic.
func insertDigest(t *testing.T, pool *pgxpool.Pool, digestDate, text string, wordCount int, quiet bool, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO digests (id, digest_date, digest_text, word_count, is_quiet, model_used, created_at)
		VALUES ($1, $2::date, $3, $4, $5, $6, $7)`,
		"bug-002-007-"+digestDate, digestDate, text, wordCount, quiet, "test-model", createdAt,
	)
	if err != nil {
		t.Fatalf("insert digest %s: %v", digestDate, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM digests WHERE digest_date = $1::date", digestDate)
	})
}

// newDigestHandler wires the REAL canonical reader into a REAL web handler with
// a fixed clock. No fake reader, no interception.
func newDigestHandler(pool *pgxpool.Pool, staleAfter time.Duration, now time.Time) *web.Handler {
	h := web.NewHandler(pool, nil, time.Now())
	h.DigestReader = digest.NewGenerator(pool, nil, nil)
	h.DigestStaleAfter = staleAfter
	h.ClockOverride = func() time.Time { return now }
	return h
}

func serveDigest(h *web.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.DigestPage(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// TestRealDateRowWasHiddenByStringScanAndNowRoundTripsWithoutFalseEmpty is
// DIGEST-S02-T02 (SCN-002-007-01, SCN-002-007-02). It inserts a real current
// digest with a genuine DATE column at a calendar boundary and proves it
// round-trips through the real reader + handler as `current` with the exact
// stored date and prose — the exact row the removed string-scan + catch-all
// rendered false-empty.
func TestRealDateRowWasHiddenByStringScanAndNowRoundTripsWithoutFalseEmpty(t *testing.T) {
	pool := digestReadTestPool(t)
	cleanDigests(t, pool)

	const digestDate = "2027-03-15"
	const marker = "BUG-002-007 live marker: overnight capture surfaced three artifacts and one open action item about the ramen research thread."
	insertDigest(t, pool, digestDate, marker, 380, false, time.Date(2027, 3, 15, 6, 0, 0, 0, time.UTC))

	now := time.Date(2027, 3, 15, 12, 0, 0, 0, time.UTC)
	h := newDigestHandler(pool, 24*time.Hour, now)

	rec := serveDigest(h, "/digest")
	if rec.Code != http.StatusOK {
		t.Fatalf("current stored DATE row: want HTTP 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-digest-state="current"`) {
		t.Errorf("real DATE row must render current (the removed string scan rendered it false-empty); body=%q", body)
	}
	if !strings.Contains(body, marker) {
		t.Errorf("stored prose must round-trip verbatim; marker missing from body=%q", body)
	}
	if !strings.Contains(body, digestDate) {
		t.Errorf("exact stored calendar date %q must render (no viewer/host timezone substitution); body=%q", digestDate, body)
	}
	if strings.Contains(body, "No digest generated yet") {
		t.Error("a real stored row must NEVER render the false-empty first-use copy")
	}
	if strings.Contains(body, now.Format("2006-01-02")) && now.Format("2006-01-02") != digestDate {
		t.Errorf("must NOT substitute today's date %q for the stored digest date", now.Format("2006-01-02"))
	}
}

// TestFalseEmptyCatchAllIsGoneAndRealRowsMapToEmptyQuietStaleOrErrorTruthfully
// is DIGEST-S03-T02 (SCN-002-007-04, 05, 06 + the real read-error boundary). It
// proves against real PostgreSQL that only a genuine no-row read is empty and
// every other real outcome maps to its own truthful, mutually exclusive state.
func TestFalseEmptyCatchAllIsGoneAndRealRowsMapToEmptyQuietStaleOrErrorTruthfully(t *testing.T) {
	pool := digestReadTestPool(t)
	now := time.Date(2027, 3, 15, 12, 0, 0, 0, time.UTC)

	t.Run("true first-use empty is honest (SCN-002-007-04)", func(t *testing.T) {
		cleanDigests(t, pool)
		rec := serveDigest(newDigestHandler(pool, 24*time.Hour, now), "/digest")
		if rec.Code != http.StatusOK {
			t.Fatalf("true empty: want HTTP 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-digest-state="first_use_empty"`) {
			t.Errorf("a successful zero-row read must render first_use_empty; body=%q", body)
		}
		if !strings.Contains(body, "No digest generated yet") {
			t.Errorf("first-use copy expected; body=%q", body)
		}
	})

	t.Run("quiet row is a real digest not empty (SCN-002-007-05)", func(t *testing.T) {
		cleanDigests(t, pool)
		insertDigest(t, pool, "2027-03-14", "Nothing crossed your digest threshold today.", 7, true, time.Date(2027, 3, 14, 6, 0, 0, 0, time.UTC))
		rec := serveDigest(newDigestHandler(pool, 24*time.Hour, now), "/digest")
		if rec.Code != http.StatusOK {
			t.Fatalf("quiet: want HTTP 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-digest-state="quiet"`) {
			t.Errorf("real quiet row must render quiet; body=%q", body)
		}
		if !strings.Contains(body, "2027-03-14") {
			t.Errorf("quiet row must show its persisted date; body=%q", body)
		}
		if strings.Contains(body, `data-digest-state="first_use_empty"`) || strings.Contains(body, `data-digest-state="read_error"`) {
			t.Errorf("quiet must be distinct from empty and error; body=%q", body)
		}
	})

	t.Run("old row under the configured threshold is stale not empty (SCN-002-007-06)", func(t *testing.T) {
		cleanDigests(t, pool)
		const stalePr = "BUG-002-007 stale marker: this older stored digest must remain visible while degraded."
		// created_at six days before the fixed clock; threshold 24h => stale.
		insertDigest(t, pool, "2027-03-09", stalePr, 12, false, time.Date(2027, 3, 9, 12, 0, 0, 0, time.UTC))

		staleRec := serveDigest(newDigestHandler(pool, 24*time.Hour, now), "/digest")
		if staleRec.Code != http.StatusOK {
			t.Fatalf("stale: want HTTP 200, got %d", staleRec.Code)
		}
		staleBody := staleRec.Body.String()
		if !strings.Contains(staleBody, `data-digest-state="stale"`) {
			t.Errorf("old row + configured DIGEST_STALE_AFTER_HOURS threshold must render stale; body=%q", staleBody)
		}
		if !strings.Contains(staleBody, stalePr) {
			t.Errorf("stale must retain the stored prose; body=%q", staleBody)
		}
		if strings.Contains(staleBody, `data-digest-state="current"`) || strings.Contains(staleBody, "No digest generated yet") {
			t.Errorf("stale must not be current or empty; body=%q", staleBody)
		}

		// Deferred-config honesty at the live layer: the SAME real old row with
		// the threshold unconfigured (staleAfter == 0) renders current, never
		// arbitrarily stale — proving the Scope 01 freshness value, not a hidden
		// default, drives staleness.
		currentRec := serveDigest(newDigestHandler(pool, 0, now), "/digest")
		if body := currentRec.Body.String(); !strings.Contains(body, `data-digest-state="current"`) {
			t.Errorf("unconfigured threshold: the same old row must render current (not arbitrarily stale); body=%q", body)
		}
	})

	t.Run("selected-date miss is distinct from first-use (SCN-002-007-04)", func(t *testing.T) {
		cleanDigests(t, pool)
		insertDigest(t, pool, "2027-03-13", "A stored digest exists for another date.", 8, false, time.Date(2027, 3, 13, 6, 0, 0, 0, time.UTC))
		rec := serveDigest(newDigestHandler(pool, 24*time.Hour, now), "/digest?date=2027-03-01")
		if rec.Code != http.StatusOK {
			t.Fatalf("selected-date miss: want HTTP 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-digest-state="selected_date_empty"`) {
			t.Errorf("a validated date with no row (while history exists) must render selected_date_empty; body=%q", body)
		}
		if !strings.Contains(body, "2027-03-01") {
			t.Errorf("selected-date miss must name the selected date; body=%q", body)
		}
		if strings.Contains(body, `data-digest-state="first_use_empty"`) {
			t.Errorf("selected-date miss must be distinct from first-use; body=%q", body)
		}
	})

	t.Run("real connection fault is read_error at 500 not false-empty (SCN-002-007-03)", func(t *testing.T) {
		// Build a real generator on an independent pool, then CLOSE the pool so
		// the real GetLatest hits a genuine connection fault. This is a real
		// read fault through the real reader — not a mock and not interception.
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			t.Fatal("read-error subtest requires DATABASE_URL")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		faultPool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("open fault pool: %v", err)
		}
		faultPool.Close() // force every subsequent query to fail

		h := web.NewHandler(pool, nil, time.Now())
		h.DigestReader = digest.NewGenerator(faultPool, nil, nil)
		h.DigestStaleAfter = 24 * time.Hour
		h.ClockOverride = func() time.Time { return now }

		rec := serveDigest(h, "/digest")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("real connection fault: want HTTP 500 (the removed catch-all returned 200), got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-digest-state="read_error"`) {
			t.Errorf("real read fault must render read_error; body=%q", body)
		}
		if strings.Contains(body, "No digest generated yet") {
			t.Error("read_error must NOT render the false-empty first-use copy")
		}
		if strings.Contains(body, now.Format("2006-01-02")) {
			t.Errorf("read_error must NOT substitute today's date %q", now.Format("2006-01-02"))
		}
	})
}

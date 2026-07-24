package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/smackerel/smackerel/internal/digest"
)

// fakeDigestReader is a focused observation seam for the typed Digest read
// contract (BUG-002-007). It returns a preset row or error and never touches a
// database. No runtime input selects it.
type fakeDigestReader struct {
	d    *digest.Digest
	err  error
	last string // last requested date (proves the exact-date pass-through)
}

func (f *fakeDigestReader) GetLatest(_ context.Context, date string) (*digest.Digest, error) {
	f.last = date
	return f.d, f.err
}

// wrappedNoRows mirrors generator.GetLatest's `fmt.Errorf("get digest: %w", err)`
// wrapping so the classifier's errors.Is unwrap is exercised realistically.
func wrappedNoRows() error { return fmt.Errorf("get digest: %w", pgx.ErrNoRows) }

// TestClassifyDigestStateMatrix proves the pure determination core maps exactly
// one (row, error) read result to exactly one state, and — the heart of the
// false-empty repair — that only a wrapped pgx.ErrNoRows is ever empty.
func TestClassifyDigestStateMatrix(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	const staleAfter = 48 * time.Hour

	freshRow := func() *digest.Digest {
		return &digest.Digest{
			ID:         "d-current",
			DigestDate: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC),
			DigestText: "Today you captured three artifacts and one open action item.",
			WordCount:  10,
			IsQuiet:    false,
			CreatedAt:  now.Add(-2 * time.Hour),
		}
	}

	// 1. A day WITH stored digest data renders populated, NOT empty. This is the
	//    exact product contract the bug violated.
	if m := classifyDigest(freshRow(), nil, "", now, staleAfter); m.State != DigestCurrent {
		t.Errorf("stored current row: want current, got %q", m.State)
	} else {
		if m.Date != "2026-07-24" {
			t.Errorf("current: want stored calendar date 2026-07-24, got %q", m.Date)
		}
		if m.Text == "" || !strings.Contains(m.Text, "three artifacts") {
			t.Errorf("current: stored prose must round-trip, got %q", m.Text)
		}
		if m.WordCount != 10 {
			t.Errorf("current: want stored word count 10, got %d", m.WordCount)
		}
	}

	// 2. Quiet is a real digest, distinct from empty.
	quiet := freshRow()
	quiet.IsQuiet = true
	if m := classifyDigest(quiet, nil, "", now, staleAfter); m.State != DigestQuiet {
		t.Errorf("quiet row: want quiet, got %q", m.State)
	} else if !m.IsQuiet || m.Date != "2026-07-24" {
		t.Errorf("quiet: metadata must persist (isQuiet=%v date=%q)", m.IsQuiet, m.Date)
	}

	// 3. An old row past the configured threshold is stale (degraded), NOT empty,
	//    and retains its stored prose.
	old := freshRow()
	old.ID = "d-old"
	old.CreatedAt = now.Add(-5 * 24 * time.Hour)
	if m := classifyDigest(old, nil, "", now, staleAfter); m.State != DigestStale {
		t.Errorf("old row + configured threshold: want stale, got %q", m.State)
	} else {
		if m.Text == "" {
			t.Error("stale: stored prose must remain visible")
		}
		if m.AgeDays != 5 {
			t.Errorf("stale: want age 5 days, got %d", m.AgeDays)
		}
	}

	// 4. Deferred-config honesty: with the freshness threshold unconfigured
	//    (staleAfter == 0, Scope 01 not yet wired) the SAME old row is current,
	//    never arbitrarily called stale.
	if m := classifyDigest(old, nil, "", now, 0); m.State != DigestCurrent {
		t.Errorf("old row + unconfigured threshold: want current (not arbitrarily stale), got %q", m.State)
	}

	// 5. Wrapped no-row with no selected date is the ONLY true first-use empty.
	if m := classifyDigest(nil, wrappedNoRows(), "", now, staleAfter); m.State != DigestFirstUseEmpty {
		t.Errorf("wrapped ErrNoRows / no date: want first_use_empty, got %q", m.State)
	} else if m.Text != "" {
		t.Errorf("first_use_empty: must carry no digest text, got %q", m.Text)
	}

	// 6. Wrapped no-row WITH a selected date is a distinct selected-date miss,
	//    never merged with first-use.
	if m := classifyDigest(nil, wrappedNoRows(), "2026-07-01", now, staleAfter); m.State != DigestSelectedDateEmpty {
		t.Errorf("wrapped ErrNoRows / selected date: want selected_date_empty, got %q", m.State)
	} else if m.Date != "2026-07-01" {
		t.Errorf("selected_date_empty: want named date 2026-07-01, got %q", m.Date)
	}

	// 7. Every non-no-row fault is a read_error with NO digest-derived fields —
	//    never empty. Distinct kinds are typed, not string-matched.
	pgQuery := &pgconn.PgError{Code: "42P01", Message: "relation \"digests\" does not exist"}
	pgConn := &pgconn.PgError{Code: "08006", Message: "connection failure"}
	for _, tc := range []struct {
		name string
		err  error
		kind DigestReadErrorKind
	}{
		{"server query error", fmt.Errorf("get digest: %w", pgQuery), DigestReadErrorQuery},
		{"connection class 08", fmt.Errorf("get digest: %w", pgConn), DigestReadErrorDatabaseUnavailable},
		{"deadline exceeded", fmt.Errorf("get digest: %w", context.DeadlineExceeded), DigestReadErrorDatabaseUnavailable},
		{"unknown generic error", errors.New("boom"), DigestReadErrorQuery},
		{"nil reader sentinel", errDigestReaderUnavailable, DigestReadErrorDatabaseUnavailable},
	} {
		m := classifyDigest(nil, tc.err, "", now, staleAfter)
		if m.State != DigestReadError {
			t.Errorf("%s: want read_error state, got %q", tc.name, m.State)
		}
		if m.ErrorKind != tc.kind {
			t.Errorf("%s: want kind %q, got %q", tc.name, tc.kind, m.ErrorKind)
		}
		if m.Text != "" || m.Date != "" || m.GeneratedAtUTC != "" {
			t.Errorf("%s: read_error must clear all digest-derived fields (text=%q date=%q gen=%q)", tc.name, m.Text, m.Date, m.GeneratedAtUTC)
		}
	}

	// 8. A nil row with a nil error is an honest read error, never empty.
	if m := classifyDigest(nil, nil, "", now, staleAfter); m.State != DigestReadError {
		t.Errorf("nil row + nil error: want read_error, got %q", m.State)
	}
}

// TestDigestPageTruthfulHTTPStates drives the real handler through a fake reader
// and asserts the HTTP status and rendered markers. It is adversarial against
// the removed behavior: the old handler mapped EVERY read error to HTTP 200 with
// "No digest generated yet." and today's date. These assertions fail against
// that code and pass against the repair.
func TestDigestPageTruthfulHTTPStates(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	todayStr := now.Format("2006-01-02")

	newHandler := func(fr *fakeDigestReader, staleAfter time.Duration) *Handler {
		h := NewHandler(nil, nil, time.Now())
		h.DigestReader = fr
		h.DigestStaleAfter = staleAfter
		h.ClockOverride = func() time.Time { return now }
		return h
	}
	call := func(h *Handler, target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.DigestPage(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec
	}

	// Current: a stored row renders populated at HTTP 200, NOT empty.
	current := &digest.Digest{
		ID: "d1", DigestDate: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC),
		DigestText: "Overnight capture summary about the ramen research thread.",
		WordCount:  9, CreatedAt: now.Add(-90 * time.Minute),
	}
	rec := call(newHandler(&fakeDigestReader{d: current}, 48*time.Hour), "/digest")
	if rec.Code != http.StatusOK {
		t.Fatalf("current: want 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `data-digest-state="current"`) ||
		!strings.Contains(body, "ramen research thread") || strings.Contains(body, "No digest generated yet") {
		t.Errorf("current: want populated current state without false-empty copy; body=%q", body)
	}

	// The exact regression: a NON-no-row read error is HTTP 500 read_error with
	// NO false-empty copy and NO today's-date substitution.
	pgErr := fmt.Errorf("get digest: %w", &pgconn.PgError{Code: "42P01", Message: "boom"})
	rec = call(newHandler(&fakeDigestReader{err: pgErr}, 48*time.Hour), "/digest")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("read error: want 500 (old code returned 200), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-digest-state="read_error"`) {
		t.Errorf("read error: want read_error marker; body=%q", body)
	}
	if strings.Contains(body, "No digest generated yet") {
		t.Error("read error: must NOT render the false-empty first-use copy")
	}
	if strings.Contains(body, todayStr) {
		t.Errorf("read error: must NOT substitute today's date %q", todayStr)
	}

	// First-use empty: a wrapped no-row with no selected date is 200 first_use_empty.
	rec = call(newHandler(&fakeDigestReader{err: wrappedNoRows()}, 48*time.Hour), "/digest")
	if rec.Code != http.StatusOK {
		t.Fatalf("first-use: want 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `data-digest-state="first_use_empty"`) ||
		!strings.Contains(body, "No digest generated yet") {
		t.Errorf("first-use: want first_use_empty copy; body=%q", body)
	}

	// Selected-date miss: a validated ?date= that has no row is a distinct
	// selected_date_empty, and the exact-date value reaches the reader.
	fr := &fakeDigestReader{err: wrappedNoRows()}
	rec = call(newHandler(fr, 48*time.Hour), "/digest?date=2026-07-01")
	if rec.Code != http.StatusOK {
		t.Fatalf("selected-date: want 200, got %d", rec.Code)
	}
	if fr.last != "2026-07-01" {
		t.Errorf("selected-date: reader must receive the exact date, got %q", fr.last)
	}
	if body := rec.Body.String(); !strings.Contains(body, `data-digest-state="selected_date_empty"`) ||
		strings.Contains(body, "No digest generated yet") {
		t.Errorf("selected-date: want distinct selected_date_empty (not first-use); body=%q", body)
	}

	// Stale is activated only when the freshness threshold is configured.
	staleRow := &digest.Digest{
		ID: "d-stale", DigestDate: time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		DigestText: "Older stored digest that must remain visible while degraded.",
		WordCount:  9, CreatedAt: now.Add(-6 * 24 * time.Hour),
	}
	rec = call(newHandler(&fakeDigestReader{d: staleRow}, 48*time.Hour), "/digest")
	if rec.Code != http.StatusOK {
		t.Fatalf("stale: want 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `data-digest-state="stale"`) ||
		!strings.Contains(body, "must remain visible while degraded") {
		t.Errorf("stale: want degraded state retaining stored prose; body=%q", body)
	}

	// Deferred-config honesty at the handler: the SAME old row with the threshold
	// unconfigured (staleAfter == 0) renders current, never arbitrarily stale.
	rec = call(newHandler(&fakeDigestReader{d: staleRow}, 0), "/digest")
	if body := rec.Body.String(); !strings.Contains(body, `data-digest-state="current"`) {
		t.Errorf("unconfigured threshold: old row must render current (not stale); body=%q", body)
	}
}

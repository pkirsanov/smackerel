package list

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// fakeRowScanner is a deterministic in-memory rowScanner for unit-testing
// scanSources error propagation. It does NOT depend on pgx or a real DB.
type fakeRowScanner struct {
	rows         []AggregationSource
	idx          int
	scanErrAt    int   // index at which Scan returns an error (-1 = never)
	scanErr      error // error returned by Scan at scanErrAt
	finalErr     error // returned by Err() after iteration
	scanCalls    int
	errCalls     int
	consumedNext bool // true after Next() has returned false
}

func (f *fakeRowScanner) Next() bool {
	if f.idx >= len(f.rows) {
		f.consumedNext = true
		return false
	}
	return true
}

func (f *fakeRowScanner) Scan(dest ...any) error {
	f.scanCalls++
	if f.scanErrAt >= 0 && f.idx == f.scanErrAt {
		f.idx++
		return f.scanErr
	}
	if len(dest) >= 2 {
		// Only AggregationSource fields are scanned: ArtifactID, DomainData.
		if id, ok := dest[0].(*string); ok {
			*id = f.rows[f.idx].ArtifactID
		}
		// Skip DomainData copy in the test double; scanSources callers don't
		// inspect its bytes for the error-propagation tests.
	}
	f.idx++
	return nil
}

func (f *fakeRowScanner) Err() error {
	f.errCalls++
	return f.finalErr
}

// TestScanSources_PropagatesPerRowScanError covers the harden fix where the
// previous implementation silently `continue`d on a Scan error, hiding row
// corruption from callers. After the fix, scanSources MUST return the error.
func TestScanSources_PropagatesPerRowScanError(t *testing.T) {
	wantErr := errors.New("simulated pgx scan failure on row 1")
	rows := &fakeRowScanner{
		rows: []AggregationSource{
			{ArtifactID: "art-0"},
			{ArtifactID: "art-1"},
			{ArtifactID: "art-2"},
		},
		scanErrAt: 1,
		scanErr:   wantErr,
	}

	got, err := scanSources(rows)
	if err == nil {
		t.Fatalf("expected error from scanSources, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Errorf("expected nil sources on error, got %d sources", len(got))
	}
	if !strings.Contains(err.Error(), "scan artifact domain_data") {
		t.Errorf("expected error message to include context, got: %s", err.Error())
	}
}

// TestScanSources_PropagatesRowsErr covers the harden fix where the previous
// implementation never called rows.Err() after Next() returned false, hiding
// mid-iteration network/transport failures from callers. After the fix,
// scanSources MUST surface rows.Err() as a wrapped error.
func TestScanSources_PropagatesRowsErr(t *testing.T) {
	wantErr := errors.New("simulated mid-iteration network drop")
	rows := &fakeRowScanner{
		rows: []AggregationSource{
			{ArtifactID: "art-0"},
			{ArtifactID: "art-1"},
		},
		scanErrAt: -1,
		finalErr:  wantErr,
	}

	got, err := scanSources(rows)
	if err == nil {
		t.Fatalf("expected error from scanSources when rows.Err() is set, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Errorf("expected nil sources on error, got %d sources", len(got))
	}
	if rows.errCalls == 0 {
		t.Errorf("expected scanSources to call rows.Err() at least once, got %d calls", rows.errCalls)
	}
	if !strings.Contains(err.Error(), "iterate artifact rows") {
		t.Errorf("expected error message to include iteration context, got: %s", err.Error())
	}
}

// TestScanSources_HappyPathChecksRowsErr verifies that even on the success
// path, scanSources still consults rows.Err() so a clean termination is
// distinguishable from a silent truncation.
func TestScanSources_HappyPathChecksRowsErr(t *testing.T) {
	rows := &fakeRowScanner{
		rows: []AggregationSource{
			{ArtifactID: "art-0"},
			{ArtifactID: "art-1"},
		},
		scanErrAt: -1,
		finalErr:  nil,
	}

	got, err := scanSources(rows)
	if err != nil {
		t.Fatalf("expected no error on happy path, got: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 sources, got %d", len(got))
	}
	if rows.errCalls == 0 {
		t.Errorf("expected scanSources to call rows.Err() on happy path, got %d calls", rows.errCalls)
	}
	if !rows.consumedNext {
		t.Errorf("expected scanSources to fully consume Next()")
	}
}

// TestRecipeAggregator_LogsAndSkipsBadJSON re-exercises the malformed-JSON
// path in the recipe aggregator. The harden fix replaced silent `continue`
// with a `slog.Warn` + continue. Behavior (skip-the-bad-source) is preserved;
// this test guards against accidental regression of skip-on-error semantics.
func TestRecipeAggregator_LogsAndSkipsBadJSON(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "bad-1", DomainData: []byte(`{"domain":"recipe","ingredients":[`)}, // truncated JSON
		{ArtifactID: "bad-2", DomainData: []byte(`not even json at all`)},
		{ArtifactID: "good-1", DomainData: []byte(`{"domain":"recipe","ingredients":[{"name":"flour","quantity":"1","unit":"cup"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatalf("Aggregate returned unexpected error: %v", err)
	}
	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed (only good-1 contributed), got %d", len(seeds))
	}
	if seeds[0].NormalizedName != "flour" {
		t.Errorf("expected good-1 ingredient 'flour', got %q", seeds[0].NormalizedName)
	}
	// The good source's artifact ID MUST be preserved; bad sources MUST NOT
	// appear in the output's source artifact list (silent contamination check).
	for _, src := range seeds[0].SourceArtifactIDs {
		if src == "bad-1" || src == "bad-2" {
			t.Errorf("bad source %q leaked into seeds; aggregator failed to skip on bad JSON", src)
		}
	}
}

// TestReadingAggregator_FallsBackOnBadJSON verifies the reading aggregator
// continues to produce a placeholder item for a malformed-JSON artifact
// (since reading lists are tolerant of minimal/missing domain_data). The
// harden fix replaced `_ = json.Unmarshal(...)` with a logged warning;
// behavior (placeholder title fallback) is preserved.
func TestReadingAggregator_FallsBackOnBadJSON(t *testing.T) {
	a := &ReadingAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "bad-1", DomainData: []byte(`{not valid json`)},
		{ArtifactID: "good-1", DomainData: []byte(`{"domain":"reading","title":"Real Article"}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatalf("Aggregate returned unexpected error: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds (bad-1 fallback + good-1), got %d", len(seeds))
	}
	// bad-1 MUST get a placeholder title ("Article 1" since i=0)
	if !strings.Contains(seeds[0].Content, "Article 1") {
		t.Errorf("expected placeholder title 'Article 1' for bad JSON source, got %q", seeds[0].Content)
	}
	// good-1 MUST keep its real title
	if !strings.Contains(seeds[1].Content, "Real Article") {
		t.Errorf("expected real title 'Real Article' for good source, got %q", seeds[1].Content)
	}
}

// TestCompareAggregator_LogsAndSkipsBadJSON is the adversarial regression
// test for BUG-028-002: the compare aggregator previously bare-`continue`d on
// json.Unmarshal failure, silently dropping product/comparison sources whose
// extracted domain_data was malformed. Operators had no way to detect upstream
// extraction regressions for this domain (parity gap with the recipe and
// reading aggregators that already log+skip).
//
// This test would FAIL if the bare `continue` were reintroduced: the slog
// capture buffer would contain no `compare aggregator: skipping artifact with
// malformed domain_data` record. It is therefore non-tautological — there is
// no way to satisfy the assertion without the harden fix being in place.
func TestCompareAggregator_LogsAndSkipsBadJSON(t *testing.T) {
	// Capture slog output for the duration of this test only.
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(prev) })

	a := &CompareAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "bad-1", DomainData: []byte(`{"domain":"product","product_name":"Truncated`)}, // truncated JSON
		{ArtifactID: "bad-2", DomainData: []byte(`not even json at all`)},
		{ArtifactID: "good-1", DomainData: []byte(`{"domain":"product","product_name":"Widget","price":{"amount":42.0,"currency":"USD"}}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatalf("Aggregate returned unexpected error: %v", err)
	}

	// Behavior preservation: only the good source contributes a seed.
	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed (only good-1 contributed), got %d", len(seeds))
	}
	if !strings.Contains(seeds[0].Content, "Widget") {
		t.Errorf("expected good-1 product 'Widget' in seed content, got %q", seeds[0].Content)
	}

	// Adversarial cross-check: bad-source artifact IDs MUST NOT leak into seeds.
	for _, src := range seeds[0].SourceArtifactIDs {
		if src == "bad-1" || src == "bad-2" {
			t.Errorf("bad source %q leaked into seeds; aggregator failed to skip on bad JSON", src)
		}
	}

	// Visibility assertion (the harden fix). Without slog.Warn on unmarshal
	// failure, this buffer would be empty and the test would fail.
	logs := buf.String()
	if !strings.Contains(logs, "compare aggregator: skipping artifact with malformed domain_data") {
		t.Errorf("expected slog.Warn for compare aggregator malformed domain_data, got logs:\n%s", logs)
	}
	// Each malformed source MUST be individually identified in the logs so
	// operators can pinpoint the upstream extractor that produced bad data.
	if !strings.Contains(logs, "bad-1") {
		t.Errorf("expected slog.Warn to identify artifact_id=bad-1, got logs:\n%s", logs)
	}
	if !strings.Contains(logs, "bad-2") {
		t.Errorf("expected slog.Warn to identify artifact_id=bad-2, got logs:\n%s", logs)
	}
	// The good source MUST NOT appear in warning logs (would indicate a
	// regression where good data is also being treated as malformed).
	if strings.Contains(logs, "artifact_id=good-1") {
		t.Errorf("good source unexpectedly logged as malformed:\n%s", logs)
	}
}

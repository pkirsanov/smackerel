package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeExpenseRows is a deterministic in-memory rowScanner for unit-testing the
// expense row-collection helpers' error propagation. It does NOT depend on pgx or a
// real DB, mirroring internal/list/harden_test.go (fakeRowScanner). BUG-034-004.
type fakeExpenseRows struct {
	rows      [][]any // each inner slice = column values for one row
	idx       int
	scanErrAt int   // row index at which Scan returns scanErr (-1 = never)
	scanErr   error // error returned by Scan at scanErrAt
	finalErr  error // returned by Err() after iteration
	scanCalls int
	errCalls  int
}

func (f *fakeExpenseRows) Next() bool {
	return f.idx < len(f.rows)
}

func (f *fakeExpenseRows) Scan(dest ...any) error {
	f.scanCalls++
	if f.scanErrAt >= 0 && f.idx == f.scanErrAt {
		f.idx++
		return f.scanErr
	}
	cols := f.rows[f.idx]
	f.idx++
	for i := range dest {
		if i >= len(cols) {
			break
		}
		switch d := dest[i].(type) {
		case *string:
			s, _ := cols[i].(string)
			*d = s
		case *int:
			n, _ := cols[i].(int)
			*d = n
		case *json.RawMessage:
			switch v := cols[i].(type) {
			case json.RawMessage:
				*d = v
			case []byte:
				*d = json.RawMessage(v)
			case string:
				*d = json.RawMessage(v)
			}
		default:
			return fmt.Errorf("fakeExpenseRows: unsupported dest type %T at col %d", dest[i], i)
		}
	}
	return nil
}

func (f *fakeExpenseRows) Err() error {
	f.errCalls++
	return f.finalErr
}

// --- scanExpenseCurrencySummaries -------------------------------------------------

// TestScanExpenseCurrencySummaries_PropagatesRowsErr is the headline adversarial case:
// a mid-iteration cursor error (rows.Err()) MUST surface as an error, never a
// truncated summary that under-reports the expense total. If the post-loop rows.Err()
// check is removed from scanExpenseCurrencySummaries, this test FAILS.
func TestScanExpenseCurrencySummaries_PropagatesRowsErr(t *testing.T) {
	wantErr := errors.New("simulated mid-iteration cursor drop")
	rows := &fakeExpenseRows{
		rows: [][]any{
			{"USD", 3, "300.00"},
			{"EUR", 2, "50.00"},
		},
		scanErrAt: -1,
		finalErr:  wantErr,
	}

	got, total, err := scanExpenseCurrencySummaries(rows)
	if err == nil {
		t.Fatalf("expected error when rows.Err() is set, got nil (truncated total would be returned as HTTP 200)")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Errorf("expected nil summaries on error, got %d", len(got))
	}
	if total != 0 {
		t.Errorf("expected total 0 on error, got %d", total)
	}
	if rows.errCalls == 0 {
		t.Errorf("expected scanExpenseCurrencySummaries to call rows.Err() at least once, got %d", rows.errCalls)
	}
	if !strings.Contains(err.Error(), "iterate expense currency summaries") {
		t.Errorf("expected iteration context in error, got: %s", err.Error())
	}
}

// TestScanExpenseCurrencySummaries_PropagatesScanError proves the continue->propagate
// change: a per-row Scan failure returns an error instead of silently dropping the row.
func TestScanExpenseCurrencySummaries_PropagatesScanError(t *testing.T) {
	wantErr := errors.New("simulated scan failure on row 1")
	rows := &fakeExpenseRows{
		rows: [][]any{
			{"USD", 3, "300.00"},
			{"EUR", 2, "50.00"},
			{"GBP", 1, "10.00"},
		},
		scanErrAt: 1,
		scanErr:   wantErr,
	}

	got, total, err := scanExpenseCurrencySummaries(rows)
	if err == nil {
		t.Fatalf("expected error from a per-row Scan failure, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if got != nil || total != 0 {
		t.Errorf("expected nil summaries and 0 total on error, got %d summaries / total %d", len(got), total)
	}
	if !strings.Contains(err.Error(), "scan expense currency summary") {
		t.Errorf("expected scan context in error, got: %s", err.Error())
	}
}

// TestScanExpenseCurrencySummaries_HappyPathChecksRowsErr verifies that even on the
// success path the helper still consults rows.Err(), so a clean termination is
// distinguishable from a silent truncation, and that totals are summed correctly.
func TestScanExpenseCurrencySummaries_HappyPathChecksRowsErr(t *testing.T) {
	rows := &fakeExpenseRows{
		rows: [][]any{
			{"USD", 3, "300.00"},
			{"EUR", 2, "50.00"},
		},
		scanErrAt: -1,
		finalErr:  nil,
	}

	got, total, err := scanExpenseCurrencySummaries(rows)
	if err != nil {
		t.Fatalf("expected no error on happy path, got: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(got))
	}
	if total != 5 {
		t.Errorf("expected total count 5 (3+2), got %d", total)
	}
	if got[0].Currency != "USD" || got[0].Count != 3 || got[0].Total != "300.00" {
		t.Errorf("unexpected first summary: %+v", got[0])
	}
	if rows.errCalls == 0 {
		t.Errorf("expected helper to call rows.Err() on happy path, got %d calls", rows.errCalls)
	}
}

// --- scanExpenseListItems ---------------------------------------------------------

// TestScanExpenseListItems_PropagatesRowsErr: mid-iteration cursor error surfaces.
func TestScanExpenseListItems_PropagatesRowsErr(t *testing.T) {
	wantErr := errors.New("simulated mid-iteration cursor drop")
	rows := &fakeExpenseRows{
		rows: [][]any{
			{"art-0", "Coffee", json.RawMessage(`{}`), "gmail"},
			{"art-1", "Lunch", json.RawMessage(`{}`), "gmail"},
		},
		scanErrAt: -1,
		finalErr:  wantErr,
	}

	got, err := scanExpenseListItems(rows)
	if err == nil {
		t.Fatalf("expected error when rows.Err() is set, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Errorf("expected nil items on error, got %d", len(got))
	}
	if rows.errCalls == 0 {
		t.Errorf("expected helper to call rows.Err() at least once, got %d", rows.errCalls)
	}
	if !strings.Contains(err.Error(), "iterate expense list items") {
		t.Errorf("expected iteration context in error, got: %s", err.Error())
	}
}

// TestScanExpenseListItems_PropagatesScanError: per-row Scan failure surfaces.
func TestScanExpenseListItems_PropagatesScanError(t *testing.T) {
	wantErr := errors.New("simulated scan failure on row 0")
	rows := &fakeExpenseRows{
		rows: [][]any{
			{"art-0", "Coffee", json.RawMessage(`{}`), "gmail"},
		},
		scanErrAt: 0,
		scanErr:   wantErr,
	}

	got, err := scanExpenseListItems(rows)
	if err == nil {
		t.Fatalf("expected error from a per-row Scan failure, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Errorf("expected nil items on error, got %d", len(got))
	}
	if !strings.Contains(err.Error(), "scan expense list item") {
		t.Errorf("expected scan context in error, got: %s", err.Error())
	}
}

// TestScanExpenseListItems_HappyPath: success path returns all rows and checks Err().
func TestScanExpenseListItems_HappyPath(t *testing.T) {
	rows := &fakeExpenseRows{
		rows: [][]any{
			{"art-0", "Coffee", json.RawMessage(`{"vendor":"Cafe"}`), "gmail"},
			{"art-1", "Lunch", json.RawMessage(`{"vendor":"Deli"}`), "telegram"},
		},
		scanErrAt: -1,
	}

	got, err := scanExpenseListItems(rows)
	if err != nil {
		t.Fatalf("expected no error on happy path, got: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[1].ID != "art-1" || got[1].Source != "telegram" {
		t.Errorf("unexpected second item: %+v", got[1])
	}
	if rows.errCalls == 0 {
		t.Errorf("expected helper to call rows.Err() on happy path, got %d calls", rows.errCalls)
	}
}

// --- decodeExportExpenseRow -------------------------------------------------------

// TestDecodeExportExpenseRow_PropagatesScanError: Scan failure surfaces (was silent
// `continue` at expenses.go L827 pre-fix).
func TestDecodeExportExpenseRow_PropagatesScanError(t *testing.T) {
	wantErr := errors.New("simulated scan failure")
	rows := &fakeExpenseRows{
		rows:      [][]any{{json.RawMessage(`{}`), "art-0", "gmail", "Coffee"}},
		scanErrAt: 0,
		scanErr:   wantErr,
	}

	_, err := decodeExportExpenseRow(rows)
	if err == nil {
		t.Fatalf("expected error from a Scan failure, got nil (row would be silently dropped from tax CSV)")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if !strings.Contains(err.Error(), "scan expense export row") {
		t.Errorf("expected scan context in error, got: %s", err.Error())
	}
}

// TestDecodeExportExpenseRow_PropagatesUnmarshalError: a corrupt JSONB expense row
// surfaces an unmarshal error instead of being silently `continue`d (was expenses.go
// L831 pre-fix). This is the silently-incomplete-tax-CSV defect.
func TestDecodeExportExpenseRow_PropagatesUnmarshalError(t *testing.T) {
	rows := &fakeExpenseRows{
		// First column is the expense JSON; "not-json" is invalid → Unmarshal fails.
		rows:      [][]any{{json.RawMessage(`not-json`), "art-0", "gmail", "Coffee"}},
		scanErrAt: -1,
	}

	_, err := decodeExportExpenseRow(rows)
	if err == nil {
		t.Fatalf("expected unmarshal error from a corrupt expense JSON row, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal expense export row") {
		t.Errorf("expected unmarshal context in error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "art-0") {
		t.Errorf("expected row id in error context, got: %s", err.Error())
	}
}

// TestDecodeExportExpenseRow_HappyPath: a valid row decodes into the export struct.
func TestDecodeExportExpenseRow_HappyPath(t *testing.T) {
	rows := &fakeExpenseRows{
		rows: [][]any{
			{json.RawMessage(`{"vendor":"ACME","amount":"12.34","currency":"USD"}`), "art-9", "gmail", "Receipt"},
		},
		scanErrAt: -1,
	}

	got, err := decodeExportExpenseRow(rows)
	if err != nil {
		t.Fatalf("expected no error on happy path, got: %v", err)
	}
	if got.RowID != "art-9" || got.Source != "gmail" || got.Title != "Receipt" {
		t.Errorf("unexpected decoded row metadata: %+v", got)
	}
	if got.Exp.Vendor != "ACME" || got.Exp.Currency != "USD" {
		t.Errorf("unexpected decoded expense: vendor=%q currency=%q", got.Exp.Vendor, got.Exp.Currency)
	}
	if got.Exp.Amount == nil || *got.Exp.Amount != "12.34" {
		t.Errorf("expected amount 12.34, got %v", got.Exp.Amount)
	}
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
)

func TestExpenseList_InvalidDateRange(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{ExpensesExportMaxRows: 10000},
	}
	req := httptest.NewRequest("GET", "/api/expenses?from=2026-05-01&to=2026-04-01", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "INVALID_DATE_RANGE" {
		t.Errorf("expected INVALID_DATE_RANGE error code, got %v", resp)
	}
}

func TestExpenseList_InvalidDateFormat(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{ExpensesExportMaxRows: 10000},
	}
	req := httptest.NewRequest("GET", "/api/expenses?from=not-a-date&to=2026-04-30", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExpenseCorrect_InvalidAmount(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	body := `{"amount": "bad"}`
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Correct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "INVALID_AMOUNT" {
		t.Errorf("expected INVALID_AMOUNT error, got %v", resp)
	}
}

func TestExpenseCorrect_InvalidCurrency(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	body := `{"currency": "us"}`
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Correct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExpenseCorrect_InvalidClassification(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	body := `{"classification": "invalid"}`
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Correct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExpenseCorrect_InvalidBody(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	h.Correct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestClassifyEndpoint_InvalidClassification(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	body := `{"classification": "bogus"}`
	req := httptest.NewRequest("POST", "/api/expenses/test-id/classify", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ClassifyEndpoint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExport_InvalidFormat(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{ExpensesExportMaxRows: 10000},
	}
	req := httptest.NewRequest("GET", "/api/expenses/export?format=xml", nil)
	w := httptest.NewRecorder()

	h.Export(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExport_InvalidDateRange(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{ExpensesExportMaxRows: 10000},
	}
	req := httptest.NewRequest("GET", "/api/expenses/export?from=2026-05-01&to=2026-04-01", nil)
	w := httptest.NewRecorder()

	h.Export(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "INVALID_DATE_RANGE" {
		t.Errorf("expected INVALID_DATE_RANGE error code, got %v", resp)
	}
}

func TestWriteExpenseError(t *testing.T) {
	w := httptest.NewRecorder()
	writeExpenseError(w, http.StatusNotFound, "NOT_FOUND", "Item not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != false {
		t.Error("expected ok=false")
	}
}

func TestAmountPatternValidation(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"108.25", true},
		{"0.99", true},
		{"1000.00", true},
		{"bad", false},
		{"10.5", false},
		{"10", false},
		{"-29.99", false}, // negative amounts use different validation in PATCH
		// CHAOS: absurdly large amounts must be rejected (>10 digits)
		{"99999999999.99", false},  // 11 digits — rejected
		{"9999999999.99", true},    // 10 digits — max allowed
		{"00000000001.00", false},  // 11 digits (leading zeros still count)
		{"123456789012.34", false}, // 12 digits — rejected
	}
	for _, tt := range tests {
		if got := amountPattern.MatchString(tt.input); got != tt.valid {
			t.Errorf("amountPattern(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

func TestSanitizeCSVCell(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Normal vendor", "Normal vendor"},
		{"", ""},
		{"=HYPERLINK(\"http://evil.com\")", "'=HYPERLINK(\"http://evil.com\")"},
		{"+cmd|'/C calc'!A0", "'+cmd|'/C calc'!A0"},
		{"-1+1", "'-1+1"},
		{"@SUM(A1:A10)", "'@SUM(A1:A10)"},
		{"\tcmd", "'\tcmd"},
		{"\rcmd", "'\rcmd"},
		{"\ncmd", "'\ncmd"},
		{"Starbucks #1234", "Starbucks #1234"},
		{"100.00", "100.00"},
	}
	for _, tt := range tests {
		if got := sanitizeCSVCell(tt.input); got != tt.want {
			t.Errorf("sanitizeCSVCell(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// CHAOS: Vendor name 10,000 chars must be rejected by PATCH
func TestExpenseCorrect_VendorTooLong(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	longVendor := strings.Repeat("A", 10000)
	body := fmt.Sprintf(`{"vendor": "%s"}`, longVendor)
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Correct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for 10000-char vendor, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "INVALID_VENDOR" {
		t.Errorf("expected INVALID_VENDOR, got %v", resp)
	}
}

// CHAOS: Notes field at maximum boundary
func TestExpenseCorrect_NotesTooLong(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	longNotes := strings.Repeat("N", 2001)
	body := fmt.Sprintf(`{"notes": "%s"}`, longNotes)
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Correct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized notes, got %d", w.Code)
	}
}

// CHAOS: Amount at exact 10-digit boundary — regex validation only
func TestExpenseCorrect_AmountBoundary(t *testing.T) {
	// 10 digits: valid per regex
	if !amountPattern.MatchString("9999999999.99") {
		t.Error("10-digit amount should pass regex validation")
	}
	// 11 digits: invalid per regex
	if amountPattern.MatchString("99999999999.99") {
		t.Error("11-digit amount should fail regex validation")
	}

	// Verify the PATCH handler rejects 11-digit amount
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	body := `{"amount": "99999999999.99"}`
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Correct(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for 11-digit amount, got %d", w.Code)
	}
}

// CHAOS: sanitizeCSVCell with binary-like content
func TestSanitizeCSVCell_Pathological(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Null bytes in vendor names
		{"Vendor\x00Name", "Vendor\x00Name"},
		// All emoji
		{"🏪🛒💰", "🏪🛒💰"},
		// Leading formula char followed by emoji
		{"=🏪", "'=🏪"},
		// Very long string
		{strings.Repeat("X", 10000), strings.Repeat("X", 10000)},
	}
	for _, tt := range tests {
		if got := sanitizeCSVCell(tt.input); got != tt.want {
			t.Errorf("sanitizeCSVCell(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExpenseCorrect_InvalidDate(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}

	tests := []struct {
		name string
		body string
	}{
		{"non-date string", `{"date": "not-a-date"}`},
		{"wrong format", `{"date": "04/18/2026"}`},
		{"impossible date", `{"date": "2026-13-45"}`},
		{"partial date", `{"date": "2026-04"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.Correct(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d for body %s", w.Code, tt.body)
			}
			var resp map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			errObj, _ := resp["error"].(map[string]any)
			if errObj == nil || errObj["code"] != "INVALID_DATE" {
				t.Errorf("expected INVALID_DATE error, got %v", resp)
			}
		})
	}
}

func TestExpenseCorrect_InvalidCategory(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{
			ExpensesCategories: []config.ExpenseCategory{
				{Slug: "food", Display: "Food & Drink"},
				{Slug: "transport", Display: "Transport"},
			},
		},
	}
	body := `{"category": "nonexistent"}`
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Correct(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "INVALID_CATEGORY" {
		t.Errorf("expected INVALID_CATEGORY error, got %v", resp)
	}
}

func TestExpenseCorrect_VendorTooLong_Boundary(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	longVendor := strings.Repeat("A", 201)
	body := fmt.Sprintf(`{"vendor": "%s"}`, longVendor)
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Correct(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExpenseCorrect_NotesTooLong_Boundary(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	longNotes := strings.Repeat("X", 2001)
	body := fmt.Sprintf(`{"notes": "%s"}`, longNotes)
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Correct(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDatePatternValidation(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"2026-04-18", true},
		{"2026-12-31", true},
		{"not-a-date", false},
		{"04/18/2026", false},
		{"2026-4-18", false},
		{"2026-04", false},
	}
	for _, tt := range tests {
		if got := datePattern.MatchString(tt.input); got != tt.valid {
			t.Errorf("datePattern(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

// Round 32: filenameClean sanitizes classification for Content-Disposition header
func TestFilenameClean_HeaderInjection(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"business", "business"},
		{"personal", "personal"},
		{"all", "all"},
		// Injection attempts
		{"business\"\r\nX-Evil: pwned", "businessX-Evilpwned"},
		{"business;rm -rf /", "businessrm-rf"},
		{"../../../etc/passwd", "etcpasswd"},
		{"", ""},
	}
	for _, tt := range tests {
		got := filenameClean.ReplaceAllString(tt.input, "")
		if got != tt.want {
			t.Errorf("filenameClean(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// Round 39: Export handler validates date params individually
func TestExport_InvalidDateParam(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{ExpensesExportMaxRows: 10000},
	}

	// Invalid from date (no to)
	req := httptest.NewRequest("GET", "/api/expenses/export?from=bad-date", nil)
	w := httptest.NewRecorder()
	h.Export(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad from date, got %d", w.Code)
	}

	// Invalid to date (no from)
	req = httptest.NewRequest("GET", "/api/expenses/export?to=bad-date", nil)
	w = httptest.NewRecorder()
	h.Export(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad to date, got %d", w.Code)
	}
}

// Round 52: List handler validates from/to individually (not only when both present)
func TestExpenseList_InvalidFromOnly(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{ExpensesExportMaxRows: 10000},
	}
	req := httptest.NewRequest("GET", "/api/expenses?from=bad-date", nil)
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad from-only date, got %d", w.Code)
	}
}

func TestExpenseList_InvalidToOnly(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{ExpensesExportMaxRows: 10000},
	}
	req := httptest.NewRequest("GET", "/api/expenses?to=not-a-date", nil)
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad to-only date, got %d", w.Code)
	}
}

// SEC-034-001: Oversized request body must be rejected by MaxBytesReader
func TestExpenseCorrect_OversizedBody(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	// 128 KB body — exceeds the 64 KB limit
	bigBody := `{"notes": "` + strings.Repeat("X", 128*1024) + `"}`
	req := httptest.NewRequest("PATCH", "/api/expenses/test-id", strings.NewReader(bigBody))
	w := httptest.NewRecorder()
	h.Correct(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized body, got %d", w.Code)
	}
}

func TestClassifyEndpoint_OversizedBody(t *testing.T) {
	h := &ExpenseHandler{
		Cfg: &config.Config{},
	}
	bigBody := `{"classification": "` + strings.Repeat("X", 128*1024) + `"}`
	req := httptest.NewRequest("POST", "/api/expenses/test-id/classify", strings.NewReader(bigBody))
	w := httptest.NewRecorder()
	h.ClassifyEndpoint(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized classify body, got %d", w.Code)
	}
}

// STB-034-001: AcceptSuggestion requires a DB transaction (nil pool → 500)
func TestAcceptSuggestion_NilPool(t *testing.T) {
	h := &ExpenseHandler{
		Pool: nil,
		Cfg:  &config.Config{},
	}
	req := httptest.NewRequest("POST", "/api/expenses/suggestions/test-id/accept", nil)
	w := httptest.NewRecorder()
	h.AcceptSuggestion(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "TX_FAILED" {
		t.Errorf("expected TX_FAILED error, got %v", resp)
	}
}

// STB-034-001: DismissSuggestion requires a DB transaction (nil pool → 500)
func TestDismissSuggestion_NilPool(t *testing.T) {
	h := &ExpenseHandler{
		Pool: nil,
		Cfg:  &config.Config{},
	}
	req := httptest.NewRequest("POST", "/api/expenses/suggestions/test-id/dismiss", nil)
	w := httptest.NewRecorder()
	h.DismissSuggestion(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "TX_FAILED" {
		t.Errorf("expected TX_FAILED error, got %v", resp)
	}
}

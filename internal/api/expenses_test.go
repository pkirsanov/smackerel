package api

import (
	"encoding/json"
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
	}
	for _, tt := range tests {
		if got := amountPattern.MatchString(tt.input); got != tt.valid {
			t.Errorf("amountPattern(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

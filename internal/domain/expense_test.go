package domain

import (
	"encoding/json"
	"testing"
)

func TestNewExpenseMetadata_Defaults(t *testing.T) {
	e := NewExpenseMetadata()
	if e.Vendor != "Unknown" {
		t.Errorf("expected vendor 'Unknown', got %q", e.Vendor)
	}
	if e.Currency != "USD" {
		t.Errorf("expected currency 'USD', got %q", e.Currency)
	}
	if e.Category != "uncategorized" {
		t.Errorf("expected category 'uncategorized', got %q", e.Category)
	}
	if e.Classification != "uncategorized" {
		t.Errorf("expected classification 'uncategorized', got %q", e.Classification)
	}
	if e.LineItems == nil {
		t.Error("expected non-nil LineItems slice")
	}
	if len(e.LineItems) != 0 {
		t.Errorf("expected empty LineItems, got %d", len(e.LineItems))
	}
	if e.ExtractionStatus != "complete" {
		t.Errorf("expected extraction_status 'complete', got %q", e.ExtractionStatus)
	}
}

func TestExpenseMetadata_JSONRoundTrip(t *testing.T) {
	amt := "108.25"
	tax := "8.25"
	subtotal := "100.00"
	date := "2026-04-03"
	e := &ExpenseMetadata{
		Vendor:           "Corner Coffee",
		VendorRaw:        "SQ *CORNER COFFEE",
		Date:             &date,
		Amount:           &amt,
		Currency:         "USD",
		Subtotal:         &subtotal,
		Tax:              &tax,
		Category:         "food-and-drink",
		Classification:   "business",
		LineItems:        []ExpenseLineItem{{Description: "Latte", Amount: strPtr("4.75")}},
		ExtractionStatus: "complete",
		CorrectedFields:  []string{},
		SourceQualifiers: []string{"Business-Receipts"},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ExpenseMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Vendor != "Corner Coffee" {
		t.Errorf("expected vendor 'Corner Coffee', got %q", decoded.Vendor)
	}
	if decoded.Amount == nil || *decoded.Amount != "108.25" {
		t.Errorf("expected amount '108.25', got %v", decoded.Amount)
	}
	if decoded.Tax == nil || *decoded.Tax != "8.25" {
		t.Errorf("expected tax '8.25', got %v", decoded.Tax)
	}
	if len(decoded.LineItems) != 1 {
		t.Errorf("expected 1 line item, got %d", len(decoded.LineItems))
	}
	if len(decoded.SourceQualifiers) != 1 || decoded.SourceQualifiers[0] != "Business-Receipts" {
		t.Errorf("expected source_qualifiers ['Business-Receipts'], got %v", decoded.SourceQualifiers)
	}
}

func TestExpenseMetadata_NullableFields(t *testing.T) {
	e := NewExpenseMetadata()
	e.AmountMissing = true

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ExpenseMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Amount != nil {
		t.Errorf("expected nil amount, got %v", decoded.Amount)
	}
	if !decoded.AmountMissing {
		t.Error("expected amount_missing = true")
	}
	if decoded.Date != nil {
		t.Errorf("expected nil date, got %v", decoded.Date)
	}
}

func TestExpenseCorrectionRequest_PartialFields(t *testing.T) {
	jsonStr := `{"vendor": "Amazon Marketplace", "category": "office-supplies"}`
	var req ExpenseCorrectionRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.Vendor == nil || *req.Vendor != "Amazon Marketplace" {
		t.Errorf("expected vendor 'Amazon Marketplace', got %v", req.Vendor)
	}
	if req.Category == nil || *req.Category != "office-supplies" {
		t.Errorf("expected category 'office-supplies', got %v", req.Category)
	}
	if req.Amount != nil {
		t.Errorf("expected nil amount for partial correction, got %v", req.Amount)
	}
}

func strPtr(s string) *string { return &s }

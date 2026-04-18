package digest

import (
	"testing"
)

// SCN-034-059: Summary computation
func TestExpenseDigestSummary(t *testing.T) {
	ctx := &ExpenseDigestContext{
		Summary: &ExpenseDigestSummary{
			TotalCount:    12,
			BusinessCount: 7,
			PersonalCount: 5,
			TotalByCurrency: []ExpenseDigestCurrTotal{
				{Currency: "USD", Total: "847.32"},
			},
		},
	}
	if ctx.IsEmpty() {
		t.Error("expected non-empty with summary")
	}
	if ctx.Summary.TotalCount != 12 {
		t.Errorf("expected 12, got %d", ctx.Summary.TotalCount)
	}
	if ctx.Summary.BusinessCount != 7 {
		t.Errorf("expected 7 business, got %d", ctx.Summary.BusinessCount)
	}
}

// SCN-034-060: Needs review
func TestExpenseDigestNeedsReview(t *testing.T) {
	ctx := &ExpenseDigestContext{
		NeedsReview: []ExpenseDigestReviewItem{
			{Vendor: "Uber", Amount: "", Reason: "amount not detected"},
			{Vendor: "Target", Amount: "83.47", Reason: "partial extraction"},
		},
	}
	if ctx.IsEmpty() {
		t.Error("expected non-empty with review items")
	}
	if len(ctx.NeedsReview) != 2 {
		t.Errorf("expected 2 review items, got %d", len(ctx.NeedsReview))
	}
	if ctx.NeedsReview[0].Reason != "amount not detected" {
		t.Errorf("unexpected reason: %s", ctx.NeedsReview[0].Reason)
	}
}

// SCN-034-061: Suggestions
func TestExpenseDigestSuggestions(t *testing.T) {
	ctx := &ExpenseDigestContext{
		Suggestions: []ExpenseDigestSuggestion{
			{Vendor: "Zoom", Amount: "14.99", SuggestedClass: "business", Evidence: "3 previous business expenses"},
		},
	}
	if ctx.IsEmpty() {
		t.Error("expected non-empty with suggestions")
	}
}

// SCN-034-062: Missing receipts
func TestExpenseDigestMissingReceipts(t *testing.T) {
	ctx := &ExpenseDigestContext{
		MissingReceipts: []ExpenseDigestMissing{
			{ServiceName: "Netflix", Amount: "15.99"},
		},
	}
	if ctx.IsEmpty() {
		t.Error("expected non-empty with missing receipts")
	}
	if ctx.MissingReceipts[0].ServiceName != "Netflix" {
		t.Error("expected Netflix")
	}
}

// SCN-034-063: Unusual charges
func TestExpenseDigestUnusualCharges(t *testing.T) {
	ctx := &ExpenseDigestContext{
		UnusualCharges: []ExpenseDigestUnusual{
			{Vendor: "CloudFlare Workers", Amount: "5.00"},
		},
	}
	if ctx.IsEmpty() {
		t.Error("expected non-empty with unusual charges")
	}
}

// SCN-034-065: Empty period
func TestExpenseDigestContext_IsEmpty(t *testing.T) {
	ctx := &ExpenseDigestContext{}
	if !ctx.IsEmpty() {
		t.Error("expected empty")
	}
}

// SCN-034-064: Word limit enforcement
func TestEnforceWordLimit_DropsLowPriorityFirst(t *testing.T) {
	ctx := &ExpenseDigestContext{
		Summary: &ExpenseDigestSummary{
			TotalCount:    12,
			BusinessCount: 7,
			PersonalCount: 5,
			TotalByCurrency: []ExpenseDigestCurrTotal{
				{Currency: "USD", Total: "847.32"},
			},
		},
		NeedsReview: []ExpenseDigestReviewItem{
			{Vendor: "Uber", Amount: "", Reason: "amount not detected"},
		},
		Suggestions: []ExpenseDigestSuggestion{
			{Vendor: "Zoom", Amount: "14.99", SuggestedClass: "business", Evidence: "3 previous"},
		},
		MissingReceipts: []ExpenseDigestMissing{
			{ServiceName: "Netflix", Amount: "15.99"},
		},
		UnusualCharges: []ExpenseDigestUnusual{
			{Vendor: "CloudFlare", Amount: "5.00"},
		},
	}

	// Force very low word limit to trigger all drops
	EnforceWordLimit(ctx, 5)

	// Highest priority (NeedsReview) should survive longest
	// Summary (lowest) should be dropped first
	if ctx.Summary != nil {
		t.Error("expected Summary dropped (lowest priority)")
	}
}

func TestEnforceWordLimit_NoDropWhenUnder(t *testing.T) {
	ctx := &ExpenseDigestContext{
		Summary: &ExpenseDigestSummary{
			TotalCount: 3,
			TotalByCurrency: []ExpenseDigestCurrTotal{
				{Currency: "USD", Total: "50.00"},
			},
		},
	}

	EnforceWordLimit(ctx, 200) // generous limit

	if ctx.Summary == nil {
		t.Error("expected Summary preserved under word limit")
	}
}

func TestCountWords(t *testing.T) {
	ctx := &ExpenseDigestContext{}
	if w := countWords(ctx); w != 0 {
		t.Errorf("expected 0 words for empty context, got %d", w)
	}

	ctx.Summary = &ExpenseDigestSummary{
		TotalCount: 5,
		TotalByCurrency: []ExpenseDigestCurrTotal{
			{Currency: "USD", Total: "100.00"},
		},
	}
	w := countWords(ctx)
	if w < 3 {
		t.Errorf("expected >= 3 words with summary, got %d", w)
	}
}

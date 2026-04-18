package telegram

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/domain"
)

// SCN-034-048: T-001 format
func TestFormatExpenseConfirmation(t *testing.T) {
	amt := "147.30"
	tax := "10.88"
	expense := &domain.ExpenseMetadata{
		Vendor:         "Home Depot",
		Amount:         &amt,
		Tax:            &tax,
		Classification: "business",
		LineItems: []domain.ExpenseLineItem{
			{Description: "Lumber", Amount: strPtr("127.43")},
			{Description: "Screws", Amount: strPtr("8.99")},
		},
	}

	result := FormatExpenseConfirmation(expense)

	if !strings.Contains(result, "Home Depot") {
		t.Error("expected vendor in confirmation")
	}
	if !strings.Contains(result, "$147.30") {
		t.Error("expected amount in confirmation")
	}
	if !strings.Contains(result, "(business)") {
		t.Error("expected classification in confirmation")
	}
	if !strings.Contains(result, "Tax: $10.88") {
		t.Error("expected tax in confirmation")
	}
	if !strings.Contains(result, "2 line items") {
		t.Error("expected line item count")
	}
	if !strings.Contains(result, "Reply 'fix'") {
		t.Error("expected fix prompt")
	}
}

// SCN-034-049: T-002 format
func TestFormatOCRFailure(t *testing.T) {
	result := FormatOCRFailure()
	if !strings.Contains(result, "Couldn't read") {
		t.Error("expected failure message")
	}
	if !strings.Contains(result, "Lunch at Deli") {
		t.Error("expected example entry hint")
	}
}

// SCN-034-050: T-003 partial extraction
func TestFormatPartialExtraction(t *testing.T) {
	amt := "83.47"
	expense := &domain.ExpenseMetadata{
		Vendor: "Target",
		Amount: &amt,
	}
	result := FormatPartialExtraction(expense)
	if !strings.Contains(result, "Target") {
		t.Error("expected vendor")
	}
	if !strings.Contains(result, "$83.47") {
		t.Error("expected amount")
	}
	if !strings.Contains(result, "hard to read") {
		t.Error("expected partial warning")
	}
}

// SCN-034-051: T-004 amount missing
func TestFormatAmountMissing(t *testing.T) {
	expense := &domain.ExpenseMetadata{
		Vendor: "Uber",
	}
	result := FormatAmountMissing(expense)
	if !strings.Contains(result, "Uber") {
		t.Error("expected vendor")
	}
	if !strings.Contains(result, "amount not detected") {
		t.Error("expected missing amount message")
	}
	if !strings.Contains(result, "$23.50") {
		t.Error("expected example amount")
	}
}

// SCN-034-053: T-006 expense list format
func TestFormatExpenseList(t *testing.T) {
	date1 := "2026-04-01"
	date2 := "2026-04-02"
	amt1 := "4.75"
	amt2 := "147.30"
	expenses := []domain.ExpenseMetadata{
		{Vendor: "Coffee", Date: &date1, Amount: &amt1},
		{Vendor: "Home Depot", Date: &date2, Amount: &amt2},
	}
	result := FormatExpenseList(expenses, "Business expenses April 2026", "152.05")
	if !strings.Contains(result, "Business expenses April 2026") {
		t.Error("expected filter header")
	}
	if !strings.Contains(result, "Total: $152.05") {
		t.Error("expected total")
	}
	if !strings.Contains(result, "Coffee") {
		t.Error("expected vendor in list")
	}
	if !strings.Contains(result, "export") {
		t.Error("expected export hint")
	}
}

// SCN-034-053: T-006 list with >10 items shows "more"
func TestFormatExpenseList_MoreThan10(t *testing.T) {
	expenses := make([]domain.ExpenseMetadata, 15)
	for i := range expenses {
		expenses[i] = domain.ExpenseMetadata{Vendor: "Store"}
	}
	result := FormatExpenseList(expenses, "All", "100.00")
	if !strings.Contains(result, "5 more") {
		t.Errorf("expected '5 more', got: %s", result)
	}
}

// T-007 CSV export message
func TestFormatExpenseCSVMessage(t *testing.T) {
	result := FormatExpenseCSVMessage(25, "1234.56", 3)
	if !strings.Contains(result, "25 expenses") {
		t.Error("expected count")
	}
	if !strings.Contains(result, "$1234.56") {
		t.Error("expected total")
	}
	if !strings.Contains(result, "3 expenses have incomplete data") {
		t.Error("expected incomplete warning")
	}
}

// T-007 CSV message without incomplete
func TestFormatExpenseCSVMessage_NoIncomplete(t *testing.T) {
	result := FormatExpenseCSVMessage(5, "50.00", 0)
	if strings.Contains(result, "incomplete") {
		t.Error("should not show incomplete warning when 0")
	}
}

// SCN-034-055: T-009 fix prompt
func TestFormatFixPrompt(t *testing.T) {
	amt := "29.99"
	date := "2026-04-15"
	expense := &domain.ExpenseMetadata{
		Vendor:         "AMZN MKTP",
		Date:           &date,
		Amount:         &amt,
		Currency:       "USD",
		Category:       "other",
		Classification: "uncategorized",
	}
	result := FormatFixPrompt(expense)
	if !strings.Contains(result, "AMZN MKTP") {
		t.Error("expected vendor in fix prompt")
	}
	if !strings.Contains(result, "$29.99") {
		t.Error("expected amount in fix prompt")
	}
	if !strings.Contains(result, "'done'") {
		t.Error("expected done instruction")
	}
}

// T-009 field update confirmation
func TestFormatFieldUpdated(t *testing.T) {
	result := FormatFieldUpdated("vendor", "Acme Hardware")
	if !strings.Contains(result, "vendor → Acme Hardware") {
		t.Error("expected field update format")
	}
	if !strings.Contains(result, "'done'") {
		t.Error("expected done prompt")
	}
}

// Message detection tests
func TestIsExpenseQuery(t *testing.T) {
	tests := []struct {
		text   string
		expect bool
	}{
		{"show business expenses for April", true},
		{"how much did I spend on food?", true},
		{"my expenses this month", true},
		{"hello world", false},
		{"what's the weather", false},
	}
	for _, tt := range tests {
		if got := isExpenseQuery(tt.text); got != tt.expect {
			t.Errorf("isExpenseQuery(%q) = %v, want %v", tt.text, got, tt.expect)
		}
	}
}

func TestIsExpenseExport(t *testing.T) {
	if !isExpenseExport("export business expenses April 2026") {
		t.Error("expected true for export command")
	}
	if isExpenseExport("show expenses") {
		t.Error("expected false for non-export query")
	}
}

func TestIsExpenseEntry(t *testing.T) {
	if !isExpenseEntry("Lunch at Olive Garden $47.82 business") {
		t.Error("expected true for manual entry")
	}
	if !isExpenseEntry("Coffee at Starbucks $4.75") {
		t.Error("expected true for food+amount pattern")
	}
	if isExpenseEntry("hello world") {
		t.Error("expected false for non-expense text")
	}
}

func TestIsSuggestionAccept(t *testing.T) {
	if !isSuggestionAccept("accept Zoom as business") {
		t.Error("expected true")
	}
	if isSuggestionAccept("dismiss Zoom suggestion") {
		t.Error("expected false")
	}
}

func TestIsSuggestionDismiss(t *testing.T) {
	if !isSuggestionDismiss("dismiss Zoom suggestion") {
		t.Error("expected true")
	}
	if isSuggestionDismiss("accept Zoom as business") {
		t.Error("expected false")
	}
}

func TestAmountReplyPattern(t *testing.T) {
	tests := []struct {
		input string
		match bool
	}{
		{"$23.50", true},
		{"23.50", true},
		{"$100", true},
		{"23.50 USD", true},
		{"hello", false},
		{"$", false},
	}
	for _, tt := range tests {
		if got := expenseAmountReplyPattern.MatchString(tt.input); got != tt.match {
			t.Errorf("amountReply(%q) = %v, want %v", tt.input, got, tt.match)
		}
	}
}

// State store tests
func TestExpenseStateStore_SetGetClear(t *testing.T) {
	store := newExpenseStateStore(120)

	store.Set(123, &expenseConversationState{
		LastExpenseID: "exp-001",
		AwaitingField: "amount",
	})

	state := store.Get(123)
	if state == nil {
		t.Fatal("expected state")
	}
	if state.LastExpenseID != "exp-001" {
		t.Errorf("expected exp-001, got %s", state.LastExpenseID)
	}

	store.Clear(123)
	if store.Get(123) != nil {
		t.Error("expected nil after clear")
	}
}

func TestExpenseStateStore_TTLExpiry(t *testing.T) {
	store := newExpenseStateStore(0) // 0-second TTL = immediate expiry
	store.Set(456, &expenseConversationState{LastExpenseID: "exp-002"})

	// With 0-second TTL, ExpiresAt is set to now, so Get should return nil
	state := store.Get(456)
	if state != nil {
		t.Error("expected nil for expired state")
	}
}

func strPtr(s string) *string { return &s }

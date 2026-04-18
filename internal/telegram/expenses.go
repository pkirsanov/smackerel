package telegram

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/domain"
)

// expenseConversationState tracks multi-turn expense interactions.
type expenseConversationState struct {
	LastExpenseID string
	AwaitingField string // "amount", "fix_field", ""
	ExpiresAt     time.Time
}

// expenseStateStore manages conversation state for expense interactions.
type expenseStateStore struct {
	mu    sync.RWMutex
	store map[int64]*expenseConversationState
	ttl   time.Duration
}

func newExpenseStateStore(ttlSeconds int) *expenseStateStore {
	return &expenseStateStore{
		store: make(map[int64]*expenseConversationState),
		ttl:   time.Duration(ttlSeconds) * time.Second,
	}
}

func (s *expenseStateStore) Get(chatID int64) *expenseConversationState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.store[chatID]
	if !ok || time.Now().After(state.ExpiresAt) {
		return nil
	}
	return state
}

func (s *expenseStateStore) Set(chatID int64, state *expenseConversationState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.ExpiresAt = time.Now().Add(s.ttl)
	s.store[chatID] = state
}

func (s *expenseStateStore) Clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, chatID)
}

// Expense message detection patterns
var (
	expenseAmountReplyPattern = regexp.MustCompile(`^\$?\d+\.?\d*(\s*(USD|EUR|GBP|CAD|AUD))?$`)
	expenseQueryPatterns      = []string{"show expenses", "show business expenses", "how much", "my expenses", "expense report", "expenses for"}
	expenseExportPattern      = regexp.MustCompile(`(?i)export\s+.*expenses?`)
)

// isExpenseQuery checks if a message is an expense query.
func isExpenseQuery(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range expenseQueryPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isExpenseExport checks if a message is an expense export request.
func isExpenseExport(text string) bool {
	return expenseExportPattern.MatchString(text)
}

// isExpenseEntry checks if text looks like a manual expense entry.
func isExpenseEntry(text string) bool {
	lower := strings.ToLower(text)
	hasAmount := regexp.MustCompile(`\$\d+\.?\d*`).MatchString(text)
	hasExpenseKeyword := strings.Contains(lower, "expense") || strings.Contains(lower, "spent") || strings.Contains(lower, "cost") || strings.Contains(lower, "receipt")
	// Also match simple patterns like "Lunch at Place $XX.XX"
	hasFoodWord := strings.Contains(lower, "lunch") || strings.Contains(lower, "dinner") || strings.Contains(lower, "breakfast") || strings.Contains(lower, "coffee")
	return hasAmount && (hasExpenseKeyword || hasFoodWord)
}

// isSuggestionAccept checks if text accepts a suggestion.
func isSuggestionAccept(text string) bool {
	lower := strings.ToLower(text)
	return strings.HasPrefix(lower, "accept ") && strings.Contains(lower, " as ")
}

// isSuggestionDismiss checks if text dismisses a suggestion.
func isSuggestionDismiss(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "dismiss") && strings.Contains(lower, "suggestion")
}

// Format functions per UX spec T-001 through T-011

// FormatExpenseConfirmation produces the T-001 confirmation format.
func FormatExpenseConfirmation(expense *domain.ExpenseMetadata) string {
	var sb strings.Builder
	sb.WriteString("✅ Saved: ")
	sb.WriteString(expense.Vendor)
	if expense.Amount != nil {
		sb.WriteString(fmt.Sprintf(" $%s", *expense.Amount))
	}
	if expense.Classification != "uncategorized" {
		sb.WriteString(fmt.Sprintf(" (%s)", expense.Classification))
	}
	if expense.Tax != nil {
		sb.WriteString(fmt.Sprintf("\nTax: $%s", *expense.Tax))
	}
	if len(expense.LineItems) > 0 {
		sb.WriteString(fmt.Sprintf("\n%d line items", len(expense.LineItems)))
	}
	sb.WriteString("\n\nReply 'details' to see line items")
	sb.WriteString("\nReply 'fix' to correct anything")
	return sb.String()
}

// FormatOCRFailure produces the T-002 failure message.
func FormatOCRFailure() string {
	return "❌ Couldn't read this receipt. Try a clearer photo, or type the details: 'Lunch at Deli $12.50 business'"
}

// FormatPartialExtraction produces the T-003 partial result format.
func FormatPartialExtraction(expense *domain.ExpenseMetadata) string {
	var sb strings.Builder
	sb.WriteString("✅ Saved: ")
	sb.WriteString(expense.Vendor)
	if expense.Amount != nil {
		sb.WriteString(fmt.Sprintf(" $%s", *expense.Amount))
	}
	sb.WriteString("\n⚠️ Some details were hard to read. Reply 'fix' to correct anything.")
	return sb.String()
}

// FormatAmountMissing produces the T-004 amount-missing prompt.
func FormatAmountMissing(expense *domain.ExpenseMetadata) string {
	return fmt.Sprintf("✅ Saved: %s · amount not detected\nReply with the amount to add it, e.g. '$23.50'", expense.Vendor)
}

// FormatExpenseList produces the T-006 formatted list (max 10 items).
func FormatExpenseList(expenses []domain.ExpenseMetadata, filter string, total string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 %s\n", filter))
	if total != "" {
		sb.WriteString(fmt.Sprintf("Total: $%s\n\n", total))
	}

	limit := 10
	if len(expenses) < limit {
		limit = len(expenses)
	}
	for i := 0; i < limit; i++ {
		e := expenses[i]
		date := "N/A"
		if e.Date != nil {
			date = *e.Date
		}
		amount := "–"
		if e.Amount != nil {
			amount = "$" + *e.Amount
		}
		sb.WriteString(fmt.Sprintf("  %s  %s  %s\n", date, e.Vendor, amount))
	}
	if len(expenses) > 10 {
		sb.WriteString(fmt.Sprintf("\n...and %d more. Reply 'all' to see the full list", len(expenses)-10))
	}
	sb.WriteString("\nReply 'export' to download CSV")
	return sb.String()
}

// FormatExpenseCSVMessage produces the T-007 export summary message.
func FormatExpenseCSVMessage(count int, total string, incomplete int) string {
	msg := fmt.Sprintf("📎 Exported %d expenses, total $%s", count, total)
	if incomplete > 0 {
		msg += fmt.Sprintf("\n⚠️ %d expenses have incomplete data", incomplete)
	}
	return msg
}

// FormatFixPrompt produces the T-009 fix prompt showing current fields.
func FormatFixPrompt(expense *domain.ExpenseMetadata) string {
	var sb strings.Builder
	sb.WriteString("📝 Current expense details:\n")
	sb.WriteString(fmt.Sprintf("  vendor: %s\n", expense.Vendor))
	if expense.Date != nil {
		sb.WriteString(fmt.Sprintf("  date: %s\n", *expense.Date))
	}
	if expense.Amount != nil {
		sb.WriteString(fmt.Sprintf("  amount: $%s\n", *expense.Amount))
	}
	sb.WriteString(fmt.Sprintf("  currency: %s\n", expense.Currency))
	sb.WriteString(fmt.Sprintf("  category: %s\n", expense.Category))
	sb.WriteString(fmt.Sprintf("  classification: %s\n", expense.Classification))
	sb.WriteString("\nSend a correction like 'vendor Acme Hardware' or 'amount 99.99'")
	sb.WriteString("\nReply 'done' when finished.")
	return sb.String()
}

// FormatFieldUpdated produces the T-009 field update confirmation.
func FormatFieldUpdated(field, value string) string {
	return fmt.Sprintf("✅ Updated: %s → %s\nAnything else to fix? Reply 'done' when finished.", field, value)
}

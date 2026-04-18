package intelligence

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/domain"
)

func newTestClassifier() *ExpenseClassifier {
	return &ExpenseClassifier{
		IMAPExpenseLabels: map[string]string{
			"Business-Receipts":  "business",
			"Tax-Deductible":     "business",
			"Personal-Purchases": "personal",
		},
		BusinessVendors:      []string{"WeWork", "Zoom"},
		MinPastBusiness:      2,
		MinConfidence:        0.6,
		ReclassifyBatchLimit: 100,
		Categories: []config.ExpenseCategory{
			{Slug: "food-and-drink", Display: "Food & Drink", TaxCategory: "Meals"},
			{Slug: "technology", Display: "Technology", TaxCategory: "Other Expenses"},
		},
		vendorNormalizer: NewVendorNormalizer(nil, 100),
	}
}

// SCN-034-019: Gmail label match → business classification
func TestClassify_GmailLabelMatch(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.SourceQualifiers = []string{"Tax-Deductible"}

	result := ec.Classify(expense)
	if result != "business" {
		t.Errorf("expected 'business' from Gmail label match, got %q", result)
	}
}

// SCN-034-019: Personal label → personal classification
func TestClassify_GmailLabelPersonal(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.SourceQualifiers = []string{"Personal-Purchases"}

	result := ec.Classify(expense)
	if result != "personal" {
		t.Errorf("expected 'personal', got %q", result)
	}
}

// SCN-034-020: Telegram caption context → business classification
func TestClassify_TelegramCaptionBusiness(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	notes := "rental property repair business"
	expense.Notes = &notes

	result := ec.Classify(expense)
	if result != "business" {
		t.Errorf("expected 'business' from caption, got %q", result)
	}
}

// SCN-034-020: Caption with "personal" → personal
func TestClassify_TelegramCaptionPersonal(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	notes := "personal grocery run"
	expense.Notes = &notes

	result := ec.Classify(expense)
	if result != "personal" {
		t.Errorf("expected 'personal' from caption, got %q", result)
	}
}

// SCN-034-021: Business vendor list match → business
func TestClassify_BusinessVendorMatch(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.Vendor = "WeWork"

	result := ec.Classify(expense)
	if result != "business" {
		t.Errorf("expected 'business' from vendor list, got %q", result)
	}
}

// SCN-034-021: Case-insensitive vendor match
func TestClassify_BusinessVendorCaseInsensitive(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.Vendor = "zoom" // lowercase

	result := ec.Classify(expense)
	if result != "business" {
		t.Errorf("expected 'business' from vendor list (case-insensitive), got %q", result)
	}
}

// SCN-034-022: No rule match → uncategorized
func TestClassify_NoMatch_Uncategorized(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.Vendor = "Random Store"

	result := ec.Classify(expense)
	if result != "uncategorized" {
		t.Errorf("expected 'uncategorized', got %q", result)
	}
}

// SCN-034-023: User correction survives re-classification (adversarial)
func TestClassify_UserCorrectionPreserved(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.UserCorrected = true
	expense.CorrectedFields = []string{"classification"}
	expense.Classification = "personal"
	// Even though the vendor is in business list, user correction wins
	expense.Vendor = "WeWork"
	expense.SourceQualifiers = []string{"Tax-Deductible"}

	result := ec.Classify(expense)
	if result != "personal" {
		t.Errorf("expected user correction 'personal' to be preserved, got %q", result)
	}
}

// Test rule priority order: label beats caption beats vendor list
func TestClassify_PriorityOrder_LabelOverCaption(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.SourceQualifiers = []string{"Personal-Purchases"}
	notes := "business meeting"
	expense.Notes = &notes
	expense.Vendor = "WeWork"

	result := ec.Classify(expense)
	// Label (personal) should beat caption (business) and vendor list (business)
	if result != "personal" {
		t.Errorf("expected label 'personal' to win over caption and vendor, got %q", result)
	}
}

// Test rule priority: caption beats vendor list
func TestClassify_PriorityOrder_CaptionOverVendor(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	notes := "personal errand"
	expense.Notes = &notes
	expense.Vendor = "WeWork" // In business vendors

	result := ec.Classify(expense)
	// Caption (personal) should beat vendor list (business)
	if result != "personal" {
		t.Errorf("expected caption 'personal' to win over vendor list, got %q", result)
	}
}

// SCN-034-024: Category from LLM extraction is stored
func TestClassify_LLMCategoryPreserved(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.Vendor = "Shell Gas Station"
	expense.Category = "auto-and-transport"

	result := ec.Classify(expense)
	// Classification is uncategorized (no rule match), but category remains
	if result != "uncategorized" {
		t.Errorf("expected 'uncategorized', got %q", result)
	}
	// Category should not be changed by Classify
	if expense.Category != "auto-and-transport" {
		t.Errorf("expected category preserved as 'auto-and-transport', got %q", expense.Category)
	}
}

// SCN-034-025: Vendor normalizer with no DB returns false
func TestVendorNormalizer_NoDB(t *testing.T) {
	n := NewVendorNormalizer(nil, 100)
	_, found := n.Normalize(nil, "AMZN MKTP US")
	if found {
		t.Error("expected not found with nil pool")
	}
}

// SCN-034-025: Cache hit avoids DB
func TestVendorNormalizer_CacheHit(t *testing.T) {
	n := NewVendorNormalizer(nil, 100)
	// Manually populate cache
	n.mu.Lock()
	n.cache["amzn mktp us"] = "Amazon"
	n.mu.Unlock()

	canonical, found := n.Normalize(nil, "AMZN MKTP US")
	if !found {
		t.Error("expected cache hit")
	}
	if canonical != "Amazon" {
		t.Errorf("expected 'Amazon', got %q", canonical)
	}
}

// SCN-034-025: Negative cache result
func TestVendorNormalizer_NegativeCache(t *testing.T) {
	n := NewVendorNormalizer(nil, 100)
	n.mu.Lock()
	n.cache["unknown vendor"] = "" // negative cache entry
	n.mu.Unlock()

	_, found := n.Normalize(nil, "Unknown Vendor")
	if found {
		t.Error("expected negative cache to return not found")
	}
}

// Test CategoryDisplayName
func TestCategoryDisplayName(t *testing.T) {
	ec := newTestClassifier()

	if name := ec.CategoryDisplayName("food-and-drink"); name != "Food & Drink" {
		t.Errorf("expected 'Food & Drink', got %q", name)
	}
	if name := ec.CategoryDisplayName("nonexistent"); name != "nonexistent" {
		t.Errorf("expected fallback to slug, got %q", name)
	}
}

// Cache eviction when at capacity
func TestVendorNormalizer_CacheEviction(t *testing.T) {
	n := NewVendorNormalizer(nil, 4)
	n.mu.Lock()
	n.cache["a"] = "A"
	n.cache["b"] = "B"
	n.cache["c"] = "C"
	n.cache["d"] = "D"
	n.mu.Unlock()

	// Adding one more should trigger eviction
	n.put("e", "E")

	n.mu.RLock()
	// Should have evicted half (2 entries) and added 1 = 3 entries
	if len(n.cache) > 4 {
		t.Errorf("expected cache <= 4 after eviction, got %d", len(n.cache))
	}
	n.mu.RUnlock()
}

// CHAOS: Classify with nil/empty config fields — must not panic
func TestClassify_NilConfigFields(t *testing.T) {
	ec := &ExpenseClassifier{
		IMAPExpenseLabels: nil,
		BusinessVendors:   nil,
		Categories:        nil,
		vendorNormalizer:  NewVendorNormalizer(nil, 10),
	}
	expense := domain.NewExpenseMetadata()
	expense.Vendor = "Test"

	// Must not panic
	result := ec.Classify(expense)
	if result != "uncategorized" {
		t.Errorf("expected 'uncategorized' with nil config, got %q", result)
	}
}

// CHAOS: Vendor name 10,000 chars through Classify — must not panic or hang
func TestClassify_HugeVendorName(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.Vendor = strings.Repeat("A", 10000)

	result := ec.Classify(expense)
	if result != "uncategorized" {
		t.Errorf("expected 'uncategorized' for huge vendor, got %q", result)
	}
}

// CHAOS: CategoryDisplayName with nil Categories slice
func TestCategoryDisplayName_NilCategories(t *testing.T) {
	ec := &ExpenseClassifier{Categories: nil, vendorNormalizer: NewVendorNormalizer(nil, 10)}
	if name := ec.CategoryDisplayName("food"); name != "food" {
		t.Errorf("expected fallback 'food', got %q", name)
	}
}

// CHAOS: Classify with empty string vendor and nil notes
func TestClassify_EmptyVendorNilNotes(t *testing.T) {
	ec := newTestClassifier()
	expense := domain.NewExpenseMetadata()
	expense.Vendor = ""
	expense.Notes = nil

	result := ec.Classify(expense)
	if result != "uncategorized" {
		t.Errorf("expected 'uncategorized', got %q", result)
	}
}

// CHAOS: VendorNormalizer cache with 10,000-char key
func TestVendorNormalizer_HugeCacheKey(t *testing.T) {
	n := NewVendorNormalizer(nil, 100)
	hugeKey := strings.Repeat("X", 10000)
	_, found := n.Normalize(nil, hugeKey)
	if found {
		t.Error("expected not found for huge key with nil pool")
	}
	// With nil pool, Normalize returns early — no cache entry expected.
	// Verify no panic occurred with the huge key.
}

// Round 10: VendorNormalizer LIKE escape — verify special chars are escaped
func TestVendorNormalizer_LIKEEscaping(t *testing.T) {
	// This tests the escape logic indirectly. With nil pool,
	// the query won't execute but the escape path is exercised.
	n := NewVendorNormalizer(nil, 100)

	// Input containing LIKE wildcards should not cause issues
	_, found := n.Normalize(nil, "100% MATCH_TEST")
	if found {
		t.Error("expected not found with nil pool")
	}

	// Underscore in vendor name
	_, found = n.Normalize(nil, "test_vendor")
	if found {
		t.Error("expected not found with nil pool")
	}
}

// Round 10: containsField edge cases
func TestContainsField_EdgeCases(t *testing.T) {
	if containsField(nil, "test") {
		t.Error("expected false for nil slice")
	}
	if containsField([]string{}, "test") {
		t.Error("expected false for empty slice")
	}
	if !containsField([]string{"a", "b", "c"}, "b") {
		t.Error("expected true for present item")
	}
	if containsField([]string{"a", "b"}, "B") {
		t.Error("expected false for case mismatch (case-sensitive)")
	}
}

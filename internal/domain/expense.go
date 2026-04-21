package domain

// ExpenseMetadata represents the structured expense data stored in
// artifacts.metadata under the "expense" key.
// All monetary amounts are decimal strings — never floats.
type ExpenseMetadata struct {
	Vendor            string            `json:"vendor"`
	VendorRaw         string            `json:"vendor_raw"`
	Date              *string           `json:"date"`              // YYYY-MM-DD, nullable
	Amount            *string           `json:"amount"`            // decimal string, nullable if amount_missing
	RawAmount         *string           `json:"raw_amount"`        // original amount text
	Currency          string            `json:"currency"`          // ISO 4217
	Subtotal          *string           `json:"subtotal"`          // pre-tax, pre-tip
	Tax               *string           `json:"tax"`               // tax amount
	Tip               *string           `json:"tip"`               // tip/gratuity
	PaymentMethod     *string           `json:"payment_method"`    // visa-ending-4242, cash, etc.
	Category          string            `json:"category"`          // slug
	Classification    string            `json:"classification"`    // business, personal, uncategorized
	LineItems         []ExpenseLineItem `json:"line_items"`        // never nil, empty array default
	Notes             *string           `json:"notes"`             // user or extracted notes
	ExtractionStatus  string            `json:"extraction_status"` // complete, partial, failed
	ExtractionPartial bool              `json:"extraction_partial"`
	AmountMissing     bool              `json:"amount_missing"`
	UserCorrected     bool              `json:"user_corrected"`
	CorrectedFields   []string          `json:"corrected_fields"`  // which fields user corrected
	SourceQualifiers  []string          `json:"source_qualifiers"` // Gmail labels etc.
}

// ExpenseLineItem represents a single line item on a receipt.
type ExpenseLineItem struct {
	Description string  `json:"description"`
	Amount      *string `json:"amount,omitempty"`
	Quantity    *string `json:"quantity,omitempty"`
}

// VendorAlias maps a raw vendor text to a canonical name.
type VendorAlias struct {
	ID        string `json:"id"`
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Source    string `json:"source"` // "system" or "user"
}

// ExpenseSuggestion represents a business classification suggestion.
type ExpenseSuggestion struct {
	ID             string  `json:"id"`
	ArtifactID     string  `json:"artifact_id"`
	Vendor         string  `json:"vendor"`
	SuggestedClass string  `json:"suggested_class"`
	Confidence     float64 `json:"confidence"`
	Evidence       string  `json:"evidence"`
	Status         string  `json:"status"` // pending, accepted, dismissed
}

// NewExpenseMetadata returns an ExpenseMetadata with safe defaults.
func NewExpenseMetadata() *ExpenseMetadata {
	return &ExpenseMetadata{
		Vendor:           "Unknown",
		Currency:         "USD",
		Category:         "uncategorized",
		Classification:   "uncategorized",
		LineItems:        []ExpenseLineItem{},
		ExtractionStatus: "complete",
		CorrectedFields:  []string{},
		SourceQualifiers: []string{},
	}
}

// ExpenseCorrectionRequest represents the fields a user can correct.
type ExpenseCorrectionRequest struct {
	Vendor         *string `json:"vendor,omitempty"`
	Date           *string `json:"date,omitempty"`
	Amount         *string `json:"amount,omitempty"`
	Currency       *string `json:"currency,omitempty"`
	Category       *string `json:"category,omitempty"`
	Classification *string `json:"classification,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	PaymentMethod  *string `json:"payment_method,omitempty"`
}

// ClassifyRequest is the body for POST /api/expenses/{id}/classify.
type ClassifyRequest struct {
	Classification string `json:"classification"`
}

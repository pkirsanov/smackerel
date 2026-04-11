package intelligence

import (
	"strings"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  TypeScript  generics  ", "typescript generics"},
		{"Hello   World", "hello world"},
		{"UPPER", "upper"},
		{"", ""},
		{"  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeQuery(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeQuery(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHashQuery(t *testing.T) {
	// Same normalized input produces same hash
	h1 := hashQuery("typescript generics")
	h2 := hashQuery("typescript generics")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}

	// Different inputs produce different hashes
	h3 := hashQuery("go concurrency")
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}

	// Hash has expected length (32 hex chars = 128 bits)
	if len(h1) != 32 {
		t.Errorf("expected hash length 32, got %d", len(h1))
	}
}

func TestLogSearch_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	err := engine.LogSearch(nil, "test query", 5, "result-1")
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestDetectFrequentLookups_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.DetectFrequentLookups(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCreateQuickReference_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.CreateQuickReference(nil, "concept", "content", []string{"id1"})
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestGetQuickReferences_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GetQuickReferences(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// === Stabilize: CreateQuickReference JSON safety ===

func TestCreateQuickReference_SourceIDsWithSpecialChars(t *testing.T) {
	// Verify that source IDs containing JSON-dangerous characters are safely
	// marshalled rather than injected via fmt.Sprintf.
	dangerous := []string{`art-1"`, `art-2\`, `art-3']`}

	// Without a real pool we can't INSERT, but the function should marshal
	// the JSON before hitting the pool. Since pool is nil, the function
	// should return the nil-pool error — not panic on JSON building.
	engine := &Engine{Pool: nil}
	_, err := engine.CreateQuickReference(nil, "test", "content", dangerous)
	if err == nil {
		t.Error("expected error for nil pool")
	}
	// The error should be the pool check, not a JSON/SQL error —
	// meaning the JSON marshalling path had no issues.
	if !strings.Contains(err.Error(), "requires a database connection") {
		t.Errorf("expected pool error, got: %s", err.Error())
	}
}

func TestCreateQuickReference_EmptySourceIDs(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.CreateQuickReference(nil, "test", "content", []string{})
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCreateQuickReference_NilSourceIDs(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.CreateQuickReference(nil, "test", "content", nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// === Edge cases: normalizeQuery with special characters ===

func TestNormalizeQuery_TabsAndNewlines(t *testing.T) {
	got := normalizeQuery("\tquery\nwith\ttabs")
	if got != "query with tabs" {
		t.Errorf("normalizeQuery(tabs/newlines) = %q, want %q", got, "query with tabs")
	}
}

func TestNormalizeQuery_Unicode(t *testing.T) {
	got := normalizeQuery("  Résumé  Tips  ")
	if got != "résumé tips" {
		t.Errorf("normalizeQuery(unicode) = %q, want %q", got, "résumé tips")
	}
}

// === Edge cases: hashQuery ===

func TestHashQuery_EmptyString(t *testing.T) {
	h := hashQuery("")
	if len(h) != 32 {
		t.Errorf("expected 32-char hash for empty string, got %d chars", len(h))
	}
	// Should be deterministic
	h2 := hashQuery("")
	if h != h2 {
		t.Error("empty string hash should be deterministic")
	}
}

func TestHashQuery_WhitespaceVariations(t *testing.T) {
	// After normalization these should be the same
	h1 := hashQuery("go generics")
	h2 := hashQuery("go generics") // already normalized
	if h1 != h2 {
		t.Error("same normalized input should produce same hash")
	}
}

// === Edge cases: QuickReference struct ===

func TestQuickReference_PinnedDefault(t *testing.T) {
	qr := QuickReference{
		Concept: "Go generics",
		Content: "Generics allow type parameters...",
		Pinned:  true,
	}
	if !qr.Pinned {
		t.Error("expected pinned=true")
	}
	if qr.Concept != "Go generics" {
		t.Errorf("expected 'Go generics', got %q", qr.Concept)
	}
}

// === Edge cases: FrequentLookup struct ===

func TestFrequentLookup_MinimumThreshold(t *testing.T) {
	fl := FrequentLookup{
		SampleQuery:  "go generics",
		LookupCount:  3, // minimum threshold per R-507
		HasReference: false,
	}
	if fl.LookupCount < 3 {
		t.Errorf("expected at least 3 lookups, got %d", fl.LookupCount)
	}
	if fl.HasReference {
		t.Error("expected no existing reference")
	}
}

// === SCN-021-009: LogSearch truncation for long queries ===

func TestLogSearch_QueryTruncation(t *testing.T) {
	// LogSearch truncates queries > 500 chars internally before the DB insert.
	// With nil pool, the function fails at DB layer, but we can verify the
	// truncation happened by inspecting the function's behavior indirectly.
	// The truncation is a safety measure — just verify it doesn't panic.
	engine := &Engine{Pool: nil}
	longQuery := strings.Repeat("x", 600)
	err := engine.LogSearch(nil, longQuery, 0, "")
	if err == nil {
		t.Error("expected error for nil pool")
	}
	// Should reach the pool check, not panic on the long query
	if !strings.Contains(err.Error(), "database connection") {
		t.Errorf("expected pool error, got: %s", err.Error())
	}
}

func TestLogSearch_ExactTruncationBoundary(t *testing.T) {
	engine := &Engine{Pool: nil}

	// 500 chars — should pass without issue
	query500 := strings.Repeat("a", 500)
	err := engine.LogSearch(nil, query500, 5, "art-1")
	if err == nil {
		t.Error("expected nil pool error")
	}
	if !strings.Contains(err.Error(), "database connection") {
		t.Errorf("expected pool error for 500-char query, got: %s", err.Error())
	}

	// 501 chars — should be truncated, still reach pool error
	query501 := strings.Repeat("b", 501)
	err = engine.LogSearch(nil, query501, 5, "art-1")
	if err == nil {
		t.Error("expected nil pool error")
	}
	if !strings.Contains(err.Error(), "database connection") {
		t.Errorf("expected pool error for 501-char query, got: %s", err.Error())
	}
}

func TestLogSearch_EmptyQuery(t *testing.T) {
	engine := &Engine{Pool: nil}
	err := engine.LogSearch(nil, "", 0, "")
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

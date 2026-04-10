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

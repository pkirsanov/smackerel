// Spec 071 SCOPE-02 — Redactor unit tests (SCN-071-A03).

package intenttrace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultRedactor_PersistRawTextFalseHidesRawText(t *testing.T) {
	policy := NewSourcePolicy(false, []string{"location", "email"})
	result := NewDefaultRedactor().Redact(policy, "weather in Paris", map[string]any{
		"location": "Paris",
		"unit":     "celsius",
		"email":    "user@example.com",
	})
	if result.RawText != "absent" {
		t.Fatalf("expected raw_text=absent, got %q", result.RawText)
	}
	if result.Summary.RawText != "absent" {
		t.Fatalf("summary raw_text drift: %q", result.Summary.RawText)
	}
	if result.Summary.RedactedCount != 2 {
		t.Fatalf("expected 2 sensitive classes redacted, got %d", result.Summary.RedactedCount)
	}
	if result.Summary.SlotClasses["location"] != "redacted" || result.Summary.SlotClasses["email"] != "redacted" {
		t.Fatalf("sensitive slots must be marked redacted: %+v", result.Summary.SlotClasses)
	}
	if result.Summary.SlotClasses["unit"] != "safe" {
		t.Fatalf("non-sensitive slot must be marked safe: %+v", result.Summary.SlotClasses)
	}

	// No raw slot VALUE should appear anywhere in the serialized
	// summary — adversarial check that the redactor never leaks.
	b, err := json.Marshal(result.Summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "Paris") || strings.Contains(string(b), "user@example.com") {
		t.Fatalf("redaction leak: %s", string(b))
	}
}

func TestDefaultRedactor_PersistRawTextTrueKeepsDispositionMarker(t *testing.T) {
	policy := NewSourcePolicy(true, nil)
	result := NewDefaultRedactor().Redact(policy, "hello", map[string]any{"q": "x"})
	if result.RawText != "present" {
		t.Fatalf("expected raw_text=present, got %q", result.RawText)
	}
	if result.Summary.SlotClasses["q"] != "safe" {
		t.Fatalf("non-sensitive slot must be safe, got %v", result.Summary.SlotClasses)
	}
}

func TestDefaultRedactor_EmptyRawTextIsAbsent(t *testing.T) {
	policy := NewSourcePolicy(true, nil)
	result := NewDefaultRedactor().Redact(policy, "", nil)
	if result.RawText != "absent" {
		t.Fatalf("empty raw text must be absent, got %q", result.RawText)
	}
	if result.Summary.RedactedCount != 0 {
		t.Fatalf("no slots → redacted count 0, got %d", result.Summary.RedactedCount)
	}
	if len(result.Summary.SlotClasses) != 0 {
		t.Fatalf("no slots → empty classes, got %v", result.Summary.SlotClasses)
	}
}

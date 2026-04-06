package telegram

import (
	"testing"
)

func TestAllMarkers(t *testing.T) {
	markers := AllMarkers()
	if len(markers) != 8 {
		t.Errorf("expected 8 markers, got %d", len(markers))
	}

	expectedChars := []string{". ", "? ", "! ", "> ", "- ", "~ ", "# ", "@ "}
	for i, m := range markers {
		if m != expectedChars[i] {
			t.Errorf("marker %d: expected %q, got %q", i, expectedChars[i], m)
		}
	}
}

func TestFormatSuccess(t *testing.T) {
	result := FormatSuccess(`Saved: "Title" (article, 3 connections)`)
	expected := `. Saved: "Title" (article, 3 connections)`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatInfo(t *testing.T) {
	result := FormatInfo("3 results found")
	if result != "> 3 results found" {
		t.Errorf("expected '> 3 results found', got %q", result)
	}
}

func TestFormatList(t *testing.T) {
	result := FormatList([]string{"item 1", "item 2", "item 3"})
	expected := "- item 1\n- item 2\n- item 3"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestContainsEmoji_True(t *testing.T) {
	if !ContainsEmoji("Hello 😀 world") {
		t.Error("should detect emoji")
	}
}

func TestContainsEmoji_False(t *testing.T) {
	if ContainsEmoji("Hello world! - test #heading") {
		t.Error("should not detect emoji in normal text")
	}
}

func TestContainsEmoji_Markers(t *testing.T) {
	for _, m := range AllMarkers() {
		if ContainsEmoji(m) {
			t.Errorf("marker %q should not be detected as emoji", m)
		}
	}
}

func TestSanitizeOutput(t *testing.T) {
	result := SanitizeOutput("Hello 😀 world 🎉")
	if ContainsEmoji(result) {
		t.Errorf("sanitized output still contains emoji: %q", result)
	}
}

func TestSanitizeOutput_NoChange(t *testing.T) {
	input := ". Saved: \"Title\" (article)"
	result := SanitizeOutput(input)
	if result != input {
		t.Errorf("expected no change, got %q", result)
	}
}

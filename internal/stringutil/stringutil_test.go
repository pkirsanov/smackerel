package stringutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		wantLen  int
	}{
		{"short string", "abc", 10, 3},
		{"exact boundary", "abc", 3, 3},
		{"truncate ascii", "abcdef", 4, 4},
		{"empty string", "", 5, 0},
		{"multi-byte safe (3-byte rune)", "aé€x", 4, 3}, // 'a'(1) + 'é'(2) = 3; '€' starts at 3 (3 bytes) exceeds 4
		{"multi-byte safe (2-byte rune)", "aé€x", 3, 3},
		{"zero max", "abc", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateUTF8(tt.input, tt.maxBytes)
			if len(got) != tt.wantLen {
				t.Errorf("TruncateUTF8(%q, %d) len = %d, want %d", tt.input, tt.maxBytes, len(got), tt.wantLen)
			}
			if !utf8.ValidString(got) {
				t.Errorf("TruncateUTF8(%q, %d) produced invalid UTF-8: %q", tt.input, tt.maxBytes, got)
			}
		})
	}
}

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no special chars", "hello", "hello"},
		{"percent", "50%", "50\\%"},
		{"underscore", "a_b", "a\\_b"},
		{"backslash", `a\b`, `a\\b`},
		{"all specials", `a\%_b`, `a\\\%\_b`},
		{"empty", "", ""},
		{"email address", "user@host.com", "user@host.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeLikePattern(tt.input)
			if got != tt.want {
				t.Errorf("EscapeLikePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

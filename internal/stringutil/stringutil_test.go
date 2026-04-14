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

// SEC-021-002: SanitizeControlChars strips embedded C0 control characters from
// connector-imported data to prevent Telegram output corruption (CWE-116).
func TestSanitizeControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean ascii", "Hello World", "Hello World"},
		{"empty", "", ""},
		{"preserves newline", "line1\nline2", "line1\nline2"},
		{"preserves tab", "col1\tcol2", "col1\tcol2"},
		{"strips null byte", "before\x00after", "before after"},
		{"strips carriage return", "hello\rworld", "hello world"},
		{"strips escape sequence", "text\x1b[31mred", "text [31mred"},
		{"strips multiple control chars", "\x01\x02\x03abc", "   abc"},
		{"strips bell char", "alert\x07beep", "alert beep"},
		{"strips form feed", "page\x0cbreak", "page break"},
		{"strips vertical tab", "text\x0bmore", "text more"},
		{"mixed control and unicode", "café\x00naïve\x01end", "café naïve end"},
		{"only control chars", "\x00\x01\x02", "   "},
		{"preserves emoji", "🔔 alert\x00text", "🔔 alert text"},
		{"fast path no alloc", "no control chars here", "no control chars here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeControlChars(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeControlChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("SanitizeControlChars(%q) produced invalid UTF-8", tt.input)
			}
		})
	}
}

// SEC-021-002: Adversarial — verify that connector-sourced person/subscription
// names with embedded control chars are cleaned BEFORE alert text reaches the
// messaging layer.
func TestSanitizeControlChars_ConnectorDataAdversarial(t *testing.T) {
	// Simulates a person name from email headers with injected control chars
	maliciousName := "Alice\x00\x01Bob\rCharlie\x1b[0m"
	got := SanitizeControlChars(maliciousName)
	if got != "Alice  Bob Charlie [0m" {
		t.Errorf("connector name not sanitized: got %q", got)
	}

	// Simulates a subscription service_name with embedded null
	maliciousService := "Netflix\x00Premium"
	got = SanitizeControlChars(maliciousService)
	if got != "Netflix Premium" {
		t.Errorf("service name not sanitized: got %q", got)
	}

	// Simulates a trip destination with ANSI escape
	maliciousDest := "Tokyo\x1b[31m (hacked)"
	got = SanitizeControlChars(maliciousDest)
	if got != "Tokyo [31m (hacked)" {
		t.Errorf("destination not sanitized: got %q", got)
	}
}

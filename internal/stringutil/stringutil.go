package stringutil

import (
	"strings"
	"unicode/utf8"
)

// TruncateUTF8 truncates s to at most maxBytes bytes without splitting a
// multi-byte UTF-8 rune. The result is always valid UTF-8.
func TruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backward from the cut point to find a clean rune boundary.
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// SanitizeControlChars replaces ASCII C0 control characters (U+0000–U+001F)
// except newline (U+000A) and tab (U+0009) with spaces. This prevents output
// corruption when connector-imported data contains embedded null bytes, carriage
// returns, escape sequences, or other control characters (CWE-116).
func SanitizeControlChars(s string) string {
	clean := false
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' {
			clean = true
			break
		}
	}
	if !clean {
		return s // fast path: no control chars
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// EscapeLikePattern escapes SQL LIKE wildcard characters (\, %, _) in a string
// to prevent unintended pattern matching when used in LIKE clauses.
// Backslash must be escaped first to avoid double-escaping the other replacements.
func EscapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

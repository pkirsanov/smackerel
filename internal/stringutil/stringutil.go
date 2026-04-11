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

// EscapeLikePattern escapes SQL LIKE wildcard characters (\, %, _) in a string
// to prevent unintended pattern matching when used in LIKE clauses.
// Backslash must be escaped first to avoid double-escaping the other replacements.
func EscapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

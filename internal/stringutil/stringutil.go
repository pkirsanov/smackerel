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

// SanitizeHeaderValue replaces every byte that is an ASCII C0 control character
// (0x00–0x1F, which includes CR 0x0D, LF 0x0A, and TAB 0x09) or DEL (0x7F) with a
// single ASCII space (0x20). This neutralizes header-injection (CRLF) attacks
// (CWE-113) when an untrusted error string is written into a single-line wire
// header value such as the dead-letter Smackerel-Last-Error header (SEC-081-R1).
//
// It differs from SanitizeControlChars (which deliberately PRESERVES \n and \t
// for human-readable connector text): a wire header value must be a single line,
// so newlines and tabs are also collapsed to spaces here.
//
// Byte-length is preserved: every offending byte is a single-byte control replaced
// by a single-byte space, and multi-byte UTF-8 sequences never contain a byte < 0x80,
// so byte-oriented scanning never touches a continuation byte. Because the byte
// length is unchanged, a subsequent fixed-byte UTF-8 truncation lands on an identical
// boundary on every runtime — this is what lets the Go core and the Python sidecar
// (ml/app/nats_client.py::_sanitize_header_value) produce byte-for-byte equal values
// for the same input (spec 081 parity invariant).
func SanitizeHeaderValue(s string) string {
	needs := false
	for i := 0; i < len(s); i++ {
		if b := s[i]; b < 0x20 || b == 0x7F {
			needs = true
			break
		}
	}
	if !needs {
		return s // fast path: no control/DEL bytes
	}
	out := []byte(s)
	for i := 0; i < len(out); i++ {
		if out[i] < 0x20 || out[i] == 0x7F {
			out[i] = ' '
		}
	}
	return string(out)
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

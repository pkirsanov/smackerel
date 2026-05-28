// Spec 057 Scope 1 — unit tests for sanitizeNext.
//
// Test 1.4: rejects ALL 12 inputs from spec.md Scenario 6 matrix,
// accepts known-good paths.
package api

import "testing"

func TestSanitizeNext_RejectsHostileInputs(t *testing.T) {
	hostile := []struct {
		name string
		in   string
	}{
		{"absolute_https", "https://evil/"},
		{"protocol_relative", "//evil/"},
		{"backslash_trick", "/\\evil/"},
		{"javascript_lower", "javascript:x"},
		{"javascript_mixed", "JavaScript:x"},
		{"data_url", "data:text/html,x"},
		{"encoded_double_slash", "%2F%2Fevil"},
		{"encoded_backslash", "%5C%5Cevil"},
		{"login_loop", "/login"},
		{"login_loop_with_next", "/login?next=/foo"},
		{"empty", ""},
		{"no_leading_slash", "foo/bar"},
		{"cr_injection", "/foo\rSet-Cookie: x"},
		{"lf_injection", "/foo\nSet-Cookie: x"},
	}
	for _, tc := range hostile {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeNext(tc.in)
			if got != sanitizeNextDefault {
				t.Errorf("sanitizeNext(%q) = %q, want %q", tc.in, got, sanitizeNextDefault)
			}
		})
	}
}

func TestSanitizeNext_AcceptsSafePaths(t *testing.T) {
	safe := []string{"/", "/dashboard", "/notes/abc?q=1#frag"}
	for _, in := range safe {
		t.Run(in, func(t *testing.T) {
			got := sanitizeNext(in)
			if got != in {
				t.Errorf("sanitizeNext(%q) = %q, want %q", in, got, in)
			}
		})
	}
}

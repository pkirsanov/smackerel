// Spec 057 — open-redirect protection for the `?next=` query
// parameter and the hidden form field on /login. design.md §"next
// Sanitization (open-redirect protection)" defines the full input
// matrix; see Scenario 6 in spec.md.
//
// Single source of truth. Called from BOTH the login page renderer
// (GET /login `?next=` query) AND the form POST handler (hidden
// `next` field) — never trust the client.
package api

import (
	"net/url"
	"strings"
)

// sanitizeNextDefault is the fallback path returned whenever the
// supplied `raw` is missing, malformed, or considered hostile.
const sanitizeNextDefault = "/"

// sanitizeNext validates and normalises a candidate `next=` value.
// It returns the original (decoded) value when safe, or "/" otherwise.
//
// The validator MUST reject (per spec.md Scenario 6 + design.md):
//   - empty string
//   - inputs containing CR or LF (header-injection)
//   - inputs whose percent-decoded form does not start with exactly one "/"
//   - protocol-relative ("//evil") or backslash-trick ("/\\evil")
//   - parse failures, non-empty scheme, non-empty host
//   - path == "/login" (login-loop)
func sanitizeNext(raw string) string {
	if raw == "" {
		return sanitizeNextDefault
	}
	// 1. Header-injection guard on the raw (pre-decode) input.
	if strings.ContainsAny(raw, "\r\n") {
		return sanitizeNextDefault
	}
	// 2. Decode percent-encoding so smuggled `%2F%2F` etc. are caught.
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return sanitizeNextDefault
	}
	// 3. Must be a path — starts with exactly one "/".
	if !strings.HasPrefix(decoded, "/") {
		return sanitizeNextDefault
	}
	if strings.HasPrefix(decoded, "//") || strings.HasPrefix(decoded, "/\\") {
		return sanitizeNextDefault
	}
	// 4. Parse and require empty scheme + empty host.
	u, err := url.Parse(decoded)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return sanitizeNextDefault
	}
	// 5. Reject login-loop on the path component only.
	if u.Path == loginPath {
		return sanitizeNextDefault
	}
	return decoded
}

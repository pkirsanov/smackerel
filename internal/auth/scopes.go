// Package auth — canonical scope-name registry and validator (spec 060).
//
// The PASETO `scope` claim carries one or more scope names of the
// shape `<surface>:<capability>` (e.g. `extension:bookmarks,history`).
// Surfaces are declared once in `RegisteredScopeSurfaces`; the CLI and
// the middleware both consult this list so the spec 060 single
// source of truth holds.
//
// Spec 060 design.md §5.3 — single registry at internal/auth/scopes.go.
package auth

import (
	"fmt"
	"regexp"
	"strings"
)

// ScopeNameRegex is the wire-format validator for individual scope
// strings. The form `<surface>:<capabilities>` is the spec 060
// invariant; the surface must start with a lowercase letter and
// contain only lowercase alphanumerics; the capability tail accepts
// lowercase alphanumerics plus `,`, `_`, and `-` (the `,` supports
// the comma-separated capability list `bookmarks,history`).
var ScopeNameRegex = regexp.MustCompile(`^[a-z][a-z0-9]*:[a-z0-9,_-]+$`)

// RegisteredScopeSurfaces is the closed-set allowlist of scope
// surfaces accepted by `ValidateScopeName` without the
// `--allow-unknown-surface` escape hatch. Additions to this list
// MUST land in the same change set as the spec that introduces the
// new surface.
//
// Spec 060 scope 1 — initial entry is `extension` (consumed by spec
// 058 OQ-DSN-1). Spec 027 scope 9 adds `annotation` (consumed by the
// spec 073 graph-browse UI; annotation:edit, annotation:read).
var RegisteredScopeSurfaces = []string{"extension", "annotation"}

// ValidateScopeName returns nil when `scope` matches `ScopeNameRegex`
// and a non-nil error otherwise. The error wraps the offending value
// so operator CLI output names the bad token. The scope vocabulary
// is operator-defined; there is no secret-leakage risk in echoing
// the value back.
func ValidateScopeName(scope string) error {
	if !ScopeNameRegex.MatchString(scope) {
		return fmt.Errorf("invalid scope name %q: must match %s", scope, ScopeNameRegex.String())
	}
	return nil
}

// ExtractScopeSurface returns the substring before the first `:`.
// Pre-condition: the caller has validated `scope` via
// `ValidateScopeName`. Behavior on an unvalidated input is the empty
// string when `:` is absent.
func ExtractScopeSurface(scope string) string {
	idx := strings.IndexByte(scope, ':')
	if idx < 0 {
		return ""
	}
	return scope[:idx]
}

// IsRegisteredScopeSurface reports whether `surface` appears in the
// `RegisteredScopeSurfaces` allowlist. Used by the CLI to decide
// whether `--allow-unknown-surface` is required.
func IsRegisteredScopeSurface(surface string) bool {
	for _, s := range RegisteredScopeSurfaces {
		if s == surface {
			return true
		}
	}
	return false
}

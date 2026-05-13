package config

import "strings"

// Spec 051 — defense-in-depth dev-default secret detection.
//
// DevDBPasswords lists known-bad Postgres password values that MUST be
// rejected outside dev/test. The list intentionally lives in Go (this
// file) and is duplicated as a parallel grep-friendly list inside
// scripts/commands/config.sh — both layers MUST refuse the same values
// (FR-051-005, SCN-051-S02). Keep the two lists in sync; a downstream
// hardening spec may introduce code-generation if drift becomes a
// concern, but spec 051 keeps the list explicit and short.
//
// Update procedure: when adding a value here, also add it to the case
// arms in the dev-default rejection block in scripts/commands/config.sh
// adjacent to the POSTGRES_PASSWORD resolution.
var DevDBPasswords = []string{
	"smackerel", // config/smackerel.yaml dev default
	"postgres",  // upstream image default
	"password",  // common dev smell
	"changeme",  // common default
	"change-me", // common default
	"default",   // common default
}

// IsDevDBPassword reports whether pw matches a known dev-default
// Postgres password value. The comparison is case-insensitive because
// operators sometimes uppercase secrets when copy-pasting and the gate
// is meant to refuse the value regardless of casing.
//
// Empty input returns false: empty-password handling is the
// responsibility of the caller (in practice the SST loader rejects
// empty values via required_value before this helper is ever consulted).
func IsDevDBPassword(pw string) bool {
	if pw == "" {
		return false
	}
	lower := strings.ToLower(pw)
	for _, dev := range DevDBPasswords {
		if lower == dev {
			return true
		}
	}
	return false
}

// extractDatabasePassword extracts the password component from a
// PostgreSQL connection URL of the form:
//
//	postgres://user:password@host:port/dbname?param=value
//
// Returns the empty string when the URL has no password component or
// when the URL cannot be parsed. The function is intentionally
// permissive: it does NOT validate the URL shape (Validate() does that
// elsewhere). Spec 051 only needs the password substring to compare it
// against DevDBPasswords.
func extractDatabasePassword(databaseURL string) string {
	if databaseURL == "" {
		return ""
	}
	// Find the scheme separator.
	schemeIdx := strings.Index(databaseURL, "://")
	if schemeIdx < 0 {
		return ""
	}
	rest := databaseURL[schemeIdx+3:]
	// Userinfo ends at the first '@' before any path segment. We use
	// LastIndex over the userinfo portion because passwords can contain
	// special characters but '@' inside the password would be
	// percent-encoded by any compliant URL producer.
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		return ""
	}
	userinfo := rest[:atIdx]
	colonIdx := strings.Index(userinfo, ":")
	if colonIdx < 0 {
		return ""
	}
	return userinfo[colonIdx+1:]
}

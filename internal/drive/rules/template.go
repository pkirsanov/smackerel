package rules

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidToken is returned by RenderTargetPath when the template references
// a {token} that is not present in the supplied token set. It is wrapped with
// %w so the Save Service can use errors.Is to surface "invalid token" failures
// in drive_save_requests.last_error.
var ErrInvalidToken = errors.New("rules: invalid template token")

// RenderTargetPath substitutes {token} placeholders in template with the
// supplied tokens map. Tokens MUST be lowercase ASCII identifiers (matching
// design.md §5.2). Missing tokens fail with ErrInvalidToken so misconfigured
// rules surface in Screen 7 rather than silently writing garbage paths.
//
// The renderer normalizes the result by trimming leading/trailing slashes
// and collapsing repeated slashes, so callers can author templates with or
// without leading "/" without affecting the resolved provider path.
func RenderTargetPath(template string, tokens map[string]string) (string, error) {
	if strings.TrimSpace(template) == "" {
		return "", errors.New("rules: empty target template")
	}
	var builder strings.Builder
	idx := 0
	for idx < len(template) {
		ch := template[idx]
		if ch != '{' {
			builder.WriteByte(ch)
			idx = idx + 1
			continue
		}
		closeIndex := strings.IndexByte(template[idx:], '}')
		if closeIndex < 0 {
			return "", fmt.Errorf("%w: unterminated token at offset %d", ErrInvalidToken, idx)
		}
		tokenName := template[idx+1 : idx+closeIndex]
		if !isLegalTokenName(tokenName) {
			return "", fmt.Errorf("%w: %q", ErrInvalidToken, tokenName)
		}
		value, ok := tokens[tokenName]
		if !ok {
			return "", fmt.Errorf("%w: %q", ErrInvalidToken, tokenName)
		}
		if value == "" {
			return "", fmt.Errorf("%w: %q resolved to empty value", ErrInvalidToken, tokenName)
		}
		builder.WriteString(value)
		idx = idx + closeIndex + 1
	}
	return normalizeRenderedPath(builder.String()), nil
}

func isLegalTokenName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i = i + 1 {
		ch := name[i]
		isLower := ch >= 'a' && ch <= 'z'
		isDigit := ch >= '0' && ch <= '9'
		if !(isLower || isDigit || ch == '_') {
			return false
		}
	}
	return true
}

func normalizeRenderedPath(p string) string {
	parts := strings.Split(p, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	return strings.Join(cleaned, "/")
}

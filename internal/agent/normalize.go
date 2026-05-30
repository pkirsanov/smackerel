// Package agent — BUG-061-003 router input normalization.
//
// NormalizeForRouting applies a closed alias map to the raw utterance
// before the router embeds it. The original RawInput in the envelope
// is preserved for downstream skills, audit, and disambiguation
// traces; only the text handed to the embedder is rewritten.
//
// The alias set is intentionally tiny and locked. Future expansion is
// a separate scope per BUG-061-003 design D2.
package agent

import (
	"strings"
	"unicode"
)

// routerAliases is the closed normalization map. Lower-cased compare;
// emit casing is the canonical map value verbatim.
var routerAliases = map[string]string{
	"recepie":  "recipe",
	"recipie":  "recipe",
	"recipies": "recipes",
	"recepies": "recipes",
}

// NormalizeForRouting rewrites tokens in s according to routerAliases.
// Tokenization preserves the original whitespace/punctuation
// separators so the embedder sees an utterance shaped identically to
// the user input apart from the aliased words.
func NormalizeForRouting(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))

	start := 0
	inToken := false
	for i, r := range s {
		if isTokenRune(r) {
			if !inToken {
				start = i
				inToken = true
			}
			continue
		}
		if inToken {
			emitToken(&b, s[start:i])
			inToken = false
		}
		b.WriteRune(r)
	}
	if inToken {
		emitToken(&b, s[start:])
	}
	return b.String()
}

func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func emitToken(b *strings.Builder, tok string) {
	if alias, ok := routerAliases[strings.ToLower(tok)]; ok {
		b.WriteString(alias)
		return
	}
	b.WriteString(tok)
}

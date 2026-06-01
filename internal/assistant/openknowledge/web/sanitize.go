package web

import (
	"strings"
	"unicode/utf8"
)

// MaxSnippetRunes is the upper bound applied by SanitizeSnippet to
// every provider-returned snippet before the agent loop hands it to
// the LLM. Rationale: a single web provider may return tens of
// kilobytes per result; the planner's per-query token budget
// (PerQueryTokenBudget — currently 8000 in config/smackerel.yaml)
// makes it expensive to ferry untruncated payloads through the
// model. 2_000 runes is roughly 500 tokens worst-case, which lines
// up with the design's WebSnippet "summary, not full page" contract
// (design.md §Tooling). Adversarial: a planner that emits a craft
// query designed to harvest a large dump cannot blow the token
// budget through snippet bloat.
const MaxSnippetRunes = 2000

// truncationMarker is appended (replacing the last rune of the
// truncated body) when a snippet exceeds MaxSnippetRunes, so the
// LLM observably sees that content was cut and does not silently
// over-trust a partial payload.
const truncationMarker = "…"

// SuspiciousSnippetRecorder is the optional metric sink invoked by
// SanitizeSnippet whenever a suspicious-prompt-injection pattern is
// matched. Implementations MUST be safe for concurrent use. nil is
// permitted — sanitisation runs without recording.
//
// This is the boundary the metrics package implements
// (openknowledge_suspicious_snippet_total counter). Keeping the
// interface inside the web package preserves the package-level
// independence of the openknowledge/metrics package from
// openknowledge/web.
type SuspiciousSnippetRecorder interface {
	IncSuspiciousSnippet(provider string)
}

// suspiciousPatterns are case-insensitive substrings that are
// frequently found in attempted prompt-injection payloads embedded
// in user-visible web content. The detector is intentionally
// conservative: SanitizeSnippet does NOT strip matched content —
// the LLM-side prompt boundary (tool_output envelope from
// design.md §Security) is the primary defence; this detector
// merely emits an observability signal so an operator can spot
// anomalous traffic.
//
// Adding patterns is safe; removing them weakens defence-in-depth.
var suspiciousPatterns = []string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"disregard previous instructions",
	"disregard the above",
	"you are now in developer mode",
	"you are now in dan mode",
	"system:",
	"<|im_start|>",
	"<|im_end|>",
	"<|endoftext|>",
	"[inst]",
	"[/inst]",
	"<<sys>>",
	"<</sys>>",
}

// SanitizeSnippet treats an incoming web-provider snippet as
// untrusted text and applies three transforms:
//
//  1. Replace invalid UTF-8 byte sequences with the Unicode
//     replacement rune (U+FFFD), so the LLM never sees a malformed
//     prompt that could confuse downstream tokenisers.
//  2. Strip ASCII control characters except '\t' and '\n' — these
//     are the only whitespace runes the planner needs to see.
//     Stripping prevents an upstream payload from injecting BEL,
//     NUL, ESC sequences, or other ANSI smuggling vectors into the
//     tool_output envelope.
//  3. Truncate to MaxSnippetRunes, appending truncationMarker so
//     the LLM can observe that the snippet was clipped.
//
// SanitizeSnippet ALSO scans the (post-strip, pre-truncate) text
// for suspiciousPatterns. When provider is non-empty AND a
// suspicious pattern matches, the recorder (if non-nil) is
// incremented. Content is intentionally NOT stripped — the
// LLM-side prompt boundary (tool_output envelope) is the primary
// defence. This is defence-in-depth observability, not a
// substitute for LLM-side prompt fencing.
//
// SanitizeSnippet is pure with respect to its inputs aside from
// the recorder side-effect; calling it with provider == ""
// suppresses the metric increment (useful in unit tests that want
// to validate sanitisation without metric noise).
func SanitizeSnippet(provider, snippet string, recorder SuspiciousSnippetRecorder) string {
	if snippet == "" {
		return ""
	}
	// 1. UTF-8 normalisation.
	if !utf8.ValidString(snippet) {
		snippet = strings.ToValidUTF8(snippet, string(utf8.RuneError))
	}
	// 2. Strip control characters (keep \t and \n).
	var b strings.Builder
	b.Grow(len(snippet))
	for _, r := range snippet {
		if r == '\t' || r == '\n' {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	stripped := b.String()

	// 3. Suspicious-pattern detection (observability only).
	if recorder != nil && provider != "" {
		lower := strings.ToLower(stripped)
		for _, pat := range suspiciousPatterns {
			if strings.Contains(lower, pat) {
				recorder.IncSuspiciousSnippet(provider)
				break
			}
		}
	}

	// 4. Truncate.
	return truncateRunes(stripped, MaxSnippetRunes)
}

// truncateRunes returns s clipped to at most max runes. When s is
// truncated, the last rune of the kept portion is replaced by
// truncationMarker so the boundary is observable downstream.
// max == 0 returns "" unconditionally; max < 0 returns s unchanged.
func truncateRunes(s string, max int) string {
	if max < 0 {
		return s
	}
	if max == 0 {
		return ""
	}
	count := utf8.RuneCountInString(s)
	if count <= max {
		return s
	}
	// Keep (max-1) runes from s, then append truncationMarker.
	keep := max - 1
	if keep <= 0 {
		return truncationMarker
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for _, r := range s {
		if i == keep {
			break
		}
		b.WriteRune(r)
		i++
	}
	b.WriteString(truncationMarker)
	return b.String()
}

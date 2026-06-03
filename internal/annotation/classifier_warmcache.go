// Spec 076 SCOPE-4b — WarmCacheClassifier decorator.
//
// Bounded ≤5-entry exact-token table covering the highest-frequency
// inputs from recipe / book / place / generic flows. Documented in
// design.md (spec 066, SCOPE-5 subsection) as
// **"latency cache, not source of truth"**: every entry MUST be
// derivable from the underlying Classifier's training examples; a
// separate consistency test (`TestAnnotationWarmCacheAgreesWithCompiledIntent`)
// guards drift.
//
// Activation contract:
//
//  1. The decorator consults the cache ONLY for inputs whose
//     normalized form (lower-cased + whitespace-collapsed + trimmed)
//     is EXACTLY one of the cached tokens. No substring matching,
//     no multi-phrase composition.
//  2. The cache is consulted ONLY when `enabled = true`
//     (resolved from SST key
//     `assistant.annotation.classifier.warm_cache_enabled`, owned
//     by Scope 1).
//  3. A cache hit returns the cached InteractionType with
//     confidence = 1.0 and skips the underlying classifier. A miss
//     (or `enabled = false`) delegates to the underlying classifier
//     verbatim.
//  4. The cache is NEVER consulted as a fallback for classifier
//     errors — on `inner.Classify` failure the decorator surfaces
//     the error to the caller.
package annotation

import (
	"context"
	"regexp"
	"strings"
)

// warmCacheTable is the canonical ≤5-entry exact-token table. The
// values match the legacy inline `interactionMap` literal in
// parser.go for the same tokens; SCOPE-4b's shadow comparator
// detects any drift between this table and the LLM classifier's
// view.
var warmCacheTable = map[string]InteractionType{
	"made it":   InteractionMadeIt,
	"cooked it": InteractionMadeIt,
	"bought it": InteractionBoughtIt,
	"read it":   InteractionReadIt,
	"visited":   InteractionVisited,
}

var warmCacheWhitespaceRe = regexp.MustCompile(`\s+`)

// WarmCacheClassifier wraps an underlying Classifier with the
// bounded exact-token warm cache.
type WarmCacheClassifier struct {
	Inner   Classifier
	Enabled bool
}

// NewWarmCacheClassifier returns a WarmCacheClassifier. `enabled` is
// the SST-resolved value of
// `assistant.annotation.classifier.warm_cache_enabled`; the caller
// (cmd/core/wiring.go) MUST pass the LoadAnnotationClassifier()
// result, never an in-source default (Gate G028 / no-defaults).
func NewWarmCacheClassifier(inner Classifier, enabled bool) *WarmCacheClassifier {
	return &WarmCacheClassifier{Inner: inner, Enabled: enabled}
}

// Classify consults the warm cache when enabled; falls through to
// the underlying classifier on miss or when disabled.
func (w *WarmCacheClassifier) Classify(ctx context.Context, text string, channel SourceChannel) (InteractionType, float64, error) {
	if w.Enabled {
		key := normalizeWarmCacheKey(text)
		if it, ok := warmCacheTable[key]; ok {
			return it, 1.0, nil
		}
	}
	if w.Inner == nil {
		return "", 0.0, ErrClassifierUnavailable
	}
	return w.Inner.Classify(ctx, text, channel)
}

// normalizeWarmCacheKey applies the exact-token activation
// normalization documented above: lower-case, collapse whitespace,
// trim.
func normalizeWarmCacheKey(text string) string {
	return strings.TrimSpace(warmCacheWhitespaceRe.ReplaceAllString(strings.ToLower(text), " "))
}

// WarmCacheTokens returns a snapshot of the cached tokens. Test-only
// helper consumed by the consistency-with-compiled-intent guard.
func WarmCacheTokens() []string {
	out := make([]string, 0, len(warmCacheTable))
	for k := range warmCacheTable {
		out = append(out, k)
	}
	return out
}

// Compile-time assertion: *WarmCacheClassifier implements Classifier.
var _ Classifier = (*WarmCacheClassifier)(nil)

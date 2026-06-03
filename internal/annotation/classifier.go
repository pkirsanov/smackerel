// Spec 076 SCOPE-4b — Annotation Classifier interface + variants +
// dual-write shadow comparator (additive only; the inline
// `interactionMap` literal in parser.go is intentionally UNCHANGED).
//
// Scope boundary (per specs/076-assistant-completion-rescope/scopes.md):
//
//   - SCOPE-4b ships the Classifier interface, the
//     `annotation.classify.v1` bridge-backed implementation, the
//     warm-cache decorator, and a dual-write ShadowComparator that
//     runs both classification paths and emits divergence telemetry
//     (counter + structured log line) for every annotation call.
//   - SCOPE-4b explicitly DOES NOT delete the inline `interactionMap`
//     literal in `internal/annotation/parser.go`. Deletion is owned
//     by SCOPE-4c, gated on documented zero-divergence shadow
//     telemetry across a full release window.
//
// Design source: specs/066-legacy-keyword-surface-retirement/design.md
// → "Annotation Classification Replacement (SCOPE-5)" subsection.
package annotation

import (
	"context"
	"errors"
)

// Classifier maps free-text annotation input to a domain InteractionType.
//
// Implementations:
//
//   - InlineClassifier — wraps the legacy inline `interactionMap` /
//     `sortedInteractionPhrasesList` path via `Parse(text)`. Used as
//     the PRIMARY result by the dual-write shadow comparator while
//     SCOPE-4b is in shadow mode.
//   - BridgeClassifier — wraps `agent.Bridge.Invoke` with an
//     explicit `ScenarioID: "annotation_classify"` envelope. The
//     production replacement target.
//   - WarmCacheClassifier — bounded ≤5-entry exact-token decorator
//     in front of any underlying Classifier; documented as
//     "latency cache, not source of truth" (see design.md).
//
// Confidence ∈ [0,1]. Implementations MUST return
// `ErrBelowConfidenceFloor` with empty InteractionType when the
// classifier is uncertain (confidence below
// `assistant.annotation.classifier.confidence_floor`); callers route
// borderline cases through spec 061 disambiguation rather than
// guessing.
type Classifier interface {
	Classify(ctx context.Context, text string, channel SourceChannel) (InteractionType, float64, error)
}

// ErrBelowConfidenceFloor is returned by classifiers that produced a
// classification whose calibrated confidence is below the configured
// floor. The InteractionType returned alongside this error MUST be
// empty so callers cannot accidentally consume a guessed value.
var ErrBelowConfidenceFloor = errors.New("annotation classifier: confidence below configured floor")

// ErrClassifierUnavailable indicates the underlying classifier
// (Bridge / warm cache miss / LLM driver) could not produce a result.
// Surfaces translate this to an operational error and do NOT silently
// fall back — the user-facing handler returns a structured error
// rather than guessing.
var ErrClassifierUnavailable = errors.New("annotation classifier: unavailable")

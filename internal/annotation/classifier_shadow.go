// Spec 076 SCOPE-4b — dual-write shadow comparator.
//
// For every annotation call wired through this comparator, the
// existing inline `interactionMap` path (PRIMARY, computed by the
// caller via `Parse(text)`) is compared against the new
// `annotation.classify.v1` classifier (SHADOW). Both outcomes are
// recorded; divergences emit a per-call structured log line plus a
// Prometheus counter increment so dashboards can prove zero-divergence
// across the release window before SCOPE-4c removes the inline
// literal.
//
// The shadow path is fire-and-compare ONLY — the comparator never
// changes the value the caller acts on. Its result is for telemetry
// only. The caller MUST continue to use the PRIMARY value
// (parsed.InteractionType from the inline literal) as the source of
// truth until SCOPE-4c flips the cut-over.
package annotation

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
)

// ShadowComparator runs the new classifier alongside the legacy
// inline `interactionMap` path and emits divergence telemetry.
//
// Zero value is unusable; construct via NewShadowComparator. A nil
// *ShadowComparator is a safe no-op for Compare so call-sites can
// guard wiring with a single `if cmp != nil` check.
type ShadowComparator struct {
	Shadow Classifier

	// Logger is the slog handle used to emit per-divergence log
	// lines. MUST be non-nil; wiring passes slog.Default() unless
	// the operator scopes per-subsystem.
	Logger *slog.Logger

	// ShadowTimeout caps each shadow Classifier call. Required
	// (> 0); wiring sets this from the scenario's `limits.timeout_ms`
	// to keep the shadow path from extending the user-facing
	// annotation request beyond its own budget.
	ShadowTimeout time.Duration
}

// NewShadowComparator constructs a fully-wired ShadowComparator.
// `shadow` is the (typically warm-cache decorated) BridgeClassifier
// produced by the wiring layer; passing nil disables the comparator
// (the returned value is nil and call-sites no-op).
func NewShadowComparator(shadow Classifier, logger *slog.Logger, shadowTimeout time.Duration) *ShadowComparator {
	if shadow == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	if shadowTimeout <= 0 {
		// Defensive cap so a misconfigured wiring layer cannot let
		// the shadow path block the request indefinitely. The
		// production wiring MUST pass a positive value sourced
		// from the scenario's limits.timeout_ms; this fallback
		// only protects against constructor misuse, never silently
		// hides production drift.
		shadowTimeout = 15 * time.Second
	}
	return &ShadowComparator{Shadow: shadow, Logger: logger, ShadowTimeout: shadowTimeout}
}

// Compare runs the shadow classifier and records the comparator
// outcome relative to `primary` (the InteractionType the caller
// already computed from the inline interactionMap via Parse()).
//
// Compare is safe for concurrent use; it does NOT mutate ParsedAnnotation
// and does NOT return a value the caller should act on. Its sole job is
// to emit telemetry so SCOPE-4c can prove the shadow agrees with the
// inline literal before the literal is removed.
//
// A nil *ShadowComparator is a no-op (safe to wire when the bridge is
// not yet available, e.g. during early-boot integration tests).
func (s *ShadowComparator) Compare(ctx context.Context, text string, channel SourceChannel, primary InteractionType) {
	if s == nil || s.Shadow == nil {
		return
	}

	shadowCtx, cancel := context.WithTimeout(ctx, s.ShadowTimeout)
	defer cancel()

	shadowResult, shadowConfidence, err := s.Shadow.Classify(shadowCtx, text, channel)

	channelLabel := string(channel)
	primaryLabel := string(primary)
	shadowLabel := string(shadowResult)

	switch {
	case errors.Is(err, ErrBelowConfidenceFloor):
		metrics.AnnotationClassifierShadowCalls.WithLabelValues(channelLabel, "shadow_below_floor").Inc()
		s.Logger.Info("annotation classifier shadow declined (below confidence floor)",
			"spec", "076",
			"scope", "SCOPE-4b",
			"channel", channelLabel,
			"primary_type", primaryLabel,
			"shadow_confidence", shadowConfidence,
		)
		return
	case err != nil:
		metrics.AnnotationClassifierShadowCalls.WithLabelValues(channelLabel, "shadow_error").Inc()
		s.Logger.Warn("annotation classifier shadow error",
			"spec", "076",
			"scope", "SCOPE-4b",
			"channel", channelLabel,
			"primary_type", primaryLabel,
			"error", err.Error(),
		)
		return
	case shadowResult == primary:
		metrics.AnnotationClassifierShadowCalls.WithLabelValues(channelLabel, "match").Inc()
		return
	default:
		metrics.AnnotationClassifierShadowCalls.WithLabelValues(channelLabel, "divergence").Inc()
		metrics.AnnotationClassifierDivergence.WithLabelValues(channelLabel, primaryLabel, shadowLabel).Inc()
		s.Logger.Warn("annotation classifier shadow divergence",
			"spec", "076",
			"scope", "SCOPE-4b",
			"channel", channelLabel,
			"primary_type", primaryLabel,
			"shadow_type", shadowLabel,
			"shadow_confidence", shadowConfidence,
		)
	}
}

// Spec 076 SCOPE-4b — annotation classifier shadow-comparator wiring.
//
// Constructs the `Classifier` chain
//
//	*BridgeClassifier --> *WarmCacheClassifier --> *ShadowComparator
//
// from the live agent.Bridge and the SST-resolved
// `assistant.annotation.classifier.*` configuration, then injects the
// comparator into:
//
//   - deps.AnnotationHandlers (HTTP POST /api/artifacts/{id}/annotations)
//   - the Telegram bot (reply-to capture + disambiguation finalizer)
//
// The comparator runs the new `annotation.classify.v1` scenario
// alongside the legacy inline `interactionMap` path and emits
// divergence telemetry. The PRIMARY behaviour (the inline literal in
// internal/annotation/parser.go) is intentionally untouched in
// SCOPE-4b; deletion is owned by SCOPE-4c after the shadow telemetry
// proves zero divergence across a release window.
package main

import (
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/telegram"
)

// wireAnnotationShadowComparator builds the shadow comparator from a
// live bridge + SST config and injects it into the API handler and
// (optionally) the Telegram bot. Nil bridge ⇒ no-op (the comparator
// is silently disabled, which is the correct behaviour when the
// bridge has not been wired yet, e.g. during partial-boot integration
// tests).
func wireAnnotationShadowComparator(bridge *agent.Bridge, deps *api.Dependencies, tgBot *telegram.Bot) {
	if bridge == nil {
		slog.Warn("annotation shadow comparator skipped: agent bridge is nil",
			"spec", "076", "scope", "SCOPE-4b")
		return
	}

	cls, err := config.LoadAnnotationClassifier()
	if err != nil {
		// Fail-loud: SCOPE-1 already ships these SST keys, so a load
		// error here means an operator misconfigured the environment.
		// We log and leave the comparator disabled so the rest of the
		// runtime keeps starting up — the operator can fix and restart.
		slog.Error("annotation shadow comparator disabled: SST load failed",
			"spec", "076", "scope", "SCOPE-4b", "error", err.Error())
		return
	}

	bridgeClassifier := &annotation.BridgeClassifier{
		Runner:          bridge,
		ConfidenceFloor: cls.ConfidenceFloor,
	}
	warmCache := annotation.NewWarmCacheClassifier(bridgeClassifier, cls.WarmCacheEnabled)

	// Shadow timeout sourced from the annotation_classify scenario's
	// limits.timeout_ms (15s — see config/prompt_contracts/annotation-classify-v1.yaml).
	// Hard-coded here rather than reading the loaded scenario's
	// runtime metadata to avoid coupling the wiring layer to the
	// scenario loader's internal types; the YAML value is the
	// authoritative source.
	const shadowTimeout = 15 * time.Second

	comparator := annotation.NewShadowComparator(warmCache, slog.Default(), shadowTimeout)

	if deps != nil && deps.AnnotationHandlers != nil {
		deps.AnnotationHandlers.ShadowComparator = comparator
	}
	if tgBot != nil {
		tgBot.SetAnnotationShadowComparator(comparator)
	}

	slog.Info("annotation shadow comparator wired",
		"spec", "076",
		"scope", "SCOPE-4b",
		"warm_cache_enabled", cls.WarmCacheEnabled,
		"confidence_floor", cls.ConfidenceFloor,
		"shadow_timeout", shadowTimeout,
	)
}

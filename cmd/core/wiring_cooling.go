// Spec 021 BUG-021-005 — LLM-driven relationship-cooling evaluator wiring.
//
// Constructs the BridgeCoolingEvaluator from the live agent.Bridge and the
// SST-resolved intelligence.relationship_cooling.* operational bounds, then
// injects it into the intelligence engine. This replaces the previous
// hardcoded magic-number cooling heuristic: the "is this relationship
// cooling?" judgment now flows through the relationship_cooling_evaluate
// scenario (docs/smackerel.md §3.6 — domain reasoning is LLM-driven).
//
// Nil bridge ⇒ no-op: cooling-alert production stays disabled (there is NO
// hardcoded fallback heuristic), which is the correct behavior when the
// bridge has not been wired yet (e.g. partial-boot integration tests).
package main

import (
	"log/slog"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence"
)

func wireRelationshipCoolingEvaluator(bridge *agent.Bridge, engine *intelligence.Engine) {
	if engine == nil {
		return
	}
	if bridge == nil {
		slog.Warn("relationship cooling evaluator skipped: agent bridge is nil; cooling alerts disabled",
			"spec", "021", "bug", "BUG-021-005")
		return
	}

	coolingCfg, err := config.LoadRelationshipCoolingConfig()
	if err != nil {
		// Fail-loud SST keys are shipped by this change; a load error means
		// the operator misconfigured the environment. Leave cooling disabled
		// (no hardcoded fallback) and keep the rest of the runtime booting.
		slog.Error("relationship cooling evaluator disabled: SST load failed",
			"spec", "021", "bug", "BUG-021-005", "error", err.Error())
		return
	}

	engine.SetCoolingConfig(&intelligence.CoolingConfig{
		Evaluator:       &intelligence.BridgeCoolingEvaluator{Runner: bridge},
		MaxCandidates:   coolingCfg.MaxCandidates,
		ConfidenceFloor: coolingCfg.ConfidenceFloor,
		DedupWindowDays: coolingCfg.DedupWindowDays,
	})
	slog.Info("relationship cooling evaluator wired (LLM-driven)",
		"spec", "021", "bug", "BUG-021-005",
		"max_candidates", coolingCfg.MaxCandidates,
		"confidence_floor", coolingCfg.ConfidenceFloor,
		"dedup_window_days", coolingCfg.DedupWindowDays)
}

// wireAlertTimingEvaluator injects the LLM-driven alert-timing evaluator into
// the intelligence engine for the bill / trip-prep / return-window producers
// (BUG-021-006). Nil bridge ⇒ no-op (those producers stay disabled; there is
// no hardcoded alert-timing window fallback).
func wireAlertTimingEvaluator(bridge *agent.Bridge, engine *intelligence.Engine) {
	if engine == nil {
		return
	}
	if bridge == nil {
		slog.Warn("alert timing evaluator skipped: agent bridge is nil; bill/trip/return alerts disabled",
			"spec", "021", "bug", "BUG-021-006")
		return
	}

	timingCfg, err := config.LoadAlertTimingConfig()
	if err != nil {
		slog.Error("alert timing evaluator disabled: SST load failed",
			"spec", "021", "bug", "BUG-021-006", "error", err.Error())
		return
	}

	engine.SetAlertTimingConfig(&intelligence.AlertTimingConfig{
		Evaluator:       &intelligence.BridgeAlertTimingEvaluator{Runner: bridge},
		LookaheadDays:   timingCfg.LookaheadDays,
		MaxCandidates:   timingCfg.MaxCandidates,
		ConfidenceFloor: timingCfg.ConfidenceFloor,
	})
	slog.Info("alert timing evaluator wired (LLM-driven)",
		"spec", "021", "bug", "BUG-021-006",
		"lookahead_days", timingCfg.LookaheadDays,
		"max_candidates", timingCfg.MaxCandidates,
		"confidence_floor", timingCfg.ConfidenceFloor)
}

// wireResurfaceEvaluator injects the LLM-driven resurfacing-worthiness
// evaluator into the intelligence engine for the dormancy strategy
// (BUG-021-007). Nil bridge ⇒ no-op (the dormancy strategy stays disabled;
// there is no hardcoded dormancy/relevance threshold fallback; serendipity is
// unaffected).
func wireResurfaceEvaluator(bridge *agent.Bridge, engine *intelligence.Engine) {
	if engine == nil {
		return
	}
	if bridge == nil {
		slog.Warn("resurface evaluator skipped: agent bridge is nil; dormancy resurfacing disabled",
			"spec", "021", "bug", "BUG-021-007")
		return
	}

	resurfaceCfg, err := config.LoadResurfaceConfig()
	if err != nil {
		slog.Error("resurface evaluator disabled: SST load failed",
			"spec", "021", "bug", "BUG-021-007", "error", err.Error())
		return
	}

	engine.SetResurfaceConfig(&intelligence.ResurfaceConfig{
		Evaluator:       &intelligence.BridgeResurfaceEvaluator{Runner: bridge},
		MinDormancyDays: resurfaceCfg.MinDormancyDays,
		MaxCandidates:   resurfaceCfg.MaxCandidates,
		ConfidenceFloor: resurfaceCfg.ConfidenceFloor,
	})
	slog.Info("resurface evaluator wired (LLM-driven)",
		"spec", "021", "bug", "BUG-021-007",
		"min_dormancy_days", resurfaceCfg.MinDormancyDays,
		"max_candidates", resurfaceCfg.MaxCandidates,
		"confidence_floor", resurfaceCfg.ConfidenceFloor)
}

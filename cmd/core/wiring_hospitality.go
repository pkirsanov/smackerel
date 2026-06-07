// Spec 021 BUG-021-010 — LLM-driven hospitality concern evaluator wiring.
//
// Constructs the BridgeHospitalityEvaluator from the live agent.Bridge and the
// SST-resolved digest.hospitality.* operational candidate caps, then injects it
// into the digest generator. This replaces the previous hardcoded guest/property
// alert thresholds (sentiment_score < 0.3, avg_rating < 3.5, issue_count >= 5):
// the "is this guest/property a concern?" judgment now flows through the
// hospitality_concern_evaluate scenario (docs/smackerel.md §3.6) on the reusable
// agent.InvokeJudgment foundation.
//
// Nil bridge ⇒ no-op: concern alerts stay disabled (there is NO hardcoded
// threshold fallback), which is correct when the bridge has not been wired yet
// (e.g. partial-boot integration tests).
package main

import (
	"log/slog"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/digest"
)

func wireHospitalityEvaluator(bridge *agent.Bridge, gen *digest.Generator) {
	if gen == nil {
		return
	}
	if bridge == nil {
		slog.Warn("hospitality evaluator skipped: agent bridge is nil; guest/property concern alerts disabled",
			"spec", "021", "bug", "BUG-021-010")
		return
	}

	hospitalityCfg, err := config.LoadHospitalityConfig()
	if err != nil {
		slog.Error("hospitality evaluator disabled: SST load failed",
			"spec", "021", "bug", "BUG-021-010", "error", err.Error())
		return
	}

	gen.SetHospitalityEvaluator(
		&digest.BridgeHospitalityEvaluator{Runner: bridge},
		digest.HospitalityBounds{
			GuestCandidateLimit:    hospitalityCfg.GuestCandidateLimit,
			PropertyCandidateLimit: hospitalityCfg.PropertyCandidateLimit,
		},
	)
	slog.Info("hospitality evaluator wired (LLM-driven)",
		"spec", "021", "bug", "BUG-021-010",
		"guest_candidate_limit", hospitalityCfg.GuestCandidateLimit,
		"property_candidate_limit", hospitalityCfg.PropertyCandidateLimit)
}

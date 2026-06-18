// Spec 095 SCOPE-07 / PKT-095-B — late-bind the production evergreen judge.
//
// The evergreen Scorer is built in buildCoreServices (from the fail-loud SST
// evergreen config) and already injected into the live ingestion publisher
// BEFORE connectors start. This wiring runs AFTER the agent bridge is
// constructed and swaps the production scenario judge into the Scorer race-free
// (Scorer.SetJudge uses an atomic pointer; connector goroutines read it
// atomically). It mirrors the spec 021 cooling/alert-timing/resurface wiring:
// nil bridge ⇒ no-op (ingestion keeps using the deterministic TierSignals
// fallback — there is NO hardcoded business cutoff; NFR-2, Principle 9).
package main

import (
	"log/slog"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
)

// wireEvergreenScorer late-binds the production agent-bridge EvergreenJudge into
// the already-constructed evergreen Scorer. It is a no-op when:
//   - the scorer is nil (retrieval.evergreen.enabled=false), or
//   - judgment_source is not "scenario" (the deterministic tier_signals source
//     was selected by SST — no LLM judge is wired), or
//   - the agent bridge is nil (partial-boot/integration; the deterministic
//     fallback keeps ingestion working).
//
// Only when judgment_source=scenario AND the bridge exists does the live
// retrieval_evergreen scenario judgment go live at the ingestion front door.
func wireEvergreenScorer(bridge *agent.Bridge, scorer *evergreen.Scorer, judgmentSource string) {
	if scorer == nil {
		return
	}
	if judgmentSource != evergreen.JudgmentSourceScenario {
		slog.Info("evergreen scorer: deterministic tier_signals source selected by SST; no scenario judge wired",
			"spec", "095", "judgment_source", judgmentSource)
		return
	}
	if bridge == nil {
		slog.Warn("evergreen scenario judge skipped: agent bridge is nil; ingestion uses the deterministic tier_signals fallback",
			"spec", "095")
		return
	}
	scorer.SetJudge(&evergreen.BridgeEvergreenJudge{Runner: bridge})
	slog.Info("evergreen scenario judge wired (LLM-driven)",
		"spec", "095", "scenario", evergreen.EvergreenScenarioID)
}

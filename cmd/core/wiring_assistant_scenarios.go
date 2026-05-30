// Spec 061 SCOPE-03 — runtime wiring for assistant scenario validation
// (design §7.2 rule #6).
//
// This file is a thin glue between cmd/core's startup sequence and
// `assistant.ValidateScenariosPresent`. It runs ONLY when the
// assistant is SST-enabled (`cfg.Assistant.Enabled`). When disabled
// the rule is skipped because the manifest is part of a capability
// layer that does not run.
//
// The rule executes AFTER `wireAgentBridge` returns successfully so
// the agent registry has already been populated by the tool init()
// side-effects (blank imports in wiring_agent.go). Running it earlier
// would yield false rejections from the loader's
// "allowed_tool not registered" path.
package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/config"
)

// assistantManifestRelPath is the well-known location of the spec 061
// sibling skills manifest, relative to the same parent that hosts the
// agent scenario directory. The convention is fixed by
// SUBSTRATE-TOUCHPOINT-1 Option (b) (see docs/smackerel.md "Assistant
// > Scenarios") so it is not exposed through SST — moving it would
// silently break the sibling-lookup contract.
const assistantManifestRelPath = "assistant/scenarios.yaml"

// validateAssistantScenariosPresent runs spec 061 design §7.2 rule #6
// when the assistant capability is enabled. Returns nil (and logs a
// single info line) when the rule is satisfied; returns a wrapped
// error when ANY manifest-declared scenario id does not have a
// loadable YAML. Returns nil silently when the assistant is disabled.
//
// `agentScenarioDir` is the directory the bridge loader scanned; the
// manifest path is derived from its parent so the two stay aligned.
func validateAssistantScenariosPresent(cfg *config.Config, agentScenarioDir string) error {
	if cfg == nil {
		return fmt.Errorf("validateAssistantScenariosPresent: nil config")
	}
	if !cfg.Assistant.Enabled {
		return nil
	}
	if agentScenarioDir == "" {
		// Agent has no scenario directory (BS-001 empty-scenarios
		// path). The assistant cannot have user-facing scenarios if
		// nothing is loaded; fail loud so operators see the rule
		// rather than a silent no-op.
		return fmt.Errorf("[F061-SCENARIO-MISSING] assistant.enabled=true but agent.scenario_dir is empty")
	}

	manifestPath := filepath.Join(filepath.Dir(agentScenarioDir), assistantManifestRelPath)
	resolver := assistantEnableResolver(cfg)

	if err := assistant.ValidateScenariosPresent(manifestPath, agentScenarioDir, resolver); err != nil {
		return err
	}
	slog.Info("assistant scenarios present",
		"manifest", manifestPath,
		"scenario_dir", agentScenarioDir,
	)
	return nil
}

// assistantEnableResolver returns the EnableResolver used by the
// spec 061 skills manifest. Each `enable_sst_key` declared in the
// sibling YAML resolves to one of the cfg.Assistant per-skill
// *Enabled bool fields. Unknown keys return found=false so the
// manifest loader (which fails loud on unknown keys per BS-008) can
// surface the typo.
func assistantEnableResolver(cfg *config.Config) assistant.EnableResolver {
	return func(key string) (enabled bool, found bool) {
		switch key {
		case "assistant.skills.retrieval.enabled":
			return cfg.Assistant.RetrievalEnabled, true
		case "assistant.skills.weather.enabled":
			return cfg.Assistant.WeatherEnabled, true
		case "assistant.skills.notifications.enabled":
			return cfg.Assistant.NotificationsEnabled, true
		case "assistant.skills.recipe_search.enabled":
			return cfg.Assistant.RecipeSearchEnabled, true
		default:
			return false, false
		}
	}
}

// agentScenarioDir reads AGENT_SCENARIO_DIR via the same load path the
// bridge uses, returning the absolute scenario directory. Kept here
// (not in wiring_agent.go) so SCOPE-03's wiring file owns every
// touch-point it introduces.
func agentScenarioDir() (string, error) {
	agentCfg, err := agent.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("agent config: %w", err)
	}
	return agentCfg.ScenarioDir, nil
}

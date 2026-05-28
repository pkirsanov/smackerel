// Validation rule #6 — "scenario YAMLs present" per spec 061 design
// §7.2. Implemented for SCOPE-03 (BS-008-adjacent). The rule asserts
// that every scenario id declared in the user-facing skills manifest
// at `manifestPath` MUST resolve to a scenario file in `scenarioDir`
// that the Spec 037 loader (`agent.DefaultLoader().Load`) accepts.
//
// The rule runs at startup AFTER tool init() side-effects have
// populated the registry; otherwise the loader rejects new scenarios
// with "allowed_tool not registered" and the rule's error message
// would be misleading. Wiring point: cmd/core right after
// `wireAgentBridge` returns successfully.
//
// On failure the returned error is prefixed with `[F061-SCENARIO-
// MISSING]` so operators can pattern-match it in logs and shell tests.

package assistant

import (
	"errors"
	"fmt"
	"sort"

	"github.com/smackerel/smackerel/internal/agent"
)

// ValidateScenariosPresent is the SCOPE-03 implementation of design
// §7.2 rule #6. It loads the sibling manifest at `manifestPath`, runs
// the Spec 037 loader against `scenarioDir`, and returns an error
// listing every manifest-declared scenario id whose YAML failed to
// load (or is missing entirely).
//
// `resolve` MUST satisfy the same contract as for `LoadSkillsManifest`
// — every declared enable_sst_key must resolve. In production this is
// the SST snapshot lookup; tests can pass a constant resolver.
//
// Returns nil if every manifest id has a successfully loaded scenario.
// Returns a non-nil `[F061-SCENARIO-MISSING] …` error otherwise; the
// error message names every missing id and surfaces the loader's
// rejection reason when available.
func ValidateScenariosPresent(manifestPath, scenarioDir string, resolve EnableResolver) error {
	if manifestPath == "" {
		return errors.New("[F061-SCENARIO-MISSING] manifest path is empty")
	}
	if scenarioDir == "" {
		return errors.New("[F061-SCENARIO-MISSING] scenario dir is empty")
	}

	manifest, err := LoadSkillsManifest(manifestPath, resolve)
	if err != nil {
		// Manifest itself is broken — that is its own failure surface
		// (the SCOPE-01 SST validator), but we wrap with the rule-#6
		// prefix so operators see a single signal at startup.
		return fmt.Errorf("[F061-SCENARIO-MISSING] cannot load manifest %s: %w", manifestPath, err)
	}

	registered, rejected, fatal := agent.DefaultLoader().Load(scenarioDir, "")
	if fatal != nil {
		return fmt.Errorf("[F061-SCENARIO-MISSING] loader fatal on %s: %w", scenarioDir, fatal)
	}
	loadedByID := make(map[string]bool, len(registered))
	for _, s := range registered {
		loadedByID[s.ID] = true
	}
	rejectionByPath := make(map[string]string, len(rejected))
	for _, r := range rejected {
		rejectionByPath[r.Path] = r.Message
	}

	manifestIDs := manifest.AllScenarioIDs()
	sort.Strings(manifestIDs)

	var missing []string
	for _, id := range manifestIDs {
		if !loadedByID[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		// Surface loader rejections (when present) so the operator can
		// distinguish "file is absent" from "file is present but
		// rejected by the loader". The first reason wins per id.
		details := ""
		if len(rejected) > 0 {
			details = " loader rejections:"
			rPaths := make([]string, 0, len(rejected))
			for p := range rejectionByPath {
				rPaths = append(rPaths, p)
			}
			sort.Strings(rPaths)
			for _, p := range rPaths {
				details += fmt.Sprintf(" %s=%q;", p, rejectionByPath[p])
			}
		}
		return fmt.Errorf("[F061-SCENARIO-MISSING] manifest %s references scenario ids that did not load from %s: %v.%s",
			manifestPath, scenarioDir, missing, details)
	}
	return nil
}

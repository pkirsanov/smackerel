// Functional test for SCOPE-03 DoD anchor `scope-03-manifest-reader`:
// when the production tool packages are imported (so init() runs and
// retrieval_search / weather_lookup / notification_propose /
// notification_execute are registered), the Spec 037 loader MUST load
// every YAML in `config/prompt_contracts/`, and every scenario id
// declared in the sibling `config/assistant/scenarios.yaml` manifest
// MUST map 1:1 to a successfully loaded scenario.
//
// This is the wiring proof that SUBSTRATE-TOUCHPOINT-1 Option (b)
// works end-to-end: the spec 061 user-facing metadata file is a real
// sibling of the spec 037 scenario directory, and the scenario ids in
// the manifest are not vaporware — they correspond to YAML files the
// runtime can parse, schema-compile, and execute against the registry.
package assistant

import (
	"sort"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	// Blank imports so init() registers the tool handlers BEFORE the
	// loader runs. Without these, the loader rejects each new scenario
	// with "allowed_tool not registered" and the assertion below fails.
	_ "github.com/smackerel/smackerel/internal/agent/tools/notification"
	_ "github.com/smackerel/smackerel/internal/agent/tools/recipesearch"
	_ "github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	_ "github.com/smackerel/smackerel/internal/agent/tools/weather"
)

func TestSkillsManifest_AllScenariosLoadFromPromptContractsDir(t *testing.T) {
	scenarioDir := repoFile(t, "config", "prompt_contracts")
	manifestPath := repoFile(t, "config", "assistant", "scenarios.yaml")

	// Step 1: load the manifest with a resolver that accepts every
	// declared enable_sst_key (test fixture; production resolves
	// against the SST snapshot).
	manifest, err := LoadSkillsManifest(manifestPath, func(string) (bool, bool) {
		return true, true
	})
	if err != nil {
		t.Fatalf("LoadSkillsManifest(%s): %v", manifestPath, err)
	}
	manifestIDs := manifest.AllScenarioIDs()
	sort.Strings(manifestIDs)
	if len(manifestIDs) == 0 {
		t.Fatalf("manifest %s declared zero scenarios", manifestPath)
	}

	// Step 2: load the live prompt-contracts directory through the
	// real Spec 037 loader. The tool init()s above must have populated
	// the registry; otherwise the loader will reject the new YAMLs.
	registered, rejected, fatal := agent.DefaultLoader().Load(scenarioDir, "")
	if fatal != nil {
		t.Fatalf("loader fatal: %v", fatal)
	}
	loadedByID := make(map[string]bool, len(registered))
	for _, s := range registered {
		loadedByID[s.ID] = true
	}

	// Step 3: every manifest-declared scenario id MUST have a loaded
	// counterpart. This is the SUBSTRATE-TOUCHPOINT-1 contract.
	var missing []string
	for _, id := range manifestIDs {
		if !loadedByID[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		// Surface the loader's rejection messages so the failure tells
		// the operator exactly which file failed and why.
		t.Logf("loader rejected %d files:", len(rejected))
		for _, r := range rejected {
			t.Logf("  REJECT %s — %s", r.Path, r.Message)
		}
		t.Logf("loader registered %d scenarios:", len(registered))
		for _, s := range registered {
			t.Logf("  OK     %s @ %s", s.ID, s.SourcePath)
		}
		t.Fatalf("manifest references scenario ids that did NOT load: %v", missing)
	}
}

func TestSkillsManifest_EnabledIDsHaveLoadedScenarios(t *testing.T) {
	scenarioDir := repoFile(t, "config", "prompt_contracts")
	manifestPath := repoFile(t, "config", "assistant", "scenarios.yaml")

	manifest, err := LoadSkillsManifest(manifestPath, func(string) (bool, bool) {
		return true, true
	})
	if err != nil {
		t.Fatalf("LoadSkillsManifest: %v", err)
	}

	registered, _, fatal := agent.DefaultLoader().Load(scenarioDir, "")
	if fatal != nil {
		t.Fatalf("loader fatal: %v", fatal)
	}
	loaded := make(map[string]bool, len(registered))
	for _, s := range registered {
		loaded[s.ID] = true
	}

	enabled := manifest.EnabledScenarioIDs()
	if len(enabled) == 0 {
		t.Fatal("expected at least one enabled scenario id; got none")
	}
	for _, id := range enabled {
		if !loaded[id] {
			t.Errorf("enabled scenario id %q has no matching loaded scenario", id)
		}
	}
}

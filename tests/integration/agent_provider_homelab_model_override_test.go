//go:build integration

// Home-lab single-source model selection — SST contract proving the home-lab
// bundle does NOT hardcode any operator/hardware-specific model.
//
// As of the single-source change, home-lab's real model selection AND the
// Ollama memory envelope are owned by the deploy adapter's per-target
// params.yaml `model_selection:` block; the adapter's apply.sh injects them
// into the bundle's app.env at apply time (appended last, last-occurrence-wins
// — the HOST_BIND_ADDRESS injection pattern). The product repo therefore drops
// every per-env home-lab model override from config/smackerel.yaml's
// environments.home-lab block, so the home-lab bundle inherits the SAME generic
// commodity base that dev resolves.
//
// This test enforces that contract at the SST -> env-file boundary: it
// generates BOTH the home-lab and dev env files from the live SST into isolated
// temp dirs, then asserts that for EACH of the 17 adapter-owned model env vars
// the home-lab value EQUALS the dev value. Equality proves home-lab carries no
// home-lab-specific override; any divergence proves a re-hardcoded model.
//
// The 17 fields (the exact set the deploy adapter now injects):
//
//	OLLAMA_MEMORY_LIMIT
//	LLM_MODEL, OLLAMA_MODEL, OLLAMA_VISION_MODEL
//	AGENT_PROVIDER_DEFAULT_MODEL, AGENT_PROVIDER_FAST_MODEL,
//	AGENT_PROVIDER_VISION_MODEL, AGENT_PROVIDER_REASONING_MODEL,
//	AGENT_PROVIDER_OCR_MODEL
//	ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID,
//	ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID,
//	ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS,
//	ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS
//	PHOTOS_INTELLIGENCE_CLASSIFY_MODEL,
//	PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL,
//	PHOTOS_INTELLIGENCE_AESTHETIC_MODEL, PHOTOS_INTELLIGENCE_OCR_MODEL
//
// Why SMACKEREL_HARDWARE_TIER=accel: dev pins its four interactive model slots
// (agent_provider_default_model, agent_provider_fast_model, llm_model,
// ollama_model) to gemma3:4b via environments.dev.*. Home-lab, having dropped
// all overrides, takes those four from the tier matrix
// (models.tiers.<tier>.interactive.model). Only the accel tier's interactive
// model is gemma3:4b, so accel makes home-lab's tier-resolved interactive
// models match dev's explicit gemma3:4b — isolating the variable under test
// (home-lab's per-env overrides) from the orthogonal hardware-tier dimension.
// At the cpu tier the tier model is qwen2.5:0.5b-instruct, which would differ
// from dev's gemma3:4b for a NON-violation reason (dev's own cpu override), so
// cpu is the wrong tier for this equality contract.
//
// Adversarial properties (every assertion is unconditional after the env-file
// read — no bailout-style early return):
//
//   - If someone re-adds ANY home-lab per-env model override to
//     config/smackerel.yaml (e.g.
//     environments.home-lab.agent_provider_default_model: gpt-oss:20b, or
//     .ollama_memory_limit: "48G"), the home-lab env emits the operator value
//     while dev keeps the commodity base, so homeLab[k] != dev[k] and the
//     matching assertion FAILs naming the re-hardcoded key + both values.
//
//   - The operator-model-absence backstop additionally FAILs if a home-lab
//     model field ever contains gpt-oss:20b / gemma4:26b / 48G, catching the
//     edge case where BOTH envs were changed to an operator model (which the
//     equality check alone would miss).
//
// Generation harness mirrors scripts/commands/config_home_lab_runtime_env_test.sh:
// it supplies a non-dev-default POSTGRES_PASSWORD so the production-class
// home-lab generate clears the spec 051 FR-051-005 dev-default-password guard
// (config.sh resolution path 1). config-validate is skipped for
// production-class targets (placeholder mode), so home-lab generation succeeds
// shipping the commodity base.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// homeLabModelEnvKeys is the exact set of model env vars the deploy adapter
// now owns and injects for home-lab. The home-lab bundle MUST resolve each to
// the same generic commodity value dev resolves (no home-lab-specific override
// remains in config/smackerel.yaml).
var homeLabModelEnvKeys = []string{
	"OLLAMA_MEMORY_LIMIT",
	"LLM_MODEL",
	"OLLAMA_MODEL",
	"OLLAMA_VISION_MODEL",
	"AGENT_PROVIDER_DEFAULT_MODEL",
	"AGENT_PROVIDER_FAST_MODEL",
	"AGENT_PROVIDER_VISION_MODEL",
	"AGENT_PROVIDER_REASONING_MODEL",
	"AGENT_PROVIDER_OCR_MODEL",
	"ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID",
	"ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID",
	"ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS",
	"ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS",
	"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL",
	"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL",
	"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL",
	"PHOTOS_INTELLIGENCE_OCR_MODEL",
}

// repoRootForHomeLabNoHardcode climbs from CWD looking for
// config/smackerel.yaml. Independent of the `go test` working dir.
func repoRootForHomeLabNoHardcode(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i = i + 1 {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}

// generateEnvForHomeLabNoHardcode invokes scripts/commands/config.sh for a
// single target env into the supplied isolated output dir and returns the
// parsed KEY->VALUE map. SMACKEREL_HARDWARE_TIER=accel is pinned (see the file
// header) so both envs resolve the tier-matrix interactive model to the same
// gemma3:4b commodity value, making them apples-to-apples for every model
// field. A non-dev-default POSTGRES_PASSWORD override is supplied because plain
// `config.sh --env home-lab` is production-class and rejects the dev-default
// Postgres password (spec 051 FR-051-005); the env override (resolution path 1)
// clears that generator-side guard. It is a non-secret test literal, does not
// affect the model fields asserted here, and dev (non-production-class) is
// unaffected by it.
func generateEnvForHomeLabNoHardcode(t *testing.T, root, targetEnv, outDir string) map[string]string {
	t.Helper()
	scriptPath := filepath.Join(root, "scripts", "commands", "config.sh")
	cmd := exec.Command("bash", scriptPath, "--env", targetEnv)
	cmd.Env = append(os.Environ(),
		"SMACKEREL_GENERATED_DIR="+outDir,
		"SMACKEREL_HARDWARE_TIER=accel",
		// Production-class home-lab generation rejects the dev-default Postgres
		// password (spec 051 FR-051-005). config.sh honours a POSTGRES_PASSWORD
		// env override (resolution path 1); supply a clearly-non-default test
		// value so generation proceeds. NOT a real secret — only unblocks the
		// generator-side dev-default guard; the model fields (the assertion
		// target) are unaffected. dev is non-production-class and ignores it.
		"POSTGRES_PASSWORD=homelab-no-hardcode-integration-test-not-the-dev-default",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config.sh --env %s failed: %v\n--- output ---\n%s\n--- end ---", targetEnv, err, string(out))
	}
	envPath := filepath.Join(outDir, targetEnv+".env")
	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read generated %s at %s: %v\n--- generator output ---\n%s", targetEnv+".env", envPath, err, string(out))
	}
	return homeLabParseEnvFile(string(envBytes))
}

// homeLabParseEnvFile parses KEY=VALUE lines (one per line) into a map. Blank
// lines and comment lines (leading '#') are skipped; only the first '=' splits
// the pair so values containing '=' survive intact.
func homeLabParseEnvFile(text string) map[string]string {
	m := make(map[string]string)
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimRight(raw, "\r")
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		eq := strings.Index(ln, "=")
		if eq < 0 {
			continue
		}
		m[ln[:eq]] = ln[eq+1:]
	}
	return m
}

// TestHomeLabDoesNotHardcodeModels asserts the single-source contract: the
// home-lab bundle carries NO operator/hardware-specific model override, so for
// every one of the 17 adapter-owned model env vars the resolved home-lab value
// equals the resolved dev (commodity-base) value. See the file header for the
// full rationale and adversarial properties.
func TestHomeLabDoesNotHardcodeModels(t *testing.T) {
	root := repoRootForHomeLabNoHardcode(t)

	homeLab := generateEnvForHomeLabNoHardcode(t, root, "home-lab", t.TempDir())
	dev := generateEnvForHomeLabNoHardcode(t, root, "dev", t.TempDir())

	// Primary contract: home-lab == dev for every adapter-owned model field.
	// A mismatch means a home-lab-specific override was re-hardcoded into
	// config/smackerel.yaml (the regression this gate forbids).
	for _, k := range homeLabModelEnvKeys {
		hlVal, hlOK := homeLab[k]
		devVal, devOK := dev[k]
		if !hlOK {
			t.Errorf("home-lab.env is missing model key %q (every adapter-owned model field MUST still be emitted from the commodity base, fail-loud)", k)
			continue
		}
		if !devOK {
			t.Errorf("dev.env is missing model key %q (comparison baseline incomplete)", k)
			continue
		}
		if hlVal != devVal {
			t.Errorf("home-lab re-hardcodes model %q: home-lab=%q but dev (commodity base)=%q — home-lab MUST inherit the commodity base; operator/hardware-specific model selection is owned by the deploy adapter's params.yaml model_selection block and injected at apply, NOT pinned in config/smackerel.yaml", k, hlVal, devVal)
		}
	}

	// Concrete commodity-base proof (== dev). The accel tier resolves the
	// interactive slots to gemma3:4b; the synthesis id and the envelope are
	// tier-independent base values. These are exactly the values the deploy
	// adapter overrides at apply with the operator's real selection.
	wantCommodity := map[string]string{
		"AGENT_PROVIDER_DEFAULT_MODEL":                "gemma3:4b",
		"ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID": "gemma3:4b",
		"OLLAMA_MEMORY_LIMIT":                         "8G",
	}
	for k, want := range wantCommodity {
		if got := homeLab[k]; got != want {
			t.Errorf("home-lab commodity base %q = %q, want %q (the in-repo bundle MUST ship the generic commodity value; the operator value is adapter-injected at apply)", k, got, want)
		}
	}

	// Adversarial backstop: the in-repo home-lab bundle MUST NOT carry the
	// operator-specific values. If any model field contains one, a per-env
	// override was re-hardcoded into config/smackerel.yaml (this also catches
	// the case where BOTH envs were changed, which the equality loop misses).
	for _, k := range homeLabModelEnvKeys {
		v := homeLab[k]
		for _, operatorValue := range []string{"gpt-oss:20b", "gemma4:26b", "48G"} {
			if strings.Contains(v, operatorValue) {
				t.Errorf("home-lab.env %q=%q carries operator-specific value %q — the in-repo bundle MUST ship the commodity base; gpt-oss:20b / gemma4:26b / 48G are adapter-injected at apply only", k, v, operatorValue)
			}
		}
	}

	t.Logf("home-lab emits the commodity base (== dev) for all %d adapter-owned model fields (e.g. AGENT_PROVIDER_DEFAULT_MODEL=%q, ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID=%q, OLLAMA_MEMORY_LIMIT=%q); operator selection (gpt-oss:20b / gemma4:26b / 48G) is injected by the deploy adapter at apply, not pinned in config/smackerel.yaml",
		len(homeLabModelEnvKeys), homeLab["AGENT_PROVIDER_DEFAULT_MODEL"], homeLab["ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID"], homeLab["OLLAMA_MEMORY_LIMIT"])
}

//go:build integration

// Home-lab unpulled-model retirement — SST contract for the per-env home-lab
// override of the env-INDEPENDENT model specialists that would otherwise
// resolve to a model the home-lab Ollama host does NOT pull. Two groups:
//
//   - Residual-deepseek trio: AGENT_PROVIDER_REASONING_MODEL,
//     AGENT_PROVIDER_OCR_MODEL, PHOTOS_INTELLIGENCE_OCR_MODEL.
//   - Photos vision trio: PHOTOS_INTELLIGENCE_CLASSIFY_MODEL,
//     PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL,
//     PHOTOS_INTELLIGENCE_AESTHETIC_MODEL.
//
// Background: all six fields were emitted from the commodity base via
// env-INDEPENDENT required_value calls:
//
//	reasoning   = agent.provider_routing.reasoning.model = deepseek-r1:7b
//	ocr         = agent.provider_routing.ocr.model       = deepseek-ocr:3b
//	photos ocr  = photos.intelligence.ocr_model          = deepseek-ocr:3b
//	classify    = photos.intelligence.classify_model     = gemma3:4b
//	sensitivity = photos.intelligence.sensitivity_model  = gemma3:4b
//	aesthetic   = photos.intelligence.aesthetic_model    = gemma3:4b
//
// so the resolved home-lab env still carried deepseek-* / gemma3:4b names
// even though the operator's home-lab Ollama host only pulls gpt-oss:20b +
// gemma4:26b. scripts/commands/config.sh now resolves all six through
// env_override_value, and config/smackerel.yaml's environments.home-lab block
// pins them to the pulled set (reasoning -> gpt-oss:20b; ocr / photos-ocr /
// classify / sensitivity / aesthetic -> gemma4:26b) while dev keeps the
// commodity base values. The photos EMBED model is deliberately excluded: it
// is ml-sidecar-routed (sentence-transformers in smackerel_ml, not Ollama),
// so it never points at an unpulled Ollama model.
//
// This test enforces both halves of the override at the SST -> env-file
// boundary by generating BOTH envs from the live SST into an isolated temp
// dir (pinned SMACKEREL_HARDWARE_TIER so it is hermetic w.r.t. the ambient
// shell), then asserting:
//
//  1. home-lab.env pins all six specialists to the pulled set (reasoning =
//     gpt-oss:20b; ocr / photos-ocr / classify / sensitivity / aesthetic =
//     gemma4:26b — only models the host actually pulls, zero deepseek).
//  2. dev.env keeps the commodity base (reasoning = deepseek-r1:7b; ocr /
//     photos-ocr = deepseek-ocr:3b; classify / sensitivity / aesthetic =
//     gemma3:4b) — the override is home-lab-scoped by design.
//
// Adversarial properties (every assertion is unconditional after the
// env-file read — no bailout-style early return):
//
//   - If config.sh reverts any of the six resolutions to the
//     env-INDEPENDENT required_value form, the home-lab env reverts to the
//     commodity value (deepseek-* or gemma3:4b) and the matching home-lab
//     assertion FAILs naming the regressed key.
//
//   - If the commodity base is silently strengthened (e.g. someone edits
//     agent.provider_routing.reasoning.model itself to gpt-oss:20b "to save
//     a config line"), the dev env shows the stronger model and the matching
//     dev assertion FAILs naming the broken commodity binding.
//
// Pattern mirrored from
// tests/integration/agent_provider_default_test_override_test.go (the
// SST -> env-file boundary precedent) and the home-lab generation harness in
// scripts/commands/config_home_lab_runtime_env_test.sh (production-class
// generation needs no real secret — POSTGRES_PASSWORD resolves to a
// placeholder marker for home-lab).
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootForAgentProviderHomeLabOverride climbs from CWD looking for
// config/smackerel.yaml. Independent of the `go test` working dir.
func repoRootForAgentProviderHomeLabOverride(t *testing.T) string {
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

// generateEnvForHomeLabOverrideTest invokes scripts/commands/config.sh for a
// single target env into the supplied isolated output dir and returns the
// resolved env-file text. SMACKEREL_HARDWARE_TIER is pinned so resolution is
// hermetic w.r.t. the ambient shell (config.sh requires the tier and is
// normally fed it by the smackerel.sh wrapper, which this direct exec
// bypasses). No POSTGRES_PASSWORD is set: for the production-class home-lab
// target config.sh emits a placeholder marker rather than requiring a real
// secret, so the generation needs no credential material.
func generateEnvForHomeLabOverrideTest(t *testing.T, root, targetEnv, outDir string) string {
	t.Helper()
	scriptPath := filepath.Join(root, "scripts", "commands", "config.sh")
	cmd := exec.Command("bash", scriptPath, "--env", targetEnv)
	cmd.Env = append(os.Environ(),
		"SMACKEREL_GENERATED_DIR="+outDir,
		"SMACKEREL_HARDWARE_TIER=cpu",
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
	return string(envBytes)
}

// TestAgentProviderHomeLabModelOverride asserts the home-lab unpulled-model
// retirement at the SST -> env-file boundary for the six previously
// env-independent specialists. See the file header for the full rationale
// and adversarial properties.
func TestAgentProviderHomeLabModelOverride(t *testing.T) {
	root := repoRootForAgentProviderHomeLabOverride(t)

	homeLabDir := t.TempDir()
	devDir := t.TempDir()

	homeLabEnvText := generateEnvForHomeLabOverrideTest(t, root, "home-lab", homeLabDir)
	devEnvText := generateEnvForHomeLabOverrideTest(t, root, "dev", devDir)

	// Home-lab MUST pin all six specialists to the pulled set (gpt-oss:20b
	// reasoning; gemma4:26b OCR + photos OCR + photos classify/sensitivity/
	// aesthetic). Any deepseek-* or gemma3:4b value here means config.sh
	// reverted a resolution to the env-independent required_value form.
	homeLabWant := []struct{ key, line string }{
		{"AGENT_PROVIDER_REASONING_MODEL", "AGENT_PROVIDER_REASONING_MODEL=gpt-oss:20b"},
		{"AGENT_PROVIDER_OCR_MODEL", "AGENT_PROVIDER_OCR_MODEL=gemma4:26b"},
		{"PHOTOS_INTELLIGENCE_OCR_MODEL", "PHOTOS_INTELLIGENCE_OCR_MODEL=gemma4:26b"},
		{"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "PHOTOS_INTELLIGENCE_CLASSIFY_MODEL=gemma4:26b"},
		{"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", "PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL=gemma4:26b"},
		{"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "PHOTOS_INTELLIGENCE_AESTHETIC_MODEL=gemma4:26b"},
	}
	for _, w := range homeLabWant {
		if !envFileContainsExactLine(homeLabEnvText, w.line) {
			t.Errorf("home-lab.env must contain %q (residual-deepseek retirement via environments.home-lab per-env override); got line: %q",
				w.line, findEnvKeyLine(homeLabEnvText, w.key))
		}
	}

	// Dev MUST keep the commodity base — the override is home-lab-scoped. For
	// reasoning/ocr/photos-ocr that base is deepseek-*; for the photos vision
	// trio (classify/sensitivity/aesthetic) it is gemma3:4b. A gpt-oss /
	// gemma4:26b value here means the commodity base itself was silently
	// strengthened (or the override leaked into dev).
	devWant := []struct{ key, line string }{
		{"AGENT_PROVIDER_REASONING_MODEL", "AGENT_PROVIDER_REASONING_MODEL=deepseek-r1:7b"},
		{"AGENT_PROVIDER_OCR_MODEL", "AGENT_PROVIDER_OCR_MODEL=deepseek-ocr:3b"},
		{"PHOTOS_INTELLIGENCE_OCR_MODEL", "PHOTOS_INTELLIGENCE_OCR_MODEL=deepseek-ocr:3b"},
		{"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "PHOTOS_INTELLIGENCE_CLASSIFY_MODEL=gemma3:4b"},
		{"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", "PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL=gemma3:4b"},
		{"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "PHOTOS_INTELLIGENCE_AESTHETIC_MODEL=gemma3:4b"},
	}
	for _, w := range devWant {
		if !envFileContainsExactLine(devEnvText, w.line) {
			t.Errorf("dev.env must contain %q (commodity base preserved — override is home-lab-only); got line: %q",
				w.line, findEnvKeyLine(devEnvText, w.key))
		}
	}

	t.Logf("home-lab pins reasoning=gpt-oss:20b ocr=gemma4:26b photos-ocr/classify/sensitivity/aesthetic=gemma4:26b; dev keeps reasoning=deepseek-r1:7b ocr=deepseek-ocr:3b photos-ocr=deepseek-ocr:3b classify/sensitivity/aesthetic=gemma3:4b (per-env override working, zero unpulled-model selections on home-lab)")
}

// envFileContainsExactLine reports whether `text` has a line equal to `want`
// (env files are KEY=VALUE, one per line). Exact-line match avoids a false
// positive from a longer key that has `want` as a prefix.
func envFileContainsExactLine(text, want string) bool {
	for _, ln := range strings.Split(text, "\n") {
		if strings.TrimRight(ln, "\r") == want {
			return true
		}
	}
	return false
}

// findEnvKeyLine returns the first line in `text` whose key is `key`, or the
// literal string `<not found>`. Diagnostic helper for the error messages.
func findEnvKeyLine(text, key string) string {
	prefix := key + "="
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(ln, prefix) {
			return strings.TrimRight(ln, "\r")
		}
	}
	return "<not found>"
}

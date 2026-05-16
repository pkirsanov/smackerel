//go:build integration

// Spec 045 BUG-045-001 Scope 2 — AC-5(c) integration test.
//
// SCN-045-001-C: When the operator runs config generate against an env
// file in which a model's profile memory exceeds the configured ollama
// envelope, the config-validate binary MUST reject the env file at
// pre-emit time with exit code 1 and stderr that names:
//
//   - the envelope key (`OLLAMA_MEMORY_LIMIT`),
//   - the offending model name (`bug-045-fixture-llm-20gib`),
//   - the required memory value (`20480`).
//
// This integration test exercises the BUILT binary in a subprocess
// (build + run + exit-code surface), complementing the in-process
// unit tests under cmd/config-validate/ which test the run() entry
// point directly. It uses the live config/generated/test.env as the
// base shape and overrides the relevant model/profile fields with
// synthetic fixture models (so the test is stable across pre-fix
// Scope 1 state and post-fix Scope 3 YAML).
//
// References:
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-001-ml-envelope-cross-service-routing/spec.md AC-5(c)
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-001-ml-envelope-cross-service-routing/scopes.md §2.C
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-001-ml-envelope-cross-service-routing/
//     scenario-manifest.json SCN-045-001-C
package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repoRootForConfigValidateContract climbs from CWD looking for
// config/smackerel.yaml. Mirrors the helper in
// ollama_config_contract_test.go (Spec 043) to avoid cross-test
// coupling.
func repoRootForConfigValidateContract(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
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

// buildOversizedEnvFile constructs a TEMP env file by taking the live
// config/generated/test.env and overriding every model-name field with
// synthetic fixture models, plus an ML_MODEL_MEMORY_PROFILES_JSON that
// declares bug-045-fixture-llm-20gib at 20480 MiB (> 8G envelope) for
// the LLM_MODEL/OLLAMA_MODEL slots, with the rest sized to fit.
//
// Returns the path to the constructed env file (under t.TempDir).
func buildOversizedEnvFile(t *testing.T, root string) string {
	t.Helper()
	livePath := filepath.Join(root, "config", "generated", "test.env")
	liveBytes, err := os.ReadFile(livePath)
	if err != nil {
		t.Skipf("live config/generated/test.env not present: %v (run './smackerel.sh config generate --env test' first)", err)
	}
	live := string(liveBytes)

	oversizedKeys := []string{"LLM_MODEL", "OLLAMA_MODEL"}
	fittingKeys := []string{
		"OLLAMA_VISION_MODEL", "OLLAMA_OCR_MODEL", "OLLAMA_REASONING_MODEL", "OLLAMA_FAST_MODEL",
		"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL",
		"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "PHOTOS_INTELLIGENCE_OCR_MODEL",
		"AGENT_PROVIDER_DEFAULT_MODEL", "AGENT_PROVIDER_REASONING_MODEL",
		"AGENT_PROVIDER_FAST_MODEL", "AGENT_PROVIDER_VISION_MODEL", "AGENT_PROVIDER_OCR_MODEL",
	}

	lines := strings.Split(live, "\n")
	for i, ln := range lines {
		for _, key := range oversizedKeys {
			if strings.HasPrefix(ln, key+"=") {
				lines[i] = key + "=\"bug-045-fixture-llm-20gib\""
			}
		}
		for _, key := range fittingKeys {
			if strings.HasPrefix(ln, key+"=") {
				lines[i] = key + "=\"bug-045-fixture-llm-6gib\""
			}
		}
		if strings.HasPrefix(ln, "ML_MODEL_MEMORY_PROFILES_JSON=") {
			lines[i] = `ML_MODEL_MEMORY_PROFILES_JSON='[{"model":"bug-045-fixture-llm-20gib","memory_mib":20480},{"model":"bug-045-fixture-llm-6gib","memory_mib":6144},{"model":"bug-045-fixture-embed-512mib","memory_mib":512},{"model":"nomic-embed-text","memory_mib":768}]'`
		}
		if strings.HasPrefix(ln, "PHOTOS_INTELLIGENCE_EMBED_MODEL=") {
			lines[i] = `PHOTOS_INTELLIGENCE_EMBED_MODEL="bug-045-fixture-embed-512mib"`
		}
	}

	tmp := filepath.Join(t.TempDir(), "oversized.env")
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatalf("write tmp env: %v", err)
	}
	return tmp
}

// TestConfigValidate_AC5c_BinaryRejectsOversizedModel is the AC-5(c)
// adversarial regression detector at the BINARY surface. It would
// FAIL on HEAD `de49b2f9` (pre-Scope-1) because the validator there
// conflated buckets and rejected with the wrong envelope name; it
// would FAIL on a hypothetical post-fix HEAD where the binary was
// removed; it PASSES only when both Scope 1 (per-service validator)
// AND Scope 2 (config-validate binary) are correctly in place.
func TestConfigValidate_AC5c_BinaryRejectsOversizedModel(t *testing.T) {
	root := repoRootForConfigValidateContract(t)
	envFile := buildOversizedEnvFile(t, root)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/config-validate", "--env-file="+envFile)
	cmd.Dir = root
	// Hermetic env: pass through only PATH + HOME + GOCACHE so `go run`
	// can compile the binary. The binary itself reads env vars only via
	// os.Environ AFTER loading the env file, so additional vars would
	// leak into Validate(). loadEnvFile uses os.Setenv on the keys it
	// parses, but os.Environ in the subprocess starts with the values
	// in cmd.Env below — so values NOT in our env file remain unset.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"GOCACHE=" + os.Getenv("GOCACHE"),
		"GOMODCACHE=" + os.Getenv("GOMODCACHE"),
		"GOFLAGS=" + os.Getenv("GOFLAGS"),
	}

	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("exec go run cmd/config-validate: %v output=%s", err, string(out))
		}
		exitCode = ee.ExitCode()
	}

	t.Logf("config-validate exit=%d (expected 1) output=%s", exitCode, strings.TrimSpace(string(out)))
	if exitCode != 1 {
		t.Fatalf("expected exit 1 (oversized model rejected); got %d. output=%s", exitCode, string(out))
	}
	for _, want := range []string{
		"OLLAMA_MEMORY_LIMIT",
		"bug-045-fixture-llm-20gib",
		"20480",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("config-validate output missing required substring %q (output=%s)", want, string(out))
		}
	}
}

// TestConfigValidate_AC5c_WrapperPropagatesRejection is the AC-5(c)
// end-to-end test through the `./smackerel.sh config generate` shell
// wrapper. It constructs a temp YAML by inlining a bad LLM model name
// + matching profile + lowered envelope, then runs config.sh with
// --config and --env. The wrapper MUST exit non-zero, stderr MUST name
// OLLAMA_MEMORY_LIMIT, and the wrapper MUST NOT leave a stale .tmp
// file behind.
//
// Uses --env test so the live dev.env is never at risk of being
// overwritten.
func TestConfigValidate_AC5c_WrapperPropagatesRejection(t *testing.T) {
	root := repoRootForConfigValidateContract(t)

	srcYAML := filepath.Join(root, "config", "smackerel.yaml")
	srcBytes, err := os.ReadFile(srcYAML)
	if err != nil {
		t.Fatalf("read source yaml: %v", err)
	}

	// Build a fixture YAML by overriding the llm.model and adding a
	// profile entry, then write to a temp file.
	src := string(srcBytes)

	// Locate the llm.model: line (sibling under `llm:` block). The
	// live yaml uses unquoted form: `  model: gemma4:26b` followed by
	// `  api_key:` on the next line.
	llmMarker := "  model: gemma4:26b\n  api_key:"
	if !strings.Contains(src, llmMarker) {
		t.Skipf("source yaml does not contain expected llm.model marker — yaml shape changed; rebase the fixture")
	}
	overridden := strings.Replace(src, llmMarker,
		"  model: bug-045-fixture-llm-20gib\n  api_key:", 1)
	if overridden == src {
		t.Fatalf("substitution of llm.model produced no change")
	}

	// Add the fixture profile entry inside model_memory_profiles. Use
	// the nomic-embed-text profile entry as the anchor — it's stable
	// across Scopes 1-3 (Scope 3 only changes ollama-routed defaults,
	// not the ml-sidecar-routed embedding model).
	profileAnchor := "    - model: \"nomic-embed-text\""
	if !strings.Contains(overridden, profileAnchor) {
		t.Skipf("source yaml does not contain expected profile anchor — yaml shape changed")
	}
	overridden = strings.Replace(overridden, profileAnchor,
		"    - model: \"bug-045-fixture-llm-20gib\"\n      memory_mib: 20480\n    - model: \"nomic-embed-text\"", 1)

	tmpYAML := filepath.Join(t.TempDir(), "smackerel.yaml")
	if err := os.WriteFile(tmpYAML, []byte(overridden), 0o600); err != nil {
		t.Fatalf("write tmp yaml: %v", err)
	}

	// Run config.sh against the temp YAML with --env test. The
	// wrapper writes to config/generated/test.env.tmp first; on
	// rejection it must remove the .tmp file before exiting.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash",
		filepath.Join(root, "scripts", "commands", "config.sh"),
		"--config", tmpYAML,
		"--env", "test",
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TARGET_ENV_GUARD=integration-045-001-bug")
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("exec config.sh: %v output=%s", err, string(out))
		}
		exitCode = ee.ExitCode()
	}

	t.Logf("config.sh exit=%d (expected non-zero) output=%s", exitCode, strings.TrimSpace(string(out)))
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit; got 0. output=%s", string(out))
	}
	if !strings.Contains(string(out), "OLLAMA_MEMORY_LIMIT") {
		t.Errorf("config.sh output should name OLLAMA_MEMORY_LIMIT; got: %s", string(out))
	}
	if !strings.Contains(string(out), "bug-045-fixture-llm-20gib") {
		t.Errorf("config.sh output should name bug-045-fixture-llm-20gib; got: %s", string(out))
	}

	// .tmp file must NOT remain after a rejection.
	tmpPath := filepath.Join(root, "config", "generated", "test.env.tmp")
	if _, err := os.Stat(tmpPath); err == nil {
		t.Errorf("config.sh left a stale .tmp file at %s after rejection; expected fail-loud cleanup", tmpPath)
		_ = os.Remove(tmpPath)
	}
}

// Spec 045 BUG-045-001 Scope 2 — unit tests for cmd/config-validate.
//
// These tests exercise the binary's contract WITHOUT spawning a
// subprocess: they call run() directly with crafted args + a stub
// stderr/stdout, and they construct the env-file fixtures from the
// live config/generated/test.env (which scripts/commands/config.sh
// already produces and which contains the full SST emission shape
// the parser is built for).
//
// Three cases:
//
//   - TestRun_MissingFlag_ExitsTwo — usage-error path.
//   - TestRun_NonexistentEnvFile_ExitsTwo — unreadable-file path.
//   - TestRun_ValidLiveTestEnv_ExitsZero — happy path against the
//     live generated test.env (proves the binary accepts a real,
//     well-formed env file when present).
//
// Per-test env-var isolation is done via t.Setenv (Go's testing
// helper auto-restores at test end), so these tests are safe to
// run in parallel within the package.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot climbs from CWD looking for config/smackerel.yaml. The
// binary's tests run from cmd/config-validate/, so the climb is two
// levels.
func repoRoot(t *testing.T) string {
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

func TestRun_MissingFlag_ExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 (usage), got %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Errorf("stderr should contain 'usage', got %q", stderr.String())
	}
}

func TestRun_NonexistentEnvFile_ExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--env-file=/nonexistent/path/does-not-exist.env"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 (file unreadable), got %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "ERROR") {
		t.Errorf("stderr should contain 'ERROR', got %q", stderr.String())
	}
}

// TestRun_MalformedEnvFile_ExitsTwo proves the parser rejects a line
// that does not match KEY=VALUE shape.
func TestRun_MalformedEnvFile_ExitsTwo(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bad.env")
	if err := os.WriteFile(tmp, []byte("this is not a valid env line\n"), 0o600); err != nil {
		t.Fatalf("write tmp env: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--env-file=" + tmp}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 (malformed), got %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "malformed") {
		t.Errorf("stderr should explain malformed input, got %q", stderr.String())
	}
}

// TestRun_ConstructedValidEnv_ExitsZero builds a fixture env file by
// taking the live config/generated/test.env (the SST emission shape)
// and overriding every ollama-routed model env var + ML profile JSON
// with a synthetic fixture model that FITS the live envelope. This is
// stable across pre-fix Scope 1 (gemma4:26b > 8G) and post-fix Scope 3
// (YAML defaults switched to gemma3:4b) because it constructs the env
// from primitives instead of relying on the YAML default.
//
// If the live test.env is absent, the test skips.
func TestRun_ConstructedValidEnv_ExitsZero(t *testing.T) {
	root := repoRoot(t)
	livePath := filepath.Join(root, "config", "generated", "test.env")
	liveBytes, err := os.ReadFile(livePath)
	if err != nil {
		t.Skipf("live config/generated/test.env not present: %v (run './smackerel.sh config generate --env test' first)", err)
	}
	live := string(liveBytes)

	// Override the model-name lines so every ollama-routed model points
	// to bug-045-fixture-llm-6gib (which we declare in a profile JSON
	// override below at 6144 MiB, well under the live 8G envelope).
	overrideKeys := []string{
		"LLM_MODEL", "OLLAMA_MODEL",
		"OLLAMA_VISION_MODEL", "OLLAMA_OCR_MODEL", "OLLAMA_REASONING_MODEL", "OLLAMA_FAST_MODEL",
		"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL",
		"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "PHOTOS_INTELLIGENCE_OCR_MODEL",
		"AGENT_PROVIDER_DEFAULT_MODEL", "AGENT_PROVIDER_REASONING_MODEL",
		"AGENT_PROVIDER_FAST_MODEL", "AGENT_PROVIDER_VISION_MODEL", "AGENT_PROVIDER_OCR_MODEL",
	}
	lines := strings.Split(live, "\n")
	for i, ln := range lines {
		for _, key := range overrideKeys {
			if strings.HasPrefix(ln, key+"=") {
				lines[i] = key + "=\"bug-045-fixture-llm-6gib\""
			}
		}
		if strings.HasPrefix(ln, "ML_MODEL_MEMORY_PROFILES_JSON=") {
			lines[i] = `ML_MODEL_MEMORY_PROFILES_JSON='[{"model":"bug-045-fixture-llm-6gib","memory_mib":6144},{"model":"bug-045-fixture-embed-512mib","memory_mib":512},{"model":"nomic-embed-text","memory_mib":768}]'`
		}
		if strings.HasPrefix(ln, "PHOTOS_INTELLIGENCE_EMBED_MODEL=") {
			lines[i] = `PHOTOS_INTELLIGENCE_EMBED_MODEL="bug-045-fixture-embed-512mib"`
		}
	}
	tmp := filepath.Join(t.TempDir(), "valid.env")
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatalf("write tmp env: %v", err)
	}

	// Snapshot env vars loadEnvFile will overwrite so other tests in
	// the same `go test` process do not inherit mutations.
	snapshot := map[string]string{}
	for _, key := range append(overrideKeys, "ML_MODEL_MEMORY_PROFILES_JSON", "PHOTOS_INTELLIGENCE_EMBED_MODEL") {
		snapshot[key] = os.Getenv(key)
		keyCopy := key
		t.Cleanup(func() {
			if snapshot[keyCopy] == "" {
				_ = os.Unsetenv(keyCopy)
				return
			}
			_ = os.Setenv(keyCopy, snapshot[keyCopy])
		})
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--env-file=" + tmp}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0 with fixture-model override, got %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "OK") {
		t.Errorf("stdout should contain 'OK', got %q", stdout.String())
	}
}

// TestRun_OversizedModel_ExitsOne is the RED detector for BUG-045-001
// at the binary surface. Maps to AC-5(c): a model whose declared
// profile memory exceeds the configured envelope MUST be rejected
// at config-validate time, with stderr naming the envelope key, the
// model name, and the required-memory value.
func TestRun_OversizedModel_ExitsOne(t *testing.T) {
	root := repoRoot(t)
	livePath := filepath.Join(root, "config", "generated", "test.env")
	liveBytes, err := os.ReadFile(livePath)
	if err != nil {
		t.Skipf("live config/generated/test.env not present: %v (run './smackerel.sh config generate --env test' first)", err)
	}
	live := string(liveBytes)

	lines := strings.Split(live, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(ln, "LLM_MODEL=") || strings.HasPrefix(ln, "OLLAMA_MODEL=") {
			lines[i] = strings.Split(ln, "=")[0] + "=\"bug-045-fixture-llm-20gib\""
		}
		for _, key := range []string{
			"OLLAMA_VISION_MODEL", "OLLAMA_OCR_MODEL", "OLLAMA_REASONING_MODEL", "OLLAMA_FAST_MODEL",
			"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL",
			"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "PHOTOS_INTELLIGENCE_OCR_MODEL",
			"AGENT_PROVIDER_DEFAULT_MODEL", "AGENT_PROVIDER_REASONING_MODEL",
			"AGENT_PROVIDER_FAST_MODEL", "AGENT_PROVIDER_VISION_MODEL", "AGENT_PROVIDER_OCR_MODEL",
		} {
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

	// Snapshot mutated env vars for restoration.
	snapshot := map[string]string{}
	for _, key := range []string{
		"LLM_MODEL", "OLLAMA_MODEL",
		"OLLAMA_VISION_MODEL", "OLLAMA_OCR_MODEL", "OLLAMA_REASONING_MODEL", "OLLAMA_FAST_MODEL",
		"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL",
		"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "PHOTOS_INTELLIGENCE_OCR_MODEL",
		"AGENT_PROVIDER_DEFAULT_MODEL", "AGENT_PROVIDER_REASONING_MODEL",
		"AGENT_PROVIDER_FAST_MODEL", "AGENT_PROVIDER_VISION_MODEL", "AGENT_PROVIDER_OCR_MODEL",
		"ML_MODEL_MEMORY_PROFILES_JSON", "PHOTOS_INTELLIGENCE_EMBED_MODEL",
	} {
		snapshot[key] = os.Getenv(key)
		keyCopy := key
		t.Cleanup(func() {
			if snapshot[keyCopy] == "" {
				_ = os.Unsetenv(keyCopy)
				return
			}
			_ = os.Setenv(keyCopy, snapshot[keyCopy])
		})
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--env-file=" + tmp}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 (oversized model rejected), got %d; stderr=%q", code, stderr.String())
	}
	for _, want := range []string{
		"OLLAMA_MEMORY_LIMIT",
		"bug-045-fixture-llm-20gib",
		"20480",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr missing required substring %q; stderr=%q", want, stderr.String())
		}
	}
}

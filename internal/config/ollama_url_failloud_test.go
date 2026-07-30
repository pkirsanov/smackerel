package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/testsupport/configgen"
)

// TestConfigGenerate_AbortsWhenOllamaURLAbsent guards the fail-loud
// [F-OLLAMA-URL-MISSING] contract in scripts/commands/config.sh.
//
// The Ollama daemon address is per-operator deployment topology, so
// config/smackerel.yaml carries llm.ollama_url: "" and the operator supplies
// the real address out of band (the gitignored .smackerel.local.env for dev,
// the deploy adapter's app.env injection for self-hosted). When neither source
// yields a value the loader MUST abort — never fall back to a hidden default,
// a ${VAR:-...} expansion, or a silent localhost (smackerel-no-defaults).
//
// Test-harnesses now pin configgen.SyntheticOllamaURL so they can exercise the
// behavior past this guard. That pin is a fixture, not a relaxation, and this
// test is what keeps the two apart:
//
//   - "absent" asserts the guard still aborts with the named failure code.
//   - "present" is the adversarial arm: the SAME invocation, differing ONLY in
//     the pin, must succeed. Without it the "absent" arm would still pass if
//     the generator broke for some unrelated reason, making the regression
//     tautological.
//
// Adversarial proof: replacing the guard with a default (e.g.
// OLLAMA_URL="${SMACKEREL_OLLAMA_URL:-http://localhost:11434}") makes the
// "absent" arm fail, because generation would then succeed.
func TestConfigGenerate_AbortsWhenOllamaURLAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("config generator shell test requires bash; skipping on windows")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	configSh := filepath.Join(repoRoot, "scripts", "commands", "config.sh")

	// The guard only reaches the SMACKEREL_OLLAMA_URL branch when the SST key
	// is also empty. Assert that precondition so this test cannot silently
	// become a no-op if a downstream overlay ever populates llm.ollama_url.
	rawYAML, err := os.ReadFile(filepath.Join(repoRoot, "config", "smackerel.yaml"))
	if err != nil {
		t.Fatalf("read config/smackerel.yaml: %v", err)
	}
	if !strings.Contains(string(rawYAML), `ollama_url: ""`) {
		t.Fatal("precondition failed: config/smackerel.yaml no longer carries an empty llm.ollama_url, so the [F-OLLAMA-URL-MISSING] branch is unreachable from this test")
	}

	run := func(t *testing.T, pinURL bool) (string, int) {
		t.Helper()
		cmd := exec.Command("bash", configSh, "--env", "dev")
		env := append(os.Environ(),
			"REPO_ROOT="+repoRoot,
			"SMACKEREL_GENERATED_DIR="+t.TempDir(),
			"SMACKEREL_HARDWARE_TIER=cpu",
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=safe.directory",
			"GIT_CONFIG_VALUE_0=*",
		)
		// Empty-string wins over any inherited value, so the "absent" arm is
		// hermetic even when a developer has the real address exported locally.
		env = append(env, "SMACKEREL_OLLAMA_URL=")
		if pinURL {
			env = append(env, "SMACKEREL_OLLAMA_URL="+configgen.SyntheticOllamaURL)
		}
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		exitCode := 0
		if err != nil {
			exitErr, isExit := err.(*exec.ExitError)
			if !isExit {
				t.Fatalf("exec config.sh: %v\n--- output ---\n%s\n--- end ---", err, out)
			}
			exitCode = exitErr.ExitCode()
		}
		return string(out), exitCode
	}

	t.Run("absent aborts fail-loud", func(t *testing.T) {
		out, exitCode := run(t, false)
		if exitCode == 0 {
			t.Fatalf("config.sh exited 0 with no Ollama URL — the [F-OLLAMA-URL-MISSING] guard was weakened or a default was reintroduced\n--- output ---\n%s\n--- end ---", out)
		}
		if !strings.Contains(out, "[F-OLLAMA-URL-MISSING]") {
			t.Fatalf("config.sh exited %d but did not emit [F-OLLAMA-URL-MISSING]\n--- output ---\n%s\n--- end ---", exitCode, out)
		}
	})

	t.Run("synthetic pin generates", func(t *testing.T) {
		out, exitCode := run(t, true)
		if exitCode != 0 {
			t.Fatalf("config.sh exited %d with the synthetic URL pinned — the fixture no longer clears the guard, so the sibling sub-test proves nothing\n--- output ---\n%s\n--- end ---", exitCode, out)
		}
	})
}

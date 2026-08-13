//go:build integration

// Spec 108 SCOPE-05 — TP-05-03.
//
// Asserts SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT reaches EVERY generated
// environment, and that a missing SST key fails generation loudly.
//
// WHY THIS IS NOT A "read the generated files" TEST
//
// `config/generated/*.env` are written by a root-owned container process, so a
// test running as the invoking user cannot read most of them. The established
// pattern elsewhere is `t.Skipf` when the file is unreadable — which turns a
// missing variable into a silent pass, exactly the failure mode this spec has
// been removing.
//
// So the authority here is the GENERATOR, which is always readable and is the
// thing that actually decides what every environment gets. The emission is
// unconditional (no per-environment branch), which is what makes "every
// environment" provable from one assertion rather than from N file reads that
// might all be skipped.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func corpusEnvRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot locate the repo root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestIntegration_CorpusGrantEnv_EmittedForEveryEnvironment_TP_05_03 proves the
// variable is emitted unconditionally and that at least one real generated
// environment carries it.
func TestIntegration_CorpusGrantEnv_EmittedForEveryEnvironment_TP_05_03(t *testing.T) {
	root := corpusEnvRepoRoot(t)

	genPath := filepath.Join(root, "scripts", "commands", "config.sh")
	genRaw, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatalf("read %s: %v", genPath, err)
	}
	gen := string(genRaw)

	t.Run("emission_is_unconditional_so_every_environment_gets_it", func(t *testing.T) {
		emit := regexp.MustCompile(`SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=\$\{SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT:\?[^}]*\}`)
		if !emit.MatchString(gen) {
			t.Fatal("config.sh does not emit SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT with a fail-loud ${VAR:?...} guard; an environment could receive an empty value")
		}

		// If the emission were wrapped in a per-environment conditional, some
		// environment would silently lack the variable and its core would fail
		// to resolve the stage at boot. Assert the emission line is not inside
		// an env-specific branch by checking it appears exactly once, in the
		// shared heredoc.
		if got := strings.Count(gen, "SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=${SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT:?"); got != 1 {
			t.Errorf("the emission appears %d times, want exactly 1; multiple emissions suggest per-environment branching, which is how one environment ends up without the variable", got)
		}
	})

	t.Run("sst_key_exists_so_generation_can_resolve_it", func(t *testing.T) {
		sstPath := filepath.Join(root, "config", "smackerel.yaml")
		sst, err := os.ReadFile(sstPath)
		if err != nil {
			t.Fatalf("read %s: %v", sstPath, err)
		}
		if !strings.Contains(string(sst), "corpus_grant_enforcement:") {
			t.Fatal("config/smackerel.yaml does not declare auth.corpus_grant_enforcement; generation would abort for every environment")
		}
		// required_value is the fail-loud reader. Without it a missing key
		// would resolve to empty rather than aborting.
		if !strings.Contains(gen, "required_value auth.corpus_grant_enforcement") {
			t.Error("config.sh does not read the key via required_value, so a missing key would NOT fail generation loudly")
		}
	})

	// At least one generated environment must actually carry it. This is the
	// end-to-end confirmation that the generator's intent survives to a real
	// artifact. It FAILS rather than skips when no env file is readable —
	// otherwise the whole test could pass on a machine where none exist.
	t.Run("a_real_generated_env_carries_the_variable", func(t *testing.T) {
		genDir := filepath.Join(root, "config", "generated")
		entries, err := os.ReadDir(genDir)
		if err != nil {
			t.Fatalf("read %s: %v", genDir, err)
		}

		checked := 0
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".env") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(genDir, e.Name()))
			if err != nil {
				// Root-owned artifact from a container write. Not readable is
				// not the same as not correct, so this file is skipped — but
				// the count below still requires SOME file to have been read.
				continue
			}
			checked++
			if !strings.Contains(string(raw), "SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=") {
				t.Errorf("generated env %s does not carry SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT; that environment's core cannot resolve the enforcement stage at boot", e.Name())
			}
		}

		if checked == 0 {
			t.Fatal("no generated .env file was readable, so this assertion proved nothing; run './smackerel.sh config generate' first rather than letting it pass silently")
		}
	})
}

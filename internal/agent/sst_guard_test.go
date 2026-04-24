package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Adversarial regression: SST zero-defaults guard for AGENT_* config.
//
// Spec 037 §11 and .github/copilot-instructions.md (SST Zero-Defaults
// Enforcement) forbid any hardcoded numeric or string default for an
// AGENT_* configuration value inside internal/agent/. Every value MUST flow
// from config/smackerel.yaml via ./smackerel.sh config generate and be read
// through os.Getenv / os.LookupEnv with no Go-side fallback.
//
// This guard scans every non-test .go file under internal/agent/ for the
// canonical ceiling literals declared in config/smackerel.yaml (0.65,
// 30000, 120000). If any of these literals appears outside an explicitly
// allowlisted location, the test fails — a future drift that re-introduces
// `if v == "" { v = "0.65" }` or similar would trip immediately.
func TestSST_NoHardcodedAgentDefaults(t *testing.T) {
	// The exact ceiling literals that MUST NOT appear as Go literals.
	// Strings are matched verbatim. They were chosen because they are
	// distinctive enough to be unambiguous (unlike "5" or "32").
	forbidden := []string{
		"0.65",   // routing.confidence_floor
		"120000", // defaults.timeout_ms_ceiling
		"30000",  // defaults.per_tool_timeout_ms_ceiling
	}

	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read internal/agent: %v", err)
	}

	// Pattern matching `os.Getenv("AGENT_X")` followed shortly by an "||"
	// or default-assignment pattern would also be a SST violation, but Go
	// doesn't have shell-style ${VAR:-default} so the literal-presence
	// guard above catches realistic drift. We additionally reject any
	// `getEnv("AGENT_*", "..."` two-arg helper that might be introduced
	// later as a convenience wrapper with a fallback default.
	twoArgGetEnv := regexp.MustCompile(`getEnv\(\s*"AGENT_[A-Z0-9_]+"\s*,\s*"[^"]+"\s*\)`)

	// The forbidden literal scan only applies to files that READ AGENT_*
	// environment variables — those are the files where a hardcoded
	// fallback could substitute for a missing config value. Other files
	// (e.g., the scenario loader, which encodes the design's §2.2 schema
	// validation ranges as literal upper/lower bounds) legitimately
	// contain the same numbers as bounded validation constants and are
	// NOT config-default sites. Scoping the guard this way matches its
	// stated purpose: prevent reintroduction of `getEnv("AGENT_X","0.65")`.
	envHandlingFiles := func(text string) bool {
		return strings.Contains(text, `os.Getenv("AGENT_`) ||
			strings.Contains(text, `os.LookupEnv("AGENT_`) ||
			twoArgGetEnv.MatchString(text)
	}

	scanned := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		full := filepath.Join(dir, name)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read %s: %v", full, err)
		}
		text := string(data)
		scanned++

		// Two-arg getEnv with a string literal default is forbidden
		// EVERYWHERE in the package — there is no legitimate use of that
		// pattern for AGENT_* config.
		if m := twoArgGetEnv.FindString(text); m != "" {
			t.Errorf("%s uses two-arg getEnv-with-default pattern %q — forbidden by SST zero-defaults policy", name, m)
		}

		// The literal-presence check only applies to env-handling files.
		if !envHandlingFiles(text) {
			continue
		}
		for _, lit := range forbidden {
			if strings.Contains(text, lit) {
				t.Errorf("%s reads AGENT_* env vars and contains forbidden hardcoded default literal %q — every AGENT_* value MUST flow from config/smackerel.yaml via os.Getenv with no Go-side fallback (spec 037 §11)",
					name, lit)
			}
		}
	}

	if scanned == 0 {
		t.Fatal("SST guard scanned zero files; agent package unexpectedly empty")
	}
}

//go:build integration

// Spec 065 SCOPE-4 — prompt-contract regression.
//
// TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent
// (SCN-065-A01 / SCN-065-A02) is the Success Signal regression
// required by scopes.md DoD: once the location_normalize micro-tool
// owns geocoder normalization, the weather scenario system_prompt
// MUST drop the per-state/per-nickname dictionary, and the
// allowed_tools list MUST advertise the new tool. We freeze a
// historical byte baseline so a future edit that re-bloats the
// prompt (or accidentally removes location_normalize from the
// allow-list) fails this test loudly.

package assistant_integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// weatherPromptHistoricalBytes is the size of the
// `system_prompt:` literal block in config/prompt_contracts/
// weather-query-v1.yaml as it shipped immediately before
// spec 065 introduced location_normalize. The block was measured
// (`awk '/^system_prompt:/,/^allowed_tools:/' ... | wc -c` = 1764
// bytes). The Success Signal in scopes.md SCOPE-4 requires a >=40%
// reduction once the micro-tool owns normalization, so the new
// block MUST be <= 60% of this baseline.
const weatherPromptHistoricalBytes = 1764

// shrinkFloorRatio is the upper bound on the post-shrink size as a
// fraction of the historical baseline. 0.60 encodes "shrink by at
// least 40%". Declared as a var (not const) so the int(...) cast at
// the call site is performed at runtime rather than triggering the
// Go "cannot convert constant" compile-time error.
var shrinkFloorRatio = 0.60

func TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent(t *testing.T) {
	repoRoot, err := findRepoRootFromCWD()
	if err != nil {
		t.Fatalf("findRepoRootFromCWD: %v", err)
	}
	yamlPath := filepath.Join(repoRoot, "config", "prompt_contracts", "weather-query-v1.yaml")
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read %s: %v", yamlPath, err)
	}

	t.Run("system_prompt_block_shrunk_by_at_least_40_percent", func(t *testing.T) {
		block := extractTopLevelBlockBytes(string(raw), "system_prompt:", "allowed_tools:")
		if block == 0 {
			t.Fatalf("could not locate `system_prompt:` ... `allowed_tools:` block in %s", yamlPath)
		}
		if got, want := block, int(float64(weatherPromptHistoricalBytes)*shrinkFloorRatio); got > want {
			t.Fatalf("weather system_prompt block = %d bytes, want <= %d (>=40%% reduction from baseline %d). Re-bloating the prompt regresses the SCOPE-4 Success Signal — restore the location_normalize delegation.",
				got, want, weatherPromptHistoricalBytes)
		}
	})

	t.Run("allowed_tools_lists_location_normalize", func(t *testing.T) {
		var parsed struct {
			AllowedTools []struct {
				Name string `yaml:"name"`
			} `yaml:"allowed_tools"`
		}
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("yaml unmarshal: %v", err)
		}
		var names []string
		for _, t := range parsed.AllowedTools {
			names = append(names, t.Name)
		}
		if !containsString(names, "location_normalize") {
			t.Fatalf("weather scenario allowed_tools = %v; want to include \"location_normalize\" so the agent can resolve canonical locations via the micro-tool instead of prompt-side dictionaries", names)
		}
		if !containsString(names, "weather_lookup") {
			t.Fatalf("weather scenario allowed_tools = %v; weather_lookup MUST remain allow-listed (regression check)", names)
		}
	})

	t.Run("prompt_no_longer_carries_inline_location_dictionary", func(t *testing.T) {
		// These two literals were the prompt-side normalization
		// dictionary the micro-tool now owns. If they reappear in
		// the prompt the test fails so a reviewer audits why the
		// dictionary leaked back into the LLM context.
		needles := []string{`"palm springs ca"`, `"nyc"`}
		body := string(raw)
		for _, n := range needles {
			if strings.Contains(body, n) {
				t.Errorf("weather prompt still contains inline normalization example %q; location_normalize is supposed to own this mapping", n)
			}
		}
	})
}

// extractTopLevelBlockBytes returns the byte length of the text
// starting at the first line that equals `startMarker` (top-level,
// no leading whitespace) up to but not including the next line that
// equals `endMarker`. Returns 0 if either marker is missing.
func extractTopLevelBlockBytes(body, startMarker, endMarker string) int {
	lines := strings.Split(body, "\n")
	start := -1
	end := -1
	for i, line := range lines {
		if strings.HasPrefix(line, startMarker) {
			start = i
			continue
		}
		if start >= 0 && strings.HasPrefix(line, endMarker) {
			end = i
			break
		}
	}
	if start < 0 || end < 0 {
		return 0
	}
	block := strings.Join(lines[start:end], "\n")
	// Preserve the trailing newline that wc -c sees between the
	// last line of the block and the next line.
	return len(block) + 1
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// findRepoRootFromCWD walks up from the current working directory
// looking for `go.mod` — the canonical repo-root marker. Tests
// already run with cwd inside the repo so this is reliable.
func findRepoRootFromCWD() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

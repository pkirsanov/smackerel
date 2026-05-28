// Unit tests for the spec 061 SCOPE-03 validation rule #6 hook
// (`ValidateScenariosPresent`).
package assistant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	// Tool init()s must run so the live scenario YAMLs validate
	// against a populated registry; without these the loader would
	// reject the 3 v1 scenarios and the happy-path test would fail
	// with the wrong error.
	_ "github.com/smackerel/smackerel/internal/agent/tools/notification"
	_ "github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	_ "github.com/smackerel/smackerel/internal/agent/tools/weather"
)

func TestValidateScenariosPresent_HappyPath(t *testing.T) {
	manifestPath := repoFile(t, "config", "assistant", "scenarios.yaml")
	scenarioDir := repoFile(t, "config", "prompt_contracts")
	if err := ValidateScenariosPresent(manifestPath, scenarioDir, func(string) (bool, bool) {
		return true, true
	}); err != nil {
		t.Fatalf("rule #6 unexpectedly failed: %v", err)
	}
}

func TestValidateScenariosPresent_MissingYAMLFlagged(t *testing.T) {
	// Stage a tampered scenario dir: copy the live prompt_contracts
	// dir into a temp dir, delete one of the 3 v1 YAMLs, and run the
	// validator. It MUST exit with the [F061-SCENARIO-MISSING] prefix
	// and name the now-missing scenario id.
	srcDir := repoFile(t, "config", "prompt_contracts")
	tmpDir := t.TempDir()

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("read live scenario dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Skip the weather scenario to simulate operator deletion.
		if e.Name() == "weather-query-v1.yaml" {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(tmpDir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("copy %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}

	manifestPath := repoFile(t, "config", "assistant", "scenarios.yaml")
	err = ValidateScenariosPresent(manifestPath, tmpDir, func(string) (bool, bool) {
		return true, true
	})
	if err == nil {
		t.Fatal("expected rule #6 to fail when weather YAML is missing")
	}
	if !strings.HasPrefix(err.Error(), "[F061-SCENARIO-MISSING]") {
		t.Errorf("missing prefix; got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "weather_query") {
		t.Errorf("error must name the missing scenario id; got %q", err.Error())
	}
}

func TestValidateScenariosPresent_EmptyArgsFailLoud(t *testing.T) {
	cases := []struct {
		name, manifest, scenarios string
	}{
		{"manifest empty", "", "/tmp/x"},
		{"scenarios empty", "/tmp/y", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScenariosPresent(tc.manifest, tc.scenarios, func(string) (bool, bool) { return true, true })
			if err == nil {
				t.Fatal("expected error on empty arg")
			}
			if !strings.HasPrefix(err.Error(), "[F061-SCENARIO-MISSING]") {
				t.Errorf("missing prefix; got %q", err.Error())
			}
		})
	}
}

func TestValidateScenariosPresent_BrokenManifestSurfacesPrefix(t *testing.T) {
	// A manifest that fails to parse should still surface the rule #6
	// prefix so operators see one signal at startup.
	tmp := t.TempDir()
	badPath := filepath.Join(tmp, "bad.yaml")
	writeTestFile(t, badPath, "not: a: valid: manifest")
	err := ValidateScenariosPresent(badPath, repoFile(t, "config", "prompt_contracts"),
		func(string) (bool, bool) { return true, true })
	if err == nil || !strings.HasPrefix(err.Error(), "[F061-SCENARIO-MISSING]") {
		t.Fatalf("expected [F061-SCENARIO-MISSING] prefix; got %v", err)
	}
}

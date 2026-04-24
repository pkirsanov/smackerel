package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadOneScenario writes `body` to a tmp dir, loads it, and returns the
// resulting Scenario. Test failures bubble up via t.Fatalf.
func loadOneScenario(t *testing.T, body string) *Scenario {
	t.Helper()
	registerLoaderTestTool(t)
	t.Cleanup(resetRegistryForTest)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	registered, rejected, err := DefaultLoader().Load(dir, "")
	if err != nil {
		t.Fatalf("fatal: %v", err)
	}
	if len(rejected) > 0 {
		t.Fatalf("rejected: %+v", rejected)
	}
	if len(registered) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(registered))
	}
	return registered[0]
}

// content_hash MUST be identical across whitespace-only / comment-only
// changes. The canonical projection collapses formatting; only semantic
// differences move the hash.
func TestContentHash_StableAcrossWhitespaceAndComments(t *testing.T) {
	a := loadOneScenario(t, validScenarioYAML)

	// Sprinkle blank lines + comments throughout. Re-parsed YAML is
	// semantically identical to the original.
	noisy := `# leading comment
` + strings.Replace(validScenarioYAML,
		"description: \"Answer expense questions\"",
		"\n# inline comment\ndescription:    \"Answer expense questions\"  # trailing", 1) + "\n\n"

	b := loadOneScenario(t, noisy)
	if a.ContentHash != b.ContentHash {
		t.Errorf("content hash drifted across whitespace/comment changes:\n  a=%s\n  b=%s",
			a.ContentHash, b.ContentHash)
	}
}

// content_hash MUST change when scenario semantics change.
func TestContentHash_ChangesWithSemantics(t *testing.T) {
	a := loadOneScenario(t, validScenarioYAML)

	mutated := strings.Replace(validScenarioYAML,
		"description: \"Answer expense questions\"",
		"description: \"Answer expense questions, but differently\"",
		1)
	b := loadOneScenario(t, mutated)
	if a.ContentHash == b.ContentHash {
		t.Errorf("content hash did not change when description changed: %s", a.ContentHash)
	}
}

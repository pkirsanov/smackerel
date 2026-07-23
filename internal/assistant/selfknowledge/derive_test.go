package selfknowledge

// derive_test.go — spec 104 SCOPE-02.
//
// Derives from the REAL committed config/assistant/scenarios.yaml (proving
// fresh-by-construction: no hand-maintained corpus) and asserts the derived
// entries carry the scenario labels + command descriptions, are complete,
// unique, and deterministic.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant"
)

func loadRealManifest(t *testing.T) *assistant.SkillsManifest {
	t.Helper()
	path := filepath.Join("..", "..", "..", "config", "assistant", "scenarios.yaml")
	m, err := assistant.LoadSkillsManifest(path, func(string) (bool, bool) { return true, true })
	if err != nil {
		t.Fatalf("LoadSkillsManifest(%s): %v", path, err)
	}
	return m
}

func TestDerive_FromRealScenariosYAML(t *testing.T) {
	m := loadRealManifest(t)
	entries := Derive(m)
	if len(entries) == 0 {
		t.Fatal("Derive returned no entries")
	}

	byID := make(map[string]CapabilityEntry, len(entries))
	for _, e := range entries {
		if e.Kind == "" || e.ID == "" || e.Title == "" || e.Body == "" || e.SourceRef == "" {
			t.Fatalf("entry has an empty field: %+v", e)
		}
		if _, dup := byID[e.ID]; dup {
			t.Fatalf("duplicate entry ID %q", e.ID)
		}
		byID[e.ID] = e
	}

	// The open_knowledge scenario (the /ask agent) MUST be present with its label.
	okEntry, found := byID["scenario:open_knowledge"]
	if !found {
		t.Fatal("missing scenario:open_knowledge entry")
	}
	if okEntry.Kind != KindScenario {
		t.Errorf("open_knowledge kind = %q, want %q", okEntry.Kind, KindScenario)
	}
	if okEntry.Title != "answer open question" {
		t.Errorf("open_knowledge title = %q, want %q", okEntry.Title, "answer open question")
	}
	if !strings.Contains(okEntry.Body, "open_knowledge") {
		t.Errorf("open_knowledge body missing skill id: %q", okEntry.Body)
	}
	// open_knowledge requires provenance → body advertises grounded/cited answers.
	if !strings.Contains(strings.ToLower(okEntry.Body), "cite") {
		t.Errorf("open_knowledge body should note grounded/cited answers: %q", okEntry.Body)
	}

	// The /ask command MUST be present and describe the mapped open_knowledge skill.
	ask, found := byID["command:/ask"]
	if !found {
		t.Fatal("missing command:/ask entry")
	}
	if ask.Kind != KindCommand {
		t.Errorf("/ask kind = %q, want %q", ask.Kind, KindCommand)
	}
	if !strings.Contains(ask.Body, "open question") {
		t.Errorf("/ask body should describe the mapped skill: %q", ask.Body)
	}

	// The /reset command maps to the capability reset action, not a scenario.
	reset, found := byID["command:/reset"]
	if !found {
		t.Fatal("missing command:/reset entry")
	}
	if !strings.Contains(strings.ToLower(reset.Body), "reset") {
		t.Errorf("/reset body should mention reset: %q", reset.Body)
	}
}

func TestDerive_Deterministic(t *testing.T) {
	m := loadRealManifest(t)
	a := Derive(m)
	b := Derive(m)
	if len(a) != len(b) {
		t.Fatalf("nondeterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic entry at %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestDerive_NilManifest(t *testing.T) {
	if got := Derive(nil); got != nil {
		t.Fatalf("Derive(nil) = %v, want nil", got)
	}
}

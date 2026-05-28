// Spec 061 SCOPE-10 — corpus structural validation test.
//
// Asserts the corpus YAML meets every structural invariant from
// design.md §13 item 6 (≥150 rows, ≥30 per label, no dup IDs,
// closed-vocabulary intent labels). Runs in every `go test` pass so
// a corpus mutation that drops below quota fails fast.

package assistanteval

import (
	"path/filepath"
	"testing"
)

func corpusPath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("corpus.yaml")
	if err != nil {
		t.Fatalf("filepath.Abs corpus.yaml: %v", err)
	}
	return abs
}

func TestCorpus_LoadsAndValidates(t *testing.T) {
	c, err := LoadCorpus(corpusPath(t))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if err := ValidateCorpus(c); err != nil {
		t.Fatalf("ValidateCorpus: %v", err)
	}
}

func TestCorpus_PerLabelFloor(t *testing.T) {
	c, err := LoadCorpus(corpusPath(t))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	counts := map[string]int{}
	for _, r := range c.Rows {
		counts[r.GroundTruthIntent]++
	}
	for _, l := range AllLabels {
		if counts[l] < MinPerLabel {
			t.Errorf("label %q has %d rows, want >= %d", l, counts[l], MinPerLabel)
		}
	}
}

func TestCorpus_TotalFloor(t *testing.T) {
	c, err := LoadCorpus(corpusPath(t))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(c.Rows) < MinCorpusSize {
		t.Errorf("corpus total %d below floor %d", len(c.Rows), MinCorpusSize)
	}
}

func TestCorpus_NoDuplicateIDs(t *testing.T) {
	c, err := LoadCorpus(corpusPath(t))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	seen := map[string]struct{}{}
	for _, r := range c.Rows {
		if _, dup := seen[r.ID]; dup {
			t.Errorf("duplicate row id %q", r.ID)
		}
		seen[r.ID] = struct{}{}
	}
}

func TestCorpus_OnlyAllowedLabels(t *testing.T) {
	c, err := LoadCorpus(corpusPath(t))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	allowed := map[string]struct{}{}
	for _, l := range AllLabels {
		allowed[l] = struct{}{}
	}
	for _, r := range c.Rows {
		if _, ok := allowed[r.GroundTruthIntent]; !ok {
			t.Errorf("row %q has unknown ground_truth_intent %q", r.ID, r.GroundTruthIntent)
		}
	}
}

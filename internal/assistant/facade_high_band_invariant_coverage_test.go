// BUG-061-009 (regression phase) — coverage closure for INV-HB-REFUSAL.
//
// TestHighBandNeverMaskedAsSavedAsIdea is the class-killer for the "saved as an
// idea" masking, but it sweeps a HAND-WRITTEN list (requiresProvenanceScenarios).
// A hand-written list is only a class-killer while it matches reality: a
// requires_provenance scenario absent from it is an uncovered copy of the same
// defect, and nothing fails when the SST gains one. That is not hypothetical —
// `open_knowledge` (the `/ask` scenario BUG-061-009 was actually reported
// against, and the only one with its own facade fast-path and its own
// OutcomeOK→StatusAnswered mapping) was missing from the list while the packet
// claimed the invariant covered it.
//
// This test closes the list over the SST: the covered set is checked against
// config/assistant/scenarios.yaml, the same file the runtime manifest loads. It
// fails on drift in either direction, so the invariant sweep cannot silently
// stop covering the class it exists to kill.

package assistant

import (
	"sort"
	"testing"
)

// TestRequiresProvenanceScenarios_ClosedOverSST — SCN-061-009-02 durability.
// Adversarial by construction: it fails when a requires_provenance scenario in
// the SST is missing from the invariant sweep (an uncovered masking path), and
// it fails when the sweep names a scenario the SST does not gate — including
// one retired from the manifest outright, which the sweep would keep "covering"
// against its own synthetic manifest forever. The final emptiness check keeps
// it from passing vacuously if the manifest ever loads without
// provenance-bearing scenarios.
func TestRequiresProvenanceScenarios_ClosedOverSST(t *testing.T) {
	t.Parallel()

	// Permissive resolver: this test asserts the provenance-coverage property,
	// not SST enable-key bookkeeping (TestLoadSkillsManifest_HappyPath owns
	// that). Resolving every key keeps the test from breaking for the wrong
	// reason when a new scenario is added.
	manifest, err := LoadSkillsManifest(
		repoFile(t, "config", "assistant", "scenarios.yaml"),
		func(string) (bool, bool) { return true, true },
	)
	if err != nil {
		t.Fatalf("LoadSkillsManifest(config/assistant/scenarios.yaml): %v", err)
	}

	swept := make(map[string]bool, len(requiresProvenanceScenarios))
	for _, id := range requiresProvenanceScenarios {
		swept[id] = true
	}

	allIDs := manifest.AllScenarioIDs()
	gated := make(map[string]bool, len(allIDs))
	var declared, uncovered, notProvenanceBearing []string
	for _, id := range allIDs {
		if !manifest.RequiresProvenance(id) {
			continue
		}
		gated[id] = true
		declared = append(declared, id)
		if !swept[id] {
			uncovered = append(uncovered, id)
		}
	}
	// Asked from the sweep side: an id retired from the SST is never visited
	// by a manifest walk, so a manifest walk cannot report it.
	for _, id := range requiresProvenanceScenarios {
		if !gated[id] {
			notProvenanceBearing = append(notProvenanceBearing, id)
		}
	}
	sort.Strings(declared)
	sort.Strings(uncovered)
	sort.Strings(notProvenanceBearing)

	if len(uncovered) > 0 {
		t.Errorf("requires_provenance scenario(s) %v are declared in config/assistant/scenarios.yaml but absent from requiresProvenanceScenarios %v — "+
			"each is an uncovered high-band path that can mask a refusal as 'saved as an idea' (INV-HB-REFUSAL)",
			uncovered, requiresProvenanceScenarios)
	}
	if len(notProvenanceBearing) > 0 {
		t.Errorf("requiresProvenanceScenarios names %v which config/assistant/scenarios.yaml does NOT declare with requires_provenance: true "+
			"(absent from the manifest, or present and false) — those sweep rows never exercise the provenance gate and prove nothing",
			notProvenanceBearing)
	}
	if len(declared) == 0 {
		t.Fatal("manifest declared zero requires_provenance scenarios — the closure assertion would pass vacuously; the manifest or its path is wrong")
	}
	t.Logf("SST requires_provenance scenarios (all swept by the INV-HB-REFUSAL invariant): %v", declared)
}

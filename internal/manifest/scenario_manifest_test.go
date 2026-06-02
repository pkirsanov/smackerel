// Package manifest — TP-076-01-01 (SCN-076-F01) unit test.
//
// Asserts the spec-076 inherited-behavior manifest invariants:
//
//   - Every scenario listed in spec 076 spec.md §5 has an entry in
//     scenario-manifest.json with `inheritsFrom` set to the
//     predecessor `(spec, scenarioId)`.
//   - Every inherited entry's `inheritsFrom.scenarioId` actually
//     appears as an `### SCN-...` heading or a `Scenario: SCN-...`
//     Gherkin line in the predecessor spec.md (byte-anchored link
//     check — the manifest entry's inheritsFrom IS the byte-stable
//     pointer to the predecessor's canonical Gherkin text per spec
//     076 SCN-076-F01).
//   - Foundation SCN-076-F01..F03 are present.
//   - No duplicate scenarioIds.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const spec076ManifestPath = "../../specs/076-assistant-completion-rescope/scenario-manifest.json"

func TestScenario076Manifest_InheritsFromLinksComplete(t *testing.T) {
	m, err := Load(spec076ManifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	// 1) No duplicate scenarioIds.
	seen := map[string]int{}
	for i, s := range m.Scenarios {
		if s.ScenarioID == "" {
			t.Fatalf("scenarios[%d]: empty scenarioId", i)
		}
		if prev, ok := seen[s.ScenarioID]; ok {
			t.Fatalf("scenarios[%d]: duplicate scenarioId %q (first at index %d)", i, s.ScenarioID, prev)
		}
		seen[s.ScenarioID] = i
	}

	// 2) Foundation scenarios present.
	for _, want := range []string{"SCN-076-F01", "SCN-076-F02", "SCN-076-F03"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("manifest missing foundation scenario %s", want)
		}
	}

	// 3) The set of inherited scenarios equals the set declared in
	//    spec 076 spec.md §5 (extracted by SCN-NNN-AXX token).
	spec076Path := "../../specs/076-assistant-completion-rescope/spec.md"
	declared, err := extractInheritedScenarioIDs(spec076Path)
	if err != nil {
		t.Fatalf("extract spec.md §5 SCNs: %v", err)
	}

	inheritedInManifest := map[string]InheritsFrom{}
	for _, s := range m.InheritedScenarios() {
		if s.InheritsFrom == nil || s.InheritsFrom.ScenarioID == "" || s.InheritsFrom.Spec == "" {
			t.Errorf("scenario %s: malformed inheritsFrom %+v", s.ScenarioID, s.InheritsFrom)
			continue
		}
		inheritedInManifest[s.ScenarioID] = *s.InheritsFrom
	}

	for id := range declared {
		if _, ok := inheritedInManifest[id]; !ok {
			t.Errorf("spec.md §5 names %s but scenario-manifest.json has no entry", id)
		}
	}
	for id := range inheritedInManifest {
		if _, ok := declared[id]; !ok {
			t.Errorf("scenario-manifest.json has inherited entry %s not listed in spec.md §5", id)
		}
	}

	// 4) Every inheritsFrom.scenarioId is anchored in the predecessor
	//    spec.md (byte-anchored link to the canonical Gherkin text).
	predCache := map[string]string{}
	for id, link := range inheritedInManifest {
		predPath := "../../" + link.Spec + "/spec.md"
		body, ok := predCache[predPath]
		if !ok {
			b, err := os.ReadFile(predPath)
			if err != nil {
				t.Errorf("predecessor spec not readable for %s → %s: %v", id, predPath, err)
				continue
			}
			body = string(b)
			predCache[predPath] = body
		}
		// Predecessors use either Markdown headings (### SCN-...) or
		// Gherkin (Scenario: SCN-...). Accept either anchor.
		needles := []string{
			"### " + link.ScenarioID + " ",
			"### " + link.ScenarioID + "\n",
			"Scenario: " + link.ScenarioID + " ",
			"Scenario: " + link.ScenarioID + "\n",
		}
		anchored := false
		for _, n := range needles {
			if strings.Contains(body, n) {
				anchored = true
				break
			}
		}
		if !anchored {
			t.Errorf("inherited %s → %s: %s not found as canonical heading in %s",
				id, link.Spec, link.ScenarioID, filepath.Base(predPath))
		}
	}
}

var inheritedScnPattern = regexp.MustCompile(`SCN-(064|065|066|073|074|075)-A\d{2}`)

// extractInheritedScenarioIDs scans spec.md §5 (Functional
// Requirements / Inherited BDD Scenarios) and returns the set of
// SCN-NNN-AXX ids it explicitly lists. Bound to §5 only by using the
// `## 5.` … `## 6.` slice.
func extractInheritedScenarioIDs(specPath string) (map[string]struct{}, error) {
	body, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	text := string(body)
	startIdx := strings.Index(text, "## 5.")
	if startIdx < 0 {
		return nil, fmt.Errorf("spec.md missing ## 5. section")
	}
	endIdx := strings.Index(text[startIdx:], "## 6.")
	if endIdx < 0 {
		return nil, fmt.Errorf("spec.md missing ## 6. section after ## 5.")
	}
	section := text[startIdx : startIdx+endIdx]
	out := map[string]struct{}{}
	for _, m := range inheritedScnPattern.FindAllString(section, -1) {
		out[m] = struct{}{}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("spec.md §5 contains no SCN-NNN-AXX identifiers")
	}
	return out, nil
}

// Package manifest — Spec 076 SCOPE-1: scenario manifest schema +
// loader for the spec-076 inherited-behavior manifest.
//
// Spec 076 inherits scenarios from specs 064/065/066/073/074/075. Each
// entry in `specs/076-assistant-completion-rescope/scenario-manifest.json`
// either (a) introduces a foundation scenario owned by spec 076
// (SCN-076-Fxx) or (b) inherits from a predecessor spec via
// `inheritsFrom: { spec, scenarioId }`.
//
// This package exposes the typed Manifest plus the
// scenario invariants the spec-076 SCN-076-F01 fail-loud test
// (`internal/manifest/scenario_manifest_test.go`) verifies:
//
//  1. Every entry has a non-empty scenarioId.
//  2. Every inherited entry has a well-formed inheritsFrom.
//  3. Every inheritsFrom.scenarioId actually exists in the
//     predecessor spec's `spec.md` source.
//  4. The set of inherited scenarios exactly equals the set named in
//     spec 076 `spec.md` §5 (no extras, no omissions).
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// InheritsFrom is the link from a spec-076 scenario to its
// predecessor canonical Gherkin (spec.md heading).
type InheritsFrom struct {
	Spec       string `json:"spec"`
	ScenarioID string `json:"scenarioId"`
}

// Scenario is one entry in the scenario-manifest.json `scenarios`
// array. Only the fields spec 076 SCOPE-1 invariants test are typed;
// unknown fields are tolerated.
type Scenario struct {
	ScenarioID       string        `json:"scenarioId"`
	ScopeID          string        `json:"scopeId"`
	Title            string        `json:"title"`
	BehaviorClass    string        `json:"behaviorClass"`
	ChangeType       string        `json:"changeType"`
	RequiredTestType []string      `json:"requiredTestType"`
	InheritsFrom     *InheritsFrom `json:"inheritsFrom,omitempty"`
}

// Manifest is the top-level JSON shape.
type Manifest struct {
	Version    int        `json:"version"`
	FeatureDir string     `json:"featureDir"`
	Scenarios  []Scenario `json:"scenarios"`
}

// Load reads a scenario-manifest.json file from disk and returns the
// typed shape. Returns a typed error if the file is missing, the JSON
// is malformed, or any structural invariant fails.
func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: read %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse %s: %w", path, err)
	}
	if len(m.Scenarios) == 0 {
		return nil, fmt.Errorf("manifest: %s has no scenarios", path)
	}
	return &m, nil
}

// ScenarioIDs returns the sorted list of scenarioIds present in the
// manifest. Used by callers asserting predecessor coverage.
func (m *Manifest) ScenarioIDs() []string {
	ids := make([]string, 0, len(m.Scenarios))
	for _, s := range m.Scenarios {
		ids = append(ids, s.ScenarioID)
	}
	sort.Strings(ids)
	return ids
}

// InheritedScenarios returns the subset of scenarios that carry an
// inheritsFrom link.
func (m *Manifest) InheritedScenarios() []Scenario {
	out := make([]Scenario, 0, len(m.Scenarios))
	for _, s := range m.Scenarios {
		if s.InheritsFrom != nil {
			out = append(out, s)
		}
	}
	return out
}

// FoundationScenarios returns the subset of scenarios introduced by
// this spec (no inheritsFrom link).
func (m *Manifest) FoundationScenarios() []Scenario {
	out := make([]Scenario, 0, len(m.Scenarios))
	for _, s := range m.Scenarios {
		if s.InheritsFrom == nil {
			out = append(out, s)
		}
	}
	return out
}

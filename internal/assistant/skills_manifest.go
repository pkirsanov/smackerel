// Package skills_manifest is part of internal/assistant/.
// Owned by spec 061 SCOPE-03. Reads config/assistant/scenarios.yaml
// (sibling lookup per SUBSTRATE-TOUCHPOINT-1 Option (b)) and exposes
// per-scenario user-facing metadata plus a runtime-enable filter
// (BS-008).
//
// The sibling file is consulted on construction only; reload requires
// reconstructing the manifest. This matches the bridge/loader lifecycle
// in Spec 037 (SIGHUP rescan) and keeps the manifest immutable per
// instance.

package assistant

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SkillsManifest is the immutable, parsed view of
// config/assistant/scenarios.yaml plus the resolved enable values
// (read once from the runtime SST snapshot supplied at construction).
type SkillsManifest struct {
	entries map[string]manifestEntry
}

// manifestEntry is the per-scenario record after sibling-file parse
// and SST enable resolution.
type manifestEntry struct {
	UserFacingLabel    string
	SlashShortcut      string
	RequiresProvenance bool
	ConfirmRequired    bool
	EnableSSTKey       string
	Enabled            bool // resolved from SST snapshot at construction
}

// raw* types mirror the on-disk YAML schema for parsing.
type rawManifest struct {
	Scenarios map[string]rawScenarioEntry `yaml:"scenarios"`
}

type rawScenarioEntry struct {
	UserFacing         *bool   `yaml:"user_facing"`
	UserFacingLabel    *string `yaml:"user_facing_label"`
	SlashShortcut      *string `yaml:"slash_shortcut"`
	RequiresProvenance *bool   `yaml:"requires_provenance"`
	ConfirmRequired    *bool   `yaml:"confirm_required"`
	EnableSSTKey       *string `yaml:"enable_sst_key"`
}

// EnableResolver returns whether the given SST key is enabled.
// Supplied at construction so the manifest is independent of any
// concrete config-loading mechanism.
type EnableResolver func(sstKey string) (enabled bool, found bool)

// LoadSkillsManifest reads the sibling lookup file at `path` and
// returns the resolved manifest. Every entry MUST have every field
// present and non-empty (zero-defaults / fail-loud per
// smackerel-no-defaults policy). `resolve` MUST resolve every
// declared `enable_sst_key`; missing keys are a fatal load error.
func LoadSkillsManifest(path string, resolve EnableResolver) (*SkillsManifest, error) {
	if path == "" {
		return nil, fmt.Errorf("skills_manifest: path is empty")
	}
	if resolve == nil {
		return nil, fmt.Errorf("skills_manifest: EnableResolver is nil")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("skills_manifest: read %s: %w", path, err)
	}

	var raw rawManifest
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("skills_manifest: parse %s: %w", path, err)
	}
	if len(raw.Scenarios) == 0 {
		return nil, fmt.Errorf("skills_manifest: %s: scenarios map is empty or missing", path)
	}

	entries := make(map[string]manifestEntry, len(raw.Scenarios))
	for id, r := range raw.Scenarios {
		if id == "" {
			return nil, fmt.Errorf("skills_manifest: %s: empty scenario id", path)
		}
		if r.UserFacing == nil {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q missing required field user_facing", path, id)
		}
		if !*r.UserFacing {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q has user_facing=false; non-user-facing scenarios MUST NOT appear in the sibling lookup file", path, id)
		}
		if r.UserFacingLabel == nil || *r.UserFacingLabel == "" {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q missing required non-empty user_facing_label", path, id)
		}
		if r.SlashShortcut == nil {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q missing required slash_shortcut (use empty string to opt out)", path, id)
		}
		if r.RequiresProvenance == nil {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q missing required field requires_provenance", path, id)
		}
		if r.ConfirmRequired == nil {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q missing required field confirm_required", path, id)
		}
		if r.EnableSSTKey == nil || *r.EnableSSTKey == "" {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q missing required non-empty enable_sst_key", path, id)
		}

		enabled, found := resolve(*r.EnableSSTKey)
		if !found {
			return nil, fmt.Errorf("skills_manifest: %s: scenario %q references enable_sst_key %q which is not present in the runtime SST snapshot", path, id, *r.EnableSSTKey)
		}

		entries[id] = manifestEntry{
			UserFacingLabel:    *r.UserFacingLabel,
			SlashShortcut:      *r.SlashShortcut,
			RequiresProvenance: *r.RequiresProvenance,
			ConfirmRequired:    *r.ConfirmRequired,
			EnableSSTKey:       *r.EnableSSTKey,
			Enabled:            enabled,
		}
	}
	return &SkillsManifest{entries: entries}, nil
}

// UserFacingLabel returns the per-design §4.1 short label for the
// given scenario id. The second return is false iff the id is not
// in the manifest.
func (m *SkillsManifest) UserFacingLabel(scenarioID string) (string, bool) {
	e, ok := m.entries[scenarioID]
	if !ok {
		return "", false
	}
	return e.UserFacingLabel, true
}

// RequiresProvenance reports whether the scenario MUST attach at
// least one Source to its response. Unknown scenarios default to
// false because the caller (the facade) is expected to have already
// validated that the scenario id is known; provenance enforcement
// for an unknown id is undefined and intentionally permissive here.
func (m *SkillsManifest) RequiresProvenance(scenarioID string) bool {
	return m.entries[scenarioID].RequiresProvenance
}

// ConfirmRequired reports whether the scenario triggers the
// capability confirm-card state machine.
func (m *SkillsManifest) ConfirmRequired(scenarioID string) bool {
	return m.entries[scenarioID].ConfirmRequired
}

// Enabled reports the runtime-resolved enable bit (BS-008 gate).
// An unknown id reports false so the facade filters it out of the
// candidate set.
func (m *SkillsManifest) Enabled(scenarioID string) bool {
	e, ok := m.entries[scenarioID]
	if !ok {
		return false
	}
	return e.Enabled
}

// EnabledScenarioIDs returns the ids of every scenario whose
// enable_sst_key resolved to true. Order is unspecified; callers
// that need a deterministic order MUST sort the result themselves.
func (m *SkillsManifest) EnabledScenarioIDs() []string {
	out := make([]string, 0, len(m.entries))
	for id, e := range m.entries {
		if e.Enabled {
			out = append(out, id)
		}
	}
	return out
}

// AllScenarioIDs returns every scenario id present in the manifest,
// regardless of enable state. Used by the loader-wiring test (SCOPE-03
// DoD #8) to assert sibling-file IDs all map to real Spec 037
// scenarios on disk.
func (m *SkillsManifest) AllScenarioIDs() []string {
	out := make([]string, 0, len(m.entries))
	for id := range m.entries {
		out = append(out, id)
	}
	return out
}

// Spec 061 SCOPE-04 — test-support helpers exported for out-of-package
// stress and integration test consumers (tests/stress, tests/e2e, etc.).
//
// This file is part of the production package on purpose — Go's
// internal_test.go pattern only exposes helpers to tests *within the
// same package*, but the spec 061 stress test lives under
// tests/stress/ to keep `./smackerel.sh test stress` discoverable.
// The helpers here are documented as test-only and intentionally
// surfaced as the minimum surface required to construct a Facade from
// outside the package.
//
// Do NOT call these from production callers. Production code must use
// LoadSkillsManifest against the on-disk sibling YAML.

package assistant

// ManifestEntryForTest mirrors the unexported manifestEntry struct
// one-to-one so test consumers in other packages can stage a manifest
// without going through the YAML loader.
//
// FOR TESTS ONLY.
type ManifestEntryForTest struct {
	UserFacingLabel    string
	SlashShortcut      string
	RequiresProvenance bool
	ConfirmRequired    bool
	EnableSSTKey       string
	Enabled            bool
}

// NewManifestForTest builds a SkillsManifest from an inline map. The
// supplied entries are stored verbatim — no validation is performed
// (callers in tests are expected to know what they are constructing).
//
// FOR TESTS ONLY.
func NewManifestForTest(entries map[string]ManifestEntryForTest) (*SkillsManifest, error) {
	out := &SkillsManifest{entries: make(map[string]manifestEntry, len(entries))}
	for id, e := range entries {
		out.entries[id] = manifestEntry{
			UserFacingLabel:    e.UserFacingLabel,
			SlashShortcut:      e.SlashShortcut,
			RequiresProvenance: e.RequiresProvenance,
			ConfirmRequired:    e.ConfirmRequired,
			EnableSSTKey:       e.EnableSSTKey,
			Enabled:            e.Enabled,
		}
	}
	return out, nil
}

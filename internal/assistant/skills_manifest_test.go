package assistant

import (
	"path/filepath"
	"strings"
	"testing"
)

// fakeResolve returns a closure satisfying EnableResolver against an
// in-memory map. Missing keys report found=false.
func fakeResolve(values map[string]bool) EnableResolver {
	return func(key string) (bool, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func TestLoadSkillsManifest_HappyPath(t *testing.T) {
	t.Parallel()
	path := repoFile(t, "config", "assistant", "scenarios.yaml")
	resolve := fakeResolve(map[string]bool{
		"assistant.skills.retrieval.enabled":     true,
		"assistant.skills.weather.enabled":       false,
		"assistant.skills.notifications.enabled": false,
	})

	m, err := LoadSkillsManifest(path, resolve)
	if err != nil {
		t.Fatalf("LoadSkillsManifest: %v", err)
	}

	// UserFacingLabel
	if label, ok := m.UserFacingLabel("retrieval_qa"); !ok || label != "search my notes" {
		t.Fatalf("retrieval_qa UserFacingLabel = (%q,%v); want (\"search my notes\", true)", label, ok)
	}
	if _, ok := m.UserFacingLabel("does_not_exist"); ok {
		t.Fatalf("unknown id should report ok=false")
	}

	// RequiresProvenance contract per design §5
	if !m.RequiresProvenance("retrieval_qa") {
		t.Fatalf("retrieval_qa MUST require provenance (Principle 8)")
	}
	if !m.RequiresProvenance("weather_query") {
		t.Fatalf("weather_query MUST require provenance (external attribution)")
	}
	if m.RequiresProvenance("notification_schedule") {
		t.Fatalf("notification_schedule MUST NOT require provenance — scheduler record IS the provenance")
	}

	// ConfirmRequired contract per design §5
	if !m.ConfirmRequired("notification_schedule") {
		t.Fatalf("notification_schedule MUST require confirm")
	}
	if m.ConfirmRequired("retrieval_qa") || m.ConfirmRequired("weather_query") {
		t.Fatalf("retrieval/weather MUST NOT require confirm")
	}
}

// TestSkillsManifest_DisabledScenarioFiltered is the BS-008 unit proof.
// When enable_sst_key resolves to false, Enabled() returns false and
// the id is excluded from EnabledScenarioIDs(); a fake candidate-filter
// downstream consumer then drops the scenario from the candidate set.
func TestSkillsManifest_DisabledScenarioFiltered(t *testing.T) {
	t.Parallel()
	path := repoFile(t, "config", "assistant", "scenarios.yaml")

	// retrieval_qa disabled; the other two enabled.
	resolve := fakeResolve(map[string]bool{
		"assistant.skills.retrieval.enabled":     false,
		"assistant.skills.weather.enabled":       true,
		"assistant.skills.notifications.enabled": true,
	})

	m, err := LoadSkillsManifest(path, resolve)
	if err != nil {
		t.Fatalf("LoadSkillsManifest: %v", err)
	}

	if m.Enabled("retrieval_qa") {
		t.Fatalf("retrieval_qa MUST report Enabled()=false when SST key is false")
	}
	if !m.Enabled("weather_query") || !m.Enabled("notification_schedule") {
		t.Fatalf("weather/notification MUST report Enabled()=true")
	}

	enabled := m.EnabledScenarioIDs()
	for _, id := range enabled {
		if id == "retrieval_qa" {
			t.Fatalf("EnabledScenarioIDs MUST NOT include retrieval_qa: got %v", enabled)
		}
	}
	if len(enabled) != 2 {
		t.Fatalf("EnabledScenarioIDs len = %d; want 2 (weather + notification): %v", len(enabled), enabled)
	}

	// Adversarial assertion: a downstream candidate-filter that drops
	// non-enabled scenarios MUST exclude retrieval_qa from the dispatch set.
	candidates := []string{"retrieval_qa", "weather_query", "notification_schedule"}
	filtered := filterCandidates(candidates, m)
	for _, id := range filtered {
		if id == "retrieval_qa" {
			t.Fatalf("filterCandidates leaked the disabled scenario: %v", filtered)
		}
	}
}

// TestSkillsManifest_MissingSSTKey proves a referenced enable_sst_key
// absent from the runtime SST snapshot is a fatal load error
// (zero-defaults / fail-loud).
func TestSkillsManifest_MissingSSTKey(t *testing.T) {
	t.Parallel()
	path := repoFile(t, "config", "assistant", "scenarios.yaml")

	// Intentionally omit the retrieval enable key.
	resolve := fakeResolve(map[string]bool{
		"assistant.skills.weather.enabled":       true,
		"assistant.skills.notifications.enabled": true,
	})

	_, err := LoadSkillsManifest(path, resolve)
	if err == nil {
		t.Fatalf("expected load error for missing SST key; got nil")
	}
	if !strings.Contains(err.Error(), "assistant.skills.retrieval.enabled") {
		t.Fatalf("error did not name the missing SST key: %v", err)
	}
}

// TestSkillsManifest_MissingRequiredField proves any missing required
// field in the sibling YAML is a fatal load error.
func TestSkillsManifest_MissingRequiredField(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "scenarios.yaml")
	writeTestFile(t, bad, `
scenarios:
  broken_one:
    user_facing: true
    user_facing_label: "x"
    slash_shortcut: ""
    # requires_provenance missing
    confirm_required: false
    enable_sst_key: "foo"
`)
	_, err := LoadSkillsManifest(bad, fakeResolve(map[string]bool{"foo": true}))
	if err == nil {
		t.Fatalf("expected load error for missing requires_provenance; got nil")
	}
	if !strings.Contains(err.Error(), "requires_provenance") {
		t.Fatalf("error did not name the missing field: %v", err)
	}
}

// TestSkillsManifest_NonUserFacingRejected proves an entry with
// user_facing=false is a fatal load error — non-user-facing scenarios
// MUST NOT appear in the sibling lookup file at all.
func TestSkillsManifest_NonUserFacingRejected(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "scenarios.yaml")
	writeTestFile(t, bad, `
scenarios:
  not_user_facing:
    user_facing: false
    user_facing_label: "x"
    slash_shortcut: ""
    requires_provenance: false
    confirm_required: false
    enable_sst_key: "foo"
`)
	_, err := LoadSkillsManifest(bad, fakeResolve(map[string]bool{"foo": true}))
	if err == nil {
		t.Fatalf("expected load error for user_facing=false; got nil")
	}
	if !strings.Contains(err.Error(), "user_facing=false") {
		t.Fatalf("error did not flag user_facing=false: %v", err)
	}
}

// filterCandidates is the local fake of the SCOPE-04 facade's
// candidate-set filter. Keeping it here keeps BS-008 self-contained;
// SCOPE-04 will use the same Enabled() contract on the real router.
func filterCandidates(ids []string, m *SkillsManifest) []string {
	out := ids[:0:0]
	for _, id := range ids {
		if m.Enabled(id) {
			out = append(out, id)
		}
	}
	return out
}

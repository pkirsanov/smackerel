// Package selfknowledge derives smackerel's self-knowledge corpus
// (spec 104) fresh-by-construction from the live product SSTs, so the
// assistant can answer questions about the product itself and the corpus
// never drifts from a hand-maintained doc.
//
// SCOPE-02 derives the auto-facets — scenarios/skills (from the parsed
// scenarios.yaml manifest) and slash commands (from the frozen shortcut
// map). SCOPE-05 adds the curated product-doc facet. The output is
// deterministic (sorted), so re-ingestion is idempotent (content_hash
// stable).
package selfknowledge

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant"
)

// Capability entry kinds.
const (
	KindScenario = "scenario"
	KindCommand  = "command"
	KindFeature  = "feature" // SCOPE-05 (curated docs)
	KindUsecase  = "usecase" // SCOPE-05 (curated docs)
)

// Source-ref anchors (provenance back to the owning SST).
const (
	scenariosSourceRef = "config/assistant/scenarios.yaml"
	shortcutsSourceRef = "internal/assistant/shortcuts.go"
)

// CapabilityEntry is one derived self-knowledge item. Body is the
// searchable/answerable text; SourceRef points back to the SST it came
// from. ID is stable across derivations so ingestion dedups cleanly.
type CapabilityEntry struct {
	Kind      string
	ID        string
	Title     string
	Body      string
	SourceRef string
}

// Derive builds the auto-derived self-knowledge corpus from the parsed
// skills manifest (the live scenarios.yaml view) and the frozen
// slash-shortcut map. Deterministic: entries are emitted in a stable
// sorted order (scenarios by id, then commands by name) so identical
// SSTs produce byte-identical bodies (idempotent re-ingestion, NFR-2).
//
// Scenarios reflect the runtime-ENABLED set (a disabled skill is not a
// current capability). Commands are the frozen v1 shortcut surface.
func Derive(m *assistant.SkillsManifest) []CapabilityEntry {
	if m == nil {
		return nil
	}
	out := make([]CapabilityEntry, 0, len(m.AllScenarioIDs())+len(assistant.SlashShortcuts))

	ids := m.EnabledScenarioIDs()
	sort.Strings(ids)
	for _, id := range ids {
		label, _ := m.UserFacingLabel(id)
		shortcut, _ := m.SlashShortcut(id)
		out = append(out, CapabilityEntry{
			Kind:      KindScenario,
			ID:        "scenario:" + id,
			Title:     label,
			Body:      scenarioBody(id, label, shortcut, m.RequiresProvenance(id)),
			SourceRef: scenariosSourceRef + "#" + id,
		})
	}

	cmds := make([]string, 0, len(assistant.SlashShortcuts))
	for c := range assistant.SlashShortcuts {
		cmds = append(cmds, c)
	}
	sort.Strings(cmds)
	for _, cmd := range cmds {
		out = append(out, CapabilityEntry{
			Kind:      KindCommand,
			ID:        "command:" + cmd,
			Title:     cmd,
			Body:      commandBody(cmd, assistant.SlashShortcuts[cmd], m),
			SourceRef: shortcutsSourceRef,
		})
	}
	return out
}

// scenarioBody renders the human/answerable description of a scenario.
func scenarioBody(id, label, shortcut string, grounded bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "smackerel can %s (skill %q).", label, id)
	if strings.TrimSpace(shortcut) != "" {
		fmt.Fprintf(&b, " Slash shortcut: %s.", shortcut)
	} else {
		b.WriteString(" No slash shortcut — reached by asking in natural language.")
	}
	if grounded {
		b.WriteString(" Answers are grounded in and cite their sources.")
	}
	return b.String()
}

// commandBody renders the human/answerable description of a slash command
// from the scenario it maps to (or the capability-level reset action).
func commandBody(cmd, target string, m *assistant.SkillsManifest) string {
	if target == assistant.ResetActionID {
		return fmt.Sprintf("The %s command resets the conversation, clearing any pending confirmation or disambiguation state.", cmd)
	}
	if label, ok := m.UserFacingLabel(target); ok {
		return fmt.Sprintf("The %s command lets you %s (skill %q).", cmd, label, target)
	}
	return fmt.Sprintf("The %s command routes to the %q skill.", cmd, target)
}

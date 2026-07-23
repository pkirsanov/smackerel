// Spec 066 SCOPE-1 — unit coverage for the /help body. SCN-066-A06.
// Spec 104 SCOPE-06 — /help is now rendered from the shared self-knowledge
// corpus (a newly-added scenario appears with no help-code edit).
package telegram

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/selfknowledge"
)

// helpTestCaps derives the real capability corpus from the committed
// scenarios.yaml, exactly as cmd/core does at boot, so /help assertions run
// against the production corpus (no hand-maintained fixture).
func helpTestCaps(t *testing.T) []selfknowledge.CapabilityEntry {
	t.Helper()
	m, err := assistant.LoadSkillsManifest(
		"../../config/assistant/scenarios.yaml",
		func(string) (bool, bool) { return true, true },
	)
	if err != nil {
		t.Fatalf("load skills manifest: %v", err)
	}
	return selfknowledge.Derive(m)
}

func TestHelpListsNaturalLanguageExamplesAndNoRetiredCommands(t *testing.T) {
	body := HelpText(helpTestCaps(t))

	// Plain-English examples MUST be taught for the retired surfaces.
	mustContain := []string{
		`"find my notes`,
		`"rate the`,
		`"what do I know about`,
		`"show me the concept`,
		`"add milk to my shopping list"`,
		`"record a `,
		`"watch for `,
		`"plan meals `,
		`"find the pad thai recipe"`,
		`"start cooking`,
		`"show knowledge quality issues"`,
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Errorf("HelpText missing required plain-English example %q", want)
		}
	}

	// Operational commands MUST be listed.
	for _, op := range []string{"/help", "/status", "/reset", "/digest", "/recent", "/done"} {
		if !strings.Contains(body, op) {
			t.Errorf("HelpText missing operational command %q", op)
		}
	}
	// Retained shortcuts MUST be listed.
	for _, sc := range []string{"/ask ", "/weather", "/remind"} {
		if !strings.Contains(body, sc) {
			t.Errorf("HelpText missing retained shortcut %q", sc)
		}
	}

	// Retired slash commands MUST NOT appear as active instructions.
	// Adversarial: if a future edit re-introduces "/find <query>" as
	// an active help line this assertion catches it. Substrings that
	// would naturally appear inside a plain-English example
	// ("/help", "/status", etc.) are excluded by the leading space +
	// the closing token list.
	retired := []string{
		"/find ", "/rate ", "/concept", "/person", "/list ",
		"/expense", "/watch ", "/lint", "/meal_plan", "/recipe", "/cook",
	}
	for _, r := range retired {
		if strings.Contains(body, r) {
			t.Errorf("HelpText must not advertise retired command %q as an active option", r)
		}
	}
}

// TestHelp_RendersCapabilitiesFromSharedCorpus proves the /help menu and the
// /ask self-knowledge answers cannot diverge: the "What I can help with"
// section is built from the SAME derived corpus, so every enabled scenario's
// label appears — and a newly-added scenario appears with NO help-code edit.
func TestHelp_RendersCapabilitiesFromSharedCorpus(t *testing.T) {
	body := HelpText(helpTestCaps(t))
	if !strings.Contains(body, "What I can help with:") {
		t.Fatal("capability section header missing")
	}
	for _, label := range []string{"search my notes", "check weather", "remind me", "find recipes", "answer open question"} {
		if !strings.Contains(body, label) {
			t.Errorf("capability section missing scenario label %q (should be derived from the corpus)", label)
		}
	}

	// Adversarial: a brand-new scenario the help code has never seen appears
	// solely because it is in the corpus (no HelpText edit required).
	withNew := append(helpTestCaps(t), selfknowledge.CapabilityEntry{
		Kind:  selfknowledge.KindScenario,
		ID:    "scenario:brand_new_capability",
		Title: "do a brand new thing",
	})
	if !strings.Contains(HelpText(withNew), "do a brand new thing") {
		t.Error("a newly-added corpus scenario must appear in /help with no help-code edit")
	}

	// Command-kind entries do NOT surface retiring slash commands in the
	// capability list (spec 066: /recipe, /cook stay hidden).
	onlyCmd := []selfknowledge.CapabilityEntry{
		{Kind: selfknowledge.KindCommand, ID: "command:/recipe", Title: "/recipe"},
	}
	if strings.Contains(HelpText(onlyCmd), "\n- /recipe") {
		t.Error("command-kind entries must not surface retiring slash commands in /help")
	}
}

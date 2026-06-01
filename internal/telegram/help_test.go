// Spec 066 SCOPE-1 — unit coverage for the /help body. SCN-066-A06.
package telegram

import (
	"strings"
	"testing"
)

func TestHelpListsNaturalLanguageExamplesAndNoRetiredCommands(t *testing.T) {
	body := HelpText()

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

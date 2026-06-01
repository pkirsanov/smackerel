// Spec 066 SCOPE-1 — unit coverage for the Retired Command Policy
// Foundation. Covers SCN-066-A01 (BotCommands inventory after the
// alias window) and the closed classifier table.
package telegram

import (
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// retiredCommandTokens lists every command token the spec retires —
// the inverse assertion target for SCN-066-A01.
var retiredCommandTokens = []string{
	"find", "rate", "concept", "person", "list",
	"expense", "watch", "lint", "meal_plan", "recipe", "cook",
}

func TestBotCommandsAfterRetirementContainsOnlyOperationalSet(t *testing.T) {
	// SCN-066-A01 — Given the legacy_alias_window_until date is in
	// the past, When a Telegram client requests the BotCommands menu,
	// Then the menu contains exactly the operational set plus
	// retained shortcuts and contains none of the retired aliases.
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	windowUntil := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	cmds := BotCommandsForWindow(now, windowUntil)

	want := []string{
		"help", "status", "reset", "digest", "recent", "done",
		"ask", "weather", "remind",
	}
	if len(cmds) != len(want) {
		t.Fatalf("BotCommands count after window: got %d (%v), want %d (%v)",
			len(cmds), commandTokens(cmds), len(want), want)
	}
	for i, w := range want {
		if cmds[i].Command != w {
			t.Errorf("BotCommands[%d]: got %q, want %q (full list: %v)",
				i, cmds[i].Command, w, commandTokens(cmds))
		}
	}
	// Adversarial: prove the assertion would catch a retired-alias
	// leak. Iterate the closed retired list and reject membership.
	got := map[string]bool{}
	for _, c := range cmds {
		got[c.Command] = true
	}
	for _, retired := range retiredCommandTokens {
		if got[retired] {
			t.Errorf("retired command %q must not appear in post-window BotCommands", retired)
		}
	}
}

func TestBotCommandsInsideWindowStillAdvertisesRetiredAliases(t *testing.T) {
	// Inverse of SCN-066-A01 — inside the window the menu must keep
	// the retired aliases so existing muscle memory keeps working
	// alongside the SCOPE-2 transparent rewrite. This is the
	// adversarial pair of the post-window assertion above; without
	// it a regression that simply hides the aliases at all times
	// would silently pass SCN-066-A01.
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	windowUntil := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	cmds := BotCommandsForWindow(now, windowUntil)
	got := map[string]bool{}
	for _, c := range cmds {
		got[c.Command] = true
	}
	for _, retired := range retiredCommandTokens {
		if !got[retired] {
			t.Errorf("retired command %q MUST appear in in-window BotCommands so the alias rewrite remains discoverable", retired)
		}
	}
	// Every retired alias description must carry the "[retiring]"
	// prefix so the menu visibly signals deprecation while the
	// rewrite is still active.
	for _, c := range cmds {
		if !containsToken(retiredCommandTokens, c.Command) {
			continue
		}
		if !strings.HasPrefix(c.Description, "[retiring]") {
			t.Errorf("retired alias %q in-window description must start with \"[retiring]\"; got %q",
				c.Command, c.Description)
		}
	}
}

func TestClassifyCommandClosedTable(t *testing.T) {
	cases := []struct {
		cmd  string
		want LegacyCommandClass
	}{
		{"help", LegacyCommandOperational},
		{"status", LegacyCommandOperational},
		{"reset", LegacyCommandOperational},
		{"digest", LegacyCommandOperational},
		{"recent", LegacyCommandOperational},
		{"done", LegacyCommandOperational},

		{"ask", LegacyCommandRetainedShortcut},
		{"weather", LegacyCommandRetainedShortcut},
		{"remind", LegacyCommandRetainedShortcut},

		{"find", LegacyCommandRetiredAlias},
		{"rate", LegacyCommandRetiredAlias},
		{"concept", LegacyCommandRetiredAlias},
		{"person", LegacyCommandRetiredAlias},
		{"list", LegacyCommandRetiredAlias},
		{"expense", LegacyCommandRetiredAlias},
		{"watch", LegacyCommandRetiredAlias},
		{"lint", LegacyCommandRetiredAlias},
		{"meal_plan", LegacyCommandRetiredAlias},
		{"recipe", LegacyCommandRetiredAlias},
		{"cook", LegacyCommandRetiredAlias},

		{"", LegacyCommandUnknown},
		{"unknownish", LegacyCommandUnknown},
		// Casing is significant — Telegram normalises to lowercase
		// before delivery so the classifier intentionally rejects
		// other casings to keep the table closed.
		{"Find", LegacyCommandUnknown},
		{"STATUS", LegacyCommandUnknown},
	}
	for _, tc := range cases {
		if got := ClassifyCommand(tc.cmd); got != tc.want {
			t.Errorf("ClassifyCommand(%q) = %d, want %d", tc.cmd, got, tc.want)
		}
	}
}

func TestRetiredAliasTableHasNonEmptyReplacementPrompts(t *testing.T) {
	// The runtime rewrite in SCOPE-2 substitutes `{args}` into
	// PromptTemplate. Every row MUST therefore declare a non-empty
	// template; an empty template would collapse the rewrite to a
	// bare prompt and produce a silent regression.
	for _, row := range RetiredAliasTable() {
		if row.Command == "" {
			t.Errorf("retired alias row with empty Command: %+v", row)
		}
		if strings.TrimSpace(row.PromptTemplate) == "" {
			t.Errorf("retired alias %q has empty PromptTemplate", row.Command)
		}
		if row.RetiredSurface == "" {
			t.Errorf("retired alias %q missing RetiredSurface label", row.Command)
		}
		if len(row.SuccessorSpecs) == 0 {
			t.Errorf("retired alias %q missing SuccessorSpecs", row.Command)
		}
	}
}

func commandTokens(cmds []tgbotapi.BotCommand) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, c.Command)
	}
	return out
}

func containsToken(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// Spec 066 SCOPE-1 — Retired Command Policy Foundation.
//
// This file is the pure-policy foundation for spec 066. It defines:
//
//   - The finite Telegram command classifier (operational | retained
//     shortcut | retired alias | unknown).
//   - The finite retired-alias table from design.md "Concrete
//     Implementations → Telegram Legacy Alias Adapter".
//   - BotCommandsForWindow — the time-aware BotCommands menu selector
//     that hides retired-alias entries once the configured alias
//     window has expired (SCN-066-A01).
//   - HelpText — the canonical /help body that teaches plain-English
//     examples and contains no active instructions to use any
//     retired command (SCN-066-A06).
//
// Runtime wiring (SetMyCommands, the alias rewrite + notice store,
// the expired-command rejection envelope, and SST keys for the
// alias-window timestamp) is owned by SCOPE-2 and is intentionally
// NOT touched here. This file is consumed by unit tests and by the
// later runtime wiring without itself introducing new dependencies.
package telegram

import (
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// LegacyCommandClass is the closed enumeration of Telegram command
// classes the spec 066 classifier recognises. design.md §Capability
// Foundation "Variation Axes → Command class".
type LegacyCommandClass int

const (
	// LegacyCommandUnknown is the default zero value and is returned
	// for any token the classifier does not recognise. The caller is
	// responsible for emitting the deterministic unknown-command
	// envelope (see design.md "API And Contracts").
	LegacyCommandUnknown LegacyCommandClass = iota

	// LegacyCommandOperational identifies deterministic transport
	// controls that MUST NOT invoke the LLM or the assistant facade
	// (SCN-066-A09). Membership is the operational command inventory
	// in spec.md §Outcome Contract.
	LegacyCommandOperational

	// LegacyCommandRetainedShortcut identifies v1 power shortcuts
	// retained by spec 061 (SCOPE-06 Round 49 BS-002/007/010) that
	// route through the assistant facade via synthetic CompiledIntent
	// traces. These remain available after the alias window expires.
	LegacyCommandRetainedShortcut

	// LegacyCommandRetiredAlias identifies a token from the finite
	// retired-alias table. Inside the configured alias window the
	// caller rewrites the input to the canonical natural-language
	// prompt and emits the one-time deprecation notice (SCN-066-A04);
	// after the window expires the caller rejects the input before
	// facade invocation (SCN-066-A05).
	LegacyCommandRetiredAlias
)

// LegacyAlias is one row of the finite retired-alias table. design.md
// "Capability Foundation → LegacyAlias" + "Concrete Implementations
// → Telegram Legacy Alias Adapter".
//
// Command is the bare command token without the leading "/". The
// table is deliberately closed: new entries require an explicit code
// change + spec review.
type LegacyAlias struct {
	// Command is the retired Telegram command token (no leading slash).
	Command string
	// PromptTemplate is the canonical natural-language replacement.
	// `{args}` is substituted with the trimmed command arguments at
	// rewrite time by SCOPE-2; an empty `{args}` token MUST collapse
	// to a sensible bare prompt.
	PromptTemplate string
	// RetiredSurface is the spec-section label this row supersedes
	// (recorded for audit + observability labels).
	RetiredSurface string
	// SuccessorSpecs lists the spec identifiers that implement the
	// replacement capability (consumed by docs / Consumer Impact
	// Sweep tooling).
	SuccessorSpecs []string
}

// operationalCommands is the closed inventory of deterministic
// Telegram commands retained after the alias window. Order matches
// spec.md §Outcome Contract.
var operationalCommands = []tgbotapi.BotCommand{
	{Command: "help", Description: "Show available commands"},
	{Command: "status", Description: "System status"},
	{Command: "reset", Description: "Reset assistant conversation"},
	{Command: "digest", Description: "Get today's digest"},
	{Command: "recent", Description: "Recent captured items"},
	{Command: "done", Description: "Finalize conversation assembly"},
}

// retainedShortcutCommands lists the spec 061 v1 power shortcuts
// retained alongside the operational set. Order matches spec.md.
var retainedShortcutCommands = []tgbotapi.BotCommand{
	{Command: "ask", Description: "Ask the assistant (retrieval Q&A)"},
	{Command: "weather", Description: "Weather lookup"},
	{Command: "remind", Description: "Schedule a reminder"},
}

// retiredAliasCommands is the closed retired-alias table. Inside the
// configured alias window these entries are still exposed via
// SetMyCommands so existing muscle memory keeps working with the
// transparent rewrite; after the window expires SetMyCommands omits
// them and the runtime rejects invocations.
var retiredAliasCommands = []tgbotapi.BotCommand{
	{Command: "find", Description: "[retiring] use plain English to search"},
	{Command: "rate", Description: "[retiring] use plain English to rate"},
	{Command: "concept", Description: "[retiring] use plain English to ask about a concept"},
	{Command: "person", Description: "[retiring] use plain English to ask about a person"},
	{Command: "list", Description: "[retiring] use plain English to manage lists"},
	{Command: "expense", Description: "[retiring] use plain English for expenses"},
	{Command: "watch", Description: "[retiring] use plain English to manage watches"},
	{Command: "lint", Description: "[retiring] ask about knowledge quality"},
	{Command: "meal_plan", Description: "[retiring] use plain English to plan meals"},
	{Command: "recipe", Description: "[retiring] use plain English for recipes"},
	{Command: "cook", Description: "[retiring] use plain English to start cooking"},
}

// retiredAliasTable is the canonical rewrite table consumed by SCOPE-2.
// design.md "Concrete Implementations → Telegram Legacy Alias Adapter".
var retiredAliasTable = []LegacyAlias{
	{Command: "find", PromptTemplate: "find {args}", RetiredSurface: "spec 026 retrieval slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
	{Command: "rate", PromptTemplate: "rate {args}", RetiredSurface: "spec 027 annotation slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
	{Command: "concept", PromptTemplate: "show me the concept {args}", RetiredSurface: "spec 025 knowledge slash", SuccessorSpecs: []string{"spec 061", "spec 065"}},
	{Command: "person", PromptTemplate: "show me what I know about {args}", RetiredSurface: "spec 026 person slash", SuccessorSpecs: []string{"spec 061", "spec 065"}},
	{Command: "list", PromptTemplate: "manage my list: {args}", RetiredSurface: "spec 028 list slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
	{Command: "expense", PromptTemplate: "record or review expense: {args}", RetiredSurface: "spec 034 expense slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
	{Command: "watch", PromptTemplate: "watch for {args}", RetiredSurface: "spec 039 watch slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
	{Command: "lint", PromptTemplate: "show knowledge quality issues", RetiredSurface: "spec 025 lint slash", SuccessorSpecs: []string{"spec 061"}},
	{Command: "meal_plan", PromptTemplate: "plan meals {args}", RetiredSurface: "spec 036 meal-plan slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
	{Command: "recipe", PromptTemplate: "find or use recipe {args}", RetiredSurface: "spec 035 recipe slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
	{Command: "cook", PromptTemplate: "start cooking {args}", RetiredSurface: "spec 035 cook slash", SuccessorSpecs: []string{"spec 061", "spec 068"}},
}

// ClassifyCommand returns the finite class of a bare command token
// (without the leading "/"). The classifier is total: every input
// resolves to exactly one class. Casing is significant — Telegram
// normalises slash commands to lowercase before delivery so the
// classifier intentionally does not case-fold.
func ClassifyCommand(cmd string) LegacyCommandClass {
	for _, op := range operationalCommands {
		if op.Command == cmd {
			return LegacyCommandOperational
		}
	}
	for _, sc := range retainedShortcutCommands {
		if sc.Command == cmd {
			return LegacyCommandRetainedShortcut
		}
	}
	for _, ra := range retiredAliasTable {
		if ra.Command == cmd {
			return LegacyCommandRetiredAlias
		}
	}
	return LegacyCommandUnknown
}

// RetiredAliasTable returns a defensive copy of the closed
// retired-alias table for consumers (SCOPE-2, docs tooling) that need
// to iterate without risking mutation.
func RetiredAliasTable() []LegacyAlias {
	out := make([]LegacyAlias, len(retiredAliasTable))
	copy(out, retiredAliasTable)
	return out
}

// OperationalCommands returns a defensive copy of the deterministic
// operational command inventory.
func OperationalCommands() []tgbotapi.BotCommand {
	out := make([]tgbotapi.BotCommand, len(operationalCommands))
	copy(out, operationalCommands)
	return out
}

// RetainedShortcutCommands returns a defensive copy of the spec 061
// retained shortcut inventory.
func RetainedShortcutCommands() []tgbotapi.BotCommand {
	out := make([]tgbotapi.BotCommand, len(retainedShortcutCommands))
	copy(out, retainedShortcutCommands)
	return out
}

// BotCommandsForWindow returns the SetMyCommands inventory for the
// given moment. Inside the configured alias window the menu still
// advertises the retired aliases (with a "[retiring]" description
// prefix) so existing muscle memory keeps working with the
// transparent rewrite. Once `now` is at or after `windowUntil` the
// menu reduces to the operational + retained-shortcut set only —
// this is the SCN-066-A01 observable.
//
// The function is pure (no I/O, no globals beyond the closed tables)
// so it is trivially unit-testable without standing up SST or the
// Telegram client.
func BotCommandsForWindow(now, windowUntil time.Time) []tgbotapi.BotCommand {
	out := make([]tgbotapi.BotCommand, 0, len(operationalCommands)+len(retainedShortcutCommands)+len(retiredAliasCommands))
	out = append(out, operationalCommands...)
	out = append(out, retainedShortcutCommands...)
	if now.Before(windowUntil) {
		out = append(out, retiredAliasCommands...)
	}
	return out
}

// BotCommandsForState returns the SetMyCommands inventory driven by
// the spec 075 effective WindowState. WindowClosed reduces the menu
// to the operational + retained-shortcut set (SCN-066-A01).
// WindowOpen and WindowPaused both keep the retired aliases visible
// so muscle memory continues to work alongside the transparent
// rewrite (open) or the safety-mode passthrough (paused).
func BotCommandsForState(state legacyretirement.WindowState) []tgbotapi.BotCommand {
	out := make([]tgbotapi.BotCommand, 0, len(operationalCommands)+len(retainedShortcutCommands)+len(retiredAliasCommands))
	out = append(out, operationalCommands...)
	out = append(out, retainedShortcutCommands...)
	if state != legacyretirement.WindowClosed {
		out = append(out, retiredAliasCommands...)
	}
	return out
}

// HelpText returns the canonical /help body. It enumerates the
// operational + retained-shortcut commands and teaches the
// natural-language replacement examples for retired surfaces; it
// contains no instruction to use any retired slash command as an
// active option. SCN-066-A06.
func HelpText() string {
	return `> Smackerel Bot
Just type what you want — no slash commands needed for everyday use.

Operational commands:
- /help — show this help
- /status — system status
- /reset — reset assistant conversation
- /digest — today's digest
- /recent — recent captured items
- /done — finalize conversation assembly

Power shortcuts:
- /ask <question> — ask the assistant (retrieval Q&A)
- /weather [city] — weather lookup
- /remind <when> <what> — schedule a reminder

Plain-English examples (no slash needed):
- "find my notes about ACL tags"
- "rate the carbonara recipe 4 out of 5"
- "what do I know about Alice?"
- "show me the concept for vector indexes"
- "add milk to my shopping list"
- "record a 12 EUR coffee expense"
- "watch for new posts about Postgres"
- "plan meals for this week"
- "find the pad thai recipe"
- "start cooking tonight's dinner"
- "show knowledge quality issues"

Capture is automatic:
- Send a URL to save an article or video
- Send text to save an idea
- Send a voice note to transcribe and save
- Forward messages to assemble conversations
- Reply to a saved item to annotate it`
}

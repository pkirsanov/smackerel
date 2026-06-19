// Spec 096 SCOPE-07 — the Telegram `/model` COMBINED (multi-provider) picker.
//
// This EXTENDS the spec-089 numbered picker (model_command.go) to render the
// SCOPE-04 combined, provider-qualified catalog across every connected provider
// — it does NOT fork the picker, the validator, or the store. The numbered-reply
// mechanic + the `current` / `system default` tags + the claim-bound modelpref
// persistence are preserved VERBATIM (the same handleModelSelectionReply path);
// the only change is the LIST the user sees:
//
//   - ONE provider-grouped, provider-qualified, cost-hinted list (Ollama/local
//     group FIRST).
//   - Only REACHABLE models are numbered, so a numbered reply ALWAYS maps to a
//     dispatchable model (the returned ordered id list is exactly the numbered
//     set, fed to the existing pendingModelSelection).
//   - An unreachable / timed-out / auth-failed provider is SHOWN-BUT-DISABLED
//     with its typed ProviderDiscoveryStatus — never silently dropped
//     (Principle 8 transparency, graceful degradation NFR-1).
//
// When no combined-catalog source is wired (the deferred live-aggregator
// activation state), modelPickerReplyCombined delegates to the byte-for-byte
// 089 flat picker (modelPickerReply) — an 089 deployment is unchanged.
package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// kindOllama is the local-inference provider kind that anchors the free/local
// cost class + the Ollama-first group order. Mirrors
// config.ModelConnectionKindOllama / catalog's kindOllama; duplicated as a local
// const so this presentation file carries no config import (same pattern as
// catalog.go).
const kindOllama = "ollama"

// modelPickerReplyCombined renders the spec-096 combined provider-grouped picker
// when a catalog source is wired, else delegates to the byte-for-byte 089 flat
// picker (deferred-activation baseline). Returns the rendered text + the ORDERED
// list of NUMBERED (reachable) model ids so the caller arms the per-chat pending
// selection with EXACTLY the list the user saw (numbered N → models[N-1]).
func modelPickerReplyCombined(ctx context.Context, source agenttool.CatalogProvider, budget agenttool.BudgetProvider, allow *modelswitch.Allowlist, store modelpref.Store, userID string) (string, []string) {
	if source == nil {
		// No combined catalog wired — the 089 flat numbered list, unchanged.
		return modelPickerReply(ctx, allow, store, userID)
	}
	cat, statuses := source.GetCatalog(ctx)
	return renderCombinedPicker(ctx, cat, statuses, budget, allow, store, userID)
}

// renderCombinedPicker is the pure (no Telegram I/O) combined-picker renderer,
// so it is directly table-testable. It groups the catalog by provider
// (Ollama/local FIRST), numbers ONLY reachable models, shows an unreachable
// provider but does NOT number it, preserves the `current` / `system default`
// tags, and (when a paid model is in the catalog and a budget source is wired)
// shows a compact month-to-date budget line. The returned []string is the flat
// ordered set of numbered (reachable) ids.
func renderCombinedPicker(ctx context.Context, cat catalog.ModelCatalog, statuses []catalog.ProviderDiscoveryStatus, budget agenttool.BudgetProvider, allow *modelswitch.Allowlist, store modelpref.Store, userID string) (string, []string) {
	systemDefault := allow.DefaultModel()
	effective, source := systemDefault, "system default"
	if pref, ok, err := store.Get(ctx, userID); err == nil && ok && strings.TrimSpace(pref.SynthesisModel) != "" {
		effective, source = pref.SynthesisModel, "your default"
	}

	// Models grouped by their connection (the aggregator emits one status per
	// connection; a reachable connection's models are looked up here).
	modelsByConn := make(map[string][]catalog.ModelDescriptor, len(statuses))
	hasPaid := false
	for _, m := range cat.Models {
		modelsByConn[m.ConnectionID] = append(modelsByConn[m.ConnectionID], m)
		if m.Kind != kindOllama {
			hasPaid = true
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Your /ask model: %s (%s)\n", effective, source)
	// Budget line — only when a paid model is in the catalog AND a budget
	// source is wired (Principle 6 pull-not-push: no nag when free/local only).
	if hasPaid && budget != nil {
		if perUser, _, err := budget.MonthToDateSpend(ctx); err == nil {
			fmt.Fprintf(&b, "Budget: $%.2f used this month\n", perUser)
		}
	}
	b.WriteString("Choose a model by replying with its number:\n")

	// Ollama/local group FIRST, then the rest — independent of declaration
	// order so the binding "Ollama first" UX holds (spec.md §UX Specification).
	ordered := orderOllamaFirst(statuses)

	numbered := make([]string, 0, len(cat.Models))
	for _, st := range ordered {
		b.WriteString("\n")
		if st.State == catalog.StateOK {
			fmt.Fprintf(&b, "— %s · %s —\n", displayKind(st.Kind), costClass(st.Kind))
			for _, m := range modelsByConn[st.ConnectionID] {
				numbered = append(numbered, m.ID)
				fmt.Fprintf(&b, "  %d. %s%s%s\n", len(numbered), m.ID, modelTags(m.ID, effective, systemDefault), capabilityMarker(m))
			}
			continue
		}
		// Unreachable / timed-out / auth-failed / disabled provider: SHOWN with
		// its typed status, but NOT numbered (so a reply number never maps to a
		// non-dispatchable model). Never silently dropped (Principle 8).
		fmt.Fprintf(&b, "— %s · %s · %s —\n", displayKind(st.Kind), costClass(st.Kind), statusWord(st.State))
		b.WriteString("  temporarily unavailable — operator must re-test this connection\n")
		b.WriteString("  (its models are hidden from selection until it recovers; not silently dropped)\n")
	}

	if len(numbered) == 0 {
		b.WriteString("\nNo models are currently reachable. Run /model default to reset.")
		return b.String(), numbered
	}
	fmt.Fprintf(&b, "\nReply with 1-%d, or /model default to reset.", len(numbered))
	return b.String(), numbered
}

// orderOllamaFirst returns the statuses with every ollama-kind group first (in
// their original order), then every other group (in their original order). The
// binding UX is "Ollama/local group first" regardless of SST declaration order.
func orderOllamaFirst(statuses []catalog.ProviderDiscoveryStatus) []catalog.ProviderDiscoveryStatus {
	out := make([]catalog.ProviderDiscoveryStatus, 0, len(statuses))
	for _, st := range statuses {
		if st.Kind == kindOllama {
			out = append(out, st)
		}
	}
	for _, st := range statuses {
		if st.Kind != kindOllama {
			out = append(out, st)
		}
	}
	return out
}

// modelTags renders the verbatim 089 ` (current · system default)` suffix: a
// model that is the caller's effective selection is `current`, the SST default
// is `system default`, a model that is both carries `current · system default`.
func modelTags(id, effective, systemDefault string) string {
	tags := make([]string, 0, 2)
	if id == effective {
		tags = append(tags, "current")
	}
	if id == systemDefault {
		tags = append(tags, "system default")
	}
	if len(tags) == 0 {
		return ""
	}
	return " (" + strings.Join(tags, " · ") + ")"
}

// capabilityMarker is the single phone-screen-fit capability marker (Principle
// 7): tool-capable models carry ` 🔧 tools` (relevant to the 089 gather turn);
// everything else carries none.
func capabilityMarker(m catalog.ModelDescriptor) string {
	if m.ToolCapable {
		return " 🔧 tools"
	}
	return ""
}

// costClass is the per-group cost hint: ollama/local is free, every hosted
// provider is paid (the per-model rate lives in SCOPE-05 model_costs).
func costClass(kind string) string {
	if kind == kindOllama {
		return "local · free"
	}
	return "paid"
}

// statusWord maps a non-ok DiscoveryState to its text-and-glyph reachability
// label (never color-only — Telegram is plain text). Closed set.
func statusWord(state catalog.DiscoveryState) string {
	switch state {
	case catalog.StateUnreachable:
		return "⚠ unreachable"
	case catalog.StateTimeout:
		return "⚠ slow / timed out"
	case catalog.StateAuthFailed:
		return "⚠ auth failed"
	case catalog.StateDisabled:
		return "○ disabled"
	default:
		return "⚠ unavailable"
	}
}

// displayKind renders the operator-friendly provider name for the closed set of
// provider kinds (spec.md §UX Specification voice). A future/unknown kind falls
// back to its raw slug capitalized.
func displayKind(kind string) string {
	switch kind {
	case kindOllama:
		return "Ollama"
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "azure-foundry":
		return "Microsoft Foundry / Azure"
	case "google":
		return "Google"
	case "bedrock":
		return "Amazon Bedrock"
	case "":
		return "Provider"
	default:
		return strings.ToUpper(kind[:1]) + kind[1:]
	}
}

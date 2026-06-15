// Spec 089 (Fork B/C) — the Telegram /model set/show/reset affordance.
//
// /model is a claim-bound per-user CRUD + discovery affordance, NOT an agent
// run: it does NOT flow through the assistant adapter / facade. The actor is
// resolved via resolveActorUserID (the production chat→user claim guard); the
// sticky synthesis preference is read/written through the shared
// agenttool.ModelPref() store and validated against the shared
// agenttool.SwitchableModels() allowlist — the SAME store + validator the HTTP
// GET/PUT/DELETE /v1/agent/model surface uses, so both surfaces render the SAME
// result from ONE validator + ONE store (SCN-089-A11 parity).
package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// handleModelCommand resolves the claim-bound actor and renders the /model
// reply via the shared validator + store. A nil store/allowlist (capability not
// wired) yields a clear "not enabled" notice, never a panic.
func (b *Bot) handleModelCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID, err := b.resolveActorUserID(chatID)
	if err != nil || userID == "" {
		b.reply(chatID, "couldn't resolve your user identity for /model.")
		return
	}
	allow := agenttool.SwitchableModels()
	store := agenttool.ModelPref()
	if allow == nil || store == nil {
		b.reply(chatID, "runtime model selection is not enabled in this install.")
		return
	}
	arg := strings.TrimSpace(msg.CommandArguments())
	if arg == "" {
		// Spec 089 SCOPE-05 — `/model` (no arg) renders the NUMBERED picker AND
		// arms a per-chat pending selection so a bare-number reply selects
		// (handleModelSelectionReply). The explicit-id set / reset paths below
		// are unchanged (the spec-089 `/model <id>` / `/model default` flow).
		text, models := modelPickerReply(ctx, allow, store, userID)
		b.modelSelections.set(chatID, &pendingModelSelection{Models: models})
		b.reply(chatID, text)
		return
	}
	b.reply(chatID, modelCommandReply(ctx, allow, store, userID, arg))
}

// modelCommandReply is the pure /model set/show/reset renderer (no Telegram I/O,
// so it is directly table-testable). arg is the trimmed text after `/model`:
//   - ""                     ⇒ SHOW: effective + allowed switchable set + system
//     default, tagged "your default" (sticky-set) or "system default" (inherited).
//   - "default" / "reset"    ⇒ RESET: clear the sticky preference, confirm the
//     revert to the SST default.
//   - "<id>"                 ⇒ SET: validate via the shared allowlist; an
//     off-allowlist id renders the verbatim rejection and leaves the existing
//     preference UNCHANGED (the failed set is a no-op).
func modelCommandReply(ctx context.Context, allow *modelswitch.Allowlist, store modelpref.Store, userID, arg string) string {
	switch {
	case arg == "":
		return modelShowReply(ctx, allow, store, userID)
	case strings.EqualFold(arg, "default") || strings.EqualFold(arg, "reset"):
		if err := store.Clear(ctx, userID); err != nil {
			return "couldn't reset your model preference; try again."
		}
		return fmt.Sprintf("Your /ask synthesis model is reset to the system default (%s).", allow.DefaultModel())
	default:
		ov, rej := allow.Resolve(arg)
		if rej != nil {
			// Off-allowlist: render the verbatim shared rejection; the existing
			// sticky preference is UNCHANGED (no Set call).
			return rej.Message
		}
		if err := store.Set(ctx, userID, ov.SynthesisModel); err != nil {
			return "couldn't save your model preference; try again."
		}
		return fmt.Sprintf("Your /ask synthesis model is set to %s (your default). It applies to every /ask until you change it or run /model default.", ov.SynthesisModel)
	}
}

// modelShowReply renders the discovery view: the caller's effective model with
// its source tag, the full switchable set, and the system default.
func modelShowReply(ctx context.Context, allow *modelswitch.Allowlist, store modelpref.Store, userID string) string {
	systemDefault := allow.DefaultModel()
	effective, source := systemDefault, "system default"
	if pref, ok, err := store.Get(ctx, userID); err == nil && ok && strings.TrimSpace(pref.SynthesisModel) != "" {
		effective, source = pref.SynthesisModel, "your default"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Your /ask synthesis model: %s (%s)\n", effective, source)
	fmt.Fprintf(&b, "System default: %s\n", systemDefault)
	fmt.Fprintf(&b, "Switchable: %s\n", strings.Join(allow.AllowedModels(), ", "))
	b.WriteString("Set with /model <id> · reset with /model default · per-question with /ask --model=<id>")
	return b.String()
}

// modelPickerReply renders the NUMBERED picker shown by `/model` with no
// argument (spec 089 SCOPE-05) AND returns the ORDERED model-id list in display
// order so the caller can arm a per-chat pending selection with the EXACT list
// the user saw. Pure (no Telegram I/O) so it is table-testable. The display
// order is allow.AllowedModels() (the operator-curated switchable order —
// stable), so the printed number N always maps to the returned models[N-1]. The
// caller's effective model is tagged "current" and the SST default "system
// default" (a model that is both carries "current · system default").
func modelPickerReply(ctx context.Context, allow *modelswitch.Allowlist, store modelpref.Store, userID string) (string, []string) {
	models := allow.AllowedModels()
	systemDefault := allow.DefaultModel()
	effective, source := systemDefault, "system default"
	if pref, ok, err := store.Get(ctx, userID); err == nil && ok && strings.TrimSpace(pref.SynthesisModel) != "" {
		effective, source = pref.SynthesisModel, "your default"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Your /ask synthesis model: %s (%s)\n", effective, source)
	b.WriteString("Choose a model by replying with its number:\n")
	for i, m := range models {
		tags := make([]string, 0, 2)
		if m == effective {
			tags = append(tags, "current")
		}
		if m == systemDefault {
			tags = append(tags, "system default")
		}
		if len(tags) > 0 {
			fmt.Fprintf(&b, "  %d. %s (%s)\n", i+1, m, strings.Join(tags, " · "))
		} else {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, m)
		}
	}
	fmt.Fprintf(&b, "Reply with 1-%d, or /model default to reset.", len(models))
	return b.String(), models
}

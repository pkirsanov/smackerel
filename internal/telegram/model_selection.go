// Spec 089 SCOPE-05 — the Telegram `/model` NUMBERED PICKER selection flow.
//
// `/model` (no arg) renders the switchable models as a 1-indexed numbered list
// AND arms a per-chat pending selection (the ORDERED id list shown). A bare
// number reply then selects the corresponding model and writes the sticky
// synthesis preference — CLAIM-BOUND to resolveActorUserID(chatID), re-validated
// against the SAME modelswitch allowlist + written through the SAME modelpref
// store the spec-089 `/model <id>` path and the HTTP /v1/agent/model surface use
// (SCN-089-A11 parity). It is NOT an agent run.
//
// The pending store mirrors disambiguationStore (a thread-safe per-chat
// in-memory store with a TTL); the reply resolver mirrors
// handleDisambiguationReply (catch a bare-number reply, bounds-check, resolve,
// clear). The resolver returns false (falls through) unless THIS chat has an
// armed, unexpired pending selection AND the text is a number, so it never
// swallows a number meant for another numeric-reply flow (annotation/cook
// disambiguation, servings) or an ordinary message.
package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
)

// pendingModelSelection is a per-chat armed numbered `/model` picker. Models is
// the ORDERED switchable id list EXACTLY as it was shown to the user, so the
// reply number N resolves to Models[N-1] even if the operator later re-curates
// the allowlist. ExpiresAt bounds the window so a stale number falls through
// instead of resolving a different model.
type pendingModelSelection struct {
	Models    []string
	ExpiresAt time.Time
}

// modelSelectionStore is the thread-safe per-chat store for armed `/model`
// pickers. It mirrors disambiguationStore (sync.Mutex, set/get/clear, TTL) —
// in-memory per-chat runtime state, NOT config (no SST key, G028 N/A).
type modelSelectionStore struct {
	mu      sync.Mutex
	pending map[int64]*pendingModelSelection // keyed by chat_id
	timeout time.Duration
}

func newModelSelectionStore(timeoutSeconds int) *modelSelectionStore {
	return &modelSelectionStore{
		pending: make(map[int64]*pendingModelSelection),
		timeout: time.Duration(timeoutSeconds) * time.Second,
	}
}

func (ms *modelSelectionStore) set(chatID int64, p *pendingModelSelection) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	p.ExpiresAt = time.Now().Add(ms.timeout)
	ms.pending[chatID] = p
}

func (ms *modelSelectionStore) get(chatID int64) *pendingModelSelection {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	p, ok := ms.pending[chatID]
	if !ok {
		return nil
	}
	if time.Now().After(p.ExpiresAt) {
		delete(ms.pending, chatID)
		return nil
	}
	return p
}

func (ms *modelSelectionStore) clear(chatID int64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.pending, chatID)
}

// handleModelSelectionReply resolves a bare-number reply to an armed `/model`
// numbered picker (SCN-089-A14). It mirrors handleDisambiguationReply: returns
// true ONLY when THIS chat has an armed, unexpired pending selection AND the
// trimmed text is a number — otherwise false (fall through), so it never
// swallows a number meant for another numeric-reply flow (disambiguation, cook,
// servings) or an ordinary message.
//
// On a valid in-range pick it re-validates the chosen id against the SHARED
// modelswitch allowlist (defense-in-depth — the list came from the allowlist,
// but a stale armed list is re-checked) and SETS the sticky synthesis
// preference CLAIM-BOUND to resolveActorUserID(chatID) — NEVER a body/text-
// supplied id (OWASP A01, the spec-089 #1 invariant). The same store + the same
// validator the `/model <id>` path and the HTTP /v1/agent/model surface use.
func (b *Bot) handleModelSelectionReply(ctx context.Context, msg *tgbotapi.Message) bool {
	if b.modelSelections == nil {
		return false
	}
	chatID := msg.Chat.ID
	pending := b.modelSelections.get(chatID)
	if pending == nil {
		return false // no armed picker (or it expired) — fall through to the other flows
	}

	text := strings.TrimSpace(msg.Text)
	choice, err := strconv.Atoi(text)
	if err != nil {
		return false // not a number at all — fall through (never swallow words)
	}
	if choice < 1 || choice > len(pending.Models) {
		// An out-of-range NUMBER against an armed picker is a clear mis-pick.
		// The picker is armed ONLY by an explicit `/model`, so the number is
		// meant for it: re-prompt and KEEP the picker armed (handled = true).
		b.reply(chatID, fmt.Sprintf("Pick a number from 1 to %d, or /model default to reset.", len(pending.Models)))
		return true
	}

	selected := pending.Models[choice-1]

	// Claim-binding boundary: the sticky preference is keyed ONLY on the
	// resolved actor, NEVER on anything supplied in the reply.
	userID, uerr := b.resolveActorUserID(chatID)
	if uerr != nil || userID == "" {
		b.reply(chatID, "couldn't resolve your user identity for /model.")
		b.modelSelections.clear(chatID)
		return true
	}
	allow := agenttool.SwitchableModels()
	store := agenttool.ModelPref()
	if allow == nil || store == nil {
		b.reply(chatID, "runtime model selection is not enabled in this install.")
		b.modelSelections.clear(chatID)
		return true
	}
	// Defense-in-depth: re-validate the picked id against the shared allowlist.
	// An off-allowlist id (e.g. a stale armed list) renders the verbatim shared
	// rejection and SETS NOTHING — no arbitrary model string reaches the backend.
	ov, rej := allow.Resolve(selected)
	if rej != nil {
		b.reply(chatID, rej.Message)
		b.modelSelections.clear(chatID)
		return true
	}
	if err := store.Set(ctx, userID, ov.SynthesisModel); err != nil {
		b.reply(chatID, "couldn't save your model preference; try again.")
		return true
	}
	b.modelSelections.clear(chatID)
	b.reply(chatID, fmt.Sprintf("Your /ask synthesis model is set to %s (your default). It applies to every /ask until you change it or run /model default.", ov.SynthesisModel))
	return true
}

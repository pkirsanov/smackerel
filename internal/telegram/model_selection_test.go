// Spec 089 SCOPE-05 — tests for the Telegram `/model` NUMBERED PICKER flow:
// the pure numbered renderer (modelPickerReply), the per-chat pending store
// (modelSelectionStore), and the bare-number reply resolver
// (handleModelSelectionReply). The resolver tests install a real
// modelswitch.Allowlist + a fake claim-bound modelpref.Store into the agenttool
// singletons (the SAME validator + store the /model <id> path + the HTTP
// surface read) and drive a Bot with a replyFunc hook + a userMapping, so the
// claim-binding, re-validation, bounds, and don't-hijack invariants are proven
// without Telegram I/O. SCN-089-A14.
package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
)

// ---- modelPickerReply (pure numbered renderer) -----------------------------

// TestModelPickerReply_NumberedListMarksCurrentAndDefault_Spec089 — `/model`
// (no arg) renders a 1-indexed numbered list in the stable switchable order,
// tags the caller's effective model "current" and the SST default "system
// default" (both on a model that is both), and returns the ordered id list so
// the printed number N maps to models[N-1].
func TestModelPickerReply_NumberedListMarksCurrentAndDefault_Spec089(t *testing.T) {
	allow := spec089ModelAllowlist(t)
	ctx := context.Background()

	t.Run("inherited_default_is_current_and_system_default", func(t *testing.T) {
		store := &fakeModelPrefStore{}
		text, models := modelPickerReply(ctx, allow, store, "user-A")
		// Ordered id list mirrors the switchable order, number N -> models[N-1].
		want := []string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"}
		if len(models) != len(want) {
			t.Fatalf("returned model order = %v, want %v", models, want)
		}
		for i := range want {
			if models[i] != want[i] {
				t.Fatalf("returned model order = %v, want %v", models, want)
			}
		}
		for _, sub := range []string{
			"Your /ask synthesis model: deepseek-r1:32b (system default)",
			"Choose a model by replying with its number:",
			"  1. deepseek-r1:32b (current · system default)",
			"  2. deepseek-r1:7b",
			"  3. gemma4:26b",
			"Reply with 1-3, or /model default to reset.",
		} {
			if !strings.Contains(text, sub) {
				t.Fatalf("picker MUST contain %q, got:\n%s", sub, text)
			}
		}
	})

	t.Run("sticky_user_marks_current_separate_from_system_default", func(t *testing.T) {
		store := &fakeModelPrefStore{}
		if err := store.Set(ctx, "user-A", "deepseek-r1:7b"); err != nil {
			t.Fatalf("seed Set: %v", err)
		}
		text, _ := modelPickerReply(ctx, allow, store, "user-A")
		for _, sub := range []string{
			"Your /ask synthesis model: deepseek-r1:7b (your default)",
			"  1. deepseek-r1:32b (system default)",
			"  2. deepseek-r1:7b (current)",
			"  3. gemma4:26b",
		} {
			if !strings.Contains(text, sub) {
				t.Fatalf("sticky picker MUST contain %q, got:\n%s", sub, text)
			}
		}
		// The sticky model is NOT also tagged "system default".
		if strings.Contains(text, "deepseek-r1:7b (current · system default)") {
			t.Fatalf("a non-default sticky model MUST NOT be tagged system default, got:\n%s", text)
		}
	})
}

// ---- modelSelectionStore (per-chat pending store) --------------------------

func TestModelSelectionStore_SetGetClear_Spec089(t *testing.T) {
	ms := newModelSelectionStore(120)

	ms.set(5555, &pendingModelSelection{Models: []string{"deepseek-r1:32b", "deepseek-r1:7b"}})

	got := ms.get(5555)
	if got == nil {
		t.Fatal("expected an armed pending selection")
	}
	if len(got.Models) != 2 || got.Models[0] != "deepseek-r1:32b" {
		t.Fatalf("Models = %v, want [deepseek-r1:32b deepseek-r1:7b]", got.Models)
	}

	ms.clear(5555)
	if ms.get(5555) != nil {
		t.Fatal("expected nil after clear")
	}
}

func TestModelSelectionStore_Expiry_Spec089(t *testing.T) {
	ms := newModelSelectionStore(0) // 0s timeout = immediate expiry

	ms.set(5555, &pendingModelSelection{Models: []string{"deepseek-r1:32b"}})

	if got := ms.get(5555); got != nil {
		t.Fatalf("expected nil for an expired pending selection, got %v", got)
	}
}

// ---- handleModelSelectionReply (bare-number reply resolver) ----------------

// spec089SelectionBot installs the shared allowlist + a fake claim-bound store
// into the agenttool singletons (restored on cleanup) and returns a Bot wired
// with a replyFunc capture + a userMapping resolving chatID -> actor.
func spec089SelectionBot(t *testing.T, chatID int64, actor string) (*Bot, *fakeModelPrefStore, *[]string) {
	t.Helper()
	agenttool.SetSwitchableModels(spec089ModelAllowlist(t))
	store := &fakeModelPrefStore{}
	agenttool.SetModelPref(store)
	t.Cleanup(func() {
		agenttool.SetSwitchableModels(nil)
		agenttool.SetModelPref(nil)
	})
	replies := &[]string{}
	bot := &Bot{
		done:            make(chan struct{}),
		modelSelections: newModelSelectionStore(120),
		userMapping:     map[int64]string{chatID: actor},
		environment:     "test",
		replyFunc: func(_ int64, text string) {
			*replies = append(*replies, text)
		},
	}
	return bot, store, replies
}

// TestHandleModelSelectionReply_ValidPickSetsStickyForResolvedActor_Spec089 —
// a valid in-range number selects the corresponding model and SETS the sticky
// preference for the resolved actor, then clears the picker and confirms.
func TestHandleModelSelectionReply_ValidPickSetsStickyForResolvedActor_Spec089(t *testing.T) {
	const chatID = int64(7001)
	bot, store, replies := spec089SelectionBot(t, chatID, "user-A")
	bot.modelSelections.set(chatID, &pendingModelSelection{Models: []string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"}})

	msg := &tgbotapi.Message{Text: "2", Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: 4242}}
	if !bot.handleModelSelectionReply(context.Background(), msg) {
		t.Fatal("a valid in-range number against an armed picker MUST be handled (true)")
	}
	if pref, ok, _ := store.Get(context.Background(), "user-A"); !ok || pref.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("pick #2 MUST set the resolved actor's pref to deepseek-r1:7b; got ok=%v pref=%+v", ok, pref)
	}
	if len(*replies) != 1 || !strings.Contains((*replies)[0], "set to deepseek-r1:7b (your default)") {
		t.Fatalf("a valid pick MUST confirm the sticky set, got %v", *replies)
	}
	if bot.modelSelections.get(chatID) != nil {
		t.Fatal("a resolved pick MUST clear the armed picker")
	}
}

// TestHandleModelSelectionReply_ClaimBoundToResolvedActor_Spec089 (ADVERSARIAL)
// — the selection binds to resolveActorUserID(chatID), NEVER to a message-
// supplied id. A reply whose From.ID differs from the resolved actor MUST still
// write ONLY the resolved actor's preference (OWASP A01). Fails if From.ID ever
// becomes the preference key.
func TestHandleModelSelectionReply_ClaimBoundToResolvedActor_Spec089(t *testing.T) {
	const chatID = int64(7011)
	bot, store, _ := spec089SelectionBot(t, chatID, "resolved-actor")
	bot.modelSelections.set(chatID, &pendingModelSelection{Models: []string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"}})

	// The reply carries a DIFFERENT From.ID — the spoof surface.
	msg := &tgbotapi.Message{Text: "2", Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: 999999}}
	if !bot.handleModelSelectionReply(context.Background(), msg) {
		t.Fatal("a valid in-range number MUST be handled")
	}
	if pref, ok, _ := store.Get(context.Background(), "resolved-actor"); !ok || pref.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("the selection MUST bind to the resolved actor; got ok=%v pref=%+v", ok, pref)
	}
	if _, ok, _ := store.Get(context.Background(), "999999"); ok {
		t.Fatal("CLAIM-BINDING BREACH: the message From.ID was used as the preference key")
	}
}

// TestHandleModelSelectionReply_OffAllowlistStalePending_RejectsPrefUnchanged_Spec089
// (ADVERSARIAL) — a picked id that is NOT on the shared allowlist (simulating a
// stale armed list rendered before the operator re-curated the allowlist) is
// re-validated and refused fail-loud; the existing preference is UNCHANGED and
// nothing reaches the backend. Fails if an off-allowlist id is set.
func TestHandleModelSelectionReply_OffAllowlistStalePending_RejectsPrefUnchanged_Spec089(t *testing.T) {
	const chatID = int64(7012)
	bot, store, replies := spec089SelectionBot(t, chatID, "user-A")
	if err := store.Set(context.Background(), "user-A", "deepseek-r1:7b"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	// Arm a STALE picker whose entry is off the current allowlist.
	bot.modelSelections.set(chatID, &pendingModelSelection{Models: []string{"gpt-4o-stale"}})

	msg := &tgbotapi.Message{Text: "1", Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: 4242}}
	if !bot.handleModelSelectionReply(context.Background(), msg) {
		t.Fatal("an off-allowlist pick is still handled (rejection reply)")
	}
	if len(*replies) != 1 || !strings.Contains((*replies)[0], "not a switchable model") {
		t.Fatalf("off-allowlist pick MUST render the verbatim shared rejection, got %v", *replies)
	}
	if pref, ok, _ := store.Get(context.Background(), "user-A"); !ok || pref.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("a rejected pick MUST NOT change the pref; got ok=%v pref=%+v", ok, pref)
	}
}

// TestHandleModelSelectionReply_OutOfRange_RepromptsPrefUnchanged_Spec089
// (ADVERSARIAL) — an out-of-range NUMBER against an armed picker re-prompts with
// the valid range, sets NO preference, and keeps the picker armed for a retry.
// Fails if an out-of-range index is dereferenced or a pref is written.
func TestHandleModelSelectionReply_OutOfRange_RepromptsPrefUnchanged_Spec089(t *testing.T) {
	const chatID = int64(7013)
	bot, store, replies := spec089SelectionBot(t, chatID, "user-A")
	bot.modelSelections.set(chatID, &pendingModelSelection{Models: []string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"}})

	msg := &tgbotapi.Message{Text: "9", Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: 4242}}
	if !bot.handleModelSelectionReply(context.Background(), msg) {
		t.Fatal("an out-of-range number against an armed picker MUST be handled (re-prompt)")
	}
	if len(*replies) != 1 || !strings.Contains((*replies)[0], "Pick a number from 1 to 3") {
		t.Fatalf("out-of-range MUST re-prompt with the valid range, got %v", *replies)
	}
	if _, ok, _ := store.Get(context.Background(), "user-A"); ok {
		t.Fatal("an out-of-range pick MUST NOT set any preference")
	}
	if bot.modelSelections.get(chatID) == nil {
		t.Fatal("an out-of-range pick MUST keep the picker armed for a retry")
	}
}

// TestHandleModelSelectionReply_NoArmedPicker_FallsThrough_Spec089 (ADVERSARIAL)
// — a bare number with NO armed pending selection for this chat MUST fall
// through (return false), so it never hijacks a number meant for another
// numeric-reply flow (annotation/cook disambiguation, servings). Fails if an
// unrelated number is swallowed.
func TestHandleModelSelectionReply_NoArmedPicker_FallsThrough_Spec089(t *testing.T) {
	const chatID = int64(7014)
	bot, store, replies := spec089SelectionBot(t, chatID, "user-A")
	// No picker armed for this chat.

	msg := &tgbotapi.Message{Text: "2", Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: 4242}}
	if bot.handleModelSelectionReply(context.Background(), msg) {
		t.Fatal("a bare number with NO armed picker MUST fall through (return false), not be swallowed")
	}
	if len(*replies) != 0 {
		t.Fatalf("an un-armed fall-through MUST NOT reply, got %v", *replies)
	}
	if _, ok, _ := store.Get(context.Background(), "user-A"); ok {
		t.Fatal("an un-armed fall-through MUST NOT set any preference")
	}
}

// TestHandleModelSelectionReply_NonNumberReply_FallsThrough_Spec089 — a non-
// number reply against an armed picker falls through (returns false) so an
// ordinary message / question is never swallowed by the picker.
func TestHandleModelSelectionReply_NonNumberReply_FallsThrough_Spec089(t *testing.T) {
	const chatID = int64(7015)
	bot, _, replies := spec089SelectionBot(t, chatID, "user-A")
	bot.modelSelections.set(chatID, &pendingModelSelection{Models: []string{"deepseek-r1:32b", "deepseek-r1:7b"}})

	msg := &tgbotapi.Message{Text: "what is the capital of France?", Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: 4242}}
	if bot.handleModelSelectionReply(context.Background(), msg) {
		t.Fatal("a non-number reply MUST fall through (return false), never be swallowed by the picker")
	}
	if len(*replies) != 0 {
		t.Fatalf("a non-number fall-through MUST NOT reply, got %v", *replies)
	}
}

// TestHandleModelSelectionReply_ExpiredPending_FallsThrough_Spec089
// (ADVERSARIAL) — once the pending selection has expired, a bare number falls
// through (return false) and is NOT resolved as a selection. Fails if an expired
// picker still resolves a number.
func TestHandleModelSelectionReply_ExpiredPending_FallsThrough_Spec089(t *testing.T) {
	const chatID = int64(7016)
	bot, store, replies := spec089SelectionBot(t, chatID, "user-A")
	// Directly install an already-expired pending selection (deterministic).
	bot.modelSelections.pending[chatID] = &pendingModelSelection{
		Models:    []string{"deepseek-r1:32b", "deepseek-r1:7b"},
		ExpiresAt: time.Now().Add(-time.Hour),
	}

	msg := &tgbotapi.Message{Text: "1", Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: 4242}}
	if bot.handleModelSelectionReply(context.Background(), msg) {
		t.Fatal("an expired pending selection MUST NOT resolve a number — it MUST fall through")
	}
	if len(*replies) != 0 {
		t.Fatalf("an expired fall-through MUST NOT reply, got %v", *replies)
	}
	if _, ok, _ := store.Get(context.Background(), "user-A"); ok {
		t.Fatal("an expired pick MUST NOT set any preference")
	}
}

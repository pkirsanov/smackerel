package assistant_adapter

import (
	"errors"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// fixedResolver returns the canonical (userID, nil) for any
// non-zero chat_id, simulating a successful spec 044 mapping.
func fixedResolver(userID string) UserResolver {
	return func(chatID int64) (string, error) {
		if chatID == 0 {
			return "", errors.New("zero chat_id")
		}
		return userID, nil
	}
}

// rejectResolver simulates the production spec 044 contract for an
// unmapped chat: returns (empty, non-nil error).
func rejectResolver(sentinel error) UserResolver {
	return func(chatID int64) (string, error) {
		return "", sentinel
	}
}

func updateWithText(chatID int64, msgID int, text string) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: msgID,
			Chat:      &tgbotapi.Chat{ID: chatID},
			Text:      text,
		},
	}
}

func updateWithCallback(chatID int64, msgID int, data string) *tgbotapi.Update {
	return &tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb-1",
			Data: data,
			Message: &tgbotapi.Message{
				MessageID: msgID,
				Chat:      &tgbotapi.Chat{ID: chatID},
			},
		},
	}
}

// TestTranslateInbound_PlainText is the canonical happy path:
// a free-form text message resolves to KindText with the bot's
// resolved user_id and "telegram" transport.
func TestTranslateInbound_PlainText(t *testing.T) {
	t.Parallel()
	update := updateWithText(123, 42, "what's the weather like?")
	msg, err := translateInbound(update, fixedResolver("user-abc"))
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if msg.Kind != contracts.KindText {
		t.Errorf("Kind = %v; want KindText", msg.Kind)
	}
	if msg.UserID != "user-abc" {
		t.Errorf("UserID = %q; want user-abc", msg.UserID)
	}
	if msg.Transport != "telegram" {
		t.Errorf("Transport = %q; want telegram", msg.Transport)
	}
	if msg.Text != "what's the weather like?" {
		t.Errorf("Text = %q; want round-trip", msg.Text)
	}
	if msg.TransportMessageID != "42" {
		t.Errorf("TransportMessageID = %q; want 42", msg.TransportMessageID)
	}
	if msg.ReceivedAt.IsZero() {
		t.Error("ReceivedAt is zero; want non-zero")
	}
}

// TestTranslateInbound_Reset asserts /reset translates to KindReset
// (case-insensitive) so the capability layer can drop pending
// confirm/disambig state.
func TestTranslateInbound_Reset(t *testing.T) {
	t.Parallel()
	tests := []string{"/reset", "/RESET", "/Reset", "/reset now"}
	for _, in := range tests {
		update := updateWithText(123, 7, in)
		msg, err := translateInbound(update, fixedResolver("user-abc"))
		if err != nil {
			t.Errorf("translate(%q) error = %v; want nil", in, err)
			continue
		}
		if msg.Kind != contracts.KindReset {
			t.Errorf("translate(%q).Kind = %v; want KindReset", in, msg.Kind)
		}
	}
}

// TestTranslateInbound_OtherSlash asserts non-/reset slash commands
// return ErrNotAssistantMessage so the bot's existing /find, /list,
// /watch, etc. handlers run unchanged.
func TestTranslateInbound_OtherSlash(t *testing.T) {
	t.Parallel()
	tests := []string{"/find x", "/list", "/watch foo", "/help"}
	for _, in := range tests {
		update := updateWithText(123, 9, in)
		_, err := translateInbound(update, fixedResolver("user-abc"))
		if err != ErrNotAssistantMessage {
			t.Errorf("translate(%q) err = %v; want ErrNotAssistantMessage", in, err)
		}
	}
}

// TestTranslateInbound_SlashShortcuts asserts that v1 slash shortcuts
// (/ask, /weather, /remind — Spec 061 SCOPE-06 Round 49) are forwarded
// to the facade as KindText with the slash preserved so the facade's
// LookupShortcut pre-check can stamp the explicit ScenarioID. An
// unknown slash command (adversarial) MUST still return
// ErrNotAssistantMessage.
func TestTranslateInbound_SlashShortcuts(t *testing.T) {
	t.Parallel()
	shortcuts := []string{
		"/ask what's in my notes about Foo?",
		"/weather in Paris tomorrow",
		"/remind me to call Mom at 6pm",
		"/ask",
		"/weather",
		"/remind",
	}
	for _, in := range shortcuts {
		update := updateWithText(123, 13, in)
		msg, err := translateInbound(update, fixedResolver("user-abc"))
		if err != nil {
			t.Errorf("translate(%q) err = %v; want nil", in, err)
			continue
		}
		if msg.Kind != contracts.KindText {
			t.Errorf("translate(%q).Kind = %v; want KindText (facade owns shortcut routing)", in, msg.Kind)
		}
		if msg.Text != in {
			t.Errorf("translate(%q).Text = %q; want round-trip with slash preserved", in, msg.Text)
		}
	}

	// Adversarial: an unknown slash command MUST NOT be claimed by the
	// adapter (would steal it from the bot's legacy handlers).
	update := updateWithText(123, 14, "/notarealcommand foo")
	if _, err := translateInbound(update, fixedResolver("user-abc")); err != ErrNotAssistantMessage {
		t.Errorf("translate(/notarealcommand) err = %v; want ErrNotAssistantMessage", err)
	}
}

// TestTranslateInbound_EmptyText returns the fallthrough sentinel
// so the bot's other media-type handlers (voice, photo, etc.) run.
func TestTranslateInbound_EmptyText(t *testing.T) {
	t.Parallel()
	update := updateWithText(123, 11, "")
	_, err := translateInbound(update, fixedResolver("user-abc"))
	if err != ErrNotAssistantMessage {
		t.Errorf("err = %v; want ErrNotAssistantMessage", err)
	}
}

// TestTranslateInbound_UnmappedChatPropagatesError asserts the spec
// 044 production contract: an unmapped chat MUST surface the resolver
// error verbatim and MUST NOT produce an AssistantMessage. The bot
// drops the message; the capability layer never sees it.
func TestTranslateInbound_UnmappedChatPropagatesError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("no user mapping for chat_id")
	update := updateWithText(123, 11, "hello")
	msg, err := translateInbound(update, rejectResolver(sentinel))
	if err == nil {
		t.Fatalf("err = nil; want non-nil")
	}
	if err != sentinel {
		t.Errorf("err = %v; want propagated sentinel", err)
	}
	if msg.UserID != "" {
		t.Errorf("UserID = %q; want empty on unmapped chat", msg.UserID)
	}
}

// TestTranslateInbound_CallbackConfirm exercises the confirm-card
// callback path end-to-end through translateCallback +
// decodeCallbackData.
func TestTranslateInbound_CallbackConfirm(t *testing.T) {
	t.Parallel()
	data := encodeConfirmCallback("01HCONFIRMREFABC", contracts.ConfirmPositive)
	update := updateWithCallback(123, 17, data)
	msg, err := translateInbound(update, fixedResolver("user-abc"))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if msg.Kind != contracts.KindConfirm {
		t.Errorf("Kind = %v; want KindConfirm", msg.Kind)
	}
	if msg.ConfirmRef != "01HCONFIRMREFABC" {
		t.Errorf("ConfirmRef = %q; want round-trip", msg.ConfirmRef)
	}
	if msg.ConfirmChoice != contracts.ConfirmPositive {
		t.Errorf("ConfirmChoice = %v; want positive", msg.ConfirmChoice)
	}
}

// TestTranslateInbound_CallbackDisambig exercises the disambiguation
// callback path.
func TestTranslateInbound_CallbackDisambig(t *testing.T) {
	t.Parallel()
	data := encodeDisambigCallback("01HDISAMBIG12345", 2)
	update := updateWithCallback(123, 17, data)
	msg, err := translateInbound(update, fixedResolver("user-abc"))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if msg.Kind != contracts.KindDisambiguation {
		t.Errorf("Kind = %v; want KindDisambiguation", msg.Kind)
	}
	if msg.DisambiguationRef != "01HDISAMBIG12345" {
		t.Errorf("DisambiguationRef = %q; want round-trip", msg.DisambiguationRef)
	}
	if msg.DisambiguationChoice != 2 {
		t.Errorf("DisambiguationChoice = %d; want 2", msg.DisambiguationChoice)
	}
}

// TestTranslateInbound_ModelFlagParsedAndStripped_SlashPreserved_Spec088 —
// ADVERSARIAL. `/ask --model=<id> <q>` parses the model into ModelOverride and
// strips ONLY the --model= token from Text (slash prefix preserved); a bare
// `/ask <q>` yields an empty ModelOverride; a non-/ask shortcut keeps its text
// verbatim (the flag is /ask-only). Fails if the flag leaks into the question
// or the slash is dropped.
func TestTranslateInbound_ModelFlagParsedAndStripped_SlashPreserved_Spec088(t *testing.T) {
	t.Parallel()
	t.Run("flag_parsed_and_stripped", func(t *testing.T) {
		update := updateWithText(123, 42, "/ask --model=deepseek-r1:7b what is a better place to grow pomegranate?")
		msg, err := translateInbound(update, fixedResolver("u"))
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if msg.ModelOverride != "deepseek-r1:7b" {
			t.Fatalf("ModelOverride = %q, want deepseek-r1:7b", msg.ModelOverride)
		}
		if msg.Text != "/ask what is a better place to grow pomegranate?" {
			t.Fatalf("Text = %q, want the clean question with slash preserved and flag removed", msg.Text)
		}
		if strings.Contains(msg.Text, "--model=") {
			t.Fatalf("--model= flag leaked into Text: %q", msg.Text)
		}
		if !strings.HasPrefix(msg.Text, "/ask ") {
			t.Fatalf("slash prefix dropped (facade LookupShortcut depends on it): %q", msg.Text)
		}
	})
	t.Run("bare_ask_no_override", func(t *testing.T) {
		update := updateWithText(123, 42, "/ask what is the capital of france?")
		msg, err := translateInbound(update, fixedResolver("u"))
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if msg.ModelOverride != "" {
			t.Fatalf("bare /ask must have empty ModelOverride, got %q", msg.ModelOverride)
		}
		if msg.Text != "/ask what is the capital of france?" {
			t.Fatalf("bare /ask Text changed: %q", msg.Text)
		}
	})
	t.Run("model_flag_is_ask_only_not_other_shortcuts", func(t *testing.T) {
		// /weather is a shortcut but does NOT support --model=; the flag must
		// stay in the text verbatim (parse scoped to open_knowledge only).
		update := updateWithText(123, 42, "/weather --model=x lisbon")
		msg, err := translateInbound(update, fixedResolver("u"))
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if msg.ModelOverride != "" {
			t.Fatalf("/weather must NOT parse --model=, got %q", msg.ModelOverride)
		}
		if msg.Text != "/weather --model=x lisbon" {
			t.Fatalf("/weather text must be verbatim, got %q", msg.Text)
		}
	})
}

// TestTranslateInbound_GatherModelFlagParsedAndStripped_SlashPreserved_Spec089 —
// ADVERSARIAL. `/ask --gather-model=<g> --model=<s> <q>` parses BOTH flags
// (order-independent) into GatherModelOverride + ModelOverride and strips both
// from Text (slash preserved, clean question); a bare /ask yields both empty.
// Fails if a flag leaks into the question or the slash is dropped.
func TestTranslateInbound_GatherModelFlagParsedAndStripped_SlashPreserved_Spec089(t *testing.T) {
	t.Parallel()
	t.Run("both_flags_parsed_gather_first", func(t *testing.T) {
		update := updateWithText(123, 42, "/ask --gather-model=llama3.1:8b --model=deepseek-r1:7b which town for pomegranate?")
		msg, err := translateInbound(update, fixedResolver("u"))
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if msg.GatherModelOverride != "llama3.1:8b" {
			t.Fatalf("GatherModelOverride = %q, want llama3.1:8b", msg.GatherModelOverride)
		}
		if msg.ModelOverride != "deepseek-r1:7b" {
			t.Fatalf("ModelOverride = %q, want deepseek-r1:7b", msg.ModelOverride)
		}
		if msg.Text != "/ask which town for pomegranate?" {
			t.Fatalf("Text = %q, want the clean question (both flags stripped, slash preserved)", msg.Text)
		}
		if strings.Contains(msg.Text, "--gather-model=") || strings.Contains(msg.Text, "--model=") {
			t.Fatalf("a flag leaked into Text: %q", msg.Text)
		}
	})
	t.Run("both_flags_parsed_model_first_order_independent", func(t *testing.T) {
		update := updateWithText(123, 42, "/ask --model=deepseek-r1:7b --gather-model=llama3.1:8b q?")
		msg, err := translateInbound(update, fixedResolver("u"))
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if msg.ModelOverride != "deepseek-r1:7b" || msg.GatherModelOverride != "llama3.1:8b" {
			t.Fatalf("order-independent parse failed: model=%q gather=%q", msg.ModelOverride, msg.GatherModelOverride)
		}
		if msg.Text != "/ask q?" {
			t.Fatalf("Text = %q, want '/ask q?'", msg.Text)
		}
	})
	t.Run("gather_only", func(t *testing.T) {
		update := updateWithText(123, 42, "/ask --gather-model=gemma4:26b q?")
		msg, err := translateInbound(update, fixedResolver("u"))
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if msg.GatherModelOverride != "gemma4:26b" || msg.ModelOverride != "" {
			t.Fatalf("gather-only parse failed: model=%q gather=%q", msg.ModelOverride, msg.GatherModelOverride)
		}
	})
	t.Run("bare_ask_both_empty", func(t *testing.T) {
		update := updateWithText(123, 42, "/ask what is the capital of france?")
		msg, err := translateInbound(update, fixedResolver("u"))
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if msg.ModelOverride != "" || msg.GatherModelOverride != "" {
			t.Fatalf("bare /ask must have both overrides empty, got model=%q gather=%q", msg.ModelOverride, msg.GatherModelOverride)
		}
	})
}

// TestTranslateInbound_NilUpdate asserts the adapter refuses a nil
// update rather than panicking.
func TestTranslateInbound_NilUpdate(t *testing.T) {
	t.Parallel()
	_, err := translateInbound(nil, fixedResolver("user-abc"))
	if err == nil {
		t.Fatal("err = nil; want non-nil")
	}
}

// TestStripBotMention covers the group-chat @bot_username stripping
// path that runs before any slash-command classification.
func TestStripBotMention(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"@smackerel_bot /reset", "/reset"},
		{"@smackerel_bot hello there", "hello there"},
		{"/reset", "/reset"},
		{"plain text", "plain text"},
		{"", ""},
		{"  /reset  ", "/reset"},
	}
	for _, tc := range tests {
		got := stripBotMention(tc.in)
		if got != tc.want {
			t.Errorf("stripBotMention(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// fixedTime is helper for any test that wants to assert ReceivedAt
// monotonicity without relying on wall clock.
func fixedTime() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }

//go:build e2e

// Spec 076 SCOPE-7c — TP-076-07c-02 / SCN-073-A05.
//
// Cross-surface render-descriptor parity for the confirm card. The
// facade emits one AssistantResponse with a ConfirmCard; each
// transport projects that response into a canonical render-descriptor
// whose (prompt body, action kind+ref+label) tuples MUST be
// byte-identical across web, Telegram, and WhatsApp.
//
// Sources of truth:
//
//   - Web: tests/fixtures/assistant_response_v1/confirm_accept_decline.input.json
//     + confirm_accept_decline.descriptor.json (JS render CLI golden;
//     proven equivalent to Dart by the cross-language canary).
//   - Telegram: assistant_adapter renders body+ProposedAction with a
//     two-button inline keyboard ("✅ <pos>" / "❌ <neg>") carrying
//     callback_data "a:c:<ref>:<pos|neg>".
//   - WhatsApp: assistant_adapter.Render returns an OutboundMessage
//     whose interactive body == prompt body (plus appended
//     ProposedAction) and whose buttons carry
//     EncodeConfirmPayload(ref, positive) as ID with the canonical
//     label as Title.
//
// The canonical projection extracts the prompt body (first paragraph
// of the rendered body) and the two actions in their natural order
// (accept first, decline second). Adversarial coverage: if any
// transport swapped accept/decline, renamed ref, or mutated the
// canonical label, reflect.DeepEqual against the web descriptor
// projection would trip.

package transports_e2e

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reflect"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	telegramadapter "github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
	whatsappadapter "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

type captureSenderConfirm struct {
	lastText     string
	lastKeyboard *tgbotapi.InlineKeyboardMarkup
}

func (s *captureSenderConfirm) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if m, ok := c.(tgbotapi.MessageConfig); ok {
		s.lastText = m.Text
		if k, ok := m.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup); ok {
			s.lastKeyboard = &k
		}
	}
	return tgbotapi.Message{MessageID: 1}, nil
}

// TestConfirmCardParity_AcrossWebTelegramWhatsApp covers
// TP-076-07c-02 / SCN-073-A05.
func TestConfirmCardParity_AcrossWebTelegramWhatsApp(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	inputPath := filepath.Join(repoRoot, "tests/fixtures/assistant_response_v1/confirm_accept_decline.input.json")
	descriptorPath := filepath.Join(repoRoot, "tests/fixtures/assistant_response_v1/confirm_accept_decline.descriptor.json")

	resp := mustLoadAssistantResponseFromFixture(t, inputPath)
	if resp.ConfirmCard == nil {
		t.Fatalf("fixture %s missing confirm_card", inputPath)
	}

	webRender := mustProjectWebDescriptor(t, descriptorPath)
	tgRender := projectTelegramConfirm(t, resp)
	waRender := projectWhatsAppConfirm(t, resp)

	if !reflect.DeepEqual(webRender, tgRender) {
		t.Fatalf("web vs telegram confirm render mismatch:\nweb=%+v\ntelegram=%+v",
			webRender, tgRender)
	}
	if !reflect.DeepEqual(webRender, waRender) {
		t.Fatalf("web vs whatsapp confirm render mismatch:\nweb=%+v\nwhatsapp=%+v",
			webRender, waRender)
	}
	if !reflect.DeepEqual(tgRender, waRender) {
		t.Fatalf("telegram vs whatsapp confirm render mismatch:\ntelegram=%+v\nwhatsapp=%+v",
			tgRender, waRender)
	}
}

// firstParagraph returns the substring before the first newline in
// body. The Telegram renderer joins (body, ProposedAction) with a
// single "\n" while the WhatsApp renderer uses "\n\n"; splitting on
// the first "\n" peels both transports' canonical prompt body away
// from the appended ProposedAction tail.
func firstParagraph(body string) string {
	if i := strings.Index(body, "\n"); i >= 0 {
		return body[:i]
	}
	return body
}

// stripConfirmEmoji removes the leading "✅ " / "❌ " emoji prefix
// applied by the Telegram confirm-card renderer so the canonical
// label can be compared byte-for-byte across transports.
func stripConfirmEmoji(label string) string {
	for _, p := range []string{"✅ ", "❌ "} {
		if strings.HasPrefix(label, p) {
			return strings.TrimPrefix(label, p)
		}
	}
	return label
}

func projectTelegramConfirm(t *testing.T, resp contracts.AssistantResponse) canonicalRender {
	t.Helper()
	sender := &captureSenderConfirm{}
	adapter, err := telegramadapter.NewAdapter(telegramadapter.Options{
		Sender:          sender,
		Capture:         func(context.Context, *tgbotapi.Message, string) {},
		ResolveUser:     func(int64) (string, error) { return "test-user", nil },
		MarkdownMode:    telegramadapter.PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter(telegram): %v", err)
	}
	if err := adapter.RenderToChat(context.Background(), 12345, resp); err != nil {
		t.Fatalf("Telegram RenderToChat: %v", err)
	}
	if sender.lastKeyboard == nil {
		t.Fatalf("Telegram confirm render produced no inline keyboard")
	}

	r := canonicalRender{PromptBody: firstParagraph(sender.lastText)}
	for _, row := range sender.lastKeyboard.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				continue
			}
			data := *btn.CallbackData
			const prefix = "a:c:"
			if !strings.HasPrefix(data, prefix) {
				t.Fatalf("Telegram callback_data %q lacks confirm prefix %q", data, prefix)
			}
			rest := strings.TrimPrefix(data, prefix)
			idx := strings.LastIndex(rest, ":")
			if idx <= 0 {
				t.Fatalf("Telegram callback_data %q malformed", data)
			}
			ref := rest[:idx]
			suffix := rest[idx+1:]
			var kind string
			switch suffix {
			case "pos":
				kind = "confirm_accept"
			case "neg":
				kind = "confirm_decline"
			default:
				t.Fatalf("Telegram callback_data %q has unknown confirm suffix %q", data, suffix)
			}
			r.Actions = append(r.Actions, canonicalAction{
				Kind:  kind,
				Ref:   ref,
				Label: stripConfirmEmoji(btn.Text),
				Index: 0,
			})
		}
	}
	return r
}

func projectWhatsAppConfirm(t *testing.T, resp contracts.AssistantResponse) canonicalRender {
	t.Helper()
	out, err := whatsappadapter.Render(resp, 4096)
	if err != nil {
		t.Fatalf("WhatsApp Render: %v", err)
	}
	if out.Interactive == nil || out.Interactive.Kind != whatsappadapter.OutboundInteractiveButtons {
		t.Fatalf("WhatsApp confirm render expected interactive_buttons, got kind=%q", out.Kind)
	}
	r := canonicalRender{PromptBody: firstParagraph(out.Interactive.Body)}
	for _, b := range out.Interactive.Buttons {
		ref, positive, ok := whatsappadapter.DecodeConfirmPayload(b.ID)
		if !ok {
			t.Fatalf("WhatsApp button ID %q is not a confirm payload", b.ID)
		}
		kind := "confirm_decline"
		if positive {
			kind = "confirm_accept"
		}
		r.Actions = append(r.Actions, canonicalAction{
			Kind:  kind,
			Ref:   ref,
			Label: b.Title,
			Index: 0,
		})
	}
	return r
}

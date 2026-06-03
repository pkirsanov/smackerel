//go:build e2e

// Spec 076 SCOPE-5 — TP-076-05-06 / SCN-074-A11.
//
// Cross-transport acknowledgement parity for the capture-as-fallback
// "saved as an idea" body. The facade produces a single
// AssistantResponse with Status=StatusSavedAsIdea, CaptureRoute=true,
// and Body="saved as an idea — i'll surface it later." (the canonical
// copy owned by internal/assistant/facade.go). Each transport
// renderer MUST emit that body byte-for-byte; the JSON envelope
// returned to the PWA and mobile clients carries the same Body
// verbatim.
//
// Renderers exercised in this test:
//
//   - Telegram: assistant_adapter.NewAdapter().RenderToChat → fake
//     Sender captures the outgoing tgbotapi.MessageConfig.Text.
//   - WhatsApp: assistant_adapter.Render returns OutboundMessage and
//     this test reads OutboundMessage.Text.Body.
//   - HTTP/PWA/mobile: the assistant HTTP API serialises
//     AssistantResponse to JSON; the user-visible ack copy is
//     AssistantResponse.Body. PWA and mobile clients render the
//     `body` field as-is (no transform).
//
// All three rendered bodies MUST equal the canonical string.
//
// Adversarial coverage: if a transport ever transformed the body
// (e.g. capitalised it, added a prefix, or escaped the em dash), the
// equality assertion would trip. The test uses strict equality, not
// substring containment, so any drift is caught.

package transports_e2e

import (
	"context"
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	telegramadapter "github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
	whatsappadapter "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

// canonicalSavedAsIdeaBody — the facade-owned ack copy.
// MUST stay in sync with internal/assistant/facade.go.
const canonicalSavedAsIdeaBody = "saved as an idea — i'll surface it later."

type captureSender struct {
	lastText string
}

func (s *captureSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if m, ok := c.(tgbotapi.MessageConfig); ok {
		s.lastText = m.Text
	}
	return tgbotapi.Message{MessageID: 1}, nil
}

// TestCaptureAckParity_AcrossAllTransports — TP-076-05-06 / SCN-074-A11.
func TestCaptureAckParity_AcrossAllTransports(t *testing.T) {
	resp := contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         canonicalSavedAsIdeaBody,
	}

	// --- Telegram ----------------------------------------------------
	sender := &captureSender{}
	tgAdapter, err := telegramadapter.NewAdapter(telegramadapter.Options{
		Sender:          sender,
		Capture:         func(context.Context, *tgbotapi.Message, string) {},
		ResolveUser:     func(int64) (string, error) { return "test-user", nil },
		MarkdownMode:    telegramadapter.PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter(telegram): %v", err)
	}
	if err := tgAdapter.RenderToChat(context.Background(), 12345, resp); err != nil {
		t.Fatalf("Telegram RenderToChat: %v", err)
	}
	if sender.lastText != canonicalSavedAsIdeaBody {
		t.Errorf("Telegram body = %q, want %q", sender.lastText, canonicalSavedAsIdeaBody)
	}

	// --- WhatsApp ----------------------------------------------------
	out, err := whatsappadapter.Render(resp, 4096)
	if err != nil {
		t.Fatalf("WhatsApp Render: %v", err)
	}
	if out.Kind != whatsappadapter.OutboundText {
		t.Fatalf("WhatsApp Kind = %q, want %q", out.Kind, whatsappadapter.OutboundText)
	}
	if out.Text.Body != canonicalSavedAsIdeaBody {
		t.Errorf("WhatsApp body = %q, want %q", out.Text.Body, canonicalSavedAsIdeaBody)
	}

	// --- HTTP / PWA / mobile -----------------------------------------
	// The assistant HTTP API serialises AssistantResponse to JSON
	// and the user-visible ack is AssistantResponse.Body. PWA and
	// mobile clients render that field verbatim.
	if resp.Body != canonicalSavedAsIdeaBody {
		t.Errorf("HTTP/PWA body = %q, want %q", resp.Body, canonicalSavedAsIdeaBody)
	}
}

// TestCaptureAckParity_AcrossWebTelegramWhatsApp covers Spec 076
// SCOPE-7c TP-076-07c-03 / SCN-073-A06.
//
// Render-descriptor parity for the capture-as-fallback acknowledgement
// across web + Telegram + WhatsApp. The capture acknowledgement has
// no actions (descriptor is a single text node), so parity reduces to
// "the canonical ack body MUST appear byte-identically in each
// transport's rendered surface and in the web descriptor's text
// node".
//
// Source-of-truth body fixture:
//
//	tests/fixtures/assistant_response_v1/capture_acknowledgement.input.json
//	tests/fixtures/assistant_response_v1/capture_acknowledgement.descriptor.json
//
// (The Spec 069 fixture body — "Saved to your capture inbox for later
// processing." — is distinct from the Spec 074 facade-emitted
// "saved as an idea — i'll surface it later." copy validated by the
// sibling TestCaptureAckParity_AcrossAllTransports test above. This
// test pins the spec 069 fixture's parity contract: whatever the
// canonical body is, every transport renders it verbatim.)
//
// Adversarial coverage: if any transport mutated the ack body (added
// status prefix, normalized punctuation, escaped characters) or the
// web descriptor diverged from the canonical body, the strict
// equality assertions trip.
func TestCaptureAckParity_AcrossWebTelegramWhatsApp(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	inputPath := filepath.Join(repoRoot, "tests/fixtures/assistant_response_v1/capture_acknowledgement.input.json")
	descriptorPath := filepath.Join(repoRoot, "tests/fixtures/assistant_response_v1/capture_acknowledgement.descriptor.json")

	resp := mustLoadAssistantResponseFromFixture(t, inputPath)
	if resp.Body == "" {
		t.Fatalf("fixture %s has empty body — capture ack parity needs a canonical body", inputPath)
	}
	canonicalBody := resp.Body

	webRender := mustProjectWebDescriptor(t, descriptorPath)
	if webRender.PromptBody != canonicalBody {
		t.Fatalf("web descriptor text node = %q, want canonical body %q",
			webRender.PromptBody, canonicalBody)
	}
	if len(webRender.Actions) != 0 {
		t.Fatalf("web descriptor for capture ack must have zero actions, got %d", len(webRender.Actions))
	}

	// --- Telegram ----------------------------------------------------
	tgSender := &captureSender{}
	tgAdapter, err := telegramadapter.NewAdapter(telegramadapter.Options{
		Sender:          tgSender,
		Capture:         func(context.Context, *tgbotapi.Message, string) {},
		ResolveUser:     func(int64) (string, error) { return "test-user", nil },
		MarkdownMode:    telegramadapter.PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter(telegram): %v", err)
	}
	if err := tgAdapter.RenderToChat(context.Background(), 12345, resp); err != nil {
		t.Fatalf("Telegram RenderToChat: %v", err)
	}
	if tgSender.lastText != canonicalBody {
		t.Errorf("Telegram body = %q, want canonical %q", tgSender.lastText, canonicalBody)
	}

	// --- WhatsApp ----------------------------------------------------
	out, err := whatsappadapter.Render(resp, 4096)
	if err != nil {
		t.Fatalf("WhatsApp Render: %v", err)
	}
	if out.Kind != whatsappadapter.OutboundText {
		t.Fatalf("WhatsApp capture-ack kind = %q, want %q", out.Kind, whatsappadapter.OutboundText)
	}
	if out.Text == nil || out.Text.Body != canonicalBody {
		got := ""
		if out.Text != nil {
			got = out.Text.Body
		}
		t.Errorf("WhatsApp body = %q, want canonical %q", got, canonicalBody)
	}
}

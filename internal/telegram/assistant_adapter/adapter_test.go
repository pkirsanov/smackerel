package assistant_adapter

import (
	"context"
	"errors"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// recordingSender captures every outbound message for assertion.
type recordingSender struct {
	mu       sync.Mutex
	messages []tgbotapi.Chattable
	sendErr  error
}

func (r *recordingSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, c)
	if r.sendErr != nil {
		return tgbotapi.Message{}, r.sendErr
	}
	return tgbotapi.Message{MessageID: len(r.messages)}, nil
}

func (r *recordingSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

// noopCapture is a placeholder CaptureFn that records invocations.
type recordingCapture struct {
	mu    sync.Mutex
	calls []capturedCall
}

type capturedCall struct {
	chatID int64
	text   string
}

func (c *recordingCapture) fn() CaptureFn {
	return func(_ context.Context, msg *tgbotapi.Message, text string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		chatID := int64(0)
		if msg != nil && msg.Chat != nil {
			chatID = msg.Chat.ID
		}
		c.calls = append(c.calls, capturedCall{chatID: chatID, text: text})
	}
}

func (c *recordingCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// stubAssistant is a contracts.Assistant fake that returns a
// canned AssistantResponse for every Handle call.
type stubAssistant struct {
	mu       sync.Mutex
	handled  []contracts.AssistantMessage
	response contracts.AssistantResponse
	err      error
}

func (s *stubAssistant) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handled = append(s.handled, msg)
	if s.err != nil {
		return contracts.AssistantResponse{}, s.err
	}
	return s.response, nil
}

// TestNewAdapter_RequiresAllDeps asserts NewAdapter fails-loud when
// any required dependency is missing — the smackerel-no-defaults
// contract for SST-managed runtime config.
func TestNewAdapter_RequiresAllDeps(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := (&recordingCapture{}).fn()
	resolve := fixedResolver("u1")

	tests := []struct {
		name string
		opts Options
	}{
		{"missing sender", Options{Capture: capture, ResolveUser: resolve, MarkdownMode: PlainText, MaxMessageChars: 4096}},
		{"missing capture", Options{Sender: sender, ResolveUser: resolve, MarkdownMode: PlainText, MaxMessageChars: 4096}},
		{"missing resolver", Options{Sender: sender, Capture: capture, MarkdownMode: PlainText, MaxMessageChars: 4096}},
		{"invalid markdown mode", Options{Sender: sender, Capture: capture, ResolveUser: resolve, MarkdownMode: "Markdown1", MaxMessageChars: 4096}},
		{"empty markdown mode", Options{Sender: sender, Capture: capture, ResolveUser: resolve, MaxMessageChars: 4096}},
		{"zero max chars", Options{Sender: sender, Capture: capture, ResolveUser: resolve, MarkdownMode: PlainText, MaxMessageChars: 0}},
		{"negative max chars", Options{Sender: sender, Capture: capture, ResolveUser: resolve, MarkdownMode: PlainText, MaxMessageChars: -1}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewAdapter(tc.opts)
			if err == nil {
				t.Fatalf("NewAdapter(%s) error = nil; want non-nil", tc.name)
			}
		})
	}
}

// TestNewAdapter_HappyPath asserts the constructor returns a usable
// adapter when all dependencies are present.
func TestNewAdapter_HappyPath(t *testing.T) {
	t.Parallel()
	a, err := NewAdapter(Options{
		Sender:          &recordingSender{},
		Capture:         (&recordingCapture{}).fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    MarkdownV2,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter err = %v; want nil", err)
	}
	if a.Name() != "telegram" {
		t.Errorf("Name() = %q; want telegram", a.Name())
	}
	if a.IsBound() {
		t.Error("IsBound() = true before Start; want false")
	}
}

// TestHandleUpdate_NoFacadeReturnsFallthrough asserts the BS-001
// regression-safe contract: when no capability facade is bound,
// HandleUpdate returns (false, nil) so the bot falls through to
// its legacy handleTextCapture path.
func TestHandleUpdate_NoFacadeReturnsFallthrough(t *testing.T) {
	t.Parallel()
	a, err := NewAdapter(Options{
		Sender:          &recordingSender{},
		Capture:         (&recordingCapture{}).fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter err = %v", err)
	}
	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "hello"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if handled {
		t.Error("handled = true; want false (no facade bound)")
	}
}

// TestHandleUpdate_NonAssistantSlashFallsThrough asserts that
// non-/reset slash commands (which translateInbound returns
// ErrNotAssistantMessage for) leave the bot free to run its
// existing handlers. The adapter MUST report handled=false here
// even when the facade IS bound.
func TestHandleUpdate_NonAssistantSlashFallsThrough(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{}
	stub := &stubAssistant{}
	a, _ := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "/find pad thai"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if handled {
		t.Error("handled = true; want false for non-assistant slash")
	}
	if sender.count() != 0 {
		t.Errorf("sender.count() = %d; want 0", sender.count())
	}
	if capture.count() != 0 {
		t.Errorf("capture.count() = %d; want 0", capture.count())
	}
}

// TestHandleUpdate_PlainTextRendersAndDoesNotCapture asserts the
// happy path: facade returns a body with no CaptureRoute, adapter
// sends one outbound message and never invokes the capture hook.
func TestHandleUpdate_PlainTextRendersAndDoesNotCapture(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{}
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status: contracts.StatusCheckingWeather,
			Body:   "in london, it is currently cloudy at 12 degrees C.",
		},
	}
	a, _ := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "weather in london"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if sender.count() != 1 {
		t.Errorf("sender.count() = %d; want 1", sender.count())
	}
	if capture.count() != 0 {
		t.Errorf("capture.count() = %d; want 0", capture.count())
	}
	if len(stub.handled) != 1 || stub.handled[0].Kind != contracts.KindText {
		t.Errorf("stub.handled = %v; want one KindText", stub.handled)
	}
}

// TestHandleUpdate_CaptureRouteInvokesBotHook asserts the BS-001
// regression contract: when CaptureRoute=true, the bot-side
// CaptureFn is invoked with the inbound *tgbotapi.Message + verbatim
// text BEFORE the user-facing send.
func TestHandleUpdate_CaptureRouteInvokesBotHook(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{}
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			// No body — silent capture (renderer short-circuits the send).
		},
	}
	a, _ := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "random thought to save"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if capture.count() != 1 {
		t.Fatalf("capture.count() = %d; want 1", capture.count())
	}
	capture.mu.Lock()
	got := capture.calls[0]
	capture.mu.Unlock()
	if got.chatID != 123 {
		t.Errorf("chatID = %d; want 123", got.chatID)
	}
	if got.text != "random thought to save" {
		t.Errorf("text = %q; want verbatim inbound text", got.text)
	}
	// Silent capture: renderer should short-circuit the Telegram send.
	if sender.count() != 0 {
		t.Errorf("sender.count() = %d; want 0 (silent capture)", sender.count())
	}
}

// TestHandleUpdate_UnmappedChatPropagatesError asserts a spec 044
// production refusal surfaces as a HandleUpdate error (the bot
// drops the message; the capability layer never sees it).
func TestHandleUpdate_UnmappedChatPropagatesError(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{}
	stub := &stubAssistant{}
	sentinel := errors.New("no user mapping")
	a, _ := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     rejectResolver(sentinel),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "hello"))
	if !handled {
		t.Error("handled = false; want true (adapter claimed but failed)")
	}
	if err == nil {
		t.Fatal("err = nil; want non-nil")
	}
	if sender.count() != 0 {
		t.Errorf("sender.count() = %d; want 0", sender.count())
	}
	if len(stub.handled) != 0 {
		t.Errorf("stub.handled = %v; want capability layer never invoked", stub.handled)
	}
}

// TestHandleUpdate_ResetReachesCapabilityLayer asserts /reset
// translates to KindReset and reaches the capability facade.
func TestHandleUpdate_ResetReachesCapabilityLayer(t *testing.T) {
	t.Parallel()
	stub := &stubAssistant{
		response: contracts.AssistantResponse{Status: contracts.StatusSavedAsIdea, Body: "reset"},
	}
	a, _ := NewAdapter(Options{
		Sender:          &recordingSender{},
		Capture:         (&recordingCapture{}).fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)
	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "/reset"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if len(stub.handled) != 1 || stub.handled[0].Kind != contracts.KindReset {
		t.Errorf("stub.handled = %v; want one KindReset", stub.handled)
	}
}

// TestRender_RequiresChatID asserts contracts.TransportAdapter.Render
// (identity-only) fails-loud because Telegram requires a chat_id.
// Callers MUST use RenderToChat or HandleUpdate.
func TestRender_RequiresChatID(t *testing.T) {
	t.Parallel()
	a, _ := NewAdapter(Options{
		Sender:          &recordingSender{},
		Capture:         (&recordingCapture{}).fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	err := a.Render(context.Background(), contracts.TransportIdentity{UserID: "u1", Transport: "telegram"}, contracts.AssistantResponse{})
	if err == nil {
		t.Fatal("err = nil; want non-nil (Render requires chat_id)")
	}
}

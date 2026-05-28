package assistant_adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// transportName is the closed-vocabulary token for this adapter.
// It MUST match the value used by the facade audit layer and the
// AssistantMessage.Transport field. (design.md §2.1.)
const transportName = "telegram"

// Sender is the minimal subset of *tgbotapi.BotAPI the adapter
// depends on. Production wiring passes the live API; tests substitute
// a recording fake.
type Sender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// CaptureFn is the bot-side hook the adapter calls when the
// capability layer returns AssistantResponse.CaptureRoute == true.
// The implementation is expected to delegate to the parent
// internal/telegram.Bot.handleTextCapture path, preserving the
// BS-001 regression contract.
//
// Signature carries the inbound *tgbotapi.Message so the bot-side
// handler can reuse its existing handleTextCapture(ctx, msg, text)
// path verbatim — the artifact written is byte-for-byte identical
// to the BS-001 fallthrough path.
type CaptureFn func(ctx context.Context, msg *tgbotapi.Message, text string)

// UserResolver maps a Telegram chat_id to the canonical Smackerel
// user_id via spec 044 (TELEGRAM_USER_MAPPING). It MUST return
// (non-empty user_id, nil) on success and (empty string, non-nil
// error) on every refusal. The adapter never relaxes this contract —
// production refusals propagate as errors to the bot and the message
// is dropped without ever reaching the capability layer.
type UserResolver func(chatID int64) (string, error)

// MarkdownMode is the closed-vocabulary text formatting selector
// enforced by SCOPE-01 SST (config key
// `assistant.transports.telegram.markdown_mode`).
type MarkdownMode string

const (
	// MarkdownV2 uses Telegram's official escaped Markdown dialect.
	// All renderer-controlled body text is escaped before send.
	MarkdownV2 MarkdownMode = "MarkdownV2"
	// PlainText skips escaping; the parse_mode is left unset on the
	// outbound message.
	PlainText MarkdownMode = "plain"
	// HTML uses Telegram's HTML parse_mode.
	HTML MarkdownMode = "HTML"
)

// allMarkdownModes is the exhaustive closed-vocabulary list.
var allMarkdownModes = []MarkdownMode{MarkdownV2, PlainText, HTML}

// Options is the constructor input for NewAdapter. Every field is
// required (the constructor returns an error when any required
// dependency is nil). The adapter does not synthesize defaults; the
// caller is responsible for sourcing values from SST config per the
// smackerel-no-defaults policy.
type Options struct {
	// Sender is the tgbotapi.BotAPI (or test fake) used to deliver
	// outbound messages. REQUIRED.
	Sender Sender

	// Capture is the bot-side hook called when CaptureRoute=true.
	// REQUIRED — even a noop is unacceptable because it would silently
	// drop notes (BS-001 regression).
	Capture CaptureFn

	// ResolveUser maps chat_id → user_id via spec 044. REQUIRED.
	ResolveUser UserResolver

	// MarkdownMode is the closed-vocabulary text formatting selector.
	// REQUIRED — must be one of MarkdownV2, PlainText, HTML.
	MarkdownMode MarkdownMode

	// MaxMessageChars is the per-message character ceiling enforced
	// by SCOPE-01 SST. Sourced from
	// `assistant.transports.telegram.max_message_chars` (4096 for
	// Telegram per the protocol). REQUIRED — must be > 0.
	MaxMessageChars int
}

// Adapter is the Telegram implementation of
// contracts.TransportAdapter. Safe for concurrent use.
type Adapter struct {
	sender          Sender
	capture         CaptureFn
	resolveUser     UserResolver
	markdownMode    MarkdownMode
	maxMessageChars int

	mu        sync.RWMutex
	assistant contracts.Assistant
}

// NewAdapter constructs a Telegram TransportAdapter. The adapter is
// inert until Start binds the capability facade; the bot is free to
// construct the adapter at startup and inject the facade later.
func NewAdapter(opts Options) (*Adapter, error) {
	if opts.Sender == nil {
		return nil, errors.New("assistant_adapter: Sender is required")
	}
	if opts.Capture == nil {
		return nil, errors.New("assistant_adapter: Capture is required (BS-001 regression contract)")
	}
	if opts.ResolveUser == nil {
		return nil, errors.New("assistant_adapter: ResolveUser is required (spec 044 boundary)")
	}
	if !validMarkdownMode(opts.MarkdownMode) {
		return nil, fmt.Errorf("assistant_adapter: MarkdownMode %q is not one of %v", opts.MarkdownMode, allMarkdownModes)
	}
	if opts.MaxMessageChars <= 0 {
		return nil, fmt.Errorf("assistant_adapter: MaxMessageChars must be > 0 (got %d)", opts.MaxMessageChars)
	}
	return &Adapter{
		sender:          opts.Sender,
		capture:         opts.Capture,
		resolveUser:     opts.ResolveUser,
		markdownMode:    opts.MarkdownMode,
		maxMessageChars: opts.MaxMessageChars,
	}, nil
}

// validMarkdownMode is the exhaustive closed-vocabulary check.
func validMarkdownMode(m MarkdownMode) bool {
	for _, v := range allMarkdownModes {
		if v == m {
			return true
		}
	}
	return false
}

// Name returns the closed-vocabulary transport name. Always
// "telegram" — the value MUST match AssistantMessage.Transport for
// every message the adapter emits.
func (a *Adapter) Name() string { return transportName }

// Translate converts an *tgbotapi.Update into an AssistantMessage.
// Implements contracts.TransportAdapter.
func (a *Adapter) Translate(ctx context.Context, payload contracts.TransportPayload) (contracts.AssistantMessage, error) {
	update, ok := payload.(*tgbotapi.Update)
	if !ok || update == nil {
		return contracts.AssistantMessage{}, fmt.Errorf("assistant_adapter: Translate expects *tgbotapi.Update, got %T", payload)
	}
	return translateInbound(update, a.resolveUser)
}

// Identity resolves the chat_id on the supplied Update via the spec
// 044 resolver. Implements contracts.TransportAdapter.
func (a *Adapter) Identity(ctx context.Context, payload contracts.TransportPayload) (contracts.TransportIdentity, error) {
	update, ok := payload.(*tgbotapi.Update)
	if !ok || update == nil {
		return contracts.TransportIdentity{}, fmt.Errorf("assistant_adapter: Identity expects *tgbotapi.Update, got %T", payload)
	}
	chatID := chatIDFromUpdate(update)
	if chatID == 0 {
		return contracts.TransportIdentity{}, errors.New("assistant_adapter: update has no chat_id")
	}
	userID, err := a.resolveUser(chatID)
	if err != nil {
		return contracts.TransportIdentity{}, err
	}
	if userID == "" {
		return contracts.TransportIdentity{}, errors.New("assistant_adapter: UserResolver returned empty user_id without error")
	}
	return contracts.TransportIdentity{UserID: userID, Transport: transportName}, nil
}

// Render delivers an AssistantResponse to the supplied identity.
// Implements contracts.TransportAdapter.
func (a *Adapter) Render(ctx context.Context, identity contracts.TransportIdentity, resp contracts.AssistantResponse) error {
	return errors.New("assistant_adapter: Render(identity, resp) requires a chat_id; use RenderToChat or HandleUpdate")
}

// RenderToChat is the chat-id-targeted render entry point. Identity
// alone is insufficient on Telegram because user_id ≠ chat_id; the
// adapter must know which chat to write to.
func (a *Adapter) RenderToChat(ctx context.Context, chatID int64, resp contracts.AssistantResponse) error {
	return renderOutbound(ctx, a.sender, chatID, a.markdownMode, a.maxMessageChars, resp)
}

// Start binds the capability facade. Idempotent: calling Start with a
// new facade rebinds; calling with nil clears the binding.
func (a *Adapter) Start(ctx context.Context, assistant contracts.Assistant) error {
	a.mu.Lock()
	a.assistant = assistant
	a.mu.Unlock()
	return nil
}

// Stop is currently a no-op; there are no background goroutines.
// Reserved for future buffered confirm/disambig timeout sweeps.
func (a *Adapter) Stop(ctx context.Context) error { return nil }

// Assistant returns the currently-bound facade (or nil before Start).
// Exposed for the bot's plain-text branch — it MUST check
// IsBound() before calling HandleUpdate so the BS-001 regression
// path is preserved.
func (a *Adapter) Assistant() contracts.Assistant {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.assistant
}

// IsBound reports whether Start has bound a non-nil facade.
func (a *Adapter) IsBound() bool {
	return a.Assistant() != nil
}

// HandleUpdate is the bot-side entry point. It runs Identity →
// Translate → Assistant.Handle → RenderToChat in one call.
//
// Returned (handled, err):
//
//   - (false, nil) when the adapter has no bound facade (BS-001
//     regression-safe fallthrough — the bot SHOULD invoke its legacy
//     handleTextCapture path).
//   - (true, nil) when the message was processed end-to-end.
//   - (true, err) when the message was claimed by the adapter but a
//     downstream step failed; the bot SHOULD log and NOT re-handle.
func (a *Adapter) HandleUpdate(ctx context.Context, update *tgbotapi.Update) (bool, error) {
	assistant := a.Assistant()
	if assistant == nil {
		return false, nil
	}
	if update == nil {
		return false, errors.New("assistant_adapter: HandleUpdate called with nil update")
	}
	chatID := chatIDFromUpdate(update)
	if chatID == 0 {
		return false, errors.New("assistant_adapter: update has no chat_id")
	}
	msg, err := translateInbound(update, a.resolveUser)
	if err != nil {
		if errors.Is(err, ErrNotAssistantMessage) {
			return false, nil
		}
		return true, fmt.Errorf("translate: %w", err)
	}
	resp, err := assistant.Handle(ctx, msg)
	if err != nil {
		return true, fmt.Errorf("assistant.Handle: %w", err)
	}
	// CaptureRoute fires BEFORE the user-facing send so the artifact
	// is durable even if Telegram is unreachable. The bot-side hook
	// receives the verbatim inbound *tgbotapi.Message so the existing
	// capture path produces the same idea artifact it would have
	// produced on the legacy BS-001 fallthrough.
	if resp.CaptureRoute && update.Message != nil {
		a.capture(ctx, update.Message, msg.Text)
	}
	if err := a.RenderToChat(ctx, chatID, resp); err != nil {
		return true, fmt.Errorf("render: %w", err)
	}
	return true, nil
}

// chatIDFromUpdate extracts the chat_id from either a message or a
// callback query payload. Returns 0 when the update has neither.
func chatIDFromUpdate(update *tgbotapi.Update) int64 {
	if update == nil {
		return 0
	}
	if update.Message != nil && update.Message.Chat != nil {
		return update.Message.Chat.ID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat != nil {
		return update.CallbackQuery.Message.Chat.ID
	}
	return 0
}

// stripBotMention strips a leading "@bot_username" mention from a
// text message — Telegram appends it to commands when the user
// targets a specific bot in a group chat. This is a defensive
// no-op for direct chats.
func stripBotMention(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "@") {
		return text
	}
	if i := strings.Index(text, " "); i > 0 {
		return strings.TrimSpace(text[i:])
	}
	return ""
}

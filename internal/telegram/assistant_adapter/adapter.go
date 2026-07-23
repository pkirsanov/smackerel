package assistant_adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
	"go.opentelemetry.io/otel/attribute"
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
// handler can reuse its existing capture path — the artifact written is
// byte-for-byte identical to the BS-001 fallthrough path.
//
// BUG-061-006 — the hook MUST persist the idea WITHOUT sending its own
// user-facing Telegram reply: the assistant renderer owns the single
// acknowledgement, so a self-reply here produces the duplicate
// ". Saved …" + "saved as an idea" pair the user saw. The returned error
// lets the adapter keep the acknowledgement honest:
//   - nil                 → the idea was persisted (or already existed);
//     the renderer's "saved as an idea" ack stands.
//   - ErrNothingToCapture  → there was nothing to save (e.g. a bare
//     "/ask"); the adapter renders an honest prompt
//     instead of a false "saved as an idea".
//   - any other error      → the capture failed; the adapter renders an
//     honest failure line, never "saved as an idea".
type CaptureFn func(ctx context.Context, msg *tgbotapi.Message, text string) error

// ErrNothingToCapture is the sentinel a CaptureFn returns when the
// capture-as-fallback text was empty/whitespace so there was nothing to
// persist (BUG-061-006). Distinct from a real capture failure.
var ErrNothingToCapture = errors.New("assistant_adapter: nothing to capture")

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

	// Tracer is the spec 061 SCOPE-09a OTel substrate seam. When
	// non-nil, Translate emits the canonical root span
	// `assistant.adapter.translate` (design §8.3.1.A item 1) carrying
	// the 5 mandatory attrs from §8.3.1.B. Production wiring
	// (cmd/core/wiring.go) always passes a real Tracer (the no-op
	// path stays unconditional inside the tracing package itself).
	// May be nil in tests that do not exercise the tracing seam; in
	// that case a no-op tracer is substituted so emission stays
	// unconditional.
	Tracer *tracing.Tracer
}

// Adapter is the Telegram implementation of
// contracts.TransportAdapter. Safe for concurrent use.
type Adapter struct {
	sender          Sender
	capture         CaptureFn
	resolveUser     UserResolver
	markdownMode    MarkdownMode
	maxMessageChars int
	tracer          *tracing.Tracer

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
	tr := opts.Tracer
	if tr == nil {
		// Tests may omit the tracer; substitute a no-op so emission
		// sites stay unconditional. Production wiring always supplies
		// the real tracer from cmd/core/services.go.
		noopTr, _, err := tracing.NewTracer(context.Background(), tracing.Config{
			Enabled:     false,
			ServiceName: "smackerel-core",
		})
		if err != nil {
			return nil, fmt.Errorf("assistant_adapter: build noop tracer fallback: %w", err)
		}
		tr = noopTr
	}
	return &Adapter{
		sender:          opts.Sender,
		capture:         opts.Capture,
		resolveUser:     opts.ResolveUser,
		markdownMode:    opts.MarkdownMode,
		maxMessageChars: opts.MaxMessageChars,
		tracer:          tr,
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
//
// Spec 061 SCOPE-09a (design §8.3.1.A item 1) — this is the canonical
// root span site `assistant.adapter.translate`. The 5 mandatory
// attributes from §8.3.1.B are stamped at start; the 2 outcome
// attributes are stamped at end via tracing.EndSpan. scenario_id is
// stamped empty because routing has not yet selected a scenario at
// the adapter-translate stage (SCOPE-09b will stamp scenario_id on
// child spans further down the chain).
func (a *Adapter) Translate(ctx context.Context, payload contracts.TransportPayload) (contracts.AssistantMessage, error) {
	update, ok := payload.(*tgbotapi.Update)
	if !ok || update == nil {
		// Span the failure path too so dashboards can count the
		// transport-payload misuse class.
		_, span := a.tracer.StartSpan(ctx, "assistant.adapter.translate",
			transportName, "", "", "", "")
		err := fmt.Errorf("assistant_adapter: Translate expects *tgbotapi.Update, got %T", payload)
		tracing.EndSpan(span, "error", "invalid_payload")
		return contracts.AssistantMessage{}, err
	}
	// Pre-resolve the correlation_id (telegram_update_id) so the
	// span carries it even on early failures. update.UpdateID is
	// always populated by Telegram for inbound updates.
	correlationID := fmt.Sprintf("%d", update.UpdateID)
	_, span := a.tracer.StartSpan(ctx, "assistant.adapter.translate",
		transportName, "", "", "", correlationID)
	msg, err := translateInbound(update, a.resolveUser)
	if err != nil {
		// Distinguish "not for the assistant" (noop, expected) from
		// hard translation failures (error). Both end the span; only
		// the latter promotes the OTel status to Error.
		if errors.Is(err, ErrNotAssistantMessage) {
			tracing.EndSpan(span, "noop", "not_assistant_message")
			return msg, err
		}
		tracing.EndSpan(span, "error", "translate_failed")
		return msg, err
	}
	// Re-stamp user_id_hashed now that we know it. Set as an
	// attribute on the span before End — EndSpan does not overwrite
	// canonical-attr keys.
	span.SetAttributes(
		canonicalAttr("user_id_hashed", tracing.HashUserID(msg.UserID)),
	)
	tracing.EndSpan(span, "ok", "")
	return msg, nil
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
	// BUG-064-001 — this local is named `facade` (not `assistant`) so it does
	// not shadow the imported `internal/assistant` package, which HandleUpdate
	// uses below for StripShortcutPrefix on the capture path.
	facade := a.Assistant()
	if facade == nil {
		return false, nil
	}
	if update == nil {
		return false, errors.New("assistant_adapter: HandleUpdate called with nil update")
	}
	chatID := chatIDFromUpdate(update)
	if chatID == 0 {
		return false, errors.New("assistant_adapter: update has no chat_id")
	}
	// Spec 061 SCOPE-09b — start the canonical root span
	// `assistant.adapter.translate` here so the facade.handle child
	// span (started inside assistant.Handle) and the
	// `assistant.adapter.render` sibling child (started around
	// RenderToChat below) both nest under the same root per design
	// §8.3.1.A. correlation_id is the telegram_update_id, available
	// before the resolver runs. user_id_hashed is re-stamped after
	// translateInbound resolves the user id.
	correlationID := fmt.Sprintf("%d", update.UpdateID)
	ctx, rootSpan := a.tracer.StartSpan(ctx, "assistant.adapter.translate",
		transportName, "", "", "", correlationID)
	rootStatus := "ok"
	rootCause := ""
	defer func() {
		tracing.EndSpan(rootSpan, rootStatus, rootCause)
	}()

	msg, err := translateInbound(update, a.resolveUser)
	if err != nil {
		if errors.Is(err, ErrNotAssistantMessage) {
			rootStatus = "noop"
			rootCause = "not_assistant_message"
			return false, nil
		}
		rootStatus = "error"
		rootCause = "translate_failed"
		return true, fmt.Errorf("translate: %w", err)
	}
	rootSpan.SetAttributes(canonicalAttr("user_id_hashed", tracing.HashUserID(msg.UserID)))

	resp, err := facade.Handle(ctx, msg)
	if err != nil {
		rootStatus = "error"
		rootCause = "handle_failed"
		return true, fmt.Errorf("assistant.Handle: %w", err)
	}
	// CaptureRoute fires BEFORE the user-facing send so the artifact
	// is durable even if Telegram is unreachable. The bot-side hook
	// receives the inbound *tgbotapi.Message so the existing capture
	// path produces the same idea artifact it would have produced on
	// the legacy BS-001 fallthrough.
	//
	// BUG-064-001 — strip the v1 slash-command prefix from the captured
	// text. translate_inbound preserves the prefix in msg.Text so the
	// facade's LookupShortcut can pin the scenario, but a captured idea
	// must store the natural-language tail only (never "/ask",
	// "/weather", ...). StripShortcutPrefix is a no-op for non-shortcut
	// text, so ordinary plain-text captures are unchanged.
	if resp.CaptureRoute && update.Message != nil {
		// BUG-061-006 — the capture hook persists silently and reports
		// whether an idea was actually saved. When it was NOT (empty bare
		// shortcut or a real failure) we replace the "saved as an idea"
		// response with an honest single acknowledgement so the user never
		// sees a false "saved" or the contradictory "? Failed to save" +
		// "saved as an idea" pair.
		if capErr := a.capture(ctx, update.Message, assistant.StripShortcutPrefix(msg.Text)); capErr != nil {
			resp = honestCaptureFallbackFailure(resp, capErr)
		}
	}
	// Spec 061 SCOPE-09b — `assistant.adapter.render` span (design
	// §8.3.1.A item 10). Sibling of facade.handle under the same
	// translate root.
	scenarioForRender := ""
	if resp.Routing != nil {
		scenarioForRender = resp.Routing.Chosen
	}
	ctxRender, renderSpan := a.tracer.StartSpan(ctx, "assistant.adapter.render",
		transportName, tracing.HashUserID(msg.UserID), "", scenarioForRender, correlationID)
	if renderErr := a.RenderToChat(ctxRender, chatID, resp); renderErr != nil {
		tracing.EndSpan(renderSpan, "error", "render_failed")
		rootStatus = "error"
		rootCause = "render_failed"
		return true, fmt.Errorf("render: %w", renderErr)
	}
	tracing.EndSpan(renderSpan, "ok", "")
	return true, nil
}

// honestCaptureFallbackFailure replaces a capture-as-fallback response
// whose body would claim "saved as an idea" with an honest single
// acknowledgement when the bot-side capture hook did NOT persist an idea.
// This prevents the BUG-061-006 defects: the contradictory
// "? Failed to save" + "saved as an idea" pair, and the false
// "saved as an idea" on a bare shortcut that carried no text.
//
// The returned response renders through the default body path (no status
// prefix), so the user sees exactly one honest line.
func honestCaptureFallbackFailure(resp contracts.AssistantResponse, capErr error) contracts.AssistantResponse {
	body := "Couldn't save that just now — please try again."
	if errors.Is(capErr, ErrNothingToCapture) {
		body = "Nothing to save — add some text or a question after the command."
	}
	return contracts.AssistantResponse{
		Status:    contracts.StatusAnswered,
		Body:      body,
		Routing:   resp.Routing,
		EmittedAt: resp.EmittedAt,
	}
}

// canonicalAttr is a thin convenience for stamping a single
// attribute.KeyValue with a string value. Used by Translate to
// re-stamp user_id_hashed after the resolver supplies it.
func canonicalAttr(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
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

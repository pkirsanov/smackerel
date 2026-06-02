// Package assistant_adapter is the WhatsApp Business Cloud API
// implementation of contracts.TransportAdapter. Spec 072 SCOPE-1
// owns the webhook ingress, Meta signature verification, generic
// transport identity lookup, and translation of inbound WhatsApp
// payloads into the canonical AssistantMessage. Outbound rendering
// (text/list/buttons) is owned by SCOPE-2 and is intentionally
// stubbed here — Render returns a fixed "renderer not yet wired"
// error so a misconfigured caller fails loud instead of silently.
package assistant_adapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/transportidentity"
)

// CloudClient is the narrow outbound interface the WhatsApp adapter
// uses to deliver rendered messages to the Meta Cloud API. SCOPE-2
// owns only the interface; the concrete HTTP client lives in a
// future internal/whatsapp/cloudapi package and is wired by
// cmd/core.
type CloudClient interface {
	SendText(ctx context.Context, to string, msg TextMessage) error
	SendInteractive(ctx context.Context, to string, msg InteractiveMessage) error
}

// CaptureFn is the capture-as-fallback hook the adapter calls when
// AssistantResponse.CaptureRoute == true. The implementation MUST
// persist the inbound user message as a durable idea artifact (the
// canonical "saved as an idea" path). The hook is invoked exactly
// once per accepted message BEFORE the user-facing render so the
// artifact is durable even if the Cloud API send fails.
type CaptureFn func(ctx context.Context, msg contracts.AssistantMessage)

var (
	renderTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_render_total",
			Help: "WhatsApp outbound renders by message family (labels: kind=text|interactive_buttons|interactive_list).",
		},
		[]string{"kind"},
	)
	renderErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_render_errors_total",
			Help: "WhatsApp render failures (labels: reason=nothing_to_render|invalid_response|cloud_api|other).",
		},
		[]string{"reason"},
	)
	captureInvocations = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_capture_invocations_total",
			Help: "WhatsApp capture-as-fallback hook invocations (CaptureRoute == true on the AssistantResponse).",
		},
	)
	idempotentRetries = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_idempotent_retries_total",
			Help: "WhatsApp webhook deliveries swallowed as Meta retries of a TransportMessageID already processed by this adapter instance.",
		},
	)
	// Spec 072 SCOPE-4 — operator-visible transport status gauges
	// and last-send counter. The gauges are set exactly once by the
	// MountWebhookRoutes helper; operators can read them via
	// /metrics to distinguish disabled (0) from enabled-and-
	// credential-ready (1).
	transportEnabledGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "assistant_whatsapp_enabled",
			Help: "1 when assistant.transports.whatsapp.enabled=true at boot, 0 when disabled.",
		},
	)
	transportCredsReadyGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "assistant_whatsapp_credentials_ready",
			Help: "1 when the WhatsApp adapter constructed successfully (verifier, identity registry, cloud client, capture hook all bound), 0 otherwise.",
		},
	)
	cloudSendTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_cloud_send_total",
			Help: "WhatsApp Cloud API send attempts by result (labels: result=ok|error, kind=text|interactive_buttons|interactive_list).",
		},
		[]string{"result", "kind"},
	)
)

func init() {
	prometheus.MustRegister(
		renderTotal, renderErrors, captureInvocations, idempotentRetries,
		transportEnabledGauge, transportCredsReadyGauge, cloudSendTotal,
	)
}

// SetTransportStatus is the operator-status surface invoked by
// cmd/core during webhook mount. enabled mirrors the SST flag;
// credsReady reports whether the adapter is bound and ready to
// authenticate and translate Meta deliveries. Callers MUST invoke
// this exactly once at boot (and again at any reconfigure point) so
// /metrics reflects the live transport status.
func SetTransportStatus(enabled, credsReady bool) {
	if enabled {
		transportEnabledGauge.Set(1)
	} else {
		transportEnabledGauge.Set(0)
	}
	if credsReady {
		transportCredsReadyGauge.Set(1)
	} else {
		transportCredsReadyGauge.Set(0)
	}
}

// TransportName is the closed-vocabulary token for this adapter.
// MUST match AssistantMessage.Transport and the value used by the
// facade audit layer.
const TransportName = "whatsapp"

// SignatureHeader is the Meta-defined HMAC-SHA256 header sent on
// every signed webhook delivery. Spec 072 design §5 ("Inbound
// Delivery"): the header value is the literal string
// "sha256=<hex>" where <hex> is HMAC-SHA256(app_secret, raw_body).
const SignatureHeader = "X-Hub-Signature-256"

// signaturePrefix is the closed-vocabulary algorithm tag. Anything
// else is rejected as malformed.
const signaturePrefix = "sha256="

// WebhookVerifier authenticates an inbound Meta webhook. Two
// distinct paths exist: VerifyChallenge for the one-time
// GET subscribe/verify_token handshake, and Verify for every signed
// POST delivery. Implementations MUST use constant-time compare.
type WebhookVerifier interface {
	Verify(rawBody []byte, signature string) error
	VerifyChallenge(token string) error
}

// ErrInvalidSignature is the closed-vocabulary signature-rejection
// error. The webhook handler MUST stop before facade invocation
// when Verify returns this error.
var ErrInvalidSignature = errors.New("whatsapp_adapter: invalid signature")

// ErrInvalidChallenge is the closed-vocabulary challenge-rejection
// error for the GET verify-token handshake.
var ErrInvalidChallenge = errors.New("whatsapp_adapter: invalid challenge")

// HMACVerifier verifies the Meta X-Hub-Signature-256 header by
// HMAC-SHA256-ing the raw request body with the configured app
// secret and comparing constant-time to the provided hex digest.
type HMACVerifier struct {
	AppSecret   string
	VerifyToken string
}

// Verify implements WebhookVerifier.
func (h HMACVerifier) Verify(rawBody []byte, signature string) error {
	if h.AppSecret == "" {
		return errors.New("whatsapp_adapter: HMACVerifier.AppSecret is required")
	}
	if signature == "" {
		return ErrInvalidSignature
	}
	if !strings.HasPrefix(signature, signaturePrefix) {
		return ErrInvalidSignature
	}
	provided, err := hex.DecodeString(signature[len(signaturePrefix):])
	if err != nil {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(h.AppSecret))
	_, _ = mac.Write(rawBody)
	expected := mac.Sum(nil)
	if !hmac.Equal(provided, expected) {
		return ErrInvalidSignature
	}
	return nil
}

// VerifyChallenge implements WebhookVerifier.
func (h HMACVerifier) VerifyChallenge(token string) error {
	if h.VerifyToken == "" {
		return errors.New("whatsapp_adapter: HMACVerifier.VerifyToken is required")
	}
	if subtleConstantTimeStringEq(token, h.VerifyToken) {
		return nil
	}
	return ErrInvalidChallenge
}

// subtleConstantTimeStringEq is hmac.Equal over the underlying
// bytes; we avoid importing crypto/subtle just for the length check.
func subtleConstantTimeStringEq(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

// Options is the constructor input for NewAdapter. Every field is
// REQUIRED when WhatsApp is enabled; the constructor returns an
// error on any nil dependency. SCOPE-1 surfaces only the
// dependencies needed for ingress + identity + translate; the
// CloudClient / Capture / Tracer additions land in SCOPE-2 and
// SCOPE-3 and will extend this struct without breaking SCOPE-1.
type Options struct {
	// Verify is the webhook verifier. REQUIRED.
	Verify WebhookVerifier

	// IdentityRegistry resolves a hashed phone subject to a
	// canonical user_id. REQUIRED.
	IdentityRegistry transportidentity.Registry

	// IdentityHashKey is the HMAC key used to hash inbound phone
	// numbers before lookup. REQUIRED.
	IdentityHashKey string

	// MaxTextChars is the SST-supplied per-message text cap.
	// REQUIRED, MUST be > 0.
	MaxTextChars int

	// RateLimitPerUserPerMinute is the SST-supplied per-user
	// rate-limit. REQUIRED, MUST be > 0. SCOPE-1 stores the value;
	// SCOPE-3 enforces it.
	RateLimitPerUserPerMinute int

	// Cloud is the outbound WhatsApp Cloud API client used by
	// Render. Optional at construction time so SCOPE-1 unit fixtures
	// (which exercise ingress only) keep compiling; the Render method
	// fails loud when Cloud is nil at call time.
	Cloud CloudClient

	// Capture is the capture-as-fallback hook. Optional at
	// construction time for the same reason; the webhook handler
	// skips capture-route dispatch when Capture is nil and the
	// response has CaptureRoute=true (with a fail-loud log).
	Capture CaptureFn
}

// Adapter is the WhatsApp implementation of
// contracts.TransportAdapter. Safe for concurrent use.
type Adapter struct {
	verify           WebhookVerifier
	identity         transportidentity.Registry
	identityHashKey  string
	maxTextChars     int
	rateLimitPerUser int
	cloud            CloudClient
	capture          CaptureFn
	dedup            *idempotencyCache

	mu        sync.RWMutex
	assistant contracts.Assistant
}

// NewAdapter constructs a WhatsApp TransportAdapter. The adapter is
// inert until Start binds the capability facade.
func NewAdapter(opts Options) (*Adapter, error) {
	if opts.Verify == nil {
		return nil, errors.New("whatsapp_adapter: Verify is required")
	}
	if opts.IdentityRegistry == nil {
		return nil, errors.New("whatsapp_adapter: IdentityRegistry is required")
	}
	if opts.IdentityHashKey == "" {
		return nil, errors.New("whatsapp_adapter: IdentityHashKey is required")
	}
	if opts.MaxTextChars <= 0 {
		return nil, fmt.Errorf("whatsapp_adapter: MaxTextChars must be > 0 (got %d)", opts.MaxTextChars)
	}
	if opts.RateLimitPerUserPerMinute <= 0 {
		return nil, fmt.Errorf("whatsapp_adapter: RateLimitPerUserPerMinute must be > 0 (got %d)", opts.RateLimitPerUserPerMinute)
	}
	return &Adapter{
		verify:           opts.Verify,
		identity:         opts.IdentityRegistry,
		identityHashKey:  opts.IdentityHashKey,
		maxTextChars:     opts.MaxTextChars,
		rateLimitPerUser: opts.RateLimitPerUserPerMinute,
		cloud:            opts.Cloud,
		capture:          opts.Capture,
		dedup:            newIdempotencyCache(IdempotencyCacheCapacity),
	}, nil
}

// Capture exposes the configured capture-as-fallback hook (nil when
// not configured). The webhook handler uses it to honor CaptureRoute
// before render.
func (a *Adapter) Capture() CaptureFn { return a.capture }

// HasCloud reports whether a CloudClient was wired at construction.
func (a *Adapter) HasCloud() bool { return a.cloud != nil }

// Name returns "whatsapp".
func (a *Adapter) Name() string { return TransportName }

// VerifyRaw is the package-level entry point the HTTP webhook
// handler calls before parsing the JSON body. It exposes the
// verifier without leaking the *Adapter type.
func (a *Adapter) VerifyRaw(rawBody []byte, signature string) error {
	return a.verify.Verify(rawBody, signature)
}

// VerifyChallenge is the GET verify-token handshake check.
func (a *Adapter) VerifyChallenge(token string) error {
	return a.verify.VerifyChallenge(token)
}

// inboundEnvelope is the minimal subset of the Meta WhatsApp
// Business webhook payload that SCOPE-1 needs. Unknown fields are
// silently ignored (forward-compat); unsupported message types are
// rejected by Translate so the facade never sees them.
type inboundEnvelope struct {
	Object string         `json:"object"`
	Entry  []inboundEntry `json:"entry"`
}

type inboundEntry struct {
	ID      string          `json:"id"`
	Changes []inboundChange `json:"changes"`
}

type inboundChange struct {
	Field string       `json:"field"`
	Value inboundValue `json:"value"`
}

type inboundValue struct {
	MessagingProduct string           `json:"messaging_product"`
	Metadata         inboundValueMeta `json:"metadata"`
	Messages         []inboundMessage `json:"messages"`
}

type inboundValueMeta struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type inboundMessage struct {
	ID          string              `json:"id"`
	From        string              `json:"from"`
	Timestamp   string              `json:"timestamp"`
	Type        string              `json:"type"`
	Text        *inboundText        `json:"text,omitempty"`
	Interactive *inboundInteractive `json:"interactive,omitempty"`
}

type inboundText struct {
	Body string `json:"body"`
}

type inboundInteractive struct {
	Type        string                  `json:"type"`
	ButtonReply *inboundInteractiveBtn  `json:"button_reply,omitempty"`
	ListReply   *inboundInteractiveList `json:"list_reply,omitempty"`
}

type inboundInteractiveBtn struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type inboundInteractiveList struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ErrEmptyDelivery is returned when a verified payload contains no
// processable message (Meta also delivers status/read receipts that
// SCOPE-1 silently ignores).
var ErrEmptyDelivery = errors.New("whatsapp_adapter: no processable message in delivery")

// ErrUnsupportedMessageType is the closed-vocabulary rejection for
// inbound types SCOPE-1 does not yet translate.
var ErrUnsupportedMessageType = errors.New("whatsapp_adapter: unsupported message type")

// ParsePayload decodes a verified raw body into the first
// processable inboundMessage plus the canonical
// AssistantMessage shell (Transport + TransportMessageID set). The
// caller is expected to add UserID via Identity() before invoking
// the facade.
func ParsePayload(rawBody []byte) (inboundMessage, error) {
	var env inboundEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return inboundMessage{}, fmt.Errorf("whatsapp_adapter: invalid JSON: %w", err)
	}
	for _, entry := range env.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				if msg.ID == "" {
					continue
				}
				return msg, nil
			}
		}
	}
	return inboundMessage{}, ErrEmptyDelivery
}

// Translate implements contracts.TransportAdapter.Translate. payload
// MUST be a *inboundMessage produced by ParsePayload OR a raw []byte
// body the adapter will parse itself. Returned AssistantMessage
// carries Transport="whatsapp" and TransportMessageID=<Meta message
// id> — both required for facade-level idempotency.
func (a *Adapter) Translate(ctx context.Context, payload contracts.TransportPayload) (contracts.AssistantMessage, error) {
	msg, err := coercePayload(payload)
	if err != nil {
		return contracts.AssistantMessage{}, err
	}
	out := contracts.AssistantMessage{
		Transport:          TransportName,
		TransportMessageID: msg.ID,
	}
	switch msg.Type {
	case "text":
		if msg.Text == nil {
			return contracts.AssistantMessage{}, errors.New("whatsapp_adapter: text message missing body")
		}
		body := msg.Text.Body
		if strings.EqualFold(strings.TrimSpace(body), ResetTextCommand) {
			out.Kind = contracts.KindReset
			out.Text = body
		} else {
			out.Kind = contracts.KindText
			out.Text = body
		}
	case "interactive":
		if msg.Interactive == nil {
			return contracts.AssistantMessage{}, errors.New("whatsapp_adapter: interactive message missing payload")
		}
		switch msg.Interactive.Type {
		case "button_reply":
			if msg.Interactive.ButtonReply == nil {
				return contracts.AssistantMessage{}, errors.New("whatsapp_adapter: interactive button_reply missing inner payload")
			}
			if err := decodeInteractivePayload(msg.Interactive.ButtonReply.ID, &out); err != nil {
				return contracts.AssistantMessage{}, err
			}
		case "list_reply":
			if msg.Interactive.ListReply == nil {
				return contracts.AssistantMessage{}, errors.New("whatsapp_adapter: interactive list_reply missing inner payload")
			}
			if err := decodeInteractivePayload(msg.Interactive.ListReply.ID, &out); err != nil {
				return contracts.AssistantMessage{}, err
			}
		default:
			return contracts.AssistantMessage{}, fmt.Errorf("%w: interactive=%q", ErrUnsupportedMessageType, msg.Interactive.Type)
		}
	default:
		return contracts.AssistantMessage{}, fmt.Errorf("%w: type=%q", ErrUnsupportedMessageType, msg.Type)
	}
	userID, err := a.resolveUserID(ctx, msg.From)
	if err != nil {
		return contracts.AssistantMessage{}, err
	}
	out.UserID = userID
	return out, nil
}

// Identity implements contracts.TransportAdapter.Identity by
// resolving the inbound phone subject to a canonical user_id.
func (a *Adapter) Identity(ctx context.Context, payload contracts.TransportPayload) (contracts.TransportIdentity, error) {
	msg, err := coercePayload(payload)
	if err != nil {
		return contracts.TransportIdentity{}, err
	}
	userID, err := a.resolveUserID(ctx, msg.From)
	if err != nil {
		return contracts.TransportIdentity{}, err
	}
	return contracts.TransportIdentity{UserID: userID, Transport: TransportName}, nil
}

// resolveUserID hashes the inbound phone and resolves it to the
// canonical user_id via the identity registry.
func (a *Adapter) resolveUserID(ctx context.Context, fromPhone string) (string, error) {
	subjectHash, err := transportidentity.HashPhoneE164(a.identityHashKey, fromPhone)
	if err != nil {
		return "", fmt.Errorf("whatsapp_adapter: hash phone: %w", err)
	}
	return a.identity.Resolve(ctx, TransportName, subjectHash)
}

// Render implements contracts.TransportAdapter.Render. WhatsApp
// needs the destination phone number (E.164) which is NOT carried on
// TransportIdentity; the webhook handler uses RenderToPhone after
// extracting the inbound `from` field. Calling this method with only
// an identity is a wiring error.
func (a *Adapter) Render(ctx context.Context, identity contracts.TransportIdentity, resp contracts.AssistantResponse) error {
	return errors.New("whatsapp_adapter: Render(identity, resp) requires a destination phone; use RenderToPhone")
}

// RenderToPhone is the phone-targeted render entry point used by the
// webhook handler. It runs the pure Render mapping and dispatches
// the result through the configured CloudClient.
func (a *Adapter) RenderToPhone(ctx context.Context, toPhone string, resp contracts.AssistantResponse) error {
	if a.cloud == nil {
		renderErrors.WithLabelValues("other").Inc()
		return errors.New("whatsapp_adapter: RenderToPhone called without configured CloudClient")
	}
	if strings.TrimSpace(toPhone) == "" {
		renderErrors.WithLabelValues("other").Inc()
		return errors.New("whatsapp_adapter: RenderToPhone called with empty destination phone")
	}
	out, err := Render(resp, a.maxTextChars)
	if err != nil {
		if errors.Is(err, ErrNothingToRender) {
			renderErrors.WithLabelValues("nothing_to_render").Inc()
		} else {
			renderErrors.WithLabelValues("invalid_response").Inc()
		}
		return err
	}
	switch out.Kind {
	case OutboundText:
		if err := a.cloud.SendText(ctx, toPhone, *out.Text); err != nil {
			renderErrors.WithLabelValues("cloud_api").Inc()
			cloudSendTotal.WithLabelValues("error", string(out.Kind)).Inc()
			return fmt.Errorf("whatsapp_adapter: cloud send text: %w", err)
		}
		cloudSendTotal.WithLabelValues("ok", string(out.Kind)).Inc()
	case OutboundInteractiveButtons, OutboundInteractiveList:
		if err := a.cloud.SendInteractive(ctx, toPhone, *out.Interactive); err != nil {
			renderErrors.WithLabelValues("cloud_api").Inc()
			cloudSendTotal.WithLabelValues("error", string(out.Kind)).Inc()
			return fmt.Errorf("whatsapp_adapter: cloud send interactive: %w", err)
		}
		cloudSendTotal.WithLabelValues("ok", string(out.Kind)).Inc()
	default:
		renderErrors.WithLabelValues("other").Inc()
		return fmt.Errorf("whatsapp_adapter: unknown outbound kind %q", out.Kind)
	}
	renderTotal.WithLabelValues(string(out.Kind)).Inc()
	return nil
}

// InvokeCapture runs the configured capture hook with the canonical
// inbound message. Safe to call when capture is nil — it returns
// false so the caller can log a fail-loud diagnostic.
func (a *Adapter) InvokeCapture(ctx context.Context, msg contracts.AssistantMessage) bool {
	if a.capture == nil {
		return false
	}
	a.capture(ctx, msg)
	captureInvocations.Inc()
	return true
}

// Start binds the capability facade. Idempotent.
func (a *Adapter) Start(ctx context.Context, assistant contracts.Assistant) error {
	a.mu.Lock()
	a.assistant = assistant
	a.mu.Unlock()
	return nil
}

// Stop is currently a no-op; no background goroutines exist.
func (a *Adapter) Stop(ctx context.Context) error { return nil }

// Assistant returns the bound facade (or nil before Start).
func (a *Adapter) Assistant() contracts.Assistant {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.assistant
}

// IsBound reports whether Start has bound a non-nil facade.
func (a *Adapter) IsBound() bool { return a.Assistant() != nil }

// MarkSeen records the inbound TransportMessageID and reports
// whether the adapter has already processed it. SCOPE-3 idempotency:
// the webhook handler MUST call MarkSeen BEFORE facade invocation
// and BEFORE capture so Meta retries are swallowed without a second
// facade/capture/scenario invocation.
func (a *Adapter) MarkSeen(id string) bool {
	return a.dedup.markSeen(id)
}

// MaxTextChars returns the SST-supplied outbound text cap. Exposed
// for the SCOPE-2 renderer.
func (a *Adapter) MaxTextChars() int { return a.maxTextChars }

// RateLimitPerUserPerMinute returns the SST-supplied per-user rate
// cap. Exposed for the SCOPE-3 ingress limiter.
func (a *Adapter) RateLimitPerUserPerMinute() int { return a.rateLimitPerUser }

// decodeInteractivePayload routes a WhatsApp interactive reply id
// (button or list) to the right AssistantMessage kind. Disambig and
// confirm payloads are emitted by the renderer with closed-vocabulary
// prefixes (see render.go); anything else is refused so unknown
// round-trip ids never reach the facade.
func decodeInteractivePayload(payloadID string, out *contracts.AssistantMessage) error {
	if ref, choice, ok := DecodeDisambigPayload(payloadID); ok {
		out.Kind = contracts.KindDisambiguation
		out.DisambiguationRef = ref
		out.DisambiguationChoice = choice
		return nil
	}
	if ref, positive, ok := DecodeConfirmPayload(payloadID); ok {
		out.Kind = contracts.KindConfirm
		out.ConfirmRef = ref
		if positive {
			out.ConfirmChoice = contracts.ConfirmPositive
		} else {
			out.ConfirmChoice = contracts.ConfirmNegative
		}
		return nil
	}
	if _, ok := DecodeResetPayload(payloadID); ok {
		out.Kind = contracts.KindReset
		return nil
	}
	return fmt.Errorf("%w: interactive_payload_id=%q", ErrUnsupportedMessageType, payloadID)
}

func coercePayload(payload contracts.TransportPayload) (inboundMessage, error) {
	switch v := payload.(type) {
	case inboundMessage:
		return v, nil
	case *inboundMessage:
		if v == nil {
			return inboundMessage{}, errors.New("whatsapp_adapter: nil *inboundMessage payload")
		}
		return *v, nil
	case []byte:
		return ParsePayload(v)
	default:
		return inboundMessage{}, fmt.Errorf("whatsapp_adapter: unsupported payload type %T", payload)
	}
}

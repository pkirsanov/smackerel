// Spec 072 SCOPE-1 — WhatsApp Business Cloud API webhook handler.
//
// Mirrors the Telegram webhook handler shape (internal/telegram/
// webhook_handler.go) but enforces the Meta-defined
// X-Hub-Signature-256 HMAC over the raw request body BEFORE any
// JSON parsing reaches the facade, and BEFORE identity resolution
// runs. The handler also implements the one-time GET subscribe/
// verify_token handshake Meta requires before enabling delivery.
//
// NO request-body content is logged (Principle 8 + spec 072 §8
// "Security/Compliance"). Only structured envelope fields
// (message id, message type, hashed user id once resolved,
// latency_ms) are emitted on the success path.

package assistant_adapter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/transportidentity"
)

// WebhookMaxBodyBytes caps the request body at 1 MiB. Meta's
// documented per-event payload is well below this; the cap is
// defensive against malicious or runaway upstreams.
const WebhookMaxBodyBytes = 1 << 20

var (
	webhookAuthFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_webhook_auth_failures_total",
			Help: "WhatsApp webhook deliveries rejected for missing or mismatched X-Hub-Signature-256 (labels: reason=missing|mismatch|malformed).",
		},
		[]string{"reason"},
	)
	webhookParseFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_webhook_parse_failures_total",
			Help: "WhatsApp webhook deliveries rejected for invalid or empty request body (labels: reason=invalid_json|empty|oversize|unsupported|empty_delivery).",
		},
		[]string{"reason"},
	)
	webhookIdentityFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_webhook_identity_failures_total",
			Help: "WhatsApp webhook deliveries refused for unknown or invalid phone subject (labels: reason=unknown|invalid).",
		},
		[]string{"reason"},
	)
	webhookAccepted = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "assistant_whatsapp_webhook_accepted_total",
			Help: "WhatsApp webhook deliveries verified, translated, and dispatched to the assistant facade.",
		},
	)
)

func init() {
	prometheus.MustRegister(webhookAuthFailures, webhookParseFailures, webhookIdentityFailures, webhookAccepted)
}

// WebhookHandlerOptions bundles inputs NewWebhookHandler needs.
type WebhookHandlerOptions struct {
	Adapter *Adapter
	Logger  *slog.Logger
}

// NewWebhookHandler returns the chi-mountable HTTP handler that
// authenticates and dispatches one Meta WhatsApp Business webhook
// delivery. Panics on missing required dependencies — a misconfig
// here would silently authorize every POST.
func NewWebhookHandler(opts WebhookHandlerOptions) http.Handler {
	if opts.Adapter == nil {
		panic("whatsapp_adapter.NewWebhookHandler: Adapter is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &webhookHandler{adapter: opts.Adapter, logger: logger}
}

type webhookHandler struct {
	adapter *Adapter
	logger  *slog.Logger
}

func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.serveChallenge(w, r)
	case http.MethodPost:
		h.serveDelivery(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
	}
}

// serveChallenge implements the Meta GET subscribe/verify_token
// handshake: when `hub.mode=subscribe` AND `hub.verify_token`
// matches the configured value, the handler echoes `hub.challenge`
// with status 200 and content-type text/plain. Any failure returns
// 403 with no challenge echoed.
func (h *webhookHandler) serveChallenge(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	mode := q.Get("hub.mode")
	token := q.Get("hub.verify_token")
	challenge := q.Get("hub.challenge")
	if mode != "subscribe" || challenge == "" {
		writeJSONError(w, http.StatusForbidden, "invalid_challenge_request")
		return
	}
	if err := h.adapter.VerifyChallenge(token); err != nil {
		webhookAuthFailures.WithLabelValues("mismatch").Inc()
		writeJSONError(w, http.StatusForbidden, "invalid_verify_token")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(challenge))
}

func (h *webhookHandler) serveDelivery(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	remote := remoteIPForLog(r)

	signature := r.Header.Get(SignatureHeader)
	if signature == "" {
		webhookAuthFailures.WithLabelValues("missing").Inc()
		h.logger.Warn("whatsapp webhook auth failed",
			"kind", "whatsapp_webhook_auth_fail",
			"reason", "missing",
			"remote_ip", remote,
		)
		writeJSONError(w, http.StatusUnauthorized, "missing_signature")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, WebhookMaxBodyBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			webhookParseFailures.WithLabelValues("oversize").Inc()
			writeJSONError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
			return
		}
		webhookParseFailures.WithLabelValues("invalid_json").Inc()
		writeJSONError(w, http.StatusBadRequest, "read_error")
		return
	}
	if len(body) == 0 {
		webhookParseFailures.WithLabelValues("empty").Inc()
		writeJSONError(w, http.StatusBadRequest, "empty_body")
		return
	}

	if err := h.adapter.VerifyRaw(body, signature); err != nil {
		reason := "malformed"
		if errors.Is(err, ErrInvalidSignature) {
			reason = "mismatch"
		}
		webhookAuthFailures.WithLabelValues(reason).Inc()
		h.logger.Warn("whatsapp webhook auth failed",
			"kind", "whatsapp_webhook_auth_fail",
			"reason", reason,
			"remote_ip", remote,
		)
		writeJSONError(w, http.StatusUnauthorized, "invalid_signature")
		return
	}

	msg, err := ParsePayload(body)
	if err != nil {
		reason := "invalid_json"
		switch {
		case errors.Is(err, ErrEmptyDelivery):
			reason = "empty_delivery"
		case errors.Is(err, ErrUnsupportedMessageType):
			reason = "unsupported"
		}
		webhookParseFailures.WithLabelValues(reason).Inc()
		// Empty deliveries (e.g. Meta status pings) ack 200 so Meta
		// does not retry; everything else returns 400.
		if errors.Is(err, ErrEmptyDelivery) {
			h.logger.Info("whatsapp webhook empty delivery", "kind", "whatsapp_webhook_empty")
			w.WriteHeader(http.StatusOK)
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid_payload")
		return
	}

	ctx := r.Context()
	canonical, err := h.adapter.Translate(ctx, msg)
	if err != nil {
		// Translate folds identity resolution into the canonical
		// message — distinguish the two failure classes for ops.
		switch {
		case errors.Is(err, transportidentity.ErrUnknownSubject):
			webhookIdentityFailures.WithLabelValues("unknown").Inc()
			writeJSONError(w, http.StatusForbidden, "unknown_subject")
		case errors.Is(err, ErrUnsupportedMessageType):
			webhookParseFailures.WithLabelValues("unsupported").Inc()
			writeJSONError(w, http.StatusBadRequest, "unsupported_type")
		default:
			webhookIdentityFailures.WithLabelValues("invalid").Inc()
			writeJSONError(w, http.StatusBadRequest, "translate_failed")
		}
		return
	}

	if !h.adapter.IsBound() {
		// Facade not yet wired — log and ack 200 so Meta does not
		// retry. Render is owned by SCOPE-2 in any case.
		h.logger.Warn("whatsapp webhook accepted without bound facade",
			"kind", "whatsapp_webhook_unbound",
			"message_kind", string(canonical.Kind),
			"transport_message_id", canonical.TransportMessageID,
		)
		webhookAccepted.Inc()
		w.WriteHeader(http.StatusOK)
		return
	}

	// SCOPE-3 idempotency: Meta retries the same delivery (same
	// `messages[].id`) until we ack 2xx. Swallow duplicates BEFORE
	// the facade and capture run so each real inbound message is
	// processed exactly once per adapter instance.
	if h.adapter.MarkSeen(canonical.TransportMessageID) {
		idempotentRetries.Inc()
		h.logger.Info("whatsapp webhook duplicate suppressed",
			"kind", "whatsapp_webhook_duplicate",
			"transport_message_id", canonical.TransportMessageID,
			"latency_ms", time.Since(start).Milliseconds(),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	resp, err := h.adapter.Assistant().Handle(ctx, canonical)
	if err != nil {
		h.logger.Error("whatsapp facade handle failed",
			"kind", "whatsapp_webhook_handle_error",
			"transport_message_id", canonical.TransportMessageID,
			"error", err.Error(),
		)
		// Still ack 200 to suppress Meta retry; capture-as-fallback
		// is the canonical durability path (spec 072 SCOPE-2).
	} else {
		// Spec 072 SCOPE-2 — honor capture-as-fallback BEFORE the
		// user-facing send so the artifact is durable even if the
		// Cloud API send fails.
		h.dispatchResponse(ctx, canonical, msg.From, resp)
	}

	webhookAccepted.Inc()
	h.logger.Info("whatsapp webhook accepted",
		"kind", "whatsapp_webhook",
		"message_kind", string(canonical.Kind),
		"transport_message_id", canonical.TransportMessageID,
		"body_len", len(body),
		"latency_ms", time.Since(start).Milliseconds(),
	)
	w.WriteHeader(http.StatusOK)
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + code + `"}`))
}

func remoteIPForLog(r *http.Request) string {
	if r == nil {
		return ""
	}
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

// AssistantHandler is the narrow interface the webhook handler uses
// from the bound facade — kept here so future tests can inject a
// recording fake without depending on the full contracts.Assistant.
type AssistantHandler interface {
	Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error)
}

// dispatchResponse honors CaptureRoute (capture-as-fallback) BEFORE
// rendering, then sends the rendered message through the Cloud API.
// Errors are logged but never re-surface to the Meta retry path —
// capture has already persisted the artifact.
func (h *webhookHandler) dispatchResponse(ctx context.Context, canonical contracts.AssistantMessage, toPhone string, resp contracts.AssistantResponse) {
	if resp.CaptureRoute {
		if !h.adapter.InvokeCapture(ctx, canonical) {
			h.logger.Warn("whatsapp capture-as-fallback skipped (no CaptureFn configured)",
				"kind", "whatsapp_capture_skipped",
				"transport_message_id", canonical.TransportMessageID,
			)
		}
	}
	if !h.adapter.HasCloud() {
		// SCOPE-1 wiring path — no outbound Cloud client yet. Log
		// and return; capture has already run if applicable.
		h.logger.Warn("whatsapp render skipped (no CloudClient configured)",
			"kind", "whatsapp_render_skipped",
			"transport_message_id", canonical.TransportMessageID,
		)
		return
	}
	if err := h.adapter.RenderToPhone(ctx, toPhone, resp); err != nil {
		if errors.Is(err, ErrNothingToRender) {
			// Empty silent-capture path — already captured above; no
			// user-facing message is sent on purpose.
			return
		}
		h.logger.Error("whatsapp render failed",
			"kind", "whatsapp_render_error",
			"transport_message_id", canonical.TransportMessageID,
			"error", err.Error(),
		)
	}
}

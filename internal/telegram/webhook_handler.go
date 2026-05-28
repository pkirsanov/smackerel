// Spec 061 SCOPE-05 design §17 — Telegram webhook ingress (Option A,
// Transport Entry-Point Capability Foundation, second variant alongside
// the existing long-poll tgbotapi.GetUpdatesChan ingress in Bot.Start).
//
// This file ships a thin HTTP handler that authenticates a single
// Telegram-shaped POST via constant-time compare on the
// X-Telegram-Bot-Api-Secret-Token header, JSON-unmarshals the body
// into a *tgbotapi.Update, and dispatches it through the SAME
// Bot.safeHandleMessage / Bot.safeHandleCallback path the long-poll
// loop already uses. Zero changes to the assistant adapter or the
// spec 037 substrate.
//
// NO request-body content is logged (Principle 8 + spec 061 PII
// discipline). Only the structured envelope fields (update_id,
// chat_id, kind, latency_ms) are emitted on the success path.
package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// WebhookHeaderSecretToken is the Telegram-defined header the bot API
// sends on every webhook delivery when setWebhook was called with a
// secret_token. Per Telegram's bot API docs this is the canonical
// shared-secret-header pattern; we ENFORCE it on every request and
// fail with 401 on missing or mismatched values.
const WebhookHeaderSecretToken = "X-Telegram-Bot-Api-Secret-Token"

// WebhookMaxBodyBytes caps the request body size at 1 MiB. Telegram
// updates are bounded by the bot API protocol, so this is a defensive
// guard against arbitrarily large bodies (e.g., from a leaked-secret
// attacker) rather than an SST knob. Oversize bodies are rejected with
// 413 Payload Too Large before any JSON parse work runs.
const WebhookMaxBodyBytes = 1 << 20 // 1 MiB

// Webhook observability counters. Registration lives in
// metrics_webhook.go (sibling) to keep the prometheus import cost off
// callers that vendor this package without metrics.
var (
	webhookAuthFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_telegram_webhook_auth_failures_total",
			Help: "Telegram webhook deliveries rejected for missing or mismatched X-Telegram-Bot-Api-Secret-Token (labels: reason=missing|mismatch).",
		},
		[]string{"reason"},
	)
	webhookParseFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assistant_telegram_webhook_parse_failures_total",
			Help: "Telegram webhook deliveries rejected for invalid or empty request body (labels: reason=invalid_json|empty|oversize).",
		},
		[]string{"reason"},
	)
)

func init() {
	prometheus.MustRegister(webhookAuthFailures, webhookParseFailures)
}

// WebhookHandlerOptions bundles the inputs NewWebhookHandler needs.
// Dispatcher defaults to the supplied Bot when nil; tests inject a
// recording fake satisfying the WebhookDispatcher interface to assert
// the §17.3 dispatch matrix without spinning up a full bot.
type WebhookHandlerOptions struct {
	Bot        *Bot
	Dispatcher WebhookDispatcher
	Secret     string // resolved value (NOT the SST ref); see config.AssistantConfig.TelegramWebhookSecret
	Logger     *slog.Logger
}

// WebhookDispatcher is the narrow interface the webhook handler uses
// to dispatch a parsed update. *Bot satisfies it via safeHandleMessage
// / safeHandleCallback; the test suite injects a recording fake.
type WebhookDispatcher interface {
	DispatchMessage(ctx context.Context, msg *tgbotapi.Message)
	DispatchCallback(ctx context.Context, cb *tgbotapi.CallbackQuery)
}

// DispatchMessage satisfies WebhookDispatcher by delegating to the
// same panic-guarded handler the long-poll loop uses.
func (b *Bot) DispatchMessage(ctx context.Context, msg *tgbotapi.Message) {
	b.safeHandleMessage(ctx, msg)
}

// DispatchCallback satisfies WebhookDispatcher.
func (b *Bot) DispatchCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	b.safeHandleCallback(ctx, cb)
}

// NewWebhookHandler returns an http.Handler that implements the
// design §17.3 behavior matrix. Construction panics if Secret is empty
// — that is a MUST-have, and a silent fallback would silently
// authorize every POST. Either Bot or Dispatcher MUST be supplied;
// when both are present Dispatcher wins (test override path).
func NewWebhookHandler(opts WebhookHandlerOptions) http.Handler {
	if opts.Secret == "" {
		panic("telegram.NewWebhookHandler: Secret is required (spec 061 SCOPE-05 §17.3 — empty secret would silently authorize every POST)")
	}
	dispatcher := opts.Dispatcher
	if dispatcher == nil {
		if opts.Bot == nil {
			panic("telegram.NewWebhookHandler: Bot or Dispatcher is required (spec 061 SCOPE-05 §17.3)")
		}
		dispatcher = opts.Bot
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &webhookHandler{
		dispatcher: dispatcher,
		secret:     opts.Secret,
		logger:     logger,
	}
}

type webhookHandler struct {
	dispatcher WebhookDispatcher
	secret     string
	logger     *slog.Logger
}

func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	remote := remoteIPForLog(r)

	// Method gate — POST only (design §17.3).
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Auth gate — X-Telegram-Bot-Api-Secret-Token MUST be present and
	// MUST match via subtle.ConstantTimeCompare. Missing and mismatch
	// share status 401 but carry distinct counters so operators can
	// distinguish scanners from spoof attempts.
	provided := r.Header.Get(WebhookHeaderSecretToken)
	if provided == "" {
		webhookAuthFailures.WithLabelValues("missing").Inc()
		h.logger.Warn("telegram webhook auth failed",
			"kind", "telegram_webhook_auth_fail",
			"reason", "missing",
			"remote_ip", remote,
		)
		writeJSONError(w, http.StatusUnauthorized, "missing_secret_token")
		return
	}
	// subtle.ConstantTimeCompare returns 1 only when lengths match and
	// every byte matches; it returns 0 if lengths differ. This rejects
	// the obvious early-return timing attack a naive `==` would expose.
	if subtle.ConstantTimeCompare([]byte(provided), []byte(h.secret)) != 1 {
		webhookAuthFailures.WithLabelValues("mismatch").Inc()
		h.logger.Warn("telegram webhook auth failed",
			"kind", "telegram_webhook_auth_fail",
			"reason", "mismatch",
			"remote_ip", remote,
		)
		writeJSONError(w, http.StatusUnauthorized, "invalid_secret_token")
		return
	}

	// Body cap — wrap in http.MaxBytesReader so an oversize body short-
	// circuits during read rather than after a full buffer copy.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, WebhookMaxBodyBytes))
	if err != nil {
		// MaxBytesError is the only error io.ReadAll surfaces here in
		// practice; the http library writes its own 413 to w but we
		// keep the counter + log structured.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			webhookParseFailures.WithLabelValues("oversize").Inc()
			h.logger.Info("telegram webhook rejected",
				"kind", "telegram_webhook_parse_fail",
				"reason", "oversize",
				"limit_bytes", WebhookMaxBodyBytes,
			)
			// MaxBytesReader already wrote the 413; ensure we still
			// emit the structured body so curl --fail-with-body shows it.
			writeJSONError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
			return
		}
		webhookParseFailures.WithLabelValues("invalid_json").Inc()
		h.logger.Info("telegram webhook rejected",
			"kind", "telegram_webhook_parse_fail",
			"reason", "read_error",
			"error", err.Error(),
		)
		writeJSONError(w, http.StatusBadRequest, "invalid_update_json")
		return
	}
	if len(body) == 0 {
		webhookParseFailures.WithLabelValues("empty").Inc()
		h.logger.Info("telegram webhook rejected",
			"kind", "telegram_webhook_parse_fail",
			"reason", "empty",
		)
		writeJSONError(w, http.StatusBadRequest, "empty_request_body")
		return
	}

	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		webhookParseFailures.WithLabelValues("invalid_json").Inc()
		h.logger.Info("telegram webhook rejected",
			"kind", "telegram_webhook_parse_fail",
			"reason", "invalid_json",
			// Deliberately NOT including the body or the error
			// content beyond err.Error() to avoid leaking PII.
			"error", err.Error(),
		)
		writeJSONError(w, http.StatusBadRequest, "invalid_update_json")
		return
	}

	// Dispatch via the same safeHandle* paths the long-poll loop uses.
	// Both wrap a panic guard so a single malformed update cannot
	// crash the http server goroutine.
	ctx := r.Context()
	kind := "other"
	var chatID int64
	switch {
	case update.CallbackQuery != nil:
		kind = "callback"
		if update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat != nil {
			chatID = update.CallbackQuery.Message.Chat.ID
		}
		h.dispatcher.DispatchCallback(ctx, update.CallbackQuery)
	case update.Message != nil:
		kind = "message"
		if update.Message.Chat != nil {
			chatID = update.Message.Chat.ID
		}
		h.dispatcher.DispatchMessage(ctx, update.Message)
	default:
		kind = "empty"
		// Nothing to dispatch; still return 200 so Telegram does not
		// retry with backoff.
	}

	h.logger.Info("telegram webhook accepted",
		"kind", "telegram_webhook",
		"message_kind", kind,
		"update_id", update.UpdateID,
		"chat_id", chatID,
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

// remoteIPForLog returns r.RemoteAddr with any port stripped. We do
// NOT consult X-Forwarded-For here because the webhook handler
// intentionally sits OUTSIDE the trusted-proxy middleware that runs
// inside chi (the webhook secret IS the authentication; logging IP is
// observational only). Spoofable headers stay out of the log.
func remoteIPForLog(r *http.Request) string {
	if r == nil {
		return ""
	}
	addr := r.RemoteAddr
	// Strip trailing :port if present (IPv4 or bracketed IPv6).
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

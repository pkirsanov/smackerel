package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// AssistantConfig holds the SST-resolved Conversational Assistant
// (spec 061) configuration. Every field is REQUIRED at the generator
// boundary; missing values produce a fail-loud error from
// loadAssistantConfig() invoked at the tail of Load().
//
// Design source: specs/061-conversational-assistant/design.md §7.1 +
// scopes.md SCOPE-01 (literal-values-in-smackerel.yaml convention; the
// ${VAR:?...} substitution form is reserved for deploy compose per
// Gate G028 / smackerel-no-defaults).
type AssistantConfig struct {
	// Enabled gates the entire capability layer. When false, the
	// Telegram bot continues through its existing handleTextCapture
	// path unchanged (BS-001 regression-safe path).
	Enabled bool

	// BorderlineFloor is the three-band routing post-processor floor
	// on agent.RoutingDecision.TopScore. MUST be strictly greater than
	// agent.routing.confidence_floor (validation rule #2).
	BorderlineFloor float64

	// ContextWindowTurns bounds the conversation window for context
	// reconstruction.
	ContextWindowTurns int

	// ContextIdleTimeout is the TTL after which an idle conversation
	// row is swept from assistant_conversations.
	ContextIdleTimeout time.Duration

	// ContextIdleSweepInterval is how often the idle sweeper runs.
	ContextIdleSweepInterval time.Duration

	// ContextStateKey selects the conversation primary-key shape.
	// "transport_user" (recommended) keys context by
	// (user_id, transport); "user" keys context by user_id alone and
	// emits a startup WARN log (validation rule #4).
	ContextStateKey string

	// SourcesMax is the per-response cap on Source entries.
	SourcesMax int

	// BodyMaxChars is the transport-agnostic body cap.
	BodyMaxChars int

	// StatusMaxDuration is the longest time a status token may be
	// displayed before the transport substitutes a fallback.
	StatusMaxDuration time.Duration

	// DisambiguateTimeout is the TTL of a disambiguation prompt before
	// it is discarded.
	DisambiguateTimeout time.Duration

	// ErrorCaptureTimeout is the TTL of an offer-to-capture confirm
	// card shown on error.
	ErrorCaptureTimeout time.Duration

	// Per-skill rate limits (requests per minute).
	RateLimitRetrievalRPM     int
	RateLimitWeatherRPM       int
	RateLimitNotificationsRPM int

	// Skills.
	RetrievalEnabled bool
	RetrievalTopK    int

	WeatherEnabled  bool
	WeatherProvider string
	// WeatherAPIKeyRef may be empty when the chosen provider does not
	// require an API key.
	WeatherAPIKeyRef string
	WeatherCacheTTL  time.Duration

	NotificationsEnabled        bool
	NotificationsConfirmTimeout time.Duration

	// Transports.
	TelegramEnabled         bool
	TelegramMarkdownMode    string
	TelegramMaxMessageChars int

	// Spec 061 SCOPE-05 design §17 — Telegram webhook mode.
	// TelegramMode is the transport ingress mode ("long_poll" | "webhook").
	// TelegramWebhookSecretRef is the name of the env var that holds the
	// webhook shared secret (Infisical-style indirection); validated
	// non-empty when TelegramMode == "webhook".
	// TelegramWebhookSecret is the resolved secret (read from
	// os.Getenv(TelegramWebhookSecretRef) at config-load time when
	// TelegramMode == "webhook"); the webhook handler uses this value
	// directly for constant-time compare.
	// TelegramWebhookPath is the chi route path; MUST start with "/".
	TelegramMode             string
	TelegramWebhookSecretRef string
	TelegramWebhookSecret    string
	TelegramWebhookPath      string

	// Spec 061 SCOPE-10 — offline evaluation harness acceptance gates.
	// Read from ASSISTANT_EVAL_* env vars. Consumed by the harness in
	// tests/eval/assistant/harness.go to fail the acceptance suite when
	// either threshold is missed.
	Eval AssistantEvalConfig
}

// AssistantEvalConfig holds the spec 061 SCOPE-10 offline evaluation
// harness thresholds. Both fields are REQUIRED at the SST boundary.
// design.md §13 names this "Acceptance Gate".
type AssistantEvalConfig struct {
	// RoutingAccuracyMin is the minimum fraction of corpus rows whose
	// ground_truth_intent matches the facade's selected scenario_id.
	// Spec 061 §17 contracts this at 0.85; lowering is a regression.
	RoutingAccuracyMin float64
	// CaptureFallbackMin is the minimum fraction of capture-expected
	// rows that took a capture path. Spec 061 §17 contracts this at
	// 1.0 (MUST capture every time the ground truth says so).
	CaptureFallbackMin float64
}

// loadAssistantConfig populates cfg.Assistant from ASSISTANT_* env vars
// and validates every required value is present + every constraint from
// design §7.2 (rules #1–#4) holds. Rule #5 (skill reachability) and
// rule #6 (scenario YAMLs present) are validated by their downstream
// scope owners (SCOPE-03/06/07/08) at registration time — SCOPE-01
// ships the validator hook surface in the form of the per-skill
// *Enabled fields, which the downstream scopes consult to decide
// whether to register their predicate.
func loadAssistantConfig(cfg *Config) error {
	var errs []string

	mustBool := func(key string, dst *bool) {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, key)
			return
		}
		*dst = v == "true"
	}
	mustString := func(key string, dst *string) {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, key)
			return
		}
		*dst = v
	}
	// permissiveString accepts empty values — used for the lone
	// optional ASSISTANT_SKILLS_WEATHER_API_KEY_REF field, which is
	// legitimately empty when the chosen provider requires no key.
	permissiveString := func(key string, dst *string) {
		// os.Getenv on a missing var also returns "" — we still treat
		// "missing entirely" as a config error, so check via LookupEnv.
		v, ok := os.LookupEnv(key)
		if !ok {
			errs = append(errs, key)
			return
		}
		*dst = v
	}
	mustInt := func(key string, dst *int, minVal int) {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, key)
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
			return
		}
		if n < minVal {
			errs = append(errs, fmt.Sprintf("%s (must be >= %d, got %d)", key, minVal, n))
			return
		}
		*dst = n
	}
	mustFloat := func(key string, dst *float64) {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, key)
			return
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (must be a float, got %q)", key, v))
			return
		}
		*dst = f
	}
	mustDuration := func(key string, dst *time.Duration) {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, key)
			return
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (must be a Go duration, got %q)", key, v))
			return
		}
		if d <= 0 {
			errs = append(errs, fmt.Sprintf("%s (must be > 0, got %s)", key, d))
			return
		}
		*dst = d
	}

	mustBool("ASSISTANT_ENABLED", &cfg.Assistant.Enabled)
	mustFloat("ASSISTANT_BORDERLINE_FLOOR", &cfg.Assistant.BorderlineFloor)
	mustInt("ASSISTANT_CONTEXT_WINDOW_TURNS", &cfg.Assistant.ContextWindowTurns, 1)
	mustDuration("ASSISTANT_CONTEXT_IDLE_TIMEOUT", &cfg.Assistant.ContextIdleTimeout)
	mustDuration("ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL", &cfg.Assistant.ContextIdleSweepInterval)
	mustString("ASSISTANT_CONTEXT_STATE_KEY", &cfg.Assistant.ContextStateKey)
	mustInt("ASSISTANT_SOURCES_MAX", &cfg.Assistant.SourcesMax, 1)
	mustInt("ASSISTANT_BODY_MAX_CHARS", &cfg.Assistant.BodyMaxChars, 1)
	mustDuration("ASSISTANT_STATUS_MAX_DURATION", &cfg.Assistant.StatusMaxDuration)
	mustDuration("ASSISTANT_DISAMBIGUATE_TIMEOUT", &cfg.Assistant.DisambiguateTimeout)
	mustDuration("ASSISTANT_ERROR_CAPTURE_TIMEOUT", &cfg.Assistant.ErrorCaptureTimeout)
	mustInt("ASSISTANT_RATE_LIMIT_RETRIEVAL_RPM", &cfg.Assistant.RateLimitRetrievalRPM, 1)
	mustInt("ASSISTANT_RATE_LIMIT_WEATHER_RPM", &cfg.Assistant.RateLimitWeatherRPM, 1)
	mustInt("ASSISTANT_RATE_LIMIT_NOTIFICATIONS_RPM", &cfg.Assistant.RateLimitNotificationsRPM, 1)
	mustBool("ASSISTANT_SKILLS_RETRIEVAL_ENABLED", &cfg.Assistant.RetrievalEnabled)
	mustInt("ASSISTANT_SKILLS_RETRIEVAL_TOP_K", &cfg.Assistant.RetrievalTopK, 1)
	mustBool("ASSISTANT_SKILLS_WEATHER_ENABLED", &cfg.Assistant.WeatherEnabled)
	mustString("ASSISTANT_SKILLS_WEATHER_PROVIDER", &cfg.Assistant.WeatherProvider)
	permissiveString("ASSISTANT_SKILLS_WEATHER_API_KEY_REF", &cfg.Assistant.WeatherAPIKeyRef)
	mustDuration("ASSISTANT_SKILLS_WEATHER_CACHE_TTL", &cfg.Assistant.WeatherCacheTTL)
	mustBool("ASSISTANT_SKILLS_NOTIFICATIONS_ENABLED", &cfg.Assistant.NotificationsEnabled)
	mustDuration("ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT", &cfg.Assistant.NotificationsConfirmTimeout)
	mustBool("ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED", &cfg.Assistant.TelegramEnabled)
	mustString("ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE", &cfg.Assistant.TelegramMarkdownMode)
	mustInt("ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS", &cfg.Assistant.TelegramMaxMessageChars, 1)
	// Spec 061 SCOPE-05 design §17 — Telegram webhook mode SST keys.
	// `mode` and `webhook_path` are always REQUIRED (literal yaml has
	// them). `webhook_secret_ref` is permissively-empty when
	// mode=long_poll; validateAssistantConfig rules 7–10 enforce the
	// fail-loud non-empty resolution when mode=webhook.
	mustString("ASSISTANT_TRANSPORTS_TELEGRAM_MODE", &cfg.Assistant.TelegramMode)
	permissiveString("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF", &cfg.Assistant.TelegramWebhookSecretRef)
	mustString("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH", &cfg.Assistant.TelegramWebhookPath)

	// Spec 061 SCOPE-10 — offline evaluation harness acceptance gates.
	mustFloat("ASSISTANT_EVAL_ROUTING_ACCURACY_MIN", &cfg.Assistant.Eval.RoutingAccuracyMin)
	mustFloat("ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN", &cfg.Assistant.Eval.CaptureFallbackMin)

	if len(errs) > 0 {
		return fmt.Errorf("[F061-SST-MISSING] missing or invalid required assistant configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// validateAssistantConfig enforces the SCOPE-01-owned rules #2, #3, #4
// from design §7.2 (rule #1 — required values — is enforced inline by
// loadAssistantConfig above). Returns an error on any failure; logs a
// WARN on the rule #4 advisory.
func (c *Config) validateAssistantConfig() error {
	if !c.Assistant.Enabled {
		// When the capability is disabled there is no surface to
		// constrain. Skill-enable flags and transport flags remain
		// valid as part of the dehydrated configuration.
		return nil
	}
	// Rule #2 — borderline_floor MUST be strictly greater than
	// agent.routing.confidence_floor; equal-or-less would erase the
	// borderline band entirely.
	floorStr := os.Getenv("AGENT_ROUTING_CONFIDENCE_FLOOR")
	if floorStr == "" {
		return fmt.Errorf("AGENT_ROUTING_CONFIDENCE_FLOOR must be set so ASSISTANT_BORDERLINE_FLOOR can be validated against it (spec 061 design §7.2 rule #2)")
	}
	agentFloor, err := strconv.ParseFloat(floorStr, 64)
	if err != nil {
		return fmt.Errorf("AGENT_ROUTING_CONFIDENCE_FLOOR: invalid float %q: %w", floorStr, err)
	}
	if c.Assistant.BorderlineFloor <= agentFloor {
		return fmt.Errorf("ASSISTANT_BORDERLINE_FLOOR (%.4f) must be > AGENT_ROUTING_CONFIDENCE_FLOOR (%.4f); equal-or-less erases the borderline band (spec 061 design §7.2 rule #2)", c.Assistant.BorderlineFloor, agentFloor)
	}
	// Rule #3 — at least one transport MUST be enabled when the
	// capability is enabled.
	if !c.Assistant.TelegramEnabled {
		return fmt.Errorf("ASSISTANT_ENABLED=true requires at least one assistant.transports.*.enabled=true (spec 061 design §7.2 rule #3); only telegram is wired in v1 and ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED=false")
	}
	// Rule #4 — non-recommended state_key emits WARN.
	switch c.Assistant.ContextStateKey {
	case "transport_user":
		// recommended
	case "user":
		slog.Warn("assistant.context.state_key=\"user\" is non-recommended; cross-transport conversations may collide. Recommended value is \"transport_user\" (spec 061 design §7.2 rule #4).")
	default:
		return fmt.Errorf("ASSISTANT_CONTEXT_STATE_KEY must be one of \"transport_user\" | \"user\"; got %q", c.Assistant.ContextStateKey)
	}
	// Rule #5 (skill reachability) and rule #6 (scenario YAMLs
	// present) are owned by SCOPE-03/06/07/08 and injected as
	// concrete predicates at registration time. SCOPE-01 ships the
	// per-skill *Enabled fields above; downstream scopes read those
	// flags to decide whether to wire their reachability predicate.

	// Spec 061 SCOPE-05 design §17 §7.2 rules 7–10 — Telegram webhook
	// mode SST enforcement.
	switch c.Assistant.TelegramMode {
	case "long_poll":
		// No further constraint on webhook_secret_ref; webhook_path
		// still MUST start with "/" because the literal yaml requires
		// it. The path is unused in long_poll mode but kept valid so
		// switching modes only requires flipping `mode`.
		if !strings.HasPrefix(c.Assistant.TelegramWebhookPath, "/") {
			return fmt.Errorf("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH must start with \"/\"; got %q (spec 061 design §7.2 rule #9)", c.Assistant.TelegramWebhookPath)
		}
	case "webhook":
		// Rule #8a — webhook_secret_ref MUST resolve in env to a
		// non-empty string. We use os.Getenv-style indirection: the
		// SST key names the env var that holds the actual secret
		// (Infisical injection model). Empty resolution → abort.
		if c.Assistant.TelegramWebhookSecretRef == "" {
			return fmt.Errorf("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF must be set when mode=webhook (spec 061 design §7.2 rule #8)")
		}
		resolved := os.Getenv(c.Assistant.TelegramWebhookSecretRef)
		if resolved == "" {
			return fmt.Errorf("FATAL assistant config invalid: assistant.transports.telegram.webhook_secret_ref: empty resolved secret (env var %q is unset or empty; spec 061 design §7.2 rule #8)", c.Assistant.TelegramWebhookSecretRef)
		}
		c.Assistant.TelegramWebhookSecret = resolved
		// Rule #9 — webhook_path MUST start with "/".
		if !strings.HasPrefix(c.Assistant.TelegramWebhookPath, "/") {
			return fmt.Errorf("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH must start with \"/\"; got %q (spec 061 design §7.2 rule #9)", c.Assistant.TelegramWebhookPath)
		}
		// Rule #9 — webhook_path MUST NOT collide with an existing API
		// route. The full chi route tree is not available here, but
		// the canonical fixed paths registered in internal/api/router.go
		// are enumerated below so a misconfigured operator gets a
		// fail-loud error at startup before the router boots.
		for _, reserved := range reservedAPIRoutePrefixes {
			if c.Assistant.TelegramWebhookPath == reserved ||
				strings.HasPrefix(c.Assistant.TelegramWebhookPath, reserved+"/") {
				return fmt.Errorf("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH %q collides with reserved API route %q (spec 061 design §7.2 rule #9)", c.Assistant.TelegramWebhookPath, reserved)
			}
		}
	default:
		return fmt.Errorf("ASSISTANT_TRANSPORTS_TELEGRAM_MODE must be one of \"long_poll\" | \"webhook\"; got %q (spec 061 design §7.2 rule #7)", c.Assistant.TelegramMode)
	}
	// Rule #10 — switching modes requires process restart. This is
	// enforced by the fact that loadAssistantConfig + validate run
	// only at startup; there is no runtime swap path.

	// Spec 061 SCOPE-10 — acceptance-gate threshold range checks.
	// Both values are fractions in [0.0, 1.0]; out-of-range values
	// produce a fail-loud error so a typo (e.g., 85 instead of 0.85)
	// never silently inverts a gate.
	if c.Assistant.Eval.RoutingAccuracyMin < 0 || c.Assistant.Eval.RoutingAccuracyMin > 1 {
		return fmt.Errorf("ASSISTANT_EVAL_ROUTING_ACCURACY_MIN (%.4f) must be in [0.0, 1.0] (spec 061 SCOPE-10)", c.Assistant.Eval.RoutingAccuracyMin)
	}
	if c.Assistant.Eval.CaptureFallbackMin < 0 || c.Assistant.Eval.CaptureFallbackMin > 1 {
		return fmt.Errorf("ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN (%.4f) must be in [0.0, 1.0] (spec 061 SCOPE-10)", c.Assistant.Eval.CaptureFallbackMin)
	}
	return nil
}

// reservedAPIRoutePrefixes enumerates the static path prefixes already
// registered in internal/api/router.go that the Telegram webhook path
// MUST NOT collide with. This list is a conservative subset (Group
// blocks under /api are owned by bearer-auth and are not at risk for a
// /v1/... webhook path); it catches accidental misconfiguration like
// webhook_path: "/metrics" or "/api".
var reservedAPIRoutePrefixes = []string{
	"/api",
	"/metrics",
	"/readyz",
	"/ping",
	"/login",
	"/admin_ui_static",
	"/auth",
	"/v1/web",
}

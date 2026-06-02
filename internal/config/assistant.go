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
	// BUG-061-003 — recipe_search rate limit.
	RateLimitRecipeSearchRPM int

	// Skills.
	RetrievalEnabled bool
	RetrievalTopK    int

	// BUG-061-003 — recipe_search skill SST.
	RecipeSearchEnabled bool
	RecipeSearchTopK    int

	WeatherEnabled  bool
	WeatherProvider string
	// WeatherAPIKeyRef may be empty when the chosen provider does not
	// require an API key.
	WeatherAPIKeyRef string
	WeatherCacheTTL  time.Duration

	// Spec 061 design §18.3 — per-URL SST keys for the external-provider
	// URL injection seam. Both are REQUIRED; the test stack overrides
	// them to http://stub-providers:8080/v1/{search,forecast} via the
	// TARGET_ENV=test block in scripts/commands/config.sh. The
	// production-safety guard in validateAssistantConfig rejects startup
	// when any non-test environment contains the substring "stub-providers".
	WeatherGeocodeURL  string
	WeatherForecastURL string

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

	// Spec 072 SCOPE-1 — WhatsApp Business Cloud API transport SST.
	// All fields originate in assistant.transports.whatsapp.* in
	// smackerel.yaml. The *Ref fields name the env vars that hold
	// the actual secrets (Infisical-style indirection); the non-Ref
	// twin holds the resolved value populated by validateAssistantConfig
	// when WhatsappEnabled=true. When WhatsappEnabled=false the *Ref
	// fields may be empty and credential resolution is skipped.
	WhatsappEnabled                   bool
	WhatsappWebhookPath               string
	WhatsappPhoneNumberID             string
	WhatsappBusinessAccountID         string
	WhatsappWebhookVerifyTokenRef     string
	WhatsappWebhookVerifyToken        string
	WhatsappAppSecretRef              string
	WhatsappAppSecret                 string
	WhatsappAccessTokenRef            string
	WhatsappAccessToken               string
	WhatsappIdentityHashKeyRef        string
	WhatsappIdentityHashKey           string
	WhatsappAPIBaseURL                string
	WhatsappAPIVersion                string
	WhatsappRateLimitPerUserPerMinute int
	WhatsappMaxTextChars              int

	// Spec 061 SCOPE-10 — offline evaluation harness acceptance gates.
	// Read from ASSISTANT_EVAL_* env vars. Consumed by the harness in
	// tests/eval/assistant/harness.go to fail the acceptance suite when
	// either threshold is missed.
	Eval AssistantEvalConfig

	// Spec 061 SCOPE-09a (design §8.3.1 + §8.3.2 Step 1) — OTel SDK
	// substrate configuration. All three fields are REQUIRED at the
	// SST boundary; loadAssistantConfig reads OtelEnabled / OtelEndpoint
	// / OtelServiceName from ASSISTANT_OBSERVABILITY_OTEL_* env vars.
	// validateAssistantConfig enforces non-empty OtelEndpoint when
	// OtelEnabled=true (rule §7.2-OTel-A) and non-empty OtelServiceName
	// unconditionally (rule §7.2-OTel-B). Consumed by
	// internal/assistant/tracing.NewTracer in cmd/core/wiring.go.
	Observability AssistantObservabilityConfig

	// BUG-061-004 — routing embedder substrate. EmbedderMode selects
	// between "sidecar" (production: HTTP /embed on ML sidecar) and
	// "noop" (test/dev only: fixed unit vector — alphabetical tie-break
	// guaranteed). EmbedTimeout is the per-call timeout for the
	// sidecar HTTP request. Loaded from ASSISTANT_ROUTING_EMBEDDER_MODE
	// and ASSISTANT_ROUTING_EMBED_TIMEOUT_MS env vars.
	EmbedderMode string
	EmbedTimeout time.Duration

	// Spec 064 SCOPE-03 — open-ended knowledge agent SST.
	// Populated by LoadOpenKnowledge() (internal/config/openknowledge.go)
	// from ASSISTANT_OPEN_KNOWLEDGE_* env vars; wired in config.go Load().
	OpenKnowledge OpenKnowledgeConfig

	// Spec 068 SCOPE-1 — structured intent compiler SST.
	// Populated by loadIntentCompilerConfig (internal/config/
	// assistant_intent_compiler.go) at the tail of loadAssistantConfig.
	// All keys are REQUIRED at the generator boundary (Gate G028 /
	// smackerel-no-defaults); missing values fail loud at startup.
	IntentCompiler IntentCompilerConfig

	// Spec 071 SCOPE-01 — IntentTrace observability SST.
	IntentTrace IntentTraceConfig

	// Spec 065 SCOPE-1 — generic micro-tools SST.
	Tools AssistantToolsConfig

	// Spec 069 SCOPE-1c-bis — HTTP transport SST contract. Typed
	// block consumed by Scope 1d wiring (httpadapter.NewHTTPAdapter)
	// and Scope 2 middleware. Every key is REQUIRED at the generator
	// boundary (Gate G028 / smackerel-no-defaults); missing values
	// fail loud at startup.
	HTTP AssistantHTTPTransportConfig
}

// AssistantHTTPTransportConfig holds the spec 069 SCOPE-1c-bis SST
// envelope for the HTTP transport adapter. Field names follow the
// scope's exact HTTP*-prefix vocabulary so consumer call sites
// (`cfg.Assistant.HTTP.HTTPEnabled` etc.) read identically to the
// scope contract. Mapped to `httpadapter.HTTPTransportConfig` at
// wiring time (cmd/core/wiring_assistant_facade.go).
type AssistantHTTPTransportConfig struct {
	HTTPEnabled                   bool
	HTTPSchemaVersion             string
	HTTPBodySizeMaxBytes          int
	HTTPRateLimitPerUserPerMinute int
	HTTPConversationTTL           time.Duration
	HTTPRequiredScope             string
	HTTPCORSAllowedOrigins        []string
	HTTPTransportHintAllowlist    []string
	HTTPSharedUserID              string
}

// AssistantObservabilityConfig holds the spec 061 SCOPE-09a OTel SDK
// substrate SST values. design §8.3.1 + §8.3.2 Step 1.
type AssistantObservabilityConfig struct {
	// OtelEnabled gates whether NewTracer returns a real OTLP/gRPC
	// TracerProvider (true) or the no-op TracerProvider (false).
	OtelEnabled bool
	// OtelEndpoint is the OTLP/gRPC exporter target (e.g.
	// "smackerel-test-jaeger:4317"). MUST be non-empty when
	// OtelEnabled=true; permissively-empty when OtelEnabled=false.
	OtelEndpoint string
	// OtelServiceName is the OTel resource service.name attribute
	// stamped on every span. REQUIRED non-empty regardless of
	// OtelEnabled state — the no-op tracer still carries the
	// resource for consistency between configurations.
	OtelServiceName string
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
	mustInt("ASSISTANT_RATE_LIMIT_RECIPE_SEARCH_RPM", &cfg.Assistant.RateLimitRecipeSearchRPM, 1)
	mustBool("ASSISTANT_SKILLS_RETRIEVAL_ENABLED", &cfg.Assistant.RetrievalEnabled)
	mustInt("ASSISTANT_SKILLS_RETRIEVAL_TOP_K", &cfg.Assistant.RetrievalTopK, 1)
	mustBool("ASSISTANT_SKILLS_RECIPE_SEARCH_ENABLED", &cfg.Assistant.RecipeSearchEnabled)
	mustInt("ASSISTANT_SKILLS_RECIPE_SEARCH_TOP_K", &cfg.Assistant.RecipeSearchTopK, 1)
	mustBool("ASSISTANT_SKILLS_WEATHER_ENABLED", &cfg.Assistant.WeatherEnabled)
	mustString("ASSISTANT_SKILLS_WEATHER_PROVIDER", &cfg.Assistant.WeatherProvider)
	permissiveString("ASSISTANT_SKILLS_WEATHER_API_KEY_REF", &cfg.Assistant.WeatherAPIKeyRef)
	mustDuration("ASSISTANT_SKILLS_WEATHER_CACHE_TTL", &cfg.Assistant.WeatherCacheTTL)
	// Spec 061 design §18.3 — provider URLs are REQUIRED.
	mustString("ASSISTANT_SKILLS_WEATHER_GEOCODE_URL", &cfg.Assistant.WeatherGeocodeURL)
	mustString("ASSISTANT_SKILLS_WEATHER_FORECAST_URL", &cfg.Assistant.WeatherForecastURL)
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

	// Spec 072 SCOPE-1 — WhatsApp Business Cloud API transport SST.
	// All WhatsApp keys are permissively-loaded here so that callers
	// which build assistant config without the transport (disabled or
	// unset) do not fail. Fail-loud enforcement of required values
	// when ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED=true happens in
	// validateAssistantConfig (matches Telegram webhook_secret_ref
	// pattern and SCN-072-A06 contract: "enabled with missing access
	// token fails loud" — disabled state remains permissive).
	if v, ok := os.LookupEnv("ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED"); ok {
		cfg.Assistant.WhatsappEnabled = v == "true"
	}
	cfg.Assistant.WhatsappWebhookPath = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH")
	cfg.Assistant.WhatsappPhoneNumberID = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID")
	cfg.Assistant.WhatsappBusinessAccountID = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID")
	cfg.Assistant.WhatsappWebhookVerifyTokenRef = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF")
	cfg.Assistant.WhatsappAppSecretRef = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF")
	cfg.Assistant.WhatsappAccessTokenRef = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF")
	cfg.Assistant.WhatsappIdentityHashKeyRef = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF")
	cfg.Assistant.WhatsappAPIBaseURL = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL")
	cfg.Assistant.WhatsappAPIVersion = os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION")
	if v := os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Assistant.WhatsappRateLimitPerUserPerMinute = n
		} else {
			errs = append(errs, fmt.Sprintf("ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE (must be an integer, got %q)", v))
		}
	}
	if v := os.Getenv("ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Assistant.WhatsappMaxTextChars = n
		} else {
			errs = append(errs, fmt.Sprintf("ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS (must be an integer, got %q)", v))
		}
	}

	// Spec 061 SCOPE-10 — offline evaluation harness acceptance gates.
	mustFloat("ASSISTANT_EVAL_ROUTING_ACCURACY_MIN", &cfg.Assistant.Eval.RoutingAccuracyMin)
	mustFloat("ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN", &cfg.Assistant.Eval.CaptureFallbackMin)

	// Spec 061 SCOPE-09a (design §8.3.1 + §8.3.2 Step 1) — OTel SDK
	// substrate SST. otel_endpoint is permissively-empty here because
	// empty resolution is legal when otel_enabled=false; the
	// validator enforces non-empty when otel_enabled=true.
	mustBool("ASSISTANT_OBSERVABILITY_OTEL_ENABLED", &cfg.Assistant.Observability.OtelEnabled)
	permissiveString("ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT", &cfg.Assistant.Observability.OtelEndpoint)
	mustString("ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME", &cfg.Assistant.Observability.OtelServiceName)

	// BUG-061-004 — routing embedder SST.
	mustString("ASSISTANT_ROUTING_EMBEDDER_MODE", &cfg.Assistant.EmbedderMode)
	var embedTimeoutMs int
	mustInt("ASSISTANT_ROUTING_EMBED_TIMEOUT_MS", &embedTimeoutMs, 1)
	cfg.Assistant.EmbedTimeout = time.Duration(embedTimeoutMs) * time.Millisecond

	// Spec 068 SCOPE-1 — structured intent compiler SST (fail-loud).
	loadIntentCompilerConfig(cfg, &errs)

	// Spec 065 SCOPE-1 — generic micro-tools SST (fail-loud). Every
	// ASSISTANT_TOOLS_* key is REQUIRED; loadAssistantToolsConfig
	// appends missing/invalid keys into the shared errs slice so the
	// aggregate error names every offender at once.
	loadAssistantToolsConfig(cfg, &errs)

	// Spec 069 SCOPE-1c-bis — HTTP transport SST (fail-loud).
	loadAssistantHTTPTransportConfig(cfg, &errs)

	if len(errs) > 0 {
		return fmt.Errorf("[F061-SST-MISSING] missing or invalid required assistant configuration: %s", strings.Join(errs, ", "))
	}

	// Spec 061 SCOPE-09a (design §8.3.1 + §8.3.2 Step 1) — OTel SDK
	// substrate cross-field validation. Rule §7.2-OTel-B (service_name
	// non-empty) is already enforced above by mustString. Rule
	// §7.2-OTel-A — otel_endpoint MUST be non-empty when otel_enabled=true
	// (otherwise the OTLP/gRPC exporter has no target) — is enforced
	// here so an enabled-without-endpoint config aborts startup.
	if cfg.Assistant.Observability.OtelEnabled && cfg.Assistant.Observability.OtelEndpoint == "" {
		return fmt.Errorf("ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT must be non-empty when ASSISTANT_OBSERVABILITY_OTEL_ENABLED=true (spec 061 SCOPE-09a design §8.3.2 Step 1)")
	}
	return nil
}

// validateAssistantConfig enforces the SCOPE-01-owned rules #2, #3, #4
// from design §7.2 (rule #1 — required values — is enforced inline by
// loadAssistantConfig above). Returns an error on any failure; logs a
// WARN on the rule #4 advisory.
func (c *Config) validateAssistantConfig() error {
	// Spec 061 design §18.3 — production-safety guard runs BEFORE the
	// !Enabled early-return below: the threat model is a misrouted env
	// file leaking the test stub URLs into a production-class bundle.
	// Skill-enable state is irrelevant — the value being present in
	// SST at all is the leak signal. No bypass flag exists.
	stubSubstring := "stub-providers"
	urlKeys := map[string]string{
		"assistant.skills.weather.geocode_url":  c.Assistant.WeatherGeocodeURL,
		"assistant.skills.weather.forecast_url": c.Assistant.WeatherForecastURL,
	}
	if c.Environment != "test" {
		for key, val := range urlKeys {
			if strings.Contains(val, stubSubstring) {
				return fmt.Errorf("[F061-PROD-SAFETY] URL %s contains test-only %q reference (value=%q) but SMACKEREL_ENV=%q (only \"test\" permitted); reject startup", key, stubSubstring, val, c.Environment)
			}
		}
	}
	// Spec 061 SCOPE-09a OTel substrate validation (rules §7.2-OTel-A
	// and -B) lives in loadAssistantConfig — running it here too
	// would false-fire when validateAssistantConfig is invoked on a
	// pre-load Config (e.g. by the early cfg.Validate() in Load).
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

	// BUG-061-004 — embedder_mode whitelist + timeout sanity.
	// Skipped when both fields are zero-value (loader didn't run, e.g.
	// targeted unit tests that construct Config{} directly to exercise
	// other rules). Any real config-load flow populates both fields
	// unconditionally; production cannot satisfy this skip clause.
	if c.Assistant.EmbedderMode != "" || c.Assistant.EmbedTimeout != 0 {
		switch c.Assistant.EmbedderMode {
		case "sidecar", "noop":
		default:
			return fmt.Errorf("ASSISTANT_ROUTING_EMBEDDER_MODE must be one of \"sidecar\" | \"noop\"; got %q (BUG-061-004 D8)", c.Assistant.EmbedderMode)
		}
		if c.Assistant.EmbedTimeout <= 0 {
			return fmt.Errorf("ASSISTANT_ROUTING_EMBED_TIMEOUT_MS must be > 0; got %s (BUG-061-004 D3)", c.Assistant.EmbedTimeout)
		}
	}

	// Spec 072 SCOPE-1 — WhatsApp Business Cloud API transport.
	if c.Assistant.WhatsappWebhookPath != "" && !strings.HasPrefix(c.Assistant.WhatsappWebhookPath, "/") {
		return fmt.Errorf("ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH must start with \"/\"; got %q (spec 072 SCOPE-1)", c.Assistant.WhatsappWebhookPath)
	}
	if c.Assistant.WhatsappEnabled {
		requiredStrings := []struct {
			envName string
			val     string
		}{
			{"ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH", c.Assistant.WhatsappWebhookPath},
			{"ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID", c.Assistant.WhatsappPhoneNumberID},
			{"ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID", c.Assistant.WhatsappBusinessAccountID},
			{"ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL", c.Assistant.WhatsappAPIBaseURL},
			{"ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION", c.Assistant.WhatsappAPIVersion},
		}
		for _, r := range requiredStrings {
			if r.val == "" {
				return fmt.Errorf("%s must be set when whatsapp.enabled=true (spec 072 SCOPE-1)", r.envName)
			}
		}
		if c.Assistant.WhatsappRateLimitPerUserPerMinute < 1 {
			return fmt.Errorf("ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE must be >= 1 when whatsapp.enabled=true; got %d (spec 072 SCOPE-1)", c.Assistant.WhatsappRateLimitPerUserPerMinute)
		}
		if c.Assistant.WhatsappMaxTextChars < 1 {
			return fmt.Errorf("ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS must be >= 1 when whatsapp.enabled=true; got %d (spec 072 SCOPE-1)", c.Assistant.WhatsappMaxTextChars)
		}
		if !strings.HasPrefix(c.Assistant.WhatsappAPIBaseURL, "https://") {
			return fmt.Errorf("ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL must be HTTPS when whatsapp.enabled=true; got %q (spec 072 SCOPE-1)", c.Assistant.WhatsappAPIBaseURL)
		}
		whatsappRefs := []struct {
			sstKey  string
			envName string
			ref     string
			dst     *string
		}{
			{"access_token_ref", "ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF", c.Assistant.WhatsappAccessTokenRef, &c.Assistant.WhatsappAccessToken},
			{"app_secret_ref", "ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF", c.Assistant.WhatsappAppSecretRef, &c.Assistant.WhatsappAppSecret},
			{"webhook_verify_token_ref", "ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF", c.Assistant.WhatsappWebhookVerifyTokenRef, &c.Assistant.WhatsappWebhookVerifyToken},
			{"identity_hash_key_ref", "ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF", c.Assistant.WhatsappIdentityHashKeyRef, &c.Assistant.WhatsappIdentityHashKey},
		}
		for _, r := range whatsappRefs {
			if r.ref == "" {
				return fmt.Errorf("%s must be set when whatsapp.enabled=true (spec 072 SCOPE-1)", r.envName)
			}
			resolved := os.Getenv(r.ref)
			if resolved == "" {
				return fmt.Errorf("FATAL assistant config invalid: assistant.transports.whatsapp.%s: empty resolved secret (env var %q is unset or empty; spec 072 SCOPE-1)", r.sstKey, r.ref)
			}
			*r.dst = resolved
		}
	}
	if err := validateAssistantHTTPTransportConfig(c.Assistant.HTTP); err != nil {
		return err
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

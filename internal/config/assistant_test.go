package config

import (
	"os"
	"strings"
	"testing"
)

// envSet is a tiny helper that applies a map of env vars for the
// lifetime of a single test then restores prior values via t.Setenv.
func envSet(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

// minimalAssistantEnv returns the complete set of ASSISTANT_* keys
// needed for a clean loadAssistantConfig pass. Tests start from this
// set and mutate specific keys to drive a failure case.
func minimalAssistantEnv() map[string]string {
	return map[string]string{
		"ASSISTANT_ENABLED":                                "true",
		"ASSISTANT_BORDERLINE_FLOOR":                       "0.75",
		"ASSISTANT_CONTEXT_WINDOW_TURNS":                   "8",
		"ASSISTANT_CONTEXT_IDLE_TIMEOUT":                   "30m",
		"ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL":            "5m",
		"ASSISTANT_CONTEXT_STATE_KEY":                      "transport_user",
		"ASSISTANT_SOURCES_MAX":                            "5",
		"ASSISTANT_BODY_MAX_CHARS":                         "4000",
		"ASSISTANT_STATUS_MAX_DURATION":                    "60s",
		"ASSISTANT_DISAMBIGUATE_TIMEOUT":                   "2m",
		"ASSISTANT_ERROR_CAPTURE_TIMEOUT":                  "10s",
		"ASSISTANT_RATE_LIMIT_RETRIEVAL_RPM":               "30",
		"ASSISTANT_RATE_LIMIT_WEATHER_RPM":                 "20",
		"ASSISTANT_RATE_LIMIT_NOTIFICATIONS_RPM":           "10",
		"ASSISTANT_RATE_LIMIT_RECIPE_SEARCH_RPM":           "20",
		"ASSISTANT_SKILLS_RETRIEVAL_ENABLED":               "true",
		"ASSISTANT_SKILLS_RETRIEVAL_TOP_K":                 "8",
		"ASSISTANT_SKILLS_RECIPE_SEARCH_ENABLED":           "true",
		"ASSISTANT_SKILLS_RECIPE_SEARCH_TOP_K":             "8",
		"ASSISTANT_SKILLS_WEATHER_ENABLED":                 "false",
		"ASSISTANT_SKILLS_WEATHER_PROVIDER":                "open-meteo",
		"ASSISTANT_SKILLS_WEATHER_API_KEY_REF":             "",
		"ASSISTANT_SKILLS_WEATHER_CACHE_TTL":               "10m",
		"ASSISTANT_SKILLS_WEATHER_GEOCODE_URL":             "https://geocoding-api.open-meteo.com/v1/search",
		"ASSISTANT_SKILLS_WEATHER_FORECAST_URL":            "https://api.open-meteo.com/v1/forecast",
		"ASSISTANT_SKILLS_NOTIFICATIONS_ENABLED":           "false",
		"ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT":   "5m",
		"ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED":            "true",
		"ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE":      "MarkdownV2",
		"ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS":  "4096",
		"ASSISTANT_TRANSPORTS_TELEGRAM_MODE":               "long_poll",
		"ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF": "",
		"ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH":       "/v1/telegram/webhook",
		// Spec 061 SCOPE-10 — acceptance-gate thresholds.
		"ASSISTANT_EVAL_ROUTING_ACCURACY_MIN": "0.85",
		"ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN": "1.0",
		// Spec 061 SCOPE-09a — OTel SDK substrate SST.
		"ASSISTANT_OBSERVABILITY_OTEL_ENABLED":      "false",
		"ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT":     "",
		"ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME": "smackerel-core",
		// BUG-061-004 — routing embedder SST.
		"ASSISTANT_ROUTING_EMBEDDER_MODE":   "sidecar",
		"ASSISTANT_ROUTING_EMBED_TIMEOUT_MS": "500",
	}
}

// TestLoadAssistantConfig_HappyPath proves every required key is read
// into the typed struct and that BS-009 does NOT fire when all keys
// are present (regression: a fabricated missing-key error here would
// silently mask real broken config later).
func TestLoadAssistantConfig_HappyPath(t *testing.T) {
	envSet(t, minimalAssistantEnv())
	cfg := &Config{}
	if err := loadAssistantConfig(cfg); err != nil {
		t.Fatalf("loadAssistantConfig should succeed with full env, got: %v", err)
	}
	if !cfg.Assistant.Enabled {
		t.Errorf("Enabled: want true, got false")
	}
	if cfg.Assistant.BorderlineFloor != 0.75 {
		t.Errorf("BorderlineFloor: want 0.75, got %v", cfg.Assistant.BorderlineFloor)
	}
	if cfg.Assistant.ContextWindowTurns != 8 {
		t.Errorf("ContextWindowTurns: want 8, got %v", cfg.Assistant.ContextWindowTurns)
	}
	if cfg.Assistant.ContextStateKey != "transport_user" {
		t.Errorf("ContextStateKey: want transport_user, got %q", cfg.Assistant.ContextStateKey)
	}
	if cfg.Assistant.ContextIdleTimeout.Minutes() != 30 {
		t.Errorf("ContextIdleTimeout: want 30m, got %v", cfg.Assistant.ContextIdleTimeout)
	}
	if cfg.Assistant.WeatherProvider != "open-meteo" {
		t.Errorf("WeatherProvider: want open-meteo, got %q", cfg.Assistant.WeatherProvider)
	}
	if cfg.Assistant.WeatherAPIKeyRef != "" {
		t.Errorf("WeatherAPIKeyRef: want empty, got %q", cfg.Assistant.WeatherAPIKeyRef)
	}
	if cfg.Assistant.TelegramMaxMessageChars != 4096 {
		t.Errorf("TelegramMaxMessageChars: want 4096, got %v", cfg.Assistant.TelegramMaxMessageChars)
	}
	// Spec 061 SCOPE-10 — acceptance-gate field reads.
	if cfg.Assistant.Eval.RoutingAccuracyMin != 0.85 {
		t.Errorf("Eval.RoutingAccuracyMin: want 0.85, got %v", cfg.Assistant.Eval.RoutingAccuracyMin)
	}
	if cfg.Assistant.Eval.CaptureFallbackMin != 1.0 {
		t.Errorf("Eval.CaptureFallbackMin: want 1.0, got %v", cfg.Assistant.Eval.CaptureFallbackMin)
	}
}

// TestLoadAssistantConfig_MissingKey_BS009 proves the BS-009 contract
// from spec 061 §5 (NO-DEFAULTS): any single missing ASSISTANT_* key
// surfaces a fail-loud error tagged with the [F061-SST-MISSING] prefix
// and naming the offending key. Adversarial: tests every required key
// individually so a future regression that silently defaulted one key
// would surface here.
func TestLoadAssistantConfig_MissingKey_BS009(t *testing.T) {
	for key := range minimalAssistantEnv() {
		key := key
		if key == "ASSISTANT_SKILLS_WEATHER_API_KEY_REF" {
			// permissively-empty key — covered by the dedicated
			// TestLoadAssistantConfig_WeatherAPIKeyRef_PermissiveButRequired
			// test below, which proves the key MUST be set (to ""
			// or non-empty) but accepts empty value.
			continue
		}
		if key == "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF" {
			// Spec 061 SCOPE-05 design §17 — permissively-empty key.
			// May be empty when mode=long_poll; validation rule #8
			// enforces non-empty resolution when mode=webhook
			// (covered by TestValidateAssistantConfig_Rule8_*).
			continue
		}
		if key == "ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT" {
			// Spec 061 SCOPE-09a design §8.3.2 Step 1 — permissively-empty
			// key. May be empty when otel_enabled=false; rule §7.2-OTel-A
			// enforces non-empty when otel_enabled=true (covered by
			// TestLoadAssistantConfig_OtelRuleA_EndpointRequiredWhenEnabled
			// in observability_test.go).
			continue
		}
		t.Run(key, func(t *testing.T) {
			env := minimalAssistantEnv()
			delete(env, key)
			envSet(t, env)
			// the key must be unset, not empty, because envSet
			// ignores it. t.Setenv is per-test so prior runs
			// don't leak.
			t.Setenv(key, "")
			cfg := &Config{}
			err := loadAssistantConfig(cfg)
			if err == nil {
				t.Fatalf("expected error for missing %s, got nil", key)
			}
			if !strings.Contains(err.Error(), "[F061-SST-MISSING]") {
				t.Errorf("error should carry [F061-SST-MISSING] prefix; got: %v", err)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error should name the missing key %s; got: %v", key, err)
			}
		})
	}
}

// TestLoadAssistantConfig_WeatherAPIKeyRef_PermissiveButRequired
// proves the lone permissively-empty key still fails loud when missing
// from the environment entirely (LookupEnv path), but accepts an
// explicit empty value. This keeps the SST envelope honest while
// allowing providers that need no API key.
func TestLoadAssistantConfig_WeatherAPIKeyRef_PermissiveButRequired(t *testing.T) {
	t.Run("empty value accepted", func(t *testing.T) {
		envSet(t, minimalAssistantEnv())
		cfg := &Config{}
		if err := loadAssistantConfig(cfg); err != nil {
			t.Fatalf("empty api_key_ref should be accepted; got: %v", err)
		}
	})
	t.Run("missing entirely rejected", func(t *testing.T) {
		env := minimalAssistantEnv()
		envSet(t, env)
		// LookupEnv must return false: use Unsetenv via t.Setenv("",
		// "") won't work — use os.Unsetenv-via-t.Setenv-cleanup
		// indirection is awkward. Use Setenv then explicit unset via
		// a Cleanup that re-deletes; simplest: Setenv to "" then
		// Unsetenv directly. testing.T offers no direct Unsetenv but
		// t.Setenv("",...) is invalid. Workaround: call os.Unsetenv
		// after envSet so LookupEnv reports false; t.Cleanup is
		// already installed by t.Setenv so the prior value will be
		// restored automatically on test exit.
		mustUnset(t, "ASSISTANT_SKILLS_WEATHER_API_KEY_REF")
		cfg := &Config{}
		err := loadAssistantConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "ASSISTANT_SKILLS_WEATHER_API_KEY_REF") {
			t.Fatalf("expected missing-key error for ASSISTANT_SKILLS_WEATHER_API_KEY_REF; got: %v", err)
		}
	})
}

// TestValidateAssistantConfig_Rule2_BorderlineMustExceedAgentFloor
// proves design §7.2 rule #2: equal or less is REJECTED to preserve
// the borderline band.
func TestValidateAssistantConfig_Rule2_BorderlineMustExceedAgentFloor(t *testing.T) {
	cases := []struct {
		name       string
		borderline float64
		agentFloor string
		wantReject bool
		wantSubstr string
	}{
		{"strictly greater accepted", 0.75, "0.65", false, ""},
		{"equal rejected", 0.65, "0.65", true, "must be >"},
		{"less rejected", 0.50, "0.65", true, "must be >"},
		{"agent floor missing rejected", 0.75, "", true, "AGENT_ROUTING_CONFIDENCE_FLOOR must be set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", tc.agentFloor)
			cfg := &Config{Assistant: AssistantConfig{
				Enabled:             true,
				BorderlineFloor:     tc.borderline,
				TelegramEnabled:     true,
				ContextStateKey:     "transport_user",
				TelegramMode:        "long_poll",
				TelegramWebhookPath: "/v1/telegram/webhook",
			}}
			err := cfg.validateAssistantConfig()
			if tc.wantReject {
				if err == nil {
					t.Fatalf("want rejection, got nil")
				}
				if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantSubstr)
				}
			} else if err != nil {
				t.Fatalf("want accept, got error: %v", err)
			}
		})
	}
}

// TestValidateAssistantConfig_Rule3_EnabledRequiresATransport proves
// design §7.2 rule #3: enabled=true with zero transports is REJECTED.
// Adversarial: a regression that silently flipped TelegramEnabled to
// true here would mask a config-level disable bug.
func TestValidateAssistantConfig_Rule3_EnabledRequiresATransport(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	cfg := &Config{Assistant: AssistantConfig{
		Enabled:         true,
		BorderlineFloor: 0.75,
		ContextStateKey: "transport_user",
		TelegramEnabled: false, // <-- only transport in v1
	}}
	err := cfg.validateAssistantConfig()
	if err == nil {
		t.Fatalf("want rejection when no transport is enabled, got nil")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Errorf("error should call out the missing transport; got: %v", err)
	}
}

// TestValidateAssistantConfig_Rule4_StateKeyAdvisory proves design
// §7.2 rule #4: state_key="user" is ACCEPTED but logs a WARN; an
// unknown value is REJECTED.
func TestValidateAssistantConfig_Rule4_StateKeyAdvisory(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	base := AssistantConfig{
		Enabled:             true,
		BorderlineFloor:     0.75,
		TelegramEnabled:     true,
		TelegramMode:        "long_poll",
		TelegramWebhookPath: "/v1/telegram/webhook",
	}
	t.Run("transport_user recommended", func(t *testing.T) {
		c := &Config{Assistant: base}
		c.Assistant.ContextStateKey = "transport_user"
		if err := c.validateAssistantConfig(); err != nil {
			t.Fatalf("want accept, got: %v", err)
		}
	})
	t.Run("user advisory accepted", func(t *testing.T) {
		c := &Config{Assistant: base}
		c.Assistant.ContextStateKey = "user"
		if err := c.validateAssistantConfig(); err != nil {
			t.Fatalf("want accept (advisory), got: %v", err)
		}
	})
	t.Run("unknown rejected", func(t *testing.T) {
		c := &Config{Assistant: base}
		c.Assistant.ContextStateKey = "bogus"
		err := c.validateAssistantConfig()
		if err == nil || !strings.Contains(err.Error(), "ASSISTANT_CONTEXT_STATE_KEY") {
			t.Fatalf("want rejection naming the key, got: %v", err)
		}
	})
}

// TestValidateAssistantConfig_DisabledSkipsRuleChecks proves the
// dehydrated configuration is not over-validated: when Enabled is
// false, rule #2/#3/#4 are skipped (since there's no live surface to
// constrain). Adversarial: this guards against a future regression
// that would fail-loud on legitimately-disabled deployments.
func TestValidateAssistantConfig_DisabledSkipsRuleChecks(t *testing.T) {
	// Note: AGENT_ROUTING_CONFIDENCE_FLOOR deliberately UNSET to
	// prove the disabled-path does not even consult it.
	mustUnset(t, "AGENT_ROUTING_CONFIDENCE_FLOOR")
	cfg := &Config{Assistant: AssistantConfig{
		Enabled:         false,
		BorderlineFloor: 0.0,
		TelegramEnabled: false,
		ContextStateKey: "anything",
	}}
	if err := cfg.validateAssistantConfig(); err != nil {
		t.Fatalf("disabled assistant should skip rule checks, got: %v", err)
	}
}

// mustUnset deletes an env var so LookupEnv returns false, with a
// t.Cleanup restoring whatever was there before. testing.T offers no
// Unsetenv helper, so this thin wrapper keeps the call sites readable.
func mustUnset(t *testing.T, key string) {
	t.Helper()
	prior, hadPrior := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if hadPrior {
			_ = os.Setenv(key, prior)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// baseWebhookCfg returns a Config wired for assistant.enabled=true with
// passing rules #2/#3/#4 so the webhook-specific tests below isolate
// rules #7/#8/#9 cleanly.
func baseWebhookCfg(mode, ref, path string) *Config {
	return &Config{Assistant: AssistantConfig{
		Enabled:                  true,
		BorderlineFloor:          0.75,
		ContextStateKey:          "transport_user",
		TelegramEnabled:          true,
		TelegramMode:             mode,
		TelegramWebhookSecretRef: ref,
		TelegramWebhookPath:      path,
	}}
}

// TestValidateAssistantConfig_Rule7_ModeEnum proves the mode-enum rule
// rejects any value other than "long_poll" | "webhook". Adversarial:
// "Webhook" (capitalized) and "" both fail.
func TestValidateAssistantConfig_Rule7_ModeEnum(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	for _, mode := range []string{"", "Webhook", "longpoll", "polling", "http"} {
		c := baseWebhookCfg(mode, "", "/v1/telegram/webhook")
		err := c.validateAssistantConfig()
		if err == nil {
			t.Errorf("mode=%q: want error, got nil", mode)
			continue
		}
		if !strings.Contains(err.Error(), "rule #7") {
			t.Errorf("mode=%q: error should cite rule #7, got: %v", mode, err)
		}
	}
}

// TestValidateAssistantConfig_Rule8_WebhookSecretMustResolve proves
// mode=webhook fails fast when webhook_secret_ref is empty OR when the
// named env var resolves to empty. Adversarial: a future regression
// that accepted an empty secret would silently authorize every POST.
func TestValidateAssistantConfig_Rule8_WebhookSecretMustResolve(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")

	t.Run("empty_ref", func(t *testing.T) {
		c := baseWebhookCfg("webhook", "", "/v1/telegram/webhook")
		err := c.validateAssistantConfig()
		if err == nil || !strings.Contains(err.Error(), "rule #8") {
			t.Fatalf("empty ref: want rule #8 error, got: %v", err)
		}
	})

	t.Run("ref_resolves_empty", func(t *testing.T) {
		mustUnset(t, "BS001_WEBHOOK_SECRET_UNSET")
		c := baseWebhookCfg("webhook", "BS001_WEBHOOK_SECRET_UNSET", "/v1/telegram/webhook")
		err := c.validateAssistantConfig()
		if err == nil || !strings.Contains(err.Error(), "empty resolved secret") {
			t.Fatalf("unset env: want empty-resolved-secret error, got: %v", err)
		}
	})

	t.Run("ref_resolves_non_empty", func(t *testing.T) {
		t.Setenv("BS001_WEBHOOK_SECRET_OK", "real-secret-value")
		c := baseWebhookCfg("webhook", "BS001_WEBHOOK_SECRET_OK", "/v1/telegram/webhook")
		if err := c.validateAssistantConfig(); err != nil {
			t.Fatalf("resolved non-empty: want nil error, got: %v", err)
		}
		if c.Assistant.TelegramWebhookSecret != "real-secret-value" {
			t.Errorf("resolved secret not stored on cfg: got %q", c.Assistant.TelegramWebhookSecret)
		}
	})
}

// TestValidateAssistantConfig_Rule9_WebhookPath proves the path must
// start with "/" and must not collide with reserved API prefixes.
func TestValidateAssistantConfig_Rule9_WebhookPath(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	t.Setenv("BS001_WEBHOOK_SECRET_PATHTEST", "real-secret-value")

	t.Run("missing_leading_slash", func(t *testing.T) {
		c := baseWebhookCfg("webhook", "BS001_WEBHOOK_SECRET_PATHTEST", "v1/telegram/webhook")
		err := c.validateAssistantConfig()
		if err == nil || !strings.Contains(err.Error(), "rule #9") {
			t.Fatalf("missing slash: want rule #9 error, got: %v", err)
		}
	})

	t.Run("collides_with_api", func(t *testing.T) {
		c := baseWebhookCfg("webhook", "BS001_WEBHOOK_SECRET_PATHTEST", "/api")
		err := c.validateAssistantConfig()
		if err == nil || !strings.Contains(err.Error(), "collides") {
			t.Fatalf("collision: want collision error, got: %v", err)
		}
	})

	t.Run("collides_with_metrics", func(t *testing.T) {
		c := baseWebhookCfg("webhook", "BS001_WEBHOOK_SECRET_PATHTEST", "/metrics")
		err := c.validateAssistantConfig()
		if err == nil || !strings.Contains(err.Error(), "collides") {
			t.Fatalf("metrics collision: want collision error, got: %v", err)
		}
	})

	t.Run("long_poll_mode_still_validates_path_prefix", func(t *testing.T) {
		// long_poll skips the secret check but the path leading-slash
		// constraint is still enforced (literal yaml requires the key).
		c := baseWebhookCfg("long_poll", "", "no-leading-slash")
		err := c.validateAssistantConfig()
		if err == nil || !strings.Contains(err.Error(), "rule #9") {
			t.Fatalf("long_poll bad path: want rule #9 error, got: %v", err)
		}
	})
}

// TestLoadAssistantConfig_WeatherURLsRequired proves the new §18.3
// external-provider URL injection seam keys are required at the loader
// boundary — missing either ASSISTANT_SKILLS_WEATHER_GEOCODE_URL or
// ASSISTANT_SKILLS_WEATHER_FORECAST_URL produces a fail-loud error
// naming the exact missing key.
func TestLoadAssistantConfig_WeatherURLsRequired(t *testing.T) {
	for _, key := range []string{
		"ASSISTANT_SKILLS_WEATHER_GEOCODE_URL",
		"ASSISTANT_SKILLS_WEATHER_FORECAST_URL",
	} {
		t.Run(key, func(t *testing.T) {
			envSet(t, minimalAssistantEnv())
			_ = os.Unsetenv(key)
			cfg := &Config{}
			err := loadAssistantConfig(cfg)
			if err == nil {
				t.Fatalf("loadAssistantConfig should fail when %s is unset", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error should name %s, got: %v", key, err)
			}
		})
	}
}

// TestLoadAssistantConfig_WeatherURLsRoundTrip proves the loader writes
// the URLs to the typed struct verbatim — adversarial against a
// regression that hard-codes the production URLs and ignores the env.
func TestLoadAssistantConfig_WeatherURLsRoundTrip(t *testing.T) {
	env := minimalAssistantEnv()
	env["ASSISTANT_SKILLS_WEATHER_GEOCODE_URL"] = "http://stub-providers:8080/v1/search"
	env["ASSISTANT_SKILLS_WEATHER_FORECAST_URL"] = "http://stub-providers:8080/v1/forecast"
	envSet(t, env)
	cfg := &Config{}
	if err := loadAssistantConfig(cfg); err != nil {
		t.Fatalf("loadAssistantConfig: %v", err)
	}
	if cfg.Assistant.WeatherGeocodeURL != "http://stub-providers:8080/v1/search" {
		t.Errorf("WeatherGeocodeURL: want stub url, got %q", cfg.Assistant.WeatherGeocodeURL)
	}
	if cfg.Assistant.WeatherForecastURL != "http://stub-providers:8080/v1/forecast" {
		t.Errorf("WeatherForecastURL: want stub url, got %q", cfg.Assistant.WeatherForecastURL)
	}
}

// TestValidateAssistantConfig_StubProviders_ProductionSafetyGuard proves
// the design §18.3 adversarial guard refuses startup when a weather URL
// contains the test-only marker "stub-providers" outside
// Environment="test". Adversarial matrix: each URL alone, both URLs,
// multiple non-test environments. Tautology guard: the same stub URL
// MUST pass when Environment="test" (otherwise the guard would block
// the actual test stack and this test would be vacuously satisfied).
func TestValidateAssistantConfig_StubProviders_ProductionSafetyGuard(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	build := func(env, geocode, forecast string) *Config {
		return &Config{
			Environment: env,
			Assistant: AssistantConfig{
				Enabled:                  true,
				BorderlineFloor:          0.75,
				ContextStateKey:          "transport_user",
				TelegramEnabled:          true,
				TelegramMode:             "long_poll",
				TelegramWebhookSecretRef: "",
				TelegramWebhookPath:      "/v1/telegram/webhook",
				WeatherGeocodeURL:        geocode,
				WeatherForecastURL:       forecast,
				Eval:                     AssistantEvalConfig{RoutingAccuracyMin: 0.85, CaptureFallbackMin: 1.0},
			},
		}
	}
	prodURL := "https://api.open-meteo.com/v1/forecast"
	prodGeo := "https://geocoding-api.open-meteo.com/v1/search"
	stubGeo := "http://stub-providers:8080/v1/search"
	stubFcst := "http://stub-providers:8080/v1/forecast"

	cases := []struct {
		name      string
		env       string
		geocode   string
		forecast  string
		wantError bool
	}{
		{"production_with_stub_geocode_rejects", "production", stubGeo, prodURL, true},
		{"production_with_stub_forecast_rejects", "production", prodGeo, stubFcst, true},
		{"production_with_both_stub_rejects", "production", stubGeo, stubFcst, true},
		{"development_with_stub_rejects", "development", stubGeo, prodURL, true},
		{"empty_environment_with_stub_rejects", "", stubGeo, prodURL, true},
		{"production_with_real_urls_passes", "production", prodGeo, prodURL, false},
		// Tautology guard: stub URLs MUST be accepted in the test env,
		// otherwise the test stack itself would fail to boot.
		{"test_env_with_both_stub_passes", "test", stubGeo, stubFcst, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := build(tc.env, tc.geocode, tc.forecast)
			err := c.validateAssistantConfig()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected production-safety guard to reject env=%q with stub url, got nil", tc.env)
				}
				if !strings.Contains(err.Error(), "stub-providers") || !strings.Contains(err.Error(), "F061-PROD-SAFETY") {
					t.Errorf("error should name stub-providers and F061-PROD-SAFETY, got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error for env=%q with urls %q/%q, got: %v", tc.env, tc.geocode, tc.forecast, err)
			}
		})
	}
}

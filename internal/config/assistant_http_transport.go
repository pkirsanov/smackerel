package config

// Spec 069 SCOPE-1c-bis — HTTP transport SST contract loader.
//
// Every assistant.transports.http.* key is REQUIRED at the SST
// boundary (Gate G028 / smackerel-no-defaults). Missing or empty
// values fail loud, naming the offender. The two list-shaped keys
// (cors_allowed_origins and transport_hint_allowlist) MUST be
// PRESENT in the env (LookupEnv must succeed) but may resolve to
// an empty string when the SST list is empty; cross-field rules
// that require a non-empty list only fire when HTTPEnabled=true.
//
// Consumed by Scope 1d wiring (cmd/core/wiring_assistant_facade.go)
// which maps cfg.Assistant.HTTP into httpadapter.HTTPTransportConfig
// and Scope 2 middleware which reads the same struct.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

const (
	envHTTPEnabled                = "ASSISTANT_TRANSPORTS_HTTP_ENABLED"
	envHTTPSchemaVersion          = "ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION"
	envHTTPBodySizeMaxBytes       = "ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES"
	envHTTPRateLimitPerUserPerMin = "ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE"
	envHTTPConversationTTLSeconds = "ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS"
	envHTTPRequiredScope          = "ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE"
	envHTTPCORSAllowedOrigins     = "ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS"
	envHTTPTransportHintAllowlist = "ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST"
	envHTTPSharedUserID           = "ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID"
)

func loadAssistantHTTPTransportConfig(cfg *Config, errs *[]string) {
	mustBool := func(key string, dst *bool) {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			*errs = append(*errs, key)
			return
		}
		switch v {
		case "true":
			*dst = true
		case "false":
			*dst = false
		default:
			*errs = append(*errs, fmt.Sprintf("%s (must be \"true\"|\"false\", got %q)", key, v))
		}
	}
	mustString := func(key string, dst *string) {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			*errs = append(*errs, key)
			return
		}
		*dst = v
	}
	mustInt := func(key string, dst *int, minVal int) {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			*errs = append(*errs, key)
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
			return
		}
		if n < minVal {
			*errs = append(*errs, fmt.Sprintf("%s (must be >= %d, got %d)", key, minVal, n))
			return
		}
		*dst = n
	}
	mustPresent := func(key string) (string, bool) {
		v, ok := os.LookupEnv(key)
		if !ok {
			*errs = append(*errs, key)
			return "", false
		}
		return v, true
	}

	mustBool(envHTTPEnabled, &cfg.Assistant.HTTP.HTTPEnabled)
	mustString(envHTTPSchemaVersion, &cfg.Assistant.HTTP.HTTPSchemaVersion)
	mustInt(envHTTPBodySizeMaxBytes, &cfg.Assistant.HTTP.HTTPBodySizeMaxBytes, 1)
	mustInt(envHTTPRateLimitPerUserPerMin, &cfg.Assistant.HTTP.HTTPRateLimitPerUserPerMinute, 1)
	var ttlSeconds int
	mustInt(envHTTPConversationTTLSeconds, &ttlSeconds, 1)
	cfg.Assistant.HTTP.HTTPConversationTTL = time.Duration(ttlSeconds) * time.Second
	mustString(envHTTPRequiredScope, &cfg.Assistant.HTTP.HTTPRequiredScope)
	mustString(envHTTPSharedUserID, &cfg.Assistant.HTTP.HTTPSharedUserID)
	if v, ok := mustPresent(envHTTPCORSAllowedOrigins); ok {
		cfg.Assistant.HTTP.HTTPCORSAllowedOrigins = splitNonEmptyCSV(v)
	}
	if v, ok := mustPresent(envHTTPTransportHintAllowlist); ok {
		cfg.Assistant.HTTP.HTTPTransportHintAllowlist = splitNonEmptyCSV(v)
	}

	if cfg.Assistant.HTTP.HTTPSchemaVersion != "" && cfg.Assistant.HTTP.HTTPSchemaVersion != "v1" {
		*errs = append(*errs, fmt.Sprintf("%s (must be \"v1\", got %q)", envHTTPSchemaVersion, cfg.Assistant.HTTP.HTTPSchemaVersion))
	}
	if cfg.Assistant.HTTP.HTTPRequiredScope != "" {
		if err := auth.ValidateScopeName(cfg.Assistant.HTTP.HTTPRequiredScope); err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must match spec 060 scope grammar <surface>:<capabilities>: %v)", envHTTPRequiredScope, err))
		}
	}
}

// validateAssistantHTTPTransportConfig enforces enabled=true
// cross-field rules: when the transport is enabled, the hint
// allowlist MUST be non-empty. When enabled=false the empty
// allowlist is legal.
func validateAssistantHTTPTransportConfig(cfg AssistantHTTPTransportConfig) error {
	if !cfg.HTTPEnabled {
		return nil
	}
	if len(cfg.HTTPTransportHintAllowlist) == 0 {
		return fmt.Errorf("%s must be a non-empty list when ASSISTANT_TRANSPORTS_HTTP_ENABLED=true (spec 069 SCOPE-1c-bis)", envHTTPTransportHintAllowlist)
	}
	return nil
}

func splitNonEmptyCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

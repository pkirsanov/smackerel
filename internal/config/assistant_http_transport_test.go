// Spec 069 SCOPE-2 — TP-069-04: every assistant HTTP transport SST
// key fails loud at startup with a named error.
//
// The BS-009 sweep in assistant_test.go already covers the SCOPE-1a
// keys; this file focuses on the SCOPE-2-relevant subset and the
// validator-level cross-field rules (required_scope and
// transport_hint_allowlist must be non-empty when enabled=true).

package config

import (
	"strings"
	"testing"
)

// TestAssistantHTTPTransportConfigRequiresEverySSTKey proves the
// loader and validator together reject any missing or empty HTTP
// transport key with a named error. Adversarial: a future
// regression that silently defaults any of these keys would surface
// here.
func TestAssistantHTTPTransportConfigRequiresEverySSTKey(t *testing.T) {
	type tc struct {
		name        string
		env         map[string]string // mutation applied AFTER copying minimalAssistantEnv()
		mutate      func(env map[string]string)
		wantSubstrs []string
	}

	deleteKey := func(key string) func(map[string]string) {
		return func(env map[string]string) { delete(env, key) }
	}
	emptyKey := func(key string) func(map[string]string) {
		return func(env map[string]string) { env[key] = "" }
	}

	cases := []tc{
		{
			name:        "enabled_missing",
			mutate:      deleteKey("ASSISTANT_TRANSPORTS_HTTP_ENABLED"),
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_ENABLED", "[F061-SST-MISSING]"},
		},
		{
			name:        "schema_version_missing",
			mutate:      deleteKey("ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION"),
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION", "[F061-SST-MISSING]"},
		},
		{
			name:        "body_size_max_bytes_missing",
			mutate:      deleteKey("ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES"),
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES", "[F061-SST-MISSING]"},
		},
		{
			name:        "rate_limit_per_user_per_minute_missing",
			mutate:      deleteKey("ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE"),
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE", "[F061-SST-MISSING]"},
		},
		{
			name:        "conversation_ttl_seconds_missing",
			mutate:      deleteKey("ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS"),
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS", "[F061-SST-MISSING]"},
		},
		{
			name:        "required_scope_missing",
			mutate:      deleteKey("ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE"),
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE", "[F061-SST-MISSING]"},
		},
		{
			name:        "required_scope_empty_when_enabled",
			mutate:      emptyKey("ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE"),
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE", "[F061-SST-MISSING]"},
		},
		{
			name:        "schema_version_wrong_value",
			mutate:      func(env map[string]string) { env["ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION"] = "v2" },
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION", "v1"},
		},
		{
			name: "transport_hint_allowlist_empty_when_enabled",
			mutate: func(env map[string]string) {
				env["ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST"] = ""
			},
			wantSubstrs: []string{"ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST"},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			env := minimalAssistantEnv()
			c.mutate(env)
			envSet(t, env)
			// Defensive: ensure deleted keys are also explicitly unset
			// for the duration of this sub-test.
			for k := range minimalAssistantEnv() {
				if _, ok := env[k]; !ok {
					t.Setenv(k, "")
				}
			}
			t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
			cfg := &Config{}
			loadErr := loadAssistantConfig(cfg)
			var err error
			if loadErr != nil {
				err = loadErr
			} else {
				err = cfg.validateAssistantConfig()
			}
			if err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
			msg := err.Error()
			for _, sub := range c.wantSubstrs {
				if !strings.Contains(msg, sub) {
					t.Errorf("error must contain %q; got: %v", sub, err)
				}
			}
		})
	}
}

// TestAssistantHTTPTransportConfig_DisabledSkipsCrossFieldChecks
// proves the validator's enabled=true cross-field rules
// (required_scope non-empty, transport_hint_allowlist non-empty) do
// NOT fire when assistant.transports.http.enabled=false. The
// per-key BS-009 rules still apply regardless of the enabled flag —
// the loader-level mustString/mustBool/mustInt run unconditionally.
func TestAssistantHTTPTransportConfig_DisabledSkipsCrossFieldChecks(t *testing.T) {
	env := minimalAssistantEnv()
	env["ASSISTANT_TRANSPORTS_HTTP_ENABLED"] = "false"
	env["ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST"] = ""
	envSet(t, env)
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	cfg := &Config{}
	if err := loadAssistantConfig(cfg); err != nil {
		t.Fatalf("loadAssistantConfig should succeed with disabled HTTP transport, got: %v", err)
	}
	if err := cfg.validateAssistantConfig(); err != nil {
		t.Fatalf("validateAssistantConfig should skip enabled=true cross-field checks when disabled, got: %v", err)
	}
}

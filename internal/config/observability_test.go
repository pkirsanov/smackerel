package config

import (
	"strings"
	"testing"
)

// Spec 061 SCOPE-09a (design §8.3.1 + §8.3.2 Step 1) — OTel SDK
// substrate SST coverage. These tests are dedicated to the
// `assistant.observability.otel_*` triple so a regression to any one
// key (loader read, permissive-empty semantics, or validator rule
// §7.2-OTel-A/B) fails with a tightly-scoped error. The shared
// happy-path test in assistant_test.go intentionally does NOT assert
// these fields so a future generalization there cannot mask a
// regression here.

// TestLoadAssistantConfig_OtelObservability_HappyPath proves
// loadAssistantConfig round-trips all three OTel SST keys from env
// vars into the typed struct. Drives a non-default value for every
// field so a regression that hard-coded one wouldn't escape.
func TestLoadAssistantConfig_OtelObservability_HappyPath(t *testing.T) {
	env := minimalAssistantEnv()
	env["ASSISTANT_OBSERVABILITY_OTEL_ENABLED"] = "true"
	env["ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT"] = "smackerel-test-jaeger:4317"
	env["ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME"] = "smackerel-otel-test"
	envSet(t, env)
	cfg := &Config{}
	if err := loadAssistantConfig(cfg); err != nil {
		t.Fatalf("loadAssistantConfig: %v", err)
	}
	if !cfg.Assistant.Observability.OtelEnabled {
		t.Errorf("OtelEnabled: want true, got false")
	}
	if cfg.Assistant.Observability.OtelEndpoint != "smackerel-test-jaeger:4317" {
		t.Errorf("OtelEndpoint: want smackerel-test-jaeger:4317, got %q", cfg.Assistant.Observability.OtelEndpoint)
	}
	if cfg.Assistant.Observability.OtelServiceName != "smackerel-otel-test" {
		t.Errorf("OtelServiceName: want smackerel-otel-test, got %q", cfg.Assistant.Observability.OtelServiceName)
	}
}

// TestLoadAssistantConfig_OtelEndpoint_PermissiveButRequired proves
// the loader contract for ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT:
//
//   - explicit empty value ("") is accepted because the validator
//     enforces non-empty only when OtelEnabled=true (rule §7.2-OTel-A)
//     and a disabled deployment legitimately runs with an empty
//     endpoint;
//   - missing entirely from the environment (LookupEnv=false) is
//     REJECTED so the SST envelope stays honest.
//
// Adversarial: the test asserts both halves so a regression that
// silently defaulted the missing case to "" (collapsing the two paths)
// would be caught.
func TestLoadAssistantConfig_OtelEndpoint_PermissiveButRequired(t *testing.T) {
	t.Run("empty value accepted", func(t *testing.T) {
		env := minimalAssistantEnv()
		env["ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT"] = ""
		envSet(t, env)
		cfg := &Config{}
		if err := loadAssistantConfig(cfg); err != nil {
			t.Fatalf("explicit empty endpoint should be accepted; got: %v", err)
		}
		if cfg.Assistant.Observability.OtelEndpoint != "" {
			t.Errorf("OtelEndpoint: want empty string, got %q", cfg.Assistant.Observability.OtelEndpoint)
		}
	})
	t.Run("missing entirely rejected", func(t *testing.T) {
		envSet(t, minimalAssistantEnv())
		mustUnset(t, "ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT")
		cfg := &Config{}
		err := loadAssistantConfig(cfg)
		if err == nil {
			t.Fatalf("expected missing-key error for ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT, got nil")
		}
		if !strings.Contains(err.Error(), "ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT") {
			t.Errorf("error should name the missing key; got: %v", err)
		}
		if !strings.Contains(err.Error(), "[F061-SST-MISSING]") {
			t.Errorf("error should carry the [F061-SST-MISSING] prefix; got: %v", err)
		}
	})
}

// TestLoadAssistantConfig_OtelRuleA_EndpointRequiredWhenEnabled
// proves design §7.2-OTel-A: ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT
// MUST be non-empty when ASSISTANT_OBSERVABILITY_OTEL_ENABLED=true.
//
// The rule lives INLINE in loadAssistantConfig (after the mustString
// block) per the loader-comment contract: validateAssistantConfig is
// invoked twice — once via cfg.Validate() BEFORE the Observability
// fields are populated and once via Load() AFTER — so the inline-loader
// placement guarantees a single-fire validation against freshly-loaded
// fields. The tests therefore drive the rule via env vars + loader.
//
// Three-row adversarial matrix:
//
//   - enabled + empty endpoint → REJECT (the rule under test);
//   - enabled + non-empty endpoint → ACCEPT (tautology guard — proves
//     the rejection above is endpoint-driven, not enabled-driven);
//   - disabled + empty endpoint → ACCEPT (proves the rule only fires
//     when enabled, so flipping disabled deployments don't fail).
func TestLoadAssistantConfig_OtelRuleA_EndpointRequiredWhenEnabled(t *testing.T) {
	cases := []struct {
		name        string
		otelEnabled string
		endpoint    string
		wantReject  bool
	}{
		{"enabled_with_empty_endpoint_rejects", "true", "", true},
		{"enabled_with_non_empty_endpoint_accepts", "true", "smackerel-test-jaeger:4317", false},
		{"disabled_with_empty_endpoint_accepts", "false", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := minimalAssistantEnv()
			env["ASSISTANT_OBSERVABILITY_OTEL_ENABLED"] = tc.otelEnabled
			env["ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT"] = tc.endpoint
			envSet(t, env)
			cfg := &Config{}
			err := loadAssistantConfig(cfg)
			if tc.wantReject {
				if err == nil {
					t.Fatalf("want rejection, got nil")
				}
				if !strings.Contains(err.Error(), "ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT") {
					t.Errorf("error should name the SST key; got: %v", err)
				}
				if !strings.Contains(err.Error(), "ASSISTANT_OBSERVABILITY_OTEL_ENABLED=true") {
					t.Errorf("error should reference the enabled-true precondition; got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("want accept, got error: %v", err)
			}
		})
	}
}

// TestLoadAssistantConfig_OtelRuleB_ServiceNameAlwaysRequired
// proves design §7.2-OTel-B: ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME
// MUST be non-empty regardless of OtelEnabled state because the no-op
// TracerProvider still carries the service.name resource attribute for
// symmetric span shape.
//
// The service_name key uses `mustString` (NOT permissiveString) in the
// loader, so empty values fail at the mustString gate with the
// "[F061-SST-MISSING]" wrapper rather than the inline §7.2-OTel-B
// safety net. The inline rule is a regression net against a future
// loader refactor that switches to permissiveString; this test asserts
// the user-visible behavior (empty service_name is rejected at load
// time regardless of OtelEnabled), which is what the design contract
// guarantees.
func TestLoadAssistantConfig_OtelRuleB_ServiceNameAlwaysRequired(t *testing.T) {
	cases := []struct {
		name        string
		otelEnabled string
		serviceName string
		endpoint    string
		wantReject  bool
	}{
		{"disabled_with_empty_service_name_rejects", "false", "", "", true},
		{"enabled_with_empty_service_name_rejects", "true", "", "smackerel-test-jaeger:4317", true},
		{"disabled_with_non_empty_service_name_accepts", "false", "smackerel-core", "", false},
		{"enabled_with_non_empty_service_name_accepts", "true", "smackerel-core", "smackerel-test-jaeger:4317", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := minimalAssistantEnv()
			env["ASSISTANT_OBSERVABILITY_OTEL_ENABLED"] = tc.otelEnabled
			env["ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT"] = tc.endpoint
			env["ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME"] = tc.serviceName
			envSet(t, env)
			cfg := &Config{}
			err := loadAssistantConfig(cfg)
			if tc.wantReject {
				if err == nil {
					t.Fatalf("want rejection, got nil")
				}
				if !strings.Contains(err.Error(), "ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME") {
					t.Errorf("error should name the SST key; got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("want accept, got error: %v", err)
			}
		})
	}
}

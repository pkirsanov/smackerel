package config

import (
	"os"
	"strings"
	"testing"
)

// baseOpenKnowledgeEnv returns a map of all 12 ASSISTANT_OPEN_KNOWLEDGE_*
// env vars set to valid values that pass Validate() when Enabled=true.
// Individual subtests mutate one entry to exercise a single failure
// mode (G021 adversarial — every assertion must be one a regression
// would actually trip).
func baseOpenKnowledgeEnv() map[string]string {
	return map[string]string{
		"ASSISTANT_OPEN_KNOWLEDGE_ENABLED":                                 "true",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER":                                "brave",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT":                       "https://api.search.brave.com/res/v1/web/search",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY":                        "test-key",
		"ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID":                            "llama3.1:8b",
		"ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS":                          "8",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET":                  "8000",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET":                    "0.05",
		"ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD":                      "10.0",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD":             "1.0",
		"ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST":                          `["web_search","fetch_snippet"]`,
		"ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED":               "true",
		"ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS":                          "30000",
		"ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS":                    `[]`,
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD":       "5",
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS":     "60",
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS": "30",
		"ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE":               "shadow",
	}
}

func applyOpenKnowledgeEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestOpenKnowledgeConfig_HappyPath(t *testing.T) {
	applyOpenKnowledgeEnv(t, baseOpenKnowledgeEnv())
	cfg, err := LoadOpenKnowledge()
	if err != nil {
		t.Fatalf("expected happy path to load, got: %v", err)
	}
	if !cfg.Enabled || cfg.Provider != "brave" || cfg.MaxIterations != 8 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if len(cfg.ToolAllowlist) != 2 {
		t.Fatalf("expected 2 allowlist entries, got %d", len(cfg.ToolAllowlist))
	}
}

func TestOpenKnowledgeConfig_MissingEnvVars(t *testing.T) {
	// One subtest per required env var — unsetting it (LookupEnv=false)
	// MUST produce a typed F064-SST-MISSING error naming the key.
	keys := []string{
		"ASSISTANT_OPEN_KNOWLEDGE_ENABLED",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY",
		"ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID",
		"ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET",
		"ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD",
		"ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST",
		"ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED",
		"ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS",
		"ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS",
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD",
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS",
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS",
		"ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			env := baseOpenKnowledgeEnv()
			applyOpenKnowledgeEnv(t, env)
			// t.Setenv cannot unset; use os.Unsetenv directly + cleanup
			// to restore the value t.Setenv recorded for us.
			if err := os.Unsetenv(missing); err != nil {
				t.Fatalf("unset failed: %v", err)
			}
			_, err := LoadOpenKnowledge()
			if err == nil {
				t.Fatalf("expected error for missing %s", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("error should name %s, got: %v", missing, err)
			}
			if !strings.Contains(err.Error(), "F064-SST-MISSING") {
				t.Fatalf("expected [F064-SST-MISSING] tag, got: %v", err)
			}
		})
	}
}

func TestOpenKnowledgeConfig_EnabledStrictBool(t *testing.T) {
	// "1" must NOT be accepted as a bool — strict-bool contract.
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_ENABLED"] = "1"
	applyOpenKnowledgeEnv(t, env)
	_, err := LoadOpenKnowledge()
	if err == nil {
		t.Fatal("expected strict-bool rejection of \"1\"")
	}
	if !strings.Contains(err.Error(), "ASSISTANT_OPEN_KNOWLEDGE_ENABLED") {
		t.Fatalf("error should name ENABLED key, got: %v", err)
	}
}

func TestOpenKnowledgeConfig_ProviderEnum(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER"] = "bing"
	applyOpenKnowledgeEnv(t, env)
	_, err := LoadOpenKnowledge()
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "provider") {
		t.Fatalf("error should mention provider, got: %v", err)
	}
}

func TestOpenKnowledgeConfig_PositiveBudgets(t *testing.T) {
	cases := []struct {
		key string
		val string
	}{
		{"ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS", "0"},
		{"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET", "0"},
		{"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET", "0"},
	}
	for _, c := range cases {
		t.Run(c.key+"="+c.val, func(t *testing.T) {
			env := baseOpenKnowledgeEnv()
			env[c.key] = c.val
			applyOpenKnowledgeEnv(t, env)
			_, err := LoadOpenKnowledge()
			if err == nil {
				t.Fatalf("expected error for %s=%s", c.key, c.val)
			}
			if !strings.Contains(err.Error(), "> 0") {
				t.Fatalf("error should require > 0, got: %v", err)
			}
		})
	}
}

func TestOpenKnowledgeConfig_NonNegativeBudgets(t *testing.T) {
	cases := []string{
		"ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD",
	}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			env := baseOpenKnowledgeEnv()
			env[key] = "-0.01"
			applyOpenKnowledgeEnv(t, env)
			_, err := LoadOpenKnowledge()
			if err == nil {
				t.Fatalf("expected error for negative %s", key)
			}
			if !strings.Contains(err.Error(), ">= 0") {
				t.Fatalf("error should require >= 0, got: %v", err)
			}
		})
		t.Run(key+"_zero_ok", func(t *testing.T) {
			// Adversarial-positive: 0 MUST be accepted (>= 0 contract).
			env := baseOpenKnowledgeEnv()
			env[key] = "0"
			applyOpenKnowledgeEnv(t, env)
			if _, err := LoadOpenKnowledge(); err != nil {
				t.Fatalf("zero MUST be accepted for %s, got: %v", key, err)
			}
		})
	}
}

func TestOpenKnowledgeConfig_ToolAllowlistEmpty(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST"] = "[]"
	applyOpenKnowledgeEnv(t, env)
	_, err := LoadOpenKnowledge()
	if err == nil {
		t.Fatal("expected error for empty tool_allowlist")
	}
	if !strings.Contains(err.Error(), "tool_allowlist") {
		t.Fatalf("error should mention tool_allowlist, got: %v", err)
	}
}

func TestOpenKnowledgeConfig_BraveRequiresAPIKey(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER"] = "brave"
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY"] = "   "
	applyOpenKnowledgeEnv(t, env)
	_, err := LoadOpenKnowledge()
	if err == nil {
		t.Fatal("expected error for brave provider with empty api_key")
	}
	if !strings.Contains(err.Error(), "provider_api_key") {
		t.Fatalf("error should mention provider_api_key, got: %v", err)
	}
}

func TestOpenKnowledgeConfig_TavilyRequiresAPIKey(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER"] = "tavily"
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT"] = "https://api.tavily.com/search"
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY"] = ""
	applyOpenKnowledgeEnv(t, env)
	_, err := LoadOpenKnowledge()
	if err == nil {
		t.Fatal("expected error for tavily provider with empty api_key")
	}
	if !strings.Contains(err.Error(), "provider_api_key") {
		t.Fatalf("error should mention provider_api_key, got: %v", err)
	}
}

func TestOpenKnowledgeConfig_SearxngAcceptsEmptyAPIKey(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER"] = "searxng"
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT"] = "http://searxng.local/search"
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY"] = ""
	applyOpenKnowledgeEnv(t, env)
	cfg, err := LoadOpenKnowledge()
	if err != nil {
		t.Fatalf("searxng with empty api_key MUST load, got: %v", err)
	}
	if cfg.ProviderAPIKey != "" {
		t.Fatalf("expected empty api_key preserved")
	}
}

func TestOpenKnowledgeConfig_DisabledSkipsValidation(t *testing.T) {
	// Adversarial: when Enabled=false, every other field can be bogus
	// and Load MUST still succeed (operator can disable without filling
	// keys per design §SST).
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_ENABLED"] = "false"
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER"] = "totally-bogus"
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT"] = ""
	env["ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY"] = ""
	env["ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID"] = ""
	env["ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS"] = "0"
	env["ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST"] = "[]"
	applyOpenKnowledgeEnv(t, env)
	cfg, err := LoadOpenKnowledge()
	if err != nil {
		t.Fatalf("disabled config MUST load with bogus fields, got: %v", err)
	}
	if cfg.Enabled {
		t.Fatal("expected Enabled=false")
	}
}

func TestOpenKnowledgeConfig_InvalidJSONToolAllowlist(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST"] = "not json"
	applyOpenKnowledgeEnv(t, env)
	_, err := LoadOpenKnowledge()
	if err == nil {
		t.Fatal("expected invalid-json error")
	}
	if !strings.Contains(err.Error(), "TOOL_ALLOWLIST") {
		t.Fatalf("error should name TOOL_ALLOWLIST, got: %v", err)
	}
}

func TestOpenKnowledgeConfig_InvalidIntFloat(t *testing.T) {
	cases := map[string]string{
		"ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS":       "abc",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET": "not-a-float",
	}
	for key, bad := range cases {
		t.Run(key, func(t *testing.T) {
			env := baseOpenKnowledgeEnv()
			env[key] = bad
			applyOpenKnowledgeEnv(t, env)
			_, err := LoadOpenKnowledge()
			if err == nil {
				t.Fatalf("expected parse error for %s=%s", key, bad)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error should name %s, got: %v", key, err)
			}
		})
	}
}

// (no helpers; tests use os.Unsetenv directly.)

// TestOpenKnowledgeConfig_AllowedEgressHosts_HappyPath proves a
// well-formed bare-host list loads.
func TestOpenKnowledgeConfig_AllowedEgressHosts_HappyPath(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS"] = `["example.com","wiki.example.org"]`
	applyOpenKnowledgeEnv(t, env)
	cfg, err := LoadOpenKnowledge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.AllowedEgressHosts) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cfg.AllowedEgressHosts))
	}
}

// TestOpenKnowledgeConfig_AllowedEgressHosts_RejectsMalformedEntries
// — adversarial G021 + G028: a scheme, path, port, userinfo, or
// whitespace in any entry MUST fail Validate() loudly so a typo
// cannot become a silent allow-all when the runtime allowlist
// transport normalises it.
func TestOpenKnowledgeConfig_AllowedEgressHosts_RejectsMalformedEntries(t *testing.T) {
	cases := map[string]string{
		"with_scheme":   `["https://example.com"]`,
		"with_path":     `["example.com/foo"]`,
		"with_port":     `["example.com:8080"]`,
		"with_userinfo": `["user:pass@example.com"]`,
		"with_space":    `["bad host"]`,
		"empty_entry":   `[""]`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			env := baseOpenKnowledgeEnv()
			env["ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS"] = raw
			applyOpenKnowledgeEnv(t, env)
			_, err := LoadOpenKnowledge()
			if err == nil {
				t.Fatalf("expected validation error for %q", raw)
			}
			if !strings.Contains(err.Error(), "allowed_egress_hosts") {
				t.Fatalf("error should mention allowed_egress_hosts, got: %v", err)
			}
		})
	}
}

// TestOpenKnowledgeConfig_AllowedEgressHosts_EmptyAllowedWhenEnabled
// proves the deny-by-default contract: an empty list is permitted
// at the config layer (provider endpoint is implicit) — the runtime
// transport is what enforces deny-by-default against unallowed
// hosts.
func TestOpenKnowledgeConfig_AllowedEgressHosts_EmptyAllowedWhenEnabled(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS"] = `[]`
	applyOpenKnowledgeEnv(t, env)
	cfg, err := LoadOpenKnowledge()
	if err != nil {
		t.Fatalf("empty list MUST be valid, got: %v", err)
	}
	if len(cfg.AllowedEgressHosts) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(cfg.AllowedEgressHosts))
	}
}

// TestOpenKnowledgeConfig_CircuitBreaker_HappyPath proves the SCOPE-16
// SST surface loads + validates the three required circuit fields.
func TestOpenKnowledgeConfig_CircuitBreaker_HappyPath(t *testing.T) {
	applyOpenKnowledgeEnv(t, baseOpenKnowledgeEnv())
	cfg, err := LoadOpenKnowledge()
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}
	if cfg.CircuitBreaker.FailureThreshold != 5 ||
		cfg.CircuitBreaker.OpenWindowSeconds != 60 ||
		cfg.CircuitBreaker.HalfOpenAfterSeconds != 30 {
		t.Fatalf("CircuitBreaker=%+v want {5 60 30}", cfg.CircuitBreaker)
	}
}

// TestOpenKnowledgeConfig_CircuitBreaker_RejectsNonPositive — G028
// adversarial: zero / negative must be rejected with a typed
// [F064-SST-INVALID] error naming the offending key.
func TestOpenKnowledgeConfig_CircuitBreaker_RejectsNonPositive(t *testing.T) {
	cases := []struct {
		name      string
		env       string
		bad       string
		errSubstr string
	}{
		{"failure_threshold zero", "ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD", "0", "circuit_breaker.failure_threshold"},
		{"failure_threshold neg", "ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD", "-1", "circuit_breaker.failure_threshold"},
		{"open_window zero", "ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS", "0", "circuit_breaker.open_window_seconds"},
		{"open_window neg", "ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS", "-5", "circuit_breaker.open_window_seconds"},
		{"half_open zero", "ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS", "0", "circuit_breaker.half_open_after_seconds"},
		{"half_open neg", "ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS", "-7", "circuit_breaker.half_open_after_seconds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := baseOpenKnowledgeEnv()
			env[tc.env] = tc.bad
			applyOpenKnowledgeEnv(t, env)
			_, err := LoadOpenKnowledge()
			if err == nil {
				t.Fatalf("expected error for %s=%s", tc.env, tc.bad)
			}
			if !strings.Contains(err.Error(), tc.errSubstr) {
				t.Errorf("err=%v want substring %q", err, tc.errSubstr)
			}
			if !strings.Contains(err.Error(), "F064-SST-INVALID") {
				t.Errorf("err=%v want [F064-SST-INVALID] tag", err)
			}
		})
	}
}

// TestOpenKnowledgeConfig_CircuitBreaker_DisabledSkipsValidation —
// when Enabled=false, deep validation is skipped (consistent with
// the rest of the SST block). Zero values are tolerated.
func TestOpenKnowledgeConfig_CircuitBreaker_DisabledSkipsValidation(t *testing.T) {
	env := baseOpenKnowledgeEnv()
	env["ASSISTANT_OPEN_KNOWLEDGE_ENABLED"] = "false"
	env["ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD"] = "0"
	env["ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS"] = "0"
	env["ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS"] = "0"
	applyOpenKnowledgeEnv(t, env)
	if _, err := LoadOpenKnowledge(); err != nil {
		t.Fatalf("disabled with zero circuit fields MUST load, got: %v", err)
	}
}

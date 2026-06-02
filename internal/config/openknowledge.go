// Package config — Spec 064 SCOPE-03: open-ended knowledge agent SST.
//
// OpenKnowledgeConfig governs the `assistant.open_knowledge.*` block.
// Every field originates in config/smackerel.yaml and flows through
// scripts/commands/config.sh into the generated env file as
// ASSISTANT_OPEN_KNOWLEDGE_* variables. There are no in-source defaults
// (Gate G028, smackerel-no-defaults): every env var MUST be present at
// load time; deep validation is gated on Enabled per design §SST.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// OpenKnowledgeProvider enumerates the supported web-search providers.
const (
	OpenKnowledgeProviderSearxng = "searxng"
	OpenKnowledgeProviderBrave   = "brave"
	OpenKnowledgeProviderTavily  = "tavily"
)

// OpenKnowledgeConfig is the SST surface for spec 064 SCOPE-03.
type OpenKnowledgeConfig struct {
	Enabled                 bool
	Provider                string
	ProviderEndpoint        string
	ProviderAPIKey          string
	LLMModelID              string
	MaxIterations           int
	PerQueryTokenBudget     int
	PerQueryUSDBudget       float64
	MonthlyBudgetUSD        float64
	PerUserMonthlyBudgetUSD float64
	ToolAllowlist           []string
	WebSnippetCacheEnabled  bool
	// LLMTimeoutMs caps each ML sidecar /llm/chat roundtrip the
	// open-knowledge agent makes. Spec 064 SCOPE-12 — required when
	// Enabled=true; > 0.
	LLMTimeoutMs int
	// AllowedEgressHosts is the spec 064 SCOPE-15 allowlist of
	// additional outbound hosts the open-knowledge subsystem may
	// reach beyond the provider_endpoint host (which is always
	// implicitly allowed). Each entry MUST be a bare host (no
	// scheme, path, port, or userinfo). Empty list (the SST default)
	// means "provider endpoint only" — deny-by-default per G028.
	// Wildcards are NOT supported in v1; PKT-020-A asks spec 020
	// whether wildcard support + a network-layer egress firewall
	// should layer on top.
	AllowedEgressHosts []string
	// CircuitBreaker bundles the SCOPE-16 resilience knobs for the
	// web-search provider circuit breaker. All fields REQUIRED
	// when Enabled=true (G028 — no defaults).
	CircuitBreaker OpenKnowledgeCircuitBreakerConfig
	// CitebackEnforcementMode is the spec 076 SCOPE-1 foundation
	// seam that gates the cite-back verifier roll-out: "shadow"
	// records mismatches without flipping the answer; "enforce"
	// flips the answer to refusal-with-capture on any mismatch.
	// REQUIRED non-empty regardless of Enabled state — spec 076
	// SCN-076-F02 lists this key as a foundation fail-loud key.
	CitebackEnforcementMode string
}

// Citeback enforcement modes — spec 076 SCOPE-1.
const (
	OpenKnowledgeCitebackShadow  = "shadow"
	OpenKnowledgeCitebackEnforce = "enforce"
)

// OpenKnowledgeCircuitBreakerConfig is the SST sub-block governing
// the web-provider circuit breaker (spec 064 SCOPE-16). The wiring
// layer threads these fields into web.NewCircuitBreaker; the
// breaker rejects zero / negative values at construction time.
type OpenKnowledgeCircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive countable
	// provider failures that trips the breaker from Closed to Open.
	// REQUIRED when Enabled=true; > 0.
	FailureThreshold int
	// OpenWindowSeconds is the operator-documented Open window
	// (kept for documentation parity with the breaker contract; the
	// effective window the breaker honours is HalfOpenAfterSeconds).
	// REQUIRED when Enabled=true; > 0.
	OpenWindowSeconds int
	// HalfOpenAfterSeconds is the elapsed-time threshold after a
	// trip before the next Search call is allowed through as a
	// HalfOpen probe. REQUIRED when Enabled=true; > 0; SHOULD be
	// <= OpenWindowSeconds.
	HalfOpenAfterSeconds int
}

// LoadOpenKnowledge reads every ASSISTANT_OPEN_KNOWLEDGE_* env var and
// returns a populated OpenKnowledgeConfig plus Validate() result.
// Missing env vars (LookupEnv == false) are always a fail-loud error.
// Empty/invalid values are tolerated at load time and rejected only by
// Validate() when Enabled=true (spec 064 design §SST: enabled=false
// skips deep validation so an operator can disable without filling
// keys).
func LoadOpenKnowledge() (OpenKnowledgeConfig, error) {
	var cfg OpenKnowledgeConfig
	var errs []string

	cfg.Enabled, errs = strictBool("ASSISTANT_OPEN_KNOWLEDGE_ENABLED", errs)
	cfg.Provider, errs = lookupString("ASSISTANT_OPEN_KNOWLEDGE_PROVIDER", errs)
	cfg.ProviderEndpoint, errs = lookupString("ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT", errs)
	cfg.ProviderAPIKey, errs = lookupString("ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY", errs)
	cfg.LLMModelID, errs = lookupString("ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID", errs)
	cfg.MaxIterations, errs = lookupInt("ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS", errs)
	cfg.PerQueryTokenBudget, errs = lookupInt("ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET", errs)
	cfg.PerQueryUSDBudget, errs = lookupFloat("ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET", errs)
	cfg.MonthlyBudgetUSD, errs = lookupFloat("ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD", errs)
	cfg.PerUserMonthlyBudgetUSD, errs = lookupFloat("ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD", errs)
	cfg.ToolAllowlist, errs = lookupJSONStringList("ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST", errs)
	cfg.WebSnippetCacheEnabled, errs = strictBool("ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED", errs)
	cfg.LLMTimeoutMs, errs = lookupInt("ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS", errs)
	cfg.AllowedEgressHosts, errs = lookupJSONStringList("ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS", errs)
	cfg.CircuitBreaker.FailureThreshold, errs = lookupInt("ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD", errs)
	cfg.CircuitBreaker.OpenWindowSeconds, errs = lookupInt("ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS", errs)
	cfg.CircuitBreaker.HalfOpenAfterSeconds, errs = lookupInt("ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS", errs)
	cfg.CitebackEnforcementMode, errs = lookupString("ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE", errs)

	if len(errs) > 0 {
		return OpenKnowledgeConfig{}, fmt.Errorf("[F064-SST-MISSING] missing or invalid required assistant.open_knowledge configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return OpenKnowledgeConfig{}, err
	}
	return cfg, nil
}

// Validate enforces spec 064 design §SST deep-validation rules.
// Enabled=false short-circuits — operators can disable without filling
// provider/budget/allowlist fields (adversarial coverage in
// openknowledge_test.go::TestOpenKnowledgeConfig_DisabledSkipsValidation).
func (c *OpenKnowledgeConfig) Validate() error {
	// Spec 076 SCOPE-1 foundation key (SCN-076-F02) — citeback
	// enforcement mode is always required even when the
	// open-knowledge agent is disabled, because the verifier seam is
	// consumed by every later spec-076 scope.
	var foundationErrs []string
	switch strings.TrimSpace(c.CitebackEnforcementMode) {
	case OpenKnowledgeCitebackShadow, OpenKnowledgeCitebackEnforce:
	case "":
		foundationErrs = append(foundationErrs, "assistant.open_knowledge.citeback.enforcement_mode (empty)")
	default:
		foundationErrs = append(foundationErrs, fmt.Sprintf("assistant.open_knowledge.citeback.enforcement_mode (must be one of %q|%q, got %q)",
			OpenKnowledgeCitebackShadow, OpenKnowledgeCitebackEnforce, c.CitebackEnforcementMode))
	}
	if !c.Enabled {
		if len(foundationErrs) > 0 {
			return fmt.Errorf("[F076-SST-INVALID] invalid foundation configuration: %s", strings.Join(foundationErrs, ", "))
		}
		return nil
	}
	errs := foundationErrs

	if strings.TrimSpace(c.Provider) == "" {
		errs = append(errs, "assistant.open_knowledge.provider (empty)")
	} else {
		switch c.Provider {
		case OpenKnowledgeProviderSearxng, OpenKnowledgeProviderBrave, OpenKnowledgeProviderTavily:
		default:
			errs = append(errs, fmt.Sprintf("assistant.open_knowledge.provider (must be one of %q|%q|%q, got %q)",
				OpenKnowledgeProviderSearxng, OpenKnowledgeProviderBrave, OpenKnowledgeProviderTavily, c.Provider))
		}
	}
	if strings.TrimSpace(c.ProviderEndpoint) == "" {
		errs = append(errs, "assistant.open_knowledge.provider_endpoint (empty)")
	}
	if strings.TrimSpace(c.LLMModelID) == "" {
		errs = append(errs, "assistant.open_knowledge.llm_model_id (empty)")
	}
	if c.MaxIterations <= 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.max_iterations (must be > 0, got %d)", c.MaxIterations))
	}
	if c.PerQueryTokenBudget <= 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.per_query_token_budget (must be > 0, got %d)", c.PerQueryTokenBudget))
	}
	if c.PerQueryUSDBudget <= 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.per_query_usd_budget (must be > 0, got %f)", c.PerQueryUSDBudget))
	}
	if c.MonthlyBudgetUSD < 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.monthly_budget_usd (must be >= 0, got %f)", c.MonthlyBudgetUSD))
	}
	if c.PerUserMonthlyBudgetUSD < 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.per_user_monthly_budget_usd (must be >= 0, got %f)", c.PerUserMonthlyBudgetUSD))
	}
	if c.LLMTimeoutMs <= 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.llm_timeout_ms (must be > 0, got %d)", c.LLMTimeoutMs))
	}
	if len(c.ToolAllowlist) == 0 {
		errs = append(errs, "assistant.open_knowledge.tool_allowlist (must be non-empty)")
	} else {
		for _, t := range c.ToolAllowlist {
			if strings.TrimSpace(t) == "" {
				errs = append(errs, "assistant.open_knowledge.tool_allowlist (contains empty entry)")
				break
			}
		}
	}
	// Provider-specific api_key requirement. searxng allows empty;
	// brave and tavily MUST have a non-empty (post-trim) api_key.
	switch c.Provider {
	case OpenKnowledgeProviderBrave, OpenKnowledgeProviderTavily:
		if strings.TrimSpace(c.ProviderAPIKey) == "" {
			errs = append(errs, fmt.Sprintf("assistant.open_knowledge.provider_api_key (required non-empty for provider %q)", c.Provider))
		}
	}

	// SCOPE-15 — allowed_egress_hosts entry format validation. Empty
	// list is permitted (provider endpoint is implicitly allowed);
	// any provided entry MUST be a bare host (no scheme, path, port,
	// userinfo, or whitespace) so a typo cannot become a silent
	// allow-all when the EgressAllowlistTransport normalises it.
	for _, h := range c.AllowedEgressHosts {
		trimmed := strings.TrimSpace(h)
		if trimmed == "" {
			errs = append(errs, "assistant.open_knowledge.allowed_egress_hosts (contains empty entry)")
			break
		}
		if strings.ContainsAny(trimmed, "/@: \t") {
			errs = append(errs, fmt.Sprintf("assistant.open_knowledge.allowed_egress_hosts (entry %q must be a bare host — no scheme, path, port, userinfo, or whitespace)", h))
			break
		}
	}

	// SCOPE-16 — circuit breaker bounds (G028 fail-loud).
	if c.CircuitBreaker.FailureThreshold <= 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.circuit_breaker.failure_threshold (must be > 0, got %d)", c.CircuitBreaker.FailureThreshold))
	}
	if c.CircuitBreaker.OpenWindowSeconds <= 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.circuit_breaker.open_window_seconds (must be > 0, got %d)", c.CircuitBreaker.OpenWindowSeconds))
	}
	if c.CircuitBreaker.HalfOpenAfterSeconds <= 0 {
		errs = append(errs, fmt.Sprintf("assistant.open_knowledge.circuit_breaker.half_open_after_seconds (must be > 0, got %d)", c.CircuitBreaker.HalfOpenAfterSeconds))
	}

	if len(errs) > 0 {
		return fmt.Errorf("[F064-SST-INVALID] invalid assistant.open_knowledge configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// "true" or "false". Missing var or any other value (including "1",
// "0", "True") → typed error appended to errs.
func strictBool(key string, errs []string) (bool, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return false, append(errs, key+" (env var not set)")
	}
	switch v {
	case "true":
		return true, errs
	case "false":
		return false, errs
	default:
		return false, append(errs, fmt.Sprintf("%s (must be exactly %q or %q, got %q)", key, "true", "false", v))
	}
}

// lookupString reads an env var. Missing (LookupEnv == false) is a
// load-time error; empty value is tolerated for Validate() to gate.
func lookupString(key string, errs []string) (string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", append(errs, key+" (env var not set)")
	}
	return v, errs
}

// lookupInt reads an env var as int. Missing var or unparseable
// value → typed error. Range checks live in Validate().
func lookupInt(key string, errs []string) (int, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return 0, append(errs, key+" (env var not set)")
	}
	if v == "" {
		// Empty tolerated at load; Validate() catches when Enabled.
		return 0, errs
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, append(errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
	}
	return n, errs
}

// lookupFloat reads an env var as float64. Missing var or unparseable
// value → typed error. Range checks live in Validate().
func lookupFloat(key string, errs []string) (float64, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return 0, append(errs, key+" (env var not set)")
	}
	if v == "" {
		return 0, errs
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, append(errs, fmt.Sprintf("%s (must be a float, got %q)", key, v))
	}
	return f, errs
}

// lookupJSONStringList reads an env var as a JSON string array.
// Missing var or invalid JSON → typed error. Empty list is tolerated
// at load; Validate() enforces non-empty when Enabled.
func lookupJSONStringList(key string, errs []string) ([]string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil, append(errs, key+" (env var not set)")
	}
	if v == "" {
		return nil, errs
	}
	var out []string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, append(errs, fmt.Sprintf("%s (invalid JSON list: %v)", key, err))
	}
	return out, errs
}

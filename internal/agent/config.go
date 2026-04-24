// Package agent owns the Smackerel LLM scenario agent runtime — config,
// tool registry (spec 037 Scope 2), scenario loader (Scope 3), router
// (Scope 4), executor loop (Scope 5), tracer/replay (Scope 6), and
// security/concurrency hardening (Scope 7).
//
// Scope 1 lands the SST configuration block plus the AGENT NATS contract.
// Every value comes from config/smackerel.yaml via ./smackerel.sh config
// generate. There are no Go fallback defaults; missing or malformed values
// cause LoadConfig to return an error so the binary refuses to start.
package agent

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ProviderRoute names a provider/model pair selected by a scenario's
// model_preference. See design §11 (provider_routing).
type ProviderRoute struct {
	Provider string
	Model    string
}

// RoutingConfig governs intent → scenario selection.
type RoutingConfig struct {
	ConfidenceFloor    float64
	ConsiderTopN       int
	FallbackScenarioID string // empty = no fallback (below-floor → unknown-intent)
	EmbeddingModel     string // empty = inherit runtime.embedding_model
}

// TraceConfig governs PostgreSQL trace persistence.
type TraceConfig struct {
	RetentionDays     int
	RecordLLMMessages bool
	RedactMarker      string
}

// LimitsCeilings clamps any scenario-declared limit that exceeds the cap.
// They are NOT defaults — a scenario MUST declare its own limits.
type LimitsCeilings struct {
	MaxLoopIterations int
	TimeoutMs         int
	SchemaRetryBudget int
	PerToolTimeoutMs  int
}

// Config is the fully resolved agent runtime configuration.
type Config struct {
	ScenarioDir     string
	ScenarioGlob    string
	HotReload       bool
	Routing         RoutingConfig
	Trace           TraceConfig
	Defaults        LimitsCeilings
	ProviderRouting map[string]ProviderRoute
}

// requiredProviderRoutes is the canonical set of model_preference keys.
// Adding a new key here is intentional and must be matched in smackerel.yaml.
var requiredProviderRoutes = []string{"default", "reasoning", "fast", "vision", "ocr"}

// LoadConfig resolves agent runtime config from environment variables that
// are populated by ./smackerel.sh config generate. Every required AGENT_*
// variable MUST be present in the environment; the loader collects all
// missing or malformed values and returns a single error naming each one,
// so the operator sees the complete picture in a single failed boot.
//
// Empty-string values are accepted ONLY for AGENT_ROUTING_FALLBACK_SCENARIO_ID
// and AGENT_ROUTING_EMBEDDING_MODEL, which the design explicitly documents as
// optional opt-outs (no fallback scenario, inherit runtime embedding model).
// All other empty values are fatal.
func LoadConfig() (*Config, error) {
	var missing []string
	var bad []string

	requireString := func(key string) string {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			missing = append(missing, key)
			return ""
		}
		return v
	}

	allowEmptyString := func(key string) (string, bool) {
		v, ok := os.LookupEnv(key)
		if !ok {
			missing = append(missing, key)
			return "", false
		}
		return v, true
	}

	requireBool := func(key string) bool {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			missing = append(missing, key)
			return false
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true":
			return true
		case "false":
			return false
		default:
			bad = append(bad, fmt.Sprintf("%s (must be true or false, got %q)", key, v))
			return false
		}
	}

	requireFloat := func(key string, lo, hi float64) float64 {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			missing = append(missing, key)
			return 0
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s (must be a float, got %q)", key, v))
			return 0
		}
		if f < lo || f > hi {
			bad = append(bad, fmt.Sprintf("%s (must be in range [%g, %g], got %g)", key, lo, hi, f))
			return 0
		}
		return f
	}

	requireInt := func(key string, lo int) int {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			missing = append(missing, key)
			return 0
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
			return 0
		}
		if n < lo {
			bad = append(bad, fmt.Sprintf("%s (must be >= %d, got %d)", key, lo, n))
			return 0
		}
		return n
	}

	cfg := &Config{
		ScenarioDir:  requireString("AGENT_SCENARIO_DIR"),
		ScenarioGlob: requireString("AGENT_SCENARIO_GLOB"),
		HotReload:    requireBool("AGENT_HOT_RELOAD"),
		Routing: RoutingConfig{
			ConfidenceFloor: requireFloat("AGENT_ROUTING_CONFIDENCE_FLOOR", 0, 1),
			ConsiderTopN:    requireInt("AGENT_ROUTING_CONSIDER_TOP_N", 1),
		},
		Trace: TraceConfig{
			RetentionDays:     requireInt("AGENT_TRACE_RETENTION_DAYS", 1),
			RecordLLMMessages: requireBool("AGENT_TRACE_RECORD_LLM_MESSAGES"),
			RedactMarker:      requireString("AGENT_TRACE_REDACT_MARKER"),
		},
		Defaults: LimitsCeilings{
			MaxLoopIterations: requireInt("AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING", 1),
			TimeoutMs:         requireInt("AGENT_DEFAULTS_TIMEOUT_MS_CEILING", 1),
			SchemaRetryBudget: requireInt("AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING", 0),
			PerToolTimeoutMs:  requireInt("AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING", 1),
		},
	}

	// Empty-string is allowed for these two; absent is fatal.
	if v, ok := allowEmptyString("AGENT_ROUTING_FALLBACK_SCENARIO_ID"); ok {
		cfg.Routing.FallbackScenarioID = v
	}
	if v, ok := allowEmptyString("AGENT_ROUTING_EMBEDDING_MODEL"); ok {
		cfg.Routing.EmbeddingModel = v
	}

	cfg.ProviderRouting = make(map[string]ProviderRoute, len(requiredProviderRoutes))
	for _, name := range requiredProviderRoutes {
		upper := strings.ToUpper(name)
		provider := requireString("AGENT_PROVIDER_" + upper + "_PROVIDER")
		model := requireString("AGENT_PROVIDER_" + upper + "_MODEL")
		cfg.ProviderRouting[name] = ProviderRoute{Provider: provider, Model: model}
	}

	if len(missing) > 0 || len(bad) > 0 {
		var parts []string
		if len(missing) > 0 {
			parts = append(parts, "missing required agent configuration: "+strings.Join(missing, ", "))
		}
		if len(bad) > 0 {
			parts = append(parts, "invalid agent configuration: "+strings.Join(bad, "; "))
		}
		return nil, fmt.Errorf("%s", strings.Join(parts, "; "))
	}

	return cfg, nil
}

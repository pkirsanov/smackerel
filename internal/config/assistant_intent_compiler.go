// Spec 068 SCOPE-1 — Intent compiler SST config.
//
// All keys are REQUIRED at the generator boundary (smackerel-no-defaults /
// Gate G028). Missing values fail loud during config load; there are
// no fallback model, prompt, schema, confidence, or budget defaults
// (spec.md Hard Constraint 2).
//
// The struct + loader live in their own file (per scopes.md surfaces
// "internal/config/assistant_intent_compiler*.go") to keep spec 061
// assistant config decoupled from spec 068's compiler config.

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// IntentCompilerConfig holds the spec 068 SCOPE-1 SST values.
// design.md §Configuration enumerates the keys.
type IntentCompilerConfig struct {
	// Enabled gates compiler invocation. When false the facade
	// follows its existing path; Scope 2 will wire compilation into
	// the facade flow when Enabled=true.
	Enabled bool

	// ModelRole names the LLM bridge model role used for compilation.
	// Resolved by the ML sidecar bridge.
	ModelRole string

	// PromptContractVersion identifies the compiler prompt contract
	// the sidecar must honor. Bumped any time the prompt schema or
	// vocabulary changes.
	PromptContractVersion string

	// SchemaVersion identifies the CompiledIntent schema version the
	// Go side will accept. Mismatched sidecar output is treated as
	// schema_invalid.
	SchemaVersion string

	// Timeout is the per-compile request deadline.
	Timeout time.Duration

	// ConfidenceFloor is the scenario-hint confidence floor below
	// which the router treats the compiled intent as
	// no-strong-hint and may fall back to similarity ranking.
	ConfidenceFloor float64

	// MaxContextTurns bounds the conversation window passed to the
	// compiler (oldest turns are dropped).
	MaxContextTurns int

	// MaxOutputBytes caps the sidecar response body to defend against
	// runaway LLM output.
	MaxOutputBytes int

	// RetryBudget is the number of schema-validation retries the
	// compiler is allowed before declaring schema_invalid.
	RetryBudget int
}

// loadIntentCompilerConfig populates cfg.Assistant.IntentCompiler from
// ASSISTANT_INTENT_COMPILER_* env vars. It appends to errs (passed by
// pointer) using the same pattern as the rest of loadAssistantConfig
// so the caller can emit one aggregate F068-SST-MISSING error.
//
// Every key is REQUIRED; missing/unparsable values produce an entry in
// errs and abort startup once the caller collects them.
func loadIntentCompilerConfig(cfg *Config, errs *[]string) {
	mustBool := func(key string, dst *bool) {
		v := os.Getenv(key)
		if v == "" {
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
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		*dst = v
	}
	mustInt := func(key string, dst *int, minVal int) {
		v := os.Getenv(key)
		if v == "" {
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
	mustFloat := func(key string, dst *float64, lo, hi float64) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must be a float, got %q)", key, v))
			return
		}
		if f < lo || f > hi {
			*errs = append(*errs, fmt.Sprintf("%s (must be in [%g,%g], got %g)", key, lo, hi, f))
			return
		}
		*dst = f
	}

	mustBool("ASSISTANT_INTENT_COMPILER_ENABLED", &cfg.Assistant.IntentCompiler.Enabled)
	mustString("ASSISTANT_INTENT_COMPILER_MODEL_ROLE", &cfg.Assistant.IntentCompiler.ModelRole)
	mustString("ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION", &cfg.Assistant.IntentCompiler.PromptContractVersion)
	mustString("ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION", &cfg.Assistant.IntentCompiler.SchemaVersion)
	var timeoutMs int
	mustInt("ASSISTANT_INTENT_COMPILER_TIMEOUT_MS", &timeoutMs, 1)
	cfg.Assistant.IntentCompiler.Timeout = time.Duration(timeoutMs) * time.Millisecond
	mustFloat("ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR", &cfg.Assistant.IntentCompiler.ConfidenceFloor, 0, 1)
	mustInt("ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS", &cfg.Assistant.IntentCompiler.MaxContextTurns, 0)
	mustInt("ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES", &cfg.Assistant.IntentCompiler.MaxOutputBytes, 1)
	mustInt("ASSISTANT_INTENT_COMPILER_RETRY_BUDGET", &cfg.Assistant.IntentCompiler.RetryBudget, 0)
}

// IntentCompilerMissingKeyError formats an aggregate fail-loud error
// for the spec 068 SCOPE-1 keys. Exposed for the dedicated test in
// assistant_intent_compiler_test.go.
func IntentCompilerMissingKeyError(missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("[F068-SST-MISSING] missing or invalid required assistant intent compiler configuration: %s", strings.Join(missing, ", "))
}

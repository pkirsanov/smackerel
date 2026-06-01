// Spec 068 SCOPE-1 — LLM-backed intent compiler.
//
// The compiler is a thin wrapper around the ML sidecar's
// POST /assistant/intent/compile route (design.md §"ML Sidecar
// Compiler Contract"). The HTTP transport is injected so unit tests
// can drive deterministic provider responses (malformed JSON, schema
// violations, etc.) without standing up the sidecar.

package intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CompileRequest is the wire shape sent to the ML sidecar's
// POST /assistant/intent/compile.
type CompileRequest struct {
	SchemaVersion         string                  `json:"schema_version"`
	ModelRole             string                  `json:"model_role"`
	PromptContractVersion string                  `json:"prompt_contract_version"`
	RawTurn               CompileRequestTurn      `json:"raw_turn"`
	ConversationContext   []CompileRequestTurnCtx `json:"conversation_context"`
	ResponseSchema        string                  `json:"response_schema"`
	MaxOutputBytes        int                     `json:"max_output_bytes"`
}

// CompileRequestTurn is the trimmed RawTurn carried in the wire
// request (no go-internal fields like ReceivedAt time).
type CompileRequestTurn struct {
	UserID             string `json:"user_id"`
	Transport          string `json:"transport"`
	TransportMessageID string `json:"transport_message_id"`
	Text               string `json:"text"`
}

// CompileRequestTurnCtx is one prior turn passed to the compiler.
type CompileRequestTurnCtx struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// CompileResponse is the wire shape returned by the ML sidecar. The
// compiled intent travels as a raw JSON object so the Go side runs
// strict schema validation independent of the sidecar's encoding.
type CompileResponse struct {
	SchemaVersion  string          `json:"schema_version"`
	CompiledIntent json.RawMessage `json:"compiled_intent"`
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	LatencyMS      int64           `json:"latency_ms"`
}

// Transport abstracts the HTTP call to the ML sidecar so unit tests
// can drive deterministic provider responses.
type Transport interface {
	Compile(ctx context.Context, req CompileRequest) (CompileResponse, error)
}

// CompilerConfig is the SST-resolved configuration consumed by
// LLMCompiler. design.md §Configuration enumerates the keys; all
// fields are REQUIRED (callers must validate before constructing the
// compiler).
type CompilerConfig struct {
	Enabled               bool
	ModelRole             string
	PromptContractVersion string
	SchemaVersion         string
	Timeout               time.Duration
	ConfidenceFloor       float64
	MaxContextTurns       int
	MaxOutputBytes        int
	RetryBudget           int
}

// Validate enforces the required-field contract. Missing values are
// fail-loud per Hard Constraint 2 in spec.md.
func (c CompilerConfig) Validate() error {
	if c.ModelRole == "" {
		return errors.New("intent: CompilerConfig.ModelRole is required")
	}
	if c.PromptContractVersion == "" {
		return errors.New("intent: CompilerConfig.PromptContractVersion is required")
	}
	if c.SchemaVersion == "" {
		return errors.New("intent: CompilerConfig.SchemaVersion is required")
	}
	if c.Timeout <= 0 {
		return errors.New("intent: CompilerConfig.Timeout must be > 0")
	}
	if c.ConfidenceFloor < 0 || c.ConfidenceFloor > 1 {
		return errors.New("intent: CompilerConfig.ConfidenceFloor must be in [0,1]")
	}
	if c.MaxContextTurns < 0 {
		return errors.New("intent: CompilerConfig.MaxContextTurns must be >= 0")
	}
	if c.MaxOutputBytes <= 0 {
		return errors.New("intent: CompilerConfig.MaxOutputBytes must be > 0")
	}
	if c.RetryBudget < 0 {
		return errors.New("intent: CompilerConfig.RetryBudget must be >= 0")
	}
	return nil
}

// LLMCompiler is the default Compiler implementation. It calls the ML
// sidecar via the injected Transport, parses + validates the response,
// and emits intent-compiler metrics.
type LLMCompiler struct {
	cfg       CompilerConfig
	transport Transport
	now       func() time.Time
}

// NewLLMCompiler constructs a compiler. The transport is required;
// callers wire the production HTTP transport in cmd/core wiring.
func NewLLMCompiler(cfg CompilerConfig, transport Transport) (*LLMCompiler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if transport == nil {
		return nil, errors.New("intent: NewLLMCompiler requires a non-nil transport")
	}
	return &LLMCompiler{cfg: cfg, transport: transport, now: time.Now}, nil
}

// WithClock replaces the wall clock for deterministic tests.
func (c *LLMCompiler) WithClock(now func() time.Time) *LLMCompiler {
	if now != nil {
		c.now = now
	}
	return c
}

// Compile implements Compiler.
//
// Failure semantics (spec 068 SCN-068-A06):
//   - Transport / provider error          → OutcomeProviderError + non-nil error.
//   - JSON decode failure                 → OutcomeSchemaInvalid + SchemaError{Cause:"json_invalid"}.
//   - Closed-vocabulary / required-field  → OutcomeSchemaInvalid + SchemaError{Cause:"schema_invalid"}.
//
// All failure paths increment intent_compiler_error_total{cause=...}
// and intent_compiler_requests_total{outcome=...,action_class=""}.
// The router MUST NOT be invoked on failure; the caller emits the
// canonical refusal-with-capture response.
func (c *LLMCompiler) Compile(ctx context.Context, turn RawTurn) (CompiledIntent, CompilerTrace, error) {
	start := c.now()

	ctxBudget := c.cfg.MaxContextTurns
	if ctxBudget > len(turn.ConversationWindow) {
		ctxBudget = len(turn.ConversationWindow)
	}
	contextWindow := make([]CompileRequestTurnCtx, 0, ctxBudget)
	// Take the most-recent ctxBudget turns.
	for i := len(turn.ConversationWindow) - ctxBudget; i < len(turn.ConversationWindow); i++ {
		ct := turn.ConversationWindow[i]
		contextWindow = append(contextWindow, CompileRequestTurnCtx{Role: ct.Role, Text: ct.Text})
	}

	req := CompileRequest{
		SchemaVersion:         c.cfg.SchemaVersion,
		ModelRole:             c.cfg.ModelRole,
		PromptContractVersion: c.cfg.PromptContractVersion,
		RawTurn: CompileRequestTurn{
			UserID:             turn.UserID,
			Transport:          turn.Transport,
			TransportMessageID: turn.TransportMessageID,
			Text:               turn.Text,
		},
		ConversationContext: contextWindow,
		ResponseSchema:      "compiled-intent-" + c.cfg.SchemaVersion,
		MaxOutputBytes:      c.cfg.MaxOutputBytes,
	}

	callCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	resp, err := c.transport.Compile(callCtx, req)
	if err != nil {
		CompilerErrorTotal.WithLabelValues("provider_error").Inc()
		CompilerRequestsTotal.WithLabelValues(string(OutcomeProviderError), "").Inc()
		return CompiledIntent{}, CompilerTrace{
			RawText:    turn.Text,
			Outcome:    OutcomeProviderError,
			ErrorCause: "provider_error",
			LatencyMS:  c.now().Sub(start).Milliseconds(),
		}, fmt.Errorf("intent compiler: transport: %w", err)
	}

	ci, parseErr := ParseAndValidate([]byte(resp.CompiledIntent))
	if parseErr != nil {
		cause := "schema_invalid"
		if se, ok := IsSchemaError(parseErr); ok {
			cause = se.Cause
		}
		CompilerErrorTotal.WithLabelValues(cause).Inc()
		CompilerRequestsTotal.WithLabelValues(string(OutcomeSchemaInvalid), "").Inc()
		return CompiledIntent{}, CompilerTrace{
			RawText:    turn.Text,
			Outcome:    OutcomeSchemaInvalid,
			ErrorCause: cause,
			LatencyMS:  c.now().Sub(start).Milliseconds(),
		}, parseErr
	}

	trace := CompilerTrace{
		RawText:   turn.Text,
		Outcome:   OutcomeCompiled,
		Compiled:  &ci,
		LatencyMS: c.now().Sub(start).Milliseconds(),
	}
	CompilerRequestsTotal.WithLabelValues(string(OutcomeCompiled), string(ci.ActionClass)).Inc()
	return ci, trace, nil
}

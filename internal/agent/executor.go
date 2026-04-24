// Execution loop for spec 037 Scope 5.
//
// The executor implements design §5.1 verbatim:
//
//  1. Validate the input envelope against scenario.input_schema.
//  2. Loop, with iter > scenario.Limits.MaxLoopIterations terminating
//     the invocation with outcome `loop-limit`.
//  3. Each iteration asks the LLM (via an LLMDriver — wired to the
//     Python ML sidecar over NATS in production, scripted in unit
//     tests) for the next turn. Provider errors terminate with
//     `provider-error`; ctx-deadline-exceeded terminates with
//     `timeout`.
//  4. If the LLM returns no tool calls, validate the final output
//     against scenario.output_schema. Success → outcome `ok`.
//     Failure → bump the schema-retry counter; once it exceeds
//     scenario.Limits.SchemaRetryBudget, terminate with
//     `schema-failure`. Otherwise append a system retry message
//     and loop.
//  5. Otherwise, process every tool call sequentially:
//     - unknown tool name → record per-call rejection
//     (reason: "unknown_tool"); append structured tool-error
//     message to turn_messages; continue (does NOT consume the
//     per-iteration budget — but the next LLM turn does).
//     - tool not in scenario allowlist → reason "not_in_allowlist".
//     - argument-schema violation → reason
//     "argument_schema_violation".
//     - tool handler returns an error → reason "tool_error";
//     loop continues so the LLM can recover.
//     - return-schema violation → terminate with
//     `tool-return-invalid`.
//     Otherwise the result is appended to turn_messages and the loop
//     continues.
//
// Allowlist enforcement, schema validation in/out, and the loop limit
// are all enforced here in Go. The LLM is never trusted to police
// itself (BS-020). Hallucinated tool names (BS-006) are rejected
// before any registry lookup with side effects.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

// Outcome is the terminal result class for an invocation. Mirrors the
// `outcome` enum of the agent_traces table (design §6.1).
type Outcome string

const (
	// OutcomeOK — final output validated against scenario.output_schema.
	OutcomeOK Outcome = "ok"
	// OutcomeUnknownIntent — produced by the router, not the executor;
	// surfaces (telegram/api) translate it to a user-facing message.
	OutcomeUnknownIntent Outcome = "unknown-intent"
	// OutcomeAllowlistViolation — recorded against an individual tool
	// call; the §5.1 loop does not terminate on it (the LLM may
	// recover). Reserved for trace-store filtering.
	OutcomeAllowlistViolation Outcome = "allowlist-violation"
	// OutcomeHallucinatedTool — recorded against an individual tool
	// call (BS-006); §5.1 loop continues.
	OutcomeHallucinatedTool Outcome = "hallucinated-tool"
	// OutcomeToolError — recorded against an individual tool call
	// (BS-015); §5.1 loop continues so the LLM may try a different
	// approach.
	OutcomeToolError Outcome = "tool-error"
	// OutcomeToolReturnInvalid — terminal: a tool's return value
	// failed its declared output schema (BS-005).
	OutcomeToolReturnInvalid Outcome = "tool-return-invalid"
	// OutcomeSchemaFailure — terminal: the LLM exhausted
	// scenario.Limits.SchemaRetryBudget producing schema-valid output
	// (BS-007).
	OutcomeSchemaFailure Outcome = "schema-failure"
	// OutcomeLoopLimit — terminal: iter > Limits.MaxLoopIterations
	// (BS-008).
	OutcomeLoopLimit Outcome = "loop-limit"
	// OutcomeTimeout — terminal: ctx deadline exceeded (BS-021).
	OutcomeTimeout Outcome = "timeout"
	// OutcomeProviderError — terminal: the LLM driver returned a
	// non-deadline error.
	OutcomeProviderError Outcome = "provider-error"
	// OutcomeInputSchemaViolation — terminal: the input envelope
	// failed scenario.input_schema before the loop started.
	OutcomeInputSchemaViolation Outcome = "input-schema-violation"
)

// String implements fmt.Stringer.
func (o Outcome) String() string { return string(o) }

// Tokens carries provider-reported token counts for a single LLM turn.
type Tokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
}

// LLMToolDef is one tool definition handed to the LLM driver. The driver
// (Python ML sidecar) renders these into the provider's tool-calling
// format (OpenAI tools, Anthropic tools, etc.).
type LLMToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// TurnMessageRole names where a turn message originated.
type TurnMessageRole string

const (
	// RoleUser carries the executor-rendered initial input (envelope).
	RoleUser TurnMessageRole = "user"
	// RoleAssistant carries the LLM's previous turn (final or tool calls).
	RoleAssistant TurnMessageRole = "assistant"
	// RoleTool carries a single tool's structured result, including
	// the per-call rejection envelopes for hallucinated/disallowed/
	// argument-invalid/tool-error cases.
	RoleTool TurnMessageRole = "tool"
	// RoleSystem carries an executor-injected schema-retry nudge after
	// an output-schema violation.
	RoleSystem TurnMessageRole = "system"
)

// TurnMessage is one entry in the accumulating LLM conversation.
type TurnMessage struct {
	Role     TurnMessageRole `json:"role"`
	ToolName string          `json:"tool_name,omitempty"` // for RoleTool
	Content  json.RawMessage `json:"content"`             // structured payload
}

// TurnRequest is the per-turn payload handed to the LLM driver.
// Mirrors design §5.1 step (1).
type TurnRequest struct {
	TraceID         string          `json:"trace_id"`
	ScenarioID      string          `json:"scenario_id"`
	ScenarioVersion string          `json:"scenario_version"`
	SystemPrompt    string          `json:"system_prompt"`
	ToolDefs        []LLMToolDef    `json:"tool_defs"`
	TurnMessages    []TurnMessage   `json:"turn_messages"`
	TokenBudget     int             `json:"token_budget"`
	Temperature     float64         `json:"temperature"`
	ModelPreference string          `json:"model_preference"`
	DeadlineUnixMs  int64           `json:"deadline_unix_ms"`
	StructuredInput json.RawMessage `json:"structured_input"`
}

// LLMToolCall is one tool call proposed by the LLM in a turn response.
type LLMToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// TurnResponse is what the LLM driver returns per turn. Either ToolCalls
// is non-empty (the LLM wants to invoke tools) or Final is non-nil (the
// LLM is finalising the response). Both empty is a provider error.
type TurnResponse struct {
	ToolCalls []LLMToolCall   `json:"tool_calls"`
	Final     json.RawMessage `json:"final"`
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Tokens    Tokens          `json:"tokens"`
}

// LLMDriver is the abstraction the executor uses to obtain one LLM turn.
// Production wires this to a NATS request/response client backed by the
// Python ML sidecar. Tests inject scripted drivers to exercise every
// failure mode without spinning up the live stack.
type LLMDriver interface {
	Turn(ctx context.Context, req TurnRequest) (TurnResponse, error)
}

// ExecutedToolCall is the per-call audit record. Scope 6's tracer
// persists these to agent_tool_calls; until then the executor returns
// them on InvocationResult so callers (and tests) can inspect them.
type ExecutedToolCall struct {
	Seq             int             `json:"seq"`
	Name            string          `json:"name"`
	Arguments       json.RawMessage `json:"arguments"`
	Result          json.RawMessage `json:"result,omitempty"`
	Outcome         Outcome         `json:"outcome"` // ok | hallucinated-tool | allowlist-violation | tool-error | tool-return-invalid (per-call)
	RejectionReason string          `json:"rejection_reason,omitempty"`
	Error           string          `json:"error,omitempty"`
	LatencyMs       int             `json:"latency_ms"`
}

// TraceContext is the immutable invocation context handed to Tracer.Begin
// once per Run. It carries the scenario, envelope, routing decision, and
// start time so the tracer can persist a complete trace row at End time
// without needing extra hooks per event.
type TraceContext struct {
	TraceID   string
	Scenario  *Scenario
	Envelope  IntentEnvelope
	Decision  RoutingDecision
	StartedAt time.Time
}

// Tracer is the optional sink the executor calls on every event. Scope 6
// supplies the PostgreSQL-backed implementation. The default NopTracer
// keeps the executor usable in unit tests without a database.
//
// Lifecycle per invocation:
//
//	Begin                    once, before the loop
//	RecordTurn               once per LLM turn
//	RecordToolCall           per successful tool call
//	RecordRejection          per rejected tool call (hallucinated, allowlist, arg-schema)
//	RecordToolError          per tool handler error
//	RecordReturnInvalid      per return-schema violation
//	RecordSchemaRetry        per output-schema retry
//	RecordOutcome            once, AFTER the loop terminates — flush + publish
type Tracer interface {
	Begin(tc TraceContext)
	RecordTurn(traceID string, iter int, resp TurnResponse)
	RecordToolCall(traceID string, call ExecutedToolCall)
	RecordRejection(traceID string, call ExecutedToolCall)
	RecordToolError(traceID string, call ExecutedToolCall)
	RecordReturnInvalid(traceID string, call ExecutedToolCall, schemaErr error)
	RecordSchemaRetry(traceID string, attempt int, schemaErr error)
	RecordOutcome(traceID string, result *InvocationResult)
}

// NopTracer ignores every call.
type NopTracer struct{}

func (NopTracer) Begin(TraceContext)                                  {}
func (NopTracer) RecordTurn(string, int, TurnResponse)                {}
func (NopTracer) RecordToolCall(string, ExecutedToolCall)             {}
func (NopTracer) RecordRejection(string, ExecutedToolCall)            {}
func (NopTracer) RecordToolError(string, ExecutedToolCall)            {}
func (NopTracer) RecordReturnInvalid(string, ExecutedToolCall, error) {}
func (NopTracer) RecordSchemaRetry(string, int, error)                {}
func (NopTracer) RecordOutcome(string, *InvocationResult)             {}

// InvocationResult is the terminal envelope returned by Executor.Run.
// Surfaces translate Outcome into surface-specific responses (Telegram
// message, REST status, etc.).
type InvocationResult struct {
	TraceID          string             `json:"trace_id"`
	ScenarioID       string             `json:"scenario_id"`
	ScenarioVersion  string             `json:"scenario_version"`
	Outcome          Outcome            `json:"outcome"`
	Final            json.RawMessage    `json:"final,omitempty"`
	OutcomeDetail    map[string]any     `json:"outcome_detail,omitempty"`
	ToolCalls        []ExecutedToolCall `json:"tool_calls"`
	Iterations       int                `json:"iterations"`
	SchemaRetries    int                `json:"schema_retries"`
	Provider         string             `json:"provider,omitempty"`
	Model            string             `json:"model,omitempty"`
	TokensPrompt     int                `json:"tokens_prompt"`
	TokensCompletion int                `json:"tokens_completion"`
	StartedAt        time.Time          `json:"started_at"`
	EndedAt          time.Time          `json:"ended_at"`
}

// Executor runs scenarios. Construct one via NewExecutor and reuse it
// across goroutines — each Run call is fully isolated by trace_id and
// per-invocation context.
type Executor struct {
	driver  LLMDriver
	tracer  Tracer
	traceN  atomic.Uint64 // monotonic counter for trace_id generation
	nowFunc func() time.Time
}

// NewExecutor builds an Executor. driver is required; tracer may be nil
// (NopTracer is substituted).
func NewExecutor(driver LLMDriver, tracer Tracer) (*Executor, error) {
	if driver == nil {
		return nil, errors.New("agent.NewExecutor: driver is required")
	}
	if tracer == nil {
		tracer = NopTracer{}
	}
	return &Executor{driver: driver, tracer: tracer, nowFunc: time.Now}, nil
}

// SetClock overrides the time source. Test-only; production paths use
// time.Now via the default clock.
func (e *Executor) SetClock(now func() time.Time) {
	if now != nil {
		e.nowFunc = now
	}
}

// Run executes scenario sc against env, returning the structured outcome
// envelope. parent is the caller-supplied context; the executor wraps it
// with scenario.Limits.TimeoutMs. Run is safe for concurrent use; each
// invocation owns its own turn_messages slice and trace_id.
func (e *Executor) Run(parent context.Context, sc *Scenario, env IntentEnvelope) *InvocationResult {
	if sc == nil {
		// Defensive: callers obtain the scenario from the router which
		// guarantees non-nil. A direct nil here is a programmer error,
		// but we still return a structured outcome rather than panic so
		// the surface can log it.
		return &InvocationResult{
			Outcome:       OutcomeProviderError,
			OutcomeDetail: map[string]any{"error": "executor.Run called with nil scenario"},
			StartedAt:     e.nowFunc(),
			EndedAt:       e.nowFunc(),
		}
	}

	traceID := e.newTraceID()
	startedAt := e.nowFunc()

	result := &InvocationResult{
		TraceID:         traceID,
		ScenarioID:      sc.ID,
		ScenarioVersion: sc.Version,
		StartedAt:       startedAt,
		ToolCalls:       []ExecutedToolCall{},
	}

	// Notify the tracer the invocation is starting. The tracer holds a
	// pointer to the same *Scenario the executor uses for the entire
	// loop (BS-019: even if SIGHUP swaps the global registry mid-flight,
	// this invocation completes against the version it began with — the
	// tracer records that exact version).
	e.tracer.Begin(TraceContext{
		TraceID:   traceID,
		Scenario:  sc,
		Envelope:  env,
		Decision:  env.Routing,
		StartedAt: startedAt,
	})

	// Step (1) — input schema validation (BS-009 A4 mirror at runtime).
	if sc.inputSchema != nil {
		var inputAny any
		if len(env.StructuredContext) > 0 {
			if err := json.Unmarshal(env.StructuredContext, &inputAny); err != nil {
				result.Outcome = OutcomeInputSchemaViolation
				result.OutcomeDetail = map[string]any{
					"error":  "structured_context is not valid JSON",
					"detail": err.Error(),
				}
				return e.finalize(result)
			}
		}
		if err := sc.inputSchema.Validate(inputAny); err != nil {
			result.Outcome = OutcomeInputSchemaViolation
			result.OutcomeDetail = map[string]any{
				"error":  "input_schema_violation",
				"detail": err.Error(),
			}
			return e.finalize(result)
		}
	}

	// Build the bounded ctx ONCE per invocation. Per-tool deadlines are
	// derived from this within the loop.
	timeout := time.Duration(sc.Limits.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	// Tool definitions handed to the LLM each turn. Hallucinated names
	// (anything outside this set) are rejected by the executor before
	// any registry lookup with side effects (BS-006).
	toolDefs := make([]LLMToolDef, 0, len(sc.AllowedTools))
	allowSet := make(map[string]struct{}, len(sc.AllowedTools))
	for _, at := range sc.AllowedTools {
		allowSet[at.Name] = struct{}{}
		// Best effort: include the registered description + input
		// schema so the LLM knows how to call. If the tool has been
		// removed from the registry between load time and now, the
		// per-call rejection path (unknown_tool) catches it.
		if t, ok := ByName(at.Name); ok {
			toolDefs = append(toolDefs, LLMToolDef{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		} else {
			toolDefs = append(toolDefs, LLMToolDef{Name: at.Name})
		}
	}
	sort.Slice(toolDefs, func(i, j int) bool { return toolDefs[i].Name < toolDefs[j].Name })

	// Seed turn_messages with the user envelope so the LLM has the input.
	turnMessages := []TurnMessage{}
	if len(env.StructuredContext) > 0 || env.RawInput != "" {
		userPayload := map[string]any{}
		if env.RawInput != "" {
			userPayload["raw_input"] = env.RawInput
		}
		if len(env.StructuredContext) > 0 {
			// Pass the structured context through verbatim. The LLM
			// driver may serialize it however the provider expects.
			userPayload["structured_context"] = json.RawMessage(env.StructuredContext)
		}
		raw, _ := json.Marshal(userPayload)
		turnMessages = append(turnMessages, TurnMessage{Role: RoleUser, Content: raw})
	}

	// The §5.1 loop.
	for {
		result.Iterations++
		if result.Iterations > sc.Limits.MaxLoopIterations {
			// Loop-limit fires AT the (K+1)-th iteration. We do not
			// emit the K+1 turn — the K calls already recorded are
			// the evidence per BS-008. Roll the iteration counter
			// back to the cap so the trace shows exactly K turns.
			result.Iterations = sc.Limits.MaxLoopIterations
			result.Outcome = OutcomeLoopLimit
			result.OutcomeDetail = map[string]any{
				"reason":              "max_iterations_exceeded",
				"max_loop_iterations": sc.Limits.MaxLoopIterations,
			}
			return e.finalize(result)
		}

		// Step (1) of §5.1 — request the next LLM turn.
		req := TurnRequest{
			TraceID:         traceID,
			ScenarioID:      sc.ID,
			ScenarioVersion: sc.Version,
			SystemPrompt:    sc.SystemPrompt,
			ToolDefs:        toolDefs,
			TurnMessages:    cloneTurnMessages(turnMessages),
			TokenBudget:     sc.TokenBudget,
			Temperature:     sc.Temperature,
			ModelPreference: sc.ModelPreference,
			StructuredInput: env.StructuredContext,
		}
		if dl, ok := ctx.Deadline(); ok {
			req.DeadlineUnixMs = dl.UnixMilli()
		}

		resp, err := e.driver.Turn(ctx, req)
		if err != nil {
			// Distinguish ctx-deadline (timeout) from other provider errors.
			if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
				result.Outcome = OutcomeTimeout
				result.OutcomeDetail = map[string]any{
					"deadline_s": int(timeout / time.Second),
					"reason":     "provider_did_not_respond_before_deadline",
				}
				return e.finalize(result)
			}
			result.Outcome = OutcomeProviderError
			result.OutcomeDetail = map[string]any{
				"error":  "llm_driver_error",
				"detail": err.Error(),
			}
			return e.finalize(result)
		}

		// Even if Turn returned nil error but ctx fired between dispatch
		// and parsing, treat it as timeout.
		if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
			result.Outcome = OutcomeTimeout
			result.OutcomeDetail = map[string]any{
				"deadline_s": int(timeout / time.Second),
				"reason":     "deadline_exceeded_after_provider_response",
			}
			return e.finalize(result)
		}

		result.Provider = resp.Provider
		result.Model = resp.Model
		result.TokensPrompt += resp.Tokens.Prompt
		result.TokensCompletion += resp.Tokens.Completion
		e.tracer.RecordTurn(traceID, result.Iterations, resp)

		// Step (2) — final answer? Validate against output_schema.
		if len(resp.ToolCalls) == 0 {
			if len(resp.Final) == 0 {
				// Provider returned neither tool calls nor a final
				// answer. Treat as a provider error so the loop does
				// not spin forever.
				result.Outcome = OutcomeProviderError
				result.OutcomeDetail = map[string]any{
					"error": "llm_returned_no_tool_calls_and_no_final",
				}
				return e.finalize(result)
			}
			var finalAny any
			if err := json.Unmarshal(resp.Final, &finalAny); err != nil {
				// Treat invalid JSON as a schema error.
				result.SchemaRetries++
				e.tracer.RecordSchemaRetry(traceID, result.SchemaRetries, err)
				if result.SchemaRetries > sc.Limits.SchemaRetryBudget {
					result.Outcome = OutcomeSchemaFailure
					result.OutcomeDetail = map[string]any{
						"attempts":   sc.Limits.SchemaRetryBudget,
						"last_error": err.Error(),
					}
					return e.finalize(result)
				}
				turnMessages = appendAssistantFinal(turnMessages, resp.Final)
				turnMessages = appendSchemaRetryMessage(turnMessages, err)
				continue
			}
			if err := sc.outputSchema.Validate(finalAny); err != nil {
				result.SchemaRetries++
				e.tracer.RecordSchemaRetry(traceID, result.SchemaRetries, err)
				if result.SchemaRetries > sc.Limits.SchemaRetryBudget {
					result.Outcome = OutcomeSchemaFailure
					result.OutcomeDetail = map[string]any{
						"attempts":   sc.Limits.SchemaRetryBudget,
						"last_error": err.Error(),
					}
					return e.finalize(result)
				}
				turnMessages = appendAssistantFinal(turnMessages, resp.Final)
				turnMessages = appendSchemaRetryMessage(turnMessages, err)
				continue
			}
			result.Outcome = OutcomeOK
			result.Final = append(json.RawMessage(nil), resp.Final...)
			return e.finalize(result)
		}

		// Step (3) — process each tool call sequentially.
		// First: append the assistant turn so the LLM sees it next iteration.
		turnMessages = appendAssistantToolCalls(turnMessages, resp.ToolCalls)

		for _, call := range resp.ToolCalls {
			seq := len(result.ToolCalls) + 1
			callStart := e.nowFunc()

			// 3a — hallucinated tool name (BS-006). Reject BEFORE any
			// registry lookup that could have side effects, and BEFORE
			// the allowlist check, so a hallucinated name with the
			// shape of an allowed name is still treated as unknown.
			if !Has(call.Name) {
				rec := ExecutedToolCall{
					Seq:             seq,
					Name:            call.Name,
					Arguments:       cloneRaw(call.Arguments),
					Outcome:         OutcomeHallucinatedTool,
					RejectionReason: "unknown_tool",
					LatencyMs:       latencyMs(callStart, e.nowFunc()),
				}
				result.ToolCalls = append(result.ToolCalls, rec)
				e.tracer.RecordRejection(traceID, rec)
				turnMessages = appendToolErrorMessage(turnMessages, call, "tool_not_found", availableNames(toolDefs))
				continue
			}

			// 3b — disallowed tool (BS-003). Tool exists in the global
			// registry but the scenario does not list it.
			if _, ok := allowSet[call.Name]; !ok {
				rec := ExecutedToolCall{
					Seq:             seq,
					Name:            call.Name,
					Arguments:       cloneRaw(call.Arguments),
					Outcome:         OutcomeAllowlistViolation,
					RejectionReason: "not_in_allowlist",
					LatencyMs:       latencyMs(callStart, e.nowFunc()),
				}
				result.ToolCalls = append(result.ToolCalls, rec)
				e.tracer.RecordRejection(traceID, rec)
				turnMessages = appendToolErrorMessage(turnMessages, call, "tool_not_allowed", availableNames(toolDefs))
				continue
			}

			// 3c — argument schema (BS-004).
			inSch, outSch, ok := SchemasFor(call.Name)
			if !ok {
				// Race: tool was unregistered between Has() and now.
				// Treat as unknown_tool to keep behavior consistent.
				rec := ExecutedToolCall{
					Seq:             seq,
					Name:            call.Name,
					Arguments:       cloneRaw(call.Arguments),
					Outcome:         OutcomeHallucinatedTool,
					RejectionReason: "unknown_tool",
					LatencyMs:       latencyMs(callStart, e.nowFunc()),
				}
				result.ToolCalls = append(result.ToolCalls, rec)
				e.tracer.RecordRejection(traceID, rec)
				turnMessages = appendToolErrorMessage(turnMessages, call, "tool_not_found", availableNames(toolDefs))
				continue
			}
			if err := inSch.ValidateBytes(call.Arguments); err != nil {
				rec := ExecutedToolCall{
					Seq:             seq,
					Name:            call.Name,
					Arguments:       cloneRaw(call.Arguments),
					Outcome:         OutcomeToolError,
					RejectionReason: "argument_schema_violation",
					Error:           err.Error(),
					LatencyMs:       latencyMs(callStart, e.nowFunc()),
				}
				result.ToolCalls = append(result.ToolCalls, rec)
				e.tracer.RecordRejection(traceID, rec)
				turnMessages = appendToolErrorMessage(turnMessages, call, "argument_invalid", err.Error())
				continue
			}

			// 3d — dispatch with per-tool deadline.
			toolMeta, _ := ByName(call.Name)
			perToolMs := toolMeta.PerCallTimeoutMs
			if perToolMs <= 0 {
				perToolMs = sc.Limits.PerToolTimeoutMs
			}
			toolCtx, toolCancel := context.WithTimeout(ctx, time.Duration(perToolMs)*time.Millisecond)
			toolResult, toolErr := toolMeta.Handler(toolCtx, call.Arguments)
			toolCancel()

			if toolErr != nil {
				// Tool errors do NOT terminate the loop — the LLM gets
				// to recover (BS-015).
				rec := ExecutedToolCall{
					Seq:             seq,
					Name:            call.Name,
					Arguments:       cloneRaw(call.Arguments),
					Outcome:         OutcomeToolError,
					RejectionReason: "tool_error",
					Error:           toolErr.Error(),
					LatencyMs:       latencyMs(callStart, e.nowFunc()),
				}
				result.ToolCalls = append(result.ToolCalls, rec)
				e.tracer.RecordToolError(traceID, rec)
				turnMessages = appendToolErrorMessage(turnMessages, call, "tool_error", toolErr.Error())
				continue
			}

			// 3e — return schema (BS-005). Failure is terminal.
			if err := outSch.ValidateBytes(toolResult); err != nil {
				rec := ExecutedToolCall{
					Seq:             seq,
					Name:            call.Name,
					Arguments:       cloneRaw(call.Arguments),
					Result:          cloneRaw(toolResult),
					Outcome:         OutcomeToolReturnInvalid,
					RejectionReason: "return_schema_violation",
					Error:           err.Error(),
					LatencyMs:       latencyMs(callStart, e.nowFunc()),
				}
				result.ToolCalls = append(result.ToolCalls, rec)
				e.tracer.RecordReturnInvalid(traceID, rec, err)
				result.Outcome = OutcomeToolReturnInvalid
				result.OutcomeDetail = map[string]any{
					"tool":   call.Name,
					"error":  "return_schema_violation",
					"detail": err.Error(),
				}
				return e.finalize(result)
			}

			rec := ExecutedToolCall{
				Seq:       seq,
				Name:      call.Name,
				Arguments: cloneRaw(call.Arguments),
				Result:    cloneRaw(toolResult),
				Outcome:   OutcomeOK,
				LatencyMs: latencyMs(callStart, e.nowFunc()),
			}
			result.ToolCalls = append(result.ToolCalls, rec)
			e.tracer.RecordToolCall(traceID, rec)
			turnMessages = appendToolResultMessage(turnMessages, call, toolResult)
		}
		// Loop continues — next LLM turn sees the appended results.
	}
}

// finalize stamps EndedAt, calls the tracer, and returns the result so
// callers can chain `return e.finalize(result)` from outcome branches.
func (e *Executor) finalize(r *InvocationResult) *InvocationResult {
	r.EndedAt = e.nowFunc()
	e.tracer.RecordOutcome(r.TraceID, r)
	return r
}

// newTraceID returns a unique trace_id of the shape
// `trace_<rfc3339>_<seq>`. Scope 6 swaps this for a more compact form
// that uses random bytes; the shape here is already unique within a
// process run and is good enough for unit tests.
func (e *Executor) newTraceID() string {
	n := e.traceN.Add(1)
	return fmt.Sprintf("trace_%s_%d", e.nowFunc().UTC().Format("20060102T150405.000000000"), n)
}

// availableNames returns the sorted list of tool names the LLM may use.
// Surfaced to the LLM in tool-error envelopes so it can retry with a
// real allowed name.
func availableNames(defs []LLMToolDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

// latencyMs returns the elapsed milliseconds, clamped to >= 0 so a
// regressed clock cannot produce negative durations in the trace.
func latencyMs(start, end time.Time) int {
	d := end.Sub(start)
	if d < 0 {
		return 0
	}
	return int(d / time.Millisecond)
}

// cloneRaw returns an independent copy of raw so callers cannot mutate
// the executor's recorded view.
func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

// cloneTurnMessages defensively copies the slice handed to the driver
// so a buggy driver cannot mutate the executor's accumulating
// conversation.
func cloneTurnMessages(in []TurnMessage) []TurnMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]TurnMessage, len(in))
	for i, m := range in {
		out[i] = TurnMessage{
			Role:     m.Role,
			ToolName: m.ToolName,
			Content:  cloneRaw(m.Content),
		}
	}
	return out
}

// appendAssistantFinal records the LLM's (rejected) final answer in the
// conversation so the schema-retry follow-up makes sense in context.
func appendAssistantFinal(msgs []TurnMessage, final json.RawMessage) []TurnMessage {
	body, _ := json.Marshal(map[string]json.RawMessage{"final": final})
	return append(msgs, TurnMessage{Role: RoleAssistant, Content: body})
}

// appendAssistantToolCalls records the LLM's tool-call turn so the next
// turn sees its own prior calls.
func appendAssistantToolCalls(msgs []TurnMessage, calls []LLMToolCall) []TurnMessage {
	body, _ := json.Marshal(map[string]any{"tool_calls": calls})
	return append(msgs, TurnMessage{Role: RoleAssistant, Content: body})
}

// appendToolErrorMessage records a per-call rejection back to the LLM in
// a structured shape it can parse on the next turn (BS-003, BS-006).
func appendToolErrorMessage(msgs []TurnMessage, call LLMToolCall, reason string, available any) []TurnMessage {
	body, _ := json.Marshal(map[string]any{
		"error":     reason,
		"tool":      call.Name,
		"available": available,
	})
	return append(msgs, TurnMessage{Role: RoleTool, ToolName: call.Name, Content: body})
}

// appendToolResultMessage records a successful tool result so the next
// turn can incorporate it.
func appendToolResultMessage(msgs []TurnMessage, call LLMToolCall, result json.RawMessage) []TurnMessage {
	body, _ := json.Marshal(map[string]json.RawMessage{"result": result})
	return append(msgs, TurnMessage{Role: RoleTool, ToolName: call.Name, Content: body})
}

// appendSchemaRetryMessage nudges the LLM toward producing a schema-valid
// final answer on the next turn (BS-007).
func appendSchemaRetryMessage(msgs []TurnMessage, schemaErr error) []TurnMessage {
	body, _ := json.Marshal(map[string]string{
		"error":  "output_schema_violation",
		"detail": schemaErr.Error(),
		"hint":   "the previous final answer did not match the scenario output schema; produce a corrected final answer",
	})
	return append(msgs, TurnMessage{Role: RoleSystem, Content: body})
}

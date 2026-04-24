// PostgreSQL-backed Tracer for spec 037 Scope 6.
//
// The PostgresTracer collects per-invocation events in an in-memory
// scratchpad keyed by trace_id and flushes a single transaction on
// RecordOutcome:
//
//   - one row in agent_traces with the denormalized scenario snapshot,
//     input envelope, routing decision, denormalized tool-call array,
//     turn log (when AGENT_TRACE_RECORD_LLM_MESSAGES=true), final
//     output, outcome enum, and outcome detail.
//   - N rows in agent_tool_calls — one per executed tool call,
//     including rejections and tool errors. Concurrent invocations
//     write only their own (trace_id, seq) rows; isolation (BS-018) is
//     trivially guaranteed by the primary key.
//
// The tracer also mirrors two events to the AGENT NATS stream so
// downstream consumers (Operator UI, Scope 8; future analytics) can
// react without polling Postgres:
//
//   - agent.tool_call.executed — one event per ExecutedToolCall.
//   - agent.complete           — one event per terminal outcome.
//
// All publishes are best-effort: a NATS failure is logged but does not
// fail the invocation, because the persisted Postgres trace is the
// authoritative record.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TracePublisher is the minimal interface PostgresTracer needs to mirror
// trace events onto NATS. *nats.Client (internal/nats) satisfies this
// shape via its Publish method. The interface keeps internal/agent free
// of an internal/nats import (avoiding a cycle and keeping the tracer
// trivially mockable in tests).
type TracePublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// NopPublisher discards every publish. Suitable for unit tests that
// don't care about NATS mirroring.
type NopPublisher struct{}

// Publish implements TracePublisher.
func (NopPublisher) Publish(context.Context, string, []byte) error { return nil }

// Subjects mirrored by the tracer (locked to the AGENT NATS contract
// in config/nats_contract.json).
const (
	SubjectToolCallExecuted = "agent.tool_call.executed"
	SubjectAgentComplete    = "agent.complete"
)

// TraceRow is the on-disk shape of agent_traces. Exposed so the replay
// command can deserialize a stored trace without re-implementing the
// schema knowledge.
type TraceRow struct {
	TraceID          string          `json:"trace_id"`
	ScenarioID       string          `json:"scenario_id"`
	ScenarioVersion  string          `json:"scenario_version"`
	ScenarioHash     string          `json:"scenario_hash"`
	ScenarioSnapshot json.RawMessage `json:"scenario_snapshot"`
	Source           string          `json:"source"`
	InputEnvelope    json.RawMessage `json:"input_envelope"`
	Routing          json.RawMessage `json:"routing"`
	ToolCalls        json.RawMessage `json:"tool_calls"`
	TurnLog          json.RawMessage `json:"turn_log"`
	FinalOutput      json.RawMessage `json:"final_output,omitempty"`
	Outcome          string          `json:"outcome"`
	OutcomeDetail    json.RawMessage `json:"outcome_detail,omitempty"`
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	TokensPrompt     int             `json:"tokens_prompt"`
	TokensCompletion int             `json:"tokens_completion"`
	LatencyMs        int             `json:"latency_ms"`
	StartedAt        time.Time       `json:"started_at"`
	EndedAt          time.Time       `json:"ended_at"`
}

// PostgresTracer implements Tracer. Construct one per process via
// NewPostgresTracer and share it across goroutines — every method is
// safe for concurrent use thanks to per-trace-id scratchpad locking
// plus the primary-key isolation in agent_tool_calls.
type PostgresTracer struct {
	pool         *pgxpool.Pool
	publisher    TracePublisher
	recordLLM    bool
	mu           sync.Mutex
	pads         map[string]*tracePad
	publishCtxFn func() (context.Context, context.CancelFunc)
}

// tracePad accumulates per-invocation state from Begin to RecordOutcome.
type tracePad struct {
	tc      TraceContext
	turnLog []recordedTurn
	mu      sync.Mutex
}

// recordedTurn is the persisted projection of a single TurnResponse.
// Stored inside the trace's turn_log JSONB column when the tracer was
// configured with record_llm_messages=true.
type recordedTurn struct {
	Iter      int             `json:"iter"`
	ToolCalls []LLMToolCall   `json:"tool_calls"`
	Final     json.RawMessage `json:"final,omitempty"`
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Tokens    Tokens          `json:"tokens"`
}

// NewPostgresTracer builds a Tracer backed by pool. publisher mirrors
// trace events onto NATS; pass NopPublisher{} to skip mirroring.
// recordLLM enables persisting per-turn LLM responses (turn_log) which
// the replay command needs to reconstruct an invocation.
func NewPostgresTracer(pool *pgxpool.Pool, publisher TracePublisher, recordLLM bool) (*PostgresTracer, error) {
	if pool == nil {
		return nil, errors.New("agent.NewPostgresTracer: pool is required")
	}
	if publisher == nil {
		publisher = NopPublisher{}
	}
	return &PostgresTracer{
		pool:      pool,
		publisher: publisher,
		recordLLM: recordLLM,
		pads:      make(map[string]*tracePad),
		publishCtxFn: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), 5*time.Second)
		},
	}, nil
}

// Begin stashes the invocation context until RecordOutcome flushes it.
func (t *PostgresTracer) Begin(tc TraceContext) {
	if tc.TraceID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pads[tc.TraceID] = &tracePad{tc: tc}
}

// RecordTurn appends the turn to the in-memory turn_log when the tracer
// was constructed with recordLLM=true; otherwise it is a no-op.
func (t *PostgresTracer) RecordTurn(traceID string, iter int, resp TurnResponse) {
	if !t.recordLLM {
		return
	}
	pad := t.padFor(traceID)
	if pad == nil {
		return
	}
	pad.mu.Lock()
	defer pad.mu.Unlock()
	pad.turnLog = append(pad.turnLog, recordedTurn{
		Iter:      iter,
		ToolCalls: append([]LLMToolCall(nil), resp.ToolCalls...),
		Final:     append(json.RawMessage(nil), resp.Final...),
		Provider:  resp.Provider,
		Model:     resp.Model,
		Tokens:    resp.Tokens,
	})
}

// RecordToolCall mirrors a successful tool call to NATS. Persistence
// itself happens in the RecordOutcome flush so the per-call rows and
// the trace row commit as one transaction.
func (t *PostgresTracer) RecordToolCall(traceID string, call ExecutedToolCall) {
	t.publishToolCall(traceID, call)
}

// RecordRejection mirrors a rejected tool call (hallucinated, allowlist,
// argument-schema) to NATS.
func (t *PostgresTracer) RecordRejection(traceID string, call ExecutedToolCall) {
	t.publishToolCall(traceID, call)
}

// RecordToolError mirrors a tool handler error to NATS.
func (t *PostgresTracer) RecordToolError(traceID string, call ExecutedToolCall) {
	t.publishToolCall(traceID, call)
}

// RecordReturnInvalid mirrors a return-schema violation to NATS.
func (t *PostgresTracer) RecordReturnInvalid(traceID string, call ExecutedToolCall, _ error) {
	t.publishToolCall(traceID, call)
}

// RecordSchemaRetry is currently a no-op for the Postgres tracer; the
// schema retry counter ends up in InvocationResult and is persisted by
// RecordOutcome. The hook exists so future telemetry can opt in
// without changing the executor.
func (t *PostgresTracer) RecordSchemaRetry(string, int, error) {}

// RecordOutcome flushes the trace and tool calls to PostgreSQL and
// publishes agent.complete on NATS. Errors are logged (slog) so a
// transient DB hiccup doesn't crash the executor; the operator UI
// will surface gaps.
func (t *PostgresTracer) RecordOutcome(traceID string, result *InvocationResult) {
	t.mu.Lock()
	pad := t.pads[traceID]
	delete(t.pads, traceID)
	t.mu.Unlock()
	if pad == nil {
		slog.Warn("agent tracer: RecordOutcome without Begin", "trace_id", traceID)
		return
	}

	ctx, cancel := t.publishCtxFn()
	defer cancel()

	if err := t.writeTrace(ctx, pad, result); err != nil {
		slog.Error("agent tracer: write trace failed", "trace_id", traceID, "error", err)
		// Still attempt to publish the agent.complete event so
		// downstream consumers see the terminal outcome even if the
		// DB write missed.
	}

	t.publishComplete(ctx, traceID, result)
}

// publishToolCall publishes one agent.tool_call.executed event. NATS
// errors are logged and swallowed.
func (t *PostgresTracer) publishToolCall(traceID string, call ExecutedToolCall) {
	if t.publisher == nil {
		return
	}
	pad := t.padFor(traceID)
	scenarioID, scenarioVersion := "", ""
	if pad != nil {
		scenarioID = pad.tc.Scenario.ID
		scenarioVersion = pad.tc.Scenario.Version
	}
	envelope := map[string]any{
		"trace_id":         traceID,
		"scenario_id":      scenarioID,
		"scenario_version": scenarioVersion,
		"seq":              call.Seq,
		"tool_name":        call.Name,
		"outcome":          string(call.Outcome),
		"rejection_reason": call.RejectionReason,
		"latency_ms":       call.LatencyMs,
	}
	if call.Error != "" {
		envelope["error"] = call.Error
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		slog.Warn("agent tracer: marshal tool_call event", "trace_id", traceID, "error", err)
		return
	}
	ctx, cancel := t.publishCtxFn()
	defer cancel()
	if err := t.publisher.Publish(ctx, SubjectToolCallExecuted, body); err != nil {
		slog.Warn("agent tracer: publish tool_call event", "trace_id", traceID, "error", err)
	}
}

// publishComplete publishes one agent.complete event.
func (t *PostgresTracer) publishComplete(ctx context.Context, traceID string, result *InvocationResult) {
	if t.publisher == nil {
		return
	}
	envelope := map[string]any{
		"trace_id":          traceID,
		"scenario_id":       result.ScenarioID,
		"scenario_version":  result.ScenarioVersion,
		"outcome":           string(result.Outcome),
		"iterations":        result.Iterations,
		"schema_retries":    result.SchemaRetries,
		"tool_calls_count":  len(result.ToolCalls),
		"latency_ms":        latencyMs(result.StartedAt, result.EndedAt),
		"tokens_prompt":     result.TokensPrompt,
		"tokens_completion": result.TokensCompletion,
		"provider":          result.Provider,
		"model":             result.Model,
	}
	if result.OutcomeDetail != nil {
		envelope["outcome_detail"] = result.OutcomeDetail
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		slog.Warn("agent tracer: marshal complete event", "trace_id", traceID, "error", err)
		return
	}
	if err := t.publisher.Publish(ctx, SubjectAgentComplete, body); err != nil {
		slog.Warn("agent tracer: publish complete event", "trace_id", traceID, "error", err)
	}
}

// writeTrace inserts the agent_traces row and N agent_tool_calls rows in
// one transaction. The denormalized tool_calls JSONB column on the
// trace row is the fast path for list/detail UIs; the per-call rows
// are the authoritative record used by replay.
func (t *PostgresTracer) writeTrace(ctx context.Context, pad *tracePad, result *InvocationResult) error {
	if t.pool == nil {
		// No DB attached — used by publisher-only unit tests. The
		// agent.complete event still publishes via the caller.
		return nil
	}
	scenarioSnapshot, err := buildScenarioSnapshot(pad.tc.Scenario)
	if err != nil {
		return fmt.Errorf("scenario snapshot: %w", err)
	}
	envelopeJSON, err := buildEnvelopeJSON(pad.tc.Envelope)
	if err != nil {
		return fmt.Errorf("input envelope: %w", err)
	}
	routingJSON, err := json.Marshal(pad.tc.Decision)
	if err != nil {
		return fmt.Errorf("routing decision: %w", err)
	}
	toolCallsJSON, err := json.Marshal(result.ToolCalls)
	if err != nil {
		return fmt.Errorf("tool calls snapshot: %w", err)
	}
	pad.mu.Lock()
	turnLogCopy := append([]recordedTurn(nil), pad.turnLog...)
	pad.mu.Unlock()
	turnLogJSON, err := json.Marshal(turnLogCopy)
	if err != nil {
		return fmt.Errorf("turn log: %w", err)
	}
	var finalJSON []byte
	if len(result.Final) > 0 {
		finalJSON = []byte(result.Final)
	}
	var outcomeDetailJSON []byte
	if result.OutcomeDetail != nil {
		outcomeDetailJSON, err = json.Marshal(result.OutcomeDetail)
		if err != nil {
			return fmt.Errorf("outcome detail: %w", err)
		}
	}

	source := pad.tc.Envelope.Source
	if source == "" {
		source = "unknown"
	}

	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
INSERT INTO agent_traces (
    trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
    source, input_envelope, routing, tool_calls, turn_log,
    final_output, outcome, outcome_detail,
    provider, model, tokens_prompt, tokens_completion,
    latency_ms, started_at, ended_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13,
    $14, $15, $16, $17,
    $18, $19, $20
)`,
		pad.tc.TraceID,
		pad.tc.Scenario.ID,
		pad.tc.Scenario.Version,
		pad.tc.Scenario.ContentHash,
		scenarioSnapshot,
		source,
		envelopeJSON,
		routingJSON,
		toolCallsJSON,
		turnLogJSON,
		nullableJSON(finalJSON),
		string(result.Outcome),
		nullableJSON(outcomeDetailJSON),
		result.Provider,
		result.Model,
		result.TokensPrompt,
		result.TokensCompletion,
		latencyMs(result.StartedAt, result.EndedAt),
		result.StartedAt.UTC(),
		result.EndedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert agent_traces: %w", err)
	}

	for _, c := range result.ToolCalls {
		// Look up side_effect_class from the registry; if the tool was
		// removed mid-run (or was rejected as hallucinated and never
		// existed), persist "unknown" so the row remains queryable.
		side := "unknown"
		if tool, ok := ByName(c.Name); ok {
			side = string(tool.SideEffectClass)
		}
		argsJSON := []byte(c.Arguments)
		if len(argsJSON) == 0 {
			argsJSON = []byte("null")
		}
		var resultJSON []byte
		if len(c.Result) > 0 {
			resultJSON = []byte(c.Result)
		}
		var rejReason any
		if c.RejectionReason != "" {
			rejReason = c.RejectionReason
		}
		var errStr any
		if c.Error != "" {
			errStr = c.Error
		}
		_, err = tx.Exec(ctx, `
INSERT INTO agent_tool_calls (
    trace_id, seq, tool_name, side_effect_class,
    arguments, result, rejection_reason, error,
    latency_ms, started_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10
)`,
			pad.tc.TraceID,
			c.Seq,
			c.Name,
			side,
			argsJSON,
			nullableJSON(resultJSON),
			rejReason,
			errStr,
			c.LatencyMs,
			pad.tc.StartedAt.UTC(),
		)
		if err != nil {
			return fmt.Errorf("insert agent_tool_calls seq=%d: %w", c.Seq, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// padFor returns the in-flight tracePad for traceID or nil.
func (t *PostgresTracer) padFor(traceID string) *tracePad {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pads[traceID]
}

// buildScenarioSnapshot freezes the scenario as it was at invocation
// start. Hot reload (BS-019) replaces the global registry with newer
// versions, but each in-flight invocation still records the bytes it
// began with.
func buildScenarioSnapshot(sc *Scenario) ([]byte, error) {
	if sc == nil {
		return nil, errors.New("nil scenario")
	}
	allowed := make([]map[string]any, 0, len(sc.AllowedTools))
	for _, at := range sc.AllowedTools {
		allowed = append(allowed, map[string]any{
			"name":              at.Name,
			"side_effect_class": string(at.SideEffectClass),
		})
	}
	snap := map[string]any{
		"id":              sc.ID,
		"version":         sc.Version,
		"description":     sc.Description,
		"intent_examples": sc.IntentExamples,
		"system_prompt":   sc.SystemPrompt,
		"allowed_tools":   allowed,
		"input_schema":    json.RawMessage(sc.InputSchema),
		"output_schema":   json.RawMessage(sc.OutputSchema),
		"limits": map[string]any{
			"max_loop_iterations": sc.Limits.MaxLoopIterations,
			"timeout_ms":          sc.Limits.TimeoutMs,
			"schema_retry_budget": sc.Limits.SchemaRetryBudget,
			"per_tool_timeout_ms": sc.Limits.PerToolTimeoutMs,
		},
		"token_budget":      sc.TokenBudget,
		"temperature":       sc.Temperature,
		"model_preference":  sc.ModelPreference,
		"side_effect_class": string(sc.SideEffectClass),
		"content_hash":      sc.ContentHash,
		"source_path":       sc.SourcePath,
	}
	return json.Marshal(snap)
}

// buildEnvelopeJSON projects the IntentEnvelope to a queryable JSON
// payload. The structured_context field is preserved verbatim so
// replay can re-feed the executor with the exact same input.
func buildEnvelopeJSON(env IntentEnvelope) ([]byte, error) {
	payload := map[string]any{
		"source":           env.Source,
		"raw_input":        env.RawInput,
		"scenario_id":      env.ScenarioID,
		"confidence_floor": env.ConfidenceFloor,
	}
	if len(env.StructuredContext) > 0 {
		payload["structured_context"] = json.RawMessage(env.StructuredContext)
	}
	return json.Marshal(payload)
}

// nullableJSON returns nil for an empty buffer so pgx writes SQL NULL.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

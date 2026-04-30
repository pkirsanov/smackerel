package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	FeedbackScenarioID      = "recommendation_feedback"
	FeedbackScenarioVersion = "recommendation-feedback-v1"
	WhyScenarioID           = "recommendation_why"
	WhyScenarioVersion      = "recommendation-why-v1"
)

func insertScenarioTrace(ctx context.Context, tx pgx.Tx, input scenarioTraceInput) (string, error) {
	traceID, err := newTextID("rec_trace")
	if err != nil {
		return "", err
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	endedAt := input.EndedAt
	if endedAt.IsZero() {
		endedAt = startedAt
	}
	scenarioSnapshot, err := marshalAny(map[string]any{
		"id":      input.ScenarioID,
		"version": input.ScenarioVersion,
		"scope":   "scope-03-feedback-suppression-why",
	})
	if err != nil {
		return "", fmt.Errorf("marshal scenario snapshot: %w", err)
	}
	inputEnvelope, err := marshalAny(input.InputEnvelope)
	if err != nil {
		return "", fmt.Errorf("marshal input envelope: %w", err)
	}
	routing, err := marshalAny(map[string]any{"scenario_id": input.ScenarioID, "status": input.Outcome})
	if err != nil {
		return "", fmt.Errorf("marshal routing: %w", err)
	}
	toolCallsJSON, err := marshalAny(input.ToolCalls)
	if err != nil {
		return "", fmt.Errorf("marshal tool calls: %w", err)
	}
	finalOutput, err := marshalAny(input.FinalOutput)
	if err != nil {
		return "", fmt.Errorf("marshal final output: %w", err)
	}
	outcomeDetail, err := marshalAny(input.OutcomeDetail)
	if err != nil {
		return "", fmt.Errorf("marshal outcome detail: %w", err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO agent_traces (
    trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
    source, input_envelope, routing, tool_calls, turn_log,
    final_output, outcome, outcome_detail,
    provider, model, tokens_prompt, tokens_completion,
    latency_ms, started_at, ended_at, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13,
    $14, $15, $16, $17,
    $18, $19, $20, $21
)`,
		traceID,
		input.ScenarioID,
		input.ScenarioVersion,
		input.ScenarioVersion,
		scenarioSnapshot,
		input.Source,
		inputEnvelope,
		routing,
		toolCallsJSON,
		[]byte("[]"),
		finalOutput,
		input.Outcome,
		outcomeDetail,
		"",
		"",
		0,
		0,
		int(endedAt.Sub(startedAt).Milliseconds()),
		startedAt,
		endedAt,
		endedAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert recommendation scenario trace: %w", err)
	}
	for seq, call := range input.ToolCalls {
		arguments, err := marshalAny(call.Arguments)
		if err != nil {
			return "", fmt.Errorf("marshal tool arguments %s: %w", call.Name, err)
		}
		result, err := marshalAny(call.Result)
		if err != nil {
			return "", fmt.Errorf("marshal tool result %s: %w", call.Name, err)
		}
		started := call.StartedAt
		if started.IsZero() {
			started = startedAt
		}
		_, err = tx.Exec(ctx, `
INSERT INTO agent_tool_calls (
    trace_id, seq, tool_name, side_effect_class, arguments, result,
    rejection_reason, error, latency_ms, started_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			traceID,
			seq+1,
			call.Name,
			call.SideEffectClass,
			arguments,
			result,
			call.RejectionReason,
			call.Error,
			call.LatencyMillis,
			started,
		)
		if err != nil {
			return "", fmt.Errorf("insert recommendation scenario tool call %s: %w", call.Name, err)
		}
	}
	return traceID, nil
}

type scenarioTraceInput struct {
	ScenarioID      string
	ScenarioVersion string
	Source          string
	Outcome         string
	InputEnvelope   map[string]any
	FinalOutput     map[string]any
	OutcomeDetail   map[string]any
	ToolCalls       []ToolCallRecord
	StartedAt       time.Time
	EndedAt         time.Time
}

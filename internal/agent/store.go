// Trace-store query helpers for spec 037 Scope 8 operator UI.
//
// The Scope 6 tracer writes agent_traces and agent_tool_calls; Scope 8
// reads them back. List queries page on (created_at DESC) and use the
// `outcome` index for the operator's outcome filter (BS-012, design §8).
//
// The functions here intentionally return TraceRow slices so the
// render package can build identical views for CLI and web.

package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TraceListFilter narrows which traces ListTraces returns. Empty fields
// mean "no filter applied".
type TraceListFilter struct {
	Outcome string // exact match on agent_traces.outcome (indexed)
}

// ListTraces returns up to limit trace rows newest-first, skipping the
// first offset rows. limit MUST be > 0; offset MUST be >= 0. The
// scenario_snapshot, turn_log, and final_output columns are NOT
// returned by the list query — they can be large; callers that need
// them must call LoadTrace.
func ListTraces(ctx context.Context, pool *pgxpool.Pool, filter TraceListFilter, limit, offset int) ([]TraceRow, error) {
	if pool == nil {
		return nil, errors.New("agent.ListTraces: pool is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("agent.ListTraces: limit must be > 0, got %d", limit)
	}
	if offset < 0 {
		return nil, fmt.Errorf("agent.ListTraces: offset must be >= 0, got %d", offset)
	}

	const baseQuery = `
SELECT
    trace_id, scenario_id, scenario_version, scenario_hash,
    source, input_envelope, routing, tool_calls,
    outcome, outcome_detail,
    provider, model, tokens_prompt, tokens_completion,
    latency_ms, started_at, ended_at
FROM agent_traces
`
	args := []any{limit, offset}
	q := baseQuery
	if filter.Outcome != "" {
		q += "WHERE outcome = $3\n"
		args = append(args, filter.Outcome)
	}
	q += "ORDER BY created_at DESC\nLIMIT $1 OFFSET $2"

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query agent_traces: %w", err)
	}
	defer rows.Close()

	var out []TraceRow
	for rows.Next() {
		var tr TraceRow
		var (
			finalCol  []byte
			detailCol []byte
		)
		// scenario_snapshot, turn_log, final_output are NOT scanned in
		// the list query (size). Substitute an empty placeholder for
		// the unused fields so existing TraceRow consumers see zero.
		if err := rows.Scan(
			&tr.TraceID, &tr.ScenarioID, &tr.ScenarioVersion, &tr.ScenarioHash,
			&tr.Source, &tr.InputEnvelope, &tr.Routing, &tr.ToolCalls,
			&tr.Outcome, &detailCol,
			&tr.Provider, &tr.Model, &tr.TokensPrompt, &tr.TokensCompletion,
			&tr.LatencyMs, &tr.StartedAt, &tr.EndedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent_traces: %w", err)
		}
		_ = finalCol
		if len(detailCol) > 0 {
			tr.OutcomeDetail = detailCol
		}
		out = append(out, tr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent_traces: %w", err)
	}
	return out, nil
}

// CountTraces returns the row count matching filter. Used by the web
// pager to compute total pages.
func CountTraces(ctx context.Context, pool *pgxpool.Pool, filter TraceListFilter) (int, error) {
	if pool == nil {
		return 0, errors.New("agent.CountTraces: pool is required")
	}
	q := "SELECT COUNT(*) FROM agent_traces"
	var args []any
	if filter.Outcome != "" {
		q += " WHERE outcome = $1"
		args = append(args, filter.Outcome)
	}
	var n int
	row := pool.QueryRow(ctx, q, args...)
	if err := row.Scan(&n); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("count agent_traces: %w", err)
	}
	return n, nil
}

//go:build integration

// Spec 076 SCOPE-2a — TP-076-02a-TR.
//
// Live-stack integration test for the assistant_tool_traces writer.
// Verifies the contract the agent loop now depends on:
//
//   1. one row per Write() call;
//   2. call_outcome ∈ {running, succeeded, failed, refused} round-trips
//      exactly (CHECK constraint, NOT NULL);
//   3. lifecycle_state is independently seeded to 'active' and is
//      NOT collapsed into call_outcome — adversarial assertion that
//      catches a regression where the columns are swapped or merged;
//   4. payload_redacted carries only tool_name, arg_keys, and outcome
//      (no raw values), proving the redaction layer is in force.

package openknowledge_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestToolTraceWriter_PersistsLifecycleState(t *testing.T) {
	pool := newTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	turnID := fmt.Sprintf("trace-%s-%d", t.Name(), time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM assistant_tool_traces WHERE turn_id = $1", turnID)
	})

	w := tracewriter.New(pool)

	calls := []tracewriter.Entry{
		{TurnID: turnID, ToolName: "unit_convert", ArgKeys: []string{"from", "to", "value"}, CallOutcome: tracewriter.OutcomeSucceeded},
		{TurnID: turnID, ToolName: "web_search", ArgKeys: []string{"query"}, CallOutcome: tracewriter.OutcomeFailed, ErrorCode: "provider_circuit_open"},
		{TurnID: turnID, ToolName: "calculator", ArgKeys: []string{"expr"}, CallOutcome: tracewriter.OutcomeRefused, ErrorCode: "budget_exhausted"},
		{TurnID: turnID, ToolName: "entity_resolve", ArgKeys: []string{"name"}, CallOutcome: tracewriter.OutcomeRunning},
	}
	for i, e := range calls {
		if err := w.Write(ctx, e); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM assistant_tool_traces WHERE turn_id = $1", turnID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != len(calls) {
		t.Fatalf("row count = %d, want %d", count, len(calls))
	}

	rows, err := pool.Query(ctx,
		`SELECT tool_name, lifecycle_state, call_outcome, payload_redacted
		   FROM assistant_tool_traces
		  WHERE turn_id = $1
		  ORDER BY id`, turnID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	seenOutcomes := map[string]bool{}
	idx := 0
	for rows.Next() {
		var (
			toolName       string
			lifecycleState string
			callOutcome    string
			payloadBytes   []byte
		)
		if err := rows.Scan(&toolName, &lifecycleState, &callOutcome, &payloadBytes); err != nil {
			t.Fatalf("scan row %d: %v", idx, err)
		}
		if lifecycleState != "active" {
			t.Errorf("row %d (%s): lifecycle_state = %q, want active",
				idx, toolName, lifecycleState)
		}
		if callOutcome == "" {
			t.Errorf("row %d (%s): call_outcome is empty", idx, toolName)
		}
		if callOutcome == lifecycleState {
			t.Errorf("row %d (%s): call_outcome (%q) collapsed into lifecycle_state — columns are not distinct",
				idx, toolName, callOutcome)
		}
		switch callOutcome {
		case "running", "succeeded", "failed", "refused":
			seenOutcomes[callOutcome] = true
		default:
			t.Errorf("row %d (%s): call_outcome = %q outside vocabulary",
				idx, toolName, callOutcome)
		}

		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			t.Errorf("row %d (%s): payload not JSON: %v", idx, toolName, err)
		} else {
			if payload["tool_name"] != toolName {
				t.Errorf("row %d: payload.tool_name = %v, want %s", idx, payload["tool_name"], toolName)
			}
			if _, ok := payload["arg_keys"]; !ok {
				t.Errorf("row %d: payload missing arg_keys: %v", idx, payload)
			}
			for _, leak := range []string{"value", "query", "expr", "name"} {
				if _, present := payload[leak]; present {
					t.Errorf("row %d: redacted payload leaked raw arg %q: %v", idx, leak, payload)
				}
			}
		}
		idx++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	for _, want := range []string{"running", "succeeded", "failed", "refused"} {
		if !seenOutcomes[want] {
			t.Errorf("call_outcome %q not observed in round-trip", want)
		}
	}
}

// TestToolTraceWriter_RejectsInvalidOutcome locks in the per-row
// validation contract before the row reaches the database. Adversarial
// proof that the writer is not silently coercing unknown values.
func TestToolTraceWriter_RejectsInvalidOutcome(t *testing.T) {
	pool := newTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := tracewriter.New(pool)
	err := w.Write(ctx, tracewriter.Entry{
		TurnID:      fmt.Sprintf("trace-invalid-%d", time.Now().UnixNano()),
		ToolName:    "unit_convert",
		CallOutcome: tracewriter.CallOutcome("bogus"),
	})
	if err == nil {
		t.Fatal("expected error for bogus call_outcome, got nil")
	}
}

// Package tracewriter persists per-tool-call rows to
// `assistant_tool_traces` (migrations 053 + 054) for the spec 064
// open-knowledge agent loop.
//
// Spec 076 SCOPE-2a contract:
//   - exactly one row per tool invocation
//   - `call_outcome` ∈ {running, succeeded, failed, refused}, NOT NULL
//   - `lifecycle_state` is the prune-lifecycle column owned by the
//     lifecycle worker; this writer always seeds it to `active`.
//   - `payload_redacted` carries tool name, arg-key list, and outcome
//     only — never raw arg values, raw tool results, URLs, or secrets.
package tracewriter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CallOutcome is the per-tool-call outcome vocabulary persisted in
// the `call_outcome` column (migration 054). Values match the
// CHECK constraint exactly.
type CallOutcome string

const (
	OutcomeRunning   CallOutcome = "running"
	OutcomeSucceeded CallOutcome = "succeeded"
	OutcomeFailed    CallOutcome = "failed"
	OutcomeRefused   CallOutcome = "refused"
)

// LifecycleActive is the initial prune-lifecycle value the writer
// seeds. The lifecycle worker (separate scope) transitions rows to
// `cooling`/`pruned` over time.
const LifecycleActive = "active"

// Entry is one persisted tool-call row.
type Entry struct {
	TurnID      string
	ToolName    string
	ArgKeys     []string
	CallOutcome CallOutcome
	ErrorCode   string
	CreatedAt   time.Time
}

// Writer is the narrow contract the agent loop depends on.
type Writer interface {
	Write(ctx context.Context, e Entry) error
}

// PgxWriter is the production Writer backed by a pgxpool.
type PgxWriter struct {
	pool *pgxpool.Pool
}

// New returns a PgxWriter that writes through the supplied pool.
func New(pool *pgxpool.Pool) *PgxWriter {
	if pool == nil {
		panic("tracewriter: pool is required")
	}
	return &PgxWriter{pool: pool}
}

// Write persists one row. Returns an error if validation fails or
// the INSERT fails.
func (w *PgxWriter) Write(ctx context.Context, e Entry) error {
	payload, err := validateAndBuildPayload(e)
	if err != nil {
		return err
	}
	ts := e.CreatedAt
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	_, err = w.pool.Exec(ctx,
		`INSERT INTO assistant_tool_traces
		   (turn_id, tool_name, payload_redacted, lifecycle_state, call_outcome, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.TurnID, e.ToolName, payload, LifecycleActive, string(e.CallOutcome), ts,
	)
	if err != nil {
		return fmt.Errorf("tracewriter: insert: %w", err)
	}
	return nil
}

// Nop is a Writer that drops every entry. Used as the default for
// tests and any wiring that has not yet bound a database.
type Nop struct{}

// Write satisfies Writer.
func (Nop) Write(context.Context, Entry) error { return nil }

// validateAndBuildPayload enforces the per-row invariants and returns
// the JSONB payload that goes into `payload_redacted`. The payload
// records the tool name, sorted arg-key set, outcome, and (if
// failed/refused) an error code — never arg values or raw results.
func validateAndBuildPayload(e Entry) ([]byte, error) {
	if e.TurnID == "" {
		return nil, errors.New("tracewriter: turn_id required")
	}
	if e.ToolName == "" {
		return nil, errors.New("tracewriter: tool_name required")
	}
	switch e.CallOutcome {
	case OutcomeRunning, OutcomeSucceeded, OutcomeFailed, OutcomeRefused:
	default:
		return nil, fmt.Errorf("tracewriter: invalid call_outcome %q", e.CallOutcome)
	}
	keys := append([]string(nil), e.ArgKeys...)
	sort.Strings(keys)
	doc := map[string]any{
		"tool_name": e.ToolName,
		"arg_keys":  keys,
		"outcome":   string(e.CallOutcome),
	}
	if e.ErrorCode != "" {
		doc["error_code"] = e.ErrorCode
	}
	return json.Marshal(doc)
}

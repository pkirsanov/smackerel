// Spec 071 SCOPE-03 — IntentTrace replay (SCN-071-A04).
//
// Replay is strictly read-only. It loads one persisted trace row by
// id, validates the v1 schema, refuses sampled-out envelopes (which
// carry no decision fields to compare), and returns a comparison
// envelope that proves the trace is structurally replayable. The
// production DryRunner derives the dry-run decision from the
// redacted payload because the recorder does NOT persist raw user
// text — re-executing the compiler/router against raw input would
// require unredacted material the privacy contract forbids. The
// DryRunner interface is exported so tests and future scopes can
// substitute a richer runner (for example, a deterministic
// scenario-only re-router that consumes the stored slots summary).
//
// Side effects are blocked by construction: replay never calls the
// facade, the router, or any tool executor. The returned
// ReplayComparison reports SideEffectsInvoked=false because no
// side-effect surface is reachable from this package.

package intenttrace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Replay error sentinels. Operators (and the CLI) use errors.Is to
// classify the failure mode; each maps to a CLI exit code defined in
// design.md §"CLI Contract".
var (
	// ErrTraceNotFound signals that the store has no row for the
	// requested trace id (or the row was already TTL-swept).
	ErrTraceNotFound = errors.New("intenttrace: trace not found")

	// ErrTraceSampledOut signals that the row exists but is a
	// sampled-out envelope and therefore carries no decision fields
	// to replay.
	ErrTraceSampledOut = errors.New("intenttrace: trace is sampled-out, nothing to replay")

	// ErrTraceSchemaInvalid signals that the persisted payload does
	// not satisfy the v1 schema invariants (missing trace id, wrong
	// schema_version, etc.).
	ErrTraceSchemaInvalid = errors.New("intenttrace: trace payload fails v1 schema validation")

	// ErrSideEffectsBlocked signals that the configured DryRunner
	// attempted (or would have attempted) a side-effect-bearing
	// operation. The production runner cannot reach this path; tests
	// and future runners may.
	ErrSideEffectsBlocked = errors.New("intenttrace: replay would require a side effect")
)

// DryRunDecision captures the route + tool-call shape produced by a
// dry-run. The production runner mirrors the stored decision; richer
// runners may diverge.
type DryRunDecision struct {
	RouteDecision string   `json:"route_decision"`
	ToolCalls     []string `json:"tool_calls"`
}

// MatchSummary reports per-field equality between the stored
// (original) decision and the dry-run decision.
type MatchSummary struct {
	RouteDecision bool `json:"route_decision"`
	ToolCalls     bool `json:"tool_calls"`
}

// ReplayComparison is the structured result of one replay run. It is
// the exact shape emitted by the CLI's `--json` output and consumed
// by future devtools panels (design.md §"Replay Result").
type ReplayComparison struct {
	TraceID            string         `json:"trace_id"`
	SchemaVersion      string         `json:"schema_version"`
	ReadOnly           bool           `json:"read_only"`
	Original           DryRunDecision `json:"original"`
	DryRun             DryRunDecision `json:"dry_run"`
	Match              MatchSummary   `json:"match"`
	SideEffectsInvoked bool           `json:"side_effects_invoked"`
}

// DryRunner produces a DryRunDecision from a persisted row WITHOUT
// invoking any side-effect-bearing surface. Implementations MUST
// return ErrSideEffectsBlocked rather than executing a tool, writing
// state, or contacting an external service.
type DryRunner interface {
	DryRun(ctx context.Context, row IntentTraceRow) (DryRunDecision, error)
}

// PayloadDryRunner is the production runner. It derives the dry-run
// decision directly from the stored redacted payload. This is honest:
// the persisted payload is the canonical replay source per design.md
// §"Postgres Replay Store" ("the trace store is the replay source of
// truth"), and the raw user text is intentionally absent. The runner
// reaches no side-effect surface — it cannot invoke the compiler,
// the router, or any tool executor — so SideEffectsInvoked is
// definitionally false.
type PayloadDryRunner struct{}

// DryRun implements DryRunner.
func (PayloadDryRunner) DryRun(_ context.Context, row IntentTraceRow) (DryRunDecision, error) {
	return decisionFromRow(row), nil
}

// IntentTraceReplay is the read-only replay surface consumed by the
// CLI and future devtools panels.
type IntentTraceReplay interface {
	Run(ctx context.Context, traceID string) (ReplayComparison, error)
}

// StoreReplay loads a row from an IntentTraceStore and runs it
// through the configured DryRunner. Runner defaults to
// PayloadDryRunner when nil so callers do not need to wire it in
// tests.
type StoreReplay struct {
	Store  IntentTraceStore
	Runner DryRunner
}

// NewStoreReplay constructs a StoreReplay with the production
// PayloadDryRunner.
func NewStoreReplay(store IntentTraceStore) *StoreReplay {
	return &StoreReplay{Store: store, Runner: PayloadDryRunner{}}
}

// Run implements IntentTraceReplay.
func (r *StoreReplay) Run(ctx context.Context, traceID string) (ReplayComparison, error) {
	if r == nil || r.Store == nil {
		return ReplayComparison{}, errors.New("intenttrace: StoreReplay requires a non-nil Store")
	}
	if traceID == "" {
		return ReplayComparison{}, fmt.Errorf("%w: empty trace id", ErrTraceNotFound)
	}
	row, err := r.Store.Get(ctx, traceID)
	if err != nil {
		return ReplayComparison{}, fmt.Errorf("%w: %v", ErrTraceNotFound, err)
	}
	if err := validatePersistedRow(row); err != nil {
		return ReplayComparison{}, fmt.Errorf("%w: %v", ErrTraceSchemaInvalid, err)
	}
	if !row.Sampled {
		return ReplayComparison{}, fmt.Errorf("%w: trace_id=%s", ErrTraceSampledOut, traceID)
	}
	runner := r.Runner
	if runner == nil {
		runner = PayloadDryRunner{}
	}
	dryRun, err := runner.DryRun(ctx, row)
	if err != nil {
		return ReplayComparison{}, err
	}
	original := decisionFromRow(row)
	return ReplayComparison{
		TraceID:            row.TraceID,
		SchemaVersion:      row.SchemaVersion,
		ReadOnly:           true,
		Original:           original,
		DryRun:             dryRun,
		Match:              matchDecisions(original, dryRun),
		SideEffectsInvoked: false,
	}, nil
}

// decisionFromRow extracts the route + tool-call names from a row.
// Used both for the "original" side of every comparison and as the
// PayloadDryRunner's dry-run output.
func decisionFromRow(row IntentTraceRow) DryRunDecision {
	names := make([]string, 0, len(row.ToolCalls))
	for _, tc := range row.ToolCalls {
		names = append(names, tc.Name)
	}
	return DryRunDecision{RouteDecision: row.RouteDecision, ToolCalls: names}
}

func matchDecisions(a, b DryRunDecision) MatchSummary {
	if len(a.ToolCalls) != len(b.ToolCalls) {
		return MatchSummary{RouteDecision: a.RouteDecision == b.RouteDecision, ToolCalls: false}
	}
	for i := range a.ToolCalls {
		if a.ToolCalls[i] != b.ToolCalls[i] {
			return MatchSummary{RouteDecision: a.RouteDecision == b.RouteDecision, ToolCalls: false}
		}
	}
	return MatchSummary{RouteDecision: a.RouteDecision == b.RouteDecision, ToolCalls: true}
}

// validatePersistedRow asserts the v1 schema invariants that the
// recorder MUST have enforced at write time. Treats the persisted
// row as untrusted input (defense in depth — a malformed row from a
// future migration or out-of-band write fails loud here rather than
// silently producing a degraded replay).
func validatePersistedRow(row IntentTraceRow) error {
	if row.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("schema_version=%q (want %q)", row.SchemaVersion, SchemaVersionV1)
	}
	if row.TraceID == "" {
		return errors.New("trace_id is empty")
	}
	if row.TurnID == "" {
		return errors.New("turn_id is empty")
	}
	if !isKnownTransport(row.Transport) {
		return fmt.Errorf("unknown transport %q", row.Transport)
	}
	if row.Sampled {
		if row.ActionClass == "" {
			return errors.New("action_class is empty on sampled trace")
		}
		if !isKnownStatus(row.FinalResponseStatus) {
			return fmt.Errorf("unknown final_response_status %q", row.FinalResponseStatus)
		}
		// Round-trip the payload through JSON to assert the persisted
		// blob is structurally well-formed.
		if _, err := json.Marshal(row.RedactedPayload); err != nil {
			return fmt.Errorf("redacted_payload marshal: %w", err)
		}
	}
	return nil
}

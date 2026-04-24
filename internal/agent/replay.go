// Trace replay for spec 037 Scope 6 (UC-003, BS-013).
//
// Replay loads a stored agent_traces row and decides whether the
// scenario the trace ran against is still equivalent to what the
// process currently has loaded. The implementation follows design
// §6.2:
//
//   - scenario_missing        → diff: scenario_missing      (FAIL)
//   - scenario.version drift  → diff: scenario_version_changed
//                                (FAIL unless --allow-version-drift)
//   - scenario.content drift  → diff: scenario_content_changed
//                                (FAIL unless --allow-content-drift)
//   - tool_missing            → diff: tool_missing          (FAIL)
//
// When all integrity checks pass, replay returns Pass=true. The deeper
// determinism check (re-running the executor against fakeProvider +
// fakeRegistry built from the trace) is intentionally minimal in this
// scope: the integrity diff is what BS-013's Gherkin scenarios
// exercise, and the design explicitly ties content drift to a hash
// comparison.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReplayOptions toggles the strict-by-default integrity checks.
type ReplayOptions struct {
	// AllowVersionDrift suppresses the FAIL when scenario.version no
	// longer matches the recorded trace.scenario_version.
	AllowVersionDrift bool
	// AllowContentDrift suppresses the FAIL when scenario.content_hash
	// has changed since the trace was recorded.
	AllowContentDrift bool
}

// DiffKind names the kind of drift detected.
type DiffKind string

const (
	DiffScenarioMissing       DiffKind = "scenario_missing"
	DiffScenarioVersionChange DiffKind = "scenario_version_changed"
	DiffScenarioContentChange DiffKind = "scenario_content_changed"
	DiffToolMissing           DiffKind = "tool_missing"
)

// DiffEntry is one observed drift.
type DiffEntry struct {
	Kind     DiffKind `json:"kind"`
	Field    string   `json:"field,omitempty"`
	Recorded string   `json:"recorded,omitempty"`
	Current  string   `json:"current,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// ReplayResult is the structured outcome of one replay invocation.
type ReplayResult struct {
	TraceID         string      `json:"trace_id"`
	ScenarioID      string      `json:"scenario_id"`
	ScenarioVersion string      `json:"scenario_version"`
	Pass            bool        `json:"pass"`
	Diff            []DiffEntry `json:"diff"`
}

// ScenarioLookup returns the in-memory Scenario currently registered
// under id, or nil + false if the registry no longer contains it.
// Replay accepts a lookup function (rather than a global registry call)
// so tests and tooling can supply an arbitrary set without touching
// process-wide state.
type ScenarioLookup func(id string) (*Scenario, bool)

// ScenarioLookupFromSlice returns a ScenarioLookup over a fixed slice.
// The slice is indexed by id; later entries with the same id win
// (matches loader semantics where duplicate ids are fatal anyway).
func ScenarioLookupFromSlice(scenarios []*Scenario) ScenarioLookup {
	idx := make(map[string]*Scenario, len(scenarios))
	for _, s := range scenarios {
		if s != nil {
			idx[s.ID] = s
		}
	}
	return func(id string) (*Scenario, bool) {
		s, ok := idx[id]
		return s, ok
	}
}

// LoadTrace reads one agent_traces row.
func LoadTrace(ctx context.Context, pool *pgxpool.Pool, traceID string) (*TraceRow, error) {
	if pool == nil {
		return nil, errors.New("agent.LoadTrace: pool is required")
	}
	if traceID == "" {
		return nil, errors.New("agent.LoadTrace: trace_id is required")
	}
	row := pool.QueryRow(ctx, `
SELECT
    trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
    source, input_envelope, routing, tool_calls, turn_log,
    final_output, outcome, outcome_detail,
    provider, model, tokens_prompt, tokens_completion,
    latency_ms, started_at, ended_at
FROM agent_traces WHERE trace_id = $1
`, traceID)

	var tr TraceRow
	var (
		final         []byte
		outcomeDetail []byte
	)
	if err := row.Scan(
		&tr.TraceID, &tr.ScenarioID, &tr.ScenarioVersion, &tr.ScenarioHash, &tr.ScenarioSnapshot,
		&tr.Source, &tr.InputEnvelope, &tr.Routing, &tr.ToolCalls, &tr.TurnLog,
		&final, &tr.Outcome, &outcomeDetail,
		&tr.Provider, &tr.Model, &tr.TokensPrompt, &tr.TokensCompletion,
		&tr.LatencyMs, &tr.StartedAt, &tr.EndedAt,
	); err != nil {
		return nil, fmt.Errorf("load trace %s: %w", traceID, err)
	}
	if len(final) > 0 {
		tr.FinalOutput = final
	}
	if len(outcomeDetail) > 0 {
		tr.OutcomeDetail = outcomeDetail
	}
	return &tr, nil
}

// ReplayTrace compares a stored trace against the currently registered
// scenario and tool registry. Returns a ReplayResult whose Pass field
// is true iff every integrity check passes (or the relevant drift was
// explicitly allowed via opts).
func ReplayTrace(trace *TraceRow, lookup ScenarioLookup, opts ReplayOptions) *ReplayResult {
	res := &ReplayResult{
		TraceID:         trace.TraceID,
		ScenarioID:      trace.ScenarioID,
		ScenarioVersion: trace.ScenarioVersion,
	}

	scenario, ok := lookup(trace.ScenarioID)
	if !ok {
		res.Diff = append(res.Diff, DiffEntry{
			Kind:     DiffScenarioMissing,
			Recorded: trace.ScenarioID,
			Detail:   "scenario_id not registered in current process",
		})
		res.Pass = false
		return res
	}

	if scenario.Version != trace.ScenarioVersion {
		entry := DiffEntry{
			Kind:     DiffScenarioVersionChange,
			Field:    "version",
			Recorded: trace.ScenarioVersion,
			Current:  scenario.Version,
		}
		if !opts.AllowVersionDrift {
			res.Diff = append(res.Diff, entry)
		}
	}
	if scenario.ContentHash != trace.ScenarioHash {
		entry := DiffEntry{
			Kind:     DiffScenarioContentChange,
			Field:    "content_hash",
			Recorded: trace.ScenarioHash,
			Current:  scenario.ContentHash,
		}
		if !opts.AllowContentDrift {
			res.Diff = append(res.Diff, entry)
		}
	}

	// Walk recorded tool calls; flag any tool the registry no longer
	// knows about. Hallucinated/rejected calls (whose name was never
	// registered) are excluded so the diff stays signal-rich.
	var calls []ExecutedToolCall
	if len(trace.ToolCalls) > 0 {
		_ = json.Unmarshal(trace.ToolCalls, &calls)
	}
	seen := make(map[string]struct{})
	for _, c := range calls {
		if c.Outcome == OutcomeHallucinatedTool {
			continue
		}
		if _, dup := seen[c.Name]; dup {
			continue
		}
		seen[c.Name] = struct{}{}
		if !Has(c.Name) {
			res.Diff = append(res.Diff, DiffEntry{
				Kind:     DiffToolMissing,
				Field:    "tool_name",
				Recorded: c.Name,
				Detail:   "tool unregistered since trace was recorded",
			})
		}
	}

	res.Pass = len(res.Diff) == 0
	return res
}

// Package render is the shared field-extraction + view-model layer for
// the spec 037 Scope 8 operator UI. Both the CLI subcommands
// (cmd/core/cmd_agent_*.go) and the admin web handlers
// (internal/web/agent_admin*.go) build their output from these views,
// so a single change to required fields propagates to both surfaces.
//
// The layer is intentionally small and pure (no I/O, no template
// rendering, no HTML escaping). CLI prints `Field.Key: Field.Value`.
// Web pipes the same `Field` slice into html/template.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// Severity classifies an outcome for visual treatment.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Field is a single labeled value rendered by both surfaces.
type Field struct {
	Key   string
	Value string
}

// OutcomeView is the canonical structured view of a terminal outcome.
type OutcomeView struct {
	Class    string   // raw outcome string, identical to agent.Outcome
	Label    string   // human-readable banner title
	Severity Severity // info|warning|error
	Summary  string   // short one-line description
	Fields   []Field  // ordered, never-empty list of key/value lines
}

// AllOutcomeClasses returns the canonical ordered list of outcome
// classes that the operator UI renders. Mirrors design §8 plus the two
// auxiliary terminal outcomes the executor produces.
func AllOutcomeClasses() []string {
	return []string{
		string(agent.OutcomeOK),
		string(agent.OutcomeUnknownIntent),
		string(agent.OutcomeAllowlistViolation),
		string(agent.OutcomeHallucinatedTool),
		string(agent.OutcomeToolError),
		string(agent.OutcomeToolReturnInvalid),
		string(agent.OutcomeSchemaFailure),
		string(agent.OutcomeLoopLimit),
		string(agent.OutcomeTimeout),
		string(agent.OutcomeProviderError),
		string(agent.OutcomeInputSchemaViolation),
	}
}

// IsValidOutcomeClass reports whether s is one of the renderable
// outcome classes. Used by the trace-list outcome filter.
func IsValidOutcomeClass(s string) bool {
	for _, c := range AllOutcomeClasses() {
		if c == s {
			return true
		}
	}
	return false
}

// outcomeMeta is the static description of one outcome class.
type outcomeMeta struct {
	label    string
	severity Severity
	summary  string
	required []string // fields that MUST appear in every render
}

// outcomeRegistry is the source of truth for label/severity/required
// fields per outcome class. Keep in lockstep with design §8.
var outcomeRegistry = map[string]outcomeMeta{
	string(agent.OutcomeOK): {
		label: "OK", severity: SeverityInfo,
		summary:  "Final output validated against scenario.output_schema.",
		required: []string{"scenario", "version", "latency_ms", "tool_calls"},
	},
	string(agent.OutcomeUnknownIntent): {
		label: "Unknown Intent", severity: SeverityWarning,
		summary:  "Router could not match the input above the confidence floor (BS-014).",
		required: []string{"scenario", "version", "routing_reason", "top_score", "threshold"},
	},
	string(agent.OutcomeAllowlistViolation): {
		label: "Allowlist Violation", severity: SeverityWarning,
		summary:  "LLM proposed a tool not in scenario.allowed_tools (BS-003 / BS-020).",
		required: []string{"scenario", "version", "rejected_tools", "tool_calls"},
	},
	string(agent.OutcomeHallucinatedTool): {
		label: "Hallucinated Tool", severity: SeverityWarning,
		summary:  "LLM called a tool that is not registered (BS-006).",
		required: []string{"scenario", "version", "rejected_tools", "tool_calls"},
	},
	string(agent.OutcomeToolError): {
		label: "Tool Error", severity: SeverityError,
		summary:  "Tool handler returned an error (BS-015).",
		required: []string{"scenario", "version", "tool", "error"},
	},
	string(agent.OutcomeToolReturnInvalid): {
		label: "Tool Return Invalid", severity: SeverityError,
		summary:  "Tool result failed its declared output schema (BS-005).",
		required: []string{"scenario", "version", "tool", "error", "detail"},
	},
	string(agent.OutcomeSchemaFailure): {
		label: "Schema Failure", severity: SeverityError,
		summary:  "LLM exhausted schema_retry_budget (BS-007).",
		required: []string{"scenario", "version", "attempts", "last_error"},
	},
	string(agent.OutcomeLoopLimit): {
		label: "Loop Limit", severity: SeverityError,
		summary:  "Iteration count exceeded scenario.limits.max_loop_iterations (BS-008).",
		required: []string{"scenario", "version", "max_loop_iterations", "iterations"},
	},
	string(agent.OutcomeTimeout): {
		label: "Timeout", severity: SeverityError,
		summary:  "context.WithTimeout fired before the LLM finalized (BS-021).",
		required: []string{"scenario", "version", "deadline_s", "reason"},
	},
	string(agent.OutcomeProviderError): {
		label: "Provider Error", severity: SeverityError,
		summary:  "LLM driver returned a non-deadline error.",
		required: []string{"scenario", "version", "error", "detail"},
	},
	string(agent.OutcomeInputSchemaViolation): {
		label: "Input Schema Violation", severity: SeverityError,
		summary:  "Input envelope failed scenario.input_schema before the loop started.",
		required: []string{"scenario", "version", "error", "detail"},
	},
}

// RequiredFields exposes the per-outcome required field list to tests
// and to the validation layer in cmd/core.
func RequiredFields(outcome string) []string {
	m, ok := outcomeRegistry[outcome]
	if !ok {
		return nil
	}
	out := make([]string, len(m.required))
	copy(out, m.required)
	return out
}

// TraceSummary is the projection used for the trace-list view. Every
// field maps directly to an indexed agent_traces column so list pages
// can be cheap.
type TraceSummary struct {
	TraceID         string
	ScenarioID      string
	ScenarioVersion string
	Source          string
	Outcome         string
	OutcomeLabel    string
	OutcomeSeverity Severity
	ToolCallCount   int
	LatencyMs       int
	StartedAt       time.Time
}

// TraceDetail is the full inspector view of one persisted trace.
type TraceDetail struct {
	Summary       TraceSummary
	Outcome       OutcomeView
	Routing       RoutingView
	Envelope      EnvelopeView
	ToolCalls     []ToolCallView
	Provider      string
	Model         string
	TokensPrompt  int
	TokensComp    int
	StartedAt     time.Time
	EndedAt       time.Time
	FinalOutput   string // pretty-printed JSON, empty when nil
	OutcomeDetail string // pretty-printed JSON, empty when nil
	TurnLog       string // pretty-printed JSON, empty when nil
}

// EnvelopeView is the rendered form of agent_traces.input_envelope.
type EnvelopeView struct {
	Source             string
	RawInput           string
	ScenarioIDOverride string
	StructuredContext  string // pretty JSON
	Pretty             string // full pretty JSON
}

// RoutingView is the rendered form of agent_traces.routing.
type RoutingView struct {
	Reason     string
	Chosen     string
	TopScore   float64
	Threshold  float64
	Considered []CandidateView
	Pretty     string // full pretty JSON
}

// CandidateView is one entry in the routing.considered list.
type CandidateView struct {
	ScenarioID string
	Score      float64
}

// ToolCallView is one row in the per-trace tool-call table.
type ToolCallView struct {
	Seq             int
	Name            string
	Outcome         string
	OutcomeLabel    string
	OutcomeSeverity Severity
	RejectionReason string
	Error           string
	LatencyMs       int
	ArgsPretty      string
	ResultPretty    string
}

// ScenarioSummary is the projection used for the scenario-catalog view.
type ScenarioSummary struct {
	ID              string
	Version         string
	Description     string
	SideEffectClass string
	AllowedTools    []string
	ContentHash     string
	SourcePath      string
}

// ScenarioDetail is the per-scenario inspector view.
type ScenarioDetail struct {
	Summary         ScenarioSummary
	SystemPrompt    string
	IntentExamples  []string
	Limits          ScenarioLimitsView
	TokenBudget     int
	Temperature     float64
	ModelPreference string
	InputSchema     string // pretty JSON
	OutputSchema    string // pretty JSON
}

// ScenarioLimitsView is the rendered form of scenario.limits.
type ScenarioLimitsView struct {
	MaxLoopIterations int
	TimeoutMs         int
	SchemaRetryBudget int
	PerToolTimeoutMs  int
}

// LoadRejectionView is one rejected scenario file surfaced in the
// catalog. Sourced from agent.LoadError.
type LoadRejectionView struct {
	Path   string
	Reason string
}

// ToolSummary is the projection used for the tool-registry view.
type ToolSummary struct {
	Name             string
	Description      string
	SideEffectClass  string
	SideEffectBadge  string // CSS class fragment ("read"|"write"|"external")
	OwningPackage    string
	PerCallTimeoutMs int
	AllowlistedByIDs []string
}

// ToolDetail is the per-tool inspector view.
type ToolDetail struct {
	Summary      ToolSummary
	InputSchema  string // pretty JSON
	OutputSchema string // pretty JSON
}

// BuildTraceSummary populates the list-row view from a TraceRow.
func BuildTraceSummary(tr *agent.TraceRow) TraceSummary {
	if tr == nil {
		return TraceSummary{}
	}
	count := 0
	if len(tr.ToolCalls) > 0 {
		var calls []agent.ExecutedToolCall
		if err := json.Unmarshal(tr.ToolCalls, &calls); err == nil {
			count = len(calls)
		}
	}
	meta := outcomeRegistry[tr.Outcome]
	return TraceSummary{
		TraceID:         tr.TraceID,
		ScenarioID:      tr.ScenarioID,
		ScenarioVersion: tr.ScenarioVersion,
		Source:          tr.Source,
		Outcome:         tr.Outcome,
		OutcomeLabel:    meta.label,
		OutcomeSeverity: meta.severity,
		ToolCallCount:   count,
		LatencyMs:       tr.LatencyMs,
		StartedAt:       tr.StartedAt,
	}
}

// BuildTraceDetail populates the full inspector view from a TraceRow.
func BuildTraceDetail(tr *agent.TraceRow) TraceDetail {
	if tr == nil {
		return TraceDetail{}
	}
	det := TraceDetail{
		Summary:      BuildTraceSummary(tr),
		Outcome:      buildOutcomeView(tr),
		Routing:      buildRoutingView(tr.Routing),
		Envelope:     buildEnvelopeView(tr.InputEnvelope),
		Provider:     tr.Provider,
		Model:        tr.Model,
		TokensPrompt: tr.TokensPrompt,
		TokensComp:   tr.TokensCompletion,
		StartedAt:    tr.StartedAt,
		EndedAt:      tr.EndedAt,
	}
	if len(tr.FinalOutput) > 0 {
		det.FinalOutput = prettyJSON(tr.FinalOutput)
	}
	if len(tr.OutcomeDetail) > 0 {
		det.OutcomeDetail = prettyJSON(tr.OutcomeDetail)
	}
	if len(tr.TurnLog) > 0 && string(tr.TurnLog) != "[]" {
		det.TurnLog = prettyJSON(tr.TurnLog)
	}
	if len(tr.ToolCalls) > 0 {
		var calls []agent.ExecutedToolCall
		if err := json.Unmarshal(tr.ToolCalls, &calls); err == nil {
			for _, c := range calls {
				det.ToolCalls = append(det.ToolCalls, buildToolCallView(c))
			}
		}
	}
	return det
}

func buildToolCallView(c agent.ExecutedToolCall) ToolCallView {
	meta := outcomeRegistry[string(c.Outcome)]
	v := ToolCallView{
		Seq:             c.Seq,
		Name:            c.Name,
		Outcome:         string(c.Outcome),
		OutcomeLabel:    meta.label,
		OutcomeSeverity: meta.severity,
		RejectionReason: c.RejectionReason,
		Error:           c.Error,
		LatencyMs:       c.LatencyMs,
	}
	if v.OutcomeLabel == "" {
		v.OutcomeLabel = string(c.Outcome)
	}
	if len(c.Arguments) > 0 {
		v.ArgsPretty = prettyJSON(c.Arguments)
	}
	if len(c.Result) > 0 {
		v.ResultPretty = prettyJSON(c.Result)
	}
	return v
}

func buildEnvelopeView(raw json.RawMessage) EnvelopeView {
	v := EnvelopeView{Pretty: prettyJSON(raw)}
	if len(raw) == 0 {
		return v
	}
	var env agent.IntentEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return v
	}
	v.Source = env.Source
	v.RawInput = env.RawInput
	v.ScenarioIDOverride = env.ScenarioID
	if len(env.StructuredContext) > 0 {
		v.StructuredContext = prettyJSON(env.StructuredContext)
	}
	return v
}

func buildRoutingView(raw json.RawMessage) RoutingView {
	v := RoutingView{Pretty: prettyJSON(raw)}
	if len(raw) == 0 {
		return v
	}
	var dec agent.RoutingDecision
	if err := json.Unmarshal(raw, &dec); err != nil {
		return v
	}
	v.Reason = string(dec.Reason)
	v.Chosen = dec.Chosen
	v.TopScore = dec.TopScore
	v.Threshold = dec.Threshold
	for _, c := range dec.Considered {
		v.Considered = append(v.Considered, CandidateView{ScenarioID: c.ScenarioID, Score: c.Score})
	}
	return v
}

// buildOutcomeView assembles the OutcomeView for one TraceRow. Required
// fields are guaranteed to be present in the returned view; missing
// detail keys render as "(unset)" so the operator sees the gap rather
// than silently missing a row.
func buildOutcomeView(tr *agent.TraceRow) OutcomeView {
	meta, ok := outcomeRegistry[tr.Outcome]
	if !ok {
		return OutcomeView{
			Class:    tr.Outcome,
			Label:    tr.Outcome,
			Severity: SeverityWarning,
			Summary:  "Unknown outcome class",
			Fields: []Field{
				{Key: "scenario", Value: tr.ScenarioID},
				{Key: "version", Value: tr.ScenarioVersion},
			},
		}
	}

	detail := map[string]any{}
	if len(tr.OutcomeDetail) > 0 {
		_ = json.Unmarshal(tr.OutcomeDetail, &detail)
	}
	routing := map[string]any{}
	if len(tr.Routing) > 0 {
		_ = json.Unmarshal(tr.Routing, &routing)
	}
	var calls []agent.ExecutedToolCall
	if len(tr.ToolCalls) > 0 {
		_ = json.Unmarshal(tr.ToolCalls, &calls)
	}

	view := OutcomeView{
		Class:    tr.Outcome,
		Label:    meta.label,
		Severity: meta.severity,
		Summary:  meta.summary,
	}
	for _, key := range meta.required {
		view.Fields = append(view.Fields, Field{
			Key:   key,
			Value: extractField(key, tr, detail, routing, calls),
		})
	}
	return view
}

// extractField resolves one logical field from the available sources.
// Keep this table aligned with outcomeRegistry.required keys.
func extractField(key string, tr *agent.TraceRow, detail, routing map[string]any, calls []agent.ExecutedToolCall) string {
	switch key {
	case "scenario":
		return defaultUnset(tr.ScenarioID)
	case "version":
		return defaultUnset(tr.ScenarioVersion)
	case "latency_ms":
		return fmt.Sprintf("%d", tr.LatencyMs)
	case "tool_calls":
		return fmt.Sprintf("%d", len(calls))
	case "routing_reason":
		return defaultUnset(stringFromAny(routing["reason"]))
	case "top_score":
		return fmt.Sprintf("%.3f", floatFromAny(routing["top_score"]))
	case "threshold":
		return fmt.Sprintf("%.3f", floatFromAny(routing["threshold"]))
	case "rejected_tools":
		var names []string
		for _, c := range calls {
			if string(c.Outcome) == tr.Outcome {
				names = append(names, c.Name)
			}
		}
		if len(names) == 0 {
			return "(none recorded)"
		}
		return strings.Join(names, ", ")
	case "tool":
		return defaultUnset(stringFromAny(detail["tool"]))
	case "error":
		return defaultUnset(stringFromAny(detail["error"]))
	case "detail":
		return defaultUnset(stringFromAny(detail["detail"]))
	case "attempts":
		return fmt.Sprintf("%d", intFromAny(detail["attempts"]))
	case "last_error":
		return defaultUnset(stringFromAny(detail["last_error"]))
	case "max_loop_iterations":
		return fmt.Sprintf("%d", intFromAny(detail["max_loop_iterations"]))
	case "iterations":
		// Persisted indirectly via tool_calls length is wrong; the
		// trace row carries no Iterations column. Use latency vs
		// reason as a fallback so the field is always populated.
		if v, ok := detail["iterations"]; ok {
			return fmt.Sprintf("%d", intFromAny(v))
		}
		// Loop-limit detail records the cap; iterations equals it.
		if v, ok := detail["max_loop_iterations"]; ok {
			return fmt.Sprintf("%d", intFromAny(v))
		}
		return "(unset)"
	case "deadline_s":
		return fmt.Sprintf("%d", intFromAny(detail["deadline_s"]))
	case "reason":
		return defaultUnset(stringFromAny(detail["reason"]))
	}
	return "(unset)"
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprintf("%v", x)
	}
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	}
	return 0
}

func floatFromAny(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func defaultUnset(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

// BuildScenarioSummary populates the catalog list view.
func BuildScenarioSummary(s *agent.Scenario) ScenarioSummary {
	if s == nil {
		return ScenarioSummary{}
	}
	tools := make([]string, 0, len(s.AllowedTools))
	for _, at := range s.AllowedTools {
		tools = append(tools, at.Name)
	}
	sort.Strings(tools)
	return ScenarioSummary{
		ID:              s.ID,
		Version:         s.Version,
		Description:     s.Description,
		SideEffectClass: string(s.SideEffectClass),
		AllowedTools:    tools,
		ContentHash:     s.ContentHash,
		SourcePath:      s.SourcePath,
	}
}

// BuildScenarioDetail populates the per-scenario view.
func BuildScenarioDetail(s *agent.Scenario) ScenarioDetail {
	if s == nil {
		return ScenarioDetail{}
	}
	d := ScenarioDetail{
		Summary:         BuildScenarioSummary(s),
		SystemPrompt:    s.SystemPrompt,
		IntentExamples:  append([]string(nil), s.IntentExamples...),
		TokenBudget:     s.TokenBudget,
		Temperature:     s.Temperature,
		ModelPreference: s.ModelPreference,
		Limits: ScenarioLimitsView{
			MaxLoopIterations: s.Limits.MaxLoopIterations,
			TimeoutMs:         s.Limits.TimeoutMs,
			SchemaRetryBudget: s.Limits.SchemaRetryBudget,
			PerToolTimeoutMs:  s.Limits.PerToolTimeoutMs,
		},
		InputSchema:  prettyJSON(s.InputSchema),
		OutputSchema: prettyJSON(s.OutputSchema),
	}
	return d
}

// BuildLoadRejection adapts an agent.LoadError to a view row.
func BuildLoadRejection(e agent.LoadError) LoadRejectionView {
	return LoadRejectionView{Path: e.Path, Reason: e.Message}
}

// BuildToolSummary populates the registry list view. allowlistedBy is
// passed in (computed by the caller from the loaded scenario set) so
// the render layer stays free of registry walks.
func BuildToolSummary(t agent.Tool, allowlistedBy []string) ToolSummary {
	sortedBy := append([]string(nil), allowlistedBy...)
	sort.Strings(sortedBy)
	return ToolSummary{
		Name:             t.Name,
		Description:      t.Description,
		SideEffectClass:  string(t.SideEffectClass),
		SideEffectBadge:  string(t.SideEffectClass),
		OwningPackage:    t.OwningPackage,
		PerCallTimeoutMs: t.PerCallTimeoutMs,
		AllowlistedByIDs: sortedBy,
	}
}

// BuildToolDetail populates the per-tool inspector view.
func BuildToolDetail(t agent.Tool, allowlistedBy []string) ToolDetail {
	return ToolDetail{
		Summary:      BuildToolSummary(t, allowlistedBy),
		InputSchema:  prettyJSON(t.InputSchema),
		OutputSchema: prettyJSON(t.OutputSchema),
	}
}

// AllowlistedBy returns the scenario ids that allowlist tool name. Pure
// helper kept here so the caller does not need to reimplement it for
// CLI vs web.
func AllowlistedBy(name string, scenarios []*agent.Scenario) []string {
	var out []string
	for _, s := range scenarios {
		if s == nil {
			continue
		}
		for _, at := range s.AllowedTools {
			if at.Name == name {
				out = append(out, s.ID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

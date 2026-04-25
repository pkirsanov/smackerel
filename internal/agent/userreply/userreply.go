// Package userreply maps a structured agent.InvocationResult to the
// surface-specific user reply (Telegram message text, REST JSON envelope)
// per spec 037 §UX and design §8.
//
// Why this package exists:
//
//  1. BS-014 — never invent. The bot/API MUST NOT free-form a response
//     when the agent fails. Every outcome class has a fixed-structure
//     reply with a trace ref so the operator can investigate. All copy
//     lives here and is unit-tested. No surface is allowed to assemble
//     reply text on its own.
//  2. BS-020 — user-facing copy is reviewable. Centralising the strings
//     means a single diff shows wording changes; scope 9's review surface
//     is this package.
//  3. BS-021 — bounded surfaces. The Telegram limit ≤4 lines and the
//     trace-ref-always rule are enforced by tests, not by the surfaces.
//
// The package is intentionally pure: in → (*agent.InvocationResult,
// agent.RoutingDecision, []string knownIntents) and out → strings or
// JSON-shaped maps. No I/O, no logging, no allocations of executor
// state. Surfaces wire the pieces together.
package userreply

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/agent"
)

// MaxTelegramLines is the hard cap from spec.md §UX (≤4 lines per
// failure reply). Tests in this package and in tests/e2e/agent assert
// every reply respects it.
const MaxTelegramLines = 4

// TraceRefPrefix is the literal prefix used for the human-readable trace
// reference appended to every Telegram reply. Operators copy/paste the
// id portion into the trace UI search box (spec §UX).
const TraceRefPrefix = "trace: "

// HTTPStatus is the HTTP status code chosen for an outcome by the API
// surface. Spec §UX:
//   - 200 for any in-spec outcome (including handled failures).
//   - 4xx for malformed request envelopes (input-schema-violation).
//   - 5xx is reserved for infrastructure failures BEFORE the agent runs
//     (trace-store-unreachable, provider-unreachable). Those are
//     produced by the surface, not by this package.
type HTTPStatus int

// APIResponse is the (status, body) pair the API surface emits for an
// outcome. Body is shaped per spec §UX "End-User Failure Surface — API"
// and varies by outcome class.
type APIResponse struct {
	Status HTTPStatus
	Body   map[string]any
}

// TelegramReply is the plain-text reply the Telegram surface sends to
// the chat. Lines is always 1..MaxTelegramLines for in-spec outcomes.
// The last line is always the trace ref.
type TelegramReply struct {
	Text string
}

// Lines returns the reply text split on "\n". Tests use this to assert
// the ≤4-line cap.
func (r TelegramReply) Lines() []string {
	if r.Text == "" {
		return nil
	}
	return strings.Split(r.Text, "\n")
}

// HasTraceRef reports whether the reply ends with the trace-ref line.
// Tests assert this for every outcome class.
func (r TelegramReply) HasTraceRef() bool {
	if r.Text == "" {
		return false
	}
	lines := r.Lines()
	last := lines[len(lines)-1]
	return strings.Contains(last, TraceRefPrefix)
}

// UnknownIntentMarker is the structural sentinel BS-014's regression
// test asserts on. Wording around it may evolve, but this exact phrase
// MUST appear in every unknown-intent reply so the regression cannot
// be removed by paraphrasing.
const UnknownIntentMarker = "I don't know how to handle that yet"

// TimeoutMarker is the sentinel for BS-021's regression. The deadline
// value is appended after it.
const TimeoutMarker = "That took longer than I'm allowed to wait"

// SchemaFailureMarker is the sentinel for BS-007's surface text.
const SchemaFailureMarker = "I worked out the answer but couldn't format it cleanly"

// LoopLimitMarker is the sentinel for BS-008's surface text.
const LoopLimitMarker = "I tried"

// AllowlistMarker is the sentinel for BS-003/BS-020's surface text.
const AllowlistMarker = "blocked a write action that wasn't allowed"

// ToolErrorMarker is the sentinel for BS-015's surface text.
const ToolErrorMarker = "I couldn't reach the data I needed"

// HallucinatedToolMarker is the sentinel for BS-006's surface text.
const HallucinatedToolMarker = "I tried to use a tool that doesn't exist"

// ToolReturnInvalidMarker is the sentinel for BS-005's surface text.
const ToolReturnInvalidMarker = "tool returned an unexpected shape"

// ProviderErrorMarker is the sentinel for the provider-error surface.
const ProviderErrorMarker = "I couldn't reach the language model"

// InputSchemaViolationMarker is the sentinel for input-schema-violation
// (telegram surface only — the API returns 4xx).
const InputSchemaViolationMarker = "I couldn't parse that request"

// Inputs is the bundle the surface assembles before asking userreply
// to render. KnownIntents is the list the unknown-intent reply must
// list verbatim (BS-014 regression asserts this set exactly equals the
// router's registered scenarios).
type Inputs struct {
	Result       *agent.InvocationResult
	KnownIntents []string
	// Routing is non-nil when the surface routed the request itself
	// (telegram/api built an envelope and called router.Route). It
	// carries the candidate scores used in the unknown-intent API
	// envelope.
	Routing *agent.RoutingDecision
}

// validateInputs returns a programmer-error when the inputs are
// structurally wrong. Surfaces must not pass nil results; this is a
// fail-loud guard, not a soft default.
func (in Inputs) validate() error {
	if in.Result == nil {
		return fmt.Errorf("userreply: nil InvocationResult")
	}
	return nil
}

// RenderTelegram returns the chat reply text for a given invocation
// result. Falls back to the input-schema-violation reply when the
// outcome is unrecognised — surfaces should never reach that branch
// because the executor's outcome enum is closed.
func RenderTelegram(in Inputs) TelegramReply {
	if err := in.validate(); err != nil {
		// Programmer-error path: render a structured reply rather than
		// panic so the surface can still ship something to the user.
		return TelegramReply{Text: "Internal error: " + err.Error()}
	}
	r := in.Result
	traceLine := traceRefLine(r.TraceID)

	switch r.Outcome {
	case agent.OutcomeOK:
		return TelegramReply{Text: renderOKTelegram(r, traceLine)}

	case agent.OutcomeUnknownIntent:
		return TelegramReply{Text: renderUnknownIntentTelegram(in.KnownIntents, traceLine)}

	case agent.OutcomeAllowlistViolation:
		blocked := pickBlockedToolName(r)
		return TelegramReply{Text: joinLines(
			AllowlistMarker+" ("+blocked+").",
			"I answered only the part I was allowed to.",
			traceLine,
		)}

	case agent.OutcomeHallucinatedTool:
		return TelegramReply{Text: joinLines(
			HallucinatedToolMarker+"; I stopped here.",
			traceLine,
		)}

	case agent.OutcomeToolError:
		tool := pickFailedToolName(r)
		return TelegramReply{Text: joinLines(
			ToolErrorMarker+" ("+tool+").",
			"I tried more than once, then gave up. You can resend in a minute.",
			traceLine,
		)}

	case agent.OutcomeToolReturnInvalid:
		tool := pickFailedToolName(r)
		return TelegramReply{Text: joinLines(
			"A "+tool+" "+ToolReturnInvalidMarker+"; I stopped to avoid a bad write.",
			traceLine,
		)}

	case agent.OutcomeSchemaFailure:
		return TelegramReply{Text: joinLines(
			SchemaFailureMarker+".",
			"Try rephrasing — short, one question at a time helps.",
			traceLine,
		)}

	case agent.OutcomeLoopLimit:
		k := r.Iterations
		return TelegramReply{Text: joinLines(
			fmt.Sprintf("%s %d things and stopped.", LoopLimitMarker, k),
			traceLine,
		)}

	case agent.OutcomeTimeout:
		seconds := timeoutDeadlineSeconds(r)
		var deadline string
		if seconds > 0 {
			deadline = fmt.Sprintf(" (%ds)", seconds)
		}
		return TelegramReply{Text: joinLines(
			TimeoutMarker+deadline+".",
			"Try a smaller window (e.g. one month).",
			traceLine,
		)}

	case agent.OutcomeProviderError:
		return TelegramReply{Text: joinLines(
			ProviderErrorMarker+". Please try again.",
			traceLine,
		)}

	case agent.OutcomeInputSchemaViolation:
		return TelegramReply{Text: joinLines(
			InputSchemaViolationMarker+".",
			traceLine,
		)}
	}

	// Unrecognised outcome — never expected. Emit a structured failure
	// rather than free-form text so BS-014 stays honoured.
	return TelegramReply{Text: joinLines(
		"I hit an unexpected internal state ("+string(r.Outcome)+").",
		traceLine,
	)}
}

// RenderAPI returns the (status, JSON body) pair for a given invocation
// result. Body keys mirror spec §UX exactly; surfaces JSON-encode the
// map directly without further transformation.
func RenderAPI(in Inputs) APIResponse {
	if err := in.validate(); err != nil {
		// Programmer-error path: a 500 is the only honest answer. Other
		// real 5xx cases (trace store unreachable, etc.) are produced
		// by the surface BEFORE calling RenderAPI.
		return APIResponse{
			Status: 500,
			Body: map[string]any{
				"error":  "internal_error",
				"detail": err.Error(),
			},
		}
	}
	r := in.Result

	switch r.Outcome {
	case agent.OutcomeOK:
		body := map[string]any{
			"outcome":  string(agent.OutcomeOK),
			"scenario": r.ScenarioID,
			"version":  r.ScenarioVersion,
			"trace_id": r.TraceID,
			"result":   rawOrNil(r.Final),
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeUnknownIntent:
		body := map[string]any{
			"outcome":    string(agent.OutcomeUnknownIntent),
			"trace_id":   r.TraceID,
			"candidates": candidatesFromRouting(in.Routing),
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeAllowlistViolation:
		body := map[string]any{
			"outcome":  string(agent.OutcomeAllowlistViolation),
			"scenario": r.ScenarioID,
			"version":  r.ScenarioVersion,
			"trace_id": r.TraceID,
			"blocked":  blockedListFromCalls(r),
		}
		if r.Final != nil {
			body["result"] = rawOrNil(r.Final)
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeHallucinatedTool:
		body := map[string]any{
			"outcome":  string(agent.OutcomeHallucinatedTool),
			"scenario": r.ScenarioID,
			"version":  r.ScenarioVersion,
			"trace_id": r.TraceID,
			"tool":     pickFailedToolName(r),
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeToolError:
		body := map[string]any{
			"outcome":  string(agent.OutcomeToolError),
			"scenario": r.ScenarioID,
			"version":  r.ScenarioVersion,
			"trace_id": r.TraceID,
			"tool":     pickFailedToolName(r),
			"message":  pickFailedToolMessage(r),
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeToolReturnInvalid:
		body := map[string]any{
			"outcome":  string(agent.OutcomeToolReturnInvalid),
			"scenario": r.ScenarioID,
			"version":  r.ScenarioVersion,
			"trace_id": r.TraceID,
			"tool":     pickFailedToolName(r),
			"details":  "return value did not match declared schema",
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeSchemaFailure:
		body := map[string]any{
			"outcome":    string(agent.OutcomeSchemaFailure),
			"scenario":   r.ScenarioID,
			"version":    r.ScenarioVersion,
			"trace_id":   r.TraceID,
			"attempts":   r.SchemaRetries,
			"last_error": detailString(r, "error"),
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeLoopLimit:
		body := map[string]any{
			"outcome":  string(agent.OutcomeLoopLimit),
			"scenario": r.ScenarioID,
			"version":  r.ScenarioVersion,
			"trace_id": r.TraceID,
			"calls":    r.Iterations,
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeTimeout:
		body := map[string]any{
			"outcome":    string(agent.OutcomeTimeout),
			"scenario":   r.ScenarioID,
			"version":    r.ScenarioVersion,
			"trace_id":   r.TraceID,
			"deadline_s": timeoutDeadlineSeconds(r),
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeProviderError:
		body := map[string]any{
			"outcome":  string(agent.OutcomeProviderError),
			"scenario": r.ScenarioID,
			"version":  r.ScenarioVersion,
			"trace_id": r.TraceID,
			"message":  detailString(r, "error"),
		}
		return APIResponse{Status: 200, Body: body}

	case agent.OutcomeInputSchemaViolation:
		// 4xx per spec §UX. Body is the documented "NOT an agent
		// outcome" envelope. trace_id is included when the agent
		// actually started; surfaces may pass it.
		body := map[string]any{
			"error":    "input_schema_violation",
			"trace_id": r.TraceID,
			"detail":   detailString(r, "error"),
		}
		return APIResponse{Status: 400, Body: body}
	}

	// Closed enum — but if a future outcome is added without updating
	// this map, fail loud rather than silently dropping it. 500 is
	// honest: the surface didn't know how to render it.
	return APIResponse{
		Status: 500,
		Body: map[string]any{
			"error":    "unknown_outcome",
			"trace_id": r.TraceID,
			"outcome":  string(r.Outcome),
		},
	}
}

// MalformedRequestResponse is the API surface's 400 reply when the
// request envelope itself is malformed (missing required fields, bad
// JSON). The agent did not run; there is no trace_id.
func MalformedRequestResponse(field string) APIResponse {
	return APIResponse{
		Status: 400,
		Body: map[string]any{
			"error": "missing_field",
			"field": field,
		},
	}
}

// InfrastructureFailureResponse is the API surface's 5xx reply when the
// agent could not start (e.g. trace store unreachable). The agent did
// not run; there is no trace_id.
func InfrastructureFailureResponse(reason string) APIResponse {
	return APIResponse{
		Status: 503,
		Body: map[string]any{
			"error": reason,
		},
	}
}

// --- helpers (private) ----------------------------------------------------

// renderOKTelegram emits the success reply. We keep it ≤4 lines by
// projecting the JSON final to a single "answer" line when present,
// otherwise rendering the raw JSON.
func renderOKTelegram(r *agent.InvocationResult, traceLine string) string {
	answer := pickAnswerString(r.Final)
	if answer == "" {
		return joinLines("(empty answer)", traceLine)
	}
	// Truncate at MaxTelegramLines-1 lines (last is trace ref).
	answerLines := strings.Split(answer, "\n")
	if len(answerLines) > MaxTelegramLines-1 {
		answerLines = answerLines[:MaxTelegramLines-1]
	}
	return joinLines(append(answerLines, traceLine)...)
}

// renderUnknownIntentTelegram emits the BS-014 reply. The marker
// phrase MUST appear; the known-intents list is appended literally
// (no LLM-generated text).
func renderUnknownIntentTelegram(known []string, traceLine string) string {
	intents := "(none configured)"
	if len(known) > 0 {
		intents = strings.Join(known, ", ")
	}
	return joinLines(
		UnknownIntentMarker+".",
		"I can help with: "+intents+".",
		"Try rephrasing as one question.",
		traceLine,
	)
}

// pickAnswerString extracts a reasonable single-string from a JSON
// final output. The output schema always validates first, but the
// shape varies per scenario, so we look for common keys before falling
// back to the raw JSON.
func pickAnswerString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try common shapes first.
	var asObj map[string]any
	if err := json.Unmarshal(raw, &asObj); err == nil {
		for _, k := range []string{"answer", "message", "summary", "text"} {
			if v, ok := asObj[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	// Try a bare string.
	var asStr string
	if err := json.Unmarshal(raw, &asStr); err == nil && asStr != "" {
		return asStr
	}
	// Fall through: render compact JSON.
	return string(raw)
}

// pickBlockedToolName scans tool calls for the first allowlist
// rejection so the user reply can name it. Returns "(unknown)" when
// no per-call record is available.
func pickBlockedToolName(r *agent.InvocationResult) string {
	for _, c := range r.ToolCalls {
		if c.Outcome == agent.OutcomeAllowlistViolation {
			if c.Name != "" {
				return c.Name
			}
		}
	}
	return "(unknown)"
}

// pickFailedToolName returns the first tool that errored or whose
// return was invalid; falls back to outcome detail; then "(unknown)".
func pickFailedToolName(r *agent.InvocationResult) string {
	for _, c := range r.ToolCalls {
		switch c.Outcome {
		case agent.OutcomeToolError, agent.OutcomeToolReturnInvalid, agent.OutcomeHallucinatedTool:
			if c.Name != "" {
				return c.Name
			}
		}
	}
	if v, ok := r.OutcomeDetail["tool"].(string); ok && v != "" {
		return v
	}
	return "(unknown)"
}

// pickFailedToolMessage extracts the per-tool error message for the
// API tool-error envelope.
func pickFailedToolMessage(r *agent.InvocationResult) string {
	for _, c := range r.ToolCalls {
		if c.Outcome == agent.OutcomeToolError && c.Error != "" {
			return c.Error
		}
	}
	if s := detailString(r, "error"); s != "" {
		return s
	}
	return "tool_error"
}

// timeoutDeadlineSeconds reads the configured deadline from the
// outcome detail (executor records it as "deadline_s" or
// "timeout_ms"). Returns 0 when neither is present so the caller can
// omit the parenthetical.
func timeoutDeadlineSeconds(r *agent.InvocationResult) int {
	if r.OutcomeDetail == nil {
		return 0
	}
	if v, ok := r.OutcomeDetail["deadline_s"]; ok {
		switch x := v.(type) {
		case int:
			return x
		case int64:
			return int(x)
		case float64:
			return int(x)
		}
	}
	if v, ok := r.OutcomeDetail["timeout_ms"]; ok {
		switch x := v.(type) {
		case int:
			return x / 1000
		case int64:
			return int(x) / 1000
		case float64:
			return int(x) / 1000
		}
	}
	return 0
}

// detailString fetches a string field from OutcomeDetail; "" when
// missing. Used by error/message renderers.
func detailString(r *agent.InvocationResult, key string) string {
	if r.OutcomeDetail == nil {
		return ""
	}
	if v, ok := r.OutcomeDetail[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// candidatesFromRouting projects the router's considered list into
// the spec §UX shape. Empty list when no routing was performed.
func candidatesFromRouting(d *agent.RoutingDecision) []map[string]any {
	if d == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(d.Considered))
	for _, c := range d.Considered {
		out = append(out, map[string]any{
			"scenario": c.ScenarioID,
			"score":    c.Score,
		})
	}
	return out
}

// blockedListFromCalls projects the per-call allowlist rejections into
// the spec §UX "blocked" array.
func blockedListFromCalls(r *agent.InvocationResult) []map[string]any {
	out := []map[string]any{}
	for _, c := range r.ToolCalls {
		if c.Outcome == agent.OutcomeAllowlistViolation {
			out = append(out, map[string]any{
				"tool":   c.Name,
				"reason": c.RejectionReason,
			})
		}
	}
	return out
}

// rawOrNil returns the raw JSON unmarshalled to any so the surrounding
// map[string]any encodes it correctly (avoids double-encoding).
func rawOrNil(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Last-ditch: return as string so the response still ships.
		return string(raw)
	}
	return v
}

// traceRefLine renders the standard trace-ref footer. Empty trace ids
// produce a stable placeholder so tests can still assert presence.
func traceRefLine(traceID string) string {
	if traceID == "" {
		traceID = "(none)"
	}
	return "(" + TraceRefPrefix + traceID + ")"
}

// joinLines joins non-empty lines with "\n". Empty lines are dropped
// to keep the ≤4-line cap honest even when callers pass in optional
// segments.
func joinLines(lines ...string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

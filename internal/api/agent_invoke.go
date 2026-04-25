// Spec 037 Scope 9 — POST /v1/agent/invoke handler.
//
// The endpoint accepts an IntentEnvelope, routes it through the
// configured agent.Router, executes via agent.Executor, and emits the
// structured envelope documented in spec.md §UX
// "End-User Failure Surface — API".
//
// HTTP semantics (spec §UX):
//
//   - 200 — for ANY in-spec outcome, including handled adversarial
//     ones (unknown-intent, schema-failure, tool-error, ...).
//   - 4xx — only for malformed REQUEST envelopes (missing raw_input
//     or scenario_id, malformed JSON, input-schema-violation against
//     the chosen scenario's input_schema).
//   - 5xx — only when the agent could not start (trace store
//     unreachable, router unconfigured). The agent itself never
//     returns 5xx for adversarial outcomes; those are handled.
//
// All bodies — including 5xx — carry a stable JSON shape so callers
// can branch on a single field. Trace ids are emitted whenever the
// agent actually ran.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/userreply"
)

// AgentInvokeRunner is the dependency the handler asks for. The
// production wiring (cmd/core) provides one backed by the real router
// and executor; tests inject scripted runners that return canned
// outcomes.
//
// Why not pass *agent.Executor + agent.Router directly?
//  1. Surfaces should not know the order of route → execute; the
//     bridge enforces it once and applies the same policy everywhere
//     (telegram + api + future scheduler/pipeline).
//  2. It lets tests substitute behaviour without wrapping every
//     downstream type.
type AgentInvokeRunner interface {
	// Invoke routes env, runs the executor, and returns the result.
	// The returned RoutingDecision is non-nil EXCEPT when the runner
	// short-circuited before routing (e.g. nil scenario id with no
	// router). When the router selects no scenario, decision.Reason
	// is ReasonUnknownIntent and the result Outcome is
	// OutcomeUnknownIntent.
	Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision)
	// KnownIntents lists scenario ids the bridge can route to. Used
	// by the API surface to populate unknown-intent candidate context
	// when the router did not produce one (defence in depth).
	KnownIntents() []string
}

// AgentInvokeHandler holds the wired runner. Construct via the wiring
// in cmd/core; the api router checks for nil before mounting the route.
type AgentInvokeHandler struct {
	Runner AgentInvokeRunner
}

// AgentInvokeRequest is the on-wire request envelope. Fields mirror
// agent.IntentEnvelope so callers can construct them directly.
//
// JSON keys deliberately match the snake_case used elsewhere in the
// API. structured_context is opaque (any JSON value) to give callers
// scenario-specific freedom; the executor validates it against the
// chosen scenario's input_schema.
type AgentInvokeRequest struct {
	RawInput          string          `json:"raw_input"`
	StructuredContext json.RawMessage `json:"structured_context,omitempty"`
	ScenarioID        string          `json:"scenario_id,omitempty"`
	Source            string          `json:"source,omitempty"`
	ConfidenceFloor   float64         `json:"confidence_floor,omitempty"`
}

// AgentInvokeHandlerFunc is the http.HandlerFunc closure registered on
// the chi router by NewRouter. Returns 503 when no runner is wired
// (defence in depth — the router gates with a nil check first).
func (h *AgentInvokeHandler) AgentInvokeHandlerFunc(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Runner == nil {
		writeAgentResponse(w, userreply.InfrastructureFailureResponse("agent_runner_not_configured"))
		return
	}

	// Cap request bodies at 64 KiB. Scenarios with larger structured
	// inputs should call internal services directly; the public agent
	// surface is intent-driven and short by design.
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		slog.Warn("agent_invoke: read body failed", "error", err)
		writeAgentResponse(w, userreply.MalformedRequestResponse("body_read_error"))
		return
	}
	if len(body) == 0 {
		writeAgentResponse(w, userreply.MalformedRequestResponse("body"))
		return
	}

	var req AgentInvokeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAgentResponse(w, userreply.MalformedRequestResponse("body_invalid_json"))
		return
	}

	// raw_input OR scenario_id is required — otherwise there is
	// nothing to route or override on. Spec §UX names raw_input as the
	// canonical missing field.
	if req.RawInput == "" && req.ScenarioID == "" {
		writeAgentResponse(w, userreply.MalformedRequestResponse("raw_input"))
		return
	}

	source := req.Source
	if source == "" {
		source = "api"
	}

	env := agent.IntentEnvelope{
		Source:            source,
		RawInput:          req.RawInput,
		StructuredContext: req.StructuredContext,
		ScenarioID:        req.ScenarioID,
		ConfidenceFloor:   req.ConfidenceFloor,
	}

	result, decision := h.Runner.Invoke(r.Context(), env)
	if result == nil {
		// Runner short-circuited before producing any outcome — treat
		// as infrastructure failure. The runner will already have
		// logged the cause.
		writeAgentResponse(w, userreply.InfrastructureFailureResponse("agent_invoke_failed"))
		return
	}

	resp := userreply.RenderAPI(userreply.Inputs{
		Result:       result,
		Routing:      decision,
		KnownIntents: h.Runner.KnownIntents(),
	})
	writeAgentResponse(w, resp)
}

// writeAgentResponse writes a userreply.APIResponse to the wire.
// Errors during encoding are logged but cannot be returned to the
// caller (headers are already flushed).
func writeAgentResponse(w http.ResponseWriter, resp userreply.APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(int(resp.Status))
	if err := json.NewEncoder(w).Encode(resp.Body); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		slog.Warn("agent_invoke: encode response failed", "error", err)
	}
}

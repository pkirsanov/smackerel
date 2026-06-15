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
	"strings"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/userreply"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
	"github.com/smackerel/smackerel/internal/auth"
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
	// Model is the spec 088 OPTIONAL per-request open-knowledge model
	// override. UNTRUSTED: validated against the switchable-model allowlist
	// in the open_knowledge fast-path BEFORE any agent/Ollama call; an empty
	// value is the baseline (no override). Off-allowlist / over-envelope
	// values yield an HTTP 400 rejection envelope (never a silent default).
	Model string `json:"model,omitempty"`
	// GatherModel is the spec 089 (Fork C) OPTIONAL per-request open-knowledge
	// GATHER (tool-calling) model override — SEPARATE from Model (synthesis).
	// UNTRUSTED: validated against the tool_capable_gather_models set BEFORE
	// any gather turn runs; a non-tool-capable value yields an HTTP 400
	// rejection (error_code model_not_tool_capable, rejected_turn gather).
	// Empty is the baseline (no gather override).
	GatherModel string `json:"gather_model,omitempty"`
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

	// Spec 064 SCOPE-17 fast-path — when the caller explicitly targets the
	// open_knowledge scenario, bypass the spec 037 substrate planner and
	// invoke the open-knowledge agent loop directly. Rationale: the
	// substrate planner makes its own LLM round-trip to pick a tool, which
	// on CPU dev machines plus the inherent latency of the agent loop
	// blows past the substrate scenario timeout ceiling. The substrate
	// already routes below-floor queries here via fallback_scenario_id, so
	// this short-circuit only helps the explicit-scenario path; implicit
	// fallback routing still flows through the substrate for traceability.
	if req.ScenarioID == "open_knowledge" && agenttool.CurrentAgent() != nil {
		prompt := strings.TrimSpace(req.RawInput)
		if prompt == "" {
			writeAgentResponse(w, userreply.MalformedRequestResponse("raw_input"))
			return
		}
		// Spec 088/089 — resolve the model selection BEFORE any agent/Ollama
		// call. Precedence (spec 089): per-request (req.Model / req.GatherModel)
		// > the caller's claim-bound sticky preference > the SST default. The
		// route is behind bearerAuthMiddleware (CT-2), so auth.UserIDFromContext
		// is the PASETO subject — the sticky read is claim-bound for free and a
		// request-body user id can never reach the key. An off-allowlist
		// synthesis or a non-tool-capable gather is rejected fail-loud with HTTP
		// 400 (no silent default, no backend passthrough). A nil allowlist /
		// nil store yields baseline passthrough, never a panic.
		var ov modelswitch.Override
		var eff modelswitch.Effective
		haveSelection := false
		if allow := agenttool.SwitchableModels(); allow != nil {
			stickySynth := ""
			if subject := auth.UserIDFromContext(r.Context()); subject != "" {
				if ps := agenttool.ModelPref(); ps != nil {
					if pref, ok, _ := ps.Get(r.Context(), subject); ok {
						stickySynth = pref.SynthesisModel
					}
				}
			}
			resolved, rej := allow.ResolveEffective(req.Model, req.GatherModel, stickySynth)
			if rej != nil {
				writeOpenKnowledgeRejection(w, rej)
				return
			}
			eff = resolved
			ov = resolved.Override()
			haveSelection = true
		}
		turn, runErr := agenttool.CurrentAgent().WithModelOverride(ov).Run(r.Context(), prompt)
		if runErr != nil {
			slog.Warn("agent_invoke: open_knowledge fast-path failed", "error", runErr)
			writeAgentResponse(w, userreply.InfrastructureFailureResponse("open_knowledge_agent_failed"))
			return
		}
		env := agenttool.MapTurnResult(turn)
		if haveSelection {
			env = agenttool.WithSelection(env, eff)
		}
		writeOpenKnowledgeResponse(w, env)
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

// writeOpenKnowledgeResponse encodes the open-knowledge fast-path envelope
// (matches the substrate scenario's output_schema) as HTTP 200 JSON.
func writeOpenKnowledgeResponse(w http.ResponseWriter, env any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(env); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		slog.Warn("agent_invoke: encode open_knowledge response failed", "error", err)
	}
}

// openKnowledgeRejectionEnvelope is the spec 088 HTTP 400 body for a rejected
// per-request model override. error_code is the modelswitch reason-code
// (model_not_allowlisted | model_over_memory_envelope); message is the SAME
// verbatim sentence the Telegram surface renders (SCN-088-A06 parity).
type openKnowledgeRejectionEnvelope struct {
	Status        string   `json:"status"`
	ErrorCode     string   `json:"error_code"`
	RejectedModel string   `json:"rejected_model"`
	AllowedModels []string `json:"allowed_models"`
	DefaultModel  string   `json:"default_model"`
	// RejectedTurn (spec 089) names the refused turn ("synthesis" | "gather")
	// so a caller can tell a synthesis rejection from a gather rejection.
	// omitempty so the spec-088 synthesis-only path is byte-for-byte unchanged
	// when the resolver leaves it empty.
	RejectedTurn string `json:"rejected_turn,omitempty"`
	Message      string `json:"message"`
}

// writeOpenKnowledgeRejection encodes a modelswitch.Rejection as an HTTP 400
// envelope. The override is a malformed request value caught before the agent
// runs (parallel to the raw_input 4xx path), so 400 is the correct status —
// NOT a 5xx (the agent never failed) and NOT a 200 (nothing was answered).
func writeOpenKnowledgeRejection(w http.ResponseWriter, rej *modelswitch.Rejection) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	env := openKnowledgeRejectionEnvelope{
		Status:        "rejected",
		ErrorCode:     rej.ReasonCode,
		RejectedModel: rej.RejectedModel,
		AllowedModels: rej.AllowedModels,
		DefaultModel:  rej.DefaultModel,
		RejectedTurn:  rej.RejectedTurn,
		Message:       rej.Message,
	}
	if err := json.NewEncoder(w).Encode(env); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		slog.Warn("agent_invoke: encode open_knowledge rejection failed", "error", err)
	}
}

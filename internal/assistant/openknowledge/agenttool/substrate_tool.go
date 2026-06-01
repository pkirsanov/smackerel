// Package agenttool — Spec 064 SCOPE-12: substrate bridge that
// exposes the open-knowledge agent loop as a single Spec 037
// agent.Tool named "open_knowledge_invoke".
//
// Why a subpackage and not the openknowledge root:
//
//   - The openknowledge root package owns Tool / Source / Registry
//     types that openknowledge/agent imports.
//   - This file must import openknowledge/agent (for *Agent and
//     TurnResult), so it cannot live in openknowledge without
//     creating an import cycle (openknowledge → agent → openknowledge
//     ← openknowledge.substrate_tool).
//   - Therefore the substrate bridge ships in openknowledge/agenttool/.
//
// Why init()-time substrate registration with late-bound agent:
//
//   - cmd/scenario-lint loads every YAML in config/prompt_contracts/
//     against the live spec 037 registry. open_knowledge.yaml lists
//     "open_knowledge_invoke" as its sole allowed tool, so that name
//     MUST be registered with the substrate at package-init time or
//     the loader rejects the scenario.
//   - The real *openknowledge/agent.Agent needs runtime deps
//     (LLM client, web provider, graph searcher) that are not
//     available at package-init time. The same constraint forced
//     openknowledge/tools/registration.go to expose RegisterAll
//     instead of init() registration.
//   - Resolution: init() registers a substrate Tool whose Handler
//     reads a package-level atomic *Agent pointer. cmd/core wiring
//     calls SetAgent() once after constructing the openknowledge
//     subsystem. If a request arrives before SetAgent runs, the
//     Handler returns a structured "agent not wired" output that
//     validates against the static OutputSchema — the executor then
//     surfaces a normal scenario refusal instead of crashing.
//
// NO defaults (G028): no silent fallback agent, no silent fallback
// prompt; both must be installed by wiring. Capture-as-fallback is
// the facade's responsibility — this Handler never short-circuits the
// facade's capture-route logic.
package agenttool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
)

// ToolName is the substrate identifier the open_knowledge scenario
// allowlists. Stable; any rename is a breaking schema change.
const ToolName = "open_knowledge_invoke"

// owningPackage is the OwningPackage attribution recorded with the
// substrate registry; surfaced in traces and ops dashboards.
const owningPackage = "internal/assistant/openknowledge/agenttool"

// inputSchema validates the substrate-side tool arguments. The
// open-knowledge scenario receives {raw_query, user_id} from the
// facade's structured-context shim; the substrate tool only needs the
// prompt text plus the user id (already validated upstream).
var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["prompt"],
  "additionalProperties": false,
  "properties": {
    "prompt":  {"type": "string", "minLength": 1},
    "user_id": {"type": "string"}
  }
}`)

// outputSchema validates what the Handler returns. The scenario YAML
// uses direct_output_from_tool, so the executor emits this JSON as
// the scenario's Final answer. The facade's source-assembler then
// translates {status, body, sources, refusal_cause} into the
// AssistantResponse fields (Body, Sources, CaptureRoute).
var outputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["status", "body", "refusal_cause", "sources"],
  "additionalProperties": false,
  "properties": {
    "status":         {"type": "string", "enum": ["success", "refused"]},
    "body":           {"type": "string"},
    "refusal_cause":  {"type": "string"},
    "termination":    {"type": "string"},
    "sources":        {"type": "array", "items": {"type": "object"}}
  }
}`)

// agentRef is the late-bound *okagent.Agent pointer installed by
// SetAgent. Using atomic.Pointer keeps Handler reads lock-free.
var agentRef atomic.Pointer[okagent.Agent]

// ErrAgentNotWired is the sentinel surfaced inside the structured
// JSON when the substrate Tool is invoked before SetAgent has been
// called. It is also returned by direct Go callers of the exposed
// helpers so tests can assert against it.
var ErrAgentNotWired = errors.New("agenttool: open-knowledge agent not wired (SetAgent has not been called)")

// SetAgent installs the runtime agent used by the substrate Handler.
// Passing nil clears the binding; subsequent Handler invocations
// surface ErrAgentNotWired through a structured refusal envelope.
// cmd/core wiring calls SetAgent exactly once at startup.
func SetAgent(a *okagent.Agent) { agentRef.Store(a) }

// CurrentAgent returns the currently bound *Agent (or nil). Exported
// for tests; production code does not need this.
func CurrentAgent() *okagent.Agent { return agentRef.Load() }

// InputSchema returns a defensive copy of the substrate input schema
// so callers cannot mutate the package buffer. Used by tests.
func InputSchema() json.RawMessage {
	out := make(json.RawMessage, len(inputSchema))
	copy(out, inputSchema)
	return out
}

// OutputSchema returns a defensive copy of the substrate output schema.
func OutputSchema() json.RawMessage {
	out := make(json.RawMessage, len(outputSchema))
	copy(out, outputSchema)
	return out
}

// invokeInput is the parsed Handler argument envelope.
type invokeInput struct {
	Prompt string `json:"prompt"`
	UserID string `json:"user_id,omitempty"`
}

// outputEnvelope is the JSON shape the Handler emits.
type outputEnvelope struct {
	Status       string           `json:"status"`
	Body         string           `json:"body"`
	RefusalCause string           `json:"refusal_cause"`
	Termination  string           `json:"termination,omitempty"`
	Sources      []map[string]any `json:"sources"`
}

// Handler is the substrate ToolHandler. Exported so cmd/core wiring
// (and tests) can re-register on its own substrate instance if it
// ever needed to bypass the package-level init.
func Handler(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in invokeInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("agenttool: parse args: %w", err)
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return nil, errors.New("agenttool: prompt is required")
	}
	a := agentRef.Load()
	if a == nil {
		env := refusalEnvelope(contracts.RefusalToolUnavailable, "not_wired")
		return marshalEnvelope(env)
	}
	turn, err := a.Run(ctx, in.Prompt)
	if err != nil {
		// Infra failure — surface as a substrate error so the
		// executor records the trace with OutcomeError. The facade
		// still drives capture-as-fallback unconditionally.
		return nil, fmt.Errorf("agenttool: agent.Run: %w", err)
	}
	env := MapTurnResult(turn)
	return marshalEnvelope(env)
}

// MapTurnResult translates an openknowledge agent TurnResult into the
// substrate output envelope. Exported so substrate_tool_test.go can
// table-drive the termination-reason → refusal-cause mapping without
// constructing a live Agent.
func MapTurnResult(turn okagent.TurnResult) outputEnvelope {
	if turn.Status == okagent.StatusSuccess {
		return outputEnvelope{
			Status:      "success",
			Body:        turn.FinalText,
			Termination: string(turn.TerminationReason),
			Sources:     marshalSources(turn.Sources),
		}
	}
	cause := MapTerminationToRefusalCause(turn.TerminationReason)
	return outputEnvelope{
		Status:       "refused",
		Body:         contracts.CanonicalRefusalBodyFor(cause),
		RefusalCause: string(cause),
		Termination:  string(turn.TerminationReason),
		// Refused turns surface zero sources — the cite-back verifier
		// already rejected any unverified citations the planner emitted.
		Sources: []map[string]any{},
	}
}

// MapTerminationToRefusalCause is the closed-vocabulary mapping from
// openknowledge agent TerminationReason → spec 061 RefusalCause.
// Every non-success TerminationReason maps to a specific cause; the
// default arm is RefusalDefault so a future TerminationReason that
// pre-dates this mapping still produces a typed canonical body.
func MapTerminationToRefusalCause(r okagent.TerminationReason) contracts.RefusalCause {
	switch r {
	case okagent.TerminationCapIterations,
		okagent.TerminationCapTokens,
		okagent.TerminationCapUSD:
		return contracts.RefusalBudgetExhausted
	case okagent.TerminationToolError:
		return contracts.RefusalToolUnavailable
	case okagent.TerminationToolUnavailable:
		return contracts.RefusalToolUnavailable
	case okagent.TerminationFabricatedSource:
		return contracts.RefusalFabricatedSourceBlocked
	case okagent.TerminationRefused:
		return contracts.RefusalDefault
	case okagent.TerminationFinal:
		// TerminationFinal on Status=refused shouldn't happen; treat
		// it as the default refusal so the contract is total.
		return contracts.RefusalDefault
	default:
		return contracts.RefusalDefault
	}
}

func marshalSources(srcs []ok.Source) []map[string]any {
	if len(srcs) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(srcs))
	for _, s := range srcs {
		entry := map[string]any{"kind": s.Kind.String()}
		switch {
		case s.Artifact != nil:
			entry["artifact_id"] = s.Artifact.ID
			entry["title"] = s.Artifact.Title
		case s.Web != nil:
			entry["url"] = s.Web.URL
			entry["title"] = s.Web.Title
			entry["provider"] = s.Web.Provider
			entry["content_hash"] = s.Web.ContentHash
			entry["snippet"] = s.Web.Snippet
		case s.Computation != nil:
			entry["tool"] = s.Computation.Tool
			entry["input"] = json.RawMessage(s.Computation.Input)
			entry["output"] = json.RawMessage(s.Computation.Output)
		}
		out = append(out, entry)
	}
	return out
}

func refusalEnvelope(cause contracts.RefusalCause, termination string) outputEnvelope {
	return outputEnvelope{
		Status:       "refused",
		Body:         contracts.CanonicalRefusalBodyFor(cause),
		RefusalCause: string(cause),
		Termination:  termination,
		Sources:      []map[string]any{},
	}
}

func marshalEnvelope(env outputEnvelope) (json.RawMessage, error) {
	b, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("agenttool: marshal envelope: %w", err)
	}
	return b, nil
}

// init registers open_knowledge_invoke with the substrate registry.
// cmd/scenario-lint and cmd/core both blank-import this package so
// the substrate loader sees the tool before scenarios load.
func init() {
	agent.RegisterTool(agent.Tool{
		Name:             ToolName,
		Description:      "Bridge to the open-knowledge agent loop (spec 064). Plans web/internal/computation tools, verifies cite-back, and returns a typed body or canonical refusal.",
		InputSchema:      inputSchema,
		OutputSchema:     outputSchema,
		SideEffectClass:  agent.SideEffectExternal,
		OwningPackage:    owningPackage,
		PerCallTimeoutMs: 0, // scenario default applies (open_knowledge.yaml limits)
		Handler:          Handler,
	})
}

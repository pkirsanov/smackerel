// Spec 076 SCOPE-4b — register the no-op tool required by the
// `annotation.classify.v1` scenario.
//
// The agent loader (spec 037, `internal/agent/loader.go`) enforces
// "every scenario MUST declare at least one allowed_tools entry, and
// every named tool MUST be registered via agent.RegisterTool". The
// annotation classifier scenario is a pure classification turn — it
// has no real tool to invoke. To satisfy the loader contract without
// inviting tool-call traffic, we register `noop_annotation_classify`
// as a read-only tool whose handler always returns a structured
// rejection (the system prompt instructs the LLM NEVER to call it).
//
// This registration runs from the `internal/annotation` package's
// init() chain, which executes when `cmd/core/wiring.go` imports the
// annotation package (it already does — see annotation.NewStore).
package annotation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smackerel/smackerel/internal/agent"
)

func init() {
	agent.RegisterTool(agent.Tool{
		Name:        "noop_annotation_classify",
		Description: "Spec 076 SCOPE-4b — no-op tool registered solely to satisfy the agent loader's allowed_tools contract for the annotation_classify scenario. MUST NOT be invoked by the LLM; the scenario system prompt forbids it.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "properties": {},
            "description": "noop_annotation_classify takes no arguments and must never be invoked."
        }`),
		OutputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "required": ["rejected"],
            "properties": {
                "rejected": { "type": "boolean", "const": true }
            }
        }`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "internal/annotation",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("noop_annotation_classify must not be invoked; the annotation_classify scenario classifies in a single LLM turn")
		},
	})
}

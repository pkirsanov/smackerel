package tools

import (
	"context"
	"encoding/json"

	"github.com/smackerel/smackerel/internal/agent"
)

var genericInputSchema = json.RawMessage(`{"type":"object"}`)
var genericOutputSchema = json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}},"additionalProperties":true}`)

func init() {
	registerTool("recommendation_parse_intent", agent.SideEffectRead)
	registerTool("recommendation_reduce_location", agent.SideEffectRead)
	registerTool("recommendation_fetch_candidates", agent.SideEffectExternal)
	registerTool("recommendation_dedupe_candidates", agent.SideEffectRead)
	registerTool("recommendation_get_graph_snapshot", agent.SideEffectRead)
	registerTool("recommendation_rank_candidates", agent.SideEffectRead)
	registerTool("recommendation_apply_policy", agent.SideEffectRead)
	registerTool("recommendation_apply_quality_guard", agent.SideEffectRead)
	registerTool("recommendation_persist_outcome", agent.SideEffectWrite)
}

func registerTool(name string, sideEffect agent.SideEffectClass) {
	agent.RegisterTool(agent.Tool{
		Name:            name,
		Description:     name + " tool for the recommendation reactive scenario",
		InputSchema:     genericInputSchema,
		OutputSchema:    genericOutputSchema,
		SideEffectClass: sideEffect,
		OwningPackage:   "internal/recommendation/tools",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		},
	})
}

package main

// Spec 071 SCOPE-03 — `smackerel-core assistant <subcommand>` operator
// surface for IntentTrace replay (SCN-071-A04).
//
// Subcommands:
//
//   replay-intent <trace_id>   Load one persisted IntentTrace row and
//                              run it through the dry-run replay
//                              comparison. Prints the structured
//                              ReplayComparison as JSON.
//
// Exit codes (per design.md §"CLI Contract"):
//
//   0  PASS — replay matched (route_decision + tool_calls).
//   1  FAIL — replay completed but produced a divergence.
//   2  ERROR — trace not found, expired, or sampled-out.
//   3  ERROR — replay would require a side effect.
//   4  ERROR — persisted payload fails v1 schema validation.
//   5  ERROR — runtime failure (config load, DB connect).
//
// The CLI shares DATABASE_URL with the runtime server. It does NOT
// start the HTTP server, NATS subscribers, or any other long-lived
// goroutines — runs to completion and exits. Replay is read-only by
// construction: it never invokes the assistant facade, the router,
// or any tool executor.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
	"github.com/smackerel/smackerel/internal/config"
)

const (
	assistantExitMatch              = 0
	assistantExitDivergence         = 1
	assistantExitNotFound           = 2
	assistantExitSideEffectsBlocked = 3
	assistantExitSchemaInvalid      = 4
	assistantExitRuntime            = 5
)

func runAssistantCommand(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel-core assistant <replay-intent> [args...]")
		return assistantExitRuntime
	}
	switch args[0] {
	case "replay-intent":
		return runAssistantReplayIntent(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "smackerel-core assistant: unknown subcommand %q (want replay-intent)\n", args[0])
		return assistantExitRuntime
	}
}

func runAssistantReplayIntent(ctx context.Context, args []string) int {
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(os.Stderr, "usage: smackerel-core assistant replay-intent <trace_id>")
		return assistantExitRuntime
	}
	traceID := args[0]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core assistant replay-intent: config load: %v\n", err)
		return assistantExitRuntime
	}
	if !cfg.Assistant.IntentTrace.ReplayEnabled {
		fmt.Fprintln(os.Stderr, "smackerel-core assistant replay-intent: assistant.intent_trace.replay_enabled is false")
		return assistantExitRuntime
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel-core assistant replay-intent: DATABASE_URL is required")
		return assistantExitRuntime
	}

	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core assistant replay-intent: connect: %v\n", err)
		return assistantExitRuntime
	}
	defer pool.Close()

	store := intenttrace.NewPostgresStore(pool)
	replay := intenttrace.NewStoreReplay(store)
	res, err := replay.Run(ctx, traceID)
	if err != nil {
		switch {
		case errors.Is(err, intenttrace.ErrTraceNotFound):
			fmt.Fprintf(os.Stderr, "smackerel-core assistant replay-intent: intent_trace_not_found: %v\n", err)
			return assistantExitNotFound
		case errors.Is(err, intenttrace.ErrTraceSampledOut):
			fmt.Fprintf(os.Stderr, "smackerel-core assistant replay-intent: intent_trace_sampled_out: %v\n", err)
			return assistantExitNotFound
		case errors.Is(err, intenttrace.ErrSideEffectsBlocked):
			fmt.Fprintf(os.Stderr, "smackerel-core assistant replay-intent: side_effects_blocked: %v\n", err)
			return assistantExitSideEffectsBlocked
		case errors.Is(err, intenttrace.ErrTraceSchemaInvalid):
			fmt.Fprintf(os.Stderr, "smackerel-core assistant replay-intent: intent_trace_schema_invalid: %v\n", err)
			return assistantExitSchemaInvalid
		default:
			fmt.Fprintf(os.Stderr, "smackerel-core assistant replay-intent: %v\n", err)
			return assistantExitRuntime
		}
	}

	body, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(body))
	if res.Match.RouteDecision && res.Match.ToolCalls {
		return assistantExitMatch
	}
	return assistantExitDivergence
}

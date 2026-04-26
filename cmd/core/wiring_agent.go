// Spec 037 Scope 10 — agent runtime wiring.
//
// This file builds the production agent.Bridge and injects it into the
// API surface (api.AgentInvokeHandler) plus exposes a Reload entry
// point that main.go wires to SIGHUP.
//
// Wiring is unconditional: there is no `cfg.Agent.Enabled` flag. The
// bridge is cheap to construct when AGENT_SCENARIO_DIR is empty (the
// loader returns an empty scenario list, the router is built with no
// candidates, and every Invoke returns OutcomeUnknownIntent without
// touching the LLM driver). This keeps the wiring idempotent across
// environments — adding scenarios is a YAML drop, not a code change
// (BS-001).
//
// The embedder is intentionally agent.NoopEmbedder until specs
// 034/035/036 land their first scenarios with similarity-based
// intent_examples. Explicit-id routing (BS-002) works without any
// embedder, and the integration tests exercise BS-001 by invoking
// scenarios via their explicit id rather than relying on similarity.
//
// Hot reload (BS-019): main.go installs a SIGHUP handler that calls
// Bridge.Reload. In-flight invocations pin the *Scenario pointer they
// started with, so a swap mid-flight does not retro-edit the running
// loop.
package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
)

// wireAgentBridge constructs the production agent.Bridge and attaches
// it to deps.AgentInvokeHandler. Returns the bridge so main.go can
// install SIGHUP-triggered Reload.
//
// Returns nil bridge AND nil error when AGENT_* config is unavailable
// — this lets the rest of the runtime keep starting up while flagging
// the misconfiguration loudly. Callers MUST log the warning (the
// helper logs once for the wiring layer's audit trail).
func wireAgentBridge(ctx context.Context, svc *coreServices, deps *api.Dependencies) (*agent.Bridge, error) {
	agentCfg, err := agent.LoadConfig()
	if err != nil {
		// Fail-loud per SST contract: a misconfigured agent block must
		// surface immediately, not silently disable the bridge.
		return nil, fmt.Errorf("agent config: %w", err)
	}

	driver, err := agent.NewNATSLLMDriver(svc.nc.Conn)
	if err != nil {
		return nil, fmt.Errorf("agent NATS driver: %w", err)
	}

	tracer, err := agent.NewPostgresTracer(svc.pg.Pool, svc.nc, agentCfg.Trace.RecordLLMMessages)
	if err != nil {
		return nil, fmt.Errorf("agent tracer: %w", err)
	}
	if agentCfg.Trace.RedactMarker != "" {
		tracer = tracer.WithRedactMarker(agentCfg.Trace.RedactMarker)
	}

	exe, err := agent.NewExecutor(driver, tracer)
	if err != nil {
		return nil, fmt.Errorf("agent executor: %w", err)
	}

	bridge, rejected, err := agent.NewBridge(ctx, agent.BridgeOptions{
		Config:   agentCfg,
		Executor: exe,
	})
	if err != nil {
		return nil, fmt.Errorf("agent bridge: %w", err)
	}
	for _, r := range rejected {
		slog.Warn("agent scenario rejected by loader", "path", r.Path, "message", r.Message)
	}

	deps.AgentInvokeHandler = &api.AgentInvokeHandler{Runner: bridge}
	slog.Info("agent bridge wired",
		"scenario_dir", agentCfg.ScenarioDir,
		"scenario_count", len(bridge.KnownIntents()),
		"hot_reload", agentCfg.HotReload,
	)
	return bridge, nil
}

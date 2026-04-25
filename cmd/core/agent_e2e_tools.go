//go:build e2e_agent_tools

// E2E test tool registration. This file compiles into the smackerel-core
// binary ONLY when built with `-tags=e2e_agent_tools`. It registers a
// minimal read-only diagnostic tool ("scope6_e2e_echo") whose only
// purpose is to satisfy the scenario loader's allowed_tools registry
// check during Scope 6 replay-CLI e2e tests
// (tests/e2e/agent/replay_*_test.go).
//
// The tool is deliberately diagnostic-only: it echoes its arguments
// back. It is NOT registered in production builds (default build tags
// exclude this file), so the prod registry contract — "register tools
// from their owning package init()" — remains unviolated. This is the
// same pattern the integration tests use to register scope6_echo from
// the test package; the e2e CLI subprocess just needs the registration
// to happen via build tag instead of test file import.

package main

import (
	"context"
	"encoding/json"

	"github.com/smackerel/smackerel/internal/agent"
)

func init() {
	agent.RegisterTool(agent.Tool{
		Name:            "scope6_e2e_echo",
		Description:     "scope 6 e2e diagnostic echo tool (build tag: e2e_agent_tools)",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "cmd/core/e2e_agent_tools",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

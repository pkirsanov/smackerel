package main

// BUG-056-002 Scope B — dispatch + invocation-contract tests for the
// `connector twitter authorize-*` CLI. These exercise ONLY the pre-config
// validation paths (unknown connector / unknown subcommand / missing required
// flags / unknown flag), all of which MUST exit 2 BEFORE any config load or DB
// connect — so the tests are CI-runnable without a live Postgres (mirroring the
// spec 070 dispatchUsersSubcommand validate-before-connect structure). The
// happy-path begin/finalize/status orchestration is covered against an
// in-memory store + httptest token endpoint in
// internal/connector/twitter/oauth_authorize_test.go.

import (
	"context"
	"testing"
)

func TestRunConnectorCommand_InvocationContractFailsLoud(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"unknown connector", []string{"bogus"}},
		{"twitter with no subcommand", []string{"twitter"}},
		{"twitter unknown subcommand", []string{"twitter", "bogus-sub"}},
		{"authorize-finalize missing --code", []string{"twitter", "authorize-finalize", "--state", "s"}},
		{"authorize-finalize missing --state", []string{"twitter", "authorize-finalize", "--code", "c"}},
		{"authorize-finalize missing both flags", []string{"twitter", "authorize-finalize"}},
		{"authorize-begin unknown flag", []string{"twitter", "authorize-begin", "--bogus"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runConnectorCommand(ctx, tc.args); got != 2 {
				t.Fatalf("runConnectorCommand(%v) = %d, want 2 (invocation error before any DB/config touch)", tc.args, got)
			}
		})
	}
}

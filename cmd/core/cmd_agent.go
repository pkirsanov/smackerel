package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
)

// runAgentCommand dispatches `smackerel agent <subcommand>`. Currently
// only `replay` is implemented (spec 037 Scope 6 UC-003). Returns the
// process exit code:
//
//	0  PASS — replay returned no diff (or all drift was --allow-*ed).
//	1  FAIL — at least one diff entry survived.
//	2  ERROR — could not load the trace, scenarios, or DB.
func runAgentCommand(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel agent <replay> [args...]")
		return 2
	}
	switch args[0] {
	case "replay":
		return runAgentReplay(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "smackerel agent: unknown subcommand %q (expected: replay)\n", args[0])
		return 2
	}
}

// runAgentReplay implements `smackerel agent replay <trace_id>`.
//
// Exit codes:
//
//	0  PASS — replay matches stored trace (no drift, or drift was allowed).
//	1  FAIL — drift detected; structured diff printed to stdout.
//	2  ERROR — runtime failure (DB, missing trace, scenario load).
func runAgentReplay(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agent replay", flag.ContinueOnError)
	allowVersion := fs.Bool("allow-version-drift", false,
		"do not FAIL when scenario version differs from recorded trace")
	allowContent := fs.Bool("allow-content-drift", false,
		"do not FAIL when scenario content_hash differs from recorded trace")
	jsonOut := fs.Bool("json", false, "emit ReplayResult as JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel agent replay [--allow-version-drift] [--allow-content-drift] [--json] <trace_id>")
		return 2
	}
	traceID := fs.Arg(0)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel agent replay: DATABASE_URL must be set")
		return 2
	}

	pool, err := openReplayPool(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent replay: connect db: %v\n", err)
		return 2
	}
	defer pool.Close()

	trace, err := agent.LoadTrace(ctx, pool, traceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent replay: %v\n", err)
		return 2
	}

	scenarios, err := loadScenarioRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent replay: load scenarios: %v\n", err)
		return 2
	}

	res := agent.ReplayTrace(trace, agent.ScenarioLookupFromSlice(scenarios), agent.ReplayOptions{
		AllowVersionDrift: *allowVersion,
		AllowContentDrift: *allowContent,
	})

	if *jsonOut {
		body, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(body))
	} else {
		printReplayResultHuman(res)
	}

	if res.Pass {
		return 0
	}
	return 1
}

// openReplayPool opens a tiny pgx pool sufficient for a single-shot
// replay command (1 connection is enough; we don't want to thrash the
// running runtime's connection slots from a CLI tool).
func openReplayPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// loadScenarioRegistry calls the agent loader against AGENT_SCENARIO_DIR.
// The replay command is a CLI; it doesn't run the live router or the
// LLM driver, but it must load scenarios so it can compare versions
// and content hashes.
func loadScenarioRegistry() ([]*agent.Scenario, error) {
	cfg, err := agent.LoadConfig()
	if err != nil {
		return nil, err
	}
	registered, rejected, fatal := agent.DefaultLoader().Load(cfg.ScenarioDir, cfg.ScenarioGlob)
	if fatal != nil {
		return nil, fatal
	}
	if len(rejected) > 0 {
		// Surface but do not fail — the replay command should be
		// usable even if some unrelated scenario file is malformed.
		fmt.Fprintf(os.Stderr, "smackerel agent replay: WARNING %d scenario(s) rejected by loader:\n", len(rejected))
		for _, r := range rejected {
			fmt.Fprintf(os.Stderr, "  - %s\n", r.Error())
		}
	}
	return registered, nil
}

// printReplayResultHuman writes a short human-readable summary to stdout.
func printReplayResultHuman(res *agent.ReplayResult) {
	verdict := "PASS"
	if !res.Pass {
		verdict = "FAIL"
	}
	fmt.Printf("trace_id=%s scenario=%s version=%s verdict=%s\n",
		res.TraceID, res.ScenarioID, res.ScenarioVersion, verdict)
	if len(res.Diff) == 0 {
		return
	}
	for i, d := range res.Diff {
		fmt.Printf("  diff[%d] kind=%s field=%s recorded=%q current=%q detail=%q\n",
			i, d.Kind, d.Field, d.Recorded, d.Current, d.Detail)
	}
}

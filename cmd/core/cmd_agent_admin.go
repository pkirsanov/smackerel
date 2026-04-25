// Spec 037 Scope 8 — `smackerel agent traces|scenarios|tools` CLI
// subcommands. Each subcommand mirrors an admin web route and
// shares the rendering layer in internal/agent/render so output stays
// in sync.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/render"
)

// runAgentTraces dispatches `smackerel agent traces` and `traces show`.
func runAgentTraces(ctx context.Context, args []string) int {
	if len(args) >= 1 && args[0] == "show" {
		return runAgentTracesShow(ctx, args[1:])
	}
	return runAgentTracesList(ctx, args)
}

func runAgentTracesList(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agent traces", flag.ContinueOnError)
	outcome := fs.String("outcome", "", "filter by outcome class (ok, unknown-intent, allowlist-violation, ...)")
	limit := fs.Int("limit", 50, "max rows to return")
	offset := fs.Int("offset", 0, "row offset for paging")
	jsonOut := fs.Bool("json", false, "emit JSON instead of a text table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *outcome != "" && !render.IsValidOutcomeClass(*outcome) {
		fmt.Fprintf(os.Stderr, "smackerel agent traces: unknown outcome class %q (valid: %s)\n",
			*outcome, strings.Join(render.AllOutcomeClasses(), ", "))
		return 2
	}
	if *limit <= 0 || *limit > 1000 {
		fmt.Fprintf(os.Stderr, "smackerel agent traces: --limit must be between 1 and 1000, got %d\n", *limit)
		return 2
	}
	if *offset < 0 {
		fmt.Fprintf(os.Stderr, "smackerel agent traces: --offset must be >= 0, got %d\n", *offset)
		return 2
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel agent traces: DATABASE_URL must be set")
		return 2
	}
	pool, err := openReplayPool(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent traces: connect db: %v\n", err)
		return 2
	}
	defer pool.Close()

	rows, err := agent.ListTraces(ctx, pool, agent.TraceListFilter{Outcome: *outcome}, *limit, *offset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent traces: %v\n", err)
		return 2
	}
	summaries := make([]render.TraceSummary, 0, len(rows))
	for i := range rows {
		summaries = append(summaries, render.BuildTraceSummary(&rows[i]))
	}
	if *jsonOut {
		body, _ := json.MarshalIndent(summaries, "", "  ")
		fmt.Println(string(body))
		return 0
	}
	printTraceList(os.Stdout, summaries, *outcome)
	return 0
}

func printTraceList(w io.Writer, rows []render.TraceSummary, outcomeFilter string) {
	if outcomeFilter != "" {
		fmt.Fprintf(w, "Filter: outcome=%s   Rows: %d\n\n", outcomeFilter, len(rows))
	} else {
		fmt.Fprintf(w, "Rows: %d\n\n", len(rows))
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STARTED_AT\tTRACE_ID\tSCENARIO\tVERSION\tSOURCE\tOUTCOME\tCALLS\tLATENCY")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%dms\n",
			r.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
			truncate(r.TraceID, 24),
			r.ScenarioID,
			r.ScenarioVersion,
			r.Source,
			r.Outcome,
			r.ToolCallCount,
			r.LatencyMs,
		)
	}
	_ = tw.Flush()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func runAgentTracesShow(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agent traces show", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel agent traces show [--json] <trace_id>")
		return 2
	}
	traceID := fs.Arg(0)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel agent traces show: DATABASE_URL must be set")
		return 2
	}
	pool, err := openReplayPool(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent traces show: connect db: %v\n", err)
		return 2
	}
	defer pool.Close()

	tr, err := agent.LoadTrace(ctx, pool, traceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent traces show: %v\n", err)
		return 2
	}
	det := render.BuildTraceDetail(tr)
	if *jsonOut {
		body, _ := json.MarshalIndent(det, "", "  ")
		fmt.Println(string(body))
		return 0
	}
	printTraceDetail(os.Stdout, det)
	return 0
}

func printTraceDetail(w io.Writer, det render.TraceDetail) {
	fmt.Fprintf(w, "Trace %s\n", det.Summary.TraceID)
	fmt.Fprintf(w, "  scenario:  %s (version %s)\n", det.Summary.ScenarioID, det.Summary.ScenarioVersion)
	fmt.Fprintf(w, "  source:    %s\n", det.Summary.Source)
	fmt.Fprintf(w, "  started:   %s\n", det.Summary.StartedAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(w, "  latency:   %dms\n", det.Summary.LatencyMs)
	fmt.Fprintf(w, "  provider:  %s / model: %s\n", det.Provider, det.Model)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Outcome: [%s] %s\n", det.Outcome.Severity, det.Outcome.Label)
	fmt.Fprintf(w, "  %s\n", det.Outcome.Summary)
	for _, f := range det.Outcome.Fields {
		fmt.Fprintf(w, "    %-20s %s\n", f.Key+":", f.Value)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Routing: reason=%s chosen=%s top_score=%.3f threshold=%.3f\n",
		det.Routing.Reason, det.Routing.Chosen, det.Routing.TopScore, det.Routing.Threshold)
	for _, c := range det.Routing.Considered {
		fmt.Fprintf(w, "  considered %s @ %.3f\n", c.ScenarioID, c.Score)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Envelope: source=%s raw_input=%q\n", det.Envelope.Source, det.Envelope.RawInput)
	if det.Envelope.StructuredContext != "" {
		fmt.Fprintln(w, det.Envelope.StructuredContext)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Tool Calls (%d)\n", len(det.ToolCalls))
	for _, c := range det.ToolCalls {
		fmt.Fprintf(w, "  [%d] %s -> %s (%dms)\n", c.Seq, c.Name, c.Outcome, c.LatencyMs)
		if c.RejectionReason != "" {
			fmt.Fprintf(w, "      rejection: %s\n", c.RejectionReason)
		}
		if c.Error != "" {
			fmt.Fprintf(w, "      error: %s\n", c.Error)
		}
	}
	if det.FinalOutput != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Final Output:")
		fmt.Fprintln(w, det.FinalOutput)
	}
}

// loadScenariosForCLI runs the loader and returns (registered, rejected,
// fatal). Distinct from cmd_agent.go's loadScenarioRegistry which
// hides rejections behind a stderr warning; the operator UI surfaces
// them explicitly.
func loadScenariosForCLI() ([]*agent.Scenario, []agent.LoadError, error) {
	cfg, err := agent.LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	registered, rejected, fatal := agent.DefaultLoader().Load(cfg.ScenarioDir, cfg.ScenarioGlob)
	if fatal != nil {
		return registered, rejected, fatal
	}
	return registered, rejected, nil
}

// runAgentScenarios dispatches `smackerel agent scenarios` and
// `scenarios show <id>`.
func runAgentScenarios(_ context.Context, args []string) int {
	if len(args) >= 1 && args[0] == "show" {
		return runAgentScenariosShow(args[1:])
	}
	return runAgentScenariosList(args)
}

func runAgentScenariosList(args []string) int {
	fs := flag.NewFlagSet("agent scenarios", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	registered, rejected, fatal := loadScenariosForCLI()
	if fatal != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent scenarios: %v\n", fatal)
		// Still print whatever we have — operator may want to inspect.
	}
	registeredViews := make([]render.ScenarioSummary, 0, len(registered))
	for _, s := range registered {
		registeredViews = append(registeredViews, render.BuildScenarioSummary(s))
	}
	rejectedViews := make([]render.LoadRejectionView, 0, len(rejected))
	for _, e := range rejected {
		rejectedViews = append(rejectedViews, render.BuildLoadRejection(e))
	}
	if *jsonOut {
		out := map[string]any{
			"registered": registeredViews,
			"rejected":   rejectedViews,
		}
		body, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(body))
		if fatal != nil {
			return 2
		}
		return 0
	}
	printScenarioList(os.Stdout, registeredViews, rejectedViews)
	if fatal != nil {
		return 2
	}
	return 0
}

func printScenarioList(w io.Writer, registered []render.ScenarioSummary, rejected []render.LoadRejectionView) {
	fmt.Fprintf(w, "Registered scenarios: %d\n\n", len(registered))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tVERSION\tSIDE_EFFECT\tTOOLS\tSOURCE")
	for _, s := range registered {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			s.ID, s.Version, s.SideEffectClass, strings.Join(s.AllowedTools, ","), s.SourcePath)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Rejected at load time: %d\n", len(rejected))
	if len(rejected) == 0 {
		return
	}
	for _, r := range rejected {
		fmt.Fprintf(w, "  %s\n    %s\n", r.Path, r.Reason)
	}
}

func runAgentScenariosShow(args []string) int {
	fs := flag.NewFlagSet("agent scenarios show", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel agent scenarios show [--json] <scenario_id>")
		return 2
	}
	target := fs.Arg(0)
	registered, _, fatal := loadScenariosForCLI()
	if fatal != nil {
		fmt.Fprintf(os.Stderr, "smackerel agent scenarios show: %v\n", fatal)
		return 2
	}
	for _, s := range registered {
		if s.ID == target {
			det := render.BuildScenarioDetail(s)
			if *jsonOut {
				body, _ := json.MarshalIndent(det, "", "  ")
				fmt.Println(string(body))
				return 0
			}
			printScenarioDetail(os.Stdout, det)
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "smackerel agent scenarios show: scenario %q not found among %d registered scenarios\n",
		target, len(registered))
	return 2
}

func printScenarioDetail(w io.Writer, d render.ScenarioDetail) {
	fmt.Fprintf(w, "Scenario %s (version %s)\n", d.Summary.ID, d.Summary.Version)
	fmt.Fprintf(w, "  description:       %s\n", d.Summary.Description)
	fmt.Fprintf(w, "  side_effect_class: %s\n", d.Summary.SideEffectClass)
	fmt.Fprintf(w, "  source:            %s\n", d.Summary.SourcePath)
	fmt.Fprintf(w, "  content_hash:      %s\n", d.Summary.ContentHash)
	fmt.Fprintf(w, "  allowed_tools:     %s\n", strings.Join(d.Summary.AllowedTools, ","))
	fmt.Fprintf(w, "  model_preference:  %s\n", d.ModelPreference)
	fmt.Fprintf(w, "  token_budget:      %d\n", d.TokenBudget)
	fmt.Fprintf(w, "  temperature:       %.2f\n", d.Temperature)
	fmt.Fprintln(w, "  limits:")
	fmt.Fprintf(w, "    max_loop_iterations: %d\n", d.Limits.MaxLoopIterations)
	fmt.Fprintf(w, "    timeout_ms:          %d\n", d.Limits.TimeoutMs)
	fmt.Fprintf(w, "    schema_retry_budget: %d\n", d.Limits.SchemaRetryBudget)
	fmt.Fprintf(w, "    per_tool_timeout_ms: %d\n", d.Limits.PerToolTimeoutMs)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Intent examples:")
	for _, e := range d.IntentExamples {
		fmt.Fprintf(w, "  - %s\n", e)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "System prompt:")
	fmt.Fprintln(w, indent(d.SystemPrompt, "  "))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Input schema:")
	fmt.Fprintln(w, indent(d.InputSchema, "  "))
	fmt.Fprintln(w, "Output schema:")
	fmt.Fprintln(w, indent(d.OutputSchema, "  "))
}

func indent(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// runAgentTools dispatches `smackerel agent tools` and `tools show <name>`.
func runAgentTools(_ context.Context, args []string) int {
	if len(args) >= 1 && args[0] == "show" {
		return runAgentToolsShow(args[1:])
	}
	return runAgentToolsList(args)
}

func runAgentToolsList(args []string) int {
	fs := flag.NewFlagSet("agent tools", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	tools := agent.All()
	scenarios, _, _ := loadScenariosForCLI()
	views := make([]render.ToolSummary, 0, len(tools))
	for _, t := range tools {
		views = append(views, render.BuildToolSummary(t, render.AllowlistedBy(t.Name, scenarios)))
	}
	if *jsonOut {
		body, _ := json.MarshalIndent(views, "", "  ")
		fmt.Println(string(body))
		return 0
	}
	printToolList(os.Stdout, views)
	return 0
}

func printToolList(w io.Writer, tools []render.ToolSummary) {
	fmt.Fprintf(w, "Registered tools: %d\n\n", len(tools))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSIDE_EFFECT\tOWNING_PACKAGE\tALLOWLISTED_BY\tDESCRIPTION")
	for _, t := range tools {
		fmt.Fprintf(tw, "%s\t[%s]\t%s\t%s\t%s\n",
			t.Name, t.SideEffectClass, t.OwningPackage,
			strings.Join(t.AllowlistedByIDs, ","),
			truncate(t.Description, 60),
		)
	}
	_ = tw.Flush()
}

func runAgentToolsShow(args []string) int {
	fs := flag.NewFlagSet("agent tools show", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel agent tools show [--json] <tool_name>")
		return 2
	}
	name := fs.Arg(0)
	tool, ok := agent.ByName(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "smackerel agent tools show: tool %q not registered (have %d tools)\n",
			name, len(agent.All()))
		return 2
	}
	scenarios, _, _ := loadScenariosForCLI()
	det := render.BuildToolDetail(tool, render.AllowlistedBy(name, scenarios))
	if *jsonOut {
		body, _ := json.MarshalIndent(det, "", "  ")
		fmt.Println(string(body))
		return 0
	}
	printToolDetail(os.Stdout, det)
	return 0
}

func printToolDetail(w io.Writer, d render.ToolDetail) {
	fmt.Fprintf(w, "Tool %s\n", d.Summary.Name)
	fmt.Fprintf(w, "  side_effect_class:    [%s]\n", d.Summary.SideEffectClass)
	fmt.Fprintf(w, "  owning_package:       %s\n", d.Summary.OwningPackage)
	fmt.Fprintf(w, "  per_call_timeout_ms:  %d\n", d.Summary.PerCallTimeoutMs)
	fmt.Fprintf(w, "  description:          %s\n", d.Summary.Description)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Allowlisted by %d scenario(s):\n", len(d.Summary.AllowlistedByIDs))
	if len(d.Summary.AllowlistedByIDs) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		ids := append([]string(nil), d.Summary.AllowlistedByIDs...)
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(w, "  - %s\n", id)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Input schema:")
	fmt.Fprintln(w, indent(d.InputSchema, "  "))
	fmt.Fprintln(w, "Output schema:")
	fmt.Fprintln(w, indent(d.OutputSchema, "  "))
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// runKnowledgeCommand dispatches `smackerel knowledge <subcommand>`.
//
// Subcommands:
//
//	backfill-synthesis   Requeue artifacts stuck at synthesis_status='pending'
//	                     whose original synthesis.extract JetStream message is
//	                     gone (e.g. after an incident that lost in-flight
//	                     messages), by republishing a fresh request per
//	                     artifact using the exact same message contract the
//	                     live synthesis subscriber consumes. Batched,
//	                     rate-limited, and idempotent — see internal/knowledge
//	                     /backfill.go for the full contract.
//
// Exit codes:
//
//	0  success (including a dry-run, or a run that reached MaxArtifacts)
//	1  command-level failure (DB/NATS error, etc.)
//	2  invocation error (missing/invalid flags)
func runKnowledgeCommand(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel knowledge <backfill-synthesis> [args...]")
		return 2
	}
	switch args[0] {
	case "backfill-synthesis":
		return runKnowledgeBackfillSynthesis(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "smackerel knowledge: unknown subcommand %q (expected: backfill-synthesis)\n", args[0])
		return 2
	}
}

// runKnowledgeBackfillSynthesis implements:
//
//	smackerel knowledge backfill-synthesis \
//	    [--batch-size N] [--rate-limit-ms N] [--cooldown-minutes N] \
//	    [--max N] [--dry-run]
//
// It requires DATABASE_URL and NATS_URL (via the standard SST config load) —
// the same knowledge-layer config the running server uses — so it always
// requeues against the live database and the live JetStream SYNTHESIS
// stream, never a hand-picked target.
func runKnowledgeBackfillSynthesis(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("knowledge backfill-synthesis", flag.ContinueOnError)
	batchSize := fs.Int("batch-size", 100, "number of artifacts to select and republish per batch")
	rateLimitMs := fs.Int("rate-limit-ms", 50, "delay in milliseconds between individual publishes (0 disables rate limiting)")
	cooldownMinutes := fs.Int("cooldown-minutes", 30, "minutes an already-requeued artifact is excluded from re-selection (idempotency window)")
	maxArtifacts := fs.Int("max", 0, "cap on total artifacts republished this run; 0 means drain the full backlog")
	dryRun := fs.Bool("dry-run", false, "select and log candidates without publishing or mutating any row")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: smackerel knowledge backfill-synthesis [--batch-size N] [--rate-limit-ms N] [--cooldown-minutes N] [--max N] [--dry-run]")
		return 2
	}
	if *batchSize <= 0 {
		fmt.Fprintln(os.Stderr, "smackerel knowledge backfill-synthesis: --batch-size must be > 0")
		return 2
	}
	if *rateLimitMs < 0 {
		fmt.Fprintln(os.Stderr, "smackerel knowledge backfill-synthesis: --rate-limit-ms must be >= 0")
		return 2
	}
	if *cooldownMinutes <= 0 {
		fmt.Fprintln(os.Stderr, "smackerel knowledge backfill-synthesis: --cooldown-minutes must be > 0")
		return 2
	}
	if *maxArtifacts < 0 {
		fmt.Fprintln(os.Stderr, "smackerel knowledge backfill-synthesis: --max must be >= 0")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel knowledge backfill-synthesis: config load: %v\n", err)
		return 1
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel knowledge backfill-synthesis: DATABASE_URL is required")
		return 1
	}
	if cfg.NATSURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel knowledge backfill-synthesis: NATS_URL is required")
		return 1
	}
	if !cfg.KnowledgeEnabled {
		fmt.Fprintln(os.Stderr, "smackerel knowledge backfill-synthesis: knowledge layer is disabled (KNOWLEDGE_ENABLED != true); nothing to backfill")
		return 1
	}

	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel knowledge backfill-synthesis: connect db: %v\n", err)
		return 1
	}
	defer pool.Close()

	nc, err := smacknats.Connect(ctx, cfg.NATSURL, cfg.AuthToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel knowledge backfill-synthesis: connect nats: %v\n", err)
		return 1
	}
	defer nc.Conn.Close()

	store := knowledge.NewKnowledgeStore(pool)
	linterCfg := knowledge.LinterConfig{
		StaleDays:                cfg.KnowledgeLintStaleDays,
		MaxSynthesisRetries:      cfg.KnowledgeMaxSynthesisRetries,
		PromptContractVersion:    cfg.KnowledgePromptContractIngestSynthesis,
		MaxSynthesisContextItems: 50,
		MaxSynthesisContentChars: 8000,
	}
	linter := knowledge.NewLinter(store, pool, linterCfg, nc)

	pendingBefore, err := store.CountArtifactsBySynthesisStatus(ctx, "pending")
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel knowledge backfill-synthesis: count pending backlog: %v\n", err)
		return 1
	}
	fmt.Printf("synthesis backlog: %d artifacts currently pending\n", pendingBefore)
	if *dryRun {
		fmt.Println("dry-run: no messages will be published, no rows will be modified")
	}

	opts := knowledge.SynthesisBackfillOptions{
		BatchSize:       *batchSize,
		PerMessageDelay: time.Duration(*rateLimitMs) * time.Millisecond,
		Cooldown:        time.Duration(*cooldownMinutes) * time.Minute,
		MaxArtifacts:    *maxArtifacts,
		DryRun:          *dryRun,
		TriggeredBy:     "backfill_cli",
		Progress: func(p knowledge.SynthesisBackfillProgress) {
			fmt.Printf("progress: batch=%d processed=%d failed=%d\n", p.BatchCount, p.Processed, p.Failed)
		},
	}

	result, err := linter.RunSynthesisBackfill(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel knowledge backfill-synthesis: %v\n", err)
		fmt.Fprintf(os.Stderr, "partial progress before failure: processed=%d failed=%d batches=%d\n",
			result.Processed, result.Failed, result.Batches)
		return 1
	}

	pendingAfter, countErr := store.CountArtifactsBySynthesisStatus(ctx, "pending")
	if countErr != nil {
		slog.Warn("knowledge backfill-synthesis: failed to count final pending backlog", "error", countErr)
	}

	fmt.Printf("done: processed=%d failed=%d batches=%d exhausted=%v\n",
		result.Processed, result.Failed, result.Batches, result.Exhausted)
	fmt.Printf("synthesis backlog: %d pending before this run, %d pending now\n", pendingBefore, pendingAfter)
	if result.Failed > 0 {
		fmt.Printf("warning: %d artifacts failed to republish — re-run the command to retry them\n", result.Failed)
	}
	return 0
}

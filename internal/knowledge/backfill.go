package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// SynthesisBackfillOptions configures a requeue/backfill run over artifacts
// stuck at synthesis_status='pending' whose original synthesis.extract
// request is gone (e.g. the JetStream messages that would have driven them
// were lost in an incident and the stream now reports zero pending messages
// for them).
type SynthesisBackfillOptions struct {
	// BatchSize is how many candidate artifacts are selected and republished
	// per iteration. Keeps memory bounded and gives operators a natural
	// checkpoint: an interrupted run has published at most BatchSize items
	// past its last logged progress line.
	BatchSize int

	// PerMessageDelay is slept between individual publishes within a batch,
	// so a backlog of thousands of artifacts does not slam NATS (or the ML
	// sidecar behind it) with a burst of thousands of requests at once.
	PerMessageDelay time.Duration

	// Cooldown is how long a "requeued_by_backfill" marker suppresses
	// re-selection of the same artifact (see GetSynthesisBackfillCandidates).
	// Must be long enough for the ML sidecar to plausibly finish a normal
	// extraction; a run resumed sooner than this will simply see fewer
	// candidates and make no forward progress on those items yet, which is
	// the intended, safe behavior rather than an error.
	Cooldown time.Duration

	// MaxArtifacts caps the total number of artifacts republished in this
	// invocation, across all batches. Zero means no cap (drain the full
	// backlog). Operators can pass a small value to do a canary run before
	// requeuing the entire backlog.
	MaxArtifacts int

	// DryRun selects and logs candidates without publishing or mutating any
	// row, so an operator can see exactly what a run would do first.
	DryRun bool

	// TriggeredBy is recorded on every published request's "triggered_by"
	// field for operator-facing traceability (e.g. "backfill_cli").
	TriggeredBy string

	// Progress, if non-nil, is invoked after every batch with cumulative
	// counters so a caller (the CLI) can render live progress. Optional.
	Progress func(SynthesisBackfillProgress)
}

// SynthesisBackfillProgress reports cumulative counters after each batch.
type SynthesisBackfillProgress struct {
	Processed  int // artifacts successfully republished so far in this run
	Failed     int // artifacts that failed to republish so far in this run
	BatchCount int // number of batches completed so far
}

// SynthesisBackfillResult summarizes a completed (or interrupted) run.
type SynthesisBackfillResult struct {
	Processed int
	Failed    int
	Batches   int
	// Exhausted is true when the run stopped because no more candidates
	// remained (as opposed to stopping because MaxArtifacts was reached or
	// the context was cancelled).
	Exhausted bool
}

func (o SynthesisBackfillOptions) normalized() SynthesisBackfillOptions {
	if o.BatchSize <= 0 {
		o.BatchSize = 100
	}
	if o.Cooldown <= 0 {
		o.Cooldown = 30 * time.Minute
	}
	if o.TriggeredBy == "" {
		o.TriggeredBy = "backfill_cli"
	}
	return o
}

// RunSynthesisBackfill drains the synthesis_status='pending' backlog in
// batches, republishing a fresh synthesis.extract request (built via
// BuildSynthesisExtractRequest — the exact same contract the live
// SynthesisResultSubscriber consumes) for each candidate artifact, rate
// limited by PerMessageDelay, and marking each one requeued so a re-run
// (including one resuming after this process was interrupted) is idempotent
// per GetSynthesisBackfillCandidates' cooldown rule.
//
// It stops when: no candidates remain (Exhausted=true), MaxArtifacts is
// reached, or ctx is cancelled. It never panics on a single artifact's
// failure — that artifact is counted in Failed and the run continues, since
// one bad row (e.g. a partially-corrupted content_raw) must not block
// progress on the other ~20,000.
func (l *Linter) RunSynthesisBackfill(ctx context.Context, opts SynthesisBackfillOptions) (SynthesisBackfillResult, error) {
	opts = opts.normalized()
	var result SynthesisBackfillResult

	for {
		if err := ctx.Err(); err != nil {
			slog.Warn("knowledge backfill: context cancelled, stopping",
				"processed", result.Processed, "failed", result.Failed, "batches", result.Batches)
			return result, err
		}

		batchLimit := opts.BatchSize
		if opts.MaxArtifacts > 0 {
			remaining := opts.MaxArtifacts - result.Processed
			if remaining <= 0 {
				return result, nil
			}
			if remaining < batchLimit {
				batchLimit = remaining
			}
		}

		candidates, err := l.store.GetSynthesisBackfillCandidates(ctx, batchLimit, opts.Cooldown)
		if err != nil {
			return result, fmt.Errorf("select backfill candidates: %w", err)
		}
		if len(candidates) == 0 {
			result.Exhausted = true
			slog.Info("knowledge backfill: no more candidates, backlog drained",
				"processed", result.Processed, "failed", result.Failed, "batches", result.Batches)
			return result, nil
		}

		for _, a := range candidates {
			if err := ctx.Err(); err != nil {
				return result, err
			}

			if opts.DryRun {
				slog.Info("knowledge backfill: dry-run candidate", "artifact_id", a.ID, "title", a.Title)
				result.Processed++
			} else if pubErr := l.PublishSynthesisExtractRequest(ctx, a, opts.TriggeredBy); pubErr != nil {
				slog.Warn("knowledge backfill: publish failed, will retry on a later run",
					"artifact_id", a.ID, "error", pubErr)
				result.Failed++
			} else if markErr := l.store.MarkRequeuedByBackfill(ctx, a.ID); markErr != nil {
				// The publish already went out — NATS has the message — but
				// we could not stamp the idempotency marker. Log loudly:
				// a re-run before the ML sidecar responds may re-publish
				// this one artifact (duplicate synthesis, not data loss;
				// the subscriber's transactional status update makes a
				// second successful extraction a harmless no-op overwrite).
				slog.Warn("knowledge backfill: published but failed to mark requeued (may be re-selected by a re-run)",
					"artifact_id", a.ID, "error", markErr)
				result.Processed++
			} else {
				result.Processed++
			}

			if opts.PerMessageDelay > 0 {
				select {
				case <-ctx.Done():
					return result, ctx.Err()
				case <-time.After(opts.PerMessageDelay):
				}
			}
		}

		result.Batches++
		remainingPending, countErr := l.store.CountArtifactsBySynthesisStatus(ctx, "pending")
		if countErr != nil {
			slog.Warn("knowledge backfill: failed to count remaining pending backlog", "error", countErr)
		}
		slog.Info("knowledge backfill: batch complete",
			"batch", result.Batches,
			"batch_size", len(candidates),
			"processed_total", result.Processed,
			"failed_total", result.Failed,
			"pending_remaining", remainingPending,
			"dry_run", opts.DryRun,
		)
		if opts.Progress != nil {
			opts.Progress(SynthesisBackfillProgress{
				Processed:  result.Processed,
				Failed:     result.Failed,
				BatchCount: result.Batches,
			})
		}
	}
}

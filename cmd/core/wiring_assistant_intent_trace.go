// Spec 071 SCOPE-02 — IntentTrace wiring (recorder + sampler + redactor + exporter + retention sweep).
//
// This file is the single startup glue that installs the spec 071
// observability surface onto an already-built assistant.Facade. It
// reads the SST-resolved IntentTraceConfig, constructs the
// deterministic sampler, the central redactor, the Postgres store,
// the derived-export fan-out, and the recorder. The recorder is then
// attached to the facade through WithIntentTrace. A retention-sweep
// goroutine is started against the same store.
//
// Fail-loud per SST: missing or invalid config is rejected upstream
// by loadIntentTraceConfig; this function defensively checks for
// the structural invariants (non-nil pool, non-empty export targets)
// and refuses to wire on violation. The assistant facade falls back
// to the pre-spec-071 no-op recorder when wiring is refused so the
// runtime continues to function — the missing trace plane is
// surfaced via the structured log warning written here.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
	"github.com/smackerel/smackerel/internal/config"
)

// wireAssistantIntentTrace builds the spec 071 SCOPE-02 wiring and
// installs it on the supplied facade. Called from wireAssistantFacade
// after the facade is constructed.
func wireAssistantIntentTrace(
	ctx context.Context,
	cfg *config.Config,
	svc *coreServices,
	facade *assistant.Facade,
) error {
	if cfg == nil {
		return errors.New("wireAssistantIntentTrace: nil config")
	}
	if facade == nil {
		return errors.New("wireAssistantIntentTrace: nil facade")
	}
	if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
		return errors.New("wireAssistantIntentTrace: postgres pool is required")
	}
	it := cfg.Assistant.IntentTrace
	if len(it.ExportTargets) == 0 {
		return errors.New("wireAssistantIntentTrace: ExportTargets empty (SST loader must have failed loud first)")
	}
	sampler, err := intenttrace.NewRatioSampler(it.SamplingRatio)
	if err != nil {
		return fmt.Errorf("wireAssistantIntentTrace: build sampler: %w", err)
	}
	redactor := intenttrace.NewDefaultRedactor()
	// Spec 071 §"Hard Constraint 2" — redaction is centralised; the
	// universal v1 policy refuses to persist raw user text and treats
	// no slot class as universally sensitive (per-source overrides
	// land in a follow-up scope when a SourcePolicy SST surface
	// exists). PersistRawText=false is the safe minimum; leaving it
	// explicit prevents a silent default.
	policy := intenttrace.NewSourcePolicy(false, nil)
	store := intenttrace.NewPostgresStore(svc.pg.Pool)
	exporter := intenttrace.NewDefaultExporter(it.ExportTargets)
	recorder := intenttrace.NewStoreRecorder(store, time.Duration(it.RetentionDays)*24*time.Hour).WithExporter(exporter)
	facade.WithIntentTrace(assistant.IntentTraceWiring{
		Recorder: recorder,
		Sampler:  sampler,
		Redactor: redactor,
		Policy:   policy,
	})
	go func() {
		if err := intenttrace.RunRetentionSweep(ctx, store, it.RetentionSweepInterval, func() time.Time { return time.Now().UTC() }); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("assistant_intent_trace_retention_sweep_stopped", "error", err)
		}
	}()
	slog.Info("assistant IntentTrace wired",
		"sampling_ratio", it.SamplingRatio,
		"retention_days", it.RetentionDays,
		"export_targets", it.ExportTargets,
		"sweep_interval", it.RetentionSweepInterval.String(),
	)
	return nil
}

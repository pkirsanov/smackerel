// legacy_retirement.go — spec 076 SCOPE-6a scheduler bindings.
//
// Registers two periodic jobs sourced from the SST keys carried by
// internal/config.LegacyRetirementConfig:
//
//   - Threshold evaluator: invoked every
//     legacy_retirement.threshold_evaluator_interval_seconds; calls
//     the supplied ThresholdEvaluator.Evaluate closure to apply the
//     percent + flat-count rollback gates and pause the runtime
//     window on breach (SCN-075-A05 / A06).
//   - Post-window observation cron: invoked on the SST cron
//     expression to persist the zero-invocation gate evidence row
//     (SCN-075-A08) that spec 066 final-deletion consumes.
//
// The scheduler is the SST-coupled surface. The actual evaluator /
// observation closures are constructed in cmd/core (where the SQL
// pool and SST config are wired) and handed to SetLegacyRetirementJobs
// as plain func(ctx) error callbacks to keep the scheduler package
// free of legacyretirement / pgx dependencies.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// LegacyRetirementJobFunc is the closure invoked by the threshold and
// observation jobs. Returning an error logs a warning; the scheduler
// continues running.
type LegacyRetirementJobFunc func(ctx context.Context) error

// SetLegacyRetirementJobs registers the threshold-evaluator interval
// job and the post-window observation cron. Must be called before
// Start(). Either closure may be nil to disable its corresponding
// job, but the interval / cron MUST still be non-zero / non-empty
// when the closure is supplied (fail-loud during Start()).
func (s *Scheduler) SetLegacyRetirementJobs(
	thresholdInterval time.Duration,
	thresholdFn LegacyRetirementJobFunc,
	observationCron string,
	observationFn LegacyRetirementJobFunc,
) {
	s.muLegacyRetirement.Lock()
	defer s.muLegacyRetirement.Unlock()
	s.legacyRetirementThresholdInterval = thresholdInterval
	s.legacyRetirementThresholdFn = thresholdFn
	s.legacyRetirementObservationCron = observationCron
	s.legacyRetirementObservationFn = observationFn
}

func (s *Scheduler) scheduleLegacyRetirementJobs() {
	s.muLegacyRetirement.Lock()
	thresholdFn := s.legacyRetirementThresholdFn
	thresholdInterval := s.legacyRetirementThresholdInterval
	observationFn := s.legacyRetirementObservationFn
	observationCron := s.legacyRetirementObservationCron
	s.muLegacyRetirement.Unlock()

	if thresholdFn != nil {
		if thresholdInterval <= 0 {
			slog.Warn("legacy retirement threshold evaluator: interval must be > 0; job not scheduled",
				"interval", thresholdInterval)
		} else {
			expr := fmt.Sprintf("@every %ds", int(thresholdInterval/time.Second))
			if _, err := s.cron.AddFunc(expr, s.runLegacyRetirementThresholdJob); err != nil {
				slog.Warn("failed to schedule legacy retirement threshold evaluator",
					"error", err, "interval", thresholdInterval)
			} else {
				slog.Info("legacy retirement threshold evaluator scheduled", "interval", thresholdInterval)
			}
		}
	}
	if observationFn != nil {
		if observationCron == "" {
			slog.Warn("legacy retirement observation cron: cron expression empty; job not scheduled")
		} else {
			if _, err := s.cron.AddFunc(observationCron, s.runLegacyRetirementObservationJob); err != nil {
				slog.Warn("failed to schedule legacy retirement observation cron",
					"error", err, "cron", observationCron)
			} else {
				slog.Info("legacy retirement observation cron scheduled", "cron", observationCron)
			}
		}
	}
}

func (s *Scheduler) runLegacyRetirementThresholdJob() {
	s.runGuarded(&s.muLegacyRetirementThreshold, "legacy-retirement-threshold", "legacy-retirement-threshold", s.doLegacyRetirementThresholdJob)
}

func (s *Scheduler) doLegacyRetirementThresholdJob() {
	s.muLegacyRetirement.Lock()
	fn := s.legacyRetirementThresholdFn
	s.muLegacyRetirement.Unlock()
	if fn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.baseCtx, 60*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		slog.Warn("legacy retirement threshold evaluator failed", "error", err)
	}
}

func (s *Scheduler) runLegacyRetirementObservationJob() {
	s.runGuarded(&s.muLegacyRetirementObservation, "legacy-retirement-observation", "legacy-retirement-observation", s.doLegacyRetirementObservationJob)
}

func (s *Scheduler) doLegacyRetirementObservationJob() {
	s.muLegacyRetirement.Lock()
	fn := s.legacyRetirementObservationFn
	s.muLegacyRetirement.Unlock()
	if fn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
	defer cancel()
	if err := fn(ctx); err != nil {
		slog.Warn("legacy retirement observation cron failed", "error", err)
	}
}

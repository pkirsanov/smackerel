// wiring_legacy_retirement_scheduler.go — spec 076 SCOPE-6a.
//
// Constructs the threshold evaluator and post-window observation
// closures from the SQL pool + SST config and hands them to the
// scheduler's SetLegacyRetirementJobs setter. Called from main()
// after the scheduler is constructed and before Start().
//
// Fail-loud per Gate G028 / smackerel-no-defaults: every SST key is
// already validated by config.LoadLegacyRetirement(); this wiring
// helper additionally fails-loud (WARN + skip) if the SQL pool is
// unavailable, so dev/test installs without a database still boot
// (the jobs are simply not registered).
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/scheduler"
)

func wireLegacyRetirementScheduler(cfg *config.Config, svc *coreServices, sched *scheduler.Scheduler) {
	if sched == nil {
		return
	}
	if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
		slog.Warn("legacy retirement scheduler: postgres pool unavailable; threshold + observation jobs not registered")
		return
	}
	pool := svc.pg.Pool
	lr := cfg.LegacyRetirement

	pauseStore, err := legacyretirement.NewSQLPauseStateStore(pool)
	if err != nil {
		slog.Warn("legacy retirement scheduler: pause store construction failed; jobs not registered",
			"error", err)
		return
	}
	residualStore, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: lr.WindowID,
		Clock:    time.Now,
	})
	if err != nil {
		slog.Warn("legacy retirement scheduler: residual store construction failed; threshold job not registered",
			"error", err)
		return
	}
	activeUsers, err := legacyretirement.NewSQLActiveUsersProvider(pool, lr.WindowID)
	if err != nil {
		slog.Warn("legacy retirement scheduler: active-users provider construction failed; threshold job not registered",
			"error", err)
		return
	}
	evalCfg := legacyretirement.ThresholdConfig{
		WindowID:                  lr.WindowID,
		PercentActiveUsers:        lr.RollbackThresholdPercentActiveUsers,
		DaysConsecutive:           lr.RollbackThresholdDaysConsecutive,
		ActiveUserWindowDays:      lr.ActiveUserWindowDays,
		ThresholdEvaluatorUpdater: "threshold_evaluator",
		DailyInvocationsThreshold: lr.RollbackThresholdDailyInvocations,
	}
	evaluator, err := legacyretirement.NewThresholdEvaluator(evalCfg, residualStore, activeUsers, pauseStore)
	if err != nil {
		slog.Warn("legacy retirement scheduler: threshold evaluator construction failed; threshold job not registered",
			"error", err)
		return
	}
	thresholdFn := func(ctx context.Context) error {
		_, evErr := evaluator.Evaluate(ctx, time.Now().UTC())
		return evErr
	}

	observationReport, err := legacyretirement.NewSQLObservationReport(legacyretirement.SQLObservationReportConfig{
		Pool:   pool,
		Source: &legacyretirement.PrometheusInvocationSource{},
		Clock:  time.Now,
	})
	if err != nil {
		slog.Warn("legacy retirement scheduler: observation report construction failed; observation job not registered",
			"error", err)
		// Still register the threshold job alone.
		sched.SetLegacyRetirementJobs(
			time.Duration(lr.ThresholdEvaluatorIntervalSeconds)*time.Second,
			thresholdFn,
			"",
			nil,
		)
		return
	}
	observationDuration := time.Duration(lr.PostWindowObservationDays) * 24 * time.Hour
	observationFn := func(ctx context.Context) error {
		_, oErr := observationReport.Generate(ctx, lr.WindowID, observationDuration)
		return oErr
	}

	sched.SetLegacyRetirementJobs(
		time.Duration(lr.ThresholdEvaluatorIntervalSeconds)*time.Second,
		thresholdFn,
		lr.ObservationCronExpr,
		observationFn,
	)
	slog.Info("legacy retirement scheduler wired (spec 076 SCOPE-6a)",
		"interval_seconds", lr.ThresholdEvaluatorIntervalSeconds,
		"observation_cron", lr.ObservationCronExpr,
		"daily_invocations_threshold", lr.RollbackThresholdDailyInvocations,
	)
}

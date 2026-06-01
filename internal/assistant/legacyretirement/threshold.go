// threshold.go — spec 075 SCOPE-4 rollback threshold evaluator.
//
// Inputs:
//
//   - Rolling residual report (per-command, per-day distinct buckets)
//     from SCOPE-3 ResidualReportQuery.
//   - Active-user denominator for the configured lookback window
//     (ActiveUserWindowDays, from SST). Provided by the caller via
//     ActiveUsersProvider so this package does not depend on the
//     assistant data plane.
//   - SST thresholds: RollbackThresholdPercentActiveUsers and
//     RollbackThresholdDaysConsecutive (NO hardcoded defaults — both
//     come from LegacyRetirementConfig).
//
// Decision (SCN-075-A05): for each retired command, walk the most
// recent N UTC dates ending at `now` (where N =
// daysConsecutive). A day is "over threshold" when
// (distinct_user_buckets_for_command / active_users) * 100 strictly
// exceeds percentThreshold. A command breaches when EVERY one of
// those N dates is over threshold.
//
// Side effects:
//
//   - ThresholdOverCounter (per-command) is incremented once per
//     evaluation pass per breaching day.
//   - On a breach: the SCOPE-4 PauseStateStore is asked to UPSERT
//     the runtime pause row (effective_state="paused"), which the
//     existing WindowStateResolver (sststate.go) reads on its next
//     Resolve(). Subsequent Policy.Handle() calls then resolve to
//     WindowPaused, which suppresses new notices while leaving
//     ServeNL=true (SCN-075-A05 "legacy handlers continue serving").
//   - On resume (SCN-075-A06): PauseStateStore.Resume resets
//     consecutive_days_over_threshold and flips effective_state
//     back to "open". Residual telemetry counters are NOT reset.
package legacyretirement

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// ActiveUsersProvider returns the distinct-active-user count over
// the SST-defined ActiveUserWindowDays lookback ending at `now`.
// The implementation lives outside this package (the assistant data
// plane owns "what counts as active"); the evaluator only consumes
// the denominator.
type ActiveUsersProvider interface {
	ActiveUsers(ctx context.Context, now time.Time, lookbackDays int) (int64, error)
}

// StaticActiveUsersProvider is a fixed denominator used by unit tests.
type StaticActiveUsersProvider struct{ Count int64 }

// ActiveUsers implements ActiveUsersProvider.
func (s StaticActiveUsersProvider) ActiveUsers(context.Context, time.Time, int) (int64, error) {
	return s.Count, nil
}

// ThresholdEvaluation is the per-command result of one evaluation pass.
type ThresholdEvaluation struct {
	Command         string
	ConsecutiveDays int
	Breached        bool
	// BreachingDays lists the UTC dates (newest first) that
	// individually crossed the percent threshold during this pass.
	BreachingDays []time.Time
}

// PauseStateStore is the SCOPE-4 writer/reader contract for the
// durable runtime pause-state row (assistant_legacy_retirement_state).
// It composes PauseStateReader so a single instance can serve both
// the WindowStateResolver and the evaluator.
type PauseStateStore interface {
	PauseStateReader
	// Pause records or refreshes the pause row for the supplied
	// breaching command and consecutive-day count. updatedBy is the
	// audit string written to the row (e.g. "threshold_evaluator"
	// or an admin operator id).
	Pause(ctx context.Context, windowID, command string, consecutiveDays int, now time.Time, updatedBy string) error
	// Resume flips the row back to effective_state="open" and
	// resets consecutive_days_over_threshold to 0 (SCN-075-A06).
	Resume(ctx context.Context, windowID string, now time.Time, updatedBy string) error
}

// ThresholdConfig captures the SST inputs the evaluator needs.
// Every field is required; there are no defaults.
type ThresholdConfig struct {
	WindowID                  string
	PercentActiveUsers        float64
	DaysConsecutive           int
	ActiveUserWindowDays      int
	ThresholdEvaluatorUpdater string // updated_by audit label
}

// Validate rejects any zero/empty field. The caller is expected to
// hand in a ThresholdConfig derived from LegacyRetirementConfig.
func (c ThresholdConfig) Validate() error {
	if c.WindowID == "" {
		return fmt.Errorf("legacyretirement: ThresholdConfig.WindowID empty")
	}
	if c.PercentActiveUsers <= 0 || c.PercentActiveUsers > 100 {
		return fmt.Errorf("legacyretirement: ThresholdConfig.PercentActiveUsers=%v out of range (0,100]", c.PercentActiveUsers)
	}
	if c.DaysConsecutive < 1 {
		return fmt.Errorf("legacyretirement: ThresholdConfig.DaysConsecutive=%d must be >= 1", c.DaysConsecutive)
	}
	if c.ActiveUserWindowDays < 1 {
		return fmt.Errorf("legacyretirement: ThresholdConfig.ActiveUserWindowDays=%d must be >= 1", c.ActiveUserWindowDays)
	}
	if c.ThresholdEvaluatorUpdater == "" {
		return fmt.Errorf("legacyretirement: ThresholdConfig.ThresholdEvaluatorUpdater empty")
	}
	return nil
}

// ThresholdEvaluator runs the per-command breach check and drives
// the PauseStateStore. Construct one per process; Evaluate is safe
// to invoke from a scheduler.
type ThresholdEvaluator struct {
	cfg     ThresholdConfig
	report  ResidualReportQuery
	active  ActiveUsersProvider
	pauseDB PauseStateStore
}

// NewThresholdEvaluator validates inputs and returns the evaluator.
func NewThresholdEvaluator(cfg ThresholdConfig, report ResidualReportQuery, active ActiveUsersProvider, pauseDB PauseStateStore) (*ThresholdEvaluator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if report == nil {
		return nil, fmt.Errorf("legacyretirement: NewThresholdEvaluator: report is nil")
	}
	if active == nil {
		return nil, fmt.Errorf("legacyretirement: NewThresholdEvaluator: active is nil")
	}
	if pauseDB == nil {
		return nil, fmt.Errorf("legacyretirement: NewThresholdEvaluator: pauseDB is nil")
	}
	return &ThresholdEvaluator{cfg: cfg, report: report, active: active, pauseDB: pauseDB}, nil
}

// Evaluate pulls the rolling residual report, computes per-command
// breach status against the SST thresholds, increments the
// ThresholdOverCounter for breaching days, and on breach asks the
// PauseStateStore to record the paused state. Returns the
// per-command evaluations and the first breach error (if any).
func (e *ThresholdEvaluator) Evaluate(ctx context.Context, now time.Time) ([]ThresholdEvaluation, error) {
	report, err := e.report.RollingSevenDay(ctx, e.cfg.WindowID, now)
	if err != nil {
		return nil, fmt.Errorf("legacyretirement: evaluator rolling report: %w", err)
	}
	active, err := e.active.ActiveUsers(ctx, now, e.cfg.ActiveUserWindowDays)
	if err != nil {
		return nil, fmt.Errorf("legacyretirement: evaluator active users: %w", err)
	}
	if active <= 0 {
		return nil, fmt.Errorf("legacyretirement: evaluator: active users denominator is %d (must be > 0)", active)
	}

	// Group per-day rows by command for the breach walk.
	byCommand := make(map[string][]ResidualPerDayRow)
	for _, r := range report.PerDay {
		byCommand[r.Command] = append(byCommand[r.Command], r)
	}

	nowUTC := now.UTC()
	endDay := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)

	commands := make([]string, 0, len(byCommand))
	for c := range byCommand {
		commands = append(commands, c)
	}
	sort.Strings(commands)

	out := make([]ThresholdEvaluation, 0, len(commands))
	for _, command := range commands {
		rows := byCommand[command]
		distinctByDay := make(map[time.Time]int64, len(rows))
		for _, r := range rows {
			d := r.Day.UTC()
			d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
			distinctByDay[d] = r.DistinctUsers
		}

		eval := ThresholdEvaluation{Command: command}
		breachedAll := true
		for i := 0; i < e.cfg.DaysConsecutive; i++ {
			day := endDay.AddDate(0, 0, -i)
			distinct := distinctByDay[day]
			pct := (float64(distinct) / float64(active)) * 100.0
			if pct > e.cfg.PercentActiveUsers {
				eval.BreachingDays = append(eval.BreachingDays, day)
				ThresholdOverCounter.WithLabelValues(command).Inc()
			} else {
				breachedAll = false
			}
		}
		eval.ConsecutiveDays = len(eval.BreachingDays)
		eval.Breached = breachedAll && eval.ConsecutiveDays == e.cfg.DaysConsecutive

		if eval.Breached {
			if err := e.pauseDB.Pause(ctx, e.cfg.WindowID, command, eval.ConsecutiveDays, now, e.cfg.ThresholdEvaluatorUpdater); err != nil {
				return out, fmt.Errorf("legacyretirement: evaluator pause for %q: %w", command, err)
			}
			WindowStateGauge.WithLabelValues(string(WindowPaused)).Set(1)
			WindowStateGauge.WithLabelValues(string(WindowOpen)).Set(0)
		}
		out = append(out, eval)
	}
	return out, nil
}

// Resume is a convenience that forwards to the underlying
// PauseStateStore and updates the effective-state gauge so dashboards
// reflect the operator action immediately (SCN-075-A06).
func (e *ThresholdEvaluator) Resume(ctx context.Context, now time.Time, operatorID string) error {
	if operatorID == "" {
		return fmt.Errorf("legacyretirement: Resume: operatorID empty (audit required)")
	}
	if err := e.pauseDB.Resume(ctx, e.cfg.WindowID, now, operatorID); err != nil {
		return err
	}
	WindowStateGauge.WithLabelValues(string(WindowOpen)).Set(1)
	WindowStateGauge.WithLabelValues(string(WindowPaused)).Set(0)
	return nil
}

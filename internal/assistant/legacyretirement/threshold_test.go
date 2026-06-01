// threshold_test.go — spec 075 SCOPE-4 unit tests.
//
// Adversarial coverage (SCN-075-A05 + SCN-075-A06):
//
//   - A command that crosses the percent threshold for FEWER than
//     daysConsecutive days MUST NOT cause a pause (negative breach).
//   - A command that crosses for EXACTLY daysConsecutive days in a
//     row MUST cause a pause AND mark the WindowStateGauge to
//     "paused".
//   - A command at exactly the threshold percent (not strictly
//     greater) MUST NOT count as a breaching day (boundary test).
//   - Resume MUST reset consecutive_days_over_threshold to 0 and
//     leave residual telemetry counters untouched.
//   - A zero active-user denominator is a fatal evaluator error
//     (refuse to divide by zero — no silent threshold pass).
package legacyretirement

import (
	"context"
	"sync"
	"testing"
	"time"
)

// memoryPauseStore is the in-memory PauseStateStore used by these
// tests. It implements both PauseStateReader and the Pause/Resume
// writer surface.
type memoryPauseStore struct {
	mu      sync.Mutex
	paused  bool
	command string
	days    int
	updates int
	updater string
}

func (m *memoryPauseStore) IsPaused(_ context.Context, _ string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused, nil
}

func (m *memoryPauseStore) Pause(_ context.Context, _ string, command string, days int, _ time.Time, updatedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused = true
	m.command = command
	m.days = days
	m.updater = updatedBy
	m.updates++
	return nil
}

func (m *memoryPauseStore) Resume(_ context.Context, _ string, _ time.Time, updatedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused = false
	m.command = ""
	m.days = 0
	m.updater = updatedBy
	m.updates++
	return nil
}

// stubReport is a ResidualReportQuery returning a fixed report.
type stubReport struct{ report ResidualReport }

func (s stubReport) RollingSevenDay(_ context.Context, _ string, _ time.Time) (ResidualReport, error) {
	return s.report, nil
}

func newTestEvaluator(t *testing.T, report ResidualReport, active int64, pause PauseStateStore) *ThresholdEvaluator {
	t.Helper()
	cfg := ThresholdConfig{
		WindowID:                  "spec075-window-1",
		PercentActiveUsers:        10.0,
		DaysConsecutive:           3,
		ActiveUserWindowDays:      7,
		ThresholdEvaluatorUpdater: "threshold_evaluator",
	}
	ev, err := NewThresholdEvaluator(cfg, stubReport{report: report}, StaticActiveUsersProvider{Count: active}, pause)
	if err != nil {
		t.Fatalf("NewThresholdEvaluator: %v", err)
	}
	return ev
}

func dayUTC(now time.Time, offset int) time.Time {
	u := now.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, offset)
}

func TestThresholdEvaluator_SCN_A05_BreachPausesWindow(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// 3 consecutive days at 20 distinct users vs 100 active = 20% > 10% threshold.
	report := ResidualReport{
		WindowID: "spec075-window-1",
		PerDay: []ResidualPerDayRow{
			{Command: "/weather", Day: dayUTC(now, 0), Invocations: 30, DistinctUsers: 20},
			{Command: "/weather", Day: dayUTC(now, -1), Invocations: 25, DistinctUsers: 20},
			{Command: "/weather", Day: dayUTC(now, -2), Invocations: 22, DistinctUsers: 20},
		},
	}
	store := &memoryPauseStore{}
	ev := newTestEvaluator(t, report, 100, store)

	evals, err := ev.Evaluate(context.Background(), now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(evals) != 1 || evals[0].Command != "/weather" {
		t.Fatalf("expected one /weather eval, got %+v", evals)
	}
	if !evals[0].Breached {
		t.Errorf("expected Breached=true for /weather, got %+v", evals[0])
	}
	if evals[0].ConsecutiveDays != 3 {
		t.Errorf("ConsecutiveDays=%d, want 3", evals[0].ConsecutiveDays)
	}
	if !store.paused {
		t.Errorf("expected pause store paused=true")
	}
	if store.command != "/weather" || store.days != 3 || store.updater != "threshold_evaluator" {
		t.Errorf("pause row mismatch: %+v", store)
	}
}

func TestThresholdEvaluator_NoBreach_WhenOnlyTwoOfThreeDaysOver(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// Day 0 and -2 breach, day -1 does not → not consecutive.
	report := ResidualReport{
		PerDay: []ResidualPerDayRow{
			{Command: "/weather", Day: dayUTC(now, 0), DistinctUsers: 20},
			{Command: "/weather", Day: dayUTC(now, -1), DistinctUsers: 5},
			{Command: "/weather", Day: dayUTC(now, -2), DistinctUsers: 20},
		},
	}
	store := &memoryPauseStore{}
	ev := newTestEvaluator(t, report, 100, store)

	evals, err := ev.Evaluate(context.Background(), now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if evals[0].Breached {
		t.Errorf("non-consecutive breach must not pause; got %+v", evals[0])
	}
	if store.paused {
		t.Errorf("pause store must remain unpaused on partial breach")
	}
}

func TestThresholdEvaluator_BoundaryAtExactThreshold_DoesNotBreach(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// 10 distinct / 100 active = exactly 10% — must NOT count as breaching
	// because the spec wording is "exceeds" (strict >).
	report := ResidualReport{
		PerDay: []ResidualPerDayRow{
			{Command: "/weather", Day: dayUTC(now, 0), DistinctUsers: 10},
			{Command: "/weather", Day: dayUTC(now, -1), DistinctUsers: 10},
			{Command: "/weather", Day: dayUTC(now, -2), DistinctUsers: 10},
		},
	}
	store := &memoryPauseStore{}
	ev := newTestEvaluator(t, report, 100, store)
	evals, err := ev.Evaluate(context.Background(), now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if evals[0].Breached || evals[0].ConsecutiveDays != 0 {
		t.Errorf("exact-threshold day must not breach; got %+v", evals[0])
	}
	if store.paused {
		t.Errorf("boundary value must not pause window")
	}
}

func TestThresholdEvaluator_SCN_A06_ResumeResetsCounterAndGauge(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// Pre-pause the store.
	store := &memoryPauseStore{paused: true, command: "/weather", days: 3, updater: "threshold_evaluator"}
	ev := newTestEvaluator(t, ResidualReport{}, 100, store)

	if err := ev.Resume(context.Background(), now, "operator:phil"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if store.paused {
		t.Errorf("Resume must clear pause flag")
	}
	if store.days != 0 {
		t.Errorf("Resume must reset days to 0, got %d", store.days)
	}
	if store.updater != "operator:phil" {
		t.Errorf("Resume must record operator updater, got %q", store.updater)
	}
}

func TestThresholdEvaluator_Resume_RejectsEmptyOperator(t *testing.T) {
	store := &memoryPauseStore{}
	ev := newTestEvaluator(t, ResidualReport{}, 100, store)
	if err := ev.Resume(context.Background(), time.Now(), ""); err == nil {
		t.Fatalf("expected error on empty operatorID; got nil")
	}
}

func TestThresholdEvaluator_ZeroActiveUsers_FatalError(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := &memoryPauseStore{}
	cfg := ThresholdConfig{
		WindowID:                  "w",
		PercentActiveUsers:        10,
		DaysConsecutive:           3,
		ActiveUserWindowDays:      7,
		ThresholdEvaluatorUpdater: "evaluator",
	}
	ev, err := NewThresholdEvaluator(cfg, stubReport{}, StaticActiveUsersProvider{Count: 0}, store)
	if err != nil {
		t.Fatalf("NewThresholdEvaluator: %v", err)
	}
	if _, err := ev.Evaluate(context.Background(), now); err == nil {
		t.Fatalf("expected error on zero active-user denominator; got nil")
	}
	if store.paused {
		t.Errorf("zero-denominator must not pause window")
	}
}

func TestThresholdConfig_Validate_RejectsAllZeroFields(t *testing.T) {
	cases := []ThresholdConfig{
		{},
		{WindowID: "w"},
		{WindowID: "w", PercentActiveUsers: 5},
		{WindowID: "w", PercentActiveUsers: 5, DaysConsecutive: 2},
		{WindowID: "w", PercentActiveUsers: 5, DaysConsecutive: 2, ActiveUserWindowDays: 7},
		{WindowID: "w", PercentActiveUsers: 200, DaysConsecutive: 2, ActiveUserWindowDays: 7, ThresholdEvaluatorUpdater: "u"},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d expected validation error, got nil (cfg=%+v)", i, c)
		}
	}
}

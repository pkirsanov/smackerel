// Package config — contract test for `config/upkeep-calendar.yaml`.
//
// upkeep-calendar.yaml drives `bubbles.upkeep` and its 7 calendar-bound
// tasks: backup, restore-test, bcdr-drill, patch-cycle, secret-rotation,
// flag-cleanup-audit, compliance-sweep. None of these tasks had Go-side
// validation today — a regression that (a) dropped a required task,
// (b) regressed a cadence to an invalid value, (c) silently changed
// retention below a compliance floor, or (d) broke the YAML shape
// would slip past `./smackerel.sh test unit` and break:
//   - upkeep-monthly / upkeep-patch-cycle / upkeep-secret-rotation /
//     upkeep-bcdr-drill / upkeep-backup-verify / upkeep-restore-drill /
//     upkeep-flag-cleanup / upkeep-compliance-sweep workflow modes
//   - the operator runbook in docs/Upkeep_Runbook.md
//   - the upkeep instruction file's "All recurring operational hygiene
//     flows through bubbles.upkeep" contract
//
// This Go contract test enforces structural invariants and required-task
// presence so a regression fails at `go test ./...` time. Adversarial
// sub-tests prove each invariant catches its target failure mode.
//
// References:
//   - .github/instructions/bubbles-upkeep-operations.instructions.md
//   - .github/skills/bubbles-upkeep-cadence/SKILL.md
//   - .github/skills/bubbles-backup-bcdr-doctrine/SKILL.md
//   - docs/Upkeep_Runbook.md
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type upkeepDoc struct {
	Version int          `yaml:"version"`
	Tasks   []upkeepTask `yaml:"tasks"`
}

type upkeepTask struct {
	ID                     string   `yaml:"id"`
	Cadence                string   `yaml:"cadence"`
	At                     string   `yaml:"at"`
	Retention              string   `yaml:"retention"`
	BlocksOnFailure        []string `yaml:"blocks_on_failure"`
	RequiresOffsiteBackend bool     `yaml:"requires_offsite_backend"`
}

var validUpkeepCadences = map[string]bool{
	"daily":     true,
	"weekly":    true,
	"monthly":   true,
	"quarterly": true,
	"yearly":    true,
}

// Required upkeep tasks per
// .github/instructions/bubbles-upkeep-operations.instructions.md
// "All recurring operational hygiene (backup, restore-drill, BCDR-drill,
// patch-cycle, secret-rotation, flag-cleanup, compliance-sweep) flows
// through bubbles.upkeep". Restore is called `restore-test` in the
// calendar; both names refer to the same task. Flag-cleanup is
// `flag-cleanup-audit` in the calendar.
type requiredUpkeepTask struct {
	id      string
	cadence string // expected minimum cadence (anti-regression: don't let monthly drop to quarterly)
	purpose string
}

var requiredUpkeepTasks = []requiredUpkeepTask{
	{id: "backup", cadence: "daily", purpose: "data durability"},
	{id: "restore-test", cadence: "weekly", purpose: "G115 restore-drill"},
	{id: "bcdr-drill", cadence: "quarterly", purpose: "BCDR-drill"},
	{id: "patch-cycle", cadence: "monthly", purpose: "security patches"},
	{id: "secret-rotation", cadence: "quarterly", purpose: "G119 secret rotation"},
	{id: "flag-cleanup-audit", cadence: "monthly", purpose: "feature-flag hygiene"},
	{id: "compliance-sweep", cadence: "quarterly", purpose: "G117/G118/G119/G120 compliance evidence"},
}

func upkeepRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// assertUpkeepContract validates upkeep-calendar.yaml structure.
func assertUpkeepContract(yamlBytes []byte) error {
	var doc upkeepDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	if doc.Version != 1 {
		return fmt.Errorf("contract violation: version=%d (expected 1)", doc.Version)
	}
	if len(doc.Tasks) == 0 {
		return fmt.Errorf("contract violation: no tasks declared (expected >= 1)")
	}

	seenIDs := make(map[string]upkeepTask, len(doc.Tasks))
	for i, t := range doc.Tasks {
		where := fmt.Sprintf("tasks[%d]", i)
		if strings.TrimSpace(t.ID) == "" {
			return fmt.Errorf("contract violation: %s.id is empty", where)
		}
		if _, exists := seenIDs[t.ID]; exists {
			return fmt.Errorf("contract violation: duplicate task id %q", t.ID)
		}
		where = fmt.Sprintf("tasks[%d] (id=%q)", i, t.ID)
		if !validUpkeepCadences[t.Cadence] {
			return fmt.Errorf("contract violation: %s.cadence=%q (expected one of daily|weekly|monthly|quarterly|yearly)", where, t.Cadence)
		}
		if strings.TrimSpace(t.Retention) == "" {
			return fmt.Errorf("contract violation: %s.retention is empty (Gate G118 requires every upkeep task to declare retention)", where)
		}
		seenIDs[t.ID] = t
	}

	for _, req := range requiredUpkeepTasks {
		task, ok := seenIDs[req.id]
		if !ok {
			return fmt.Errorf("contract violation: required task %q (purpose: %s) is missing — deleting this task removes operational hygiene that the upkeep instructions require", req.id, req.purpose)
		}
		// Cadence regression check: a required task's cadence must be at least as frequent
		// as the documented baseline. We encode this by comparing string positions in the
		// cadence-frequency ordering (daily < weekly < monthly < quarterly < yearly).
		// Going from daily -> weekly silently weakens the operational posture.
		freq := map[string]int{"daily": 0, "weekly": 1, "monthly": 2, "quarterly": 3, "yearly": 4}
		liveFreq, ok1 := freq[task.Cadence]
		minFreq, ok2 := freq[req.cadence]
		if ok1 && ok2 && liveFreq > minFreq {
			return fmt.Errorf("contract violation: required task %q has cadence %q which is LESS FREQUENT than the documented baseline %q (purpose: %s) — this silently weakens operational posture", req.id, task.Cadence, req.cadence, req.purpose)
		}
	}
	return nil
}

// TestUpkeepCalendarContract_LiveFile parses the live upkeep-calendar.yaml
// and asserts every invariant holds.
func TestUpkeepCalendarContract_LiveFile(t *testing.T) {
	path := filepath.Join(upkeepRepoRoot(t), "config", "upkeep-calendar.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := assertUpkeepContract(b); err != nil {
		t.Fatalf("live upkeep-calendar.yaml violates contract: %v", err)
	}
}

// TestUpkeepCalendarContract_AdversarialMissingRequiredTask proves the
// contract test would FAIL if a required upkeep task were silently
// deleted from the calendar.
func TestUpkeepCalendarContract_AdversarialMissingRequiredTask(t *testing.T) {
	bad := []byte(`version: 1
tasks:
- id: backup
  cadence: daily
  retention: "30d"
- id: restore-test
  cadence: weekly
  retention: "90d"
# bcdr-drill deleted
- id: patch-cycle
  cadence: monthly
  retention: "1y"
- id: secret-rotation
  cadence: quarterly
  retention: "5y"
- id: flag-cleanup-audit
  cadence: monthly
  retention: "1y"
- id: compliance-sweep
  cadence: quarterly
  retention: "7y"
`)
	err := assertUpkeepContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted calendar without bcdr-drill — would break BCDR posture")
	}
	if !strings.Contains(err.Error(), "bcdr-drill") {
		t.Fatalf("expected message naming missing bcdr-drill task, got: %v", err)
	}
}

// TestUpkeepCalendarContract_AdversarialInvalidCadence proves the contract
// test would FAIL if a task regressed to an invalid cadence (typo or
// invented value).
func TestUpkeepCalendarContract_AdversarialInvalidCadence(t *testing.T) {
	bad := []byte(`version: 1
tasks:
- id: backup
  cadence: hourly
  retention: "30d"
`)
	err := assertUpkeepContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted cadence=hourly — would break engine scheduling assumptions")
	}
	if !strings.Contains(err.Error(), "cadence") {
		t.Fatalf("expected cadence rejection, got: %v", err)
	}
}

// TestUpkeepCalendarContract_AdversarialCadenceRegression proves the
// contract test would FAIL if a required task's cadence silently
// regressed to a less frequent value (e.g., daily backup → weekly).
func TestUpkeepCalendarContract_AdversarialCadenceRegression(t *testing.T) {
	bad := []byte(`version: 1
tasks:
- id: backup
  cadence: weekly
  retention: "30d"
- id: restore-test
  cadence: weekly
  retention: "90d"
- id: bcdr-drill
  cadence: quarterly
  retention: "5y"
- id: patch-cycle
  cadence: monthly
  retention: "1y"
- id: secret-rotation
  cadence: quarterly
  retention: "5y"
- id: flag-cleanup-audit
  cadence: monthly
  retention: "1y"
- id: compliance-sweep
  cadence: quarterly
  retention: "7y"
`)
	err := assertUpkeepContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted backup cadence regression daily -> weekly — silently weakens data durability")
	}
	if !strings.Contains(err.Error(), "LESS FREQUENT") {
		t.Fatalf("expected LESS FREQUENT rejection, got: %v", err)
	}
}

// TestUpkeepCalendarContract_AdversarialMissingRetention proves the
// contract test would FAIL if a task dropped its retention field —
// Gate G118 requires every upkeep task to declare retention.
func TestUpkeepCalendarContract_AdversarialMissingRetention(t *testing.T) {
	bad := []byte(`version: 1
tasks:
- id: backup
  cadence: daily
`)
	err := assertUpkeepContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted task without retention — would violate Gate G118")
	}
	if !strings.Contains(err.Error(), "retention") {
		t.Fatalf("expected retention rejection, got: %v", err)
	}
}

// TestUpkeepCalendarContract_AdversarialDuplicateTaskID proves the
// contract test would FAIL if two tasks accidentally collided on id.
func TestUpkeepCalendarContract_AdversarialDuplicateTaskID(t *testing.T) {
	bad := []byte(`version: 1
tasks:
- id: backup
  cadence: daily
  retention: "30d"
- id: backup
  cadence: weekly
  retention: "90d"
`)
	err := assertUpkeepContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted duplicate task id — would create non-deterministic scheduler behavior")
	}
	if !strings.Contains(err.Error(), "duplicate task id") {
		t.Fatalf("expected duplicate-id rejection, got: %v", err)
	}
}

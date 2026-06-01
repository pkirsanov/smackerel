//go:build integration

// Spec 067 Scope 1 — SCN-067-A07 (policy-exception ratchet).
//
// Three tests:
//   1. Rejects unreviewed exception growth (an accepted exception
//      whose ID is not in the committed baseline).
//   2. Accepts a current exception that matches the baseline.
//   3. (Scope 4 forward-compat) Tracks NO-DEFAULTS exceptions by
//      rule_id so the per-rule accounting is provable here without
//      waiting on Scope 4's scanners.

package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeBaselineForTest(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "policy-exception-baseline.json")
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	return p
}

func validCfg() PolicyConfig {
	return PolicyConfig{
		ScenarioPromptMaxLines:   120,
		ExceptionBaselinePath:    "policy-exception-baseline.json",
		ExceptionMaxAgeDays:      90,
		IntentBypassGuardEnabled: true,
	}
}

// TestPolicyExceptionGuardRejectsUnreviewedExceptionGrowth — SCN-067-A07.
// A current exception not present in the committed baseline MUST
// produce a G067-A07 violation that names the unreviewed ID and the
// required resolution (bump baseline with reviewer approval).
func TestPolicyExceptionGuardRejectsUnreviewedExceptionGrowth(t *testing.T) {
	path := writeBaselineForTest(t, `{
      "schema_version": "v1",
      "policy": "specs/067-intent-driven-policy-enforcement",
      "exceptions": []
    }`)
	baseline, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	current := []Exception{
		{
			ID:        "G067-A05-ml-main-embedding-model-20260601",
			RuleID:    "G067-A05",
			Path:      "ml/app/main.py",
			Owner:     "reviewer",
			Reason:    "migration window",
			ExpiresOn: "2026-06-30",
		},
	}
	v, delta := RatchetExceptions(baseline, current, now, validCfg())
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1; v=%+v", len(v), v)
	}
	if v[0].RuleID != "G067-A07" {
		t.Fatalf("violation RuleID = %q, want G067-A07", v[0].RuleID)
	}
	if v[0].Resolution == "" {
		t.Fatalf("violation must carry a Resolution naming the required baseline bump")
	}
	if delta.DeltaStatus != "grew" {
		t.Fatalf("delta = %q, want grew", delta.DeltaStatus)
	}
}

// TestPolicyExceptionGuardAcceptsBaselineMatchedExceptions — SCN-067-A07
// positive (canary). A current exception whose ID matches the
// baseline, and whose metadata is valid and unexpired, MUST NOT
// produce a violation; delta is "unchanged".
func TestPolicyExceptionGuardAcceptsBaselineMatchedExceptions(t *testing.T) {
	path := writeBaselineForTest(t, `{
      "schema_version": "v1",
      "policy": "specs/067-intent-driven-policy-enforcement",
      "exceptions": [
        {
          "id": "G067-A05-ml-main-embedding-model-20260601",
          "rule_id": "G067-A05",
          "path": "ml/app/main.py",
          "owner": "reviewer",
          "reason": "migration window",
          "expires_on": "2026-06-30"
        }
      ]
    }`)
	baseline, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	current := []Exception{
		{
			ID:        "G067-A05-ml-main-embedding-model-20260601",
			RuleID:    "G067-A05",
			Path:      "ml/app/main.py",
			Owner:     "reviewer",
			Reason:    "migration window",
			ExpiresOn: "2026-06-30",
		},
	}
	v, delta := RatchetExceptions(baseline, current, now, validCfg())
	if len(v) != 0 {
		t.Fatalf("expected zero violations; got %+v", v)
	}
	if delta.DeltaStatus != "unchanged" {
		t.Fatalf("delta = %q, want unchanged", delta.DeltaStatus)
	}
}

// TestPolicyExceptionGuardRejectsExpiredException is the adversarial
// time-axis case: an exception present in the baseline but past its
// expires_on date MUST still produce a violation. Without this, the
// ratchet would silently retain stale exceptions indefinitely.
func TestPolicyExceptionGuardRejectsExpiredException(t *testing.T) {
	path := writeBaselineForTest(t, `{
      "schema_version": "v1",
      "policy": "specs/067-intent-driven-policy-enforcement",
      "exceptions": [
        {
          "id": "G067-A05-expired",
          "rule_id": "G067-A05",
          "path": "ml/app/main.py",
          "owner": "reviewer",
          "reason": "stale",
          "expires_on": "2026-05-01"
        }
      ]
    }`)
	baseline, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	current := []Exception{
		{
			ID: "G067-A05-expired", RuleID: "G067-A05",
			Path: "ml/app/main.py", Owner: "reviewer",
			Reason: "stale", ExpiresOn: "2026-05-01",
		},
	}
	v, _ := RatchetExceptions(baseline, current, now, validCfg())
	if len(v) != 1 || v[0].RuleID != "G067-A07" {
		t.Fatalf("expected single G067-A07 expired-exception violation, got %+v", v)
	}
}

// TestPolicyExceptionGuardTracksNoDefaultsExceptionsByRuleID — SCN-067-A05
// and SCN-067-A06 forward-compat. The ratchet groups exceptions by
// rule_id, so the per-rule "how many NO-DEFAULTS exceptions are
// accepted" question is answerable from the committed baseline alone.
// Scope 4 scanners will plug into this same shape; this test pins it
// now so Scope 4 cannot quietly re-define the contract.
func TestPolicyExceptionGuardTracksNoDefaultsExceptionsByRuleID(t *testing.T) {
	path := writeBaselineForTest(t, `{
      "schema_version": "v1",
      "policy": "specs/067-intent-driven-policy-enforcement",
      "exceptions": [
        {"id":"G067-A05-1","rule_id":"G067-A05","path":"ml/app/a.py","owner":"r","reason":"x","expires_on":"2026-06-30"},
        {"id":"G067-A05-2","rule_id":"G067-A05","path":"ml/app/b.py","owner":"r","reason":"x","expires_on":"2026-06-30"},
        {"id":"G067-A06-1","rule_id":"G067-A06","path":"internal/x.go","owner":"r","reason":"x","expires_on":"2026-06-30"}
      ]
    }`)
	baseline, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	counts := map[string]int{}
	for _, e := range baseline.Exceptions {
		counts[e.RuleID]++
	}
	if counts["G067-A05"] != 2 {
		t.Fatalf("G067-A05 count = %d, want 2", counts["G067-A05"])
	}
	if counts["G067-A06"] != 1 {
		t.Fatalf("G067-A06 count = %d, want 1", counts["G067-A06"])
	}
}

// TestLoadBaselineFailsLoudOnMissingFile is the bootstrap-error path:
// no silent fallback to "empty baseline".
func TestLoadBaselineFailsLoudOnMissingFile(t *testing.T) {
	_, err := LoadBaseline(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatalf("LoadBaseline accepted missing file; expected fail-loud bootstrap error")
	}
}

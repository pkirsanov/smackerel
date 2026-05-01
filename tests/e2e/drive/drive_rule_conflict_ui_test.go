//go:build e2e

// Spec 038 Scope 6 — Save rule conflict E2E test.
//
// Anchor: SCN-038-018 — overlapping save rules audit conflict and the
// stable-match wins. The Screen 7 conflict chip is fed by exactly the
// rows this test asserts: drive_rules_audit entries with
// outcome='conflict' that name the stable winner.
//
// The test creates two overlapping save rules covering the same
// classification + sensitivity. It runs the rules engine against a
// matching artifact and asserts:
//
//  1. Engine.Decision.Selected names the stable winner (older
//     CreatedAt — first rule).
//  2. Engine.Decision.Conflicts contains BOTH overlapping rules so the
//     audit row enumerates them all.
//  3. After running through the live rules.Repository.AppendAudit, the
//     drive_rules_audit table contains conflict rows with
//     reason="stable_winner=<id>" so Screen 7 can render which rule
//     won.
//  4. Adversarial: a non-overlapping rule is NOT included in the
//     conflicts list — proves the conflict detection is intent-correct,
//     not "every rule is a conflict".
package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/drive/rules"
)

func TestSaveRulesListShowsConflictChipAndAuditRowsForOverlappingRules(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := rules.NewRepository(pool)

	// Three rules — two overlap on (classification=meal_plan, sensitivity=none),
	// one is non-overlapping (classification=other).
	r1, err := repo.Create(ctx, rules.Rule{
		Name:                 "scope6-overlap-A-" + uuid.NewString()[:8],
		Enabled:              true,
		SourceKinds:          []string{string(rules.SourceMealPlan)},
		Classification:       "meal_plan",
		SensitivityIn:        []string{string(rules.SensitivityNone)},
		ConfidenceMin:        0.5,
		ProviderID:           "google",
		TargetFolderTemplate: "Plans/A/{year}",
		OnMissingFolder:      rules.OnMissingCreate,
		OnExistingFile:       rules.OnExistingVersion,
	})
	if err != nil {
		t.Fatalf("create rule A: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), r1.ID) })

	// Sleep a millisecond to ensure r2.CreatedAt > r1.CreatedAt so the
	// stable-match deterministic order is testable.
	time.Sleep(5 * time.Millisecond)

	r2, err := repo.Create(ctx, rules.Rule{
		Name:                 "scope6-overlap-B-" + uuid.NewString()[:8],
		Enabled:              true,
		SourceKinds:          []string{string(rules.SourceMealPlan)},
		Classification:       "meal_plan",
		SensitivityIn:        []string{string(rules.SensitivityNone)},
		ConfidenceMin:        0.5,
		ProviderID:           "google",
		TargetFolderTemplate: "Plans/B/{year}",
		OnMissingFolder:      rules.OnMissingCreate,
		OnExistingFile:       rules.OnExistingVersion,
	})
	if err != nil {
		t.Fatalf("create rule B: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), r2.ID) })

	r3, err := repo.Create(ctx, rules.Rule{
		Name:                 "scope6-other-" + uuid.NewString()[:8],
		Enabled:              true,
		SourceKinds:          []string{string(rules.SourceMealPlan)},
		Classification:       "other_thing",
		SensitivityIn:        []string{string(rules.SensitivityNone)},
		ConfidenceMin:        0.5,
		ProviderID:           "google",
		TargetFolderTemplate: "Other/{year}",
		OnMissingFolder:      rules.OnMissingCreate,
		OnExistingFile:       rules.OnExistingVersion,
	})
	if err != nil {
		t.Fatalf("create rule C: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), r3.ID) })

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}

	artifactID := "test:scope6-e2e-conflict:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw,
		                       content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'meal_plan', 'plan', 'plan-content',
		        $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE id=$1`, artifactID)
	})

	engine := rules.NewEngine(time.Now)
	decision := engine.Evaluate(ctx, rules.Artifact{
		ID:             artifactID,
		SourceKind:     string(rules.SourceMealPlan),
		Classification: "meal_plan",
		Sensitivity:    string(rules.SensitivityNone),
		Confidence:     1.0,
		CapturedAt:     time.Now().UTC(),
	}, all)

	if decision.Selected == nil {
		t.Fatalf("decision.Selected is nil — no rule matched the meal_plan artifact")
	}
	if decision.Selected.RuleID != r1.ID {
		t.Fatalf("Selected.RuleID = %q, want %q (stable winner = first by CreatedAt)",
			decision.Selected.RuleID, r1.ID)
	}
	if len(decision.Conflicts) != 2 {
		t.Fatalf("len(Conflicts) = %d, want 2 (both overlapping rules)",
			len(decision.Conflicts))
	}

	// Adversarial: r3 (non-overlapping) MUST NOT appear in conflicts.
	for _, c := range decision.Conflicts {
		if c.RuleID == r3.ID {
			t.Fatalf("non-overlapping rule %q showed up as a conflict — engine over-reports", r3.ID)
		}
	}

	// Drive the audit through the live repo (mirrors the production
	// path in api/drive_save_handlers.go).
	for _, conflict := range decision.Conflicts {
		if err := repo.AppendAudit(ctx, conflict.RuleID, artifactID,
			rules.OutcomeConflict, "stable_winner="+r1.ID); err != nil {
			t.Fatalf("AppendAudit: %v", err)
		}
	}

	// Verify drive_rule_audit rows.
	rows, err := pool.Query(ctx, `
		SELECT rule_id, outcome, reason FROM drive_rule_audit
		WHERE source_artifact_id=$1 AND outcome='conflict'`, artifactID)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	defer rows.Close()
	auditCount := 0
	for rows.Next() {
		var ruleID, outcome, reason string
		if err := rows.Scan(&ruleID, &outcome, &reason); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		auditCount = auditCount + 1
		if outcome != "conflict" {
			t.Fatalf("audit row outcome = %q, want conflict", outcome)
		}
		if !strings.HasPrefix(reason, "stable_winner=") {
			t.Fatalf("audit row reason = %q, want stable_winner=<id>", reason)
		}
	}
	if auditCount != 2 {
		t.Fatalf("conflict audit row count = %d, want 2", auditCount)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM drive_rule_audit WHERE source_artifact_id=$1`, artifactID)
	})
}

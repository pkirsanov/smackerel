// Spec 038 Scope 6 — SCN-038-018 unit anchor.
//
// TestOverlappingRulesAuditConflictAndExecuteStableMatch asserts the
// engine's behavior when more than one rule matches the same artifact:
//
//  1. The engine MUST select the first stable match (sorted by created_at
//     then by ID) so two operators committing similar rules in different
//     orders converge on the same winner.
//  2. The engine MUST surface every matching rule in Decision.Conflicts
//     so the audit log can record one conflict row per matched rule.
//     The conflict list MUST include the winner so Screen 7's conflict
//     chip can show "winner + N collisions".
//  3. Adversarial: rules whose source_kind / classification / sensitivity
//     do NOT match MUST NOT appear in Conflicts. A regression that
//     widened "matched" to include all evaluated rules would surface
//     here.
//  4. Adversarial: a single matching rule MUST NOT populate Conflicts.
//     The engine MUST distinguish "one match" from "many matches" so
//     Screen 7 only flags real conflicts.
//  5. Adversarial: when two rules have identical CreatedAt timestamps,
//     ID order breaks the tie. A regression that fell back to map order
//     would produce non-deterministic winners and would fail this case.
package rules

import (
	"context"
	"sort"
	"testing"
	"time"
)

func TestOverlappingRulesAuditConflictAndExecuteStableMatch(t *testing.T) {
	engine := NewEngine(func() time.Time { return time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC) })
	baseRule := Rule{
		Enabled:              true,
		SourceKinds:          []string{string(SourceTelegram)},
		Classification:       "receipt",
		SensitivityIn:        []string{string(SensitivityFinancial)},
		ConfidenceMin:        0.5,
		ProviderID:           "google",
		TargetFolderTemplate: "Receipts/{year}",
		OnMissingFolder:      OnMissingCreate,
		OnExistingFile:       OnExistingVersion,
	}

	t.Run("first stable match wins and Conflicts includes every matched rule", func(t *testing.T) {
		ruleEarly := baseRule
		ruleEarly.ID = "rule-early"
		ruleEarly.Name = "Early"
		ruleEarly.CreatedAt = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		ruleLate := baseRule
		ruleLate.ID = "rule-late"
		ruleLate.Name = "Late"
		ruleLate.TargetFolderTemplate = "Receipts/Override/{year}"
		ruleLate.CreatedAt = time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
		artifact := Artifact{
			ID:             "artifact-conflict",
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.92,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}

		decision := engine.Evaluate(context.Background(), artifact, []Rule{ruleLate, ruleEarly})
		if decision.Selected == nil {
			t.Fatalf("Selected = nil, want first stable match %q", ruleEarly.ID)
		}
		if decision.Selected.RuleID != ruleEarly.ID {
			t.Fatalf("Selected.RuleID = %q, want %q (oldest CreatedAt wins)", decision.Selected.RuleID, ruleEarly.ID)
		}
		if decision.Selected.RenderedPath != "Receipts/2026" {
			t.Fatalf("RenderedPath = %q, want %q", decision.Selected.RenderedPath, "Receipts/2026")
		}
		if len(decision.Conflicts) != 2 {
			t.Fatalf("Conflicts len = %d, want 2 (winner + collision)", len(decision.Conflicts))
		}
		// Every conflict row MUST point back to the selected rule via
		// ConflictGroupID so audit consumers can reconstruct the group.
		for i, c := range decision.Conflicts {
			if c.ConflictGroupID != ruleEarly.ID {
				t.Fatalf("Conflicts[%d].ConflictGroupID = %q, want %q", i, c.ConflictGroupID, ruleEarly.ID)
			}
		}
		got := []string{decision.Conflicts[0].RuleID, decision.Conflicts[1].RuleID}
		sort.Strings(got)
		want := []string{ruleEarly.ID, ruleLate.ID}
		sort.Strings(want)
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("Conflicts ids = %v, want %v", got, want)
			}
		}
	})

	// Adversarial: non-matching rules MUST NOT appear in Conflicts.
	t.Run("non-matching rules excluded from conflicts (adversarial)", func(t *testing.T) {
		matchingA := baseRule
		matchingA.ID = "match-a"
		matchingA.CreatedAt = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		matchingB := baseRule
		matchingB.ID = "match-b"
		matchingB.CreatedAt = time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
		nonMatchingClassification := baseRule
		nonMatchingClassification.ID = "no-class"
		nonMatchingClassification.Classification = "recipe"
		nonMatchingClassification.CreatedAt = time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
		nonMatchingSource := baseRule
		nonMatchingSource.ID = "no-source"
		nonMatchingSource.SourceKinds = []string{string(SourceMobile)}
		nonMatchingSource.CreatedAt = time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)
		nonMatchingConfidence := baseRule
		nonMatchingConfidence.ID = "no-confidence"
		nonMatchingConfidence.ConfidenceMin = 0.99
		nonMatchingConfidence.CreatedAt = time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)

		artifact := Artifact{
			ID:             "artifact-mixed",
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.85,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact,
			[]Rule{nonMatchingSource, nonMatchingClassification, matchingA, nonMatchingConfidence, matchingB})

		if decision.Selected == nil || decision.Selected.RuleID != matchingA.ID {
			t.Fatalf("Selected = %+v, want first stable match %q", decision.Selected, matchingA.ID)
		}
		if len(decision.Conflicts) != 2 {
			t.Fatalf("Conflicts len = %d, want exactly 2 — non-matching rules MUST NOT appear", len(decision.Conflicts))
		}
		for _, c := range decision.Conflicts {
			if c.RuleID != matchingA.ID && c.RuleID != matchingB.ID {
				t.Fatalf("Conflicts contains non-matching rule %q", c.RuleID)
			}
		}
	})

	// Adversarial: a SINGLE matching rule MUST NOT populate Conflicts.
	t.Run("single match leaves Conflicts empty (adversarial)", func(t *testing.T) {
		only := baseRule
		only.ID = "rule-only"
		only.CreatedAt = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		nonMatch := baseRule
		nonMatch.ID = "rule-nomatch"
		nonMatch.Classification = "recipe"
		nonMatch.CreatedAt = time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)

		artifact := Artifact{
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.91,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact, []Rule{nonMatch, only})
		if decision.Selected == nil || decision.Selected.RuleID != only.ID {
			t.Fatalf("Selected = %+v, want %q", decision.Selected, only.ID)
		}
		if len(decision.Conflicts) != 0 {
			t.Fatalf("Conflicts len = %d, want 0 (single match — no audit conflict)", len(decision.Conflicts))
		}
	})

	// Adversarial: when CreatedAt is identical, ID order MUST break the
	// tie deterministically. A regression to map iteration would surface
	// as flaky test failures.
	t.Run("identical CreatedAt resolved by ID order (adversarial)", func(t *testing.T) {
		sameTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		ruleA := baseRule
		ruleA.ID = "aaa-rule"
		ruleA.CreatedAt = sameTime
		ruleB := baseRule
		ruleB.ID = "bbb-rule"
		ruleB.CreatedAt = sameTime
		ruleC := baseRule
		ruleC.ID = "ccc-rule"
		ruleC.CreatedAt = sameTime

		artifact := Artifact{
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.91,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		// Run multiple times in different input orders — selected MUST
		// always be the lexicographically smallest ID.
		permutations := [][]Rule{
			{ruleA, ruleB, ruleC},
			{ruleC, ruleB, ruleA},
			{ruleB, ruleC, ruleA},
		}
		for _, perm := range permutations {
			decision := engine.Evaluate(context.Background(), artifact, perm)
			if decision.Selected == nil || decision.Selected.RuleID != ruleA.ID {
				t.Fatalf("Selected.RuleID = %v, want %q (stable by ID)", decision.Selected, ruleA.ID)
			}
		}
	})
}

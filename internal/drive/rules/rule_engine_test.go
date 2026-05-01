package rules

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRuleEngineMatchesTelegramReceiptAndRendersTargetPath is the SCN-038-013
// unit anchor. It asserts the Rule Engine matches a Telegram receipt photo
// classified above the rule's confidence floor against a rule that targets
// "Receipts/{year}", renders the {year} token using the artifact's captured
// time, and selects exactly that rule. Adversarial sub-cases prove the engine
// rejects:
//   - artifacts with a different source_kind
//   - artifacts whose classification does not match the rule
//   - artifacts whose confidence falls beneath the rule's floor
//   - rules whose template references an unknown {token} (invalid token)
//
// so a future regression that flipped any of the four match clauses to
// "always pass" would surface immediately.
func TestRuleEngineMatchesTelegramReceiptAndRendersTargetPath(t *testing.T) {
	engine := NewEngine(func() time.Time { return time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC) })
	rule := Rule{
		ID:                   "rule-receipt",
		Name:                 "Telegram receipts",
		Enabled:              true,
		SourceKinds:          []string{string(SourceTelegram)},
		Classification:       "receipt",
		SensitivityIn:        []string{string(SensitivityFinancial)},
		ConfidenceMin:        0.75,
		ProviderID:           "google",
		TargetFolderTemplate: "Receipts/{year}",
		OnMissingFolder:      OnMissingCreate,
		OnExistingFile:       OnExistingVersion,
		Guardrails:           Guardrails{NeverLinkShare: true},
		CreatedAt:            time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}

	t.Run("happy path renders Receipts/2026", func(t *testing.T) {
		artifact := Artifact{
			ID:             "artifact-1",
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.92,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact, []Rule{rule})
		if decision.Selected == nil {
			t.Fatalf("Selected = nil, want rule match. Decision=%+v", decision)
		}
		if decision.Selected.RuleID != rule.ID {
			t.Fatalf("Selected.RuleID = %q, want %q", decision.Selected.RuleID, rule.ID)
		}
		if decision.Selected.RenderedPath != "Receipts/2026" {
			t.Fatalf("RenderedPath = %q, want %q", decision.Selected.RenderedPath, "Receipts/2026")
		}
		if decision.Selected.RenderError != nil {
			t.Fatalf("RenderError = %v, want nil", decision.Selected.RenderError)
		}
	})

	t.Run("source kind mismatch is rejected (adversarial)", func(t *testing.T) {
		artifact := Artifact{
			SourceKind:     string(SourceMobile),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.95,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact, []Rule{rule})
		if decision.Selected != nil {
			t.Fatalf("Selected = %+v, want nil for source mismatch", *decision.Selected)
		}
		if len(decision.Outcomes) != 1 || decision.Outcomes[0].Reason != "source_kind_mismatch" {
			t.Fatalf("Outcomes = %+v, want one source_kind_mismatch", decision.Outcomes)
		}
	})

	t.Run("classification mismatch is rejected (adversarial)", func(t *testing.T) {
		artifact := Artifact{
			SourceKind:     string(SourceTelegram),
			Classification: "recipe",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.95,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact, []Rule{rule})
		if decision.Selected != nil {
			t.Fatalf("Selected = %+v, want nil for classification mismatch", *decision.Selected)
		}
		if decision.Outcomes[0].Reason != "classification_mismatch" {
			t.Fatalf("Outcomes[0].Reason = %q, want classification_mismatch", decision.Outcomes[0].Reason)
		}
	})

	t.Run("below confidence floor is rejected (adversarial)", func(t *testing.T) {
		artifact := Artifact{
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.5,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact, []Rule{rule})
		if decision.Selected != nil {
			t.Fatalf("Selected = %+v, want nil for low confidence", *decision.Selected)
		}
		if decision.Outcomes[0].Reason != "below_confidence_min" {
			t.Fatalf("Outcomes[0].Reason = %q, want below_confidence_min", decision.Outcomes[0].Reason)
		}
	})

	t.Run("invalid template token surfaces ErrInvalidToken (adversarial)", func(t *testing.T) {
		bad := rule
		bad.ID = "rule-bad"
		bad.TargetFolderTemplate = "Receipts/{nonexistent}"
		artifact := Artifact{
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.95,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact, []Rule{bad})
		if decision.Selected == nil {
			t.Fatalf("Selected = nil, want a matched outcome (with render error)")
		}
		if !errors.Is(decision.Selected.RenderError, ErrInvalidToken) {
			t.Fatalf("RenderError = %v, want ErrInvalidToken", decision.Selected.RenderError)
		}
	})

	t.Run("conflict surfaces both matched rules (adversarial)", func(t *testing.T) {
		first := rule
		first.ID = "rule-a"
		first.CreatedAt = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		second := rule
		second.ID = "rule-b"
		second.TargetFolderTemplate = "Receipts/{year}/Doubled"
		second.CreatedAt = time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
		artifact := Artifact{
			SourceKind:     string(SourceTelegram),
			Classification: "receipt",
			Sensitivity:    string(SensitivityFinancial),
			Confidence:     0.99,
			CapturedAt:     time.Date(2026, 4, 30, 14, 12, 0, 0, time.UTC),
		}
		decision := engine.Evaluate(context.Background(), artifact, []Rule{second, first})
		if decision.Selected == nil || decision.Selected.RuleID != first.ID {
			t.Fatalf("Selected = %+v, want first stable match %q", decision.Selected, first.ID)
		}
		if len(decision.Conflicts) != 2 {
			t.Fatalf("Conflicts len = %d, want 2 (first stable + later override)", len(decision.Conflicts))
		}
	})
}

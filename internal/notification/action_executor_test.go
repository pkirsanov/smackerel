package notification

import (
	"context"
	"testing"
)

func TestActionExecutorRunsOnlyLowRiskAllowlistedAutonomousActions(t *testing.T) {
	executor := NewActionExecutor(map[string]ActionDefinition{
		"clear-cache": {Policy: ActionPolicy{Risk: RiskLow, ActionClass: ActionClassLowRisk, Destructive: false}, Run: func(ctx context.Context, input ActionInput) (ActionOutput, error) {
			return ActionOutput{Status: ActionSucceeded, ExternalEffects: map[string]string{"cache": "cleared"}}, nil
		}},
		"drop-volume": {Policy: ActionPolicy{Risk: RiskBlocked, ActionClass: ActionClassDestructive, Destructive: true}, Run: func(ctx context.Context, input ActionInput) (ActionOutput, error) {
			return ActionOutput{Status: ActionSucceeded}, nil
		}},
	})
	lowRisk, err := executor.ExecuteAutonomous(context.Background(), ActionInput{ActionKey: "clear-cache", TargetRef: "checkout-api"})
	if err != nil {
		t.Fatalf("low-risk action: %v", err)
	}
	if lowRisk.Status != ActionSucceeded || lowRisk.ExternalEffects["cache"] != "cleared" {
		t.Fatalf("unexpected low-risk action output: %+v", lowRisk)
	}
	refused, err := executor.ExecuteAutonomous(context.Background(), ActionInput{ActionKey: "drop-volume", TargetRef: "postgres"})
	if err == nil {
		t.Fatal("expected destructive action refusal")
	}
	if refused.Status != ActionRefused || refused.RefusalReason == "" {
		t.Fatalf("destructive action refusal was not durable: %+v", refused)
	}
}

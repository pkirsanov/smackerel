package notification

import "testing"

func TestHighBlastRadiusRequiresApprovalAndDestructiveActionsAreRefused(t *testing.T) {
	policy := NewApprovalPolicy()
	highBlast := policy.Evaluate(ActionPolicy{Risk: RiskHigh, ActionClass: ActionClassHighBlastRadius, Destructive: false}, "restart-shared-service")
	if highBlast.Decision != DecisionApprovalRequest || !highBlast.RequiresApproval {
		t.Fatalf("high-blast action did not require approval: %+v", highBlast)
	}
	destructive := policy.Evaluate(ActionPolicy{Risk: RiskBlocked, ActionClass: ActionClassDestructive, Destructive: true}, "delete-volume")
	if destructive.Decision != DecisionNoAction || !destructive.Refused || destructive.RefusalReason == "" {
		t.Fatalf("destructive action was not refused: %+v", destructive)
	}
}

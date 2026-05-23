package notification

import (
	"context"
	"fmt"
	"time"
)

const (
	DiagnosticSucceeded = "succeeded"
	DiagnosticRefused   = "refused"
	ActionSucceeded     = "succeeded"
	ActionRefused       = "refused"
)

type DiagnosticInput struct {
	DiagnosticKey string
	TargetRef     string
}

type DiagnosticOutput struct {
	Status         string
	OutputRedacted map[string]string
	ErrorKind      string
	ErrorRedacted  string
}

type DiagnosticDefinition struct {
	ReadOnly bool
	Timeout  time.Duration
	Run      func(context.Context, DiagnosticInput) (DiagnosticOutput, error)
}

type DiagnosticsRunner struct {
	definitions map[string]DiagnosticDefinition
}

func NewDiagnosticsRunner(definitions map[string]DiagnosticDefinition) DiagnosticsRunner {
	return DiagnosticsRunner{definitions: definitions}
}

func (r DiagnosticsRunner) Run(ctx context.Context, input DiagnosticInput) (DiagnosticOutput, error) {
	definition, ok := r.definitions[input.DiagnosticKey]
	if !ok {
		return DiagnosticOutput{Status: DiagnosticRefused, ErrorKind: "diagnostic_not_allowlisted", ErrorRedacted: "diagnostic is not allowlisted"}, fmt.Errorf("diagnostic %q is not allowlisted", input.DiagnosticKey)
	}
	if !definition.ReadOnly {
		return DiagnosticOutput{Status: DiagnosticRefused, ErrorKind: "diagnostic_not_read_only", ErrorRedacted: "diagnostic is not read-only"}, fmt.Errorf("diagnostic %q is not read-only", input.DiagnosticKey)
	}
	if definition.Run == nil {
		return DiagnosticOutput{Status: DiagnosticRefused, ErrorKind: "diagnostic_missing_runner", ErrorRedacted: "diagnostic runner missing"}, fmt.Errorf("diagnostic %q runner is required", input.DiagnosticKey)
	}
	runCtx := ctx
	cancel := func() {}
	if definition.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, definition.Timeout)
	}
	defer cancel()
	return definition.Run(runCtx, input)
}

type ActionInput struct {
	ActionKey string
	TargetRef string
}

type ActionOutput struct {
	Status          string
	ExternalEffects map[string]string
	RefusalReason   string
}

type ActionDefinition struct {
	Policy ActionPolicy
	Run    func(context.Context, ActionInput) (ActionOutput, error)
}

type ActionExecutor struct {
	definitions map[string]ActionDefinition
}

func NewActionExecutor(definitions map[string]ActionDefinition) ActionExecutor {
	return ActionExecutor{definitions: definitions}
}

func (e ActionExecutor) ExecuteAutonomous(ctx context.Context, input ActionInput) (ActionOutput, error) {
	definition, ok := e.definitions[input.ActionKey]
	if !ok {
		return ActionOutput{Status: ActionRefused, RefusalReason: "action is not allowlisted"}, fmt.Errorf("action %q is not allowlisted", input.ActionKey)
	}
	if definition.Policy.Destructive || definition.Policy.ActionClass == ActionClassDestructive || definition.Policy.Risk == RiskBlocked {
		return ActionOutput{Status: ActionRefused, RefusalReason: "destructive automatic actions are refused"}, fmt.Errorf("action %q is destructive or blocked", input.ActionKey)
	}
	if definition.Policy.ActionClass != ActionClassLowRisk || definition.Policy.Risk != RiskLow {
		return ActionOutput{Status: ActionRefused, RefusalReason: "autonomous action must be low risk"}, fmt.Errorf("action %q requires approval", input.ActionKey)
	}
	if definition.Run == nil {
		return ActionOutput{Status: ActionRefused, RefusalReason: "action runner missing"}, fmt.Errorf("action %q runner is required", input.ActionKey)
	}
	return definition.Run(ctx, input)
}

type ApprovalEvaluation struct {
	Decision         DecisionType
	RequiresApproval bool
	Refused          bool
	RefusalReason    string
}

type ApprovalPolicy struct{}

func NewApprovalPolicy() ApprovalPolicy { return ApprovalPolicy{} }

func (p ApprovalPolicy) Evaluate(policy ActionPolicy, actionKey string) ApprovalEvaluation {
	if policy.Destructive || policy.ActionClass == ActionClassDestructive || policy.Risk == RiskBlocked {
		return ApprovalEvaluation{Decision: DecisionNoAction, Refused: true, RefusalReason: "destructive automatic actions are refused"}
	}
	if policy.ActionClass == ActionClassHighBlastRadius || policy.Risk == RiskHigh {
		return ApprovalEvaluation{Decision: DecisionApprovalRequest, RequiresApproval: true}
	}
	if policy.Risk == RiskLow && policy.ActionClass == ActionClassLowRisk {
		return ApprovalEvaluation{Decision: DecisionAutonomousHandling}
	}
	return ApprovalEvaluation{Decision: DecisionDiagnostics}
}

type LoopOrigin struct {
	DecisionID    string
	OutputChannel string
	PayloadHash   string
	EmittedAt     time.Time
}

func (o LoopOrigin) Key() string {
	return hashParts(o.DecisionID, o.OutputChannel, o.PayloadHash, o.EmittedAt.UTC().Format(time.RFC3339))
}

type LoopGuard struct {
	window time.Duration
}

func NewLoopGuard(window time.Duration) LoopGuard { return LoopGuard{window: window} }

func (g LoopGuard) Evaluate(envelope SourceEventEnvelope, origins []LoopOrigin) *Suppression {
	key := firstNonEmpty(envelope.LoopMetadata["loop_guard_key"], envelope.DeliveryMetadata["loop_guard_key"])
	if key == "" {
		return nil
	}
	for _, origin := range origins {
		if key == origin.Key() && (g.window <= 0 || envelope.ObservedAt.Sub(origin.EmittedAt) <= g.window) {
			now := envelope.ObservedAt
			return &Suppression{SourceInstanceID: envelope.SourceInstanceID, Kind: SuppressionReactionLoop, Scope: map[string]any{"origin_decision_id": origin.DecisionID, "output_channel": origin.OutputChannel}, Reason: "source event matches prior handler output loop metadata", StartsAt: now, CreatedAt: now}
		}
	}
	return nil
}

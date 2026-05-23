package notification

import (
	"context"
	"testing"
	"time"
)

func TestDiagnosticsRunnerExecutesOnlyReadOnlyAllowlistedChecks(t *testing.T) {
	runner := NewDiagnosticsRunner(map[string]DiagnosticDefinition{
		"ping-service": {ReadOnly: true, Timeout: time.Second, Run: func(ctx context.Context, input DiagnosticInput) (DiagnosticOutput, error) {
			return DiagnosticOutput{Status: DiagnosticSucceeded, OutputRedacted: map[string]string{"status": "reachable"}}, nil
		}},
		"restart-service": {ReadOnly: false, Timeout: time.Second, Run: func(ctx context.Context, input DiagnosticInput) (DiagnosticOutput, error) {
			return DiagnosticOutput{Status: DiagnosticSucceeded}, nil
		}},
	})
	result, err := runner.Run(context.Background(), DiagnosticInput{DiagnosticKey: "ping-service", TargetRef: "checkout-api"})
	if err != nil {
		t.Fatalf("read-only diagnostic: %v", err)
	}
	if result.Status != DiagnosticSucceeded || result.OutputRedacted["status"] != "reachable" {
		t.Fatalf("unexpected diagnostic output: %+v", result)
	}
	refused, err := runner.Run(context.Background(), DiagnosticInput{DiagnosticKey: "restart-service", TargetRef: "checkout-api"})
	if err == nil {
		t.Fatal("expected mutation-capable diagnostic refusal")
	}
	if refused.Status != DiagnosticRefused {
		t.Fatalf("mutation-capable diagnostic was not refused: %+v", refused)
	}
}

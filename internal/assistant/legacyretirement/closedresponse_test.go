// closedresponse_test.go — spec 075 SCOPE-5 unit tests for the
// canonical unknown-command renderer and the retired-handler
// invocation guard.
package legacyretirement

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestClosedResponseFor_OK(t *testing.T) {
	decision := RetirementDecision{
		Matched:        true,
		EffectiveState: WindowClosed,
		Command: RetiredCommand{
			Command:            "/weather",
			ReplacementExample: "I do not use /weather anymore. Ask in plain English instead, for example: weather in Barcelona tomorrow",
		},
	}
	got, err := ClosedResponseFor(decision)
	if err != nil {
		t.Fatalf("ClosedResponseFor: %v", err)
	}
	if got.Status != "unavailable" {
		t.Errorf("Status=%q, want %q", got.Status, "unavailable")
	}
	if got.ErrorCause != "retired_command_closed" {
		t.Errorf("ErrorCause=%q, want %q", got.ErrorCause, "retired_command_closed")
	}
	if got.FacadeInvoked {
		t.Error("FacadeInvoked must be false for closed-window response")
	}
	if !strings.Contains(got.Body, "/weather") {
		t.Errorf("Body=%q must contain /weather token", got.Body)
	}
}

// Adversarial: a regression that loosened the closed-state guard
// (e.g., started returning the closed body for a non-closed decision)
// would let confidential paused-window content leak into the closed
// transport contract. The check MUST refuse non-closed input.
func TestClosedResponseFor_RejectsNonClosed(t *testing.T) {
	cases := map[WindowState]string{
		WindowOpen:   "open",
		WindowPaused: "paused",
		"":           "empty",
	}
	for state, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ClosedResponseFor(RetirementDecision{
				EffectiveState: state,
				Command: RetiredCommand{
					Command:            "/weather",
					ReplacementExample: "x",
				},
			})
			if err == nil {
				t.Fatalf("ClosedResponseFor must reject state=%q", state)
			}
		})
	}
}

func TestClosedResponseFor_RejectsEmptyBody(t *testing.T) {
	_, err := ClosedResponseFor(RetirementDecision{
		EffectiveState: WindowClosed,
		Command: RetiredCommand{
			Command:            "/weather",
			ReplacementExample: "   ",
		},
	})
	if err == nil {
		t.Fatal("ClosedResponseFor must reject empty body (SST coverage gap)")
	}
}

// TestRecordRetiredHandlerInvocation_Increments proves the
// structural safety hook increments the closed-state counter. This
// is the metric the observation report sources from; if it ever
// stops incrementing, the deletion gate would silently advance on a
// real regression.
func TestRecordRetiredHandlerInvocation_Increments(t *testing.T) {
	before := readCounterTotal(t, MetricNameRetiredHandlerInvocation)
	RecordRetiredHandlerInvocation("/spec075-test-token")
	after := readCounterTotal(t, MetricNameRetiredHandlerInvocation)
	if after-before < 1 {
		t.Fatalf("expected counter to advance by at least 1, before=%f after=%f", before, after)
	}
}

func TestRecordRetiredHandlerInvocation_EmptyCommandStillRecords(t *testing.T) {
	before := readCounterTotal(t, MetricNameRetiredHandlerInvocation)
	RecordRetiredHandlerInvocation("")
	after := readCounterTotal(t, MetricNameRetiredHandlerInvocation)
	if after <= before {
		t.Fatalf("expected sentinel-labeled increment, before=%f after=%f", before, after)
	}
}

func readCounterTotal(t *testing.T, name string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var total float64
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if c := m.GetCounter(); c != nil {
				total += c.GetValue()
			}
		}
	}
	return total
}

var _ = (*dto.Metric)(nil)

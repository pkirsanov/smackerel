// Spec 068 SCOPE-3 — Side-effect gate unit tests (SCN-068-A09).

package intent

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

// TestSideEffectGateBlocksExternalWriteWithoutConfirmation pins the
// SCN-068-A09 contract: external_write MUST require confirmation.
// The adversarial sub-tests prove the gate would catch a regression
// to a "pass-through" implementation by also asserting that read
// and external_read side-effect classes are NOT gated.
func TestSideEffectGateBlocksExternalWriteWithoutConfirmation(t *testing.T) {
	// Primary assertion: external_write is gated.
	intent := CompiledIntent{SideEffectClass: SideEffectExternalWrite}
	if !RequiresConfirmation(intent) {
		t.Fatalf("RequiresConfirmation(external_write) = false, want true (SCN-068-A09)")
	}

	// Internal write is also gated — SCN-068-A03 list-write path.
	if !RequiresConfirmation(CompiledIntent{SideEffectClass: SideEffectWrite}) {
		t.Fatalf("RequiresConfirmation(write) = false, want true (SCN-068-A03)")
	}

	// Adversarial cases: every non-mutating class MUST pass through.
	// Without these, a regression that always returns true (or
	// always returns false) would only fail the primary assertion in
	// one direction.
	for _, sec := range []SideEffectClass{SideEffectNone, SideEffectRead, SideEffectExternalRead} {
		if RequiresConfirmation(CompiledIntent{SideEffectClass: sec}) {
			t.Errorf("RequiresConfirmation(%q) = true, want false (must not gate non-mutating turns)", sec)
		}
	}

	// SideEffectBlockedTotal counter is registered and labels are
	// accepted (proves the wiring used by facade.go does not panic
	// at runtime).
	SideEffectBlockedTotal.WithLabelValues(string(SideEffectExternalWrite), "missing_confirmation").Inc()
	got := readCounter(t, string(SideEffectExternalWrite), "missing_confirmation")
	if got < 1 {
		t.Fatalf("SideEffectBlockedTotal not incremented; got %v", got)
	}
}

func readCounter(t *testing.T, sec, cause string) float64 {
	t.Helper()
	m, err := SideEffectBlockedTotal.GetMetricWithLabelValues(sec, cause)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	var pb dto.Metric
	if err := m.Write(&pb); err != nil {
		t.Fatalf("metric.Write: %v", err)
	}
	if pb.Counter == nil {
		return 0
	}
	return pb.Counter.GetValue()
}

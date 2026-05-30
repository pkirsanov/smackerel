// Spec 061 SCOPE-06c (Round 71d) — adversarial unit tests for the
// hardware-tier validator. Each case maps to SCOPE-06c DoD #1 (build clean
// with named fail-loud guards).
package config

import (
	"strings"
	"testing"
)

func TestValidateHardwareTier_AcceptsCPU(t *testing.T) {
	got, err := ValidateHardwareTier("cpu")
	if err != nil {
		t.Fatalf("expected cpu to validate, got error: %v", err)
	}
	if got != HardwareTierCPU {
		t.Fatalf("expected HardwareTierCPU, got %q", got)
	}
}

func TestValidateHardwareTier_AcceptsAccel(t *testing.T) {
	got, err := ValidateHardwareTier("accel")
	if err != nil {
		t.Fatalf("expected accel to validate, got error: %v", err)
	}
	if got != HardwareTierAccel {
		t.Fatalf("expected HardwareTierAccel, got %q", got)
	}
}

// SCOPE-06c DoD #1 adversarial — missing tier must fail loud with the named error.
func TestValidateHardwareTier_RejectsEmpty(t *testing.T) {
	_, err := ValidateHardwareTier("")
	if err == nil {
		t.Fatal("expected empty tier to fail validation")
	}
	if !strings.Contains(err.Error(), "[F061-HARDWARE-TIER-MISSING]") {
		t.Fatalf("expected [F061-HARDWARE-TIER-MISSING] in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "SMACKEREL_HARDWARE_TIER") {
		t.Fatalf("expected env var name in error message, got: %v", err)
	}
}

// SCOPE-06c DoD #1 adversarial — invalid tier value must fail loud and name the offending value.
func TestValidateHardwareTier_RejectsInvalid(t *testing.T) {
	cases := []string{"gpu", "CPU", "Accel", " cpu", "cpu ", "auto", "tpu"}
	for _, c := range cases {
		_, err := ValidateHardwareTier(c)
		if err == nil {
			t.Fatalf("expected %q to be rejected", c)
		}
		if !strings.Contains(err.Error(), "[F061-HARDWARE-TIER-INVALID]") {
			t.Fatalf("expected [F061-HARDWARE-TIER-INVALID] in error for %q, got: %v", c, err)
		}
	}
}

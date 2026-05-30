// Package config — Spec 061 SCOPE-06c (Round 71d) hardware-tier validator.
//
// The SMACKEREL_HARDWARE_TIER switch is resolved at config-generate time in
// scripts/commands/config.sh and emitted to config/generated/<env>.env. This
// validator is defense-in-depth: it asserts the env var the runtime sees is
// present and in {cpu, accel} so a malformed env file fails loud at startup
// rather than letting a downstream model lookup silently misroute. Cites
// SCOPE-06c DoD #1 (build clean + adversarial unit tests).
package config

import (
	"fmt"
	"os"
)

// HardwareTier is the validated value of SMACKEREL_HARDWARE_TIER.
type HardwareTier string

const (
	HardwareTierCPU   HardwareTier = "cpu"
	HardwareTierAccel HardwareTier = "accel"
)

// ValidateHardwareTier returns the parsed tier or a fail-loud error naming
// the offending key. The tier is read from the SMACKEREL_HARDWARE_TIER env
// var (emitted by `./smackerel.sh config generate`).
func ValidateHardwareTier(raw string) (HardwareTier, error) {
	if raw == "" {
		return "", fmt.Errorf("[F061-HARDWARE-TIER-MISSING] SMACKEREL_HARDWARE_TIER is required (config/generated/<env>.env should set it via scripts/commands/config.sh from .smackerel.local.env)")
	}
	switch HardwareTier(raw) {
	case HardwareTierCPU, HardwareTierAccel:
		return HardwareTier(raw), nil
	default:
		return "", fmt.Errorf("[F061-HARDWARE-TIER-INVALID] SMACKEREL_HARDWARE_TIER must be 'cpu' or 'accel', got: %q", raw)
	}
}

// LoadHardwareTier reads SMACKEREL_HARDWARE_TIER from the environment and
// validates it.
func LoadHardwareTier() (HardwareTier, error) {
	return ValidateHardwareTier(os.Getenv("SMACKEREL_HARDWARE_TIER"))
}

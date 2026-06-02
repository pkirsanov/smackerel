package citeback

import (
	"fmt"
	"strings"
)

// EnforcementMode controls whether a failing VerifyResult flips the
// agent loop to refusal-with-capture (`EnforcementEnforce`) or only
// logs the mismatch and lets the response through unchanged
// (`EnforcementShadow`). Spec 076 SCOPE-2c wires this behind the
// `openknowledge.citeback.enforcement_mode` SST key.
type EnforcementMode string

const (
	EnforcementShadow  EnforcementMode = "shadow"
	EnforcementEnforce EnforcementMode = "enforce"
)

// ParseEnforcementMode validates the raw SST string and returns the
// typed mode. Unknown / empty values fail loud (G028).
func ParseEnforcementMode(s string) (EnforcementMode, error) {
	switch strings.TrimSpace(s) {
	case string(EnforcementShadow):
		return EnforcementShadow, nil
	case string(EnforcementEnforce):
		return EnforcementEnforce, nil
	default:
		return "", fmt.Errorf("citeback: enforcement_mode must be %q or %q, got %q",
			EnforcementShadow, EnforcementEnforce, s)
	}
}

// Decision is the agent-loop-facing verdict applied through the
// configured EnforcementMode.
type Decision struct {
	Mode     EnforcementMode
	Verdict  VerifyResult
	Refuse   bool
	Mismatch bool
}

// Decide applies the enforcement mode to a raw VerifyResult. In
// shadow mode a failing verdict produces Mismatch=true / Refuse=false,
// so callers can log without flipping the user-facing response. In
// enforce mode a failing verdict produces Refuse=true.
func Decide(verdict VerifyResult, mode EnforcementMode) Decision {
	d := Decision{Mode: mode, Verdict: verdict}
	if verdict.OK {
		return d
	}
	d.Mismatch = true
	if mode == EnforcementEnforce {
		d.Refuse = true
	}
	return d
}

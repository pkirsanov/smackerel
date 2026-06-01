// observation.go — spec 075 SCOPE-1 observation report seam.
package legacyretirement

import (
	"context"
	"time"
)

// ObservationSnapshot is the row persisted into
// assistant_legacy_retirement_observations by Scope 5. It is the only
// artifact the deletion gate consults: a snapshot with
// RetiredHandlerInvocations==0 spanning at least
// legacy_retirement.post_window_observation_days proves the
// retired handlers can be safely removed by spec 066.
type ObservationSnapshot struct {
	ReportID                  string
	WindowID                  string
	ObservationStartedAt      time.Time
	ObservationEndedAt        time.Time
	RetiredHandlerInvocations int
	GeneratedAt               time.Time
	SchemaVersion             int
}

// EligibleForFinalDeletion returns true iff the snapshot satisfies
// the deletion gate: zero retired-handler invocations over an
// interval of at least minDuration. Used by Scope 5 and by the
// observation report CLI; lives in Scope 1 because it is a pure
// invariant the foundation owns.
func (s ObservationSnapshot) EligibleForFinalDeletion(minDuration time.Duration) bool {
	if s.RetiredHandlerInvocations != 0 {
		return false
	}
	if s.ObservationEndedAt.Before(s.ObservationStartedAt) {
		return false
	}
	return s.ObservationEndedAt.Sub(s.ObservationStartedAt) >= minDuration
}

// ObservationReport is the producer seam Scope 5 wires. Scope 1
// declares only the contract.
type ObservationReport interface {
	// Generate computes a fresh observation snapshot for windowID
	// covering [now-observationDuration, now]. Implementations
	// MUST count retired-handler invocations from the
	// RetiredHandlerInvocationCounter metric family (or its
	// persisted equivalent) and MUST persist the snapshot to
	// assistant_legacy_retirement_observations so the deletion gate
	// can be audited.
	Generate(ctx context.Context, windowID string, observationDuration time.Duration) (ObservationSnapshot, error)
}

package qfdecisions

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxCredentialRotationOverlap = 24 * time.Hour

type RotatingCredential struct {
	Ref       string
	NotBefore time.Time
	NotAfter  time.Time
}

type CredentialRotationState struct {
	SyncCursor             string
	CapabilityResponseJSON string
	CapabilityFetchedAt    time.Time
	CapabilityStatus       string
	EvidenceExportIDs      []string
}

type CredentialRotationPlan struct {
	SelectedCredentialRef    string
	PreviousCredentialRef    string
	OverlapSeconds           int64
	CapabilityReReadRequired bool
	PreservedState           CredentialRotationState
	Diagnostics              []string
	AuditEnvelope            EvidenceAuditEnvelope
}

func PlanCredentialRotation(credentials []RotatingCredential, state CredentialRotationState, now time.Time) (CredentialRotationPlan, error) {
	observedAt := now.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if len(credentials) != 2 {
		return credentialRotationRejected("expected_exactly_two_active_credentials", state, observedAt), fmt.Errorf("credential rotation requires exactly two active QF credentials, got %d", len(credentials))
	}
	active := append([]RotatingCredential(nil), credentials...)
	for i, credential := range active {
		active[i].Ref = strings.TrimSpace(credential.Ref)
		if active[i].Ref == "" {
			return credentialRotationRejected("credential_ref_required", state, observedAt), fmt.Errorf("credential rotation credential_ref is required")
		}
		if active[i].NotBefore.IsZero() || active[i].NotAfter.IsZero() {
			return credentialRotationRejected("credential_window_required", state, observedAt), fmt.Errorf("credential rotation requires not_before and not_after for %s", active[i].Ref)
		}
		active[i].NotBefore = active[i].NotBefore.UTC()
		active[i].NotAfter = active[i].NotAfter.UTC()
		if !active[i].NotAfter.After(active[i].NotBefore) {
			return credentialRotationRejected("credential_window_inverted", state, observedAt), fmt.Errorf("credential rotation window is inverted for %s", active[i].Ref)
		}
		if observedAt.Before(active[i].NotBefore) || !observedAt.Before(active[i].NotAfter) {
			return credentialRotationRejected("credential_not_active", state, observedAt), fmt.Errorf("credential %s is not active at rotation time", active[i].Ref)
		}
	}
	overlapStart := latestTime(active[0].NotBefore, active[1].NotBefore)
	overlapEnd := earliestTime(active[0].NotAfter, active[1].NotAfter)
	if !overlapEnd.After(overlapStart) {
		return credentialRotationRejected("credential_windows_do_not_overlap", state, observedAt), fmt.Errorf("credential rotation windows do not overlap")
	}
	overlap := overlapEnd.Sub(overlapStart)
	if overlap > maxCredentialRotationOverlap {
		return credentialRotationRejected("credential_overlap_exceeds_24h", state, observedAt), fmt.Errorf("credential rotation overlap %s exceeds %s", overlap, maxCredentialRotationOverlap)
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].NotBefore.After(active[j].NotBefore) })
	plan := CredentialRotationPlan{
		SelectedCredentialRef:    active[0].Ref,
		PreviousCredentialRef:    active[1].Ref,
		OverlapSeconds:           int64(overlap.Seconds()),
		CapabilityReReadRequired: true,
		PreservedState:           cloneCredentialRotationState(state),
		Diagnostics:              []string{"capability_re_read_required", "sync_cursor_preserved", "evidence_export_state_preserved"},
		AuditEnvelope: BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    DefaultConnectorID,
			Action:     AuditActionCredentialRotation,
			Outcome:    AuditOutcomeOK,
			ObservedAt: observedAt,
		}),
	}
	EmitConnectorAuditEnvelope(plan.AuditEnvelope)
	return plan, nil
}

func credentialRotationRejected(reason string, state CredentialRotationState, observedAt time.Time) CredentialRotationPlan {
	envelope := BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		ActorRef:   AuditActorSmackerelConnector,
		Surface:    DefaultConnectorID,
		Action:     AuditActionCredentialRotation,
		Outcome:    AuditOutcomeRejected,
		Reason:     reason,
		ObservedAt: observedAt,
	})
	EmitConnectorAuditEnvelope(envelope)
	return CredentialRotationPlan{
		CapabilityReReadRequired: true,
		PreservedState:           cloneCredentialRotationState(state),
		Diagnostics:              []string{reason},
		AuditEnvelope:            envelope,
	}
}

func cloneCredentialRotationState(state CredentialRotationState) CredentialRotationState {
	state.EvidenceExportIDs = append([]string(nil), state.EvidenceExportIDs...)
	return state
}

func latestTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earliestTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

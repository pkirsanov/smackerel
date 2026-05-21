package qfdecisions

import (
	"strings"
	"testing"
	"time"
)

func TestPlanCredentialRotationSelectsNewestValidCredentialAndPreservesState(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	state := CredentialRotationState{
		SyncCursor:             "cursor-before-rotation",
		CapabilityResponseJSON: `{"audit_envelope_version":"v1"}`,
		CapabilityFetchedAt:    now.Add(-time.Hour),
		CapabilityStatus:       CapabilityStatusCompatible,
		EvidenceExportIDs:      []string{"export-before-rotation"},
	}
	plan, err := PlanCredentialRotation([]RotatingCredential{
		{Ref: "qf-old", NotBefore: now.Add(-20 * time.Hour), NotAfter: now.Add(2 * time.Hour)},
		{Ref: "qf-new", NotBefore: now.Add(-time.Hour), NotAfter: now.Add(20 * time.Hour)},
	}, state, now)
	if err != nil {
		t.Fatalf("PlanCredentialRotation returned error: %v", err)
	}
	if plan.SelectedCredentialRef != "qf-new" || plan.PreviousCredentialRef != "qf-old" {
		t.Fatalf("rotation credential selection = %+v, want newest qf-new over qf-old", plan)
	}
	if plan.OverlapSeconds != int64(3*time.Hour/time.Second) {
		t.Fatalf("OverlapSeconds = %d, want %d", plan.OverlapSeconds, int64(3*time.Hour/time.Second))
	}
	if !plan.CapabilityReReadRequired {
		t.Fatal("rotation plan must require capability re-read at rotation start")
	}
	if plan.PreservedState.SyncCursor != state.SyncCursor || plan.PreservedState.EvidenceExportIDs[0] != state.EvidenceExportIDs[0] {
		t.Fatalf("rotation did not preserve cursor/evidence state: %+v", plan.PreservedState)
	}
	plan.PreservedState.EvidenceExportIDs[0] = "mutated"
	if state.EvidenceExportIDs[0] != "export-before-rotation" {
		t.Fatal("rotation state copy aliased evidence export idempotency slice")
	}
	if plan.AuditEnvelope.Action != AuditActionCredentialRotation || plan.AuditEnvelope.Outcome != AuditOutcomeOK || plan.AuditEnvelope.AuditEnvelopeVersion != AuditEnvelopeVersionV1 {
		t.Fatalf("rotation audit envelope = %+v", plan.AuditEnvelope)
	}
}

func TestPlanCredentialRotationRejectsInvalidCredentialBoundaries(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	state := CredentialRotationState{SyncCursor: "cursor-stays"}
	cases := []struct {
		name        string
		credentials []RotatingCredential
		wantReason  string
	}{
		{
			name:        "one active credential",
			credentials: []RotatingCredential{{Ref: "qf-only", NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}},
			wantReason:  "expected_exactly_two_active_credentials",
		},
		{
			name: "overlap longer than 24 hours",
			credentials: []RotatingCredential{
				{Ref: "qf-old", NotBefore: now.Add(-30 * time.Hour), NotAfter: now.Add(30 * time.Hour)},
				{Ref: "qf-new", NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * time.Hour)},
			},
			wantReason: "credential_overlap_exceeds_24h",
		},
		{
			name: "future credential is not active",
			credentials: []RotatingCredential{
				{Ref: "qf-old", NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)},
				{Ref: "qf-new", NotBefore: now.Add(time.Minute), NotAfter: now.Add(time.Hour)},
			},
			wantReason: "credential_not_active",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanCredentialRotation(tc.credentials, state, now)
			if err == nil {
				t.Fatal("expected rotation boundary error")
			}
			if !strings.Contains(strings.Join(plan.Diagnostics, ","), tc.wantReason) {
				t.Fatalf("diagnostics = %v, want %q", plan.Diagnostics, tc.wantReason)
			}
			if plan.AuditEnvelope.Outcome != AuditOutcomeRejected || plan.AuditEnvelope.Reason != tc.wantReason {
				t.Fatalf("audit envelope = %+v, want rejected reason %q", plan.AuditEnvelope, tc.wantReason)
			}
			if plan.PreservedState.SyncCursor != state.SyncCursor {
				t.Fatalf("rejected rotation changed cursor: %+v", plan.PreservedState)
			}
		})
	}
}

package qfdecisions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// validCapability returns a capability response that mirrors the verbatim
// 21-field schema from
// ~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md (F2).
// Tests mutate a single field at a time to exercise CompatibilityCheck().
func validCapability() QFBridgeCapability {
	return QFBridgeCapability{
		SupportedPacketVersions:            []string{"v1"},
		SupportedEventTypes:                []string{"created", "updated", "badge_changed", "approval_state_changed", "archived", "superseded"},
		SupportedDecisionTypes:             []string{"recommendation", "no_action", "policy_denial", "analysis_note"},
		MaxPageSize:                        200,
		MinPageSize:                        1,
		SupportedTargetContextTypes:        []string{"guided_analysis", "rhai_run", "saved_result", "analysis_context", "packet_context"},
		EvidenceMaxBundleSizeBytes:         524288,
		EvidenceMaxClaimsPerBundle:         50,
		EvidenceRateLimitPerMinute:         10,
		FreshnessSLAP95Seconds:             60,
		AuditEnvelopeVersion:               "v1",
		TenantAware:                        false,
		PreferredSurfaceHintSupported:      true,
		EngagementSignalSupported:          true,
		PersonalContextPullSupported:       true,
		WatchSignalDirection:               "qf_emit_only_pre_mvp",
		CallbackSigningSupported:           false,
		DeepLinkSigningSupported:           true,
		CredentialRotationOverlapSupported: true,
		NoActionEmitEnabled:                false,
		EligibleSmackerelSourceClasses:     []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"},
	}
}

func TestQFBridgeCapability_CompatibilityCheck_Compatible(t *testing.T) {
	if err := validCapability().CompatibilityCheck(); err != nil {
		t.Fatalf("expected compatible capability to pass, got: %v", err)
	}
}

func TestQFBridgeCapability_CompatibilityCheck_RejectsAuditEnvelopeMismatch(t *testing.T) {
	cap := validCapability()
	cap.AuditEnvelopeVersion = "v2"
	err := cap.CompatibilityCheck()
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	var mismatch CapabilityMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected CapabilityMismatchError, got %T: %v", err, err)
	}
	if mismatch.Field != "audit_envelope_version" {
		t.Fatalf("Field = %q, want audit_envelope_version", mismatch.Field)
	}
	if mismatch.Required != "v1" || mismatch.Actual != "v2" {
		t.Fatalf("Required/Actual = %q/%q, want v1/v2", mismatch.Required, mismatch.Actual)
	}
}

// TestCapabilityMismatchDetectsRequiredPacketVersion (SCN-SM-041-004) proves
// that CompatibilityCheck() rejects a capability response whose
// supported_packet_versions does NOT contain "v1" (the only packet version
// the connector build consumes). Renamed from
// TestQFBridgeCapability_CompatibilityCheck_RejectsMissingPacketVersion in
// Round 2K to align with the scopes.md Test Plan declared name; behavior
// and assertions are unchanged.
func TestCapabilityMismatchDetectsRequiredPacketVersion(t *testing.T) {
	cap := validCapability()
	cap.SupportedPacketVersions = []string{"v2"}
	err := cap.CompatibilityCheck()
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	var mismatch CapabilityMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected CapabilityMismatchError, got %T: %v", err, err)
	}
	if mismatch.Field != "supported_packet_versions" {
		t.Fatalf("Field = %q, want supported_packet_versions", mismatch.Field)
	}
	if mismatch.Required != "v1" {
		t.Fatalf("Required = %q, want v1", mismatch.Required)
	}
	if !strings.Contains(mismatch.Actual, "v2") {
		t.Fatalf("Actual = %q, want to contain v2", mismatch.Actual)
	}
}

func TestQFBridgeCapability_CompatibilityCheck_RejectsMissingDecisionType(t *testing.T) {
	cap := validCapability()
	// Drop "recommendation" — the first required type checked.
	cap.SupportedDecisionTypes = []string{"no_action", "policy_denial", "analysis_note"}
	err := cap.CompatibilityCheck()
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	var mismatch CapabilityMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected CapabilityMismatchError, got %T: %v", err, err)
	}
	if mismatch.Field != "supported_decision_types" {
		t.Fatalf("Field = %q, want supported_decision_types", mismatch.Field)
	}
	if mismatch.Required != "recommendation" {
		t.Fatalf("Required = %q, want recommendation", mismatch.Required)
	}
}

func TestQFBridgeCapability_CompatibilityCheck_AcceptsAbsentNoActionType(t *testing.T) {
	cap := validCapability()
	// no_action is capability-gated by no_action_emit_enabled and is NOT a hard requirement.
	cap.SupportedDecisionTypes = []string{"recommendation", "policy_denial", "analysis_note"}
	cap.NoActionEmitEnabled = false
	if err := cap.CompatibilityCheck(); err != nil {
		t.Fatalf("absence of no_action MUST be tolerated, got: %v", err)
	}
}

func TestQFBridgeCapability_CompatibilityCheck_RejectsInvalidMaxPageSize(t *testing.T) {
	cap := validCapability()
	cap.MaxPageSize = 0
	err := cap.CompatibilityCheck()
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	var mismatch CapabilityMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected CapabilityMismatchError, got %T: %v", err, err)
	}
	if mismatch.Field != "max_page_size" {
		t.Fatalf("Field = %q, want max_page_size", mismatch.Field)
	}
	if mismatch.Required != ">=1" {
		t.Fatalf("Required = %q, want >=1", mismatch.Required)
	}
	if mismatch.Actual != "0" {
		t.Fatalf("Actual = %q, want 0", mismatch.Actual)
	}
}

// TestParseCapabilityResponseFields (SCN-SM-041-003) proves that the QF
// capability JSON response decodes into the QFBridgeCapability struct with
// EVERY one of the 21 fields enumerated in
// ~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md
// (F2 §"GET /api/private/smackerel/v1/capabilities") preserved verbatim.
// Added in Round 2K to satisfy the scopes.md Test Plan declared name. The
// existing TestClient_FetchCapability_Success covers transport + auth-header
// concerns and rounds-trips a representative subset; this test focuses
// exclusively on the parse-fidelity contract by encoding the canonical
// validCapability() value and asserting every field round-trips byte-for-byte.
func TestParseCapabilityResponseFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(validCapability())
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 50)
	got, err := client.FetchCapability(context.Background())
	if err != nil {
		t.Fatalf("FetchCapability failed: %v", err)
	}
	want := validCapability()

	// --- 21-field exhaustive parse check (matches QFBridgeCapability struct
	// declaration in capability.go). Each field is asserted explicitly so a
	// future schema addition that bypasses the decoder is caught by this
	// test rather than slipping through as a silent zero-value. ---
	if !stringSlicesEqual(got.SupportedPacketVersions, want.SupportedPacketVersions) {
		t.Errorf("SupportedPacketVersions = %v, want %v", got.SupportedPacketVersions, want.SupportedPacketVersions)
	}
	if !stringSlicesEqual(got.SupportedEventTypes, want.SupportedEventTypes) {
		t.Errorf("SupportedEventTypes = %v, want %v", got.SupportedEventTypes, want.SupportedEventTypes)
	}
	if !stringSlicesEqual(got.SupportedDecisionTypes, want.SupportedDecisionTypes) {
		t.Errorf("SupportedDecisionTypes = %v, want %v", got.SupportedDecisionTypes, want.SupportedDecisionTypes)
	}
	if got.MaxPageSize != want.MaxPageSize {
		t.Errorf("MaxPageSize = %d, want %d", got.MaxPageSize, want.MaxPageSize)
	}
	if got.MinPageSize != want.MinPageSize {
		t.Errorf("MinPageSize = %d, want %d", got.MinPageSize, want.MinPageSize)
	}
	if !stringSlicesEqual(got.SupportedTargetContextTypes, want.SupportedTargetContextTypes) {
		t.Errorf("SupportedTargetContextTypes = %v, want %v", got.SupportedTargetContextTypes, want.SupportedTargetContextTypes)
	}
	if got.EvidenceMaxBundleSizeBytes != want.EvidenceMaxBundleSizeBytes {
		t.Errorf("EvidenceMaxBundleSizeBytes = %d, want %d", got.EvidenceMaxBundleSizeBytes, want.EvidenceMaxBundleSizeBytes)
	}
	if got.EvidenceMaxClaimsPerBundle != want.EvidenceMaxClaimsPerBundle {
		t.Errorf("EvidenceMaxClaimsPerBundle = %d, want %d", got.EvidenceMaxClaimsPerBundle, want.EvidenceMaxClaimsPerBundle)
	}
	if got.EvidenceRateLimitPerMinute != want.EvidenceRateLimitPerMinute {
		t.Errorf("EvidenceRateLimitPerMinute = %d, want %d", got.EvidenceRateLimitPerMinute, want.EvidenceRateLimitPerMinute)
	}
	if got.FreshnessSLAP95Seconds != want.FreshnessSLAP95Seconds {
		t.Errorf("FreshnessSLAP95Seconds = %d, want %d", got.FreshnessSLAP95Seconds, want.FreshnessSLAP95Seconds)
	}
	if got.AuditEnvelopeVersion != want.AuditEnvelopeVersion {
		t.Errorf("AuditEnvelopeVersion = %q, want %q", got.AuditEnvelopeVersion, want.AuditEnvelopeVersion)
	}
	if got.TenantAware != want.TenantAware {
		t.Errorf("TenantAware = %v, want %v", got.TenantAware, want.TenantAware)
	}
	if got.PreferredSurfaceHintSupported != want.PreferredSurfaceHintSupported {
		t.Errorf("PreferredSurfaceHintSupported = %v, want %v", got.PreferredSurfaceHintSupported, want.PreferredSurfaceHintSupported)
	}
	if got.EngagementSignalSupported != want.EngagementSignalSupported {
		t.Errorf("EngagementSignalSupported = %v, want %v", got.EngagementSignalSupported, want.EngagementSignalSupported)
	}
	if got.PersonalContextPullSupported != want.PersonalContextPullSupported {
		t.Errorf("PersonalContextPullSupported = %v, want %v", got.PersonalContextPullSupported, want.PersonalContextPullSupported)
	}
	if got.WatchSignalDirection != want.WatchSignalDirection {
		t.Errorf("WatchSignalDirection = %q, want %q", got.WatchSignalDirection, want.WatchSignalDirection)
	}
	if got.CallbackSigningSupported != want.CallbackSigningSupported {
		t.Errorf("CallbackSigningSupported = %v, want %v", got.CallbackSigningSupported, want.CallbackSigningSupported)
	}
	if got.DeepLinkSigningSupported != want.DeepLinkSigningSupported {
		t.Errorf("DeepLinkSigningSupported = %v, want %v", got.DeepLinkSigningSupported, want.DeepLinkSigningSupported)
	}
	if got.CredentialRotationOverlapSupported != want.CredentialRotationOverlapSupported {
		t.Errorf("CredentialRotationOverlapSupported = %v, want %v", got.CredentialRotationOverlapSupported, want.CredentialRotationOverlapSupported)
	}
	if got.NoActionEmitEnabled != want.NoActionEmitEnabled {
		t.Errorf("NoActionEmitEnabled = %v, want %v", got.NoActionEmitEnabled, want.NoActionEmitEnabled)
	}
	if !stringSlicesEqual(got.EligibleSmackerelSourceClasses, want.EligibleSmackerelSourceClasses) {
		t.Errorf("EligibleSmackerelSourceClasses = %v, want %v", got.EligibleSmackerelSourceClasses, want.EligibleSmackerelSourceClasses)
	}
}

// stringSlicesEqual reports whether two string slices contain the same
// elements in the same order. Used by TestParseCapabilityResponseFields
// for slice-typed fields.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestClient_FetchCapability_Success(t *testing.T) {
	var gotAuth string
	var gotAccept string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(validCapability())
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 50)
	cap, err := client.FetchCapability(context.Background())
	if err != nil {
		t.Fatalf("FetchCapability failed: %v", err)
	}
	if gotAuth != "Bearer qf-service-token" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept header = %q", gotAccept)
	}
	if gotPath != CapabilitiesPath {
		t.Fatalf("path = %q, want %q", gotPath, CapabilitiesPath)
	}
	// Round-trip a representative subset of the 21 fields.
	if cap.AuditEnvelopeVersion != "v1" {
		t.Fatalf("AuditEnvelopeVersion = %q", cap.AuditEnvelopeVersion)
	}
	if len(cap.SupportedPacketVersions) != 1 || cap.SupportedPacketVersions[0] != "v1" {
		t.Fatalf("SupportedPacketVersions = %v", cap.SupportedPacketVersions)
	}
	if cap.MaxPageSize != 200 {
		t.Fatalf("MaxPageSize = %d", cap.MaxPageSize)
	}
	if cap.MinPageSize != 1 {
		t.Fatalf("MinPageSize = %d", cap.MinPageSize)
	}
	if cap.EvidenceMaxBundleSizeBytes != 524288 {
		t.Fatalf("EvidenceMaxBundleSizeBytes = %d", cap.EvidenceMaxBundleSizeBytes)
	}
	if cap.WatchSignalDirection != "qf_emit_only_pre_mvp" {
		t.Fatalf("WatchSignalDirection = %q", cap.WatchSignalDirection)
	}
	if !cap.DeepLinkSigningSupported {
		t.Fatalf("DeepLinkSigningSupported = false, want true")
	}
	if cap.NoActionEmitEnabled {
		t.Fatalf("NoActionEmitEnabled = true, want false")
	}
	if len(cap.EligibleSmackerelSourceClasses) != 6 {
		t.Fatalf("EligibleSmackerelSourceClasses count = %d, want 6", len(cap.EligibleSmackerelSourceClasses))
	}
}

func TestClient_FetchCapability_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(BridgeErrorResponse{Code: "unauthorized", Message: "invalid bearer"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "expired-token", 1, 50)
	_, err := client.FetchCapability(context.Background())
	if err == nil {
		t.Fatal("expected AuthError, got nil")
	}
	var authErr AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %T: %v", err, err)
	}
	if authErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d, want 401", authErr.StatusCode)
	}
}

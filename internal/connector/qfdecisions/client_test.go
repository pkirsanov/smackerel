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

func TestClientValidateUsesQFPrivateReadContract(t *testing.T) {
	var gotAuth string
	var gotAccept string
	var gotPath string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DecisionEventsResponse{
			Events:     []QFDecisionEvent{},
			NextCursor: "qf-smackerel-v1:0",
			HasMore:    false,
			ServerTime: "2026-05-06T00:00:00Z",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL+"/", "qf-service-token", 1, 50)
	client.SetCapability(&QFBridgeCapability{MinPageSize: 1, MaxPageSize: 200}, CapabilityStatusCompatible)
	if err := client.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if gotAuth != "Bearer qf-service-token" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept header = %q", gotAccept)
	}
	if gotPath != DecisionEventsPath {
		t.Fatalf("path = %q, want %q", gotPath, DecisionEventsPath)
	}
	if !strings.Contains(gotQuery, "limit=50") || !strings.Contains(gotQuery, "packet_version=1") {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestClientRejectsIncompatibleQFPacketVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BridgeErrorResponse{Code: "invalid_query_parameter", Message: "packet_version 99 is unsupported"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 99, 50)
	client.SetCapability(&QFBridgeCapability{MinPageSize: 1, MaxPageSize: 200}, CapabilityStatusCompatible)
	err := client.Validate(context.Background())
	if err == nil {
		t.Fatal("expected schema compatibility error")
	}
	if !strings.Contains(err.Error(), "packet_version") || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientFetchDecisionEventsPassesOpaqueCursor(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DecisionEventsResponse{
			Events: []QFDecisionEvent{
				{EventID: "event-1", PacketID: "packet-1", PacketVersion: 1, Cursor: "qf-smackerel-v1:1"},
			},
			NextCursor: "qf-smackerel-v1:1",
			HasMore:    false,
			ServerTime: "2026-05-06T00:00:00Z",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 25)
	client.SetCapability(&QFBridgeCapability{MinPageSize: 1, MaxPageSize: 200}, CapabilityStatusCompatible)
	resp, err := client.FetchDecisionEvents(context.Background(), "qf-smackerel-v1:9")
	if err != nil {
		t.Fatalf("FetchDecisionEvents failed: %v", err)
	}
	if resp.NextCursor != "qf-smackerel-v1:1" {
		t.Fatalf("NextCursor = %q", resp.NextCursor)
	}
	if !strings.Contains(gotQuery, "cursor=qf-smackerel-v1%3A9") {
		t.Fatalf("query did not contain escaped cursor: %q", gotQuery)
	}
}

func TestClientFetchDecisionPacketUsesPacketPathAndVersion(t *testing.T) {
	var gotPath string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
			ContractVersion:      1,
			PacketID:             "packet-123",
			IntentID:             "intent-1",
			ScenarioID:           "scenario-1",
			TraceID:              "trace-1",
			Thesis:               "QF-authored thesis",
			WhyNow:               "QF-authored timing",
			QuantifiedImpact:     map[string]any{"unit": "bps"},
			ExpertAnalysisBundle: map[string]any{"ref": "qf-analysis"},
			CalibrationBadge:     map[string]any{"status": "calibrated"},
			DataProvenanceBadge:  map[string]any{"status": "complete"},
			ApprovalState:        "display_only",
			DeepLink:             "https://qf.example.test/packets/packet-123",
			PacketVersion:        1,
			DecisionType:         DecisionTypeRecommendation,
			CreatedAt:            "2026-05-06T00:00:00Z",
			UpdatedAt:            "2026-05-06T00:00:00Z",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 25)
	packet, err := client.FetchDecisionPacket(context.Background(), "packet-123")
	if err != nil {
		t.Fatalf("FetchDecisionPacket failed: %v", err)
	}
	if packet.TraceID != "trace-1" {
		t.Fatalf("TraceID = %q", packet.TraceID)
	}
	if packet.ContractVersion != 1 {
		t.Fatalf("ContractVersion = %d", packet.ContractVersion)
	}
	if gotPath != DecisionPacketsPath+"/packet-123" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "packet_version=1" {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestDTOJSONFieldNamesMirrorQFContract(t *testing.T) {
	eventJSON, err := json.Marshal(QFDecisionEvent{
		ContractVersion: 1,
		EventID:         "event-1",
		PacketID:        "packet-1",
		IntentID:        "intent-1",
		ScenarioID:      "scenario-1",
		TraceID:         "trace-1",
		EventType:       "packet_created",
		DecisionType:    DecisionTypeRecommendation,
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          "qf-smackerel-v1:1",
		PacketURL:       "https://qf.example.test/packets/packet-1",
		SourceSurface:   "gateway-route",
		CreatedAt:       "2026-05-06T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	assertJSONKeys(t, eventJSON, []string{
		"contract_version", "event_id", "packet_id", "intent_id", "scenario_id", "trace_id",
		"event_type", "decision_type", "approval_state", "packet_version", "cursor", "packet_url", "source_surface", "created_at",
	})

	envelopeJSON, err := json.Marshal(QFDecisionPacketEnvelope{
		ContractVersion:      1,
		PacketID:             "packet-1",
		IntentID:             "intent-1",
		ScenarioID:           "scenario-1",
		TraceID:              "trace-1",
		Thesis:               "QF-authored thesis",
		WhyNow:               "QF-authored timing",
		QuantifiedImpact:     map[string]any{"unit": "bps"},
		ExpertAnalysisBundle: map[string]any{"ref": "qf-analysis"},
		CalibrationBadge:     map[string]any{"state": "calibrated"},
		DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
		ApprovalState:        "display_only",
		DeepLink:             "https://qf.example.test/packets/packet-1",
		PacketVersion:        1,
		DecisionType:         DecisionTypeRecommendation,
		CreatedAt:            "2026-05-06T00:00:00Z",
		UpdatedAt:            "2026-05-06T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	assertJSONKeys(t, envelopeJSON, []string{
		"contract_version", "packet_id", "intent_id", "scenario_id", "trace_id", "thesis", "why_now",
		"quantified_impact", "expert_analysis_bundle", "calibration_badge", "data_provenance_badge",
		"approval_state", "deep_link", "packet_version", "decision_type", "created_at", "updated_at",
	})

	bundleJSON, err := json.Marshal(PersonalEvidenceBundle{
		ContractVersion:   1,
		BundleID:          "bundle-1",
		ExportID:          "export-1",
		CreatedAt:         "2026-05-06T00:00:00Z",
		ConsentScope:      "personal_context_analysis_only",
		SensitivityTier:   "personal",
		SourceArtifactIDs: []string{"artifact-1"},
		ExtractedClaims:   []string{"user context"},
		Provenance:        map[string]any{"generator": "smackerel"},
		RedactionSummary:  map[string]any{"omitted": 0},
		TargetContext:     map[string]any{"type": "rhai_run"},
	})
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	assertJSONKeys(t, bundleJSON, []string{
		"contract_version", "bundle_id", "export_id", "created_at", "consent_scope", "sensitivity_tier",
		"source_artifact_ids", "extracted_claims", "provenance", "redaction_summary", "target_context",
	})
	if strings.Contains(string(bundleJSON), "source_refs") {
		t.Fatalf("source_refs should be omitted when absent: %s", bundleJSON)
	}

	bundleWithRefsJSON, err := json.Marshal(PersonalEvidenceBundle{
		ContractVersion:   1,
		BundleID:          "bundle-2",
		ExportID:          "export-2",
		CreatedAt:         "2026-05-06T00:00:00Z",
		ConsentScope:      "personal_context_analysis_only",
		SensitivityTier:   "personal",
		SourceArtifactIDs: []string{"artifact-2"},
		ExtractedClaims:   []string{"second user context"},
		Provenance:        map[string]any{"generator": "smackerel"},
		RedactionSummary:  map[string]any{"omitted": 0},
		TargetContext:     map[string]any{"type": "analysis_context"},
		SourceRefs:        []string{"url:https://source.example.test/research"},
	})
	if err != nil {
		t.Fatalf("marshal bundle with refs: %v", err)
	}
	assertJSONKeys(t, bundleWithRefsJSON, []string{"source_refs", "target_context"})
}

func TestDecisionTypeContentTypeMappings(t *testing.T) {
	cases := []struct {
		decisionType string
		contentType  string
		subtype      string
	}{
		{DecisionTypeRecommendation, ContentTypeDecisionPacket, ""},
		{DecisionTypeNoAction, ContentTypeNoActionDecision, ""},
		{DecisionTypePolicyDenial, ContentTypePolicyDenial, ""},
		{DecisionTypeAnalysisNote, ContentTypeDecisionPacket, DecisionTypeAnalysisNote},
	}

	for _, tc := range cases {
		mapping, ok := ContentTypeForDecisionType(tc.decisionType)
		if !ok {
			t.Fatalf("missing mapping for %s", tc.decisionType)
		}
		if mapping.ContentType != tc.contentType || mapping.MetadataDecisionSubtype != tc.subtype {
			t.Fatalf("mapping for %s = %#v", tc.decisionType, mapping)
		}
	}
	if _, ok := ContentTypeForDecisionType(ContentTypeApprovalRequest); ok {
		t.Fatal("reserved approval request content type must not be a decision_type mapping")
	}
}

func assertJSONKeys(t *testing.T, raw []byte, keys []string) {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	for _, key := range keys {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing JSON key %q in %s", key, raw)
		}
	}
}

// --- ClampPageSize (spec 041 Scope 2 Round 2A) ---

func TestClient_ClampPageSize_WithinBounds(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)
	if got := client.ClampPageSize(50, 1, 200); got != 50 {
		t.Fatalf("ClampPageSize(50, 1, 200) = %d, want 50", got)
	}
}

func TestClient_ClampPageSize_AboveMax(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)
	if got := client.ClampPageSize(500, 1, 200); got != 200 {
		t.Fatalf("ClampPageSize(500, 1, 200) = %d, want 200 (clamped to capability max)", got)
	}
}

func TestClient_ClampPageSize_BelowMin(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)
	if got := client.ClampPageSize(0, 1, 200); got != 1 {
		t.Fatalf("ClampPageSize(0, 1, 200) = %d, want 1 (capability min)", got)
	}
	if got := client.ClampPageSize(-5, 1, 200); got != 1 {
		t.Fatalf("ClampPageSize(-5, 1, 200) = %d, want 1 (capability min)", got)
	}
	if got := client.ClampPageSize(3, 5, 200); got != 5 {
		t.Fatalf("ClampPageSize(3, 5, 200) = %d, want 5 (capability min)", got)
	}
}

func TestClientBlocksPollingWithoutPersistedCapability(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 25)
	_, err := client.FetchDecisionEvents(context.Background(), "")
	if err == nil {
		t.Fatal("expected capability-unavailable error before polling, got nil")
	}
	var unavailable CapabilityUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected CapabilityUnavailableError, got %T: %v", err, err)
	}
	if attempts != 0 {
		t.Fatalf("decision-events endpoint was called without persisted capability: attempts=%d", attempts)
	}

	client.SetCapability(nil, CapabilityStatusUnfetched)
	_, err = client.FetchDecisionEvents(context.Background(), "")
	if err == nil {
		t.Fatal("expected capability-unavailable error after explicit unfetched reset, got nil")
	}
	if attempts != 0 {
		t.Fatalf("decision-events endpoint was called after unfetched reset: attempts=%d", attempts)
	}
}

// TestClientClampsPageSizeToCapabilityRange (SCN-SM-041-005) is an umbrella
// table-driven test that exercises Client.ClampPageSize across the full
// range scenario set the spec describes:
//
//   - within bounds → return requested verbatim
//   - above capability_max → clamp to capability_max
//   - below capability_min → clamp to capability_min
//
// The existing TestClient_ClampPageSize_{WithinBounds,AboveMax,BelowMin}
// tests exercise each branch in isolation; this umbrella matches the
// scopes.md Test Plan declared name while rejecting unfetched-capability
// pass-through behavior.
func TestClientClampsPageSizeToCapabilityRange(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)

	cases := []struct {
		name          string
		requested     int
		capabilityMin int
		capabilityMax int
		want          int
	}{
		{name: "within bounds", requested: 50, capabilityMin: 1, capabilityMax: 200, want: 50},
		{name: "at lower bound", requested: 1, capabilityMin: 1, capabilityMax: 200, want: 1},
		{name: "at upper bound", requested: 200, capabilityMin: 1, capabilityMax: 200, want: 200},
		{name: "above capability_max clamps down", requested: 500, capabilityMin: 1, capabilityMax: 200, want: 200},
		{name: "below capability_min zero clamps up", requested: 0, capabilityMin: 1, capabilityMax: 200, want: 1},
		{name: "below capability_min negative clamps up", requested: -5, capabilityMin: 1, capabilityMax: 200, want: 1},
		{name: "custom capability_min clamps up", requested: 7, capabilityMin: 10, capabilityMax: 200, want: 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := client.ClampPageSize(tc.requested, tc.capabilityMin, tc.capabilityMax)
			if got != tc.want {
				t.Fatalf("ClampPageSize(%d, %d, %d) = %d, want %d", tc.requested, tc.capabilityMin, tc.capabilityMax, got, tc.want)
			}
		})
	}
}

// --- FetchDecisionEvents page-size clamping + PAGE_SIZE_OUT_OF_RANGE rejection
// (spec 041 Scope 2 Round 2F) ---

// TestClient_FetchDecisionEvents_ClampsAboveCapabilityMax proves the client
// clamps the connector-configured page_size DOWN to capability.max_page_size
// when SetCapability records a CapabilityStatusCompatible handshake.
//
// Scenario: configured=500, capability.MaxPageSize=200, status=compatible
// Expected: request URL contains limit=200 (NOT limit=500).
func TestClient_FetchDecisionEvents_ClampsAboveCapabilityMax(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
			Events:     []QFDecisionEvent{},
			NextCursor: "qf-smackerel-v1:0",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 500)
	client.SetCapability(&QFBridgeCapability{MinPageSize: 1, MaxPageSize: 200}, CapabilityStatusCompatible)

	if _, err := client.FetchDecisionEvents(context.Background(), ""); err != nil {
		t.Fatalf("FetchDecisionEvents: %v", err)
	}
	if !strings.Contains(gotQuery, "limit=200") {
		t.Fatalf("expected request to clamp to limit=200, got query=%q", gotQuery)
	}
	if strings.Contains(gotQuery, "limit=500") {
		t.Fatalf("clamp failed; query unexpectedly contained limit=500: %q", gotQuery)
	}
}

// TestClient_FetchDecisionEvents_ClampsConfiguredZeroToCapabilityMin proves
// the client derives the lower bound from min_page_size in the compatible
// capability response, not from a hidden local default.
//
// Scenario: configured=0, capability.MinPageSize=5, status=compatible
// Expected: request URL contains limit=5.
func TestClient_FetchDecisionEvents_ClampsConfiguredZeroToCapabilityMin(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DecisionEventsResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 0)
	client.SetCapability(&QFBridgeCapability{MinPageSize: 5, MaxPageSize: 200}, CapabilityStatusCompatible)

	if _, err := client.FetchDecisionEvents(context.Background(), ""); err != nil {
		t.Fatalf("FetchDecisionEvents: %v", err)
	}
	if !strings.Contains(gotQuery, "limit=5") {
		t.Fatalf("expected capability-min-clamped limit=5, got query=%q", gotQuery)
	}
}

// TestClient_FetchDecisionEvents_IncompatibleStatusBlocksPolling proves the
// client does NOT send decision-events requests after the handshake declared
// the capability incompatible.
//
// Scenario: configured=500, capability.MaxPageSize=200, status=incompatible
// Expected: no HTTP request is issued.
func TestClient_FetchDecisionEvents_IncompatibleStatusBlocksPolling(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 500)
	client.SetCapability(&QFBridgeCapability{MinPageSize: 1, MaxPageSize: 200}, CapabilityStatusIncompatible)

	_, err := client.FetchDecisionEvents(context.Background(), "")
	if err == nil {
		t.Fatal("expected capability-unavailable error under incompatible status, got nil")
	}
	var unavailable CapabilityUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected CapabilityUnavailableError, got %T: %v", err, err)
	}
	if attempts != 0 {
		t.Fatalf("decision-events endpoint was called under incompatible status: attempts=%d", attempts)
	}
}

// TestClientPageSizeOutOfRangeAlertsWithoutRetry proves the client surfaces
// QF's typed PAGE_SIZE_OUT_OF_RANGE rejection without retrying the same sync
// cycle with any guessed, smaller, or hardcoded local limit.
func TestClientPageSizeOutOfRangeAlertsWithoutRetry(t *testing.T) {
	var attempts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts = append(attempts, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BridgeErrorResponse{
			Code:    "PAGE_SIZE_OUT_OF_RANGE",
			Message: "requested page_size is outside the current capability range",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 500)
	client.SetCapability(&QFBridgeCapability{MinPageSize: 1, MaxPageSize: 200}, CapabilityStatusCompatible)

	_, err := client.FetchDecisionEvents(context.Background(), "")
	if err == nil {
		t.Fatal("expected PAGE_SIZE_OUT_OF_RANGE error, got nil")
	}
	var oor PageSizeOutOfRangeError
	if !errors.As(err, &oor) {
		t.Fatalf("expected PageSizeOutOfRangeError, got %T: %v", err, err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected exactly 1 attempt (no retry), got %d: %v", len(attempts), attempts)
	}
	if !strings.Contains(attempts[0], "limit=200") {
		t.Fatalf("first attempt did not use capability-clamped limit=200: %q", attempts[0])
	}
	if strings.Contains(err.Error(), "retry") {
		t.Fatalf("error should not mention retry path: %v", err)
	}
	if oor.Code != "PAGE_SIZE_OUT_OF_RANGE" {
		t.Fatalf("expected error code PAGE_SIZE_OUT_OF_RANGE, got %q", oor.Code)
	}
}

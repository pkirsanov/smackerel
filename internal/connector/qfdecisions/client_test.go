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
	if got := client.ClampPageSize(50, 200); got != 50 {
		t.Fatalf("ClampPageSize(50, 200) = %d, want 50", got)
	}
}

func TestClient_ClampPageSize_AboveMax(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)
	if got := client.ClampPageSize(500, 200); got != 200 {
		t.Fatalf("ClampPageSize(500, 200) = %d, want 200 (clamped to capability max)", got)
	}
}

func TestClient_ClampPageSize_BelowMin(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)
	if got := client.ClampPageSize(0, 200); got != 1 {
		t.Fatalf("ClampPageSize(0, 200) = %d, want 1 (floor)", got)
	}
	if got := client.ClampPageSize(-5, 200); got != 1 {
		t.Fatalf("ClampPageSize(-5, 200) = %d, want 1 (floor)", got)
	}
}

func TestClient_ClampPageSize_UnfetchedCapability(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)
	// capabilityMax == 0 means handshake has not been performed yet; fall back
	// to the connector-configured request value verbatim.
	if got := client.ClampPageSize(25, 0); got != 25 {
		t.Fatalf("ClampPageSize(25, 0) = %d, want 25 (unfetched fallback)", got)
	}
}

// TestClientClampsPageSizeToCapabilityRange (SCN-SM-041-005) is an umbrella
// table-driven test that exercises Client.ClampPageSize across the full
// range scenario set the spec describes:
//
//   - within bounds → return requested verbatim
//   - above capability_max → clamp to capability_max
//   - below 1 (zero/negative) → clamp to floor of 1
//   - unfetched capability (max == 0) → return requested verbatim
//
// The existing TestClient_ClampPageSize_{WithinBounds,AboveMax,BelowMin,
// UnfetchedCapability} tests exercise each branch in isolation; this
// umbrella was added in Round 2K to match the scopes.md Test Plan declared
// name without removing the granular existing tests. No behavior changes.
func TestClientClampsPageSizeToCapabilityRange(t *testing.T) {
	client := NewClient("http://example.test", "token", 1, 50)

	cases := []struct {
		name          string
		requested     int
		capabilityMax int
		want          int
	}{
		{name: "within bounds", requested: 50, capabilityMax: 200, want: 50},
		{name: "at lower bound", requested: 1, capabilityMax: 200, want: 1},
		{name: "at upper bound", requested: 200, capabilityMax: 200, want: 200},
		{name: "above capability_max clamps down", requested: 500, capabilityMax: 200, want: 200},
		{name: "below floor (zero) clamps up to 1", requested: 0, capabilityMax: 200, want: 1},
		{name: "below floor (negative) clamps up to 1", requested: -5, capabilityMax: 200, want: 1},
		{name: "unfetched capability returns requested verbatim", requested: 25, capabilityMax: 0, want: 25},
		{name: "unfetched capability also passes through small requested", requested: 7, capabilityMax: 0, want: 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := client.ClampPageSize(tc.requested, tc.capabilityMax)
			if got != tc.want {
				t.Fatalf("ClampPageSize(%d, %d) = %d, want %d", tc.requested, tc.capabilityMax, got, tc.want)
			}
		})
	}
}

// --- FetchDecisionEvents page-size clamping + PAGE_SIZE_OUT_OF_RANGE retry
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
	client.SetCapability(&QFBridgeCapability{MaxPageSize: 200}, CapabilityStatusCompatible)

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

// TestClient_FetchDecisionEvents_ClampsConfiguredZeroToFloor proves the
// client clamps an invalid configured page_size UP to the floor of 1 when
// the capability is compatible. The configured-zero case is treated as an
// operator misconfiguration that should fail loud via a structured warn log,
// not silently substitute a default — but the request itself still ships
// with a valid value so the poll completes.
//
// Scenario: configured=0, capability.MaxPageSize=200, status=compatible
// Expected: request URL contains limit=1.
func TestClient_FetchDecisionEvents_ClampsConfiguredZeroToFloor(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DecisionEventsResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 0)
	client.SetCapability(&QFBridgeCapability{MaxPageSize: 200}, CapabilityStatusCompatible)

	if _, err := client.FetchDecisionEvents(context.Background(), ""); err != nil {
		t.Fatalf("FetchDecisionEvents: %v", err)
	}
	if !strings.Contains(gotQuery, "limit=1") {
		t.Fatalf("expected floor-clamped limit=1, got query=%q", gotQuery)
	}
}

// TestClient_FetchDecisionEvents_IncompatibleStatusBypassesClamp proves the
// client does NOT clamp when the handshake declared the capability
// incompatible. The connector is responsible for blocking polling in that
// state; the client's job is only to ensure any in-flight request before
// tear-down stays well-formed, which means using the configured value as-is.
//
// Scenario: configured=500, capability.MaxPageSize=200, status=incompatible
// Expected: request URL contains limit=500 (no clamp applied).
func TestClient_FetchDecisionEvents_IncompatibleStatusBypassesClamp(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DecisionEventsResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 500)
	client.SetCapability(&QFBridgeCapability{MaxPageSize: 200}, CapabilityStatusIncompatible)

	if _, err := client.FetchDecisionEvents(context.Background(), ""); err != nil {
		t.Fatalf("FetchDecisionEvents: %v", err)
	}
	if !strings.Contains(gotQuery, "limit=500") {
		t.Fatalf("incompatible status must not clamp; query=%q", gotQuery)
	}
	if strings.Contains(gotQuery, "limit=200") {
		t.Fatalf("unexpected clamp under incompatible status; query=%q", gotQuery)
	}
}

// TestClient_FetchDecisionEvents_RetriesOnPageSizeOutOfRange proves the
// client retries exactly once when QF returns 400 PAGE_SIZE_OUT_OF_RANGE,
// using the capability-clamped page size on retry. Two attempts MUST be
// observed at the test server; the second attempt succeeds.
//
// The retry path also exercises the structured WARN log (visible to operators
// in container stdout) — assertion is on the request count + retry success,
// not on slog output (which is plumbed through the global default handler).
func TestClient_FetchDecisionEvents_RetriesOnPageSizeOutOfRange(t *testing.T) {
	var attempts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts = append(attempts, r.URL.RawQuery)
		if len(attempts) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(BridgeErrorResponse{
				Code:    "PAGE_SIZE_OUT_OF_RANGE",
				Message: "requested page_size 500 exceeds max_page_size 200",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
			Events:     []QFDecisionEvent{},
			NextCursor: "qf-smackerel-v1:after-retry",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 500)
	client.SetCapability(&QFBridgeCapability{MaxPageSize: 200}, CapabilityStatusCompatible)

	resp, err := client.FetchDecisionEvents(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchDecisionEvents after retry: %v", err)
	}
	if resp.NextCursor != "qf-smackerel-v1:after-retry" {
		t.Fatalf("expected retry success NextCursor=%q, got %q", "qf-smackerel-v1:after-retry", resp.NextCursor)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected exactly 2 attempts (initial + retry), got %d: %v", len(attempts), attempts)
	}
	// Both attempts ship the capability-clamped limit=200; the test server's
	// first response simulates a stale-capability rejection that resolves on
	// the second try.
	for i, q := range attempts {
		if !strings.Contains(q, "limit=200") {
			t.Fatalf("attempt %d did not use clamped limit=200: %q", i+1, q)
		}
	}
}

// TestClient_FetchDecisionEvents_PageSizeOutOfRangePersistsAfterRetry proves
// the retry does NOT loop infinitely. When QF returns PAGE_SIZE_OUT_OF_RANGE
// on both the initial poll AND the retry, the client surfaces the wrapped
// error to the caller (no third attempt).
func TestClient_FetchDecisionEvents_PageSizeOutOfRangePersistsAfterRetry(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BridgeErrorResponse{
			Code:    "PAGE_SIZE_OUT_OF_RANGE",
			Message: "page_size still out of range",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "qf-service-token", 1, 500)
	client.SetCapability(&QFBridgeCapability{MaxPageSize: 200}, CapabilityStatusCompatible)

	_, err := client.FetchDecisionEvents(context.Background(), "")
	if err == nil {
		t.Fatal("expected PAGE_SIZE_OUT_OF_RANGE error after retry, got nil")
	}
	if attempts != 2 {
		t.Fatalf("expected exactly 2 attempts before surfacing error, got %d", attempts)
	}
	if !strings.Contains(err.Error(), "page_size_out_of_range persisted after retry") {
		t.Fatalf("expected wrapped retry-persistence error, got: %v", err)
	}
	// The underlying typed error must still be reachable via errors.As so
	// upstream callers can branch on the contract violation if needed.
	var oor PageSizeOutOfRangeError
	if !errors.As(err, &oor) {
		t.Fatalf("expected wrapped error to be unwrappable as PageSizeOutOfRangeError, got: %v", err)
	}
	if oor.Code != "PAGE_SIZE_OUT_OF_RANGE" {
		t.Fatalf("expected error code PAGE_SIZE_OUT_OF_RANGE, got %q", oor.Code)
	}
}

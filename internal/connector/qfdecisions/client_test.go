package qfdecisions

import (
	"context"
	"encoding/json"
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

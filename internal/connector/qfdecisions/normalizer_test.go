package qfdecisions

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/metrics"
)

func validQFEnvelope() QFDecisionPacketEnvelope {
	return QFDecisionPacketEnvelope{
		ContractVersion:      1,
		PacketID:             "packet-001",
		IntentID:             "intent-001",
		ScenarioID:           "scenario-001",
		TraceID:              "trace-001",
		Thesis:               "QF-authored thesis text",
		WhyNow:               "QF-authored timing rationale",
		QuantifiedImpact:     map[string]any{"unit": "bps", "value": 12.5},
		ExpertAnalysisBundle: map[string]any{"ref": "qf-analysis-001"},
		CalibrationBadge:     map[string]any{"state": "calibrated", "score": 0.92},
		DataProvenanceBadge:  map[string]any{"source": "qf-owned", "complete": true},
		ApprovalState:        "display_only",
		DeepLink:             "https://qf.example.test/packets/packet-001",
		PacketVersion:        1,
		DecisionType:         DecisionTypeRecommendation,
		CreatedAt:            "2026-05-06T00:00:00Z",
		UpdatedAt:            "2026-05-06T00:01:00Z",
	}
}

func validQFEvent() QFDecisionEvent {
	return QFDecisionEvent{
		ContractVersion: 1,
		EventID:         "event-001",
		PacketID:        "packet-001",
		IntentID:        "intent-001",
		ScenarioID:      "scenario-001",
		TraceID:         "trace-001",
		EventType:       "packet_created",
		DecisionType:    DecisionTypeRecommendation,
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          "qf-smackerel-v1:9",
		PacketURL:       "https://qf.example.test/packets/packet-001",
		SourceSurface:   "gateway-route",
		CreatedAt:       "2026-05-06T00:00:00Z",
	}
}

func TestNormalizerPreservesQFTrustMetadataForValidPacket(t *testing.T) {
	n := NewNormalizer(DefaultConnectorID, 1)
	captured := time.Date(2026, 5, 6, 0, 1, 0, 0, time.UTC)

	artifact, diag := n.Normalize(validQFEvent(), validQFEnvelope(), captured)
	if diag != nil {
		t.Fatalf("expected nil diagnostic, got %#v", diag)
	}
	if artifact == nil {
		t.Fatal("expected normalized artifact for valid packet")
	}

	if artifact.SourceID != DefaultConnectorID {
		t.Fatalf("SourceID = %q, want %q", artifact.SourceID, DefaultConnectorID)
	}
	if artifact.SourceRef != "packet-001" {
		t.Fatalf("SourceRef = %q, want packet-001", artifact.SourceRef)
	}
	if artifact.ContentType != ContentTypeDecisionPacket {
		t.Fatalf("ContentType = %q, want %q", artifact.ContentType, ContentTypeDecisionPacket)
	}
	if artifact.URL != "https://qf.example.test/packets/packet-001" {
		t.Fatalf("URL = %q, want deep link", artifact.URL)
	}
	if !artifact.CapturedAt.Equal(captured) {
		t.Fatalf("CapturedAt = %v, want %v", artifact.CapturedAt, captured)
	}
	if artifact.Title == "" {
		t.Fatal("Title must not be empty")
	}

	requireString(t, artifact.Metadata, "packet_id", "packet-001")
	requireString(t, artifact.Metadata, "intent_id", "intent-001")
	requireString(t, artifact.Metadata, "scenario_id", "scenario-001")
	requireString(t, artifact.Metadata, "trace_id", "trace-001")
	requireString(t, artifact.Metadata, "approval_state", "display_only")
	requireString(t, artifact.Metadata, "deep_link", "https://qf.example.test/packets/packet-001")
	requireString(t, artifact.Metadata, "decision_type", DecisionTypeRecommendation)
	requireInt(t, artifact.Metadata, "packet_version", 1)

	for _, key := range []string{"calibration_badge", "data_provenance_badge", "quantified_impact", "expert_analysis_bundle"} {
		v, ok := artifact.Metadata[key]
		if !ok {
			t.Fatalf("metadata missing %s", key)
		}
		if _, isMap := v.(map[string]any); !isMap {
			t.Fatalf("metadata %s should be map, got %T", key, v)
		}
	}

	if subtype, ok := artifact.Metadata["decision_subtype"]; ok {
		t.Fatalf("recommendation packet should NOT carry decision_subtype, got %v", subtype)
	}

	// Raw envelope must round-trip without dropping required QF fields.
	var roundTrip map[string]any
	if err := json.Unmarshal([]byte(artifact.RawContent), &roundTrip); err != nil {
		t.Fatalf("RawContent is not valid JSON: %v", err)
	}
	for _, key := range []string{
		"packet_id", "intent_id", "scenario_id", "trace_id",
		"thesis", "why_now", "approval_state", "deep_link",
		"calibration_badge", "data_provenance_badge",
		"packet_version", "decision_type",
	} {
		if _, ok := roundTrip[key]; !ok {
			t.Fatalf("RawContent missing QF field %q", key)
		}
	}
}

func TestNormalizerRejectsIncompletePacketEnvelopes(t *testing.T) {
	n := NewNormalizer(DefaultConnectorID, 1)
	captured := time.Now().UTC()

	requiredCases := []struct {
		name           string
		mutate         func(*QFDecisionPacketEnvelope)
		wantMissingKey string
	}{
		{
			name:           "missing packet_id",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.PacketID = "" },
			wantMissingKey: "packet_id",
		},
		{
			name:           "missing intent_id",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.IntentID = "" },
			wantMissingKey: "intent_id",
		},
		{
			name:           "missing scenario_id",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.ScenarioID = "" },
			wantMissingKey: "scenario_id",
		},
		{
			name:           "missing trace_id",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.TraceID = "" },
			wantMissingKey: "trace_id",
		},
		{
			name:           "missing approval_state",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.ApprovalState = "" },
			wantMissingKey: "approval_state",
		},
		{
			name:           "missing deep_link",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.DeepLink = "" },
			wantMissingKey: "deep_link",
		},
		{
			name:           "missing calibration_badge",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.CalibrationBadge = nil },
			wantMissingKey: "calibration_badge",
		},
		{
			name:           "missing data_provenance_badge",
			mutate:         func(e *QFDecisionPacketEnvelope) { e.DataProvenanceBadge = nil },
			wantMissingKey: "data_provenance_badge",
		},
	}

	for _, tc := range requiredCases {
		t.Run(tc.name, func(t *testing.T) {
			env := validQFEnvelope()
			tc.mutate(&env)
			artifact, diag := n.Normalize(validQFEvent(), env, captured)
			if artifact != nil {
				t.Fatalf("expected nil artifact for %s, got %+v", tc.name, artifact)
			}
			if diag == nil {
				t.Fatalf("expected diagnostic for %s, got nil", tc.name)
			}
			found := false
			for _, missing := range diag.MissingFields {
				if missing == tc.wantMissingKey {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected MissingFields to include %q, got %v", tc.wantMissingKey, diag.MissingFields)
			}
		})
	}

	t.Run("unknown packet version", func(t *testing.T) {
		env := validQFEnvelope()
		env.PacketVersion = 99
		artifact, diag := n.Normalize(validQFEvent(), env, captured)
		if artifact != nil {
			t.Fatalf("expected nil artifact for unknown packet version, got %+v", artifact)
		}
		if diag == nil {
			t.Fatal("expected diagnostic for unknown packet version")
		}
		if !strings.Contains(diag.Reason, "packet_version") {
			t.Fatalf("diagnostic reason should mention packet_version, got %q", diag.Reason)
		}
	})

	t.Run("unknown decision type marks metadata flag", func(t *testing.T) {
		// design.md §F8 ("Forward-Compatible decision_type Handling"):
		// unknown decision_type values MUST NOT be rejected. The
		// normalizer falls through to the canonical qf/decision-packet
		// content type, sets metadata.unknown_decision_type=true, and
		// preserves the raw decision_type value so downstream consumers
		// can render via the generic packet card variant.
		env := validQFEnvelope()
		env.DecisionType = "unknown_decision_type"
		ev := validQFEvent()
		ev.DecisionType = "unknown_decision_type"
		artifact, diag := n.Normalize(ev, env, captured)
		if diag != nil {
			t.Fatalf("expected nil diagnostic for unknown decision type (design.md §F8), got %#v", diag)
		}
		if artifact == nil {
			t.Fatal("expected normalized artifact for unknown decision type (design.md §F8 forbids rejection)")
		}
		if artifact.ContentType != ContentTypeDecisionPacket {
			t.Fatalf("ContentType = %q, want %q (unknown decision_type must NOT invent a new qf/... type)", artifact.ContentType, ContentTypeDecisionPacket)
		}
		flag, ok := artifact.Metadata["unknown_decision_type"].(bool)
		if !ok {
			t.Fatalf("metadata[unknown_decision_type] = %v (%T), want bool true", artifact.Metadata["unknown_decision_type"], artifact.Metadata["unknown_decision_type"])
		}
		if !flag {
			t.Fatal("metadata[unknown_decision_type] = false, want true")
		}
		requireString(t, artifact.Metadata, "decision_type", "unknown_decision_type")
	})
}

func TestNormalizerAnalysisNotePreservesSubtype(t *testing.T) {
	n := NewNormalizer(DefaultConnectorID, 1)
	env := validQFEnvelope()
	env.DecisionType = DecisionTypeAnalysisNote
	ev := validQFEvent()
	ev.DecisionType = DecisionTypeAnalysisNote

	artifact, diag := n.Normalize(ev, env, time.Now().UTC())
	if diag != nil {
		t.Fatalf("expected nil diagnostic, got %#v", diag)
	}
	if artifact == nil {
		t.Fatal("expected normalized artifact for analysis_note")
	}
	if artifact.ContentType != ContentTypeDecisionPacket {
		t.Fatalf("analysis_note ContentType = %q, want %q", artifact.ContentType, ContentTypeDecisionPacket)
	}
	subtype, ok := artifact.Metadata["decision_subtype"].(string)
	if !ok || subtype != DecisionTypeAnalysisNote {
		t.Fatalf("decision_subtype = %v, want %q", artifact.Metadata["decision_subtype"], DecisionTypeAnalysisNote)
	}
}

// TestNormalizerMarksUnknownDecisionTypeWithMetadata proves the spec-correct
// behavior for forward-compatible decision_type handling (design.md §F8 and
// scopes.md SCN-SM-041-006): when QF emits a decision_type value that is
// outside the canonical set {recommendation, no_action, policy_denial,
// analysis_note}, the normalizer MUST NOT reject the packet. Instead it:
//
//  1. produces a RawArtifact (not a DegradedDiagnostic),
//  2. uses the canonical qf/decision-packet ContentType (never invents a
//     new qf/... content type for the unknown value),
//  3. sets Metadata["unknown_decision_type"] = true so downstream consumers
//     (Scope 3 generic-card variant, search, digest, Telegram) can route
//     the artifact through the generic packet card,
//  4. preserves the raw unknown value in Metadata["decision_type"] so
//     operators and downstream consumers can label it accurately,
//  5. increments smackerel_qf_unknown_decision_type_total{value=<raw>}
//     exactly once (the normalizer is the single source of truth; the
//     capability-gate emission was removed to avoid double-counting).
//
// This test is the unit-level counterpart to the e2e-api test
// TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata.
func TestNormalizerMarksUnknownDecisionTypeWithMetadata(t *testing.T) {
	// Reset the metric so the increment assertion isolates this test from
	// other tests that exercise the same counter.
	metrics.QFUnknownDecisionType.Reset()

	const unknownValue = "new-future-decision-shape-v9"
	n := NewNormalizer(DefaultConnectorID, 1)
	captured := time.Date(2026, 5, 6, 0, 1, 0, 0, time.UTC)

	env := validQFEnvelope()
	env.DecisionType = unknownValue
	ev := validQFEvent()
	ev.DecisionType = unknownValue

	artifact, diag := n.Normalize(ev, env, captured)
	if diag != nil {
		t.Fatalf("design.md §F8 forbids rejection of unknown decision_type; got diagnostic %#v", diag)
	}
	if artifact == nil {
		t.Fatal("expected RawArtifact for unknown decision_type (design.md §F8 forward-compatibility)")
	}

	// (a) Canonical content type — no qf/... invention.
	if artifact.ContentType != ContentTypeDecisionPacket {
		t.Fatalf("ContentType = %q, want %q (unknown decision_type MUST fall through to the canonical qf/decision-packet)", artifact.ContentType, ContentTypeDecisionPacket)
	}

	// (b) Metadata flag persisted as bool true.
	flagAny, ok := artifact.Metadata["unknown_decision_type"]
	if !ok {
		t.Fatalf("metadata missing unknown_decision_type key; metadata keys=%v", metadataKeys(artifact.Metadata))
	}
	flag, isBool := flagAny.(bool)
	if !isBool {
		t.Fatalf("metadata[unknown_decision_type] = %v (%T), want bool true", flagAny, flagAny)
	}
	if !flag {
		t.Fatal("metadata[unknown_decision_type] = false, want true")
	}

	// (c) Raw unknown decision_type value preserved verbatim for
	// downstream rendering (generic-card label, debug surfaces).
	requireString(t, artifact.Metadata, "decision_type", unknownValue)

	// (d) Source attribution still correct.
	if artifact.SourceID != DefaultConnectorID {
		t.Fatalf("SourceID = %q, want %q (unknown decision_type must NOT bypass source attribution)", artifact.SourceID, DefaultConnectorID)
	}
	if artifact.SourceRef != env.PacketID {
		t.Fatalf("SourceRef = %q, want %q (packet identity preserved)", artifact.SourceRef, env.PacketID)
	}

	// (e) Metric incremented exactly once with the raw unknown value as
	// the label. design.md §F8 requires this on every unknown_decision_type
	// packet regardless of capability advertisement.
	got := testutil.ToFloat64(metrics.QFUnknownDecisionType.WithLabelValues(unknownValue))
	if got != 1 {
		t.Fatalf("smackerel_qf_unknown_decision_type_total{value=%q} = %v, want 1", unknownValue, got)
	}
}

func TestNormalizerPreservesSignedLinkAndPreferredSurfaceMetadata(t *testing.T) {
	n := NewNormalizer(DefaultConnectorID, 1)
	env := validQFEnvelope()
	env.PacketURLSigned = "https://qf.example.test/packets/packet-001?sig=qf-owned"
	env.SignatureExpiresAt = "2026-05-06T00:10:00Z"
	env.PreferredSurface = "smackerel_telegram"

	artifact, diag := n.Normalize(validQFEvent(), env, time.Date(2026, 5, 6, 0, 1, 0, 0, time.UTC))
	if diag != nil {
		t.Fatalf("expected nil diagnostic, got %#v", diag)
	}
	if artifact == nil {
		t.Fatal("expected normalized artifact")
	}
	requireString(t, artifact.Metadata, "packet_url_signed", env.PacketURLSigned)
	requireString(t, artifact.Metadata, "signature_expires_at", env.SignatureExpiresAt)
	requireString(t, artifact.Metadata, "preferred_surface", env.PreferredSurface)

	var raw map[string]any
	if err := json.Unmarshal([]byte(artifact.RawContent), &raw); err != nil {
		t.Fatalf("RawContent is not valid JSON: %v", err)
	}
	for key, want := range map[string]string{
		"packet_url_signed":    env.PacketURLSigned,
		"signature_expires_at": env.SignatureExpiresAt,
		"preferred_surface":    env.PreferredSurface,
	} {
		got, ok := raw[key].(string)
		if !ok || got != want {
			t.Fatalf("RawContent[%q] = %v, want %q", key, raw[key], want)
		}
	}
}

// metadataKeys returns a sorted slice of map keys for diagnostic output.
func metadataKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestNormalizerContentTypeMappings(t *testing.T) {
	n := NewNormalizer(DefaultConnectorID, 1)
	captured := time.Now().UTC()
	cases := []struct {
		decisionType    string
		wantContentType string
		wantSubtype     string
	}{
		{DecisionTypeRecommendation, ContentTypeDecisionPacket, ""},
		{DecisionTypeNoAction, ContentTypeNoActionDecision, ""},
		{DecisionTypePolicyDenial, ContentTypePolicyDenial, ""},
		{DecisionTypeAnalysisNote, ContentTypeDecisionPacket, DecisionTypeAnalysisNote},
	}
	for _, tc := range cases {
		t.Run(tc.decisionType, func(t *testing.T) {
			env := validQFEnvelope()
			env.DecisionType = tc.decisionType
			ev := validQFEvent()
			ev.DecisionType = tc.decisionType
			artifact, diag := n.Normalize(ev, env, captured)
			if diag != nil {
				t.Fatalf("unexpected diagnostic for %s: %#v", tc.decisionType, diag)
			}
			if artifact.ContentType != tc.wantContentType {
				t.Fatalf("content_type for %s = %q, want %q", tc.decisionType, artifact.ContentType, tc.wantContentType)
			}
			subtype, _ := artifact.Metadata["decision_subtype"].(string)
			if subtype != tc.wantSubtype {
				t.Fatalf("subtype for %s = %q, want %q", tc.decisionType, subtype, tc.wantSubtype)
			}
		})
	}
}

func requireString(t *testing.T, m map[string]any, key, want string) {
	t.Helper()
	got, ok := m[key].(string)
	if !ok {
		t.Fatalf("metadata[%q] = %v, want string", key, m[key])
	}
	if got != want {
		t.Fatalf("metadata[%q] = %q, want %q", key, got, want)
	}
}

func requireInt(t *testing.T, m map[string]any, key string, want int) {
	t.Helper()
	switch v := m[key].(type) {
	case int:
		if v != want {
			t.Fatalf("metadata[%q] = %d, want %d", key, v, want)
		}
	case int64:
		if int(v) != want {
			t.Fatalf("metadata[%q] = %d, want %d", key, v, want)
		}
	case float64:
		if int(v) != want {
			t.Fatalf("metadata[%q] = %f, want %d", key, v, want)
		}
	default:
		t.Fatalf("metadata[%q] = %v (%T), want int-like %d", key, m[key], m[key], want)
	}
}

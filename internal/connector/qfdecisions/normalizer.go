package qfdecisions

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Normalizer converts a QF decision event + envelope pair into a Smackerel
// RawArtifact when the envelope has all required QF trust metadata.
//
// The normalizer never mints Smackerel-local recommendation identities: the
// QF packet identity (packet_id, intent_id, scenario_id, trace_id, approval
// state, badges, deep link, packet version, decision type) is preserved
// verbatim in the resulting artifact. Missing or incompatible required
// metadata produces a DegradedDiagnostic and NO trusted artifact.
type Normalizer struct {
	connectorID           string
	expectedPacketVersion int
}

// NewNormalizer returns a Normalizer bound to the given connector instance ID
// and expected QF packet version. The connector instance ID is the source ID
// stamped on each produced RawArtifact so live-stack tests can isolate their
// rows from the canonical qf-decisions source. expectedPacketVersion <= 0
// disables the version check (used only by tests that want to inspect
// missing-field behavior in isolation).
func NewNormalizer(connectorID string, expectedPacketVersion int) *Normalizer {
	id := strings.TrimSpace(connectorID)
	if id == "" {
		id = DefaultConnectorID
	}
	return &Normalizer{connectorID: id, expectedPacketVersion: expectedPacketVersion}
}

// DegradedDiagnostic describes why a QF event/envelope was NOT promoted to a
// trusted artifact. It is consumed for connector logs/metrics; nothing in this
// struct ever becomes Smackerel UI text.
type DegradedDiagnostic struct {
	PacketID      string
	EventID       string
	TraceID       string
	Reason        string
	MissingFields []string
}

// Normalize maps a QF event + envelope pair into a Smackerel RawArtifact.
// The returned diagnostic is non-nil ONLY when the envelope is degraded.
// At most one of (artifact, diagnostic) is non-nil.
func (n *Normalizer) Normalize(event QFDecisionEvent, envelope QFDecisionPacketEnvelope, capturedAt time.Time) (*connector.RawArtifact, *DegradedDiagnostic) {
	missing := requiredEnvelopeFieldsMissing(envelope)

	if n.expectedPacketVersion > 0 && envelope.PacketVersion != n.expectedPacketVersion {
		return nil, &DegradedDiagnostic{
			PacketID:      envelope.PacketID,
			EventID:       event.EventID,
			TraceID:       envelope.TraceID,
			Reason:        fmt.Sprintf("packet_version mismatch: got %d, expected %d", envelope.PacketVersion, n.expectedPacketVersion),
			MissingFields: missing,
		}
	}

	decisionType := strings.TrimSpace(envelope.DecisionType)
	if decisionType == "" {
		decisionType = strings.TrimSpace(event.DecisionType)
	}
	mapping, ok := ContentTypeForDecisionType(decisionType)
	isUnknownDecisionType := false
	if !ok {
		// design.md §F8 ("Forward-Compatible decision_type Handling"):
		// NEVER reject a packet for unknown decision_type alone. Fall
		// through to the canonical qf/decision-packet content type, mark
		// the artifact with Metadata.unknown_decision_type=true so
		// downstream consumers (Scope 3 generic-card variant, search,
		// digest) can route it through the generic packet card, and
		// increment the smackerel_qf_unknown_decision_type_total metric
		// labelled with the offending value. Trust metadata validation
		// (calibration badge, provenance badge, packet_version, required
		// envelope fields) still applies — those rejection paths run
		// AFTER this fall-through.
		mapping = ContentTypeMapping{ContentType: ContentTypeDecisionPacket}
		isUnknownDecisionType = true
		metrics.QFUnknownDecisionType.WithLabelValues(decisionType).Inc()
	}

	if len(missing) > 0 {
		return nil, &DegradedDiagnostic{
			PacketID:      envelope.PacketID,
			EventID:       event.EventID,
			TraceID:       envelope.TraceID,
			Reason:        "missing required QF trust metadata",
			MissingFields: missing,
		}
	}

	rawContent, err := json.Marshal(envelope)
	if err != nil {
		return nil, &DegradedDiagnostic{
			PacketID: envelope.PacketID,
			EventID:  event.EventID,
			TraceID:  envelope.TraceID,
			Reason:   fmt.Sprintf("encode envelope: %v", err),
		}
	}

	metadata := map[string]any{
		"packet_id":              envelope.PacketID,
		"intent_id":              envelope.IntentID,
		"scenario_id":            envelope.ScenarioID,
		"trace_id":               envelope.TraceID,
		"approval_state":         envelope.ApprovalState,
		"deep_link":              envelope.DeepLink,
		"packet_version":         envelope.PacketVersion,
		"decision_type":          decisionType,
		"calibration_badge":      envelope.CalibrationBadge,
		"data_provenance_badge":  envelope.DataProvenanceBadge,
		"quantified_impact":      envelope.QuantifiedImpact,
		"expert_analysis_bundle": envelope.ExpertAnalysisBundle,
		"thesis":                 envelope.Thesis,
		"why_now":                envelope.WhyNow,
		"contract_version":       envelope.ContractVersion,
		"qf_created_at":          envelope.CreatedAt,
		"qf_updated_at":          envelope.UpdatedAt,
		"event_id":               event.EventID,
		"event_type":             event.EventType,
		"source_surface":         event.SourceSurface,
	}
	if mapping.MetadataDecisionSubtype != "" {
		metadata["decision_subtype"] = mapping.MetadataDecisionSubtype
	}
	if isUnknownDecisionType {
		// design.md §F8: the raw unknown decision_type is preserved in
		// metadata["decision_type"] (set above) so downstream consumers
		// can display the actual value; this boolean flag is the
		// dispatch hint for the generic packet card variant.
		metadata["unknown_decision_type"] = true
	}
	if len(envelope.Metadata) > 0 {
		metadata["envelope_metadata"] = envelope.Metadata
	}

	title := strings.TrimSpace(envelope.Thesis)
	if title == "" {
		title = fmt.Sprintf("QF %s %s", decisionType, envelope.PacketID)
	}

	artifact := &connector.RawArtifact{
		SourceID:    n.connectorID,
		SourceRef:   envelope.PacketID,
		ContentType: mapping.ContentType,
		Title:       title,
		RawContent:  string(rawContent),
		URL:         envelope.DeepLink,
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}
	return artifact, nil
}

// requiredEnvelopeFieldsMissing returns the set of required QF trust fields
// that are absent or empty on the envelope. Order is stable for diagnostic
// log output.
func requiredEnvelopeFieldsMissing(envelope QFDecisionPacketEnvelope) []string {
	var missing []string
	if strings.TrimSpace(envelope.PacketID) == "" {
		missing = append(missing, "packet_id")
	}
	if strings.TrimSpace(envelope.IntentID) == "" {
		missing = append(missing, "intent_id")
	}
	if strings.TrimSpace(envelope.ScenarioID) == "" {
		missing = append(missing, "scenario_id")
	}
	if strings.TrimSpace(envelope.TraceID) == "" {
		missing = append(missing, "trace_id")
	}
	if strings.TrimSpace(envelope.ApprovalState) == "" {
		missing = append(missing, "approval_state")
	}
	if strings.TrimSpace(envelope.DeepLink) == "" {
		missing = append(missing, "deep_link")
	}
	if len(envelope.CalibrationBadge) == 0 {
		missing = append(missing, "calibration_badge")
	}
	if len(envelope.DataProvenanceBadge) == 0 {
		missing = append(missing, "data_provenance_badge")
	}
	return missing
}

// envelopeCapturedAt picks the best timestamp for RawArtifact.CapturedAt:
// envelope.UpdatedAt → envelope.CreatedAt → event.CreatedAt → fallback.
// QF timestamps are RFC3339-encoded strings.
func envelopeCapturedAt(envelope QFDecisionPacketEnvelope, event QFDecisionEvent, fallback time.Time) time.Time {
	candidates := []string{envelope.UpdatedAt, envelope.CreatedAt, event.CreatedAt}
	for _, raw := range candidates {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts.UTC()
		}
		if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return ts.UTC()
		}
	}
	return fallback.UTC()
}

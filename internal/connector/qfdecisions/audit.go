package qfdecisions

import (
	"log/slog"
	"strings"
	"time"
)

type AuditEnvelopeInput struct {
	TraceID         string
	PacketID        string
	ExportID        string
	SignalID        string
	ActorRef        string
	Surface         string
	Action          string
	Outcome         string
	Reason          string
	BundleID        string
	TargetContext   string
	SensitivityTier string
	ObservedAt      time.Time
}

func BuildCrossProductAuditEnvelopeV1(input AuditEnvelopeInput) EvidenceAuditEnvelope {
	observedAt := input.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	actorRef := strings.TrimSpace(input.ActorRef)
	if actorRef == "" {
		actorRef = AuditActorSmackerelConnector
	}
	surface := strings.TrimSpace(input.Surface)
	if surface == "" {
		surface = DefaultConnectorID
	}
	stamp := observedAt.Format(time.RFC3339)
	return EvidenceAuditEnvelope{
		TraceID:              strings.TrimSpace(input.TraceID),
		PacketID:             strings.TrimSpace(input.PacketID),
		ExportID:             strings.TrimSpace(input.ExportID),
		SignalID:             strings.TrimSpace(input.SignalID),
		ActorRef:             actorRef,
		Surface:              surface,
		Action:               strings.TrimSpace(input.Action),
		Outcome:              strings.TrimSpace(input.Outcome),
		Reason:               strings.TrimSpace(input.Reason),
		TS:                   stamp,
		AuditEnvelopeVersion: AuditEnvelopeVersionV1,
		BundleID:             strings.TrimSpace(input.BundleID),
		TargetContextType:    strings.TrimSpace(input.TargetContext),
		SensitivityTier:      strings.TrimSpace(input.SensitivityTier),
		RecordedAt:           stamp,
	}
}

func EmitConnectorAuditEnvelope(envelope EvidenceAuditEnvelope) {
	slog.Info("qf-decisions: cross_product_audit",
		slog.String("audit_envelope_version", envelope.AuditEnvelopeVersion),
		slog.String("trace_id", envelope.TraceID),
		slog.String("packet_id", envelope.PacketID),
		slog.String("export_id", envelope.ExportID),
		slog.String("signal_id", envelope.SignalID),
		slog.String("actor_ref", envelope.ActorRef),
		slog.String("surface", envelope.Surface),
		slog.String("action", envelope.Action),
		slog.String("outcome", envelope.Outcome),
		slog.String("reason", envelope.Reason),
		slog.String("ts", envelope.TS),
	)
}

// EngagementSignalAuditInput carries the fields a Scope 6 engagement
// signal flush MUST populate when emitting Cross-Product Audit Envelope
// v1 records. The connector audit shape is owned here so Scope 6's
// transport implementation only has to provide the per-event values.
// SCN-SM-041-021.
type EngagementSignalAuditInput struct {
	SignalID   string
	TraceID    string
	PacketID   string
	ActorRef   string
	Surface    string
	Event      string
	Status     string
	Reason     string
	ObservedAt time.Time
}

// EmitEngagementSignalFlushAudit records a Cross-Product Audit Envelope
// v1 entry for the QF engagement_signal_flush emission point. The
// outcome derives from the supplied `Status` (`ok`, `rejected`,
// `error`); the corresponding metric increment is owned by the Scope 6
// transport (RecordQFEngagementSignalAttempt). The helper trims input
// fields and defaults `ActorRef`/`Surface` to the connector identity.
// SCN-SM-041-021.
func EmitEngagementSignalFlushAudit(input EngagementSignalAuditInput) EvidenceAuditEnvelope {
	envelope := BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:    input.TraceID,
		PacketID:   input.PacketID,
		SignalID:   input.SignalID,
		ActorRef:   input.ActorRef,
		Surface:    input.Surface,
		Action:     AuditActionEngagementSignalFlush,
		Outcome:    auditOutcomeForStatus(input.Status),
		Reason:     input.Reason,
		ObservedAt: input.ObservedAt,
	})
	EmitConnectorAuditEnvelope(envelope)
	return envelope
}

// CallbackAttemptAuditInput carries the fields a Scope 8 signed-callback
// attempt MUST populate when emitting Cross-Product Audit Envelope v1
// records. The connector audit shape is owned here so Scope 8's signing
// transport only has to provide the per-attempt values. SCN-SM-041-021.
type CallbackAttemptAuditInput struct {
	TraceID    string
	PacketID   string
	ActorRef   string
	Surface    string
	Action     string
	Status     string
	Reason     string
	ObservedAt time.Time
}

// EmitCallbackAttemptAudit records a Cross-Product Audit Envelope v1
// entry for the QF callback_attempt emission point. The outcome derives
// from the supplied `Status` (`ok`, `rejected`, `error`); the
// corresponding metric increment is owned by the Scope 8 transport
// (RecordQFCallbackAttempt). The supplied `Action` describes the
// callback's payload action (e.g., `surface_dismiss`, `surface_engage`)
// and is preserved verbatim in the envelope `reason` slot so audit
// consumers can correlate against the callback transport log without
// new envelope fields. SCN-SM-041-021.
func EmitCallbackAttemptAudit(input CallbackAttemptAuditInput) EvidenceAuditEnvelope {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = strings.TrimSpace(input.Action)
	}
	envelope := BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:    input.TraceID,
		PacketID:   input.PacketID,
		ActorRef:   input.ActorRef,
		Surface:    input.Surface,
		Action:     AuditActionCallbackAttempt,
		Outcome:    auditOutcomeForStatus(input.Status),
		Reason:     reason,
		ObservedAt: input.ObservedAt,
	})
	EmitConnectorAuditEnvelope(envelope)
	return envelope
}

func auditOutcomeForStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "ok", "success", "accepted":
		return AuditOutcomeOK
	case "rejected", "refused", "denied":
		return AuditOutcomeRejected
	case "error", "failed", "failure":
		return AuditOutcomeError
	case "":
		return AuditOutcomeOK
	default:
		return strings.TrimSpace(strings.ToLower(status))
	}
}

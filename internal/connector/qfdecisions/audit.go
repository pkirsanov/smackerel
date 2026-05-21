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

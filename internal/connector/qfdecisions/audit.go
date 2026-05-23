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

// EmitConnectorAuditEnvelope logs the complete Cross-Product Audit
// Envelope v1 record to the connector audit sink (slog). Every field of
// the EvidenceAuditEnvelope struct that appears in the canonical JSON
// shape (per spec 041 design.md §F4 / scopes.md L837) is emitted so
// downstream consumers reading the structured log see the same shape
// the in-process call-sites receive. SCN-SM-041-021.
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
		slog.String("recorded_at", envelope.RecordedAt),
		slog.String("bundle_id", envelope.BundleID),
		slog.String("target_context_type", envelope.TargetContextType),
		slog.String("sensitivity_tier", envelope.SensitivityTier),
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
//
// SCN-SM-041-020 callback-adjacent safety-boundary defense-in-depth:
// the helper pre-checks `input.Action` via EnforceQFActionBoundary
// BEFORE building or emitting the callback audit envelope. If the
// callback payload's action is a forbidden QF action type (approval,
// execution, mandate_change, emergency_stop, watch_*,
// callback_acceptance, qf_trust_reconstruction) the boundary helper
// fires first and the callback envelope's outcome is forced to
// AuditOutcomeRejected so the same emission point cannot accidentally
// report `ok` for a payload that violated the pre-MVP no-action
// contract. Scope 8 has NOT wired the signed-callback transport yet
// (audit.go owns shape only; HMAC signing, dispatch, and acceptance
// remain Scope 8 territory) so no production caller invokes this
// helper at HEAD; the guard is forward-ready and will become operative
// when Scope 8 wires the signed-callback transport.
func EmitCallbackAttemptAudit(input CallbackAttemptAuditInput) EvidenceAuditEnvelope {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = strings.TrimSpace(input.Action)
	}
	outcome := auditOutcomeForStatus(input.Status)
	if _, fired, _ := EnforceQFActionBoundary(ActionBoundaryAttempt{
		AttemptedActionType: strings.TrimSpace(input.Action),
		TraceID:             input.TraceID,
		PacketID:            input.PacketID,
		ActorRef:            input.ActorRef,
		Surface:             input.Surface,
		Reason:              "callback_action_rejected",
		ObservedAt:          input.ObservedAt,
	}); fired {
		outcome = AuditOutcomeRejected
	}
	envelope := BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:    input.TraceID,
		PacketID:   input.PacketID,
		ActorRef:   input.ActorRef,
		Surface:    input.Surface,
		Action:     AuditActionCallbackAttempt,
		Outcome:    outcome,
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

// SCN-SM-041-020 watch-adjacent safety-boundary gate.
//
// Scope 9 (Watch-Signal Proposal Exporter) wires its safety-boundary gate
// inside internal/connector/qfdecisions/watch_proposal.go
// WatchProposalClient.Propose: BEFORE any signing or HTTP transport, the
// client invokes EnforceQFActionBoundary with attempted action type
// "watch_proposal" so the Scope 5 safety boundary helper has the
// opportunity to reject the attempt if a future code change ever adds
// `watch_proposal` to IsForbiddenQFActionType. In MVP "watch_proposal" is
// NOT forbidden (the diagnostic path is the intended pre-MVP behavior), so
// the boundary helper is a no-op gate today; the explicit invocation
// locks in the pattern. Smackerel MUST NOT author, evaluate, or accept QF
// watch-signal proposals as actionable state mutations; the pre-MVP
// rejection envelope `WATCH_PROPOSALS_DEFERRED_TO_V1` is the only
// expected QF response and is parsed without retry, without local
// watch-state mutation, and without any user-visible affordance.

// WatchProposalAttemptAuditInput carries the fields a Scope 9
// watch-proposal attempt MUST populate when emitting Cross-Product
// Audit Envelope v1 records. The connector audit shape is owned here
// so Scope 9's diagnostic client only has to provide the per-attempt
// values. SCN-SM-041-031 / SCN-SM-041-033.
type WatchProposalAttemptAuditInput struct {
	TraceID    string
	EntityRef  string
	ActorRef   string
	Surface    string
	Status     string
	Reason     string
	ObservedAt time.Time
}

// EmitWatchProposalAttemptAudit records a Cross-Product Audit
// Envelope v1 entry for the QF watch_proposal emission point. The
// outcome derives from the supplied `Status` (`ok`, `rejected`,
// `error`); the corresponding metric increment is owned by the
// Scope 9 transport (RecordQFWatchProposalAttempt). The supplied
// `EntityRef` describes the entity the proposal references and is
// preserved verbatim in the envelope `target_context_type` slot so
// audit consumers can correlate against the watch-proposal transport
// log without new envelope fields. SCN-SM-041-031 / SCN-SM-041-033.
//
// SCN-SM-041-020 watch-adjacent safety-boundary defense-in-depth:
// the helper pre-checks the watch-proposal action type via
// EnforceQFActionBoundary BEFORE building or emitting the
// envelope. In MVP `watch_proposal` is NOT in the forbidden action
// vocabulary so the boundary helper is a no-op; any future change
// adding `watch_proposal` to IsForbiddenQFActionType will force the
// envelope outcome to AuditOutcomeRejected.
func EmitWatchProposalAttemptAudit(input WatchProposalAttemptAuditInput) EvidenceAuditEnvelope {
	reason := strings.TrimSpace(input.Reason)
	outcome := auditOutcomeForStatus(input.Status)
	if _, fired, _ := EnforceQFActionBoundary(ActionBoundaryAttempt{
		AttemptedActionType: AuditActionWatchProposalAttempt,
		TraceID:             input.TraceID,
		ActorRef:            input.ActorRef,
		Surface:             input.Surface,
		Reason:              "watch_proposal_action_rejected",
		ObservedAt:          input.ObservedAt,
	}); fired {
		outcome = AuditOutcomeRejected
	}
	envelope := BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:       input.TraceID,
		ActorRef:      input.ActorRef,
		Surface:       input.Surface,
		Action:        AuditActionWatchProposalAttempt,
		Outcome:       outcome,
		Reason:        reason,
		TargetContext: input.EntityRef,
		ObservedAt:    input.ObservedAt,
	})
	EmitConnectorAuditEnvelope(envelope)
	return envelope
}

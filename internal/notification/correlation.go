package notification

import (
	"fmt"
	"strings"
	"time"
)

const (
	SuppressionDedupe       = "dedupe"
	SuppressionReactionLoop = "reaction_loop"
	CorrelationExact        = "exact_duplicate"
	CorrelationNear         = "near_duplicate"
	CorrelationSameSubject  = "same_subject"
	CorrelationSameService  = "same_service"
	ActorSystem             = "system"
	ActorUser               = "user"
	ActorOperator           = "operator"
)

type Deduper struct {
	window time.Duration
}

func NewDeduper(window time.Duration) Deduper {
	return Deduper{window: window}
}

func (d Deduper) Evaluate(current NormalizedNotification, prior []NormalizedNotification, incidentID string, now time.Time) *Suppression {
	for _, candidate := range prior {
		withinWindow := d.window == 0 || current.ObservedAt.Sub(candidate.ObservedAt) <= d.window
		if withinWindow && current.SourceInstanceID == candidate.SourceInstanceID && current.SourceEventID == candidate.SourceEventID && current.PayloadHash == candidate.PayloadHash {
			return &Suppression{ID: "supp_" + strings.TrimPrefix(hashParts("suppression", current.ID, candidate.ID), "sha256:"), NotificationID: current.ID, IncidentID: incidentID, SourceInstanceID: current.SourceInstanceID, Kind: SuppressionDedupe, Scope: map[string]any{"original_notification_id": candidate.ID}, Reason: "duplicate source event suppressed without deleting raw history", StartsAt: now, CreatedAt: now}
		}
	}
	return nil
}

func (s Suppression) AuditPreservesRawHistory() bool {
	return s.Kind == SuppressionDedupe || s.Kind == SuppressionReactionLoop
}

type Correlation struct {
	Kind      string
	Score     float64
	Rationale string
}

type CorrelationResult struct {
	Incident    Incident
	Correlation Correlation
	Created     bool
}

type Correlator struct{}

func NewCorrelator() Correlator {
	return Correlator{}
}

func (Correlator) Correlate(notification NormalizedNotification, classification Classification, incidents []Incident, now time.Time) CorrelationResult {
	key := IncidentKey(notification)
	for _, incident := range incidents {
		if incident.IncidentKey == key || sameServiceIncident(notification, incident) {
			incident.PersistenceCount++
			incident.LastEventAt = now
			incident.Severity = maxSeverity(incident.Severity, classification.Severity)
			incident.SourceInstanceIDs = appendUnique(incident.SourceInstanceIDs, notification.SourceInstanceID)
			return CorrelationResult{Incident: incident, Correlation: Correlation{Kind: CorrelationSameService, Score: 0.91, Rationale: "normalized service, subject, domain, and intent matched active incident"}}
		}
	}
	state := IncidentObserving
	if severityRank(classification.Severity) >= severityRank(SeverityHigh) {
		state = IncidentActive
	}
	incident := Incident{ID: "inc_" + strings.TrimPrefix(hashParts("incident", key), "sha256:"), IncidentKey: key, State: state, Title: notification.Title, Subject: notification.Subject, Service: notification.Service, Severity: classification.Severity, Domain: classification.Domain, Intent: classification.Intent, RiskLevel: riskFromClassification(classification.Severity, classification.Intent), FirstEventAt: notification.ObservedAt, LastEventAt: notification.ObservedAt, PersistenceCount: 1, SourceInstanceIDs: []string{notification.SourceInstanceID}, StateReason: "created from normalized notification correlation", RedactionState: cloneAnyMap(notification.RedactionState), CreatedAt: now, UpdatedAt: now}
	return CorrelationResult{Incident: incident, Correlation: Correlation{Kind: CorrelationSameSubject, Score: 0.75, Rationale: "created incident from normalized subject/domain/intent"}, Created: true}
}

func IncidentKey(notification NormalizedNotification) string {
	return hashParts("incident-key", string(notification.Domain), strings.ToLower(firstNonEmpty(notification.Service, notification.Subject)), string(notification.Intent))
}

type TransitionCause struct {
	ActorKind string
	ActorRef  string
	Rationale string
	At        time.Time
}

type StateTransitionResult struct {
	IncidentTransition
	Refused bool
}

type IncidentStateMachine struct{}

func NewIncidentStateMachine() IncidentStateMachine {
	return IncidentStateMachine{}
}

func (IncidentStateMachine) Transition(incident Incident, next IncidentState, cause TransitionCause) (StateTransitionResult, Incident, error) {
	if cause.At.IsZero() {
		return StateTransitionResult{}, incident, fmt.Errorf("incident transition: timestamp is required")
	}
	from := incident.State
	transition := StateTransitionResult{IncidentTransition: IncidentTransition{ID: "trans_" + strings.TrimPrefix(hashParts("transition", incident.ID, string(from), string(next), cause.At.Format(time.RFC3339Nano)), "sha256:"), IncidentID: incident.ID, FromState: &from, ToState: next, ActorKind: cause.ActorKind, ActorRef: cause.ActorRef, Rationale: cause.Rationale, CreatedAt: cause.At}}
	if !validIncidentTransition(from, next) {
		transition.Refused = true
		return transition, incident, fmt.Errorf("incident transition %s -> %s is invalid", from, next)
	}
	incident.State = next
	incident.UpdatedAt = cause.At
	return transition, incident, nil
}

func sameServiceIncident(notification NormalizedNotification, incident Incident) bool {
	return firstNonEmpty(notification.Service, notification.Subject) != "" && strings.EqualFold(firstNonEmpty(notification.Service, notification.Subject), firstNonEmpty(incident.Service, incident.Subject)) && notification.Domain == incident.Domain && notification.Intent == incident.Intent
}

func appendUnique(values []string, next string) []string {
	for _, value := range values {
		if value == next {
			return values
		}
	}
	return append(values, next)
}

func maxSeverity(left Severity, right Severity) Severity {
	if severityRank(right) > severityRank(left) {
		return right
	}
	return left
}

func riskFromSeverity(severity Severity) RiskLevel {
	if severityRank(severity) >= severityRank(SeverityHigh) {
		return RiskMedium
	}
	return RiskLow
}

func riskFromClassification(severity Severity, intent Intent) RiskLevel {
	if intent == IntentApproval {
		return RiskHigh
	}
	return riskFromSeverity(severity)
}

func validIncidentTransition(from IncidentState, to IncidentState) bool {
	allowed := map[IncidentState][]IncidentState{
		IncidentObserving:         {IncidentActive, IncidentSuppressed, IncidentResolved},
		IncidentActive:            {IncidentDiagnosing, IncidentApprovalRequested, IncidentEscalated, IncidentMitigating, IncidentSuppressed, IncidentResolved},
		IncidentDiagnosing:        {IncidentActive, IncidentEscalated, IncidentMitigating, IncidentResolved},
		IncidentMitigating:        {IncidentActive, IncidentResolved, IncidentEscalated},
		IncidentApprovalRequested: {IncidentMitigating, IncidentEscalated, IncidentResolved},
		IncidentEscalated:         {IncidentActive, IncidentResolved, IncidentSuppressed},
		IncidentSuppressed:        {IncidentActive, IncidentResolved},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

//go:build ignore

package notification

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	SuppressionDedupe       = "dedupe"
	SuppressionReactionLoop = "reaction_loop"
	CorrelationSameService  = "same_service"
	CorrelationExact        = "exact_duplicate"
)

type Deduper struct {
	window time.Duration
}

func NewDeduper(window time.Duration) Deduper {
	return Deduper{window: window}
}

func (d Deduper) Evaluate(current NormalizedNotification, prior []NormalizedNotification, incidentID string, now time.Time) *Suppression {
	for _, candidate := range prior {
		if d.sameDuplicate(current, candidate, now) {
			return &Suppression{ID: "notif_supp_" + uuid.NewString(), NotificationID: current.ID, IncidentID: incidentID, SourceInstanceID: current.SourceInstanceID, Kind: SuppressionDedupe, Scope: map[string]any{"matched_notification_id": candidate.ID}, Reason: "duplicate notification inside cooldown window", StartsAt: now, CreatedAt: now}
		}
	}
	return nil
}

func (d Deduper) sameDuplicate(current NormalizedNotification, candidate NormalizedNotification, now time.Time) bool {
	if current.PayloadHash == "" || candidate.PayloadHash == "" || current.PayloadHash != candidate.PayloadHash {
		return false
	}
	if current.SourceInstanceID != candidate.SourceInstanceID || current.Subject != candidate.Subject {
		return false
	}
	return d.window <= 0 || now.Sub(candidate.ObservedAt) <= d.window
}

type CorrelationResult struct {
	Incident    Incident
	Correlation IncidentEventLink
}

type Correlator struct{}

func NewCorrelator() Correlator { return Correlator{} }

func (c Correlator) Correlate(notification NormalizedNotification, classification Classification, active []Incident, now time.Time) CorrelationResult {
	key := IncidentKey(notification)
	for _, incident := range active {
		if incident.IncidentKey == key || sameServiceIncident(incident, notification) {
			updated := incident
			updated.PersistenceCount++
			updated.LastEventAt = now
			updated.SourceInstanceIDs = appendMissingString(updated.SourceInstanceIDs, notification.SourceInstanceID)
			if severityRank(classification.Severity) > severityRank(updated.Severity) {
				updated.Severity = classification.Severity
			}
			return CorrelationResult{Incident: updated, Correlation: IncidentEventLink{IncidentID: updated.ID, NotificationID: notification.ID, CorrelationKind: CorrelationSameService, CorrelationScore: 0.92, Rationale: "same normalized service/subject/domain/intent", CreatedAt: now}}
		}
	}
	state := IncidentObserving
	if severityRank(classification.Severity) >= severityRank(SeverityHigh) {
		state = IncidentActive
	}
	incident := Incident{ID: "notif_inc_" + uuid.NewString(), IncidentKey: key, State: state, Title: notification.Title, Subject: notification.Subject, Service: notification.Service, Severity: classification.Severity, Domain: classification.Domain, Intent: classification.Intent, RiskLevel: RiskUnknown, FirstEventAt: notification.ObservedAt, LastEventAt: now, PersistenceCount: 1, SourceInstanceIDs: []string{notification.SourceInstanceID}, StateReason: "created from normalized notification", RedactionState: cloneAnyMap(notification.RedactionState), CreatedAt: now, UpdatedAt: now}
	return CorrelationResult{Incident: incident, Correlation: IncidentEventLink{IncidentID: incident.ID, NotificationID: notification.ID, CorrelationKind: "same_subject", CorrelationScore: 0.8, Rationale: "created incident from normalized notification", CreatedAt: now}}
}

func IncidentKey(notification NormalizedNotification) string {
	return hashParts(string(notification.Domain), firstNonEmpty(notification.Service, notification.Subject), string(notification.Intent))
}

func sameServiceIncident(incident Incident, notification NormalizedNotification) bool {
	return incident.Service != "" && incident.Service == notification.Service && incident.Domain == notification.Domain && incident.Intent == notification.Intent
}

func appendMissingString(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

const (
	ActorSystem = "system"
	ActorUser   = "user"
)

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

func NewIncidentStateMachine() IncidentStateMachine { return IncidentStateMachine{} }

func (m IncidentStateMachine) Transition(incident Incident, to IncidentState, cause TransitionCause) (StateTransitionResult, Incident, error) {
	if cause.At.IsZero() {
		return StateTransitionResult{}, incident, fmt.Errorf("incident transition: timestamp is required")
	}
	from := incident.State
	transition := StateTransitionResult{IncidentTransition: IncidentTransition{ID: "notif_trans_" + uuid.NewString(), IncidentID: incident.ID, FromState: &from, ToState: to, ActorKind: cause.ActorKind, ActorRef: cause.ActorRef, Rationale: cause.Rationale, CreatedAt: cause.At}}
	if !validIncidentTransition(from, to) {
		transition.Refused = true
		transition.Rationale = strings.TrimSpace("refused invalid transition: " + cause.Rationale)
		return transition, incident, fmt.Errorf("incident transition %s -> %s is invalid", from, to)
	}
	updated := incident
	updated.State = to
	updated.UpdatedAt = cause.At
	return transition, updated, nil
}

func validIncidentTransition(from IncidentState, to IncidentState) bool {
	if from == to {
		return true
	}
	allowed := map[IncidentState][]IncidentState{
		IncidentObserving:         {IncidentActive, IncidentSuppressed, IncidentResolved},
		IncidentActive:            {IncidentDiagnosing, IncidentMitigating, IncidentApprovalRequested, IncidentEscalated, IncidentSuppressed, IncidentResolved},
		IncidentDiagnosing:        {IncidentActive, IncidentMitigating, IncidentEscalated, IncidentResolved},
		IncidentMitigating:        {IncidentActive, IncidentResolved, IncidentEscalated},
		IncidentApprovalRequested: {IncidentMitigating, IncidentEscalated, IncidentResolved},
		IncidentEscalated:         {IncidentDiagnosing, IncidentMitigating, IncidentResolved, IncidentSuppressed},
		IncidentSuppressed:        {IncidentActive, IncidentResolved},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

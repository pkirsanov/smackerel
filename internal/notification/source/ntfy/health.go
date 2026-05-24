package ntfy

import (
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

const (
	SubscriptionConnected    = "connected"
	SubscriptionReconnecting = "reconnecting"
	SubscriptionDisconnected = "disconnected"
	SubscriptionStalled      = "stalled"
	SubscriptionDisabled     = "disabled"
)

type SubscriptionState struct {
	SourceInstanceID      string
	Topic                 string
	SourceForm            notification.SourceForm
	TransportMode         string
	SubscriptionState     string
	LastNtfyEventID       string
	LastEventAt           *time.Time
	LastOpenAt            *time.Time
	LastKeepaliveAt       *time.Time
	LastSuccessfulCheckAt *time.Time
	LagSeconds            int
	PossibleGap           bool
	RetryCount            int
	RetryBudget           int
	LastErrorKind         string
	LastErrorRedacted     string
	RedactionState        map[string]any
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func AuthFailureHealth(cfg Config, observedAt time.Time) notification.SourceHealthReport {
	return disconnectedHealth(cfg, ErrorAuthFailed, observedAt)
}

func DeadLetterPressureHealth(cfg Config, observedAt time.Time) notification.SourceHealthReport {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	report := notification.SourceHealthReport{SourceType: SourceType, SourceInstanceID: cfg.SourceInstanceID, SourceForm: cfg.SourceForm, State: notification.SourceHealthDegraded, LastErrorKind: ErrorDeadLetterPressure, ObservedAt: observedAt}
	redacted, err := notification.RedactHealthReport(report)
	if err != nil {
		return disconnectedHealth(cfg, "source_error", observedAt)
	}
	return redacted
}

func HealthFromTopics(cfg Config, topics []SubscriptionState, observedAt time.Time) notification.SourceHealthReport {
	if len(topics) == 0 {
		return disconnectedHealth(cfg, "no_topic_health", observedAt)
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	state := notification.SourceHealthConnected
	retryCount := 0
	lastError := ""
	var lastEvent *time.Time
	var lastCheck *time.Time
	allDisconnected := true
	for _, topic := range topics {
		topic = FinalizeSubscriptionState(cfg, topic, observedAt)
		if topic.RetryCount > retryCount {
			retryCount = topic.RetryCount
		}
		if topic.LastErrorKind != "" {
			lastError = topic.LastErrorKind
		}
		lastEvent = maxTimePtr(lastEvent, topic.LastEventAt)
		lastCheck = maxTimePtr(lastCheck, topic.LastSuccessfulCheckAt)
		switch topic.SubscriptionState {
		case SubscriptionConnected:
			allDisconnected = false
		case SubscriptionReconnecting, SubscriptionStalled:
			state = notification.SourceHealthDegraded
			allDisconnected = false
		case SubscriptionDisconnected, SubscriptionDisabled:
			if state == notification.SourceHealthConnected {
				state = notification.SourceHealthDegraded
			}
		}
	}
	if allDisconnected {
		state = notification.SourceHealthDisconnected
	}
	report := notification.SourceHealthReport{SourceType: SourceType, SourceInstanceID: cfg.SourceInstanceID, SourceForm: cfg.SourceForm, State: state, LastEventAt: lastEvent, LastSuccessfulCheckAt: lastCheck, RetryCount: retryCount, LastErrorKind: lastError, ObservedAt: observedAt}
	redacted, err := notification.RedactHealthReport(report)
	if err != nil {
		return disconnectedHealth(cfg, "source_error", observedAt)
	}
	return redacted
}

func FinalizeSubscriptionState(cfg Config, state SubscriptionState, observedAt time.Time) SubscriptionState {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	state.LagSeconds = topicLagSeconds(state, observedAt)
	if state.SubscriptionState == SubscriptionConnected {
		if state.LagSeconds >= cfg.Lag.DisconnectedAfterSeconds {
			state.SubscriptionState = SubscriptionDisconnected
			state.PossibleGap = true
			state.LastErrorKind = ErrorRetryBudgetExhausted
			state.LastErrorRedacted = "source lag exceeded disconnected threshold"
		} else if state.LagSeconds >= cfg.Lag.DegradedAfterSeconds {
			state.SubscriptionState = SubscriptionStalled
			state.PossibleGap = true
			state.LastErrorKind = ErrorConnectivityFailed
			state.LastErrorRedacted = "source lag exceeded degraded threshold"
		}
	}
	return state
}

func topicLagSeconds(state SubscriptionState, observedAt time.Time) int {
	latest := maxTimePtr(nil, state.LastEventAt)
	latest = maxTimePtr(latest, state.LastSuccessfulCheckAt)
	latest = maxTimePtr(latest, state.LastOpenAt)
	latest = maxTimePtr(latest, state.LastKeepaliveAt)
	if latest == nil || observedAt.Before(*latest) {
		return 0
	}
	return int(observedAt.Sub(*latest).Seconds())
}

func disconnectedHealth(cfg Config, kind string, observedAt time.Time) notification.SourceHealthReport {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	report := notification.SourceHealthReport{SourceType: SourceType, SourceInstanceID: cfg.SourceInstanceID, SourceForm: cfg.SourceForm, State: notification.SourceHealthDisconnected, LastErrorKind: kind, ObservedAt: observedAt}
	redacted, err := notification.RedactHealthReport(report)
	if err != nil {
		report.LastErrorRedacted = "source health check failed"
		return report
	}
	return redacted
}

func maxTimePtr(left *time.Time, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if right.After(*left) {
		return right
	}
	return left
}

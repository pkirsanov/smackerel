package notification

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Classification = ClassificationRecord

type ClassificationContext struct {
	KnownServices []string
	GraphRefs     []string
}

type Classifier struct {
	version string
}

func NewClassifier(version string) Classifier {
	return Classifier{version: strings.TrimSpace(version)}
}

func (c Classifier) Classify(notification NormalizedNotification, ctx ClassificationContext) (Classification, error) {
	if notification.ID == "" {
		return Classification{}, fmt.Errorf("classify notification: notification id is required")
	}
	version := c.version
	if version == "" {
		return Classification{}, fmt.Errorf("classify notification: classifier version is required")
	}
	text := strings.ToLower(notification.Title + " " + notification.Body + " " + notification.Subject + " " + notification.Service)
	severity, severityPolicy := reconcileSeverity(notification.Severity, notification.SourceSeverity, text)
	domain := notification.Domain
	if domain == DomainUnknown {
		domain = classifyDomain(text)
	}
	intent := notification.Intent
	if intent == IntentUnknown {
		intent = classifyIntent(text)
	}
	confidence := 0.62
	signals := map[string]any{"text_terms": textSignalSummary(text), "source_severity_policy": severityPolicy}
	uncertainty := map[string]any{}
	if knownService(notification.Service, ctx.KnownServices) {
		confidence += 0.22
		signals["known_service"] = notification.Service
	} else {
		uncertainty["service_context"] = "context_unavailable"
		confidence -= 0.18
	}
	if severity == SeverityUnknown || domain == DomainUnknown || intent == IntentUnknown {
		uncertainty["classification"] = "insufficient_evidence"
		confidence -= 0.12
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return Classification{
		ID:                   "notif_class_" + uuid.NewString(),
		NotificationID:       notification.ID,
		Severity:             severity,
		Domain:               domain,
		Intent:               intent,
		Confidence:           confidence,
		SourceSeverityPolicy: severityPolicy,
		Signals:              signals,
		Rationale:            fmt.Sprintf("classified from normalized title/body/subject/service with %d uncertainty signals", len(uncertainty)),
		Uncertainty:          uncertainty,
		ClassifierVersion:    version,
		CreatedAt:            time.Now().UTC(),
	}, nil
}

func reconcileSeverity(current Severity, sourceSeverity string, text string) (Severity, string) {
	ruleSeverity := severityFromText(text)
	source := ParseSeverity(sourceSeverity)
	if current != SeverityUnknown {
		return current, "accepted"
	}
	if ruleSeverity != SeverityUnknown {
		if source != SeverityUnknown && severityRank(ruleSeverity) > severityRank(source) {
			return ruleSeverity, "upgraded"
		}
		if source != SeverityUnknown && severityRank(ruleSeverity) < severityRank(source) {
			return ruleSeverity, "downgraded"
		}
		return ruleSeverity, "none"
	}
	if source != SeverityUnknown {
		return source, "accepted"
	}
	return SeverityUnknown, "none"
}

func severityFromText(text string) Severity {
	switch {
	case strings.Contains(text, "critical") || strings.Contains(text, "outage") || strings.Contains(text, " down"):
		return SeverityCritical
	case strings.Contains(text, "high") || strings.Contains(text, "threshold") || strings.Contains(text, "failed"):
		return SeverityHigh
	case strings.Contains(text, "warning") || strings.Contains(text, "investigate"):
		return SeverityMedium
	case strings.Contains(text, "routine") || strings.Contains(text, "complete"):
		return SeverityLow
	default:
		return SeverityUnknown
	}
}

func classifyDomain(text string) Domain {
	switch {
	case strings.Contains(text, "invoice") || strings.Contains(text, "payment"):
		return DomainFinance
	case strings.Contains(text, "travel") || strings.Contains(text, "booking"):
		return DomainTravel
	case strings.Contains(text, "service") || strings.Contains(text, "api") || strings.Contains(text, "host") || strings.Contains(text, "outage"):
		return DomainOps
	case strings.Contains(text, "system"):
		return DomainSystem
	default:
		return DomainUnknown
	}
}

func classifyIntent(text string) Intent {
	switch {
	case strings.Contains(text, "recover") || strings.Contains(text, "resolved"):
		return IntentRecovery
	case strings.Contains(text, "approve") || strings.Contains(text, "approval"):
		return IntentApproval
	case strings.Contains(text, "outage") || strings.Contains(text, " down"):
		return IntentOutage
	case strings.Contains(text, "mitigat") || strings.Contains(text, "restart"):
		return IntentMitigation
	case strings.Contains(text, "investigate") || strings.Contains(text, "threshold") || strings.Contains(text, "failed"):
		return IntentInvestigate
	case strings.Contains(text, "routine") || strings.Contains(text, "complete"):
		return IntentRoutine
	default:
		return IntentUnknown
	}
}

func textSignalSummary(text string) []string {
	terms := []string{}
	for _, term := range []string{"critical", "outage", "threshold", "failed", "routine", "complete", "approval"} {
		if strings.Contains(text, term) {
			terms = append(terms, term)
		}
	}
	return terms
}

func knownService(service string, known []string) bool {
	service = strings.TrimSpace(service)
	if service == "" {
		return false
	}
	for _, candidate := range known {
		if candidate == service {
			return true
		}
	}
	return false
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityInfo:
		return 1
	case SeverityLow:
		return 2
	case SeverityMedium:
		return 3
	case SeverityHigh:
		return 4
	case SeverityCritical:
		return 5
	default:
		return 0
	}
}

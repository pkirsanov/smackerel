//go:build ignore

package notification

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	EnrichmentArtifact = "artifact"
)

type EnrichmentContext struct {
	ArtifactRefs   []string
	KnownServices  []string
	MaintenanceRef string
}

type Enricher struct{}

func NewEnricher() Enricher { return Enricher{} }

func (e Enricher) Enrich(notification NormalizedNotification, incident Incident, ctx EnrichmentContext) []EnrichmentRef {
	now := time.Now().UTC()
	refs := []EnrichmentRef{}
	for _, artifactID := range ctx.ArtifactRefs {
		confidence := 0.8
		refs = append(refs, EnrichmentRef{ID: "notif_enrich_" + uuid.NewString(), NotificationID: notification.ID, IncidentID: incident.ID, RefType: EnrichmentArtifact, RefID: artifactID, SignalKind: "related_artifact", Confidence: &confidence, UsedInDecision: true, CreatedAt: now})
	}
	if len(refs) == 0 {
		refs = append(refs, EnrichmentRef{ID: "notif_enrich_" + uuid.NewString(), NotificationID: notification.ID, IncidentID: incident.ID, RefType: "sync_state", RefID: firstNonEmpty(notification.Service, notification.Subject), SignalKind: "missing_context", MissingContextReason: "context_unavailable", UsedInDecision: false, CreatedAt: now})
	}
	return refs
}

type ActionPolicy struct {
	Risk        RiskLevel
	ActionClass ActionClass
	Destructive bool
}

type DecisionPolicy struct {
	PersistenceThreshold   int
	EscalationSeverity     Severity
	LowConfidenceThreshold float64
	AutonomousActions      map[string]ActionPolicy
	OutputChannels         []string
	MaxRetries             int
}

type Decision struct {
	Type           DecisionType
	ReasonCodes    []string
	Rationale      string
	RequiresOutput bool
	RequiresAction bool
	Alternatives   []DecisionType
}

type DecisionEngine struct {
	policy DecisionPolicy
}

func NewDecisionEngine(policy DecisionPolicy) (DecisionEngine, error) {
	if err := policy.Validate(); err != nil {
		return DecisionEngine{}, err
	}
	return DecisionEngine{policy: policy}, nil
}

func (p DecisionPolicy) Validate() error {
	var missing []string
	if p.PersistenceThreshold < 1 {
		missing = append(missing, "NOTIFICATION_PERSISTENCE_THRESHOLD")
	}
	if !validEscalationSeverity(string(p.EscalationSeverity)) {
		missing = append(missing, "NOTIFICATION_ESCALATION_SEVERITY")
	}
	if p.LowConfidenceThreshold <= 0 || p.LowConfidenceThreshold > 1 {
		missing = append(missing, "NOTIFICATION_LOW_CONFIDENCE_THRESHOLD")
	}
	if p.MaxRetries < 1 {
		missing = append(missing, "NOTIFICATION_MAX_RETRIES")
	}
	if len(p.OutputChannels) == 0 {
		missing = append(missing, "NOTIFICATION_OUTPUT_CHANNELS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("notification decision policy missing or invalid required values: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (e DecisionEngine) Decide(notification NormalizedNotification, classification Classification, incident Incident, enrichments []EnrichmentRef, suppressions []Suppression) Decision {
	if len(suppressions) > 0 {
		return Decision{Type: DecisionNoAction, ReasonCodes: []string{"suppressed"}, Rationale: "suppression policy already explains this notification"}
	}
	if classification.Confidence < e.policy.LowConfidenceThreshold && severityRank(classification.Severity) >= severityRank(SeverityMedium) {
		return Decision{Type: DecisionDiagnostics, ReasonCodes: []string{"classification_uncertain"}, Rationale: "diagnostics selected to reduce uncertainty"}
	}
	if classification.Intent == IntentRoutine || classification.Severity == SeverityLow || classification.Severity == SeverityInfo {
		return Decision{Type: DecisionRecordOnly, ReasonCodes: []string{"routine_or_low_severity"}, Rationale: "routine notification stays silent"}
	}
	if incident.RiskLevel == RiskHigh {
		return Decision{Type: DecisionApprovalRequest, ReasonCodes: []string{"risk_requires_approval"}, Rationale: "high risk action requires user approval", RequiresOutput: true}
	}
	if severityRank(classification.Severity) >= severityRank(e.policy.EscalationSeverity) || incident.PersistenceCount >= e.policy.PersistenceThreshold {
		return Decision{Type: DecisionUserEscalation, ReasonCodes: []string{"threshold_crossed"}, Rationale: "severity or persistence threshold crossed", RequiresOutput: true}
	}
	return Decision{Type: DecisionRecordOnly, ReasonCodes: []string{"below_threshold"}, Rationale: "notification recorded for history"}
}

func DecisionToRecord(notification NormalizedNotification, incident Incident, decision Decision, now time.Time) ProcessingDecision {
	return ProcessingDecision{ID: "notif_decision_" + uuid.NewString(), NotificationID: notification.ID, IncidentID: incident.ID, DecisionType: decision.Type, ReasonCodes: append([]string(nil), decision.ReasonCodes...), ThresholdInputs: map[string]any{"severity": notification.Severity, "persistence_count": incident.PersistenceCount}, RiskAssessment: map[string]any{"risk_level": incident.RiskLevel}, Rationale: decision.Rationale, CreatedAt: now}
}

type NotificationConfig struct {
	Enabled                bool
	PersistenceThreshold   int
	EscalationSeverity     Severity
	LowConfidenceThreshold float64
	MaxRetries             int
	OutputChannels         []string
}

func LoadNotificationConfig(env map[string]string) (NotificationConfig, error) {
	enabledRaw, ok := env["NOTIFICATION_INTELLIGENCE_ENABLED"]
	if !ok || enabledRaw == "" {
		return NotificationConfig{}, fmt.Errorf("NOTIFICATION_INTELLIGENCE_ENABLED is required")
	}
	if enabledRaw != "true" && enabledRaw != "false" {
		return NotificationConfig{}, fmt.Errorf("NOTIFICATION_INTELLIGENCE_ENABLED must be true or false")
	}
	cfg := NotificationConfig{Enabled: enabledRaw == "true"}
	if !cfg.Enabled {
		return cfg, nil
	}
	var errs []string
	if v := env["NOTIFICATION_PERSISTENCE_THRESHOLD"]; v == "" {
		errs = append(errs, "NOTIFICATION_PERSISTENCE_THRESHOLD")
	} else if parsed, err := strconv.Atoi(v); err != nil || parsed < 1 {
		errs = append(errs, "NOTIFICATION_PERSISTENCE_THRESHOLD")
	} else {
		cfg.PersistenceThreshold = parsed
	}
	if v := env["NOTIFICATION_ESCALATION_SEVERITY"]; v == "" || !validEscalationSeverity(v) {
		errs = append(errs, "NOTIFICATION_ESCALATION_SEVERITY")
	} else {
		cfg.EscalationSeverity = ParseSeverity(v)
	}
	if v := env["NOTIFICATION_LOW_CONFIDENCE_THRESHOLD"]; v == "" {
		errs = append(errs, "NOTIFICATION_LOW_CONFIDENCE_THRESHOLD")
	} else if parsed, err := strconv.ParseFloat(v, 64); err != nil || parsed <= 0 || parsed > 1 {
		errs = append(errs, "NOTIFICATION_LOW_CONFIDENCE_THRESHOLD")
	} else {
		cfg.LowConfidenceThreshold = parsed
	}
	if v := env["NOTIFICATION_MAX_RETRIES"]; v == "" {
		errs = append(errs, "NOTIFICATION_MAX_RETRIES")
	} else if parsed, err := strconv.Atoi(v); err != nil || parsed < 1 {
		errs = append(errs, "NOTIFICATION_MAX_RETRIES")
	} else {
		cfg.MaxRetries = parsed
	}
	if v := env["NOTIFICATION_OUTPUT_CHANNELS"]; v == "" {
		errs = append(errs, "NOTIFICATION_OUTPUT_CHANNELS")
	} else {
		for _, channel := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(channel); trimmed != "" {
				cfg.OutputChannels = append(cfg.OutputChannels, trimmed)
			}
		}
		if len(cfg.OutputChannels) == 0 {
			errs = append(errs, "NOTIFICATION_OUTPUT_CHANNELS")
		}
	}
	if len(errs) > 0 {
		return NotificationConfig{}, fmt.Errorf("missing or invalid required notification configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

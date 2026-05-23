package notification

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	EnrichmentArtifact        = "artifact"
	EnrichmentKnowledgeEntity = "knowledge_entity"
	EnrichmentTopic           = "topic"
)

type EnrichmentContext struct {
	ArtifactRefs   []string
	KnownServices  []string
	PriorIncidents []string
}

type Enricher struct{}

func NewEnricher() Enricher {
	return Enricher{}
}

func (Enricher) Enrich(notification NormalizedNotification, incident Incident, context EnrichmentContext) []EnrichmentRef {
	refs := []EnrichmentRef{}
	for _, artifactID := range context.ArtifactRefs {
		confidence := 0.8
		refs = append(refs, EnrichmentRef{ID: "enrich_" + strings.TrimPrefix(hashParts("artifact", notification.ID, artifactID), "sha256:"), NotificationID: notification.ID, IncidentID: incident.ID, RefType: EnrichmentArtifact, RefID: artifactID, SignalKind: "subject_match", Confidence: &confidence, UsedInDecision: true})
	}
	if len(context.ArtifactRefs) == 0 && len(context.KnownServices) == 0 && len(context.PriorIncidents) == 0 {
		refs = append(refs, EnrichmentRef{ID: "enrich_" + strings.TrimPrefix(hashParts("missing", notification.ID, incident.ID), "sha256:"), NotificationID: notification.ID, IncidentID: incident.ID, RefType: EnrichmentKnowledgeEntity, RefID: "unavailable", SignalKind: "missing_context", UsedInDecision: false, MissingContextReason: "context_unavailable"})
	}
	return refs
}

type DecisionPolicy struct {
	PersistenceThreshold   int
	EscalationSeverity     Severity
	LowConfidenceThreshold float64
	AutonomousActions      map[string]ActionPolicy
	OutputChannels         []string
	MaxRetries             int
}

type DecisionEngine struct {
	policy DecisionPolicy
}

func NewDecisionEngine(policy DecisionPolicy) (DecisionEngine, error) {
	if policy.PersistenceThreshold < 1 {
		return DecisionEngine{}, fmt.Errorf("decision policy: persistence threshold must be positive")
	}
	if severityRank(policy.EscalationSeverity) == 0 {
		return DecisionEngine{}, fmt.Errorf("decision policy: escalation severity is required")
	}
	if policy.LowConfidenceThreshold <= 0 || policy.LowConfidenceThreshold > 1 {
		return DecisionEngine{}, fmt.Errorf("decision policy: low confidence threshold must be in (0,1]")
	}
	if policy.MaxRetries < 0 {
		return DecisionEngine{}, fmt.Errorf("decision policy: max retries must be non-negative")
	}
	if len(policy.OutputChannels) == 0 {
		return DecisionEngine{}, fmt.Errorf("decision policy: at least one output channel is required")
	}
	return DecisionEngine{policy: policy}, nil
}

func (e DecisionEngine) Decide(notification NormalizedNotification, classification Classification, incident Incident, enrichments []EnrichmentRef, suppressions []Suppression) DecisionEvaluation {
	decision := DecisionEvaluation{ID: "decision_" + strings.TrimPrefix(hashParts("decision", notification.ID, incident.ID, strconv.Itoa(incident.PersistenceCount)), "sha256:"), NotificationID: notification.ID, IncidentID: incident.ID, ThresholdInputs: map[string]any{"persistence_count": incident.PersistenceCount, "classification_confidence": classification.Confidence}, RiskAssessment: map[string]any{"risk_level": incident.RiskLevel}, Rationale: "decision chosen from severity, persistence, uncertainty, risk, suppressions, and enrichment references"}
	if len(suppressions) > 0 {
		decision.Type = DecisionNoAction
		decision.ReasonCodes = []string{"suppressed"}
		return decision
	}
	if classification.Intent == IntentRoutine || severityRank(classification.Severity) <= severityRank(SeverityLow) {
		decision.Type = DecisionRecordOnly
		decision.ReasonCodes = []string{"routine_or_low_severity"}
		return decision
	}
	if classification.Confidence < e.policy.LowConfidenceThreshold {
		decision.Type = DecisionDiagnostics
		decision.ReasonCodes = []string{"low_confidence"}
		decision.RequiresDiagnostics = true
		return decision
	}
	if incident.RiskLevel == RiskHigh {
		decision.Type = DecisionApprovalRequest
		decision.ReasonCodes = []string{"high_blast_radius"}
		decision.RequiresOutput = true
		decision.RequiresApproval = true
		return decision
	}
	if severityRank(classification.Severity) >= severityRank(e.policy.EscalationSeverity) || incident.PersistenceCount >= e.policy.PersistenceThreshold {
		decision.Type = DecisionUserEscalation
		decision.ReasonCodes = []string{"threshold_crossed"}
		decision.RequiresOutput = true
		return decision
	}
	decision.Type = DecisionRecordOnly
	decision.ReasonCodes = []string{"below_threshold"}
	return decision
}

type DecisionEvaluation struct {
	ID                  string
	NotificationID      string
	IncidentID          string
	Type                DecisionType
	ReasonCodes         []string
	ThresholdInputs     map[string]any
	RiskAssessment      map[string]any
	Rationale           string
	RequiresDiagnostics bool
	RequiresAction      bool
	RequiresApproval    bool
	RequiresOutput      bool
	Alternatives        []DecisionType
}

func (d DecisionEvaluation) Record() ProcessingDecision {
	return ProcessingDecision{ID: d.ID, NotificationID: d.NotificationID, IncidentID: d.IncidentID, DecisionType: d.Type, ReasonCodes: append([]string(nil), d.ReasonCodes...), ThresholdInputs: cloneAnyMap(d.ThresholdInputs), RiskAssessment: cloneAnyMap(d.RiskAssessment), Rationale: d.Rationale}
}

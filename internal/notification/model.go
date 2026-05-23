package notification

import "time"

type RawPayloadKind string

const (
	RawPayloadJSON            RawPayloadKind = "json"
	RawPayloadText            RawPayloadKind = "text"
	RawPayloadBytes           RawPayloadKind = "bytes"
	RawPayloadHeadersBody     RawPayloadKind = "headers_body"
	RawPayloadFileRef         RawPayloadKind = "file_ref"
	RawPayloadKindJSON                       = string(RawPayloadJSON)
	RawPayloadKindText                       = string(RawPayloadText)
	RawPayloadKindBytes                      = string(RawPayloadBytes)
	RawPayloadKindHeadersBody                = string(RawPayloadHeadersBody)
	RawPayloadKindFileRef                    = string(RawPayloadFileRef)
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
	SeverityUnknown  Severity = "unknown"
)

type Domain string

const (
	DomainOps      Domain = "ops"
	DomainFinance  Domain = "finance"
	DomainTravel   Domain = "travel"
	DomainPersonal Domain = "personal"
	DomainSystem   Domain = "system"
	DomainUnknown  Domain = "unknown"
)

type Intent string

const (
	IntentRoutine     Intent = "routine"
	IntentInvestigate Intent = "investigate"
	IntentOutage      Intent = "outage"
	IntentRecovery    Intent = "recovery"
	IntentMitigation  Intent = "mitigation"
	IntentApproval    Intent = "approval"
	IntentUnknown     Intent = "unknown"
)

type IncidentState string

const (
	IncidentObserving              IncidentState = "observing"
	IncidentActive                 IncidentState = "active"
	IncidentDiagnosing             IncidentState = "diagnosing"
	IncidentMitigating             IncidentState = "mitigating"
	IncidentApprovalRequested      IncidentState = "approval_requested"
	IncidentEscalated              IncidentState = "escalated"
	IncidentSuppressed             IncidentState = "suppressed"
	IncidentResolved               IncidentState = "resolved"
	IncidentStateObserving                       = IncidentObserving
	IncidentStateActive                          = IncidentActive
	IncidentStateDiagnosing                      = IncidentDiagnosing
	IncidentStateMitigating                      = IncidentMitigating
	IncidentStateApprovalRequested               = IncidentApprovalRequested
	IncidentStateEscalated                       = IncidentEscalated
	IncidentStateSuppressed                      = IncidentSuppressed
	IncidentStateResolved                        = IncidentResolved
)

type DecisionType string

const (
	DecisionNoAction           DecisionType = "no_action"
	DecisionRecordOnly         DecisionType = "record_only"
	DecisionDiagnostics        DecisionType = "diagnostics"
	DecisionAutonomousHandling DecisionType = "autonomous_handling"
	DecisionUserEscalation     DecisionType = "user_escalation"
	DecisionApprovalRequest    DecisionType = "approval_request"
)

type RiskLevel string

const (
	RiskLow     RiskLevel = "low"
	RiskMedium  RiskLevel = "medium"
	RiskHigh    RiskLevel = "high"
	RiskBlocked RiskLevel = "blocked"
	RiskUnknown RiskLevel = "unknown"
)

type ActionClass string

const (
	ActionClassReadOnlyDiagnostic ActionClass = "read_only_diagnostic"
	ActionClassLowRisk            ActionClass = "low_risk"
	ActionClassHighBlastRadius    ActionClass = "high_blast_radius"
	ActionClassDestructive        ActionClass = "destructive"
)

type ActionPolicy struct {
	Risk        RiskLevel
	ActionClass ActionClass
	Destructive bool
}

type RawEventRecord struct {
	ID                  string
	SourceType          string
	SourceInstanceID    string
	SourceForm          SourceForm
	SourceEventID       string
	SourceEventIDOrigin string
	ObservedAt          time.Time
	EventTimestamp      *time.Time
	PayloadHash         string
	RawPayloadKind      RawPayloadKind
	RawPayload          []byte
	PayloadSizeBytes    int
	DeliveryMetadata    map[string]string
	SourceSpecific      map[string]string
	RedactionState      map[string]any
	ValidationStatus    string
	ValidationErrors    []string
	LoopGuardKey        string
	CreatedAt           time.Time
}

type NormalizedNotification struct {
	ID                  string
	RawEventID          string
	SourceType          string
	SourceInstanceID    string
	SourceForm          SourceForm
	SourceEventID       string
	ObservedAt          time.Time
	EventTimestamp      *time.Time
	Title               string
	TitleDerivation     map[string]any
	Body                string
	BodyHash            string
	PayloadHash         string
	Severity            Severity
	SourceSeverity      string
	Tags                map[string][]string
	Subject             string
	Service             string
	Domain              Domain
	Intent              Intent
	CanonicalKey        string
	RawPayloadRef       string
	DeliveryMetadata    map[string]string
	SourceSpecificRef   map[string]any
	RedactionState      map[string]any
	NormalizationState  string
	NormalizationErrors []string
	CreatedAt           time.Time
}

type ClassificationRecord struct {
	ID                   string
	NotificationID       string
	Severity             Severity
	Domain               Domain
	Intent               Intent
	Confidence           float64
	SourceSeverityPolicy string
	Signals              map[string]any
	Rationale            string
	Uncertainty          map[string]any
	ClassifierVersion    string
	CreatedAt            time.Time
}

type Incident struct {
	ID                string
	IncidentKey       string
	State             IncidentState
	Title             string
	Subject           string
	Service           string
	Severity          Severity
	Domain            Domain
	Intent            Intent
	RiskLevel         RiskLevel
	FirstEventAt      time.Time
	LastEventAt       time.Time
	PersistenceCount  int
	SourceInstanceIDs []string
	StateReason       string
	RedactionState    map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ResolvedAt        *time.Time
}

type IncidentEventLink struct {
	IncidentID       string
	NotificationID   string
	CorrelationKind  string
	CorrelationScore float64
	Rationale        string
	CreatedAt        time.Time
}

type IncidentTransition struct {
	ID                       string
	IncidentID               string
	FromState                *IncidentState
	ToState                  IncidentState
	TriggeringNotificationID string
	DecisionID               string
	ActorKind                string
	ActorRef                 string
	Rationale                string
	CreatedAt                time.Time
}

type EnrichmentRef struct {
	ID                   string
	NotificationID       string
	IncidentID           string
	RefType              string
	RefID                string
	SignalKind           string
	Confidence           *float64
	UsedInDecision       bool
	MissingContextReason string
	CreatedAt            time.Time
}

type ProcessingDecision struct {
	ID              string
	NotificationID  string
	IncidentID      string
	DecisionType    DecisionType
	ReasonCodes     []string
	ThresholdInputs map[string]any
	RiskAssessment  map[string]any
	Rationale       string
	CreatedAt       time.Time
}

type DiagnosticRecord struct {
	ID              string
	DecisionID      string
	NotificationID  string
	IncidentID      string
	DiagnosticKey   string
	TargetRef       string
	Status          string
	InputsRedacted  map[string]any
	OutputsRedacted map[string]any
	ErrorKind       string
	ErrorRedacted   string
	StartedAt       *time.Time
	CompletedAt     *time.Time
	DurationMS      int
}

type ApprovalRequest struct {
	ID               string
	IncidentID       string
	DecisionID       string
	ActionKey        string
	TargetRef        string
	RiskExplanation  string
	ExpectedEffect   string
	VerificationPlan map[string]any
	ExpiresAt        time.Time
	Status           string
	CreatedAt        time.Time
	ResolvedAt       *time.Time
}

type ApprovalDecision struct {
	ID                string
	ApprovalRequestID string
	Decision          string
	ActorKind         string
	ActorRef          string
	Channel           string
	Reason            string
	CreatedAt         time.Time
}

type ActionAttempt struct {
	ID                string
	DecisionID        string
	IncidentID        string
	ApprovalRequestID string
	ActionKey         string
	ActionClass       ActionClass
	Status            string
	ActorKind         string
	TargetRef         string
	RiskLevel         RiskLevel
	BlastRadius       map[string]any
	InputRedacted     map[string]any
	RetryCount        int
	IdempotencyKey    string
	LoopGuardKey      string
	RequestedAt       time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
}

type ActionResult struct {
	ID              string
	ActionAttemptID string
	Outcome         string
	ExternalEffects map[string]any
	OutputRedacted  map[string]any
	Verification    map[string]any
	ErrorKind       string
	ErrorRedacted   string
	CompletedAt     time.Time
}

type Suppression struct {
	ID               string
	NotificationID   string
	IncidentID       string
	SourceInstanceID string
	Kind             string
	Scope            map[string]any
	Reason           string
	StartsAt         time.Time
	ExpiresAt        *time.Time
	CreatedAt        time.Time
}

type DeliveryAttempt struct {
	ID                string
	DecisionID        string
	IncidentID        string
	ApprovalRequestID string
	Channel           string
	DestinationRef    string
	PayloadHash       string
	RedactionState    map[string]any
	Status            string
	ErrorKind         string
	ErrorRedacted     string
	AttemptedAt       time.Time
	CompletedAt       *time.Time
}

type PipelineResult struct {
	Receipt        IngestReceipt
	RawEvent       RawEventRecord
	Notification   NormalizedNotification
	Classification ClassificationRecord
	Incident       Incident
	Decision       ProcessingDecision
	Suppressions   []Suppression
	Diagnostics    []DiagnosticRecord
	Approval       *ApprovalRequest
	ActionAttempt  *ActionAttempt
	ActionResult   *ActionResult
	Delivery       *DeliveryAttempt
}

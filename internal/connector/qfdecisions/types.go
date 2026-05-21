package qfdecisions

const (
	DefaultConnectorID = "qf-decisions"

	DecisionEventsPath          = "/api/private/smackerel/v1/decision-events"
	DecisionPacketsPath         = "/api/private/smackerel/v1/decision-packets"
	PersonalEvidenceBundlesPath = "/api/private/smackerel/v1/personal-evidence-bundles"
	TargetContextPacketContext  = "packet_context"
	TargetContextRefKey         = "target_context_ref"
	TargetContextTypeKey        = "target_context_type"
	TargetContextPacketIDKey    = "packet_id"
	TargetContextTraceIDKey     = "trace_id"

	DecisionTypeRecommendation = "recommendation"
	DecisionTypeNoAction       = "no_action"
	DecisionTypePolicyDenial   = "policy_denial"
	DecisionTypeAnalysisNote   = "analysis_note"

	ContentTypeDecisionPacket   = "qf/decision-packet"
	ContentTypeNoActionDecision = "qf/no-action-decision"
	ContentTypePolicyDenial     = "qf/policy-denial"
	ContentTypeApprovalRequest  = "qf/approval-request"

	EvidenceExportStatusAccepted                   = "accepted"
	EvidenceExportStatusLocalReject                = "local_reject"
	EvidenceExportStatusExportIDCollision          = "export_id_collision"
	EvidenceExportStatusExportIDPreviouslyRejected = "export_id_previously_rejected"
	EvidenceExportStatusTransportFailed            = "transport_failed"
	EvidenceExportStatusRevoked                    = "revoked"
	EvidenceExportStatusRevokedRemoteMissing       = "revoked_remote_missing"

	EvidenceRejectBundleTooLarge           = "BUNDLE_TOO_LARGE"
	EvidenceRejectTooManyClaims            = "TOO_MANY_CLAIMS"
	EvidenceRejectRateLimitExceeded        = "RATE_LIMIT_EXCEEDED"
	EvidenceRejectProvenanceRefNotInBundle = "EVIDENCE_PROVENANCE_REF_NOT_IN_BUNDLE"
	EvidenceRejectSourceClassNotEligible   = "EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE"
	EvidenceRejectTargetContextUnsupported = "EVIDENCE_TARGET_CONTEXT_NOT_SUPPORTED"
	EvidenceRejectCapabilityUnavailable    = "EVIDENCE_CAPABILITY_UNAVAILABLE"

	EvidenceBridgeExportIDReuseWithDifferentPayload = "EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD"
	EvidenceBridgeExportIDPreviouslyRejected        = "EXPORT_ID_PREVIOUSLY_REJECTED"
	EvidenceBridgeExportIDNotFound                  = "EVIDENCE_EXPORT_ID_NOT_FOUND"
	EvidenceBridgeExportIDAlreadyRevoked            = "EVIDENCE_EXPORT_ID_ALREADY_REVOKED"
	EvidenceRevokeReasonConsentRevoked              = "consent_revoked"

	AuditEnvelopeVersionV1 = "v1"

	AuditActorSmackerelConnector = "smackerel:qf-decisions-connector"

	AuditActionPacketIngest          = "packet_ingest"
	AuditActionEvidenceExportAttempt = "evidence_export_attempt"
	AuditActionEvidenceRevocation    = "evidence_revocation"
	AuditActionEngagementSignalFlush = "engagement_signal_flush"
	AuditActionCallbackAttempt       = "callback_attempt"
	AuditActionDeepLinkRender        = "deep_link_render"
	AuditActionCapabilityHandshake   = "capability_handshake"
	AuditActionActionBoundaryKick    = "action_boundary_kick"
	AuditActionCredentialRotation    = "credential_rotation"

	AuditOutcomeOK               = "ok"
	AuditOutcomeRejected         = "rejected"
	AuditOutcomeError            = "error"
	AuditOutcomeIdempotentReplay = "idempotent_replay"

	ActionTypeApproval              = "approval"
	ActionTypeExecution             = "execution"
	ActionTypeMandateChange         = "mandate_change"
	ActionTypeEmergencyStop         = "emergency_stop"
	ActionTypeWatchCreation         = "watch_creation"
	ActionTypeWatchEvaluation       = "watch_evaluation"
	ActionTypeCallbackAcceptance    = "callback_acceptance"
	ActionTypeQFTrustReconstruction = "qf_trust_reconstruction"

	// EventTypePacketActionBoundaryAttempted is the QF-bridge diagnostic
	// event the connector receives when an upstream caller tried to invoke
	// a forbidden financial action against QF. The connector MUST emit
	// the action-boundary-kick audit envelope plus increment
	// smackerel_qf_action_boundary_attempts_total without normalizing
	// the event into a trusted artifact. SCN-SM-041-020.
	EventTypePacketActionBoundaryAttempted = "packet_action_boundary_attempted"
)

type ContentTypeMapping struct {
	ContentType             string
	MetadataDecisionSubtype string
}

func ContentTypeForDecisionType(decisionType string) (ContentTypeMapping, bool) {
	switch decisionType {
	case DecisionTypeRecommendation:
		return ContentTypeMapping{ContentType: ContentTypeDecisionPacket}, true
	case DecisionTypeNoAction:
		return ContentTypeMapping{ContentType: ContentTypeNoActionDecision}, true
	case DecisionTypePolicyDenial:
		return ContentTypeMapping{ContentType: ContentTypePolicyDenial}, true
	case DecisionTypeAnalysisNote:
		return ContentTypeMapping{ContentType: ContentTypeDecisionPacket, MetadataDecisionSubtype: DecisionTypeAnalysisNote}, true
	default:
		return ContentTypeMapping{}, false
	}
}

type QFDecisionEvent struct {
	ContractVersion int    `json:"contract_version"`
	EventID         string `json:"event_id"`
	PacketID        string `json:"packet_id"`
	IntentID        string `json:"intent_id"`
	ScenarioID      string `json:"scenario_id"`
	TraceID         string `json:"trace_id"`
	EventType       string `json:"event_type"`
	DecisionType    string `json:"decision_type"`
	ApprovalState   string `json:"approval_state"`
	PacketVersion   int    `json:"packet_version"`
	Cursor          string `json:"cursor"`
	PacketURL       string `json:"packet_url"`
	SourceSurface   string `json:"source_surface"`
	CreatedAt       string `json:"created_at"`
	// EventsSkipped is populated only on a QF-issued cursor fast-forward
	// diagnostic event (spec 041 SCN-SM-041-008 / QF spec 063 F13). When
	// non-zero the connector treats this event as a recovery marker:
	// increments the fast-forward counter, transitions to
	// HealthDegradedRecovered, and skips normalization of the event itself.
	EventsSkipped int `json:"events_skipped,omitempty"`
}

type DecisionEventsResponse struct {
	Events     []QFDecisionEvent `json:"events"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
	ServerTime string            `json:"server_time"`
}

type QFDecisionPacketEnvelope struct {
	ContractVersion      int            `json:"contract_version"`
	PacketID             string         `json:"packet_id"`
	IntentID             string         `json:"intent_id"`
	ScenarioID           string         `json:"scenario_id"`
	TraceID              string         `json:"trace_id"`
	Thesis               string         `json:"thesis"`
	WhyNow               string         `json:"why_now"`
	QuantifiedImpact     map[string]any `json:"quantified_impact"`
	ExpertAnalysisBundle map[string]any `json:"expert_analysis_bundle"`
	CalibrationBadge     map[string]any `json:"calibration_badge"`
	DataProvenanceBadge  map[string]any `json:"data_provenance_badge"`
	ApprovalState        string         `json:"approval_state"`
	DeepLink             string         `json:"deep_link"`
	PacketURLSigned      string         `json:"packet_url_signed,omitempty"`
	SignatureExpiresAt   string         `json:"signature_expires_at,omitempty"`
	PreferredSurface     string         `json:"preferred_surface,omitempty"`
	PacketVersion        int            `json:"packet_version"`
	DecisionType         string         `json:"decision_type"`
	CreatedAt            string         `json:"created_at"`
	UpdatedAt            string         `json:"updated_at"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

type PersonalEvidenceBundle struct {
	ContractVersion         int                     `json:"contract_version"`
	BundleID                string                  `json:"bundle_id"`
	ExportID                string                  `json:"export_id"`
	CreatedAt               string                  `json:"created_at"`
	ConsentScope            string                  `json:"consent_scope"`
	SensitivityTier         string                  `json:"sensitivity_tier"`
	SourceArtifactIDs       []string                `json:"source_artifact_ids"`
	ExtractedClaims         []string                `json:"extracted_claims"`
	Confidence              float64                 `json:"confidence"`
	Provenance              map[string]any          `json:"provenance"`
	RedactionSummary        map[string]any          `json:"redaction_summary"`
	TargetContext           map[string]any          `json:"target_context"`
	SourceProvenanceClasses []SourceProvenanceClass `json:"source_provenance_classes"`
	SourceRefs              []string                `json:"source_refs,omitempty"`
	RelatedSymbols          []string                `json:"related_symbols,omitempty"`
	RelatedEntities         []string                `json:"related_entities,omitempty"`
}

type SourceProvenanceClass struct {
	SourceArtifactID      string `json:"source_artifact_id"`
	SourceProvenanceClass string `json:"source_provenance_class"`
}

type EvidenceExportResponse struct {
	ExportID         string `json:"export_id"`
	BundleID         string `json:"bundle_id"`
	PayloadHash      string `json:"payload_hash"`
	AttachmentID     string `json:"attachment_id,omitempty"`
	Status           string `json:"status,omitempty"`
	IdempotentReplay bool   `json:"idempotent_replay,omitempty"`
}

type EvidenceRevocationResponse struct {
	ExportID       string `json:"export_id"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	RemoteMissing  bool   `json:"remote_missing,omitempty"`
	AlreadyRevoked bool   `json:"already_revoked,omitempty"`
}

type EvidenceRevocationRequest struct {
	Reason string `json:"reason"`
}

type EvidenceAuditEnvelope struct {
	TraceID              string `json:"trace_id,omitempty"`
	PacketID             string `json:"packet_id,omitempty"`
	ExportID             string `json:"export_id,omitempty"`
	SignalID             string `json:"signal_id,omitempty"`
	ActorRef             string `json:"actor_ref"`
	Surface              string `json:"surface"`
	Action               string `json:"action"`
	Outcome              string `json:"outcome"`
	Reason               string `json:"reason,omitempty"`
	TS                   string `json:"ts"`
	AuditEnvelopeVersion string `json:"audit_envelope_version"`
	BundleID             string `json:"bundle_id,omitempty"`
	TargetContextType    string `json:"target_context_type,omitempty"`
	SensitivityTier      string `json:"sensitivity_tier,omitempty"`
	RecordedAt           string `json:"recorded_at"`
}

type BridgeErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

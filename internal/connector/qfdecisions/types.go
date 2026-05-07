package qfdecisions

const (
	DefaultConnectorID = "qf-decisions"

	DecisionEventsPath  = "/api/private/smackerel/v1/decision-events"
	DecisionPacketsPath = "/api/private/smackerel/v1/decision-packets"

	DecisionTypeRecommendation = "recommendation"
	DecisionTypeNoAction       = "no_action"
	DecisionTypePolicyDenial   = "policy_denial"
	DecisionTypeAnalysisNote   = "analysis_note"

	ContentTypeDecisionPacket   = "qf/decision-packet"
	ContentTypeNoActionDecision = "qf/no-action-decision"
	ContentTypePolicyDenial     = "qf/policy-denial"
	ContentTypeApprovalRequest  = "qf/approval-request"
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
	PacketVersion        int            `json:"packet_version"`
	DecisionType         string         `json:"decision_type"`
	CreatedAt            string         `json:"created_at"`
	UpdatedAt            string         `json:"updated_at"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

type PersonalEvidenceBundle struct {
	ContractVersion   int            `json:"contract_version"`
	BundleID          string         `json:"bundle_id"`
	ExportID          string         `json:"export_id"`
	CreatedAt         string         `json:"created_at"`
	ConsentScope      string         `json:"consent_scope"`
	SensitivityTier   string         `json:"sensitivity_tier"`
	SourceArtifactIDs []string       `json:"source_artifact_ids"`
	ExtractedClaims   []string       `json:"extracted_claims"`
	Provenance        map[string]any `json:"provenance"`
	RedactionSummary  map[string]any `json:"redaction_summary"`
	TargetContext     map[string]any `json:"target_context"`
	SourceRefs        []string       `json:"source_refs,omitempty"`
}

type BridgeErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

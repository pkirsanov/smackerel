package qfdecisions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

const (
	SurfaceWeb            = "web"
	SurfaceDigest         = "digest"
	SurfaceTelegram       = "telegram"
	SurfaceSearch         = "search"
	SurfaceArtifactDetail = "artifact_detail"

	CardKindQFPacket      = "qf_packet"
	CardKindGenericPacket = "generic_qf_packet"

	DeepLinkStatusSignedUsed                    = "signed_used"
	DeepLinkStatusSignedExpiredFallbackUnsigned = "signed_expired_fallback_unsigned"
	DeepLinkStatusUnsignedOnly                  = "unsigned_only"
	TrustFallbackMissingRequiredField           = "missing_required_field"
	PreferredSurfaceSmackerelDigest             = "smackerel_digest"
	PreferredSurfaceSmackerelTelegram           = "smackerel_telegram"
	PreferredSurfaceQFDashboard                 = "qf_dashboard"
	PreferredSurfaceAny                         = "any"
	defaultQFPacketDisplayLabel                 = "QF packet"
)

type RenderOptions struct {
	Surface                       string
	DeepLinkSigningSupported      bool
	PreferredSurfaceHintSupported bool
	Now                           time.Time
	FetchPacket                   func(context.Context, string) (QFDecisionPacketEnvelope, error)
}

type PacketCard struct {
	CardKind            string              `json:"card_kind"`
	DisplayLabel        string              `json:"display_label"`
	Title               string              `json:"title"`
	PacketID            string              `json:"packet_id"`
	TraceID             string              `json:"trace_id"`
	ApprovalState       string              `json:"approval_state"`
	DecisionType        string              `json:"decision_type"`
	Thesis              string              `json:"thesis"`
	WhyNow              string              `json:"why_now"`
	ReadOnly            bool                `json:"read_only"`
	ActionEligible      bool                `json:"action_eligible"`
	UnknownDecisionType bool                `json:"unknown_decision_type"`
	FallbackReason      string              `json:"fallback_reason,omitempty"`
	TrustObjects        []TrustObjectRender `json:"trust_objects,omitempty"`
	DeepLink            DeepLinkRender      `json:"deep_link"`
	Placement           PlacementRender     `json:"placement"`
}

type TrustObjectRender struct {
	Kind     string            `json:"kind"`
	Label    string            `json:"label"`
	Severity string            `json:"severity"`
	Summary  string            `json:"summary"`
	Detail   string            `json:"detail,omitempty"`
	Links    []TrustObjectLink `json:"links,omitempty"`
}

type TrustObjectLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type DeepLinkRender struct {
	URL    string `json:"url"`
	Status string `json:"status"`
}

type PlacementRender struct {
	PrimarySurface          string `json:"primary_surface"`
	IncludeInDigest         bool   `json:"include_in_digest"`
	QueueTelegram           bool   `json:"queue_telegram"`
	ShowInQFDashboard       bool   `json:"show_in_qf_dashboard"`
	IncludeInSearch         bool   `json:"include_in_search"`
	IncludeInArtifactDetail bool   `json:"include_in_artifact_detail"`
}

func RenderPacketCard(ctx context.Context, artifact connector.RawArtifact, options RenderOptions) (PacketCard, error) {
	metadata := artifact.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	surface := renderSurface(options.Surface)
	decisionType := stringFromMetadata(metadata, "decision_type")
	unknownDecisionType := boolFromMetadata(metadata, "unknown_decision_type")
	packetID := stringFromMetadata(metadata, "packet_id")

	// SCN-SM-041-020 render-path safety-boundary defense-in-depth:
	// QF packet cards are read-only by contract (ReadOnly=true,
	// ActionEligible=false set unconditionally below). If upstream metadata
	// ever carries a structured action-request hint (`action_request` map
	// with `action_type`, or a top-level `requested_action_type` /
	// `pending_action_type` field) for a forbidden QF action type
	// (approval, execution, mandate_change, emergency_stop, watch_*,
	// callback_acceptance, qf_trust_reconstruction), the render path MUST
	// emit the action-boundary-kick audit envelope and increment
	// smackerel_qf_action_boundary_attempts_total BEFORE the card is built
	// so the violation is observable. The card is still rendered as
	// read-only so the user never sees an action surface. Pre-MVP QF does
	// NOT send these metadata keys (capability snapshot constrains
	// SupportedDecisionTypes to recommendation|no_action|policy_denial|
	// analysis_note and design 063 §F2 prohibits action-eligible
	// rendering), so this gate is silent in the happy path. The gate
	// becomes operative if (a) a future QF regression injects an
	// action-request hint or (b) a future Scope 8 transport hand-off
	// attempts to mark a packet action-eligible at render time.
	enforceRenderBoundary(metadata, packetID)

	card := PacketCard{
		CardKind:            CardKindQFPacket,
		DisplayLabel:        defaultQFPacketDisplayLabel,
		Title:               publicPacketTitle(metadata, artifact.Title, packetID),
		PacketID:            packetID,
		TraceID:             stringFromMetadata(metadata, "trace_id"),
		ApprovalState:       stringFromMetadata(metadata, "approval_state"),
		DecisionType:        decisionType,
		Thesis:              stringFromMetadata(metadata, "thesis"),
		WhyNow:              stringFromMetadata(metadata, "why_now"),
		ReadOnly:            true,
		ActionEligible:      false,
		UnknownDecisionType: unknownDecisionType,
		Placement:           placementFor(metadata, decisionType, options.PreferredSurfaceHintSupported),
	}
	if unknownDecisionType {
		card.CardKind = CardKindGenericPacket
	}
	recordRenderFreshness(artifact, options.Now)

	trustObjects, trustOK := renderTrustObjects(metadata)
	if trustOK {
		card.TrustObjects = trustObjects
	} else {
		card.CardKind = CardKindGenericPacket
		card.FallbackReason = TrustFallbackMissingRequiredField
	}

	card.DeepLink = renderDeepLink(ctx, metadata, surface, options, packetID)
	return card, nil
}

func publicPacketTitle(metadata map[string]any, artifactTitle string, packetID string) string {
	if thesis := stringFromMetadata(metadata, "thesis"); thesis != "" {
		return thesis
	}
	artifactTitle = strings.TrimSpace(artifactTitle)
	if artifactTitle != "" && !looksLikeStructuredPayload(artifactTitle) {
		return artifactTitle
	}
	if packetID != "" {
		return packetID
	}
	return defaultQFPacketDisplayLabel
}

func looksLikeStructuredPayload(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// enforceRenderBoundary applies SCN-SM-041-020 defense-in-depth to the render
// surface. It scans `metadata` for forbidden-action hints in three locations:
//   - top-level `requested_action_type` (string)
//   - top-level `pending_action_type` (string)
//   - nested `action_request` map's `action_type` key (string)
//
// For each hit, the helper calls EnforceQFActionBoundary so a forbidden type
// (approval, execution, mandate_change, emergency_stop, watch_*,
// callback_acceptance, qf_trust_reconstruction) fires the action-boundary
// audit envelope and increments
// smackerel_qf_action_boundary_attempts_total{attempted_action_type=<value>}
// BEFORE the card is built. The render surface never becomes action-eligible
// regardless of metadata content (RenderPacketCard sets ActionEligible=false
// unconditionally on every code path).
func enforceRenderBoundary(metadata map[string]any, packetID string) {
	traceID := stringFromMetadata(metadata, "trace_id")
	reason := "render_metadata_action_hint_rejected"
	candidates := []string{
		stringFromMetadata(metadata, "requested_action_type"),
		stringFromMetadata(metadata, "pending_action_type"),
	}
	if actionRequest, ok := mapFromMetadata(metadata, "action_request"); ok {
		candidates = append(candidates, stringFromMap(actionRequest, "action_type"))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		_, _, _ = EnforceQFActionBoundary(ActionBoundaryAttempt{
			AttemptedActionType: candidate,
			TraceID:             traceID,
			PacketID:            packetID,
			ActorRef:            AuditActorSmackerelConnector,
			Surface:             SurfaceWeb,
			Reason:              reason,
			ObservedAt:          time.Now().UTC(),
		})
	}
}

func renderTrustObjects(metadata map[string]any) ([]TrustObjectRender, bool) {
	trustKeys := []struct {
		metadataKey string
		kind        string
	}{
		{metadataKey: "calibration_badge", kind: "CalibrationBadge"},
		{metadataKey: "data_provenance_badge", kind: "DataProvenanceBadge"},
		{metadataKey: "quantified_impact", kind: "QuantifiedImpact"},
		{metadataKey: "expert_analysis_bundle", kind: "ExpertAnalysisBundle"},
	}

	rendered := make([]TrustObjectRender, 0, len(trustKeys))
	for _, trustKey := range trustKeys {
		trustMap, ok := mapFromMetadata(metadata, trustKey.metadataKey)
		if !ok || len(trustMap) == 0 {
			continue
		}
		trustObject, ok := renderTrustObject(trustKey.kind, trustMap)
		if !ok {
			metrics.QFTrustObjectRenderFailures.WithLabelValues(TrustFallbackMissingRequiredField).Inc()
			return nil, false
		}
		rendered = append(rendered, trustObject)
	}
	return rendered, true
}

func renderTrustObject(kind string, trustMap map[string]any) (TrustObjectRender, bool) {
	label := stringFromMap(trustMap, "label")
	severity := stringFromMap(trustMap, "severity")
	if label == "" || severity == "" {
		return TrustObjectRender{}, false
	}
	return TrustObjectRender{
		Kind:     kind,
		Label:    label,
		Severity: severity,
		Summary:  stringFromMap(trustMap, "summary"),
		Detail:   stringFromMap(trustMap, "detail"),
		Links:    trustLinksFromMap(trustMap),
	}, true
}

func trustLinksFromMap(trustMap map[string]any) []TrustObjectLink {
	rawLinks, ok := trustMap["links"].([]any)
	if !ok {
		return nil
	}
	links := make([]TrustObjectLink, 0, len(rawLinks))
	for _, rawLink := range rawLinks {
		linkMap, ok := rawLink.(map[string]any)
		if !ok {
			continue
		}
		label := stringFromMap(linkMap, "label")
		url := stringFromMap(linkMap, "url")
		if label == "" || url == "" {
			continue
		}
		links = append(links, TrustObjectLink{Label: label, URL: url})
	}
	return links
}

func renderDeepLink(ctx context.Context, metadata map[string]any, surface string, options RenderOptions, packetID string) DeepLinkRender {
	unsignedURL := stringFromMetadata(metadata, "deep_link")
	if !options.DeepLinkSigningSupported {
		metrics.QFDeepLinkRenderTotal.WithLabelValues(surface, DeepLinkStatusUnsignedOnly).Inc()
		emitDeepLinkAudit(metadata, surface, packetID, DeepLinkStatusUnsignedOnly, options.Now)
		return DeepLinkRender{URL: unsignedURL, Status: DeepLinkStatusUnsignedOnly}
	}

	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	signedURL := stringFromMetadata(metadata, "packet_url_signed")
	expiresAt := stringFromMetadata(metadata, "signature_expires_at")
	if signedURL == "" {
		metrics.QFDeepLinkRenderTotal.WithLabelValues(surface, DeepLinkStatusUnsignedOnly).Inc()
		emitDeepLinkAudit(metadata, surface, packetID, DeepLinkStatusUnsignedOnly, now)
		return DeepLinkRender{URL: unsignedURL, Status: DeepLinkStatusUnsignedOnly}
	}
	if signedURL != "" && signatureIsFresh(expiresAt, now) {
		metrics.QFDeepLinkRenderTotal.WithLabelValues(surface, DeepLinkStatusSignedUsed).Inc()
		emitDeepLinkAudit(metadata, surface, packetID, DeepLinkStatusSignedUsed, now)
		return DeepLinkRender{URL: signedURL, Status: DeepLinkStatusSignedUsed}
	}

	if options.FetchPacket != nil {
		refetched, err := options.FetchPacket(ctx, packetID)
		if err == nil && refetched.PacketURLSigned != "" && signatureIsFresh(refetched.SignatureExpiresAt, now) {
			metrics.QFDeepLinkRenderTotal.WithLabelValues(surface, DeepLinkStatusSignedUsed).Inc()
			emitDeepLinkAudit(metadata, surface, packetID, DeepLinkStatusSignedUsed, now)
			return DeepLinkRender{URL: refetched.PacketURLSigned, Status: DeepLinkStatusSignedUsed}
		}
	}

	metrics.QFDeepLinkRenderTotal.WithLabelValues(surface, DeepLinkStatusSignedExpiredFallbackUnsigned).Inc()
	emitDeepLinkAudit(metadata, surface, packetID, DeepLinkStatusSignedExpiredFallbackUnsigned, now)
	return DeepLinkRender{URL: unsignedURL, Status: DeepLinkStatusSignedExpiredFallbackUnsigned}
}

func recordRenderFreshness(artifact connector.RawArtifact, observedAt time.Time) {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	if !artifact.CapturedAt.IsZero() {
		RecordFreshnessObservation(FreshnessStageRender, observedAt.Sub(artifact.CapturedAt.UTC()).Seconds())
	}
	if rawCreatedAt := stringFromMetadata(artifact.Metadata, "qf_created_at"); rawCreatedAt != "" {
		if createdAt, err := parseQFTime(rawCreatedAt); err == nil {
			RecordFreshnessObservation(FreshnessStageTotal, observedAt.Sub(createdAt).Seconds())
		}
	}
}

func emitDeepLinkAudit(metadata map[string]any, surface, packetID, status string, observedAt time.Time) {
	EmitConnectorAuditEnvelope(BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:    stringFromMetadata(metadata, "trace_id"),
		PacketID:   packetID,
		Surface:    surface,
		Action:     AuditActionDeepLinkRender,
		Outcome:    status,
		ObservedAt: observedAt,
	}))
}

func placementFor(metadata map[string]any, decisionType string, preferredSurfaceHintSupported bool) PlacementRender {
	preferredSurface := ""
	if preferredSurfaceHintSupported {
		preferredSurface = stringFromMetadata(metadata, "preferred_surface")
	}
	if preferredSurface == "" {
		preferredSurface = defaultPreferredSurface(decisionType)
	}

	placement := PlacementRender{
		PrimarySurface:          preferredSurface,
		IncludeInSearch:         true,
		IncludeInArtifactDetail: true,
	}
	switch preferredSurface {
	case PreferredSurfaceSmackerelDigest:
		placement.IncludeInDigest = true
	case PreferredSurfaceSmackerelTelegram:
		placement.QueueTelegram = true
	case PreferredSurfaceQFDashboard:
		placement.ShowInQFDashboard = true
	case PreferredSurfaceAny:
		placement.IncludeInDigest = true
		placement.QueueTelegram = true
		placement.ShowInQFDashboard = true
	default:
		placement.PrimarySurface = PreferredSurfaceAny
		placement.IncludeInDigest = true
		placement.QueueTelegram = true
		placement.ShowInQFDashboard = true
	}
	return placement
}

func defaultPreferredSurface(decisionType string) string {
	switch strings.TrimSpace(decisionType) {
	case DecisionTypeAnalysisNote:
		return PreferredSurfaceSmackerelDigest
	case DecisionTypeRecommendation, DecisionTypePolicyDenial:
		return PreferredSurfaceQFDashboard
	case DecisionTypeNoAction:
		return PreferredSurfaceAny
	default:
		return PreferredSurfaceAny
	}
}

func signatureIsFresh(rawExpiresAt string, now time.Time) bool {
	expiresAt, err := parseQFTime(rawExpiresAt)
	if err != nil {
		return false
	}
	return expiresAt.After(now.UTC())
}

func parseQFTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func renderSurface(surface string) string {
	switch strings.TrimSpace(surface) {
	case SurfaceWeb, SurfaceDigest, SurfaceTelegram, SurfaceSearch, SurfaceArtifactDetail:
		return surface
	default:
		return SurfaceWeb
	}
}

func stringFromMetadata(metadata map[string]any, key string) string {
	return stringFromMap(metadata, key)
}

func stringFromMap(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	stringValue, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue)
}

func boolFromMetadata(metadata map[string]any, key string) bool {
	value, ok := metadata[key]
	if !ok {
		return false
	}
	boolValue, ok := value.(bool)
	return ok && boolValue
}

func mapFromMetadata(metadata map[string]any, key string) (map[string]any, bool) {
	value, ok := metadata[key]
	if !ok {
		return nil, false
	}
	trustMap, ok := value.(map[string]any)
	return trustMap, ok
}

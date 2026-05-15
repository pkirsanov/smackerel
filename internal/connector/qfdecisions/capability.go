package qfdecisions

import (
	"context"
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/metrics"
)

const (
	CapabilitiesPath = "/api/private/smackerel/v1/capabilities"

	CapabilityStatusCompatible   = "compatible"
	CapabilityStatusIncompatible = "incompatible"
	CapabilityStatusUnfetched    = "unfetched"
)

// QFBridgeCapability mirrors the response of GET /api/private/smackerel/v1/capabilities.
// Schema source of truth: ~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md
// (F2 §"GET /api/private/smackerel/v1/capabilities"). All 21 fields are required by
// the QF-side contract; the connector persists the raw JSON on sync_state for the
// operator status surface and post-restart compatibility checks.
type QFBridgeCapability struct {
	SupportedPacketVersions            []string `json:"supported_packet_versions"`
	SupportedEventTypes                []string `json:"supported_event_types"`
	SupportedDecisionTypes             []string `json:"supported_decision_types"`
	MaxPageSize                        int      `json:"max_page_size"`
	MinPageSize                        int      `json:"min_page_size"`
	SupportedTargetContextTypes        []string `json:"supported_target_context_types"`
	EvidenceMaxBundleSizeBytes         int      `json:"evidence_max_bundle_size_bytes"`
	EvidenceMaxClaimsPerBundle         int      `json:"evidence_max_claims_per_bundle"`
	EvidenceRateLimitPerMinute         int      `json:"evidence_rate_limit_per_minute"`
	FreshnessSLAP95Seconds             int      `json:"freshness_sla_p95_seconds"`
	AuditEnvelopeVersion               string   `json:"audit_envelope_version"`
	TenantAware                        bool     `json:"tenant_aware"`
	PreferredSurfaceHintSupported      bool     `json:"preferred_surface_hint_supported"`
	EngagementSignalSupported          bool     `json:"engagement_signal_supported"`
	PersonalContextPullSupported       bool     `json:"personal_context_pull_supported"`
	WatchSignalDirection               string   `json:"watch_signal_direction"`
	CallbackSigningSupported           bool     `json:"callback_signing_supported"`
	DeepLinkSigningSupported           bool     `json:"deep_link_signing_supported"`
	CredentialRotationOverlapSupported bool     `json:"credential_rotation_overlap_supported"`
	NoActionEmitEnabled                bool     `json:"no_action_emit_enabled"`
	EligibleSmackerelSourceClasses     []string `json:"eligible_smackerel_source_classes"`
}

// CapabilityMismatchError is returned when the QF-advertised capability does not
// satisfy a hard contract the connector requires (packet_version, decision_type,
// audit_envelope_version, or page-size bounds). The connector MUST refuse to
// poll decision events until the mismatch is resolved.
type CapabilityMismatchError struct {
	Field    string
	Required string
	Actual   string
}

func (e CapabilityMismatchError) Error() string {
	return fmt.Sprintf("QF capability mismatch on %s: required=%s actual=%s", e.Field, e.Required, e.Actual)
}

// FetchCapability calls GET /api/private/smackerel/v1/capabilities using the same
// credential and JSON decoding plumbing as the existing decision-event/packet
// methods. It does NOT mutate connector state — callers persist the returned
// capability via the sync_state row.
func (c *Client) FetchCapability(ctx context.Context) (QFBridgeCapability, error) {
	endpoint, err := c.urlFor(CapabilitiesPath)
	if err != nil {
		return QFBridgeCapability{}, err
	}
	var capability QFBridgeCapability
	if err := c.doGet(ctx, endpoint.String(), &capability); err != nil {
		return QFBridgeCapability{}, err
	}
	return capability, nil
}

// CompatibilityCheck verifies the capability response declares everything the
// connector requires for safe polling. Returns CapabilityMismatchError on the
// FIRST violating field; the caller logs the mismatch and refuses to poll.
//
// Required invariants for Scope 2:
//   - audit_envelope_version MUST equal "v1" (Smackerel build only consumes v1 envelopes)
//   - supported_packet_versions MUST contain "v1"
//   - supported_decision_types MUST contain "recommendation", "policy_denial", "analysis_note"
//     ("no_action" is capability-gated by no_action_emit_enabled and is NOT required)
//   - max_page_size MUST be >= 1 (negative or zero is a contract bug on QF side)
//
// Emits smackerel_qf_capability_mismatch_total{required,actual} on failure.
func (cap QFBridgeCapability) CompatibilityCheck() error {
	if cap.AuditEnvelopeVersion != "v1" {
		metrics.QFCapabilityMismatch.WithLabelValues("v1", cap.AuditEnvelopeVersion).Inc()
		return CapabilityMismatchError{Field: "audit_envelope_version", Required: "v1", Actual: cap.AuditEnvelopeVersion}
	}
	if !containsString(cap.SupportedPacketVersions, "v1") {
		metrics.QFCapabilityMismatch.WithLabelValues("v1", strings.Join(cap.SupportedPacketVersions, ",")).Inc()
		return CapabilityMismatchError{Field: "supported_packet_versions", Required: "v1", Actual: strings.Join(cap.SupportedPacketVersions, ",")}
	}
	for _, required := range []string{"recommendation", "policy_denial", "analysis_note"} {
		if !containsString(cap.SupportedDecisionTypes, required) {
			metrics.QFCapabilityMismatch.WithLabelValues(required, strings.Join(cap.SupportedDecisionTypes, ",")).Inc()
			return CapabilityMismatchError{Field: "supported_decision_types", Required: required, Actual: strings.Join(cap.SupportedDecisionTypes, ",")}
		}
	}
	if cap.MaxPageSize < 1 {
		metrics.QFCapabilityMismatch.WithLabelValues(">=1", fmt.Sprintf("%d", cap.MaxPageSize)).Inc()
		return CapabilityMismatchError{Field: "max_page_size", Required: ">=1", Actual: fmt.Sprintf("%d", cap.MaxPageSize)}
	}
	return nil
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

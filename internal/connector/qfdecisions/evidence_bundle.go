package qfdecisions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
)

type EvidenceBundleInput struct {
	BundleID                string
	ExportID                string
	CreatedAt               time.Time
	ConsentScope            string
	SensitivityTier         string
	PacketID                string
	TraceID                 string
	SourceArtifactIDs       []string
	SourceRefs              []string
	SourceProvenanceClasses []SourceProvenanceClass
	ExtractedClaims         []string
	Confidence              float64
	Provenance              map[string]any
	RedactionSummary        map[string]any
	RelatedSymbols          []string
	RelatedEntities         []string
}

type EvidencePreflightError struct {
	Reason string
	Detail string
}

func (e EvidencePreflightError) Error() string {
	if e.Detail == "" {
		return e.Reason
	}
	return e.Reason + ": " + e.Detail
}

type EvidenceExportError struct {
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
}

func (e EvidenceExportError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

type EvidenceExporter struct {
	client        *Client
	store         *EvidenceExportStore
	limiter       *EvidenceRateLimiter
	credentialRef string
	now           func() time.Time
}

func NewEvidenceExporter(client *Client, store *EvidenceExportStore, limiter *EvidenceRateLimiter, credentialRef string, now func() time.Time) *EvidenceExporter {
	if now == nil {
		now = time.Now
	}
	return &EvidenceExporter{client: client, store: store, limiter: limiter, credentialRef: credentialRef, now: now}
}

func (e *EvidenceExporter) Export(ctx context.Context, bundle PersonalEvidenceBundle, capability QFBridgeCapability) (EvidenceExportRecord, EvidenceExportResponse, error) {
	observedAt := e.now().UTC()
	// SCN-SM-041-020 export-path safety-boundary defense-in-depth:
	// PersonalEvidenceBundle exports are a consent-scoped read-out of
	// already-captured smackerel artifacts (no financial-action surface).
	// If a bundle ever carries a forbidden QF action type in its
	// `target_context` map (either as TargetContextTypeKey or as a nested
	// `requested_action_type` field), the export path MUST emit the
	// action-boundary-kick audit envelope and increment
	// smackerel_qf_action_boundary_attempts_total BEFORE the bundle is
	// validated, persisted, or transmitted to QF. Pre-MVP QF capability
	// snapshots constrain SupportedTargetContextTypes to non-action types
	// (guided_analysis, rhai_run, saved_result, analysis_context,
	// packet_context), so this gate is silent in the happy path. It
	// becomes operative if (a) a future caller constructs a bundle whose
	// TargetContext claims an action surface or (b) future expansion of
	// the evidence export API introduces a transport that could enable a
	// financial action. The ValidateEvidenceBundleForExport call below
	// remains responsible for capability-supported target-context
	// rejection; this guard is a higher-precedence pre-MVP veto so the
	// boundary metric and audit envelope land even if validation passes.
	if exportTargetContextRequestsForbiddenAction(bundle) {
		_, _, _ = EnforceQFActionBoundary(ActionBoundaryAttempt{
			AttemptedActionType: stringFromExportTargetContext(bundle.TargetContext),
			TraceID:             stringFromTargetContextKey(bundle.TargetContext, TargetContextTraceIDKey),
			PacketID:            stringFromTargetContextKey(bundle.TargetContext, TargetContextPacketIDKey),
			ActorRef:            AuditActorSmackerelConnector,
			Surface:             "evidence_export",
			Reason:              "evidence_target_context_action_request_rejected",
			ObservedAt:          observedAt,
		})
	}
	if err := ValidateEvidenceBundleForExport(bundle, capability, e.limiter, e.credentialRef); err != nil {
		reason := evidenceErrorReason(err)
		audit := BuildEvidenceAuditEnvelope(AuditActionEvidenceExportAttempt, AuditOutcomeRejected, reason, bundle, observedAt)
		record, storeErr := e.store.SaveAttempt(ctx, bundle, EvidenceExportStatusLocalReject, reason, audit, observedAt)
		if storeErr != nil {
			return EvidenceExportRecord{}, EvidenceExportResponse{}, storeErr
		}
		EmitConnectorAuditEnvelope(audit)
		return record, EvidenceExportResponse{}, err
	}
	response, err := e.client.ExportPersonalEvidenceBundle(ctx, bundle)
	if err != nil {
		status, reason := evidenceExportStatusAndReason(err)
		audit := BuildEvidenceAuditEnvelope(AuditActionEvidenceExportAttempt, AuditOutcomeRejected, reason, bundle, observedAt)
		record, storeErr := e.store.SaveAttempt(ctx, bundle, status, reason, audit, observedAt)
		if storeErr != nil {
			return EvidenceExportRecord{}, EvidenceExportResponse{}, storeErr
		}
		RecordQFEvidenceExportAttempt(status, record.TargetContextType, record.SensitivityTier)
		EmitConnectorAuditEnvelope(audit)
		return record, EvidenceExportResponse{}, err
	}
	if response.IdempotentReplay {
		record, storeErr := e.store.MarkIdempotentObserved(ctx, bundle.ExportID, observedAt)
		if storeErr != nil {
			return EvidenceExportRecord{}, EvidenceExportResponse{}, storeErr
		}
		RecordQFEvidenceExportAttempt(record.Status, record.TargetContextType, record.SensitivityTier)
		EmitConnectorAuditEnvelope(BuildEvidenceAuditEnvelope(AuditActionEvidenceExportAttempt, AuditOutcomeIdempotentReplay, "", bundle, observedAt))
		return record, response, nil
	}
	audit := BuildEvidenceAuditEnvelope(AuditActionEvidenceExportAttempt, AuditOutcomeOK, "", bundle, observedAt)
	record, storeErr := e.store.SaveAttempt(ctx, bundle, EvidenceExportStatusAccepted, "", audit, observedAt)
	if storeErr != nil {
		return EvidenceExportRecord{}, EvidenceExportResponse{}, storeErr
	}
	RecordQFEvidenceExportAttempt(EvidenceExportStatusAccepted, record.TargetContextType, record.SensitivityTier)
	EmitConnectorAuditEnvelope(audit)
	return record, response, nil
}

func (e *EvidenceExporter) Revoke(ctx context.Context, exportID, reason string) (EvidenceExportRecord, EvidenceRevocationResponse, error) {
	observedAt := e.now().UTC()
	existing, ok, err := e.store.Get(ctx, exportID)
	if err != nil {
		return EvidenceExportRecord{}, EvidenceRevocationResponse{}, err
	}
	if !ok {
		return EvidenceExportRecord{}, EvidenceRevocationResponse{}, fmt.Errorf("qf evidence export %s not found", exportID)
	}
	response, err := e.client.RevokePersonalEvidenceBundle(ctx, exportID, reason)
	if err != nil {
		return EvidenceExportRecord{}, EvidenceRevocationResponse{}, err
	}
	statusReason := response.Reason
	if response.RemoteMissing {
		statusReason = "remote_missing"
	}
	status := response.Status
	if status == "" {
		status = EvidenceExportStatusRevoked
	}
	audit := BuildEvidenceAuditEnvelope(AuditActionEvidenceRevocation, AuditOutcomeOK, statusReason, EvidenceBundleForAuditFromRecord(existing), observedAt)
	record, storeErr := e.store.MarkRevoked(ctx, exportID, status, statusReason, audit, observedAt)
	if storeErr != nil {
		return EvidenceExportRecord{}, EvidenceRevocationResponse{}, storeErr
	}
	metrics.QFEvidenceRevokedTotal.WithLabelValues(reason).Inc()
	EmitConnectorAuditEnvelope(audit)
	return record, response, nil
}

type EvidenceRateLimiter struct {
	now     func() time.Time
	mu      sync.Mutex
	buckets map[string]evidenceBucket
}

type evidenceBucket struct {
	tokens       float64
	lastRefilled time.Time
}

func NewEvidenceRateLimiter(now func() time.Time) *EvidenceRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &EvidenceRateLimiter{now: now, buckets: make(map[string]evidenceBucket)}
}

func (l *EvidenceRateLimiter) Allow(credentialRef string, limitPerMinute int) bool {
	if l == nil {
		return true
	}
	if credentialRef == "" || limitPerMinute <= 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now().UTC()
	bucket, exists := l.buckets[credentialRef]
	if !exists {
		bucket = evidenceBucket{tokens: float64(limitPerMinute), lastRefilled: now}
	}
	elapsed := now.Sub(bucket.lastRefilled)
	if elapsed > 0 {
		refill := elapsed.Minutes() * float64(limitPerMinute)
		bucket.tokens += refill
		if bucket.tokens > float64(limitPerMinute) {
			bucket.tokens = float64(limitPerMinute)
		}
		bucket.lastRefilled = now
	}
	if bucket.tokens < 1 {
		l.buckets[credentialRef] = bucket
		return false
	}
	bucket.tokens -= 1
	l.buckets[credentialRef] = bucket
	return true
}

func BuildPacketContextEvidenceBundle(input EvidenceBundleInput) (PersonalEvidenceBundle, error) {
	missing := missingEvidenceBundleInput(input)
	if len(missing) > 0 {
		return PersonalEvidenceBundle{}, EvidencePreflightError{Reason: EvidenceRejectCapabilityUnavailable, Detail: "missing required evidence fields: " + strings.Join(missing, ",")}
	}
	createdAt := input.CreatedAt.UTC()
	return PersonalEvidenceBundle{
		ContractVersion:   1,
		BundleID:          input.BundleID,
		ExportID:          input.ExportID,
		CreatedAt:         createdAt.Format(time.RFC3339),
		ConsentScope:      input.ConsentScope,
		SensitivityTier:   input.SensitivityTier,
		SourceArtifactIDs: append([]string(nil), input.SourceArtifactIDs...),
		ExtractedClaims:   append([]string(nil), input.ExtractedClaims...),
		Confidence:        input.Confidence,
		Provenance:        cloneMap(input.Provenance),
		RedactionSummary:  cloneMap(input.RedactionSummary),
		TargetContext: map[string]any{
			TargetContextTypeKey:     TargetContextPacketContext,
			TargetContextRefKey:      input.PacketID,
			TargetContextPacketIDKey: input.PacketID,
			TargetContextTraceIDKey:  input.TraceID,
		},
		SourceProvenanceClasses: append([]SourceProvenanceClass(nil), input.SourceProvenanceClasses...),
		SourceRefs:              append([]string(nil), input.SourceRefs...),
		RelatedSymbols:          append([]string(nil), input.RelatedSymbols...),
		RelatedEntities:         append([]string(nil), input.RelatedEntities...),
	}, nil
}

func ValidateEvidenceBundleForExport(bundle PersonalEvidenceBundle, capability QFBridgeCapability, limiter *EvidenceRateLimiter, credentialRef string) error {
	targetContextType, _ := bundle.TargetContext[TargetContextTypeKey].(string)
	if !containsString(capability.SupportedTargetContextTypes, targetContextType) {
		err := EvidencePreflightError{Reason: EvidenceRejectTargetContextUnsupported, Detail: targetContextType}
		recordEvidenceLocalReject(bundle, err.Reason)
		return err
	}
	if capability.EvidenceMaxBundleSizeBytes <= 0 || capability.EvidenceMaxClaimsPerBundle <= 0 || capability.EvidenceRateLimitPerMinute <= 0 {
		err := EvidencePreflightError{Reason: EvidenceRejectCapabilityUnavailable, Detail: "evidence import limits are absent from QF capability"}
		recordEvidenceLocalReject(bundle, err.Reason)
		return err
	}
	canonical, err := CanonicalEvidenceBundleJSON(bundle)
	if err != nil {
		return err
	}
	if len(canonical) > capability.EvidenceMaxBundleSizeBytes {
		reject := EvidencePreflightError{Reason: EvidenceRejectBundleTooLarge, Detail: fmt.Sprintf("bundle bytes=%d max=%d", len(canonical), capability.EvidenceMaxBundleSizeBytes)}
		recordEvidenceLocalReject(bundle, reject.Reason)
		return reject
	}
	if len(bundle.ExtractedClaims) > capability.EvidenceMaxClaimsPerBundle {
		reject := EvidencePreflightError{Reason: EvidenceRejectTooManyClaims, Detail: fmt.Sprintf("claims=%d max=%d", len(bundle.ExtractedClaims), capability.EvidenceMaxClaimsPerBundle)}
		recordEvidenceLocalReject(bundle, reject.Reason)
		return reject
	}
	if err := validateSourceProvenanceClasses(bundle, capability); err != nil {
		recordEvidenceLocalReject(bundle, err.Reason)
		return err
	}
	if !limiter.Allow(credentialRef, capability.EvidenceRateLimitPerMinute) {
		reject := EvidencePreflightError{Reason: EvidenceRejectRateLimitExceeded, Detail: credentialRef}
		recordEvidenceLocalReject(bundle, reject.Reason)
		return reject
	}
	return nil
}

func CanonicalEvidenceBundleJSON(bundle PersonalEvidenceBundle) ([]byte, error) {
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal personal evidence bundle: %w", err)
	}
	return encoded, nil
}

func EvidenceBundlePayloadHash(bundle PersonalEvidenceBundle) (string, error) {
	encoded, err := CanonicalEvidenceBundleJSON(bundle)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func EvidenceSourceClassNotEligibleReason(sourceClass string) string {
	return fmt.Sprintf("EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE{%s}", sourceClass)
}

func BuildEvidenceAuditEnvelope(action, outcome, reason string, bundle PersonalEvidenceBundle, recordedAt time.Time) EvidenceAuditEnvelope {
	targetContextType, _ := bundle.TargetContext[TargetContextTypeKey].(string)
	packetID, _ := bundle.TargetContext[TargetContextPacketIDKey].(string)
	traceID, _ := bundle.TargetContext[TargetContextTraceIDKey].(string)
	return BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:         traceID,
		PacketID:        packetID,
		ExportID:        bundle.ExportID,
		Surface:         targetContextType,
		Action:          action,
		Outcome:         outcome,
		Reason:          reason,
		BundleID:        bundle.BundleID,
		TargetContext:   targetContextType,
		SensitivityTier: bundle.SensitivityTier,
		ObservedAt:      recordedAt,
	})
}

func (c *Client) ExportPersonalEvidenceBundle(ctx context.Context, bundle PersonalEvidenceBundle) (EvidenceExportResponse, error) {
	payloadHash, err := EvidenceBundlePayloadHash(bundle)
	if err != nil {
		return EvidenceExportResponse{}, err
	}
	statusCode, body, err := c.doJSON(ctx, http.MethodPost, PersonalEvidenceBundlesPath, bundle)
	if err != nil {
		return EvidenceExportResponse{}, err
	}
	if statusCode == http.StatusCreated || statusCode == http.StatusOK {
		var response EvidenceExportResponse
		if len(body) > 0 {
			if err := json.Unmarshal(body, &response); err != nil {
				return EvidenceExportResponse{}, fmt.Errorf("decode personal evidence export response: %w", err)
			}
		}
		if response.ExportID == "" {
			response.ExportID = bundle.ExportID
		}
		if response.BundleID == "" {
			response.BundleID = bundle.BundleID
		}
		if response.PayloadHash == "" {
			response.PayloadHash = payloadHash
		}
		if statusCode == http.StatusOK {
			response.IdempotentReplay = true
		}
		if response.ExportID != bundle.ExportID || response.PayloadHash != payloadHash {
			return EvidenceExportResponse{}, EvidenceExportError{StatusCode: statusCode, Code: "EVIDENCE_EXPORT_RESPONSE_MISMATCH", Message: "QF response did not match export_id and payload_hash", Retryable: false}
		}
		return response, nil
	}
	if statusCode == http.StatusConflict {
		bridgeErr := decodeEvidenceBridgeError(body)
		code := bridgeErr.Code
		if bridgeErr.Reason != "" {
			code = bridgeErr.Reason
		}
		if code == EvidenceBridgeExportIDReuseWithDifferentPayload {
			return EvidenceExportResponse{}, EvidenceExportError{StatusCode: statusCode, Code: EvidenceExportStatusExportIDCollision, Message: bridgeErr.Message, Retryable: false}
		}
		if code == EvidenceBridgeExportIDPreviouslyRejected {
			return EvidenceExportResponse{}, EvidenceExportError{StatusCode: statusCode, Code: EvidenceExportStatusExportIDPreviouslyRejected, Message: bridgeErr.Message, Retryable: false}
		}
		return EvidenceExportResponse{}, EvidenceExportError{StatusCode: statusCode, Code: code, Message: bridgeErr.Message, Retryable: false}
	}
	if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
		bridgeErr := decodeEvidenceBridgeError(body)
		return EvidenceExportResponse{}, EvidenceExportError{StatusCode: statusCode, Code: bridgeErr.Code, Message: bridgeErr.Message, Retryable: false}
	}
	return EvidenceExportResponse{}, EvidenceExportError{StatusCode: statusCode, Code: EvidenceExportStatusTransportFailed, Message: string(body), Retryable: true}
}

func (c *Client) RevokePersonalEvidenceBundle(ctx context.Context, exportID, reason string) (EvidenceRevocationResponse, error) {
	if exportID == "" {
		return EvidenceRevocationResponse{}, EvidencePreflightError{Reason: EvidenceRejectCapabilityUnavailable, Detail: "export_id is required"}
	}
	if reason == "" {
		return EvidenceRevocationResponse{}, EvidencePreflightError{Reason: EvidenceRejectCapabilityUnavailable, Detail: "revocation reason is required"}
	}
	path := PersonalEvidenceBundlesPath + "/" + exportID
	statusCode, body, err := c.doJSON(ctx, http.MethodDelete, path, EvidenceRevocationRequest{Reason: reason})
	if err != nil {
		return EvidenceRevocationResponse{}, err
	}
	if statusCode == http.StatusOK || statusCode == http.StatusNoContent {
		response := EvidenceRevocationResponse{ExportID: exportID, Status: EvidenceExportStatusRevoked, Reason: reason}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &response); err != nil {
				return EvidenceRevocationResponse{}, fmt.Errorf("decode personal evidence revocation response: %w", err)
			}
			if response.ExportID == "" {
				response.ExportID = exportID
			}
			if response.Reason == "" {
				response.Reason = reason
			}
			if response.Status == "" {
				response.Status = EvidenceExportStatusRevoked
			}
		}
		return response, nil
	}
	if statusCode == http.StatusNotFound || statusCode == http.StatusConflict {
		bridgeErr := decodeEvidenceBridgeError(body)
		code := bridgeErr.Code
		if bridgeErr.Reason != "" {
			code = bridgeErr.Reason
		}
		if statusCode == http.StatusNotFound && code == EvidenceBridgeExportIDNotFound {
			return EvidenceRevocationResponse{ExportID: exportID, Status: EvidenceExportStatusRevokedRemoteMissing, Reason: "remote_missing", RemoteMissing: true}, nil
		}
		if statusCode == http.StatusConflict && code == EvidenceBridgeExportIDAlreadyRevoked {
			return EvidenceRevocationResponse{ExportID: exportID, Status: EvidenceExportStatusRevoked, Reason: reason, AlreadyRevoked: true}, nil
		}
		return EvidenceRevocationResponse{}, EvidenceExportError{StatusCode: statusCode, Code: code, Message: bridgeErr.Message, Retryable: false}
	}
	if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
		bridgeErr := decodeEvidenceBridgeError(body)
		return EvidenceRevocationResponse{}, EvidenceExportError{StatusCode: statusCode, Code: bridgeErr.Code, Message: bridgeErr.Message, Retryable: false}
	}
	return EvidenceRevocationResponse{}, EvidenceExportError{StatusCode: statusCode, Code: EvidenceExportStatusTransportFailed, Message: string(body), Retryable: true}
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any) (int, []byte, error) {
	endpoint, err := c.urlFor(path)
	if err != nil {
		return 0, nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal QF request payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(encoded))
	if err != nil {
		return 0, nil, fmt.Errorf("create QF request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.credentialRef)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("QF bridge request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read QF bridge response: %w", err)
	}
	return resp.StatusCode, body, nil
}

func recordEvidenceLocalReject(bundle PersonalEvidenceBundle, reason string) {
	targetContextType, _ := bundle.TargetContext[TargetContextTypeKey].(string)
	RecordQFEvidenceExportAttempt(EvidenceExportStatusLocalReject, targetContextType, bundle.SensitivityTier)
}

func evidenceErrorReason(err error) string {
	var preflight EvidencePreflightError
	if errors.As(err, &preflight) {
		return preflight.Reason
	}
	var preflightPtr *EvidencePreflightError
	if errors.As(err, &preflightPtr) {
		return preflightPtr.Reason
	}
	return EvidenceExportStatusTransportFailed
}

func evidenceExportStatusAndReason(err error) (string, string) {
	var exportErr EvidenceExportError
	if !errors.As(err, &exportErr) {
		return EvidenceExportStatusTransportFailed, EvidenceExportStatusTransportFailed
	}
	switch exportErr.Code {
	case EvidenceExportStatusExportIDCollision:
		return EvidenceExportStatusExportIDCollision, "EXPORT_ID_COLLISION"
	case EvidenceExportStatusExportIDPreviouslyRejected:
		return EvidenceExportStatusExportIDPreviouslyRejected, EvidenceBridgeExportIDPreviouslyRejected
	default:
		if exportErr.Retryable {
			return EvidenceExportStatusTransportFailed, EvidenceExportStatusTransportFailed
		}
		return EvidenceExportStatusLocalReject, exportErr.Code
	}
}

func missingEvidenceBundleInput(input EvidenceBundleInput) []string {
	missing := make([]string, 0)
	if input.BundleID == "" {
		missing = append(missing, "bundle_id")
	}
	if input.ExportID == "" {
		missing = append(missing, "export_id")
	}
	if input.CreatedAt.IsZero() {
		missing = append(missing, "created_at")
	}
	if input.ConsentScope == "" {
		missing = append(missing, "consent_scope")
	}
	if input.SensitivityTier == "" {
		missing = append(missing, "sensitivity_tier")
	}
	if input.PacketID == "" {
		missing = append(missing, "packet_id")
	}
	if input.TraceID == "" {
		missing = append(missing, "trace_id")
	}
	if len(input.SourceArtifactIDs) == 0 {
		missing = append(missing, "source_artifact_ids")
	}
	if len(input.SourceProvenanceClasses) == 0 {
		missing = append(missing, "source_provenance_classes")
	}
	if len(input.ExtractedClaims) == 0 {
		missing = append(missing, "extracted_claims")
	}
	if input.Provenance == nil {
		missing = append(missing, "provenance")
	}
	if input.RedactionSummary == nil {
		missing = append(missing, "redaction_summary")
	}
	return missing
}

func validateSourceProvenanceClasses(bundle PersonalEvidenceBundle, capability QFBridgeCapability) *EvidencePreflightError {
	artifactIDs := make(map[string]struct{}, len(bundle.SourceArtifactIDs))
	for _, artifactID := range bundle.SourceArtifactIDs {
		artifactIDs[artifactID] = struct{}{}
	}
	for _, provenanceClass := range bundle.SourceProvenanceClasses {
		if _, ok := artifactIDs[provenanceClass.SourceArtifactID]; !ok {
			return &EvidencePreflightError{Reason: EvidenceRejectProvenanceRefNotInBundle, Detail: provenanceClass.SourceArtifactID}
		}
		if !containsString(capability.EligibleSmackerelSourceClasses, provenanceClass.SourceProvenanceClass) {
			return &EvidencePreflightError{Reason: EvidenceSourceClassNotEligibleReason(provenanceClass.SourceProvenanceClass), Detail: provenanceClass.SourceArtifactID}
		}
	}
	return nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	copyMap := make(map[string]any, len(input))
	for key, value := range input {
		copyMap[key] = value
	}
	return copyMap
}

func decodeEvidenceBridgeError(body []byte) BridgeErrorResponse {
	var bridgeErr BridgeErrorResponse
	if len(body) == 0 {
		return bridgeErr
	}
	if err := json.Unmarshal(body, &bridgeErr); err != nil {
		return BridgeErrorResponse{Code: "QF_BRIDGE_ERROR", Message: string(body)}
	}
	if bridgeErr.Code == "" && bridgeErr.Reason != "" {
		bridgeErr.Code = bridgeErr.Reason
	}
	return bridgeErr
}

// exportTargetContextRequestsForbiddenAction returns true when a
// PersonalEvidenceBundle's TargetContext requests a forbidden QF action type
// via either TargetContextTypeKey or a nested `requested_action_type` /
// `pending_action_type` field. Used by Export to fire the SCN-SM-041-020
// safety-boundary guard before validation/persistence/transmission.
func exportTargetContextRequestsForbiddenAction(bundle PersonalEvidenceBundle) bool {
	if bundle.TargetContext == nil {
		return false
	}
	if IsForbiddenQFActionType(stringFromExportTargetContext(bundle.TargetContext)) {
		return true
	}
	if IsForbiddenQFActionType(stringFromTargetContextKey(bundle.TargetContext, "requested_action_type")) {
		return true
	}
	if IsForbiddenQFActionType(stringFromTargetContextKey(bundle.TargetContext, "pending_action_type")) {
		return true
	}
	return false
}

// stringFromExportTargetContext returns the bundle's TargetContextTypeKey
// value as a string (empty if missing/non-string). Kept package-local so the
// boundary guard doesn't have to depend on render.go helpers.
func stringFromExportTargetContext(targetContext map[string]any) string {
	return stringFromTargetContextKey(targetContext, TargetContextTypeKey)
}

func stringFromTargetContextKey(targetContext map[string]any, key string) string {
	if targetContext == nil {
		return ""
	}
	if raw, ok := targetContext[key].(string); ok {
		return raw
	}
	return ""
}

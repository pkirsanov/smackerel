package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

type QFEvidenceCapabilityStore interface {
	GetCapability(ctx context.Context, sourceID string) (responseJSON string, fetchedAt time.Time, status string, err error)
}

type QFEvidenceHandlers struct {
	ArtifactStore   ArtifactQuerier
	CapabilityStore QFEvidenceCapabilityStore
	ExportStore     *qfdecisions.EvidenceExportStore
	Exporter        *qfdecisions.EvidenceExporter
}

func NewQFEvidenceHandlers(artifactStore ArtifactQuerier, capabilityStore QFEvidenceCapabilityStore, exportStore *qfdecisions.EvidenceExportStore, exporter *qfdecisions.EvidenceExporter) *QFEvidenceHandlers {
	return &QFEvidenceHandlers{ArtifactStore: artifactStore, CapabilityStore: capabilityStore, ExportStore: exportStore, Exporter: exporter}
}

type QFEvidenceExportRequest struct {
	PacketArtifactID        string                              `json:"packet_artifact_id"`
	SourceArtifactIDs       []string                            `json:"source_artifact_ids"`
	SourceRefs              []string                            `json:"source_refs"`
	SourceProvenanceClasses []qfdecisions.SourceProvenanceClass `json:"source_provenance_classes"`
	ExtractedClaims         []string                            `json:"extracted_claims"`
	Confidence              float64                             `json:"confidence"`
	ConsentScope            string                              `json:"consent_scope"`
	SensitivityTier         string                              `json:"sensitivity_tier"`
	Provenance              map[string]any                      `json:"provenance"`
	RedactionSummary        map[string]any                      `json:"redaction_summary"`
	RelatedSymbols          []string                            `json:"related_symbols"`
	RelatedEntities         []string                            `json:"related_entities"`
}

type QFEvidenceRevokeRequest struct {
	Reason string `json:"reason"`
}

type QFEvidenceExportHTTPResponse struct {
	Record     QFEvidenceExportRecordResponse     `json:"record"`
	QFResponse qfdecisions.EvidenceExportResponse `json:"qf_response,omitempty"`
	Error      *ErrorDetail                       `json:"error,omitempty"`
}

type QFEvidenceRevocationHTTPResponse struct {
	Record     QFEvidenceExportRecordResponse         `json:"record"`
	QFResponse qfdecisions.EvidenceRevocationResponse `json:"qf_response"`
}

type QFEvidenceExportRecordResponse struct {
	ExportID                string                              `json:"export_id"`
	BundleID                string                              `json:"bundle_id"`
	PayloadHash             string                              `json:"payload_hash"`
	Status                  string                              `json:"status"`
	Reason                  string                              `json:"reason,omitempty"`
	TargetContextType       string                              `json:"target_context_type"`
	TargetContextRef        string                              `json:"target_context_ref"`
	PacketID                string                              `json:"packet_id"`
	TraceID                 string                              `json:"trace_id"`
	ConsentScope            string                              `json:"consent_scope"`
	SensitivityTier         string                              `json:"sensitivity_tier"`
	SourceArtifactIDs       []string                            `json:"source_artifact_ids"`
	SourceProvenanceClasses []qfdecisions.SourceProvenanceClass `json:"source_provenance_classes"`
	AuditEnvelope           qfdecisions.EvidenceAuditEnvelope   `json:"audit_envelope"`
	CreatedAt               string                              `json:"created_at"`
	UpdatedAt               string                              `json:"updated_at"`
	AcceptedAt              string                              `json:"accepted_at,omitempty"`
	RevokedAt               string                              `json:"revoked_at,omitempty"`
	LastObservedAt          string                              `json:"last_observed_at,omitempty"`
}

func (h *QFEvidenceHandlers) CreateExport(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Exporter == nil || h.ArtifactStore == nil || h.CapabilityStore == nil {
		writeError(w, http.StatusServiceUnavailable, "qf_evidence_unavailable", "QF evidence export is not configured")
		return
	}
	var req QFEvidenceExportRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid QF evidence export JSON")
		return
	}
	if err := validateQFEvidenceExportRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_qf_evidence_request", err.Error())
		return
	}
	artifact, err := h.ArtifactStore.GetArtifactWithDomain(r.Context(), req.PacketArtifactID)
	if err != nil {
		writeError(w, http.StatusNotFound, "qf_packet_not_found", "QF packet artifact not found")
		return
	}
	if !strings.HasPrefix(artifact.ArtifactType, "qf/") {
		writeError(w, http.StatusBadRequest, "not_qf_packet", "packet_artifact_id must identify a QF artifact")
		return
	}
	metadata := map[string]any{}
	if len(artifact.Metadata) == 0 || !json.Valid(artifact.Metadata) {
		writeError(w, http.StatusUnprocessableEntity, "qf_packet_metadata_unreadable", "QF packet metadata is missing or unreadable")
		return
	}
	if err := json.Unmarshal(artifact.Metadata, &metadata); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "qf_packet_metadata_unreadable", "QF packet metadata is missing or unreadable")
		return
	}
	packetID := strings.TrimSpace(qfEvidenceString(metadata["packet_id"]))
	traceID := strings.TrimSpace(qfEvidenceString(metadata["trace_id"]))
	if packetID == "" || traceID == "" {
		writeError(w, http.StatusUnprocessableEntity, "qf_packet_context_unavailable", "QF packet metadata must include packet_id and trace_id")
		return
	}
	capability, err := h.loadCapability(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, qfdecisions.EvidenceRejectCapabilityUnavailable, err.Error())
		return
	}
	bundle, err := qfdecisions.BuildPacketContextEvidenceBundle(qfdecisions.EvidenceBundleInput{
		BundleID:                qfEvidenceStableID("bundle", packetID, traceID, req.ConsentScope, req.SensitivityTier, req.SourceArtifactIDs, req.ExtractedClaims),
		ExportID:                qfEvidenceStableID("export", packetID, traceID, req.ConsentScope, req.SensitivityTier, req.SourceArtifactIDs, req.ExtractedClaims),
		CreatedAt:               artifact.CreatedAt,
		ConsentScope:            req.ConsentScope,
		SensitivityTier:         req.SensitivityTier,
		PacketID:                packetID,
		TraceID:                 traceID,
		SourceArtifactIDs:       append([]string(nil), req.SourceArtifactIDs...),
		SourceRefs:              append([]string(nil), req.SourceRefs...),
		SourceProvenanceClasses: append([]qfdecisions.SourceProvenanceClass(nil), req.SourceProvenanceClasses...),
		ExtractedClaims:         append([]string(nil), req.ExtractedClaims...),
		Confidence:              req.Confidence,
		Provenance:              qfEvidenceCloneMap(req.Provenance),
		RedactionSummary:        qfEvidenceCloneMap(req.RedactionSummary),
		RelatedSymbols:          append([]string(nil), req.RelatedSymbols...),
		RelatedEntities:         append([]string(nil), req.RelatedEntities...),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, qfdecisions.EvidenceRejectCapabilityUnavailable, err.Error())
		return
	}
	record, qfResponse, err := h.Exporter.Export(r.Context(), bundle, capability)
	if err != nil {
		status := http.StatusBadGateway
		code := "qf_evidence_export_failed"
		var preflight qfdecisions.EvidencePreflightError
		var preflightPtr *qfdecisions.EvidencePreflightError
		if errors.As(err, &preflight) {
			status = http.StatusUnprocessableEntity
			code = preflight.Reason
		} else if errors.As(err, &preflightPtr) {
			status = http.StatusUnprocessableEntity
			code = preflightPtr.Reason
		}
		writeJSON(w, status, QFEvidenceExportHTTPResponse{Record: qfEvidenceRecordResponse(record), Error: &ErrorDetail{Code: code, Message: err.Error()}})
		return
	}
	writeJSON(w, http.StatusOK, QFEvidenceExportHTTPResponse{Record: qfEvidenceRecordResponse(record), QFResponse: qfResponse})
}

func (h *QFEvidenceHandlers) GetExport(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.ExportStore == nil {
		writeError(w, http.StatusServiceUnavailable, "qf_evidence_unavailable", "QF evidence export status is not configured")
		return
	}
	exportID := strings.TrimSpace(chi.URLParam(r, "exportID"))
	if exportID == "" {
		writeError(w, http.StatusBadRequest, "missing_export_id", "export_id is required")
		return
	}
	record, ok, err := h.ExportStore.Get(r.Context(), exportID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "qf_evidence_status_failed", "failed to read QF evidence export status")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "qf_evidence_export_not_found", "QF evidence export not found")
		return
	}
	writeJSON(w, http.StatusOK, qfEvidenceRecordResponse(record))
}

func (h *QFEvidenceHandlers) RevokeExport(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Exporter == nil {
		writeError(w, http.StatusServiceUnavailable, "qf_evidence_unavailable", "QF evidence revocation is not configured")
		return
	}
	exportID := strings.TrimSpace(chi.URLParam(r, "exportID"))
	if exportID == "" {
		writeError(w, http.StatusBadRequest, "missing_export_id", "export_id is required")
		return
	}
	var req QFEvidenceRevokeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid QF evidence revocation JSON")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		writeError(w, http.StatusBadRequest, "missing_revocation_reason", "revocation reason is required")
		return
	}
	record, qfResponse, err := h.Exporter.Revoke(r.Context(), exportID, reason)
	if err != nil {
		writeError(w, http.StatusBadGateway, "qf_evidence_revoke_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, QFEvidenceRevocationHTTPResponse{Record: qfEvidenceRecordResponse(record), QFResponse: qfResponse})
}

func (h *QFEvidenceHandlers) loadCapability(ctx context.Context) (qfdecisions.QFBridgeCapability, error) {
	responseJSON, _, status, err := h.CapabilityStore.GetCapability(ctx, qfdecisions.DefaultConnectorID)
	if err != nil {
		return qfdecisions.QFBridgeCapability{}, fmt.Errorf("persisted QF capability is unavailable: %w", err)
	}
	if status != qfdecisions.CapabilityStatusCompatible {
		return qfdecisions.QFBridgeCapability{}, fmt.Errorf("persisted QF capability status is %q, want %q", status, qfdecisions.CapabilityStatusCompatible)
	}
	if strings.TrimSpace(responseJSON) == "" {
		return qfdecisions.QFBridgeCapability{}, fmt.Errorf("persisted QF capability response is empty")
	}
	var capability qfdecisions.QFBridgeCapability
	if err := json.Unmarshal([]byte(responseJSON), &capability); err != nil {
		return qfdecisions.QFBridgeCapability{}, fmt.Errorf("persisted QF capability response is unreadable: %w", err)
	}
	return capability, nil
}

func validateQFEvidenceExportRequest(req QFEvidenceExportRequest) error {
	missing := make([]string, 0)
	if strings.TrimSpace(req.PacketArtifactID) == "" {
		missing = append(missing, "packet_artifact_id")
	}
	if strings.TrimSpace(req.ConsentScope) == "" {
		missing = append(missing, "consent_scope")
	}
	if strings.TrimSpace(req.SensitivityTier) == "" {
		missing = append(missing, "sensitivity_tier")
	}
	if len(req.SourceArtifactIDs) == 0 {
		missing = append(missing, "source_artifact_ids")
	}
	if len(req.ExtractedClaims) == 0 {
		missing = append(missing, "extracted_claims")
	}
	if req.Confidence <= 0 {
		missing = append(missing, "confidence")
	}
	if len(req.Provenance) == 0 {
		missing = append(missing, "provenance")
	}
	if len(req.RedactionSummary) == 0 {
		missing = append(missing, "redaction_summary")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required QF evidence fields: %s", strings.Join(missing, ","))
	}
	for _, sourceID := range req.SourceArtifactIDs {
		if strings.TrimSpace(sourceID) == "" {
			return fmt.Errorf("source_artifact_ids cannot contain empty values")
		}
	}
	for _, claim := range req.ExtractedClaims {
		if strings.TrimSpace(claim) == "" {
			return fmt.Errorf("extracted_claims cannot contain empty values")
		}
	}
	return nil
}

func qfEvidenceStableID(prefix, packetID, traceID, consentScope, sensitivityTier string, sourceArtifactIDs, claims []string) string {
	parts := append([]string{packetID, traceID, consentScope, sensitivityTier}, sourceArtifactIDs...)
	parts = append(parts, claims...)
	encoded := append([]string(nil), parts...)
	sort.Strings(encoded)
	hash := sha256.Sum256([]byte(strings.Join(encoded, "\x00")))
	return prefix + "-" + hex.EncodeToString(hash[:])[:32]
}

func qfEvidenceCloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, val := range value {
		cloned[key] = val
	}
	return cloned
}

func qfEvidenceString(value any) string {
	stringValue, ok := value.(string)
	if !ok {
		return ""
	}
	return stringValue
}

func qfEvidenceRecordResponse(record qfdecisions.EvidenceExportRecord) QFEvidenceExportRecordResponse {
	response := QFEvidenceExportRecordResponse{
		ExportID:                record.ExportID,
		BundleID:                record.BundleID,
		PayloadHash:             record.PayloadHash,
		Status:                  record.Status,
		Reason:                  record.Reason,
		TargetContextType:       record.TargetContextType,
		TargetContextRef:        record.TargetContextRef,
		PacketID:                record.PacketID,
		TraceID:                 record.TraceID,
		ConsentScope:            record.ConsentScope,
		SensitivityTier:         record.SensitivityTier,
		SourceArtifactIDs:       append([]string(nil), record.SourceArtifactIDs...),
		SourceProvenanceClasses: append([]qfdecisions.SourceProvenanceClass(nil), record.SourceProvenanceClasses...),
		AuditEnvelope:           record.AuditEnvelope,
		CreatedAt:               record.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:               record.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if record.AcceptedAt != nil {
		response.AcceptedAt = record.AcceptedAt.UTC().Format(time.RFC3339)
	}
	if record.RevokedAt != nil {
		response.RevokedAt = record.RevokedAt.UTC().Format(time.RFC3339)
	}
	if record.LastObservedAt != nil {
		response.LastObservedAt = record.LastObservedAt.UTC().Format(time.RFC3339)
	}
	return response
}

package qfdecisions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EvidenceExportRecord struct {
	ExportID                string
	BundleID                string
	PayloadHash             string
	Status                  string
	Reason                  string
	TargetContextType       string
	TargetContextRef        string
	PacketID                string
	TraceID                 string
	ConsentScope            string
	SensitivityTier         string
	SourceArtifactIDs       []string
	SourceProvenanceClasses []SourceProvenanceClass
	AuditEnvelope           EvidenceAuditEnvelope
	CreatedAt               time.Time
	UpdatedAt               time.Time
	AcceptedAt              *time.Time
	RevokedAt               *time.Time
	LastObservedAt          *time.Time
}

type EvidenceExportStore struct {
	pool *pgxpool.Pool
}

func NewEvidenceExportStore(pool *pgxpool.Pool) *EvidenceExportStore {
	return &EvidenceExportStore{pool: pool}
}

func (s *EvidenceExportStore) SaveAttempt(ctx context.Context, bundle PersonalEvidenceBundle, status, reason string, audit EvidenceAuditEnvelope, observedAt time.Time) (EvidenceExportRecord, error) {
	if s == nil || s.pool == nil {
		return EvidenceExportRecord{}, fmt.Errorf("qf evidence export store requires postgres pool")
	}
	payloadHash, err := EvidenceBundlePayloadHash(bundle)
	if err != nil {
		return EvidenceExportRecord{}, err
	}
	targetContextType, _ := bundle.TargetContext[TargetContextTypeKey].(string)
	targetContextRef, _ := bundle.TargetContext[TargetContextRefKey].(string)
	packetID, _ := bundle.TargetContext[TargetContextPacketIDKey].(string)
	traceID, _ := bundle.TargetContext[TargetContextTraceIDKey].(string)
	sourceArtifactIDs, err := json.Marshal(bundle.SourceArtifactIDs)
	if err != nil {
		return EvidenceExportRecord{}, fmt.Errorf("marshal source artifact ids: %w", err)
	}
	sourceClasses, err := json.Marshal(bundle.SourceProvenanceClasses)
	if err != nil {
		return EvidenceExportRecord{}, fmt.Errorf("marshal source provenance classes: %w", err)
	}
	auditJSON, err := json.Marshal(audit)
	if err != nil {
		return EvidenceExportRecord{}, fmt.Errorf("marshal evidence audit envelope: %w", err)
	}
	acceptedAt := nullableTime(status == EvidenceExportStatusAccepted, observedAt)
	lastObservedAt := nullableTime(status == EvidenceExportStatusAccepted, observedAt)
	row := s.pool.QueryRow(ctx, `
INSERT INTO qf_personal_evidence_exports (
    export_id, bundle_id, payload_hash, status, reason,
    target_context_type, target_context_ref, packet_id, trace_id,
    consent_scope, sensitivity_tier, source_artifact_ids,
    source_provenance_classes, audit_envelope, accepted_at,
    last_observed_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)
ON CONFLICT (export_id) DO UPDATE SET
    bundle_id = EXCLUDED.bundle_id,
    payload_hash = EXCLUDED.payload_hash,
    status = EXCLUDED.status,
    reason = EXCLUDED.reason,
    target_context_type = EXCLUDED.target_context_type,
    target_context_ref = EXCLUDED.target_context_ref,
    packet_id = EXCLUDED.packet_id,
    trace_id = EXCLUDED.trace_id,
    consent_scope = EXCLUDED.consent_scope,
    sensitivity_tier = EXCLUDED.sensitivity_tier,
    source_artifact_ids = EXCLUDED.source_artifact_ids,
    source_provenance_classes = EXCLUDED.source_provenance_classes,
    audit_envelope = EXCLUDED.audit_envelope,
    accepted_at = COALESCE(EXCLUDED.accepted_at, qf_personal_evidence_exports.accepted_at),
    last_observed_at = COALESCE(EXCLUDED.last_observed_at, qf_personal_evidence_exports.last_observed_at),
    updated_at = EXCLUDED.updated_at
RETURNING export_id, bundle_id, payload_hash, status, COALESCE(reason, ''),
    target_context_type, target_context_ref, packet_id, trace_id,
    consent_scope, sensitivity_tier, source_artifact_ids, source_provenance_classes,
    audit_envelope, created_at, updated_at, accepted_at, revoked_at, last_observed_at`, bundle.ExportID, bundle.BundleID, payloadHash, status, reason, targetContextType, targetContextRef, packetID, traceID, bundle.ConsentScope, bundle.SensitivityTier, sourceArtifactIDs, sourceClasses, auditJSON, acceptedAt, lastObservedAt, observedAt.UTC())
	return scanEvidenceExportRecord(row)
}

func (s *EvidenceExportStore) MarkIdempotentObserved(ctx context.Context, exportID string, observedAt time.Time) (EvidenceExportRecord, error) {
	if s == nil || s.pool == nil {
		return EvidenceExportRecord{}, fmt.Errorf("qf evidence export store requires postgres pool")
	}
	row := s.pool.QueryRow(ctx, `
UPDATE qf_personal_evidence_exports
SET last_observed_at = $2, updated_at = $2
WHERE export_id = $1
RETURNING export_id, bundle_id, payload_hash, status, COALESCE(reason, ''),
    target_context_type, target_context_ref, packet_id, trace_id,
    consent_scope, sensitivity_tier, source_artifact_ids, source_provenance_classes,
    audit_envelope, created_at, updated_at, accepted_at, revoked_at, last_observed_at`, exportID, observedAt.UTC())
	return scanEvidenceExportRecord(row)
}

func (s *EvidenceExportStore) MarkRevoked(ctx context.Context, exportID, status, reason string, audit EvidenceAuditEnvelope, observedAt time.Time) (EvidenceExportRecord, error) {
	if s == nil || s.pool == nil {
		return EvidenceExportRecord{}, fmt.Errorf("qf evidence export store requires postgres pool")
	}
	auditJSON, err := json.Marshal(audit)
	if err != nil {
		return EvidenceExportRecord{}, fmt.Errorf("marshal evidence revocation audit envelope: %w", err)
	}
	row := s.pool.QueryRow(ctx, `
UPDATE qf_personal_evidence_exports
SET status = $2, reason = $3, audit_envelope = $4, revoked_at = $5, last_observed_at = $5, updated_at = $5
WHERE export_id = $1
RETURNING export_id, bundle_id, payload_hash, status, COALESCE(reason, ''),
    target_context_type, target_context_ref, packet_id, trace_id,
    consent_scope, sensitivity_tier, source_artifact_ids, source_provenance_classes,
    audit_envelope, created_at, updated_at, accepted_at, revoked_at, last_observed_at`, exportID, status, reason, auditJSON, observedAt.UTC())
	return scanEvidenceExportRecord(row)
}

func EvidenceBundleForAuditFromRecord(record EvidenceExportRecord) PersonalEvidenceBundle {
	return PersonalEvidenceBundle{
		BundleID:          record.BundleID,
		ExportID:          record.ExportID,
		ConsentScope:      record.ConsentScope,
		SensitivityTier:   record.SensitivityTier,
		SourceArtifactIDs: append([]string(nil), record.SourceArtifactIDs...),
		TargetContext: map[string]any{
			TargetContextTypeKey:     record.TargetContextType,
			TargetContextRefKey:      record.TargetContextRef,
			TargetContextPacketIDKey: record.PacketID,
			TargetContextTraceIDKey:  record.TraceID,
		},
		SourceProvenanceClasses: append([]SourceProvenanceClass(nil), record.SourceProvenanceClasses...),
	}
}

func (s *EvidenceExportStore) Get(ctx context.Context, exportID string) (EvidenceExportRecord, bool, error) {
	if s == nil || s.pool == nil {
		return EvidenceExportRecord{}, false, fmt.Errorf("qf evidence export store requires postgres pool")
	}
	row := s.pool.QueryRow(ctx, `
SELECT export_id, bundle_id, payload_hash, status, COALESCE(reason, ''),
    target_context_type, target_context_ref, packet_id, trace_id,
    consent_scope, sensitivity_tier, source_artifact_ids, source_provenance_classes,
    audit_envelope, created_at, updated_at, accepted_at, revoked_at, last_observed_at
FROM qf_personal_evidence_exports WHERE export_id = $1`, exportID)
	record, err := scanEvidenceExportRecord(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return EvidenceExportRecord{}, false, nil
		}
		return EvidenceExportRecord{}, false, err
	}
	return record, true, nil
}

func scanEvidenceExportRecord(row pgx.Row) (EvidenceExportRecord, error) {
	var record EvidenceExportRecord
	var sourceArtifactIDs []byte
	var sourceClasses []byte
	var auditEnvelope []byte
	if err := row.Scan(&record.ExportID, &record.BundleID, &record.PayloadHash, &record.Status, &record.Reason, &record.TargetContextType, &record.TargetContextRef, &record.PacketID, &record.TraceID, &record.ConsentScope, &record.SensitivityTier, &sourceArtifactIDs, &sourceClasses, &auditEnvelope, &record.CreatedAt, &record.UpdatedAt, &record.AcceptedAt, &record.RevokedAt, &record.LastObservedAt); err != nil {
		return EvidenceExportRecord{}, err
	}
	if err := json.Unmarshal(sourceArtifactIDs, &record.SourceArtifactIDs); err != nil {
		return EvidenceExportRecord{}, fmt.Errorf("decode source artifact ids: %w", err)
	}
	if err := json.Unmarshal(sourceClasses, &record.SourceProvenanceClasses); err != nil {
		return EvidenceExportRecord{}, fmt.Errorf("decode source provenance classes: %w", err)
	}
	if len(auditEnvelope) > 0 {
		if err := json.Unmarshal(auditEnvelope, &record.AuditEnvelope); err != nil {
			return EvidenceExportRecord{}, fmt.Errorf("decode evidence audit envelope: %w", err)
		}
	}
	return record, nil
}

func nullableTime(enabled bool, observedAt time.Time) *time.Time {
	if !enabled {
		return nil
	}
	value := observedAt.UTC()
	return &value
}

-- 035_qf_personal_evidence_exports.sql
-- Spec 041 Scope 4: local state for QF PersonalEvidenceBundle exports and revocations.

CREATE TABLE IF NOT EXISTS qf_personal_evidence_exports (
    export_id                 TEXT PRIMARY KEY,
    bundle_id                 TEXT NOT NULL,
    payload_hash              TEXT NOT NULL,
    status                    TEXT NOT NULL,
    reason                    TEXT,
    target_context_type       TEXT NOT NULL,
    target_context_ref        TEXT NOT NULL,
    packet_id                 TEXT NOT NULL,
    trace_id                  TEXT NOT NULL,
    consent_scope             TEXT NOT NULL,
    sensitivity_tier          TEXT NOT NULL,
    source_artifact_ids       JSONB NOT NULL,
    source_provenance_classes JSONB NOT NULL,
    audit_envelope            JSONB NOT NULL,
    accepted_at               TIMESTAMPTZ,
    revoked_at                TIMESTAMPTZ,
    last_observed_at          TIMESTAMPTZ,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('accepted', 'local_reject', 'export_id_collision', 'export_id_previously_rejected', 'transport_failed', 'revoked', 'revoked_remote_missing'))
);

CREATE INDEX IF NOT EXISTS idx_qf_personal_evidence_exports_status ON qf_personal_evidence_exports (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_qf_personal_evidence_exports_packet ON qf_personal_evidence_exports (packet_id, trace_id);
CREATE INDEX IF NOT EXISTS idx_qf_personal_evidence_exports_consent ON qf_personal_evidence_exports (consent_scope, status);
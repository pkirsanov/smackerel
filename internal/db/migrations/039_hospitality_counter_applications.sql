-- 039_hospitality_counter_applications.sql
-- BUG-013-001 (sweep R5 / stabilize-to-doc): introduce an idempotency ledger for
-- hospitality counter mutations applied by internal/graph/hospitality_linker.go.
--
-- Problem: HospitalityLinker.LinkArtifact calls GuestRepository.IncrementStay,
-- PropertyRepository.IncrementBookings, and PropertyRepository.UpdateIssueCount
-- unconditionally. The artifacts.processed NATS consumer
-- (internal/pipeline/subscriber.go) is configured with
-- AckPolicy=AckExplicitPolicy, AckWait=30s, and MaxDeliver>1, which means any
-- of the following triggers a redelivery of the SAME artifact_id through
-- Processor.HandleProcessedResult → HospitalityLinker.LinkArtifact:
--   * AckWait expiry during slow DB latency
--   * Process crash between the artifacts UPDATE and msg.Ack()
--   * Transient HandleProcessedResult error producing a Nak (retry counted
--     against MaxDeliver)
-- On every redelivery the counters drift: guests.total_stays / total_spend,
-- properties.total_bookings / total_revenue, and properties.issue_count all
-- get re-applied.
--
-- Fix: track which (artifact_id, op_kind) pairs have already had their counter
-- side-effect applied. The linker claims a row with ON CONFLICT DO NOTHING
-- before invoking the counter mutator. Edge writes remain unchanged because
-- they are already idempotent via ON CONFLICT DO UPDATE on the edges unique
-- constraint.

CREATE TABLE IF NOT EXISTS hospitality_counter_applications (
    artifact_id TEXT NOT NULL,
    op_kind     TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (artifact_id, op_kind),
    CHECK (op_kind IN (
        'guest_stay_increment',
        'property_booking_increment',
        'property_issue_delta'
    ))
);

CREATE INDEX IF NOT EXISTS idx_hospitality_counter_applications_applied_at
    ON hospitality_counter_applications (applied_at);

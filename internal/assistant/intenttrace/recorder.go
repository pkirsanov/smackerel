// Spec 071 SCOPE-01 — IntentTrace recorder + Postgres store.
//
// The recorder validates the input, computes the canonical redacted
// payload + payload hash, and forwards to the store. A no-op recorder
// is provided so the facade can be wired without a backing Postgres
// pool in unit tests. The Postgres store implements both sampled and
// sampled-out persistence against the assistant_intent_traces table
// from migration 046.

package intenttrace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NopRecorder is a recorder that returns Recorded=false without
// touching any store. Used in tests where trace persistence is not
// under test, and as the facade default when no recorder is wired.
type NopRecorder struct{}

// Record implements IntentTraceRecorder.
func (NopRecorder) Record(_ context.Context, in TurnTraceInput) (IntentTraceResult, error) {
	return IntentTraceResult{TraceID: in.TraceID, Recorded: false, WasSampled: in.Sampled}, nil
}

// StoreRecorder is the production recorder. It validates the input,
// builds the redacted payload + IntentTraceRow, and forwards to the
// underlying store. RetentionWindow is added to EmittedAt to compute
// ExpiresAt (Scope 2 sweep consumes this column). Exporter (Scope 2)
// is invoked AFTER a successful Put so derived sinks (logs, metrics,
// OTel) see the same row that was persisted (Hard Constraint:
// single source of truth, no parallel telemetry path).
type StoreRecorder struct {
	Store           IntentTraceStore
	RetentionWindow time.Duration
	Exporter        Exporter
}

// NewStoreRecorder constructs a StoreRecorder. Exporter defaults to
// NopExporter when nil so unit tests do not need to wire fan-out.
func NewStoreRecorder(store IntentTraceStore, retention time.Duration) *StoreRecorder {
	return &StoreRecorder{Store: store, RetentionWindow: retention, Exporter: NopExporter{}}
}

// WithExporter returns a copy of r with the supplied Exporter
// installed. Used by cmd/core wiring to attach the SST-resolved
// DefaultExporter.
func (r *StoreRecorder) WithExporter(e Exporter) *StoreRecorder {
	if r == nil {
		return nil
	}
	if e == nil {
		e = NopExporter{}
	}
	clone := *r
	clone.Exporter = e
	return &clone
}

// Record implements IntentTraceRecorder.
func (r *StoreRecorder) Record(ctx context.Context, in TurnTraceInput) (IntentTraceResult, error) {
	if r == nil || r.Store == nil {
		return IntentTraceResult{}, errors.New("intenttrace: StoreRecorder requires a non-nil Store")
	}
	if r.RetentionWindow <= 0 {
		return IntentTraceResult{}, errors.New("intenttrace: RetentionWindow must be > 0")
	}
	if err := validateTurnTraceInput(in); err != nil {
		return IntentTraceResult{}, err
	}
	emittedAt := in.EmittedAt
	if emittedAt.IsZero() {
		emittedAt = time.Now().UTC()
	}
	emittedAt = emittedAt.UTC()
	row := buildRow(in, emittedAt, r.RetentionWindow)
	if err := r.Store.Put(ctx, row); err != nil {
		return IntentTraceResult{}, fmt.Errorf("intenttrace: store put: %w", err)
	}
	hash, err := canonicalPayloadHash(row.RedactedPayload)
	if err != nil {
		return IntentTraceResult{}, fmt.Errorf("intenttrace: payload hash: %w", err)
	}
	if r.Exporter != nil {
		r.Exporter.Export(ctx, row)
	}
	return IntentTraceResult{
		TraceID:     row.TraceID,
		Recorded:    true,
		WasSampled:  in.Sampled,
		PayloadHash: hash,
	}, nil
}

func validateTurnTraceInput(in TurnTraceInput) error {
	if in.TraceID == "" {
		return errors.New("intenttrace: TraceID is required")
	}
	if in.TurnID == "" {
		return errors.New("intenttrace: TurnID is required")
	}
	if in.UserIDHash == "" {
		return errors.New("intenttrace: UserIDHash is required")
	}
	if !isKnownTransport(in.Transport) {
		return fmt.Errorf("intenttrace: unknown transport %q", in.Transport)
	}
	if in.TransportMessageID == "" {
		return errors.New("intenttrace: TransportMessageID is required")
	}
	if in.Sampled {
		if in.ActionClass == "" {
			return errors.New("intenttrace: ActionClass is required for sampled traces")
		}
		if in.SideEffectClass == "" {
			return errors.New("intenttrace: SideEffectClass is required for sampled traces")
		}
		if !isKnownStatus(in.FinalResponseStatus) {
			return fmt.Errorf("intenttrace: unknown final_response_status %q", in.FinalResponseStatus)
		}
		if in.SlotsRedactionSummary.RawText == "" {
			return errors.New("intenttrace: SlotsRedactionSummary.RawText is required for sampled traces")
		}
	} else {
		if in.SampledOutReason == "" {
			return errors.New("intenttrace: SampledOutReason is required for sampled-out envelopes")
		}
	}
	return nil
}

func isKnownTransport(t Transport) bool {
	for _, allowed := range AllTransports {
		if allowed == t {
			return true
		}
	}
	return false
}

func isKnownStatus(s FinalResponseStatus) bool {
	for _, allowed := range AllFinalResponseStatuses {
		if allowed == s {
			return true
		}
	}
	return false
}

func buildRow(in TurnTraceInput, emittedAt time.Time, retention time.Duration) IntentTraceRow {
	tools := in.ToolCalls
	if tools == nil {
		tools = []ToolCallSummary{}
	}
	slots := in.SlotsRedactionSummary
	if slots.SlotClasses == nil {
		slots.SlotClasses = map[string]string{}
	}
	if !in.Sampled {
		// Minimal envelope — clear full-trace fields to keep the
		// redacted payload provably empty for replay readers.
		slots = SlotsRedactionSummary{RawText: "absent", SlotClasses: map[string]string{}}
		tools = []ToolCallSummary{}
	}
	payload := RedactedPayload{
		SchemaVersion:         SchemaVersionV1,
		TraceID:               in.TraceID,
		TurnID:                in.TurnID,
		UserIDHash:            in.UserIDHash,
		Transport:             in.Transport,
		TransportMessageID:    in.TransportMessageID,
		Sampled:               in.Sampled,
		SampledOutReason:      in.SampledOutReason,
		CompilerInvoked:       in.CompilerInvoked,
		ActionClass:           in.ActionClass,
		SideEffectClass:       in.SideEffectClass,
		Confidence:            in.Confidence,
		RouteDecision:         in.RouteDecision,
		ToolCalls:             tools,
		FinalResponseStatus:   in.FinalResponseStatus,
		RefusalCause:          in.RefusalCause,
		CaptureCause:          in.CaptureCause,
		IdeaArtifactID:        in.IdeaArtifactID,
		ModelRoute:            in.ModelRoute,
		Seed:                  in.Seed,
		SlotsRedactionSummary: slots,
	}
	return IntentTraceRow{
		TraceID:               in.TraceID,
		SchemaVersion:         SchemaVersionV1,
		TurnID:                in.TurnID,
		UserIDHash:            in.UserIDHash,
		Transport:             in.Transport,
		TransportMessageID:    in.TransportMessageID,
		Sampled:               in.Sampled,
		SampledOutReason:      in.SampledOutReason,
		ActionClass:           in.ActionClass,
		SideEffectClass:       in.SideEffectClass,
		Confidence:            in.Confidence,
		RouteDecision:         in.RouteDecision,
		ToolCalls:             tools,
		FinalResponseStatus:   in.FinalResponseStatus,
		CompilerInvoked:       in.CompilerInvoked,
		ModelRoute:            in.ModelRoute,
		Seed:                  in.Seed,
		RefusalCause:          in.RefusalCause,
		CaptureCause:          in.CaptureCause,
		IdeaArtifactID:        in.IdeaArtifactID,
		AgentTraceID:          in.AgentTraceID,
		SlotsRedactionSummary: slots,
		RedactedPayload:       payload,
		EmittedAt:             emittedAt,
		ExpiresAt:             emittedAt.Add(retention),
	}
}

func canonicalPayloadHash(p RedactedPayload) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// PostgresStore implements IntentTraceStore against
// assistant_intent_traces (migration 046).
type PostgresStore struct {
	Pool *pgxpool.Pool
}

// NewPostgresStore constructs a PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{Pool: pool}
}

const putSQL = `
INSERT INTO assistant_intent_traces (
    trace_id, schema_version, turn_id, user_id_hash, transport,
    transport_message_id, sampled, sampled_out_reason, action_class,
    side_effect_class, confidence, route_decision, tool_calls,
    final_response_status, compiler_invoked, model_route, seed,
    refusal_cause, capture_cause, idea_artifact_id, agent_trace_id,
    slots_redaction_summary, redacted_payload, emitted_at, expires_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, NULLIF($8, ''), $9,
    $10, $11, NULLIF($12, ''), $13,
    $14, $15, NULLIF($16, ''), NULLIF($17, ''),
    NULLIF($18, ''), NULLIF($19, ''), NULLIF($20, ''), NULLIF($21, ''),
    $22, $23, $24, $25
)
`

// Put implements IntentTraceStore.
func (s *PostgresStore) Put(ctx context.Context, row IntentTraceRow) error {
	if s == nil || s.Pool == nil {
		return errors.New("intenttrace: PostgresStore requires a non-nil Pool")
	}
	toolsJSON, err := json.Marshal(row.ToolCalls)
	if err != nil {
		return fmt.Errorf("intenttrace: marshal tool_calls: %w", err)
	}
	slotsJSON, err := json.Marshal(row.SlotsRedactionSummary)
	if err != nil {
		return fmt.Errorf("intenttrace: marshal slots_redaction_summary: %w", err)
	}
	payloadJSON, err := json.Marshal(row.RedactedPayload)
	if err != nil {
		return fmt.Errorf("intenttrace: marshal redacted_payload: %w", err)
	}
	var confidence any
	if row.Confidence != nil {
		confidence = *row.Confidence
	}
	_, err = s.Pool.Exec(ctx, putSQL,
		row.TraceID, row.SchemaVersion, row.TurnID, row.UserIDHash, string(row.Transport),
		row.TransportMessageID, row.Sampled, row.SampledOutReason, row.ActionClass,
		row.SideEffectClass, confidence, row.RouteDecision, toolsJSON,
		string(row.FinalResponseStatus), row.CompilerInvoked, row.ModelRoute, row.Seed,
		row.RefusalCause, row.CaptureCause, row.IdeaArtifactID, row.AgentTraceID,
		slotsJSON, payloadJSON, row.EmittedAt, row.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("intenttrace: insert row: %w", err)
	}
	return nil
}

const getSQL = `
SELECT trace_id, schema_version, turn_id, user_id_hash, transport,
       transport_message_id, sampled, COALESCE(sampled_out_reason, ''),
       action_class, side_effect_class, confidence,
       COALESCE(route_decision, ''), tool_calls,
       final_response_status, compiler_invoked,
       COALESCE(model_route, ''), COALESCE(seed, ''),
       COALESCE(refusal_cause, ''), COALESCE(capture_cause, ''),
       COALESCE(idea_artifact_id, ''), COALESCE(agent_trace_id, ''),
       slots_redaction_summary, redacted_payload, emitted_at, expires_at
FROM assistant_intent_traces
WHERE trace_id = $1
`

// Get implements IntentTraceStore.
func (s *PostgresStore) Get(ctx context.Context, traceID string) (IntentTraceRow, error) {
	if s == nil || s.Pool == nil {
		return IntentTraceRow{}, errors.New("intenttrace: PostgresStore requires a non-nil Pool")
	}
	var (
		row         IntentTraceRow
		transport   string
		status      string
		toolsJSON   []byte
		slotsJSON   []byte
		payloadJSON []byte
		confidence  *float64
		schemaVer   string
	)
	err := s.Pool.QueryRow(ctx, getSQL, traceID).Scan(
		&row.TraceID, &schemaVer, &row.TurnID, &row.UserIDHash, &transport,
		&row.TransportMessageID, &row.Sampled, &row.SampledOutReason,
		&row.ActionClass, &row.SideEffectClass, &confidence,
		&row.RouteDecision, &toolsJSON,
		&status, &row.CompilerInvoked,
		&row.ModelRoute, &row.Seed,
		&row.RefusalCause, &row.CaptureCause,
		&row.IdeaArtifactID, &row.AgentTraceID,
		&slotsJSON, &payloadJSON, &row.EmittedAt, &row.ExpiresAt,
	)
	if err != nil {
		return IntentTraceRow{}, fmt.Errorf("intenttrace: get %s: %w", traceID, err)
	}
	row.SchemaVersion = schemaVer
	row.Transport = Transport(transport)
	row.FinalResponseStatus = FinalResponseStatus(status)
	row.Confidence = confidence
	if err := json.Unmarshal(toolsJSON, &row.ToolCalls); err != nil {
		return IntentTraceRow{}, fmt.Errorf("intenttrace: unmarshal tool_calls: %w", err)
	}
	if row.ToolCalls == nil {
		row.ToolCalls = []ToolCallSummary{}
	}
	if err := json.Unmarshal(slotsJSON, &row.SlotsRedactionSummary); err != nil {
		return IntentTraceRow{}, fmt.Errorf("intenttrace: unmarshal slots_redaction_summary: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &row.RedactedPayload); err != nil {
		return IntentTraceRow{}, fmt.Errorf("intenttrace: unmarshal redacted_payload: %w", err)
	}
	return row, nil
}

const sweepSQL = `
DELETE FROM assistant_intent_traces
WHERE expires_at <= $1
`

// SweepExpired implements IntentTraceStore.
func (s *PostgresStore) SweepExpired(ctx context.Context, now time.Time) (SweepResult, error) {
	if s == nil || s.Pool == nil {
		return SweepResult{}, errors.New("intenttrace: PostgresStore requires a non-nil Pool")
	}
	tag, err := s.Pool.Exec(ctx, sweepSQL, now.UTC())
	if err != nil {
		return SweepResult{}, fmt.Errorf("intenttrace: sweep expired: %w", err)
	}
	return SweepResult{Deleted: int(tag.RowsAffected()), SweptAt: now.UTC()}, nil
}

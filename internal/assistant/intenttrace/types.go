// Spec 071 SCOPE-01 — IntentTrace observability contract types.
//
// This package defines the versioned v1 IntentTrace contract emitted
// exactly once per compiled assistant turn (full trace) or once per
// sampled-out turn (minimal envelope). The schema is pinned by the
// golden contract test in this package; any field rename, type change,
// or required-field change MUST bump SchemaVersionV1 (and add a new
// closed vocabulary constant).
//
// Scope 1 owns the contract, configuration, migration, recorder
// interface, and Postgres store. Scope 2 owns sampling, redaction,
// metric/log export, and retention sweep. Scope 3 owns replay and
// the spec 067 bypass-guard read path.

package intenttrace

import (
	"context"
	"time"
)

// SchemaVersionV1 is the only currently-accepted schema version. The
// migration in internal/db/migrations/046_assistant_intent_traces.sql
// pins the same string via CHECK constraint.
const SchemaVersionV1 = "v1"

// Closed vocabularies. Every value below is part of the v1 contract;
// changes require a SchemaVersion bump.

// Transport is the inbound transport identifier.
type Transport string

const (
	TransportTelegram Transport = "telegram"
	TransportWhatsApp Transport = "whatsapp"
	TransportWeb      Transport = "web"
	TransportMobile   Transport = "mobile"
)

// AllTransports enumerates the v1 transports.
var AllTransports = []Transport{
	TransportTelegram, TransportWhatsApp, TransportWeb, TransportMobile,
}

// FinalResponseStatus is the closed vocabulary of terminal statuses
// reported back to the user. Spec 061 status tokens map onto this.
type FinalResponseStatus string

const (
	StatusOK              FinalResponseStatus = "ok"
	StatusClarify         FinalResponseStatus = "clarify"
	StatusRefused         FinalResponseStatus = "refused"
	StatusCaptureFallback FinalResponseStatus = "capture_fallback"
	StatusUnavailable     FinalResponseStatus = "unavailable"
	StatusCheckingWeather FinalResponseStatus = "checking_weather"
)

// AllFinalResponseStatuses enumerates the v1 statuses.
var AllFinalResponseStatuses = []FinalResponseStatus{
	StatusOK, StatusClarify, StatusRefused, StatusCaptureFallback,
	StatusUnavailable, StatusCheckingWeather,
}

// SampledOutReason explains why a turn produced only an envelope.
type SampledOutReason string

const (
	SampledOutDeterministic SampledOutReason = "deterministic_sampling"
)

// ToolCallSummary is the redacted per-tool-call record stored on a
// full trace. Arguments are NEVER stored raw; the recorder reports
// only whether they were redacted and the outcome.
type ToolCallSummary struct {
	Name              string `json:"name"`
	ArgumentsRedacted bool   `json:"arguments_redacted"`
	Outcome           string `json:"outcome"`
}

// SlotsRedactionSummary is the typed redaction summary attached to a
// full trace. Slot values are never stored; only counts and per-class
// disposition labels.
type SlotsRedactionSummary struct {
	RawText       string            `json:"raw_text"` // "absent" | "present"
	SlotClasses   map[string]string `json:"slot_classes"`
	RedactedCount int               `json:"redacted_count"`
}

// RedactedPayload is the JSONB payload persisted on each row. It is
// the canonical replay source.
type RedactedPayload struct {
	SchemaVersion         string                `json:"schema_version"`
	TraceID               string                `json:"trace_id"`
	TurnID                string                `json:"turn_id"`
	UserIDHash            string                `json:"user_id_hash"`
	Transport             Transport             `json:"transport"`
	TransportMessageID    string                `json:"transport_message_id"`
	Sampled               bool                  `json:"sampled"`
	SampledOutReason      string                `json:"sampled_out_reason,omitempty"`
	CompilerInvoked       bool                  `json:"compiler_invoked"`
	ActionClass           string                `json:"action_class"`
	SideEffectClass       string                `json:"side_effect_class"`
	Confidence            *float64              `json:"confidence,omitempty"`
	RouteDecision         string                `json:"route_decision,omitempty"`
	ToolCalls             []ToolCallSummary     `json:"tool_calls"`
	FinalResponseStatus   FinalResponseStatus   `json:"final_response_status"`
	RefusalCause          string                `json:"refusal_cause,omitempty"`
	CaptureCause          string                `json:"capture_cause,omitempty"`
	IdeaArtifactID        string                `json:"idea_artifact_id,omitempty"`
	ModelRoute            string                `json:"model_route,omitempty"`
	Seed                  string                `json:"seed,omitempty"`
	SlotsRedactionSummary SlotsRedactionSummary `json:"slots_redaction_summary"`
}

// IntentTraceRow is the persisted row written to assistant_intent_traces.
// Field order matches the SQL DDL in migration 046.
type IntentTraceRow struct {
	TraceID               string
	SchemaVersion         string
	TurnID                string
	UserIDHash            string
	Transport             Transport
	TransportMessageID    string
	Sampled               bool
	SampledOutReason      string
	ActionClass           string
	SideEffectClass       string
	Confidence            *float64
	RouteDecision         string
	ToolCalls             []ToolCallSummary
	FinalResponseStatus   FinalResponseStatus
	CompilerInvoked       bool
	ModelRoute            string
	Seed                  string
	RefusalCause          string
	CaptureCause          string
	IdeaArtifactID        string
	AgentTraceID          string
	SlotsRedactionSummary SlotsRedactionSummary
	RedactedPayload       RedactedPayload
	EmittedAt             time.Time
	ExpiresAt             time.Time
}

// TurnTraceInput is the recorder's input contract: everything the
// recorder needs to assemble one IntentTraceRow. The recorder
// (Scope 1) accepts the already-redacted payload from the caller for
// now; Scope 2 will move redaction inside the recorder behind an
// IntentTraceRedactor.
type TurnTraceInput struct {
	TraceID            string
	TurnID             string
	UserIDHash         string
	Transport          Transport
	TransportMessageID string
	CompilerInvoked    bool
	// Full-trace fields. Required when Sampled=true.
	ActionClass         string
	SideEffectClass     string
	Confidence          *float64
	RouteDecision       string
	ToolCalls           []ToolCallSummary
	FinalResponseStatus FinalResponseStatus
	ModelRoute          string
	Seed                string
	RefusalCause        string
	CaptureCause        string
	IdeaArtifactID      string
	AgentTraceID        string
	// Sampled is the deterministic sampling decision; when false the
	// recorder writes a minimal sampled-out envelope (Scope 2 wires
	// the deterministic sampler that flips this).
	Sampled          bool
	SampledOutReason string
	// SlotsRedactionSummary describes what was redacted. The caller
	// MUST populate this; the recorder validates non-nil maps.
	SlotsRedactionSummary SlotsRedactionSummary
	EmittedAt             time.Time
}

// IntentTraceResult is the outcome of a Record() call.
type IntentTraceResult struct {
	TraceID     string
	Recorded    bool
	WasSampled  bool
	PayloadHash string // sha256 hex of the canonical JSON, useful for replay audits
}

// IntentTraceRecorder accepts one compiled turn and writes exactly
// one trace/envelope.
type IntentTraceRecorder interface {
	Record(ctx context.Context, in TurnTraceInput) (IntentTraceResult, error)
}

// SweepResult reports the outcome of an IntentTraceStore TTL sweep.
// Spec 071 SCOPE-02 — retention TTL enforcement (SCN-071-A09). The
// sweep emits counts only; raw row content is never carried out of
// the store.
type SweepResult struct {
	Deleted int
	SweptAt time.Time
}

// IntentTraceStore persists rows, reads them back by trace id, and
// (Scope 2) sweeps expired rows. Scope 3 adds the typed errors
// required by the replay CLI.
type IntentTraceStore interface {
	Put(ctx context.Context, row IntentTraceRow) error
	Get(ctx context.Context, traceID string) (IntentTraceRow, error)
	SweepExpired(ctx context.Context, now time.Time) (SweepResult, error)
}

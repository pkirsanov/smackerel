// Package httpadapter implements the HTTP transport adapter (spec
// 069). It is the second concrete contracts.TransportAdapter peer to
// the Telegram adapter. SCOPE-1a lands the foundation: schema v1
// request/response types, JSON translation/rendering, the route
// handler, and SST config keys under assistant.transports.http.*.
//
// Schema v1 is pinned by golden_contract_test.go. Adding, removing,
// renaming, or retyping any wire field — including in nested types —
// MUST bump SchemaVersionV1 and update the golden fixtures.
package httpadapter

import (
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// SchemaVersionV1 is the literal value the request and response
// JSON contracts carry in their schema_version field. Any wire
// change MUST bump this constant and the golden fixtures.
const SchemaVersionV1 = "v1"

// TransportName is the closed-vocabulary contracts.AssistantMessage
// transport token for the HTTP adapter. Must match the value
// validated by spec 061's AssistantMessage.Transport contract.
const TransportName = "web"

// AllowedTransportHints is the closed vocabulary the request schema
// accepts in transport_hint. An unknown hint rejects before facade
// invocation. Hints never affect routing, tools, or response shape
// (spec 069 design §"hints are telemetry only").
var AllowedTransportHints = []string{"web", "mobile", "bridge"}

// TurnRequest is the v1 wire request for POST /api/assistant/turn.
//
// MANDATORY fields: SchemaVersion, TransportMessageID, Kind.
// Callback kinds require their refs/choices; validation lives in
// (TurnRequest).Validate.
type TurnRequest struct {
	SchemaVersion        string         `json:"schema_version"`
	TransportMessageID   string         `json:"transport_message_id"`
	Kind                 string         `json:"kind"`
	TransportHint        string         `json:"transport_hint"`
	Text                 string         `json:"text"`
	ConfirmRef           string         `json:"confirm_ref"`
	ConfirmChoice        string         `json:"confirm_choice"`
	DisambiguationRef    string         `json:"disambiguation_ref"`
	DisambiguationChoice int            `json:"disambiguation_choice"`
	ClientContext        map[string]any `json:"client_context"`
}

// TurnResponse is the v1 wire response for POST /api/assistant/turn.
// Every field is always serialized so the wire shape is stable
// across success, error, confirm, and disambiguation paths.
type TurnResponse struct {
	SchemaVersion        string              `json:"schema_version"`
	Transport            string              `json:"transport"`
	TransportMessageID   string              `json:"transport_message_id"`
	Status               string              `json:"status"`
	Body                 string              `json:"body"`
	Sources              []SourceJSON        `json:"sources"`
	SourcesOverflowCount int                 `json:"sources_overflow_count"`
	ConfirmCard          *ConfirmCardJSON    `json:"confirm_card"`
	DisambiguationPrompt *DisambiguationJSON `json:"disambiguation_prompt"`
	ErrorCause           string              `json:"error_cause"`
	CaptureRoute         bool                `json:"capture_route"`
	Trace                TraceJSON           `json:"trace"`
	FacadeInvoked        bool                `json:"facade_invoked"`
	EmittedAt            string              `json:"emitted_at"`
	// Notice is the OPTIONAL legacy-retirement deprecation notice
	// payload (spec 075 SCOPE-075-06.2b). Additive v1-compatible
	// optional field: when nil it is omitted from the wire body and
	// schema_version remains "v1".
	Notice *NoticeJSON `json:"notice,omitempty"`
}

// NoticeJSON is the wire rendering of contracts.NoticePayload (the
// legacy-retirement deprecation-notice metadata). Spec 075
// SCOPE-075-06.2b.
type NoticeJSON struct {
	Command            string `json:"command"`
	ReplacementExample string `json:"replacement_example"`
	CopyKey            string `json:"copy_key"`
	WindowID           string `json:"window_id"`
}

// SourceJSON is the wire rendering of contracts.Source. Ref-shape
// fields are flattened with discriminator-keyed prefixes so the
// schema stays a flat object.
type SourceJSON struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	Kind                  string `json:"kind"`
	ArtifactID            string `json:"artifact_id"`
	ArtifactCapturedAt    string `json:"artifact_captured_at"`
	ProviderName          string `json:"provider_name"`
	ProviderRetrievedAt   string `json:"provider_retrieved_at"`
	URL                   string `json:"url"`
	WebProvider           string `json:"web_provider"`
	WebFetchedAt          string `json:"web_fetched_at"`
	WebContentHash        string `json:"web_content_hash"`
	WebSnippet            string `json:"web_snippet"`
	ComputationTool       string `json:"computation_tool"`
	ComputationInputHash  string `json:"computation_input_hash"`
	ComputationOutputHash string `json:"computation_output_hash"`
}

// ConfirmCardJSON is the wire rendering of contracts.ConfirmCard.
type ConfirmCardJSON struct {
	ProposedAction string `json:"proposed_action"`
	ConfirmRef     string `json:"confirm_ref"`
	PositiveLabel  string `json:"positive_label"`
	NegativeLabel  string `json:"negative_label"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// DisambiguationJSON is the wire rendering of
// contracts.DisambiguationPrompt.
type DisambiguationJSON struct {
	DisambiguationRef string                     `json:"disambiguation_ref"`
	TimeoutSeconds    int                        `json:"timeout_seconds"`
	Choices           []DisambiguationChoiceJSON `json:"choices"`
}

// DisambiguationChoiceJSON is the wire rendering of
// contracts.DisambiguationChoice.
type DisambiguationChoiceJSON struct {
	Number   int    `json:"number"`
	ID       string `json:"id"`
	Label    string `json:"label"`
	Shortcut string `json:"shortcut"`
}

// TraceJSON carries identifiers a caller (test or operator) uses to
// correlate a turn with the audit + agent-trace substrates.
type TraceJSON struct {
	AssistantTurnID string `json:"assistant_turn_id"`
	AgentTraceID    string `json:"agent_trace_id"`
	RequestID       string `json:"request_id"`
}

// HTTPTransportConfig holds the SST-resolved
// assistant.transports.http.* values consumed by the adapter and the
// later auth/limit middleware (SCOPE-2). Every field is REQUIRED at
// the SST boundary (Gate G028 / smackerel-no-defaults).
type HTTPTransportConfig struct {
	Enabled                   bool
	SchemaVersion             string
	BodySizeMaxBytes          int
	RateLimitPerUserPerMinute int
	CORSAllowedOrigins        []string
	ConversationTTL           time.Duration
	TransportHintAllowlist    []string
	RequiredScope             string
}

// allowedKinds is the closed vocabulary the wire request accepts.
// It mirrors contracts.AllMessageKinds verbatim; declared as a
// separate slice so the wire schema is decoupled from the in-memory
// enum if the latter ever extends without a corresponding wire bump.
var allowedKinds = []contracts.MessageKind{
	contracts.KindText,
	contracts.KindConfirm,
	contracts.KindDisambiguation,
	contracts.KindReset,
}

// Package schema is the canonical wire-schema source for the
// assistant turn HTTP contract (spec 069 schema_version="v1").
//
// This package is the single source of truth that downstream
// codegen pipelines consume:
//   - Web codegen (spec 073 Scope 1c) emits TypeScript from
//     assistant_turn_v1.json into web/pwa/generated/.
//   - Flutter shared-core codegen (spec 073 Scope 1d) emits Dart
//     from the same artifact into clients/mobile/assistant/.
//
// The Go contract types declared here mirror assistant_turn_v1.json
// field-for-field. The golden contract test in this package pins
// the schema artifact, the Go types, and the canonical request and
// response fixtures together — any drift between them fails the
// test before any downstream client consumes incompatible types.
//
// Adding, removing, renaming, or retyping any wire field — at the
// top level or in a nested definition — MUST bump SchemaVersionV1
// and update the canonical fixtures plus every downstream
// generated artifact in lockstep.
package schema

// SchemaVersionV1 is the literal value the v1 request and response
// contracts carry in their schema_version field.
const SchemaVersionV1 = "v1"

// AllowedTransportHints is the closed vocabulary the request schema
// accepts in transport_hint. Hints are telemetry only; they never
// affect routing, tools, or response shape.
var AllowedTransportHints = []string{"web", "mobile", "bridge"}

// AllowedKinds is the closed vocabulary the request schema accepts
// in kind.
var AllowedKinds = []string{"text", "confirm", "disambiguation", "reset"}

// TurnRequest is the v1 wire request for POST /api/assistant/turn.
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
	SchemaVersion        string          `json:"schema_version"`
	Transport            string          `json:"transport"`
	TransportMessageID   string          `json:"transport_message_id"`
	Status               string          `json:"status"`
	Body                 string          `json:"body"`
	Sources              []Source        `json:"sources"`
	SourcesOverflowCount int             `json:"sources_overflow_count"`
	ConfirmCard          *ConfirmCard    `json:"confirm_card"`
	DisambiguationPrompt *Disambiguation `json:"disambiguation_prompt"`
	ErrorCause           string          `json:"error_cause"`
	CaptureRoute         bool            `json:"capture_route"`
	Trace                Trace           `json:"trace"`
	FacadeInvoked        bool            `json:"facade_invoked"`
	EmittedAt            string          `json:"emitted_at"`
	// Notice is the OPTIONAL legacy-retirement deprecation notice
	// payload (spec 075 SCOPE-075-06.2b). Additive v1-compatible
	// field: when nil it is omitted from the wire body entirely;
	// schema_version remains "v1".
	Notice *NoticePayload `json:"notice,omitempty"`
}

// NoticePayload is the OPTIONAL legacy-retirement deprecation notice
// metadata (spec 075 SCOPE-075-06.2b) attached to TurnResponse when
// the facade Policy decides the turn is the first one in the dedup
// ledger for (user, retired_command, window_id).
type NoticePayload struct {
	Command            string `json:"command"`
	ReplacementExample string `json:"replacement_example"`
	CopyKey            string `json:"copy_key"`
	WindowID           string `json:"window_id"`
}

// Source is the wire rendering of a single assistant response
// citation. Ref-shape fields are flattened with discriminator-keyed
// prefixes so the schema stays a flat object.
type Source struct {
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

// ConfirmCard is the wire rendering of a confirmation prompt.
type ConfirmCard struct {
	ProposedAction string `json:"proposed_action"`
	ConfirmRef     string `json:"confirm_ref"`
	PositiveLabel  string `json:"positive_label"`
	NegativeLabel  string `json:"negative_label"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// Disambiguation is the wire rendering of a disambiguation prompt.
type Disambiguation struct {
	DisambiguationRef string                 `json:"disambiguation_ref"`
	TimeoutSeconds    int                    `json:"timeout_seconds"`
	Choices           []DisambiguationChoice `json:"choices"`
}

// DisambiguationChoice is one option within a Disambiguation prompt.
type DisambiguationChoice struct {
	Number   int    `json:"number"`
	ID       string `json:"id"`
	Label    string `json:"label"`
	Shortcut string `json:"shortcut"`
}

// Trace carries identifiers a caller uses to correlate a turn with
// the audit and agent-trace substrates.
type Trace struct {
	AssistantTurnID string `json:"assistant_turn_id"`
	AgentTraceID    string `json:"agent_trace_id"`
	RequestID       string `json:"request_id"`
}

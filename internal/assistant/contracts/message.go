package contracts

import "time"

// AssistantMessage is the canonical inbound message handed to the
// capability layer by any TransportAdapter. It is trivially convertible
// to an agent.IntentEnvelope (see the facade in the parent package).
//
// Source of truth: design.md §2.1.
type AssistantMessage struct {
	// UserID is resolved by the adapter from the transport identity
	// (chat_id, phone number, web session, etc.) per spec 044.
	UserID string

	// Transport names the inbound transport. Closed vocabulary:
	// "telegram", "whatsapp", "web", "mobile". The facade and audit
	// layer key conversation state on (UserID, Transport).
	Transport string

	// TransportMessageID is opaque to the capability layer and used by
	// the adapter for its own idempotency / dedupe.
	TransportMessageID string

	// Text is the plain-text body with transport markup already
	// stripped by the adapter.
	Text string

	// Kind discriminates the four supported inbound message shapes.
	Kind MessageKind

	// ConfirmRef echoes a prior AssistantResponse.ConfirmCard.ConfirmRef
	// when Kind == KindConfirm. Otherwise empty.
	ConfirmRef string

	// ConfirmChoice carries the user's positive/negative response when
	// Kind == KindConfirm. Otherwise the zero value.
	ConfirmChoice ConfirmChoice

	// DisambiguationRef echoes a prior
	// AssistantResponse.DisambiguationPrompt.DisambiguationRef when
	// Kind == KindDisambiguation.
	DisambiguationRef string

	// DisambiguationChoice is the 1-indexed selection from the prior
	// DisambiguationPrompt.Choices list when Kind == KindDisambiguation.
	DisambiguationChoice int

	// Attachments is unused in v1 but reserved on the canonical type
	// so future capability extensions need not break the contract.
	Attachments []Attachment

	// ReceivedAt is the adapter-side observe time.
	ReceivedAt time.Time

	// TransportMetadata is opaque to the capability layer. Adapters
	// MAY use it to round-trip per-transport hints (e.g. message
	// thread id) through the audit layer.
	TransportMetadata map[string]string

	// ModelOverride is the spec 088 per-request, runtime-switchable
	// model selection for the open-knowledge /ask agent. UNTRUSTED:
	// it is a user-supplied raw model string (e.g. parsed from a
	// Telegram `/ask --model=<id>` flag) that MUST be validated against
	// the switchable-model allowlist (internal/assistant/openknowledge/
	// modelswitch) BEFORE it reaches any agent or inference backend. The
	// zero value (empty string) is the baseline — no override — so the
	// no-override path is byte-for-byte the spec-087 behaviour (NFR-4).
	// A typed field (owner directive), NOT a TransportMetadata key.
	ModelOverride string

	// GatherModelOverride is the spec 089 (Fork C) per-request,
	// runtime-switchable GATHER (tool-calling) model selection for the
	// open-knowledge /ask agent — SEPARATE from ModelOverride (which
	// re-points the synthesis turn only). UNTRUSTED: a user-supplied raw
	// model string (e.g. parsed from a Telegram `/ask --gather-model=<id>`
	// flag) that MUST be validated against the tool-capable gather set
	// (modelswitch.ResolveGather) BEFORE any gather turn runs; a
	// non-tool-capable selection is refused fail-loud. The zero value is
	// the baseline (no gather override) so the no-override path is
	// byte-for-byte spec 087/088 (NFR-4). A typed field, NOT a
	// TransportMetadata key.
	GatherModelOverride string
}

// MessageKind discriminates the four supported inbound message shapes.
// The closed-vocabulary check in message_test.go fails if a new value
// is added without an explicit test entry.
type MessageKind string

const (
	// KindText is a free-form text message from the user.
	KindText MessageKind = "text"
	// KindConfirm is a positive/negative answer to a prior
	// ConfirmCard.
	KindConfirm MessageKind = "confirm"
	// KindDisambiguation is a numeric selection from a prior
	// DisambiguationPrompt.
	KindDisambiguation MessageKind = "disambiguation"
	// KindReset is a capability-level reset (drops pending
	// confirm/disambig state for the user/transport).
	KindReset MessageKind = "reset"
)

// AllMessageKinds is the exhaustive closed-vocabulary list. Update
// in lock-step with the constants above. message_test.go enforces
// that AllMessageKinds round-trips every literal value declared in
// this file.
var AllMessageKinds = []MessageKind{
	KindText,
	KindConfirm,
	KindDisambiguation,
	KindReset,
}

// ConfirmChoice is the user's response to a ConfirmCard. The zero
// value ("") is reserved for messages where Kind != KindConfirm.
type ConfirmChoice string

const (
	ConfirmPositive ConfirmChoice = "positive"
	ConfirmNegative ConfirmChoice = "negative"
)

// AllConfirmChoices is the exhaustive closed-vocabulary list.
var AllConfirmChoices = []ConfirmChoice{
	ConfirmPositive,
	ConfirmNegative,
}

// Attachment is reserved for v2. Present on the canonical type so
// future capability extensions can populate it without breaking the
// adapter contract.
type Attachment struct {
	Kind        string
	MimeType    string
	URL         string
	SizeBytes   int64
	Description string
}

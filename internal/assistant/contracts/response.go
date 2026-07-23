package contracts

import (
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// AssistantResponse is a thin facade over agent.InvocationResult +
// agent.RoutingDecision. It carries references (NOT copies) to the
// underlying spec 037 substrate so trace IDs and tool-call details
// are reachable without duplication, plus exactly six net-new fields
// added by spec 061 per spec.md §3.1.3.
//
// Net-new fields (count enforced by response_test.go): Status, Sources,
// ConfirmCard, DisambiguationPrompt, ErrorCause, CaptureRoute.
//
// Convenience derivatives (Body, SourcesOverflowCount, EmittedAt) are
// computed from the net-new fields plus Invocation and are exposed
// here so adapters do not have to re-derive them.
//
// Source of truth: design.md §2.2.
type AssistantResponse struct {
	// --- Substrate references (REUSED, NOT COPIED) ---

	// Invocation may be nil for short-circuit paths that never
	// reached the executor (e.g. low-band capture, borderline
	// disambiguation, confirm-card propose phase shortcut).
	Invocation *agent.InvocationResult

	// Routing is nil iff the capability layer never called
	// agent.Router (e.g. slash-shortcut fast path that bypasses
	// routing entirely).
	Routing *agent.RoutingDecision

	// --- Six net-new fields added by spec 061 ---

	// Status is the closed-vocabulary user-facing status token per
	// UX §14.A.2.
	Status StatusToken

	// Sources is bounded by assistant.sources_max from SCOPE-01 SST.
	// Order is significant: adapters render in slice order.
	Sources []Source

	// ConfirmCard is non-nil iff the response is a propose-phase
	// confirm card. Mutually exclusive with DisambiguationPrompt.
	ConfirmCard *ConfirmCard

	// DisambiguationPrompt is non-nil iff the response is a
	// borderline-band disambiguation prompt. Mutually exclusive with
	// ConfirmCard.
	DisambiguationPrompt *DisambiguationPrompt

	// ErrorCause is the closed-vocabulary error discriminator;
	// populated when Status == StatusUnavailable.
	ErrorCause ErrorCause

	// CaptureRoute is true when the adapter MUST invoke the local
	// capture path instead of (or in addition to) rendering Body
	// — the spec 061 "default to capture" contract.
	CaptureRoute bool

	// LegacyRetirementNotice carries the structured deprecation
	// notice payload the spec 075 facade Policy attaches when an
	// open deprecation window's dedup ledger reports this is the
	// first inbound turn for (user, retired_command, window). nil
	// means "no notice" — the renderer MUST NOT emit any addendum.
	// Spec 075 SCOPE-6.1 (facade Policy dispatch contract).
	LegacyRetirementNotice *NoticePayload

	// ModelAttribution is the spec 088 answer-level attribution of the
	// model that produced this response. nil means baseline (no
	// override) — the Telegram renderer MUST NOT emit the `— model:`
	// footer (byte-for-byte spec-087 baseline, NFR-4 / Principle 6).
	// When non-nil with OverrideApplied=true a runtime model override
	// was in effect; the renderer appends the attribution footer. The
	// HTTP envelope carries the model id unconditionally via a separate
	// path (agenttool.outputEnvelope.Model), so this field is the
	// Telegram-surface attribution carrier only.
	ModelAttribution *ModelAttribution

	// --- Convenience derivatives ---

	// Body is the rendered text body, derived from Invocation.Final
	// (when present) OR a canonical refusal/short-circuit string
	// (otherwise). Bounded by assistant.body_max_chars from SCOPE-01.
	Body string

	// SourcesOverflowCount records how many sources were truncated
	// from Sources due to the sources_max cap.
	SourcesOverflowCount int

	// EmittedAt is the capability-layer emit time. Adapters use it
	// for per-transport telemetry latency calculation.
	EmittedAt time.Time
}

// ModelAttribution is the spec 088 answer-level model attribution
// (Principle 8 / FR-7). ModelID is the model of the turn that actually
// produced the final text (TurnResult.Model, stamped in the agent's
// finalize chokepoint) — honest across success, salvage, refuse, and
// early-StopEndTurn. OverrideApplied distinguishes a runtime override
// from the SST baseline so the Telegram renderer shows the `— model:`
// footer ONLY when an override was in effect (a baseline answer is
// byte-for-byte spec-087, NFR-4).
//
// Spec 089 extends it for the precedence resolver + gather override:
// SynthesisSource classifies how the synthesis selection was made
// (default|sticky|per_request); GatherModel is the gather/tool model
// that ran (TurnResult.GatherModel); GatherSource classifies how the
// gather selection was made (default|per_request); GatherOverridden is
// true when a gather override was in effect (GatherSource != default),
// which drives the renderer's dual `— gather: … · synth: …` footer.
type ModelAttribution struct {
	ModelID          string
	OverrideApplied  bool
	SynthesisSource  string
	GatherModel      string
	GatherSource     string
	GatherOverridden bool
}

// StatusToken is the closed-vocabulary user-facing status token.
// Adapters MUST NOT render any other status string to the user; they
// MUST translate the token to the per-transport surface (Telegram
// reply, web banner, mobile toast).
type StatusToken string

const (
	StatusThinking StatusToken = "thinking"
	// StatusAnswered is the TERMINAL success token for a delivered
	// answer (e.g. the open_knowledge synthesized answer). Unlike the
	// in-flight StatusThinking, adapters render NO status prefix for it
	// (BUG-064-002 DEFECT 3a: a completed answer must not show a
	// "thinking…" header).
	StatusAnswered          StatusToken = "answered"
	StatusCheckingWeather   StatusToken = "checking_weather"
	StatusCheckingEmail     StatusToken = "checking_email" // v2
	StatusReminderProposed  StatusToken = "reminder_proposed"
	StatusReminderConfirmed StatusToken = "reminder_confirmed"
	StatusReminderCancelled StatusToken = "reminder_cancelled"
	StatusSavedAsIdea       StatusToken = "saved_as_idea"
	StatusUnavailable       StatusToken = "unavailable"
)

// AllStatusTokens is the exhaustive closed-vocabulary list. Update in
// lock-step with the constants above. response_test.go enforces that
// every literal value declared in this file appears in this slice
// exactly once.
var AllStatusTokens = []StatusToken{
	StatusThinking,
	StatusAnswered,
	StatusCheckingWeather,
	StatusCheckingEmail,
	StatusReminderProposed,
	StatusReminderConfirmed,
	StatusReminderCancelled,
	StatusSavedAsIdea,
	StatusUnavailable,
}

// ErrorCause is the closed-vocabulary error discriminator populated
// when Status == StatusUnavailable.
type ErrorCause string

const (
	// ErrNone is the zero value for ErrorCause; used when Status is
	// not StatusUnavailable.
	ErrNone ErrorCause = ""
	// ErrProviderUnavailable indicates an external provider (weather,
	// retrieval backend) returned a non-recoverable error.
	ErrProviderUnavailable ErrorCause = "provider_unavailable"
	// ErrMissingScope indicates the active PASETO token is missing
	// the required scope for the requested skill.
	ErrMissingScope ErrorCause = "missing_scope"
	// ErrSlotMissing indicates the user's message did not supply a
	// required slot (e.g. weather query missing location).
	ErrSlotMissing ErrorCause = "slot_missing"
	// ErrInternalError indicates an unexpected capability-layer
	// error not better described by another cause.
	ErrInternalError ErrorCause = "internal_error"
	// ErrNoMatch indicates a successful skill call found zero
	// matches in the user's owned knowledge graph. Used by skills
	// like recipe_search (BUG-061-003) to distinguish "the owned
	// graph is empty for this query" from provider/auth/slot errors.
	ErrNoMatch ErrorCause = "no_match"
	// ErrModelNotSwitchable is the spec 088 fail-loud discriminator for
	// a rejected per-request model override (off-allowlist / un-profiled
	// / over-envelope). It pairs with StatusUnavailable and a Body that
	// is the verbatim modelswitch.Rejection.Message; the Telegram
	// renderer surfaces that Body as-is (NOT a canonical refusal) so the
	// operator sees exactly which model was refused and why. The
	// specific reason-code lives in the message + the HTTP error_code.
	ErrModelNotSwitchable ErrorCause = "model_not_switchable"
	// ErrNoGroundedAnswer is the honest discriminator for a
	// requires_provenance scenario that produced a body with no valid
	// sources (BUG-061-009). The provenance gate refuses into this cause
	// with StatusUnavailable and the canonical refusal body
	// ("I don't have a sourced answer for that."), NEVER the band-low
	// capture acknowledgement. It is the structural signal that this is a
	// high-band refusal (a matched, executed request the system could not
	// ground), distinct from ErrProviderUnavailable (upstream failed) and
	// ErrNoMatch (the owned graph was empty for the query).
	ErrNoGroundedAnswer ErrorCause = "no_grounded_answer"
)

// AllErrorCauses is the exhaustive non-zero closed-vocabulary list
// (ErrNone is excluded — it is the zero value).
var AllErrorCauses = []ErrorCause{
	ErrProviderUnavailable,
	ErrMissingScope,
	ErrSlotMissing,
	ErrInternalError,
	ErrNoMatch,
	ErrModelNotSwitchable,
	ErrNoGroundedAnswer,
}

// ConfirmCard is the propose-phase response that requires user
// confirmation before any side effect (notification schedule, list
// mutation, etc.) is executed. Spec 061 design §6.3 audit boundary:
// every ConfirmCard emission writes an assistant_proposal row; the
// follow-up confirm/cancel/timeout writes a second row.
type ConfirmCard struct {
	// ProposedAction is a human-readable description of the action
	// the user is being asked to confirm.
	ProposedAction string

	// Payload is opaque to the adapter. The capability layer
	// persists it server-side and the adapter echoes it through the
	// confirm callback via ConfirmRef.
	Payload []byte

	// Timeout is the per-card TTL after which the capability-layer
	// idle sweep deletes the pending row and writes a
	// "discarded_timeout" audit row.
	Timeout time.Duration

	// ConfirmRef is a ULID that uniquely identifies this pending
	// confirm. The follow-up AssistantMessage with
	// Kind == KindConfirm carries the same ConfirmRef.
	ConfirmRef string

	// PositiveLabel / NegativeLabel are the per-transport button
	// labels (Telegram inline keyboard, web button, etc.).
	PositiveLabel string
	NegativeLabel string
}

// DisambiguationPrompt is the borderline-band response that asks the
// user to choose between up to three candidate scenarios. By
// convention the "save_as_note" choice is always last
// (design.md §3.2). Mutually exclusive with ConfirmCard.
type DisambiguationPrompt struct {
	// Choices length is 1..3; "save_as_note" is always last.
	Choices []DisambiguationChoice
	// Timeout is the per-prompt TTL after which the capability-layer
	// idle sweep deletes the pending row.
	Timeout time.Duration
	// DisambiguationRef is a ULID that uniquely identifies this
	// pending disambig.
	DisambiguationRef string
}

// DisambiguationChoice is one selectable option in a
// DisambiguationPrompt.
type DisambiguationChoice struct {
	// Number is 1-indexed.
	Number int
	// ID matches a Spec 037 scenario id, or the sentinel
	// "save_as_note".
	ID string
	// Label is the human-readable rendering.
	Label string
	// Shortcut is the optional slash-shortcut equivalent (e.g.
	// "/weather"); per-transport renderers may show it next to the
	// label.
	Shortcut string
}

// SaveAsNoteChoiceID is the sentinel ID used for the always-last
// "save as a note" choice in a DisambiguationPrompt.
const SaveAsNoteChoiceID = "save_as_note"

// Spec 068 SCOPE-1 — Structured Intent Compiler foundation types.
//
// CompiledIntent is the schema-bound contract the LLM produces for
// every user-facing natural-language turn before routing, tool
// selection, or response synthesis happens. RawTurn is the
// transport-neutral inbound carrier. CompilerTrace captures the
// observability shape consumed by the assistant-turn audit and the
// (later) trace inspector (full observability is owned by spec 071).
//
// This package is the transport-neutral foundation: it MUST NOT call
// tools, mutate state, or know about Telegram/HTTP/etc.

package intent

import (
	"context"
	"time"
)

// ActionClass is the closed-vocabulary discriminator for the next
// system step requested by the user.
type ActionClass string

const (
	ActionAnswer         ActionClass = "answer"
	ActionRetrieve       ActionClass = "retrieve"
	ActionExternalLookup ActionClass = "external_lookup"
	ActionInternalAction ActionClass = "internal_action"
	ActionStateMutation  ActionClass = "state_mutation"
	ActionClarify        ActionClass = "clarify"
	ActionCaptureOnly    ActionClass = "capture_only"
	ActionRefuse         ActionClass = "refuse"
)

// AllActionClasses is the closed vocabulary used by schema validation.
var AllActionClasses = []ActionClass{
	ActionAnswer, ActionRetrieve, ActionExternalLookup, ActionInternalAction,
	ActionStateMutation, ActionClarify, ActionCaptureOnly, ActionRefuse,
}

// SideEffectClass is the closed-vocabulary discriminator that gates
// confirmation / capability checks before execution.
type SideEffectClass string

const (
	SideEffectNone          SideEffectClass = "none"
	SideEffectRead          SideEffectClass = "read"
	SideEffectWrite         SideEffectClass = "write"
	SideEffectExternalRead  SideEffectClass = "external_read"
	SideEffectExternalWrite SideEffectClass = "external_write"
)

// AllSideEffectClasses is the closed vocabulary used by schema
// validation.
var AllSideEffectClasses = []SideEffectClass{
	SideEffectNone, SideEffectRead, SideEffectWrite,
	SideEffectExternalRead, SideEffectExternalWrite,
}

// SourcePolicy carries the answer-substantiation policy attached to a
// compiled intent. spec.md §3 + design §"Data Model".
type SourcePolicy struct {
	RequiresCitations  bool     `json:"requires_citations"`
	AllowedSourceKinds []string `json:"allowed_source_kinds"`
}

// CompiledIntent is the schema-bound interpretation of one inbound
// user turn. spec.md §3 ("CompiledIntent minimum schema") and
// design.md §"Data Model" define the field set; the JSON tags are the
// wire form returned by the ML sidecar.
type CompiledIntent struct {
	Version             string          `json:"version"`
	Language            string          `json:"language"`
	UserGoal            string          `json:"user_goal"`
	ActionClass         ActionClass     `json:"action_class"`
	SideEffectClass     SideEffectClass `json:"side_effect_class"`
	ScenarioHint        *string         `json:"scenario_hint"`
	ToolHints           []string        `json:"tool_hints"`
	NormalizedRequest   map[string]any  `json:"normalized_request"`
	Slots               map[string]any  `json:"slots"`
	MissingSlots        []string        `json:"missing_slots"`
	Confidence          float64         `json:"confidence"`
	ClarificationPrompt *string         `json:"clarification_prompt"`
	SafetyFlags         []string        `json:"safety_flags"`
	SourcePolicy        SourcePolicy    `json:"source_policy"`
}

// ContextTurn is one prior turn in the bounded conversation window
// passed to the compiler. Transport-neutral.
type ContextTurn struct {
	Role string // "user" | "assistant"
	Text string
}

// RawTurn is the inbound carrier the compiler consumes. design.md
// §"Capability Foundation".
type RawTurn struct {
	UserID             string
	Transport          string
	TransportMessageID string
	Text               string
	ConversationWindow []ContextTurn
	ReceivedAt         time.Time
}

// CompilerOutcome is the closed vocabulary of compiler trace outcomes.
type CompilerOutcome string

const (
	OutcomeCompiled      CompilerOutcome = "compiled"
	OutcomeSchemaInvalid CompilerOutcome = "schema_invalid"
	OutcomeProviderError CompilerOutcome = "provider_error"
	OutcomeConfigError   CompilerOutcome = "config_error"
	OutcomeBypass        CompilerOutcome = "operational_command_bypass"
)

// BypassRecord identifies an operational-command bypass turn. spec.md
// §"Hard Constraint 1" — only the carve-out set in OperationalCommands
// produces this record.
type BypassRecord struct {
	Command string // e.g. "/status"
	Label   string // always "operational_command_bypass"
}

// CompilerTrace is the observability shape recorded for every turn the
// compiler observes, including operational bypasses. Persistence into
// audit/agent_traces is the caller's responsibility; this package
// returns the value.
type CompilerTrace struct {
	RawText    string
	Outcome    CompilerOutcome
	Bypass     *BypassRecord
	Compiled   *CompiledIntent
	LatencyMS  int64
	ErrorCause string // populated when Outcome != "compiled" and != "operational_command_bypass"
}

// Compiler is the transport-neutral compiler capability. design.md
// §"Capability Foundation".
type Compiler interface {
	Compile(ctx context.Context, turn RawTurn) (CompiledIntent, CompilerTrace, error)
}

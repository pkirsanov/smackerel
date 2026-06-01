// Package legacyretirement implements the spec 075 SCOPE-1 capability
// foundation: the runtime contracts (catalog, ledger, window-state
// resolver, residual telemetry, observation report) that later scopes
// compose into the assistant facade. This file declares the core
// types and the RetirementDecision returned by Policy.Handle().
//
// Scope boundary: this package owns the policy seam and the
// privacy-preserving primitives (HMAC user bucket, metric definition).
// It does NOT own the spec 066 retired-command list (catalog
// implementation is provided by spec 066), the notice rendering
// (Scope 2), the residual usage emission path (Scope 3), the
// threshold evaluator (Scope 4), or the observation-report producer
// (Scope 5). Those scopes consume the interfaces declared here.
package legacyretirement

import (
	"context"
	"time"
)

// WindowState is the effective deprecation-window state observed by
// the policy at decision time. "open" allows notices and the legacy
// path; "paused" suppresses new notices while the legacy path keeps
// serving; "closed" rejects retired commands with the canonical
// unknown-command response and never invokes a legacy handler.
type WindowState string

const (
	WindowOpen   WindowState = "open"
	WindowPaused WindowState = "paused"
	WindowClosed WindowState = "closed"
)

// String makes WindowState satisfy fmt.Stringer.
func (s WindowState) String() string { return string(s) }

// StateReason is the human-readable explanation a WindowStateResolver
// attaches to a Resolve() result. Operators read it on the dashboard
// (Scope 4) to understand why the effective state differs from the
// SST state. Examples: "sst_open_no_runtime_pause",
// "runtime_pause_threshold_exceeded", "sst_closed".
type StateReason string

// RetiredCommand is the finite metadata for a single retired command.
// The catalog is owned by spec 066 (Scope 1 only defines the shape);
// the policy never invents commands and never operates on a token the
// catalog does not recognise.
type RetiredCommand struct {
	// Command is the literal token a user types (e.g. "/weather").
	Command string
	// ReplacementExample is the plain-English alternative shown in
	// the notice and the closed-window response (e.g.
	// "weather in Barcelona tomorrow").
	ReplacementExample string
	// NoticeCopy is the short addendum rendered alongside the
	// primary NL response during an open window.
	NoticeCopy string
	// Spec066ID is the cross-reference into the spec 066 catalog
	// entry that owns this token. Used for traceability and audit.
	Spec066ID string
}

// RetirementOutcome enumerates the closed-set telemetry label values
// emitted by the policy. Cardinality is bounded so dashboards can
// rely on stable label sets.
type RetirementOutcome string

const (
	OutcomeNoticeAndServed       RetirementOutcome = "notice_and_served"
	OutcomeServedNoNotice        RetirementOutcome = "served_no_notice"
	OutcomePausedSuppressed      RetirementOutcome = "paused_suppressed"
	OutcomeClosedUnknown         RetirementOutcome = "closed_unknown"
	OutcomeMappingNotConfident   RetirementOutcome = "mapping_not_confident"
	OutcomeNotRetiredPassthrough RetirementOutcome = "not_retired_passthrough"
)

// RetirementDecision is what Policy.Handle returns to the assistant
// facade. The facade renders the deprecation notice (if any) as
// structured response metadata and continues into normal routing
// when ServeNL is true.
type RetirementDecision struct {
	// Matched is true when the inbound token was classified as a
	// retired command by the catalog. False means "not our problem,
	// facade should proceed as normal".
	Matched bool
	// Command is the catalog entry for Matched==true.
	Command RetiredCommand
	// EffectiveState is the resolved window state at decision time.
	EffectiveState WindowState
	// StateReason mirrors what the resolver returned.
	StateReason StateReason
	// ShowNotice is true only when the open-window dedup ledger had
	// no prior entry for (user, command, window). The facade must
	// emit the structured deprecation_notice payload when set.
	ShowNotice bool
	// ServeNL is true when the assistant should proceed through the
	// normal NL routing path. False means the closed-window
	// canonical unknown-command response must be returned and no
	// legacy handler may be invoked.
	ServeNL bool
	// Outcome is the closed-set telemetry label the residual
	// telemetry layer will tag this turn with.
	Outcome RetirementOutcome
	// DecidedAt is the policy decision timestamp (server clock).
	DecidedAt time.Time
	// WindowID is the deprecation window identifier the policy
	// resolved this turn against. Empty when Matched==false (no
	// retired-command classification implies no window key).
	// Carried on the decision so the assistant facade can attach
	// it to AssistantResponse.LegacyRetirementNotice without taking
	// a runtime dependency on the policy implementation.
	WindowID string
}

// Policy is the top-level seam invoked by the assistant facade. Scope
// 2 wires the Telegram + web transports through this entrypoint. The
// interface lives in Scope 1 so foundation tests can exercise the
// contract without pulling in the renderer or the facade.
type Policy interface {
	Handle(ctx context.Context, turn AssistantTurn) (RetirementDecision, error)
}

// AssistantTurn is the minimal transport-neutral shape the policy
// needs to make a decision. The full assistant turn type lives in
// internal/assistant; this is a deliberately thin projection so the
// foundation package does not depend on the facade.
type AssistantTurn struct {
	// UserID is the stable user identifier. It is NEVER emitted as
	// a metric label or persisted into telemetry — the HMAC bucket
	// helper (telemetry.go) is the only path through which user
	// identity reaches observable surfaces.
	UserID string
	// Transport is the inbound transport name (e.g. "telegram",
	// "web"). Ledger dedup is keyed on (user, command, window) and
	// is therefore transport-independent by construction.
	Transport string
	// RawText is the inbound user text. The classifier inspects only
	// the leading command token; raw text is NEVER emitted as a
	// metric label or persisted into telemetry.
	RawText string
	// ReceivedAt is the inbound timestamp.
	ReceivedAt time.Time
}

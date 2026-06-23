package surfacing

// Package surfacing implements the unified surfacing controller — the
// single decision point between intelligence producers (alert delivery
// sweep, digest, resurfacing, weekly synthesis, monthly report, pre-
// meeting briefs) and user-visible dispatch channels (Telegram, web
// push, ntfy, email-out).
//
// Pipeline order: normalize → dedupe → suppress → budget → escalate.
// See specs/021-intelligence-delivery/scopes.md Scope 4 design notes
// and SCN-021-016 through SCN-021-019 for the contract.

import "time"

// Channel identifies a user-visible dispatch channel. Bounded label set —
// new channels MUST extend this enum so Prometheus cardinality stays
// finite.
type Channel string

const (
	ChannelTelegram Channel = "telegram"
	ChannelWebPush  Channel = "web_push"
	ChannelNtfy     Channel = "ntfy"
	ChannelEmailOut Channel = "email_out"
	ChannelDigest   Channel = "digest"
)

// Producer identifies the intelligence producer that proposed the
// candidate. Bounded set — adding a new producer MUST extend this enum.
type Producer string

const (
	ProducerAlerts           Producer = "alerts"
	ProducerDigest           Producer = "digest"
	ProducerResurfacing      Producer = "resurfacing"
	ProducerWeeklySynthesis  Producer = "weekly_synthesis"
	ProducerMonthlyReport    Producer = "monthly_report"
	ProducerPreMeetingBriefs Producer = "pre_meeting_briefs"
	ProducerFrequentLookups  Producer = "frequent_lookups"
	// ProducerNotification identifies the spec 054 notification intelligence
	// handler as the origin of a surfacing candidate. Additive enum extension
	// (the enum doc above invites this) — spec 054 Scope 9 routes user-facing
	// notification decisions through the shared controller as a subordinate
	// producer; it does NOT fork the controller contract.
	ProducerNotification Producer = "notification"
)

// DecisionKind enumerates the controller's terminal verdicts. The five
// values match the Scope 4 contract one-for-one.
type DecisionKind string

const (
	DecisionPermit                  DecisionKind = "permit"
	DecisionDeduped                 DecisionKind = "deduped"
	DecisionSuppressed              DecisionKind = "suppressed"
	DecisionDeferredBudgetExhausted DecisionKind = "deferred-budget-exhausted"
	DecisionEscalated               DecisionKind = "escalated"
)

// SurfacingCandidate is what producers submit to the controller. ContentKey
// is the cross-channel dedupe identity — typically an artifact_id or
// insight_id; producers MUST set it for any item that could surface twice.
type SurfacingCandidate struct {
	Producer     Producer
	Channel      Channel
	ContentKey   string
	Priority     int  // 1=high, 2=medium, 3=low (mirrors intelligence.Alert)
	TimeCritical bool // p1+timeCritical may escalate past exhausted budget
	ProposedAt   time.Time
}

// SurfacingDecision is the controller's verdict for a single candidate.
// Reason carries the bounded reason vocabulary used by the
// budget_overrides_total / suppression_total / dedupe_total counters.
type SurfacingDecision struct {
	Kind   DecisionKind
	Reason string
}

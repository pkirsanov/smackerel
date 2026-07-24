// Package proactive is the single-controller card-projection foundation for
// spec 107 (Proactive & Correlated Experience), SCOPE-01.
//
// It is a reusable composition capability consumed by every proactive surface
// (web card, Telegram inline, WhatsApp interactive, Today cockpit, feed) and by
// every channel renderer. It owns exactly one set of contracts:
//
//   - ProactiveCardModel — the immutable projection of ONE permit/escalated
//     verdict from the single spec-078 surfacing controller. A card exists for
//     no other verdict; deduped/suppressed/deferred-budget-exhausted map to an
//     honest state, never a card.
//   - NudgeRegistry (NudgeRef) — the ephemeral, process-local, expiring map
//     ref -> {content_key, producer, channel, principal, issued_at}. It is the
//     SOLE anti-leak boundary: the content_key never reaches any transport wire,
//     data-* hook, or telemetry — only the opaque, ULID-shaped ref does.
//   - NudgeAck — the single acknowledge path. act/snooze/dismiss from ANY
//     channel resolve their ref and call one Acknowledge(content_key) on the
//     process-wide surfacing AckLookup, so acting once suppresses everywhere
//     within suppression_window_hours.
//   - HonestState / HonestStateForVerdict — the closed honest-state vocabulary,
//     failing closed to StateError for any unknown condition (never a card).
//   - BudgetMeter / ReadBudgetMeter — the honest "N of M used today" render;
//     exhaustion is an explicit content state, not a hidden default.
//   - EncodeNudgeCallback / DecodeNudgeCallback — the additive a:n:<ref>:<a|s|d>
//     wire form shared by the Telegram callback_data and the WhatsApp reply-id.
//     It carries only the opaque ref, never the content_key, and never collides
//     with the existing a:c:/a:d: assistant families.
//
// This package CONSUMES the spec-078 controller contract
// (internal/intelligence/surfacing) and never re-runs the dedupe -> suppress ->
// budget -> escalate pipeline, adds no second budget, and edits no owner type.
// It introduces no new business store: the NudgeRegistry is process-local
// routing state that mirrors the existing DedupeIndex / InMemoryAck pattern and
// is dropped on restart (stale refs then resolve to an honest expired render).
package proactive

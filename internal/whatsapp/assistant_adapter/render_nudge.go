package assistant_adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

// render_nudge.go — spec 107 SCOPE-03B1. Renders exactly ONE permit/escalated
// proactive.ProactiveCardModel (the projection of a single spec-078 controller
// verdict) as a WhatsApp interactive message over the shipped spec-072 transport,
// and routes a chosen interactive/list reply through the shared SCOPE-01
// NudgeAck path to the one Acknowledge(content_key). It is the WhatsApp sibling
// of the Telegram internal/telegram/assistant_adapter/render_nudge.go and reuses
// the exact same opaque a:n:<ref>:<a|s|d> wire shape, so one logical reply form
// routes every channel to the one NudgeRegistry + ack path.
//
// Additive boundary (spec-072 is done/certified — do NOT break it):
//   - This file only ADDS the nudge render + inbound a:n: seam. It does NOT
//     modify the spec-072 Render mapping (render.go), the webhook handler, the
//     idempotency/rate-limit paths, or the existing d:/c:/r: interactive
//     reply-id families (decodeInteractivePayload). The a:n: reply-id is
//     structurally distinct from d:/c:/r:, so it never collides with — and is
//     never swallowed by — spec-072's existing inbound dispatch (which cleanly
//     refuses an a:n: id with ErrUnsupportedMessageType). HandleNudgeReply is a
//     PARALLEL inbound seam, exactly like the Telegram HandleNudgeCallback sits
//     parallel to the confirm/disambig dispatch.
//
// Anti-fabrication contract (Product Principle: honest states):
//   - Only a card-bearing state (permitted/escalated) draws an interactive
//     message; every non-card / terminal state renders as an honest TEXT line
//     with NO buttons, never a fabricated card (BuildNudgeStateText).
//   - The wire never carries a content_key — only the opaque a:n:<ref>:<a|s|d>
//     produced by the foundation (card.WireCallback) reaches a reply.id
//     (FR-107-028). No telemetry label carries a content_key, node label, or
//     query text either.

// ReservedWhatsAppChannel is the bounded `whatsapp` Channel value spec-107
// SCOPE-03B1 reserves for the WhatsApp proactive nudge surface. It is a
// coordination note to the spec-078 Channel enum owner (FR-107-012):
// internal/intelligence/surfacing/types.go is NOT edited here — this call-site
// conversion of the documented `type Channel string` bounded vocabulary keeps
// the reservation additive until the owner promotes it to a named const. It is
// opaque routing/provenance metadata on the NudgeRegistry entry; suppression is
// content_key-keyed on the one controller and never consults the channel.
const ReservedWhatsAppChannel surfacing.Channel = "whatsapp"

// nudgeReplyListSectionTitle is the single list-section header the list-message
// fallback renders the three actions under. Bounded, non-sensitive.
const nudgeReplyListSectionTitle = "Actions"

// nudgeReplyListButtonCTA is the list-message CTA label. Mirrors the spec-072
// disambiguation list CTA convention (listButtonCTA) without depending on it.
const nudgeReplyListButtonCTA = "Choose"

// nudgeActionLabel maps a bounded NudgeAction to its interactive-button title.
// The three titles are exactly Act/Snooze/Dismiss (design.md OQ2). An unknown
// action yields "" so a malformed action is omitted rather than shipping a blank
// reply button.
func nudgeActionLabel(a proactive.NudgeAction) string {
	switch a {
	case proactive.ActionAct:
		return "Act"
	case proactive.ActionSnooze:
		return "Snooze"
	case proactive.ActionDismiss:
		return "Dismiss"
	default:
		return ""
	}
}

// nudgeMessageBody composes the card body: the title, then a blank line, then a
// "Why:" provenance line derived from the card's real Producer-derived
// Provenance (an escalated card's Provenance already carries the URGENT
// ESCALATION marker). WhatsApp bodies are plain text (unlike the Telegram
// markdown modes), so no escaping is applied. A card with an empty
// title/provenance still renders honestly (never a blank message) because the
// foundation always supplies a Provenance.
func nudgeMessageBody(card proactive.ProactiveCardModel) string {
	var b strings.Builder
	if title := strings.TrimSpace(card.Title); title != "" {
		b.WriteString(title)
	}
	if why := strings.TrimSpace(card.Provenance); why != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Why: " + why)
	}
	return b.String()
}

// nudgeReplyButtons builds the three interactive reply choices Act/Snooze/Dismiss
// with reply.id = a:n:<ref>:<a|s|d>, single-sourced from the foundation via
// card.WireCallback (which encodes ONLY the opaque ref, never the content_key).
// ok is false when no encodable action remains (defensive; a card always carries
// the fixed three actions). titleCap bounds the visible label to WhatsApp's
// per-element limit (20 for buttons, 24 for list rows) via the shared
// truncateLabel helper.
func nudgeReplyButtons(card proactive.ProactiveCardModel, titleCap int) ([]Button, bool) {
	buttons := make([]Button, 0, len(card.Actions))
	for _, action := range card.Actions {
		label := nudgeActionLabel(action)
		if label == "" {
			continue
		}
		id, encOK := card.WireCallback(action)
		if !encOK {
			continue
		}
		buttons = append(buttons, Button{ID: id, Title: truncateLabel(label, titleCap)})
	}
	if len(buttons) == 0 {
		return nil, false
	}
	return buttons, true
}

// BuildNudgeInteractive is the pure (no-I/O) primary builder for a card-bearing
// nudge: body + "Why:" provenance line + the three interactive Act/Snooze/Dismiss
// reply buttons carrying a:n:<ref>:<a|s|d>. It returns ok=false for ANY non-card
// state — an honest non-card render is BuildNudgeStateText, so this function can
// never emit buttons for a non-card verdict. Golden tests compare its output
// verbatim. maxTextChars bounds the body (SST per-message cap); a non-positive
// cap yields ok=false so a caller never emits an unbounded body.
func BuildNudgeInteractive(card proactive.ProactiveCardModel, maxTextChars int) (InteractiveMessage, bool) {
	if maxTextChars <= 0 {
		return InteractiveMessage{}, false
	}
	if !card.State.IsCard() {
		return InteractiveMessage{}, false
	}
	buttons, ok := nudgeReplyButtons(card, 20)
	if !ok {
		return InteractiveMessage{}, false
	}
	return InteractiveMessage{
		Kind:    OutboundInteractiveButtons,
		Body:    truncateBody(nudgeMessageBody(card), maxTextChars),
		Buttons: buttons,
	}, true
}

// BuildNudgeListFallback is the interactive-list fallback: the same three actions
// as one list section, each row carrying the SAME a:n:<ref>:<a|s|d> reply.id, plus
// the provenance line. It is used when a buttons render is unavailable but the
// interactive-list surface is (mirroring the spec-072 4..10-choice list mapping).
// ok=false for any non-card state.
func BuildNudgeListFallback(card proactive.ProactiveCardModel, maxTextChars int) (InteractiveMessage, bool) {
	if maxTextChars <= 0 {
		return InteractiveMessage{}, false
	}
	if !card.State.IsCard() {
		return InteractiveMessage{}, false
	}
	buttons, ok := nudgeReplyButtons(card, 24)
	if !ok {
		return InteractiveMessage{}, false
	}
	rows := make([]ListRow, 0, len(buttons))
	for _, b := range buttons {
		rows = append(rows, ListRow{ID: b.ID, Title: b.Title})
	}
	return InteractiveMessage{
		Kind:         OutboundInteractiveList,
		Body:         truncateBody(nudgeMessageBody(card), maxTextChars),
		ListButton:   nudgeReplyListButtonCTA,
		ListSections: []ListSection{{Title: nudgeReplyListSectionTitle, Rows: rows}},
	}, true
}

// BuildNudgeTextFallback is the numbered plain-text fallback for a client that
// cannot render an interactive message: the body + "Why:" line + a numbered list
// (1|2|3 -> Act|Snooze|Dismiss) preserving the same three actions and the
// provenance line. The digits map to actions via NumberedFallbackReply on the
// inbound side. ok=false for any non-card state.
func BuildNudgeTextFallback(card proactive.ProactiveCardModel, maxTextChars int) (TextMessage, bool) {
	if maxTextChars <= 0 {
		return TextMessage{}, false
	}
	if !card.State.IsCard() {
		return TextMessage{}, false
	}
	var b strings.Builder
	b.WriteString(nudgeMessageBody(card))
	// The three actions in fixed order, numbered 1|2|3 -> a|s|d, as a single
	// block below the body/provenance.
	b.WriteString("\n")
	for i, action := range card.Actions {
		label := nudgeActionLabel(action)
		if label == "" {
			continue
		}
		fmt.Fprintf(&b, "\n%d. %s", i+1, label)
	}
	b.WriteString("\n\nReply 1, 2, or 3.")
	return TextMessage{Body: truncateBody(b.String(), maxTextChars)}, true
}

// NumberedFallbackReply maps a numbered plain-text reply digit ("1"/"2"/"3") back
// to its NudgeAction (a|s|d), the inbound half of BuildNudgeTextFallback. ok is
// false for any other token. The ref for a numbered reply is resolved from the
// caller's most-recent-nudge context (not carried in the digit); this helper
// only decodes the action, exactly as the interactive reply.id decodes to
// (ref, action) via proactive.DecodeNudgeCallback.
func NumberedFallbackReply(digit string) (proactive.NudgeAction, bool) {
	switch strings.TrimSpace(digit) {
	case "1":
		return proactive.ActionAct, true
	case "2":
		return proactive.ActionSnooze, true
	case "3":
		return proactive.ActionDismiss, true
	default:
		return proactive.ActionUnknown, false
	}
}

// nudgeStateText maps one honest HonestState onto a distinct, user-legible
// WhatsApp line. Card-bearing states never route here (BuildNudgeInteractive
// owns them); every other verdict/terminal condition gets its own honest line,
// and the fail-closed default (StateError / any unknown) is a neutral "no longer
// available" line — NEVER a fabricated card. The lines mirror the Telegram
// sibling so both channels render the same honest vocabulary.
func nudgeStateText(state proactive.HonestState) string {
	switch state {
	case proactive.StateActed:
		return "Done — you acted on this."
	case proactive.StateSnoozed:
		return "Snoozed — held for now."
	case proactive.StateSuppressed:
		return "Dismissed."
	case proactive.StateAlreadyHandled:
		return "You already handled this one."
	case proactive.StateExpired:
		return "This nudge has expired."
	case proactive.StateBudgetExhausted:
		return "Held — you've reached today's nudge budget."
	case proactive.StateDeduped:
		return "You've already seen this recently."
	case proactive.StateQuiet:
		return "Nothing to surface right now."
	case proactive.StateDegraded:
		return "Surfacing is degraded right now — try again shortly."
	case proactive.StateUnauthorized:
		return "You can't act on this nudge."
	default:
		return "That nudge is no longer available."
	}
}

// BuildNudgeStateText renders one honest non-card / terminal WhatsApp state as a
// plain TextMessage — no interactive buttons, never a fabricated card. It is the
// honest render for budget-exhausted, deduped, already-handled, expired, acted,
// snoozed, suppressed, and the fail-closed error sink.
func BuildNudgeStateText(state proactive.HonestState, maxTextChars int) TextMessage {
	body := nudgeStateText(state)
	if maxTextChars > 0 {
		body = truncateBody(body, maxTextChars)
	}
	return TextMessage{Body: body}
}

// RenderNudge sends a permit/escalated card as a WhatsApp interactive-buttons
// message through the injected CloudClient seam. It REFUSES a non-card state
// (returns an error) rather than sending a fabricated card: a caller only
// reaches RenderNudge after a card-bearing controller verdict (ProjectCard gates
// that upstream), so a non-card state here is a wiring regression that must fail
// loudly. On a non-card verdict the caller renders BuildNudgeStateText instead.
// nil cloud / empty destination fail loud, mirroring the spec-072 RenderToPhone
// guards.
func RenderNudge(ctx context.Context, cloud CloudClient, toPhone string, card proactive.ProactiveCardModel, maxTextChars int) error {
	if cloud == nil {
		return errors.New("whatsapp_adapter: RenderNudge called without configured CloudClient")
	}
	if strings.TrimSpace(toPhone) == "" {
		return errors.New("whatsapp_adapter: RenderNudge called with empty destination phone")
	}
	msg, ok := BuildNudgeInteractive(card, maxTextChars)
	if !ok {
		return fmt.Errorf("whatsapp_adapter: RenderNudge refused non-card state %q (never draw a fabricated card)", card.State)
	}
	if err := cloud.SendInteractive(ctx, toPhone, msg); err != nil {
		return fmt.Errorf("whatsapp send nudge: %w", err)
	}
	return nil
}

// IsNudgeReplyID reports whether a WhatsApp interactive reply.id is a nudge
// (a:n:) reply. It is the structural discriminator a future inbound dispatch
// uses to route a nudge reply to HandleNudgeReply BEFORE the spec-072
// decodeInteractivePayload path (which owns d:/c:/r:). A false result means the
// id is not this family — never a mis-route.
func IsNudgeReplyID(replyID string) bool {
	_, _, ok := proactive.DecodeNudgeCallback(replyID)
	return ok
}

// HandleNudgeReply routes a chosen WhatsApp interactive/list reply.id through the
// shared SCOPE-01 registry to the ONE ack path:
//
//	DecodeNudgeCallback(reply.id) -> NudgeAck.Handle(ref, action)
//	  -> Acknowledge(content_key) (single spec-078 suppression; no second budget/
//	     store) -> honest terminal AckOutcome
//
// It is the single inbound seam for a WhatsApp nudge reply and is PARALLEL to
// spec-072's decodeInteractivePayload: a non-nudge reply.id (a d:/c:/r: payload,
// or anything malformed) returns an error rather than acking, so it never
// mis-routes a spec-072 confirm/disambig/reset reply. ack MUST be non-nil — a
// nil ack is a wiring error surfaced honestly rather than a silent success.
func HandleNudgeReply(replyID string, ack *proactive.NudgeAck) (proactive.AckOutcome, error) {
	ref, action, ok := proactive.DecodeNudgeCallback(replyID)
	if !ok {
		return proactive.AckOutcome{}, fmt.Errorf("whatsapp_adapter: reply.id %q is not a nudge (a:n:) reply", replyID)
	}
	if ack == nil {
		return proactive.AckOutcome{}, errors.New("whatsapp_adapter: HandleNudgeReply requires a NudgeAck")
	}
	return ack.Handle(ref, action), nil
}

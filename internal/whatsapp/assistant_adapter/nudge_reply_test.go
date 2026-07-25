package assistant_adapter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

// nudge_reply_test.go — spec 107 SCOPE-03B1 adapter-level unit tests
// (SCN-107-006). Rows: T107-006-U (a:n: reply.id opaque encode/decode +
// non-collision with the spec-072 d:/c:/r: families), the tap->one-ack seam, and
// T107-03B1-FALLBACK (interactive-list + numbered-text fallback preserve the
// three actions + provenance). White-box (package assistant_adapter) so it can
// reach the unexported spec-072 decodeInteractivePayload seam and prove
// additive non-interference.

// waNudgeRef is a valid opaque 26-char ULID-shaped ref (distinct from the golden
// file's nudgeGoldenRef to avoid coupling).
const waNudgeRef proactive.NudgeRef = "01HZZ0ABCDEFGHJKMNPQRSTVWX"

// TestSCN107006_NudgeReplyIDCarriesOpaqueWireForm (T107-006-U) proves every
// rendered reply.id carries ONLY the opaque a:n:<ref>:<a|s|d> shape — never the
// content_key — round-trips through the shared foundation decoder, and stays
// within WhatsApp's interactive reply-id bound (FR-107-028).
func TestSCN107006_NudgeReplyIDCarriesOpaqueWireForm(t *testing.T) {
	card := permitCard(t, waNudgeRef, "Your weekly review is ready", false)
	msg, ok := BuildNudgeInteractive(card, nudgeMaxChars)
	if !ok {
		t.Fatalf("BuildNudgeInteractive ok=false")
	}
	if len(msg.Buttons) != 3 {
		t.Fatalf("buttons = %d, want 3", len(msg.Buttons))
	}

	wantSuffix := map[int]struct {
		suffix string
		action proactive.NudgeAction
	}{
		0: {"a", proactive.ActionAct},
		1: {"s", proactive.ActionSnooze},
		2: {"d", proactive.ActionDismiss},
	}
	for i, b := range msg.Buttons {
		want := "a:n:" + string(waNudgeRef) + ":" + wantSuffix[i].suffix
		if b.ID != want {
			t.Errorf("button[%d].ID = %q, want %q", i, b.ID, want)
		}
		// The reply.id NEVER carries the content_key (anti-leak boundary).
		if strings.Contains(b.ID, card.ContentKey()) {
			t.Errorf("button[%d].ID = %q leaks content_key %q", i, b.ID, card.ContentKey())
		}
		// WhatsApp caps an interactive reply id at 256 bytes; a:n: is ~32.
		if len(b.ID) > 256 {
			t.Errorf("button[%d].ID is %d bytes, want <= 256", i, len(b.ID))
		}
		// Round-trips through the shared foundation decoder to (ref, action).
		ref, action, decOK := proactive.DecodeNudgeCallback(b.ID)
		if !decOK || ref != waNudgeRef || action != wantSuffix[i].action {
			t.Errorf("DecodeNudgeCallback(%q) = (%q,%s,%v), want (%q,%s,true)",
				b.ID, ref, action, decOK, waNudgeRef, wantSuffix[i].action)
		}
	}
}

// TestSCN107006_NudgeReplyIDDoesNotCollideWithSpec072Families proves the additive
// a:n: reply-id family never collides with the shipped spec-072 d:/c:/r:
// (disambig/confirm/reset) reply-id families, and that spec-072's own inbound
// dispatch (decodeInteractivePayload) cleanly REFUSES an a:n: id with
// ErrUnsupportedMessageType — additive non-interference (spec-072 stays unbroken).
func TestSCN107006_NudgeReplyIDDoesNotCollideWithSpec072Families(t *testing.T) {
	nudgeID := "a:n:" + string(waNudgeRef) + ":a"

	// a:n: is a nudge reply and nothing else.
	if !IsNudgeReplyID(nudgeID) {
		t.Errorf("IsNudgeReplyID(%q) = false, want true", nudgeID)
	}
	if _, _, ok := DecodeDisambigPayload(nudgeID); ok {
		t.Errorf("DecodeDisambigPayload(%q) = ok, want not-a-disambig", nudgeID)
	}
	if _, _, ok := DecodeConfirmPayload(nudgeID); ok {
		t.Errorf("DecodeConfirmPayload(%q) = ok, want not-a-confirm", nudgeID)
	}
	if _, ok := DecodeResetPayload(nudgeID); ok {
		t.Errorf("DecodeResetPayload(%q) = ok, want not-a-reset", nudgeID)
	}

	// The shipped spec-072 families are NOT nudge replies.
	spec072IDs := []string{
		EncodeDisambigPayload("REF-D", 2),
		EncodeConfirmPayload("REF-C", true),
		EncodeResetPayload("REF-R"),
	}
	for _, id := range spec072IDs {
		if IsNudgeReplyID(id) {
			t.Errorf("IsNudgeReplyID(%q) = true, want false (spec-072 family, not a nudge)", id)
		}
		if _, _, ok := proactive.DecodeNudgeCallback(id); ok {
			t.Errorf("DecodeNudgeCallback(%q) = ok, want false (spec-072 family)", id)
		}
	}

	// spec-072's existing interactive dispatch cleanly refuses an a:n: id rather
	// than mis-routing it — proving the additive family does not disturb the
	// done/certified transport's inbound behavior.
	var out contracts.AssistantMessage
	if err := decodeInteractivePayload(nudgeID, &out); !errors.Is(err, ErrUnsupportedMessageType) {
		t.Errorf("decodeInteractivePayload(%q) err = %v, want ErrUnsupportedMessageType (a:n: is owned by HandleNudgeReply, not spec-072)", nudgeID, err)
	}
	// And the spec-072 dispatch still decodes its OWN families unchanged.
	var confirmOut contracts.AssistantMessage
	if err := decodeInteractivePayload(EncodeConfirmPayload("REF-C", true), &confirmOut); err != nil {
		t.Errorf("decodeInteractivePayload(confirm) err = %v, want nil (spec-072 unbroken)", err)
	}
	if confirmOut.Kind != contracts.KindConfirm {
		t.Errorf("spec-072 confirm decoded kind = %q, want %q", confirmOut.Kind, contracts.KindConfirm)
	}
}

// TestSCN107006_TapReplyRoutesToOneAck proves a chosen WhatsApp interactive
// reply.id resolves through the shared SCOPE-01 registry to exactly one
// Acknowledge(content_key), is idempotent on a second tap, and never mis-acks a
// spec-072 d:/c:/r: reply.
func TestSCN107006_TapReplyRoutesToOneAck(t *testing.T) {
	reg := proactive.NewNudgeRegistry(time.Hour)
	fa := &fakeAck{}
	na := proactive.NewNudgeAck(reg, fa)

	const key = "content-k"
	ref := reg.Mint(key, surfacing.ProducerAlerts, ReservedWhatsAppChannel, "user-1")
	card := permitCard(t, ref, "Your weekly review is ready", false)

	msg, ok := BuildNudgeInteractive(card, nudgeMaxChars)
	if !ok {
		t.Fatalf("BuildNudgeInteractive ok=false")
	}
	actID := msg.Buttons[0].ID // Act
	if want := "a:n:" + string(ref) + ":a"; actID != want {
		t.Fatalf("Act reply.id = %q, want %q", actID, want)
	}

	out, err := HandleNudgeReply(actID, na)
	if err != nil {
		t.Fatalf("HandleNudgeReply(act) err = %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged || out.ContentKey != key {
		t.Errorf("outcome = %+v, want acted+acknowledged content_key=%q", out, key)
	}
	if len(fa.acked) != 1 || fa.acked[0] != key {
		t.Errorf("acked = %v, want exactly [%q]", fa.acked, key)
	}

	// A second tap on the same ref is idempotent: already-handled, no re-ack.
	again, err := HandleNudgeReply(actID, na)
	if err != nil {
		t.Fatalf("second HandleNudgeReply err = %v", err)
	}
	if again.State != proactive.StateAlreadyHandled || again.Acknowledged {
		t.Errorf("second outcome = %+v, want already-handled + not acknowledged", again)
	}
	if len(fa.acked) != 1 {
		t.Errorf("acked after second tap = %v, want still exactly one ack", fa.acked)
	}

	// A spec-072 confirm reply.id is refused, never mis-acked.
	if _, err := HandleNudgeReply(EncodeConfirmPayload("REF-C", true), na); err == nil {
		t.Error("HandleNudgeReply(confirm id) err = nil, want a non-nudge refusal")
	}
	// A nil ack is a wiring error surfaced honestly.
	if _, err := HandleNudgeReply(actID, nil); err == nil {
		t.Error("HandleNudgeReply(nil ack) err = nil, want a wiring error")
	}
}

// TestSCN107006_ListAndNumberedTextFallbackPreserveActionsAndProvenance
// (T107-03B1-FALLBACK) proves the interactive-list and numbered plain-text
// fallbacks preserve the same three actions (each still carrying / mapping to
// a:n:<ref>:<a|s|d>) and the producer-derived provenance line.
func TestSCN107006_ListAndNumberedTextFallbackPreserveActionsAndProvenance(t *testing.T) {
	card := permitCard(t, waNudgeRef, "Your weekly review is ready", false)

	// Interactive-list fallback: 1 section, 3 rows, each carrying the a:n: id.
	list, ok := BuildNudgeListFallback(card, nudgeMaxChars)
	if !ok {
		t.Fatalf("BuildNudgeListFallback ok=false")
	}
	if list.Kind != OutboundInteractiveList {
		t.Errorf("list kind = %q, want %q", list.Kind, OutboundInteractiveList)
	}
	if !strings.Contains(list.Body, "Why: From your alerts") {
		t.Errorf("list body missing provenance line: %q", list.Body)
	}
	if list.ListButton == "" {
		t.Errorf("list fallback has empty CTA button")
	}
	if len(list.ListSections) != 1 || len(list.ListSections[0].Rows) != 3 {
		t.Fatalf("list shape = %d sections, want 1 section / 3 rows", len(list.ListSections))
	}
	wantRow := []struct{ title, suffix string }{{"Act", "a"}, {"Snooze", "s"}, {"Dismiss", "d"}}
	for i, row := range list.ListSections[0].Rows {
		if row.Title != wantRow[i].title {
			t.Errorf("row[%d].Title = %q, want %q", i, row.Title, wantRow[i].title)
		}
		if want := "a:n:" + string(waNudgeRef) + ":" + wantRow[i].suffix; row.ID != want {
			t.Errorf("row[%d].ID = %q, want %q", i, row.ID, want)
		}
	}

	// Numbered plain-text fallback: body + Why line + 1|2|3 -> Act|Snooze|Dismiss.
	txt, ok := BuildNudgeTextFallback(card, nudgeMaxChars)
	if !ok {
		t.Fatalf("BuildNudgeTextFallback ok=false")
	}
	for _, want := range []string{"Your weekly review is ready", "Why: From your alerts", "1. Act", "2. Snooze", "3. Dismiss"} {
		if !strings.Contains(txt.Body, want) {
			t.Errorf("numbered fallback body missing %q:\n%s", want, txt.Body)
		}
	}

	// The numbered digits map back to the same three actions.
	for digit, wantAction := range map[string]proactive.NudgeAction{
		"1": proactive.ActionAct, "2": proactive.ActionSnooze, "3": proactive.ActionDismiss,
	} {
		got, ok := NumberedFallbackReply(digit)
		if !ok || got != wantAction {
			t.Errorf("NumberedFallbackReply(%q) = (%s,%v), want (%s,true)", digit, got, ok, wantAction)
		}
	}
	for _, bad := range []string{"0", "4", "", "a"} {
		if _, ok := NumberedFallbackReply(bad); ok {
			t.Errorf("NumberedFallbackReply(%q) = ok, want false", bad)
		}
	}
}

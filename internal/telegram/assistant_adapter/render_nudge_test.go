package assistant_adapter

import (
	"errors"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

// render_nudge_test.go — spec 107 SCOPE-03A adapter-level unit + golden tests
// (SCN-107-005). Rows: T107-03A-GOLDEN (golden inline render), T107-005-U (a:n:
// encode/decode within 64 bytes), T107-03-COLLISION (a:n: never collides with
// a:c:/a:d:/spec-028 list_check:), plus the honest non-card / fail-loud render
// assertions. White-box (package assistant_adapter) so it can reach the
// unexported decodeCallbackData / encodeNudgeCallback / callbackKind* seam.

// nudgeGoldenRef is a valid opaque 26-char ULID-shaped ref (distinct from
// callbacks_nudge_test.go's nudgeTestRef to avoid a redeclaration in-package).
const nudgeGoldenRef proactive.NudgeRef = "01HZZ9WVUTSRQPONMLKJIHGFED"

// recordingNudgeSender captures the outbound tgbotapi payloads so a golden /
// fail-loud assertion can inspect them without a live bot.
type recordingNudgeSender struct {
	sent []tgbotapi.Chattable
	err  error
}

func (r *recordingNudgeSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	r.sent = append(r.sent, c)
	if r.err != nil {
		return tgbotapi.Message{}, r.err
	}
	return tgbotapi.Message{MessageID: 4242}, nil
}

// fakeAck records the single Acknowledge(content_key) the ack path makes.
type fakeAck struct{ acked []string }

func (f *fakeAck) Acknowledge(k string) { f.acked = append(f.acked, k) }

// permitCard projects a permit/escalated card from the real foundation.
func permitCard(t *testing.T, ref proactive.NudgeRef, title string, escalated bool) proactive.ProactiveCardModel {
	t.Helper()
	kind := surfacing.DecisionPermit
	if escalated {
		kind = surfacing.DecisionEscalated
	}
	card, ok := proactive.ProjectCard(
		surfacing.SurfacingDecision{Kind: kind},
		surfacing.SurfacingCandidate{
			Producer:   surfacing.ProducerAlerts,
			Channel:    surfacing.ChannelTelegram,
			ContentKey: "content-k",
		},
		ref, title,
	)
	if !ok {
		t.Fatalf("ProjectCard(kind=%s) ok=false, want a card", kind)
	}
	return card
}

// TestSCN107005_NudgeGoldenInlineRender (T107-03A-GOLDEN) proves the adapter
// renders a permit card as a Telegram inline-keyboard message: title + "Why:"
// provenance line + the three a:n: Act/Snooze/Dismiss buttons — no web surface.
func TestSCN107005_NudgeGoldenInlineRender(t *testing.T) {
	const title = "Your weekly review is ready"
	card := permitCard(t, nudgeGoldenRef, title, false)

	msg, ok := BuildNudgeMessage(9001, PlainText, card)
	if !ok {
		t.Fatalf("BuildNudgeMessage ok=false, want a card")
	}

	wantBody := title + "\n\nWhy: From your alerts"
	if msg.Text != wantBody {
		t.Errorf("golden body mismatch:\n got  %q\n want %q", msg.Text, wantBody)
	}

	kb, kbOK := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !kbOK {
		t.Fatalf("ReplyMarkup type = %T, want tgbotapi.InlineKeyboardMarkup", msg.ReplyMarkup)
	}
	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 3 {
		t.Fatalf("keyboard shape = %d rows / %v, want 1 row of 3 buttons", len(kb.InlineKeyboard), kb.InlineKeyboard)
	}
	wantButtons := []struct{ text, suffix string }{
		{"Act", "a"}, {"Snooze", "s"}, {"Dismiss", "d"},
	}
	for i, w := range wantButtons {
		btn := kb.InlineKeyboard[0][i]
		if btn.Text != w.text {
			t.Errorf("button[%d].Text = %q, want %q", i, btn.Text, w.text)
		}
		wantData := "a:n:" + string(nudgeGoldenRef) + ":" + w.suffix
		if btn.CallbackData == nil || *btn.CallbackData != wantData {
			t.Errorf("button[%d].CallbackData = %v, want %q", i, btn.CallbackData, wantData)
		}
	}

	// Escalated variant carries the URGENT ESCALATION provenance marker.
	esc := permitCard(t, nudgeGoldenRef, title, true)
	escMsg, ok := BuildNudgeMessage(9001, PlainText, esc)
	if !ok {
		t.Fatalf("BuildNudgeMessage(escalated) ok=false")
	}
	wantEsc := title + "\n\nWhy: URGENT ESCALATION — From your alerts"
	if escMsg.Text != wantEsc {
		t.Errorf("escalated body mismatch:\n got  %q\n want %q", escMsg.Text, wantEsc)
	}
}

// TestSCN107005_NudgeCallbackWithin64Bytes (T107-005-U) proves the a:n: wire
// form for a 26-char ULID ref encodes within Telegram's 64-byte callback_data
// bound for every action and round-trips through the adapter decoder.
func TestSCN107005_NudgeCallbackWithin64Bytes(t *testing.T) {
	actions := []struct {
		action proactive.NudgeAction
		suffix string
	}{
		{proactive.ActionAct, "a"},
		{proactive.ActionSnooze, "s"},
		{proactive.ActionDismiss, "d"},
	}
	for _, a := range actions {
		data, ok := encodeNudgeCallback(nudgeGoldenRef, a.action)
		if !ok {
			t.Fatalf("encodeNudgeCallback(%s) ok=false", a.action)
		}
		if len(data) > 64 {
			t.Errorf("callback_data %q is %d bytes, want <= 64", data, len(data))
		}
		if want := "a:n:" + string(nudgeGoldenRef) + ":" + a.suffix; data != want {
			t.Errorf("encoded = %q, want %q", data, want)
		}
		got, err := decodeCallbackData(data)
		if err != nil {
			t.Fatalf("decodeCallbackData(%q) err = %v", data, err)
		}
		if got.kind != callbackKindNudge || got.nudgeRef != nudgeGoldenRef || got.nudgeAction != a.action {
			t.Errorf("round-trip = %+v, want kind=nudge ref=%q action=%s", got, nudgeGoldenRef, a.action)
		}
	}
}

// TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List
// (T107-03-COLLISION) proves the additive a:n: family never collides with the
// a:c:/a:d: assistant families NOR the spec-028 list_check: scheme (FR-107-010).
func TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List(t *testing.T) {
	// a:n: decodes to nudge.
	nudge, err := decodeCallbackData("a:n:" + string(nudgeGoldenRef) + ":a")
	if err != nil || nudge.kind != callbackKindNudge {
		t.Fatalf("a:n: decode = (%+v, %v), want kind=nudge", nudge, err)
	}
	// a:c: still confirm; a:d: still disambig (unchanged by the additive family).
	confirm, err := decodeCallbackData("a:c:" + string(nudgeGoldenRef) + ":pos")
	if err != nil || confirm.kind != callbackKindConfirm {
		t.Errorf("a:c: decode = (%+v, %v), want kind=confirm", confirm, err)
	}
	disambig, err := decodeCallbackData("a:d:" + string(nudgeGoldenRef) + ":3")
	if err != nil || disambig.kind != callbackKindDisambig {
		t.Errorf("a:d: decode = (%+v, %v), want kind=disambig", disambig, err)
	}

	// spec-028 list_check: scheme is NOT in the a: assistant namespace at all —
	// structural non-collision (internal/telegram/list.go handleListCallback).
	const listCB = "list_check:list1:item1"
	if IsAssistantCallback(listCB) {
		t.Errorf("IsAssistantCallback(%q) = true, want false (spec-028 list is not a: namespace)", listCB)
	}
	if _, err := decodeCallbackData(listCB); !errors.Is(err, ErrNotAssistantMessage) {
		t.Errorf("decodeCallbackData(%q) err = %v, want ErrNotAssistantMessage", listCB, err)
	}
	// A nudge wire is never a list_check: payload and vice versa.
	nudgeWire := "a:n:" + string(nudgeGoldenRef) + ":a"
	if strings.HasPrefix(nudgeWire, "list_check:") || strings.HasPrefix(listCB, "a:") {
		t.Errorf("prefix overlap between %q and %q", nudgeWire, listCB)
	}
}

// TestSCN107005_NonCardStateRendersHonestTextNoKeyboard proves every non-card /
// terminal state renders as a distinct honest TEXT line with NO keyboard, and
// BuildNudgeMessage refuses to draw a card for any of them (anti-fabrication).
func TestSCN107005_NonCardStateRendersHonestTextNoKeyboard(t *testing.T) {
	nonCard := []proactive.HonestState{
		proactive.StateBudgetExhausted, proactive.StateDeduped, proactive.StateAlreadyHandled,
		proactive.StateExpired, proactive.StateActed, proactive.StateSnoozed,
		proactive.StateSuppressed, proactive.StateError,
	}
	for _, st := range nonCard {
		if _, ok := BuildNudgeMessage(9001, PlainText, proactive.ProactiveCardModel{State: st}); ok {
			t.Errorf("BuildNudgeMessage(state=%s) ok=true, want false (never draw a card for a non-card state)", st)
		}
		m := BuildNudgeStateMessage(9001, PlainText, st)
		if m.ReplyMarkup != nil {
			t.Errorf("state %s carried a keyboard, want text-only", st)
		}
		if strings.TrimSpace(m.Text) == "" {
			t.Errorf("state %s rendered empty text", st)
		}
	}

	// The seven primary honest states must render seven DISTINCT lines (each
	// honest state is visible and never silently substituted for another).
	distinct := []proactive.HonestState{
		proactive.StateActed, proactive.StateSnoozed, proactive.StateSuppressed,
		proactive.StateAlreadyHandled, proactive.StateExpired,
		proactive.StateBudgetExhausted, proactive.StateDeduped,
	}
	seen := map[string]proactive.HonestState{}
	for _, st := range distinct {
		line := nudgeStateText(st)
		if prev, dup := seen[line]; dup {
			t.Errorf("states %s and %s share line %q (must be distinct)", prev, st, line)
		}
		seen[line] = st
	}
}

// TestSCN107005_RenderNudgeRefusesNonCardState proves RenderNudge fails loudly
// (never sends) for a non-card state and sends exactly once for a card.
func TestSCN107005_RenderNudgeRefusesNonCardState(t *testing.T) {
	rs := &recordingNudgeSender{}
	if _, err := RenderNudge(rs, 9001, PlainText, proactive.ProactiveCardModel{State: proactive.StateBudgetExhausted}); err == nil {
		t.Error("RenderNudge(non-card) err = nil, want a refusal error")
	}
	if len(rs.sent) != 0 {
		t.Errorf("RenderNudge(non-card) sent %d messages, want 0", len(rs.sent))
	}

	card := permitCard(t, nudgeGoldenRef, "Your weekly review is ready", false)
	if _, err := RenderNudge(rs, 9001, PlainText, card); err != nil {
		t.Fatalf("RenderNudge(card) err = %v", err)
	}
	if len(rs.sent) != 1 {
		t.Errorf("RenderNudge(card) sent %d messages, want 1", len(rs.sent))
	}
}

// TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace proves a tapped
// a:n: callback resolves through the shared registry to exactly one
// Acknowledge(content_key) and edits the message in place to the terminal state.
func TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace(t *testing.T) {
	reg := proactive.NewNudgeRegistry(time.Hour)
	fa := &fakeAck{}
	na := proactive.NewNudgeAck(reg, fa)
	ref := reg.Mint("content-k", surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")

	data, ok := encodeNudgeCallback(ref, proactive.ActionAct)
	if !ok {
		t.Fatalf("encodeNudgeCallback ok=false")
	}
	cb := &tgbotapi.CallbackQuery{
		Data:    data,
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 9001}},
	}
	rs := &recordingNudgeSender{}
	out, err := HandleNudgeCallback(rs, cb, na)
	if err != nil {
		t.Fatalf("HandleNudgeCallback err = %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged {
		t.Errorf("outcome = %+v, want acted+acknowledged", out)
	}
	if len(fa.acked) != 1 || fa.acked[0] != "content-k" {
		t.Errorf("acked = %v, want exactly [content-k]", fa.acked)
	}
	if len(rs.sent) != 1 {
		t.Errorf("edit sent %d times, want 1 in-place edit", len(rs.sent))
	}
	if _, isEdit := rs.sent[0].(tgbotapi.EditMessageTextConfig); !isEdit {
		t.Errorf("terminal render type = %T, want tgbotapi.EditMessageTextConfig (edit-in-place)", rs.sent[0])
	}

	// A second tap on the same ref is idempotent: already-handled, no re-ack.
	again, err := HandleNudgeCallback(rs, cb, na)
	if err != nil {
		t.Fatalf("second HandleNudgeCallback err = %v", err)
	}
	if again.State != proactive.StateAlreadyHandled || again.Acknowledged {
		t.Errorf("second outcome = %+v, want already-handled + not acknowledged", again)
	}
	if len(fa.acked) != 1 {
		t.Errorf("acked after second tap = %v, want still exactly one ack", fa.acked)
	}

	// A non-nudge (a:c:) callback is refused, never mis-acked.
	if _, err := HandleNudgeCallback(rs, &tgbotapi.CallbackQuery{Data: "a:c:" + string(ref) + ":pos"}, na); err == nil {
		t.Error("HandleNudgeCallback(a:c:) err = nil, want a non-nudge refusal")
	}
}

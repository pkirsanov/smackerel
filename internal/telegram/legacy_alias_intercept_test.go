// Spec 066 SCOPE-2 — unit coverage for the retired-alias
// interceptor. Exercises the alias rewrite + closed-window
// rejection paths against a fake legacyretirement.Policy so the
// logic can be verified without standing up Postgres or the
// assistant facade.
package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// fakePolicy is a minimal legacyretirement.Policy that returns a
// pre-canned decision.
type fakePolicy struct {
	decision legacyretirement.RetirementDecision
	err      error
	captured legacyretirement.AssistantTurn
}

func (f *fakePolicy) Handle(_ context.Context, turn legacyretirement.AssistantTurn) (legacyretirement.RetirementDecision, error) {
	f.captured = turn
	if f.err != nil {
		return legacyretirement.RetirementDecision{}, f.err
	}
	d := f.decision
	d.DecidedAt = turn.ReceivedAt
	return d, nil
}

func newTestBotWithRecorder() (*Bot, *[]string) {
	captured := []string{}
	bot := &Bot{environment: "test"}
	bot.replyFunc = func(chatID int64, text string) {
		captured = append(captured, text)
	}
	return bot, &captured
}

func newRetiredCommand(msg, args string) *tgbotapi.Message {
	full := "/" + msg
	if args != "" {
		full = full + " " + args
	}
	return &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 99},
		Text:      full,
		Entities:  []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(msg) + 1}},
	}
}

func TestLegacyAliasPromptForSubstitutesArgs(t *testing.T) {
	got, ok := legacyAliasPromptFor("find", "ACL tags")
	if !ok || got != "find ACL tags" {
		t.Fatalf("legacyAliasPromptFor(find): ok=%v got=%q", ok, got)
	}
	// Empty args collapses to a bare prompt.
	got, ok = legacyAliasPromptFor("lint", "")
	if !ok || got != "show knowledge quality issues" {
		t.Fatalf("legacyAliasPromptFor(lint): ok=%v got=%q", ok, got)
	}
	// Unknown command returns ok=false.
	if _, ok := legacyAliasPromptFor("help", ""); ok {
		t.Fatalf("legacyAliasPromptFor(help) ok=true, want false")
	}
}

func TestInterceptLegacyAlias_ClosedWindow_SendsUnknownCopyAndShortCircuits(t *testing.T) {
	// SCN-066-A05 — closed-window retired command returns the
	// canonical unknown-command copy and the bot reports handled=true
	// so the legacy /find handler is NOT invoked.
	bot, captured := newTestBotWithRecorder()
	policy := &fakePolicy{decision: legacyretirement.RetirementDecision{
		Matched: true,
		Command: legacyretirement.RetiredCommand{
			Command:            "/find",
			ReplacementExample: "I no longer answer /find. Try plain English; see /help.",
			NoticeCopy:         "should-not-render",
		},
		EffectiveState: legacyretirement.WindowClosed,
		ServeNL:        false,
		Outcome:        legacyretirement.OutcomeClosedUnknown,
	}}
	interceptor, err := NewLegacyAliasInterceptor(policy, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatalf("NewLegacyAliasInterceptor: %v", err)
	}
	bot.SetLegacyAliasInterceptor(interceptor)

	handled, err := bot.interceptLegacyAlias(context.Background(), newRetiredCommand("find", "ACL tags"), 42)
	if err != nil || !handled {
		t.Fatalf("interceptLegacyAlias: handled=%v err=%v; want handled=true err=nil", handled, err)
	}
	if len(*captured) != 1 {
		t.Fatalf("expected exactly 1 reply, got %d (%v)", len(*captured), *captured)
	}
	if !strings.Contains((*captured)[0], "I no longer answer") {
		t.Errorf("closed-window reply did not carry the unknown-command copy; got %q", (*captured)[0])
	}
	if policy.captured.RawText != "/find ACL tags" {
		t.Errorf("policy RawText: got %q, want %q", policy.captured.RawText, "/find ACL tags")
	}
	if policy.captured.Transport != "telegram" {
		t.Errorf("policy Transport: got %q, want telegram", policy.captured.Transport)
	}
}

func TestInterceptLegacyAlias_OpenWindow_NoAdapter_ShowsNoticeAndRewrite(t *testing.T) {
	// SCN-066-A04 — open window with ShowNotice=true emits the
	// notice copy AND the rewritten prompt; the legacy handler does
	// NOT run (handled=true). Without a bound facade the rewritten
	// prompt is the second reply.
	bot, captured := newTestBotWithRecorder()
	policy := &fakePolicy{decision: legacyretirement.RetirementDecision{
		Matched: true,
		Command: legacyretirement.RetiredCommand{
			Command:            "/find",
			ReplacementExample: "find ACL tags",
			NoticeCopy:         "Heads up: /find is going away. Use plain English instead.",
		},
		EffectiveState: legacyretirement.WindowOpen,
		ShowNotice:     true,
		ServeNL:        true,
		Outcome:        legacyretirement.OutcomeNoticeAndServed,
	}}
	interceptor, err := NewLegacyAliasInterceptor(policy, nil)
	if err != nil {
		t.Fatalf("NewLegacyAliasInterceptor: %v", err)
	}
	bot.SetLegacyAliasInterceptor(interceptor)

	handled, err := bot.interceptLegacyAlias(context.Background(), newRetiredCommand("find", "ACL tags"), 7)
	if err != nil || !handled {
		t.Fatalf("interceptLegacyAlias: handled=%v err=%v", handled, err)
	}
	if len(*captured) != 2 {
		t.Fatalf("expected 2 replies (notice + rewrite), got %d (%v)", len(*captured), *captured)
	}
	if !strings.Contains((*captured)[0], "Heads up") {
		t.Errorf("first reply must be the notice copy; got %q", (*captured)[0])
	}
	if (*captured)[1] != "find ACL tags" {
		t.Errorf("second reply must be the rewritten plain-English prompt; got %q", (*captured)[1])
	}
}

func TestInterceptLegacyAlias_OpenWindow_AlreadyNotified_NoNoticeJustRewrite(t *testing.T) {
	// Dedup branch — within the same window the policy returns
	// ShowNotice=false and the bot MUST NOT send the notice copy
	// again; only the rewritten prompt reaches the user.
	bot, captured := newTestBotWithRecorder()
	policy := &fakePolicy{decision: legacyretirement.RetirementDecision{
		Matched: true,
		Command: legacyretirement.RetiredCommand{
			Command:    "/rate",
			NoticeCopy: "should-not-render",
		},
		EffectiveState: legacyretirement.WindowOpen,
		ShowNotice:     false,
		ServeNL:        true,
		Outcome:        legacyretirement.OutcomeServedNoNotice,
	}}
	interceptor, _ := NewLegacyAliasInterceptor(policy, nil)
	bot.SetLegacyAliasInterceptor(interceptor)

	handled, err := bot.interceptLegacyAlias(context.Background(), newRetiredCommand("rate", "carbonara 5/5"), 8)
	if err != nil || !handled {
		t.Fatalf("interceptLegacyAlias: handled=%v err=%v", handled, err)
	}
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply (rewrite only), got %d (%v)", len(*captured), *captured)
	}
	if (*captured)[0] != "rate carbonara 5/5" {
		t.Errorf("rewrite reply mismatch: got %q", (*captured)[0])
	}
}

func TestInterceptLegacyAlias_PausedWindow_RewritesWithoutNotice(t *testing.T) {
	// Spec 075 auto-pause safety mode: rewrite + passthrough; no new
	// notice is emitted regardless of ledger state.
	bot, captured := newTestBotWithRecorder()
	policy := &fakePolicy{decision: legacyretirement.RetirementDecision{
		Matched:        true,
		Command:        legacyretirement.RetiredCommand{Command: "/find"},
		EffectiveState: legacyretirement.WindowPaused,
		ShowNotice:     false,
		ServeNL:        true,
		Outcome:        legacyretirement.OutcomePausedSuppressed,
	}}
	interceptor, _ := NewLegacyAliasInterceptor(policy, nil)
	bot.SetLegacyAliasInterceptor(interceptor)

	handled, err := bot.interceptLegacyAlias(context.Background(), newRetiredCommand("find", "Postgres tuning"), 9)
	if err != nil || !handled {
		t.Fatalf("interceptLegacyAlias: handled=%v err=%v", handled, err)
	}
	if len(*captured) != 1 || (*captured)[0] != "find Postgres tuning" {
		t.Fatalf("paused branch must rewrite + reply once; got %v", *captured)
	}
}

func TestInterceptLegacyAlias_NotRetiredCommand_NotIntercepted(t *testing.T) {
	bot, captured := newTestBotWithRecorder()
	policy := &fakePolicy{decision: legacyretirement.RetirementDecision{Matched: false}}
	interceptor, _ := NewLegacyAliasInterceptor(policy, nil)
	bot.SetLegacyAliasInterceptor(interceptor)

	// /help is operational — interceptor must NOT consult the policy
	// and MUST NOT short-circuit dispatch.
	msg := newRetiredCommand("help", "")
	handled, err := bot.interceptLegacyAlias(context.Background(), msg, 1)
	if err != nil || handled {
		t.Fatalf("operational command must not be intercepted; handled=%v err=%v", handled, err)
	}
	if len(*captured) != 0 {
		t.Fatalf("operational command must not emit replies; got %v", *captured)
	}
	if policy.captured.RawText != "" {
		t.Errorf("policy should not have been consulted for operational command; got %q", policy.captured.RawText)
	}
}

func TestInterceptLegacyAlias_NoInterceptorWired_Passthrough(t *testing.T) {
	bot := &Bot{environment: "test"}
	bot.replyFunc = func(int64, string) {}
	handled, err := bot.interceptLegacyAlias(context.Background(), newRetiredCommand("find", "x"), 1)
	if err != nil || handled {
		t.Fatalf("unwired interceptor must passthrough; handled=%v err=%v", handled, err)
	}
}

func TestInterceptLegacyAlias_PolicyError_PropagatesAsHandledFalse(t *testing.T) {
	bot, _ := newTestBotWithRecorder()
	policy := &fakePolicy{err: errors.New("ledger down")}
	interceptor, _ := NewLegacyAliasInterceptor(policy, nil)
	bot.SetLegacyAliasInterceptor(interceptor)

	handled, err := bot.interceptLegacyAlias(context.Background(), newRetiredCommand("find", "x"), 1)
	if err == nil {
		t.Fatalf("expected policy error to propagate, got nil")
	}
	if handled {
		t.Fatalf("policy error must not short-circuit dispatch; handled=true would lose the user's message")
	}
}

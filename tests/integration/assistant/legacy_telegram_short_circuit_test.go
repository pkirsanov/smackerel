//go:build integration

// Spec 075 SCOPE-6.4 — TP-075-23.
//
// Integration row: when the inbound Telegram update is already
// flagged as upstream of the assistant facade (context carries
// telegram.AssistantFacadeUpstreamKey == true), the legacy alias
// interceptor MUST short-circuit without invoking the spec 075
// Policy. Covers SCN-075-A13 — no double dispatch when facade
// Policy is upstream.
//
// Non-upstream path remains exercised by the existing unit tests in
// internal/telegram/legacy_alias_intercept_test.go; this row pins
// the new context-key contract.

package assistant_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
	"github.com/smackerel/smackerel/internal/telegram"
)

type recordingShortCircuitPolicy struct {
	calls int
}

func (p *recordingShortCircuitPolicy) Handle(_ context.Context, turn legacyretirement.AssistantTurn) (legacyretirement.RetirementDecision, error) {
	p.calls++
	return legacyretirement.RetirementDecision{
		Matched:        true,
		Command:        legacyretirement.RetiredCommand{Command: "/find", NoticeCopy: "x", ReplacementExample: "y"},
		EffectiveState: legacyretirement.WindowOpen,
		ServeNL:        true,
		ShowNotice:     true,
		WindowID:       "ignored",
		DecidedAt:      turn.ReceivedAt,
	}, nil
}

func TestTelegramLegacyAliasInterceptor_TP_075_23_ShortCircuitsWhenFacadeUpstream(t *testing.T) {
	recorded := []string{}
	bot := telegram.NewBotForLegacyInterceptIntegrationTest(&recorded)
	policy := &recordingShortCircuitPolicy{}
	interceptor, err := telegram.NewLegacyAliasInterceptor(policy, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatalf("NewLegacyAliasInterceptor: %v", err)
	}
	bot.SetLegacyAliasInterceptor(interceptor)

	upstreamCtx := telegram.WithAssistantFacadeUpstream(context.Background())
	msg := telegram.NewRetiredCommandMessageForIntegrationTest("find", "ACL tags")

	handled, err := bot.InterceptLegacyAliasForIntegrationTest(upstreamCtx, msg, 100)
	if err != nil {
		t.Fatalf("InterceptLegacyAlias: %v", err)
	}
	if handled {
		t.Errorf("handled=true; want false — upstream-facade path must short-circuit, not handle")
	}
	if policy.calls != 0 {
		t.Errorf("policy.Handle invoked %d times; want 0 — short-circuit must not call the Policy", policy.calls)
	}
	if len(recorded) != 0 {
		t.Errorf("recorded replies=%v; want none — short-circuit must not emit any user-facing message", recorded)
	}
}

// Adversarial: when the upstream marker is NOT set, the interceptor
// MUST still consult the Policy (proves the short-circuit guard is
// the only thing controlling the new path — not an accidental
// unconditional bypass).
func TestTelegramLegacyAliasInterceptor_TP_075_23_NonUpstreamStillInvokesPolicy(t *testing.T) {
	recorded := []string{}
	bot := telegram.NewBotForLegacyInterceptIntegrationTest(&recorded)
	policy := &recordingShortCircuitPolicy{}
	interceptor, err := telegram.NewLegacyAliasInterceptor(policy, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatalf("NewLegacyAliasInterceptor: %v", err)
	}
	bot.SetLegacyAliasInterceptor(interceptor)

	msg := telegram.NewRetiredCommandMessageForIntegrationTest("find", "ACL tags")

	if _, err := bot.InterceptLegacyAliasForIntegrationTest(context.Background(), msg, 101); err != nil {
		t.Fatalf("InterceptLegacyAlias: %v", err)
	}
	if policy.calls != 1 {
		t.Errorf("policy.Handle invocations=%d; want 1 — non-upstream path must consult Policy", policy.calls)
	}
}

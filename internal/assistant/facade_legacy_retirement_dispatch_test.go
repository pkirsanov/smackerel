// Spec 075 SCOPE-6.1 — Facade Policy dispatch contract test (TP-075-19).
//
// Exercises Facade.Handle's pre-routing legacy-retirement Policy
// dispatch across all five branches from SCN-075-A12 using a stub
// legacyretirement.Policy. NO transport, ledger, telemetry, or
// observation surfaces are invoked — this is a pure facade-level
// contract test.

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// stubLegacyPolicy returns a pre-canned RetirementDecision and records
// the AssistantTurn it received.
type stubLegacyPolicy struct {
	decision legacyretirement.RetirementDecision
	err      error
	captured legacyretirement.AssistantTurn
	calls    int
}

func (s *stubLegacyPolicy) Handle(_ context.Context, turn legacyretirement.AssistantTurn) (legacyretirement.RetirementDecision, error) {
	s.calls++
	s.captured = turn
	if s.err != nil {
		return legacyretirement.RetirementDecision{}, s.err
	}
	d := s.decision
	d.DecidedAt = turn.ReceivedAt
	return d, nil
}

// newPolicyFacade wires a minimal facade with the supplied Policy and
// a no-op routing pipeline that returns BandLow → StatusSavedAsIdea.
// All five branches reuse this fixture; per-branch assertions inspect
// the LegacyRetirementNotice attachment and the closed-window
// short-circuit body.
func newPolicyFacade(t *testing.T, pol legacyretirement.Policy) (*Facade, *stubExecutor, *recordingAudit) {
	t.Helper()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	cfg.Policy = pol

	registry := mapRegistry{scenarios: map[string]*agent.Scenario{}}
	manifest := newTestManifest(map[string]manifestEntry{})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	// Router returns ok=false → BandLow capture fallback path. This
	// makes the "continue normal routing" branches deterministic
	// without needing real scenarios.
	router := &stubRouter{ok: false}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)
	return facade, executor, audit
}

func newRetiredCmd() legacyretirement.RetiredCommand {
	return legacyretirement.RetiredCommand{
		Command:            "/weather",
		ReplacementExample: "weather in Barcelona tomorrow",
		NoticeCopy:         "I'm retiring /weather; just ask in plain English.",
		Spec066ID:          "spec066-weather",
	}
}

const testFacadeWindowID = "window-2026-Q2"

// Branch 1 — Matched open + ShowNotice: NoticePayload attached, normal
// routing proceeds (executor invocations / routing decisions are not
// suppressed by the Policy dispatch).
func TestFacadeLegacyRetirement_OpenNoticeAttachesPayload(t *testing.T) {
	t.Parallel()

	pol := &stubLegacyPolicy{decision: legacyretirement.RetirementDecision{
		Matched:        true,
		Command:        newRetiredCmd(),
		EffectiveState: legacyretirement.WindowOpen,
		ShowNotice:     true,
		ServeNL:        true,
		Outcome:        legacyretirement.OutcomeNoticeAndServed,
		WindowID:       testFacadeWindowID,
	}}
	facade, _, _ := newPolicyFacade(t, pol)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u1", Transport: "telegram", Text: "/weather here",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if pol.calls != 1 {
		t.Fatalf("Policy.Handle calls = %d, want 1", pol.calls)
	}
	if pol.captured.UserID != "u1" || pol.captured.Transport != "telegram" || pol.captured.RawText != "/weather here" {
		t.Errorf("captured turn = %+v, want UserID=u1 Transport=telegram RawText=/weather here", pol.captured)
	}
	if resp.LegacyRetirementNotice == nil {
		t.Fatal("LegacyRetirementNotice is nil; want populated payload on open+notice branch")
	}
	np := resp.LegacyRetirementNotice
	if np.Command != "/weather" {
		t.Errorf("NoticePayload.Command = %q, want /weather", np.Command)
	}
	if np.ReplacementExample != "weather in Barcelona tomorrow" {
		t.Errorf("NoticePayload.ReplacementExample = %q, want weather in Barcelona tomorrow", np.ReplacementExample)
	}
	if np.CopyKey != "spec066-weather" {
		t.Errorf("NoticePayload.CopyKey = %q, want spec066-weather", np.CopyKey)
	}
	if np.WindowID != testFacadeWindowID {
		t.Errorf("NoticePayload.WindowID = %q, want %q", np.WindowID, testFacadeWindowID)
	}
	// Routing still ran (router.ok=false ⇒ BandLow capture path).
	if !resp.CaptureRoute {
		t.Error("CaptureRoute = false; expected normal routing pipeline to execute after notice attach")
	}
}

// Branch 2 — Matched open + dedup suppressed: no NoticePayload, normal
// routing proceeds.
func TestFacadeLegacyRetirement_OpenDedupSuppressed(t *testing.T) {
	t.Parallel()

	pol := &stubLegacyPolicy{decision: legacyretirement.RetirementDecision{
		Matched:        true,
		Command:        newRetiredCmd(),
		EffectiveState: legacyretirement.WindowOpen,
		ShowNotice:     false,
		ServeNL:        true,
		Outcome:        legacyretirement.OutcomeServedNoNotice,
		WindowID:       testFacadeWindowID,
	}}
	facade, _, _ := newPolicyFacade(t, pol)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u2", Transport: "web", Text: "/weather again",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.LegacyRetirementNotice != nil {
		t.Fatalf("LegacyRetirementNotice = %+v; want nil on dedup-suppressed branch", resp.LegacyRetirementNotice)
	}
	if !resp.CaptureRoute {
		t.Error("CaptureRoute = false; expected normal routing pipeline to execute")
	}
}

// Branch 3 — Matched paused: no NoticePayload, normal routing proceeds
// (legacy serving preserved by ServeNL=true).
func TestFacadeLegacyRetirement_PausedPreservesLegacyServing(t *testing.T) {
	t.Parallel()

	pol := &stubLegacyPolicy{decision: legacyretirement.RetirementDecision{
		Matched:        true,
		Command:        newRetiredCmd(),
		EffectiveState: legacyretirement.WindowPaused,
		ShowNotice:     false,
		ServeNL:        true,
		Outcome:        legacyretirement.OutcomePausedSuppressed,
		WindowID:       testFacadeWindowID,
	}}
	facade, _, _ := newPolicyFacade(t, pol)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u3", Transport: "telegram", Text: "/weather paused",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.LegacyRetirementNotice != nil {
		t.Fatalf("LegacyRetirementNotice = %+v; want nil on paused branch", resp.LegacyRetirementNotice)
	}
	if !resp.CaptureRoute {
		t.Error("CaptureRoute = false; expected normal routing pipeline to execute (paused serves legacy NL)")
	}
}

// Branch 4 — Matched closed: canonical unknown-command response is
// returned and the routing pipeline does NOT run.
func TestFacadeLegacyRetirement_ClosedShortCircuitsWithCanonicalResponse(t *testing.T) {
	t.Parallel()

	pol := &stubLegacyPolicy{decision: legacyretirement.RetirementDecision{
		Matched:        true,
		Command:        newRetiredCmd(),
		EffectiveState: legacyretirement.WindowClosed,
		ShowNotice:     false,
		ServeNL:        false,
		Outcome:        legacyretirement.OutcomeClosedUnknown,
		WindowID:       testFacadeWindowID,
	}}
	facade, executor, audit := newPolicyFacade(t, pol)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u4", Transport: "telegram", Text: "/weather closed",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.Status != contracts.StatusUnavailable {
		t.Errorf("Status = %q, want %q", resp.Status, contracts.StatusUnavailable)
	}
	if resp.ErrorCause != contracts.ErrorCause("retired_command_closed") {
		t.Errorf("ErrorCause = %q, want retired_command_closed", resp.ErrorCause)
	}
	if resp.Body != "weather in Barcelona tomorrow" {
		t.Errorf("Body = %q, want canonical unknown-command body from ClosedResponseFor", resp.Body)
	}
	if resp.LegacyRetirementNotice != nil {
		t.Errorf("LegacyRetirementNotice = %+v; closed branch must not attach a notice", resp.LegacyRetirementNotice)
	}
	if executor.invocations != 0 {
		t.Errorf("executor.invocations = %d, want 0 (closed branch must short-circuit before routing)", executor.invocations)
	}
	if len(audit.snapshot()) != 1 {
		t.Errorf("audit turns = %d, want 1 (closed-window short-circuit must still audit)", len(audit.snapshot()))
	}
}

// Branch 5 — !Matched: passthrough, no NoticePayload, normal routing
// proceeds unchanged.
func TestFacadeLegacyRetirement_NoMatchPassthrough(t *testing.T) {
	t.Parallel()

	pol := &stubLegacyPolicy{decision: legacyretirement.RetirementDecision{
		Matched:        false,
		EffectiveState: legacyretirement.WindowOpen,
		ServeNL:        true,
		Outcome:        legacyretirement.OutcomeNotRetiredPassthrough,
	}}
	facade, _, _ := newPolicyFacade(t, pol)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u5", Transport: "web", Text: "remind me to call mom",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if pol.calls != 1 {
		t.Errorf("Policy.Handle calls = %d, want 1", pol.calls)
	}
	if resp.LegacyRetirementNotice != nil {
		t.Errorf("LegacyRetirementNotice = %+v; passthrough branch must not attach a notice", resp.LegacyRetirementNotice)
	}
	if !resp.CaptureRoute {
		t.Error("CaptureRoute = false; expected normal routing pipeline to execute")
	}
}

// Containment — nil Policy is the supported no-op state. Existing
// facade flows MUST be untouched (no Policy.Handle calls possible, no
// NoticePayload ever attached).
func TestFacadeLegacyRetirement_NilPolicyIsPassthrough(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now) // Policy unset
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{}}
	manifest := newTestManifest(map[string]manifestEntry{})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	router := &stubRouter{ok: false}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u6", Transport: "telegram", Text: "/weather nilpath",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.LegacyRetirementNotice != nil {
		t.Errorf("LegacyRetirementNotice = %+v; nil Policy must never attach a notice", resp.LegacyRetirementNotice)
	}
}

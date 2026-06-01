// policy_test.go — spec 075 SCOPE-2 unit tests.
//
// Covers SCN-075-A01..A03 and A09 at the Policy decision boundary,
// using the in-memory ledger so the tests run without a live stack.
// SQLNoticeLedger is exercised by the integration test in
// tests/integration/assistant/legacy_retirement_notice_test.go.
package legacyretirement

import (
	"context"
	"testing"
	"time"
)

const (
	testWindowID  = "2026-05-retirement"
	testHMACKey   = "spec-075-scope-2-unit-key"
	cmdWeather    = "/weather"
	cmdRemind     = "/remind"
	noticeWeather = "I now answer weather questions in plain English."
	noticeRemind  = "I now create reminders from plain English."
	closedWeather = "I do not use /weather anymore."
	closedRemind  = "I do not use /remind anymore."
)

func newTestCatalog(t *testing.T) RetiredCommandCatalog {
	t.Helper()
	cat, err := NewConfigCatalog(CatalogConfig{
		NoticeCopyPerCommand: map[string]string{
			cmdWeather: noticeWeather,
			cmdRemind:  noticeRemind,
		},
		PostWindowUnknownResponseCopy: map[string]string{
			cmdWeather: closedWeather,
			cmdRemind:  closedRemind,
		},
	})
	if err != nil {
		t.Fatalf("NewConfigCatalog: %v", err)
	}
	return cat
}

type noopTelemetry struct {
	records []struct {
		command, bucket string
		outcome         RetirementOutcome
	}
}

func (n *noopTelemetry) Record(command, bucket string, outcome RetirementOutcome) {
	n.records = append(n.records, struct {
		command, bucket string
		outcome         RetirementOutcome
	}{command, bucket, outcome})
}

func newTestPolicy(t *testing.T, state string, paused bool) (Policy, *InMemoryNoticeLedger, *noopTelemetry) {
	t.Helper()
	cat := newTestCatalog(t)
	ledger := NewInMemoryNoticeLedger()
	resolver, err := NewWindowStateResolver(SSTStateConfig{
		WindowID:    testWindowID,
		WindowState: state,
	}, NewStaticPauseStateReader(paused))
	if err != nil {
		t.Fatalf("NewWindowStateResolver: %v", err)
	}
	hasher, err := NewUserBucketHasher(testHMACKey)
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	tel := &noopTelemetry{}
	pol, err := NewPolicy(PolicyConfig{
		Catalog:       cat,
		Ledger:        ledger,
		StateResolver: resolver,
		Telemetry:     tel,
		BucketHasher:  hasher,
		WindowID:      testWindowID,
		Clock:         func() time.Time { return time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	return pol, ledger, tel
}

// TestPolicy_SCN_A01_FirstInvocation_ShowsNoticeAndServesNL covers
// SCN-075-A01: the first /weather invocation in an open window emits
// the notice, marks the ledger, and signals NL serving.
func TestPolicy_SCN_A01_FirstInvocation_ShowsNoticeAndServesNL(t *testing.T) {
	pol, ledger, tel := newTestPolicy(t, "open", false)
	ctx := context.Background()
	turn := AssistantTurn{UserID: "user-1", Transport: "telegram", RawText: "/weather barcelona"}

	d, err := pol.Handle(ctx, turn)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !d.Matched {
		t.Fatal("expected Matched=true for /weather")
	}
	if !d.ShowNotice {
		t.Fatal("first invocation must ShowNotice=true")
	}
	if !d.ServeNL {
		t.Fatal("open window with confident NL path must ServeNL=true")
	}
	if d.Outcome != OutcomeNoticeAndServed {
		t.Errorf("Outcome=%q, want %q", d.Outcome, OutcomeNoticeAndServed)
	}
	if d.EffectiveState != WindowOpen {
		t.Errorf("EffectiveState=%q, want %q", d.EffectiveState, WindowOpen)
	}
	got, ok, err := ledger.Get(ctx, "user-1", cmdWeather, testWindowID)
	if err != nil || !ok {
		t.Fatalf("ledger.Get: ok=%v err=%v", ok, err)
	}
	if got.NoticeCount != 1 {
		t.Errorf("NoticeCount=%d, want 1", got.NoticeCount)
	}
	if len(tel.records) != 1 {
		t.Fatalf("expected 1 telemetry record, got %d", len(tel.records))
	}
	if tel.records[0].bucket == "user-1" {
		t.Fatal("telemetry emitted raw user id as bucket — privacy violation")
	}
}

// TestPolicy_SCN_A02_RepeatInvocation_SuppressesNotice covers
// SCN-075-A02: the second invocation of the same retired command
// MUST NOT re-notify but MUST still serve via NL.
func TestPolicy_SCN_A02_RepeatInvocation_SuppressesNotice(t *testing.T) {
	pol, _, _ := newTestPolicy(t, "open", false)
	ctx := context.Background()
	turn := AssistantTurn{UserID: "user-1", Transport: "telegram", RawText: "/weather barcelona"}

	if _, err := pol.Handle(ctx, turn); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	d, err := pol.Handle(ctx, turn)
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if d.ShowNotice {
		t.Fatal("second invocation must NOT ShowNotice")
	}
	if !d.ServeNL {
		t.Fatal("second invocation must still ServeNL")
	}
	if d.Outcome != OutcomeServedNoNotice {
		t.Errorf("Outcome=%q, want %q", d.Outcome, OutcomeServedNoNotice)
	}
}

// TestPolicy_SCN_A03_PerCommandIndependence covers SCN-075-A03: a
// notice shown for /weather MUST NOT suppress the first /remind
// notice. Per-command ledger keying is the structural guarantee.
func TestPolicy_SCN_A03_PerCommandIndependence(t *testing.T) {
	pol, _, _ := newTestPolicy(t, "open", false)
	ctx := context.Background()

	if _, err := pol.Handle(ctx, AssistantTurn{UserID: "user-1", Transport: "telegram", RawText: "/weather barcelona"}); err != nil {
		t.Fatalf("weather Handle: %v", err)
	}
	d, err := pol.Handle(ctx, AssistantTurn{UserID: "user-1", Transport: "telegram", RawText: "/remind tomorrow at 9"})
	if err != nil {
		t.Fatalf("remind Handle: %v", err)
	}
	if !d.ShowNotice {
		t.Fatal("/remind first invocation must ShowNotice; per-command independence broken")
	}
	if d.Command.Command != cmdRemind {
		t.Errorf("Command=%q, want %q", d.Command.Command, cmdRemind)
	}
}

// TestPolicy_SCN_A09_CrossTransportDedup covers SCN-075-A09: the
// dedup ledger is keyed on (user, command, window) — NOT on
// transport — so a notice shown via Telegram MUST suppress the
// notice when the same user invokes from web.
func TestPolicy_SCN_A09_CrossTransportDedup(t *testing.T) {
	pol, _, _ := newTestPolicy(t, "open", false)
	ctx := context.Background()

	d1, err := pol.Handle(ctx, AssistantTurn{UserID: "user-1", Transport: "telegram", RawText: "/weather barcelona"})
	if err != nil {
		t.Fatalf("telegram Handle: %v", err)
	}
	if !d1.ShowNotice {
		t.Fatal("first invocation on telegram must show notice")
	}

	d2, err := pol.Handle(ctx, AssistantTurn{UserID: "user-1", Transport: "web", RawText: "/weather barcelona"})
	if err != nil {
		t.Fatalf("web Handle: %v", err)
	}
	if d2.ShowNotice {
		t.Fatal("web invocation must NOT re-notify; cross-transport dedup broken")
	}
	if !d2.ServeNL {
		t.Fatal("web invocation must still ServeNL")
	}

	// Adversarial: a different user must STILL get the notice — the
	// ledger key includes user_id.
	d3, err := pol.Handle(ctx, AssistantTurn{UserID: "user-2", Transport: "web", RawText: "/weather barcelona"})
	if err != nil {
		t.Fatalf("user-2 web Handle: %v", err)
	}
	if !d3.ShowNotice {
		t.Fatal("user-2's first invocation must show notice; per-user keying broken")
	}
}

// TestPolicy_NotRetiredPassthrough proves the policy does not
// interfere with non-retired tokens (no Matched, no notice).
func TestPolicy_NotRetiredPassthrough(t *testing.T) {
	pol, _, _ := newTestPolicy(t, "open", false)
	ctx := context.Background()

	cases := []string{
		"/help",
		"hello there",
		"",
		"   ",
	}
	for _, raw := range cases {
		d, err := pol.Handle(ctx, AssistantTurn{UserID: "u", RawText: raw})
		if err != nil {
			t.Fatalf("Handle(%q): %v", raw, err)
		}
		if d.Matched {
			t.Errorf("Handle(%q) Matched=true; expected passthrough", raw)
		}
		if !d.ServeNL {
			t.Errorf("Handle(%q) ServeNL=false; passthrough must keep NL serving", raw)
		}
	}
}

// TestPolicy_PausedWindow_SuppressesNoticeButServes covers the
// paused branch: notices are suppressed and NL serving continues
// per spec 066 safety mode.
func TestPolicy_PausedWindow_SuppressesNoticeButServes(t *testing.T) {
	pol, ledger, _ := newTestPolicy(t, "open", true)
	ctx := context.Background()

	d, err := pol.Handle(ctx, AssistantTurn{UserID: "user-1", Transport: "telegram", RawText: "/weather x"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.ShowNotice {
		t.Fatal("paused window must suppress notices")
	}
	if !d.ServeNL {
		t.Fatal("paused window must continue ServeNL")
	}
	if d.EffectiveState != WindowPaused {
		t.Errorf("EffectiveState=%q, want %q", d.EffectiveState, WindowPaused)
	}
	if _, ok, _ := ledger.Get(ctx, "user-1", cmdWeather, testWindowID); ok {
		t.Fatal("paused branch must NOT write to ledger")
	}
}

// TestPolicy_ClosedWindow_RejectsLegacyServe covers SCN-075-A07
// boundary (Scope 5 owns the canonical response, but Scope 2 must
// already refuse ServeNL=true so the legacy handler cannot run).
func TestPolicy_ClosedWindow_RejectsLegacyServe(t *testing.T) {
	pol, _, _ := newTestPolicy(t, "closed", false)
	ctx := context.Background()
	d, err := pol.Handle(ctx, AssistantTurn{UserID: "user-1", RawText: "/weather x"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.ServeNL {
		t.Fatal("closed window must NOT ServeNL")
	}
	if d.EffectiveState != WindowClosed {
		t.Errorf("EffectiveState=%q, want %q", d.EffectiveState, WindowClosed)
	}
	if d.Outcome != OutcomeClosedUnknown {
		t.Errorf("Outcome=%q, want %q", d.Outcome, OutcomeClosedUnknown)
	}
}

// TestClassifyToken_StripBotSuffix covers Telegram /cmd@botname
// suffix handling — the catalog must match "/weather" even when the
// transport delivers "/weather@smackerelbot".
func TestClassifyToken_StripBotSuffix(t *testing.T) {
	cases := map[string]string{
		"/weather@smackerelbot barcelona": "/weather",
		"/remind@bot tomorrow":            "/remind",
		"  /weather barcelona  ":          "/weather",
		"hi /weather":                     "",
		"":                                "",
	}
	for raw, want := range cases {
		if got := ClassifyToken(raw); got != want {
			t.Errorf("ClassifyToken(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestConfigCatalog_RejectsMissingClosedCopy proves the SST
// coverage rule: every notice command MUST have a matching
// closed-window response. Adversarial — a regression that
// silently dropped the check would let this test pass.
func TestConfigCatalog_RejectsMissingClosedCopy(t *testing.T) {
	_, err := NewConfigCatalog(CatalogConfig{
		NoticeCopyPerCommand: map[string]string{cmdWeather: noticeWeather},
		PostWindowUnknownResponseCopy: map[string]string{
			cmdRemind: closedRemind, // mismatched key
		},
	})
	if err == nil {
		t.Fatal("NewConfigCatalog must reject mismatched copy maps")
	}
}

// TestNewPolicy_NoticeFor proves NoticePayload populates from the
// catalog and is empty when ShowNotice=false.
func TestNewPolicy_NoticeFor(t *testing.T) {
	pol, _, _ := newTestPolicy(t, "open", false)
	ctx := context.Background()
	d, err := pol.Handle(ctx, AssistantTurn{UserID: "u", RawText: "/weather x"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	pi, ok := pol.(*policyImpl)
	if !ok {
		t.Fatal("pol is not *policyImpl")
	}
	np := pi.NoticeFor(d)
	if np.Command != cmdWeather || np.NoticeCopy != noticeWeather || np.WindowID != testWindowID {
		t.Errorf("NoticeFor mismatch: %+v", np)
	}
	d2, _ := pol.Handle(ctx, AssistantTurn{UserID: "u", RawText: "/weather y"})
	if pi.NoticeFor(d2) != (NoticePayload{}) {
		t.Error("NoticeFor on suppressed notice must be zero value")
	}
}

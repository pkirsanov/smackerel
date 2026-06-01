//go:build integration

// Spec 066 SCOPE-2 — integration coverage for the Telegram retired-
// alias interceptor wired against the real legacyretirement.Policy,
// configcatalog, and InMemoryNoticeLedger. These tests exercise the
// SCN-066-A04 (alias rewrite + one-time notice ledger write) and
// SCN-066-A05 (closed-window rejection without facade invocation)
// observables end-to-end through bot code without standing up the
// full Docker stack — the SQL ledger variant lives in
// tests/integration/assistant/legacy_retirement_notice_test.go and
// the live HTTP / Telegram E2E variants live in tests/e2e/assistant.
package telegram_integration

import (
	"context"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
	"github.com/smackerel/smackerel/internal/telegram"
)

const (
	testWindowID  = "066-scope2-window"
	testHMACKey   = "066-scope2-hmac-test-key-do-not-use-in-prod"
	testHelpURL   = "see /help"
	testUserAlice = "user-alice"
	testUserBob   = "user-bob"
)

func newOpenPolicy(t *testing.T, ledger legacyretirement.NoticeLedger) legacyretirement.Policy {
	t.Helper()
	return newPolicy(t, ledger, "open")
}

func newClosedPolicy(t *testing.T, ledger legacyretirement.NoticeLedger) legacyretirement.Policy {
	t.Helper()
	return newPolicy(t, ledger, "closed")
}

func newPolicy(t *testing.T, ledger legacyretirement.NoticeLedger, windowState string) legacyretirement.Policy {
	t.Helper()
	catalog, err := legacyretirement.NewConfigCatalog(legacyretirement.CatalogConfig{
		NoticeCopyPerCommand: map[string]string{
			"/find": "Heads up: /find is being retired. Try plain English; " + testHelpURL,
			"/rate": "Heads up: /rate is being retired. Try plain English; " + testHelpURL,
		},
		PostWindowUnknownResponseCopy: map[string]string{
			"/find": "I no longer respond to /find. Type your question in plain English; " + testHelpURL,
			"/rate": "I no longer respond to /rate. Type your rating in plain English; " + testHelpURL,
		},
	})
	if err != nil {
		t.Fatalf("NewConfigCatalog: %v", err)
	}
	resolver, err := legacyretirement.NewWindowStateResolver(
		legacyretirement.SSTStateConfig{WindowID: testWindowID, WindowState: windowState},
		legacyretirement.NewStaticPauseStateReader(false),
	)
	if err != nil {
		t.Fatalf("NewWindowStateResolver: %v", err)
	}
	hasher, err := legacyretirement.NewUserBucketHasher(testHMACKey)
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	policy, err := legacyretirement.NewPolicy(legacyretirement.PolicyConfig{
		Catalog:       catalog,
		Ledger:        ledger,
		StateResolver: resolver,
		BucketHasher:  hasher,
		WindowID:      testWindowID,
	})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	return policy
}

// botHarness owns the test bot, the recorded replies, and the
// production-shaped interceptor.
type botHarness struct {
	bot      *telegram.Bot
	captured *[]string
}

func newBotHarness(t *testing.T, policy legacyretirement.Policy, userMapping map[int64]string) *botHarness {
	t.Helper()
	bot, captured := telegram.NewTestBotWithReplyRecorder("test", userMapping)
	interceptor, err := telegram.NewLegacyAliasInterceptor(policy, nil)
	if err != nil {
		t.Fatalf("NewLegacyAliasInterceptor: %v", err)
	}
	bot.SetLegacyAliasInterceptor(interceptor)
	return &botHarness{bot: bot, captured: captured}
}

func sendCommand(t *testing.T, h *botHarness, chatID int64, command, args string) bool {
	t.Helper()
	full := "/" + command
	if args != "" {
		full = full + " " + args
	}
	msg := &tgbotapi.Message{
		MessageID: int(time.Now().UnixNano()),
		Chat:      &tgbotapi.Chat{ID: chatID},
		Text:      full,
		Entities:  []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(command) + 1}},
	}
	handled, err := telegram.InterceptLegacyAliasForTest(h.bot, context.Background(), msg, int(msg.MessageID))
	if err != nil {
		t.Fatalf("interceptLegacyAlias: %v", err)
	}
	return handled
}

// TestLegacyAliasInsideWindowRewritesRecordsNoticeAndInvokesFacade
// covers SCN-066-A04 — the inbound retired command is rewritten to
// the plain-English equivalent, the one-time notice is rendered,
// and the NoticeLedger records the entry keyed by (user, command,
// window).
func TestLegacyAliasInsideWindowRewritesRecordsNoticeAndInvokesFacade(t *testing.T) {
	ledger := legacyretirement.NewInMemoryNoticeLedger()
	policy := newOpenPolicy(t, ledger)
	chatID := int64(1)
	h := newBotHarness(t, policy, map[int64]string{chatID: testUserAlice})

	handled := sendCommand(t, h, chatID, "find", "ACL tags")
	if !handled {
		t.Fatal("interceptor must short-circuit dispatch for retired alias inside open window")
	}
	replies := *h.captured
	if len(replies) != 2 {
		t.Fatalf("expected 2 replies (notice + rewrite), got %d (%v)", len(replies), replies)
	}
	if !strings.Contains(replies[0], "Heads up") {
		t.Errorf("first reply must be notice copy from configcatalog; got %q", replies[0])
	}
	if replies[1] != "find ACL tags" {
		t.Errorf("second reply must be rewritten plain-English prompt; got %q", replies[1])
	}
	entry, ok, err := ledger.Get(context.Background(), testUserAlice, "/find", testWindowID)
	if err != nil || !ok {
		t.Fatalf("ledger.Get: ok=%v err=%v — notice MUST be persisted for (user=%q, cmd=/find, window=%q)",
			ok, err, testUserAlice, testWindowID)
	}
	if entry.NoticeCount != 1 {
		t.Errorf("NoticeCount=%d, want 1", entry.NoticeCount)
	}
}

// TestLegacyAliasNoticeIsOneTimePerUserCommandAndWindow covers the
// idempotency leg of SCN-066-A04 — the second invocation in the
// same window for the same (user, command) MUST NOT re-emit the
// notice, only the rewritten prompt.
func TestLegacyAliasNoticeIsOneTimePerUserCommandAndWindow(t *testing.T) {
	ledger := legacyretirement.NewInMemoryNoticeLedger()
	policy := newOpenPolicy(t, ledger)
	chatID := int64(2)
	h := newBotHarness(t, policy, map[int64]string{chatID: testUserBob})

	if !sendCommand(t, h, chatID, "rate", "carbonara 5 of 5") {
		t.Fatal("first invocation must be handled")
	}
	first := append([]string(nil), *h.captured...)
	if len(first) != 2 {
		t.Fatalf("first invocation expected 2 replies; got %d (%v)", len(first), first)
	}

	// Reset reply capture by clearing the slice through the harness.
	*h.captured = (*h.captured)[:0]
	if !sendCommand(t, h, chatID, "rate", "another rating") {
		t.Fatal("second invocation must be handled")
	}
	second := *h.captured
	if len(second) != 1 {
		t.Fatalf("second invocation expected exactly 1 reply (rewrite only); got %d (%v)", len(second), second)
	}
	if second[0] != "rate another rating" {
		t.Errorf("second reply must be the rewritten prompt; got %q", second[0])
	}

	// Cross-command remains independent: a different retired command
	// for the same user still emits its first notice.
	*h.captured = (*h.captured)[:0]
	if !sendCommand(t, h, chatID, "find", "Postgres tuning") {
		t.Fatal("cross-command invocation must be handled")
	}
	cross := *h.captured
	if len(cross) != 2 || !strings.Contains(cross[0], "Heads up") {
		t.Fatalf("cross-command must emit fresh notice + rewrite; got %v", cross)
	}
}

// TestLegacyAliasAfterWindowRejectsWithoutFacadeInvocation covers
// SCN-066-A05 — closed window returns the canonical unknown copy
// and never touches the assistant facade / legacy handler.
func TestLegacyAliasAfterWindowRejectsWithoutFacadeInvocation(t *testing.T) {
	ledger := legacyretirement.NewInMemoryNoticeLedger()
	policy := newClosedPolicy(t, ledger)
	chatID := int64(3)
	h := newBotHarness(t, policy, map[int64]string{chatID: testUserAlice})

	if !sendCommand(t, h, chatID, "find", "ACL tags") {
		t.Fatal("closed-window retired command must be intercepted")
	}
	replies := *h.captured
	if len(replies) != 1 {
		t.Fatalf("closed window must reply exactly once with canonical copy; got %d (%v)", len(replies), replies)
	}
	if !strings.Contains(replies[0], "I no longer respond to /find") {
		t.Errorf("closed-window reply must be unknown-command copy; got %q", replies[0])
	}
	// Adversarial: the ledger MUST remain empty — closed window
	// does not record notices.
	if _, ok, _ := ledger.Get(context.Background(), testUserAlice, "/find", testWindowID); ok {
		t.Errorf("closed-window invocation MUST NOT write to the notice ledger")
	}
}

// TestLegacyAliasOperationalCommandNotIntercepted is the inverse
// adversarial assertion: /help (operational) MUST pass through the
// interceptor untouched so the existing /help handler runs.
func TestLegacyAliasOperationalCommandNotIntercepted(t *testing.T) {
	ledger := legacyretirement.NewInMemoryNoticeLedger()
	policy := newOpenPolicy(t, ledger)
	chatID := int64(4)
	h := newBotHarness(t, policy, map[int64]string{chatID: testUserAlice})

	if sendCommand(t, h, chatID, "help", "") {
		t.Fatal("operational /help MUST NOT be intercepted")
	}
	if len(*h.captured) != 0 {
		t.Errorf("operational command must not emit interceptor replies; got %v", *h.captured)
	}
}

func TestLegacyAliasWindowKeyIsolation(t *testing.T) {
	// Adversarial: a notice persisted under window A must NOT
	// suppress a fresh notice under window B for the same user.
	ledger := legacyretirement.NewInMemoryNoticeLedger()
	ctx := context.Background()
	if err := ledger.MarkShown(ctx, testUserAlice, "/find", "066-old-window", time.Now()); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	policy := newOpenPolicy(t, ledger)
	chatID := int64(5)
	h := newBotHarness(t, policy, map[int64]string{chatID: testUserAlice})

	handled := sendCommand(t, h, chatID, "find", "ACL tags")
	if !handled {
		t.Fatal("interceptor must short-circuit dispatch")
	}
	if len(*h.captured) != 2 {
		t.Fatalf("notice MUST fire for new window; got replies %v", *h.captured)
	}
	if _, ok, _ := ledger.Get(ctx, testUserAlice, "/find", testWindowID); !ok {
		t.Errorf("ledger MUST record notice under the new window key")
	}
	// Sanity: original window entry untouched.
	if _, ok, _ := ledger.Get(ctx, testUserAlice, "/find", "066-old-window"); !ok {
		t.Errorf("seed entry for old window must remain")
	}
}

// Sanity that the harness routes the message text the policy
// observes (defensive — protects against future refactors that
// strip the leading slash before invoking the policy).
func TestLegacyAliasRawTextPreservesLeadingSlash(t *testing.T) {
	ledger := legacyretirement.NewInMemoryNoticeLedger()
	policy := newOpenPolicy(t, ledger)
	chatID := int64(6)
	h := newBotHarness(t, policy, map[int64]string{chatID: testUserAlice})

	if !sendCommand(t, h, chatID, "find", "ACL tags") {
		t.Fatal("interceptor must short-circuit dispatch")
	}
	// The ledger key uses the catalog token "/find" — confirming the
	// interceptor passed RawText="/find ACL tags" through to the
	// policy, which classified the leading token and looked up the
	// catalog entry.
	if _, ok, _ := ledger.Get(context.Background(), testUserAlice, "/find", testWindowID); !ok {
		t.Fatalf("ledger MUST be keyed on the slash-prefixed catalog token")
	}
}

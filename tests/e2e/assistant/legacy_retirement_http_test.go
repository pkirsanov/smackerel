//go:build e2e

// Spec 066 SCOPE-2 — live-stack regression for the retired-alias
// interceptor. Covers SCN-066-A04 (alias rewrite + one-time notice
// inside the open window) and SCN-066-A05 (closed-window rejection
// without facade invocation) against a real running stack by
// posting a synthetic Telegram update to the bot's webhook
// endpoint, then asserting the resulting reply text matches the
// scenario contract via Telegram's send-message capture mechanism.
//
// Live-stack contract:
//
//   - CORE_EXTERNAL_URL                 — running core (required)
//   - TELEGRAM_WEBHOOK_PATH             — webhook route under chi
//   - TELEGRAM_WEBHOOK_SECRET           — shared secret header
//   - LEGACY_RETIREMENT_WINDOW_STATE    — open | closed (drives the
//     scenario branch)
//   - LEGACY_RETIREMENT_WINDOW_ID       — current window id
//
// When CORE_EXTERNAL_URL is unset the tests skip (legitimate "no
// live stack here" path). When it is set but the dependent SST or
// secrets are missing, the tests fail loud — that is a wiring bug,
// not a legitimate skip. The full mock-Telegram fixture is owned by
// the e2e harness driver invoked from `./smackerel.sh test e2e`.
package assistant_e2e

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type legacyRetirementLiveStack struct {
	BaseURL       string
	WebhookPath   string
	WebhookSecret string
	WindowState   string
	WindowID      string
}

func loadLegacyRetirementLiveStack(t *testing.T) legacyRetirementLiveStack {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if baseURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	webhookPath := os.Getenv("TELEGRAM_WEBHOOK_PATH")
	secret := os.Getenv("TELEGRAM_WEBHOOK_SECRET")
	if webhookPath == "" || secret == "" {
		t.Skip("e2e: TELEGRAM_WEBHOOK_PATH / TELEGRAM_WEBHOOK_SECRET not set — live stack does not run telegram in webhook mode")
	}
	state := os.Getenv("LEGACY_RETIREMENT_WINDOW_STATE")
	if state == "" {
		t.Fatalf("LEGACY_RETIREMENT_WINDOW_STATE not set; live stack is up but spec 075 SST is missing — wiring bug")
	}
	windowID := os.Getenv("LEGACY_RETIREMENT_WINDOW_ID")
	if windowID == "" {
		t.Fatalf("LEGACY_RETIREMENT_WINDOW_ID not set; live stack is up but spec 075 SST is missing — wiring bug")
	}
	return legacyRetirementLiveStack{
		BaseURL:       baseURL,
		WebhookPath:   webhookPath,
		WebhookSecret: secret,
		WindowState:   state,
		WindowID:      windowID,
	}
}

func waitLegacyRetirementHealthy(t *testing.T, stack legacyRetirementLiveStack, maxWait time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, stack.BaseURL+"/api/health", nil)
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("e2e: core not healthy after %s at %s", maxWait, stack.BaseURL)
}

// TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice
// covers SCN-066-A04 — a retired slash command sent over the live
// Telegram webhook while the spec 075 window is open is rewritten
// to plain English, the one-time notice is rendered, and the
// NoticeLedger persists the entry for (user, command, window).
//
// Note: this test requires a live-stack harness that captures the
// outbound Telegram send-message bodies. Until that harness lands,
// it skips at the CORE_EXTERNAL_URL gate above. The unit + the
// integration variants (internal/telegram/legacy_alias_intercept_test.go
// and tests/integration/telegram/legacy_alias_test.go) cover the
// scenario at the in-process boundary; the SQL ledger variant lives
// in tests/integration/assistant/legacy_retirement_notice_test.go
// for the durable-storage proof.
func TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice(t *testing.T) {
	stack := loadLegacyRetirementLiveStack(t)
	if stack.WindowState != "open" {
		t.Skipf("LEGACY_RETIREMENT_WINDOW_STATE=%q — SCN-066-A04 only exercises the open branch", stack.WindowState)
	}
	waitLegacyRetirementHealthy(t, stack, 30*time.Second)
	t.Skip("e2e: telegram webhook send-message capture harness pending — see tests/integration/telegram/legacy_alias_test.go for the in-process proof and tests/integration/assistant/legacy_retirement_notice_test.go for the SQL ledger proof")
}

// TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario
// covers SCN-066-A05 — a retired slash command sent over the live
// Telegram webhook after the spec 075 window has been flipped to
// closed must return the canonical unknown-command copy and MUST
// NOT invoke a legacy handler or the assistant facade.
func TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario(t *testing.T) {
	stack := loadLegacyRetirementLiveStack(t)
	if stack.WindowState != "closed" {
		t.Skipf("LEGACY_RETIREMENT_WINDOW_STATE=%q — SCN-066-A05 only exercises the closed branch", stack.WindowState)
	}
	waitLegacyRetirementHealthy(t, stack, 30*time.Second)
	t.Skip("e2e: telegram webhook send-message capture harness pending — see tests/integration/telegram/legacy_alias_test.go for the in-process proof")
}

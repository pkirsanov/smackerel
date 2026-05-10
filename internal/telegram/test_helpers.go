// Spec 044 Scope 03 — Test helpers exposed to external test packages
// (e.g. tests/integration). These constructors set ONLY the fields the
// per-user PASETO claim-binding chain consults; they intentionally do
// NOT wire the Telegram API client, the assembler, or any other
// runtime surface. Callers that need a fully-wired bot must use the
// production NewBot constructor.
//
// Why a separate file (no build tag)?
//
//   - tests/integration is package `integration` (not `telegram`), so
//     it cannot reach unexported fields like Bot.environment or
//     Bot.userMapping.
//   - Putting the helper in a `_test.go` file would make it invisible
//     to external test packages.
//   - Gating with `//go:build test` would force callers to compile with
//     a special tag, which the integration runner does not set.
//
// The helper is documented as test-only via the function name suffix
// (`ForTest`) and the `Test` prefix on the type name.
package telegram

import "net/http"

// NewBotForTest constructs a minimal *Bot whose environment and
// userMapping fields are set so callers can exercise:
//
//   - Bot.resolveActorUserID(chatID) — the chat → user resolver
//   - PerUserTokenMinter.MintForChat(chatID) — the helper that calls
//     resolveActorUserID then mints a per-user PASETO
//
// All other fields stay at their zero values; callers MUST NOT invoke
// methods that touch the Telegram API client, the assembler, or the
// background polling loop.
func NewBotForTest(environment string, userMapping map[int64]string) *Bot {
	return &Bot{
		environment: environment,
		userMapping: userMapping,
	}
}

// SetSharedAuthTokenForTest sets the legacy shared bearer that
// `bearerForChat` falls back to when no PerUserTokenMinter is wired
// (or when a dev/test chat is unmapped). External integration tests
// use this to plant a sentinel that proves the F02 wiring did NOT
// silently fall through to the legacy path.
//
// Test-only: do not call from production code.
func (b *Bot) SetSharedAuthTokenForTest(token string) {
	b.authToken = token
}

// SetBearerHeaderForTest exposes the unexported `setBearerHeader`
// helper to external integration tests so they can prove the F02
// wiring observationally (i.e. that an outbound request prepared the
// way the live Telegram bridge prepares it carries the per-user
// PASETO bearer in production and the shared legacy bearer in
// dev/test).
//
// Test-only: do not call from production code. Production callers
// inside the telegram package use `setBearerHeader` directly.
func (b *Bot) SetBearerHeaderForTest(req *http.Request, chatID int64) error {
	return b.setBearerHeader(req, chatID)
}

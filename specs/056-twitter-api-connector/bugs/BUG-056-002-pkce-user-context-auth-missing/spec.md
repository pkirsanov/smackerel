# Spec: [BUG-056-002] Shipped Twitter auth matches spec 056's user-context requirement (or scope + claims corrected)

## Problem Statement
Spec 056 mandates User-Context OAuth 2.0 PKCE for the user-owned endpoints (`/2/users/me`, `/2/users/:id/bookmarks`, `/2/users/:id/liked_tweets`) and states App-Only bearer is insufficient for them (spec.md:225 NC-1; design.md:131-133). The shipped connector implements no PKCE flow and uses a single static App-Only bearer for all four endpoints (`api.go:141`), so bookmarks and likes cannot be retrieved against the real API (403). report.md:7 falsely claims PKCE was delivered. Separately, R-016's `x-rate-limit-remaining` gauge was never implemented.

## Outcome Contract
**Intent:** The connector's authentication for each Twitter v2 endpoint MUST match what that endpoint actually requires, AND spec 056's reported capability MUST match the shipped reality (no false PKCE-delivered claim). Either user-context auth is delivered for the user-owned endpoints, OR those endpoints are de-scoped and the claims corrected — a maintainer decision (design.md Q1).
**Success Signal:** For whichever resolution is chosen, (path A) bookmarks/likes/users-me are fetched with a user-context OAuth 2.0 token obtained via PKCE and refreshed on expiry, with an adversarial test proving App-Only on those endpoints is rejected; OR (path B) the connector no longer advertises bookmarks/likes/users-me and spec 056's spec/design/report state App-Only-public-endpoints-only. In both paths, `report.md` contains no unfulfilled PKCE-delivery claim.
**Hard Constraints:** No secret/token value is ever logged (existing log-scan contract preserved); refresh tokens, if persisted, are stored via the SST/encrypted config path with no plaintext on any build surface; App-Only bearer remains the auth for the genuinely-public `/2/users/:id/tweets` and `/2/users/:id/mentions`; the smackerel-no-defaults fail-loud policy is preserved (missing user-context credentials fail loudly, never silently fall back to App-Only for a user-owned endpoint).
**Failure Condition:** Any shipped state where a user-owned endpoint is requested with an App-Only bearer while still being advertised as supported, OR any spec 056 artifact that claims PKCE delivery without PKCE actually shipping.

## Goals
- The auth mode used per endpoint matches the endpoint's real requirement (user-context for user-owned, App-Only for public).
- spec 056's claims (spec.md/design.md/report.md) describe only what actually ships.
- `x-rate-limit-remaining` is exposed as a gauge updated after each API call (R-016).
- An adversarial regression test fails if a user-owned endpoint is ever served by an App-Only bearer.

## Non-Goals
- Implementing `/2/tweets/search/recent` (deferred by NC-2; out of scope).
- Changing the public-endpoint auth (`tweets`/`mentions` stay App-Only).
- Implementing the fix in this packet (deferred — product decision pending, design.md Q1).
- Editing any parent spec 056 artifact in THIS packet (create-only; the claim correction is a delivery-pass DoD item).

## Requirements
- R1 (path A): A User-Context OAuth 2.0 PKCE authorization-code flow exists (`code_verifier`/`code_challenge`, `POST /2/oauth2/token`) producing a user-context access token + refresh token.
- R2 (path A): `/2/users/me`, `/2/users/:id/bookmarks`, `/2/users/:id/liked_tweets` are fetched with the user-context access token; refresh-on-401/expiry is implemented.
- R3 (path A): Refresh-token persistence uses the SST/encrypted config path; no token value is logged or written in plaintext to any build surface.
- R4 (either path): App-Only bearer is retained for `/2/users/:id/tweets` and `/2/users/:id/mentions` (no regression).
- R5 (path B alternative): If the maintainer de-scopes bookmarks/likes/users-me, the connector stops advertising them and spec 056's spec/design/report are corrected to App-Only-public-endpoints-only.
- R6 (artifact integrity): `specs/056-twitter-api-connector/report.md` no longer claims PKCE was delivered unless PKCE actually ships.
- R7 (GAP-G2): A Prometheus gauge reports `x-rate-limit-remaining` after each API call, per spec.md:111 (R-016).

## User Scenarios (Gherkin)
```gherkin
Scenario: User-owned endpoint uses a user-context token (path A)
  Given a valid user-context OAuth 2.0 access token obtained via PKCE
  When the connector fetches /2/users/:id/bookmarks
  Then the request carries the user-context access token (not the App-Only bearer)
  And the endpoint returns 200 with bookmarks

Scenario: App-Only bearer on a user-owned endpoint is rejected (adversarial)
  Given only an App-Only bearer token is available
  When the connector attempts /2/users/:id/bookmarks
  Then the connector surfaces the 403/insufficient-auth failure
  And it never silently treats App-Only as sufficient for a user-owned endpoint

Scenario: Expired user-context token is refreshed (path A)
  Given a user-context access token that has expired
  When a user-owned endpoint returns 401
  Then the connector exchanges the refresh token at POST /2/oauth2/token and retries

Scenario: Public endpoints keep App-Only (no regression)
  Given a sync of /2/users/:id/tweets
  When the connector fetches the page
  Then it uses the App-Only bearer token

Scenario: Rate-limit headroom is observable after each call (R-016)
  Given any successful Twitter API response carrying x-rate-limit-remaining
  When the call completes
  Then a Prometheus gauge reflects the remaining quota for that endpoint

Scenario: Spec 056 claims match shipped reality (artifact integrity)
  Given the chosen resolution has shipped
  When spec 056 report.md is read
  Then it contains no PKCE-delivered claim unless PKCE actually shipped
```

## Acceptance Criteria
- AC-1 (R1/R2, path A): A non-mocked or fixture-replay test shows a user-owned endpoint fetched with a user-context token acquired via the PKCE exchange.
- AC-2 (adversarial, R2): A test FAILS (red) if a user-owned endpoint is requested with an App-Only bearer — proving the exact reintroduction of this bug is caught.
- AC-3 (R3): No token value appears in logs (log-scan) and no plaintext refresh token exists on any build surface (grep clean).
- AC-4 (R4): `/2/users/:id/tweets` + `/2/users/:id/mentions` still authenticate with App-Only (preserved tests green).
- AC-5 (R6): `grep -n 'User-Context PKCE' specs/056-twitter-api-connector/report.md` reflects only delivered reality (no false claim).
- AC-6 (R7): A gauge for `x-rate-limit-remaining` is registered and updated after each API call; a test asserts it reflects the header.
- AC-7 (path B alternative): If de-scoped, the connector no longer exposes bookmarks/likes/users-me and spec 056 artifacts state App-Only-public-endpoints-only.

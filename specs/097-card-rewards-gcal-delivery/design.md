# Design — Spec 097 Card-Rewards Google Calendar Delivery

**Spec:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md)

## Context

Spec 083 built the card-rewards calendar bridge (`internal/cardrewards.CardCalendarBridge`,
`SyncRecommendations`/`SyncReEnrollments`/`SyncPeriod`/`DeleteRecommendationEvent`)
that converts persisted monthly recommendations into stable-UID calendar events
via a `CalDAVClient` (`PutEvent`/`DeleteEvent`). But no production `CalDAVClient`
was ever constructed — `wiring_cardrewards_scheduler.go` passed a `nil` bridge and
logged a "not wired yet" warning. The only Google Calendar usage in the tree is
the READ path (`internal/connector/caldav` GETs `/calendars/primary/events`); no
event-WRITE client exists.

This spec builds the missing write client and wires it.

## Components

### 1. `internal/cardrewards/gcal_client.go` — `GoogleCalendarClient`

Implements the existing `cardrewards.CalDAVClient` interface against the Google
Calendar REST API v3 for a single target calendar.

- **Idempotency (FR-097-02):** Google event ids must be base32hex (0-9a-v),
  5..1024 chars; the application UID contains hyphens, so it can't be the event
  id directly. The client derives a deterministic event id = lowercase hex of
  `sha1(uid)` (40 chars, a subset of base32hex). `PutEvent` GETs that id: 404 →
  `POST` insert; 200 → `PUT` update. The same UID always maps to the same event,
  so a re-sync updates rather than duplicates (SCN-083-H02). `DeleteEvent` treats
  404/410 as already-gone (idempotent cleanup).
- **Auth (FR-097-03):** mints a fresh access token from the operator's refresh
  credential via the OAuth2 `refresh_token` grant, cached in-process until ~60s
  before expiry (access tokens last ~1h; the sync cron is monthly).
- **Value-safety (FR-097-07):** logs nothing; error bodies from Google describe
  API errors, never the bearer token or credential.
- **Testability:** `apiBase`/`tokenURL` are overridable so the unit tests drive
  it against an httptest server emulating the token + events endpoints (no live
  Google, no internal mocks — a real HTTP boundary fake).

`GCalCredential` + `ParseGCalCredential` parse/validate the
`CARD_REWARDS_GCAL_CREDENTIALS` JSON (client_id, client_secret, refresh_token,
token_uri) fail-loud.

### 2. `internal/config/cardrewards.go` — SST surface

Adds `CalendarID` (from `CARD_REWARDS_CALENDAR_ID`) and `GCalCredentials` (from
`CARD_REWARDS_GCAL_CREDENTIALS`) to `CardRewardsConfig`. Both are fail-loud
`readString` when `calendar_sync` is true; carried (not required) when false.

### 3. Secret-key 3-mirror (Spec 052 contract)

`CARD_REWARDS_GCAL_CREDENTIALS` is a managed secret, added to all three mirrors:
`config/smackerel.yaml infrastructure.secret_keys`,
`internal/config/secret_keys.go`, and `scripts/commands/config.sh
SHELL_SECRET_KEYS`. The `bundle_secret_contract_test` enforces the three agree
and that the key emits a `__SECRET_PLACEHOLDER__` marker (not a literal) in a
production-class bundle. `CARD_REWARDS_CALENDAR_ID` is NON-secret operator config
(emitted by the knb deploy adapter from params.yaml, like `CARD_REWARDS_SOURCES`).

### 4. `cmd/core/wiring_cardrewards_scheduler.go` — wiring

When `calendar_sync` is true: `ParseGCalCredential` → `NewGoogleCalendarClient` →
`NewCardCalendarBridge(client, store, true, uidPrefix)` → passed as the pipeline's
calendar arg. A malformed credential degrades gracefully (WARN + nil bridge:
recommendations still persist to the Web UI; only delivery is skipped) so a
calendar typo cannot take down core. When `calendar_sync` is false the prior
nil-bridge behavior is preserved.

## Why a write client and not the existing read connector

`internal/connector/caldav` is a one-way *reader* (it GETs events into the
knowledge graph). It shares the "CalDAV" name but has no insert/update path and a
different concern. Building a focused write client in the cardrewards package
(next to the bridge it feeds) keeps the dependency direction clean and avoids
overloading the read connector with an unrelated write responsibility.

## Credential provenance (operator)

The operator supplies an existing Google OAuth2 installed-app credential with
calendar write scope (`.../auth/calendar`). For the home-lab deployment this is
the credential recovered from the operator's prior CCManager Render instance
(same Google project, write scope, owner access to the target "Credit cards"
calendar). It is delivered encrypted via the knb deploy adapter
(`knb/smackerel/secrets/home-lab.enc.env`, sops/age) — never committed in
plaintext, never logged.

## Deployment

Build-once: a push to smackerel `main` triggers `.github/workflows/build.yml`
(cosign keyless sign + attest + Trivy gate), producing a new signed
`smackerel-core` digest. The knb adapter applies that digest with the new config
bundle; `card_rewards.calendar_sync` flips to true on home-lab and the gcal
secret is substituted from sops at apply time. A monthly-recommend run (or the
admin "sync calendar now" trigger) then writes events to the calendar.

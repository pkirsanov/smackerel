# Spec 097 — Card-Rewards Google Calendar Delivery

**Status:** in_progress
**Workflow mode:** full-delivery · **Status ceiling:** done
**Relates to:** [083-card-rewards-companion](../083-card-rewards-companion/spec.md) (delivers its Scope-08 calendar bridge), knb spec 029 (home-lab activation)

## Problem

The Card-Rewards Companion (spec 083) generates monthly recommendations and a
calendar bridge (`internal/cardrewards.CardCalendarBridge`) that turns them into
stable-UID calendar events — but the bridge's `CalDAVClient` (the actual
event-write boundary) is **never constructed**. `cmd/core/wiring_cardrewards_scheduler.go`
passes a `nil` bridge and logs:

> "calendar_sync is enabled in SST but the production CalDAV client is not wired
> yet; recommendations are generated and persisted, calendar events are not
> delivered"

The same gap exists for the mealplan precedent. Across the whole codebase the
only Google Calendar API usage is the **read** path
(`internal/connector/caldav` GETs events from `/calendars/primary/events`).
**No event-write client exists.** So `calendar_sync: true` is inert — nothing
reaches a calendar.

The operator's primary card-rewards consumption surface is Google Calendar
(preserved intent from the absorbed CCManager app). To deliver that, smackerel
needs a real Google Calendar write client wired into the card-rewards pipeline,
fed by an operator-supplied Google OAuth credential with calendar write scope.

## Goal

Build a production Google Calendar write client implementing the existing
`cardrewards.CalDAVClient` interface, wire it into the card-rewards scheduler
when `calendar_sync` is enabled, and deliver the operator's Google OAuth
credential through the SST secret pipeline — so monthly card-rewards
recommendations are written (idempotently) to the operator's configured Google
Calendar.

## Requirements

- **FR-097-01** — A new client implements `cardrewards.CalDAVClient`
  (`PutEvent` / `DeleteEvent`) against the Google Calendar REST API v3, targeting
  a configured `calendar_id`.
- **FR-097-02** — `PutEvent` is idempotent on the stable `uid`: a re-sync of the
  same recommendation UPDATES the same event, never duplicates it. (Achieved via
  a deterministic Google event id derived from the UID: get-or-insert, then
  update.)
- **FR-097-03** — The client mints a fresh access token from the stored refresh
  credential on each run (Google access tokens expire hourly; the sync cron is
  monthly), via the standard OAuth2 refresh-token grant.
- **FR-097-04** — The Google OAuth credential (client_id, client_secret,
  refresh_token, token_uri) is delivered as a single SST-managed secret
  `CARD_REWARDS_GCAL_CREDENTIALS` (JSON), declared in all three secret-key
  mirrors. `calendar_id` is non-secret operator config (`CARD_REWARDS_CALENDAR_ID`).
- **FR-097-05** — The config loader is fail-loud: when `card_rewards.enabled` AND
  `calendar_sync` are both true, a missing/invalid `CARD_REWARDS_CALENDAR_ID` or
  `CARD_REWARDS_GCAL_CREDENTIALS` aborts core boot naming the offending key. When
  `calendar_sync` is false the credential is not required (the bridge stays nil
  and the feature persists recommendations to the Web UI only — SCN-083-H04).
- **FR-097-06** — `wiring_cardrewards_scheduler.go` constructs the real client +
  `NewCardCalendarBridge(client, store, true, uidPrefix)` and passes it to the
  pipeline when `calendar_sync` is true and the credential is present; otherwise
  the prior nil-bridge behavior is preserved.
- **FR-097-07** — No secret value is ever logged. The client logs event UIDs and
  calendar-id, never the token/credential.

## Behavior (Gherkin)

```gherkin
Scenario: Monthly recommendation is written to Google Calendar
  Given card_rewards.enabled and calendar_sync are true
  And a valid CARD_REWARDS_GCAL_CREDENTIALS + CARD_REWARDS_CALENDAR_ID are set
  When the card-rewards calendar sync runs for a period
  Then for each recommendation with a recommended card
  And the client writes a Google Calendar event with the stable UID
  And re-running the sync updates the same event (no duplicate)

Scenario: Calendar sync disabled keeps recommendations in the Web UI only
  Given calendar_sync is false
  When the recommend pipeline runs
  Then no calendar client is constructed and no event is written
  And the recommendations remain queryable for the Web UI

Scenario: Enabled-but-misconfigured fails loud
  Given card_rewards.enabled and calendar_sync are true
  And CARD_REWARDS_GCAL_CREDENTIALS is empty
  When core boots
  Then config load fails naming CARD_REWARDS_GCAL_CREDENTIALS
```

## Out of Scope

- Wiring the mealplan calendar bridge (same gap; tracked separately).
- A browser OAuth re-consent flow for card-rewards (the operator supplies an
  existing write-scoped credential via the secret).
- Two-way calendar sync / reading back manual edits.

## Release Train

Targets the `mvp` train (the `card_rewards` flag's owning train); default-off on
other trains, consistent with spec 083.

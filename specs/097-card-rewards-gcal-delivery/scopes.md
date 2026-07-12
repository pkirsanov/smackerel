# Scopes — Spec 097 Card-Rewards Google Calendar Delivery

**Feature:** [spec.md](spec.md) · **Design:** [design.md](design.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

> Single-file scope mode (2 scopes). SCOPE-01 is the Google Calendar write
> client + config + wiring (unit-proven). SCOPE-02 is the live self-hosted delivery
> proof (new image build + redeploy + a real event on the calendar).

---

## Scope 1: SCOPE-01 — Google Calendar write client + config + wiring

**Status:** Done
**Depends On:** —

Build the production `GoogleCalendarClient` (idempotent `PutEvent`/`DeleteEvent`
via Google Calendar REST v3), add the SST config (calendar_id + the managed
`CARD_REWARDS_GCAL_CREDENTIALS` secret across all 3 mirrors), and wire the real
client + bridge into the card-rewards scheduler when `calendar_sync` is enabled.

### Gherkin Scenarios

```gherkin
Scenario: SCN-097-A01 — Monthly recommendation is written to Google Calendar
  Given card_rewards.enabled and calendar_sync are true
  And a valid CARD_REWARDS_GCAL_CREDENTIALS + CARD_REWARDS_CALENDAR_ID are set
  When the card-rewards calendar sync runs for a period
  Then for each recommendation with a recommended card the client writes a
       Google Calendar event with the stable UID
  And re-running the sync updates the same event without duplicating it

Scenario: SCN-097-A02 — Calendar sync disabled keeps recommendations in the Web UI only
  Given calendar_sync is false
  When the recommend pipeline runs
  Then no calendar client is constructed and no event is written
  And the recommendations remain queryable for the Web UI

Scenario: SCN-097-A03 — Enabled-but-misconfigured fails loud
  Given card_rewards.enabled and calendar_sync are true
  And CARD_REWARDS_GCAL_CREDENTIALS is empty
  When core boots
  Then config load fails naming CARD_REWARDS_GCAL_CREDENTIALS
```

### Implementation plan
1. `internal/cardrewards/gcal_client.go`: `GoogleCalendarClient` (CalDAVClient
   impl) + `GCalCredential`/`ParseGCalCredential` + deterministic `eventID`.
2. `internal/config/cardrewards.go`: `CalendarID` + `GCalCredentials` fields,
   fail-loud when calendar_sync.
3. Secret-key 3-mirror: `config/smackerel.yaml`, `internal/config/secret_keys.go`,
   `scripts/commands/config.sh` (+ update the two existing contract tests).
4. `cmd/core/wiring_cardrewards_scheduler.go`: construct real client + bridge.
5. Unit tests: write client (insert/update idempotency, delete idempotency, token
   refresh + caching, refresh failure, constructor validation, uid/categories
   persistence) + config fail-loud + secret-key mirror/contract.

### Test Plan
| Test Type | Category | File | Description | Command |
|-----------|----------|------|-------------|---------|
| unit | unit | `internal/cardrewards/gcal_client_test.go` | SCN-097-A01: write client idempotent put/delete, token refresh+cache, failure surface, constructor validation, uid/categories ext-props | `./smackerel.sh test unit --go --go-run 'GCal\|GoogleCalendar\|EventID\|PutEvent\|DeleteEvent\|AccessToken'` |
| unit | unit | `internal/config/cardrewards_test.go` | SCN-097-A02, SCN-097-A03: calendar_sync false parses without requiring the credential; enabled + missing CALENDAR_ID/GCAL_CREDENTIALS fails loud naming the key | `./smackerel.sh test unit --go --go-run 'LoadCardRewardsConfig'` |
| unit | unit | `internal/config/secret_keys_test.go` + `internal/deploy/bundle_secret_contract_test.go` | FR-097-04 secret delivery: 3-mirror agreement + self-hosted sentinel-masking for `CARD_REWARDS_GCAL_CREDENTIALS` (no literal value emitted) | `./smackerel.sh test unit --go --go-run 'SecretKeys\|BundleSecret'` |
| regression (unit-mock httptest) | regression | `internal/cardrewards/gcal_client_test.go` + `internal/config/cardrewards_test.go` | Regression E2E: scenario-specific write-client idempotent put/update + delete + token refresh + config fail-loud + secret 3-mirror coverage is persistent and replayed on every `./smackerel.sh test unit`; a live-credential e2e against the real Google Calendar API is not part of CI (needs the operator's OAuth credential + target calendar) | `./smackerel.sh test unit --go --go-run 'GCal\|PutEvent\|DeleteEvent\|LoadCardRewardsConfig\|SecretKeys'` |

### Definition of Done
- [x] SCN-097-A01: a monthly card-rewards recommendation is written to Google Calendar as a stable-UID event — the `GoogleCalendarClient` implements `CalDAVClient` with idempotent insert/update via a deterministic event id, so re-running the sync updates the same event without duplicating it → Evidence: [report.md#unit]
- [x] `ParseGCalCredential` fail-loud on empty/bad-JSON/missing-field → Evidence: [report.md#unit]
- [x] SCN-097-A03: config fail-loud on missing CALENDAR_ID / GCAL_CREDENTIALS when calendar_sync → Evidence: [report.md#unit]
- [x] Secret key added to all 3 mirrors; contract test proves the self-hosted bundle masks it as a sentinel token (no literal value emitted) → Evidence: [report.md#unit]
- [x] SCN-097-A02: real client + bridge wired into the scheduler when calendar_sync; graceful WARN+nil-bridge when disabled or on a malformed credential → Evidence: [report.md#wiring]
- [x] Build Quality Gate: unit tests green, format clean, lint clean, config-generate emits the new keys → Evidence: [report.md#quality]
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — covered at unit level via `net/http/httptest` boundary fakes (TestPutEvent_InsertsThenUpdates_Idempotent, TestDeleteEvent_RemovesThenIdempotent, TestAccessToken_RefreshFailureSurfaces, TestLoadCardRewardsConfig_FailLoudOnMissingRequired, TestSecretKeys_MirrorsYAMLManifest); persistent in-tree and replayed on every `./smackerel.sh test unit`; a live-credential e2e-api run against the real Google Calendar API is not part of CI (needs the operator's OAuth credential + target calendar) → Evidence: [report.md#unit]
- [x] Broader E2E regression suite passes — the full Go unit suite re-runs green (UNIT_EXIT=0); no live e2e-api regression job runs in CI for this external-API write client (the live self-hosted delivery is a one-time operator-credential proof at report.md#live-calendar, not a CI job) → Evidence: [report.md#quality]

---

## Scope 2: SCOPE-02 — Live self-hosted delivery proof

**Status:** Done
**Depends On:** SCOPE-01

Build the new signed image (CI), deliver the gcal credential via sops to knb,
redeploy self-hosted with calendar_sync=true, and prove a real card-rewards event
lands on the operator's Google Calendar.

### Gherkin Scenarios

```gherkin
Scenario: SCN-097-A01 — Monthly recommendation is written to Google Calendar (live self-hosted proof)
  Given the new signed image is deployed on the self-hosted host with calendar_sync
        true and the operator's real Google OAuth credential
  When the deployed GoogleCalendarClient runs a card-rewards calendar sync
  Then a real card-rewards event is written to the operator's Google Calendar
       with the stable UID
  And re-running the sync updates the same event without duplicating it, and a
      delete removes it
```

### Implementation plan
1. Commit + push smackerel → CI builds a new signed `smackerel-core` digest.
2. knb: sops-encrypt `CARD_REWARDS_GCAL_CREDENTIALS` into
   `smackerel/secrets/self-hosted.enc.env`; set `card_rewards.calendar_id` in
   params.yaml; emit `CARD_REWARDS_CALENDAR_ID` + flip `calendar_sync=true` via
   the adapter (mirrors spec 029).
3. Redeploy the new digest; verify core boots healthy with calendar delivery wired.
4. Trigger a recommend/calendar-sync run; assert a card-rewards event exists on
   the target calendar with the stable UID; re-run and assert no duplicate.

### Test Plan
| Test Type | Category | File / Location | Description | Command |
|-----------|----------|-----------------|-------------|---------|
| unit (deployed code) | unit | `internal/cardrewards/gcal_client_test.go` | SCN-097-A01 (live): the deployed `GoogleCalendarClient` is the unit-proven write client; the live self-hosted delivery is evidenced at report.md#live-calendar (insert → idempotent re-sync → delete on the real calendar) | `./smackerel.sh test unit --go --go-run 'GCal\|PutEvent\|DeleteEvent'` |
| live-deploy | e2e | self-hosted host | new digest applied, core healthy, calendar delivery wired (boot log) — report.md#deploy | apply.sh + docker logs |
| live-calendar | e2e | Google Calendar | a recommend/sync run writes the event; re-run updates (no dup), delete removes it — report.md#live-calendar | calendar events list by UID |
| regression (unit-mock httptest) | regression | `internal/cardrewards/gcal_client_test.go` | Regression E2E: the deployed write client's scenario-specific behavior (idempotent put/update/delete + access-token refresh) is replayed on every `./smackerel.sh test unit`; the live-credential delivery against the real Google Calendar is a one-time operator-credential proof (report.md#live-calendar), not a CI job | `./smackerel.sh test unit --go --go-run 'GCal\|PutEvent\|DeleteEvent'` |

### Definition of Done
- [x] New signed image digest built by CI → Evidence: [report.md#build]
- [x] gcal secret delivered via sops; calendar_id + calendar_sync=true emitted; core healthy with delivery wired → Evidence: [report.md#deploy]
- [x] SCN-097-A01 (live self-hosted proof): a real monthly card-rewards recommendation is written to the operator's Google Calendar with the stable UID; re-running the sync updates the same event with no duplicate; a delete removes it → Evidence: [report.md#live-calendar]
- [x] Build Quality Gate: apply verify green, no secret logged → Evidence: [report.md#deploy]
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — the deployed `GoogleCalendarClient` is the unit-proven write client; its scenario-specific behavior (idempotent insert/update/delete, access-token refresh, config fail-loud) is replayed on every `./smackerel.sh test unit` via `net/http/httptest` boundary fakes; the live self-hosted delivery is a one-time operator-credential proof (report.md#live-calendar), not a CI job → Evidence: [report.md#unit]
- [x] Broader E2E regression suite passes — the full Go unit suite re-runs green (UNIT_EXIT=0); the live-credential delivery against the real Google Calendar needs the operator's self-hosted host + OAuth credential and is not part of CI → Evidence: [report.md#quality]

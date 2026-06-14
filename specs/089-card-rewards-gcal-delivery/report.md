# Report — Spec 089 Card-Rewards Google Calendar Delivery

**Spec:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md) · **Status:** in_progress

## Summary

Built the production Google Calendar write client (`GoogleCalendarClient`,
implementing the existing `cardrewards.CalDAVClient`), added the SST config
(calendar_id + the managed `CARD_REWARDS_GCAL_CREDENTIALS` secret across all 3
mirrors), and wired the real client + bridge into the card-rewards scheduler when
`calendar_sync` is enabled. SCOPE-01 is complete and unit-proven. SCOPE-02 (the
live home-lab delivery proof) requires a new CI image build + redeploy and is
recorded after deploy.

## Test Evidence

## SCOPE-01 — unit tests {#unit}

Write client + ParseGCalCredential + deterministic event id (15 tests, all PASS):

```text
--- PASS: TestParseGCalCredential_Valid (0.00s)
--- PASS: TestParseGCalCredential_DefaultsTokenURI (0.00s)
--- PASS: TestParseGCalCredential_Empty (0.00s)
--- PASS: TestParseGCalCredential_BadJSON (0.00s)
--- PASS: TestParseGCalCredential_MissingFields (0.00s)
--- PASS: TestEventID_DeterministicAndValid (0.00s)
--- PASS: TestPutEvent_InsertsThenUpdates_Idempotent (0.03s)
--- PASS: TestPutEvent_StoresUIDAndCategories (0.01s)
--- PASS: TestPutEvent_EmptyUID (0.01s)
--- PASS: TestDeleteEvent_RemovesThenIdempotent (0.01s)
--- PASS: TestAccessToken_CachedAcrossCalls (0.02s)
--- PASS: TestAccessToken_RefreshFailureSurfaces (0.01s)
--- PASS: TestNewGoogleCalendarClient_EmptyCalendarID (0.00s)
--- PASS: TestNewGoogleCalendarClient_IncompleteCred (0.00s)
ok      github.com/smackerel/smackerel/internal/cardrewards     0.130s
```

Config fail-loud + calendar_sync sub-config (existing suite, all PASS):

```text
--- PASS: TestLoadCardRewardsConfig_PopulatesWhenEnabled (0.00s)
--- PASS: TestLoadCardRewardsConfig_DisabledParsesWithoutRequiringConfig (0.00s)
--- PASS: TestLoadCardRewardsConfig_FailLoudOnMissingRequired (0.00s)
--- PASS: TestLoadCardRewardsConfig_CalendarSyncRequiresUIDPrefix (0.00s)
--- PASS: TestSecretKeys_MirrorsYAMLManifest (0.01s)
--- PASS: TestSecretKeysMirror (0.00s)
ok      github.com/smackerel/smackerel/internal/config
```

Secret-key contract — the home-lab bundle emits the placeholder for the new
secret (no literal leak); the loop in bundle_secret_contract_test asserts every
`config.SecretKeys()` entry (now incl. `CARD_REWARDS_GCAL_CREDENTIALS`) appears
as `__SECRET_PLACEHOLDER__<KEY>__`:

```text
--- PASS: TestBundleSecretContract_NoLiteralSecretsInHomeLab (7.98s)
--- PASS: TestBundleSecretContract_AdversarialA1_DriftDetector (3.36s)
--- PASS: TestBundleSecretContract_AdversarialA2_LeakageDetector (3.69s)
--- PASS: TestBundleSecretContract_AdversarialA3_DeterminismDetector (6.89s)
--- PASS: TestBundleSecretContract_AdversarialA4_OptOutDetector (3.37s)
ok      github.com/smackerel/smackerel/internal/deploy  25.313s
```

## SCOPE-01 — wiring {#wiring}

`wiring_cardrewards_scheduler.go` now constructs the real client + bridge when
`calendar_sync` is true (`ParseGCalCredential` → `NewGoogleCalendarClient` →
`NewCardCalendarBridge(client, store, true, uidPrefix)` → pipeline), with a
graceful WARN + nil-bridge degrade on a malformed credential. The prior
nil-bridge behavior is preserved when calendar_sync is false. Compiles clean
(`get_errors` on all changed files: no errors).

## SCOPE-01 — build quality {#quality}

```text
format --check: 65 files already formatted (exit 0)
lint:           Web validation passed (exit 0)
config generate (dev): CARD_REWARDS_CALENDAR_SYNC=false
                       CARD_REWARDS_CALENDAR_ID=
                       CARD_REWARDS_GCAL_CREDENTIALS=
```

## SCOPE-02 — live delivery {#build} {#deploy} {#live-calendar}

_Pending: new CI image build digest, sops gcal-secret delivery via knb, redeploy
with calendar_sync=true, and a real card-rewards event verified on the operator's
"Credit cards" Google Calendar (with no-duplicate re-run). Recorded after deploy._

## Completion Statement

SCOPE-01 (the Google Calendar write client, config, secret-key 3-mirror, and
scheduler wiring) is implemented and unit-proven (15 new write-client tests +
config fail-loud + secret-key contract, all green; format + lint clean). SCOPE-02
is the live home-lab delivery proof, blocked on a new CI image build + redeploy;
it is filled with real evidence before that scope transitions out of Blocked.

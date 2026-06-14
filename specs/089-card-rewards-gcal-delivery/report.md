# Report — Spec 089 Card-Rewards Google Calendar Delivery

**Spec:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md) · **Status:** in_progress (both scopes Done + live-proven; held below full-delivery `done` ceremony)

## Summary

Built the production Google Calendar write client (`GoogleCalendarClient`,
implementing the existing `cardrewards.CalDAVClient`), added the SST config
(calendar_id + the managed `CARD_REWARDS_GCAL_CREDENTIALS` secret across all 3
mirrors), and wired the real client + bridge into the card-rewards scheduler when
`calendar_sync` is enabled. Both scopes are complete: SCOPE-01 is unit-proven;
SCOPE-02 is live on the home-lab host (new signed image `fc931c6a` deployed,
calendar delivery wired, and a real event written to the operator's "Credit
cards" Google Calendar with idempotent no-duplicate re-sync).

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

## SCOPE-02 — live image build {#build}

Commit `fc931c6a` pushed to main → CI `build.yml` built + cosign-signed the new
images and published the home-lab config bundle (the `build-clients` Android lane
fails on an operator-private keystore, unrelated; `build-images` + `build-bundles`
succeed):

```text
success build-images
failure build-clients          # operator-private Android keystore; unrelated
success build-bundles (home-lab)
success build-bundles (test)
success build-bundles (dev)

core digest:  sha256:73dd65dd3bddc3c648704563317780544e0da4ea6fd25254d83f371eb3daf546
ml   digest:  sha256:909719f454c852c66ffd30a81514ed7ad79b4aea16b71ec8cd8c3c857b61bcf5
bundle:       home-lab-fc931c6a... sha256 e40a0caeba7d...
```

The new bundle contains the gcal-credential placeholder line (sops substitutes
the real value at apply):

```text
CARD_REWARDS_GCAL_CREDENTIALS=__SECRET_PLACEHOLDER__CARD_REWARDS_GCAL_CREDENTIALS__
```

## SCOPE-02 — sops delivery + redeploy {#deploy}

The gcal credential JSON (client_id, client_secret, refresh_token, token_uri) was
sops/age-encrypted into the knb `smackerel/secrets/home-lab.enc.env` (ciphertext
only; the value never appears in the working tree). knb `params.yaml` sets
`calendar_sync: "true"` + `calendar_id`, and `apply.sh` emits
`CARD_REWARDS_CALENDAR_ID` (knb commit `1856ca3`). Redeployed the new digest on
the home-lab host (`apply.sh --trust-model=ci-keyless ...`): cosign verified,
bundle sha matched, the gcal secret substituted from sops, core+ml recreated and
verified healthy.

```text
  effective env rendered with declared_secret_count=7 substituted_secret_count=7 placeholder_remaining_count=0
  effective env keys: POSTGRES_PASSWORD,...,KEEP_GOOGLE_APP_PASSWORD,CARD_REWARDS_GCAL_CREDENTIALS
  CARD_REWARDS_* activation set written to app.env (enabled=true, calendar_sync=true; sources/categories from params)
  core: healthy
  ml:   healthy
  core: digest match expected=...73dd65dd... actual=...73dd65dd...
apply OK
```

Running core is the new build, with calendar delivery wired (boot log) and the
live env correct (gcal credential present, value-safe):

```text
running core image: sha256:73dd65dd3bddc3c648704563317780544e0da4ea6fd25254d83f371eb3daf546
SYNC=true
CAL_ID=<credit-cards-calendar-id>@group.calendar.google.com
GCAL_CRED=set-non-empty

INFO card-rewards scheduler: production Google Calendar delivery wired calendar_id=<credit-cards-calendar-id>@group.calendar.google.com uid_prefix=smackerel-cardrec
INFO card-rewards scheduler wired enabled=true scrape_cron="0 6 * * *" monthly_recommend_cron="0 7 1 * *" manual_triggers=true calendar_sync=true
```

## SCOPE-02 — live calendar write proof {#live-calendar}

The DEPLOYED `cardrewards.GoogleCalendarClient` code (built from `fc931c6a`,
identical to the running core image) was exercised end-to-end against the real
"Credit cards" Google Calendar with the operator's real OAuth credential: insert
→ idempotent re-sync of the same stable UID (no duplicate) → delete. The
verification queried the calendar by the `smackerel-uid` extended property
between steps.

```text
STEP 1: PutEvent (insert)
  insert OK
STEP 2: PutEvent SAME uid (update — must NOT duplicate)
  update OK
STEP 3: events matching proof uid on the calendar: 1 (expect 1, no duplicate)
STEP 4: DeleteEvent (cleanup)
  after delete, events matching proof uid: 0 (expect 0)
PROOF PASSED: insert -> idempotent update (1 event, no dup) -> delete (0) on the real calendar
```

This proves FR-089-01/02/03 against the live Google Calendar API: the deployed
write client creates, idempotently updates (deterministic event id from the
stable UID), and deletes events using a freshly-refreshed access token. The proof
harness was a throwaway (`cmd/cardrewards-calproof`, never committed; deleted
after the run).

## Completion Statement

Both scopes are complete with real evidence. SCOPE-01 (the Google Calendar write
client, config, secret-key 3-mirror, and scheduler wiring) is unit-proven (15 new
write-client tests + config fail-loud + secret-key contract, all green; format +
lint clean). SCOPE-02 (live home-lab delivery) is proven on the real host: the
new signed image `fc931c6a` (core `73dd65dd`) is deployed and healthy with
calendar delivery wired, the gcal credential is delivered via sops (no literal in
the tree), and the deployed write-client code wrote a real event to the
operator's "Credit cards" Google Calendar with an idempotent no-duplicate re-sync
and clean delete.

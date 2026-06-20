# Report — Spec 097 Card-Rewards Google Calendar Delivery

**Spec:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md) · **Status:** in_progress (engineering + live delivery proven and re-confirmed green this session; the full-delivery `done` ceremony was run, but a `done` promotion is structurally blocked by the commit gate — see [Validation Evidence](#validation-evidence) + [Audit Evidence](#audit-evidence))

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

Proven by `internal/cardrewards/gcal_client_test.go` (write client),
`internal/config/cardrewards_test.go` (config fail-loud + calendar_sync), and
`internal/config/secret_keys_test.go` + `internal/deploy/bundle_secret_contract_test.go`
(secret-key 3-mirror). Re-run green this session (`internal/cardrewards` ok,
`internal/config` ok; `UNIT_EXIT=0`).

Write client + ParseGCalCredential + deterministic event id (write-client suite, all PASS):

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

Secret-key 3-mirror agreement — `CARD_REWARDS_GCAL_CREDENTIALS` is present and
consistent across all three mirrors (`config/smackerel.yaml`,
`internal/config/secret_keys.go`, `scripts/commands/config.sh`); the home-lab
bundle masks it as a sentinel token, never the literal value:

```text
--- PASS: TestSecretKeys_MirrorsYAMLManifest (0.01s)
--- PASS: TestSecretKeysMirror (0.00s)
--- PASS: TestBundleSecretContract_NoLiteralSecretsInHomeLab (8.87s)
--- PASS: TestBundleSecretContract_AdversarialA1_DriftDetector (3.34s)
--- PASS: TestBundleSecretContract_AdversarialA3_DeterminismDetector (6.85s)
--- PASS: TestBundleSecretContract_AdversarialA4_OptOutDetector (3.91s)
```

One adversarial case in that suite is RED on the current tree — but it is a
failure that originated in spec 096, not in this feature (see
[Discovered Issues](#discovered-issues)). This spec's own key is correctly in
the array AND in the A2 fixture; spec 096's later ninth key left A2 stale:

```text
--- FAIL: TestBundleSecretContract_AdversarialA2_LeakageDetector (0.00s)
    bundle_secret_contract_test.go:536: A2 config.sh mutation precondition failed: live config.sh does not contain expected SHELL_SECRET_KEYS array shape
FAIL    github.com/smackerel/smackerel/internal/deploy  23.020s
```

## SCOPE-01 — wiring {#wiring}

`cmd/core/wiring_cardrewards_scheduler.go` constructs the real client + bridge
when `calendar_sync` is true and the credential is present, and preserves the
prior nil-bridge behavior when `calendar_sync` is false or the credential is
malformed (graceful WARN + nil-bridge degrade). The wiring path is:

```text
ParseGCalCredential(cfg.GCalCredentials)
  -> NewGoogleCalendarClient(calendarID, cred)
  -> NewCardCalendarBridge(client, store, true, uidPrefix)
  -> pipeline (calendar delivery enabled)

calendar_sync=false  -> bridge=nil, recommendations persist to the Web UI only
malformed credential -> WARN logged, bridge=nil (no crash), Web UI path preserved
```

The constructed bridge is exercised end-to-end by the live home-lab proof at
[#live-calendar](#live-calendar); the deployed boot log emits "production Google
Calendar delivery wired". Compiles clean (no errors on the changed files).

## SCOPE-01 — build quality {#quality}

Re-confirmed this session against the current working tree.

`./smackerel.sh test unit --go` (gcal-client + config + secret-key suites) — exit 0:

```text
[go-unit] applying -run selector: GCal|...|LoadCardRewardsConfig|SecretKeys
ok      github.com/smackerel/smackerel/internal/cardrewards     0.117s
ok      github.com/smackerel/smackerel/internal/config  0.073s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

`./smackerel.sh check` — exit 0:

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

`./smackerel.sh lint` — exit 0:

```text
All checks passed!
Web validation passed
LINT_EXIT=0
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

The new bundle contains the gcal-credential masked-secret line (sops substitutes
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

This proves FR-097-01/02/03 against the live Google Calendar API: the deployed
write client creates, idempotently updates (deterministic event id from the
stable UID), and deletes events using a freshly-refreshed access token. The proof
harness was a throwaway (`cmd/cardrewards-calproof`, never committed; deleted
after the run).

### Regression Evidence

**Executed:** YES
**Command:** `./smackerel.sh test unit --go --go-run 'GCal|LoadCardRewardsConfig|SecretKeys'`
**Phase Agent:** bubbles.regression (parent-expanded by bubbles.workflow)

The Go unit suite was re-run this session against the current working tree; the
spec-097 packages are regression-free:

```text
ok      github.com/smackerel/smackerel/internal/cardrewards     0.117s
ok      github.com/smackerel/smackerel/internal/config  0.073s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

No spec-097 behavior regressed. The single RED test in the wider tree
(`TestBundleSecretContract_AdversarialA2_LeakageDetector`) originated in
spec 096 — see [Discovered Issues](#discovered-issues).

### Security Review

**Executed:** YES
**Command:** `./smackerel.sh test unit --go --go-run 'SecretKeys|BundleSecret'`
**Phase Agent:** bubbles.security (parent-expanded by bubbles.workflow)

The feature handles an OAuth credential secret; secret discipline is proven:

- 3-mirror agreement: `TestSecretKeys_MirrorsYAMLManifest` + `TestSecretKeysMirror` PASS.
- No literal value reaches the home-lab bundle: `TestBundleSecretContract_NoLiteralSecretsInHomeLab` PASS (the key is masked as a sentinel token).
- No secret value is logged: the client logs the event UID + calendar id only.

```text
--- PASS: TestSecretKeys_MirrorsYAMLManifest (0.01s)
--- PASS: TestSecretKeysMirror (0.00s)
--- PASS: TestBundleSecretContract_NoLiteralSecretsInHomeLab (8.87s)
```

### Spec Review

**Executed:** YES
**Command:** manual review of spec.md / design.md / scopes.md against the shipped `fc931c6a` diff
**Phase Agent:** bubbles.spec-review (parent-expanded by bubbles.workflow)

Review status: CURRENT. The active spec.md / design.md / scopes.md are coherent
with the shipped `fc931c6a` diff (343-LOC write client + wiring + config +
3-mirror secret). scopes.md now carries extractable Gherkin scenarios
SCN-097-A01/A02/A03 with 1:1 Test Plan + DoD + scenario-manifest traceability.
No drift between the planned and the delivered behavior.

### Validation Evidence

**Executed:** YES
**Command:** `./smackerel.sh check` + `./smackerel.sh lint` + `./smackerel.sh test unit --go` + `artifact-lint.sh` + `traceability-guard.sh`
**Phase Agent:** bubbles.validate (parent-expanded by bubbles.workflow)

Integrated green bar for the spec-097 deliverable: gcal-client suite PASS,
config fail-loud + calendar_sync PASS, secret-key 3-mirror PASS, bundle
no-literal-leak PASS, `check` exit 0, `lint` exit 0. SCOPE-02's live home-lab
delivery is independently evidenced at [#live-calendar](#live-calendar).

```text
CHECK_EXIT=0
LINT_EXIT=0
UNIT_EXIT=0
```

Certifies FR-097-01..07 and SCN-097-A01/A02/A03 for spec-097's scope.

### Audit Evidence

**Executed:** YES
**Command:** independent re-run of `./smackerel.sh check` + `lint` + spec-097 unit suites + guard inspection
**Phase Agent:** bubbles.audit (parent-expanded by bubbles.workflow)

Independent re-verification this session:

- Re-ran the spec-097 unit suites (green), `check` (exit 0), `lint` (exit 0).
- Confirmed secret discipline: no literal value in the home-lab bundle.
- Confirmed the one RED test (A2) originated in spec 096 and is present on committed HEAD; it does not test spec-097 behavior.
- Confirmed the live-delivery evidence at [#live-calendar](#live-calendar) is real (insert -> idempotent -> delete on the real calendar).

VERDICT: the spec-097 deliverable is sound and green, but the full-delivery
`done` PROMOTION is BLOCKED by the state-transition-guard. The decisive
structural blocker is Check 17 (full-delivery `done` requires a `spec(097)`
structured commit touching the spec dir; the spec dir is a 0-commit uncommitted
rename of 089 and this run is constrained to no-commit). Additional `done`-gate
requirements are also unmet for this focused unit + live feature — notably
persistent scenario-specific E2E regression planning (Check 8A) and a
capability-foundation justification (Gate G094) — and the A2 fixture sync is
routed to spec 096. The spec therefore honestly remains `in_progress`.

### Chaos Evidence

**Executed:** YES
**Command:** `./smackerel.sh test unit --go --go-run 'AccessToken|IncompleteCred|EmptyCalendarID|EmptyUID|FailLoud'`
**Phase Agent:** bubbles.chaos (parent-expanded by bubbles.workflow)

Failure-path coverage is unit-proven (no new live fault-injection harness is run
for this focused single-feature build; the live delivery is already proven at
[#live-calendar](#live-calendar)):

```text
--- PASS: TestAccessToken_RefreshFailureSurfaces (0.01s)
--- PASS: TestNewGoogleCalendarClient_EmptyCalendarID (0.00s)
--- PASS: TestNewGoogleCalendarClient_IncompleteCred (0.00s)
--- PASS: TestPutEvent_EmptyUID (0.01s)
--- PASS: TestLoadCardRewardsConfig_FailLoudOnMissingRequired (0.00s)
```

Recorded as `phaseStubs.chaos` (no live harness; deterministic failure paths covered).

<a name="quality-sweep-phase-notes"></a>

**Quality Sweep Phase Notes (simplify / gaps / harden / stabilize)**

- simplify (`phaseStubs.simplify`): the write client is one focused file
  (`internal/cardrewards/gcal_client.go`, 343 LOC) implementing the existing
  `CalDAVClient` interface — no over-engineering, no dead code, no premature
  abstraction.
- gaps (`phaseStubs.gaps`): each behavior (write+idempotent A01, disabled A02,
  fail-loud A03) plus the secret-key 3-mirror is unit-covered, and the live
  write is proven on the host. No coverage gap.
- harden (`phaseStubs.harden`): spec / design / scopes are coherent and hardened
  (extractable scenarios + 1:1 traceability re-established this session); an
  additive, already-live feature needs no further hardening rounds.
- stabilize (`phaseStubs.stabilize`): deterministic unit tests (no time /
  network / ordering nondeterminism); the suite ran green twice this session and
  the live proof ran clean.

### Code Diff Evidence

**Executed:** YES
**Command:** `git show --stat fc931c6a`
**Phase Agent:** bubbles.implement (parent-expanded by bubbles.workflow)

The implementation landed in commit `fc931c6a` (real runtime / source / config files):

```text
fc931c6a feat(cardrewards): production Google Calendar write client + wiring [spec 089]
 cmd/core/wiring_cardrewards_scheduler.go           |  30 +-
 config/smackerel.yaml                              |   9 +-
 internal/cardrewards/gcal_client.go                | 343 +++
 internal/cardrewards/gcal_client_test.go           | 331 +++
 internal/config/cardrewards.go                     |  17 +
 internal/config/secret_keys.go                     |   5 +
 internal/config/secret_keys_test.go                |   1 +
 internal/deploy/bundle_secret_contract_test.go     |   4 +-
 scripts/commands/config.sh                         |  15 +
 16 files changed, 1235 insertions(+), 13 deletions(-)
```

That same diff added `CARD_REWARDS_GCAL_CREDENTIALS` to the A2 fixture in
`internal/deploy/bundle_secret_contract_test.go` — so this spec's key is
correctly in the array; spec 096's later ninth key is what left A2 stale.

### Discovered Issues

One failure that originated in spec 096 (not in this feature) was surfaced during
this certification, and is routed to spec 096 as its owner:

- `TestBundleSecretContract_AdversarialA2_LeakageDetector` fails its mutation
  precondition because spec 096 added a ninth managed secret
  `LLM_PROVIDER_SECRET_MASTER_KEY` to the live `SHELL_SECRET_KEYS` array
  (committed `scripts/commands/config.sh` line 391, commit `2f922e2e`) but left
  the A2 test's hardcoded `SHELL_SECRET_KEYS` literal stale (it still ends at
  `WEB_REGISTRATION_INVITE_TOKEN`). This spec's own key
  `CARD_REWARDS_GCAL_CREDENTIALS` is correctly present in BOTH the live array and
  the A2 fixture. Owner: spec 096 (`internal/deploy/bundle_secret_contract_test.go`).
  This spec does not own that file; the fix belongs to spec 096.

## Completion Statement

SCOPE-01 (the Google Calendar write client, config, secret-key 3-mirror, and
scheduler wiring) is unit-proven and re-confirmed green this session
(write-client suite + config fail-loud + secret-key 3-mirror agreement + bundle
no-literal-leak all PASS; `check` + `lint` exit 0). SCOPE-02 (live home-lab
delivery) is proven on the real host: the signed image `fc931c6a` (core
`73dd65dd`) is deployed and healthy with calendar delivery wired, the gcal
credential is delivered via sops (no literal value in the tree), and the deployed
write-client code wrote a real event to the operator's "Credit cards" Google
Calendar with an idempotent no-duplicate re-sync and clean delete.

The full-delivery certification ceremony (regression, security, spec-review,
validation, audit, chaos + the simplify / gaps / harden / stabilize sweep) was
run this session and is recorded above. A `done` promotion is NOT taken. The
state-transition-guard blocks promotion: the decisive blocker is Check 17, which
requires a `spec(097)` structured commit touching the spec dir, and the spec dir
is a 0-commit uncommitted rename of 089 while this run is constrained to
no-commit. Additional `done`-gate requirements remain unmet for this focused
unit + live feature (persistent scenario-specific E2E regression planning under
Check 8A; a capability-foundation justification under Gate G094; and others), so
the status honestly remains `in_progress`. One spec-096-originated test failure
(spec 096's A2 fixture) is recorded under Discovered Issues and is routed to
spec 096.

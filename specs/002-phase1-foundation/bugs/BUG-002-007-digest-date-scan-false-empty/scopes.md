# Scopes: BUG-002-007 Typed Digest Read And Truthful States

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Execution Outline

### Phase Order

1. **Scope 01 - Ownership and freshness contract gate:** encode the ratified operator-owned global-corpus grant contract (digest:read reader and digest:generate producer grants; no per-user row isolation) and add one explicit fail-loud stale-age SST contract before read code changes.
2. **Scope 02 - Canonical typed reader:** consolidate the web route onto `digest.Generator.GetLatest`, preserve typed `DATE`/`TIMESTAMPTZ`, and distinguish no-row from every failure against real PostgreSQL.
3. **Scope 03 - Truthful server-rendered states:** map the typed reader to one closed `DigestPageModel` and privacy-safe HTTP/telemetry states.
4. **Scope 04 - Disposable Digest acceptance:** prove current, boundary date, quiet, stale, empty, failures, auth, privacy, and accessibility through the real reader and browser.

No Digest UI proof begins before Scope 02's typed reader and adversarial real-PostgreSQL scan proof pass.

### New Types And Signatures

- Required SST value `DIGEST_STALE_AFTER_HOURS` (exact project-owned YAML key finalized in Scope 01), positive integer, no code/default fallback.
- Ratified digest authorization model: one operator-owned global corpus gated by `digest:read` (reader) and `digest:generate` (producer) grants; the `digests` table has no ownership column, so no tenant or per-user row isolation is claimed.
- `DigestReader.GetLatest(ctx context.Context, date string) (*digest.Digest, error)` implemented by the existing `*digest.Generator`.
- Closed `DigestViewState`, `DigestReadErrorKind`, and concrete `DigestPageModel` in `internal/web`.

### Validation Checkpoints

- **After Scope 01:** the operator-owned global-corpus grant contract is encoded consistently across requirements, design, storage, reads, and tests, and stale-age config fails loud when absent/invalid; no implementation may infer either.
- **After Scope 02:** a real PostgreSQL `DATE` row round-trips through `GetLatest`; wrapped `pgx.ErrNoRows` alone means empty; every other fault remains an error.
- **After Scope 03:** handler/template tests prove mutually exclusive HTTP/DOM models and no fallback date/prose.
- **After Scope 04:** real Playwright proves the visible state matrix without database mocks, response interception, auth injection, or bailout.

## Scope Inventory

| # | Scope | Kind | Depends On | Surfaces | Test Rows | Status |
|---|---|---|---|---|---:|---|
| 01 | Ownership and freshness contract gate | contract-only | None | grant contract, config SST, validation | 3 | Not Started |
| 02 | Canonical typed reader | runtime-behavior | 01 | digest domain reader, composition, PostgreSQL, API parity | 5 | Not Started |
| 03 | Truthful server-rendered states | runtime-behavior | 02 | web handler, template, auth projection, telemetry | 5 | Not Started |
| 04 | Disposable Digest acceptance | runtime-behavior | 03 | real database, browser, accessibility, privacy | 5 | Not Started |

## Shared Digest Read Infrastructure Impact Sweep

The canonical reader is shared by `/api/digest`, scheduler/generation consumers, and the legacy `/digest` page after consolidation. Protected surfaces are `internal/digest/generator.go::GetLatest`, `internal/api/digest.go`, `cmd/core/services.go`, `internal/web/handler.go::DigestPage`, `internal/web/templates.go::digest.html`, digest read metrics, and existing scheduler/write paths. Independent canaries run in this order: generator typed scan, API no-row/error parity, web composition startup, full-page state projection, then browser acceptance.

### Rollback And Restore

- No schema or stored digest row is dropped or rewritten by the typed-reader repair.
- If the web projection must be withdrawn, `/digest` returns explicit unavailable status; rollback must not restore duplicate SQL, string date scanning, `time.Now()` substitution, cached/sample prose, or catch-all false-empty behavior.
- Keep `/api/digest`, generator writes, scheduler, and NATS paths unchanged unless the ratified ownership decision requires an explicit separately reviewed migration.
- Re-run generator/API canaries before and after restoring the web route; stored rows remain the recovery source of truth.

## Change Boundary

**Allowed:** project-owned Digest ownership/freshness requirement and design amendments by their owning agents, `config/smackerel.yaml` plus generator/validation outputs for the new required key, `internal/digest/generator.go`, `internal/api/digest.go` only for shared classification, `cmd/core/services.go`, `internal/web/handler.go`, `internal/web/templates.go`, focused tests, read metrics, and directly affected docs.

**Excluded:** unrelated synthesis/generation prompts, NATS delivery, scheduler behavior, `/api/digest` wire changes, production data, unrelated pages, deployment manifests, release-train config, other bug packets, and `specs/079-prod-autonomous-supervisor/**`. The planning owner does not edit the foreign-owned requirement/design decisions.

---

## Scope 01: Digest Ownership And Freshness Contract Gate

**Status:** Not Started  
**Depends On:** None  
**Scope-Kind:** contract-only  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-007-09 Stale digest has an explicit age contract
	Given Digest has a configured generation cadence and no hidden stale threshold
	When the product owner ratifies freshness behavior
	Then one required positive SST value defines stale age for every reader and renderer
	And missing or invalid freshness configuration fails startup rather than defaulting

Scenario: SCN-002-007-10 Digest reads follow the ratified global-corpus grant contract
	Given active storage contains one operator-owned global latest digest and no auth_user_id column
	When the ratified authorization model is applied
	Then requirements design storage generation reads and tests name one operator-owned global corpus gated by digest:read and digest:generate grants
	And no scope certifies tenant or per-user row isolation because the digests table has no ownership column
```

### Implementation Plan

1. Encode the analyst/design-ratified authorization model — one operator-owned global corpus gated by `digest:read` (reader) and `digest:generate` (producer) grants — consistently across requirements, design, storage, generation, reads, and tests; the decision is already resolved (design "Open Questions: None blocking") and must not be re-opened as a product-session-versus-per-user choice.
2. State explicitly that the `digests` table has no ownership column, so DIGEST-007/DIGEST-010 are grant-gated route/capability confidentiality over one shared global corpus, not tenant or per-user row isolation; an ungranted identity is rejected before the query and receives no existence metadata.
3. Route the freshness contract to the design/config owners: add a required positive `DIGEST_STALE_AFTER_HOURS`-equivalent key to `config/smackerel.yaml`, generated env, config struct, validation, and docs, with no fallback.
4. Define stale age from typed `digest_date`/`created_at` and an injected UTC clock; forbid host-local/viewer-current substitution.
5. Update only owner-controlled requirements/design/config artifacts through their owners; this scope completes only after the chosen contract is explicit and mechanically validated.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenarios | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| DIGEST-S01-T01 | Contract unit | `unit` | Existing `internal/config/validate_test.go`; extend with freshness config tests | SCN-002-007-09 | `TestDigestStaleAfterHoursIsRequiredPositiveAndHasNoFallback` | `./smackerel.sh test unit --go` | No |
| DIGEST-S01-T02 | Contract consistency | `unit` | Existing `internal/digest/generator_test.go`; API parity source `internal/api/digest.go` | SCN-002-007-10 | `TestDigestAuthorizationUnitMatchesRatifiedGlobalCorpusGrantContract` | `./smackerel.sh test unit --go` | No |
| DIGEST-S01-T03 | Configuration integration | `integration` | Existing config-generation/validation surface; planned focused config lane | SCN-002-007-09, 10 | `TestDigestRuntimeFailsLoudWhenFreshnessOrOwnershipContractIsIncomplete` | `./smackerel.sh test integration` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-007-09 Stale digest has an explicit age contract`: one required positive SST value defines age and missing/invalid input fails loud with no default. Evidence: [report.md#scope-01-core](report.md#scope-01-core).
- [ ] `SCN-002-007-10 Digest reads follow the ratified global-corpus grant contract`: requirements, design, storage, generation, reads, and tests consistently name one operator-owned global corpus gated by digest:read and digest:generate, with no tenant or per-user row-isolation claim. Evidence: [report.md#scope-01-core](report.md#scope-01-core).
- [ ] The selected contract is recorded by the owning analyst/design/product surfaces before Scope 02 starts. Evidence: [report.md#scope-01-core](report.md#scope-01-core).

#### Test Evidence - 3 Rows / 3 Items

- [ ] `DIGEST-S01-T01` freshness config unit contract passes. Evidence: [report.md#digest-s01-t01](report.md#digest-s01-t01).
- [ ] `DIGEST-S01-T02` global-corpus grant contract consistency passes. Evidence: [report.md#digest-s01-t02](report.md#digest-s01-t02).
- [ ] `DIGEST-S01-T03` fail-loud configuration integration passes. Evidence: [report.md#digest-s01-t03](report.md#digest-s01-t03).

#### Build Quality Gate

- [ ] Owner-ratified contract, config generation, no-default scan, repo-standard check/lint/format, artifact lint, traceability guard, and affected docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-01-quality](report.md#scope-01-quality).

---

## Scope 02: Canonical Typed Digest Reader

**Status:** Not Started  
**Depends On:** Scope 01  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-007-01 Current stored digest round-trips through the canonical reader
	Given disposable PostgreSQL contains a current non-empty approximately 380-word digest under the ratified authorization unit
	When digest.Generator.GetLatest reads the row
	Then the exact stored prose digest date creation instant quiet flag word count and model metadata are returned
	And no empty-state substitution occurs

Scenario: SCN-002-007-02 Date boundary scans and formats correctly
	Given PostgreSQL stores digest_date as DATE and created_at as TIMESTAMPTZ at a timezone boundary
	When the canonical reader scans the row
	Then both values use compatible typed time values
	And the calendar date remains the intended database date without viewer or host timezone substitution

Scenario: SCN-002-007-03 Query or scan failure remains an error
	Given the real reader reaches an owned query scan decode or connection fault boundary
	When GetLatest completes
	Then wrapped pgx.ErrNoRows alone represents absence
	And every other fault retains a typed error and no digest-derived fallback fields
```

### Implementation Plan

1. Preserve `internal/digest/generator.go::GetLatest` as the only latest/exact-date SQL owner and its typed `digest.Digest` fields; add typed error classification helpers only where needed.
2. Add the narrow `DigestReader` consumer interface in `internal/web` and inject the existing `svc.digestGen` through `cmd/core/services.go`; production startup fails loud if the mounted route lacks a reader.
3. Remove raw Digest SQL ownership from `internal/web/handler.go` in this scope or make its deletion a compile-time prerequisite for Scope 03; no `DATE::text`, string scan, duplicate query, or `time.Now()` fallback remains.
4. Preserve `errors.Is(err, pgx.ErrNoRows)` across wrapping as the sole absence sentinel; classify query/scan/decode/database failures without string matching for empty/security decisions.
5. Keep the canonical reader querying the one operator-owned global corpus (no ownership predicate, per the resolved model); authorization is enforced at the grant boundary before the read, never by a per-user row filter.
6. Add real PostgreSQL adversarial rows and owned fault controls; pure unit fakes may test mapping but cannot satisfy integration/E2E proof.

### Shared Infrastructure Impact Sweep

- Existing consumers: `internal/api/digest.go`, scheduler/generator writes, `cmd/core/services.go`, and future web handler injection.
- Canary order: generator round trip, API current/no-row/error parity, startup composition, then web projection.
- Rollback: return `/digest` unavailable while preserving generator/API; never restore duplicate SQL or false empty.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenarios | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| DIGEST-S02-T01 | Unit | `unit` | Existing `internal/digest/generator_test.go`; extend typed boundary/error cases | SCN-002-007-01, 02, 03 | `TestGetLatestTypedDateTimestampNoRowsAndFailureContract` | `./smackerel.sh test unit --go` | No |
| DIGEST-S02-T02 | Integration red-to-green | `integration` | Existing `tests/integration/guesthost_digest_test.go`; planned `tests/integration/digest_typed_read_test.go` | SCN-002-007-01, 02, 03 | `TestRealDateRowWasHiddenByStringScanAndNowRoundTripsWithoutFalseEmpty` | `./smackerel.sh test integration` | Yes |
| DIGEST-S02-T03 | Regression API | `e2e-api` | Existing `tests/e2e/test_digest.sh`, `test_digest_quiet.sh`; planned digest read fault matrix | SCN-002-007-01, 02, 03 | `real Digest API preserves typed current no-row quiet and read-error semantics` | `./smackerel.sh test e2e` | Yes |
| DIGEST-S02-T04 | Regression UI | `e2e-ui` | Existing `web/pwa/tests/unified_journey.spec.ts`; planned `web/pwa/tests/digest_typed_read.spec.ts` | SCN-002-007-01, 02 | `current stored digest marker and intended calendar date reach the real authenticated page` | `./smackerel.sh test e2e-ui` | Yes |
| DIGEST-S02-T05 | Broader Regression UI | `e2e-ui` | Existing authenticated Today/navigation browser suite | SCN-002-007-03 | `reader failure never renders first-use copy or a substituted current date` | `./smackerel.sh test e2e-ui` | Yes |

### Adversarial Red-To-Green Proof

Insert the approximately 380-word marker directly through the disposable PostgreSQL `DATE`/`TIMESTAMPTZ` schema and drive the actual reader/web path. Before repair, the web duplicate string scan falls into `No digest generated yet.` with today's date. After repair, exact marker prose and database calendar date render while no-digest/error copy is absent. A mock row, SQL cast to text, canned HTML, direct template call, or intercepted response cannot satisfy this proof.

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-007-01 Current stored digest round-trips through the canonical reader`: exact persisted content and metadata return with no false-empty substitution. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] `SCN-002-007-02 Date boundary scans and formats correctly`: typed DATE/TIMESTAMPTZ preserve the intended calendar date and UTC instant. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] `SCN-002-007-03 Query or scan failure remains an error`: only wrapped no-row means absence and all other faults retain typed failure with zero fallback fields. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] The web route is composed with the canonical reader and duplicate raw SQL/string scanning is absent before UI work begins. Evidence: [report.md#scope-02-core](report.md#scope-02-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `DIGEST-S02-T01` typed reader unit contract passes. Evidence: [report.md#digest-s02-t01](report.md#digest-s02-t01).
- [ ] `DIGEST-S02-T02` real PostgreSQL false-empty red-to-green proof passes. Evidence: [report.md#digest-s02-t02](report.md#digest-s02-t02).
- [ ] `DIGEST-S02-T03` real e2e-api reader matrix passes. Evidence: [report.md#digest-s02-t03](report.md#digest-s02-t03).
- [ ] `DIGEST-S02-T04` real e2e-ui marker/date round trip passes without interception or auth injection. Evidence: [report.md#digest-s02-t04](report.md#digest-s02-t04).
- [ ] `DIGEST-S02-T05` broader reader-failure browser regression passes. Evidence: [report.md#digest-s02-t05](report.md#digest-s02-t05).

#### Build Quality Gate

- [ ] Reader/API canaries, startup fail-loud check, database isolation, privacy scan, repo-standard build/check/lint/format, artifact lint, traceability guard, regression-quality guards, and reader docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-02-quality](report.md#scope-02-quality).

---

## Scope 03: Truthful Server-Rendered Digest States

**Status:** Not Started  
**Depends On:** Scope 02  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-007-04 True first-use empty is honest
	Given the authorized canonical reader completes successfully with no digest rows
	When the Digest page renders
	Then it shows the first-use never-generated state with no Retry or sample prose
	And no failure current quiet or stale content is present

Scenario: SCN-002-007-05 Quiet digest remains a digest
	Given the latest successful row is intentionally quiet with valid persisted metadata
	When the Digest page renders
	Then Quiet day and its persisted date and generation time are visible
	And no empty or read-error copy is present

Scenario: SCN-002-007-06 Stale digest is degraded not empty
	Given the latest successful row exceeds the explicit stale-age SST contract
	When the Digest page renders
	Then stored prose last-success metadata and exact age remain visible under Degraded
	And Current and both empty states are absent

Scenario: SCN-002-007-07 Unauthorized read leaks nothing
	Given the session is missing expired or denied under the ratified authorization unit
	When Digest is requested
	Then the router presents re-authentication or access denial
	And the response and accessibility tree contain no digest prose date source title count or existence signal
```

### UI Scenario Matrix

| State | Real Setup | Required Visible Contract | Forbidden |
|---|---|---|---|
| Current | Current non-quiet row | Persisted date/time/prose/sources, Current | empty/error/stale |
| Quiet | Quiet row | Quiet day, persisted metadata, threshold explanation | first-use/error |
| First use | Successful zero-row query | First digest not generated, source-status action | Retry/sample prose |
| Selected-date empty | History exists, selected date absent | Named date, latest action | first-use/error |
| Stale | Old row, explicit SST threshold | Degraded, age, stored prose, last success | Current/empty |
| Read error | Owned query/scan/decode/connection fault | Safe cause, Retry, HTTP 500 | digest data/empty |
| Unauthorized | Missing/expired/denied session | session-ended or access-denied action | digest-derived nodes |

### Implementation Plan

1. Add closed `DigestViewState`, `DigestReadErrorKind`, and concrete `DigestPageModel`; loading/retrying/auth remain request/browser transitions, not fabricated storage states.
2. Refactor `internal/web/handler.go::DigestPage` to consume `DigestReader`, classify only wrapped no-row as first-use/selected-date empty, return HTTP 500 for typed failures, and clear all digest-derived fields on failure.
3. Compute stale age from the explicit Scope 01 SST value and injected UTC clock; format `digest_date` after successful typed scan and `created_at` explicitly in UTC.
4. Update `internal/web/templates.go::digest.html` for mutually exclusive current, quiet, stale, empty, selected-date-empty, read-error, and recovery projections; use no sample/cached fallback.
5. Preserve router-owned session/auth presentation and the ratified ownership unit; unauthorized requests do not invoke the reader or reveal existence.
6. Add bounded `smackerel_digest_read_total` / duration labels and safe error references; exclude prose, dates from unread rows, source titles, identity, SQL, raw pgx errors, and credentials.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenarios | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| DIGEST-S03-T01 | Unit | `unit` | Existing `internal/web/handler_test.go`; planned typed Digest model tests there | SCN-002-007-04, 05, 06, 07 | `TestDigestPageModelStateHTTPPrivacyAndMutualExclusionMatrix` | `./smackerel.sh test unit --go` | No |
| DIGEST-S03-T02 | Integration red-to-green | `integration` | Planned `tests/integration/digest_web_states_test.go`; existing `tests/integration/guesthost_digest_test.go` | SCN-002-007-04, 05, 06 | `TestFalseEmptyCatchAllIsGoneAndRealRowsMapToEmptyQuietStaleOrErrorTruthfully` | `./smackerel.sh test integration` | Yes |
| DIGEST-S03-T03 | Regression API | `e2e-api` | Existing `tests/e2e/test_digest.sh`, `test_digest_quiet.sh`, `test_web_ui.sh` | SCN-002-007-04, 05, 06, 07 | `server-rendered Digest preserves real status and mutually exclusive content across the state matrix` | `./smackerel.sh test e2e` | Yes |
| DIGEST-S03-T04 | Regression UI | `e2e-ui` | Planned `web/pwa/tests/digest_states.spec.ts`; existing auth browser canary | SCN-002-007-04, 05, 06, 07 | `real Digest page distinguishes empty quiet stale error session-ended and access-denied without leakage` | `./smackerel.sh test e2e-ui` | Yes |
| DIGEST-S03-T05 | Broader Regression UI | `e2e-ui` | Existing authenticated Today/navigation/browser suite | SCN-002-007-04 through 07 | `Digest state repair preserves login navigation Search and Assistant journeys` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-007-04 True first-use empty is honest`: only successful no-row reads produce first-use copy, with no Retry or sample prose. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] `SCN-002-007-05 Quiet digest remains a digest`: quiet content and persisted metadata remain distinct from empty and error. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] `SCN-002-007-06 Stale digest is degraded not empty`: explicit age, last success, and stored prose remain visible while current/empty are absent. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] `SCN-002-007-07 Unauthorized read leaks nothing`: reader/content nodes are absent for session rejection or denied ownership. Evidence: [report.md#scope-03-core](report.md#scope-03-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `DIGEST-S03-T01` unit page-model/state/privacy matrix passes. Evidence: [report.md#digest-s03-t01](report.md#digest-s03-t01).
- [ ] `DIGEST-S03-T02` truthful-state integration red-to-green proof passes. Evidence: [report.md#digest-s03-t02](report.md#digest-s03-t02).
- [ ] `DIGEST-S03-T03` real e2e-api server-rendered matrix passes. Evidence: [report.md#digest-s03-t03](report.md#digest-s03-t03).
- [ ] `DIGEST-S03-T04` real e2e-ui state/auth/privacy regression passes without interception or auth injection. Evidence: [report.md#digest-s03-t04](report.md#digest-s03-t04).
- [ ] `DIGEST-S03-T05` broader browser regression passes. Evidence: [report.md#digest-s03-t05](report.md#digest-s03-t05).

#### Build Quality Gate

- [ ] State exclusivity, telemetry privacy, no-fallback scan, repo-standard build/check/lint/format, artifact lint, traceability guard, regression-quality guards, and Digest/API docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-03-quality](report.md#scope-03-quality).

---

## Scope 04: Disposable Digest Browser Acceptance

**Status:** Not Started  
**Depends On:** Scope 03  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-007-08 Digest states are accessible and responsive
	Given a real authenticated keyboard or screen-reader user at 320 CSS pixels 200 percent zoom and reduced motion
	When current quiet stale first-use selected-date-empty read-error or unauthorized Digest state renders from the real reader
	Then headings semantic dates prose source links status and retry remain perceivable and operable without overlap
	And protected nodes are removed from both the visual and accessibility trees on unauthorized outcomes
```

### UI Scenario Matrix

| Journey | Real Setup | Assertions | Forbidden |
|---|---|---|---|
| Current/date boundary | Real DATE/TIMESTAMPTZ marker row | Exact prose/date/time/current/source | current date substitution |
| Quiet/stale | Real quiet and old rows, explicit threshold | quiet or degraded state with metadata/prose | empty/current drift |
| Empty/date miss | Zero rows and selected-date miss with history | distinct first-use versus selected-date empty | sample/error drift |
| Faults | Owned query/scan/decode/connection controls | HTTP 500, safe cause, Retry | empty/digest-derived fields |
| Unauthorized | Missing/expired/denied session | recovery/denial, zero protected nodes | existence/content leak |
| Accessibility | Keyboard, accessibility tree, 320px, 200%, reduced motion | logical order, one status, no clipping/scroll | pointer-only/color-only |

### Implementation Plan

1. Build a disposable Digest acceptance lane with isolated PostgreSQL, real generator/reader, real session login, and unique marker rows; never touch persistent dev/prod state or operate-plane telemetry.
2. Insert current, timezone-boundary, quiet, and stale rows using the actual schema; create successful zero-row/selected-date cases without mocks.
3. Use owned disposable fault controls at the connection/query/scan/decode boundaries; do not alter shared migrations, corrupt a shared schema, replace `DigestReader`, or intercept browser responses.
4. Drive `/digest` through a real browser and cookie jar; directly assert HTTP/DOM/accessibility mutual exclusion, Retry, date semantics, privacy, and source authorization.
5. Exercise keyboard, 320px, 200% zoom, reduced motion, source disclosure, date controls where implemented, and unauthorized protected-node removal.
6. Tear down rows, fault controls, browser state, and validate-plane evidence on success/failure; zero residue is required.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenario | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| DIGEST-S04-T01 | Unit | `unit` | Existing `internal/web/handler_test.go`; planned browser-helper policy test | SCN-002-007-08 | `TestDigestBrowserHelpersForbidDBMocksResponseInterceptionAuthInjectionBailoutAndProtectedLeakage` | `./smackerel.sh test unit --go` | No |
| DIGEST-S04-T02 | Integration red-to-green | `integration` | Planned `tests/integration/digest_disposable_acceptance_test.go` | SCN-002-007-08 | `TestPreFixDateRowRendersFalseEmptyAndPostFixEveryRealReaderStateIsTruthful` | `./smackerel.sh test integration` | Yes |
| DIGEST-S04-T03 | Regression API | `e2e-api` | Existing `tests/e2e/test_digest.sh`, `test_digest_quiet.sh`, `test_digest_pipeline.sh` | SCN-002-007-08 | `real Digest persistence API and server page remain coherent across current quiet stale empty and fault states` | `./smackerel.sh test e2e` | Yes |
| DIGEST-S04-T04 | Regression UI | `e2e-ui` | Planned `web/pwa/tests/digest_typed_read.spec.ts`, `digest_states.spec.ts` | SCN-002-007-08 | `real Digest states expose exact typed dates truthful content privacy and accessible recovery` | `./smackerel.sh test e2e-ui` | Yes |
| DIGEST-S04-T05 | Broader Regression UI | `e2e-ui` | Existing `web/pwa/tests/auth_login.spec.ts`, `unified_journey.spec.ts`, broader browser suite | SCN-002-007-01 through 08 | `Digest repair preserves authentication Today navigation Search Assistant and broader journeys` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-007-08 Digest states are accessible and responsive`: all real-reader states expose correct semantics and remain keyboard/screen-reader/mobile/zoom/reduced-motion operable without overlap. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] Disposable database/fault/browser/telemetry resources are isolated and leave zero residue. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] Implementation routing remains blocked until packet-local artifact lint and traceability guard are clean. Evidence: [report.md#planning-validation](report.md#planning-validation).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `DIGEST-S04-T01` unit browser-helper policy passes. Evidence: [report.md#digest-s04-t01](report.md#digest-s04-t01).
- [ ] `DIGEST-S04-T02` exact real-PostgreSQL false-empty red-to-green proof passes. Evidence: [report.md#digest-s04-t02](report.md#digest-s04-t02).
- [ ] `DIGEST-S04-T03` real e2e-api coherence matrix passes. Evidence: [report.md#digest-s04-t03](report.md#digest-s04-t03).
- [ ] `DIGEST-S04-T04` real e2e-ui Digest acceptance passes without database mocks, response interception, auth injection, or bailout. Evidence: [report.md#digest-s04-t04](report.md#digest-s04-t04).
- [ ] `DIGEST-S04-T05` broader browser regression passes. Evidence: [report.md#digest-s04-t05](report.md#digest-s04-t05).

#### Build Quality Gate

- [ ] Disposable teardown, regression-quality guards, database isolation, accessibility, privacy/no-fallback scans, repo-standard build/check/lint/format, artifact lint, traceability guard, and affected Digest/config/docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-04-quality](report.md#scope-04-quality).

## Planning Handoff Rule

This packet remains planning-only. Scope 01 requires foreign-owner contract resolution before implementation. Runtime implementation may proceed only after that contract is ratified and packet-local artifact lint and traceability guard are clean for the synchronized planning handoff.

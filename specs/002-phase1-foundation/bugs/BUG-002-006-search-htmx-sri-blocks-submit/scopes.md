# Scopes: BUG-002-006 Secure Progressive Search Submission

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Execution Outline

### Phase Order

1. **Scope 01 - Source-locked shared HTMX asset:** serve reviewed HTMX bytes from the product origin, tighten CSP, and prove one read plus one mutation canary before Search changes.
2. **Scope 02 - Semantic baseline Search:** make the native form and one typed server model authoritative for full-page and fragment outcomes.
3. **Scope 03 - Exactly-once enhancement:** enhance the form submit lifecycle without owning submission, duplication, validation, or terminal truth.
4. **Scope 04 - Disposable browser acceptance:** prove request cardinality plus visible DOM states with enhancement enabled and disabled on the real stack.

The dependency order is strict: the source-locked HTMX contract and shared-head canaries must pass before any Search browser proof.

### New Types And Signatures

- Same-origin immutable asset: `GET /web-assets/htmx-1.9.12.min.js`.
- `SearchState` closed vocabulary and concrete `SearchPageModel` / `SearchResultView` projection in `internal/web`.
- `SearchResults(http.ResponseWriter, *http.Request)` returns full HTML for baseline posts and the same outcome region for `HX-Request` posts.
- Same-origin `search-enhancement.js` observes HTMX lifecycle events but never calls `fetch`, `requestSubmit`, or an HTMX request API.

### Validation Checkpoints

- **After Scope 01:** locked-byte mutation fails, CSP remains strict, and existing read/mutation HTMX canaries work from the same asset.
- **After Scope 02:** baseline POST validates once, invokes `SearchEngine` at most once, and renders every server-owned terminal state without JavaScript.
- **After Scope 03:** Enter and pointer each produce one request; browser transition states remain mutually exclusive and privacy-safe.
- **After Scope 04:** real Playwright observes actual `/search` requests and DOM with no interception, auth injection, or bailout.

## Scope Inventory

| # | Scope | Depends On | Surfaces | Test Rows | Status |
|---|---|---|---|---:|---|
| 01 | Source-locked shared HTMX asset | None | embedded assets, shared head, CSP, read/mutation canaries | 5 | Not Started |
| 02 | Semantic baseline Search | 01 | template, handler, typed states, auth presentation | 5 | Not Started |
| 03 | Exactly-once enhancement | 02 | form HTMX attributes, lifecycle JS, telemetry | 5 | Not Started |
| 04 | Disposable browser acceptance | 03 | real browser, accessibility, all Search states | 5 | Not Started |

## Shared Head Infrastructure Impact Sweep

Changing `internal/web/templates.go::head` affects Search, Digest, Topics, Settings, Status, Knowledge, recommendation, notification, and admin server-rendered pages. The independent canaries are: a read-only Knowledge concepts interaction (`hx-get` in `internal/web/templates.go`, covered from `internal/web/handler_test.go`) and a Recommendations mutation interaction (`hx-post` in `internal/web/recommendations.go`, covered by `tests/e2e/recommendations_web_test.go`). `tests/e2e/test_web_ui.sh` remains the server-rendered HTTP canary. A failure in any canary blocks Search work.

### Shared-Head Rollback

- Roll back the embedded asset, static route, shared template declaration, and CSP as one atomic unit from the immediately preceding known-good revision.
- Never recover by dropping integrity, adding `unsafe-eval`, restoring an open CDN source, or retaining mixed external/self-hosted declarations.
- Because the prior Search is inert when HTMX is blocked, rollback may only make Search explicitly unavailable while unrelated pages recover; it is not a steady-state repair.
- Re-run the read and mutation canaries after rollback and after restoration; no database or business-data rollback is involved.

## Change Boundary

**Allowed:** `internal/web/templates.go`, focused embedded web assets and digest metadata, `internal/web/handler.go`, the static route/security header wiring in `internal/api/router.go`, focused existing/new tests, and directly affected web/security docs.

**Excluded:** ranking/search-engine business logic, JSON `/api/search` semantics, unrelated navigation, production data, deployment manifests, release-train config, other bug packets, `specs/079-prod-autonomous-supervisor/**`, and unrelated HTMX markup except canary compatibility. Collateral cleanup is opt-in only.

---

## Scope 01: Source-Locked Shared HTMX Asset

**Status:** Not Started  
**Depends On:** None  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-006-03 Integrity mismatch fails validation
	Given the embedded HTMX dependency bytes differ from the lock-derived digest or the shared head weakens its source policy
	When the source-lock and security-header contracts are checked
	Then validation fails before release
	And the accepted repair remains one same-origin versioned asset under strict CSP and byte-matching integrity
```

### Implementation Plan

1. Resolve the exact HTMX 1.9.12 browser artifact through the repository's pinned npm registry and lockfile; add no runtime download or extra package source.
2. Embed the reviewed bytes and lock-derived digest in the `internal/web` asset surface and serve `/web-assets/htmx-1.9.12.min.js` with JavaScript content type, `nosniff`, immutable cache identity, and strong content-derived ETag.
3. Replace the `unpkg.com` declaration in `internal/web/templates.go::head` with one same-origin script declaration whose optional SRI is generated/verified from those bytes.
4. Remove the external host from `internal/api/router.go::securityHeadersMiddleware`; retain `script-src 'self'` and existing reviewed inline hashes without wildcard, `unsafe-eval`, or bypass.
5. Add a one-byte mutation/stale-digest adversarial contract and duplicate-version/source scan.
6. Run the existing Knowledge read and Recommendations mutation canaries before any Search markup change; preserve their request and DOM semantics.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenario | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| SEARCH-S01-T01 | Unit | `unit` | Existing `internal/web/handler_test.go`; planned `internal/web/htmx_asset_contract_test.go` | SCN-002-006-03 | `TestHTMXAssetMatchesLockedDigestAndMutatedBytesOrWeakenedCSPFail` | `./smackerel.sh test unit --go` | No |
| SEARCH-S01-T02 | Integration | `integration` | Existing `tests/e2e/recommendations_web_test.go`; planned `tests/integration/web/shared_htmx_asset_test.go` | SCN-002-006-03 | `TestSharedHTMXAssetServesExactBytesAndSupportsReadAndMutationCanaries` | `./smackerel.sh test integration` | Yes |
| SEARCH-S01-T03 | Regression API | `e2e-api` | Existing `tests/e2e/test_web_ui.sh`; extend same file | SCN-002-006-03 | `shared HTMX asset has immutable headers strict CSP and no external source` | `./smackerel.sh test e2e` | Yes |
| SEARCH-S01-T04 | Regression UI | `e2e-ui` | Existing `web/pwa/tests/unified_journey.spec.ts`; planned `web/pwa/tests/server_rendered_htmx_canary.spec.ts` | SCN-002-006-03 | `same-origin HTMX loads and one read plus one mutation interaction remains operable` | `./smackerel.sh test e2e-ui` | Yes |
| SEARCH-S01-T05 | Broader Regression UI | `e2e-ui` | Existing server-rendered/PWA browser suite | SCN-002-006-03 | `shared head exposes exactly one source-locked HTMX version without console CSP or integrity errors` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-006-03 Integrity mismatch fails validation`: locked bytes pass, one-byte/stale-digest or CSP/source weakening fails, and the browser receives one same-origin immutable asset. Evidence: [report.md#scope-01-core](report.md#scope-01-core).
- [ ] Knowledge read and Recommendations mutation canaries pass before Search changes, and the atomic rollback/restore contract is proven. Evidence: [report.md#scope-01-core](report.md#scope-01-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `SEARCH-S01-T01` unit source-lock/security contract passes. Evidence: [report.md#search-s01-t01](report.md#search-s01-t01).
- [ ] `SEARCH-S01-T02` real integration read/mutation canary passes. Evidence: [report.md#search-s01-t02](report.md#search-s01-t02).
- [ ] `SEARCH-S01-T03` real e2e-api asset/header canary passes. Evidence: [report.md#search-s01-t03](report.md#search-s01-t03).
- [ ] `SEARCH-S01-T04` real e2e-ui shared-head canary passes without interception or auth injection. Evidence: [report.md#search-s01-t04](report.md#search-s01-t04).
- [ ] `SEARCH-S01-T05` broader browser regression passes. Evidence: [report.md#search-s01-t05](report.md#search-s01-t05).

#### Build Quality Gate

- [ ] Source locking, supply-chain allowlist, rollback/restore, repo-standard build/check/lint/format, artifact lint, traceability guard, console/CSP scan, and directly affected security docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-01-quality](report.md#scope-01-quality).

---

## Scope 02: Semantic Baseline Search

**Status:** Not Started  
**Depends On:** Scope 01  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-006-02 Pointer submission works without client enhancement
	Given an authenticated browser with JavaScript disabled or HTMX unavailable
	When the user enters a non-empty query and activates the semantic Search submit control
	Then one native POST /search completes through the real handler
	And a complete terminal page retains the query and recovery actions

Scenario: SCN-002-006-04 Blank query performs no search-domain work
	Given the query is empty, control-only, whitespace-only, or a mixture of whitespace and control characters
	When the user submits through the native form or HTMX path
	Then SearchEngine and every downstream search-domain dependency execute zero times
	And the HTTP layer returns or preserves accessible validation without representing a search result

Scenario: SCN-002-006-05 No match differs from request failure
	Given one authorized query completes with zero matches and another reaches a typed timeout or server failure
	When each baseline response renders
	Then zero matches has no error or retry language
	And each failure preserves the query and exposes its own actionable state

Scenario: SCN-002-006-06 Auth expiry requests re-authentication
	Given the browser session expires before the baseline form post
	When web authentication rejects the request
	Then the complete response presents re-authentication with safe route-only return context
	And it never claims no matches or includes the raw query in return metadata
```

### UI Scenario Matrix

| Scenario | Setup | Gesture | Required Visible State | Planned Test |
|---|---|---|---|---|
| No-enhancement result | JS disabled; seeded matching artifact | Pointer Search | Complete page, retained query, result count and real link | `search_progressive.spec.ts` |
| Blank/whitespace/control | JS disabled; native or HTMX path | Submit blank, whitespace-only, control-only, or mixed input | `Enter a search query`, field invalid/focused, zero SearchEngine/domain dispatch (HTTP 422 validation permitted) | `search_progressive.spec.ts` |
| Empty vs failure | Disposable empty corpus; owned timeout/error controls | Submit each | `No matches` differs from timeout/server Retry state | `search_states.spec.ts` |
| Expired session | Real expired session | Submit query | `Your session ended`, safe Sign in action, no query in return URL | `search_states.spec.ts` |

### Implementation Plan

1. Replace the standalone input in `internal/web/templates.go::search.html` with one labeled `<form method="post" action="/search" role="search">`, named required query field, and submit button; place HTMX attributes on the form only.
2. Add the closed `SearchState` and concrete `SearchPageModel` / `SearchResultView` projection in `internal/web`; no untyped map determines state.
3. Refactor `internal/web/handler.go::SearchResults` to parse and trim once, return 422 without `SearchEngine` for empty input, call the existing engine once otherwise, and map results/empty/degraded/timeout/server error explicitly.
4. Render a complete `search.html` for baseline requests and `results-partial.html` for `HX-Request`, both from the same model and real HTTP status.
5. Preserve route-owned authentication: baseline rejection renders complete recovery; HTMX rejection remains a 401 fragment; neither carries raw query in `next`.
6. Keep `/api/search`, ranking, persistence, and result content semantics unchanged; logs/metrics exclude query, excerpts, titles, and raw errors.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenarios | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| SEARCH-S02-T01 | Unit | `unit` | Existing `internal/web/handler_test.go`; planned focused model tests there | SCN-002-006-02, 04, 05, 06 | `TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix` | `./smackerel.sh test unit --go` | No |
| SEARCH-S02-T02 | Integration red-to-green | `integration` | Existing `tests/e2e/capture_process_search_test.go`; planned `tests/integration/web/search_baseline_test.go` | SCN-002-006-02, 04, 05 | `TestBaselineSearchWasInertBeforeFixAndNowCallsEngineOnceWithTypedOutcomes` | `./smackerel.sh test integration` | Yes |
| SEARCH-S02-T03 | Regression API | `e2e-api` | Existing `tests/e2e/test_web_ui.sh`; extend same file | SCN-002-006-02, 04, 05, 06 | `baseline POST search returns complete results validation empty failure and unauthorized documents` | `./smackerel.sh test e2e` | Yes |
| SEARCH-S02-T04 | Regression UI | `e2e-ui` | Existing `web/pwa/tests/auth_login.spec.ts`; planned `web/pwa/tests/search_progressive.spec.ts` | SCN-002-006-02, 04 | `pointer search works with JavaScript disabled and blank input performs no search-domain work` | `./smackerel.sh test e2e-ui` | Yes |
| SEARCH-S02-T05 | Broader Regression UI | `e2e-ui` | Planned `web/pwa/tests/search_states.spec.ts` plus existing auth journey | SCN-002-006-05, 06 | `baseline no-match failure and expired-session states remain mutually exclusive` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-006-02 Pointer submission works without client enhancement`: the native form posts once and renders a complete retained-query terminal page with JavaScript disabled. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] `SCN-002-006-04 Blank query performs no search-domain work`: empty, whitespace-only, control-only, and mixed input execute zero SearchEngine and downstream domain work while the HTTP layer may return accessible validation. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] `SCN-002-006-05 No match differs from request failure`: empty, timeout, and server failure remain mutually exclusive with truthful actions. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] `SCN-002-006-06 Auth expiry requests re-authentication`: expired auth never renders no-match and safe return omits the query. Evidence: [report.md#scope-02-core](report.md#scope-02-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `SEARCH-S02-T01` unit semantic-form/state matrix passes. Evidence: [report.md#search-s02-t01](report.md#search-s02-t01).
- [ ] `SEARCH-S02-T02` baseline inert red-to-green integration proof passes. Evidence: [report.md#search-s02-t02](report.md#search-s02-t02).
- [ ] `SEARCH-S02-T03` real e2e-api full-document matrix passes. Evidence: [report.md#search-s02-t03](report.md#search-s02-t03).
- [ ] `SEARCH-S02-T04` real JS-disabled e2e-ui baseline regression passes without interception or auth injection. Evidence: [report.md#search-s02-t04](report.md#search-s02-t04).
- [ ] `SEARCH-S02-T05` broader state/auth browser regression passes. Evidence: [report.md#search-s02-t05](report.md#search-s02-t05).

#### Build Quality Gate

- [ ] Baseline canary, privacy scan, repo-standard build/check/lint/format, artifact lint, traceability guard, regression-quality guards, and Search/auth docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-02-quality](report.md#scope-02-quality).

---

## Scope 03: Exactly-Once Search Enhancement

**Status:** Not Started  
**Depends On:** Scope 02  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-006-01 Keyboard search returns real results
	Given an authenticated user a non-empty query with live matches and the source-locked enhancement loaded
	When the user presses Enter once
	Then the browser emits exactly one real POST /search
	And Searching is replaced by a positive result count and live result links

Scenario: SCN-002-006-07 Degraded search remains honest
	Given a real search dependency is owned-degraded while verified partial rows remain available
	When enhanced Search completes
	Then Partial results names the unavailable capability and preserves operable verified rows
	And the valid session and available provenance remain intact
```

### UI Scenario Matrix

| Scenario | Setup | Gesture | Required Network / DOM | Planned Test |
|---|---|---|---|---|
| Enter exactly once | Source-locked HTMX; matching corpus | Type then Enter | One POST; `Searching` then result links | `search_progressive.spec.ts` |
| Pointer exactly once | Same setup | Click Search | One POST; same state sequence | `search_progressive.spec.ts` |
| Retry | Owned timeout/server failure | Click Retry once | One additional POST; stale state replaced | `search_states.spec.ts` |
| Degraded | Disposable owned partial-result control | Submit | `Partial results`, source limitation, verified links | `search_states.spec.ts` |

### Implementation Plan

1. Remove input-change autosubmit and custom Enter triggers; keep only HTMX enhancement on the semantic form submit event so native and enhanced paths share one browser algorithm.
2. Add same-origin embedded `search-enhancement.js`; observe `htmx:beforeRequest`, `afterRequest`, `responseError`, `sendError`, and `timeout` to set busy state, disable duplicate submit, restore controls, and project browser-only loading/network/retrying states.
3. Prohibit the enhancement from calling `fetch`, `requestSubmit`, `htmx.ajax`, or any request API; server validation and terminal state remain authoritative.
4. Replace stale rows/messages on each request; retain query/filters for retry without copying raw query into live announcements, storage, logs, metrics, or return URLs.
5. Add bounded `outcome` and `mode` telemetry plus duration/result count only; distinguish baseline and HTMX without private labels.
6. Use disposable owned dependency controls to produce timeout/server/degraded outcomes through real application seams, never Playwright interception.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenarios | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| SEARCH-S03-T01 | Unit | `unit` | Existing `internal/web/handler_test.go`; planned `internal/web/search_enhancement_contract_test.go` | SCN-002-006-01, 07 | `TestSearchEnhancementObservesLifecycleButCannotSubmitOrLeakQuery` | `./smackerel.sh test unit --go` | No |
| SEARCH-S03-T02 | Integration red-to-green | `integration` | Existing `tests/e2e/test_web_ui.sh`; planned `tests/integration/web/search_exactly_once_test.go` | SCN-002-006-01, 07 | `TestOldInputAndEnterTriggersCanDuplicateButFormEnhancementCallsSearchOnceAndPreservesDegradedState` | `./smackerel.sh test integration` | Yes |
| SEARCH-S03-T03 | Regression API | `e2e-api` | Existing `tests/e2e/capture_process_search_test.go`; planned enhanced `/search` API matrix | SCN-002-006-01, 07 | `HTMX request returns one typed result or degraded fragment with real status and provenance` | `./smackerel.sh test e2e` | Yes |
| SEARCH-S03-T04 | Regression UI | `e2e-ui` | Planned `web/pwa/tests/search_progressive.spec.ts` | SCN-002-006-01 | `Enter and pointer each issue exactly one real search request and render live results` | `./smackerel.sh test e2e-ui` | Yes |
| SEARCH-S03-T05 | Broader Regression UI | `e2e-ui` | Planned `web/pwa/tests/search_states.spec.ts` | SCN-002-006-07 | `retry timeout network server and degraded transitions replace stale state without session or privacy drift` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-006-01 Keyboard search returns real results`: Enter and pointer each produce one real request and loading resolves to real links. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] `SCN-002-006-07 Degraded search remains honest`: partial verified rows, limitation, provenance, and valid session remain visible together. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] Enhancement code cannot originate requests, and telemetry/browser state contains no raw query or result content. Evidence: [report.md#scope-03-core](report.md#scope-03-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `SEARCH-S03-T01` unit enhancement/privacy contract passes. Evidence: [report.md#search-s03-t01](report.md#search-s03-t01).
- [ ] `SEARCH-S03-T02` duplicate-trigger red-to-green integration proof passes. Evidence: [report.md#search-s03-t02](report.md#search-s03-t02).
- [ ] `SEARCH-S03-T03` real e2e-api fragment/degraded matrix passes. Evidence: [report.md#search-s03-t03](report.md#search-s03-t03).
- [ ] `SEARCH-S03-T04` real exactly-once e2e-ui regression passes without interception, auth injection, or bailout. Evidence: [report.md#search-s03-t04](report.md#search-s03-t04).
- [ ] `SEARCH-S03-T05` broader transition-state browser regression passes. Evidence: [report.md#search-s03-t05](report.md#search-s03-t05).

#### Build Quality Gate

- [ ] Enhancement canary, anti-request-origin static guard, privacy scan, repo-standard build/check/lint/format, artifact lint, traceability guard, regression-quality guards, and affected Search docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-03-quality](report.md#scope-03-quality).

---

## Scope 04: Disposable Search Browser Acceptance

**Status:** Not Started  
**Depends On:** Scope 03  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-006-08 Search is accessible and responsive
	Given an authenticated keyboard or screen-reader user at 320 CSS pixels 200 percent zoom and reduced motion
	When the user submits and reviews loading results empty filtered-empty unauthorized degraded timeout network or server error
	Then state changes are announced without raw query text
	And fields actions retries filters and result links remain ordered operable unclipped and non-overlapping
```

### UI Scenario Matrix

| Journey | Real Setup | Assertions | Forbidden |
|---|---|---|---|
| Enhanced matching Search | Disposable matching artifact | One request, announced loading/count, operable result | interception or duplicate request |
| No-JS Search | JavaScript disabled before navigation | Native post, retained query, complete terminal page | inert control |
| State matrix | Empty, filtered-empty, expired session, timeout, network, server, degraded controls | Exact mutually exclusive heading/action for each | generic empty/error collapse |
| Accessibility | 320px, 200% zoom, keyboard, accessibility tree, reduced motion | Logical focus, one live region, no horizontal scroll/overlap | pointer-only or color-only status |
| Privacy | Same journeys | No raw query/result content in live announcements, logs, metrics, storage, or safe return | private content leakage |

### Implementation Plan

1. Add one disposable server-rendered Search Playwright lane using the real stack, real session login, isolated corpus, browser requests, and DOM; no `page.route`, `context.route`, internal response stubs, direct cookie injection, or auth-state injection.
2. Seed uniquely named real artifacts through supported product paths and use owned disposable controls for empty, filtered-empty, degraded, timeout, network, server-error, and expired-session cases.
3. Count actual `/search` requests through browser observations that do not intercept or fulfill them; directly assert the terminal DOM and real result links.
4. Run enhanced and JavaScript-disabled paths, keyboard and pointer gestures, retry, Back/focus restoration, 320px, 200% zoom, reduced motion, and accessibility snapshots.
5. Verify console has no CSP/SRI errors and browser/client/telemetry surfaces contain no raw query or returned personal content.
6. Tear down corpus, dependency controls, browser state, and validate-plane artifacts on success or failure; zero residue is required.

### Test Plan

| ID | Test Type | Category | Existing Anchor / Planned Target | Scenario | Exact Test Title / Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| SEARCH-S04-T01 | Unit | `unit` | Existing `internal/web/handler_test.go`; planned browser-helper policy test | SCN-002-006-08 | `TestSearchBrowserHelpersForbidInterceptionAuthInjectionBailoutAndPrivateAnnouncements` | `./smackerel.sh test unit --go` | No |
| SEARCH-S04-T02 | Integration red-to-green | `integration` | Existing `tests/e2e/test_web_ui.sh`; planned disposable state harness | SCN-002-006-08 | `TestPreFixBrowserSendsZeroRequestWhenHTMXBlockedAndPostFixAllOwnedStatesRemainReachable` | `./smackerel.sh test integration` | Yes |
| SEARCH-S04-T03 | Regression API | `e2e-api` | Existing `tests/e2e/test_search.sh`, `test_search_empty.sh`, and `test_web_ui.sh` | SCN-002-006-08 | `real Search API and server-rendered outcome matrix remains coherent` | `./smackerel.sh test e2e` | Yes |
| SEARCH-S04-T04 | Regression UI | `e2e-ui` | Planned `web/pwa/tests/search_progressive.spec.ts` and `search_states.spec.ts` | SCN-002-006-08 | `real Search remains operable announced and non-overlapping with enhancement enabled and disabled` | `./smackerel.sh test e2e-ui` | Yes |
| SEARCH-S04-T05 | Broader Regression UI | `e2e-ui` | Existing `web/pwa/tests/auth_login.spec.ts`, `unified_journey.spec.ts`, and shared browser suite | SCN-002-006-01 through 08 | `Search repair preserves auth navigation shared HTMX and broader product journeys` | `./smackerel.sh test e2e-ui` | Yes |

### Adversarial Red-To-Green Proof

Before implementation, the exact disposable browser reproduction loads the current page under strict integrity enforcement, submits a non-empty query, and records zero `/search` requests because HTMX is blocked and no native form exists. The asset contract separately mutates one byte or digest and must fail. After implementation, the same real browser flow sends one request and renders the terminal DOM, while the intentional byte/digest mismatch still fails validation. Markup-presence checks, direct curl alone, intercepted `/search`, injected auth, or bailout returns cannot satisfy this proof.

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-002-006-08 Search is accessible and responsive`: all named states are announced without private content and controls/results remain keyboard-operable and non-overlapping at 320px/200% zoom/reduced motion. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] Disposable browser state, corpus, dependency controls, and telemetry are isolated and leave zero residue. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] Implementation routing remains blocked until packet-local artifact lint and traceability guard are clean. Evidence: [report.md#planning-validation](report.md#planning-validation).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `SEARCH-S04-T01` unit browser-helper policy passes. Evidence: [report.md#search-s04-t01](report.md#search-s04-t01).
- [ ] `SEARCH-S04-T02` exact browser zero-request red-to-green integration proof passes. Evidence: [report.md#search-s04-t02](report.md#search-s04-t02).
- [ ] `SEARCH-S04-T03` real e2e-api outcome matrix passes. Evidence: [report.md#search-s04-t03](report.md#search-s04-t03).
- [ ] `SEARCH-S04-T04` real e2e-ui Search acceptance passes without interception, auth injection, or bailout. Evidence: [report.md#search-s04-t04](report.md#search-s04-t04).
- [ ] `SEARCH-S04-T05` broader browser regression passes. Evidence: [report.md#search-s04-t05](report.md#search-s04-t05).

#### Build Quality Gate

- [ ] Disposable teardown, regression-quality guards, CSP/SRI console checks, accessibility, privacy scan, repo-standard build/check/lint/format, artifact lint, traceability guard, and affected web/security docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-04-quality](report.md#scope-04-quality).

## Planning Handoff Rule

This packet remains planning-only. Implementation may be routed only after the final `scenario-manifest.json`, `test-plan.json`, report template, and user-validation baseline match these four scopes and both packet-local planning validators execute cleanly.

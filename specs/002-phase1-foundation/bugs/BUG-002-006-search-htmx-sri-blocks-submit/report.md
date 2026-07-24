# Report: BUG-002-006 Secure Progressive Search Submission

Links: [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json) | [uservalidation.md](uservalidation.md)

## Summary

On 2026-07-23, `bubbles.plan` replaced the preliminary single-scope handoff with four dependency-ordered executable scopes: source-locked shared HTMX delivery, semantic baseline Search, exactly-once enhancement, and disposable real-browser acceptance. Shared-head canaries and atomic rollback precede Search browser proof.

No source, test, dependency, configuration, runtime, production data, requirements, design, certification, commit, push, or deployment mutation is claimed.

## Completion Statement

Planning-owned artifacts are complete for implementation routing only after packet-local artifact lint and traceability guard pass. The bug remains `in_progress`; no runtime repair is complete.

## Implementation Evidence — bubbles.implement (2026-07-24)

**Phase:** implement · **Status after run:** `in_progress` (packet NOT done). Scope 02's product-critical core (progressive-enhancement semantic baseline + zero-domain-work validation gate + typed state model + `SearchExecutor` seam) is implemented and unit-validated on the real Go toolchain. Scope 01 (self-hosted source-locked HTMX asset) is **blocked** on an unobtainable upstream artifact (see Blocker), and every live-stack row (integration / e2e / e2e-ui) is **deferred**. No commit, push, or deployment is claimed.

**Status reconciliation (2026-07-24):** The packet's terminal-for-mode status is `blocked` (recorded in `state.json` with an operator-actionable `blockedReason`); the blocker is the unobtainable Scope 01 HTMX 1.9.12 artifact documented below. `in_progress` above described the implement run's outcome, not a terminal state — the packet cannot advance further without the operator vendoring the HTMX artifact. Scope 02's core is implemented and unit-verified and is independently correct.

### Implemented (owned files only)

- **`internal/web/search_model.go`** (NEW): closed `SearchState` vocabulary (`ready`, `validation`, `results`, `empty`, `filtered_empty`, `degraded`, `unauthorized`, `timeout`, `server_error`), typed `SearchPageModel` / `SearchResultView` / `KnowledgeMatchView`, and the narrow `SearchExecutor` interface (the zero-domain-work observation seam; production uses the real `*api.SearchEngine`).
- **`internal/web/handler.go`**: `SearchResults` now (1) edge-trims Unicode whitespace+control via `trimSearchQuery`, (2) classifies blank/control/whitespace-only/mixed input and returns **HTTP 422 with ZERO `SearchEngine` dispatch** (`isBlankQuery` gate before any `executor().Search`), (3) calls the injected `SearchExecutor` **exactly once** for a searchable query, (4) projects one mutually-exclusive terminal state, and (5) renders a **complete page for a baseline (non-HTMX) request** vs the outcome **fragment for an `HX-Request`**, both buffered so a template failure cannot emit a partial/wrong-status body. `SearchPage` renders the complete `ready` page. `searchKnowledgeMatch` retyped to `*KnowledgeMatchView`. Added `SearchExecutorOverride` seam field + `executor()`.
- **`internal/web/templates.go`**: `search.html` replaced the standalone HTMX `<input>` with a **semantic `<form method="post" action="/search" role="search">`** (label + required `name="query"` input + `type="submit"` button) with the `hx-*` attributes on the **form submit event** (not a dual `input changed`/`keyup Enter` trigger). This makes Search submit via native full-page POST **even when HTMX is blocked** — the reported zero-request symptom. Added the shared `{{search-outcome}}` state region (`data-search-state` / `data-search-result-count`); `results-partial.html` now delegates to it so baseline and fragment render one closed vocabulary.
- **`internal/web/handler_test.go`**: NEW `TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix` (SEARCH-S02-T01) + `countingSearchExecutor` fake proving: semantic form markup on GET /, 422 + zero dispatch for six blank/control/whitespace/unicode-space inputs on BOTH native and HTMX paths, exactly-one dispatch + edge-trimmed query + retained query for a real query, full-page (baseline) vs fragment (HTMX) selection, and distinct `empty` vs `server_error` states.

### SEARCH-S02-T01

**Phase:** implement · **Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix|TestSCN002033_WebSearchPage|TestSearchResults_KnowledgeMatchTemplate|TestSearchPage_NilPool'` · **Exit Code:** 0 · **Claim Source:** executed

```text
=== RUN   TestSearchPage_NilPool
--- PASS: TestSearchPage_NilPool (0.01s)
=== RUN   TestSCN002033_WebSearchPage
--- PASS: TestSCN002033_WebSearchPage (0.00s)
=== RUN   TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix
2026/07/24 20:54:39 ERROR web search failed error="search engine boom"
--- PASS: TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix (0.05s)
=== RUN   TestSearchResults_KnowledgeMatchTemplate
--- PASS: TestSearchResults_KnowledgeMatchTemplate (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/web     0.416s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

The `ERROR web search failed error="search engine boom"` line is the expected `server_error`-case log emitted by the handler for the injected failure — it is asserted by the test, not a failure. A prior non-verbose full-suite run (`go test -run '<same>' -count=1 ./...`) also reported `ok github.com/smackerel/smackerel/internal/web 0.222s` + `[go-unit] go test ./... finished OK`, proving no regression to the existing web/search/template tests.

### Scope 02 Quality

**Phase:** implement · **Commands:** `./smackerel.sh check` (exit 0) and `./smackerel.sh lint` (exit 0) · **Claim Source:** executed

```text
$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.1942601 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0

$ ./smackerel.sh lint    (go vet ./... silent-pass, then python + web asset validation)
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
=== Validating JS syntax ===
  OK: web/pwa/app.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT_EXIT=0
```

### Blocker: Scope 01 self-hosted HTMX artifact (unobtainable)

The design's chosen repair for the SRI mismatch is to **self-host the reviewed HTMX 1.9.12 bytes** under the product origin (`/web-assets/htmx-1.9.12.min.js`) and tighten CSP to `'self'`. That deliverable is **blocked** in this environment:

- No HTMX artifact is vendored anywhere in the repo (`file_search **/htmx*` → none; `grep htmx.org@1.9.12` finds only the `unpkg` reference in `templates.go`/`router.go`/docs).
- There is no pinned npm registry + lockfile in-tree that resolves `htmx.org@1.9.12` (no root `package.json`; `web/pwa/package.json` pins only `@playwright/test`).
- The bytes cannot be fetched (supply-chain source-locking + terminal-discipline forbid an ad-hoc CDN/`npm install` fetch, and there is no `./smackerel.sh` command to vendor it), and a ~48 KB minified artifact cannot be hand-fabricated without violating the anti-fabrication policy.

Therefore Scope 01's asset embed/route/CSP-tighten is **not implemented**, and the CSP/asset wiring in `internal/api/router.go` was deliberately **left untouched** — changing CSP to point at a non-existent same-origin asset would break HTMX worse than the current state. Correcting the declared SRI hash is equally blocked (it also requires the real bytes/true digest). Every enhanced-browser proof (Scope 03/04 e2e-ui that needs HTMX to execute) is blocked downstream of this.

### Deferred (not run this session)

- All live-stack rows: SEARCH-S01-T02/T03/T04/T05, SEARCH-S02-T02/T03/T04/T05, SEARCH-S03-*, SEARCH-S04-* (`test integration` / `test e2e` / `test e2e-ui`). These require the disposable Docker stack (heavy build, host-OOM + concurrent-Docker hazard) and, for the enhanced paths, a working same-origin HTMX (blocked above). The JS-disabled baseline e2e-ui does not need HTMX but still needs the live stack; deferred with the rest.
- Scope 03 `search-enhancement.js` (lifecycle-only, no `fetch`/`requestSubmit`) and its contract test: not authored this session (its serving path depends on Scope 01's asset route; its exactly-once browser proof depends on HTMX executing).

### Coordination Residuals

None consumed. The fix touched only search-owned files (`internal/web/search_model.go`, `internal/web/handler.go`, `internal/web/templates.go`, `internal/web/handler_test.go`). The forbidden concurrent-edit set (`config/smackerel.yaml`, `internal/config/config.go`, `internal/config/validate_test.go`, `internal/telegram/assistant_adapter/callbacks.go`, `scripts/commands/config.sh`, `docs/Development.md`, `specs/079-*`, `specs/107-*`) was **not touched**. No git operation was performed.


## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** Search renders; the browser blocks HTMX for a wrong SHA-384 SRI value; entering a query sends zero `/search` requests.
- **Evidence status:** no browser console, network trace, DOM snapshot, or command output was captured here.

## Decision Record

- The design-owned same-origin, source-locked HTMX approach is implemented first because the shared head has high fan-out.
- `internal/web/handler_test.go`, `tests/e2e/recommendations_web_test.go`, and `tests/e2e/test_web_ui.sh` are existing read/mutation/server-rendered canary anchors; future tests are recorded separately as planned targets.
- The semantic native form and typed server model precede enhancement; enhancement observes lifecycle and cannot originate requests.
- The adversarial proof includes both a one-byte/stale-digest release failure and the exact pre-fix zero-request browser reproduction.
- Every runtime scope has five Test Plan rows and five matching test-evidence DoD items.
- Real Playwright uses the disposable stack and real session, with no request interception, auth injection, response stubbing, or bailout.
- No stress row is planned because this bug packet defines no latency, throughput, or availability SLA.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

No implementation or behavior test was run during planning, and no runtime result is claimed. The planned execution matrix is in [test-plan.json](test-plan.json).

## Planning Validation

### Artifact Lint

**Phase:** plan  
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation/bugs/BUG-002-006-search-htmx-sri-blocks-submit`  
**Exit Code:** 0  
**Claim Source:** executed

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ report.md contains required Summary, Completion Statement, and Test Evidence sections
✅ Anti-Fabrication Evidence Checks passed
Artifact lint PASSED.
```

### Traceability Guard

**Phase:** plan  
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation/bugs/BUG-002-006-search-htmx-sri-blocks-submit`  
**Exit Code:** 0  
**Claim Source:** executed

```text
✅ scenario-manifest.json covers 8 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist
✅ Scope 01 scenario maps to concrete test file: internal/web/handler_test.go
✅ Scope 02 scenarios map to concrete test file: internal/web/handler_test.go
✅ Scope 03 scenarios map to concrete test file: internal/web/handler_test.go
✅ Scope 04 scenario maps to concrete test file: internal/web/handler_test.go
✅ DoD fidelity: 8 scenarios checked, 8 mapped to DoD, 0 unmapped
ℹ️  Scenarios checked: 8
ℹ️  Test rows checked: 24
ℹ️  Scenario-to-row mappings: 8
ℹ️  Concrete test file references: 8
ℹ️  Report evidence references: 8
ℹ️  Edge confidence: declared=9 inferred=0 ambiguous=7
RESULT: PASSED (0 warnings)
```

## Uncertainty Declarations

- No before-fix browser/network execution exists; Scope 04 requires the exact strict-integrity zero-request reproduction before implementation.
- No after-fix behavior verification exists.
- The disposable degraded/timeout controls are implementation targets that harden must verify against existing test-stack seams.
- Planned test paths are executable handoff targets, not claims that those files already exist.

## Scenario Contract Evidence

The eight scenarios are assigned to Scopes 01-04 in [scenario-manifest.json](scenario-manifest.json). Existing canaries are linked; not-yet-authored targets use `plannedTests`; evidence references remain empty until execution.

### Existing Canary Anchors

The report references the concrete planning anchors used by traceability: `internal/web/handler_test.go`, `tests/e2e/recommendations_web_test.go`, `tests/e2e/test_web_ui.sh`, `tests/e2e/test_search_empty.sh`, `tests/e2e/auth/browser_login_test.go`, and `tests/e2e/capture_process_search_test.go`.

## Coverage Report

Planning covers source integrity/CSP, shared HTMX read/mutation compatibility, no-JavaScript submission, validation, results, empty, filtered-empty, unauthorized, timeout, network, server error, degraded results, request cardinality, retry, privacy, responsive layout, keyboard, screen reader, and reduced motion. No runtime coverage percentage is claimed.

## Lint/Quality

Only actual packet-local validator outcomes are recorded after execution.

## Spot-Check Recommendations

- Harden must verify the lock-derived HTMX asset path/digest and disposable failure controls against repository reality.
- Test must run anti-interception and bugfix regression-quality guards over all planned Playwright files.
- Validate must inspect shared-head rollback/restore and zero-residue disposable teardown.

## Validation Summary

Planning validation only. No state transition or certification is requested.

## Audit Verdict

Not audited. No terminal verdict is claimed.

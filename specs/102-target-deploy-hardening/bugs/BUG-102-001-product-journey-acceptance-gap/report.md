# Report: [BUG-102-001] Product Journey Acceptance Gap

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning artifacts only were initialized on 2026-07-23. No product source, adapter, host, browser, test, production, commit, push, or deployment mutation occurred.

## Completion Statement

Incomplete and non-terminal. Status remains `in_progress`; dependency fixes, spec 104 Scope 8, design/planning, implementation, adapter consumption, testing, validation, and audit are incomplete.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** strict infrastructure acceptance passes while Graph 404, modern auth, Search, Digest, and Assistant user journeys fail.
- **Evidence status:** no adapter, host, HTTP, browser, or command output was captured here.

## Decision Record

- Product behavior assertions belong in Smackerel; adapter acceptance consumes them.
- The production synthetic is read-only and uses an operator-provisioned identity.
- Required journey failure always rejects acceptance with a closed code.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

**Phase:** planning  
**Command:** none  
**Exit Code:** not applicable  
**Claim Source:** not-run

No test or deployment result is claimed.

## Uncertainty Declarations

- Final journey/result schema and required/optional policy are design-owned.
- Dependency implementations and spec 104 Scope 8 are not validated by this invocation.
- No adapter repository inspection or mutation occurred.

## Scenario Contract Evidence

Reconciled in [scenario-manifest.json](scenario-manifest.json). The nine requirement scenarios plus three plan-only stable overlay scenarios map uniquely to the five-scope DAG. Future implementation-owned files are `plannedTests`, not fabricated existing links; evidence references remain empty.

## Planned Evidence Anchors

Every Test Plan command runs through `./smackerel.sh`. Planned source/test locations are enumerated in `scopes.md` and `test-plan.json`; those future files are not claimed to exist. Dependency-owner evidence remains an independent prerequisite and is never replaced by aggregate acceptance.

## Validation Summary

No completion validation or certification was performed.

## Audit Verdict

Not audited. No deployment acceptance verdict is claimed.

---

## Fault-Registry Foundation Implementation (bubbles.implement, 2026-07-25)

**Phase:** implement  
**Claim Source:** executed (this session)  
**Scope delivered:** SCOPE-01 production-inert, test-only, machine-readable fault-profile REGISTRY FOUNDATION only (JOURNEY-016 / SCN-102-001-12; TP-102-01-07, TP-102-01-08). The remaining SCOPE-01 contract foundation (manifest/policy/result/evidence schemas, failure registry, read-only guard, reducer, validator — TP-102-01-01..06 / SCN-102-001-07) and all live-stack product-journey acceptance execution (SCOPE-02..05) are DEFERRED. No Docker stack was brought up; no concurrent/dirty file was touched; no commit or push occurred.

### IMPLEMENTED

New, disjoint files (each under a new path, none in the concurrent working-tree set):

| File | Role |
|---|---|
| `config/acceptance/fault-profiles.v1.yaml` | Machine-readable registry artifact. Test-only envelope (`version: v1`, `posture: test`, `productionExposure: forbidden`) and 15 profiles: the 8 Assistant fault profiles BUG-073-006 SCOPE-01 consumes by immutable `stableId` (`auth_401`, `access_403`, `rate_limited`, `provider_unavailable`, `server_error`, `timeout`, `network`, `schema_decode`) plus 7 profiles for the other required journeys (session, search, digest, graph, recommendations, cards, synthesis). Every profile carries the closed nine-field schema. |
| `config/acceptance/fault-profiles.v1.schema.json` | Draft 2020-12 JSON Schema companion. Closed (`additionalProperties: false`); `version`/`posture`/`productionExposure` const; all nine profile fields required; `journey` a closed enum; `noFirstPartyInterception` const `true`. |
| `internal/acceptance/fault_profile_registry.go` | Package `acceptance` loader + production-inert guard + consumer lookup. |
| `internal/acceptance/fault_profile_registry_test.go` | TP-102-01-07 unit test. |
| `internal/acceptance/fault_profile_production_inert_test.go` | TP-102-01-08 unit test. |

Mechanism:

- **Closed schema.** `ParseRegistry` strict-decodes YAML with `KnownFields(true)` (rejecting unknown fields) and validates that every profile declares all nine fields non-empty, targets a journey in the closed set, and declares `noFirstPartyInterception: true`. An omitted or `false` no-interception assertion, a missing/blank field, an unknown journey, a duplicate `stableId`, or an empty profile set each fails closed with a distinct typed error. `LoadCanonicalRegistry` additionally validates the committed registry against the JSON Schema (`santhosh-tekuri/jsonschema/v6`).
- **Production-inert guard.** `Resolve(posture, stableId)` is the single lookup/activation gate: `PostureProduction` returns `ErrFaultInertInProduction` for *every* `stableId` (a production build/config can never activate a fault), any non-test posture returns `ErrUnknownPosture`, and a registry declaring a production posture or `productionExposure` other than `forbidden` fails to load (`ErrProductionExposureDeclared`). `AssertNoFaultControlInProductionSurface` refuses any production route/config/request/UI descriptor carrying a fault selector/trigger/profile-control token, while leaving generic words that merely equal a `stableId` (for example `network`, `timeout`) unflagged.
- **Consumer lookup by stableId.** Under `PostureTest`, `Resolve` returns the exact profile; an unknown/missing `stableId` returns `ErrUnknownProfile` — no silent fallback, no inline fabrication. This is the contract BUG-073-006 SCOPE-01 consumes for its 8 Assistant faults.

### TEST-EVIDENCE

**Unit (TP-102-01-07 + TP-102-01-08):**

```
$ ./smackerel.sh test unit --go --go-run 'TestFaultProfileRegistryRequiresEveryDeclaredFieldAndRejectsFirstPartyInterception|TestProductionRoutesConfigRequestsAndUIExposeNoFaultSelectorOrTrigger'
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.374s [no tests to run]
?       github.com/smackerel/smackerel/cmd/web-assistant-codegen        [no test files]
ok      github.com/smackerel/smackerel/internal/acceptance      0.199s
ok      github.com/smackerel/smackerel/internal/agent   0.152s [no tests to run]
ok      github.com/smackerel/smackerel/internal/agent/render    0.035s [no tests to run]
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

The `internal/acceptance` line is `ok ... 0.199s` with NO `[no tests to run]`, i.e. the two `--go-run`-matched tests (TP-102-01-07, TP-102-01-08, with all adversarial subtests) executed and passed; a failing or uncompilable package would exit non-zero.

**Check (config/scenario drift — home path redacted):**

```
$ ./smackerel.sh check
config-validate: config/generated/dev.env.tmp.<n> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

**Lint (`go vet ./...` + python/web validators):**

```
$ ./smackerel.sh lint
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/extension/background.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT_EXIT=0
```

`go vet ./...` produced no findings (LINT_EXIT=0 confirms the whole `lint` chain — including go vet over the new package — passed). `gofmt --check` flags only two pre-existing/concurrent files (`internal/api/graphapi/activation.go`, `internal/web/handler_test.go`), never the three new files; the new package is gofmt-clean.

### DEFERRED (live-stack acceptance execution — Docker/live, NOT run)

The immutable product-journey ACCEPTANCE producer is off-traffic and LIVE; per the operator constraint no Docker stack was brought up, so every live Test Plan row and DoD item stays `[ ]`:

- SCOPE-02..05 live-stack journeys (Session, Search, Digest, Assistant, Wiki/Graph, Recommendations, Cards, Synthesis, aggregate release artifact and adapter handoff) — real off-traffic Playwright/API observation against a deployed candidate.
- SCOPE-01 remainder: manifest/policy/result/evidence schemas, failure registry, read-only guard, reducer, validator (TP-102-01-01..06 / SCN-102-001-07).

## Failure Registry + Read-Only Guard (Part 1, bubbles.implement, 2026-07-26)

This increment lands the two unit-verifiable SCOPE-01 contract-foundation
components that the 2026-07-25 section above listed as deferred: the closed
`E102-JOURNEY-*` failure registry (TP-102-01-04 / `SCN-102-001-07`) and the
production read-only static guard (TP-102-01-05 / `SCN-102-001-07`). It is
contract-only and unit-only: **no Docker stack was brought up, no live
integration/e2e test was run, and no concurrent `smackerel-test-*` stack was
touched or torn down** (baseline and final `docker ps -a --filter name=smackerel`
both = 0 containers; the concurrent spec-106 lane owns that stack).

### IMPLEMENTED (four files under `internal/acceptance/`, module `github.com/smackerel/smackerel`)

- `internal/acceptance/failure_registry.go` — the CLOSED product-journey
  acceptance vocabularies: aggregate verdicts (`accepted`, `accepted-degraded`,
  `blocked-prerequisite`, `rejected`, `contract-invalid`, `timed-out`), journey
  outcomes (`passed`, `allowed-empty`, `allowed-quiet`, `allowed-optional`,
  `allowed-degraded`, `failed`, `blocked`, `timed-out`, `not-evaluated`), and the
  full `E102-JOURNEY-*` code set from design.md `## Closed Failure-Code Registry`
  as an immutable `map[FailureCode]FailureMeta{Category, Owner}`. Each code maps
  to exactly one category (the design error family) and one owner (the owning
  journey group, or `acceptance-contract` for the contract-integrity family).
  `Validate()` fails closed (`ErrFailureRegistryInvalid` → contract-invalid) on an
  empty/malformed code, an unknown category or owner, a duplicate code, a
  category filed against the wrong family prefix, or an owner that is not the
  category's canonical owner. `LookupFailure` returns `(meta, ok)` — an unknown or
  unvalidated lookup is not-ok, never a guessed default. The one cross-cutting
  stem (`E102-JOURNEY-CONTRACT-CAPABILITY-*` vs `E102-JOURNEY-CONTRACT-*`) is
  disambiguated by ordered longest-prefix matching.
- `internal/acceptance/read_only_guard.go` — the production read-only static
  guard. `ScanProductionSurface` fails closed over a value-safe surface
  (classified routes + selectors + evidence-field identifiers + raw runner-source
  text) and rejects: an unclassified route, an unknown/production-forbidden
  side-effect class, canonical-classification drift, a mutating HTTP method, a
  POST that is neither `session-establish` nor `read-compute`, a state-changing
  selector, request interception (`page.route`/`context.route`/`route.fulfill`/…),
  credential/cookie injection, a direct datastore read, a service-container exec,
  a concrete target literal (URL/host/tailnet/IPv4/operator-path/private-key), and
  a forbidden evidence field. Each rejection carries one closed contract code
  (`E102-JOURNEY-CONTRACT-UNSAFE-MUTATION` or `-EVIDENCE-UNSAFE`) declared in the
  registry; the offending raw value is never echoed.
- `internal/acceptance/failure_registry_test.go` — TP-102-01-04
  (`TestEveryFailureCodeHasOneCategoryAndOwner`).
- `internal/acceptance/read_only_guard_test.go` — TP-102-01-05
  (`TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals`).

DEFERRED to Part 2 (not claimed here): the SCOPE-01 manifest/policy/result/
evidence schemas, the deterministic reducer, and the result validator
(TP-102-01-01/02/03/06) plus the whole `SCN-102-001-07` core outcome, and all
SCOPE-02..05 live-stack journeys.

### TEST-EVIDENCE (current session, 2026-07-26)

**Format (`./smackerel.sh format --check`) — my four files are gofmt-clean.**
The first run flagged one of my files; I formatted ONLY that file with host
`gofmt` (`go version go1.25.10 linux/amd64`, identical to the go-tooling
container) — never the in-place repo `format`, which would rewrite the
concurrent-locked `internal/web/handler_test.go`. The re-run flags only two
pre-existing/concurrent files and none of mine:

```
$ ./smackerel.sh format --check        # run 1 (before formatting my file)
internal/acceptance/read_only_guard_test.go
internal/api/graphapi/activation.go
internal/web/handler_test.go
FORMAT_CHECK_EXIT=1

$ gofmt -w internal/acceptance/read_only_guard_test.go   # my own new file only
$ git status --short internal/acceptance/                 # still only my 4 files
?? internal/acceptance/failure_registry.go
?? internal/acceptance/failure_registry_test.go
?? internal/acceptance/read_only_guard.go
?? internal/acceptance/read_only_guard_test.go

$ ./smackerel.sh format --check        # run 2 (after)
internal/api/graphapi/activation.go
internal/web/handler_test.go
FORMAT_CHECK_EXIT=1
```

The residual exit 1 is entirely the two pre-existing/concurrent files
(`internal/api/graphapi/activation.go`, and the concurrent-locked
`internal/web/handler_test.go`); all four `internal/acceptance/` files are
absent from the list, i.e. gofmt-clean.

**Check (`./smackerel.sh check`) — exit 0 (home path redacted per no-PII policy):**

```
$ ./smackerel.sh check
config-validate: <repo>/config/generated/dev.env.tmp.<n> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

**Lint (`./smackerel.sh lint`) — exit 0.** `go vet ./...` (go-lint.sh) ran first
and produced NO output (silent = zero findings over the new `internal/acceptance`
package); python `ruff` and web validation follow:

```
$ ./smackerel.sh lint
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT_EXIT=0
```

**Unit (`./smackerel.sh test unit --go --go-run '<TP-102-01-04>|<TP-102-01-05>'`)
— exit 0; `internal/acceptance` executed and passed.** The `--go-run` selector
runs only my two tests (with every adversarial subtest); every other package —
including the orthogonal `internal/docfreshness` doc-freshness test on the
concurrent-locked `docs/Development.md` — reports `[no tests to run]`, so that
known debt is filtered out and never fails this run:

```
$ ./smackerel.sh test unit --go --go-run 'TestEveryFailureCodeHasOneCategoryAndOwner|TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals'
+ go test -run 'TestEveryFailureCodeHasOneCategoryAndOwner|TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals' ./...
[go-unit] applying -run selector: TestEveryFailureCodeHasOneCategoryAndOwner|TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals
ok      github.com/smackerel/smackerel/cmd/alertmanager-ntfy-bridge     0.010s [no tests to run]
ok      github.com/smackerel/smackerel/internal/acceptance      0.013s
ok      github.com/smackerel/smackerel/internal/docfreshness    0.008s [no tests to run]
[go-unit] go test ./... finished OK
FINAL_UNIT_EXIT=0
```

The `internal/acceptance` line is `ok ... 0.013s` with NO `[no tests to run]` —
i.e. `TestEveryFailureCodeHasOneCategoryAndOwner` and
`TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals` (and
all their adversarial subtests) executed and passed; a failing or uncompilable
package would exit non-zero.

### RED→GREEN adversarial proof (canaries genuinely detect a permissive impl)

The adversarial subtests are not tautological. Each asserts a specific
fail-closed outcome, so a permissive implementation makes them RED. This was
proven empirically: I temporarily neutralized ONE check in each file — the
duplicate-code branch in `FailureRegistry.Validate()` and the
state-changing-selector branch in `scanSelectors()` — and re-ran the two tests.
Exactly the two matching canaries turned RED:

```
--- FAIL: TestEveryFailureCodeHasOneCategoryAndOwner (0.00s)
    --- FAIL: TestEveryFailureCodeHasOneCategoryAndOwner/adversarial:_duplicate_code (0.00s)
--- FAIL: TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals (0.00s)
    --- FAIL: TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals/adversarial:_state-changing_selector (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/acceptance      0.019s
```

Both neutralizations were then reverted (a `grep -rn "TEMP-CANARY" internal/acceptance/`
returns empty) and the final GREEN run above (`ok internal/acceptance 0.013s`,
`FINAL_UNIT_EXIT=0`) confirms the restored, correct implementation passes.

- **TP-102-01-04** (`TestEveryFailureCodeHasOneCategoryAndOwner`): asserts the
  canonical registry validates and every closed code has exactly one closed
  category and its category's canonical owner; unknown-code lookup is not-ok;
  and independent canaries (duplicate code, ownerless code, owner-mismatch,
  category-mismatch, unknown category, code outside every family prefix, empty
  registry) EACH make `Validate()` return `errors.Is(ErrFailureRegistryInvalid)`
  (contract-invalid). RED-against-permissive proven above for the duplicate-code
  canary; the other canaries fail identically against a permissive `Validate()`.
- **TP-102-01-05**
  (`TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals`):
  a valid read-only surface passes; then 14 independent canaries — state-changing
  selector, unclassified route, non-session/non-read-compute POST, mutating
  method, production-forbidden `fixture-write` class, canonical drift, request
  interception, credential/cookie injection, direct datastore read,
  service-container exec, target literal (runner source, IPv4, route template),
  and forbidden evidence field — EACH turn the fixture red with the expected
  closed `E102-JOURNEY-CONTRACT-UNSAFE-MUTATION` or `-EVIDENCE-UNSAFE` code, and
  every emitted code is confirmed registry-known. RED-against-permissive proven
  above for the state-changing-selector canary; a permissive guard returning nil
  fails every one.

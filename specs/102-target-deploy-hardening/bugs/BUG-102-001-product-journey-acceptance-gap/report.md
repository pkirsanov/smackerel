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

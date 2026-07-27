# Report: [BUG-080-001] Graph API Fails Soft Into Runtime Disablement

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning artifacts only were initialized on 2026-07-23. No source, secret, config generation, host, operator deploy repository, test, production, commit, push, or deployment mutation occurred. _(Superseded — an interim fail-soft unit core landed on 2026-07-24 and the fail-soft activation was then WIRED into the runtime and live-integration-proven; see "## Interim Fail-Soft Runtime-Disable Core" and "## Runtime Wiring And Live-Integration Proof" below.)_

## Completion Statement

Incomplete and non-terminal. Status is `blocked`. The fail-soft graph-API activation foundation (SCOPE-01) is WIRED into the runtime (`cmd/core/wiring.go`, `internal/api/router.go`, `internal/api/health.go`) and live-integration-proven (`tests/integration/graphapi/activation_test.go`: DISABLED → all eight canonical graph paths present as a typed HTTP 503 `capability_disabled` and the service keeps serving; ENABLED → `GET /api/topics` 200 over real PostgreSQL) — see "## Runtime Wiring And Live-Integration Proof" below. SCOPE-02 (authorized family reads + the operator/grant-holder/ungranted global-corpus grant matrix SCN-080-001-09), SCOPE-03 (product synthetic, readiness, content-free telemetry, stress), and SCOPE-04 (Wiki/Graph e2e-ui + accessibility) remain deferred/blocked; SCOPE-01's own e2e-api regression rows and manifest-tamper row are also deferred. Validation and audit are unclaimed.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** Graph cursor-secret indirection resolves empty; core warns and leaves handlers nil; topics/people/places/time/edges 404 while static Wiki and strict deployment verification pass.
- **Evidence status:** no secret, config, startup, HTTP, browser, or deploy output was captured here.

## Decision Record

- Required capability configuration must fail loud before serving.
- Product acceptance requires authenticated reads, not static pages or health alone.
- Smackerel owns the generic contract; operator deploy-adapter consumption is devops-owned and untouched.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

**Phase:** planning  
**Command:** none  
**Exit Code:** not applicable  
**Claim Source:** not-run

No test result is claimed.

## Uncertainty Declarations

- Exact config/wiring branches and strict-acceptance omission are not locally confirmed.
- No secret value was read and no red/green regression exists.

## Scenario Contract Evidence

Initialized in [scenario-manifest.json](scenario-manifest.json); evidence references are empty.

## Validation Summary

No completion validation or certification was performed.

## Audit Verdict

Not audited. No terminal verdict is claimed.

---

## Interim Fail-Soft Runtime-Disable Core (bubbles.implement, operator-directed)

**Phase:** implement
**Claim Source:** executed (this session)
**Scope:** the disjoint, unit-verifiable fail-soft core only. Live-stack rows
(integration / e2e-api / e2e-ui) are DEFERRED and their DoD items remain `[ ]`.

### Planning Divergence Flagged (route_required — NOT resolved here)

The committed `spec.md` / `design.md` / `scopes.md` describe **fail-LOUD** activation
(empty/missing required cursor secret ⇒ **boot refusal before serving**:
`GRAPH-ACT-001`, `SCN-080-001-01` "startup is refused before serving", design
`F080-CURSOR-SECRET-EMPTY` as a boot-failure code). The operator directed a
**fail-SOFT** contract for this session (empty/missing secret ⇒ a typed runtime
`capability_disabled` response served by the handler, never a boot refusal, a
silent 404, an opaque 500, or a panic). The packet folder name
(`...-graph-api-fail-soft-runtime-disable`) matches the fail-soft intent; the
planning artifacts do not. This is a genuine spec/design/scopes reconciliation
that is **owned by bubbles.analyst / bubbles.design / bubbles.plan**, not by an
execution agent. It is recorded as a coordination residual (see Deferred) and is
the reason NO committed (fail-loud) DoD checkbox is checked by this run.

### Implemented

- **`internal/api/graphapi/activation.go`** (new). Reuses the EXISTING typed error
  model (`APIError` / `WriteAPIError` / `ErrorEnvelope`) and the EXISTING cursor
  secret config (`Config.CursorSecretEnv`). Adds, hermetically (no datastore, no
  router, no `auth` import → no import cycle):
  - `CodeCapabilityDisabled = "capability_disabled"` + `ErrCapabilityDisabled`
    (HTTP **503**, message `"connected knowledge is disabled for this deployment"`,
    matching the design's disabled-mode JSON) — the typed, honest runtime-disabled
    response.
  - `Config.ClassifyCursorSecret()` → value-safe `SecretPresence` (`present` /
    `empty` / `missing`) via the same `os.LookupEnv` indirection `LoadCursorSecret`
    uses; it never returns, logs, or derives the secret value.
  - `ResolveActivation(cfg)` → never errors, never panics: present ⇒
    `ActivationEnabled` (`Code=OK`); empty ⇒ `ActivationDisabled`
    (`F080-CURSOR-SECRET-EMPTY`); missing ⇒ `ActivationDisabled`
    (`F080-CURSOR-SECRET-MISSING`).
  - `GraphCapability` + `Guard(next http.Handler)` middleware: DISABLED ⇒ typed
    503 `capability_disabled` for every wrapped path (never a bare Chi 404 silent
    absence, never a 500); ENABLED ⇒ delegates to the operating handler unchanged
    (operating-path typed errors flow through transparently).
  - Grant gating for the SINGLE operator-owned GLOBAL corpus (`GRAPH-ACT-005` /
    `GRAPH-ACT-011`): `GraphIdentity{Authenticated, Operator, Grants[]}` (carries
    **no** tenant/owner/row selector), `ClassifyGraphGrant` →
    `operator | grant_holder | ungranted`, `AuthorizeGraphRead` → `nil` for
    operator & grant-holder (same global rows, differentiated by grant not by a
    row predicate), leak-free `ErrMissingScope` (403 `forbidden`) for ungranted.
    `GraphReadScope = "knowledge-graph:read"` is the existing scope already
    enforced by `auth.RequireScope`, reframed under the global-corpus model.
- **`internal/api/graphapi/activation_test.go`** (new). 8 real Go unit tests
  (+6 subtests), hermetic (`t.Setenv` + `httptest` only). Proves: empty ⇒ typed
  503 disabled (asserted status + envelope shape, NOT 404/500/200, operating
  handler not called); missing (both "no env name" and "named env var unset")
  ⇒ typed 503 disabled; present ⇒ operating path runs (200); operating-path typed
  errors pass through Guard unchanged; grant matrix operator+grant-holder allowed
  / ungranted+unauthenticated denied 403 with no row-isolation predicate; ungranted
  denial is byte-static & count-free (leak-free); activation diagnostics never leak
  a sentinel secret; and an **adversarial** regression that FAILS if the code
  reverts to the original silent-absence 404, degrades to a 500, serves the
  operating path on empty, or panics (contrast leg drives the same path through an
  unguarded mux to prove the 503 and the 404 are genuinely distinct).

One test-only fix was applied during this run: the ungranted-denial leak scan had
a false positive (`"edge"` is a substring of the scope name `knowledge-graph:read`).
The IMPLEMENTATION was correct; the assertion was strengthened to compare the
denial body byte-for-byte against the static canonical `forbidden` envelope plus a
no-digit count check (a stronger, non-false-positive leak-free proof). Naming the
required scope is the same generic message `auth.RequireScope` already returns and
is not graph content.

### Test Evidence — Unit

**Command:** `./smackerel.sh test unit --go --go-run '<the 8 activation tests>' --verbose`
**Exit Code:** 0 (`[go-unit] go test ./... finished OK` prints only on exit 0 under `set -euxo pipefail`)
**Claim Source:** executed

```text
=== RUN   TestResolveActivation_EmptySecretIsTypedDisabled
--- PASS: TestResolveActivation_EmptySecretIsTypedDisabled (0.00s)
=== RUN   TestResolveActivation_MissingSecretIsTypedDisabled
=== RUN   TestResolveActivation_MissingSecretIsTypedDisabled/no_env_name_configured
=== RUN   TestResolveActivation_MissingSecretIsTypedDisabled/named_env_var_is_unset
--- PASS: TestResolveActivation_MissingSecretIsTypedDisabled (0.00s)
    --- PASS: TestResolveActivation_MissingSecretIsTypedDisabled/no_env_name_configured (0.00s)
    --- PASS: TestResolveActivation_MissingSecretIsTypedDisabled/named_env_var_is_unset (0.00s)
=== RUN   TestResolveActivation_PresentSecretOperates
--- PASS: TestResolveActivation_PresentSecretOperates (0.00s)
=== RUN   TestGuard_OperatingPathTypedErrorsFlowThrough
--- PASS: TestGuard_OperatingPathTypedErrorsFlowThrough (0.00s)
=== RUN   TestGraphReadGrantMatrix_GlobalCorpus
=== RUN   TestGraphReadGrantMatrix_GlobalCorpus/operator_reads_all_private_content
=== RUN   TestGraphReadGrantMatrix_GlobalCorpus/grant-holder_reads_authorized_global_projection
=== RUN   TestGraphReadGrantMatrix_GlobalCorpus/ungranted_authenticated_identity_is_denied
=== RUN   TestGraphReadGrantMatrix_GlobalCorpus/unauthenticated_caller_is_denied
--- PASS: TestGraphReadGrantMatrix_GlobalCorpus (0.00s)
    --- PASS: TestGraphReadGrantMatrix_GlobalCorpus/operator_reads_all_private_content (0.00s)
    --- PASS: TestGraphReadGrantMatrix_GlobalCorpus/grant-holder_reads_authorized_global_projection (0.00s)
    --- PASS: TestGraphReadGrantMatrix_GlobalCorpus/ungranted_authenticated_identity_is_denied (0.00s)
    --- PASS: TestGraphReadGrantMatrix_GlobalCorpus/unauthenticated_caller_is_denied (0.00s)
=== RUN   TestUngrantedDenialIsLeakFree
--- PASS: TestUngrantedDenialIsLeakFree (0.00s)
=== RUN   TestActivationDiagnosticsNeverLeakSecret
--- PASS: TestActivationDiagnosticsNeverLeakSecret (0.00s)
=== RUN   TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500
--- PASS: TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api/graphapi    0.016s
[go-unit] go test ./... finished OK
```

### Test Evidence — Check

**Command:** `./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
config-validate: <repo>/config/generated/dev.env.tmp.652405 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

### Test Evidence — Lint

**Command:** `./smackerel.sh lint`
**Exit Code:** 0
**Claim Source:** executed

```text
Successfully installed ... ruff-0.15.22 smackerel-ml-0.1.0 ...
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
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT_EXIT=0
```

### Deferred (NOT done this session)

1. **All live-stack rows** — every `integration`, `e2e-api`, and `e2e-ui` Test
   Plan row and DoD item across SCOPE-01..04 (the Docker test stack was NOT
   brought up: shared daemon, many concurrent agents, host-OOM/contention risk).
   These DoD items remain `[ ]`.
2. **Config-validation-test coordination residual** — the design's proposed
   `KNOWLEDGE_GRAPH_API_ACTIVATION` enum key was NOT added, because wiring a new
   config key edits **`internal/config/validate_test.go`**, which is concurrently
   owned/dirty by another agent. This core deliberately branches on the EXISTING
   cursor-secret config instead, so no new key and no edit to that file were
   needed. Adding the explicit activation enum is a coordination-required follow-up.
3. **Fail-loud → fail-soft spec/design/scopes reconciliation** — see "Planning
   Divergence Flagged" above; routed to bubbles.analyst / bubbles.design /
   bubbles.plan.
4. **Router/core wiring** — `GraphCapability.Guard` and the
   `auth.Session → GraphIdentity` adapter are unit-proven but NOT yet wired into
   `internal/api/router.go` / `cmd/core/wiring.go` (an integration step, and those
   files are outside this disjoint unit core).

### Guard Evidence

See "## Artifact Lint (bubbles.implement run)" below.

## Artifact Lint (bubbles.implement run)

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable`
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
✅ uservalidation checklist has checked-by-default entries
✅ Detected state.json workflowMode: bugfix-fastlane
✅ Top-level status matches certification.status
✅ report.md contains section matching: Summary
✅ report.md contains section matching: Completion Statement
✅ report.md contains section matching: Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

(Run captured while `state.json` still read `in_progress`; state was then transitioned to `blocked` — top-level status and `certification.status` remain equal, preserving the "Top-level status matches certification.status" invariant.)

---

## Runtime Wiring And Live-Integration Proof (bubbles.implement, 2026-07-26)

**Phase:** implement
**Claim Source (wiring):** executed/verified this session — read-only source inspection of the working tree.
**Claim Source (live-integration outcome):** interpreted — green in the prior in-session run that truncated before recording it here; the durable test file `tests/integration/graphapi/activation_test.go` was verified PRESENT this session (read-only). NOT re-executed this session (no-live-tests / no-Docker guardrail).

### Runtime Wiring (WIRED — supersedes the prior "unwired" status)

The fail-soft graph-API activation is now WIRED into the runtime. This was verified this session by read-only inspection of the working tree and is a MATERIAL change from the prior `executionHistory`, which recorded the fail-soft core (`internal/api/graphapi/activation.go`) as a standalone module with zero runtime references:

- **`cmd/core/wiring.go`** (~L392-449) — graph activation is now derived from the operator cursor-secret presence via `graphapi.LoadConfig()` → `graphapi.NewGraphCapability(...)`. An empty/missing secret (or a config-load / cursor-codec failure) resolves a DISABLED `deps.GraphCapability` with value-safe `slog` diagnostics (activation `Code` + `SecretPresence` class only — never secret material); a present secret ENABLES it and wires the live PostgreSQL-backed handlers (`NewTopicsHandlers` … `NewEdgesHandlers`). This REPLACES the prior warning-and-nil path that left the handler fields nil (the original silent Chi 404 bug).
- **`internal/api/router.go`** (~L149-205) — the five graph families register as ONE atomic route manifest gated by `deps.GraphCapability.Guard` (fail-soft 503 first) plus `auth.RequireScope("knowledge-graph:read")`. DISABLED → every one of the eight canonical paths is mounted against the typed `503 capability_disabled` responder (`GraphCapability.WriteDisabled`), so the endpoints are PRESENT (never a silent Chi 404 from nil handlers). ENABLED → the live handlers serve. The route manifest is identical in both states — there is no per-family `if handler != nil` branch.
- **`internal/api/health.go`** — the shared `api.Dependencies` struct (defined here) now carries the resolved `GraphCapability` activation field plus the five ENABLED-only graph handler fields; this is the wiring hook `internal/api/router.go` consumes. (The full authenticated `knowledge_graph` health projection is a SCOPE-03 deferred row — NOT part of this change.)
- **`tests/integration/graphapi/activation_test.go`** (new, `//go:build integration`) — the durable live proof, built on the REAL production router (`internal/api.NewRouter`, the same router `cmd/core` builds at boot), a real loopback server (`httptest`), and the disposable stack's real PostgreSQL (`DATABASE_URL`). No request interception, no mock, no stub.

### T080-01-PROC

`tests/integration/graphapi/activation_test.go` — SCN-080-001-01 live fail-soft proof. The durable test file was verified PRESENT this session (read-only); it ran green in the prior in-session integration run (`./smackerel.sh test integration` → `INTEG_LIGHT_EXIT=0`; the stack was then torn down clean — no smackerel containers remain), and was NOT re-executed this session:

- **DISABLED** (`TestGraphActivationDisabledSecretServesTyped503AndKeepsServing`) — with the cursor secret EMPTY and, separately, MISSING, every one of the eight canonical graph paths (`/api/topics`, `/api/topics/{id}`, `/api/people`, `/api/people/{id}`, `/api/places`, `/api/places/{id}`, `/api/time`, `/api/graph/edges`) answers HTTP **503** with a typed `capability_disabled` envelope — PRESENT, never a 404 (the assertion explicitly fails on a 404 as the reverted silent-absence bug) — and `/ping` still returns **200** (the service keeps serving other capabilities; never a boot refusal or panic).
- **ENABLED** (`TestGraphActivationEnabledSecretServesLiveOverPostgres`) — with the cursor secret configured, the capability is ENABLED, a uniquely-prefixed real topic row is seeded into the disposable PostgreSQL (and torn down after), and `GET /api/topics` returns HTTP **200** carrying that real row over the live database — the same wiring path as `cmd/core/wiring.go`'s enabled branch.

### T080-01-UNIT

The eight hermetic `internal/api/graphapi` unit tests are green — see the verbatim `go test` output under "### Test Evidence — Unit" above (recorded in the prior in-session unit run; the durable test `internal/api/graphapi/activation_test.go` is unchanged). They prove empty/missing secret → typed 503 disabled (never 404/500/200/panic — including the adversarial anti-regression `TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500`), the operator/grant-holder/ungranted grant matrix, leak-free ungranted denial, and value-safe activation diagnostics (`TestActivationDiagnosticsNeverLeakSecret`).

### check / lint / unit (prior in-session run — unchanged)

`./smackerel.sh check` → exit 0, `./smackerel.sh lint` → exit 0, `./smackerel.sh test unit --go` → `ok internal/api/graphapi` (8 tests), no regression. Raw output is preserved above under "### Test Evidence — Check", "### Test Evidence — Lint", and "### Test Evidence — Unit".

### Deferred (unchanged — SCOPE-02/03/04 remain blocked; SCOPE-01 e2e/manifest rows deferred)

The remaining live rows were NOT authored or run and their DoD items stay `[ ]`:

- **SCOPE-01** e2e-api regressions T080-02-ADVERSARIAL / T080-07-SECURITY / T080-01-DISABLED (`tests/e2e/graph_api_activation_e2e_test.go` absent) and the manifest-tamper integration row T080-02-MANIFEST (`tests/integration/graphapi/route_manifest_test.go` absent). Also SCN-080-001-02's manifest-tamper-rejection clause has no proving test, so that Core Outcome stays `[ ]`.
- **SCOPE-02** authorized family reads T080-03-PG/READONLY, T080-05-EMPTY, T080-06-AUTH/STORE/CURSOR and the operator/grant-holder/ungranted single-operator-owned global-corpus grant matrix SCN-080-001-09 (T080-09-CORPUS, T080-09-GRANT).
- **SCOPE-03** product synthetic, readiness, content-free telemetry, and stress (T080-03-SYNTH, T080-04-READY/STATIC, T080-07-TELEMETRY, T080-03-TRACE, T080-03-STRESS).
- **SCOPE-04** Wiki/Graph e2e-ui and accessibility (T080-04-UI, T080-05-UI, T080-06-UI, T080-08-A11Y).

### Guard Evidence (bubbles.implement, 2026-07-26 — this session, static validators only)

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable`
**Exit Code:** 0
**Claim Source:** executed (this session)

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
✅ Detected state.json status: blocked
✅ Detected state.json workflowMode: bugfix-fastlane
✅ Top-level status matches certification.status
ℹ️  Workflow mode 'bugfix-fastlane' allows status 'done'; current status is 'blocked'
✅ report.md contains section matching: Summary
✅ report.md contains section matching: Completion Statement
✅ report.md contains section matching: Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable`
**Exit Code:** 0
**Claim Source:** executed (this session)

Raw excerpt (header + scenario-manifest cross-check + SCOPE-01 evidence reference + summary; the full run printed a ✅ line per scenario/row across all four scopes and ended `RESULT: PASSED`):

```text
============================================================
  BUBBLES TRACEABILITY GUARD
  Feature: .../BUG-080-001-graph-api-fail-soft-runtime-disable
============================================================

--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 14 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist
✅ Scope 1: Fail-Soft Graph Activation Foundation scenario maps to concrete test file: internal/api/graphapi/activation_test.go
✅ Scope 1: Fail-Soft Graph Activation Foundation report references concrete test evidence: internal/api/graphapi/activation_test.go

--- Traceability Summary ---
ℹ️  Scenarios checked: 14
ℹ️  Test rows checked: 30
ℹ️  Scenario-to-row mappings: 14
ℹ️  Concrete test file references: 14
ℹ️  Report evidence references: 14
ℹ️  DoD fidelity scenarios: 14 (mapped: 14, unmapped: 0)

RESULT: PASSED (0 warnings)
TRACEABILITY_GUARD_EXIT=0
```

## Atomic Route-Manifest Closure + Deferred-Row Disposition (bubbles.implement, 2026-07-27)

**Phase:** implement
**Claim Source (T080-02-MANIFEST):** executed this session — `./smackerel.sh test integration-light --go-run 'TestGraphRouteManifestRegistersAllFamiliesAtomically'` ran green against the disposable stack's REAL PostgreSQL; stack torn down clean afterward.

This session closes SCOPE-01's atomic route-manifest row and its dependent Core
Outcome, and precisely characterizes why the remaining three e2e-api rows stay
`[ ]` (a harness limitation outside SCOPE-01's change boundary).

### Genuine defect found + fixed in-boundary (design gap SCN-080-001-02)

design.md ("Atomic Wiring And Route Registration" / `F080-ROUTE-MANIFEST-INCOMPLETE`)
requires a canonical route registrar that "validates this complete manifest
before calling Chi" so that "removing any descriptor fails construction". The
shipped code had NO such manifest — `internal/api/router.go` hardcoded the eight
graph routes, so a dropped route would silently mount a **seven-route subset**
(the exact silent-absence class this bug fix exists to eliminate). That is why
SCN-080-001-02's manifest-tamper clause had no proving test and stayed `[ ]`.

Closed in-boundary (SCOPE-01 Change Boundary allows `internal/api/graphapi/**`
and router registration):

- **`internal/api/graphapi/manifest.go`** (new) — `CanonicalGraphRouteManifest()`
  (the eight design.md families, each `GET` + `knowledge-graph:read`),
  `ValidateGraphRouteManifest(entries)` (rejects a missing/duplicated/unknown
  family, non-GET method, empty path, or wrong scope with the typed, value-safe
  `F080-ROUTE-MANIFEST-INCOMPLETE` error naming only the family + route contract
  — never secret material), and `MustValidateGraphRouteManifest()`.
- **`internal/api/router.go`** — one added call, `graphapi.MustValidateGraphRouteManifest()`,
  BEFORE the graph group registers, so an incomplete/duplicated canonical
  manifest REJECTS router construction (fail-loud panic) rather than mounting a
  subset. The existing (proven) route registration is unchanged — no regression
  to T080-01-PROC.

### T080-02-MANIFEST

`tests/integration/graphapi/route_manifest_test.go` (new, `//go:build integration`) —
`TestGraphRouteManifestRegistersAllFamiliesAtomically`, built on the REAL
production router (`internal/api.NewRouter`), a real loopback server (`httptest`),
and the disposable stack's real PostgreSQL (`DATABASE_URL`). No interception, no
mock, no stub. Proves both SCN-080-001-02 clauses: (1) all eight families mount
as ONE authenticated group (chi.Walk equivalence + unauthed→401 / authed→served);
(2) removing OR duplicating any manifest entry rejects construction (adversarial
red→green vs the old silent-subset behavior).

**Command:** `./smackerel.sh test integration-light --go-run 'TestGraphRouteManifestRegistersAllFamiliesAtomically'`
**Exit Code:** 0 (`INTEGRATION_LIGHT_EXIT=0`) — stores-only lane (postgres+nats); this test needs only PostgreSQL (in-process production router), so the light lane is the correct integration surface.

```text
=== RUN   TestGraphRouteManifestRegistersAllFamiliesAtomically
=== RUN   TestGraphRouteManifestRegistersAllFamiliesAtomically/canonical_manifest_is_complete_and_router_validates
    route_manifest_test.go:129: manifest entry: GET    /api/topics/         family=topics scope=knowledge-graph:read
    route_manifest_test.go:129: manifest entry: GET    /api/topics/{id}     family=topic_detail scope=knowledge-graph:read
    route_manifest_test.go:129: manifest entry: GET    /api/people/         family=people scope=knowledge-graph:read
    route_manifest_test.go:129: manifest entry: GET    /api/people/{id}     family=person_detail scope=knowledge-graph:read
    route_manifest_test.go:129: manifest entry: GET    /api/places/         family=places scope=knowledge-graph:read
    route_manifest_test.go:129: manifest entry: GET    /api/places/{id}     family=place_detail scope=knowledge-graph:read
    route_manifest_test.go:129: manifest entry: GET    /api/time            family=time scope=knowledge-graph:read
    route_manifest_test.go:129: manifest entry: GET    /api/graph/edges     family=edges scope=knowledge-graph:read
=== RUN   TestGraphRouteManifestRegistersAllFamiliesAtomically/removing_any_manifest_entry_rejects_construction
    route_manifest_test.go:155: remove topics       -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "topics" is absent; the manifest MUST mount all 8 families atomically, never a subset
    route_manifest_test.go:155: remove topic_detail -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "topic_detail" is absent; the manifest MUST mount all 8 families atomically, never a subset
    route_manifest_test.go:155: remove people       -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "people" is absent; the manifest MUST mount all 8 families atomically, never a subset
    route_manifest_test.go:155: remove person_detail -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "person_detail" is absent; the manifest MUST mount all 8 families atomically, never a subset
    route_manifest_test.go:155: remove places       -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "places" is absent; the manifest MUST mount all 8 families atomically, never a subset
    route_manifest_test.go:155: remove place_detail -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "place_detail" is absent; the manifest MUST mount all 8 families atomically, never a subset
    route_manifest_test.go:155: remove time         -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "time" is absent; the manifest MUST mount all 8 families atomically, never a subset
    route_manifest_test.go:155: remove edges        -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] required family "edges" is absent; the manifest MUST mount all 8 families atomically, never a subset
=== RUN   TestGraphRouteManifestRegistersAllFamiliesAtomically/duplicating_any_manifest_entry_rejects_construction
    route_manifest_test.go:175: duplicate topics       -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] family "topics" is registered 2 times; each required family MUST appear exactly once
    route_manifest_test.go:175: duplicate topic_detail -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] family "topic_detail" is registered 2 times; each required family MUST appear exactly once
    route_manifest_test.go:175: duplicate edges        -> REJECTED: [F080-ROUTE-MANIFEST-INCOMPLETE] family "edges" is registered 2 times; each required family MUST appear exactly once
=== RUN   TestGraphRouteManifestRegistersAllFamiliesAtomically/disabled_router_mounts_all_eight_present
    route_manifest_test.go:187: DISABLED router mounted graph routes:
          GET /api/graph/edges
          GET /api/people/
          GET /api/people/{id}
          GET /api/places/
          GET /api/places/{id}
          GET /api/time
          GET /api/topics/
          GET /api/topics/{id}
=== RUN   TestGraphRouteManifestRegistersAllFamiliesAtomically/enabled_router_mounts_all_eight_as_one_authenticated_group
    route_manifest_test.go:262: ENABLED router mounted graph routes:
          GET /api/graph/edges
          GET /api/people/
          GET /api/people/{id}
          GET /api/places/
          GET /api/places/{id}
          GET /api/time
          GET /api/topics/
          GET /api/topics/{id}
    route_manifest_test.go:283: unauthed GET /api/topics                                 -> 401 (behind bearer)
    route_manifest_test.go:302: authed   GET /api/topics                                 -> 200 application/json; charset=utf-8
    route_manifest_test.go:283: unauthed GET /api/topics/bug080-manifest-nonexistent     -> 401 (behind bearer)
    route_manifest_test.go:302: authed   GET /api/topics/bug080-manifest-nonexistent     -> 404 application/json; charset=utf-8
    route_manifest_test.go:283: unauthed GET /api/people                                 -> 401 (behind bearer)
    route_manifest_test.go:302: authed   GET /api/people                                 -> 200 application/json; charset=utf-8
    route_manifest_test.go:283: unauthed GET /api/graph/edges                            -> 401 (behind bearer)
    route_manifest_test.go:302: authed   GET /api/graph/edges                            -> 200 application/json; charset=utf-8
    route_manifest_test.go:322: authed   GET /api/topics?limit=50 -> 200, seeded topic bug080-manifest-20260727170415.762772-topic-0 present (live PostgreSQL)
--- PASS: TestGraphRouteManifestRegistersAllFamiliesAtomically (0.09s)
    --- PASS: TestGraphRouteManifestRegistersAllFamiliesAtomically/canonical_manifest_is_complete_and_router_validates (0.00s)
    --- PASS: TestGraphRouteManifestRegistersAllFamiliesAtomically/removing_any_manifest_entry_rejects_construction (0.00s)
    --- PASS: TestGraphRouteManifestRegistersAllFamiliesAtomically/duplicating_any_manifest_entry_rejects_construction (0.00s)
    --- PASS: TestGraphRouteManifestRegistersAllFamiliesAtomically/disabled_router_mounts_all_eight_present (0.00s)
    --- PASS: TestGraphRouteManifestRegistersAllFamiliesAtomically/enabled_router_mounts_all_eight_as_one_authenticated_group (0.09s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/graphapi       0.231s
INTEGRATION_LIGHT_EXIT=0
```

(The duplicate-rejection block above is a 3-of-8 excerpt; the run rejected all
eight one-entry duplications with the identical `…registered 2 times…` typed
error. The enabled auth-group block is a 4-of-8 excerpt; the run drove all eight
family paths — every one unauthed→401, authed→served JSON — see the full raw
terminal capture from this session.)

### Quality gates (this session)

- `./smackerel.sh check` → exit 0 (config-validate + scenario-lint OK).
- `./smackerel.sh lint` → exit 0 (`All checks passed!` — Go/golangci + ruff + web manifests; includes the new `manifest.go` + `router.go`).
- `bash .github/bubbles/scripts/pii-scan.sh` → exit 0 (`no leaks found` / `pii-scan: clean.`).
- `gofmt -l internal/api/graphapi/manifest.go internal/api/router.go tests/integration/graphapi/route_manifest_test.go` → empty (my three files are gofmt-clean).
- `./smackerel.sh format --check` → exit 1, flagging ONLY `internal/assistant/facade.go` — a **pre-existing, committed, foreign** file (git-committed 2026-07-27 06:24 under "BUG-069-005 assistant intent-compiler fixes (in_progress — dedicated to late completion)"), OUTSIDE SCOPE-01's change boundary. It is not this session's regression and was deliberately NOT touched (artifact ownership + bounded-slice discipline). Recorded as an out-of-boundary pre-existing finding, not a SCOPE-01 defect.

### DoD rows closed this session

- **Core Outcome SCN-080-001-02** → `[x]` (T080-02-MANIFEST: all eight families mount as one authenticated group; remove/duplicate rejects construction).
- **Test Evidence T080-02-MANIFEST** → `[x]` (live integration, evidence above).

### Still deferred — e2e-api rows (HARNESS LIMITATION, owner: bubbles.devops)

The three SCOPE-01 e2e-api rows in `tests/e2e/graph_api_activation_e2e_test.go`
stay `[ ]`. They were NOT faked. Root cause, verified this session:

- **T080-01-DISABLED** and **T080-02-ADVERSARIAL** both require the running
  `smackerel-core` container in the **DISABLED** activation state (cursor-secret
  enabler empty/missing) so a true-container e2e can prove `503 capability_disabled`
  (and, for the adversarial, `503`-not-`404` vs the old behavior). The e2e stack
  ALWAYS boots ENABLED: `config/generated/test.env` sets `KNOWLEDGE_GRAPH_API_CURSOR_SECRET`
  non-empty (line 195), and `docker-compose.yml` sources it from `env_file` with
  no per-run override (the core `environment:` block does not touch it). A
  disabled-mode running stack needs a NEW compose flavor / `./smackerel.sh test e2e`
  enabled+disabled split — a harness/compose change OUTSIDE SCOPE-01's Change
  Boundary (which lists `config/smackerel.yaml`, `internal/config/**`,
  `internal/api/graphapi/**`, router registration, and the named tests — NOT
  `docker-compose*.yml` or `smackerel.sh`). The DISABLED behavior IS already
  live-proven at the integration tier (T080-01-PROC, `[x]`), and value-safe
  disabled diagnostics are unit-proven (T080-01-UNIT, `[x]`).
- **T080-07-SECURITY** needs to assert the cursor-secret value never appears in
  activation output surfaced via the API. The e2e go-test runner is only given
  `DATABASE_URL` / `SMACKEREL_AUTH_TOKEN` / `CORE_EXTERNAL_URL` (smackerel.sh e2e
  lane `-e` passthrough), NOT `KNOWLEDGE_GRAPH_API_CURSOR_SECRET`, so the test
  process has no secret value to assert-absent; and activation emits only
  container-local `slog` (value-safe) with no HTTP-surfaced activation-status
  endpoint yet (that projection is a SCOPE-03 deferred row). A strong live
  enabled-stack value-safe assertion is therefore not achievable without harness
  support; the value-safety of activation diagnostics is unit-proven
  (SCN-080-001-07, `[x]`).

Because these three rows remain open, the SCOPE-01 **Build Quality Gate** row
also stays `[ ]` (it requires all scope-specific E2E regressions to pass), and
SCOPE-01 stays **In Progress**. The BUG top-level `state.json` status stays
`blocked` (SCOPE-02/03/04 remain deferred).

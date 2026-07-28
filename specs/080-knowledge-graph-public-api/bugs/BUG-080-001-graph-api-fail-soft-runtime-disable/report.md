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
- **(2026-07-27, bubbles.implement)** T080-02-ADVERSARIAL: the GREEN half is
  proven by a live DISABLED-stack `--- PASS` and the test is structurally
  adversarial (`regression-quality-guard --bugfix` → "Adversarial signal
  detected"). The **RED half is NOT proven**: no failing run of
  `TestE2E_GraphActivation_DisabledAdversarialRedGreen` against pre-fix
  warning-and-nil/omitted-route behavior was ever captured, and the fail-soft
  repair was committed before the harness existed, so it cannot be produced
  retroactively without a deliberate reversion or an ENABLED-stack contrast run.
  The DoD row demands "both outputs are recorded", so the row stays `[ ]`.
  Routed to `bubbles.plan` as finding F-1.
  **→ RESOLVED 2026-07-28.** The "deliberate reversion" path was taken: a
  throwaway `git worktree` reintroduced the defect and the unmodified test
  failed against it (`===RED_EXIT=1===`). Both halves are recorded; row is `[x]`.
- **(2026-07-27, bubbles.implement)** The SCOPE-01 Build Quality Gate row stays
  `[ ]` on two clauses only: "no skipped checks" (the F-1 RED capture) and
  "documentation alignment" (finding F-2 — the stale HARNESS LIMITATION header
  comment in `tests/e2e/graph_api_activation_e2e_test.go`). All other clauses of
  that row were executed green this invocation.
  **→ RESOLVED 2026-07-28.** F-1 and F-2 are both closed; all 8 gate commands
  re-run to exit 0 this session; row is `[x]` and SCOPE-01 is `Done`.
- **(2026-07-28, bubbles.implement) Open residue, non-blocking.** The throwaway
  RED worktree was deregistered from git, but an orphaned copy of the mutated
  `router.go` remains at `/tmp/smk-red/internal/api/router.go` (outside the
  repository, untracked, unreferenced). `main`'s `router.go` is verified
  byte-identical to HEAD. Disclosed rather than omitted; safe to delete.
- **(2026-07-28, bubbles.implement) Scope of this closure.** Only SCOPE-01 is
  closed. The bug's top-level `status` and `certification.status` remain
  `blocked` on SCOPE-02 (`in_progress`) and SCOPE-03/04 (`blocked`). No
  SCOPE-02/03/04 row was inspected, executed, or altered.
- **(2026-07-27, bubbles.implement)** The e2e raw output transcribed in
  "## E2E Harness Unblock + SCOPE-01 e2e Closure (2026-07-27)" was executed by
  the **parent orchestrator** in this session, not by this invocation. This
  invocation re-ran only the static gates (check, lint, format --check,
  pii-scan, artifact-lint, traceability-guard) and verified the harness wiring
  read-only.

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

### ~~Still deferred — e2e-api rows (HARNESS LIMITATION, owner: bubbles.devops)~~ — SUPERSEDED 2026-07-27

> **STATUS: RESOLVED / SUPERSEDED.** The blocker described below was cleared on
> 2026-07-27 by the bubbles.devops harness delivery (`docker-compose.graph-disabled.override.yml`,
> the serial graph-disabled `./smackerel.sh test e2e` phase, and the fail-loud
> `SMACKEREL_COMPOSE_OVERRIDE_FILE` hook in `scripts/lib/runtime.sh`). All three
> rows now have live-stack evidence — see
> "## E2E Harness Unblock + SCOPE-01 e2e Closure (2026-07-27)" below.
>
> **The original T080-07-SECURITY diagnosis below was FACTUALLY WRONG and is
> retained only as an audit record — do not act on it.** It asserted the e2e
> runner "is only given `DATABASE_URL` / `SMACKEREL_AUTH_TOKEN` /
> `CORE_EXTERNAL_URL` … NOT `KNOWLEDGE_GRAPH_API_CURSOR_SECRET`, so the test
> process has no secret value to assert-absent." That is false. The ENABLED
> go-e2e lane in `smackerel.sh` passes `--env-file "$env_file"`
> (`smackerel.sh:2114`, feeding `config/generated/test.env`) IN ADDITION to the
> named `-e` passthroughs, and `config/generated/test.env:195` defines
> `KNOWLEDGE_GRAPH_API_CURSOR_SECRET`. The secret was therefore ALWAYS present
> in the runner env. The earlier author enumerated only the visually adjacent
> `-e` flags and did not read the `--env-file` line one row above them. No
> harness change was ever required for T080-07-SECURITY; the test simply had
> never been run. When run on 2026-07-27 it passed first time and logged
> `secret length=64`, proving the needle was live.
>
> **Why this correction is recorded rather than deleted:** a wrong recorded
> blocker is self-perpetuating — it converts a runnable test into permanently
> deferred work, because every later reader trusts the recorded diagnosis
> instead of re-deriving it. The T080-01-DISABLED / T080-02-ADVERSARIAL half of
> the diagnosis below WAS correct (a genuinely DISABLED container really did
> require a new compose flavor). The T080-07-SECURITY half was not. Both are
> kept verbatim so the distinction stays auditable.

**Original (superseded) text follows.**

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

**End of superseded text.** See the next section for the resolving evidence.

---

## E2E Harness Unblock + SCOPE-01 e2e Closure (2026-07-27)

**Provenance of the e2e output in this section:** the run below was executed by
the **parent orchestrator in this session** on 2026-07-27 (start
`2026-07-27T18:59:04+00:00`, end `2026-07-27T19:06:30+00:00`, terminal marker
`===E2E_EXIT=0===`). This `bubbles.implement` invocation **did not re-run** the
~8-minute suite; it transcribes that session-local raw output verbatim and
attributes it accordingly. No additional e2e run is claimed. The static gates in
"### Quality gates (2026-07-27, this invocation)" WERE executed by this
invocation.

**Command:** `./smackerel.sh test e2e --go-run 'TestE2E_GraphActivation'`
**Exit Code:** 0 (`===E2E_EXIT=0===`)
**Claim Source:** executed (parent orchestrator, this session)

### Harness delivered by bubbles.devops (what unblocked these rows)

| File | Change | Status |
|---|---|---|
| `docker-compose.graph-disabled.override.yml` | NEW — overrides `smackerel-core` to boot with an EMPTY `KNOWLEDGE_GRAPH_API_CURSOR_SECRET`, producing the fail-soft DISABLED activation state | new, untracked |
| `smackerel.sh` | Serial graph-disabled e2e phase (runs LAST, recycles the stack, exports `SMACKEREL_E2E_GRAPH_DISABLED_URL` to the runner) | modified |
| `scripts/lib/runtime.sh` | `SMACKEREL_COMPOSE_OVERRIDE_FILE` hook — fail-loud, no default | modified |

Verified read-only by this invocation: `smackerel.sh:2305` exports
`SMACKEREL_E2E_GRAPH_DISABLED_URL=http://smackerel-core:${core_container_port}`
for the graph-disabled phase, which is exactly the variable
`disabledGraphStackURL()` requires — so the two disabled tests **ran** rather
than hitting their `t.Skip` guard. `scripts/lib/runtime.sh:142-147` implements
the fail-loud override hook.

### T080-07-SECURITY

**Scenario:** SCN-080-001-07 — Graph activation output never contains secret or
cursor material.
**Tier:** `e2e-api`, ENABLED stack, real HTTP against the live `smackerel-core`
container. No request interception, no mock.
**Claim Source:** executed (parent orchestrator, this session)

```text
go-e2e: applying -run selector: TestE2E_GraphActivation
=== RUN   TestE2E_GraphActivation_NeverLeaksSecretOrCursorMaterial
    graph_api_activation_e2e_test.go:204: probe topics_page1                 status=200 bodyLen=284 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:204: probe topics_page2_cursor_decode   status=200 bodyLen=284 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:204: probe people                       status=200 bodyLen=29 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:204: probe places                       status=200 bodyLen=29 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:204: probe time                         status=400 bodyLen=98 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:204: probe edges_topic_source           status=200 bodyLen=29 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:204: probe invalid_cursor_error         status=400 bodyLen=114 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:204: probe health                       status=200 bodyLen=1295 rawSecretAbsent=true
    graph_api_activation_e2e_test.go:208: VALUE-SAFE: 8 live graph/activation API probes surfaced NO cursor-secret material (raw/hex/base64/sha256); secret length=64
--- PASS: TestE2E_GraphActivation_NeverLeaksSecretOrCursorMaterial (0.05s)
ok      github.com/smackerel/smackerel/tests/e2e        0.180s
PASS: go-e2e
```

**Why this is a real assertion and not a vacuous pass:** the test `t.Skip`s if
`KNOWLEDGE_GRAPH_API_CURSOR_SECRET` is absent or shorter than 8 chars. It did
not skip, and it logged `secret length=64` — so the leak needle was the ACTUAL
deployed 64-char secret, and all seven derived needle classes (raw, hex,
base64_std, base64_rawurl, sha256_hex, sha256_b64, sha256_rawurlb) were searched
across both response bodies and headers on all 8 probes. The `topics_page1` →
`topics_page2_cursor_decode` pair exercised a real HMAC-signed `nextCursor`
round trip, so live cursor material was genuinely surfaced and scanned. The
assertion is value-safe: the secret is searched-for, never logged.

### T080-01-DISABLED

**Scenario:** SCN-080-001-01 — empty/missing enabler yields the typed 503
`capability_disabled` state and the service keeps serving other capabilities.
**Tier:** `e2e-api`, DISABLED stack, real HTTP against a genuinely
disabled-graph `smackerel-core` container.
**Claim Source:** executed (parent orchestrator, this session)

```text
Running graph-DISABLED e2e phase (BUG-080-001 SCOPE-01: T080-01-DISABLED + T080-02-ADVERSARIAL)...
Container smackerel-test-smackerel-core-1  Healthy
go-e2e: applying -run selector: TestE2E_GraphActivation_Disabled
=== RUN   TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing
--- PASS: TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing (0.02s)
=== RUN   TestE2E_GraphActivation_DisabledAdversarialRedGreen
--- PASS: TestE2E_GraphActivation_DisabledAdversarialRedGreen (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.179s
PASS: go-e2e-graph-disabled
===E2E_EXIT=0===
```

The test drove all eight canonical manifest paths (`/api/topics`,
`/api/topics/does-not-exist`, `/api/people`, `/api/people/does-not-exist`,
`/api/places`, `/api/places/does-not-exist`, `/api/time`, `/api/graph/edges`),
requiring `503` + a typed `capability_disabled` envelope on every one, then
asserted `/api/health` still returns `200` — the "keeps serving" clause.

**Value-safe proof the override genuinely produced the DISABLED state** (secret
never printed; length only). This is the differential control that rules out a
vacuous pass against an accidentally-enabled core:

```text
A) BASELINE (no override):   smackerel-core: enabler_len=64   smackerel-ml: enabler_len=64
B) WITH graph-disabled override: smackerel-core: enabler_len=0    smackerel-ml: enabler_len=64
C) bad override path: ERROR: SMACKEREL_COMPOSE_OVERRIDE_FILE is set but is not a readable file: /nonexistent/nope.yml  (exit 1 — fails loud)
```

Row A vs row B is the proof: the override drove `smackerel-core`'s enabler from
length 64 to length 0 while leaving `smackerel-ml` untouched at 64, so the
DISABLED assertions ran against a core that was really disabled, and the
override's blast radius was correctly scoped to one service. Row C proves the
hook is fail-loud with no silent default.

### T080-02-ADVERSARIAL

**Scenario:** SCN-080-001-02 (fail-soft leg) — empty/missing enabler serves the
typed 503 `capability_disabled`, never a silent 404 nil-handler absence or an
opaque 500.
**Tier:** `e2e-api`, DISABLED stack (same run as T080-01-DISABLED above).
**Claim Source:** executed (parent orchestrator, this session) — **GREEN half only**

GREEN evidence is the `TestE2E_GraphActivation_DisabledAdversarialRedGreen`
`--- PASS` line in the T080-01-DISABLED block above. The test drives
`GET /api/topics/` — the exact path the pre-fix router answered with a bare Chi
404 — and `t.Fatalf`s on `404` (RED reproduced), `t.Fatalf`s on `500` (opaque
degradation), and passes ONLY on a `503` carrying a typed `capability_disabled`
envelope.

**Anti-false-positive guards (executed by THIS invocation's parent in this
session):**

```text
bash .github/bubbles/scripts/regression-quality-guard.sh tests/e2e/graph_api_activation_e2e_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)   GUARD_EXIT=0
bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/graph_api_activation_e2e_test.go
  ✅ Adversarial signal detected      0 violation(s), 0 warning(s)   BUGFIX_GUARD_EXIT=0
```

`--bugfix` reporting "Adversarial signal detected" confirms the test is
STRUCTURALLY adversarial — it cannot pass against the reintroduced bug. That
satisfies the framework's adversarial-regression standard.

**HONEST GAP — this row remains `[ ]`.** The DoD item as written requires that
the test "**first fails** against warning-and-nil/omitted-route behavior, then
passes with the repair; **both outputs are recorded**". Only the GREEN output
exists. No RED run of this e2e test against pre-fix behavior was ever captured:
the fail-soft repair was already committed before the harness existed, and
"## Bug Reproduction - Before Fix" in this report is explicitly
`Claim Source: interpreted historical input` with "no red/green regression
exists". Marking this row `[x]` would assert a recorded RED output that does not
exist. See "## Uncertainty Declarations" and the routed finding below.

> **SUPERSEDED 2026-07-28.** The gap above was closed by *executing the missing
> check*, not by rewording the row: a throwaway `git worktree` reintroduced the
> original defect and the unmodified test failed against it
> (`===RED_EXIT=1===`, 8/8 canonical paths bare `404`). Both halves are now
> recorded in "## Adversarial RED→GREEN Closure + F-1/F-2 Resolution
> (bubbles.implement, 2026-07-28)" below, and this row is now `[x]`.

### Quality gates (2026-07-27, this invocation)

All executed by THIS `bubbles.implement` invocation. Raw output is reproduced in
the session transcript; exit codes below.

| Gate | Command | Exit |
|---|---|---|
| Check | `./smackerel.sh check` | 0 |
| Lint | `./smackerel.sh lint` | 0 (`All checks passed!`) |
| Format | `./smackerel.sh format --check` | 0 |
| PII scan | `bash .github/bubbles/scripts/pii-scan.sh` | 0 (`no leaks found` / `pii-scan: clean.`) |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/080-…/BUG-080-001-…` | 0 (`Artifact lint PASSED.`) |
| Traceability guard | `bash .github/bubbles/scripts/traceability-guard.sh specs/080-…/BUG-080-001-…` | 0 (`RESULT: PASSED (0 warnings)`) |

```text
$ ./smackerel.sh check
config-validate: <repo-root>/config/generated/dev.env.tmp.551875 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0

$ ./smackerel.sh lint
All checks passed!
LINT_EXIT=0

$ ./smackerel.sh format --check
FORMAT_CHECK_AFTER_EXIT=0

$ bash .github/bubbles/scripts/pii-scan.sh
7:14PM INF 0 commits scanned.
7:14PM INF scan completed in 10.3ms
7:14PM INF no leaks found
🫧 pii-scan: clean.
PII_SCAN_EXIT=0

$ bash .github/bubbles/scripts/traceability-guard.sh specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable
--- Traceability Summary ---
RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0
```

### gofmt repair (pre-existing, unblocks the Build Quality Gate)

`./smackerel.sh format --check` previously exited 1 flagging ONLY
`internal/assistant/facade.go` — a committed, foreign file from the spec-069
consolidation merge, recorded in the prior session as an out-of-boundary
pre-existing finding. Because the SCOPE-01 Build Quality Gate row explicitly
requires `./smackerel.sh format --check` to pass, that finding blocked the row
(and `git push`). Repaired this invocation with `./smackerel.sh format`.

**Verified purely cosmetic — zero semantic change:**

```text
$ git --no-pager diff --stat internal/assistant/facade.go
 internal/assistant/facade.go | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)

$ git --no-pager diff internal/assistant/facade.go
@@ -210,7 +210,7 @@ type Facade struct {
        // producing a provider-error the provenance gate then masks as
        // "saved as an idea"). Returns the marshaled Forecast JSON or a
        // classified error.
-       weatherLookup func(ctx context.Context, location string) (json.RawMessage, error)
+       weatherLookup        func(ctx context.Context, location string) (json.RawMessage, error)
         compiledInteractions *compiledInteractions
 }
```

The single changed line is struct-field alignment padding between the field name
`weatherLookup` and its type — gofmt aligning it with the adjacent
`compiledInteractions` field. The identifier, type, comment, and all surrounding
code are byte-identical. No behavior, signature, or assistant failure-honesty
logic changed.

### DoD rows closed this invocation

- **Test Evidence T080-07-SECURITY** → `[x]` (live ENABLED-stack, 8 probes, real 64-char secret needle).
- **Test Evidence T080-01-DISABLED** → `[x]` (live DISABLED-stack, 8 canonical paths typed 503 + health 200, differential enabler-length control).
- **Test Evidence T080-02-ADVERSARIAL** → stays `[ ]` (GREEN proven; RED output not recorded — see the HONEST GAP above). *(SUPERSEDED 2026-07-28 → now `[x]`.)*
- **Build Quality Gate** → stays `[ ]` (see the assessment below). *(SUPERSEDED 2026-07-28 → now `[x]`.)*

### Build Quality Gate assessment (SCOPE-01)

The row requires: "Scope-specific unit/integration/E2E regressions,
`./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`,
source-lock/config checks, artifact-lint, traceability guard, documentation
alignment, zero warnings, and change-boundary review all pass with executed
evidence and **no skipped checks**."

| Clause | Verdict |
|---|---|
| Scope-specific unit / integration / E2E regressions pass | ✅ unit + integration previously green; e2e green this session (`===E2E_EXIT=0===`) |
| `./smackerel.sh check` | ✅ exit 0 |
| `./smackerel.sh lint` | ✅ exit 0, `All checks passed!` |
| `./smackerel.sh format --check` | ✅ exit 0 (repaired this invocation) |
| source-lock / config checks | ✅ folded into `check` (config-in-sync + env_file drift guard OK) |
| artifact-lint | ✅ exit 0 |
| traceability guard | ✅ exit 0, `RESULT: PASSED (0 warnings)` |
| zero warnings | ✅ 0 warnings across lint + traceability |
| change-boundary review | ✅ amended + attributed — see scopes.md "Change Boundary" |
| documentation alignment | ⚠️ **NOT satisfied** — see below |
| **no skipped checks** | ⚠️ **NOT satisfied** — the T080-02-ADVERSARIAL RED half was never run |

**Row stays `[ ]`.** Two clauses fail honestly:

1. **"no skipped checks"** — T080-02-ADVERSARIAL's required RED capture was
   never executed. A gate row that certifies "no skipped checks" cannot be
   checked while a sibling row in the same scope is open precisely because a
   required output is missing.
2. **"documentation alignment"** — the header comment block of
   `tests/e2e/graph_api_activation_e2e_test.go` (lines 24-41) still narrates the
   now-resolved HARNESS LIMITATION and states the two disabled tests "`t.Skip`
   with a precise reason … until then". That text is now false: the harness
   exports `SMACKEREL_E2E_GRAPH_DISABLED_URL` and both tests run. Leaving a
   stale blocker narrative in the test file would recreate exactly the
   self-perpetuating-deferral failure this report corrects above. Correcting it
   is a test-file edit that was NOT in this invocation's assigned change set, so
   it is routed rather than silently made.

Every other clause is green. Once the two findings below are closed, this row
and T080-02-ADVERSARIAL close together.

> **SUPERSEDED 2026-07-28.** Both findings ARE now closed, and — exactly as
> predicted above — the two rows closed together. See the 2026-07-28 re-verdict
> table below.

### Routed findings (open — owner action required)

> **STATUS 2026-07-28: BOTH RESOLVED.** See
> "## Adversarial RED→GREEN Closure + F-1/F-2 Resolution (2026-07-28)" below.
> The table is retained verbatim as the historical record of why the two rows
> were held open on 2026-07-27.

| # | Finding | Owner | Detail |
|---|---|---|---|
| F-1 | T080-02-ADVERSARIAL DoD demands a recorded RED output that does not and cannot retroactively exist | `bubbles.plan` | Either (a) reword the row to the framework adversarial-regression standard actually enforced by `regression-quality-guard --bugfix` ("the test is constructed to fail against the reintroduced bug", already ✅), or (b) keep the literal red→green requirement and commission a RED capture (e.g. run the adversarial test against the ENABLED stack, or temporarily revert the fail-soft branch behind a harness flag). Row text is plan-owned; `bubbles.implement` must not rewrite a DoD behavioral claim to match delivery. |
| F-2 | `tests/e2e/graph_api_activation_e2e_test.go` header (lines 24-41) still documents the resolved HARNESS LIMITATION and a `t.Skip` that no longer occurs | `bubbles.implement` (follow-up) or `bubbles.devops` | Update the comment to state the harness now exports `SMACKEREL_E2E_GRAPH_DISABLED_URL` via the serial graph-disabled phase. Blocks the "documentation alignment" clause of the Build Quality Gate row. |

---

## Adversarial RED→GREEN Closure + F-1/F-2 Resolution (bubbles.implement, 2026-07-28)

This invocation records the **missing RED half** of T080-02-ADVERSARIAL, which
was the sole reason that row and the SCOPE-01 Build Quality Gate stayed `[ ]`.
Option (b) of finding F-1 was taken: the literal red→green DoD requirement was
**kept**, and the RED capture was **commissioned** — not the easier option (a)
of rewording the row to match what had already been delivered.

### T080-02-ADVERSARIAL

**Scenario:** SCN-080-001-02 (fail-soft leg) — empty/missing enabler serves the
typed 503 `capability_disabled`, never a silent 404 nil-handler absence or an
opaque 500.
**Tier:** `e2e-api`, DISABLED stack.
**Claim Source:** executed — **BOTH halves now recorded.**

#### RED capture method (throwaway defect reintroduction)

A **detached, throwaway `git worktree`** was created at `/tmp/smk-red` from
commit `6a12f1f4`. In that disposable tree ONLY, `internal/api/router.go` was
mutated to **reintroduce the original BUG-080-001 defect**:

1. the `r.Use(deps.GraphCapability.Guard)` fail-soft middleware was **removed**; and
2. the `if deps.GraphCapability.Disabled()` branch was emptied so it registers
   **ZERO routes** — reproducing the original "warning-and-nil / omitted routes
   → bare Chi 404" behavior.

The **UNMODIFIED** test file was then run against it. The worktree was destroyed
immediately afterwards (`git worktree remove --force`).

**`main` was never mutated.** Verified this session:

```text
$ git log -1 --format='%H %ci %s'
6a12f1f4c35a8bfc3aab965b6d674921cc56a47c 2026-07-28 02:51:16 +0000 docs(080 BUG-080-001 F-2): correct stale HARNESS LIMITATION text in graph activation e2e

$ git diff HEAD -- internal/api/router.go
(end of diff)              # empty — main's router.go is byte-identical to HEAD

$ git worktree list
<repo-root>  6a12f1f4 [main]     # only the main tree; the RED worktree is deregistered
```

**Exact mutation applied (verified, not asserted).** Diff of HEAD's repaired
`router.go` against the mutated RED copy:

```text
$ diff <(git show HEAD:internal/api/router.go) <RED-tree>/internal/api/router.go
177d176
<                                       r.Use(deps.GraphCapability.Guard)
181,205c180,182
<                                               // DISABLED: no live handlers exist (no cursor
<                                               // codec). Register the SAME manifest against the
<                                               // typed 503 disabled responder so the paths are
<                                               // present and never fall through to a Chi 404.
<                                               disabled := func(w http.ResponseWriter, _ *http.Request) {
<                                                       deps.GraphCapability.WriteDisabled(w)
<                                               }
<                                               r.Route("/topics", func(r chi.Router) {
<                                                       r.Get("/", disabled)
<                                                       r.Get("/{id}", disabled)
<                                               })
<                                               r.Route("/people", func(r chi.Router) {
<                                                       r.Get("/", disabled)
<                                                       r.Get("/{id}", disabled)
<                                               })
<                                               r.Route("/places", func(r chi.Router) {
<                                                       r.Get("/", disabled)
<                                                       r.Get("/{id}", disabled)
<                                               })
<                                               r.Get("/time", disabled)
<                                               r.Get("/graph/edges", disabled)
---
>                                               // RED MUTATION (throwaway): register NOTHING so a
>                                               // disabled capability falls through to a bare Chi 404 —
>                                               // the ORIGINAL BUG-080-001 defect.
DIFF_EXIT=1
```

This is exactly the two-part defect described above and nothing else: the Guard
line deleted, and all eight canonical route registrations replaced by a comment.

#### Why this RED is a genuine defect reproduction, not a harness artifact

Two independent properties rule out the "the test just couldn't reach the
server" explanation:

1. **It is not an auth artifact.** The test authenticates with
   `SMACKEREL_AUTH_TOKEN`. A missing route therefore yields `404` (no route
   match) rather than the `401` an unauthenticated probe would produce. The
   observed status is `404`, so the request was authenticated and simply found
   no handler.
2. **Nothing could have intercepted ahead of the 404.** Because the disabled
   branch registered zero routes, the surviving
   `RequireScope("knowledge-graph:read")` middleware never executes for those
   paths — there is no route for it to attach to. The `404` is the bare Chi
   no-match, which is precisely the original defect.

Additionally, **all EIGHT canonical families returned 404** — the omission was
total, not path-specific, matching a whole-manifest absence rather than a
single-route typo.

#### RED — raw output

Command: `cd <RED-tree> && ./smackerel.sh test e2e --go-run 'TestE2E_GraphActivation_Disabled'`
Window: `2026-07-28T02:54:16Z` → `2026-07-28T03:00:40Z`. Exit: **`===RED_EXIT=1===`** (non-zero EXPECTED — this is the evidence).

```text
go-e2e: applying -run selector: TestE2E_GraphActivation_Disabled
=== RUN   TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing
    graph_api_activation_e2e_test.go:242: path /api/topics status=404 body=404 page not found
        ; want 503 capability_disabled (never a silent 404 / opaque 500)
    graph_api_activation_e2e_test.go:242: path /api/topics/does-not-exist status=404 body=404 page not found
    graph_api_activation_e2e_test.go:242: path /api/people status=404 body=404 page not found
    graph_api_activation_e2e_test.go:242: path /api/people/does-not-exist status=404 body=404 page not found
    graph_api_activation_e2e_test.go:242: path /api/places status=404 body=404 page not found
    graph_api_activation_e2e_test.go:242: path /api/places/does-not-exist status=404 body=404 page not found
    graph_api_activation_e2e_test.go:242: path /api/time status=404 body=404 page not found
    graph_api_activation_e2e_test.go:242: path /api/graph/edges status=404 body=404 page not found
--- FAIL: TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing (0.03s)
=== RUN   TestE2E_GraphActivation_DisabledAdversarialRedGreen
    graph_api_activation_e2e_test.go:280: RED reproduced: GET /api/topics/ returned a silent 404 nil-handler absence — the original BUG-080-001 behavior; the fail-soft repair regressed
--- FAIL: TestE2E_GraphActivation_DisabledAdversarialRedGreen (0.01s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e        0.296s
FAIL: go-e2e-graph-disabled (exit=1)
===RED_EXIT=1===
```

#### GREEN — raw output

Command: `./smackerel.sh test e2e --go-run 'TestE2E_GraphActivation'` on the
unmutated tree. Window: `2026-07-28T01:22:56Z` → `2026-07-28T02:50:53Z`. Exit:
**`===F2_GREEN_EXIT=0===`**.

```text
go-e2e: applying -run selector: TestE2E_GraphActivation_Disabled
=== RUN   TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing
--- PASS: TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing (0.03s)
=== RUN   TestE2E_GraphActivation_DisabledAdversarialRedGreen
--- PASS: TestE2E_GraphActivation_DisabledAdversarialRedGreen (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.164s
PASS: go-e2e-graph-disabled
===F2_GREEN_EXIT=0===
```

#### Controlled comparison

**The test code is identical in both runs; only the product code differs.**

| | RED | GREEN |
|---|---|---|
| `internal/api/router.go` | mutated (Guard removed, 0 routes registered) | unmutated |
| `tests/e2e/graph_api_activation_e2e_test.go` | unchanged | unchanged |
| Result | `FAIL` — 8/8 paths bare `404` | `PASS` — typed `503 capability_disabled` |
| Exit | `1` | `0` |

**Timing precision (recorded rather than rounded):** the GREEN run ended at
`02:50:53Z` and commit `6a12f1f4` was authored at `02:51:16Z` — 23 seconds
later. GREEN therefore ran against the working tree that *became* `6a12f1f4`,
and the RED worktree was checked out *at* `6a12f1f4` (started `02:54:16Z`).
Since `6a12f1f4` is a comments-and-skip-message-only commit (see F-2 below),
**both runs executed byte-identical assertion code.**

**Row verdict: `[x]`.** The DoD requirement — "first fails against
warning-and-nil/omitted-route behavior, then passes with the repair; both
outputs are recorded" — is now literally satisfied, without any rewording of
the DoD row.

#### Residue disclosure (honest, non-blocking)

`git worktree remove --force` deregistered the RED worktree (`git worktree list`
shows only the main tree; `git rev-parse` inside `/tmp/smk-red` reports "not a
git repository"), but an **orphaned copy of the mutated `router.go` remains on
disk at `/tmp/smk-red/internal/api/router.go`**. It is outside the repository,
untracked, unreferenced by any build, and `main`'s `router.go` is verified
byte-identical to HEAD. It was deliberately left in place this session because
it is the artifact the mutation diff above was computed from; it is scratch
space in `/tmp` and can be deleted at any time. This is disclosed rather than
omitted.

### F-1 — RESOLVED

The RED half is now captured and recorded above. F-1 was closed by **executing
the missing check**, not by rewording the plan-owned DoD row — no DoD
behavioral claim was rewritten to match delivery.

### F-2 — RESOLVED

The stale `HARNESS LIMITATION` narrative in
`tests/e2e/graph_api_activation_e2e_test.go` was corrected in commit
`6a12f1f4` (comments + one `t.Skip` message only; **every assertion
byte-identical**), and the corrected file was live-verified by the GREEN run
recorded above.

```text
$ git show --stat 6a12f1f4
 tests/e2e/graph_api_activation_e2e_test.go | 37 +++++++++++++++++-------------
 1 file changed, 21 insertions(+), 16 deletions(-)

$ grep -c 'HARNESS LIMITATION' tests/e2e/graph_api_activation_e2e_test.go
0
```

The header now states that the disabled core is supplied by the `graph-disabled`
phase of `./smackerel.sh test e2e` via
`docker-compose.graph-disabled.override.yml`, which exports
`SMACKEREL_E2E_GRAPH_DISABLED_URL`. The "documentation alignment" clause of the
Build Quality Gate is therefore satisfied.

### Build Quality Gate — re-run (2026-07-28, this invocation)

All eight commands executed by THIS invocation. Raw output below; exit codes in
the table.

| Gate | Command | Exit |
|---|---|---|
| Check | `./smackerel.sh check` | 0 |
| Lint | `./smackerel.sh lint` | 0 |
| Format | `./smackerel.sh format --check` | 0 |
| PII scan | `bash .github/bubbles/scripts/pii-scan.sh` | 0 |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh $B` | 0 |
| Traceability guard | `bash .github/bubbles/scripts/traceability-guard.sh $B` | 0 |
| Regression quality guard | `bash .github/bubbles/scripts/regression-quality-guard.sh tests/e2e/graph_api_activation_e2e_test.go` | 0 |
| Regression quality guard (`--bugfix`) | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/graph_api_activation_e2e_test.go` | 0 |

```text
$ ./smackerel.sh check
config-validate: <repo-root>/config/generated/dev.env.tmp.433307 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0

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

$ ./smackerel.sh format --check
78 files already formatted
FORMAT_CHECK_EXIT=0

$ bash .github/bubbles/scripts/regression-quality-guard.sh tests/e2e/graph_api_activation_e2e_test.go
============================================================
  BUBBLES REGRESSION QUALITY GUARD
  Repo: <repo-root>
  Timestamp: 2026-07-28T03:08:23Z
  Bugfix mode: false
============================================================

ℹ️  Scanning tests/e2e/graph_api_activation_e2e_test.go

============================================================
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
============================================================
RQG_EXIT=0

$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/graph_api_activation_e2e_test.go
============================================================
  BUBBLES REGRESSION QUALITY GUARD
  Repo: <repo-root>
  Timestamp: 2026-07-28T03:08:23Z
  Bugfix mode: true
============================================================

ℹ️  Scanning tests/e2e/graph_api_activation_e2e_test.go
✅ Adversarial signal detected in tests/e2e/graph_api_activation_e2e_test.go

============================================================
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
  Files with adversarial signals: 1
============================================================
RQG_BUGFIX_EXIT=0
```

<!-- PII-safe: absolute operator paths are rendered as <repo-root> per the repo's
     no-env-specific-content policy (gitleaks rule linux-home-username-leak). -->

```text
$ bash .github/bubbles/scripts/pii-scan.sh
3:11AM INF 0 commits scanned.
3:11AM INF scan completed in 26ms
3:11AM INF no leaks found
🫧 pii-scan: clean.
PII_SCAN_EXIT=0

$ bash .github/bubbles/scripts/artifact-lint.sh $B
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
✅ uservalidation checklist has checked-by-default entries
✅ All checklist bullet items use checkbox syntax
✅ Detected state.json status: blocked
✅ Detected state.json workflowMode: bugfix-fastlane
✅ state.json v3 has required field: status
✅ state.json v3 has required field: execution
✅ state.json v3 has required field: certification
✅ state.json v3 has required field: policySnapshot
✅ state.json v3 has recommended field: transitionRequests
✅ state.json v3 has recommended field: reworkQueue
✅ state.json v3 has recommended field: executionHistory
✅ Top-level status matches certification.status
ℹ️  Workflow mode 'bugfix-fastlane' allows status 'done'; current status is 'blocked'
✅ report.md contains section matching: Summary
✅ report.md contains section matching: Completion Statement
✅ report.md contains section matching: Test Evidence
✅ Mode-specific report gates skipped (status not in promotion set)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0

$ bash .github/bubbles/scripts/traceability-guard.sh $B
============================================================
  BUBBLES TRACEABILITY GUARD
  Feature: <repo-root>/specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable
  Timestamp: 2026-07-28T03:12:07Z
============================================================

--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 14 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist

ℹ️  Checking traceability for Scope 1: Fail-Soft Graph Activation Foundation
✅ Scope 1 scenario mapped to Test Plan row: SCN-080-001-01 ...
✅ Scope 1 scenario maps to concrete test file: internal/api/graphapi/activation_test.go
✅ Scope 1 report references concrete test evidence: internal/api/graphapi/activation_test.go
✅ Scope 1 scenario mapped to Test Plan row: SCN-080-001-02 ...
✅ Scope 1 scenario mapped to Test Plan row: SCN-080-001-07 ...
ℹ️  Scope 1 summary: scenarios=3 test_rows=7
   (Scopes 2-4 checked identically; all ✅ — full transcript in the session log.)

--- Gherkin → DoD Content Fidelity (Gate G068) ---
✅ 14/14 scenarios map to DoD items (unmapped: 0)

--- Traceability Summary ---
ℹ️  Scenarios checked: 14
ℹ️  Test rows checked: 30
ℹ️  Scenario-to-row mappings: 14
ℹ️  Concrete test file references: 14
ℹ️  Report evidence references: 14
ℹ️  DoD fidelity scenarios: 14 (mapped: 14, unmapped: 0)
ℹ️  Edge confidence (IMP-015 Scope B): declared=14 inferred=0 ambiguous=14

RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0
```

**All 8 gate commands exited 0.**

### Build Quality Gate assessment (SCOPE-01) — 2026-07-28 re-verdict

| Clause | Verdict |
|---|---|
| Scope-specific unit / integration / E2E regressions pass | ✅ unit + integration green; e2e green (`===F2_GREEN_EXIT=0===`) |
| `./smackerel.sh check` | ✅ exit 0 |
| `./smackerel.sh lint` | ✅ exit 0, `All checks passed!` |
| `./smackerel.sh format --check` | ✅ exit 0, `78 files already formatted` |
| source-lock / config checks | ✅ folded into `check` (config-in-sync + env_file drift guard OK) |
| artifact-lint | ✅ exit 0 |
| traceability guard | ✅ exit 0 |
| zero warnings | ✅ 0 warnings across lint, traceability, and both regression guards |
| change-boundary review | ✅ amended + attributed — see scopes.md "Change Boundary" |
| **documentation alignment** | ✅ **NOW SATISFIED** — F-2 resolved in `6a12f1f4`; `grep -c 'HARNESS LIMITATION'` = 0 |
| **no skipped checks** | ✅ **NOW SATISFIED** — F-1 resolved; the T080-02-ADVERSARIAL RED capture was executed and is recorded above |

**Row verdict: `[x]`.** Both previously-failing clauses are satisfied by
executed evidence; every other clause remains green.

### DoD rows closed this invocation

- **Test Evidence T080-02-ADVERSARIAL** → `[x]` (RED `===RED_EXIT=1===` + GREEN `===F2_GREEN_EXIT=0===`, same test bytes, differing product code).
- **Build Quality Gate (SCOPE-01)** → `[x]` (all 8 gates exit 0; both blocking clauses satisfied).

**SCOPE-01 is now fully closed: 0 unchecked rows.**

### Scope closure

`SCOPE-01` status → `Done` in `scopes.md`; `certification.scopeProgress[SCOPE-01]`
→ `"done"` in `state.json`.

**The bug's top-level `status` and `certification.status` REMAIN `blocked`.**
SCOPE-02 is `in_progress` and SCOPE-03/04 are `blocked`, with ~46 DoD rows still
unchecked across them. No SCOPE-02/03/04 row was touched by this invocation.
No `git add`, `git commit`, or `git push` was performed.

## Cursor Completeness Guard RED→GREEN (bubbles.implement, 2026-07-28)

### T080-06-CURSOR

**Scenario:** SCN-080-001-06 — a non-terminal page can never lose its cursor.
**Tier:** `unit`.
**Test:** `internal/api/graphapi/cursor_test.go` -
`TestNonTerminalPageCannotLoseCursorEncodeFailure`.
**Claim Source:** executed — both halves captured in this session.

#### The defect, stated plainly

Previously, when `hasNext == true` **and** the cursor codec was unusable (encode
error, or a `nil` codec that was never wired), the handler answered **HTTP 200
with an empty `nextCursor`**. Every client reads an empty `nextCursor` as "this
was the last page", so the remaining pages were dropped without any error
surface: **silent data truncation**.

The fix makes that condition a typed **HTTP 500 `schema_error`**, per
`design.md` § "Completeness Envelope". It spans all four paginated families —
`internal/api/graphapi/topics.go`, `people.go`, `places.go`, `edges.go` — plus
the new typed errors `ErrSchemaError` (500) and `ErrStoreUnavailable` (503) in
`internal/api/graphapi/errors.go`.

#### Test-location honesty note

The Test Plan row for T080-06-CURSOR names `cursor_test.go`. The test had been
delivered as duplicated per-family cases inside `topics_test.go` and
`edges_test.go`. Those duplicates were **removed** and the test was
**consolidated into `cursor_test.go`** so the shipped code CONFORMS to the
planned Test Plan row. The plan was **not** edited to match the code — the code
was moved to match the plan.

#### RED — defect temporarily reintroduced in `topics.go`

Only the cursor guard in `internal/api/graphapi/topics.go` was reverted to the
old silent form (`if encErr == nil { next = encoded }`, keeping `if hasNext {`
so a `nil` codec still reaches `Encode`'s nil-receiver guard). The test file was
**not** touched. Command and raw output (terminal hard-wrap at 80 columns
unwrapped; bytes otherwise verbatim):

```text
$ ./smackerel.sh test unit --go --go-run 'TestNonTerminalPageCannotLoseCursorEncodeFailure'
--- FAIL: TestNonTerminalPageCannotLoseCursorEncodeFailure (0.00s)
    --- FAIL: TestNonTerminalPageCannotLoseCursorEncodeFailure/topics (0.00s)
        --- FAIL: TestNonTerminalPageCannotLoseCursorEncodeFailure/topics/adversarial_non_terminal_page_with_unusable_codec_is_500_schema_error (0.00s)
            --- FAIL: TestNonTerminalPageCannotLoseCursorEncodeFailure/topics/adversarial_non_terminal_page_with_unusable_codec_is_500_schema_error/nil_codec (0.00s)
                cursor_test.go:334: fail-soft regression: non-terminal page answered 200; a lost cursor must never look like the last page (body={"items":[{"id":"T1","label":"label-T1","linkedArtifactCount":0,"peopleCount":0,"placeCount":0},{"id":"T2","label":"label-T2","linkedArtifactCount":0,"peopleCount":0,"placeCount":0}],"nextCursor":""}
                    )
            --- FAIL: TestNonTerminalPageCannotLoseCursorEncodeFailure/topics/adversarial_non_terminal_page_with_unusable_codec_is_500_schema_error/codec_without_secret (0.00s)
                cursor_test.go:334: fail-soft regression: non-terminal page answered 200; a lost cursor must never look like the last page (body={"items":[{"id":"T1","label":"label-T1","linkedArtifactCount":0,"peopleCount":0,"placeCount":0},{"id":"T2","label":"label-T2","linkedArtifactCount":0,"peopleCount":0,"placeCount":0}],"nextCursor":""}
                    )
        --- FAIL: TestNonTerminalPageCannotLoseCursorEncodeFailure/topics/value_safety_500_body_discloses_no_secret_or_cursor_material (0.00s)
            cursor_test.go:406: unusable codec: want 500, got 200 (body={"items":[{"id":"T1","label":"label-T1","linkedArtifactCount":0,"peopleCount":0,"placeCount":0},{"id":"T2","label":"label-T2","linkedArtifactCount":0,"peopleCount":0,"placeCount":0}],"nextCursor":""}
                )
FAIL
FAIL    github.com/smackerel/smackerel/internal/api/graphapi    0.005s
FAIL
===RED_EXIT=1===
```

The failure body is the defect itself, verbatim: `"nextCursor":""` alongside a
`200` on a page that has more rows behind it.

#### Restore verification (byte-identical)

`topics.go` was restored before the GREEN run and the restore was verified
against the pre-RED baseline, not asserted:

```text
$ git --no-pager diff --stat internal/api/graphapi/topics.go
 internal/api/graphapi/topics.go | 17 ++++++++++++++---
 1 file changed, 14 insertions(+), 3 deletions(-)
```

`14 insertions(+), 3 deletions(-)` is exactly the pre-RED baseline, and the full
`git diff` shows the typed `WriteAPIError(w, ErrSchemaError); return` guard back
in place.

#### GREEN — same test bytes, repaired product code

```text
$ ./smackerel.sh test unit --go --go-run 'TestNonTerminalPageCannotLoseCursorEncodeFailure'
ok      github.com/smackerel/smackerel/internal/api/graphapi    0.011s
===GREEN_EXIT=0===
```

The `ok` line carries **no** `[no tests to run]` suffix, so the `-run` filter
genuinely matched and executed the consolidated test.

#### Supporting gates (verified in this session's work stream)

```text
$ ./smackerel.sh check              → CHECK_EXIT=0
$ ./smackerel.sh format --check     → FORMAT_EXIT=0
$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/api/graphapi/cursor_test.go
✅ Adversarial signal detected
0 violations
BUGFIX_GUARD_EXIT=0
```

**Row verdict: `[x]`.** The test lives where the Test Plan row says it lives, it
fails against the reintroduced defect and passes against the fix with identical
test bytes, and the supporting quality gates are clean.

Only the T080-06-CURSOR row was closed. No other SCOPE-02 row, and no
`state.json` field, was modified: SCOPE-02 stays `in_progress` and the bug stays
`blocked`. No `git add`, `git commit`, or `git push` was performed.


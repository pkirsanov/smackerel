# Report: [BUG-080-001] Graph API Fails Soft Into Runtime Disablement

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning artifacts only were initialized on 2026-07-23. No source, secret, config generation, host, operator deploy repository, test, production, commit, push, or deployment mutation occurred.

## Completion Statement

Incomplete and non-terminal. Status remains `in_progress`; design/planning, reproduction, implementation, devops injection, testing, validation, and audit are unclaimed.

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

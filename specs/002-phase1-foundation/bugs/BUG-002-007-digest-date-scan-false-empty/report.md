# Report: [BUG-002-007] Digest Date Scan Produces False Empty

Links: [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [uservalidation.md](uservalidation.md)

## Summary

On 2026-07-23, `bubbles.plan` replaced the preliminary single-scope handoff with four dependency-ordered executable scopes: the operator-owned global-corpus grant + freshness contract gate, the canonical typed reader, truthful server-rendered states, and disposable real-PostgreSQL browser acceptance. On 2026-07-24 a planning reconciliation re-derived the plan against the revised `spec.md`/`design.md`: the digest authorization contradiction is resolved to one operator-owned global corpus with `digest:read`/`digest:generate` grants (no tenant/per-user row isolation), `scenario-manifest.json` now covers all ten scopes-defined scenarios with existing linked-test anchors, and this report records the concrete canary anchors.

No source, test, database, browser, production, requirements, design, certification, commit, push, or deployment mutation occurred.

## Completion Statement

Non-terminal and **blocked**. `bubbles.implement` (2026-07-24) landed the disjoint unit-verifiable false-empty core (Scope 02/03 web reader composition + typed state determination) with passing Go unit tests, `check`, and `lint`. The packet is BLOCKED on two dependencies recorded in `state.json.blockedReason`: (1) Scope 01 freshness/ownership SST is coordination-required — it edits `internal/config/config.go`, `internal/config/validate_test.go`, `config/smackerel.yaml`, and `scripts/commands/config.sh`, which a concurrent agent owns, so those files were not touched and stale determination is wired inert (`DigestStaleAfter = 0`); and (2) every live-stack row (integration / e2e-api / e2e-ui, including the unauthorized/grant boundary and all of Scope 04) was intentionally deferred to avoid the shared Docker stack. No certification, audit, commit, push, or deployment occurred.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** the database has a current approximately 380-word digest, but the legacy page shows "No digest generated yet" after a date-to-string scan error is silently replaced with an empty model.
- **Evidence status:** no SQL, server log, browser, or command output was captured here.

## Decision Record

- Digest ownership is the single operator-owned global corpus. `digests` has no ownership column; authorization is grant-gated (`digest:read` reader, `digest:generate` producer) at the capability boundary, never a per-user row predicate. This resolves the earlier product-session-versus-per-user contradiction and is now consistently named across `spec.md`, `design.md`, `scopes.md`, and `scenario-manifest.json`.
- Stale age is an explicit required SST value (no hidden default); missing or invalid freshness config fails startup.
- Only wrapped `pgx.ErrNoRows` maps to an empty state; every other query/scan/decode/connection fault stays a typed error and never a false empty.
- True-empty, selected-date-miss, quiet, stale/degraded, unauthorized, and read-error remain distinct, individually testable states.
- The stored row and scan failure are treated as operator-supplied findings, not locally executed proof.
- Real PostgreSQL round-trip coverage is mandatory because template-only tests cannot catch scan-type drift; no database mock, response interception, auth injection, or bailout may satisfy live rows.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

**Phase:** implement  
**Claim Source:** executed (this invocation, 2026-07-24)

### Unit — `./smackerel.sh test unit --go --verbose --go-run '<digest/web selectors>'`

**Exit Code:** 0 (`UNIT_EXIT=0`; the wrapper ran `go test ./...`, so the whole module compiled — `cmd/core` included).

```
=== RUN   TestClassifyDigestStateMatrix
--- PASS: TestClassifyDigestStateMatrix (0.00s)
=== RUN   TestDigestPageTruthfulHTTPStates
--- PASS: TestDigestPageTruthfulHTTPStates (0.01s)
=== RUN   TestNewHandler_TemplateFuncs
--- PASS: TestNewHandler_TemplateFuncs (0.00s)
=== RUN   TestDigestPage_NoRows
--- PASS: TestDigestPage_NoRows (0.00s)
=== RUN   TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix
--- PASS: TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix (0.02s)
=== RUN   TestAllTemplates_Present
--- PASS: TestAllTemplates_Present (0.00s)
=== RUN   TestTemplates_NoInlineEventHandlers
--- PASS: TestTemplates_NoInlineEventHandlers (0.00s)
ok      github.com/smackerel/smackerel/internal/web     0.231s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

The 002-006 search regression (`TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix`) still passes — the just-committed search changes are preserved.

### Compile / SST — `./smackerel.sh check`

**Exit Code:** 0

```
config-validate: .../config/generated/dev.env.tmp OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

### Lint — `./smackerel.sh lint`

**Exit Code:** 0 (`LINT_EXIT=0`; `Web validation passed`, `All checks passed!`; zero findings in the touched Go files).

## Implementation Evidence (bubbles.implement — 2026-07-24)

### Scope 02 Core

**Reader composition + duplicate-SQL removal — DONE (unit + compile verified).**

- NEW `internal/web/digest_model.go`: the narrow `DigestReader` seam (`GetLatest(ctx, date) (*digest.Digest, error)`, satisfied by the existing `*digest.Generator`), the closed `DigestViewState` / `DigestReadErrorKind` vocabularies, the concrete `DigestPageModel`, and the pure `classifyDigest` determination.
- `internal/web/handler.go::DigestPage` REWRITTEN onto the injected reader. The confirmed root-cause path — `var digestText, digestDate string` + `Scan(&digestText, &digestDate, ...)` on a `DATE` column + the `if err != nil { digestText = "No digest generated yet."; digestDate = time.Now()... }` catch-all — is REMOVED. A grep confirms no raw `digests` SQL or `time.Now()` date substitution remains in the web page path:

```
$ grep -n "No digest generated yet\|digest_date, is_quiet FROM digests\|time.Now().Format" internal/web/handler.go
(no matches)
```

- `cmd/core/services.go` injects `svc.webHandler.DigestReader = svc.digestGen` immediately after `web.NewHandler(...)`; `go test ./...` compiled the whole module (including `cmd/core`) with exit 0, proving the wiring builds.
- Only a wrapped `pgx.ErrNoRows` yields an empty model; every other fault is a typed `read_error` (HTTP 500) with cleared digest-derived fields — proved by `TestClassifyDigestStateMatrix` and `TestDigestPageTruthfulHTTPStates`, including the exact regression that the old handler returned HTTP 200 + `"No digest generated yet."` + today's date for a read error.

**Deferred within Scope 02 (NOT claimed):** the real-PostgreSQL `DATE`/`TIMESTAMPTZ` round-trip (SCN-002-007-01/02, DIGEST-S02-T02), the real scan-fault profile (SCN-002-007-03, DIGEST-FP-DB-SCAN-001), and the e2e-api/e2e-ui rows — all live-stack, deferred this session.

### Scope 03 Progress (states + template — unit core only)

- `digest.html` (`internal/web/templates.go`) expanded to mutually-exclusive `data-digest-state` branches: current / quiet / stale / first_use_empty / selected_date_empty / read_error.
- `TestDigestPageTruthfulHTTPStates` proves distinct HTTP + DOM markers for current (200, populated, not empty), first-use empty (200), selected-date miss (200, distinct from first-use, exact date reaches the reader), stale (200, degraded, stored prose retained), read_error (500, no digest fields, no today's-date), and the deferred-config honesty case (unconfigured threshold → current, never arbitrarily stale).
- **Deferred (NOT claimed):** SCN-002-007-07 unauthorized/grant boundary (needs the live auth stack), `smackerel_digest_read_total` telemetry, and all integration/e2e rows. Scope 03 DoD checkboxes remain `[ ]` pending those live proofs.

## Uncertainty Declarations

- Exact SQL query, date type, nullable behavior, and error-swallowing branch are not locally confirmed.
- No red or green regression output exists.

## Scenario Contract Evidence

The ten scenarios are assigned to Scopes 01-04 in [scenario-manifest.json](scenario-manifest.json): Scope 01 (SCN-002-007-09, 10), Scope 02 (SCN-002-007-01, 02, 03), Scope 03 (SCN-002-007-04, 05, 06, 07), Scope 04 (SCN-002-007-08). Existing canaries are linked; not-yet-authored targets use `plannedTests`; evidence references remain empty until execution.

### Existing Canary Anchors

The report references the concrete planning anchors used by traceability: `internal/config/validate_test.go`, `internal/digest/generator_test.go`, `internal/api/digest.go`, `internal/web/handler_test.go`, `tests/integration/guesthost_digest_test.go`, `tests/e2e/test_digest.sh`, `tests/e2e/test_digest_quiet.sh`, `tests/e2e/test_digest_pipeline.sh`, `tests/e2e/test_web_ui.sh`, `web/pwa/tests/unified_journey.spec.ts`, and `web/pwa/tests/auth_login.spec.ts`.

## Planning Validation

Packet-local validators were re-run during this reconciliation and both pass.

### Artifact Lint

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation/bugs/BUG-002-007-digest-date-scan-false-empty`  
**Exit Code:** 0  
**Claim Source:** executed  
**Result:** `Artifact lint PASSED.`

### Traceability Guard

**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation/bugs/BUG-002-007-digest-date-scan-false-empty`  
**Exit Code:** 0  
**Claim Source:** executed  
**Result:** `RESULT: PASSED (0 warnings)` — 10 scenarios checked, 10 mapped to DoD, 22 test rows, 10 report evidence references, and all linked tests exist (baseline was 17 failures).

## Validation Summary

No completion certification was performed. This packet remains planning-only and `in_progress`; implementation routing awaits clean packet-local validators.

## Audit Verdict

Not audited. No terminal verdict is claimed.

# Specification: BUG-024-004 spec 024 state.json missing top-level `certifiedAt` triggers Gate G088 BLOCK

**Status:** Done

## Business Context

Sweep round 19 of `sweep-2026-06-06-r20` (`mode: gaps-to-doc`, parent-expanded) ran the `gaps` trigger probe on `specs/024-design-doc-reconciliation` and surfaced one BLOCKING governance gap that survived both BUG-024-002's reconcile-to-doc close-out (2026-05-24) and BUG-024-003's chaos-hardening close-out (2026-05-27):

1. **F1 (MEDIUM, BLOCKING).** `specs/024-design-doc-reconciliation/state.json` is missing the top-level `"certifiedAt"` string field. Gate G088 (`post_certification_spec_edit_gate`, enforced by `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` lines 179-184) requires this field for every spec with `status: done`. Without it, `state-transition-guard.sh` exits 1 with `🔴 BLOCK: Post-certification spec edit guard failed — Gate G088` and never reaches its remaining 28 checks. Two sibling bug packets in the same parent (BUG-024-002, BUG-024-003) already comply — the parent spec itself was overlooked when G088's contract was tightened.

This packet runs as `mode: gaps-to-doc` (matching the sweep round's mapped child mode for the `gaps` trigger) because the finding is a governance-completeness gap: the certification record is present in spirit (`certification.status: done`, per-scope `certifiedAt` entries, full `executionHistory`), but the explicit top-level field the framework guard reads is absent. Closing the gap is a single-field state.json backfill that restores end-to-end gate compliance without touching planning truth.

## Use Cases

### UC-01: Restore Gate G088 Compliance For Spec 024
**Actor**: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` (every sweep round, every CI pre-push gate run, every future recertification)
**Goal**: After the backfill, the guard exits 0 with `🟢 TRANSITION ALLOWED` (carrying only the 2 pre-existing non-blocking WARNs that are out of scope for this bug); `post-cert-spec-edit-guard.sh` exits 0 with `PASS Gate G088`.
**Outcome**: Spec 024 is once again governance-current; downstream sweep rounds can progress past Check 23B to evaluate the remaining 28 gate dimensions.

### UC-02: Preserve The Substantive Certification Of The Two Scopes
**Actor**: A reader of `state.json` (auditor, future agent, certification reviewer)
**Goal**: All existing certification provenance — `status: done`, `certification.status: done`, `certification.scopeProgress[0..1].status: done`, `certification.scopeProgress[0..1].certifiedAt`, `certification.completedScopes: ["1","2"]`, `certification.certifiedCompletedPhases` (15 phases), `executionHistory[]` (25 entries) — is preserved verbatim. The backfill is additive only.
**Outcome**: The 2026-04-10 certification of Scope 1 (OpenClaw + Storage Reconciliation) and Scope 2 (Competitive Matrix + Phased Plan + Connectors) stays intact; the new top-level `certifiedAt` records the moment after which no planning truth (spec.md / design.md / scopes.md) has been touched.

### UC-03: Preserve Runtime Behavior Verbatim
**Actor**: Connector registry, every committed connector, all REST endpoints, NATS publishers, search/digest paths, the `TestConnectorCountContract` test family from BUG-024-003
**Goal**: Runtime code (`cmd/core/*.go`, `internal/connector/*`, `internal/api/*`, `internal/config/*`, `internal/web/*`, `internal/notification/*`, `internal/pipeline/*`, `ml/`), schema, NATS topology, prompt contracts, web templates, Telegram commands, deploy scripts, compose files, `smackerel.yaml`, and `internal/deploy/docs_connector_count_contract_test.go` are unchanged.
**Outcome**: `./smackerel.sh test unit --go` continues to pass; `go test -run TestConnectorCountContract ./internal/deploy/...` continues to pass 4/4 sub-tests; baseline behavior preserved.

## Functional Requirements

### FR-01: Add Top-Level `certifiedAt` String To Spec 024 `state.json`
**Description**: Insert a top-level `"certifiedAt"` key on `specs/024-design-doc-reconciliation/state.json`. The value MUST be an RFC3339 timestamp string **strictly greater than** the latest commit timestamp that touched any of `spec.md`, `design.md`, `scopes.md`, `scopes/_index.md`, or `scopes/*/scope.md` — because `post-cert-spec-edit-guard.sh` invokes `git log --since=$certifiedAt` which is INCLUSIVE of commits at the exact same instant. The latest such commit is `19b31c0a` `bubbles(ops/OPS-001): sweep spec.md status banners across 54 certified specs` dated `2026-05-28T05:07:50+00:00`. The chosen value is `"2026-05-28T05:07:51Z"` — 1 second after the OPS-001 commit, the smallest RFC3339 increment that excludes it from the post-cert window.
**Acceptance**: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d.get('certifiedAt') == '2026-05-28T05:07:51Z'"` exits 0.

### FR-02: Gate G088 Direct Diagnostic Passes
**Description**: After FR-01, `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation` exits 0 with the canonical PASS message.
**Acceptance**: `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation; echo "EXIT=$?"` prints `post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=specs/024-design-doc-reconciliation status=done certifiedAt=2026-05-28T05:07:51Z trackedFiles=3` and `EXIT=0`.

### FR-03: state-transition-guard.sh Exits 0 After FR-01
**Description**: After FR-01, the full state-transition-guard exits 0 with `🟢 TRANSITION ALLOWED`. The 2 pre-existing non-blocking WARNs (`No completedAt timestamps found in state.json`, `No concrete test file paths found in Test Plan across resolved scope files`) are advisory only and are out of scope for this bug — both pre-date BUG-024-004 and are documented in BUG-024-003's WARN inventory.
**Acceptance**: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -3` shows `🟢 TRANSITION ALLOWED (with N warnings)` and `EXIT=0`. The failure count drops from `1` (pre-fix) to `0` (post-fix).

### FR-04: Backfill BUG-024-004 Closure Provenance Into Parent `state.json`
**Description**: Extend `specs/024-design-doc-reconciliation/state.json` `executionHistory[]` with one entry per closure phase executed for BUG-024-004 (analyze, design, plan, implement, test, regression, simplify, stabilize, security, chaos, validate, audit, docs, finalize — 14 entries; the `bug` phase is implicit in the existence of the bug folder and may be omitted from the parent's history). Extend the parent's `resolvedBugs[]` array (creating it if absent) with one BUG-024-004 entry carrying `bugId`, `closedAt`, `finalStatus: resolved`, `summary`. Bump `lastUpdatedAt` to `2026-06-06T00:00:00Z` (creating the field if absent).
**Acceptance**: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert any(b.get('bugId') == 'BUG-024-004' for b in d.get('resolvedBugs', []))"` exits 0; `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d.get('lastUpdatedAt', '') >= '2026-06-06'"` exits 0; `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); h=d['executionHistory']; assert sum(1 for e in h if 'BUG-024-004' in (e.get('summary') or '')) >= 7"` exits 0.

### FR-05: Append BUG-024-004 Closure Section To Parent `report.md`
**Description**: Append a new section `## BUG-024-004 Gaps-Sweep Resolution (2026-06-06)` to `specs/024-design-doc-reconciliation/report.md` carrying a Code Diff Evidence table (which files changed, what changed, which SCN scenarios are covered) plus a Git-Backed Proof block with raw pre-fix and post-fix outputs from `state-transition-guard.sh`, `post-cert-spec-edit-guard.sh`, `artifact-lint.sh`, `artifact-freshness-guard.sh`, and `traceability-guard.sh`. All terminal evidence MUST redact absolute `/home/<user>/` paths to `~/`.
**Acceptance**: `grep -cE '^## BUG-024-004 Gaps-Sweep Resolution' specs/024-design-doc-reconciliation/report.md` returns `1`; `grep -nE '/home/<user>/' specs/024-design-doc-reconciliation/report.md` returns 0 hits (gitleaks `linux-home-username-leak` will not fire on the new section).

### FR-06: Single Atomic Commit Lands All Changes With Structured Prefix
**Description**: All BUG-024-004 packet artifacts + the parent state.json edit + the parent report.md append land in a single atomic commit whose subject begins with `bubbles(024/bug-024-004):`.
**Acceptance**: `git log --oneline -1 --format='%s'` after commit begins with `bubbles(024/bug-024-004):`. `git diff --cached --name-status` before commit shows only files under `specs/024-design-doc-reconciliation/` (parent state.json + parent report.md + 8 new files under `bugs/BUG-024-004-state-missing-certifiedat-g088/`); zero stray staging from `specs/055-*`, `specs/044-*`, `cmd/`, `internal/`, `ml/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical 5 scenarios (BUG-024-004-SCN-001..SCN-005) that drive this packet. Their Gherkin rendering lives in `scopes.md`.

## Acceptance Criteria

- AC-01: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d.get('certifiedAt') == '2026-05-28T05:07:51Z'"` exits 0; the top-level field is present and is exactly 1 second after the OPS-001 sweep timestamp (the smallest RFC3339 increment that excludes the OPS-001 commit from `git log --since`).
- AC-02: `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation` exits 0 with `PASS Gate G088 (post_certification_spec_edit_gate)`.
- AC-03: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 0 with `🟢 TRANSITION ALLOWED`; failure count drops from 1 (pre-fix) to 0 (post-fix); the 2 pre-existing non-blocking WARNs survive unchanged (out of scope).
- AC-04: `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` continues to exit 0 with `Artifact lint PASSED.`
- AC-05: `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation` continues to exit 0 with `RESULT: PASS (0 failures, 0 warnings)`.
- AC-06: `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` continues to exit 0 with `RESULT: PASSED (0 warnings)`.
- AC-07: `go test -run TestConnectorCountContract ./internal/deploy/...` continues to exit 0 with all 4 sub-tests PASS (live + 3 adversarial); the BUG-024-003 forward-detection contract is preserved.
- AC-08: Spec 024 `status` remains `done`. `certification.status`, `certification.completedScopes`, `certification.certifiedCompletedPhases`, and both per-scope `certifiedAt` entries are preserved verbatim. The `executionHistory[]` count grows from 25 to ≥ 32 (≥ 7 BUG-024-004 phase entries appended).
- AC-09: `resolvedBugs[]` array exists with at least one entry carrying `bugId: "BUG-024-004"`, `finalStatus: "resolved"`, and a closedAt timestamp ≥ `2026-06-06`. Top-level `lastUpdatedAt` ≥ `2026-06-06`.
- AC-10: `specs/024-design-doc-reconciliation/report.md` has exactly 1 occurrence of `## BUG-024-004 Gaps-Sweep Resolution`; the section contains Code Diff Evidence + Git-Backed Proof block; zero absolute `/home/<user>/` paths in the new section.
- AC-11: BUG-024-004 packet's own gates pass: `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-004-state-missing-certifiedat-g088` exits 0.
- AC-12: BUG-024-004 packet's `state.json::status` is `done` (or `resolved` per bug schema) with `executionHistory[]` containing complete provenance for `bug`, `analyze`, `design`, `plan`, `implement`, `test`, `regression`, `simplify`, `stabilize`, `security`, `chaos`, `validate`, `audit`, `docs`, `finalize` phases (15 entries); top-level `certifiedAt` + `certifiedBy` populated on the bug state.json itself.
- AC-13: Single commit with subject prefix `bubbles(024/bug-024-004):` satisfies Check 17 structured commit gate.
- AC-14: Path-limited `git add specs/024-design-doc-reconciliation/` discipline confirmed via `git diff --cached --name-status`. Zero files from `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, `cmd/`, `internal/connector/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `internal/deploy/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, or `ml/` swept in.
- AC-15: Sweep ledger entry recorded (round 19 of 20, gaps trigger, mapped child mode `gaps-to-doc`, executionModel `parent-expanded-child-mode`, findings=1, findingsClosedThisRound=1, bugsSpawned=1, bugId=BUG-024-004, bugFinalStatus=resolved, guardsClean=all).

## Product Principle Alignment

**Principle 8 — Trust Through Transparency.** Adding the explicit `certifiedAt` top-level field restores a transparency contract: any reader of `state.json` (auditor, future agent, certification reviewer) can see WHEN the spec was certified at a glance, and the framework guard can mechanically verify the certification has not silently aged past planning-truth edits. The OPS-001 sweep moment is the principled choice because it's the latest moment all tracked files reached agreement.

**Principle 3 — Knowledge Breathes (Lifecycle, Not Static).** G088 enforces that certified planning truth has a lifecycle: once certified, the planning files (spec.md / design.md / scopes.md) cannot drift silently. The top-level `certifiedAt` is the pivot the lifecycle hangs on — without it, the lifecycle gate has no anchor. The backfill restores the anchor and the breathing rhythm of the certification model.

**Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product).** This packet does NOT touch `internal/connector/qfdecisions/`, `cmd/core/connectors.go` line 52, the QF Decisions row in `docs/smackerel.md` §22.7, the QF Decisions entry in `docs/Development.md` L31, or the R-006 16th entry in `spec.md`. The Principle 10 boundary text `no financial advice generation` is preserved verbatim. Spec 041's contract is untouched.

## Non-Goals

- This packet will **not** modify any production runtime code (`cmd/core/*.go`, `internal/connector/*`, `internal/api/*`, `internal/config/*`, `internal/web/*`, `internal/notification/*`, `internal/pipeline/*`, `internal/deploy/*` including the `docs_connector_count_contract_test.go` from BUG-024-003), schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, or `smackerel.yaml` value.
- This packet will not modify `docs/smackerel.md`, `docs/Development.md`, `docs/Operations.md`, `docs/Architecture.md`, `docs/Deployment.md`, `docs/INVESTOR_OVERVIEW.md`, or any other top-level docs file. The fix is state.json-only on the governance side, plus parent report.md narrative extension.
- This packet will not change spec 024's overall `status` away from `done`. It also will not change `certification.status`, `certification.completedScopes`, `certification.certifiedCompletedPhases`, or either per-scope `certifiedAt` entry.
- This packet will not weaken any framework guard. The fix satisfies G088 by adding the required field, not by relaxing the gate.
- This packet will not touch any in-flight WIP under `specs/055-*` or `specs/044-per-user-bearer-auth/state.json` even if those files appear in `git status` at HEAD. Path-limited `git add specs/024-design-doc-reconciliation/` enforces this.
- This packet will not push to remote. The pre-push hook (`./smackerel.sh test pre-push`, ~25 min) is deferred to a downstream operator/parent-workflow step; the closure here ends with a clean local commit. The state.json `finalize` summary records "commit landed; push deferred to parent sweep" honestly.
- This packet will not address the 2 pre-existing non-blocking WARNs (`No completedAt timestamps found in state.json`, `No concrete test file paths found in Test Plan across resolved scope files`). Both pre-date BUG-024-004 and are documented as out of scope; treating them would mix two distinct findings under one bug packet and violate scope-size discipline (Gate G037).

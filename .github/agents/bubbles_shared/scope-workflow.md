<!-- governance-version: 3.0.0 -->
# Shared Scope Workflow (Common to iterate & implement)

> **This file defines scope-specific workflow for `bubbles.iterate` and `bubbles.implement`.**
> For common patterns (Loop Guard, User Validation, etc.), see [agent-common.md](agent-common.md).
> For project-specific command resolution, see [project-config-contract.md](project-config-contract.md).
> 
> **Portability:** This file is **project-agnostic**. Commands are referenced as `[cmd]` placeholders resolved from `.specify/memory/agents.md`.

---

## ⚠️ PREREQUISITE: Common Patterns

**This file extends [agent-common.md](agent-common.md).** All patterns from that file apply:

- Loop Guard rules
- **Classified Work Folder Resolution - FAIL FAST (ONE search only, no loops)**
- User Validation Gate (blocking at start)
- User Validation Update (last step after audit - items checked `[x]` by default)
- Context Loading (tiered)
- Policy Compliance
- Scope Completion Requirements (ABSOLUTE - NO EXCEPTIONS)
- Test Execution Gate
- **Bug Fix Testing Requirements (unit, component, integration, E2E)**
- **Implementation Test Coverage Validation**
- **Fix ALL Test Failures Policy (including pre-existing)**
- **Bug Awareness (check for incomplete bugs before work)**

---

## ⚠️ ABSOLUTE PROHIBITIONS — FAKE/FABRICATED WORK (NON-NEGOTIABLE)

**These rules have ZERO tolerance. Violating any of them is an immediate policy failure.**

| What is Prohibited | Why | What Happens |
|---------------------|-----|--------------|
| Claiming "tests pass" without running tests | Fabrication — the system may be broken | Spec status reverts to `in_progress`, all DoD items unchecked |
| Writing expected output instead of actual output | Evidence is meaningless if fabricated | All evidence blocks invalidated, re-execution required |
| Batch-checking multiple DoD items in one edit | Prevents individual validation | All batch-checked items reverted to `[ ]` |
| Marking DoD items `[x]` at scope creation time | Pre-checking means no validation occurred | All pre-checked items reverted to `[ ]` |
| Narrative summaries as evidence | "Tests pass" is not evidence | Evidence block must be replaced with real terminal output |
| Template placeholders unfilled | "[ACTUAL terminal output]" is not evidence | Template text must be replaced with real output |
| Skipping specialist phases | Missing phases means incomplete verification | Spec cannot be promoted until all phases execute |
| Moving to next spec before current is complete | Previous work may be broken | Next spec blocked until current passes all gates |
| Default/fallback values in production code | `unwrap_or()`, `\|\| default`, `?? fallback`, `:-default` mask missing config and hide failures | Implementation blocked until all defaults removed and fail-fast applied |
| Stub/placeholder functions in production code | Stubs masquerade as real implementations | Scope cannot be Done until all stubs replaced with real implementations |
| Hardcoded data returned from API handlers | Fake data hides broken data-store integration | Reality scan (G028) blocks completion; handler must query real data store |
| Client caches used as data source | localStorage/IndexedDB/in-memory caches diverge from server truth | Implementation blocked until real API calls replace cache reads |

### Detection & Enforcement

All agents MUST apply **Fabrication Detection Heuristics** from `agent-common.md` (Gate G021) before claiming completion. The **audit agent** serves as the final verification checkpoint — if fabrication is detected during audit, the spec is blocked.

The **artifact lint script** (`artifact-lint.sh`) includes automated detection for:
- DoD items without evidence blocks
- Unfilled template placeholders
- Evidence blocks lacking terminal output signals (fabricated content)
- Narrative summary language
- Missing specialist phases in `execution.completedPhaseClaims` / `certification.certifiedCompletedPhases`
- Duplicate evidence blocks
- Missing `**Claim Source:**` provenance tags in evidence blocks
- Evidence labeled `executed` where the DoD claim requires interpretation (provenance fabrication)

The **implementation reality scan** (`implementation-reality-scan.sh`) includes automated detection for:
- Backend stub patterns (hardcoded vecs, fake/mock/stub functions)
- Frontend fake data (getSimulationData, import mock modules, hardcoded arrays)
- Frontend API/client signal absence (hooks/services with zero API/query/client transport signals)
- Default/fallback patterns (`unwrap_or`, `unwrap_or_default`, `|| default`, `?? fallback`, `getOrElse`)
- Prohibited simulation helpers in production code
- Live-system tests that use request interception or canned backend responses
- Handler/endpoint files that expose a public surface but show no real delegation or execution depth

---

## Canonical Folder Structure

See [agent-common.md](agent-common.md) → "Feature Folder Structure" for the canonical folder layout and naming conventions.

**Naming:** Feature folders: `NNN-kebab-case-name`. Scope folders: `NN-kebab-case-scope-name`.

**Required artifacts for new scope work:** `spec.md`, `design.md`, and either `scopes.md` (small specs) or `scopes/_index.md` + per-scope directories (large specs).

**⚠️ DEPRECATED:** Do NOT create folders under `specs/_iterations/`.

---

## ⛔ Scope Isolation Model (NON-NEGOTIABLE)

**Scopes are independent units of work. An agent picks up ONE scope and works ONLY within that scope's artifacts. Scopes MUST NOT cross into each other — each scope has its own Gherkin scenarios, test plan, DoD, and evidence.**

### Two Layout Modes (Based on Scope Count)

| Scope Count | Layout | When to Use |
|-------------|--------|-------------|
| **1–5 scopes** | **Single-file** — everything in `scopes.md` + `report.md` | Small features, bug fixes, simple specs |
| **6+ scopes** | **Per-scope directories** — `scopes/_index.md` + `scopes/NN-name/scope.md` + `scopes/NN-name/report.md` | Large features, multi-phase specs |

**Agents MUST check for the per-scope directory layout first.** If `scopes/_index.md` exists, use per-scope directories. If only `scopes.md` exists, use single-file mode.

### Per-Scope Directory Structure (6+ Scopes)

```
specs/NNN-feature-name/
├── spec.md                          # Shared — read-only during implementation
├── design.md                        # Shared — read-only during implementation
├── state.json                       # Spec-level orchestration + scope DAG
├── uservalidation.md                # Shared validation checklist
├── scopes/
│   ├── _index.md                    # Summary table + dependency DAG (lightweight)
│   ├── 01-scope-name/
│   │   ├── scope.md                 # Gherkin + plan + test plan + DoD for THIS scope only
│   │   └── report.md               # Evidence for THIS scope only
│   ├── 02-scope-name/
│   │   ├── scope.md
│   │   └── report.md
│   └── ...
```

**Isolation rules:**
- An agent working on scope 09 reads ONLY `scopes/09-*/scope.md` and writes ONLY to `scopes/09-*/report.md`
- An agent NEVER modifies another scope's `scope.md` or `report.md`
- The shared `_index.md` is updated only for status sync (status column in summary table)
- `state.json` is the machine-readable coordination point

### Scope Dependency DAG (Per-Scope Directory Mode)

**Scopes declare explicit dependencies instead of relying on strict sequential ordering.**

In `_index.md`, the dependency DAG replaces a linear sequence:

```markdown
## Dependency Graph

| # | Scope | Depends On | Surfaces | Status |
|---|-------|-----------|----------|--------|
| 01 | catalog-schema    | —          | Docs, Config     | Not Started |
| 02 | catalog-migration | 01         | Docs             | Not Started |
| 03 | catalog-compiler  | 01, 02     | Libs, CLI        | Not Started |
| 04 | context-plane     | —          | Backend          | Not Started |
| 05 | intent-engine     | 04         | Backend          | Not Started |
| 10 | html-viewer       | 03         | Docs, Libs       | Not Started |
| 13 | enforcement       | 03         | CI, Docs         | Not Started |
```

**Pickup rules:**
1. A scope can move to `In Progress` only when ALL scopes listed in its `Depends On` column are `Done`
2. Scopes with no dependencies (`—`) can start immediately and in parallel
3. An agent picks the **lowest-numbered eligible** scope (all deps done, status = Not Started)
4. Multiple agents can work different scopes in parallel if dependency constraints allow

In `state.json`, each scope declares its dependencies:

```json
{
  "scopeProgress": [
    {
      "scope": 1,
      "name": "Catalog Schema",
      "status": "not_started",
      "dependsOn": [],
      "scopeDir": "scopes/01-catalog-schema"
    },
    {
      "scope": 4,
      "name": "Context Plane",
      "status": "not_started",
      "dependsOn": [],
      "scopeDir": "scopes/04-context-plane"
    },
    {
      "scope": 10,
      "name": "HTML Viewer",
      "status": "not_started",
      "dependsOn": [3],
      "scopeDir": "scopes/10-html-viewer"
    }
  ]
}
```

### Legacy Format Migration (MANDATORY Before Starting Work)

**When an agent encounters a spec with `scopes.md` (single-file) that has 6+ scopes, it MUST refactor to the per-scope directory layout before starting implementation.**

#### Migration Protocol

1. **Read** the existing `scopes.md` to identify all scope sections
2. **Create** `scopes/_index.md` with the summary table and dependency DAG
3. **For each scope:** create `scopes/NN-scope-name/scope.md` by extracting that scope's section from `scopes.md` (Gherkin, plan, test plan, DoD)
4. **For each scope:** create `scopes/NN-scope-name/report.md` with the report template (extracting any existing evidence from `report.md`)
5. **Update** `state.json` to add `dependsOn` and `scopeDir` fields to each scope entry
6. **Rename** the original `scopes.md` to `scopes.md.legacy` (preserve, don't delete)
7. **Rename** the original `report.md` to `report.md.legacy` if evidence was split (preserve, don't delete)
8. **Apply the Tiered DoD** — collapse boilerplate DoD items (see Tiered DoD below) while migrating

**Migration is a NON-BLOCKING prerequisite** — agents do this as the first step before implementation, not as a separate spec/scope.

### Single-File Mode (1–5 Scopes)

For small specs, the existing single-file model continues to work:

```
specs/NNN-feature-name/
├── spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json
```

All scopes live in `scopes.md`. All evidence lives in `report.md`. This is fine when the total DoD item count is manageable (≤50 items across all scopes).

---

## Status Transition Gate

Use these rules for every scope status change.

1. A scope can move to `In Progress` only when all scopes in its `dependsOn` list are `Done` (DAG mode) OR all prior scopes are `Done` (sequential mode).
2. A scope can move to `Done` only when:
   - Scope DoD items in its `scope.md` (or `scopes.md`) are all checked `[x]`
  - Scope DoD explicitly includes scenario-specific E2E regression coverage for changed behavior plus a broader regression suite pass
  - If the scope renames/removes any route, path, contract, identifier, or UI target, the DoD includes a consumer impact sweep item and the affected consumer flows are validated
  - If the scope changes shared fixtures, harnesses, or bootstrap/auth/session/storage infrastructure, the DoD includes a Shared Infrastructure Impact Sweep, an independent canary suite item, and a rollback/restore item
  - If the scope is a narrow repair or risky refactor, the DoD includes a Change Boundary item and evidence that zero excluded file families changed
   - Matching raw evidence is present in the scope's `report.md` (must contain legitimate terminal output signals per command-backed block)
  - Scope entry in `state.json` is updated in `certification.scopeProgress` and `certification.completedScopes`
  - `certification.completedScopes` matches the actual set of Done scopes exactly — no stale omissions, no extra carried-forward entries
  - `policySnapshot` is present with effective values and provenance for grill, TDD, auto-commit, lockdown, regression, and validation
  - `scenario-manifest.json` exists, contains stable scenario contracts for the scope's Gherkin scenarios, and links them to live-system tests plus evidence refs
  - `transitionRequests` and `reworkQueue` are closed before certification
   - **Zero deferral language exists in scope artifacts** (Gate G040 — "deferred", "future scope", "follow-up", "out of scope", "will address later", "punt", "postpone", "skip for now", "not implemented yet", "placeholder", "temporary workaround" are ALL blocking)
3. If evidence is missing, contradictory, a required test type is absent, or deferral language is present, scope status must remain `In Progress`.
4. Spec status cannot move to `done` until ALL scopes are `Done`, `certification.completedScopes` contains all scope IDs, and `bubbles.validate` certifies the transition.

## Live-Stack Test Classification (ABSOLUTE)

Use these rules when writing or reviewing the Test Plan.

1. Rows labeled `integration`, `e2e-api`, `e2e-ui`, or described as live-stack MUST exercise the real running system.
2. Tests in those categories MUST NOT intercept internal requests or inject canned responses.
3. If a test uses `route`, `intercept`, `msw`, `nock`, `wiremock`, `responses`, or equivalent request interception, it is MOCKED and MUST be classified as `unit`, `functional`, or `ui-unit` instead.
4. If reclassification removes the last real test for a required live category, creating the real test becomes blocking scope work.
5. Scope artifacts and report text MUST describe mocked tests honestly. Never call them live-stack after reclassification.

Recommended verification command:

```bash
grep -rn 'page\.route\|context\.route\|route(\|intercept(\|cy\.intercept\|msw\|nock\|wiremock\|responses' [live-system-test-files]
```

**Status sync requirements:**

| Layout Mode | Checklist Source | Evidence Source | Machine Status |
|-------------|-----------------|----------------|----------------|
| Single-file | `scopes.md` | `report.md` | `state.json` |
| Per-scope dirs | `scopes/NN-name/scope.md` | `scopes/NN-name/report.md` | `state.json` |
| Both modes | `scopes/_index.md` status column (if exists) | — | `state.json` top-level status mirrors `certification.status`; certification is authoritative |

---

## Artifact Templates

Use [scope-templates.md](scope-templates.md) as the single source of truth for artifact templates, example shapes, and `state.json` structure. Use [feature-templates.md](feature-templates.md) for feature artifact scaffolding.

**Status field semantics:**
- `done` — Implementation complete, all tests pass, all gates satisfied. Only set by modes with `statusCeiling: done`.
- `specs_hardened` — Spec/design/scopes improved. No implementation work done. Set by `spec-scope-hardening` mode.
- `docs_updated` — Documentation updated. No implementation work done. Set by `docs-only` mode.
- `validated` — Validation/audit completed. No implementation work done. Set by `validate-only`/`audit-only` modes.
- `in_progress` — Work started but not finished. Used during active execution or by `resume-only`.
- `blocked` — Cannot proceed due to unresolved failures or missing inputs.

---

## Artifact Cross-Linking (MANDATORY)

All generated documents MUST include links to related artifacts:

**Single-file mode:**

| Document | Must Link To |
|----------|--------------|
| `scopes.md` | `spec.md`, `design.md`, `report.md`, `uservalidation.md` |
| `report.md` | `scopes.md` (specific scope), `uservalidation.md` |
| `uservalidation.md` | `report.md` (evidence sections) |

**Per-scope directory mode:**

| Document | Must Link To |
|----------|--------------|
| `scopes/_index.md` | `spec.md`, `design.md`, `uservalidation.md` |
| `scopes/NN/scope.md` | `spec.md`, `design.md`, `_index.md` |
| `scopes/NN/report.md` | `scope.md` (same directory), `spec.md`, `_index.md` |
| `uservalidation.md` | scope evidence (per-scope `report.md` files) |

---

## Execution Phases

Both iterate and implement follow these phases:

### Phase 0: Context & Validation
1. Resolve `{FEATURE_DIR}` from arguments
2. Load/validate `spec.md`, `design.md`, `scopes.md`
3. **Run User Validation Gate** (per [agent-common.md](agent-common.md))
4. **Run Baseline Health Check** (per [agent-common.md](agent-common.md)) — record pre-change test counts
5. Update `state.json`

### Phase 1: Scope Selection/Definition
- **implement:** Select next eligible scope from `scopes/_index.md` (per-scope dir mode) or `scopes.md` (single-file mode) using the Pickup Rule (lowest-numbered scope with all deps done)
- **iterate:** Identify work, create scope if needed, add to `_index.md` or `scopes.md`
- **Legacy migration:** If `scopes.md` exists with 6+ scopes and no `scopes/_index.md`, refactor to per-scope directories first (see Legacy Format Migration)

### Phase 2: Implementation
1. Implement changes across all surfaces
2. Add required tests (unit, component for UI, integration, E2E, stress if SLAs exist)
3. As each DoD item is completed:
   - **Validate** the item (run tests, verify behavior)
   - **Record evidence** inline under the corresponding DoD checkbox item in `scope.md` (per-scope dir) or `scopes.md` (single-file) — verbatim terminal output only, must contain recognizable terminal signals (test results, file paths, exit codes, timing, etc.)
   - **Mark `[x]`** ONLY after evidence is recorded
   - Do NOT pre-check DoD items when creating scope definitions
   - Do NOT batch-check multiple DoD items
   - **Each item MUST be validated individually** with its own execution and evidence
4. Update `state.json` after each step

**CRITICAL — THE COMPLETION CHAIN (see agent-common.md top section):**
- A DoD item CANNOT be [x] without inline raw evidence containing legitimate terminal output signals
- A scope CANNOT be "Done" until ALL DoD items are [x] with evidence
- A spec CANNOT be "done" until ALL scopes are "Done"
- Tests MUST cover ALL real scenarios (Gherkin, error paths, boundaries, parameter permutations)
- Tests MUST be real (no internal mocks, real test DBs, 100% business logic coverage)
- Tests MUST be passing (exit code 0, all tests pass)

### Phase 3: Tests

**Note:** Use project-specific test commands from `copilot-instructions.md`. Do NOT hardcode tool names.

1. Run ALL required test types (using project-specific commands)
2. Record evidence for EACH test type in `report.md`
3. Verify test coverage meets thresholds (per project config)
4. **Fix ALL failures** (including pre-existing) and re-run until all pass
5. Follow **Test Execution Gate**, **Bug Fix Testing Requirements**, and **Fix ALL Test Failures Policy** (per [agent-common.md](agent-common.md))
6. **Run Skip Marker Scan** — verify zero skip/only/todo markers in changed test files (per Skip Marker Scan Gate in agent-common.md)

### Phase 4: Validation
1. Run validation suite via `/bubbles.validate`
2. Fix issues and re-validate

### Phase 5: Documentation
1. Update feature docs (`spec.md`, `design.md`)
2. Update managed docs declared in the effective managed-doc registry (API, ARCHITECTURE, etc.)
3. Update execution evidence artifact for the active layout:
  - **Per-scope directory mode:** update only `scopes/NN-name/report.md`
  - **Single-file mode:** append/update the scope section in `report.md`

### Phase 6: Audit
1. Run `/bubbles.audit`
2. If issues: route back per routing table
3. Repeat until clean

### Phase 7: Finalize (LAST STEP)
1. **Update `uservalidation.md`** with all verifiable behaviors
2. **Mark items `[x]` by default** (just validated via audit)
3. **Resolve effective status from workflow mode `statusCeiling`** (see Status Ceiling Enforcement below)
4. Mark scope status in the active scope definition file (`scope.md` or `scopes.md`) according to resolved status, and sync `_index.md` if present
5. Route the resolved status through `bubbles.validate`, which writes the authoritative `certification.status` and mirrors the top-level compatibility status when promotion is allowed
6. Record `workflowMode` in `state.json` so resume and downstream agents know what was executed

**⛔ COMPLETION CHAIN ENFORCEMENT (ABSOLUTE — from agent-common.md):**

Before marking ANY scope "Done" or setting spec status to "done", the agent MUST verify the ENTIRE completion chain:

- **Per-DoD-Item Evidence (G025):** EVERY DoD item that is [x] MUST have raw terminal output evidence inline under it containing legitimate terminal signals (test results, file paths, exit codes, timing). Items without evidence or with fabricated content are INVALID.
- **All DoD Items Checked:** for per-scope dirs use `grep -c '^\- \[ \]' {FEATURE_DIR}/scopes/*/scope.md`; for single-file use `grep -c '^\- \[ \]' {FEATURE_DIR}/scopes.md`. Result MUST be 0.
- **All Scopes Done (G024):** for per-scope dirs use `grep -c 'Status:.*Not Started\|Status:.*In Progress\|Status:.*Blocked' {FEATURE_DIR}/scopes/*/scope.md`; for single-file use `grep -c 'Status:.*Not Started\|Status:.*In Progress\|Status:.*Blocked' {FEATURE_DIR}/scopes.md`. Result MUST be 0.
- **Test Reality:** ALL test-related DoD items show tests covering ALL real scenarios with 100% business logic coverage
- **Tests Passing:** ALL test outputs show exit code 0 with zero failures

**If ANY of these fail → scope stays "In Progress" and spec stays "in_progress". No exceptions.**

**⚠️ MANDATORY FINALIZATION CHECKS (NON-NEGOTIABLE — Execute ALL before finalizing):**

7. **Run state transition guard script (Gate G023 — MECHANICAL ENFORCEMENT):**
   ```bash
  bash bubbles/scripts/state-transition-guard.sh {FEATURE_DIR}
   ```
   - **This is the FIRST check to run.** If it exits with code 1, ALL subsequent checks are moot — status MUST remain `in_progress`.
   - The guard script consolidates checks 8-13 below into a single blocking pass, but agents MUST also verify these individually for transparency.
   - **NEVER write `"status": "done"` to state.json without guard script exit code 0.**

8. **Run artifact lint** — `bash bubbles/scripts/artifact-lint.sh {FEATURE_DIR}` must exit 0
9. **Verify ALL DoD items are `[x]`** — for per-scope dirs: `grep -c '^\- \[ \]' {FEATURE_DIR}/scopes/*/scope.md` must be 0; for single-file: `grep -c '^\- \[ \]' {FEATURE_DIR}/scopes.md` must be 0
10. **Verify ALL scope statuses are Done** — check `_index.md` status column (per-scope dirs) or `scopes.md` (single-file): `grep -c 'Status:.*Not Started\|Status:.*In Progress' {SCOPE_FILES}` must be 0
11. **Verify evidence legitimacy** — every `[x]` item must have inline evidence containing real terminal output signals (test results, file paths, exit codes, timing, build tool names)
12. **Verify no fabrication** — apply Fabrication Detection Heuristics (Gate G021 from agent-common.md):
    - No template placeholders unfilled
    - No narrative summaries as evidence
    - No duplicate evidence blocks
    - No batch-checked items
13. **Verify specialist completion** — `state.json.execution.completedPhaseClaims` and `state.json.certification.certifiedCompletedPhases` include ALL mode-required phases (Gate G022)
14. **Verify no TODOs/stubs** — `grep -r "TODO\|FIXME\|HACK\|STUB\|unimplemented!" [changed-files]` must return 0 results
15. **Verify completion chain (G024)** — ALL scopes are "Done" before spec can be "done"
16. **Verify per-DoD-item evidence (G025)** — EVERY [x] item has raw terminal output evidence inline with legitimate terminal signals. Manually verify each checked item has an evidence block containing real output (pass/fail counts, file paths, exit codes). Items without evidence or with fabricated prose MUST be reverted to [ ]
17. **Verify test reality (G025)** — ALL test-related DoD items show tests covering ALL Gherkin scenarios, error paths, boundary conditions, and parameter permutations. Tests MUST use real systems (no internal mocks, real test DBs). 100% business logic coverage required.
18. **Verify stress coverage** — If scope defines latency SLAs (e.g., "under 50ms"), stress test DoD items MUST exist and pass.
19. **Verify no defaults/fallbacks (G028)** — `bash bubbles/scripts/implementation-reality-scan.sh {FEATURE_DIR} --verbose` covers Scan 5. Zero `unwrap_or()`, `|| default`, `?? fallback`, `os.getenv("K", "default")` in production code. All config MUST fail-fast if missing.
20. **If ANY check fails** → status MUST remain `in_progress`, NOT be promoted to `done`

**⚠️ STATE TRANSITION SEQUENCE (NON-NEGOTIABLE):**
```
Step 1: Run guard script → bash bubbles/scripts/state-transition-guard.sh {FEATURE_DIR}
Step 2: IF exit code 1 → STOP. Status stays "in_progress". Fix ALL failures.
Step 3: IF exit code 0 → Run artifact lint as confirmation
Step 4: IF lint passes → Write the resolved status (never exceeding `statusCeiling`) to state.json
Step 5: IF lint fails → STOP. Status stays "in_progress". Fix failures.
```

**This sequence is ABSOLUTE. There is NO alternative path to "done" without the guard script passing.**

#### Status Ceiling Enforcement (NON-NEGOTIABLE)

**The `state.json` `status` field MUST NOT exceed the `statusCeiling` defined for the active workflow mode in `bubbles/workflows.yaml`.**

| Workflow Mode | `statusCeiling` | Meaning |
|---------------|-----------------|---------|
| `full-delivery` | `done` | Implementation + tests completed and verified |
| `value-first-e2e-batch` | `done` | Full delivery with value-first selection |
| `full-delivery` | `done` | Bootstrap + implementation completed |
| `bugfix-fastlane` | `done` | Bug fixed with reproduction evidence |
| `chaos-hardening` | `done` | Chaos rounds clean + implementation verified |
| `spec-scope-hardening` | `specs_hardened` | Planning artifacts improved — NO implementation done |
| `docs-only` | `docs_updated` | Documentation updated — NO implementation done |
| `validate-only` | `validated` | Validation completed — NO implementation done |
| `audit-only` | `validated` | Audit completed — NO implementation done |
| `resume-only` | `in_progress` | Partial work resumed |

**Rules:**
- If the mode's `statusCeiling` is NOT `done`, the finalize phase MUST NOT set `status: "done"` in `state.json` or mark scopes as `Done` in `scopes.md`
- Artifact-only modes (`spec-scope-hardening`, `docs-only`, `validate-only`, `audit-only`) produce planning/quality improvements but do NOT constitute completed implementation work
- `execution.completedPhaseClaims` records which phases ran in this session, while `certification.certifiedCompletedPhases` records which of those phases were validate-certified. Both must remain coherent with `certification.completedScopes` (Gate G027: claiming implementation phases with empty certified scope completion is fabrication).
- **Phase Recording Responsibility (MANDATORY):** Each specialist agent may append only its OWN phase name to `execution.completedPhaseClaims` in `state.json` AFTER its Tier 1 + Tier 2 validation checks pass. Agents MUST NOT add other agents' phase names and MUST NOT write `certification.certifiedCompletedPhases`; certification is validate-owned. Pre-populating phases that have not actually executed is fabrication (Gate G027). The recording happens as the agent's LAST step — never before validation succeeds.
- **Execution History Ownership (MANDATORY):** `executionHistory` is the audit trail for agent/workflow runs. Standalone specialist runs MUST append their own `executionHistory` entry. When a specialist is invoked by the workflow/orchestrator via `runSubagent`, the specialist MUST skip appending `executionHistory`; the workflow/orchestrator records the authoritative entry for that run to avoid duplicate history rows. This exception does NOT change the specialist's responsibility to append its own execution claim.
- **Phase name mapping:** `bubbles.implement` → `"implement"`, `bubbles.test` → `"test"`, `bubbles.docs` → `"docs"`, `bubbles.validate` → `"validate"`, `bubbles.audit` → `"audit"`, `bubbles.chaos` → `"chaos"`
- A subsequent `full-delivery` or `full-delivery` run is required to advance status to `done`

**Example state.json after `spec-scope-hardening`:**
```json
{
  "version": 3,
  "status": "specs_hardened",
  "workflowMode": "spec-scope-hardening",
  "execution": {
    "completedPhaseClaims": ["select", "bootstrap", "harden", "docs", "validate", "audit", "finalize"]
  },
  "certification": {
    "status": "specs_hardened",
    "completedScopes": [],
    "certifiedCompletedPhases": ["validate", "audit"]
  },
  "notes": "Specs/scopes hardened. Implementation not started — run full-delivery to advance to done."
}
```

**Example state.json after `full-delivery`:**
```json
{
  "version": 3,
  "status": "done",
  "workflowMode": "full-delivery",
  "execution": {
    "completedPhaseClaims": ["select", "implement", "test", "docs", "validate", "audit", "chaos", "finalize"]
  },
  "certification": {
    "status": "done",
    "completedScopes": ["01-core-scope", "02-follow-up-scope"],
    "certifiedCompletedPhases": ["validate", "audit", "chaos", "finalize"]
  }
}
```

---

## Phase Exit Gates (MANDATORY)

**A phase is ONLY complete when its exit conditions are ALL satisfied. Proceeding without meeting exit gates is a policy violation.**

| Phase | Exit Condition | Verification |
|-------|---------------|--------------|
| **0: Context** | All required artifacts loaded; baseline health recorded in report.md | `state.json` updated, baseline section exists in report.md |
| **1: Scope** | Scope selected/created with Gherkin scenarios, test plan, and DoD; legacy migration done if needed | `scope.md` (per-scope dir) or `scopes.md` has non-empty scope with all sections |
| **2: Implementation** | Code changes complete; all DoD items have 3-part validation (impl + behavior + evidence) | Every `[x]` has inline raw evidence with legitimate terminal output signals |
| **3: Tests** | All test types pass (exit code 0); skip marker scan clean; coverage meets threshold | Raw terminal output in report.md for each test type |
| **4: Validation** | Validation suite passes; no regressions vs baseline | Validation output in report.md |
| **5: Documentation** | All impacted docs updated; no stale references | Doc file list in report.md |
| **6: Audit** | Audit verdict is SHIP_IT or SHIP_WITH_NOTES; fabrication detection passes | Audit report in report.md; artifact lint exits 0 |
| **7: Finalize** | uservalidation.md updated; scope status set to mode's `statusCeiling`; state.json status ≤ `statusCeiling`; `workflowMode` recorded; ALL finalization checks pass (steps 7-19 above); **state transition guard script exits 0 (Gate G023)**; **ALL scopes Done before spec done (Gate G024)**; **ALL DoD items have per-item raw evidence (Gate G025)**; **ALL tests cover all real scenarios with 100% coverage (Gate G025)**; **stress tests exist when SLAs are defined** | File timestamps verify updates; status matches ceiling; guard script exits 0; artifact lint exits 0; all DoD `[x]` with inline evidence; all scopes Done; specialist completion verified; test coverage verified; stress coverage verified |

**Rollback Protocol:** If a phase fails its exit gate and the agent cannot resolve the issue within 3 attempts:
1. Revert any partial changes from the current phase
2. Return to the previous phase's entry point
3. Record the failure and reason in report.md under `## Phase Rollback`
4. Do NOT skip the phase — resolve the blocker or stop and report

---

## Audit Failure Routing

| Issue Type | Return To Phase |
|------------|-----------------|
| Spec/design/scopes mismatch | Phase 1 |
| Bug / incorrect behavior | Phase 2 (implement) |
| Missing/weak tests | Phase 3 (tests) |
| Validation failures | Phase 4 (validate) |
| Security issues | Phase 2 (implement) |
| Documentation drift | Phase 5 (docs) |

After fixes: re-run Phase 3 → 4 → 5 → 6 → 7.

---

## Resume Behavior

On startup, before selecting a new scope:

1. Check `{FEATURE_DIR}/state.json`
2. If `status` is `in_progress` or `blocked`: resume from `currentPhase`
3. If `status` is `specs_hardened`, `docs_updated`, or `validated`: these are terminal states for artifact-only modes — do NOT resume; instead, select a new scope or switch to an implementation mode (`full-delivery`)
4. If `status` is `done`: all scopes complete — report and stop
5. Do NOT re-select scope unless user explicitly requests fresh start

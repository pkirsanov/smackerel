<!-- governance-version: 2.2.0 -->
# Bubbles Critical Requirements (Project-Agnostic)

> **Purpose:** Define hard, universal, non-negotiable implementation and testing requirements for all `bubbles.*` agents.
>
> **Scope:** Portable across projects. Contains no project-specific commands, paths, ports, or stack assumptions.
>
> **Priority:** **TOP PRIORITY**. If any instruction conflicts with this file, this file wins unless a stricter safety or legal policy applies.

---

## Absolute Policy Set (Non-Negotiable)

1. **Use-Case Truthfulness**
   - All tests MUST validate defined use cases and required behavior exactly as specified.
   - Tests that do not validate required outcomes are invalid for completion.

2. **Planned Behavior Is The Source Of Truth**
   - When a test fails, agents MUST compare the behavior against `spec.md`, `design.md`, `scopes.md`, and DoD before changing the test.
   - Agents MUST NOT weaken, delete, or rewrite tests to match the currently broken implementation.
   - If the planned behavior is genuinely wrong or incomplete, the owning planning artifact MUST be corrected first; only then may tests and implementation be updated together.

3. **Persistent Regression E2E Coverage**
   - Every feature, fix, or behavior change MUST add or update at least one scenario-specific E2E regression test tied to the planned behavior it protects.
   - Regression E2E coverage MUST live with the feature/component it verifies, not in a generic catch-all bucket.

4. **Consumer Trace For Renames And Removals**
   - Renaming, removing, moving, or deprecating any route, path, contract, identifier, symbol, link target, or UI target MUST include a complete consumer inventory before completion.
   - First-party consumers such as navigation links, breadcrumbs, redirects, API clients, generated clients, docs, config, and tests MUST be updated together.
   - Stale references to removed or renamed targets are blocking failures unless an explicit compatibility shim is documented.

5. **Real Code Execution in Tests**
   - Tests MUST execute actual production code paths.
   - Mocks are allowed only for true external dependencies (third-party APIs, external services, non-owned infrastructure boundaries).
   - Internal business logic, internal modules, and owned service boundaries MUST NOT be mocked when validating integration or end-to-end behavior.

6. **Zero Fabrication / Zero Hallucination**
   - Never fabricate test status, output, evidence, files, commands, or completion claims.
   - Never claim pass/fail or completion without current-session execution evidence.

7. **No TODO Debt / No Deferral Language**
   - Never leave `TODO`, `FIXME`, placeholders, or deferred implementation markers.
   - If full implementation cannot be completed, do not mark the work complete.
   - **NEVER write deferral language** into DoD items, scope files, or report files:
     - FORBIDDEN: "deferred", "future scope", "follow-up", "out of scope", "will address later", "separate ticket", "punt", "postpone", "skip for now", "not implemented yet", "placeholder", "temporary workaround"
   - Deferral language in artifacts = spec CANNOT be marked "done" (mechanically enforced by state-transition-guard Gate G040).
   - If a DoD item cannot be completed: fix the issue NOW, remove the item with justification, or leave status as "In Progress".

8. **No Stubs**
   - Stub implementations are forbidden in production and completion-bound test code.
   - Replace all stubs with full, working implementations before completion.

9. **No Fake/Sample Data in Verification**
   - Do not use fake/sample/demo/placeholder data as proof of real behavior in required validation.
   - Required tests must use realistic, representative data sources appropriate to the test category.

10. **No Defaults**
   - Do not hide missing configuration or required inputs using defaults.
   - Missing required values must fail explicitly.

11. **No Fallbacks — Fail Fast**
   - Do not mask failures with fallback branches that simulate success.
   - Surface errors immediately with explicit failure paths.

12. **Full-Fidelity Implementation Required**
   - Do not simplify away required behavior, edge cases, error handling, or domain constraints.
   - Implement complete feature behavior with production-quality robustness.

13. **No Shortcuts**
    - No partial implementation presented as complete.
    - No reduced-scope tests presented as full validation.
    - No incomplete docs for completed work; documentation must match shipped behavior.

14. **No Selective Remediation Of Discovered Findings**
   - When an agent, workflow, or validation round discovers multiple findings, every finding MUST be accounted for individually before the work can be treated as complete.
   - Valid outcomes are only: (a) fix all findings and revalidate, or (b) return `route_required` / `blocked` with the full unresolved finding list preserved verbatim.
   - Fixing the easy subset while narrating the rest as larger, later, separate, or follow-up work is incomplete work and MUST be rejected.

15. **Planning-First Delivery**
   - Implementation, bug fixing, hardening, stabilization, gap-closure, and cross-cutting operational work MUST be anchored to real feature, bug, or ops artifacts before completion work proceeds.
   - If `spec.md`, `design.md`, `scopes.md`, or required sibling artifacts are missing, empty, or placeholder-only, agents MUST route to the owning planning agents first instead of improvising implementation.
   - Empty feature directories, partial artifact sets, or artifact files containing only skeletal headers are workflow failures, not permission to continue unplanned work.

16. **No Cosmetic Relabeling Of Incomplete Work**
   - Renaming a `TODO`, stub, placeholder, fake value, or incomplete branch to softer language does NOT count as progress.
   - Rewording incomplete work as `placeholder`, `future improvement`, `deferred`, `follow-up`, `temporary`, `compat shim`, or similar is forbidden unless the work is also moved into real tracked planning artifacts owned by the correct agent.
   - If incomplete behavior is discovered without owning artifacts, agents MUST create or update the corresponding feature, bug, or ops planning instead of disguising the incompleteness.

17. **Fixture Ownership And Shared-State Isolation**
   - Live-system work that creates or mutates state MUST use agent-owned fixtures with unique, traceable ownership.
   - Agents MUST NOT mutate shared baseline data by selecting the first existing resource from a list response.
   - Host-level defaults, inherited configs, global settings, and similar cross-scenario state are protected surfaces; mutate them only with an explicit baseline snapshot and a verified restore path.
   - If cleanup or restore fails, the work remains incomplete.

18. **No Sensitive Client Storage For Auth, Session, Or Payment Secrets**
   - Agents MUST NOT treat browser or client-side storage as an acceptable place for auth tokens, session secrets, refresh tokens, bearer credentials, payment method details, CVV/CVC, or similarly sensitive trust material.
   - `localStorage`, `sessionStorage`, IndexedDB, AsyncStorage, SharedPreferences, and similar client storage are blocking risks when used for sensitive auth/payment state.
   - If a security or trust finding is only fixed on the backend while the frontend still reads or writes the risky storage path, the finding remains open.

19. **Documentation Claims Must Match Runtime Reality**
   - README tables, capability ledgers, feature matrices, and user-facing status claims must be verified against real code paths before they can assert a capability is delivered.
   - Docs-only evidence cannot close runtime work. Delivered-status claims require implementation evidence plus executed proof.

20. **Framework File Immutability — Upstream-First Rule**
   - Agents MUST NEVER create, modify, or delete files in Bubbles framework-managed directories of downstream projects: `.github/bubbles/scripts/`, `.github/agents/bubbles_shared/`, `.github/agents/bubbles.*.agent.md`, `.github/prompts/bubbles.*.prompt.md`, `.github/bubbles/workflows.yaml`, `.github/bubbles/hooks.json`, `.github/instructions/bubbles-*.instructions.md`, or `.github/skills/bubbles-*/`.
   - These files are owned by the Bubbles framework and updated exclusively via `install.sh` upgrades.
   - **Upstream-First Flow (ABSOLUTE):** ALL Bubbles framework changes — governance docs, agent definitions, shared modules, scripts, workflows, instructions, skills, prompts — MUST be made in the **canonical Bubbles repository only**. Downstream projects (any repo that has Bubbles installed under `.github/`) MUST NEVER receive direct edits to framework-managed files. After changes are committed in the canonical Bubbles repo, downstream projects are updated using the standard Bubbles upgrade command (`bash .github/bubbles/scripts/cli.sh upgrade` or `install.sh`). Agents MUST NOT manually copy, sync, or replicate framework files between repos.
   - **Multi-Root Workspace Rule:** In workspaces containing both the canonical Bubbles repo and downstream project repos, agents MUST direct all framework file edits to the canonical Bubbles repo path (e.g., `bubbles/agents/bubbles_shared/`, `bubbles/bubbles/scripts/`). Editing the downstream copies at `.github/agents/bubbles_shared/` or `.github/bubbles/scripts/` in project repos is FORBIDDEN — those copies are install artifacts, not source-of-truth files.
   - If a framework script has a bug or needs enhancement, the change MUST be made upstream in the canonical Bubbles repository — not patched locally in downstream repos.
   - Downstream repos may record change requests only in project-owned proposal artifacts such as `.github/bubbles-project/proposals/` or via `bubbles framework-proposal <slug>`.
   - Project-specific scripts belong in `scripts/`. Project-specific quality gates belong in `.github/bubbles-project.yaml`.
   - The `.github/bubbles/.manifest` file lists all framework-owned files, and `.github/bubbles/.checksums` records the installed upstream checksum snapshot. `agnosticity-lint.sh` detects non-manifested files in managed directories, while `bubbles framework-write-guard` detects direct downstream edits to managed files.

21. **High-Fan-Out Shared Infrastructure Refactors Require Blast-Radius Planning**
   - Shared fixtures, harnesses, global setup/bootstrap, auth/login/session bootstrap code, storage injection paths, and other high-fan-out infrastructure MUST be treated as protected change surfaces.
   - Agents MUST NOT rewrite such files wholesale by default. Prefer surgical edits, wrappers, or narrowly-scoped substitutions unless planning artifacts explicitly justify broader replacement.
   - Before changing a protected shared-infrastructure surface, planning MUST record the downstream contract surfaces that depend on it (ordering, timing, session/bootstrap state, tenant/user context, role detection, storage injection, or equivalent) and define an independent canary suite that validates those contracts before broad suite reruns.
   - A rollback or restore path for the shared-infrastructure change MUST be documented and verified before completion.

21. **Collateral Change Containment For Narrow Repairs And Refactors**
   - Narrow fixes and risky refactors MUST declare a change boundary listing the allowed file families and the excluded surfaces that must remain untouched.
   - Opportunistic cleanup, unrelated test rewrites, broad handler changes, or cross-directory sweeps MUST NOT be bundled into a shared-infrastructure repair loop unless the planning artifacts explicitly expand scope first.
   - If unrelated files change during a narrow repair, the work remains incomplete until the collateral edits are either removed or promoted into explicitly planned follow-up work owned by the correct scope.

---

## Enforcement Rules

- A scope/feature/bug/ops packet cannot be marked complete if any policy above is violated.
- Evidence must be execution-backed, current-session, and specific to claimed outcomes.
- Optional or proxy assertions cannot substitute for required behavior checks.
- If uncertainty exists, agent must report uncertainty and keep status in progress instead of inventing conclusions.

## Detection Scans (MANDATORY before marking scope "Done")

These scans enforce policies 4-8 mechanically. Agents MUST run the implementation reality scan which covers all of these:

```bash
# Run the comprehensive reality scan (covers stubs, fakes, hardcoded data, defaults, fallbacks)
bash bubbles/scripts/implementation-reality-scan.sh {FEATURE_DIR} --verbose
# Exit code 0 = pass, Exit code 1 = BLOCKED
```

### What the scan detects:

| Policy | Scan | Patterns Detected |
|--------|------|-------------------|
| No Stubs (5) | Scan 1 | `fn fake_/mock_/stub_/placeholder_`, `generate_fake/mock/stub`, static RESPONSES/ITEMS arrays |
| No Fake Data (6) | Scan 1+2 | `MOCK_DATA`, `SAMPLE_DATA`, `getSimulationData()`, `useMockData()`, import mock modules |
| No Defaults (7) | Scan 5 | `unwrap_or()`, `unwrap_or_default()`, `\\|\\| "default"`, `?? "fallback"`, `os.getenv("K", "default")` |
| No Fallbacks (8) | Scan 5 | Same as defaults — any pattern that masks missing config with a silent value |
| Real Implementation (9) | Scan 3 | Data hooks/services with ZERO API/query/client transport signals — returning hardcoded data |
| No Sensitive Client Storage (17) | Scan 2B | `localStorage/sessionStorage/IndexedDB/AsyncStorage/SharedPreferences` storing auth/session/payment secrets |
| No Fake Integrations (18) | Scan 1C + 1D | 501/not-implemented handlers, random/no-op provider adapters with no real upstream call signals |

**If the reality scan exits with code 1, the scope CANNOT be "Done". Fix ALL violations first.**

---

## Completion Gate (Mandatory)

Before reporting completion, all answers must be **YES**:

1. Did tests validate the defined use cases and required outcomes?
2. Did any test edits remain faithful to `spec.md`, `design.md`, `scopes.md`, and DoD instead of drifting toward current broken behavior?
3. Does every feature/fix/change have scenario-specific persistent E2E regression coverage?
4. If any route/path/contract/identifier/UI target was renamed or removed, were all consumers traced and stale references eliminated?
5. Did required tests execute real code paths (with mocks only for true external dependencies)?
6. Are all claims backed by actual current-session execution evidence?
7. Are there zero TODOs, stubs, fake/sample verification artifacts, defaults, and fallbacks masking failures?
8. Is the implementation full-featured, edge-case complete, high-quality, and documented without shortcuts?
9. Did all live-state mutations stay isolated to owned fixtures or get fully restored before completion?
10. Was all implementation/hardening work backed by real feature, bug, or ops artifacts rather than empty or missing planning files?
11. Were any TODOs, stubs, or placeholders resolved by real implementation or tracked planning instead of cosmetic relabeling?
12. If shared fixtures, harnesses, or bootstrap contracts changed, was blast radius planned with a canary suite and rollback path before the broad suite reran?
13. If the work was a narrow repair or risky refactor, did it stay inside an explicit change boundary with zero excluded file families changed?

If any answer is **NO**, completion is prohibited.

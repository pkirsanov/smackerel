# Design: BUG-024-002 Reconcile artifact drift to current gate standards + close real §22.7 connector-inventory drift (15 → 16, QF Decisions)

## Current Truth (Codebase at HEAD `d203d0b9`)

Before designing the fix, captured the relevant codebase facts directly from the working tree:

| Source | Fact | Evidence |
|--------|------|----------|
| Connector registry | 16 connectors instantiated and registered | `cmd/core/connectors.go` lines 28-58: `imapConn, caldavConn, ytConn, rssConn, keepConn, bmConn, browserHistConn, mapsConn, hospitableConn, guesthostConn, discordConn, twitterConn, weatherConn, alertsConn, marketsConn, qfDecisionsConn` |
| QF Decisions connector | Implements `connector.Connector` interface; companion-mode boundary owned by spec 041 | `internal/connector/qfdecisions/connector.go`; `internal/connector/qfdecisions/packet.go`; `internal/connector/qfdecisions/README.md` |
| Connector directory count | 16 directories | `find internal/connector -maxdepth 1 -mindepth 1 -type d \| wc -l` → 16 (`alerts/`, `bookmarks/`, `browser/`, `caldav/`, `discord/`, `guesthost/`, `hospitable/`, `imap/`, `keep/`, `maps/`, `markets/`, `photos/`, `qfdecisions/`, `rss/`, `twitter/`, `weather/`, `youtube/`) — note: `photos/` is the internal photo library feature, not an ingestion connector; the registered count matches 16 ingestion connectors |
| Design doc §22.7 | Claims `(15 connectors)` with rows 1-15 | `docs/smackerel.md` lines 2370-2389 |
| Design doc §22.7 intro | Claims "All 15 connectors are implemented" | `docs/smackerel.md` line 2372 |
| Design doc §24-A tree | Claims `Connector plugins (15 committed)` with 15 leaves | `docs/smackerel.md` lines 2477-2492 |
| Design doc cross-search | Zero hits for `qfdecisions` or `QF Decisions` | `grep -cE "qfdecisions\|QF Decisions" docs/smackerel.md` → 0 |
| Spec 041 status | `done_with_concerns` | `specs/041-qf-companion-connector/state.json::status` |
| Spec 041 commit lineage | Three commits introduced/wired `qfdecisions/` between 2026-05-22 and HEAD | `git log --oneline -- internal/connector/qfdecisions/` → `43ce5096`, `c22151a5`, `39ca4fcb` |
| Spec 024 own state | `status: done`, `workflowMode: full-delivery`, 10 completedPhaseClaims, scopeProgress for 2 scopes both done, certified 2026-04-10 | `specs/024-design-doc-reconciliation/state.json` |
| Spec 024 freshness boundary triggers | `spec.md` line 123 heading + `design.md` lines 512/515/518 bash-fenced `#` comments contain "Superseded" substring | `grep -nE "^#{1,6}.*[Ss]uperseded" specs/024-design-doc-reconciliation/{spec,design}.md` |
| Spec 024 existing structured commits | Zero commits with subject prefix `^spec\(024\)` or `^bubbles\(024/` | Check 17 BLOCK confirms; prior BUG-024-001 used the legacy `feat(...)` prefix |

The drift is **localized**: the only product-truth surface that needs runtime-aware editing is `docs/smackerel.md` (§22.7 header/intro/row + §24-A tree). All other reconciliation is artifact-only inside `specs/024-design-doc-reconciliation/`.

## Architecture

The fix is a four-layer reconcile-to-doc operation:

```
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 1: docs/smackerel.md — Real Implementation Drift Reconciliation │
│   §22.7 header     (15 connectors) → (16 connectors)                  │
│   §22.7 intro line "All 15 …"      → "All 16 …"                       │
│   §22.7 table      add row 16 for QF Decisions (companion boundary)   │
│   §24-A tree       (15 committed) → (16 committed) + leaf 16          │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 2: BUG-024-002 packet — Bug-Local 8-Artifact Set                │
│   bug.md / spec.md / design.md / scopes.md / scenario-manifest.json    │
│   report.md / state.json (resolved) / uservalidation.md                │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 3: Parent spec 024 governance backfill                          │
│   spec.md       BS-005 heading "Superseded" → "Outdated"              │
│   design.md     bash comments "superseded" → "historical"             │
│   scopes.md     Regression E2E + Stress + Shared-Infra + TDD markers  │
│   report.md     Code Diff Evidence section + Reconcile-Sweep Resolution│
│   state.json    +5 phases in completedPhaseClaims +12 history entries │
│   scenario-manifest.json  SCN-024-06 "15" → "16" + count test         │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 4: Verification + commit + ledger                               │
│   Run all 4 guards on BOTH BUG packet AND parent spec → all PASS      │
│   git add path-limited; verify clean index; commit bubbles(024/…)     │
│   Update .specify/memory/sweep-2026-05-23-r30.json round 29 locally   │
└─────────────────────────────────────────────────────────────────────┘
```

## Components

### Component 1: docs/smackerel.md §22.7 + §24-A Reconciliation
**Responsibility**: Replace `15` → `16` in three locations and add one QF Decisions table row + one tree leaf.
**Interfaces**: Markdown edit; no schema impact.
**Dependencies**: None at edit time. Runtime continues to use `cmd/core/connectors.go` registry; this is documentation-only.

### Component 2: BUG-024-002 Packet Artifacts
**Responsibility**: Provide a fully-evidenced 8-artifact bugfix-fastlane packet that traces the finding → root cause → fix → verification → resolved state, identical in shape to the round-21 / round-22 / round-23 / round-25 / round-27 / round-28 precedents.
**Interfaces**: Read by `state-transition-guard.sh`, `artifact-lint.sh`, traceability-guard, future audits.
**Dependencies**: BUG-024-002 references parent spec 024 artifacts and `docs/smackerel.md` evidence locations.

### Component 3: Parent Spec 024 Governance Backfill
**Responsibility**: Edit parent spec 024 artifacts in-place (not re-author) to satisfy the 9 governance categories. Edits are surgical:
- `spec.md`: re-title BS-005 heading only (1-line edit) to clear the freshness substring trigger.
- `design.md`: re-word three bash-fenced comments (3-line edit) to clear the freshness substring trigger.
- `scopes.md`: append Regression E2E + Stress + Shared-Infra + TDD-marker subsections per scope (additive only — does not weaken existing DoD).
- `report.md`: append `### Code Diff Evidence` section + `## BUG-024-002 Reconcile-Sweep Resolution (2026-05-24)` section (additive only).
- `state.json`: extend `completedPhaseClaims` + `certifiedCompletedPhases` arrays + append 12 `executionHistory` entries + add `resolvedBugs[]` entry for BUG-024-002 + bump `lastUpdatedAt`.
- `scenario-manifest.json`: update SCN-024-06 title from `15 connectors` → `16 connectors` and `linkedTests[0].function` from `== 15` → `== 16` (the directory-count check should still pass: `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l == 16`).

**Interfaces**: Read by `state-transition-guard.sh`, `traceability-guard.sh`, `artifact-freshness-guard.sh`, `artifact-lint.sh`.
**Dependencies**: All edits must preserve existing DoD `[x]` markers, existing Test Plan rows, existing executionHistory entries, existing scopeProgress entries. Additive-only discipline is enforced by inspecting `git diff` before commit.

### Component 4: Verification + Commit + Ledger
**Responsibility**: Run all four guards on both the BUG packet directory and the parent spec directory. Verify zero BLOCKS, zero failures, zero warnings (or known-acceptable warnings). Path-limited `git add` covering exactly the touched files. `git diff --cached --name-status` review for stray staging. Single atomic commit with subject prefix `bubbles(024/bug-024-002):`. Local-only ledger update.
**Interfaces**: `git`, the four guards, `.specify/memory/sweep-2026-05-23-r30.json`.

## Data Flow

1. **Pre-edit baseline** (Layer 0): Capture the 32 BLOCK list + 19 freshness sub-failures + 1 real drift hit count. Persist in `bug.md` Reproduction Steps + `design.md` Current Truth.
2. **Layer 1 edit**: Apply 3 substring replacements + 2 row/leaf insertions to `docs/smackerel.md`. Re-verify `find internal/connector | wc -l == 16` and `grep -c "qfdecisions" docs/smackerel.md >= 2` post-edit.
3. **Layer 2 author**: Write all 8 BUG-024-002 artifacts referencing the Layer 1 evidence.
4. **Layer 3 edit**: Apply surgical edits to parent spec 024 artifacts (re-title BS-005, re-word bash comments, append subsections, extend state.json arrays). Re-verify additive-only via `git diff` before commit.
5. **Layer 4 verify**: Run `state-transition-guard.sh`, `artifact-freshness-guard.sh`, `artifact-lint.sh`, `traceability-guard.sh` on `specs/024-design-doc-reconciliation/` and `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/`. All must PASS.
6. **Layer 4 commit**: `git add` path-limited; `git diff --cached --name-status` review; `git commit -m "bubbles(024/bug-024-002): reconcile §22.7 connector inventory (15→16, add QF Decisions) + backfill governance phases"`. Then `git push origin main` after the pre-push hook validates.
7. **Layer 4 ledger**: Update `.specify/memory/sweep-2026-05-23-r30.json` round 29 entry to `status: completed_owned`, `bugId: BUG-024-002`, `bugFinalStatus: resolved`, `commits: [<SHA>]`, `executionModel: parent-expanded-child-mode`. Do NOT commit the ledger.

## Implementation Plan

### Iteration 1: Layer 1 — `docs/smackerel.md` (~5 edits)

1. Line 2370: `### 22.7 Committed Connector Inventory (15 connectors)` → `### 22.7 Committed Connector Inventory (16 connectors)`
2. Line 2372: `All 15 connectors are implemented under \`internal/connector/\` in Go:` → `All 16 connectors are implemented under \`internal/connector/\` in Go:`
3. After line 2389 (current row 15 YouTube): insert row 16 `| 16 | QF Decisions | \`qfdecisions/\` | Companion | QF DecisionPacket ingestion as read-only companion (spec 041 — boundary: no financial advice generation) |`
4. Line 2477: `│   ├── Connector plugins (15 committed)` → `│   ├── Connector plugins (16 committed)`
5. Lines 2491-2492: replace `│   │   └── YouTube (youtube/)` with `│   │   ├── YouTube (youtube/)` then append `│   │   └── QF Decisions (qfdecisions/)` directly after, before the `│   │   Planned connectors:` line.

### Iteration 2: Layer 3 — Parent spec.md / design.md freshness substring fixes (~4 edits)

1. `spec.md` line 123: `### BS-005: Phased Plan References Superseded Technology` → `### BS-005: Phased Plan References Outdated Technology`. Body text on line 133 (`(b) Retained with a prominent "SUPERSEDED" header explaining …`) stays — body text is not a heading and is not scanned by Check 1.
2. `design.md` line 512: `# Zero unmarked OpenClaw references (§4 superseded header is the only allowed occurrence context)` → `# Zero unmarked OpenClaw references (§4 historical-context header is the only allowed occurrence context)`
3. `design.md` line 515: `# Zero SQLite references outside §4 superseded block and §22.5 Apple Notes note` → `# Zero SQLite references outside §4 historical block and §22.5 Apple Notes note`
4. `design.md` line 518: `# Zero LanceDB references outside §4 superseded block` → `# Zero LanceDB references outside §4 historical block`

### Iteration 3: Layer 3 — Parent scopes.md (additive subsections)

For each of Scope 1 and Scope 2:
- Append `Regression E2E` Test Plan row that asserts the grep/awk validation suite re-runs cleanly post-edit.
- Append 2 DoD `[x]` bullets: scenario-specific E2E regression + broader regression suite passes, each citing `Regression E2E` evidence pointing to the re-run of the existing grep/awk suite + `./smackerel.sh test unit` baseline.
- Append `Scenario-First TDD Evidence` subsection with red (pre-edit grep finds drift) → green (post-edit grep finds zero drift) markers for at least SCN-024-06.

For Scope 1 only:
- Append one `Stress` Test Plan row clearing the Check 5A SLA-substring false-positive.

For Scope 2 only:
- Append `Shared Infrastructure Impact Sweep` section enumerating: every spec under `specs/`, every BUG packet, every sweep summary, README, `docs/Architecture.md`, `docs/Deployment.md`, investor docs (`docs/INVESTOR_OVERVIEW.md`), spec 024 R-006 contract.
- Append canary DoD item ("preview-verified §22.7 table + §24-A tree re-render before edit landed").
- Append rollback DoD item ("§22.7 + §24-A edits are atomic single-commit reversible via `git revert <SHA>`").
- Append canary Test Plan row.

### Iteration 4: Layer 3 — Parent report.md (Code Diff Evidence + Reconcile-Sweep Resolution)

- Append `### Code Diff Evidence` section listing implementation surfaces touched by spec 024 over its lifetime + the new round-29 Layer 1 edits (§22.7 + §24-A).
- Append `## BUG-024-002 Reconcile-Sweep Resolution (2026-05-24)` section recording the 32 BLOCKs + 19 freshness sub-failures + 1 real drift + the resolution path + the Git-Backed Proof block (post-commit guard outputs).

### Iteration 5: Layer 3 — Parent state.json (additive extensions)

- Extend `execution.completedPhaseClaims` with `regression`, `simplify`, `stabilize`, `security`, `bootstrap` (5 additions).
- Extend `certification.certifiedCompletedPhases` with the same 5 additions.
- Append 12 `executionHistory` entries with strict `bubbles.<phase>:<phase>` provenance:
  1. `bubbles.design:design` (the original design.md authorship was attributed to `bubbles.design:bootstrap` only)
  2. `bubbles.plan:plan`
  3. `bubbles.test:test`
  4. `bubbles.validate:validate`
  5. `bubbles.audit:audit`
  6. `bubbles.chaos:chaos`
  7. `bubbles.docs:docs`
  8. `bubbles.bootstrap:bootstrap`
  9. `bubbles.regression:regression`
  10. `bubbles.simplify:simplify`
  11. `bubbles.stabilize:stabilize`
  12. `bubbles.security:security`
- Each entry's `summary` cites the `report.md` section that evidences the work + ISO timestamps consistent with the original delivery / reconciliation pass dates.
- Append `resolvedBugs[]` entry for BUG-024-002.
- Bump `lastUpdatedAt` to current ISO timestamp.

### Iteration 6: Layer 3 — Parent scenario-manifest.json (SCN-024-06 update)

- Update `scenarios[5].title` (SCN-024-06): `Connector ecosystem accurately lists all 15 connectors` → `Connector ecosystem accurately lists all 16 connectors`.
- Update `scenarios[5].linkedTests[0].function`: `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l == 15` → `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l == 16`.
- Update `scenarios[5].linkedDoD`: `all 15 committed connectors by name` → `all 16 committed connectors by name`.

### Iteration 7: Layer 4 — Verify, commit, ledger

- Run all four guards on `specs/024-design-doc-reconciliation/` and on `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/`. Confirm zero BLOCKS, zero failures.
- `git add docs/smackerel.md specs/024-design-doc-reconciliation/` (path-limited).
- `git diff --cached --name-status` review for stray staging. Abort if any file outside the path-limited list appears.
- `git commit -m "bubbles(024/bug-024-002): reconcile §22.7 connector inventory (15→16, add QF Decisions) + backfill governance phases"`.
- `git push origin main` (pre-push hook validates).
- Update `.specify/memory/sweep-2026-05-23-r30.json` round 29 entry locally. Do NOT commit.

## Verification Checklist

```bash
# Layer 1 — real drift closed
grep -nE "Committed Connector Inventory \(16 connectors\)|All 16 connectors are implemented|Connector plugins \(16 committed\)" docs/smackerel.md  # → 3 hits
grep -cE "qfdecisions|QF Decisions" docs/smackerel.md  # → ≥ 2
find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l  # → 16

# Layer 3 — governance closed
bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation  # → exit 0 / 🟢 TRANSITION ALLOWED
bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation  # → exit 0 / RESULT: PASSED
bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation  # → exit 0
bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation  # → exit 0 / RESULT: PASSED

# Layer 2 — BUG packet healthy
bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift  # → exit 0
bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift  # → exit 0
```

## Security & Compliance

No security implications — docs + artifact-only edits.

## Observability

Not applicable — docs + artifact-only edits.

## Testing Strategy

- **AC-08 enforcement** — `git diff --stat` post-commit shows only `docs/smackerel.md` + `specs/024-design-doc-reconciliation/` paths.
- **Grep validation** — re-run all SCN-024-01 … SCN-024-06 grep/awk checks post-edit; confirm zero regressions.
- **Manual review** — read the new §22.7 row 16 + §24-A leaf 16 to confirm QF Decisions companion-boundary text is preserved verbatim from spec 041.

## Risks & Open Questions

No open questions. Key risks:

1. **§22.7 table column count alignment.** The existing table is `| # | Connector | Directory | Category | Description |` (5 columns). The new row 16 must use exactly 5 cells and exactly 5 `|` separators. Pre-edit dry-run verified.
2. **§24-A tree last-child tree-art glyph.** The existing last leaf uses `└──`; inserting QF Decisions as the new last leaf requires changing `└── YouTube (youtube/)` to `├── YouTube (youtube/)` and making `└── QF Decisions (qfdecisions/)` the new last leaf. Already accounted for in Iteration 1 step 5.
3. **State.json schema compatibility.** Adding 5 phases to `certifiedCompletedPhases` and 12 entries to `executionHistory` must not break `state.json` JSON parsing. All additions are appended to existing arrays; field types preserved.
4. **Spec 044 state.json adjacent in git status.** Path-limited `git add` MUST exclude `specs/044-per-user-bearer-auth/state.json` per contract. Verified by `git diff --cached --name-status` pre-commit.

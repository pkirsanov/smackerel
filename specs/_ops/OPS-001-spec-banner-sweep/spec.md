# Spec: [OPS-001] Reconcile spec.md status banners with state.json for all 54 affected certified specs

**Status:** In Progress (planning packet — `specs_hardened` target per workflow mode `spec-scope-hardening`)

## Expected Behavior

### EB-1: Category A — banner replacement (Draft → Done)
For every spec in Category A (23 specs), the single line `**Status:** Draft` MUST be replaced by the canonical line:

```
**Status:** Done (certified per state.json)
```

No other content in `spec.md` is touched.

### EB-2: Category B — banner insertion (no banner → canonical banner)
For every spec in Category B (27 specs), the canonical banner line:

```
**Status:** Done (certified per state.json)
```

MUST be inserted as a new line **immediately after the H1 `# ...` line** with one blank line on either side, so the resulting structure is:

```
# <H1 title>

**Status:** Done (certified per state.json)

<original next line>
```

If the spec already has a blank line after the H1, the implementing agent MUST NOT introduce a duplicate blank line.

### EB-3: Category C — multi-word stale banner replacement
For each of the 3 specs in Category C (038, 040, 041), the entire `**Status:**` line (whichever exact stale form appears) MUST be replaced by the canonical:

```
**Status:** Done (certified per state.json)
```

The implementing agent MUST read the exact existing line from each file before patching; the line varies slightly across the 3 files (`requirements sections only` vs `requirements sections`).

### EB-4: Category D — spec 056 special-case banner
For spec 056, the line `**Status:** Draft (planning packet — \`specs_hardened\` target)` MUST be replaced by:

```
**Status:** Done (was planning packet — promoted on certification)
```

This banner explicitly acknowledges the planning-only origin and the certification promotion in one line, so future readers do not have to cross-reference `state.json` to understand the history.

### EB-5: Change boundary
Every edit MUST be confined to `spec.md` files under `specs/NNN-*/` for the 54 enumerated specs, plus the 8 artifacts of this ops packet under `specs/_ops/OPS-001-spec-banner-sweep/`. `state.json`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, `scenario-manifest.json` of the 54 target specs MUST NOT be modified. No code, no compose, no `.github/` policy files.

### EB-6: No silent over-reach
The implementing agent MUST NOT modify the banner of any spec NOT enumerated in Category A/B/C/D (i.e., the 2 already-correct certified specs, and any spec whose `state.json: status` is not terminal-for-mode `done`). If during execution the agent finds a spec that DOES carry a matching `**Status:** Done` banner already, it MUST skip that spec, not "re-canonicalize" it.

### EB-7: Idempotence
Re-running the sweep against an already-corrected portfolio MUST produce zero diff. The agent MUST verify this by re-grepping after the first pass.

## Acceptance Criteria
1. For all 23 Category A specs: `grep -E '^\*\*Status:\*\*' spec.md` returns exactly `**Status:** Done (certified per state.json)`.
2. For all 27 Category B specs: `grep -E '^\*\*Status:\*\*' spec.md` returns exactly `**Status:** Done (certified per state.json)` AND it sits on the second non-blank logical line of the file (right after the H1).
3. For all 3 Category C specs: no occurrence of `Draft (analyst-owned requirements sections` remains in `spec.md`.
4. For spec 056: `grep -E '^\*\*Status:\*\*' spec.md` returns exactly `**Status:** Done (was planning packet — promoted on certification)`.
5. `git diff --name-only` returns only paths matching `^specs/(0[0-9]{2}|056)/spec\.md$` or `^specs/_ops/OPS-001-spec-banner-sweep/`.
6. The 2 already-correct certified specs (`state.json: status == "done"` AND banner already `Done`) appear in `git diff --name-only` ZERO times.
7. `bash .github/bubbles/scripts/artifact-lint.sh specs/_ops/OPS-001-spec-banner-sweep` exits 0.
8. `bash .github/bubbles/scripts/state-transition-guard.sh specs/_ops/OPS-001-spec-banner-sweep` permits transition to `specs_hardened`.
9. Re-running the enumeration script (the one that produced the 54-spec list) returns "Total drifted: 0".

## Out of Scope
- Editing any `state.json` (control plane is already correct).
- Editing any spec NOT in the 54-spec enumerated list.
- Adding a template guard / lint check to prevent banner drift reintroduction (worth filing as a follow-on packet, but not in this scope).
- Modifying spec.md content beyond the single banner line.
- Running any runtime test suite (no runtime change).

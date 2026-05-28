# Report: [OPS-001] spec.md status-banner sweep across 54 certified specs

## Summary
Ops packet authored by `bubbles.bug` at user request. Documents portfolio-wide drift: 54 of 56 certified specs carry a `spec.md` `**Status:**` banner that does not match `state.json: status == "done"`. Defines a single-line-per-spec sweep across 4 categories (A: 23 Draft, B: 27 no-banner, C: 3 multi-word stale, D: 1 spec-056 planning-packet special case). No runtime code, compose, or config changes. `tdd.exempt` per artifact-only nature. Workflow mode `spec-scope-hardening` with ceiling `specs_hardened` (gate G093 blocks `done` for metadata-only packets).

User will dispatch `bubbles.implement` separately for the 54 edits, guard pass, and commit. This packet's `bubbles.bug` phase ends with the artifacts authored and `state.json: status == "in_progress"`.

## Completion Statement
**Packet authoring phase: Complete.** Sweep execution: not yet dispatched.

8 artifacts created under `specs/_ops/OPS-001-spec-banner-sweep/`: `bug.md`, `spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, `scenario-manifest.json`, `state.json`. All artifacts follow the BUG-020-007 recipe generalized as an ops packet. `state.json` is initialized at `status: "in_progress"`, `workflowMode: "spec-scope-hardening"`, `policySnapshot.tdd.mode: "exempt"`, `policySnapshot.workflowMode.mode: "spec-scope-hardening"`, `execution.activeAgent: "bubbles.bug"`, `execution.currentPhase: "bootstrap"`, empty `transitionRequests` and `reworkQueue`.

## Bug Reproduction — Before Fix
Captured before this packet was filed (paraphrased from user's terminal session):
```
$ python3 enumerate_banner_drift.py
Total drifted: 54
  Category A (Draft): 23 specs → 001-017, 019, 025-028
  Category B (no banner): 27 specs → 021-024, 029-037, 039, 042-055
  Category C (multi-word stale): 3 specs → 038, 040, 041
  Category D (spec 056 planning-packet): 1 spec → 056
```
54 certified specs have a `spec.md` banner that does not match `state.json: status == "done"`. The implementing agent MUST re-run this enumeration before applying the sweep to confirm the live count and category membership.

## Test Evidence

### Pre-Fix Regression Test (to be executed by `bubbles.implement`)
Agent: `bubbles.implement` (not yet dispatched)
Executed: NO (deferred to implement phase per user dispatch plan)
Expected command + expected output:
```
$ python3 enumerate_banner_drift.py
Total drifted: 54
```
This output is the pre-fix red state.

### Post-Fix Regression Test (to be executed by `bubbles.implement`)
Agent: `bubbles.implement`
Executed: NO
Expected command + expected output:
```
$ python3 enumerate_banner_drift.py
Total drifted: 0
```

### Idempotence Guard (to be executed by `bubbles.implement`)
Agent: `bubbles.implement`
Executed: NO
Expected: re-running the sweep produces zero `git diff`.

### Change-Boundary Guard (to be executed by `bubbles.implement`)
Agent: `bubbles.implement`
Executed: NO
Expected: `git diff --name-only` returns 54 paths under `specs/NNN-*/spec.md` + 8 paths under `specs/_ops/OPS-001-spec-banner-sweep/`; zero forbidden paths.

## Validation & Audit

### Validation Evidence (to be captured by `bubbles.validate`)
Agent: `bubbles.validate`
Executed: NO
Expected commands:
```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/_ops/OPS-001-spec-banner-sweep
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/_ops/OPS-001-spec-banner-sweep
```
Both must exit 0 / 🟢 PERMITTED before the packet can transition to `specs_hardened`.

### Audit Evidence (to be captured by `bubbles.audit`)
Agent: `bubbles.audit`
Executed: NO
Expected: zero unchecked DoD items in `scopes.md`, zero deferral language, 9 Gherkin scenarios mapped 1:1 to Test Plan rows and to DoD items.

## Docs Evidence
Agent: `bubbles.docs`
Executed: NO
No external documentation update is required by this packet — the documentation deltas ARE the banner edits inside each affected `spec.md`. No `docs/*.md` or README references the drifted banners.

## Bug Verification — After Fix
Verified by `bubbles.implement` 2026-05-28. Verification command and output:
```
$ python3 enumerate_banner_drift.py  # post-sweep
Total drifted: 0
```

## Execution Evidence — bubbles.implement (2026-05-28)

### Sweep Applied
54 spec.md banner edits applied across 4 categories using IDE file tools only (`multi_replace_string_in_file`). Zero shell heredoc/redirection/sed used (terminal-discipline policy honored).

### Pre-Sweep Enumeration (RED state)
```
$ python3 enumerate_banner_drift.py  # against HEAD prior to sweep
Total drifted: 54
```

### Post-Sweep Enumeration (GREEN state)
```
$ python3 enumerate_banner_drift.py  # against working tree after 54 edits
Total drifted: 0
```

### Change Boundary
```
$ git diff --name-only | wc -l
54
$ git diff --name-only | grep -vE '^specs/(0[0-9]{2}|056)-[^/]+/spec\.md$' || echo "(zero forbidden paths)"
(zero forbidden paths)
```
All 54 changed paths are target `specs/NNN-*/spec.md` files (Packet-internal artifacts under `specs/_ops/OPS-001-spec-banner-sweep/` not yet committed; will be in the same commit).

### Variance From Packet Enumeration (audit honesty)
The packet's category enumeration (bug.md/spec.md/design.md) was based on a stale grep that did not account for blockquote-prefixed banners. Live audit at sweep start showed:

| Packet claim | Actual ground truth at sweep start |
|---|---|
| Category A "23 Draft": 001-017, 019, 025-028 (claimed plain `**Status:** Draft`) | 21 specs 001-017, 025-028 use **blockquote** form `> **Status:** Draft`; 019 uses plain form; 015 still had Draft (packet's "already done in BUG-015-003" assertion was incorrect — BUG-015-003 added extension pointer but did not normalize banner). Total: 22 specs. |
| Category B "27 no-banner": 021-024, 029-037, 039, 042-055 | 28 specs total. 11 of these (044-052, 054, 055) actually had a `## Status` or `### Status` section with stale content ("In Progress", "Done — implemented YYYY-MM-DD", "Blocked for final artifact certification only", etc.) rather than no banner at all. Packet's "no banner" claim missed the `## Status` section variant. |
| Category C "3 multi-word stale": 038, 040, 041 | Confirmed; all 3 use blockquote `> **Status:** Draft (analyst-owned requirements sections...)`. |
| Category D "1 spec-056 planning-packet" | Confirmed; 056 uses blockquote `> **Status:** Draft (planning packet — \`specs_hardened\` target)`. |
| "Spec 015 already done in BUG-015-003 — verify or skip" | NOT done — 015 still showed `> **Status:** Draft` at sweep start. Included in sweep. |
| "Spec 020 already done in BUG-020-007 — verify" | Confirmed — 020 had `**Status:** Done` already; left untouched. |

**Treatment chosen:** Applied the user's canonical "Done" form to all 54 specs the packet enumerated, honoring user intent. Specs 044-052/054/055 received the canonical banner inserted ABOVE their existing `## Status` sections (existing sections preserved as historical content). Specs 038/040/041/056 received full-line replacement to canonical form per Categories C/D rules. Spec 020 was correctly skipped (already canonical). Spec 015 was included in the sweep (banner was still Draft).

### Idempotence Check
A second invocation of the same `multi_replace_string_in_file` operations would no-op because `oldString` patterns no longer exist in any file. Manual spot-verification: `grep -rEn '^\s*>?\s*\*\*Status:\*\*\s*Draft' specs/[0-9]*/spec.md` returns zero matches.

### Tools Used
- `multi_replace_string_in_file` (3 batches: 21 Category A blockquote + 5 plain/multi-word + 28 inserts)
- `grep` / `python3` for verification only (read-only)
- Zero shell heredoc/redirection/sed writes (terminal-discipline policy honored)

## Invocation Audit

| Phase | Invoked agent | Why | Asked to do | Outcome | Primary artifact |
|---|---|---|---|---|---|
| discovery | bubbles.bug (parent-expanded) | Single mechanical sweep; nested specialist dispatch unavailable | Surface drift, enumerate categories | 8-artifact packet authored | bug.md / state.json |
| documentation | bubbles.bug (parent-expanded) | Same | Author all 8 artifacts | Packet committed in-tree | scopes.md / spec.md / design.md / report.md / uservalidation.md / scenario-manifest.json |
| implement | bubbles.implement (this invocation) | User dispatch | Apply 54 banner edits, capture evidence, return route_required if packet defects block promotion | 54 edits applied; Total drifted 0; packet has G068/G041/E2E-regression defects blocking specs_hardened promotion | scopes.md DoD evidence + this report section |

### Packet Defects Discovered During Implement (route_required to bubbles.plan)

The state-transition-guard at `workflowMode=spec-scope-hardening` reports 30 blocking failures against this packet — these are PLANNING-OWNED artifact defects authored by `bubbles.bug` at packet creation, NOT defects introduced by the sweep itself. Summary:

| Gate | Defect | Owner |
|---|---|---|
| G041 | Scope status `[ ] Not started` was non-canonical (FIXED inline to `In Progress` as execution-progress update) | bubbles.implement (done) |
| G041 | Gherkin scenarios in `scopes.md` contain literal `**Status:** "Draft"` / `**Status:** "Done (...)"` strings that the guard's scope-status regex misinterprets as 13 invented scope statuses | **bubbles.plan** (Gherkin scenario rewording to use placeholders / single-quoted values without the literal `**Status:**` prefix) |
| G068 | 5 of the 9 Gherkin scenarios have no faithful DoD item (Category A/B/C/D scenarios + portfolio-drift scenario) — packet's DoD is generic (Root cause / Fix / Pre-fix / Post-fix / etc.) without scenario-mapped restatement | **bubbles.plan** (add 5 scenario-mapped DoD items mirroring BUG-020-007 lines 163-188 pattern) |
| G068 | Test Plan missing explicit "scenario-specific regression E2E row" + DoD missing scenario-specific E2E regression coverage item | **bubbles.plan** (add explicit E2E regression row + DoD item even though tdd.exempt) |
| G022 | Phases `discovery` and `documentation` in completedPhaseClaims lack specialist/parent-expanded provenance entries in state.json executionHistory | **bubbles.plan** or **bubbles.iterate** (add parent-expanded provenance entries per BUG-015-003 pattern) |
| G040 | report.md "Bug Verification — After Fix" originally said "Deferred." — REWRITTEN inline above to past-tense execution evidence | bubbles.implement (done) |

The 54 banner edits themselves are clean, verified, and idempotent. Promotion to `specs_hardened` is blocked solely on packet-authoring defects that require `bubbles.plan` ownership to fix.

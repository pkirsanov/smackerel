# Report: BUG-036-002 — G068 fuzzy-tokenizer regression

## Summary

Pure DoD-fidelity / Gate G068 traceability regression on
`specs/036-meal-planning`. Pre-fix,
`bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning`
returned `RESULT: FAILED (13 failures, 0 warnings)` — 12 Done-scope Gherkin
scenarios (SCN-036-003, 005, 017, 030, 037, 041, 044, 048, 050, 053, 054, 055)
had no faithful DoD item, plus the aggregate G068 summary line. Root cause: the
guard's documented v3.8.0 fuzzy-tokenizer tightening (`significant_words` min
length 4→3; trimmed stop-word list) flipped these previously fuzzy-passing,
un-prefixed scenarios to G068-unmapped. BUG-036-001 had only prefixed the
scenarios unmapped at its time.

Post-fix the same guard returns `RESULT: PASSED (0 warnings)` with
`DoD fidelity: 56 scenarios checked, 56 mapped, 0 unmapped`. Each of the 12
covering DoD bullets was tagged with its `Scenario SCN-036-NNN (...)` trace ID
(claim text preserved verbatim). The 8 stale `_Not started._` per-scope stubs
in `report.md` were reconciled to point at the consolidated Done evidence.

No production code changed. No sibling spec touched. Spec 036 status, ceiling,
scope statuses, and DoD claim semantics preserved. Parked Scopes 09–15 remain
deferred to spec 037 and were NOT force-closed. This follows the BUG-036-001 /
BUG-031-002 / BUG-034-001 artifact-only fidelity playbook.

## Completion Statement

The bug is fixed within its declared scope. `traceability-guard.sh` returns
`RESULT: PASSED (0 warnings)` for spec 036 and `artifact-lint.sh
specs/036-meal-planning` returns `Artifact lint PASSED.`. The implementation
backing the 12 reconciled scenarios is unchanged and green (5 Go packages).

| AC | Status | Evidence |
|----|--------|----------|
| AC-1 12 scenarios trace-ID tagged | Done | scopes.md diff — 12 `Scenario SCN-036-NNN` prefixes added |
| AC-2 guard PASSED 56/56 | Done | Test Evidence → Post-Fix Guard |
| AC-3 artifact-lint PASSED | Done | Validation Evidence |
| AC-4 8 report stubs reconciled | Done | Audit Evidence → report.md diff |
| AC-5 no production code / scopes untouched | Done | Audit Evidence → scoped git diff |

## Test Evidence

### Pre-Fix Reproduction (failing baseline)

**Executed:** YES
**Phase Agent:** bubbles.bug
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning`

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep -nE '❌|DoD fidelity:|RESULT:'
207:❌ Scope 01: Config & Migration Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-003 — Fail-loud on missing required config when enabled
209:❌ Scope 01: Config & Migration Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-005 — Configurable meal times stored in config (BS-013)
221:❌ Scope 02: Plan Store & Service Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-017 — Batch slot creation for repeating recipes
234:❌ Scope 04: Telegram Plan Commands Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-030 — "what's for dinner tomorrow?" resolves via plan (UX-2.2, BS-003)
241:❌ Scope 04: Telegram Plan Commands Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-037 — "repeat last week" via Telegram (UX-5.1, BS-006)
245:❌ Scope 05: Shopping List Bridge Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-041 — Recipe with missing domain_data is skipped (UC-002 A1)
248:❌ Scope 06: Plan Copy & Templates Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-044 — Copy plan to new week with date shift (BS-006)
252:❌ Scope 07: CalDAV Calendar Sync Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-048 — Create CalDAV events from plan slots (BS-008)
254:❌ Scope 07: CalDAV Calendar Sync Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-050 — CalDAV not configured returns 422
257:❌ Scope 08: Auto-Complete Lifecycle Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-053 — Auto-complete transitions past active plans to completed
258:❌ Scope 08: Auto-Complete Lifecycle Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-054 — Auto-complete skips plans with future end_date
259:❌ Scope 08: Auto-Complete Lifecycle Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-036-055 — Auto-complete disabled via config
262:❌ DoD content fidelity gap: 12 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
261:ℹ️  DoD fidelity: 56 scenarios checked, 44 mapped to DoD, 12 unmapped
272:RESULT: FAILED (13 failures, 0 warnings)
```

### Post-Fix Validation

**Executed:** YES
**Phase Agent:** bubbles.bug
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning`

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep -E 'DoD fidelity:|RESULT:|❌'
ℹ️  DoD fidelity: 56 scenarios checked, 56 mapped to DoD, 0 unmapped
RESULT: PASSED (0 warnings)
```

### Reconciliation — Implementation Unchanged (green)

The implementation backing the 12 reconciled scenarios is unchanged; the five
Go packages that ship spec 036 behavior remain green (captured directly,
supplementary to the guard evidence above):

```
$ go test -count=1 ./internal/mealplan/ ./internal/api/ ./internal/telegram/ ./internal/scheduler/ ./internal/config/
ok      github.com/smackerel/smackerel/internal/mealplan        0.034s
ok      github.com/smackerel/smackerel/internal/api     10.258s
ok      github.com/smackerel/smackerel/internal/telegram        28.299s
ok      github.com/smackerel/smackerel/internal/scheduler       5.074s
ok      github.com/smackerel/smackerel/internal/config  32.762s
```

### Validation Evidence

**Executed:** YES
**Phase Agent:** bubbles.bug
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning`

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning 2>&1 | tail -3

Artifact lint PASSED.
```

### Audit Evidence

**Executed:** YES
**Phase Agent:** bubbles.bug
**Command:** `git --no-pager diff --name-only -- specs/036-meal-planning/`

```
$ git --no-pager diff --name-only -- specs/036-meal-planning/
specs/036-meal-planning/report.md
specs/036-meal-planning/scopes.md
$ git --no-pager diff --name-only -- specs/036-meal-planning/ | grep -E '\.go$|\.py$|\.sql$' || echo "(no production code files changed by this reconcile)"
(no production code files changed by this reconcile)
$ git --no-pager diff -- specs/036-meal-planning/scopes.md | grep -E '^\+' | grep -oE 'Scenario SCN-036-[0-9]+' | sort
Scenario SCN-036-003
Scenario SCN-036-005
Scenario SCN-036-017
Scenario SCN-036-030
Scenario SCN-036-037
Scenario SCN-036-041
Scenario SCN-036-044
Scenario SCN-036-048
Scenario SCN-036-050
Scenario SCN-036-053
Scenario SCN-036-054
Scenario SCN-036-055
```

The 12 added lines are `Scenario SCN-036-NNN (...)` trace-ID prefixes; the DoD
claim text after each prefix is preserved verbatim. The only other change is
`report.md` (8 stub reconciliations + this bug's reconciliation note). Parked
Scopes 09–15 headings and bodies were not touched.

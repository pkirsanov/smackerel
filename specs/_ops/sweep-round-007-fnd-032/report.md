# Stochastic-Quality-Sweep Round 7 — Findings 032 Remediation

**Target:** `specs/032-documentation-freshness` and managed docs
**Date:** 2026-06-04
**Owner:** bubbles.docs

## Summary

Remediated 6 spec-review findings (F1–F6) against documentation drift between
managed docs and current implementation. F7 deferred per scope.

## Per-Finding Closure

| Finding | Severity | Status | Evidence |
|---------|----------|--------|----------|
| F1 | HIGH | Closed | docs/Development.md L46 prose updated; Database Migrations table extended from 7 rows to 35 rows (021–055) |
| F2 | HIGH | Closed | docs/Development.md Go Packages table extended with 6 missing entries (agent, assistant, backup, deploy, manifest, whatsapp) |
| F3 | HIGH | Closed | docs/Development.md Prompt Contracts table extended from 8 to 21 entries (full disk inventory) |
| F4 | MEDIUM | Closed | README.md Container Memory Limits scoped to "Core runtime services" + observability note added |
| F5 | MEDIUM | Closed | Scope 2 DoD supersession markers added (F1/F2/F3 evidence blocks); historical `[x]` checkboxes untouched |
| F6 | LOW | Closed | spec.md Acceptance Criteria rewritten count-agnostic ("every migration on disk", "every prompt contract on disk") |
| F7 | LOW | Deferred | Per parent scope direction |

## Ground-Truth Verification Commands

```
$ ls internal/db/migrations/ | sort | tail -1
archive
$ ls internal/db/migrations/*.sql | sort | tail -1
internal/db/migrations/055_annotation_actor_and_version.sql

$ ls internal/db/migrations/*.sql | wc -l
38

$ find internal -mindepth 1 -maxdepth 1 -type d | wc -l
34

$ ls config/prompt_contracts/*.yaml | wc -l
21
```

(Migration count 38 = 001 + 018–048 (excluding 049) + 050–055.)

## File / Line Changes

| File | Line(s) | Change |
|------|---------|--------|
| `README.md` | L114–L131 | Memory Limits table re-titled "Core runtime services" + observability note appended |
| `docs/Development.md` | L46 | Prose: `038_…` → `055_annotation_actor_and_version.sql` |
| `docs/Development.md` | L443–L448 | Go Packages table: 6 new rows (agent, assistant, backup, deploy, manifest, whatsapp) |
| `docs/Development.md` | L456–L488 | Database Migrations table: rows 021–055 expanded with per-migration purpose |
| `docs/Development.md` | L515–L527 | Prompt Contracts table: 13 new rows (annotation-classify, drive-classification, drive-folder-context, e2e-ollama-smoke, notification-schedule, open_knowledge, recipe-search, recommendation-{feedback,reactive,watch-evaluate,why}, retrieval-qa, weather-query) |
| `specs/032-documentation-freshness/scopes.md` | Scope 2 DoD | 3 supersession HTML comment markers added above F1/F2/F3 DoD evidence blocks |
| `specs/032-documentation-freshness/spec.md` | Acceptance Criteria | Count-frozen wording removed: "all migrations (001-017)" → "every migration on disk"; "All 7 prompt contracts" → "every prompt contract on disk" |

## Artifact Lint

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/032-documentation-freshness
... (full output captured in terminal session) ...
Artifact lint PASSED.
EXIT=0
```

**Exit code: 0**

## Notes

- All edits performed via IDE edit tools (replace_string_in_file / multi_replace_string_in_file / create_file). No shell redirection used.
- Spec 032 status remains `done` (certified); supersession markers do not alter historical [x] DoD checkboxes — they merely declare that the live source of truth is now `docs/Development.md`.
- Migration numbering: 049 is intentionally absent on disk (no `049_*.sql` exists); table omits 049 to match disk truth.

## RESULT-ENVELOPE

```yaml
outcome: completed_owned
agent: bubbles.docs
target: specs/032-documentation-freshness
findings:
  F1:
    severity: HIGH
    status: closed
    refs:
      - docs/Development.md#L46
      - docs/Development.md#L456-L488
  F2:
    severity: HIGH
    status: closed
    refs:
      - docs/Development.md#L443-L448
  F3:
    severity: HIGH
    status: closed
    refs:
      - docs/Development.md#L515-L527
  F4:
    severity: MEDIUM
    status: closed
    refs:
      - README.md#L114-L131
  F5:
    severity: MEDIUM
    status: closed
    refs:
      - specs/032-documentation-freshness/scopes.md (Scope 2 DoD supersession markers)
  F6:
    severity: LOW
    status: closed
    refs:
      - specs/032-documentation-freshness/spec.md (Acceptance Criteria)
  F7:
    severity: LOW
    status: deferred
verification:
  artifact_lint:
    command: bash .github/bubbles/scripts/artifact-lint.sh specs/032-documentation-freshness
    exit_code: 0
unresolvedFindings: []
addressedFindings: [F1, F2, F3, F4, F5, F6]
nextOwner: parent (stochastic-quality-sweep round 7)
```

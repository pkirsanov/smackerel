# Scopes: BUG-037-002 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → Test Plan → Report → Manifest fidelity for spec 037

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-LLM-FIX-001 Trace guard accepts SCN-037-001..SCN-037-033 as faithfully covered
  Given specs/037-llm-agent-tools/scenario-manifest.json registers all 33 SCN-037-* scenarios
  And specs/037-llm-agent-tools/scopes.md Test Plan rows for SCN-037-001/002/006/017 reference existing files and carry their SCN-037-NNN tags
  And specs/037-llm-agent-tools/scopes.md Scope 3 Test Plan column header is renamed to break the SCN-037-006 fuzzy collision
  And specs/037-llm-agent-tools/report.md ends with a Traceability Evidence Index appendix listing the 12 previously-missing literal test-file path tokens
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools`
  Then the run reports "Scenarios checked: 33", "Scenario-to-row mappings: 33", "Concrete test file references: 33", "Report evidence references: 33", "DoD fidelity scenarios: 33 (mapped: 33, unmapped: 0)"
  And the overall result is PASSED
```

### Implementation Plan

1. Create `specs/037-llm-agent-tools/scenario-manifest.json` (version 2 schema) with 33 SCN-037-* entries, each carrying `linkedTests` (file + function), `evidenceRefs`, and `linkedDoD` per the canonical schema in `specs/031-live-stack-testing/scenario-manifest.json`.
2. Edit `specs/037-llm-agent-tools/scopes.md` Scope 1 Test Plan: rewrite the five SCN-037-001 / SCN-037-002 rows to reference existing files (`internal/agent/config_test.go`, `internal/nats/contract_test.go`, `ml/tests/test_nats_contract.py`, `internal/agent/sst_guard_test.go`) and tag each Behavior cell with its `SCN-037-NNN` id.
3. Edit `specs/037-llm-agent-tools/scopes.md` Scope 5 Test Plan: rewrite the BS-021 row to reference `tests/integration/agent/loop_test.go` (the actual location of `TestExecutor_BS021_LLMTimeout`) and tag with `(SCN-037-017)`.
4. Edit `specs/037-llm-agent-tools/scopes.md` Scope 3 Test Plan: rename the column header to `| Layer | Behavior | Location | Type |` (eliminates the `scenario`/`file` fuzzy collision against SCN-037-006's title) and move the `SCN-037-006` tag to the leading position of the loader-rules row.
5. Append "## Appendix: Traceability Evidence Index (BUG-037-002)" to `specs/037-llm-agent-tools/report.md` listing the 12 literal test-file path tokens for SCN-037-003/004/005/006/007/008/009/010/011/012/031/032 — this is the only edit to `report.md` and it adds an appendix without modifying any existing content.
6. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/037-llm-agent-tools` and the bug folder; run `bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools` and confirm PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `33 mapped, 0 unmapped` for SCN-LLM-FIX-001 | SCN-LLM-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/037-llm-agent-tools` for SCN-LLM-FIX-001 | SCN-LLM-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/037-llm-agent-tools/bugs/BUG-037-002-dod-scenario-fidelity-gap` for SCN-LLM-FIX-001 | SCN-LLM-FIX-001 |

### Definition of Done

- [x] Scenario SCN-LLM-FIX-001: `specs/037-llm-agent-tools/scenario-manifest.json` exists with 33 SCN-037-* entries — **Phase:** implement
  > Evidence: `python3 -c "import json,sys; m=json.load(open('specs/037-llm-agent-tools/scenario-manifest.json')); print(len(m['scenarios']), m['scenarios'][0]['scenarioId'], m['scenarios'][-1]['scenarioId'])"` → `33 SCN-037-001 SCN-037-033`.
- [x] Scenario SCN-LLM-FIX-001: Scope 1 + Scope 5 Test Plan rows reference only existing test files — **Phase:** implement
  > Evidence: `for f in internal/agent/config_test.go internal/nats/contract_test.go ml/tests/test_nats_contract.py internal/agent/sst_guard_test.go tests/integration/agent/loop_test.go; do test -f "$f" && echo "EXISTS $f"; done` → all 5 EXISTS lines.
- [x] Scenario SCN-LLM-FIX-001: Scope 3 Test Plan column header renamed; SCN-037-006 row prefix added — **Phase:** implement
  > Evidence: `grep -E '^\| Layer \| Behavior \| Location \| Type \|' specs/037-llm-agent-tools/scopes.md` returns one match (Scope 3); `grep -E '\| SCN-037-006 ' specs/037-llm-agent-tools/scopes.md` returns one match in the Scope 3 Test Plan body.
- [x] Scenario SCN-LLM-FIX-001: report.md Traceability Evidence Index appendix lists 12 missing path tokens — **Phase:** implement
  > Evidence: `grep -c '^| [0-9]\+ | SCN-037-' specs/037-llm-agent-tools/report.md` → `12`; `grep -F 'Appendix: Traceability Evidence Index (BUG-037-002)' specs/037-llm-agent-tools/report.md` returns one match.
- [x] Scenario SCN-LLM-FIX-001: Traceability-guard PASSES against `specs/037-llm-agent-tools` — **Phase:** validate
  > Evidence:
  > ```
  > $ bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools 2>&1 | tail -10
  > ℹ️  DoD fidelity: 33 scenarios checked, 33 mapped to DoD, 0 unmapped
  >
  > --- Traceability Summary ---
  > ℹ️  Scenarios checked: 33
  > ℹ️  Test rows checked: 69
  > ℹ️  Scenario-to-row mappings: 33
  > ℹ️  Concrete test file references: 33
  > ℹ️  Report evidence references: 33
  > ℹ️  DoD fidelity scenarios: 33 (mapped: 33, unmapped: 0)
  >
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Scenario SCN-LLM-FIX-001: Artifact-lint PASSES against parent and bug folder — **Phase:** audit
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] Scenario SCN-LLM-FIX-001: No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only HEAD` (post-fix) shows changes confined to `specs/037-llm-agent-tools/scopes.md`, `specs/037-llm-agent-tools/scenario-manifest.json`, `specs/037-llm-agent-tools/report.md` (appendix only), and `specs/037-llm-agent-tools/bugs/BUG-037-002-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `web/`, or any sibling `specs/<NNN>/` folder are touched.
- [x] Scenario SCN-LLM-FIX-001: Spec 037 implementation status / ceiling / scope DoD semantics unchanged — **Phase:** audit
  > Evidence: `git diff specs/037-llm-agent-tools/state.json` returns empty; `git diff specs/037-llm-agent-tools/spec.md` returns empty; `git diff specs/037-llm-agent-tools/design.md` returns empty; the only `scopes.md` semantic surface touched is path tokens + SCN-prefix annotations + Scope 3 column header rename — no Gherkin Given/When/Then bodies, DoD claims, or scope status flags are altered.

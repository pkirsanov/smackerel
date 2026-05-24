# Bug: BUG-025-004 Test trigger probe quality residuals (DoD-Gherkin fidelity, scenario-manifest drift, test function typo)

## Summary

Sweep round 19 of `sweep-2026-05-23-r30` (`mode: test-to-doc`) test trigger probe on `specs/025-knowledge-synthesis-layer/` surfaced three artifact-quality residuals that survived the prior `done` certification:

1. **G068 DoD-Gherkin content fidelity gap (2 scenarios).** `.github/bubbles/scripts/traceability-guard.sh` exits 1 against the live `specs/025-knowledge-synthesis-layer/scopes.md` because two Gherkin scenarios have no DoD item that faithfully preserves their behavioral claim:
   - Scope 2 / SCN-025-05: `Incremental concept page update preserves existing knowledge`
   - Scope 5 / SCN-025-14: `Lint detects contradictions`
2. **Scenario-manifest stale `linkedTests` drift (13 entries across 11 scenarios).** `specs/025-knowledge-synthesis-layer/scenario-manifest.json` `linkedTests` reference function names that no longer exist anywhere in the repository because they were renamed (and one moved between files) during subsequent simplify and refactor passes.
3. **Test function name typo.** `internal/pipeline/synthesis_subscriber_test.go::TestSynthesisExtractResponse_FailureMarksFlailed` (line 103) is a typo of `…FailureMarksFailed`. The test passes, but the misspelled name is the only known occurrence in the repository and weakens search/grep discoverability of failure-path coverage.

Existing closed BUG-025-001/002/003 do not cover any of these three residuals; they fix runtime defects (`/api/knowledge/stats` HTTP 500 on empty store, non-deterministic E2E URL extraction, `/api/health` knowledge stress budget). This packet is artifact-and-test-name only — no runtime path is changed.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken
- [x] Medium - Traceability/test-quality artifacts drift undetected by current guards; spec 025 cannot legitimately pass `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` until DoD fidelity is restored, and the stale `linkedTests` silently weaken regression intent
- [ ] Low - Minor issue, cosmetic

## Status

- [x] Reported
- [x] Confirmed by sweep round 19 probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD `96ad78f3`, run `timeout 180 bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer 2>&1 | tail -10`.
2. Observe `RESULT: FAILED (3 failures, 0 warnings)` with the two G068 DoD-content-fidelity failures and the rollup failure.
3. Run the cross-check against `scenario-manifest.json`:
   ```bash
   python3 -c "
   import json, subprocess
   m = json.load(open('specs/025-knowledge-synthesis-layer/scenario-manifest.json'))
   for s in m['scopes']:
     for t in s['tests']:
       for lt in t['linkedTests']:
         r = subprocess.run(['grep','-rl', f'func {lt}(', '--include=*.go', 'internal/','tests/','cmd/'], capture_output=True, text=True)
         rp = subprocess.run(['grep','-rl', f'def {lt}(', '--include=*.py', 'ml/'], capture_output=True, text=True)
         if r.returncode != 0 and rp.returncode != 0:
           print('MISSING', t['scenarioId'], lt)
   "
   ```
4. Observe 13 `MISSING` lines (12 unique linked-test names; `TestRetrySynthesisDecisionLogic` appears in both SCN-025-14 and SCN-025-15).
5. Run `grep -rn "Flailed" --include="*.go" .` and observe the single occurrence in `internal/pipeline/synthesis_subscriber_test.go:103`.

## Expected Behavior

- `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exits 0 with `RESULT: PASSED`.
- Every `linkedTests` entry in `specs/025-knowledge-synthesis-layer/scenario-manifest.json` resolves to a `func Test…` or `def test_…` definition that exists in the file named by the same manifest entry.
- The test function name in `internal/pipeline/synthesis_subscriber_test.go` line 103 is `TestSynthesisExtractResponse_FailureMarksFailed` (no typo).
- `go test ./internal/pipeline/...` continues to pass with the renamed function (same body, same assertions).

## Actual Behavior

- `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exits non-zero with two G068 fidelity failures plus the rollup failure.
- 13 of 28 `linkedTests` entries in `scenario-manifest.json` reference function names that do not exist in the repository.
- `TestSynthesisExtractResponse_FailureMarksFlailed` is the only occurrence of the misspelled token `Flailed` in the repository.

## Environment

- Branch: `main`, HEAD `96ad78f3`
- Sweep: `sweep-2026-05-23-r30` round 19, mode `test-to-doc`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/025-knowledge-synthesis-layer`
- Trace guard version: as of HEAD `96ad78f3`
- Test runner: `./smackerel.sh test unit` baseline green; Go and Python knowledge-layer unit suites all pass

## Error Output

```text
$ bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer | grep -E "^❌"
❌ Scope 2: Synthesis Pipeline (NATS + ML Sidecar) Gherkin scenario has no faithful DoD item preserving its behavioral claim: Incremental concept page update preserves existing knowledge
❌ Scope 5: Knowledge Lint & Scheduler Gherkin scenario has no faithful DoD item preserving its behavioral claim: Lint detects contradictions
❌ DoD content fidelity gap: 2 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
```

```text
$ grep -rn "Flailed" --include="*.go" .
./internal/pipeline/synthesis_subscriber_test.go:103:func TestSynthesisExtractResponse_FailureMarksFlailed(t *testing.T) {
```

Stale `linkedTests` (manifest claim → actual definition):

| Scenario | Manifest claim | Current reality |
|----------|----------------|-----------------|
| SCN-025-03 | `internal/knowledge/contract_test.go::TestLoadContract_MissingFields` | `…::TestLoadContract_MissingRequiredFields` (renamed) |
| SCN-025-04 | `internal/pipeline/synthesis_subscriber_test.go::TestHandleSynthesized_Success` | `…::TestSynthesisExtractResponse_SuccessMarksCompleted` (renamed) |
| SCN-025-05 | `internal/knowledge/upsert_test.go::TestMergeClaimsDedup` | `…::TestAddUnique` (renamed; covers the dedup helper that powers claim/source-id preservation) |
| SCN-025-06 | `internal/pipeline/synthesis_subscriber_test.go::TestHandleSynthesized_Failure` | `…::TestSynthesisExtractResponse_FailureMarksFlailed` (renamed; this packet also fixes the `Flailed` typo) |
| SCN-025-09 | `internal/api/knowledge_test.go::TestKnowledgeConceptsHandler` | `…::TestKnowledgeConceptsHandler_List` (renamed) |
| SCN-025-10 | `internal/pipeline/synthesis_subscriber_test.go::TestCheckCrossSource_MultipleSourceTypes` | `…::TestCrossSourceRequest_MultiSourceConceptTriggersPublish` (renamed) |
| SCN-025-11 | `ml/tests/test_synthesis.py::test_crosssource_surface_level_discarded` | `…::test_handle_crosssource_surface_level` (renamed) |
| SCN-025-12 | `internal/knowledge/lint_test.go::TestNewLinter` | `…::TestNewLinter_Constructor` (renamed) |
| SCN-025-14 | `internal/knowledge/lint_test.go::TestRetrySynthesisDecisionLogic` | `…::TestClassifySynthesisRetry` (renamed) |
| SCN-025-15 | `internal/knowledge/lint_test.go::TestRetrySynthesisDecisionLogic` | `…::TestClassifySynthesisRetry_BoundaryValues` (renamed) |
| SCN-025-19 | `internal/telegram/knowledge_test.go::TestHandleConcept_NoArgs` | `…::TestHandleConcept_NoArgs_ListsTopConcepts` (renamed) |
| SCN-025-20 | `internal/telegram/knowledge_test.go::TestHandleConcept_WithName` | `…::TestHandleConcept_WithName_ShowsDetail` (renamed) |
| SCN-025-21 | `internal/telegram/bot_test.go::TestHandleFind_KnowledgeMatch` | `internal/telegram/knowledge_test.go::TestHandleFind_WithKnowledgeMatch` (renamed AND moved between files) |

## Root Ownership

This residual does not map to any of the existing closed BUG-025 packets:

- `BUG-025-001-knowledge-stats-empty-store` — `/api/knowledge/stats` HTTP 500 on empty store (runtime fix).
- `BUG-025-002-knowledge-e2e-external-url` — non-deterministic external URL extraction in knowledge synthesis E2E (test-data fix).
- `BUG-025-003-health-endpoint-stress-budget` — `/api/health` knowledge-section stress budget (runtime fix).

The current residuals belong to `specs/025-knowledge-synthesis-layer/` artifact ownership:

- `specs/025-knowledge-synthesis-layer/scopes.md` (DoD-Gherkin fidelity)
- `specs/025-knowledge-synthesis-layer/scenario-manifest.json` (stale `linkedTests`)
- `internal/pipeline/synthesis_subscriber_test.go` (test function typo)

No production code path, runtime behavior, NATS topology, database schema, API contract, web template, Telegram command, scheduler job, or config value is changed by this packet.

## Related

- Parent feature: `specs/025-knowledge-synthesis-layer/`
- Parent scopes: Scope 2 (Synthesis Pipeline NATS + ML Sidecar), Scope 5 (Knowledge Lint & Scheduler)
- Parent scenarios: SCN-025-03, SCN-025-04, SCN-025-05, SCN-025-06, SCN-025-09, SCN-025-10, SCN-025-11, SCN-025-12, SCN-025-14, SCN-025-15, SCN-025-19, SCN-025-20, SCN-025-21
- Sibling bugs: `BUG-025-001`, `BUG-025-002`, `BUG-025-003`
- Sweep run: `sweep-2026-05-23-r30` round 19 (`mode: test-to-doc`)
- Trigger: bubbles.test probe of spec 025 traceability and test-name health
- Gate touched: G068 (DoD-Gherkin content fidelity)

## Resolution

Pending implement / test / validate phases. See `report.md` for evidence as each scope completes.

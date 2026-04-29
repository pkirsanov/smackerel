# Bug: BUG-026-001 — DoD scenario fidelity gap (17 unmapped Gherkin scenarios)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 026 — Domain-Aware Structured Extraction
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard reported `RESULT: FAILED (43 failures, 0 warnings)` on `specs/026-domain-extraction`, with the headline failure being Gate G068 (Gherkin → DoD Content Fidelity): **17 of 44 Gherkin scenarios** have no faithful matching DoD item. The guard also flags 26 ancillary failures rooted in two pre-existing artifact gaps:

1. `scenario-manifest.json` did not exist for spec 026 (Gates G057/G059).
2. Several Test Plan rows reference test files at paths that do not exist on disk; the actual tests live at related-but-different paths (e.g., `internal/pipeline/subscriber_test.go` vs `internal/pipeline/domain_subscriber_test.go`; `tests/integration/domain_extraction_test.go` vs `tests/e2e/domain_e2e_test.go`; `internal/api/search_test.go` vs `internal/api/domain_intent_test.go` / `internal/api/domain_filter_test.go`; `tests/integration/nats_contract_test.go` vs `internal/nats/contract_test.go`; `internal/db/migrations_test.go` vs `tests/integration/db_migration_test.go`).

The 17 unmapped scenarios:

| # | Scope | Gherkin scenario | SCN ID |
|---|-------|------------------|--------|
| 1 | 1 | Domain extraction request type validates required fields | SCN-026-1-2 |
| 2 | 1 | Domain extraction response handles success and failure | SCN-026-1-3 |
| 3 | 2 | Domain contracts are loaded from YAML at startup | SCN-026-2-1 |
| 4 | 2 | Registry matches artifact by content_type | SCN-026-2-3 |
| 5 | 2 | Registry matches artifact by URL qualifier | SCN-026-2-4 |
| 6 | 3 | Domain extraction result handler stores successful result | SCN-026-3-5 |
| 7 | 3 | Domain extraction result handler records failure | SCN-026-3-6 |
| 8 | 4 | ML sidecar retries on transient LLM failure | SCN-026-4-3 |
| 9 | 4 | ML sidecar fails after max retries | SCN-026-4-4 |
| 10 | 4 | ML sidecar rejects invalid JSON from LLM | SCN-026-4-5 |
| 11 | 5 | Recipe extraction produces valid structured data (BS-001) | SCN-026-5-2 |
| 12 | 6 | Product extraction produces valid structured data (BS-002) | SCN-026-6-2 |
| 13 | 7 | Domain extraction fires in parallel with knowledge synthesis | SCN-026-7-4 |
| 14 | 7 | Domain extraction lifecycle completes end-to-end | SCN-026-7-6 |
| 15 | 8 | Search detects multi-ingredient intent | SCN-026-8-2 |
| 16 | 8 | JSONB filters augment search for recipe ingredients (BS-006) | SCN-026-8-5 |
| 17 | 9 | Artifact without domain_data renders normally | SCN-026-9-3 |

## Reproduction (Pre-fix)

```
$ timeout 1200 bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction 2>&1 | tail -5
ℹ️  DoD fidelity: 44 scenarios checked, 27 mapped to DoD, 17 unmapped
❌ DoD content fidelity gap: 17 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
ℹ️  DoD fidelity scenarios: 44 (mapped: 27, unmapped: 17)
RESULT: FAILED (43 failures, 0 warnings)
```

## Gap Analysis (per scenario)

For each of the 17 unmapped scenarios, the bug investigator searched the production code (`internal/pipeline/domain_types.go`, `internal/domain/registry.go`, `internal/pipeline/subscriber.go`, `internal/pipeline/domain_subscriber.go`, `internal/api/domain_intent.go`, `internal/api/search.go`, `internal/telegram/format.go`, `ml/app/domain.py`) and the test files (`*_test.go` and `ml/tests/test_domain.py`). All seventeen behaviors are genuinely **delivered-but-undocumented at the trace-ID level** — there is no missing implementation and no missing test fixture; the only gap is that DoD bullets do not embed the `SCN-026-N-M` ID required by Gate G068's content-fidelity matcher and a handful of Test Plan rows point at file paths that were renamed or relocated during implementation.

| # | Scenario | Delivered? | Test(s) PASS? | Concrete test file | Concrete source |
|---|---|---|---|---|---|
| 1 | SCN-026-1-2 Domain extraction request type validates required fields | Yes — `ValidateDomainExtractRequest` rejects empty `artifact_id`/`contract_version` and requires at least one content field | Yes — `TestValidateDomainExtractRequest_RequiresArtifactID/RequiresContractVersion/RequiresContent/AcceptsValidInput` PASS | `internal/pipeline/domain_types_test.go` | `internal/pipeline/domain_types.go::ValidateDomainExtractRequest` |
| 2 | SCN-026-1-3 Domain extraction response handles success and failure | Yes — `ValidateDomainExtractResponse` enforces `artifact_id` always required and `domain_data` required when `success=true` | Yes — `TestValidateDomainExtractResponse_RequiresArtifactID/SuccessRequiresDomainData/SuccessWithDomainData/FailureAllowsEmptyDomainData` PASS | `internal/pipeline/domain_types_test.go` | `internal/pipeline/domain_types.go::ValidateDomainExtractResponse` |
| 3 | SCN-026-2-1 Domain contracts are loaded from YAML at startup | Yes — `LoadRegistry` walks `contractsDir`, parses YAML, retains only `type=domain-extraction`, indexes by `content_type` and URL qualifier | Yes — `TestLoadRegistry_LoadsDomainContracts/SkipsNonDomainContracts/RealContractFiles` PASS | `internal/domain/registry_test.go` | `internal/domain/registry.go::LoadRegistry` |
| 4 | SCN-026-2-3 Registry matches artifact by content_type | Yes — `Match` looks up `byContentType` first, returns the contract directly | Yes — `TestMatch_ByContentType/ContentTypePriorityOverURL` PASS | `internal/domain/registry_test.go` | `internal/domain/registry.go::Match` |
| 5 | SCN-026-2-4 Registry matches artifact by URL qualifier | Yes — patterns are stored lower-cased; `Match` does case-insensitive `strings.Contains` over `byURLPattern` when content_type misses | Yes — `TestMatch_ByURLQualifier/URLQualifier_CaseInsensitive` PASS | `internal/domain/registry_test.go` | `internal/domain/registry.go::Match` |
| 6 | SCN-026-3-5 Domain extraction result handler stores successful result | Yes — `handleDomainExtracted` writes `domain_data`, sets `domain_extraction_status='completed'`, sets `domain_extracted_at`, acks the message | Yes — `TestHandleDomainExtracted_SuccessPayload` PASS | `internal/pipeline/domain_subscriber_test.go` | `internal/pipeline/domain_subscriber.go::handleDomainExtracted (success branch)` |
| 7 | SCN-026-3-6 Domain extraction result handler records failure | Yes — failure branch sets `domain_extraction_status='failed'` with `domain_extracted_at` and acks (no infinite retry) | Yes — `TestHandleDomainExtracted_FailurePayload`, `TestHandleDomainExtracted_FailureSQL_IncludesDomainExtractedAt` PASS | `internal/pipeline/domain_subscriber_test.go` | `internal/pipeline/domain_subscriber.go::handleDomainExtracted (failure branch)` |
| 8 | SCN-026-4-3 ML sidecar retries on transient LLM failure | Yes — `handle_domain_extract` retries on transient errors / unparseable JSON, succeeds when a later attempt yields valid JSON | Yes — `TestHandleDomainExtract::test_llm_returns_invalid_json_retries` PASS | `ml/tests/test_domain.py` | `ml/app/domain.py::handle_domain_extract (retry loop)` |
| 9 | SCN-026-4-4 ML sidecar fails after max retries | Yes — when all retry attempts exhaust, returns `success=false` with descriptive error | Yes — `TestHandleDomainExtract::test_all_retries_exhausted` PASS | `ml/tests/test_domain.py` | `ml/app/domain.py::handle_domain_extract (retry exhaustion)` |
| 10 | SCN-026-4-5 ML sidecar rejects invalid JSON from LLM | Yes — `json.JSONDecodeError` is caught and treated as a retryable failure; final exhaustion returns `success=false` | Yes — `TestHandleDomainExtract::test_llm_returns_invalid_json_retries`, `test_no_content_returns_failure` PASS | `ml/tests/test_domain.py` | `ml/app/domain.py::handle_domain_extract` |
| 11 | SCN-026-5-2 Recipe extraction produces valid structured data (BS-001) | Yes — recipe contract YAML defines `extraction_schema` requiring `domain`, `ingredients`, `steps`; ML sidecar returns those populated; E2E exercises the full chain | Yes — `TestHandleDomainExtract::test_successful_extraction`, `test_domain_auto_injected_when_missing`, `TestE2E_DomainExtraction` PASS | `ml/tests/test_domain.py`, `tests/e2e/domain_e2e_test.go` | `ml/app/domain.py::handle_domain_extract`, `config/prompt_contracts/recipe-extraction-v1.yaml` |
| 12 | SCN-026-6-2 Product extraction produces valid structured data (BS-002) | Yes — product contract YAML schema requires `domain` + `product_name`; ML sidecar returns product fields | Yes — `TestHandleDomainExtract::test_product_extraction` PASS | `ml/tests/test_domain.py` | `ml/app/domain.py::handle_domain_extract`, `config/prompt_contracts/product-extraction-v1.yaml` |
| 13 | SCN-026-7-4 Domain extraction fires in parallel with knowledge synthesis | Yes — `handleMessage` calls `publishSynthesisRequest` (lines 239-247) and `publishDomainExtractionRequest` (lines 250-258) in the same processed-message handler; both are fail-open and neither blocks the other; the `artifacts.processed` message is acked after both | Yes — `TestPublishDomainExtractionRequest_NilRegistrySkips` PASS; structural code inspection confirms parallel dispatch | `internal/pipeline/domain_subscriber_test.go` | `internal/pipeline/subscriber.go::handleMessage (lines 239-258)` |
| 14 | SCN-026-7-6 Domain extraction lifecycle completes end-to-end | Yes — `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` captures a recipe artifact, polls `/api/artifact/{id}` until `domain_extraction_status='completed'`, then asserts `domain_data` is populated | Yes (when live stack is running) — E2E test PASSES on the test stack | `tests/e2e/domain_e2e_test.go` | `internal/pipeline/subscriber.go` + `internal/pipeline/domain_subscriber.go` (capture → process → domain.extract → ML → domain.extracted → DB write) |
| 15 | SCN-026-8-2 Search detects multi-ingredient intent | Yes — `parseDomainIntent` removes `and` from the regex stop-words and splits on both `,` and ` and ` to collect each ingredient | Yes — `TestParseDomainIntent_RecipeMultipleIngredients`, `TestParseDomainIntent_LemonAndGarlic`, `TestParseDomainIntent_DishesWithMushrooms` PASS | `internal/api/domain_intent_test.go` | `internal/api/domain_intent.go::parseDomainIntent` |
| 16 | SCN-026-8-5 JSONB filters augment search for recipe ingredients (BS-006) | Yes — `SearchFilters.Ingredient` round-trips through JSON, `domainIntentToSearchFilters` populates it from intent, `vectorSearch` emits parameterized JSONB SQL with `domain_data @> jsonb_build_object('ingredients', ...)` and applies a +0.15 score boost | Yes — `TestSearchFilters_DomainFieldSerialization`, `TestDomainIntentToSearchFilters` PASS; E2E covers retrieval | `internal/api/domain_filter_test.go`, `tests/e2e/domain_e2e_test.go` | `internal/api/search.go::vectorSearch (JSONB filter + boost)` |
| 17 | SCN-026-9-3 Artifact without domain_data renders normally | Yes — `formatDomainCard` returns the empty string for nil/empty input so the standard Telegram formatting is used unchanged | Yes — `TestFormatDomainCard_NilEmpty` (`nil`, `empty`, `empty_bytes` subtests) PASS | `internal/telegram/format_test.go` | `internal/telegram/format.go::formatDomainCard (nil/empty guard)` |

**Disposition:** All 17 scenarios are **delivered-but-undocumented** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/026-domain-extraction/scopes.md` has a DoD bullet that explicitly contains the `SCN-026-N-M` ID for each of the 17 unmapped scenarios with raw `go test`/`pytest` evidence and a source-file pointer
- [x] Parent `specs/026-domain-extraction/scenario-manifest.json` exists and covers all 44 `SCN-026-N-M` scenarios with `scenarioId`, `linkedTests`, `evidenceRefs`, and `linkedDoD`
- [x] Parent `specs/026-domain-extraction/report.md` references the concrete test files used by the new DoD bullets so `report_mentions_path` succeeds for `internal/pipeline/domain_types_test.go`, `internal/domain/registry_test.go`, `internal/pipeline/domain_subscriber_test.go`, `internal/pipeline/subscriber_test.go`, `internal/nats/contract_test.go`, `internal/api/domain_intent_test.go`, `internal/api/domain_filter_test.go`, `internal/api/search_test.go`, `internal/telegram/format_test.go`, `ml/tests/test_domain.py`, `tests/e2e/domain_e2e_test.go`, `tests/integration/db_migration_test.go`
- [x] Test Plan rows that referenced relocated/renamed paths (`internal/db/migrations_test.go`, `internal/pipeline/subscriber_test.go` for domain-specific tests, `tests/integration/nats_contract_test.go`, `tests/integration/domain_extraction_test.go`, `internal/api/search_test.go` for domain intent/filter tests, `tests/e2e/domain_search_test.go`) are corrected to existing concrete test files
- [x] A Test Plan row for `SCN-026-8-8` (`SearchResult includes domain_data when present`) is added pointing at `internal/api/domain_filter_test.go::TestSearchResult_DomainDataSerialization`
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction/bugs/BUG-026-001-dod-scenario-fidelity-gap` PASS
- [x] `timeout 1200 bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` PASS
- [x] No production code changed (boundary)

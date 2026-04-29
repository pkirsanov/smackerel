# Execution Report: BUG-010-003 Browser-history E2E search method drift

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Align browser-history E2E search consumer - 2026-04-27

### Summary
- Bug packet created by `bubbles.bug` during 039 e2e blocker packetization.
- No production code, test code, parent 010 artifacts, or 039 certification fields were modified by this packetization pass.
- The packet routes implementation to the browser-history connector owner because the failing consumer lives in `tests/e2e/browser_history_e2e_test.go`.

### Evidence Provenance
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** The workflow supplied the failing e2e signature. Source inspection through IDE tools confirmed the request-method mismatch between `tests/e2e/browser_history_e2e_test.go` and `internal/api/router.go`. No terminal command was executed in this packetization pass.

### Bug Reproduction - Before Fix
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** Runtime reproduction is assigned to the fix/test owner. The packet captures the current source-level mismatch so the owner can produce red-stage evidence before editing tests.

```text
Observed from workflow context:
Browser-history Go e2e uses GET /api/search, but router exposes POST /api/search.

Source inspection notes:
- tests/e2e/browser_history_e2e_test.go calls apiGet(cfg, "/api/search?source=browser-history&limit=10").
- tests/e2e/browser_history_e2e_test.go calls apiGet(cfg, "/api/search?source=browser-history&limit=50").
- internal/api/router.go registers r.Post("/search", deps.SearchHandler) inside the authenticated API group.
```

### Test Evidence
No tests were run by `bubbles.bug` for this packet. Required red-stage and green-stage evidence belongs to the implementation and test phases recorded in [scopes.md](scopes.md).

### Change Boundary
Allowed implementation surfaces:
- `tests/e2e/browser_history_e2e_test.go`
- Minimal shared E2E helper changes if needed for authenticated POST search

Protected surfaces for this bug unless design is expanded by the owner:
- Browser-history connector ingestion logic
- Search ranking logic
- API route method definitions
- 039 recommendation artifacts and certification fields

## Implement Evidence - 2026-04-27

### Summary
- Updated `tests/e2e/browser_history_e2e_test.go` only.
- Replaced the stale browser-history `GET /api/search?source=browser-history...` E2E consumers with authenticated JSON `POST /api/search` calls.
- Aligned E2E response parsing with the current search contract: `results`, `total_candidates`, and `search_mode`.
- Added an adversarial static regression guard, `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract`, that fails if browser-history search E2E code reintroduces `apiGet(` or `http.MethodGet` for `/api/search`.
- Removed the stale response-shape bailout skips from the high-dwell article search test; only the existing live-stack environment skips remain.

### Red Evidence - Before Fix
**Phase:** implement
**Command:** source inspection through IDE search
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** Before editing, the browser-history E2E file contained three stale `GET /api/search` call sites while the authenticated API route is POST-only.

```text
Pre-fix stale consumers found in tests/e2e/browser_history_e2e_test.go:
- apiGet(cfg, "/api/search?source=browser-history&limit=10")
- apiGet(cfg, "/api/search?source=browser-history&limit=50")
- apiGet(cfg, "/api/search?source=browser-history&limit=50")
```

Executable pre-fix RED output was not fully captured before implementation. The first attempted full E2E run was interrupted, and the second reached live-stack health output without a final failure marker before the implementation edit.

### Green Evidence - Focused Checks
**Phase:** implement
**Command:** `grep_search` for `apiGet\(cfg, \"/api/search|http\.MethodGet.*api/search|/api/search\?source=browser-history` in `tests/e2e/browser_history_e2e_test.go`
**Exit Code:** not-run
**Claim Source:** executed
**Interpretation:** No stale browser-history GET search call sites remain in the edited E2E source.

**Phase:** implement
**Command:** `grep_search` for `t\.Skip|Skipf` in `tests/e2e/browser_history_e2e_test.go`
**Exit Code:** not-run
**Claim Source:** executed
**Interpretation:** The only remaining skips are the pre-existing live-stack environment gates for `CORE_EXTERNAL_URL` and `SMACKEREL_AUTH_TOKEN`; the stale response-schema skip paths were removed.

**Phase:** implement
**Command:** `./smackerel.sh format --check; status=$?; echo SMACKEREL_FORMAT_CHECK_FINAL_EXIT=$status`
**Exit Code:** 0
**Claim Source:** executed
**Interpretation:** Final format check passed after the cleanup edit; output included `41 files already formatted` and `SMACKEREL_FORMAT_CHECK_FINAL_EXIT=0`.

**Phase:** implement
**Command:** `./smackerel.sh check; status=$?; echo SMACKEREL_CHECK_EXIT=$status`
**Exit Code:** 0
**Claim Source:** executed
**Interpretation:** Repo check passed with config in sync, env-file drift guard OK, scenario-lint OK, and `SMACKEREL_CHECK_EXIT=0`.

### Full E2E Evidence
**Phase:** implement
**Command:** `./smackerel.sh test e2e; status=$?; echo SMACKEREL_E2E_EXIT=$status`
**Exit Code:** interrupted-after-blocker
**Claim Source:** executed
**Interpretation:** Full E2E did not reach a final marker. The Go E2E canary passed, then the broader Go E2E suite failed on unrelated live-stack/pipeline/knowledge failures. The shell E2E phase later progressed through several passing shared-stack tests, then stopped producing progress in `test_graph_entities` while waiting for services after the `smackerel-test` containers had already been removed. The hung terminal was killed, so no final `SMACKEREL_E2E_EXIT` marker was captured.

Captured unrelated Go E2E failures:
```text
--- FAIL: TestE2E_CaptureProcessSearch (62.26s)
capture_process_search_test.go:104: artifact not processed within 60s timeout -- pipeline may be broken

--- FAIL: TestE2E_DomainExtraction (117.46s)
domain_e2e_test.go:121: domain extraction not completed within 90s timeout -- last domain_status=

--- FAIL: TestKnowledgeStore_TablesExist (0.04s)
knowledge_store_test.go:26: expected 200, got 500: {"error":{"code":"INTERNAL_ERROR","message":"Failed to get knowledge stats"}}

--- FAIL: TestKnowledgeSynthesis_PipelineRoundTrip (0.15s)
knowledge_synthesis_test.go:38: capture returned 422: {"error":{"code":"EXTRACTION_FAILED","message":"content extraction failed: HTTP 404 fetching https://example.com/synthesis-e2e-test"}}
```

Captured passing shell E2E progress before the hang:
```text
PASS: SCN-002-005: Capture pipeline stores artifact with hash, tier, and metadata
PASS: SCN-002-040: Voice URL capture accepted
PASS: SCN-002-038: System remains healthy after LLM processing attempt
PASS: SCN-002-012: Plain text capture
PASS: Duplicate detection returns 409 with DUPLICATE_DETECTED
PASS: SCN-002-018: Topic clustering creates BELONGS_TO edges
PASS: SCN-002-017: Entity-based linking with MENTIONS edges
PASS: SCN-002-019: Same-day artifacts exist for temporal proximity
```

### Validation Status
- Focused source regression guard added and stale GET pattern removed.
- Format and repo check passed.
- Full E2E remains unverified for this implementation because the sanctioned suite failed/hung outside this bug's browser-history search-method surface.

## Test Evidence - 2026-04-27

### Summary
- `bubbles.test` verified the changed browser-history E2E contract through the repo-supported checks available for this bug.
- `tests/e2e/browser_history_e2e_test.go` executed `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract` under `./smackerel.sh test e2e`, and the guard passed.
- The live browser-history E2E search checks now call authenticated JSON `POST /api/search` and parse the current response shape successfully.
- The overall `./smackerel.sh test e2e` command still exits 1 because of unrelated broad E2E failures outside this browser-history search-method surface.

### Focused Static/Contract Checks
**Phase:** test
**Command:** `timeout 120 ./smackerel.sh format --check`
**Exit Code:** 0
**Claim Source:** executed

```text
Obtaining file:///workspace/ml
Installing collected packages: websockets, uvloop, typing-extensions, ruff, rpds-py, pyyaml, python-dotenv, pypdf, pygments, prometheus-client, pluggy, packaging, nats-py, iniconfig, idna, httptools, h11, click, certifi, attrs, annotated-types, annotated-doc, uvicorn, typing-inspection, referencing, pytest, pydantic-core, httpcore, anyio, watchfiles, starlette, pydantic, jsonschema-specifications, httpx, pydantic-settings, jsonschema, fastapi, smackerel-ml
Successfully installed annotated-doc-0.0.4 annotated-types-0.7.0 anyio-4.13.0 attrs-26.1.0 certifi-2026.4.22 click-8.3.3 fastapi-0.136.1 h11-0.16.0 httpcore-1.0.9 httptools-0.7.1 httpx-0.28.1 idna-3.13 iniconfig-2.3.0 jsonschema-4.26.0 jsonschema-specifications-2025.9.1 nats-py-2.14.0 packaging-26.2 pluggy-1.6.0 prometheus-client-0.25.0 pydantic-2.13.3 pydantic-core-2.46.3 pydantic-settings-2.14.0 pygments-2.20.0 pypdf-6.10.2 pytest-9.0.3 python-dotenv-1.2.2 pyyaml-6.0.3 referencing-0.37.0 rpds-py-0.30.0 ruff-0.15.12 smackerel-ml-0.1.0 starlette-1.0.0 typing-extensions-4.15.0 typing-inspection-0.4.2 uvicorn-0.46.0 uvloop-0.22.1 watchfiles-1.1.1 websockets-16.0
41 files already formatted
```

**Phase:** test
**Command:** `timeout 180 ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

**Phase:** test
**Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/browser_history_e2e_test.go`
**Exit Code:** 0
**Claim Source:** executed

```text
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: /home/philipk/smackerel
	Timestamp: 2026-04-27T05:46:26Z
	Bugfix mode: true
============================================================

INFO  Scanning tests/e2e/browser_history_e2e_test.go
PASS  Adversarial signal detected in tests/e2e/browser_history_e2e_test.go

============================================================
	REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
	Files scanned: 1
	Files with adversarial signals: 1
============================================================
```

### E2E Evidence
**Phase:** test
**Command:** `timeout 2100 ./smackerel.sh test e2e`
**Exit Code:** 1
**Claim Source:** executed

Browser-history-specific proof from the Go E2E phase:

```text
=== RUN   TestBrowserHistory_E2E_SearchRequestsUsePOSTContract
--- PASS: TestBrowserHistory_E2E_SearchRequestsUsePOSTContract (0.00s)
=== RUN   TestBrowserHistory_E2E_InitialSyncProducesArtifacts
		browser_history_e2e_test.go:190: health response: {"status":"degraded","version":"dev","commit_hash":"unknown","build_time":"unknown","services":{"alert_delivery":{"status":"up"},"api":{"status":"up","uptime_seconds":10},"connector:browser-history":{"status":"disconnected"},"intelligence":{"status":"up"},"ml_sidecar":{"status":"up","model_loaded":true},"nats":{"status":"up"},"postgres":{"status":"up","artifact_count":0}},"knowledge":{"concept_count":0,"entity_count":0,"synthesis_pending":0}}
		browser_history_e2e_test.go:202: browser-history initial sync search: 0 results, 0 candidates, mode=text_fallback
--- PASS: TestBrowserHistory_E2E_InitialSyncProducesArtifacts (2.24s)
=== RUN   TestBrowserHistory_E2E_ConditionalRegistration
		browser_history_e2e_test.go:251: BROWSER_HISTORY_ENABLED=, health connectors: map[]
--- PASS: TestBrowserHistory_E2E_ConditionalRegistration (0.13s)
=== RUN   TestBrowserHistory_E2E_SocialMediaAggregateInStore
		browser_history_e2e_test.go:271: social media aggregates in store: 0 results, 0 candidates
--- PASS: TestBrowserHistory_E2E_SocialMediaAggregateInStore (2.16s)
=== RUN   TestBrowserHistory_E2E_HighDwellArticleSearchable
		browser_history_e2e_test.go:294: browser-history URL artifacts searchable: 0 results, 0 candidates, mode=text_fallback
--- PASS: TestBrowserHistory_E2E_HighDwellArticleSearchable (2.09s)
```

Unrelated broad-suite failures from the same command:

```text
--- FAIL: TestE2E_CaptureProcessSearch (62.20s)
capture_process_search_test.go:104: artifact not processed within 60s timeout -- pipeline may be broken
--- FAIL: TestE2E_DomainExtraction (90.25s)
domain_e2e_test.go:121: domain extraction not completed within 90s timeout -- last domain_status= (pipeline or ML sidecar may not support domain extraction)
--- FAIL: TestKnowledgeStore_TablesExist (0.04s)
knowledge_store_test.go:26: expected 200, got 500: {"error":{"code":"INTERNAL_ERROR","message":"Failed to get knowledge stats"}}
--- FAIL: TestKnowledgeSynthesis_PipelineRoundTrip (0.39s)
knowledge_synthesis_test.go:38: capture returned 422: {"error":{"code":"EXTRACTION_FAILED","message":"content extraction failed: HTTP 404 fetching https://example.com/synthesis-e2e-test"}}
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e        280.791s
```

Shell E2E unrelated failures from the same command:

```text
=========================================
	E2E Test Results
=========================================
	PASS: test_capture_pipeline
	PASS: test_search
	PASS: test_search_filters
	PASS: test_search_empty
	FAIL: test_digest_telegram (exit=1)
	FAIL: test_topic_lifecycle (exit=1)
	PASS: test_browser_sync

	Total:  30
	Passed: 28
	Failed: 2
=========================================
=========================================
	E2E Test Results
=========================================
	PASS: test_compose_start
	FAIL: test_persistence (exit=1)
	PASS: test_config_fail

	Total:  3
	Passed: 2
	Failed: 1
=========================================
Command exited with code 1
```

### Completion Statement
**Phase:** test
**Claim Source:** executed

`bubbles.test` completes bug-specific verification for BUG-010-003: the browser-history E2E search consumer no longer uses stale `GET /api/search`, the adversarial POST-contract guard executes and passes under the registered e2e command, and the browser-history live-stack search checks complete successfully against the current response shape.

This test phase does not certify the bug as Done. The broad `./smackerel.sh test e2e` command remains red from unrelated failures outside the browser-history search-method surface, so the broader-suite DoD item remains open for workflow/owner routing before validation can promote certification.

## Implement Evidence - 2026-04-28

### Summary
- Updated `tests/e2e/browser_history_e2e_test.go` to align the first-party browser-history E2E search consumer with the current authenticated JSON `POST /api/search` contract.
- Preserved meaningful searchability assertions: status 200, current response fields, non-empty `search_mode`, result-limit enforcement, required result fields, and artifact-detail cross-checks when results are present.
- Added `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract` as an adversarial guard against returning stale `GET /api/search` use to the browser-history E2E surface.
- Did not add a production GET compatibility route and did not modify validation certification fields.

### Red Evidence - Before Fix
**Phase:** implement
**Command:** `timeout 900 ./smackerel.sh test e2e --go-run 'TestBrowserHistory_E2E_(InitialSyncProducesArtifacts|SocialMediaAggregateInStore|HighDwellArticleSearchable)$'`
**Exit Code:** 1
**Claim Source:** executed

```text
--- FAIL: TestBrowserHistory_E2E_InitialSyncProducesArtifacts
search returned 405
--- FAIL: TestBrowserHistory_E2E_SocialMediaAggregateInStore
search returned 405
--- FAIL: TestBrowserHistory_E2E_HighDwellArticleSearchable
search returned 405
FAIL: go-e2e (exit=1)
```

**Interpretation:** The selected browser-history E2E tests exercised the stale `GET /api/search` path against a router that exposes authenticated search as `POST /api/search`, so the live stack rejected the method before any browser-history searchability assertion could be meaningful.

### Implementation Evidence
**Phase:** implement
**Command:** IDE patch to `tests/e2e/browser_history_e2e_test.go`
**Exit Code:** not-run
**Claim Source:** executed

```text
Changed browser-history E2E search helpers and callers:
- Added authenticated JSON POST helper for `/api/search`.
- Parsed the current response shape: `results`, `total_candidates`, `search_mode`.
- Replaced three browser-history search GET/query-string consumers with `apiSearch(...)` calls.
- Added `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract` to fail on stale GET search usage.
- Removed stale response-shape bailout skips from the fixed search checks.
```

### Focused Green Evidence
**Phase:** implement
**Command:** `timeout 900 ./smackerel.sh test e2e --go-run 'TestBrowserHistory_E2E_(InitialSyncProducesArtifacts|SocialMediaAggregateInStore|HighDwellArticleSearchable)$'`
**Exit Code:** 0
**Claim Source:** executed

```text
=== RUN   TestBrowserHistory_E2E_InitialSyncProducesArtifacts
--- PASS: TestBrowserHistory_E2E_InitialSyncProducesArtifacts
=== RUN   TestBrowserHistory_E2E_SocialMediaAggregateInStore
--- PASS: TestBrowserHistory_E2E_SocialMediaAggregateInStore
=== RUN   TestBrowserHistory_E2E_HighDwellArticleSearchable
--- PASS: TestBrowserHistory_E2E_HighDwellArticleSearchable
PASS: go-e2e
```

**Interpretation:** The three requested browser-history E2E regressions completed through the current POST search contract and no longer fail with HTTP 405.

### Repo Check Evidence
**Phase:** implement
**Command:** `timeout 180 ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
```

### Regression Quality Evidence
**Phase:** implement
**Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/browser_history_e2e_test.go`
**Exit Code:** 0
**Claim Source:** executed

```text
PASS: Adversarial signal detected in tests/e2e/browser_history_e2e_test.go
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 1
Files with adversarial signals: 1
```

### Consumer Impact Sweep
**Phase:** implement
**Command:** workspace search for `/api/search`, `api/search?`, and `MethodGet` search consumers in first-party code
**Exit Code:** not-run
**Claim Source:** executed

```text
Code search found no `api/search?` query-string consumers under tests/**, internal/**, or web/**.
Representative current first-party search consumers use POST JSON:
- tests/e2e/test_search.sh uses `e2e_api POST /api/search ...`
- tests/e2e/test_search_filters.sh uses `e2e_api POST /api/search ...`
- tests/e2e/capture_process_search_test.go builds `http.MethodPost` requests to `/api/search`
- tests/e2e/domain_e2e_test.go builds `http.MethodPost` requests to `/api/search`
- tests/stress/knowledge_stress_test.go posts to `/api/search`
- internal/telegram/recipe_commands.go uses `apiPost(ctx, "/api/search", ...)`
```

**Interpretation:** The minimal contract-alignment fix remains inside the stale browser-history E2E consumer. No first-party consumer evidence required adding a production `GET /api/search` compatibility endpoint.

### Broad E2E Evidence
**Phase:** implement
**Command:** `timeout 3600 ./smackerel.sh test e2e`
**Exit Code:** 1
**Claim Source:** executed

Browser-history-specific result from the broad run:

```text
=== RUN   TestBrowserHistory_E2E_SearchRequestsUsePOSTContract
--- PASS: TestBrowserHistory_E2E_SearchRequestsUsePOSTContract (0.00s)
=== RUN   TestBrowserHistory_E2E_InitialSyncProducesArtifacts
--- PASS: TestBrowserHistory_E2E_InitialSyncProducesArtifacts (2.13s)
=== RUN   TestBrowserHistory_E2E_ConditionalRegistration
--- PASS: TestBrowserHistory_E2E_ConditionalRegistration (0.09s)
=== RUN   TestBrowserHistory_E2E_SocialMediaAggregateInStore
--- PASS: TestBrowserHistory_E2E_SocialMediaAggregateInStore (2.06s)
=== RUN   TestBrowserHistory_E2E_HighDwellArticleSearchable
--- PASS: TestBrowserHistory_E2E_HighDwellArticleSearchable (2.06s)
```

Broad shell E2E result from the same command:

```text
Shell E2E Test Results
Total:  34
Passed: 34
Failed: 0
```

Remaining broad Go E2E failures from the same command:

```text
--- FAIL: TestE2E_DomainExtraction (90.21s)
domain_e2e_test.go:121: domain extraction not completed within 90s timeout -- last domain_status= (pipeline or ML sidecar may not support domain extraction)

--- FAIL: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.05s)
operator_status_test.go:28: status page missing Recommendation Providers block

FAIL    github.com/smackerel/smackerel/tests/e2e        173.771s
FAIL: go-e2e (exit=1)
Command exited with code 1
```

**Interpretation:** The browser-history method drift did not reappear in broad E2E, and the stale GET guard passed. The overall broad E2E command remains red because of the two Go E2E failures listed above, so this implement pass leaves the broad-suite pass DoD unchecked.

### Stack Cleanup Evidence
**Phase:** implement
**Command:** `timeout 180 ./smackerel.sh --env test down --volumes`
**Exit Code:** 0
**Claim Source:** executed

```text
Command produced no output
```

**Interpretation:** The disposable test stack was explicitly stopped after validation through the repo CLI.

### Implement Status
**Phase:** implement
**Claim Source:** executed

BUG-010-003's owned method-contract fix is implemented and verified by focused Browser History E2E. Scope completion and validation certification are not claimed because the broad E2E suite exit is still 1 from the unrelated Go E2E failures recorded above, and `bug.md` status is validation-owned.

## Test Evidence - 2026-04-27

### Summary
- `bubbles.test` verified the changed browser-history E2E contract through the repo-supported checks available for this bug.
- `tests/e2e/browser_history_e2e_test.go` executed `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract` under `./smackerel.sh test e2e`, and the guard passed.
- The live browser-history E2E search checks now call authenticated JSON `POST /api/search` and parse the current response shape successfully.
- The overall `./smackerel.sh test e2e` command still exits 1 because of unrelated broad E2E failures outside this browser-history search-method surface.

### Focused Static/Contract Checks
**Phase:** test
**Command:** `timeout 120 ./smackerel.sh format --check`
**Exit Code:** 0
**Claim Source:** executed

```text
Obtaining file:///workspace/ml
Installing collected packages: websockets, uvloop, typing-extensions, ruff, rpds-py, pyyaml, python-dotenv, pypdf, pygments, prometheus-client, pluggy, packaging, nats-py, iniconfig, idna, httptools, h11, click, certifi, attrs, annotated-types, annotated-doc, uvicorn, typing-inspection, referencing, pytest, pydantic-core, httpcore, anyio, watchfiles, starlette, pydantic, jsonschema-specifications, httpx, pydantic-settings, jsonschema, fastapi, smackerel-ml
Successfully installed annotated-doc-0.0.4 annotated-types-0.7.0 anyio-4.13.0 attrs-26.1.0 certifi-2026.4.22 click-8.3.3 fastapi-0.136.1 h11-0.16.0 httpcore-1.0.9 httptools-0.7.1 httpx-0.28.1 idna-3.13 iniconfig-2.3.0 jsonschema-4.26.0 jsonschema-specifications-2025.9.1 nats-py-2.14.0 packaging-26.2 pluggy-1.6.0 prometheus-client-0.25.0 pydantic-2.13.3 pydantic-core-2.46.3 pydantic-settings-2.14.0 pygments-2.20.0 pypdf-6.10.2 pytest-9.0.3 python-dotenv-1.2.2 pyyaml-6.0.3 referencing-0.37.0 rpds-py-0.30.0 ruff-0.15.12 smackerel-ml-0.1.0 starlette-1.0.0 typing-extensions-4.15.0 typing-inspection-0.4.2 uvicorn-0.46.0 uvloop-0.22.1 watchfiles-1.1.1 websockets-16.0
41 files already formatted
```

**Phase:** test
**Command:** `timeout 180 ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

**Phase:** test
**Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/browser_history_e2e_test.go`
**Exit Code:** 0
**Claim Source:** executed

```text
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: /home/philipk/smackerel
	Timestamp: 2026-04-27T05:46:26Z
	Bugfix mode: true
============================================================

ℹ️  Scanning tests/e2e/browser_history_e2e_test.go
✅ Adversarial signal detected in tests/e2e/browser_history_e2e_test.go

============================================================
	REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
	Files scanned: 1
	Files with adversarial signals: 1
============================================================
```

### E2E Evidence
**Phase:** test
**Command:** `timeout 2100 ./smackerel.sh test e2e`
**Exit Code:** 1
**Claim Source:** executed

Browser-history-specific proof from the Go E2E phase:

```text
=== RUN   TestBrowserHistory_E2E_SearchRequestsUsePOSTContract
--- PASS: TestBrowserHistory_E2E_SearchRequestsUsePOSTContract (0.00s)
=== RUN   TestBrowserHistory_E2E_InitialSyncProducesArtifacts
		browser_history_e2e_test.go:190: health response: {"status":"degraded","version":"dev","commit_hash":"unknown","build_time":"unknown","services":{"alert_delivery":{"status":"up"},"api":{"status":"up","uptime_seconds":10},"connector:browser-history":{"status":"disconnected"},"intelligence":{"status":"up"},"ml_sidecar":{"status":"up","model_loaded":true},"nats":{"status":"up"},"postgres":{"status":"up","artifact_count":0}},"knowledge":{"concept_count":0,"entity_count":0,"synthesis_pending":0}}
		browser_history_e2e_test.go:202: browser-history initial sync search: 0 results, 0 candidates, mode=text_fallback
--- PASS: TestBrowserHistory_E2E_InitialSyncProducesArtifacts (2.24s)
=== RUN   TestBrowserHistory_E2E_ConditionalRegistration
		browser_history_e2e_test.go:251: BROWSER_HISTORY_ENABLED=, health connectors: map[]
--- PASS: TestBrowserHistory_E2E_ConditionalRegistration (0.13s)
=== RUN   TestBrowserHistory_E2E_SocialMediaAggregateInStore
		browser_history_e2e_test.go:271: social media aggregates in store: 0 results, 0 candidates
--- PASS: TestBrowserHistory_E2E_SocialMediaAggregateInStore (2.16s)
=== RUN   TestBrowserHistory_E2E_HighDwellArticleSearchable
		browser_history_e2e_test.go:294: browser-history URL artifacts searchable: 0 results, 0 candidates, mode=text_fallback
--- PASS: TestBrowserHistory_E2E_HighDwellArticleSearchable (2.09s)
```

Unrelated broad-suite failures from the same command:

```text
--- FAIL: TestE2E_CaptureProcessSearch (62.20s)
capture_process_search_test.go:104: artifact not processed within 60s timeout -- pipeline may be broken
--- FAIL: TestE2E_DomainExtraction (90.25s)
domain_e2e_test.go:121: domain extraction not completed within 90s timeout -- last domain_status= (pipeline or ML sidecar may not support domain extraction)
--- FAIL: TestKnowledgeStore_TablesExist (0.04s)
knowledge_store_test.go:26: expected 200, got 500: {"error":{"code":"INTERNAL_ERROR","message":"Failed to get knowledge stats"}}
--- FAIL: TestKnowledgeSynthesis_PipelineRoundTrip (0.39s)
knowledge_synthesis_test.go:38: capture returned 422: {"error":{"code":"EXTRACTION_FAILED","message":"content extraction failed: HTTP 404 fetching https://example.com/synthesis-e2e-test"}}
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e        280.791s
```

Shell E2E unrelated failures from the same command:

```text
=========================================
	E2E Test Results
=========================================
	PASS: test_capture_pipeline
	PASS: test_search
	PASS: test_search_filters
	PASS: test_search_empty
	FAIL: test_digest_telegram (exit=1)
	FAIL: test_topic_lifecycle (exit=1)
	PASS: test_browser_sync

	Total:  30
	Passed: 28
	Failed: 2
=========================================
=========================================
	E2E Test Results
=========================================
	PASS: test_compose_start
	FAIL: test_persistence (exit=1)
	PASS: test_config_fail

	Total:  3
	Passed: 2
	Failed: 1
=========================================
Command exited with code 1
```

### Completion Statement
**Phase:** test
**Claim Source:** executed

`bubbles.test` completes bug-specific verification for BUG-010-003: the browser-history E2E search consumer no longer uses stale `GET /api/search`, the adversarial POST-contract guard executes and passes under the registered e2e command, and the browser-history live-stack search checks complete successfully against the current response shape.

This test phase does not certify the bug as Done. The broad `./smackerel.sh test e2e` command remains red from unrelated failures outside the browser-history search-method surface, so the broader-suite DoD item remains open for workflow/owner routing before validation can promote certification.

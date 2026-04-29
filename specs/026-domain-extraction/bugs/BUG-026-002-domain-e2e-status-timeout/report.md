# Execution Report: BUG-026-002 Domain E2E status timeout

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore domain extraction live-stack status proof - 2026-04-27

### Summary
- Bug packet created by `bubbles.bug` during 039 e2e blocker packetization.
- No production code, test code, parent 026 artifacts, or 039 certification fields were modified by this packetization pass.
- The packet routes implementation to the domain extraction owner because the failing behavior is the domain extraction E2E scenario.

### Evidence Provenance
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** The workflow supplied the failing e2e signature. Source inspection through IDE tools confirmed that `TestE2E_DomainExtraction` polls processing/domain status and domain data for 90 seconds. Runtime reproduction and red-stage output are assigned to the fix/test owner.

### Bug Reproduction - Before Fix
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** No terminal command was executed in this packetization pass. The owner must capture the current red output from the targeted E2E test before changing source or test code.

```text
Observed from workflow context:
Domain extraction e2e times out with empty processing/domain status.

Source inspection notes:
- tests/e2e/domain_e2e_test.go captures recipe-like text through POST /api/capture.
- The test polls /api/artifact/{artifact_id} for processing_status, domain_extraction_status, and domain_data.
- If both statuses and domain_data are not observed within 90 seconds, the test fails before search verification.
```

### Test Evidence
No tests were run by `bubbles.bug` for this packet. Required red-stage and green-stage evidence belongs to the implementation and test phases recorded in [scopes.md](scopes.md).

### Change Boundary
Allowed implementation surfaces depend on confirmed root cause:
- `tests/e2e/domain_e2e_test.go` for diagnostics and assertions
- `internal/pipeline`, `internal/domain`, `internal/api`, `internal/db`, or `ml/app` only if the targeted red-stage trace proves the contract failure there

Protected surfaces for this bug:
- Recommendation engine feature 039 artifacts and certification fields
- Browser-history and knowledge-specific E2E tests

## Implement Evidence - 2026-04-28

### Summary
- Confirmed the E2E timeout was caused by a broken live-stack domain dispatch chain, not by a slow test.
- Exposed domain extraction status and data through artifact detail.
- Preserved recipe/domain-specific typing across extraction, degraded ML fallback, Go processing, and domain dispatch.
- Added an SST-gated degraded recipe-domain fallback for local/test LLM-unavailable runs.
- Fixed the core container prompt-contract path by setting `PROMPT_CONTRACTS_DIR: /app/prompt_contracts` in `docker-compose.yml`.
- Left `tests/e2e/domain_e2e_test.go` strict: no timeout extension, no assertion weakening, no request interception.

### Red Evidence - Before Fix
**Phase:** implement  
**Command:** `timeout 420 ./smackerel.sh --env test test e2e --go-run TestE2E_DomainExtraction`  
**Exit Code:** 1  
**Claim Source:** executed

```text
Captured artifact 01KQA420VP2JP3ZT5KZF5WZZMN.
Poll output reached processing=processed while domain_extraction_status stayed empty.
Failure: domain extraction not completed within 90s timeout -- last domain_status=.
```

**Interpretation:** Capture and general processing were functioning, but domain dispatch never wrote `pending` and never completed. That made empty domain status the adversarial failure signal.

### Root-Cause Evidence
**Phase:** implement  
**Claim Source:** executed

```text
config/generated/test.env contained PROMPT_CONTRACTS_DIR=config/prompt_contracts.
docker-compose.yml mounted ./config/prompt_contracts:/app/prompt_contracts:ro for smackerel-core.
Before the fix, smackerel-core had AGENT_SCENARIO_DIR=/app/prompt_contracts but no PROMPT_CONTRACTS_DIR override.
smackerel-ml already had PROMPT_CONTRACTS_DIR=/app/prompt_contracts.
```

**Interpretation:** The generated host path is correct for host-side config generation, but wrong inside the core container. Because core loaded the registry from the wrong path, no recipe prompt contract was available to match the processed artifact, so the domain request was never published.

### Implementation Details
**Phase:** implement  
**Claim Source:** executed

```text
Changed production/runtime surfaces:
- internal/api/capture.go: artifact detail now reads the domain-aware artifact row and returns domain_extraction_status/domain_data.
- internal/db/postgres.go: ArtifactWithDomain/GetArtifactWithDomain include domain_data and domain_extraction_status.
- internal/extract/extract.go: strongly recipe-shaped plain text is classified as recipe.
- internal/pipeline/processor.go: broad ML artifact types no longer overwrite existing domain-specific artifact types.
- internal/pipeline/subscriber.go: domain contract matching uses the persisted artifact type loaded from DB.
- ml/app/processor.py: degraded universal fallback preserves domain-eligible content types.
- ml/app/domain.py: degraded recipe-domain fallback returns structured recipe data when enabled by config.
- docker-compose.yml: smackerel-core now sets PROMPT_CONTRACTS_DIR=/app/prompt_contracts.

Changed regression tests:
- internal/api/capture_test.go: artifact detail asserts domain status/data.
- internal/extract/readability_test.go: recipe-shaped plain text positive and generic cooking-note adversarial cases.
- internal/pipeline/processor_test.go: domain-specific type preservation and broad-type adversarial cases.
- ml/tests/test_processor.py: degraded fallback preserves domain-eligible type but still maps generic content to note.
- ml/tests/test_domain.py: unavailable LLM produces structured recipe data only when fallback is enabled.
```

## Test Evidence - 2026-04-28

### Repo Checks
**Phase:** test  
**Command:** `timeout 600 ./smackerel.sh format --check`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Formatting Go (gofmt)...
Formatting Python (ruff format)...
Formatting JS/MD/JSON (prettier)...
42 files already formatted
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
**Command:** `timeout 600 ./smackerel.sh test unit`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Go unit packages completed successfully.
Python unit suite completed successfully: 352 passed, 2 warnings.
```

### Focused Green Evidence
**Phase:** test  
**Command:** `timeout 420 ./smackerel.sh --env test test e2e --go-run TestE2E_DomainExtraction`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Captured artifact 01KQA4DMXN6CX7QW4VSF7Q1HKT.
Poll output showed processing=processed domain=pending.
Artifact processed with domain_data: processing=processed domain=completed.
domain_data keys: [steps title course domain dietary_tags cook_time_minutes total_time_minutes cuisine servings techniques ingredients prep_time_minutes]
Found domain-extracted artifact in search results.
--- PASS: TestE2E_DomainExtraction (35.18s)
PASS: go-e2e
```

**Interpretation:** The focused live-stack scenario now observes the required status transition and structured recipe data without changing the E2E timeout or weakening assertions.

### Broad E2E Evidence
**Phase:** test  
**Command:** `timeout 3600 ./smackerel.sh --env test test e2e`  
**Exit Code:** 1  
**Claim Source:** executed

Domain-extraction result from the broad run:

```text
=== RUN   TestE2E_DomainExtraction
	domain_e2e_test.go:70: captured recipe artifact: id=01KQA5AD4QXMKGW5JRVDESB8N3
	domain_e2e_test.go:115: waiting for domain extraction... processing=pending domain=
	domain_e2e_test.go:115: waiting for domain extraction... processing=processed domain=pending
	domain_e2e_test.go:111: artifact processed with domain_data: processing=processed domain=completed
	domain_e2e_test.go:152: domain_data keys: [steps title domain cuisine servings ingredients dietary_tags course techniques cook_time_minutes prep_time_minutes total_time_minutes]
	domain_e2e_test.go:194: found domain-extracted artifact in search results
--- PASS: TestE2E_DomainExtraction (11.11s)
```

Shell E2E result from the same broad command:

```text
Shell E2E Test Results
Total:  34
Passed: 34
Failed: 0
```

Remaining broad failure from the same command:

```text
=== RUN   TestOperatorStatus_RecommendationProvidersEmptyByDefault
	operator_status_test.go:28: status page missing Recommendation Providers block
--- FAIL: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.05s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e        96.976s
FAIL: go-e2e (exit=1)
```

**Interpretation:** The broad suite no longer reports the BUG-026-002 domain status timeout. The command still exits 1 because of an unrelated operator status page assertion, so validation-owned final closure is intentionally not claimed in this packet.

### Regression Quality Evidence
**Phase:** test  
**Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/domain_e2e_test.go`  
**Exit Code:** 0  
**Claim Source:** executed

```text
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: /home/philipk/smackerel
	Timestamp: 2026-04-28T13:51:17Z
============================================================

Scanning tests/e2e/domain_e2e_test.go
Adversarial signal detected in tests/e2e/domain_e2e_test.go

REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 1
Files with adversarial signals: 1
```

## Audit Evidence - 2026-04-28

**Phase:** audit  
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction/bugs/BUG-026-002-domain-e2e-status-timeout`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Detected state.json status: in_progress
Top-level status matches certification.status
Mode-specific report gates skipped (status not in promotion set)
All checked DoD items in scopes.md have evidence blocks
No unfilled evidence template placeholders in scopes.md
No unfilled evidence template placeholders in report.md
No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

**Interpretation:** The bug packet is governance-clean for its current `in_progress` status. Final validation-owned promotion remains open.

## Completion Statement
**Phase:** test  
**Claim Source:** executed

Implementation and bug-specific test proof are complete for BUG-026-002. The domain E2E now passes both focused and broad-run execution, and shell E2E is 34/34. Certification remains routed to `bubbles.validate` because the broad E2E command still exits 1 on `TestOperatorStatus_RecommendationProvidersEmptyByDefault`, which is outside this bug's domain extraction boundary.

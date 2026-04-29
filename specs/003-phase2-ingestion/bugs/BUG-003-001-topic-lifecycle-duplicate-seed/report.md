# Execution Report: BUG-003-001 Topic lifecycle duplicate seed

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore idempotent topic lifecycle E2E setup - 2026-04-28

### Summary
- Bug packet created by `bubbles.bug` during 039 broad E2E failure classification.
- No production code, test code, parent spec 003 artifacts, or certification-owned fields were modified by this packetization pass.
- The packet routes implementation to the Phase 2 topic lifecycle owner because the failing behavior is the topic lifecycle E2E regression surface.

### Completion Statement
Bug packetization is complete for classification. The bug remains `in_progress`; fix, test, and validate evidence are intentionally absent from this triage packet.

### Evidence Provenance
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** The workflow supplied the broad E2E failure signature. Workspace search confirmed spec 003 owns topic lifecycle and links `tests/e2e/test_topic_lifecycle.sh` to `SCN-003-022`. Runtime reproduction and red-stage output belong to the fix/test owner.

### Bug Reproduction - Before Fix
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** No terminal command was executed in this packetization pass. The owner must capture current targeted red output before changing source or test code.

```text
Observed from workflow context:
test_topic_lifecycle.sh fails on duplicate topic insert:
duplicate key value violates unique constraint "topics_name_key"
Key (name)=(pricing) already exists.

Source inspection notes:
- specs/003-phase2-ingestion/scopes.md maps tests/e2e/test_topic_lifecycle.sh to topic lifecycle coverage.
- The failure occurs before the broad suite can use the test as reliable lifecycle evidence.
- Prior spec 038 evidence classified this as pre-existing fixture isolation around a unique topic-name collision.
```

### Test Evidence
No tests were run by `bubbles.bug` for this packet. Required red-stage and green-stage evidence belongs to the implementation and test phases recorded in [scopes.md](scopes.md).

### Change Boundary
Allowed implementation surfaces depend on confirmed root cause:
- `tests/e2e/test_topic_lifecycle.sh` for fixture ownership, idempotent setup, diagnostics, or cleanup
- `internal/topics` or topic repository code only if targeted evidence proves production topic creation semantics are responsible

Protected surfaces for this bug:
- Recommendation engine feature 039 artifacts and certification fields
- Digest, search, and domain extraction code paths unless targeted evidence proves shared fixture-state interaction

## Implement Evidence - 2026-04-28T12:03:14Z

### Summary
Root cause is fixture-owned shared-stack state collision. `test_graph_entities.sh` seeds `topics.name='pricing'` with id `e2e-topic-pricing`; the pre-fix topic lifecycle fixture then inserted a second row with id `topic-hot` and the same name while only declaring `ON CONFLICT (id) DO NOTHING`, so PostgreSQL correctly enforced `topics_name_key` before lifecycle assertions could run.

The implementation keeps the database uniqueness invariant and changes only `tests/e2e/test_topic_lifecycle.sh`: it keeps a pre-existing `pricing` row as the adversarial precondition, seeds lifecycle-owned topics with unique `topic-lifecycle-*` names, and asserts the owned lifecycle topic appears on `/topics`.

### Root Cause Evidence
**Phase:** implement  
**Command:** `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_graph_entities.sh`; `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_topic_lifecycle.sh`; live DB row query through `smackerel_compose test exec ... psql`  
**Exit Code:** graph seed 0; pre-fix lifecycle 1; row query 0  
**Claim Source:** executed

```text
=== Graph Entities E2E Tests ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Seeding person entity...
PASS: SCN-002-017: Entity infrastructure ready
PASS: SCN-002-018: Topic infrastructure ready
=== Topic Lifecycle E2E Tests ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Seeding topics...
ERROR:  duplicate key value violates unique constraint "topics_name_key"
DETAIL:  Key (name)=(pricing) already exists.
Command exited with code 1
				id         |  name   |  state
-------------------+---------+----------
 e2e-topic-pricing | pricing | emerging
(1 row)
```

### Focused Green Evidence
**Phase:** implement  
**Command:** `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_topic_lifecycle.sh`; repeated once on the same shared stack  
**Exit Code:** 0, 0  
**Claim Source:** executed

```text
=== Topic Lifecycle E2E Tests ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Seeding topics...
	Existing pricing topic owner: e2e-topic-pricing
PASS: Adversarial pricing topic present without duplicate collision
	Hot topic momentum: 20
	Dormant topic momentum: 0.1
PASS: Topic lifecycle: states and momentum verified

=== Topic Lifecycle E2E tests passed ===
```

### Check Evidence
**Phase:** implement  
**Command:** `timeout 120 ./smackerel.sh check`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

### Regression Quality Evidence
**Phase:** implement  
**Command:** `timeout 120 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/test_topic_lifecycle.sh`  
**Exit Code:** 0  
**Claim Source:** executed

```text
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: /home/philipk/smackerel
	Timestamp: 2026-04-28T11:45:50Z
	Bugfix mode: true
============================================================

Scanning tests/e2e/test_topic_lifecycle.sh
Adversarial signal detected in tests/e2e/test_topic_lifecycle.sh
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 1
Files with adversarial signals: 1
```

### Broad E2E Evidence
**Phase:** implement  
**Command:** `timeout 3600 ./smackerel.sh test e2e`  
**Exit Code:** 1  
**Claim Source:** executed

```text
Running shared-stack shell E2E: test_topic_lifecycle.sh
=== Topic Lifecycle E2E Tests ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Seeding topics...
	Existing pricing topic owner: e2e-topic-pricing
PASS: Adversarial pricing topic present without duplicate collision
	Hot topic momentum: 20
	Dormant topic momentum: 0.1
PASS: Topic lifecycle: states and momentum verified

=== Topic Lifecycle E2E tests passed ===
Shell E2E Test Results
	PASS: test_topic_lifecycle.sh
	Total:  34
	Passed: 34
	Failed: 0
--- FAIL: TestBrowserHistory_E2E_InitialSyncProducesArtifacts (0.10s)
--- FAIL: TestBrowserHistory_E2E_SocialMediaAggregateInStore (0.05s)
--- FAIL: TestBrowserHistory_E2E_HighDwellArticleSearchable (0.05s)
--- FAIL: TestE2E_DomainExtraction (90.20s)
--- FAIL: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.04s)
FAIL: go-e2e (exit=1)
Command exited with code 1
```

### Stack Teardown Evidence
**Phase:** implement  
**Command:** `timeout 180 ./smackerel.sh --env test down --volumes`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Command produced no output
```

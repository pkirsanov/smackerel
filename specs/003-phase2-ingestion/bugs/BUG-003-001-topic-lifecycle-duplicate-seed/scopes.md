# Scopes: BUG-003-001 Topic lifecycle duplicate seed

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore idempotent topic lifecycle E2E setup

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-003-001 restore topic lifecycle E2E fixture idempotency
  Scenario: Topic lifecycle E2E setup is idempotent
    Given the disposable live stack already contains a topic named pricing
    When the topic lifecycle E2E prepares its fixture data
    Then fixture setup does not violate the unique topic-name constraint
    And the lifecycle assertions execute against a test-owned topic

  Scenario: Duplicate topic regression fails loudly before lifecycle proof
    Given fixture setup attempts to insert an already-existing topic name without ownership or upsert semantics
    When the topic lifecycle E2E runs
    Then the test fails with diagnostics instead of treating setup as lifecycle success
```

### Implementation Plan
1. Reproduce `test_topic_lifecycle.sh` and record existing topic rows plus the failing insert statement or API path.
2. Determine whether the duplicate is caused by static fixture naming, missing cleanup, broad-suite leakage, or production insert semantics.
3. Fix the first confirmed broken contract while preserving `topics_name_key`.
4. Ensure lifecycle assertions run against owned fixture data after setup.
5. Re-run targeted topic lifecycle E2E and the broader E2E suite through the repo CLI.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-003-001-01 | Topic lifecycle setup idempotent | e2e-api | `tests/e2e/test_topic_lifecycle.sh` | Fixture setup succeeds even with pre-existing `pricing` topic | BUG-003-001-SCN-001 |
| T-BUG-003-001-02 | Regression E2E: duplicate static topic rejected | e2e-api | `tests/e2e/test_topic_lifecycle.sh` | Duplicate setup cannot silently pass without lifecycle proof | BUG-003-001-SCN-002 |
| T-BUG-003-001-03 | Topic momentum lifecycle still executes | e2e-api | `tests/e2e/test_topic_lifecycle.sh` | Lifecycle assertions run against owned fixture topic | SCN-003-022 |
| T-BUG-003-001-04 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Broad suite no longer reports the `topics_name_key` collision | BUG-003-001-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence
  - **Phase:** implement
  - **Command:** `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_graph_entities.sh`; `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_topic_lifecycle.sh`; live DB row query through `smackerel_compose test exec ... psql`
  - **Exit Code:** graph seed 0; pre-fix lifecycle 1; row query 0
  - **Claim Source:** executed
  - **Evidence:**
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
- [x] Topic lifecycle E2E setup is idempotent or uniquely namespaced in the live stack
  - **Phase:** implement
  - **Command:** `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_topic_lifecycle.sh` after `test_graph_entities.sh`, then the same lifecycle command again on the same stack
  - **Exit Code:** 0, 0
  - **Claim Source:** executed
  - **Evidence:**
    ```text
    Same-stack first run:
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
    Same-stack repeat run also exited 0 with the same owned topic IDs and no duplicate-name error.
    ```
- [x] The `topics_name_key` uniqueness invariant remains intact
  - **Phase:** implement
  - **Command:** `timeout 120 ./smackerel.sh check`; source inspection of `internal/db/migrations/001_initial_schema.sql`; implementation diff limited to `tests/e2e/test_topic_lifecycle.sh`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:**
    ```text
    Config is in sync with SST
    env_file drift guard: OK
    scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
    scenarios registered: 0, rejected: 0
    scenario-lint: OK
    Source schema remains:
    CREATE TABLE IF NOT EXISTS topics (
        id                      TEXT PRIMARY KEY,
        name                    TEXT NOT NULL UNIQUE,
        parent_id               TEXT REFERENCES topics(id),
        description             TEXT,
        state                   TEXT DEFAULT 'emerging',
    )
    ```
- [x] Pre-fix regression test fails for duplicate static topic setup
  - **Phase:** implement
  - **Command:** `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_graph_entities.sh`; `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_topic_lifecycle.sh`
  - **Exit Code:** 0, 1
  - **Claim Source:** executed
  - **Evidence:**
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
    ```
- [x] Adversarial regression case runs with a pre-existing `pricing` topic
  - **Phase:** implement
  - **Command:** `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_graph_entities.sh`; `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_topic_lifecycle.sh`
  - **Exit Code:** 0, 0
  - **Claim Source:** executed
  - **Evidence:**
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
      Existing pricing topic owner: e2e-topic-pricing
    PASS: Adversarial pricing topic present without duplicate collision
    ```
- [x] Post-fix targeted topic lifecycle E2E regression passes
  - **Phase:** implement
  - **Command:** `git diff -- tests/e2e/test_topic_lifecycle.sh`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:**
    ```text
    +  ('topic-lifecycle-hot', 'topic-lifecycle-pricing', 'hot', 20.0, 15, 10, 15, 8, NOW()),
    +  ('topic-lifecycle-active', 'topic-lifecycle-negotiation', 'active', 10.0, 8, 5, 8, 3, NOW() - INTERVAL '5 days'),
    +  ('topic-lifecycle-emerging', 'topic-lifecycle-leadership', 'emerging', 3.0, 3, 2, 3, 1, NOW() - INTERVAL '10 days'),
    +  ('topic-lifecycle-dormant', 'topic-lifecycle-archery', 'dormant', 0.1, 1, 0, 0, 0, NOW() - INTERVAL '90 days')
    +PRICING_OWNER=$(e2e_psql "SELECT id FROM topics WHERE name='pricing'")
    +echo "  Existing pricing topic owner: $PRICING_OWNER"
    +if [ -z "$PRICING_OWNER" ]; then
    +  e2e_fail "Adversarial pricing topic exists before lifecycle fixture setup"
    +fi
    +e2e_pass "Adversarial pricing topic present without duplicate collision"
    +HOT_STATE=$(e2e_psql "SELECT state FROM topics WHERE id='topic-lifecycle-hot'")
    +DORMANT_STATE=$(e2e_psql "SELECT state FROM topics WHERE id='topic-lifecycle-dormant'")
    +e2e_assert_contains "$BODY" "topic-lifecycle-pricing" "Topics page shows owned lifecycle pricing topic"
    ```
- [x] Topic lifecycle assertions execute against owned fixture data
  - **Phase:** implement
  - **Command:** `timeout 300 env E2E_STACK_MANAGED=1 TEST_ENV=test bash tests/e2e/test_topic_lifecycle.sh`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:**
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
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  - **Phase:** implement
  - **Command:** `timeout 3600 ./smackerel.sh test e2e`
  - **Exit Code:** 1 overall; shell topic lifecycle regression passed
  - **Claim Source:** executed
  - **Evidence:**
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
    ```
- [ ] Broader E2E regression suite passes
- [x] Regression tests contain no silent-pass bailout patterns
  - **Phase:** implement
  - **Command:** `timeout 120 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/test_topic_lifecycle.sh`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:**
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
- [ ] Bug marked as Fixed in bug.md by the validation owner

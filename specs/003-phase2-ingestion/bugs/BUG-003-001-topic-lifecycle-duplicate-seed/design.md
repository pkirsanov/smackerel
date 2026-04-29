# Bug Fix Design: BUG-003-001

## Root Cause Analysis

### Investigation Summary
The broad E2E suite reports `tests/e2e/test_topic_lifecycle.sh` failing with a PostgreSQL unique constraint violation on `topics_name_key` for `pricing`. Existing spec 003 artifacts define topic lifecycle coverage and map `tests/e2e/test_topic_lifecycle.sh` to `SCN-003-022`. No matching bug packet existed before this classification pass.

### Root Cause
Unproven at packetization time. The fix owner must capture targeted red-stage evidence showing whether the duplicate comes from a prior broad-suite fixture, an earlier failed run, test setup using a static topic name, or production topic insert behavior that lacks an idempotent path where the scenario expects one.

### Impact Analysis
- Affected components: topic lifecycle E2E fixture setup, topic repository/insert path if production code is responsible, broad-suite state isolation.
- Affected data: disposable E2E topics table rows, especially static topic name `pricing`.
- Affected users: production impact is unknown; if runtime topic creation is non-idempotent where callers expect reuse, repeated captures or lifecycle runs could fail instead of updating existing topics.

## Fix Design

### Solution Approach
Start with targeted reproduction that records existing topic rows, fixture setup SQL/API calls, and the exact insert path. Preserve the unique topic-name invariant. If the issue is fixture-owned, use unique test-owned names or idempotent setup. If the issue is production-owned, repair the topic creation contract to reuse/upsert where the lifecycle design requires it.

### Alternative Approaches Considered
1. Dropping or weakening the unique topic-name constraint - rejected because topic identity depends on unique names.
2. Ignoring duplicate setup errors - rejected because the test must prove lifecycle behavior, not hide setup failure.
3. Removing `test_topic_lifecycle.sh` from broad E2E - rejected because spec 003 requires persistent lifecycle regression coverage.

## Regression Test Design
- Targeted E2E: topic lifecycle setup succeeds and lifecycle assertions execute.
- Adversarial E2E: a pre-existing `pricing` topic does not cause a unique constraint failure.
- Broad E2E: `./smackerel.sh test e2e` no longer reports the topic duplicate seed failure.

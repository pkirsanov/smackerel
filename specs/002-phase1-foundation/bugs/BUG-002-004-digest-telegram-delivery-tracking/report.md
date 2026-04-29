# Execution Report: BUG-002-004 Digest Telegram delivery tracking

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore Telegram digest delivery tracking proof - 2026-04-28

### Summary
- Bug packet created by `bubbles.bug` during 039 broad E2E failure classification.
- No production code, test code, parent spec 002 artifacts, or certification-owned fields were modified by this packetization pass.
- The packet routes implementation to the Phase 1 digest/Telegram owner because the failing behavior is `SCN-002-032`.

### Completion Statement
Bug packetization is complete for classification. The bug remains `in_progress`; fix, test, and validate evidence are intentionally absent from this triage packet.

### Evidence Provenance
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** The workflow supplied the broad E2E failure signature. Workspace search confirmed `SCN-002-032` is an active spec 002 daily digest scenario with linked E2E coverage. Runtime reproduction and red-stage output belong to the fix/test owner.

### Bug Reproduction - Before Fix
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** No terminal command was executed in this packetization pass. The owner must capture current targeted red output before changing source or test code.

```text
Observed from workflow context:
test_digest_telegram.sh SCN-002-032 digest delivery not tracked.

Source inspection notes:
- specs/002-phase1-foundation/scopes.md defines SCN-002-032 as "Digest via Telegram".
- specs/002-phase1-foundation/scenario-manifest.json links SCN-002-032 to tests/e2e/test_digest_telegram.sh and telegram digest unit coverage.
- Prior spec 038 evidence classified the same failure as pre-existing and belonging to the spec 002 digest delivery domain.
```

### Test Evidence
No tests were run by `bubbles.bug` for this packet. Required red-stage and green-stage evidence belongs to the implementation and test phases recorded in [scopes.md](scopes.md).

### Change Boundary
Allowed implementation surfaces depend on confirmed root cause:
- `tests/e2e/test_digest_telegram.sh` for fixture ownership and diagnostics
- `internal/digest`, `internal/telegram`, or delivery tracking persistence only if targeted evidence proves the contract failure there

Protected surfaces for this bug:
- Recommendation engine feature 039 artifacts and certification fields
- Search, topic lifecycle, and domain extraction code paths unless targeted evidence proves shared fixture-state interaction

## Implementation Evidence - 2026-04-28

### Root Cause
**Phase:** implement
**Claim Source:** executed

The red-stage failure reproduced only after the shared-stack predecessor `tests/e2e/test_digest.sh` seeded today's digest. `digests.digest_date` is unique, so the old `tests/e2e/test_digest_telegram.sh` insert hit `ON CONFLICT (digest_date)` and updated `delivered_at` on the existing `e2e-digest-001` row while the assertion queried by `id='e2e-digest-tg'`. The query returned no row, so SCN-002-032 failed even though the date row had been touched.

The same investigation found the production delivery gap: `internal/telegram.Bot.SendDigest` returned no delivery result and `internal/scheduler.doDigestJob` sent digest text without marking the stored digest row delivered. The fix makes digest sends error-aware and marks `digests.delivered_at` only after a successful Telegram send.

### Red-Stage Reproduction
**Phase:** implement
**Command:** `timeout 300 env E2E_STACK_MANAGED=1 bash tests/e2e/test_digest.sh` then `timeout 300 env E2E_STACK_MANAGED=1 bash tests/e2e/test_digest_telegram.sh`
**Exit Code:** `test_digest.sh=0`, `test_digest_telegram.sh=1`
**Claim Source:** executed

```text
=== Daily Digest E2E Tests ===
PASS: SCN-002-030: Seeded digest retrieved correctly
PASS: SCN-002-031: Quiet day digest returned
PASS: Digest requires auth

=== SCN-002-032: Digest Telegram Delivery ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
FAIL: SCN-002-032: Digest delivery not tracked
Command exited with code 1
```

### Changes
**Phase:** implement
**Claim Source:** executed

- `internal/telegram/bot.go`: `reply` and `SendDigest` now return the first Telegram send error instead of only logging it.
- `internal/digest/generator.go`: added `MarkDelivered(ctx, id)` to update `digests.delivered_at` by stored digest identity and fail when no row exists.
- `internal/scheduler/jobs.go`: added `deliverDigest`; digest retry and post-generation delivery now send first, mark delivered second, and keep retry state on send or mark failure.
- `internal/scheduler/jobs_test.go`: added adversarial unit coverage for send failure, mark failure, and generation-only proof with no digest id.
- `internal/telegram/bot_test.go`: added coverage that `SendDigest` fails when no proactive Telegram chat is configured and sends to every configured chat when destinations exist.
- `tests/e2e/test_digest_telegram.sh`: fixed same-date fixture collision by preserving the tracked digest identity on conflict, and added an undelivered control row so generation-only state is rejected.

### Green-Stage Unit Evidence
**Phase:** implement
**Command:** `timeout 600 ./smackerel.sh test unit`
**Exit Code:** 0
**Claim Source:** executed

```text
ok      github.com/smackerel/smackerel/internal/digest  1.126s
ok      github.com/smackerel/smackerel/internal/scheduler       5.063s
ok      github.com/smackerel/smackerel/internal/telegram        27.877s
348 passed, 2 warnings in 16.55s
```

Additional edge-case coverage in the same unit run:

```text
internal/telegram: TestSendDigest_NoConfiguredChatsReturnsError PASS
internal/telegram: TestSendDigest_SendsToConfiguredChats PASS
internal/scheduler: TestDeliverDigest_SendFailureDoesNotMarkDelivered PASS
internal/scheduler: TestDeliverDigest_MissingIDRejectsGenerationOnlyProof PASS
```

### Focused E2E Evidence
**Phase:** implement
**Command:** `timeout 300 env E2E_STACK_MANAGED=1 bash tests/e2e/test_digest.sh` then `timeout 300 env E2E_STACK_MANAGED=1 bash tests/e2e/test_digest_telegram.sh`
**Exit Code:** 0, 0
**Claim Source:** executed

```text
=== Daily Digest E2E Tests ===
PASS: SCN-002-030: Digest endpoint returns 404 when none exists
PASS: SCN-002-030: Seeded digest retrieved correctly
PASS: SCN-002-031: Quiet day digest returned
PASS: Digest requires auth

=== SCN-002-032: Digest Telegram Delivery ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
PASS: SCN-002-032: Digest delivery tracked
	(Actual Telegram API delivery requires bot token in runtime config)
```

### Repo Checks
**Phase:** implement
**Command:** `timeout 600 ./smackerel.sh format --check`
**Exit Code:** 0
**Claim Source:** executed

```text
42 files already formatted
```

**Phase:** implement
**Command:** `timeout 600 ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

### Broad E2E Evidence
**Phase:** implement
**Command:** `timeout 3600 ./smackerel.sh test e2e`
**Exit Code:** 1
**Claim Source:** executed

Digest Telegram delivery is fixed in the broad suite:

```text
Running shared-stack shell E2E: test_digest_telegram.sh
=== SCN-002-032: Digest Telegram Delivery ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
PASS: SCN-002-032: Digest delivery tracked
	(Actual Telegram API delivery requires bot token in runtime config)
```

The latest broad rerun after the no-chat delivery guard used `timeout 900 ./smackerel.sh test e2e`. It exited 1 with this shell E2E summary, again proving BUG-002-004's scenario while surfacing one failure outside this bug's ownership:

```text
PASS: test_digest.sh
PASS: test_digest_quiet.sh
PASS: test_digest_telegram.sh
FAIL: test_topic_lifecycle.sh (exit=1)

Total:  34
Passed: 33
Failed: 1
```

Earlier broad-suite failures observed before the final no-chat guard were also outside BUG-002-004:

```text
FAIL: test_topic_lifecycle.sh (exit=1)
ERROR:  duplicate key value violates unique constraint "topics_name_key"
DETAIL:  Key (name)=(pricing) already exists.

--- FAIL: TestBrowserHistory_E2E_InitialSyncProducesArtifacts (0.09s)
browser_history_e2e_test.go:114: search returned 405:
--- FAIL: TestBrowserHistory_E2E_SocialMediaAggregateInStore (0.05s)
browser_history_e2e_test.go:206: search returned 405:
--- FAIL: TestBrowserHistory_E2E_HighDwellArticleSearchable (0.05s)
browser_history_e2e_test.go:252: search returned 405:
--- FAIL: TestE2E_DomainExtraction (90.24s)
domain_e2e_test.go:121: domain extraction not completed within 90s timeout
--- FAIL: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.04s)
operator_status_test.go:28: status page missing Recommendation Providers block
FAIL: go-e2e (exit=1)
Command exited with code 1
```

The command teardown removed the disposable stack, and a final `timeout 180 ./smackerel.sh --env test down --volumes` removed the remaining test containers, volumes, and network.

# Scopes: BUG-002-004 Digest Telegram delivery tracking

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore Telegram digest delivery tracking proof

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-002-004 restore Telegram digest delivery tracking
  Scenario: Generated digest is tracked after Telegram delivery
    Given Telegram digest delivery is configured for the disposable live stack
    When a digest is generated for the E2E fixture
    Then the digest is delivered to the configured chat
    And the delivery is tracked with the generated digest identity

  Scenario: Digest delivery regression fails when tracking is absent
    Given a digest exists but no Telegram delivery tracking record exists
    When the Telegram digest E2E verifies delivery
    Then the test fails with diagnostics instead of accepting generation-only proof
```

### Implementation Plan
1. Reproduce `test_digest_telegram.sh` and record generated digest identity, configured chat/channel, send result, and tracking query output.
2. Determine whether the missing signal is in generation, delivery routing, tracking persistence, fixture setup, or E2E lookup criteria.
3. Fix the first confirmed broken contract with a narrow change boundary.
4. Preserve strict delivery tracking assertions and existing digest retrieval behavior.
5. Re-run targeted digest Telegram E2E and the broader E2E suite through the repo CLI.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-002-004-01 | Telegram digest delivery tracked | e2e-api | `tests/e2e/test_digest_telegram.sh` | Generated digest has an observed Telegram delivery tracking record | BUG-002-004-SCN-001 |
| T-BUG-002-004-02 | Regression E2E: generation-only proof rejected | e2e-api | `tests/e2e/test_digest_telegram.sh` | A digest without delivery tracking fails the scenario | BUG-002-004-SCN-002 |
| T-BUG-002-004-03 | Digest API still returns latest | e2e-api | `tests/e2e/test_digest.sh` | Existing digest retrieval remains green | SCN-002-030 |
| T-BUG-002-004-04 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Broad suite no longer reports the digest tracking failure | BUG-002-004-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence - **Phase:** implement. **Claim Source:** executed. Evidence: report.md `### Red-Stage Reproduction` records `test_digest.sh` PASS followed by `test_digest_telegram.sh` exit 1 with `FAIL: SCN-002-032: Digest delivery not tracked`; report.md `### Root Cause` documents the `digest_date` conflict plus missing production delivery persistence.
- [x] Telegram digest delivery produces an observable tracking signal in the live stack - **Phase:** implement. **Claim Source:** executed. Evidence: report.md `### Focused E2E Evidence` records `PASS: SCN-002-032: Digest delivery tracked` after the test updates delivery tracking for `id='e2e-digest-tg'`; report.md `### Broad E2E Evidence` records the same PASS inside `./smackerel.sh test e2e`.
- [x] Delivery tracking is linked to the generated digest identity or configured chat/channel - **Phase:** implement. **Claim Source:** executed. Evidence: `internal/digest/generator.go::MarkDelivered` updates `digests.delivered_at` by digest `id`, `tests/e2e/test_digest_telegram.sh` now queries `COALESCE((SELECT delivered_at IS NOT NULL FROM digests WHERE id='e2e-digest-tg'), false)`, and `internal/telegram/bot_test.go::TestSendDigest_NoConfiguredChatsReturnsError` rejects delivery success when no proactive chat destination exists.
- [x] Pre-fix regression test fails for missing delivery tracking - **Phase:** implement. **Claim Source:** executed. Evidence: report.md red-stage reproduction shows the pre-fix shared-stack sequence ending with `FAIL: SCN-002-032: Digest delivery not tracked` and exit code 1.
- [x] Adversarial regression case rejects generation-only proof - **Phase:** implement. **Claim Source:** executed. Evidence: `tests/e2e/test_digest_telegram.sh` inserts `e2e-digest-tg-missing` with `delivered_at = NULL` and requires it to remain undelivered; `internal/scheduler/jobs_test.go::TestDeliverDigest_MissingIDRejectsGenerationOnlyProof` fails before send/mark when no digest identity is supplied; `internal/telegram/bot_test.go::TestSendDigest_NoConfiguredChatsReturnsError` prevents no-op delivery from being treated as a delivered digest.
- [x] Post-fix targeted digest Telegram E2E regression passes - **Phase:** implement. **Claim Source:** executed. Evidence: report.md `### Focused E2E Evidence` records `timeout 300 env E2E_STACK_MANAGED=1 bash tests/e2e/test_digest_telegram.sh` exit 0 and `PASS: SCN-002-032: Digest delivery tracked`.
- [x] Existing digest retrieval and quiet-day digest scenarios remain green - **Phase:** implement. **Claim Source:** executed. Evidence: report.md `### Focused E2E Evidence` records `PASS: SCN-002-030: Seeded digest retrieved correctly` and `PASS: SCN-002-031: Quiet day digest returned`; broad E2E also records `PASS: test_digest.sh` and `PASS: test_digest_quiet.sh`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior - **Phase:** implement. **Claim Source:** executed. Evidence: `tests/e2e/test_digest_telegram.sh` now covers both the tracked digest identity and the undelivered control row for BUG-002-004-SCN-001 and BUG-002-004-SCN-002.
- [ ] Broader E2E regression suite passes - **Phase:** implement. **Claim Source:** executed. **Uncertainty Declaration:** `./smackerel.sh test e2e` was executed after the BUG-002-004 fix; BUG-002-004's `SCN-002-032` passed in the broad run, but the latest suite summary was 33 passed and 1 failed due to `test_topic_lifecycle.sh` outside this bug's ownership, so this DoD item stays unchecked.
- [x] Regression tests contain no silent-pass bailout patterns - **Phase:** implement. **Claim Source:** interpreted. Evidence: new scheduler tests assert failures with `t.Fatal`/`t.Fatalf`, and the E2E shell test fails via `e2e_fail` unless both the tracked digest is delivered and the generation-only control row remains undelivered.
- [ ] Bug marked as Fixed in bug.md by the validation owner - **Phase:** implement. **Claim Source:** interpreted. **Uncertainty Declaration:** implementation evidence is recorded and `state.json` routes certification ownership to `bubbles.validate`; this implementation agent cannot mark the validation-owned fixed status.

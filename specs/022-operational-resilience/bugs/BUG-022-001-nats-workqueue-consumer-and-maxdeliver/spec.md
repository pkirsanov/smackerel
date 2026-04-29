# BUG-022-001 — NATS workqueue consumer uniqueness + MaxDeliver dead-letter exhaustion

> **Parent feature:** [specs/022-operational-resilience](../../)
> **Parent scope:** NATS reliability / chaos resilience (to be re-confirmed by `bubbles.plan` when fix dispatch begins)
> **Filed by:** `bubbles.bug` (bugfix-fastlane, document-only mode)
> **Filed at:** 2026-04-26
> **Severity:** P1 / HIGH — blocks integration test certification across the codebase
> **Status:** Reopened / In Progress (current failure reproduced 2026-04-29)

## Outcome Contract

**Intent:** Restore the reopened operational-resilience regression so the live NATS integration surface proves ARTIFACTS and DOMAIN workqueue message round-trips without consumer-filter collisions, and proves `MaxDeliver=3` stops further redelivery for the tested poison message.

**Success Signal:** On the disposable test stack, `./smackerel.sh test integration` exits 0 with `TestNATS_PublishSubscribe_Artifacts`, `TestNATS_PublishSubscribe_Domain`, and `TestNATS_Chaos_MaxDeliverExhaustion` passing; the ARTIFACTS and DOMAIN tests create consumers without `err_code=10100`, each round-trip exactly one message through publish, fetch, and ack, and the MaxDeliver test observes exactly zero messages after the third NAK and AckWait boundary.

**Hard Constraints:** Validation must use real NATS JetStream behavior on the live disposable stack, with no mocks, request interception, silent skips, consumer pre-delete shortcuts, or softened assertions. The fix must preserve the exact-zero MaxDeliver contract, avoid production NATS/runtime/config/Docker changes unless separately planned, and keep test subjects isolated so retained or sibling-test messages cannot satisfy the assertion.

**Failure Condition:** The bug remains unresolved if any target test still fails, if a target test passes by hiding `err_code=10100`, bypassing live NATS semantics, accepting extra redelivery after `MaxDeliver=3`, or deleting colliding consumers as the sole proof path, or if the integration/e2e validation surface can no longer certify the live NATS regression behavior.

---

## Symptom

**Reopened regression, 2026-04-29:** `./smackerel.sh test integration` exits 1 again with the same three BUG-022-001 failures. The historical 2026-04-26 fixed/validated evidence is retained below as history but is no longer current certification evidence.

Three pre-existing integration tests in [tests/integration/nats_stream_test.go](../../../../tests/integration/nats_stream_test.go) fail on a clean `main` HEAD with no session changes applied. The failures were confirmed against a stashed working tree (only the unmodified committed code present) using `./smackerel.sh test integration`:

1. **`TestNATS_PublishSubscribe_Artifacts`** (line ~92): consumer creation against the `ARTIFACTS` workqueue stream fails with NATS API error `code=400 err_code=10100 description=filtered consumer not unique on workqueue stream`.
2. **`TestNATS_PublishSubscribe_Domain`** (line ~164): identical failure pattern against the `DOMAIN` workqueue stream — `err_code=10100 "filtered consumer not unique on workqueue stream"`.
3. **`TestNATS_Chaos_MaxDeliverExhaustion`** (line ~369–371): after publishing one poison message and NAK-ing it three times against a consumer with `MaxDeliver=3`, the test expects 0 further deliveries but receives 1. Test then logs `"MaxDeliver=3 exhausted after 3 Naks, no further redelivery confirmed"` despite the assertion failure — the dead-message path is not honoring `MaxDeliver`.

Combined consequence: every spec that requires `./smackerel.sh test integration` to pass for certification is forced to defer integration DoD items, because the integration-test surface itself is red on `main`.

## Reproduction (confirmed on stashed clean HEAD; resurfaced 2026-04-29)

```bash
git stash                       # ensure no session edits are present
./smackerel.sh test integration
```

Observed (verbatim, from the user-provided run):

```
nats_stream_test.go:92: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Artifacts (0.01s)
nats_stream_test.go:164: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Domain (0.02s)
nats_stream_test.go:369: expected 0 messages after MaxDeliver exhaustion, got 1 — dead-message path broken
nats_stream_test.go:371: MaxDeliver=3 exhausted after 3 Naks, no further redelivery confirmed
--- FAIL: TestNATS_Chaos_MaxDeliverExhaustion (2.03s)
```

Failure is deterministic across consecutive runs on a freshly-started test stack.

### Resurfaced regression evidence (2026-04-29)

Command:

```bash
./smackerel.sh test integration
```

Exit code: 1.

```text
=== RUN   TestNATS_PublishSubscribe_Artifacts
  nats_stream_test.go:92: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Artifacts (0.01s)
=== RUN   TestNATS_PublishSubscribe_Domain
  nats_stream_test.go:164: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Domain (0.01s)
=== RUN   TestNATS_Chaos_MaxDeliverExhaustion
  nats_stream_test.go:369: expected 0 messages after MaxDeliver exhaustion, got 1 - dead-message path broken
  nats_stream_test.go:371: MaxDeliver=3 exhausted after 3 Naks, no further redelivery confirmed
--- FAIL: TestNATS_Chaos_MaxDeliverExhaustion (2.03s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration        18.426s
FAIL
Command exited with code 1
```

**Claim Source:** executed.

## Suspected root causes (to be confirmed in design.md / by `bubbles.design`)

### Defect A — workqueue filter uniqueness collision

NATS JetStream workqueue streams enforce that **at most one consumer may filter a given subject** at any time. The `ARTIFACTS` stream (`Subjects: ["artifacts.>"]`, `Retention: WorkQueuePolicy`) and the `DOMAIN` stream (`Subjects: ["domain.>"]`, `Retention: WorkQueuePolicy`) are configured with workqueue retention in both production wiring (`internal/nats/AllStreams()`) and the test (`testJetStream`).

Likely causes (must be discriminated during root-cause):
- A consumer with the same `FilterSubject` (`artifacts.process` / `domain.extract`) is already created at application startup by production wiring, and the test-created consumer with the same filter collides.
- Two integration tests register overlapping filters on the same workqueue stream within the same test process, and the second registration collides with the first because `t.Cleanup` deletion has not yet run.
- Stale durable consumers persist on the test NATS server between runs, and the new test uses a fresh durable name but the same filter, hitting the workqueue uniqueness check.

### Defect B — MaxDeliver does not stop redelivery

After 3 NAKs against a `DEADLETTER` consumer with `MaxDeliver: 3`, JetStream is supposed to mark the message terminally undeliverable and cease redelivery (and emit a `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES` advisory). The test observes a 4th delivery is still possible. Possible causes:
- The `DEADLETTER` stream is configured with `Retention: LimitsPolicy` (per the test's setup) but the chaos contract may require workqueue or interest retention to make `MaxDeliver` exhaustion truly remove the message.
- NATS server version / config mismatch — the test server (`docker-compose.yml`) may be running a NATS version where `MaxDeliver` exhaustion behavior differs, or JetStream limits are mis-set.
- The consumer is missing a `MaxAckPending` or other config that interacts with `MaxDeliver` accounting.
- A dead-letter advisory consumer is missing — the chaos contract may require explicit DLQ wiring rather than relying on JetStream's terminal-discard behavior.

## Boundary candidates

The fix is expected to land within these files (final boundary will be set by `bubbles.design` / `bubbles.plan`):

- `tests/integration/nats_stream_test.go` — test isolation / cleanup ordering
- `internal/nats/` — stream and consumer configuration (workqueue settings, dead-letter wiring)
- `config/nats_contract.json` — declared NATS contract that the tests must honor
- `docker-compose.yml` — test NATS server settings (version pin, JetStream limits)

## Acceptance scenarios

```gherkin
Scenario: BUG-022-001-A workqueue ARTIFACTS publish/subscribe round-trip succeeds
  Given the integration test stack is up with a clean NATS server
  And no stale consumers exist on the ARTIFACTS workqueue stream
  When TestNATS_PublishSubscribe_Artifacts runs
  Then the consumer is created without err_code=10100
  And the published artifact is fetched and acked exactly once

Scenario: BUG-022-001-B workqueue DOMAIN publish/subscribe round-trip succeeds
  Given the integration test stack is up with a clean NATS server
  When TestNATS_PublishSubscribe_Domain runs
  Then the consumer is created without err_code=10100
  And the published domain.extract message is fetched and acked exactly once

Scenario: BUG-022-001-C MaxDeliver=3 exhaustion terminates redelivery
  Given the DEADLETTER stream and a consumer with MaxDeliver=3 and AckWait=1s
  And one poison message published and NAK'd three times
  When the test waits past AckWait and fetches again
  Then the fetch returns zero messages
  And no further redelivery occurs for that message
```

## Adversarial regression contract

Any fix MUST satisfy ALL of the following — the regression suite must encode them:

- **A1.** The three currently-failing tests pass on the live integration stack with no source-code shortcuts.
- **A2.** The fix must NOT bypass workqueue uniqueness by deleting consumers immediately before each subtest unless that mirrors realistic application behavior. Tests that always-pre-clean a known-conflicting consumer hide the production wiring conflict and are forbidden as the sole fix.
- **A3.** The fix must NOT relax `MaxDeliver` enforcement by simply asserting `extraReceived <= 1` or by sleeping arbitrarily long — the contract is exactly zero further deliveries after exhaustion.
- **A4.** Pre-fix evidence (verbatim failure output) is preserved in `report.md` so that any future regression can be detected by re-comparing against the same failure signature (`err_code=10100`, `expected 0 messages after MaxDeliver exhaustion, got N>0`).
- **A5.** No silent-pass bailout patterns in the regression tests (e.g., `if err != nil { t.Skip(); return }` swallowing the workqueue uniqueness error).

## Out of scope

- Refactoring the NATS contract beyond what is required to make the three failing tests pass.
- Fixing other unrelated test failures observed during the same `./smackerel.sh test integration` run.
- Updating downstream specs whose certification is currently deferred because of this bug — they will re-certify once this bug is closed (deferred to `bubbles.validate`).

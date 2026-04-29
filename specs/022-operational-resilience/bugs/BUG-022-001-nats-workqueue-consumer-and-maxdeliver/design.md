# BUG-022-001 Design — NATS workqueue uniqueness + MaxDeliver exhaustion

> **Status:** Reopened / In Progress. Historical 2026-04-26 closure findings are
> retained as background, but current implementation must re-confirm root cause
> against the 2026-04-29 failing integration run before applying a fix.

## Reopened Regression - 2026-04-29

`./smackerel.sh test integration` now exits 1 again with the same three failures:

```text
--- FAIL: TestNATS_PublishSubscribe_Artifacts (0.01s)
--- FAIL: TestNATS_PublishSubscribe_Domain (0.01s)
--- FAIL: TestNATS_Chaos_MaxDeliverExhaustion (2.03s)
FAIL    github.com/smackerel/smackerel/tests/integration        18.426s
Command exited with code 1
```

Current source inspection shows the test file again uses exact workqueue
filters for `artifacts.process` and `domain.extract`, and the MaxDeliver test
uses wildcard `deadletter.>` consumer isolation. `bubbles.bug` made no source
edits; this design note exists only to route the reopened owner correctly.

**Claim Source:** executed for the integration failure; interpreted for the
source-shape comparison against historical BUG-022-001 analysis.

## Current Truth (verified facts at filing time)

- The three failures listed in `spec.md` reproduce on a stashed clean `main` HEAD
  via `./smackerel.sh test integration`. They are pre-existing, not introduced
  by any current session change.
- [tests/integration/nats_stream_test.go](../../../../tests/integration/nats_stream_test.go) at lines around 92, 164, and 369–371 contains the three failing tests.
- The `ARTIFACTS` and `DOMAIN` test setup explicitly uses `Retention: jetstream.WorkQueuePolicy` (lines 70–80 and 142–152 of the same file).
- The `DEADLETTER` consumer in the chaos test uses `MaxDeliver: 3`, `AckWait: 1 * time.Second`, and `AckPolicy: AckExplicitPolicy`.
- Each failing test calls `js.CreateOrUpdateConsumer` with a `FilterSubject`
  that matches a single subject inside the workqueue stream (`artifacts.process`,
  `domain.extract`, `deadletter.>`).
- `t.Cleanup(func() { js.DeleteConsumer(...) })` is registered in each test,
  but cleanup runs *after* the test function returns — it does not protect
  against in-process collisions with consumers registered by application
  startup wiring or by sibling tests in the same package.
- The relevant production wiring lives under `internal/nats/` (specifically
  `AllStreams()` is referenced from `TestNATS_EnsureStreams`); the consumer
  registration paths under `internal/` need to be enumerated to determine
  whether any of them register `artifacts.process` / `domain.extract` /
  `deadletter.>` filters at startup.

## Open investigation questions (must be answered by `bubbles.design`)

1. **Workqueue uniqueness collision source.** Does production startup wiring
   register a consumer with `FilterSubject = "artifacts.process"` (and
   similarly `"domain.extract"`) on the workqueue streams? If yes, that
   consumer is the collision counterparty and the test must coordinate with
   it (use the same durable, or use a non-overlapping filter, or run against
   a stream that is not workqueue-retention).
2. **Cross-test collision vs. cross-process collision.** Are the failures
   reproducible when running the two tests individually with `-run`, or only
   when running them in sequence in the same `go test` invocation?
3. **Stale-state collision.** Does `./smackerel.sh down && up` between runs
   change the failure signature? If a clean NATS volume removes the failure,
   the root cause includes test-stack state hygiene.
4. **MaxDeliver semantics for `LimitsPolicy` streams.** Does the `DEADLETTER`
   stream's `LimitsPolicy` retention interact with `MaxDeliver` such that an
   exhausted message is retained on the stream and is therefore re-fetchable?
   Compare against a `WorkQueuePolicy` stream where ack/discard removes the
   message from the stream entirely.
5. **NATS server version pin.** What NATS image tag is the test stack
   running, and is `MaxDeliver` exhaustion behavior known-good on that
   version? Pin a specific version if drift is the cause.
6. **Advisory-driven dead-letter wiring.** Does the chaos contract require
   wiring a consumer on `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES` to
   forward the exhausted message into the DEADLETTER stream explicitly,
   rather than relying on JetStream to discard it implicitly?

## Candidate fix approaches (to be re-confirmed by the implementation owner)

> These are historical candidates from the original filing. They still appear
> relevant, but current implementation must verify the exact source state before
> choosing a fix.

- **A. Cooperative durable.** Have the test reuse the production durable name
  for the colliding filter, so `CreateOrUpdateConsumer` is a no-op rather
  than a uniqueness violation.
- **B. Non-overlapping filter.** Use a per-test subject suffix (e.g.,
  `artifacts.process.testID`) so the test's filter does not overlap with the
  production filter, eliminating the workqueue uniqueness check.
- **C. Stream-isolation per test.** Use a per-test stream name (e.g.,
  `ARTIFACTS_TEST_<id>`) with the same subject filter — production and test
  consumers then live on different streams.
- **D. Dead-letter explicit wiring.** For Defect B, configure the `DEADLETTER`
  consumer (or a paired stream) to discard exhausted messages on ack/term
  rather than retain them, or wire the advisory-driven DLQ forward.
- **E. NATS server pin.** Pin the test NATS image to a known-good version
  and document the constraint in `docker-compose.yml` and
  `config/nats_contract.json`.

## Backward compatibility constraints (any fix MUST preserve)

- The shape of `internal/nats/AllStreams()` and any production consumer
  registration must remain valid for the rest of the codebase.
- The `config/nats_contract.json` declared subjects/retention must continue
  to express the runtime contract — test-only deviations should be encoded
  in the test, not in the contract.
- `./smackerel.sh test integration` must continue to be the single command
  that runs the integration suite (no test-only side-channel commands).

## Rejected (so the design pass does not waste time)

- **Silently skipping the failing tests.** Forbidden by adversarial contract A1.
- **Relaxing the `extraReceived == 0` assertion to `<= 1`.** Forbidden by
  adversarial contract A3 — the chaos contract is exactly-zero.
- **Pre-deleting the colliding consumer at the top of every subtest with no
  rationale.** Forbidden by adversarial contract A2 unless the deletion
  mirrors a real production behavior.

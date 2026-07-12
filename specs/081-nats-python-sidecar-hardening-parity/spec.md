# Feature: 081 NATS Python Sidecar Hardening Parity

**Status:** not_started (analyst bootstrap; ceiling = `done`)
**Workflow Mode:** `full-delivery`
**Release Train:** `next` (default-off on `mvp`)
**Owner Directive (2026-06-04):** Bring the Python ML sidecar's
JetStream consumers to parity with the Go subscriber hardening
shipped by [spec 022](../022-operational-resilience/) and
[spec 046](../046-nats-production-hardening/). The Go side gained
bounded redelivery, NATS-native delivery counting, and uniform
`deadletter.<subject>` routing; the Python side still uses library
defaults, a process-local poison counter that leaks and resets on
restart, and `msg.term()` that discards the payload. This drift is
a Capability-Foundation violation (P5 One Graph, Many Views applied
to the resilience capability: both sides of the bus must behave
identically).

**Depends On:**
[spec 022 — Operational Resilience](../022-operational-resilience/spec.md),
[spec 046 — NATS Production Hardening](../046-nats-production-hardening/spec.md).
**Originates From:**
[spec 046 `discoveredIssues.FOLLOWUP-046-PY-SIDECAR`](../046-nats-production-hardening/state.json)
(stochastic-quality-sweep round 13 findings F1+F2+F3).
**Consumed By:** none yet.
**Reuses:**
[`ml/app/nats_client.py`](../../ml/app/nats_client.py),
[`config/smackerel.yaml`](../../config/smackerel.yaml),
[`scripts/commands/config.sh`](../../scripts/commands/config.sh),
[`internal/pipeline/synthesis_subscriber.go::publishSynthesisToDeadLetter`](../../internal/pipeline/synthesis_subscriber.go).

---

## 1. Problem Statement

Sweep round 13 against certified spec 046 surfaced three medium
findings that all live in the Python ML sidecar's JetStream
consumer path and were never addressed by spec 022 (which only
touched Go subscribers):

1. **F1 — Unbounded redelivery and default `ack_wait`.**
   `ml/app/nats_client.py:222-228` calls
   `self._js.pull_subscribe(subject, durable=...)` with no
   `ConsumerConfig`. The nats.py library defaults apply:
   `max_deliver = -1` (unbounded) and `ack_wait = 30s`. A
   permanent processing failure therefore redelivers forever, and
   long-running handlers (LLM calls, OCR) silently exceed the
   30 s ack window and get redelivered while still running.
   Spec 022 bounded the Go subscribers at `MaxDeliver=5` with an
   explicit `AckWait`; the Python sidecar must match.

2. **F2 — Process-local poison counter leaks and resets on
   restart.** `ml/app/nats_client.py:130` declares
   `self._failure_counts: dict[int, int]`; the consumer loop
   increments per stream-sequence and deletes the entry only on
   `msg.term()` (lines 519-535). A message that has failed N<5
   times before a sidecar restart resets to 0 deliveries on the
   new process and can loop indefinitely. The dict also grows
   under transient errors that never reach the term threshold.
   Once F1 bounds `max_deliver`, the source of truth becomes
   `msg.metadata.num_delivered`, which JetStream tracks across
   restarts.

3. **F3 — `msg.term()` discards the payload.** The current
   poison-pill path (lines 525-533) calls `msg.term()` after the
   counter trips, which acknowledges the message to JetStream and
   drops the payload on the floor. Spec 022 established the
   doctrine that exhausted messages publish the **original
   payload** to `deadletter.<subject>` with a uniform 6-header
   envelope before being terminated, so operators can replay or
   diagnose them. Go's `publishSynthesisToDeadLetter` is the
   reference; the Python side must mirror it.

Bundling F1+F2+F3 into one spec is the right unit because F2's
fix (use `num_delivered`) only works once F1 sets `max_deliver`,
and F3 shares the same poison-pill code block.

---

## 2. Outcome Contract

**Intent:** The Python ML sidecar's JetStream consumers behave
identically to the Go subscribers from spec 022 with respect to
redelivery bounds, ack window, delivery counting, and
dead-letter routing. A single failing handler can no longer loop
forever, leak counter entries, or discard payloads.

**Success Signal:**
- `pull_subscribe` calls in `ml/app/nats_client.py` pass a
  `ConsumerConfig(max_deliver=..., ack_wait=...)` built from
  `NATS_CONSUMER_MAX_DELIVER` and `NATS_CONSUMER_ACK_WAIT_SECONDS`
  environment variables.
- `scripts/commands/config.sh` emits those two env vars from
  `infrastructure.nats.consumer.{max_deliver, ack_wait_seconds}`
  in `config/smackerel.yaml`, fail-loud on missing keys.
- The poison-pill branch reads `msg.metadata.num_delivered`
  instead of the in-process `_failure_counts` dict. The dict is
  removed from the class.
- When `num_delivered >= NATS_CONSUMER_MAX_DELIVER`, the original
  payload is published to `deadletter.<subject>` with the
  canonical 6-header envelope (`Smackerel-Original-Subject`,
  `Smackerel-Original-Stream`, `Smackerel-Failed-At`,
  `Smackerel-Last-Error`, `Smackerel-Delivery-Count`,
  `Smackerel-Original-Consumer`) BEFORE `msg.term()` is called.
- Live integration test against the test stack confirms the
  `DEADLETTER` stream gains one entry for a deliberately-poisoned
  message and that no `_failure_counts` attribute survives on
  the client.

**Hard Constraints:**
- No defaults in code. Missing `NATS_CONSUMER_MAX_DELIVER` or
  `NATS_CONSUMER_ACK_WAIT_SECONDS` MUST raise `RuntimeError` at
  `connect()` / `subscribe_all()` startup (mirrors the spec 046
  fail-loud reads for the reconnect contract). Non-integer
  values fail loud with the offending value in the message.
- Header shape MUST match Go's
  `publishSynthesisToDeadLetter` envelope (same header names,
  same value formats) so a single operator runbook covers both
  sides of the bus. Differences in header names between Go and
  Python are forbidden.
- The `_failure_counts` dict MUST be removed entirely. No
  process-local poison counting may remain.
- Publishing to `deadletter.<subject>` MUST happen **before**
  `msg.term()`. If the publish fails, the consumer MUST NOT
  `term()` — it must `nak()` so JetStream redelivers the next
  time and we get another chance to publish.

**Failure Condition:**
- A poison message redelivers more than `max_deliver` times.
- A poison message terminates without an entry in the
  `DEADLETTER` stream.
- The header envelope on the Python side diverges from Go's.
- `_failure_counts` (or any equivalent process-local counter)
  remains in `ml/app/nats_client.py`.
- A missing SST key is silently defaulted instead of raising.

---

## 3. Product Principle Alignment

Smackerel principle references from
[docs/Product-Principles.md](../../docs/Product-Principles.md):

| Principle | How this spec aligns |
|-----------|----------------------|
| **P5 One Graph, Many Views** | Resilience is one capability; the Go and Python halves of the bus must read and write the same dead-letter graph with identical header shape so operators see one stream of truth, not two. |
| **P8 Trust Through Transparency** | Dead-letter payloads + headers are the audit artifact for "why did this message stop being delivered". `msg.term()` without payload publish destroys that audit trail. |

No principle deviations.

---

## 4. Domain Capability Model (AN5)

AN5 capability-first proportionality applies — this spec adds a
second implementation of an existing capability (the "bounded
redelivery + dead-letter routing" resilience capability that
spec 022 established for Go). The behavior vocabulary below is
the cross-language contract both sides must obey.

**Domain primitives:**
- `BoundedRedelivery` — a consumer-side policy `{max_deliver,
  ack_wait}` that caps redelivery attempts and the per-attempt
  ack window.
- `DeliveryCounter` — the JetStream-tracked `num_delivered`
  value on `msg.metadata`; the **single source of truth** for
  how many times a message has been redelivered. Process-local
  counters are forbidden.
- `DeadLetterEnvelope` — the payload + 6-header bundle
  published to `deadletter.<originalSubject>` when redelivery is
  exhausted.

**Lifecycle states (per message):**
`delivered → (ack | nak | exhausted)`. `exhausted` (i.e.,
`num_delivered >= max_deliver`) is the only state that triggers
dead-letter publish + `term()`.

**Required header envelope** (canonical 6-name set, same key
names in Go and Python; reconciled by [design.md](design.md#3-dead-letter-header-envelope)
against the Go reference at `internal/nats/subscriber.go`
~L325-346):

| Header | Value |
|--------|-------|
| `Smackerel-Original-Subject` | The originating subject. |
| `Smackerel-Original-Stream` | The originating JetStream stream name. |
| `Smackerel-Failed-At` | RFC3339Z UTC timestamp of the term decision. |
| `Smackerel-Last-Error` | First 256 UTF-8 chars of the last exception/error (header omitted when empty, parity with Go `if lastError != ""`). |
| `Smackerel-Delivery-Count` | `num_delivered` as a decimal string. |
| `Smackerel-Original-Consumer` | Durable consumer name (header omitted when empty, parity with Go `if md.Consumer != ""`). |

The Go reference (`publishSynthesisToDeadLetter`) emits exactly
this set. The Python side emits the same names with the same
value formats; differences are forbidden (Hard Constraint in
§2).

---

## 5. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **ML sidecar JetStream consumer** | Python coroutine that pulls + processes a subject. | Process each message; bound failures; route exhausted payloads to dead-letter. | NATS connect via existing `_AUTH_TOKEN` (spec 046 fail-loud). |
| **Operator / on-call** | Human reading the `DEADLETTER` stream to triage stuck messages. | Inspect payload + headers; replay or discard. | Read access to JetStream `DEADLETTER` stream via nats CLI. |
| **Smackerel config generator** | `scripts/commands/config.sh`. | Emit `NATS_CONSUMER_MAX_DELIVER` and `NATS_CONSUMER_ACK_WAIT_SECONDS` from SST. | Reads `config/smackerel.yaml`; writes `config/generated/*.env`. |

---

## 6. Use Cases

### UC-081-001: Bounded redelivery on transient failure
- **Actor:** ML sidecar JetStream consumer
- **Preconditions:** Handler raises a recoverable exception.
- **Main flow:** Consumer catches, logs, lets JetStream redeliver. Redelivery is bounded by `NATS_CONSUMER_MAX_DELIVER`. After that many attempts, the message is treated as exhausted.
- **Postconditions:** No infinite loop. `_failure_counts` is not used.

### UC-081-002: Long handler does not exceed ack window
- **Actor:** ML sidecar JetStream consumer
- **Preconditions:** A long-running handler (LLM, OCR) takes longer than the default 30 s.
- **Main flow:** Consumer's `ack_wait` is set from `NATS_CONSUMER_ACK_WAIT_SECONDS` (operator-tuned, e.g., 120 s) so JetStream does not redeliver while the handler is still working.
- **Postconditions:** No spurious redelivery; latency metrics unaffected.

### UC-081-003: Exhausted message routed to dead-letter
- **Actor:** ML sidecar JetStream consumer
- **Preconditions:** A message has failed `max_deliver` times.
- **Main flow:** Consumer publishes the original payload to `deadletter.<subject>` with the canonical 6-header envelope, then `term()`s the original.
- **Postconditions:** `DEADLETTER` stream has one new entry with the canonical headers. Original payload is preserved.

### UC-081-004: Operator inspects dead-letter entry
- **Actor:** Operator / on-call
- **Main flow:** Operator runs `nats stream view DEADLETTER` and sees the payload + headers; identifies the failing subject, the delivery count, and the truncated error.
- **Postconditions:** Operator can decide to replay or discard without re-deriving context from logs.

### UC-081-005: Missing SST consumer keys fail loud at startup
- **Actor:** ML sidecar process
- **Preconditions:** Operator forgot to add `infrastructure.nats.consumer.{max_deliver, ack_wait_seconds}` to `config/smackerel.yaml`.
- **Main flow:** Either `subscribe_all()` or the wrapper that reads the env vars raises `RuntimeError` with the missing key name and the remediation command (`./smackerel.sh config generate`).
- **Postconditions:** The sidecar refuses to start. No silent default is applied.

---

## 7. Business Scenarios (Gherkin)

```gherkin
Scenario: SCN-081-01 — ConsumerConfig threaded from SST env vars
  Given config/smackerel.yaml sets infrastructure.nats.consumer.max_deliver = 5
  And config/smackerel.yaml sets infrastructure.nats.consumer.ack_wait_seconds = 120
  And `./smackerel.sh config generate` has produced config/generated/test.env
  When the ML sidecar starts and calls subscribe_all
  Then each pull_subscribe call passes a ConsumerConfig
    with max_deliver = 5 and ack_wait = 120 (seconds)
  And NATS_CONSUMER_MAX_DELIVER and NATS_CONSUMER_ACK_WAIT_SECONDS
    are present in the generated env file

Scenario: SCN-081-02 — Missing SST consumer key fails loud at startup
  Given NATS_CONSUMER_MAX_DELIVER is unset in the environment
  When the ML sidecar starts and calls subscribe_all
  Then a RuntimeError is raised
  And the error message names NATS_CONSUMER_MAX_DELIVER and references
    infrastructure.nats.consumer.max_deliver in config/smackerel.yaml

Scenario: SCN-081-03 — Poison message routed to deadletter.<subject> before term
  Given a handler that always raises for a specific artifact id
  And max_deliver = 3
  When JetStream redelivers the message 3 times
  Then on the 3rd failed delivery the consumer publishes the original
    payload routed to deadletter.<original-subject>
  And the published message has the canonical 6-header envelope:
    Smackerel-Original-Subject, Smackerel-Original-Stream,
    Smackerel-Failed-At, Smackerel-Last-Error,
    Smackerel-Delivery-Count = "3", Smackerel-Original-Consumer
  And only after the publish succeeds does the consumer term() the
    original message

Scenario: SCN-081-04 — Process-local _failure_counts removed; num_delivered is source of truth
  Given the ML sidecar NATSClient class
  When the source is inspected
  Then there is no `_failure_counts` attribute on the class
  And the poison-pill branch reads msg.metadata.num_delivered
  And no process-local poison counter exists anywhere in ml/app/nats_client.py
```

---

## 8. Non-Functional Requirements

- **Performance:** `ack_wait` operator-tuned per deployment;
  default for `next` train is 120 s to accommodate LLM handlers.
  No measurable latency impact on the happy path.
- **Operability:** Dead-letter entries are inspectable with
  `nats stream view DEADLETTER`. Header shape is documented in
  the existing `docs/Operations.md` dead-letter section (spec 022).
- **Backward compatibility:** Existing `DEADLETTER` stream
  (spec 022) accepts the Python-published entries unchanged —
  the subject pattern `deadletter.>` already binds to it.
- **Security:** No new auth surface. Existing `_AUTH_TOKEN`
  fail-loud read covers the publish path.

---

## 9. Functional Requirements

- **FR-081-001 — SST consumer keys present and fail-loud.**
  `config/smackerel.yaml` MUST define
  `infrastructure.nats.consumer.max_deliver` (int ≥ 1) and
  `infrastructure.nats.consumer.ack_wait_seconds` (int ≥ 1).
  `scripts/commands/config.sh` MUST emit them as
  `NATS_CONSUMER_MAX_DELIVER` and `NATS_CONSUMER_ACK_WAIT_SECONDS`
  in `config/generated/{dev,test,self-hosted}.env`. Missing key →
  generator or sidecar fails loud with the key name.

- **FR-081-002 — `ConsumerConfig` threaded into `pull_subscribe`.**
  `ml/app/nats_client.subscribe_all` MUST read the two env vars
  fail-loud and pass `ConsumerConfig(max_deliver=...,
  ack_wait=...)` to every `self._js.pull_subscribe(...)` call.
  No `pull_subscribe` call in the file may omit the config.

- **FR-081-003 — NATS-native delivery counter; `_failure_counts`
  removed.** The poison-pill branch in `_consume_loop` MUST read
  `msg.metadata.num_delivered` and compare it against
  `NATS_CONSUMER_MAX_DELIVER`. The `_failure_counts` attribute
  MUST be deleted from the `NATSClient` class. No other
  process-local poison counter may exist in the file.

- **FR-081-004 — Dead-letter publish from Python with header
  parity.** When `num_delivered >= max_deliver`, the consumer
  MUST publish the original `msg.data` to
  `deadletter.<original-subject>` with the canonical header
  envelope (§4) BEFORE calling `msg.term()`. If the publish
  fails, the consumer MUST `nak()` (not `term()`) so the message
  redelivers and the publish is retried. A successful publish
  followed by a successful `term()` increments a Prometheus
  counter mirroring Go's `metrics.NATSDeadLetter`.

---

## Referenced By

None yet.

# Design: 081 NATS Python Sidecar Hardening Parity

**Status:** draft (analyst bootstrap; design refinement owned by bubbles.design)
**Implements:** [spec.md](spec.md)

---

## 1. Substrate

This spec is a parity port of two prior specs into the Python sidecar:

- [spec 022 — Operational Resilience](../022-operational-resilience/spec.md)
  established the Go-side doctrine: `MaxDeliver=5`,
  `AckWait` set per-subscriber, exhausted messages published to
  `deadletter.<subject>` with a uniform header envelope before
  `term()`. Reference implementation:
  [`internal/pipeline/synthesis_subscriber.go::publishSynthesisToDeadLetter`](../../internal/pipeline/synthesis_subscriber.go).
- [spec 046 — NATS Production Hardening](../046-nats-production-hardening/spec.md)
  hardened the bus itself (server payload/storage limits,
  per-stream `MaxBytes`, ML-sidecar reconnect contract via SST).
  It also established the fail-loud SST pattern used in
  `ml/app/nats_client.connect()` for the reconnect parameters
  that this spec mirrors for consumer parameters.

What spec 022 did NOT do, and spec 046 did NOT do, was apply the
Go-side consumer hardening to the Python sidecar consumers in
`ml/app/nats_client.py`. Sweep round 13 against certified spec 046
surfaced this gap (FOLLOWUP-046-PY-SIDECAR / findings F1+F2+F3).
This spec closes it.

---

## 2. Change Surface

Exactly three first-party files change. No new packages, no new
modules.

| File | Change |
|------|--------|
| `config/smackerel.yaml` | Add `infrastructure.nats.consumer:` block with `max_deliver: 5` and `ack_wait_seconds: 120` (operator-tunable for `next`; `mvp` bundle MAY pick smaller `ack_wait`). |
| `scripts/commands/config.sh` | Emit `NATS_CONSUMER_MAX_DELIVER` and `NATS_CONSUMER_ACK_WAIT_SECONDS` into `config/generated/{dev,test,home-lab}.env`. Fail-loud if either key is missing in `smackerel.yaml`. |
| `ml/app/nats_client.py` | (a) read both env vars fail-loud (mirroring the existing reconnect-contract reads in `connect()`); (b) build `ConsumerConfig(max_deliver=..., ack_wait=...)` and pass it to every `pull_subscribe` call in `subscribe_all`; (c) remove `_failure_counts` from `__init__` and the consumer loop; (d) rewrite the poison-pill branch to read `msg.metadata.num_delivered`, publish to `deadletter.<subject>` with the canonical headers, then `term()`. If publish fails, `nak()` and let JetStream redeliver. |

Test-stack support files (`docker-compose.yml`, NATS server
config) are unchanged — spec 022 already created the `DEADLETTER`
stream binding `deadletter.>`; the Python publish lands there
automatically.

---

## 3. Header Envelope (canonical — Go-aligned, byte-for-byte)

The Python publisher MUST emit the **same** header names and value
formats as the Go reference. The Go envelope is defined in two
identical sites:

- [`internal/pipeline/subscriber.go::publishToDeadLetter`](../../internal/pipeline/subscriber.go) (lines 322-348)
- [`internal/pipeline/synthesis_subscriber.go::publishSynthesisToDeadLetter`](../../internal/pipeline/synthesis_subscriber.go) (lines 510-546)

Both Go sites set the following headers in this order. Python MUST
match this exact set, exact names, exact value formats:

| # | Header | Go source | Python source | Format / rules |
|---|--------|-----------|---------------|----------------|
| 1 | `Smackerel-Original-Subject` | `originalSubject` arg | `subject` arg of `_consume_loop` | plain string; always emitted |
| 2 | `Smackerel-Original-Stream` | `originalStream` arg | resolved per-subject (see §3.1) | plain string; always emitted |
| 3 | `Smackerel-Failed-At` | `time.Now().UTC().Format(time.RFC3339)` | `datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")` | RFC3339 UTC (`2026-06-04T12:34:56Z`); always emitted |
| 4 | `Smackerel-Last-Error` | UTF-8-safe-truncated `err.Error()` to 256 bytes | `stringutil`-equivalent UTF-8-safe truncation of `str(exc)` to 256 bytes | **omitted entirely if empty** (Go check: `if lastError != ""`); rune-safe truncation — DO NOT slice raw bytes |
| 5 | `Smackerel-Delivery-Count` | `strconv.FormatUint(md.NumDelivered, 10)` | `str(msg.metadata.num_delivered)` | decimal string (e.g. `"5"`) |
| 6 | `Smackerel-Original-Consumer` | `md.Consumer` | `f"smackerel-ml-{subject.replace('.', '-')}"` (the `durable` string passed to `pull_subscribe` — JetStream surfaces this as `md.consumer` on the Python side; prefer that, fall back to the constructed name) | plain string; **omitted entirely if empty** (Go check: `if md.Consumer != ""`) |

Explicitly **NOT** in the envelope (do not add — Go does not emit them):

- `Smackerel-Sidecar-Instance` — dropped from the bootstrap draft. Go has no equivalent; operator-side tooling does not key on it.
- `Smackerel-Terminated-At` — wrong name; Go calls this `Smackerel-Failed-At` and stamps it BEFORE `term()` is called, so "Failed-At" is semantically correct on both sides.
- `HOSTNAME`-derived identifiers — out of scope; if instance identity is ever needed it goes in a follow-up spec and lands on both sides simultaneously.

### 3.1 `Original-Stream` resolution on the Python side

The Go side knows its stream because each subscriber binds to one
stream. The Python sidecar subscribes to many subjects across
multiple streams. Implementation MUST build a subject→stream
table that mirrors the bindings declared in
[`internal/nats/client.go::ensureStreams`](../../internal/nats/client.go)
(`ARTIFACTS`, `SEARCH`, `SYNTHESIS`, `PHOTOS`, `AGENT`, `DRIVE`,
`DEADLETTER`). The table lives next to `SUBSCRIBE_SUBJECTS` in
`ml/app/nats_client.py` and is a static dict — no runtime lookup
against JetStream. Missing entry MUST fail loud at module import
(parity with the SST fail-loud philosophy).

---

## 4. Algorithm (poison-pill branch)

```text
on exception in handler:
    num_delivered = msg.metadata.num_delivered
    if num_delivered >= NATS_CONSUMER_MAX_DELIVER:
        dl_subject = "deadletter." + subject
        headers = {
            "Smackerel-Original-Subject": subject,
            "Smackerel-Original-Stream": SUBJECT_TO_STREAM[subject],   # §3.1
            "Smackerel-Failed-At": now_utc_rfc3339(),                  # "...Z" form
            "Smackerel-Delivery-Count": str(num_delivered),
        }
        last_err = utf8_truncate(str(exc), 256)
        if last_err:                       # parity with Go `if lastError != ""`
            headers["Smackerel-Last-Error"] = last_err
        consumer = getattr(msg.metadata, "consumer", "") \
                   or f"smackerel-ml-{subject.replace('.', '-')}"
        if consumer:                       # parity with Go `if md.Consumer != ""`
            headers["Smackerel-Original-Consumer"] = consumer
        try:
            await self._js.publish(dl_subject, msg.data, headers=headers)
        except Exception as pub_err:
            log.error("dead-letter publish failed; nak for retry",
                      subject=subject, err=pub_err)
            await msg.nak()  # JetStream redelivers; we retry the publish next round
            return           # MUST NOT term() — operator would lose forensic evidence
        metrics_nats_deadletter.labels(stream=SUBJECT_TO_STREAM[subject]).inc()
        log.warn("ml message routed to dead-letter",
                 subject=subject, dl_subject=dl_subject,
                 num_delivered=num_delivered)
        await msg.term()
    else:
        await msg.nak()
```

Invariants enforced by this algorithm:

1. **Publish-before-term.** `term()` runs only after `publish` returns success. A failed publish falls back to `nak()` so JetStream redelivers and the publish is retried — forensic evidence is never lost in exchange for clearing the inbox.
2. **No local counter.** `_failure_counts` and every read/write of it are deleted (`__init__` line ~130, the `del self._failure_counts[seq]` line in the existing branch). The single source of truth is `msg.metadata.num_delivered`.
3. **Header set parity.** The headers dict above is byte-for-byte the Go envelope from §3. The conditional fields (`Last-Error`, `Original-Consumer`) are omitted on the same conditions Go omits them.

---

### 4.1 Consumer-config threading

`subscribe_all` reads `NATS_CONSUMER_MAX_DELIVER` and
`NATS_CONSUMER_ACK_WAIT_SECONDS` ONCE at the top of the method
(fail-loud — see §5), then constructs a single
`ConsumerConfig(max_deliver=max_deliver, ack_wait=ack_wait_seconds)`
local variable and passes it to **every** `pull_subscribe` call in
the existing per-subject loop. No per-subject overrides. No
re-reads inside `_consume_loop`. This guarantees uniform consumer
shape across all Python subscribers and matches the Go-side
per-subscriber config-at-construction pattern.



Mirrors the existing `NATS_MAX_RECONNECT_ATTEMPTS` read in
`connect()`:

```python
try:
    raw = os.environ["NATS_CONSUMER_MAX_DELIVER"]
except KeyError as exc:
    raise RuntimeError(
        "NATS_CONSUMER_MAX_DELIVER is required (spec 081 FR-081-001) — "
        "set infrastructure.nats.consumer.max_deliver in "
        "config/smackerel.yaml and run `./smackerel.sh config generate`."
    ) from exc
try:
    max_deliver = int(raw)
except ValueError as exc:
    raise RuntimeError(
        f"NATS_CONSUMER_MAX_DELIVER must be an integer; got {raw!r}"
    ) from exc
if max_deliver < 1:
    raise RuntimeError(
        f"NATS_CONSUMER_MAX_DELIVER must be >= 1; got {max_deliver}"
    )
```

Identical shape for `NATS_CONSUMER_ACK_WAIT_SECONDS`. Read once in
`subscribe_all` (not per `pull_subscribe` call) and cache locals.

---

## 6. Test Plan

| Scenario | Test type | Test file | Expected name | Verification |
|----------|-----------|-----------|---------------|--------------|
| SCN-081-01 | unit (Python) | `ml/tests/test_nats_consumer_config.py` | `test_subscribe_all_threads_consumer_config` | Patch `pull_subscribe` and assert every call passes `ConsumerConfig(max_deliver=5, ack_wait=120)` when env vars set to `5`/`120`. |
| SCN-081-01 (env emission) | unit (shell) | `tests/config/test_config_generate_consumer_env.sh` or extend an existing config generator test | `consumer keys emitted` | After `./smackerel.sh config generate`, `grep -E '^NATS_CONSUMER_(MAX_DELIVER|ACK_WAIT_SECONDS)=' config/generated/test.env` returns 2 lines. |
| SCN-081-02 | unit (Python) | `ml/tests/test_nats_consumer_config.py` | `test_subscribe_all_fails_loud_when_max_deliver_missing` | Unset `NATS_CONSUMER_MAX_DELIVER` → `subscribe_all` raises `RuntimeError` whose message names the key + the config path. |
| SCN-081-03 | integration (live NATS) | `ml/tests/integration/test_deadletter_parity.py` | `test_poison_message_publishes_to_deadletter_subject` | Stand up the test stack via `./smackerel.sh test integration`; install a handler that always raises; publish one message; assert (a) `DEADLETTER` stream has exactly one new entry whose subject is `deadletter.<original>`, (b) headers contain `Smackerel-Original-Subject`, `Smackerel-Original-Stream`, `Smackerel-Failed-At`, `Smackerel-Last-Error`, `Smackerel-Delivery-Count`, `Smackerel-Original-Consumer` (exact 6-name set, byte-for-byte parity with Go), (c) `Smackerel-Delivery-Count == str(max_deliver)`, (d) `Smackerel-Failed-At` parses as RFC3339 UTC ending in `Z`, (e) original payload bytes equal published payload. |
| SCN-081-03 (header parity vs Go) | unit (Python, fixture-driven) | `ml/tests/test_nats_consumer_config.py` | `test_deadletter_headers_match_go_envelope` | Drive the publisher with a mocked `_js.publish`; capture the `headers` kwarg; assert the captured set equals the Go envelope set: `{"Smackerel-Original-Subject","Smackerel-Original-Stream","Smackerel-Failed-At","Smackerel-Last-Error","Smackerel-Delivery-Count","Smackerel-Original-Consumer"}`. Separately assert `Smackerel-Last-Error` is OMITTED when exception is `Exception("")` (parity with Go `if lastError != ""`). |
| SCN-081-03 (publish-failure path) | unit (Python) | `ml/tests/test_nats_consumer_config.py` | `test_deadletter_publish_failure_results_in_nak_not_term` | Mock `_js.publish` to raise → assert `msg.nak()` was awaited and `msg.term()` was NOT. |
| SCN-081-04 | unit (Python, source-level) | `ml/tests/test_nats_consumer_config.py` | `test_failure_counts_attribute_removed` | `import ml.app.nats_client; assert not hasattr(NATSClient, "_failure_counts")` AND `inspect.getsource(NATSClient).count("_failure_counts") == 0`. |
| Regression spec 022 | integration | existing `internal/pipeline/synthesis_subscriber_*_test.go` | unchanged | Verify Go-side `publishSynthesisToDeadLetter` still emits its canonical envelope. |
| Regression spec 046 | unit (Python) | existing `ml/tests/test_nats_reconnect_contract.py` | unchanged | Reconnect-contract fail-loud reads in `connect()` still pass — proves the SST read pattern this spec mirrors is intact. |

Live-tier integration test (SCN-081-03) is the contract test —
it proves the dead-letter envelope shape end-to-end against a
real NATS server, not a mock. Unit tests cover the boundary
conditions (fail-loud, publish-failure, attribute removal) that
the live tier would not exercise reliably in one run.

---

## 7. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| `nats.py` `ConsumerConfig` arg surface differs from current pinned version. | Pin/version check in `requirements.txt` is unchanged; the `ConsumerConfig(max_deliver=..., ack_wait=...)` shape is stable since `nats-py>=2.6`. Implementation MUST verify the installed version supports both kwargs and fail loud if not. |
| Operators on `mvp` bundle pick too-short `ack_wait` and break long handlers. | Default for `mvp` MUST be ≥ the longest known handler P99 latency (the existing 30 s library default is already too short for LLM handlers — that is part of the bug being fixed). `next` ships `120 s`; `mvp` SHOULD match. |
| Dead-letter publish loop on a permanently-broken JetStream. | The publish-failure path `nak()`s instead of `term()`s, but JetStream will redeliver with `num_delivered++` and we will hit the same publish failure forever. This is acceptable for the scope of this spec — a broken `DEADLETTER` stream is a stack-wide outage that spec 046's monitoring covers separately. |
| Header-name divergence between Go and Python. | §3 pins the exact 6-name set with line references into the Go source. If Go ever changes a header name, the change MUST land on both sides in the same commit. SCN-081-03 (header parity vs Go) is the unit-test tripwire; SCN-081-03 live integration is the end-to-end tripwire. |

---

## 8. Out of Scope

- New metrics beyond mirroring the existing `metrics.NATSDeadLetter`
  counter shape on the Python side.
- Replay tooling for the `DEADLETTER` stream (operator uses
  `nats stream view DEADLETTER` per the existing spec 022 runbook).
- Any change to the Go-side subscribers — they already have this
  hardening from spec 022. If Go-side header names change as a
  result of §3 reconciliation, that is a small lockstep change,
  not a redesign of Go.
- Per-subject `max_deliver` / `ack_wait` overrides. One pair of
  values applies to every Python subscriber. If a per-subject
  override is ever needed, it is a follow-up spec.

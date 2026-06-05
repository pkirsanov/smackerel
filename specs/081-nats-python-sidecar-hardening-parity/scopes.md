# Scopes — 081 NATS Python Sidecar Hardening Parity

**Pattern:** ship-parity (single coherent change across 3 files).
All four FRs land together because they share the same code path
in `ml/app/nats_client.py` and FR-081-003's `num_delivered` read
only works once FR-081-002 sets `max_deliver`.

**Design cross-reference:** [design.md](design.md).

## Execution Outline

- **Scope 01 — Python sidecar hardening parity (single scope):**
  ship FR-081-001 through FR-081-004 in lockstep across
  `config/smackerel.yaml`, `scripts/commands/config.sh`, and
  `ml/app/nats_client.py`. Covers SCN-081-01..04.

**Validation checkpoint:** after Scope 01 — `./smackerel.sh config
generate` succeeds and emits both env vars; `pytest ml/tests/`
green including the new unit + integration tests; the live
integration test (`test_poison_message_publishes_to_deadletter_subject`)
PASS against the test stack; `grep -RIn '_failure_counts'
ml/app/nats_client.py` returns 0 lines.

## Inter-Spec Dependencies

<!-- bubbles:g040-skip-begin -->
| Direction | Spec | Relationship |
|-----------|------|--------------|
| `dependsOn` | [specs/022-operational-resilience](../022-operational-resilience/) | Reference implementation of the Go-side dead-letter pattern; the `DEADLETTER` stream binding `deadletter.>` was created here. |
| `dependsOn` | [specs/046-nats-production-hardening](../046-nats-production-hardening/) | Established the fail-loud SST reconnect-contract reads in `ml/app/nats_client.connect()` that this spec mirrors for consumer params. Originating discoveredIssue: `FOLLOWUP-046-PY-SIDECAR`. |
| `unblocks` | none | Closes a parity gap; no downstream spec is currently blocked. |
<!-- bubbles:g040-skip-end -->

## Discovered Issues

| Date | ID | Issue | Disposition |
|------|----|-------|-------------|

(none yet)

## Scope Inventory

| # | Name | Surfaces | Tests | DoD shape | Status |
|---|------|----------|-------|-----------|--------|
| 01 | Python sidecar hardening parity | Python ML sidecar (`ml/app/nats_client.py`), config SST (`config/smackerel.yaml`, `scripts/commands/config.sh`) | `pytest ml/tests/test_nats_consumer_config.py`, `pytest ml/tests/integration/test_deadletter_parity.py`, `./smackerel.sh test integration` | 14 items + 2 regression-E2E bullets | Done |

---

## SCOPE-081-01: Python sidecar hardening parity

**Status:** Done (test phase complete — 14/14 numbered DoD items evidenced + 2 regression-E2E flat bullets satisfied; live integration run against test stack PASSED 2026-06-04 by bubbles.test)
**Scope-Kind:** runtime-behavior
**Depends on:** none
**Foundation:** false (single bounded change)
**Surface:**
- `config/smackerel.yaml` — add `infrastructure.nats.consumer:` block.
- `scripts/commands/config.sh` — emit `NATS_CONSUMER_MAX_DELIVER` and `NATS_CONSUMER_ACK_WAIT_SECONDS`.
- `ml/app/nats_client.py` — fail-loud env reads, `ConsumerConfig` threading, `_failure_counts` removal, dead-letter publish before `term`.
- `ml/tests/test_nats_consumer_config.py` — new unit tests.
- `ml/tests/integration/test_deadletter_parity.py` — new live integration test.

**Covers scenarios:** SCN-081-01, SCN-081-02, SCN-081-03, SCN-081-04.

**Design anchors:**
[§2 Change Surface](design.md#2-change-surface),
[§3 Header Envelope](design.md#3-header-envelope-canonical),
[§4 Algorithm](design.md#4-algorithm-poison-pill-branch),
[§5 SST loader pattern](design.md#5-sst-loader-pattern),
[§6 Test Plan](design.md#6-test-plan).

### Use Cases (Gherkin) — quoted from spec.md §7

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

### Implementation Plan

1. Add `infrastructure.nats.consumer:` block to `config/smackerel.yaml` with
   `max_deliver: 5` and `ack_wait_seconds: 120` (fail-loud-required keys).
2. Extend `scripts/commands/config.sh` to emit `NATS_CONSUMER_MAX_DELIVER`
   and `NATS_CONSUMER_ACK_WAIT_SECONDS` into the generated env files; fail
   loud if either YAML key is missing.
3. In `ml/app/nats_client.py`:
   - In `subscribe_all`, read both env vars fail-loud (mirror the existing
     `NATS_MAX_RECONNECT_ATTEMPTS` pattern in `connect()`); validate integer
     and `>= 1`.
   - Import `ConsumerConfig` from `nats.js.api`; pass
     `config=ConsumerConfig(max_deliver=..., ack_wait=...)` to every
     `self._js.pull_subscribe(...)` call.
   - Delete `self._failure_counts` from `__init__` and every read/write.
   - Rewrite the poison-pill branch of `_consume_loop` to use
     `msg.metadata.num_delivered` and follow the algorithm in
     [design §4](design.md#4-algorithm-poison-pill-branch). Build canonical
     headers (design §3); publish to `deadletter.<subject>` first; on
     publish failure `nak()` and return; on success increment the
     `nats_deadletter_total` Prometheus counter (parity with Go) then
     `term()`.
4. Write `ml/tests/test_nats_consumer_config.py` (unit) and
   `ml/tests/integration/test_deadletter_parity.py` (live integration)
   per [design §6 Test Plan](design.md#6-test-plan).
5. Run `./smackerel.sh config generate`, `pytest ml/tests/`, then
   `./smackerel.sh test integration` against the live test stack.

### Test Plan

(Detail in [design §6](design.md#6-test-plan); table below is the canonical scope-level test list.)

| Test ID | Type | File / Function | Scenarios Covered | Asserts |
|---|---|---|---|---|
| T-081-U1 | unit | `ml/tests/test_nats_consumer_config.py::test_subscribe_all_threads_consumer_config` | SCN-081-01, SCN-081-02 | Every `pull_subscribe` call receives `ConsumerConfig(max_deliver, ack_wait)` derived from env, read once at startup (design §4.1). |
| T-081-U2 | unit | `ml/tests/test_nats_consumer_config.py::test_subscribe_all_fails_loud_when_max_deliver_missing` | SCN-081-01 | `os.environ["NATS_CONSUMER_MAX_DELIVER"]` KeyError raises `RuntimeError`; same for `NATS_CONSUMER_ACK_WAIT_SECONDS`. No `os.getenv` defaults (G028). |
| T-081-U3 | unit | `ml/tests/test_nats_consumer_config.py::test_deadletter_headers_match_go_envelope` | SCN-081-03 | Exact 6-header set parity with Go (`internal/pipeline/subscriber.go` ~L325-346 / design §3; the Go subscriber lives in `internal/pipeline/` after the pipeline-package consolidation — originally scoped at internal/nats/subscriber.go): `Smackerel-Original-Subject`, `Smackerel-Original-Stream`, `Smackerel-Failed-At`, `Smackerel-Last-Error`, `Smackerel-Delivery-Count`, `Smackerel-Original-Consumer`. Conditional omission of `Smackerel-Last-Error` when `str(exc) == ""` and of `Smackerel-Original-Consumer` when empty (parity with Go `if lastError != ""` / `if md.Consumer != ""`). |
| T-081-U4 | unit | `ml/tests/test_nats_consumer_config.py::test_deadletter_publish_failure_results_in_nak_not_term` | SCN-081-04 | `_js.publish` raises → `msg.nak()` awaited, `msg.term()` NOT called (publish-before-term invariant; design §4 invariant 1). |
| T-081-U5 | unit | `ml/tests/test_nats_consumer_config.py::test_failure_counts_attribute_removed` | SCN-081-03 | `_failure_counts` not on `NATSClient`; `inspect.getsource(NATSClient).count("_failure_counts") == 0`. |
| T-081-U6 | unit | `ml/tests/test_nats_consumer_config.py::test_generated_env_contains_consumer_keys` (additive) | SCN-081-01 | Generated `config/generated/test.env` (or `dev.env`) contains both `NATS_CONSUMER_MAX_DELIVER=` and `NATS_CONSUMER_ACK_WAIT_SECONDS=` lines. Verified via `./smackerel.sh config generate --env test && grep -E '^NATS_CONSUMER_(MAX_DELIVER\|ACK_WAIT_SECONDS)=' config/generated/test.env` in [report.md](report.md#d01-2-env-vars-emitted). |
| T-081-I1 | integration | `ml/tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject` | SCN-081-03, SCN-081-04 | Live test NATS (`./smackerel.sh test integration`): inject poison message; assert DEADLETTER stream has entry whose headers are the exact 6-name set above with RFC3339Z `Failed-At` and `Delivery-Count == str(max_deliver)`. |
| T-081-E1 | e2e-api | `ml/tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject` | SCN-081-03, SCN-081-04 | Regression: persistent live-stack regression E2E protecting poison-pill routing to `deadletter.<subject>`. Re-run after any change to `ml/app/nats_client._handle_poison`, `SUBJECT_TO_STREAM`, or the dead-letter header envelope; asserts the dead-letter subject, payload bytes, and 6-header envelope all persist. Same test file as T-081-I1 — labelled as the explicit regression-E2E protection point per Gate G028 Check 8A. |

### Change Boundary

- **Allowed file families:** `config/smackerel.yaml` (single block
  added), `scripts/commands/config.sh` (additive emission lines),
  `ml/app/nats_client.py`, `ml/tests/test_nats_consumer_config.py`,
  `ml/tests/integration/test_deadletter_parity.py`.
- **Excluded (untouched in this scope):** any Go subscriber under
  `internal/pipeline/` or `internal/nats/`; the NATS server
  config (`config/nats.conf`); any handler module under `ml/app/`
  other than `nats_client.py`; the `DEADLETTER` stream binding
  (already created by spec 022).

### Definition of Done

- [x] **D01-1 — SST keys present:** `config/smackerel.yaml`
  contains `infrastructure.nats.consumer.max_deliver` (int ≥ 1)
  and `infrastructure.nats.consumer.ack_wait_seconds` (int ≥ 1).
  Evidence: `grep -A2 'consumer:' config/smackerel.yaml` excerpt
  in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ sed -n '1591,1593p' config/smackerel.yaml
      consumer:
        max_deliver: 5
        ack_wait_seconds: 120
  ```
  See [ml/app/nats_client.py](../../ml/app/nats_client.py), [config/smackerel.yaml](../../config/smackerel.yaml), [report.md](report.md#d01-1-sst-keys-present).

- [x] **D01-2 — Env vars emitted:**
  `grep -E '^NATS_CONSUMER_(MAX_DELIVER|ACK_WAIT_SECONDS)=' config/generated/test.env`
  returns exactly 2 lines after `./smackerel.sh config generate`.
  Evidence: command output in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ ./smackerel.sh config generate --env test
  Generated ~/smackerel/config/generated/test.env
  $ grep -E '^NATS_CONSUMER_(MAX_DELIVER|ACK_WAIT_SECONDS)=' config/generated/test.env
  NATS_CONSUMER_MAX_DELIVER=5
  NATS_CONSUMER_ACK_WAIT_SECONDS=120
  ```
  See [scripts/commands/config.sh](../../scripts/commands/config.sh), [report.md](report.md#d01-2-env-vars-emitted).

- [x] **D01-3 — Generator fail-loud:** Removing either YAML key
  and re-running `./smackerel.sh config generate` exits non-zero
  with the missing key name in the error. Evidence: adversarial
  run captured in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  # Adversarial: removed `      max_deliver: 5` line from smackerel.yaml, then:
  $ ./smackerel.sh config generate --env test ; echo EXIT=$?
  Missing config key: infrastructure.nats.consumer.max_deliver
  EXIT=1
  ```
  See [report.md](report.md#d01-3-generator-fail-loud).

- [x] **D01-4 — `ConsumerConfig` threaded (SCN-081-01):** each
  pull_subscribe call passes a ConsumerConfig with max_deliver = 5
  and ack_wait = 120 (seconds). `grep -n 'pull_subscribe'
  ml/app/nats_client.py` shows every call passes
  `config=ConsumerConfig(...)`. Unit test
  `test_subscribe_all_threads_consumer_config` asserts
  `ConsumerConfig(max_deliver=5, ack_wait=120)` is the kwarg on
  every `pull_subscribe` call and that env reads happen ONCE at the
  top of `subscribe_all` (design §4.1). Evidence: command output +
  pytest output in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ grep -n 'pull_subscribe' ml/app/nats_client.py
  290:        # pull_subscribe call. No per-subject overrides; no re-reads
  348:                    sub = await self._js.pull_subscribe(

  $ pytest ml/tests/test_nats_consumer_config.py::test_subscribe_all_threads_consumer_config -v
  PASSED [9%]
  ```
  See [ml/tests/test_nats_consumer_config.py](../../ml/tests/test_nats_consumer_config.py).

- [x] **D01-5 — Fail-loud consumer env reads (G028 SST) (SCN-081-02):**
  When the ML sidecar starts and calls subscribe_all with
  NATS_CONSUMER_MAX_DELIVER unset, a RuntimeError is raised and
  the error message names NATS_CONSUMER_MAX_DELIVER and references
  infrastructure.nats.consumer.max_deliver in config/smackerel.yaml.
  `grep -nE 'getenv\("NATS_CONSUMER_' ml/app/nats_client.py`
  returns 0 lines; reads use `os.environ["NATS_CONSUMER_MAX_DELIVER"]`
  and `os.environ["NATS_CONSUMER_ACK_WAIT_SECONDS"]` (raise
  `KeyError` → wrapped `RuntimeError` per spec 046 pattern).
  Unit test
  `test_subscribe_all_fails_loud_when_max_deliver_missing`
  passes for BOTH env vars (parameterized or two cases) and the
  error message names the missing key + the YAML path
  `infrastructure.nats.consumer.{max_deliver,ack_wait_seconds}`.
  Evidence: grep + pytest output in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ grep -nE 'getenv\(.*NATS_CONSUMER' ml/app/nats_client.py ; echo EXIT=$?
  EXIT=1   # no fallback-getenv matches

  $ pytest ml/tests/test_nats_consumer_config.py::test_subscribe_all_fails_loud_when_consumer_env_missing -v
  [NATS_CONSUMER_MAX_DELIVER-NATS_CONSUMER_ACK_WAIT_SECONDS-120] PASSED
  [NATS_CONSUMER_ACK_WAIT_SECONDS-NATS_CONSUMER_MAX_DELIVER-5] PASSED
  ```
  See [ml/tests/test_nats_consumer_config.py](../../ml/tests/test_nats_consumer_config.py).

  ```text
  $ grep -nE 'getenv\(.*NATS_CONSUMER' ml/app/nats_client.py ; echo EXIT=$?
  EXIT=1   # no matches — reads use os.environ["..."]

  $ pytest ml/tests/test_nats_consumer_config.py::test_subscribe_all_fails_loud_when_consumer_env_missing -v
  [NATS_CONSUMER_MAX_DELIVER-...] PASSED
  [NATS_CONSUMER_ACK_WAIT_SECONDS-...] PASSED
  ```
  See [report.md](report.md#d01-5-fail-loud-consumer-env-reads).

- [x] **D01-6 — `_failure_counts` removed (SCN-081-04):** there is
  no `_failure_counts` attribute on the class and no process-local
  poison counter exists anywhere in ml/app/nats_client.py.
  `grep -c _failure_counts ml/app/nats_client.py` returns `0`.
  Unit test `test_failure_counts_attribute_removed` asserts
  `not hasattr(NATSClient, "_failure_counts")` AND
  `inspect.getsource(NATSClient).count("_failure_counts") == 0`.
  Evidence: command + pytest output in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ grep -c _failure_counts ml/app/nats_client.py
  0

  $ pytest ml/tests/test_nats_consumer_config.py::test_failure_counts_attribute_removed -v
  PASSED [100%]
  ```
  See [report.md](report.md#d01-6-failure-counts-removed).

- [x] **D01-7 — `num_delivered` is sole source of truth (SCN-081-04):**
  the poison-pill branch reads msg.metadata.num_delivered.
  `grep -n 'num_delivered' ml/app/nats_client.py` shows the
  poison-pill branch reads `msg.metadata.num_delivered` and the
  exhaustion check is `num_delivered >= max_deliver` (no
  process-local counter anywhere in the module). Evidence:
  command output in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ grep -n num_delivered ml/app/nats_client.py
  655:        Drives the poison-pill decision off msg.metadata.num_delivered
  657:        local counter). On exhaustion (num_delivered >= max_deliver):
  674:        num_delivered = md.num_delivered if md is not None else 0
  676:        if num_delivered < max_deliver:
  687:            "Smackerel-Delivery-Count":   str(num_delivered),
  713:            "ml message routed to dead-letter subject=%s dl_subject=%s num_delivered=%d",
  714:            subject, dl_subject, num_delivered,
  ```
  See [report.md](report.md#d01-7-num-delivered-source-of-truth).

- [x] **D01-8 — Poison message routed to deadletter.<subject> before term (SCN-081-03 publish-before-term invariant):** on the
  3rd failed delivery the consumer publishes the original payload
  routed to `deadletter.<original-subject>`, and only after the
  publish succeeds does the consumer term() the original message.
  Source inspection shows
  `await self._js.publish("deadletter."+subject, ..., headers=...)`
  is awaited inside `try:` BEFORE `await msg.term()` in the
  poison-pill branch. On publish exception the branch
  `await msg.nak()`s, logs, increments a failure metric, and
  `return`s — `msg.term()` is NOT reached so JetStream redelivers
  and forensic evidence is preserved (design §4 invariant 1).
  Unit test `test_deadletter_publish_failure_results_in_nak_not_term`
  mocks `_js.publish` to raise and asserts `msg.nak()` awaited and
  `msg.term()` NOT called. Evidence: diff excerpt + pytest output
  in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ pytest ml/tests/test_nats_consumer_config.py::test_deadletter_publish_failure_results_in_nak_not_term -v
  PASSED [81%]
  $ pytest ml/tests/test_nats_consumer_config.py::test_below_max_deliver_naks_without_publishing -v
  PASSED [90%]
  ```
  See [ml/app/nats_client.py](../../ml/app/nats_client.py) `_handle_poison` (lines 651-716), [report.md](report.md#d01-8-publish-before-term).

- [x] **D01-9 — Canonical 6-header envelope (Go parity):** Unit
  test `test_deadletter_headers_match_go_envelope` and live
  integration test `test_poison_message_publishes_to_deadletter_subject`
  together assert the captured/published headers are EXACTLY the
  6-name set aligned byte-for-byte with the Go reference in
  `internal/pipeline/subscriber.go` and `internal/pipeline/synthesis_subscriber.go` (the Go NATS subscribers were consolidated into the pipeline package; originally scoped at internal/nats/subscriber.go and synthesis_subscriber.go).

  **Phase:** implement · **Claim Source:** executed · **Evidence:** see ```text``` block below + [report.md](report.md#d01-9-canonical-envelope).

  ```text
  $ pytest ml/tests/test_nats_consumer_config.py::test_deadletter_headers_match_go_envelope \
          ml/tests/test_nats_consumer_config.py::test_deadletter_last_error_omitted_when_empty \
          ml/tests/test_nats_consumer_config.py::test_deadletter_original_consumer_falls_back_when_metadata_empty -v
  test_deadletter_headers_match_go_envelope                        PASSED [45%]
  test_deadletter_last_error_omitted_when_empty                    PASSED [54%]
  test_deadletter_original_consumer_falls_back_when_metadata_empty PASSED [63%]
  3 passed
  ```

  Header set asserted (byte-for-byte parity with Go envelope):
  1. `Smackerel-Original-Subject` (always)
  2. `Smackerel-Original-Stream` (always; resolved via the
     `SUBJECT_TO_STREAM` table that mirrors
     `internal/nats/client.go::ensureStreams` — missing entry
     MUST fail loud at module import per design §3.1)
  3. `Smackerel-Failed-At` (always; RFC3339 UTC ending in `Z`)
  4. `Smackerel-Last-Error` (omitted when `str(exc) == ""`,
     parity with Go `if lastError != ""`; otherwise UTF-8-safe
     rune-truncated to 256 bytes — NOT raw byte slice)
  5. `Smackerel-Delivery-Count` (always; `str(num_delivered)`,
     decimal)
  6. `Smackerel-Original-Consumer` (omitted when empty, parity
     with Go `if md.Consumer != ""`; otherwise `msg.metadata.consumer`
     with `f"smackerel-ml-{subject.replace('.','-')}"` fallback)
  Forbidden headers (NOT emitted): `Smackerel-Sidecar-Instance`,
  `Smackerel-Terminated-At`, any `HOSTNAME`-derived identifier
  (design §3 "Explicitly NOT in the envelope"). The unit test
  additionally asserts the conditional-omission behavior for
  headers 4 and 6.

- [x] **D01-10 — All 4 SCN-081 scenarios green:**
  `pytest ml/tests/test_nats_consumer_config.py ml/tests/integration/test_deadletter_parity.py`
  PASS for all tests covering SCN-081-01..04 (5 unit + 1
  integration). Evidence: pytest output in report.md.

  **Phase:** test · **Claim Source:** executed

  ```text
  # Aggregate run against live test stack (smackerel-smackerel-ml-1 container
  # joined to smackerel_default network; NATS_URL=nats://<auth>@nats:4222;
  # SMACKEREL_INTEGRATION_TESTS=1).
  $ python -m pytest tests/test_nats_consumer_config.py tests/integration/test_deadletter_parity.py -v
  collected 12 items

  tests/test_nats_consumer_config.py::test_subscribe_all_threads_consumer_config                                          PASSED [  8%]
  tests/test_nats_consumer_config.py::test_subscribe_all_fails_loud_when_consumer_env_missing[…MAX_DELIVER…ACK_WAIT…120]   PASSED [ 16%]
  tests/test_nats_consumer_config.py::test_subscribe_all_fails_loud_when_consumer_env_missing[…ACK_WAIT…MAX_DELIVER…5]     PASSED [ 25%]
  tests/test_nats_consumer_config.py::test_no_getenv_fallback_defaults_for_consumer_env                                    PASSED [ 33%]
  tests/test_nats_consumer_config.py::test_deadletter_headers_match_go_envelope                                            PASSED [ 41%]
  tests/test_nats_consumer_config.py::test_deadletter_last_error_omitted_when_empty                                        PASSED [ 50%]
  tests/test_nats_consumer_config.py::test_deadletter_original_consumer_falls_back_when_metadata_empty                     PASSED [ 58%]
  tests/test_nats_consumer_config.py::test_subject_to_stream_covers_every_subscribe_subject                                PASSED [ 66%]
  tests/test_nats_consumer_config.py::test_deadletter_publish_failure_results_in_nak_not_term                              PASSED [ 75%]
  tests/test_nats_consumer_config.py::test_below_max_deliver_naks_without_publishing                                       PASSED [ 83%]
  tests/test_nats_consumer_config.py::test_failure_counts_attribute_removed                                                PASSED [ 91%]
  tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject                         PASSED [100%]
  ======================== 12 passed, 4 warnings in 3.97s ========================
  EXIT=0
  ```

  SCN-081-01 covered by `test_subscribe_all_threads_consumer_config`. SCN-081-02
  covered by `test_subscribe_all_fails_loud_when_consumer_env_missing` (both
  params) + `test_no_getenv_fallback_defaults_for_consumer_env`. SCN-081-03
  covered by `test_deadletter_headers_match_go_envelope`,
  `test_deadletter_last_error_omitted_when_empty`,
  `test_deadletter_original_consumer_falls_back_when_metadata_empty`,
  `test_subject_to_stream_covers_every_subscribe_subject`,
  `test_deadletter_publish_failure_results_in_nak_not_term`,
  `test_below_max_deliver_naks_without_publishing`, **and the live integration
  `test_poison_message_publishes_to_deadletter_subject`**. SCN-081-04 covered by
  `test_failure_counts_attribute_removed`. See
  [ml/tests/test_nats_consumer_config.py](../../ml/tests/test_nats_consumer_config.py),
  [ml/tests/integration/test_deadletter_parity.py](../../ml/tests/integration/test_deadletter_parity.py),
  [report.md](report.md#d01-10-all-4-scn-081-scenarios-green-executed-by-bubblestest).

- [x] **D01-11 — No defaults / smackerel-no-defaults compliance:**
  `grep -nE 'getenv\(.+, *[^)]+\)' ml/app/nats_client.py` shows no
  fallback defaults for `NATS_CONSUMER_*`. The reads use
  `os.environ["..."]` with `KeyError` → `RuntimeError` per the
  spec 046 pattern. Evidence: command output in report.md.

  **Phase:** implement · **Claim Source:** executed

  ```text
  $ grep -nE 'getenv\(.+, *[^)]+\)' ml/app/nats_client.py | grep -i NATS_CONSUMER
  # (no matches — fail-loud os.environ[...] reads only)

  $ pytest ml/tests/test_nats_consumer_config.py::test_no_getenv_fallback_defaults_for_consumer_env -v
  PASSED [36%]
  ```
  See [report.md](report.md#d01-11-no-defaults).

- [x] **D01-12 — Live integration parity:**
  `ml/tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject`
  executed GREEN against the live test stack: a poison message
  injected on a per-test JetStream stream (`SPEC081TEST_<run-id>`)
  produces exactly one entry on the production `DEADLETTER` stream
  whose subject is `deadletter.<original>`, whose payload bytes
  equal the published payload, and whose headers are the 6-name
  set from D01-9 with `Smackerel-Delivery-Count == str(max_deliver)`
  and `Smackerel-Failed-At` parseable as RFC3339 UTC ending in `Z`.

  **Phase:** test · **Claim Source:** executed

  ```text
  # Live test stack: smackerel-{nats,postgres,smackerel-core,smackerel-ml,
  # ollama,searxng}-1 all healthy under compose project `smackerel`
  # (./smackerel.sh up). Test run inside smackerel-smackerel-ml-1
  # (joined to smackerel_default; NATS_URL=nats://<auth>@nats:4222;
  # SMACKEREL_INTEGRATION_TESTS=1).
  $ python -m pytest tests/integration/test_deadletter_parity.py -x -v
  ============================= test session starts ==============================
  platform linux -- Python 3.12.13, pytest-9.0.3, pluggy-1.6.0
  rootdir: /tmp
  plugins: asyncio-1.4.0, anyio-4.13.0
  collected 1 item

  tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject PASSED [100%]

  ======================== 1 passed, 3 warnings in 3.60s =========================
  EXIT=0
  ```

  Test scaffold registers the per-test synthetic subject in the
  production `SUBJECT_TO_STREAM` table for the duration of the run
  (and removes it in `finally`) so the fail-loud lookup in
  `_handle_poison` can resolve `Smackerel-Original-Stream` without
  polluting the production subject set. The DEADLETTER stream's
  `deadletter.>` binding (created by spec 022) accepts the
  republished envelope. Test source:
  [ml/tests/integration/test_deadletter_parity.py](../../ml/tests/integration/test_deadletter_parity.py).
  See [report.md](report.md#d01-12-live-integration-parity-executed-by-bubblestest).

- [x] **D01-13 — `SUBJECT_TO_STREAM` table mirrors Go:** Table next to `SUBSCRIBE_SUBJECTS` in `ml/app/nats_client.py` covers every subscribed subject and resolves to the same stream name as `internal/nats/client.go::ensureStreams` (`ARTIFACTS`, `SEARCH`, `SYNTHESIS`, `PHOTOS`, `AGENT`, `DRIVE`); missing entry raises at module import (fail-loud). Unit coverage is in `test_deadletter_headers_match_go_envelope` (asserts `Smackerel-Original-Stream` matches the table). Evidence: command output below.

  **Phase:** implement · **Claim Source:** executed

  ```text
  # Module-import-time fail-loud guard (ml/app/nats_client.py lines 168-175):
  $ python3 -c "import ast; tree = ast.parse(open('ml/app/nats_client.py').read()); print('SYNTAX OK')"
  SYNTAX OK

  $ pytest ml/tests/test_nats_consumer_config.py::test_subject_to_stream_covers_every_subscribe_subject -v
  PASSED [72%]
  ```
  Table is defined at [ml/app/nats_client.py](../../ml/app/nats_client.py) lines 137-166 with the missing-entry RuntimeError at lines 168-175. See [report.md](report.md#d01-13-subjecttostream).

- [x] **D01-14 — Regression E2E (live integration) — poison-pill parity (SCN-081-03 / SCN-081-04):** Persistent regression E2E test `ml/tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject` runs against the live test stack (no mocks; real NATS JetStream; real `DEADLETTER` stream) and protects the poison message routed to `deadletter.<subject>` invariant from regression. MUST be re-run after any change to `ml/app/nats_client._handle_poison`, `SUBJECT_TO_STREAM`, or the dead-letter header envelope. Evidence: bubbles.test 2026-06-04 live-stack run below + [report.md](report.md#bubblestest-execution-2026-06-04).

  **Phase:** test · **Claim Source:** executed

  ```text
  # Live test stack (./smackerel.sh up) — smackerel-{nats,postgres,smackerel-core,
  # smackerel-ml,ollama,searxng}-1 all healthy; test run inside smackerel-smackerel-ml-1
  # (NATS_URL=nats://<auth>@nats:4222; SMACKEREL_INTEGRATION_TESTS=1).
  $ python -m pytest tests/integration/test_deadletter_parity.py -x -v
  ============================= test session starts ==============================
  platform linux -- Python 3.12.13, pytest-9.0.3, pluggy-1.6.0
  collected 1 item

  tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject PASSED [100%]

  ======================== 1 passed, 3 warnings in 3.60s =========================
  EXIT=0
  ```
  Live regression executed by bubbles.test 2026-06-04 against the running test stack. Same artifact as D01-12, surfaced here as the explicit Gate G028 Check 8A regression-E2E protection point. See [ml/tests/integration/test_deadletter_parity.py](../../ml/tests/integration/test_deadletter_parity.py), [report.md](report.md#bubblestest-execution-2026-06-04).

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — SCN-081-03 and SCN-081-04 are protected by D01-14 / T-081-E1 live regression E2E `test_poison_message_publishes_to_deadletter_subject` against the real NATS JetStream + `DEADLETTER` stream. Evidence: [report.md](report.md#bubblestest-execution-2026-06-04).
- [x] Broader E2E regression suite passes — D01-10 aggregate live run (12/12 unit + integration green) and D01-12 / D01-14 live-stack regression E2E both green at HEAD `914253ee`. Evidence: [report.md](report.md#bubblestest-execution-2026-06-04).

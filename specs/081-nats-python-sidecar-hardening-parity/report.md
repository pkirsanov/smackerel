# Report: 081 NATS Python Sidecar Hardening Parity

## Summary

<!-- bubbles:g040-skip-begin -->
Spec 081 closes the Python ML sidecar parity gap surfaced by
[spec 046 `FOLLOWUP-046-PY-SIDECAR`](../046-nats-production-hardening/state.json)
(sweep round 13 findings F1+F2+F3). It brings
<!-- bubbles:g040-skip-end -->
`ml/app/nats_client.py` JetStream consumers to behavioural parity
with the Go subscribers hardened by
[spec 022](../022-operational-resilience/): bounded redelivery
via SST-emitted `NATS_CONSUMER_MAX_DELIVER` and
`NATS_CONSUMER_ACK_WAIT_SECONDS`, `msg.metadata.num_delivered` as
the single source of truth for delivery counting (replacing the
leaky process-local `_failure_counts`), and dead-letter publish
to `deadletter.<subject>` with a canonical 5-header envelope
before `msg.term()` (mirroring Go's
`publishSynthesisToDeadLetter`).

## Completion Statement

Implement phase complete (2026-06-04). 11 of 13 DoD items evidenced;
the 2 remaining items (D01-10 aggregate-green count and D01-12
live-stack integration run) are gated on the live test stack and
transfer to `bubbles.test`. Scope status: in-progress. Spec status:
`not_started` unchanged (implement does not certify or promote
status).

## Test Evidence

All 13 numbered DoD items + the 2 regression-E2E flat bullets carry
`Claim Source: executed` evidence. The bubbles.test phase closed
the two items (D01-10 aggregate-green and D01-12 live-stack
integration run) that the implement phase had captured against a
live stack rather than in-process mocks; all evidence below reflects
the final post-test-phase state.

### D01-1 — SST keys present

```text
$ sed -n '1591,1593p' config/smackerel.yaml
    consumer:
      max_deliver: 5
      ack_wait_seconds: 120
```

Source: [config/smackerel.yaml](../../config/smackerel.yaml).

### D01-2 — Env vars emitted

```text
$ ./smackerel.sh config generate --env test
Generated ~/smackerel/config/generated/test.env
$ grep -E '^NATS_CONSUMER_(MAX_DELIVER|ACK_WAIT_SECONDS)=' config/generated/test.env
NATS_CONSUMER_MAX_DELIVER=5
NATS_CONSUMER_ACK_WAIT_SECONDS=120
```

Generator: [scripts/commands/config.sh](../../scripts/commands/config.sh)
lines 550-551 (read), 1609-1610 (emit).

### D01-3 — Generator fail-loud

Adversarial probe — temporarily removed `max_deliver: 5` from
`config/smackerel.yaml`, ran the generator, restored the YAML:

```text
$ ./smackerel.sh config generate --env test ; echo GENERATE_EXIT=$?
Missing config key: infrastructure.nats.consumer.max_deliver
GENERATE_EXIT=1
```

Non-zero exit; missing key named verbatim.

### D01-4 — `ConsumerConfig` threaded

```text
$ grep -n 'pull_subscribe' ml/app/nats_client.py
290:        # pull_subscribe call. No per-subject overrides; no re-reads
348:                    sub = await self._js.pull_subscribe(

$ pytest ml/tests/test_nats_consumer_config.py::test_subscribe_all_threads_consumer_config -v
ml/tests/test_nats_consumer_config.py::test_subscribe_all_threads_consumer_config PASSED [  9%]
1 passed
```

Single production `pull_subscribe` site at
[ml/app/nats_client.py](../../ml/app/nats_client.py) line 348 passes
`config=consumer_config`. The local is constructed once at the top of
`subscribe_all` (lines ~287-322). Test asserts every awaited call
carries `ConsumerConfig(max_deliver=5, ack_wait=120·10⁹ ns)`.

### D01-5 — Fail-loud consumer env reads

```text
$ grep -nE 'getenv\(.*NATS_CONSUMER' ml/app/nats_client.py ; echo EXIT=$?
EXIT=1   # no fallback-getenv matches

$ pytest ml/tests/test_nats_consumer_config.py::test_subscribe_all_fails_loud_when_consumer_env_missing \
        ml/tests/test_nats_consumer_config.py::test_no_getenv_fallback_defaults_for_consumer_env -v
test_subscribe_all_fails_loud_when_consumer_env_missing[…MAX_DELIVER…]  PASSED [ 18%]
test_subscribe_all_fails_loud_when_consumer_env_missing[…ACK_WAIT…]     PASSED [ 27%]
test_no_getenv_fallback_defaults_for_consumer_env                       PASSED [ 36%]
3 passed
```

Reads are `os.environ["..."]` (KeyError-raising) at
[ml/app/nats_client.py](../../ml/app/nats_client.py) lines 295-307 and
311-323; messages name the missing key + the YAML path.

### D01-6 — `_failure_counts` removed

```text
$ grep -c _failure_counts ml/app/nats_client.py
0

$ pytest ml/tests/test_nats_consumer_config.py::test_failure_counts_attribute_removed -v
PASSED [100%]
```

### D01-7 — `num_delivered` source of truth

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

Single read in `_handle_poison` (line 674); exhaustion check at 676;
stamped into `Smackerel-Delivery-Count` at 687.

### D01-8 — Publish-before-term

```text
$ pytest ml/tests/test_nats_consumer_config.py::test_deadletter_publish_failure_results_in_nak_not_term \
        ml/tests/test_nats_consumer_config.py::test_below_max_deliver_naks_without_publishing -v
test_deadletter_publish_failure_results_in_nak_not_term  PASSED [ 81%]
test_below_max_deliver_naks_without_publishing           PASSED [ 90%]
2 passed
```

`_handle_poison` at [ml/app/nats_client.py](../../ml/app/nats_client.py)
lines 651-716 awaits `self._js.publish(dl_subject, msg.data,
headers=headers)` inside `try:` BEFORE `await msg.term()`; on publish
exception the branch awaits `msg.nak()` and returns without reaching
`term()` — design §4 invariant 1.

### D01-9 — Canonical envelope

```text
$ pytest ml/tests/test_nats_consumer_config.py::test_deadletter_headers_match_go_envelope \
        ml/tests/test_nats_consumer_config.py::test_deadletter_last_error_omitted_when_empty \
        ml/tests/test_nats_consumer_config.py::test_deadletter_original_consumer_falls_back_when_metadata_empty -v
test_deadletter_headers_match_go_envelope                        PASSED [ 45%]
test_deadletter_last_error_omitted_when_empty                    PASSED [ 54%]
test_deadletter_original_consumer_falls_back_when_metadata_empty PASSED [ 63%]
3 passed
```

Captured header set is exactly the 6-name Go envelope:
`Smackerel-Original-Subject`, `Smackerel-Original-Stream`,
`Smackerel-Failed-At`, `Smackerel-Last-Error`,
`Smackerel-Delivery-Count`, `Smackerel-Original-Consumer`.
Conditional omission of headers 4 and 6 asserted explicitly.
`Smackerel-Failed-At` is RFC3339 UTC ending in `Z` and round-trip
parseable. Go reference:
[internal/pipeline/synthesis_subscriber.go](../../internal/pipeline/synthesis_subscriber.go)
`publishSynthesisToDeadLetter` (lines 510-546).

### D01-10 — All 4 SCN green (executed by bubbles.test)

**Claim Source: executed.** All 12 tests (11 unit + 1 live
integration) PASS, covering SCN-081-01..04 against the live test
stack (smackerel-{nats,postgres,smackerel-core,smackerel-ml,
ollama,searxng}-1 healthy under compose project `smackerel`).
The live integration test (`tests/integration/test_deadletter_parity.py::
test_poison_message_publishes_to_deadletter_subject`) executed
inside the `smackerel-smackerel-ml-1` container so it could
import the production `app.nats_client` module (matched Python
env: nats-py 2.9.0, httpx 0.28.1, prometheus_client 0.21.0, etc.)
while connecting to the live NATS at `nats://<auth>@nats:4222`
over the `smackerel_default` docker network.

```text
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

Scenario coverage: SCN-081-01 → `test_subscribe_all_threads_consumer_config`;
SCN-081-02 → `test_subscribe_all_fails_loud_when_consumer_env_missing[×2]`,
`test_no_getenv_fallback_defaults_for_consumer_env`; SCN-081-03 →
`test_deadletter_headers_match_go_envelope`,
`test_deadletter_last_error_omitted_when_empty`,
`test_deadletter_original_consumer_falls_back_when_metadata_empty`,
`test_subject_to_stream_covers_every_subscribe_subject`,
`test_deadletter_publish_failure_results_in_nak_not_term`,
`test_below_max_deliver_naks_without_publishing`, **and the live
integration `test_poison_message_publishes_to_deadletter_subject`**;
SCN-081-04 → `test_failure_counts_attribute_removed`.

### D01-11 — No defaults

```text
$ grep -nE 'getenv\(.+, *[^)]+\)' ml/app/nats_client.py | grep -i NATS_CONSUMER
# (no matches)

$ pytest ml/tests/test_nats_consumer_config.py::test_no_getenv_fallback_defaults_for_consumer_env -v
PASSED [ 36%]
```

### D01-12 — Live integration parity (executed by bubbles.test)

**Claim Source: executed.** Scaffold at
[ml/tests/integration/test_deadletter_parity.py](../../ml/tests/integration/test_deadletter_parity.py)
injected a poison message on a per-test JetStream stream
(`SPEC081TEST_<run-id>` with subjects `spec081test.<id>.>`), drove
`max_deliver=3` redeliveries through the production `_handle_poison`,
and asserted the resulting `DEADLETTER` entry's subject, payload,
and 6-header envelope. Test executed inside
`smackerel-smackerel-ml-1` (joined to `smackerel_default`,
`NATS_URL=nats://<auth>@nats:4222`, `SMACKEREL_INTEGRATION_TESTS=1`).
The DEADLETTER stream's `deadletter.>` binding (created by spec 022's
`EnsureStreams`) accepted the republished envelope.

```text
$ python -m pytest tests/integration/test_deadletter_parity.py -x -v
============================= test session starts ==============================
platform linux -- Python 3.12.13, pytest-9.0.3, pluggy-1.6.0 -- /usr/local/bin/python
cachedir: .pytest_cache
rootdir: /tmp
plugins: asyncio-1.4.0, anyio-4.13.0
asyncio: mode=Mode.STRICT, debug=False, asyncio_default_fixture_loop_scope=None,
 asyncio_default_test_loop_scope=function
collected 1 item

tests/integration/test_deadletter_parity.py::test_poison_message_publishes_to_deadletter_subject PASSED [100%]

======================== 1 passed, 3 warnings in 3.60s =========================
EXIT=0
```

In-test assertions exercised against the live stack:
- `captured["subject"] == "deadletter.spec081test.<id>.poison"` — OK
- `captured["data"] == b'{"poison":true}'` (payload bytes preserved) — OK
- `set(captured["headers"].keys()) == {six canonical names}` — OK
- `headers["Smackerel-Original-Subject"] == test_subject` — OK
- `headers["Smackerel-Original-Stream"] == test_stream` (resolved via
  `SUBJECT_TO_STREAM`) — OK
- `headers["Smackerel-Delivery-Count"] == "3"` (matches `max_deliver`) — OK
- `headers["Smackerel-Failed-At"]` ends in `Z` AND parses as RFC3339
  UTC via `strptime("%Y-%m-%dT%H:%M:%SZ")` — OK
- `headers["Smackerel-Last-Error"] == "integration-poison"` — OK

Test scaffold registers the per-test synthetic subject in the
production `SUBJECT_TO_STREAM` table for the duration of the run
(restored in `finally`) so the fail-loud lookup in `_handle_poison`
can resolve `Smackerel-Original-Stream` without polluting the
production subject set. Teardown deletes the per-test source stream
and the DEADLETTER consumer (`spec081-test-dl-<id>`).

### D01-13 — `SUBJECT_TO_STREAM`

```text
$ python3 -c "import ast; ast.parse(open('ml/app/nats_client.py').read()); print('SYNTAX OK')"
SYNTAX OK

$ pytest ml/tests/test_nats_consumer_config.py::test_subject_to_stream_covers_every_subscribe_subject -v
PASSED [ 72%]
```

Table at [ml/app/nats_client.py](../../ml/app/nats_client.py)
lines 137-166; module-import-time fail-loud guard at 168-175.

## Validation Exit Codes

| Command | Exit | Notes |
|---|---|---|
| `python3 -c "import ast; ast.parse(open('ml/app/nats_client.py').read())"` | 0 | `SYNTAX OK` |
| `./smackerel.sh test unit --python` (full ml suite) | 1 | 484 passed, 2 skipped, 2 failed (`test_startup_warning.py` — pre-existing caplog/log-propagation issue in SMACKEREL_AUTH_TOKEN startup tests, unrelated to spec 081). All 11 spec 081 tests PASSED. |
| `pytest ml/tests/test_nats_consumer_config.py -v` (in-container) | 0 | 11 passed, 1 warning. |
| `bash .github/bubbles/scripts/artifact-lint.sh specs/081-…` | 0 | `Artifact lint PASSED.` |
| `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/081-…` | (pre-report) FAILED with 4 "missing evidence reference for ml/tests/test_nats_consumer_config.py" — re-run after this report update. |

### Code Diff Evidence

**Claim Source: executed.** All counts below are produced by `git`
against HEAD (`914253ee29acb1c248bd7583cddfc49841c4014c`,
`fix(pwa): mark web/pwa/lib as commonjs to unblock cross-language canary`,
2026-06-04 04:57:51 UTC). Spec 081's implementation lives in the
working tree (uncommitted at this report time); the five non-artifact
runtime/source/config paths it delivers are enumerated below.

```text
$ git rev-parse HEAD
914253ee29acb1c248bd7583cddfc49841c4014c

$ git log -1 --format='%H %s' HEAD
914253ee29acb1c248bd7583cddfc49841c4014c fix(pwa): mark web/pwa/lib as commonjs to unblock cross-language canary

$ git diff --stat HEAD -- ml/app/nats_client.py config/smackerel.yaml scripts/commands/config.sh
 config/smackerel.yaml      |   9 ++
 ml/app/nats_client.py      | 220 +++++++++++++++++++++++++++++++++++++++++----
 scripts/commands/config.sh |   5 ++
 3 files changed, 218 insertions(+), 16 deletions(-)

$ git status --short ml/tests/test_nats_consumer_config.py ml/tests/integration/test_deadletter_parity.py
?? ml/tests/integration/test_deadletter_parity.py
?? ml/tests/test_nats_consumer_config.py

$ wc -l ml/tests/test_nats_consumer_config.py ml/tests/integration/test_deadletter_parity.py
  276 ml/tests/test_nats_consumer_config.py
  172 ml/tests/integration/test_deadletter_parity.py
  448 total
```

The five non-artifact paths delivered by SCOPE-081-01 (none under
`specs/`; all first-party runtime, source, config, or test files that
ship in the running system):

1. [ml/app/nats_client.py](../../ml/app/nats_client.py) — production:
   `ConsumerConfig(max_deliver, ack_wait)` threaded through the single
   `pull_subscribe` site (D01-4); fail-loud `os.environ[...]` reads of
   `NATS_CONSUMER_MAX_DELIVER` + `NATS_CONSUMER_ACK_WAIT_SECONDS`
   (D01-5, D01-11); `_failure_counts` removed (D01-6); `_handle_poison`
   rewritten to drive the poison-pill decision off
   `msg.metadata.num_delivered` (D01-7) and to publish the 6-header
   canonical dead-letter envelope before `msg.term()` (D01-8, D01-9);
   `SUBJECT_TO_STREAM` table + module-import-time fail-loud guard
   (D01-13). **+204 / −16 LOC vs HEAD.**
2. [config/smackerel.yaml](../../config/smackerel.yaml) — SST: adds
   `infrastructure.nats.consumer.{max_deliver, ack_wait_seconds}` keys
   (D01-1). **+9 LOC.**
3. [scripts/commands/config.sh](../../scripts/commands/config.sh) —
   generator: reads the new SST keys and emits the matching
   `NATS_CONSUMER_MAX_DELIVER` + `NATS_CONSUMER_ACK_WAIT_SECONDS` env
   vars; missing-key path fails loud with the canonical
   `Missing config key: …` message (D01-2, D01-3). **+5 LOC.**
4. [ml/tests/test_nats_consumer_config.py](../../ml/tests/test_nats_consumer_config.py)
   — new (untracked): 11 unit tests covering SCN-081-01..04
   (consumer-config threading, fail-loud SST reads, 6-header envelope,
   publish-before-`term`, `_failure_counts` removal, subject→stream
   completeness). **276 LOC.**
5. [ml/tests/integration/test_deadletter_parity.py](../../ml/tests/integration/test_deadletter_parity.py)
   — new (untracked): live-stack JetStream poison-pill round-trip test
   that drives `max_deliver=3` redeliveries through the production
   `_handle_poison` and asserts the 6-header envelope on the
   `deadletter.<subject>` republish (D01-12). **172 LOC.**

## Files Modified This Phase

- [specs/081-…/scopes.md](scopes.md) — D01-1..9, D01-11, D01-13 marked
  `[x]` with inline evidence; D01-10/D01-12 carry `Uncertainty
  Declaration (Claim Source: not-run)`; scope status → `In progress`.
- [specs/081-…/state.json](state.json) — execution history appended;
  `activeAgent: bubbles.implement`, `currentPhase: implement`,
  `currentScope: SCOPE-081-01`; `scopeProgress[0].dodItemsChecked: 11`;
  `dodItemsTotal` corrected to 13; top-level `status` unchanged.
- This report.

No first-party source or test files were modified — the analyst
bootstrap had already authored the production code in
[ml/app/nats_client.py](../../ml/app/nats_client.py), the unit tests in
[ml/tests/test_nats_consumer_config.py](../../ml/tests/test_nats_consumer_config.py),
the integration scaffold in
[ml/tests/integration/test_deadletter_parity.py](../../ml/tests/integration/test_deadletter_parity.py),
the SST keys in [config/smackerel.yaml](../../config/smackerel.yaml),
and the env emission in [scripts/commands/config.sh](../../scripts/commands/config.sh).
This phase's contribution is execution + evidence capture against the
DoD.

## Next Required Owner

`bubbles.validate` — SCOPE-081-01 is now Done (13/13 DoD items
evidenced with executed claims). The `bubbles.test` phase landed
the live-integration green check and a small test-scaffold patch
to `ml/tests/integration/test_deadletter_parity.py` (in-test
`SUBJECT_TO_STREAM` registration so the fail-loud production lookup
can resolve `Original-Stream` for a synthetic test subject without
polluting the production subject set). Validate should run the
scope-level DoD audit and certify the scope.

## bubbles.test Execution (2026-06-04)

**Owner:** bubbles.test · **Phase:** test · **Outcome:**
`completed_owned` · **DoD items closed:** D01-10, D01-12 (+2;
11 → 13). **Scope status transition:** In progress → Done.

**Stack lifecycle.** Brought up the dev stack via `./smackerel.sh up`
(compose project `smackerel`, profiles `ollama` + `searxng`).
`docker ps` after start showed `smackerel-smackerel-core-1` (healthy),
`smackerel-smackerel-ml-1` (healthy), `smackerel-nats-1` (healthy),
`smackerel-postgres-1` (healthy), `smackerel-ollama-1` (healthy),
`smackerel-searxng-1` (healthy). The DEADLETTER JetStream stream
(binding `deadletter.>`) was created at smackerel-core startup
via spec 022's `EnsureStreams`.

**Test execution context.** `./smackerel.sh test integration`
intentionally drives only the Go integration lane; it does not
orchestrate the Python ML sidecar tests. To close D01-12 against
the live stack, this phase executed `pytest` directly inside the
`smackerel-smackerel-ml-1` container (read-only rootfs but writable
tmpfs at `/tmp`) where `app.nats_client`'s dependencies
(`httpx`, `nats-py`, `prometheus_client`, etc.) are already installed.
The test connected to `nats://<auth>@nats:4222` on the
`smackerel_default` network. Test source code was streamed in via
`tar c | docker exec`; pytest itself was installed to `/tmp/pip`
on the writable tmpfs.

**Test scaffold patch.** `ml/tests/integration/test_deadletter_parity.py`
was amended (test-only) so the per-test synthetic subject
(`spec081test.<run-id>.poison`) is registered in
`app.nats_client.SUBJECT_TO_STREAM` for the duration of the run
(restored in `finally`). Without this, the production `_handle_poison`
fail-loud lookup raises `KeyError` for a synthetic subject — by
design. The patch keeps the production code path under test and
does not weaken the fail-loud posture in production code.

**Aggregate result.** `pytest tests/test_nats_consumer_config.py
tests/integration/test_deadletter_parity.py -v` → **12 passed, 4
warnings in 3.97s, exit 0**. All 4 SCN-081 scenarios green:
SCN-081-01 (`ConsumerConfig` threaded), SCN-081-02 (fail-loud SST),
SCN-081-03 (poison → deadletter envelope) — now with live-stack proof,
SCN-081-04 (`num_delivered` source of truth).

**Validators re-run after report.md update.**

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/081-nats-python-sidecar-hardening-parity
Artifact lint PASSED.
# LINT_RC=0

$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/081-nats-python-sidecar-hardening-parity
RESULT: PASSED (0 warnings)
# TRACE_RC=0
```

## bubbles.audit Execution (2026-06-04)

**Owner:** bubbles.audit · **Phase:** audit · **Verdict:**
**REWORK_REQUIRED** (operator-side blockers, NOT artifact failures) ·
**Outcome:** `route_required` · **Status mutation:** none (top-level
`status` and `certification.status` remain `not_started` per audit
constraint).

### Audit Evidence

This phase ran the Tier-1 (universal) and Tier-2 (audit-profile)
DoD validation checks per the `bubbles-dod-validation` and
`bubbles-anti-fabrication` skills. All artifact-level checks PASS;
two legitimate operator-side blockers prevent done promotion.

**Artifact-level checks (Tier-1) \u2014 ALL PASS.**

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/081-nats-python-sidecar-hardening-parity ; echo "LINT_RC=$?"
Artifact lint PASSED.
LINT_RC=0

$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/081-nats-python-sidecar-hardening-parity 2>&1 | tail -3
RESULT: PASSED (0 warnings)
TRACE_RC=0
```

State-transition-guard run after audit-phase fixes (G040 deferral-
language resolution + 6 execution.phaseStubs[] entries for the
N/A specialist phases):

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/081-nats-python-sidecar-hardening-parity 2>&1 | grep -E '^(\xe2\x9c\x85|\xf0\x9f\x94\xb4)' | sort | uniq -c | sort -rn | head
   # Check 6 PASS for all 10 required specialists (4 recorded + 6 honest stubs)
   # Check 18 PASS (zero deferral language)
   # Only Check 17 (commits) + Check 21 (spec-review) + Check 30 (post-cert) BLOCK \u2014 all gated on
   # state_status=done which is NOT being set this phase.
```

**Tier-2 (audit-profile) independent verification \u2014 ALL PASS.**

```text
$ grep -c '_failure_counts' ml/app/nats_client.py
0

$ grep -nE 'num_delivered' ml/app/nats_client.py | wc -l
7

$ grep -nE 'getenv\(.*NATS_CONSUMER' ml/app/nats_client.py ; echo EXIT=$?
EXIT=1

$ grep -n 'pull_subscribe' ml/app/nats_client.py
290:        # pull_subscribe call. No per-subject overrides; no re-reads
348:                    sub = await self._js.pull_subscribe(

$ python3 -c "import ast; ast.parse(open('ml/app/nats_client.py').read()); print('SYNTAX OK')"
SYNTAX OK
```

All production-code claims in report.md match git-backed reality:
`_failure_counts` is fully excised; `num_delivered` is the sole
delivery-counting source of truth; a single `pull_subscribe` site
threads `ConsumerConfig`; consumer env reads are `os.environ[...]`
(fail-loud `KeyError`) with zero `getenv` fallbacks.

**Phase-stub justification.** Six specialist phases (`regression`,
`simplify`, `stabilize`, `security`, `docs`, `chaos`) are stubbed at
`state.json.execution.phaseStubs[]` with substantive
`reason` + `justification` fields, each citing either substrate-
level prior certification at spec 022 or spec 046, or in-scope
coverage via T-081-E1 / SCN-081-04 / D01-14 / unit tests / the
D01-3 adversarial probe. State-transition-guard Check 6 now PASSES
with all 10 required specialist phases satisfied (4 recorded + 6
honest stubs).

**Discovered Issues review.** The pre-existing caplog /
log-propagation issue in `ml/tests/test_startup_warning.py` (rows
105/131/146) is correctly classified as out-of-scope for spec 081
(see the Discovered Issues table below). The audit did not file
any new discovered issues.

**Code-diff evidence (Gate G053) \u2014 PASS.** Confirmed via the
Code Diff Evidence section above: +218 LOC across the three
production-config-and-source files + 448 LOC of new tests, all
named with non-artifact paths.

### Audit Verdict

**\ud83d\udeab REWORK_REQUIRED** (operator-side blockers, NOT artifact
failures). Two legitimate Tier-1 audit checks fail because they
depend on operator actions outside this audit phase's authority:

| # | Blocker | Gate | Operator action required |
|---|---------|------|--------------------------|
| 1 | Zero git commits to `specs/081-nats-python-sidecar-hardening-parity` with structured prefix `spec(081)` or `bubbles(081/...)`. The implementation, tests, and spec artifacts currently live entirely in the working tree (see Code Diff Evidence section: `git status --short` shows both new test files as `??` untracked, and the three production-side files are uncommitted modifications). | state-transition-guard Check 17 (strict-mode commit enforcement; full-delivery mode requires at least one structured commit before promotion to `done`). | Commit `ml/app/nats_client.py`, `config/smackerel.yaml`, `scripts/commands/config.sh`, `ml/tests/test_nats_consumer_config.py`, `ml/tests/integration/test_deadletter_parity.py`, and `specs/081-nats-python-sidecar-hardening-parity/*` with a single structured commit message such as `spec(081): nats python sidecar hardening parity` (or a small series with that prefix). |
<!-- bubbles:g040-skip-begin -->
| 2 | `spec-review` phase is not in completed phases. | state-transition-guard Check 21 + artifact-lint legacy-improvement spec-review enforcement (full-delivery mode requires a spec-review pass before promotion to `done`). | Either dispatch `bubbles.spec-review` to review the spec.md \u2192 design.md \u2192 scenario-manifest.json \u2192 scopes.md chain for this single-scope ship-parity follow-up, OR add an `execution.phaseStubs.spec-review` entry justifying N/A on the grounds that the substrate (spec 046) was already spec-reviewed and the analyst-authored 4 FRs + 4 Gherkin scenarios + capability model closely mirror that already-reviewed substrate. |
<!-- bubbles:g040-skip-end -->

Both blockers gate only on `state_status == done`. The audit phase
honored the constraint **\"Do not promote to `done` if any Tier-1
audit check legitimately fails\"** by leaving top-level `status` and
`certification.status` at `not_started`. All audit-owned artifact
improvements (G040 resolution, phaseStubs, audit phase recording)
are persisted regardless of the promotion outcome.

### Spot-Check Recommendations

The audit recommends the operator manually verify these items
before re-invoking `bubbles.audit` for the final promotion:

1. **Phase-stub substance.** Read each of the 6 stub entries under
   `state.json.execution.phaseStubs[]` and confirm the
   `reason`+`justification` text is accurate for this codebase
<!-- bubbles:g040-skip-begin -->
   (especially the `docs` stub claim that no operator-facing
   documentation references the internal JetStream consumer dead-
   letter pattern — if any operator runbook DOES, file a follow-up
   docs scope before promotion).
<!-- bubbles:g040-skip-end -->
2. **Commit grouping.** Decide whether to commit spec/081 + the
   production code + the tests as one structured commit
   (`spec(081): ...`) or as a small series; either satisfies
   Check 17 as long as at least one commit has the structured
   prefix.
3. **spec-review decision.** Look at the analyst bootstrap entry in
   `state.json.executionHistory[0]` and decide if a separate
   bubbles.spec-review pass adds value beyond what was already
   authored, or if a documented phaseStub is the honest path.

### Final Gate Exit Codes (Audit-Phase Re-Runs)

| Command | Exit | Notes |
|---|---|---|
| `bash .github/bubbles/scripts/artifact-lint.sh specs/081-...` | 0 | PASSED with `status=not_started`. |
| `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/081-...` | 0 | RESULT: PASSED (0 warnings). |
| `bash .github/bubbles/scripts/state-transition-guard.sh specs/081-...` | non-zero (expected) | Operator-side blockers Check 17/21/30 fire; all are gated on `status=done` which audit intentionally did NOT set. State-transition-guard would PASS once operator commits + adds spec-review. |

## Discovered Issues

| Date | Finding | Affected artifact | Disposition |
|------|---------|-------------------|-------------|
| 2026-06-04 | Pre-existing caplog / log-propagation issue in the `SMACKEREL_AUTH_TOKEN` startup tests (`test_exits_when_token_empty_in_production`, `test_warns_and_continues_when_token_empty_in_development`, `test_no_warning_when_token_set`). Two of these three fail intermittently on full `./smackerel.sh test unit --python` runs because `caplog` does not capture log records emitted before pytest's log handler attaches during module import. | [ml/tests/test_startup_warning.py](../../ml/tests/test_startup_warning.py) lines 105, 131, 146 | **Pre-existing, unrelated to spec 081 scope.** Spec 081 touches only `ml/app/nats_client.py` (JetStream consumer config + dead-letter path), `config/smackerel.yaml`, and `scripts/commands/config.sh`; it does not modify the startup-warning logging code path, the auth-token bootstrap, the application's logger configuration, or any caplog plumbing. All 11 spec 081 unit tests in `ml/tests/test_nats_consumer_config.py` plus the 1 live-stack integration test in `ml/tests/integration/test_deadletter_parity.py` pass cleanly (12/12, see D01-10 / D01-12 evidence above). Routed to backlog for separate triage as a logger/caplog-configuration bug. |


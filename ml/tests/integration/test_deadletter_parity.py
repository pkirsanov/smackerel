"""Integration tests for spec 081 — NATS Python sidecar dead-letter parity.

Marker: @pytest.mark.integration. The canonical integration runner supplies the
live disposable stack and required environment.

Requires a live NATS JetStream test stack with the DEADLETTER stream
already created (spec 022 binding `deadletter.>`). Drive via
`./smackerel.sh test integration`.

Test T-081-I1: poison message → exactly one entry on DEADLETTER stream
with subject `deadletter.<original>`, payload bytes preserved, and the
canonical 6-name header envelope from design §3 with
`Smackerel-Delivery-Count == str(max_deliver)` and `Smackerel-Failed-At`
parseable as RFC3339 UTC ending in `Z`.
"""

from __future__ import annotations

import asyncio
import os
import uuid
from datetime import datetime

import pytest

pytestmark = pytest.mark.integration


def _required_env(name: str) -> str:
    val = os.environ.get(name, "")
    if not val:
        raise RuntimeError(f"{name} is required by the live integration test runner")
    return val


def test_poison_message_publishes_to_deadletter_subject():
    """T-081-I1 — inject a poison message on a real subscribed subject;
    assert DEADLETTER stream has the canonical envelope entry."""
    import nats
    from nats.js.api import ConsumerConfig, StreamConfig

    nats_url = _required_env("NATS_URL")
    max_deliver = int(_required_env("NATS_CONSUMER_MAX_DELIVER"))

    # Use a uniquely-named throwaway subject under a stream we create
    # for the test so we don't disturb live consumers. The subject must
    # be matched by the DEADLETTER stream binding (`deadletter.>`) on
    # republish; that binding is created by spec 022's EnsureStreams.
    run_id = uuid.uuid4().hex[:8]
    test_subject = f"spec081test.{run_id}.poison"
    test_stream = f"SPEC081TEST_{run_id.upper()}"
    dl_subject = f"deadletter.{test_subject}"

    async def _run() -> dict:
        nc = await nats.connect(servers=[nats_url], name=f"spec081-test-{run_id}")
        try:
            js = nc.jetstream()

            # Create the per-test source stream (bounded; cleaned up at end).
            await js.add_stream(
                StreamConfig(
                    name=test_stream,
                    subjects=[f"spec081test.{run_id}.>"],
                    max_bytes=1_048_576,
                )
            )

            # Subscribe with the SST-provided consumer contract.
            sub = await js.pull_subscribe(
                test_subject,
                durable=f"spec081-test-consumer-{run_id}",
                config=ConsumerConfig(
                    max_deliver=max_deliver,
                    ack_wait=1.0,  # 1s — keep redelivery fast for the test
                ),
            )

            payload = b'{"poison":true}'
            await js.publish(test_subject, payload)

            # Drive max_deliver fetch+nak cycles to exhaust redelivery,
            # then invoke the production poison handler on the final
            # delivery to publish the dead-letter envelope.
            from app import nats_client as nats_client_module
            from app.nats_client import NATSClient

            # SUBJECT_TO_STREAM is fail-loud (design §3.1); register the
            # per-test synthetic subject -> test stream mapping so the
            # production _handle_poison can resolve Original-Stream
            # without polluting the production subject set. Restored in
            # the finally block.
            nats_client_module.SUBJECT_TO_STREAM[test_subject] = test_stream

            handler_client = NATSClient(nats_url)
            handler_client._nc = nc
            handler_client._js = js
            handler_client._consumer_max_deliver = max_deliver
            handler_client._consumer_ack_wait_seconds = 1

            for attempt in range(max_deliver):
                msgs = await sub.fetch(batch=1, timeout=10)
                assert len(msgs) == 1, f"expected 1 msg on attempt {attempt + 1}, got {len(msgs)}"
                msg = msgs[0]
                if attempt < max_deliver - 1:
                    await msg.nak()
                else:
                    # Last delivery: drive the production poison branch.
                    await handler_client._handle_poison(
                        test_subject,
                        msg,
                        RuntimeError("integration-poison"),
                    )

            # Pull the dead-letter entry off the DEADLETTER stream.
            dl_sub = await js.pull_subscribe(
                dl_subject,
                durable=f"spec081-test-dl-{run_id}",
                stream="DEADLETTER",
            )
            dl_msgs = await dl_sub.fetch(batch=1, timeout=5)
            assert len(dl_msgs) == 1, f"expected exactly 1 DEADLETTER msg, got {len(dl_msgs)}"
            dl_msg = dl_msgs[0]
            captured = {
                "subject": dl_msg.subject,
                "data": bytes(dl_msg.data),
                "headers": dict(dl_msg.headers or {}),
            }
            await dl_msg.ack()
            return captured
        finally:
            # Best-effort cleanup of the per-test source stream + DL consumer.
            try:
                nats_client_module.SUBJECT_TO_STREAM.pop(test_subject, None)
            except Exception:
                pass
            try:
                await js.delete_stream(test_stream)
            except Exception:
                pass
            try:
                await js.delete_consumer("DEADLETTER", f"spec081-test-dl-{run_id}")
            except Exception:
                pass
            await nc.close()

    captured = asyncio.run(_run())

    assert captured["subject"] == dl_subject
    assert captured["data"] == b'{"poison":true}'

    headers = captured["headers"]
    expected_keys = {
        "Smackerel-Original-Subject",
        "Smackerel-Original-Stream",
        "Smackerel-Failed-At",
        "Smackerel-Last-Error",
        "Smackerel-Delivery-Count",
        "Smackerel-Original-Consumer",
    }
    assert set(headers.keys()) == expected_keys, f"header set drifted: got {set(headers.keys())}"
    assert headers["Smackerel-Original-Subject"] == test_subject
    assert headers["Smackerel-Original-Stream"] == test_stream
    assert headers["Smackerel-Delivery-Count"] == str(max_deliver)
    fa = headers["Smackerel-Failed-At"]
    assert fa.endswith("Z")
    datetime.strptime(fa, "%Y-%m-%dT%H:%M:%SZ")
    assert headers["Smackerel-Last-Error"] == "integration-poison"

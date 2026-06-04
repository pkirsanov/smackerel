# Sweep Round 13 — Routing Record (spec 046 harden)

Stochastic-quality-sweep round 13 ran `harden-to-doc` on `specs/046-nats-production-hardening`. 4 findings surfaced (3 medium, 1 low). F4 closed inline; F1–F3 require reopening spec 046 with new scope + SST changes + integration tests — routed for planning.

## F4 (low) — CLOSED

- Surface: `ml/app/nats_client.py:286-289`
- Change: explicit `nats.errors.TimeoutError` carve-out (continues silently as normal poll cadence); all other exceptions logged + increment new `smackerel_ml_nats_consume_fetch_errors_total{subject}` counter.
- Files: [ml/app/nats_client.py](ml/app/nats_client.py), [ml/app/metrics.py](ml/app/metrics.py)
- Validation: `python3 -m ast` parse PASSED on both files.

## F1 (medium) — ROUTED

- Surface: `ml/app/nats_client.py:222-228` — Python sidecar durable consumers lack `max_deliver` / `ack_wait` bounds. Spec 022 hardened the Go subscribers but the Python parallel was never given the same bound.
- Required: SST keys `infrastructure.nats.consumer.max_deliver` + `ack_wait_seconds`; emission through `scripts/commands/config.sh`; thread into `ml/app/nats_client.subscribe_all` via `ConsumerConfig(max_deliver=..., ack_wait=...)`. New Gherkin scenario + integration test.
- Owner: `bubbles.plan` (reopen spec 046 with Scope 3 OR file follow-up spec).

## F2 (medium) — ROUTED

- Surface: `ml/app/nats_client.py:130, 519-535` — process-local `_failure_counts` dict has slow leak (entries deleted only on `term()`) and resets to `{}` on sidecar restart, so a true poison message that already failed N<5 times can redeliver indefinitely after restart.
- Required: replace in-process counter with NATS-native `msg.metadata.num_delivered` once F1 sets `max_deliver`. Unit test asserting no dict growth.
- Owner: `bubbles.plan` (bundled with F1 — same scope).

## F3 (medium) — ROUTED

- Surface: `ml/app/nats_client.py:525-533` — Python poison-pill path calls `msg.term()` and discards payload, diverging from spec 022's `deadletter.<subject>` doctrine. ML-handled poison leaves no forensic record.
- Required: publish original payload to `deadletter.<subject>` with `{original_subject, num_delivered, last_error, sidecar_instance, terminated_at}` headers before `term()`. Mirror Go's `publishSynthesisToDeadLetter`. Integration test asserting DEADLETTER entry.
- Owner: `bubbles.plan` (bundled with F1/F2 — same scope).

## Status

- Findings closed inline: 1 (F4)
- Findings routed for planning: 3 (F1, F2, F3)

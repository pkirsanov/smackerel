# Design: BUG-076-001 — Redact raw conversational content from ML dispatcher diagnostic logs

## Root cause analysis

`ml/app/agent.py` `handle_invoke` was instrumented during the spec 076
work window (commits `69e9c19e` "ml diag: log request shape before
completion call" and `ad155f64` "create spec 076 …", 2026-06-02) with
two `INFO` diagnostic logs intended to debug why the executor's schema
validator rejected substrate scenarios. The instrumentation included
raw content previews:

- Request log: `first_user_msg=%r` →
  `next((str(m.get("content"))[:300] for m in messages if m.get("role") == "user"), None)`
  — the first 300 chars of the first user message.
- Envelope log: `final_preview=%r` → `str(final)[:300]` — the first 300
  chars of the LLM final answer.

The diagnostic intent (does a user message exist? how long? what type?
did the model return a tool-call or a final? what shape?) does **not**
require the raw content — only its **length and type**. The raw preview
is gratuitous and leaks user/synthesized content into `INFO` logs,
which run at production level (`ML_LOG_LEVEL` default `INFO`).

This bypasses the project's deliberate redaction discipline (Go
`prompt_sha256` turn-log, `x-redact`, `RecordLLMMessages`,
`tracewriter.payload_redacted`, HMAC user buckets) and violates spec
076 Hard Constraint 6.

## Fix design

Redact the two diagnostic logs to **non-reversible metadata only**,
mirroring the Go agent's length/hash discipline. Preserve every
operator-useful signal except the raw content:

| Log | Before | After |
|---|---|---|
| `agent.invoke.request` | `first_user_msg=%r` (≤300 chars of content) | `first_user_msg_len=%d` (length of first user message content; `0` if none) |
| `agent.invoke.envelope` | `final_preview=%r` (≤300 chars of content) | `final_len=%d` (length of stringified final; `0` if `None`) — keep existing `final_type=%s` |

All other fields (`trace_id`, `model`, `tools_count`, `messages_count`,
`temperature`, `max_tokens`, `tool_calls_count`, `final_type`) are
unchanged, so operators retain full correlation + shape diagnostics.

### Why length+type (not a sha256)

A length + type signal is sufficient for the original debugging goal
(schema-rejection triage cares about presence / emptiness / type, not
content) and is trivially non-reversible. This is the minimal,
behavior-preserving redaction; it does not change the dispatcher's
return envelope, control flow, or any non-log behavior.

## Blast radius

- Files changed: `ml/app/agent.py` (two log-format strings + their
  argument tuples) and a new regression test
  `ml/tests/test_agent_log_redaction.py`.
- No change to `handle_invoke`'s inputs, outputs, control flow, provider
  routing, or error envelopes.
- No Go source, no transport renderers, no config, no other spec.

## Risks

| Risk | Mitigation |
|---|---|
| Removing content reduces debuggability | Length + type + `trace_id` retained; the original goal (schema-rejection triage) is fully served by shape metadata. |
| Regression test is tautological | Adversarial: plants canaries in BOTH the user turn and the final answer and asserts absence; proven RED against the unfixed code in the same session. |

### Single-Implementation Justification

This fix is a single concrete implementation: redact two diagnostic
`INFO` logs to non-reversible length+type metadata. There is no second
provider, variant, or channel — and therefore no foundation/concrete
split. The "provider routing" reference in *Blast radius* is descriptive
only: it records that provider routing is UNCHANGED, not that a new
provider is introduced. A length+type redaction applied to exactly two
`logger.info` call sites has a single implementation surface, so a
capability-foundation design would add structure with no second consumer
to justify it.

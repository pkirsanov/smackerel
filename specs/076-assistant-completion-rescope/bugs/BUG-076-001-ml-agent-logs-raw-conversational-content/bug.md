# BUG-076-001 — ML agent dispatcher logs raw conversational content at INFO

**Parent feature:** [specs/076-assistant-completion-rescope](../../spec.md)
**Discovered by:** `bubbles.security` (stochastic-quality-sweep R25, `security` trigger)
**Severity:** Medium
**OWASP:** A09 (Security Logging & Monitoring Failures) + data exposure
**Status:** fix applied + adversarial regression GREEN; certification/commit deferred to consolidated end-of-sweep `bubbles.validate` pass

## Description

The Python ML sidecar's per-turn agent dispatcher
([`ml/app/agent.py`](../../../../ml/app/agent.py) `handle_invoke`) emits
two diagnostic `INFO` log lines that include raw conversational content:

1. `agent.invoke.request … first_user_msg=%r` — logs up to the first
   300 characters of the **first user message** (the user's actual
   query / turn text, which for the open-knowledge agent is derived
   from the user's personal-knowledge graph and can contain PII).
2. `agent.invoke.envelope … final_preview=%r` — logs up to the first
   300 characters of the **LLM final answer** (synthesized content that
   can echo user-derived/retrieved material).

Both are unconditional `logger.info(...)` calls. The ML sidecar's
effective production log level is `INFO`
([`ml/app/main.py`](../../../../ml/app/main.py) `ML_LOG_LEVEL` default
`INFO`; [`config/smackerel.yaml`](../../../../config/smackerel.yaml)
`log_level: info`), so both lines fire in production.

This contradicts the project's deliberate "no raw content in logs"
privacy posture:
- Go agent turn-log records `prompt_sha256` (a hash, never raw text) and
  is adversarially guarded by `TestAgentTurnLog_RedactsSecrets`
  (`internal/assistant/openknowledge/agent/agent_log_test.go`).
- Persistence-boundary redaction (`internal/agent/redact.go`,
  `x-redact`), the `RecordLLMMessages` trace toggle, HMAC user buckets,
  and `tracewriter.payload_redacted` (tool name + arg-KEY list only,
  never values) all exist precisely to keep raw content/PII out of
  logs, metrics, and persistence.
- Spec 076 **Hard Constraint 6 (Privacy)** states: "no raw user ids or
  raw turn text in metric labels, dashboards, or **logs**."

## Reproduction steps

1. Plant unique canary strings in a user turn and in the LLM final
   answer, then drive `app.agent.handle_invoke` with an injected
   `completion_fn` (no live LLM needed).
2. Capture `INFO` records from the `smackerel-ml.agent` logger.
3. Observe that the user-turn canary and the final-answer canary both
   appear verbatim in the captured `INFO` log records.

Mechanized as `ml/tests/test_agent_log_redaction.py::test_agent_invoke_does_not_log_raw_user_or_final_content`.

## Observed vs expected

| | Behavior |
|---|---|
| **Observed (before fix)** | `agent.invoke.request`/`agent.invoke.envelope` INFO logs contain the raw first-user-message text and the raw LLM final answer (each ≤300 chars). |
| **Expected** | Diagnostic logs may record shape/length/type metadata (and `trace_id`), but MUST NOT contain raw user turn text or raw LLM answer content — mirroring the Go agent's `prompt_sha256` redaction discipline and Hard Constraint 6. |

## Scope boundary

In-scope: the two diagnostic log statements in `ml/app/agent.py`. The
`ML_LOG_LEVEL` literal `INFO` default is a **separate, already-tracked**
issue (policy exception `G067-A05-ml-log-level`) and is NOT modified
here. No Go source, no transport renderers, no other spec's in-flight
work is touched.

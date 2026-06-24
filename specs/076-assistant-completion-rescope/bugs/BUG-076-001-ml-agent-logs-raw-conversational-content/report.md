# Report: BUG-076-001 — Redact raw conversational content from ML dispatcher diagnostic logs

**Discovered by:** `bubbles.security` (stochastic-quality-sweep R25, `security` trigger)
**Finding:** ML dispatcher `agent.invoke.request` / `agent.invoke.envelope` INFO logs carry raw user turn text + raw LLM final answer.
**Severity:** Medium (OWASP A09 — Security Logging & Monitoring Failures + data exposure)

The RED evidence below was originally captured via the repo CLI (`./smackerel.sh test unit --python`) inside the ephemeral `python:3.12-slim` tooling container. The GREEN re-run for this certification pass was executed with the `ml/.venv` `pytest` directly (no Docker, no live stack, per the no-Docker certification constraint). The regression injects a `completion_fn`, so no live LLM is ever called.

---

## Summary

`ml/app/agent.py` `handle_invoke` emitted two diagnostic `INFO` logs (`agent.invoke.request`, `agent.invoke.envelope`) that carried the raw first user message (≤300 chars) and the raw LLM final answer (≤300 chars) — a Medium OWASP A09 (Security Logging & Monitoring Failures) data-exposure finding that violated spec 076 Hard Constraint 6. The fix redacts both logs to non-reversible length + type metadata while preserving `trace_id` and every shape diagnostic, mirroring the Go agent turn-log's `prompt_sha256` redaction discipline. An adversarial regression guards the behavior (RED → GREEN, same session).

---

## Before Fix (RED) — regression catches the leak

Adversarial regression `ml/tests/test_agent_log_redaction.py` run against the **unfixed** `ml/app/agent.py`. The test plants canaries in the user turn (`CANARY_USER_TURN_TEXT_pii_home_address_42_elm_street_zzz`) and the LLM final answer (`CANARY_FINAL_ANSWER_synthesized_personal_knowledge_yyy`) and asserts neither appears in any INFO record.

```text
$ ./smackerel.sh test unit --python
+ pytest ml/tests -q
s....................................................................... [ 14%]
..F..........................s.......................................... [ 28%]
...
=================================== FAILURES ===================================
___________ test_agent_invoke_does_not_log_raw_user_or_final_content ___________
...
>           assert CANARY_USER not in rendered, (
                f"PII LEAK: raw user turn text reached log record {rec.name}/{rec.levelname}: {rendered!r}"
            )
E           AssertionError: PII LEAK: raw user turn text reached log record smackerel-ml.agent/INFO: 'agent.invoke.request trace_id=trace_redaction_1 model=gpt-4o tools_count=0 messages_count=3 temperature=0.0 max_tokens=1000 first_user_msg=\'{"structured_context": {"input": "CANARY_USER_TURN_TEXT_pii_home_address_42_elm_street_zzz"}}\''
ml/tests/test_agent_log_redaction.py:93: AssertionError
----------------------------- Captured stdout call -----------------------------
2026-06-16 02:43:22,076 INFO smackerel-ml.agent agent.invoke.request trace_id=trace_redaction_1 model=gpt-4o tools_count=0 messages_count=3 temperature=0.0 max_tokens=1000 first_user_msg='{"structured_context": {"input": "CANARY_USER_TURN_TEXT_pii_home_address_42_elm_street_zzz"}}'
2026-06-16 02:43:22,077 INFO smackerel-ml.agent agent.invoke.envelope trace_id=trace_redaction_1 tool_calls_count=0 final_preview='CANARY_FINAL_ANSWER_synthesized_personal_knowledge_yyy' final_type=str
=========================== short test summary info ============================
FAILED ml/tests/test_agent_log_redaction.py::test_agent_invoke_does_not_log_raw_user_or_final_content
1 failed, 509 passed, 2 skipped, 2 warnings in 16.39s
EXIT_CODE=1
```

**Claim Source:** executed. **Interpretation:** the raw user turn (`first_user_msg=…`) and raw LLM answer (`final_preview=…`) leak verbatim into INFO logs (`agent.py:321`, `agent.py:419`). The other 509 ML tests pass — only the new adversarial guard fails, proving it is non-tautological and catches the real defect.

---

## Fix

`ml/app/agent.py` — both diagnostic logs redacted to non-reversible metadata (length + type), preserving `trace_id` and shape diagnostics:

- `agent.invoke.request`: `first_user_msg=%r` (≤300 chars of content) → `first_user_msg_len=%d`.
- `agent.invoke.envelope`: `final_preview=%r` (≤300 chars of content) → `final_len=%d` (kept `final_type=%s`).

No control-flow, return-envelope, or non-log behavior changed.

### Code Diff Evidence

Committed at `eadfada7`:

```text
$ git show eadfada7 -- ml/app/agent.py
diff --git a/ml/app/agent.py b/ml/app/agent.py
index 09e2e9f9..d560991f 100644
--- a/ml/app/agent.py
+++ b/ml/app/agent.py
@@ -320,14 +320,17 @@ async def handle_invoke(
     try:
         logger.info(
             "agent.invoke.request trace_id=%s model=%s tools_count=%d "
-            "messages_count=%d temperature=%s max_tokens=%s first_user_msg=%r",
+            "messages_count=%d temperature=%s max_tokens=%s first_user_msg_len=%d",
             trace_id,
             llm_model,
             len(tools or []),
             len(messages),
             effective_temperature,
             token_budget,
-            next((str(m.get("content"))[:300] for m in messages if m.get("role") == "user"), None),
+            # BUG-076-001 — log only the LENGTH of the first user message,
+            # never its raw content (spec 076 Hard Constraint 6: no raw turn
+            # text in logs; mirrors the Go turn-log's prompt_sha256 discipline).
+            next((len(str(m.get("content"))) for m in messages if m.get("role") == "user"), 0),
         )
         response = await completion_fn(**completion_kwargs)
     except Exception as exc:  # noqa: BLE001 — provider errors must not crash the sidecar
@@ -414,13 +417,15 @@ async def handle_invoke(
         "trace_id": trace_id,
         "processing_time_ms": int((time.time() - start) * 1000),
     }
-    # Diagnostic: log what the LLM actually returned so we can see why
-    # the executor's schema validator rejects the substrate scenarios.
+    # Diagnostic: log the SHAPE of what the LLM returned (counts, length,
+    # type) so we can see why the executor's schema validator rejects the
+    # substrate scenarios — never the raw final-answer content
+    # (BUG-076-001 / spec 076 Hard Constraint 6: no raw turn text in logs).
     logger.info(
-        "agent.invoke.envelope trace_id=%s tool_calls_count=%d final_preview=%r final_type=%s",
+        "agent.invoke.envelope trace_id=%s tool_calls_count=%d final_len=%d final_type=%s",
         trace_id,
         len(tool_calls),
-        (str(final)[:300] if final else None),
+        (len(str(final)) if final else 0),
         type(final).__name__,
     )
     return envelope
```

Files changed: `ml/app/agent.py` (two diagnostic log statements) and the new regression `ml/tests/test_agent_log_redaction.py`.

---

## Test Evidence — After Fix (GREEN)

Re-run this certification pass via the `ml/.venv` `pytest` (no Docker, no live stack; the test injects a `completion_fn`, so no live LLM is involved).

**Adversarial regression — GREEN:**

```text
$ ./.venv/bin/python -m pytest tests/test_agent_log_redaction.py -v
============================= test session starts ==============================
platform linux -- Python 3.12.3, pytest-9.0.3, pluggy-1.6.0 -- ~/smackerel/ml/.venv/bin/python
cachedir: .pytest_cache
rootdir: ~/smackerel/ml
configfile: pyproject.toml
plugins: anyio-4.13.0
collecting ... collected 1 item

tests/test_agent_log_redaction.py::test_agent_invoke_does_not_log_raw_user_or_final_content PASSED [100%]

============================== 1 passed in 0.04s ===============================
EXIT_CODE=0
```

**Full `ml/tests` suite — no regression:**

```text
$ ./.venv/bin/python -m pytest tests -q
s....................................................................... [ 13%]
....................................s................................... [ 27%]
........................................................................ [ 41%]
........................................................................ [ 55%]
........................................................................ [ 69%]
........................................................................ [ 83%]
........................................................................ [ 97%]
...........                                                              [100%]
=============================== warnings summary ===============================
tests/test_nats_consumer_config.py::test_subscribe_all_threads_consumer_config
  ~/smackerel/ml/app/nats_client.py:403: RuntimeWarning: coroutine 'NATSClient._consume_loop' was never awaited
    task = asyncio.create_task(self._consume_loop(subject, sub))
  Enable tracemalloc to get traceback where the object was allocated.
  See https://docs.pytest.org/en/stable/how-to/capture-warnings.html#resource-warnings for more info.

-- Docs: https://docs.pytest.org/en/stable/how-to/capture-warnings.html
513 passed, 3 skipped, 1 warning in 11.64s
EXIT_CODE=0
```

The user-turn canary and the final-answer canary no longer appear in any `INFO` record; both diagnostic logs still fire with `trace_id`. The full suite confirms no ML regression.

---

## Validation

The fix is behavior-preserving, verified against the committed diff (`eadfada7`): only two `logger.info` format strings and their argument expressions changed; the dispatcher's return envelope, control flow, and provider routing are untouched. The three spec scenarios are each asserted by the regression — A01 (no raw user turn in `agent.invoke.request`), A02 (no raw final answer in `agent.invoke.envelope`), and A03 (both diagnostic logs still fire with `trace_id`). The adversarial regression is GREEN and the full `ml/tests` suite is GREEN (513 passed, 3 skipped, exit 0), so the redaction holds with no ML regression.

---

## Audit

OWASP A09 (Security Logging & Monitoring Failures) closure is sound. No raw user-turn text and no raw LLM final-answer content reach `INFO` logs; the diagnostic logs retain only non-reversible length + type metadata plus `trace_id`, preserving operator correlation while removing the data-exposure surface. This matches the Go agent turn-log's `prompt_sha256` discipline and satisfies spec 076 Hard Constraint 6. The change boundary is respected (two log statements in `ml/app/agent.py` plus the new test); no new secret-bearing fields are introduced, and the tracked evidence in this folder redacts real home paths to `~/` (PII-clean).

---

## Change boundary

- **Production code:** `ml/app/agent.py` — 2 diagnostic log statements only
- **New test:** `ml/tests/test_agent_log_redaction.py`
- **Bug artifacts:** `specs/076-assistant-completion-rescope/bugs/BUG-076-001-*/`

Not modified: `ml/app/main.py` log-level default (the standing policy exception `G067-A05-ml-log-level`), any Go source, transport renderers, config, or any other spec's concurrent work.

## Disposition

The code fix and the adversarial regression are complete and GREEN (committed at `eadfada7`). The pre-existing `os.environ.get(KEY, "")` DEFAULT_FALLBACK false positive was cleared at `2fd51b44`.

All 11 `security-to-doc` phases are accounted for in `state.json`: `security` / `implement` / `test` were owned by the R25 security trigger (parent-expanded), and `regression` / `validate` / `audit` / `docs` were executed in this certification pass (also parent-expanded by the orchestrator). `simplify` / `stabilize` / `devops` / `chaos` are honest no-op `phaseStubs` — a two-log-statement metadata redaction has no structural-complexity, stability, deploy, or fault-injection surface those phases could act on.

At `status: in_progress` every security-to-doc content gate passes. The remaining step is the orchestrator's G088 two-commit done-flip: commit this planning truth, then stamp `certifiedAt` / `certifierAgent` and set `status: done` in a second commit. By design, G088 cannot be satisfied from inside the artifact edit.

---

## Completion Statement

BUG-076-001 is content-complete for the `security-to-doc` mode at `status: in_progress`. The ML dispatcher's `agent.invoke.request` and `agent.invoke.envelope` diagnostic `INFO` logs no longer emit raw user turn text or raw LLM final-answer content; they now carry non-reversible length + type metadata (`first_user_msg_len`, `final_len`, `final_type`) plus `trace_id`, satisfying spec 076 Hard Constraint 6 and mirroring the Go turn-log `prompt_sha256` discipline. The adversarial regression (`ml/tests/test_agent_log_redaction.py`) plants canaries in BOTH the user turn and the LLM final answer and was proven RED against the unfixed dispatcher, then GREEN after the fix; the full `ml/tests` suite is GREEN (513 passed, 3 skipped, exit 0). The state-transition guard permits the done transition at `in_progress`; only the orchestrator-owned G088 commit-ordering remains.

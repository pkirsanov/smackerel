# BUG-076-001 User Validation

- [x] Raw conversational content is no longer logged: the ML dispatcher's `agent.invoke.request` and `agent.invoke.envelope` INFO logs now emit length + type metadata (`first_user_msg_len`, `final_len`, `final_type`) instead of raw user-turn text and raw LLM final-answer previews (spec 076 Hard Constraint 6).
- [x] Diagnostic value is preserved: both diagnostic logs still fire with `trace_id` and the existing shape fields (`model`, counts, temperature, `max_tokens`, `tool_calls_count`, `final_type`) — only the two raw-content previews were redacted.
- [x] The fix is proven by an adversarial regression (`ml/tests/test_agent_log_redaction.py`) that plants canaries in BOTH the user turn and the LLM final answer and asserts their absence from every captured INFO record; it was RED against the unfixed dispatcher and GREEN after the fix in the same session.
- [x] No behavior beyond the two log statements changed (Change Boundary: `ml/app/agent.py` two log statements + the new test only); no control-flow, envelope, or non-log behavior was altered.
- [x] The full Python unit suite is GREEN after the fix (no ML regression).

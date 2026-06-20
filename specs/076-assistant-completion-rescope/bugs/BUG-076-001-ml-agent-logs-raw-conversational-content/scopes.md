# Scopes: BUG-076-001 — Redact raw conversational content from ML dispatcher diagnostic logs

Single-file mode (`scopeLayout: single-file`).

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md)

## Scope 1 — Redact `agent.invoke.request` / `agent.invoke.envelope` diagnostic logs

**Status:** Done
**Priority:** P1
**Depends On:** None
**Scope-Kind:** runtime-behavior (privacy/log-hygiene)

### Gherkin Scenarios

Inherits BUG-076-001-A01, A02, A03 from [spec.md](spec.md).

### Implementation Plan

- In `ml/app/agent.py` `handle_invoke`:
  - Replace `first_user_msg=%r` (raw content preview) with
    `first_user_msg_len=%d` (length of the first user message content;
    `0` when absent).
  - Replace `final_preview=%r` (raw content preview) with
    `final_len=%d` (length of stringified final; `0` when `None`),
    keeping the existing `final_type=%s`.
  - Leave every other field (`trace_id`, `model`, counts, temperature,
    `max_tokens`, `tool_calls_count`, `final_type`) unchanged.
- Add `ml/tests/test_agent_log_redaction.py` — adversarial regression
  planting canaries in the user turn AND the LLM final answer, asserting
  neither appears in any captured `INFO` record while both diagnostic
  logs still fire with `trace_id`.

### Change Boundary

- **Allowed:** `ml/app/agent.py` (two log statements only),
  `ml/tests/test_agent_log_redaction.py`,
  `specs/076-assistant-completion-rescope/bugs/BUG-076-001-*/**`.
- **Excluded:** `ml/app/main.py` log-level default (tracked separately
  under policy exception `G067-A05-ml-log-level`), any Go source,
  transport renderers, config, other specs' in-flight work.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-B1-01 | BUG-076-001-A01/A02/A03 | unit | `ml/tests/test_agent_log_redaction.py` | `test_agent_invoke_does_not_log_raw_user_or_final_content` | `./smackerel.sh test unit --python` | No |

### Definition of Done

- [x] BUG-076-001-A01/A02/A03 — adversarial regression authored and proven RED against the unfixed dispatcher, then GREEN after the redaction fix, in the same session. **Evidence:** [report.md](report.md) Before Fix / After Fix. **Claim Source:** executed.
- [x] Fix redacts both diagnostic logs to length+type metadata; diagnostic logs still fire with `trace_id`. **Evidence:** [report.md](report.md) After Fix. **Claim Source:** executed.
- [x] Full `./smackerel.sh test unit --python` suite GREEN after the fix (no ML regression). **Evidence:** [report.md](report.md) After Fix. **Claim Source:** executed.
- [x] Change Boundary respected — only `ml/app/agent.py` (2 log statements) + the new test changed. **Evidence:** [report.md](report.md) change-boundary note. **Claim Source:** executed.
- [ ] Spec-level certification + commit — deferred to the consolidated end-of-sweep `bubbles.validate` pass (not owned by the `security` trigger). **nextRequiredOwner:** `bubbles.validate`.

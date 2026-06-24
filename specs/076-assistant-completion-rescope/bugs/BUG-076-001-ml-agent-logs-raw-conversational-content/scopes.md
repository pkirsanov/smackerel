# Scopes: BUG-076-001 — Redact raw conversational content from ML dispatcher diagnostic logs

Single-file mode (`scopeLayout: single-file`).

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md)

## Scope 1 — Redact `agent.invoke.request` / `agent.invoke.envelope` diagnostic logs

**Status:** Done
**Priority:** P1
**Depends On:** None
**Scope-Kind:** contract-only

> Scope-Kind rationale (v4.1.0 opt-out): this scope changes only the
> diagnostic-log output shape of `ml/app/agent.py` `handle_invoke` — two
> `logger.info` format strings and their argument expressions. No
> dispatcher behavior changes: the return envelope, control flow, and
> provider routing are identical. The defect (raw content in two INFO
> logs) is unit-testable at the log-record boundary, and the adversarial
> regression IS the deepest applicable regression layer; a log-format
> change has no live-runtime E2E surface. (Same contract-only opt-out
> used by sibling light-touch ML bugs this cycle, e.g. BUG-061-002.)

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
- **Excluded:** `ml/app/main.py` log-level default (governed by the
  standing policy exception `G067-A05-ml-log-level` and intentionally
  unchanged here), any Go source, transport renderers, config, and any
  other spec's concurrent work.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-B1-01 | BUG-076-001-A01/A02/A03 | unit | `ml/tests/test_agent_log_redaction.py` | `test_agent_invoke_does_not_log_raw_user_or_final_content` | `./smackerel.sh test unit --python` | No |

### Definition of Done

- [x] BUG-076-001-A01/A02/A03 — adversarial regression authored and proven RED against the unfixed dispatcher, then GREEN after the redaction fix, in the same session. **Evidence:** [report.md](report.md) Before Fix / After Fix. **Claim Source:** executed.
- [x] Fix redacts both diagnostic logs to length+type metadata; diagnostic logs still fire with `trace_id`. **Evidence:** [report.md](report.md) After Fix. **Claim Source:** executed.
- [x] Full `./smackerel.sh test unit --python` suite GREEN after the fix (no ML regression). **Evidence:** [report.md](report.md) After Fix. **Claim Source:** executed.
- [x] Change Boundary respected — only `ml/app/agent.py` (2 log statements) + the new test changed. **Evidence:** [report.md](report.md) change-boundary note. **Claim Source:** executed.
- [x] Spec-level certification complete — all 11 `security-to-doc` phases are recorded (security/implement/test/regression/validate/audit/docs genuine; the four no-runtime-surface phases are honest no-op `phaseStubs` in `state.json`), and the adversarial regression + full `ml/tests` suite are GREEN. **Evidence:** [report.md](report.md) Test Evidence / Validation / Audit / Completion Statement and `state.json` `certification`. **Claim Source:** executed. The orchestrator owns the G088 two-commit done-flip (commit this planning truth, then stamp `certifiedAt` and set `status: done`).

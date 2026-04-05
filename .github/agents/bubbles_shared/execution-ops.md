# Execution Operations

Use this file for bounded retry behavior and auxiliary workflow operations that should not bloat the main governance index.

## Lessons-Learned Memory

When the repository maintains a lessons-learned memory, agents may append concise entries describing:

- problem
- root cause
- fix
- when the lesson applies

Keep lessons short and actionable.

## Self-Healing Loop Protocol

When a failure is local and fixable, agents may attempt a bounded self-healing loop.

Rules:

- narrow retries only
- maximum three retries for the same failure context
- maximum one nesting depth
- if a new failure appears, it still counts against the same retry budget
- escalate or stop when bounded retries are exhausted

## Escalation Protocol (3-Strike Rule)

When an agent encounters a failure, it MUST follow this escalation ladder:

| Strike | Action | Context Width |
|--------|--------|---------------|
| 1 | Fix attempt with full error context + surrounding code | Broad |
| 2 | Fix attempt with only the failing file + error message | Narrower |
| 3 | Fix attempt with only the specific function/block + error | Narrowest |
| After 3 | STOP — escalate to orchestrator or mark `blocked` | N/A |

**After 3 consecutive failures on the same issue:**
- The agent MUST stop attempting fixes
- Report: what was tried, what failed, and a concrete recommendation
- The orchestrator may route to a different specialist or mark the scope blocked
- Agents MUST NOT thrash beyond 3 strikes — bad work is worse than stopped work

**Escalation output format:**
```
ESCALATION: 3-strike limit reached
ISSUE: [1-2 sentences describing the persistent failure]
ATTEMPTED: [what was tried at each strike]
RECOMMENDATION: [route to different specialist | mark blocked | ask user]
```

## Atomic Commit Protocol

If a workflow mode explicitly enables automatic commit behavior, commits must remain scoped to the validated unit of work. Do not use auto-commit settings to hide incomplete or unverified work.

## Timeout Policy

All long-running commands or polling loops must have explicit time bounds. Do not wait indefinitely.

On timeout:

1. record the timeout
2. stop or kill hanging work when possible
3. report the bounded failure
4. do not silently continue as if validation succeeded
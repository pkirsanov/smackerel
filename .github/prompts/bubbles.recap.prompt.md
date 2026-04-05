---
agent: bubbles.recap
---

Review this conversation and give me a quick recap:

1. **Done** — What was completed (commits, file changes, fixes, decisions)
2. **In Progress** — Anything started but not finished
3. **Open** — Requests mentioned but not acted on
4. **Workflow Continuation** — one recommended `/bubbles.workflow ...` command, not raw specialist commands

Also check `specs/*/state.json` for any active spec work and include relevant status.

End with a `## CONTINUATION-ENVELOPE` block carrying `target`, `targetType`, `intent`, `preferredWorkflowMode`, `tags`, `reason`, and `directAgentOnly`.

Keep it short. Use bullet points. No fluff.

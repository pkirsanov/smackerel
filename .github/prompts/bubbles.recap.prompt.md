---
agent: bubbles.recap
---

Review this conversation and give me a quick recap:

1. **Done** — What was completed (commits, file changes, fixes, decisions)
2. **In Progress** — Anything started but not finished
3. **Open** — Requests mentioned but not acted on
4. **Next Priority Candidate** — at most one read-only candidate, labeled `not started`
5. **Workflow Continuation** — one `/bubbles.workflow ...` command only when concrete non-terminal work remains

Also check `specs/*/state.json` for any active spec work and include relevant status.

End with a `## CONTINUATION-ENVELOPE` block carrying `target`, `targetType`, `intent`, `preferredWorkflowMode`, `tags`, `reason`, and `directAgentOnly`.
After terminal completion, use the schema-valid redacted projection: `repositoryRoot: <redacted-local-root>`, `repositoryResolution.pathVisibility: redacted`, `repositoryResolution.actionable: false`, `target: none`, and `preferredWorkflowMode: none`.
Never start candidate work without a new explicit request.

Keep it short. Use bullet points. No fluff.

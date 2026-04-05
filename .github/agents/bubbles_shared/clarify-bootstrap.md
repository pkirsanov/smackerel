# Clarify Bootstrap

Always load:
- `critical-requirements.md`
- `artifact-ownership.md`
- Feature `spec.md`
- Feature `design.md` and `scopes.md` when they exist
- User-provided requirement or design context from `$ADDITIONAL_CONTEXT`

Load on demand:
- Project architecture/API/testing docs only for claims being clarified
- Route files, models, migrations, or UI routes needed to verify referenced behavior
- `test-fidelity.md` and `consumer-trace.md` only when a clarification changes those obligations

Constraints:
- Track work with `manage_todo_list`
- One feature-resolution attempt, then fail fast if the target is still ambiguous
- No redundant rereads without a new reason
- Do not directly edit foreign-owned planning artifacts during clarification; route the required changes to the owning agent
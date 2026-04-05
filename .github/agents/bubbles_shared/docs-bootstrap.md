# Docs Bootstrap

Always load:
- `critical-requirements.md`
- `artifact-ownership.md`
- `managed-docs.md`
- The effective managed-doc registry (framework defaults in `bubbles/docs-registry.yaml` plus any project-owned overrides)
- Feature `spec.md`
- Feature `design.md` and `scopes.md` when they exist
- Only the managed docs targeted by the requested review scope

Load on demand:
- Route files, models, migrations, or UI routes needed to verify documented claims
- `state-gates.md` and `evidence-rules.md` only when documentation updates are tied to completion or validation claims
- Project governance docs only when they define a rule the documentation must reflect exactly

Constraints:
- Track work with `manage_todo_list`
- No redundant rereads without a new reason
- Prefer targeted managed-doc loads over bulk `docs/` reads
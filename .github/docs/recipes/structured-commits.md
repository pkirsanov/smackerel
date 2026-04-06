# Recipe: Structured Git Commits

> *"Get two birds stoned at once."*

Use `autoCommit` to get structured git history that tracks scope-by-scope progress.

## Scope-Level Commits (Recommended)

One commit per completed scope. Clean, reviewable, revertable.

```
/bubbles.workflow  full-delivery for 042 autoCommit: scope
```

After each scope reaches Done (all DoD items checked with evidence), the agent runs:
```
git add -A
git commit -m "bubbles(042/01): database-schema — Done ..."
```

## DoD-Level Commits (Fine-Grained)

One commit per validated DoD item. Maximum granularity.

```
/bubbles.workflow  full-delivery for 042 autoCommit: dod
```

## What You Get

```bash
git log --oneline
# abc1234 bubbles(042/02): api-handlers — Done
# def5678 bubbles(042/01): database-schema — Done
```

Roll back a scope:
```bash
git revert abc1234  # cleanly reverts scope 02
```

See what a scope changed:
```bash
git diff def5678..abc1234  # exact diff for scope 02
```

## Combine with Git Isolation

For feature branches:
```
/bubbles.workflow  full-delivery for 042 autoCommit: scope gitIsolation: true
```

This creates a branch, commits per scope, and you merge when complete.

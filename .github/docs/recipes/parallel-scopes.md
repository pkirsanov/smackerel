# <img src="../../icons/bubbles-glasses.svg" width="28"> Parallel Scope Execution

> *"Decent! Two scopes at once? That's getting two birds stoned at once."*

## Situation

Your spec has multiple independent scopes (no mutual dependencies) and you want to speed up execution by running them in parallel via git worktrees.

## Recipe

First, preview the parallelization plan (dry run):

```
/bubbles.workflow  specs/<NNN-feature> mode: full-delivery parallelScopes: dag-dry
```

If the plan looks good, execute:

```
/bubbles.workflow  specs/<NNN-feature> mode: full-delivery parallelScopes: dag maxParallelScopes: 2
```

## How It Works

1. Reads scope dependency graph from `scopes.md` "Depends On" declarations
2. Identifies parallelizable scope sets (no mutual dependencies)
3. Creates a git worktree per parallel scope
4. Runs `bubbles.implement` in each worktree concurrently
5. Merges worktrees back via git merge
6. Runs shared quality phases (test, validate, audit) on merged result

## When to Use

- Spec has 4+ scopes with independent dependency groups
- Scopes touch different files/directories (low merge conflict risk)
- You want faster delivery on a well-planned spec

## When NOT to Use

- Scopes heavily overlap in the same files
- You're unsure about scope dependencies
- First time using the feature (start with `dag-dry`)

## Caution

- Merge conflicts are a new failure class — the self-healing loop handles them
- Start with `maxParallelScopes: 2` before trying 3 or 4
- Sequential execution (default) is always the safest option

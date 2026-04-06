# Recipe: Code Review Directly

> *"From parts unknown, I can smell what's broken in the code."*

---

## The Situation

You want a direct engineering review on a repository, service, package, module, path, or symbol before deciding what should be fixed immediately.

This is not a full workflow run. It is a lightweight engineering assessment pass.

## The Command

```
/bubbles.code-review  profile: engineering-sweep scope: path:services/gateway output: summary-doc
```

Or for the whole repo:

```
/bubbles.code-review  scope: full-repo output: summary-only
```

## Profiles

Review profiles are defined in `bubbles/code-review.yaml`.

| Profile | Use It For |
|---------|------------|
| `engineering-sweep` | balanced technical review across quality, drift, tests, and docs |
| `correctness-first` | correctness, validation, and test health |
| `maintainability-first` | complexity, duplication, and cleanup |
| `release-readiness` | shipping-focused risk review |
| `security-first` | security and compliance focused review |

## What You Get Back

Every run uses the same output shape:

1. Review scope
2. Engineering summary
3. Findings by lens
4. Prioritized actions
5. Artifact outputs

## When To Use This Instead Of A Workflow

Use `bubbles.code-review` when:
- you want to inspect code directly
- you do not want gates or done-state progression
- you want a summary document first
- you want engineering findings only

Use `bubbles.system-review` instead when:
- you want feature, component, UX, or journey review
- you want product-level or system-level prioritization
- you want findings promoted into specs for cross-cutting work

> **💡 Tip:** Run `/bubbles.retro hotspots` before launching a code review to identify the highest-churn and highest-bug-fix-ratio files. This helps you target the review scope at the areas that matter most — bug magnets, co-change coupling hotspots, and knowledge silos.
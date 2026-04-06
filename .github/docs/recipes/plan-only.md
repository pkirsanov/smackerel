# Recipe: Plan Only (No Implementation)

> *"Jim, you need a plan."*

---

## The Situation

You want to think through a feature — requirements, design, scope breakdown — without writing code yet.

This also applies when an existing feature's active planning artifacts are stale and need reconciliation before implementation resumes.

## The Command

```
/bubbles.workflow  specs/050-my-new-feature mode: product-discovery

/bubbles.workflow  specs/050-my-new-feature mode: product-discovery grillMode: required-on-ambiguity backlogExport: tasks

/bubbles.workflow  specs/019-booking-page mode: spec-scope-hardening backlogExport: tasks

/bubbles.workflow  specs/019-booking-page mode: redesign-existing backlogExport: tasks
```

**Phases:**
- New feature: analyze → bootstrap → harden → docs → validate → audit
- Existing feature: harden planning artifacts only, or redesign when the active truth is stale enough that planning must be rebuilt

## Or Step by Step

### 1. Requirements

```
/bubbles.analyst  describe the feature: <your idea>
```

### 2. Design

```
/bubbles.design  create design for <feature>
```

### 3. Scope Breakdown

```
/bubbles.plan  create scopes for <feature>
```

Planning ownership is strict: if later validation or hardening finds missing Gherkin, Test Plan, DoD, or `uservalidation.md` structure, those agents route the changes back to `bubbles.plan` instead of editing planning artifacts directly.

## What You Get

A complete `specs/NNN-feature/` folder with:
- `spec.md` — requirements and acceptance criteria
- `design.md` — architecture and data models
- `scopes.md` — implementable scopes with DoD
- `uservalidation.md` — validation checklist
- `scenario-manifest.json` — scenario contract registry for active changed behavior
- `state.json` — version 3 control-plane state with certification + policy provenance

If you use `backlogExport: tasks` or `backlogExport: issues`, `bubbles.plan` also emits copy-ready backlog sections derived from the scopes without changing `scopes.md` as the source of truth.

Ready to implement whenever you are.

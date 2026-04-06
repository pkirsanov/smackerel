# Recipe: Reconcile Or Redesign An Existing Feature

> *"Same lot, boys. New trailer."*

---

## The Situation

The feature already exists, but the active artifacts are out of sync with reality or the intended behavior has changed enough that incremental improvement is no longer safe.

Typical signals:

- `spec.md` still describes flows or actors you no longer want
- `design.md` contains obsolete contracts, models, or rollout assumptions
- `scopes.md` still plans work for behavior that is no longer valid
- validation says the state is stale, but the deeper problem is stale planning, not just stale completion flags

---

## Choose The Right Workflow

### Reconcile stale state only

Use this when the product story is still basically correct, but artifact state or scope status drifted from reality.

```text
/bubbles.workflow  reconcile-to-doc for booking
```

### Improve an existing feature

Use this when the existing feature is fundamentally right and you want competitive or quality improvements without rewriting the product story.

```text
/bubbles.workflow  improve-existing for booking
```

### Redesign an existing feature

Use this when requirements, UX, design, and scopes all need reconciliation before delivery.

```text
/bubbles.workflow  redesign-existing for booking
```

Optional tags:

```text
/bubbles.workflow  redesign-existing for booking grillMode: required-on-ambiguity socratic: true socraticQuestions: 4 backlogExport: tasks
```

---

## What `redesign-existing` Does

1. Re-runs analysis on the current feature instead of trusting old requirements.
2. Reconciles stale analyst-owned sections so only one active business truth remains.
3. Reconciles UX wireframes and flows, suppressing stale screens and journeys.
4. Reconciles `design.md` so obsolete contracts and architecture decisions are isolated or removed.
5. Rebuilds active scopes so stale scopes are no longer executable.
6. Delivers the redesigned implementation through test, validation, audit, chaos, and docs.

---

## What Good Reconciliation Looks Like

- Active sections show the current truth only.
- Old material is either deleted or moved under clearly labeled superseded sections.
- Stale scopes are removed from the active execution inventory.
- The next implement phase can trust the planning artifacts again.

If you are not sure whether you need reconcile, improve, or redesign, ask:

```text
/bubbles.super  do I need reconcile-to-doc, improve-existing, or redesign-existing for booking?
```
# Recipe: Review First, Then Improve

> *"Watch the whole show first. Then decide what to fix."*

---

## The Situation

You have an existing feature or product area and want to improve it, but you do not want to jump straight into a workflow before understanding the real issues.

## Step 1: Review The Right Thing

If you want data-driven analysis of where the problems actually are before choosing a review type:

```
/bubbles.retro hotspots
```

This surfaces bug magnets, co-change coupling, bus factor risks, and churn trends — hard data to guide what you review and where.

If the concern is engineering-only:

```
/bubbles.code-review  scope: service:gateway output: summary-doc
```

If the concern is feature/system quality:

```
/bubbles.system-review  mode: full scope: feature:booking output: summary-doc
```

## Step 2: Choose Follow-Through

If the findings point to code quality, drift, or simplification work inside an existing feature:

```
/bubbles.workflow  improve-existing for booking
```

If you want that improvement workflow to audit stale or redundant active specs once before it starts changing code:

```
/bubbles.workflow  improve-existing for booking specReview: once-before-implement
```

That one-shot review runs before implementation-capable work starts, routes stale-truth problems into reconciliation or redesign if needed, and does not repeat on every retry round.

If the findings show stale state or stale planning that must be reconciled before the existing work can be trusted again:

```
/bubbles.workflow  reconcile-to-doc for booking
```

If the findings show the existing feature needs a substantial rewrite rather than incremental improvement:

```
/bubbles.workflow  redesign-existing for booking
```

If the findings point to operational or reliability hardening:

```
/bubbles.workflow  stabilize-to-doc for booking
```

If the findings show broad quality, drift, and robustness problems:

```
/bubbles.workflow  harden-gaps-to-doc for booking
```

If the review produced cross-cutting product changes that need new planning artifacts:

```
/bubbles.system-review  mode: full scope: feature:booking output: create-specs
```

Then continue with `reconcile-to-doc`, `redesign-existing`, or `improve-existing` once the promoted specs exist.

## Why No Dedicated Review Workflow?

Review is diagnosis. Workflows are execution.

That split keeps reviews lightweight and flexible while letting existing workflows handle the heavy gated phases only when you are ready to act.
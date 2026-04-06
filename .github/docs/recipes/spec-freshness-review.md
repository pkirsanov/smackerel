# Recipe: Spec Freshness Review

> *"Gary can see right through it, boys. That spec expired three refactors ago."*

---

## The Situation

Your specs were written months ago. Code has evolved — bug fixes, refactors, dependency changes — but the specs haven't been updated. Now maintenance agents (simplify, security, stabilize) are treating stale specs as truth and making wrong decisions.

Typical signals:

- Maintenance agents flag correct code as "non-compliant" because the spec describes an old design
- Simplification keeps old patterns alive because the spec says they're correct
- Security reviews miss real attack surfaces because the spec describes a different auth architecture
- Code reviews compare against specs that describe deleted or rebuilt modules
- You're not sure which specs are still trustworthy

---

## Quick Check — Are Any Specs Stale?

```text
/bubbles.spec-review  all depth: quick
```

This does a fast scan (file existence + git history only) across all specs and gives you a trust map.

---

## Deep Review — One Feature

```text
/bubbles.spec-review  specs/005-booking depth: thorough
```

Full behavioral + contract analysis for a single feature: API endpoints, DB schemas, Gherkin scenarios, file paths, git delta.

---

## Prepare for Maintenance Work

Before running simplify, security, or stabilize on a feature, check spec freshness first:

```text
/bubbles.spec-review  maintenance: security
```

This produces a maintenance context block telling the security agent which specs to trust and which to ignore.

If you want a delivery workflow to perform that audit once automatically before it starts changing legacy code, append the one-shot execution tag:

```text
/bubbles.workflow  specs/005-booking mode: improve-existing specReview: once-before-implement
```

That uses `bubbles.spec-review` as a pre-implementation hook, then continues the workflow only after stale or redundant active artifacts are reconciled.

---

## Full Workflow — Audit All Specs Then Report

```text
/bubbles.workflow  spec-review-to-doc for all
```

Runs the `spec-review-to-doc` workflow mode: select specs → audit freshness → produce trust report → sync docs.

---

## After Spec Review

Based on trust classification:

| Trust Level | Next Step |
|-------------|-----------|
| **CURRENT** | No action needed. Spec is trustworthy. |
| **MINOR_DRIFT** | Flag for update when convenient. `sunnyvale lets-get-organized` (design) |
| **MAJOR_DRIFT** | Immediate spec update needed. `sunnyvale peanut-butter-and-jam` (gaps) |
| **OBSOLETE** | Delete or full rewrite. `sunnyvale same-lot-new-trailer` (redesign-existing) |
| **PARTIAL** | Update drifted scopes. `sunnyvale supply-and-command` (plan) |

---

## Combine with Other Recipes

- **Before security review:** `sunnyvale laser-eyes` → `sunnyvale safety-always-off`
- **Before simplification:** `sunnyvale laser-eyes` → `sunnyvale water-under-the-fridge`
- **Before stabilization:** `sunnyvale laser-eyes` → `sunnyvale just-fixes`
- **Before improvement:** `sunnyvale laser-eyes` → `sunnyvale survival-of-the-fitness`

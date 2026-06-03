# Recipe: Release Planning

> *"Plans within plans, boys. Phase one is just the introduction."* — Sonny "Iron Lung" Smith.

---

## The Situation

You need to produce or refresh a release packet for a phase of your product — vision, features, actions, business plan, deployment, marketing, monetization, ops-scalability — and have it stay honest about what's actually delivered vs planned vs aspirational.

## Quick Start — Natural Language

```
# Easiest — let bubbles.releases figure out mode and scope:
/bubbles.releases  plan v1.5 release

# Or ask super for guidance first:
/bubbles.super  I need to plan the next release, where do I start?
```

## Prerequisites — Product Direction Surfaces Trio

`bubbles.releases` REFUSES to produce a release packet for a repo missing the canonical Product Direction Surfaces trio:

- `docs/INVESTOR_OVERVIEW.md`
- `docs/Product-Principles.md`
- `.github/instructions/product-principles.instructions.md`

If the trio is missing, `bubbles.releases` routes to `bubbles.setup` first. See [`docs/guides/PRODUCT_DIRECTION_SURFACES.md`](../guides/PRODUCT_DIRECTION_SURFACES.md).

## The Steps (Manual Control)

### Step 1: Verify Trio + Phase Model

```
/bubbles.super  is my repo ready for release planning?
```

This invokes the `bubbles-repo-readiness` skill which checks trio presence + phase model alignment.

### Step 2: Bootstrap A Fresh Phase Packet

```
/bubbles.releases  plan v2.0 release
```

**What happens:** The releases agent reads direction inputs (constitution, design docs, capability ledger, prior packets), reconciles capability truth, builds carry-forward table, and writes the canonical 8-doc packet at `docs/releases/v2.0/`.

**You'll get:** 8 release-packet docs + an updated Phase Overview table in `INVESTOR_OVERVIEW.md`.

### Step 3: Refresh An Existing Packet

```
/bubbles.releases  refresh v1.0 features and marketing
```

**What happens:** The releases agent reads current capability state, updates the specified docs, surfaces drift, and reports any "delivered" claims that no longer trace to evidence.

### Step 4: Add A New Plan To An Existing Phase

```
/bubbles.releases  add a paired-product companion plan to v1.5
```

**What happens:** The releases agent creates `docs/plans/05-v1.5/P0N-paired-companion.md` and updates the v1.5 `features.md` Plan-to-Release Traceability table to reference the new plan.

### Step 5: Cross-Product Coordinated Release

```
/bubbles.releases  coordinate v2.0 across this repo and a paired repo paired_repo: /path/to/paired/repo
```

**What happens:** Produces coordinated plans in BOTH repos with cross-references, shared schema versioning, and matching boundary statements.

### Step 6: Surface New Principles That Emerged

If release planning surfaces a new product principle (e.g., a cross-product boundary), do NOT update `Product-Principles.md` from `bubbles.releases`. Route to `bubbles.analyst`:

```
/bubbles.analyst  surface a new product principle for the cross-product paired-companion boundary
```

This invokes the `bubbles-product-principle-discovery` skill.

## The Honest-Capability Rule

`bubbles.releases` enforces that every "delivered" claim in `features.md` MUST trace to one of:
- A spec marked done in `state.json` with passing certification
- A row in `Capability_Ledger.md` (when the repo uses one)
- A design-doc operational runbook section explicitly marked delivered

Marketing copy that fabricates capabilities is REFUSED. If you want aspirational copy, mark the capability "planned" or "in-progress" — never "delivered."

## Common Modes

| You want to | Use |
|-------------|-----|
| First release packet for a brand-new phase | `mode: bootstrap` (default when packet missing) |
| Reconcile existing packet against current state | `mode: refresh` (default when packet exists) |
| Add a new plan file to an existing phase | `mode: extend` |
| Plan a release that spans two repos | `mode: cross-product`, `paired_repo: <path>` |
| Restrict updates to specific docs | `docs: vision,features,marketing` |
| Get a clarifying interview before drafting | `socratic: true`, `socraticQuestions: 3` |

## Carry-Forward Discipline

For multi-phase repos, every phase's `features.md` MUST include a "Carried Forward From Prior Phases" table. `bubbles.releases` builds this automatically by reading the prior phase's `features.md`. Quietly dropping a prior-phase capability is FORBIDDEN — either carry it forward or explicitly mark it deprecated with rationale.

## Vision Restatement Discipline

Every phase's `vision.md` MUST be self-contained. `bubbles.releases` restates the vision inline rather than cross-referencing prior `vision.md`. This makes each phase's vision readable on its own.

## Related Recipes

- [`setup-project.md`](setup-project.md) — Bootstrap a new repo with the trio
- [`new-feature.md`](new-feature.md) — Take a single feature through the full pipeline
- [`framework-ops.md`](framework-ops.md) — Framework maintenance
- [`update-docs.md`](update-docs.md) — Refresh docs without touching release packets

## Related Skills

- [`bubbles-product-principle-discovery`](../../skills/bubbles-product-principle-discovery/SKILL.md) — Surface principles from existing repo evidence
- [`bubbles-repo-readiness`](../../skills/bubbles-repo-readiness/SKILL.md) — Verify trio + phase model presence

## Related Convention

- [`docs/guides/PRODUCT_DIRECTION_SURFACES.md`](../guides/PRODUCT_DIRECTION_SURFACES.md) — The convention `bubbles.releases` enforces

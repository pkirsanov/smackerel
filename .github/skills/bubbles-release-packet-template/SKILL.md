---
name: bubbles-release-packet-template
description: Use the canonical Bubbles release-packet template when authoring or refreshing a phase release packet under docs/releases/<phase>/. Use when starting a new phase, refreshing an existing phase, or auditing whether a release-packet folder has the correct shape and location. Owner is bubbles.releases — never bubbles.train, bubbles.devops, or bubbles.plan.
---

# Bubbles Release-Packet Template

## Goal
Produce release packets that live at the canonical location with the canonical 8-doc shape, so `release-packet-location-guard.sh` passes and downstream readers (operators, investors, the trio convention) get a predictable layout per phase.

## When to use
- Bootstrapping a new phase under `docs/releases/<phase>/`
- Refreshing an existing phase's release packet
- Adding a new doc to a phase (NEVER — the 8-doc set is fixed)
- Auditing a release-packet folder for shape/location compliance

## Canonical Location (BLOCKING)
Release packets MUST live at:

```
docs/releases/<phase>/
```

where `<phase>` is the lowercase phase name as it appears in the `docs/INVESTOR_OVERVIEW.md` Phase Overview table (e.g., `mvp`, `v1.0`, `v1.5`, `v2.0`).

## Canonical Shape (BLOCKING)
Exactly 8 docs per phase, no more and no fewer:

| File | Purpose |
|------|---------|
| `vision.md` | Self-contained phase vision; restated inline (no cross-references to prior phases' vision.md) |
| `features.md` | Capability list with carry-forward table; every claim traces to spec/ledger evidence. For phases that opt into machine reconciliation (Gate G101), carry a `<!-- bubbles:reconciled-packet schemaVersion=1 phase=<phase> -->` header plus a per-feature `<!-- bubbles:feature id=<id> spec=<spec-dir\|none> delivery=required\|optional\|carried\|deferred-to:<phase> -->` annotation; every `delivery=required` feature MUST bind a real spec dir (`release-delivery-reconciliation-guard.sh`) |
| `actions.md` | Concrete actions for the phase, including open questions and route_required dispatches |
| `business-plan.md` | Phase-scoped business plan |
| `deployment.md` | Phase-scoped deployment plan |
| `marketing.md` | Phase-scoped marketing copy (NEVER fabricates capabilities) |
| `monetization.md` | Phase-scoped monetization model + assumptions |
| `ops-scalability.md` | Phase-scoped operations + scalability plan |

## Forbidden In A Release-Packet Folder (BLOCKING)
- ❌ `state.json` — release packets are managed-docs, NOT workflow artifacts. They have no spec lifecycle. The state-transition guard does not apply.
- ❌ `README.md` — `vision.md` is the entry doc. Routing/dispatch information belongs in `actions.md`.
- ❌ Any 9th doc not listed above.

## Forbidden Locations (BLOCKING)
The following alternative locations are FORBIDDEN. `release-packet-location-guard.sh` rejects them mechanically:

- `specs/_ops/RELEASE-*/`
- `specs/releases/<phase>/`
- `docs/RELEASE-*/`
- `docs/release-*/`
- Anything outside `docs/releases/<phase>/`

If you find a release packet at one of these locations, route to `bubbles.releases` to relocate it.

## Companion Artifacts (not in the packet, but required alongside)
| Artifact | Owner | Notes |
|----------|-------|-------|
| `docs/INVESTOR_OVERVIEW.md` Phase Overview row | bubbles.releases | Updated when a packet is authored or refreshed; rest of file belongs to the trio convention owner |
| `docs/plans/<phase>/P0N-*.md` | bubbles.releases | Optional per-plan files when the repo uses the plans-and-features split |
| `docs/generated/release-reconciliation-<phase>.md` | bubbles.releases | OPTIONAL generated Gate G101 audit note (derived from `release-delivery-reconciliation-guard.sh`). Lives under `docs/generated/`, NOT in the packet folder — so the "no 9th doc" rule and `release-packet-location-guard.sh` are unaffected. |

## Ownership (BLOCKING)
- **Owner:** `bubbles.releases` (Sonny "Iron Lung" Smith)
- **NEVER:** `bubbles.train`, `bubbles.devops`, `bubbles.plan`, `bubbles.implement`, `bubbles.analyst`
- New product principles that emerge during release planning are routed to `bubbles.analyst` via the `bubbles-product-principle-discovery` skill — they are NEVER added directly to `Product-Principles.md` from `bubbles.releases`.

## Anti-patterns
- ❌ Adding `state.json` "to track release status" — release packets have no workflow status
- ❌ Adding `README.md` as an entry doc — `vision.md` IS the entry doc
- ❌ Placing release packets under `specs/_ops/RELEASE-*` because they "feel like ops work" — they are managed-docs and belong under `docs/releases/<phase>/`
- ❌ Cross-referencing prior phases' `vision.md` instead of restating inline
- ❌ Quietly dropping a prior-phase capability instead of carrying it forward or marking it deprecated
- ❌ Marketing copy claiming capabilities that have no spec / ledger / runbook trace

## Authoritative modules
- `agents/bubbles.releases.agent.md` — owner agent
- `docs/recipes/release-planning.md` — operator recipe
- `docs/guides/PRODUCT_DIRECTION_SURFACES.md` — trio convention this packet aligns with
- `bubbles/scripts/release-packet-location-guard.sh` — mechanical location guard
- `skills/bubbles-feature-template/SKILL.md` — sibling template for per-feature artifacts (different shape, different location)

---
name: bubbles-datastore-isolation
description: Enforce production stateful-backing-store isolation — stateful stores (PostgreSQL, Redis, message bus) are per-product/bundled by DEFAULT and MUST NOT be shared across products unless they clear the four-part "share cleanly" bar (per-product DB+role isolation, independent per-product backup AND restore, no cross-product migration coupling, connection-pool isolation). Use when deciding whether two products may share a database/cache/bus in production; when designing production or shared-host topology; when a spec proposes a shared stateful store; when distinguishing shared-SAFE stateless platform capabilities (telemetry/observability, LLM inference) from bundle-only stateful infrastructure; or when reviewing connection-pool, backup/restore, or migration blast-radius coupling. NOT for test-topology isolation.
---

# Datastore Isolation (Production Topology Doctrine)

## Portability

Portable governance skill governing **production** topology decisions. The doctrine is repo-neutral; the shared-services mechanism it cites (a shared-services selector + shared-host observability/inference specs) is an ecosystem realization, referenced by spec number, not baked into the rule. Keep this skill free of hostnames, ports, IPs, and repo-only commands; resolve execution through `.specify/memory/agents.md` per [agent-common.md](../../agents/bubbles_shared/agent-common.md).

## The Doctrine (one sentence)

**Share stateless platform capabilities; bundle stateful product-critical infrastructure.**

## Default = Per-Product / Bundled

Stateful backing stores — **PostgreSQL, Redis, message bus** (and any queue, search index, or object store that holds product-critical mutable state) — are **per-product / bundled by default**. Do NOT share a stateful store across otherwise-independent products.

Sharing a stateful store silently **couples three blast radii** that were independent:

1. **Migration blast-radius** — a schema migration for product A can lock, break, or block product B.
2. **Backup / restore** — you cannot restore product A to a point in time without dragging product B's data with it (or corrupting it).
3. **Connection-pool exhaustion** — product A's traffic spike starves product B of connections; a pool leak in one takes down both.

The whole point of independent products is independent failure domains. A shared stateful store throws that away for a marginal infrastructure saving.

## Use This Skill When

- Deciding whether two products may share a database, cache, or message bus in production.
- Designing production topology, a shared host, or a home-lab layout for more than one product.
- A spec, design, or compose file proposes pointing a second product at an existing stateful store.
- Distinguishing which platform capabilities are safe to share from which must be bundled.
- Reviewing a change for hidden connection-pool, backup/restore, or migration coupling.

## The "Share Cleanly" Bar (ALL four, or bundle it)

A shared stateful store is permissible **ONLY** if it clears **every** one of these. If **any** is unmet, keep it bundled — there is no partial credit.

1. **Per-product database + role isolation** — each product has its own logical database and its own DB role/credentials; no product can read or write another's tables.
2. **Independent per-product backup AND restore** — each product can be backed up and, critically, **restored** to a point in time without touching another product's data. (Backup alone is not enough; the restore must be independently exercisable — see [bubbles-backup-bcdr-doctrine](../bubbles-backup-bcdr-doctrine/SKILL.md).)
3. **No cross-product migration coupling** — one product's schema migration cannot lock, break, or gate another product's migrations or runtime.
4. **Connection-pool isolation** — one product's connection usage (spike, leak, exhaustion) cannot starve another; pools are partitioned per product.

```
Proposed shared stateful store?
        │
        ▼
Per-product DB + role isolation? ── no ──▶ BUNDLE
        │ yes
        ▼
Independent backup AND restore? ── no ──▶ BUNDLE
        │ yes
        ▼
No cross-product migration coupling? ── no ──▶ BUNDLE
        │ yes
        ▼
Connection-pool isolation? ── no ──▶ BUNDLE
        │ yes
        ▼
   SHARE (documented, with the 4 proofs recorded)
```

Clearing the bar is the exception, not the goal. When you do clear it, record the four proofs in the topology/design doc so an auditor can see it was earned, not assumed.

## Stateful vs Shared-Safe (the heart of the doctrine)

The line is **stateful vs stateless/read-mostly**, not "infrastructure vs application."

| Capability class | Examples | Production default | Why |
|---|---|---|---|
| **Stateful, product-critical** | PostgreSQL, Redis (as a data store), message bus, queue, search index, object store | **Bundle per product** | Holds mutable product-critical state; coupling ties migration + backup/restore + pool failure domains together |
| **Stateless / read-mostly platform** | Telemetry / observability (metrics, logs, traces), LLM inference / model serving | **Safe to share on a shared host** | No product-critical mutable state per consumer; a shared instance does not couple backup/restore or migrations |

Shared-safe capabilities are shared through an explicit **shared-services selector**: each product's deploy adapter declares, per capability, `shared` vs `bundled`, resolved fail-loud (an unset capability is a refusal, never an implicit default). The shared realization is the mechanism — e.g. shared-host observability and shared-host inference stood up as host singletons and consumed by env-var seams (see the ecosystem's **shared-services selector spec 032** and the **shared-host observability (014)** / **shared-host inference/Ollama (015)** specs). A stateful store is **never** an eligible selector value.

> Even a shared-SAFE capability keeps its own isolation posture: shared observability MUST reject test-labelled telemetry, and per-product scrape/label seams keep signals attributable. Sharing the *capability* does not mean merging the *data*.

## Production vs Test Boundary (do NOT duplicate)

This skill governs **PRODUCTION** topology — what runs long-lived and holds real data. It is deliberately distinct from the **test** topology skills:

| Concern | Skill | Governs |
|---|---|---|
| Can two products share one prod DB? | **this skill** | Production stateful topology |
| Must a test provision its own ephemeral store? | [bubbles-test-environment-isolation](../bubbles-test-environment-isolation/SKILL.md) | Ephemeral per-run test backing stores |
| Must test code never write to prod stores/metrics/backups? | [bubbles-env-pollution-isolation](../bubbles-env-pollution-isolation/SKILL.md) | Test → prod write prohibition (G115) |

Do not restate the test rules here, and do not apply this production doctrine to ephemeral test stacks (which are always per-run and disposable regardless). If a decision is about a test stack, route to those two skills; if it is about a long-lived production store, stay here.

## Forbidden vs Required

| ❌ Forbidden | ✅ Required |
|---|---|
| Pointing product B at product A's PostgreSQL "to save a container" | Bundle B's own PostgreSQL unless the 4-part bar is fully cleared |
| One Redis shared as a data store across products | Per-product Redis (or clear the bar with pool + backup + role isolation) |
| One message bus carrying two products' business state with shared subjects | Per-product bus, or partitioned with the 4 proofs recorded |
| A shared-services selector offering `database: shared` | Selector covers stateless-safe capabilities only; stateful is never selectable |
| A capability that resolves to an implicit `shared`/`bundled` default | Fail-loud on unset; the choice is explicit and reviewed |
| "We'll add per-product backup later" while already sharing | Backup **and restore** isolation proven **before** sharing |
| Sharing prod observability by merging product data | Share the observability *capability*; keep per-product labels/scrape seams |

## Audit Checklist (agent-runnable against a production topology)

Capture real output (≥10 lines) per [agent-common.md](../../agents/bubbles_shared/agent-common.md); do not fabricate.

1. **Enumerate stateful stores** — list every PostgreSQL / Redis / bus / queue / search / object store in the topology and the product(s) that consume each.
2. **Flag any store with >1 product consumer** — each is a candidate violation until proven against the 4-part bar.
3. **For each shared candidate, verify all four proofs**: per-product DB+role isolation, independent backup **and** restore, no migration coupling, pool isolation. Missing any → finding: BUNDLE.
4. **Verify the selector excludes stateful** — confirm the shared-services selector (if present) offers only stateless-safe capabilities and fails loud on unset.
5. **Verify shared-safe capabilities keep isolation posture** — shared observability rejects test-labelled telemetry; per-product labels/scrape seams intact.
6. **Confirm the prod/test boundary** — the topology under review is production; test stores are governed elsewhere and must be ephemeral.

## When NOT to Use

- **Test backing-store isolation** → [bubbles-test-environment-isolation](../bubbles-test-environment-isolation/SKILL.md).
- **Test code writing to prod stores/metrics/backups** → [bubbles-env-pollution-isolation](../bubbles-env-pollution-isolation/SKILL.md).
- **Whether a product's Python/ML tier may hold DB credentials at all** → [bubbles-isolated-ml-sidecar](../bubbles-isolated-ml-sidecar/SKILL.md) (that is the compute-only tier boundary, a different axis).
- **How to author the shared-services selector / deploy adapter itself** → [bubbles-deployment-target-adapter](../bubbles-deployment-target-adapter/SKILL.md).

## Works Well With

- [bubbles-isolated-ml-sidecar](../bubbles-isolated-ml-sidecar/SKILL.md) — the compute-only-tier sibling; together they form the "isolation doctrine" (bundle stateful stores + keep the Python tier credential-free).
- [bubbles-backup-bcdr-doctrine](../bubbles-backup-bcdr-doctrine/SKILL.md) — supplies the per-product backup **and restore** requirement in proof #2.
- [bubbles-deployment-target-adapter](../bubbles-deployment-target-adapter/SKILL.md) — where the `shared` vs `bundled` selector is declared per target.
- [bubbles-observability-adapter](../bubbles-observability-adapter/SKILL.md) — the canonical shared-SAFE capability and its per-product isolation posture.
- [bubbles-config-sst](../bubbles-config-sst/SKILL.md) — the selector is single-source-of-truth config, resolved fail-loud.

## References

- [agent-common.md](../../agents/bubbles_shared/agent-common.md) — anti-fabrication, evidence standard, timeout policy.
- [bubbles-skills.instructions.md](../../instructions/bubbles-skills.instructions.md) — skill authoring governance.
- Shared-services mechanism: ecosystem shared-services selector (spec 032) consuming shared-host observability (spec 014) and shared-host inference (spec 015).

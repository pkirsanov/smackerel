---
name: bubbles-isolated-ml-sidecar
description: Enforce the isolated-ML-sidecar invariant — the Python/ML tier is COMPUTE ONLY and reaches data ONLY through the owning strongly-typed service tier over a typed contract wire (service-gated HTTP+protobuf OR bus-gated messaging), never holding database/cache/queue credentials. Use when adding or reviewing a Python/ML/LLM/embedding/data-science sidecar; when a Python service imports a data-store driver (psycopg/asyncpg/sqlalchemy/redis/kafka) or reads DATABASE_URL/REDIS_URL/RABBITMQ_URL; when wiring a python-compute-only-guard into pre-push/CI; when reconciling "removed the web framework" shorthand against a retained protobuf transport; or when deciding whether a product is conformant-by-architecture (Go-only/Rust-only, no Python data plane).
---

# Isolated ML Sidecar (Compute-Only Invariant)

## Portability

Portable governance skill. The INVARIANT and the guard shape are architecture-level and repo-neutral. Named realizations (`smackerel`, the WA/QF sibling specs) are illustrative anchors, not part of the rule — a fresh product realizes the same invariant with its own service tier and its own guard wired through its own runner. Keep this skill free of hostnames, ports, and repo-only commands; resolve command execution through `.specify/memory/agents.md` per [agent-common.md](../../agents/bubbles_shared/agent-common.md).

## The Invariant (NON-NEGOTIABLE)

**The Python / ML sidecar is COMPUTE ONLY.** It ingests a typed request, computes (inference, embedding, scoring, feature extraction, LLM calls), and returns a typed response. It MUST NOT:

- hold database / cache / queue **credentials**,
- import a data-store **driver**,
- read a data-store **connection URL**,
- reach a database, cache, or message store **directly**.

It reaches persistent data ONLY via the owning **strongly-typed service tier** (Rust or Go), over a **typed contract wire**. The service tier owns the credentials, the schema, the migrations, and the connection pool; the sidecar owns none of them.

The single sentence: **the sidecar computes, the typed service tier persists.**

## Use This Skill When

- Adding or reviewing a Python / ML / LLM / embedding / data-science service.
- A Python module imports `psycopg`, `psycopg2`, `asyncpg`, `sqlalchemy`, `redis`, `aioredis`, `kafka`, `confluent_kafka`, `pymongo`, or uses a message-bus client as a storage layer.
- A Python service reads `DATABASE_URL`, `POSTGRES_URL`, `REDIS_URL`, `RABBITMQ_URL`, or an equivalent infra connection string.
- Wiring a `python-compute-only-guard`-style check into pre-push / CI.
- Reconciling operator shorthand like "removed the web framework" against a transport that is legitimately **retained** (see Transport-Posture Reconciliation).
- Deciding whether a product needs the guard at all (see When It Does NOT Apply).

## Two Sanctioned Transport Realizations (both valid — do not mandate one)

The invariant is transport-agnostic. Two realizations are sanctioned; pick the one that matches the product's architecture. Do NOT force a bus onto a request/response product or an HTTP call onto an event-driven one.

| | 1. Service-gated | 2. Bus-gated |
|---|---|---|
| **Shape** | Python calls the typed service tier synchronously | Message in → compute → message out |
| **Wire** | HTTP with a **protobuf-only** body (JSON forbidden for business data) | Message-bus subject/topic carrying a typed payload |
| **Transport role** | A web framework (e.g. FastAPI) is retained **purely** as a protobuf ASGI transport with a proto route registry — not as a JSON API and not as a data layer | A bus client (e.g. NATS) drives the sidecar; **no DB driver present** |
| **Data access** | Only via typed calls back into the service tier | Only via messages to/from the service tier |
| **Removed** | `database_url` / `asyncpg` / `redis` / `sqlalchemy` are REMOVED | any storage driver is absent by construction |
| **Illustrative anchor** | the Rust-services sibling refactored under WA spec 161 (Scope C) | `smackerel`'s NATS-driven ML sidecar |

Both realizations satisfy the invariant identically: the sidecar never touches a store; it only exchanges typed messages with the tier that does.

## Transport-Posture Reconciliation

Operator shorthand such as "we removed FastAPI" or "removed the web framework" almost always means **"removed the storage + JSON API surface"**, NOT "removed the transport." In the service-gated realization the web framework is legitimately **retained** as a protobuf ASGI transport. Before acting on a removal instruction, reconcile the shorthand against the two axes:

- **Storage + JSON business API → REMOVE** (this is the compute-only invariant).
- **Protobuf transport + proto route registry → KEEP** (this is how the sidecar talks to the typed tier).

Record the resolved posture explicitly in the spec/design so a later reader does not "finish the removal" and break the transport.

## Enforcement: `python-compute-only-guard`

Each downstream product wires a compute-only guard into its **existing** pre-push / CI gate (invoked through the repo runner, never a bare ad-hoc command). The guard is a static scan of the Python surface with **no bypass flag** (`--skip` / `--force` / `--ignore` / `--allow-once` MUST NOT exist). Three checks:

1. **Forbidden-driver scan** — fail if the Python surface imports a data-store driver: `psycopg`, `psycopg2`, `asyncpg`, `sqlalchemy`, `redis` / `aioredis`, `kafka` / `confluent_kafka`, `pymongo`, or a bus client used as storage.
2. **Direct-infra-URL-read scan** — fail if the Python surface reads `DATABASE_URL`, `POSTGRES_URL`, `REDIS_URL`, `RABBITMQ_URL`, or an equivalent infra connection string.
3. **REMOVED-marker persistence** — assert the sanctioned `REMOVED`/absence markers (e.g. the config keys that once held `database_url`) are still gone, so a future edit cannot silently re-add a data path.

An `allowlist` scopes the scan to the compute-only surface (excluding, e.g., a legitimately data-owning migration tool that is NOT the sidecar). The guard is wired into the product's own pre-push flow and CI — mirror the canonical implementations: **WA spec 161 (Scope C)** and its cross-product sibling **QF spec 089 (Scope C)**.

## Forbidden vs Required

| ❌ Forbidden (in the Python/ML sidecar) | ✅ Required |
|---|---|
| `import asyncpg` / `import psycopg2` / `from sqlalchemy import ...` | No data-store driver; call the typed service tier |
| `redis.Redis(...)` / `aioredis.from_url(...)` | Cache is owned by the service tier; request via typed call/message |
| `os.environ["DATABASE_URL"]` / reading `REDIS_URL` / `RABBITMQ_URL` | Sidecar env carries NO infra connection strings |
| A JSON business API (`response_model=...JSON...`) for cross-tier data | Protobuf-only wire (service-gated) or typed bus payload (bus-gated) |
| "Removing FastAPI" and deleting the protobuf transport too | Keep the protobuf ASGI transport; remove only storage + JSON API |
| A generator emitting the 4 infra URLs into the sidecar's `.env` | Per-service env filter: omit infra URLs for compute-only Python |
| A compute-only guard with a `--skip`/`--force` bypass | No bypass; a new legitimate surface is added to the allowlist, reviewed |

## Rationale (security framing)

The least-trusted, fastest-churning dependencies in the whole system live in the sidecar: LLM clients, embedding libraries, model SDKs, reverse-engineered/third-party connectors, and the long tail of the Python data-science stack. Keeping the sidecar **credential-free** bounds the blast radius of a compromised, malicious, or merely flaky ML dependency — a popped `pip` package in the sidecar cannot reach the database because there is no driver and no URL to reach it with. The service tier (statically typed, slower-churning, smaller trusted dependency set) remains the only thing holding data-store credentials.

This is also the primary structural cure for "Python development flakiness": the sidecar cannot corrupt state, cannot leak a connection pool, and cannot run a rogue migration, because it has no path to persistent state at all. Data-plane correctness is confined to the typed tier.

This composes with — and does not replace — [bubbles-supply-chain-source-locking](../bubbles-supply-chain-source-locking/SKILL.md): source-locking bounds *where* the sidecar's dependencies come from; this skill bounds *what a compromised one can reach*.

## When It Does NOT Apply (N/A path — conformant-by-architecture)

A product with **no Python data plane** is conformant by construction and needs no guard:

- A **Go-only** or **Rust-only** product whose services own their own data access in the typed tier, with no Python/ML sidecar, has nothing to isolate — the invariant is satisfied structurally. Document this as an explicit N/A in the product's spec/instructions rather than wiring an empty guard.
- A product whose Python exists ONLY as a build-time tool (codegen, linting, migration authoring) — not a runtime data-plane participant — is out of scope; the guard's allowlist excludes it.

Record the N/A path so an auditor sees a deliberate "conformant-by-architecture" decision, not an oversight.

## Audit Checklist (agent-runnable against any product's Python surface)

Run through the repo runner / standard search tools; capture real output (≥10 lines) per [agent-common.md](../../agents/bubbles_shared/agent-common.md) evidence standard. Do not fabricate results.

1. **Driver scan** — search the Python surface for `psycopg|asyncpg|sqlalchemy|redis|aioredis|kafka|confluent_kafka|pymongo`. Expectation: zero imports in the sidecar.
2. **Infra-URL scan** — search for reads of `DATABASE_URL|POSTGRES_URL|REDIS_URL|RABBITMQ_URL`. Expectation: none resolved by the sidecar (a generator may synthesize them for the *service* tier — confirm they are NOT delivered to the Python container's env).
3. **Env-delivery check** — inspect the live sidecar container's environment (compose `environment:` allowlist or filtered per-service `.env`). Expectation: no infra connection strings present.
4. **Transport-posture check** — if a web framework is present, confirm it is a protobuf transport with a proto route registry, NOT a JSON business API. Confirm JSON is not used for cross-tier business data.
5. **REMOVED-marker check** — confirm the sanctioned absence markers (former `database_url` keys, etc.) are still gone.
6. **Guard-wiring check** — confirm a compute-only guard runs in the product's pre-push AND CI, and that it has NO bypass flag.
7. **N/A decision check** — if there is no Python data plane, confirm the spec/instructions record the conformant-by-architecture rationale.

## When NOT to Use

- **Test-time backing-store isolation** → use [bubbles-test-environment-isolation](../bubbles-test-environment-isolation/SKILL.md). This skill governs a production runtime tier, not ephemeral test stacks.
- **Whether two products may share one database in production** → use [bubbles-datastore-isolation](../bubbles-datastore-isolation/SKILL.md). That skill governs cross-product topology; this one governs a single product's Python-vs-typed-tier boundary.
- **Where the sidecar's dependencies are allowed to resolve from** → use [bubbles-supply-chain-source-locking](../bubbles-supply-chain-source-locking/SKILL.md).
- **Config single-source-of-truth for the connection strings themselves** → use [bubbles-config-sst](../bubbles-config-sst/SKILL.md).

## Works Well With

- [bubbles-datastore-isolation](../bubbles-datastore-isolation/SKILL.md) — the production-topology sibling; together they form the "isolation doctrine" (compute-only sidecar + bundled stateful stores).
- [bubbles-supply-chain-source-locking](../bubbles-supply-chain-source-locking/SKILL.md) — bounds where the least-trusted deps come from; complementary blast-radius control.
- [bubbles-capability-foundation-design](../bubbles-capability-foundation-design/SKILL.md) — when the typed service tier is the reusable data-access foundation the sidecar consumes.
- [bubbles-config-sst](../bubbles-config-sst/SKILL.md) — governs the per-service env filter that keeps infra URLs out of the sidecar.

## References

- [agent-common.md](../../agents/bubbles_shared/agent-common.md) — anti-fabrication, evidence standard, timeout policy.
- [bubbles-skills.instructions.md](../../instructions/bubbles-skills.instructions.md) — skill authoring governance.
- Canonical guard implementations: WA spec 161 (Scope C), QF spec 089 (Scope C).

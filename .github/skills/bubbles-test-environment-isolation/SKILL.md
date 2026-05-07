---
name: bubbles-test-environment-isolation
description: Enforce ephemeral-only backing stores for any test category that touches mutable state (integration, e2e-api, e2e-ui, stress, load). Use when authoring or reviewing test compose files; when changing test setup/teardown; when defining test-data policy in copilot-instructions; when writing tests that touch a real DB, message bus, cache, queue, file system, or external integration; when reviewing PRs that introduce a new persistent entity. Triggers include new docker-compose.test*.yml files, test-port allocation, test-database creation, "MUST clean up after" policy text, and parallel-test infrastructure work.
---

# Bubbles Test Environment Isolation

## Goal

Make tests that touch backing state (DBs, message buses, caches, queues, file systems, object stores) run against **ephemeral, disposable** stacks — not against the dev backing store with a "clean up after" promise. The dev DB is for development; test categories provision their own backing state per run and destroy it at the end.

The single sentence: **the backing store does not survive the test run.**

## Use This Skill When

- Authoring or modifying any `docker-compose*.{yml,yaml}` file under a test, integration, e2e, stress, or load path.
- Adding a new test category or expanding coverage of an existing one.
- Writing tests that touch a database, message bus, cache, queue, file system, or external integration.
- Reviewing or editing test-data policy text in `copilot-instructions.md`, `Testing.md`, or any spec.
- Designing parallel-test infrastructure (CI matrix, multi-developer concurrent runs).
- Reviewing PRs that add a new persistent entity (table, queue, bucket).
- Investigating "test residue in dev DB" or "tests stomp on each other" incidents.

## Core Rule (NON-NEGOTIABLE)

> Cleanup-based isolation is forbidden.
> Every test category that mutates backing state runs against an ephemeral stack provisioned per test run and destroyed on completion.

The phrase "**MUST use ephemeral storage or clean up**" is a known footgun. The "or clean up" branch inevitably becomes the path of least resistance, residue accumulates in dev or staging, and the next test run fails or — worse — passes for the wrong reason. The correct policy text is:

> "Tests in this category MUST run against ephemeral backing storage. Cleanup-based isolation against persistent stores is forbidden."

## Per-Category Stack Topology

| Test Category | Backing Stack | Lifetime | Compose Project Name | Port Block |
|---------------|--------------|----------|----------------------|------------|
| `unit` | None / in-process fakes | n/a | n/a | n/a |
| `functional` | None or in-process | per-test | n/a | n/a |
| `integration` | Ephemeral (tmpfs / anon) | per-run | `<project>-test-integration` | reserved test sub-block |
| `e2e-api` | Ephemeral, full live stack | per-run | `<project>-test-e2e-api` | reserved test sub-block |
| `e2e-ui` | Ephemeral, full live stack | per-run | `<project>-test-e2e-ui` | reserved test sub-block |
| `stress` | Ephemeral, isolated from other categories | per-run | `<project>-test-stress` | reserved test sub-block |
| `load` | Ephemeral, isolated from other categories | per-run | `<project>-test-load` | reserved test sub-block |

Stress and load test stacks MUST be isolated from other test categories so that load-induced churn (volume thrash, OOM kills, connection floods) does not corrupt other test categories' state mid-run.

## Required Compose File Shape

Every test compose file MUST satisfy:

1. Top-level `name: <project>-test-<category>` (unique Compose project name).
2. Backing-store services use `tmpfs:` mounts OR anonymous volumes — no named, persistent volumes.
3. Container names include project AND category: `<project>-test-<category>-<service>`.
4. All ports come from a generated test-port range (separate from dev ports).
5. All credentials come from a test-only secrets path (separate from dev secrets).
6. A dedicated network namespace.

Example (Postgres on tmpfs):

```yaml
name: <project>-test-integration

networks:
  default:
    name: <project>_test_integration_default

services:
  postgres:
    image: postgres:16
    container_name: <project>-test-integration-postgres
    tmpfs:
      - /var/lib/postgresql/data
    environment:
      POSTGRES_DB: <project>_test
      POSTGRES_USER: <project>
      POSTGRES_PASSWORD: ${TEST_DB_PASSWORD:?missing}
    ports:
      - "${TEST_INTEGRATION_DB_HOST_PORT:?missing}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 1s
      timeout: 3s
      retries: 30
```

## Test Data Identifiability

Every entity created during a test run MUST be identifiable as test data via a synthetic prefix that includes a per-run identifier:

| Entity | Required Prefix |
|--------|-----------------|
| User accounts | `test-<run-id>-user-<n>` |
| Org / tenant / customer | `test-<run-id>-org-<n>` |
| External-id-style values | `test-<run-id>-<resource>-<n>` |
| Uploaded files | `test-<run-id>-<n>.<ext>` |
| Generated artifacts on disk | under `tests/.tmp/<run-id>/` |

`<run-id>` is unique per test run (Compose project name + timestamp + nonce is sufficient). This serves two purposes:

1. Parallel test runs (CI matrix, multi-developer) cannot collide.
2. Any leak into dev / staging / prod is greppable and immediately attributable.

## Cross-Test Isolation

Even though each test category has its own ephemeral stack, the following invariants MUST hold:

- Two parallel runs of the SAME category (e.g., two `e2e-api` runs in CI) MUST be isolatable via per-run identifiers (timestamp + nonce in Compose project name).
- A `stress` or `load` run MUST NOT be able to reach the dev DB even by accident (different ports, different network, different credentials, different DB name).
- A test run that CRASHES (process killed mid-run) MUST NOT leave residue in dev. (This is automatic if the backing store is ephemeral.)

## Verification Commands

```bash
# 1. No persistent named volumes in any test compose file
for f in docker-compose.test*.yml docker-compose.integration*.yml \
         docker-compose.e2e*.yml docker-compose.stress*.yml docker-compose.load*.yml; do
  [ -f "$f" ] || continue
  echo "--- $f"
  grep -nE 'volumes:|tmpfs:' "$f"
done
# Expectation: every storage-bearing service has tmpfs OR an anonymous volume

# 2. Each test compose file declares a unique Compose project name
for f in docker-compose.test*.yml docker-compose.integration*.yml \
         docker-compose.e2e*.yml docker-compose.stress*.yml docker-compose.load*.yml; do
  [ -f "$f" ] || continue
  printf '%-45s name: %s\n' "$f" "$(grep '^name:' "$f")"
done

# 3. No test ports collide with dev ports (run after generating config)
./<project>.sh config generate
grep -E 'port|PORT' config/generated/*.env | sort -u | head -50

# 4. After every test run, no leaked test rows remain in dev DB
psql -h <dev-db-host> -U <dev-db-user> -d <dev-db-name> -c \
  "SELECT count(*) FROM users WHERE email LIKE 'test-%@%';"
# Expectation: 0

# 5. After tests complete, the test Compose project no longer exists
docker compose --project-name <project>-test-integration ps
# Expectation: no services
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| Policy text "MUST use ephemeral storage **or clean up**" | Loophole — "or clean up" inevitably becomes the path of least resistance | Replace with: "MUST use ephemeral storage; cleanup-based isolation is forbidden." |
| `try/finally` truncate-tables script run between tests on dev DB | Tests share state with dev; cleanup misses cases | Use ephemeral test DB per test run |
| Test compose file with no `name:` field | Defaults to dir name; collides with dev compose project | Add `name: <project>-test-<category>:` at top |
| Named persistent volume `pgdata:` in test compose | Test data accumulates between runs | `tmpfs: - /var/lib/postgresql/data` |
| Test code reads `localhost:5432` directly | Hardcoded; collides with dev DB | Read `TEST_DB_URL` from generated test env |
| `e2e` and `stress` share the same Compose project | Stress thrash corrupts e2e state | Separate Compose project per category |
| Test data without identifiable prefix | Leaks invisible | Mandatory `test-<run-id>-` prefix |
| "As long as it cleans up" exception in any policy doc | First failure leaves residue | Forbidden — ephemeral per-category stack |
| Same test secrets as dev/staging/prod | Test runner gains write access to non-test envs | Test-only secrets path, scoped to test stack |

## Spec / Plan Authoring Rule

Any feature spec that introduces a new persistent entity (DB table, queue, bucket, search index) MUST declare:

1. The entity's **synthetic prefix pattern** for test data.
2. Which **test category stacks** need to provision this entity at run start.
3. Whether the entity needs a **dedicated stress/load isolation budget** (e.g., a 1M-row seeded fixture must not leak into other test categories).

Specs that introduce tests against the dev backing store MUST be rejected.

## Integration With Bubbles Governance

| Bubbles Gate | Isolation Relevance |
|--------------|---------------------|
| Test Type Integrity Gate | Live-stack tests MUST point at ephemeral stack, not dev |
| Live System Test Data Hygiene | Synthetic prefix pattern must be enforced |
| Implementation Reality Scan (G028) | Detects test code reading dev DB connection strings |
| Integration Completeness (G029) | New table → must be in test fixture set, not just dev migration |
| Pre-Completion Self-Audit | After test runs, dev DB grep for `test-` prefix must return 0 |
| Docker Lifecycle Governance | Test stack teardown must not affect dev stack |

## References

- [bubbles-test-environment-isolation.instructions.md](../../instructions/bubbles-test-environment-isolation.instructions.md)
- [bubbles-config-sst skill](../bubbles-config-sst/SKILL.md)
- [bubbles-docker-lifecycle-governance skill](../bubbles-docker-lifecycle-governance/SKILL.md)
- [bubbles-deployment-target-adapter skill](../bubbles-deployment-target-adapter/SKILL.md)

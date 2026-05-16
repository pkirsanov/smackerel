# Bubbles Test Environment Isolation Instructions

> **Portability:** This file is **project-agnostic**. Copy unchanged across projects.
> Companion to [bubbles-config-sst.instructions.md](bubbles-config-sst.instructions.md), [bubbles-docker-lifecycle-governance.instructions.md](bubbles-docker-lifecycle-governance.instructions.md), and [bubbles-deployment-target.instructions.md](bubbles-deployment-target.instructions.md).

Use this instruction when creating, modifying, or reviewing tests that touch a real database, message bus, cache, file system, or external integration; when authoring or modifying `docker-compose*.{yml,yaml}` files; when changing test setup/teardown scripts; or when defining test-data policy in copilot-instructions or other governance docs.

## Core Principle: Ephemeral-Only — Cleanup Is Not Isolation

Tests that touch shared, mutable backing state (databases, message buses, caches, queues, file systems, object stores) MUST run against **ephemeral, disposable backing state** that is created at the start of the test run and destroyed at the end of it.

**Cleanup-based isolation is forbidden.** "MUST use ephemeral storage **or clean up after**" is a known footgun. Once that loophole exists, agents and humans inevitably leave residue in the dev or pre-prod database, then "fix" it with a manual `DELETE` script, which inevitably misses rows, breaks dev, and corrupts the next test run. The only correct policy is: **the backing store does not survive the test run.**

## Required Behavior

- Every test category that touches mutable backing state (`integration`, `e2e-api`, `e2e-ui`, `stress`, `load`) MUST point at an **ephemeral backing store** for that category.
- Ephemeral backing stores MUST use one of: `tmpfs` mounts, anonymous Docker volumes, named volumes destroyed at the end of the test run, in-memory engines (where appropriate), or freshly provisioned cloud test resources that are torn down on completion.
- Ephemeral backing stores MUST run in a **dedicated Docker Compose project name** that is distinct from the dev project name (e.g., `<project>-test-integration`, `<project>-test-e2e`, `<project>-test-stress`).
- Listening ports for ephemeral test stacks MUST come from a separate test-port range allocated in the SST file, distinct from dev ports.
- Test data MUST be created with **identifiable synthetic prefixes** (e.g., `test-<run-id>-<random>`) so that any leak into a non-test environment is immediately greppable.
- Tests MUST be runnable in parallel runs (CI matrix, multiple developers) without colliding on volumes, ports, or container names.

## Do Not Do

- Run integration / e2e / stress / load tests against the dev backing store.
- Use the same Docker Compose project name for dev and test stacks.
- Use named, persistent volumes for test backing stores.
- Use `try/finally`-style "clean up after the test" patterns as the primary isolation mechanism. (They are acceptable as a defense in depth; they are not acceptable as the primary isolation strategy.)
- Use shared, long-lived secrets that grant write access to non-test environments from a test runner.
- Hardcode test ports, test container names, or test database names in test code. They MUST come from the SST and the generated test config.
- Allow a test category to write to a backing store managed by another test category or by dev.

## Compose Topology

The required topology for any project that has tests touching backing state:

```
docker-compose.yml                    ← Dev stack (persistent dev volumes)
docker-compose.test.yml               ← Optional override producing test stack(s)
docker-compose.integration.yml        ← Or per-category compose file with tmpfs
docker-compose.e2e.yml                ← ...
docker-compose.stress.yml             ← ...
```

Each test compose file MUST set:

- A unique `name:` (Compose project name) at the top of the file, e.g., `name: <project>-test-integration`.
- A unique `network` namespace per test stack so cross-talk with dev is impossible.
- Backing store services with `tmpfs:` mounts or anonymous volumes only — no `volumes: db_data:/var/lib/postgresql/data` named persistent volumes.
- Container names that include the project name AND the test category, e.g., `<project>-test-integration-postgres`.

Example (PostgreSQL on tmpfs):

```yaml
name: <project>-test-integration

services:
  postgres-test:
    image: postgres:16
    container_name: <project>-test-integration-postgres
    tmpfs:
      - /var/lib/postgresql/data
    environment:
      POSTGRES_DB: <project>_test
      POSTGRES_USER: <project>
      POSTGRES_PASSWORD: <test-password-from-secrets>
    ports:
      - "${TEST_INTEGRATION_DB_HOST_PORT:?missing}:5432"
```

## Test Data Identifiability

Every entity created during a test run MUST be identifiable as test data:

| Entity | Required Prefix Pattern |
|--------|-------------------------|
| User accounts / actors | `test-<run-id>-user-<n>` |
| Organizations / tenants | `test-<run-id>-org-<n>` |
| External-id values | `test-<run-id>-<resource>-<n>` |
| File uploads | `test-<run-id>-<n>.<ext>` |
| Generated test artifacts on disk | under `tests/.tmp/<run-id>/` |

`<run-id>` is a unique identifier for the test run (Compose project name + timestamp + nonce is sufficient). This serves two purposes: (a) parallel test runs don't collide; (b) any leak into dev/staging/prod is greppable and identifiable.

## Per-Category Stack Requirements

| Test Category | Backing Stack | Lifetime | Compose Project Name |
|---------------|--------------|----------|----------------------|
| `unit` | None (or in-process fakes) | n/a | n/a |
| `functional` | None (real deps OK if already in-process) | per-test | n/a |
| `integration` | Ephemeral (tmpfs or anon volumes) | per-run | `<project>-test-integration` |
| `e2e-api` | Ephemeral, full live stack | per-run | `<project>-test-e2e-api` |
| `e2e-ui` | Ephemeral, full live stack | per-run | `<project>-test-e2e-ui` |
| `stress` | Ephemeral, isolated from other test categories | per-run | `<project>-test-stress` |
| `load` | Ephemeral, isolated from other test categories | per-run | `<project>-test-load` |

Stress and load test stacks MUST be isolated from other test categories so that load-induced churn (volume thrash, OOM kills, connection floods) does not corrupt other test categories' state mid-run.

## Verification Commands

```bash
# 1. No persistent named volumes in any test compose file
grep -rn 'volumes:' docker-compose.test*.yml docker-compose.integration*.yml \
                    docker-compose.e2e*.yml docker-compose.stress*.yml docker-compose.load*.yml \
    | grep -v 'tmpfs' | grep -v '#'

# 2. Each test compose file declares a unique Compose project name
for f in docker-compose.test*.yml docker-compose.integration*.yml \
         docker-compose.e2e*.yml docker-compose.stress*.yml docker-compose.load*.yml; do
  [ -f "$f" ] || continue
  printf '%s -> %s\n' "$f" "$(grep '^name:' "$f")"
done

# 3. No test ports collide with dev ports (run after generating config)
./<project>.sh config generate
grep -E 'port|PORT' config/generated/*.env | sort -u

# 4. After every test run, no leaked test rows remain in the dev data store
# (Should be impossible if tests use ephemeral storage. This is a defense-in-depth check.)
# Use the dev data store's native query CLI to count rows whose key matches your test-data prefix.
# Expectation: 0

# 5. After tests complete, the test stack no longer exists
./<project>.sh test integration status
# Expected: no services for the integration test stack
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| `MUST use ephemeral storage **or clean up after**` policy text | Loophole — "or clean up" inevitably becomes the path of least resistance | Replace with: `MUST use ephemeral storage; cleanup-based isolation is forbidden.` |
| `try/finally` truncate-tables script run between tests on dev DB | Tests now share state with dev; cleanup misses cases | Use ephemeral test DB per test run |
| Test compose file with no `name:` field | Defaults to dir name; collides with dev compose project | Add `name: <project>-test-<category>:` |
| `volumes: pgdata:/var/lib/postgresql/data` in test compose | Persistent test data accumulates between runs | Use `tmpfs: - /var/lib/postgresql/data` |
| Test code references a local DB host/port literal | Hardcoded; collides with dev DB | Read `TEST_DB_URL` from generated test env |
| `e2e` and `stress` tests share the same Compose project | Stress thrash corrupts e2e state | Separate Compose project per category |
| Test data created without identifiable prefix | Leaks into dev are invisible | Mandatory `test-<run-id>-` prefix |
| Test category that mutates `dev_db` "as long as it cleans up" | First failure leaves residue; next dev session breaks | Forbidden — per-category ephemeral stack |

## Spec / Plan Authoring Rule

Any feature spec that introduces a new persistent entity (table, queue, bucket) MUST declare:

1. The entity's **synthetic prefix pattern** for test data.
2. Which **test category** stacks need a fresh ephemeral provision of this entity.
3. Whether the entity needs a dedicated stress/load isolation budget (e.g., a 1M-row seeded fixture must not leak into other test categories).

Specs that introduce tests against the dev backing store MUST be rejected.

## References

- [bubbles-config-sst.instructions.md](bubbles-config-sst.instructions.md)
- [bubbles-docker-lifecycle-governance.instructions.md](bubbles-docker-lifecycle-governance.instructions.md)
- [bubbles-deployment-target.instructions.md](bubbles-deployment-target.instructions.md)
- [test-environment-isolation skill](../skills/bubbles-test-environment-isolation/SKILL.md)

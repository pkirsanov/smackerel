---
description: Extends `bubbles-test-environment-isolation` to cover monitoring, backup destinations, and manifest pointers. Test code MUST NEVER write to prod metrics, prod logs, prod backup paths, or knb manifest. Use when authoring/reviewing integration/e2e/stress/load tests, when wiring monitoring scrape configs, when authoring backup scripts, or when reviewing pre-push gate output.
---

# Environment Pollution Isolation (G115)

## The Rule

Test code (any category: integration, e2e-api, e2e-ui, stress, load, chaos) MUST NEVER write to:

1. **Prod monitoring** — prometheus pushgateways labeled `env=prod|home-lab`, loki tenants `prod|home-lab`, jaeger collectors for prod
2. **Prod backup paths** — `/srv/backups/*` (knb-side prod backup root)
3. **knb manifest pointers** — `<product>/<target>/manifest.yaml` (any product, any target)
4. **Prod release-train config** — `config/release-trains.yaml`, `config/feature-flags.<train>.yaml`

This extends the existing `bubbles-test-environment-isolation` skill (which already mandates ephemeral storage for backing-store tests) with three new pollution vectors: monitoring, backup destinations, manifest pointers.

## Enforcement (`env-pollution-scan.sh`)

Run at pre-push:

```bash
bash .github/bubbles/scripts/env-pollution-scan.sh /path/to/repo
```

Scans test code paths for write verbs (`write|push|publish|emit|send|record|append|update|patch|put|post`) co-occurring with forbidden surfaces (prod pushgateway, prod loki tenant, `/srv/backups/`, knb manifest path, `env=prod|home-lab`).

Exit 0 = clean. Exit 1 = pollution patterns found.

No `--skip` / `--force` / `--ignore` flag.

## Per-Surface Pollution Vectors

### Monitoring (Prometheus / Loki / Jaeger)

| ❌ Forbidden | ✅ Required |
|---|---|
| `prometheus_client.push_to_gateway("prod-pushgateway:9091", ...)` | Use test-scoped pushgateway with `env=test` label |
| `loki.send(tenant_id="prod")` | `loki.send(tenant_id="test")` or no log emission at all |
| Test code that exercises real services with their real prometheus metrics enabled | Tests run against test-stack which has its own prometheus job labeled `env=test*` |
| Prod prometheus has no `env` label allowlist | Prod prometheus MUST allowlist accepted env labels; reject `test*` |

### Backup Destinations

| ❌ Forbidden | ✅ Required |
|---|---|
| Test seed script writes to `/srv/backups/test-fixtures/` | Test fixtures live under repo path or ephemeral tmpfs |
| Test code that calls `backup.sh` in any form | Test the *unit* of backup logic, not the end-to-end backup pipeline; integration test backup against test-only restic repo |
| Restore-test uses prod backup as source | Restore-test uses dedicated test backup snapshot |

### knb Manifest

| ❌ Forbidden | ✅ Required |
|---|---|
| Test that asserts behavior by mutating `knb/<product>/<target>/manifest.yaml` | Mutations go through `bubbles.devops` packet from `bubbles.train`; tests are read-only |
| `apply.sh` integration test runs against real knb path | Use ephemeral fixture directory matching the manifest contract |

## Allowed (NOT Pollution)

- Tests that READ prod config (e.g., parsing `release-trains.yaml` schema)
- Tests that MENTION prod surfaces in docstrings/comments (only flagged when paired with write verb on same line)
- Tests against ephemeral test stack's OWN monitoring (test-stack prometheus with `env=test`)
- Tests that write to `/tmp/`, repo-local fixtures, named ephemeral docker volumes

## Per-Repo Test Path Override

If repo has unusual test layout, override the scan paths:

```bash
TEST_PATHS="custom/test/path/ another/path/" \
  bash .github/bubbles/scripts/env-pollution-scan.sh /path/to/repo
```

Default scan paths: `tests/ **/__tests__/ **/test/ **/tests/ **/e2e/ **/*.test.* **/*_test.*`

## See Also

- Existing skill: `bubbles-test-environment-isolation` (ephemeral storage discipline)
- Skill: `bubbles-backup-bcdr-doctrine` (legitimate backup paths)
- Instructions: `bubbles-env-pollution-isolation.instructions.md`
- Gate: G115

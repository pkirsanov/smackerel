---
applyTo: "**"
---

# Environment Pollution Isolation Policy (NON-NEGOTIABLE)

> This instructions file extends the existing
> [`bubbles-test-environment-isolation`](bubbles-test-environment-isolation.instructions.md)
> with three new pollution vectors: monitoring, backup destinations, and knb
> manifest pointers. It encodes the rules from
> [`bubbles-env-pollution-isolation`](../skills/bubbles-env-pollution-isolation/SKILL.md).

## The Rule

**Test code (integration, e2e-api, e2e-ui, stress, load, chaos) MUST NEVER write to prod monitoring, prod backup paths, or knb manifest pointers. Detection is mechanical (`env-pollution-scan.sh`); there is no override.**

## What Counts As Pollution

1. **Prod monitoring writes**
   - Pushing to prometheus pushgateway labeled `env=prod` or `env=home-lab`
   - Sending logs to loki tenant `prod` or `home-lab`
   - Emitting jaeger traces to prod collector
   - Service initialization in tests that registers itself with prod monitoring discovery

2. **Prod backup writes**
   - Writing to `/srv/backups/*` (knb-side prod backup root)
   - Calling `backup.sh` against a real product backup path
   - Restore-test source = prod snapshot (must use dedicated test snapshot instead)

3. **knb manifest mutations**
   - Modifying `<product>/<target>/manifest.yaml` from any test
   - Calling adapter `apply.sh` / `rollback.sh` against real knb path in a test

4. **Train config mutations**
   - Modifying `config/release-trains.yaml` from any test
   - Modifying `config/feature-flags.<train>.yaml` from any test

## What Is Allowed

- Tests that READ prod config (parsing schema, asserting structure)
- Tests that MENTION prod surfaces in docstrings/comments (only flagged when paired with write verb)
- Tests against ephemeral test stack's OWN monitoring (test prometheus with `env=test*`)
- Writes to `/tmp/`, repo-local fixtures, named ephemeral docker volumes
- Calling `apply.sh` against an ephemeral fixture directory that mirrors manifest contract

## Enforcement

Run at pre-push:

```bash
bash .github/bubbles/scripts/env-pollution-scan.sh "$(pwd)" || exit 1
```

Exit codes:
- `0` — clean
- `1` — pollution patterns found (printed to stderr; commit blocked)

No `--skip` / `--force` / `--ignore` / `--allowlist-this-once` flag. If a legitimate test pattern is being flagged, fix the test (use test-scoped surfaces) or override the scan paths via `TEST_PATHS` env if the repo has unusual test layout.

## Forbidden Patterns

| ❌ Forbidden | ✅ Required |
|---|---|
| `pushgateway.send("prod-pushgateway:9091")` | Use test-stack pushgateway with `env=test` |
| `loki.send(tenant="prod", ...)` | `loki.send(tenant="test")` or no log emission |
| `fs.write("/srv/backups/test-fixtures/")` | Use repo-local fixtures or `/tmp/` |
| `yq -i ... knb/<product>/<target>/manifest.yaml` from a test | Use ephemeral fixture dir |
| `prod_prometheus.register_collector(...)` in test setup | Test-scoped prometheus only |
| `env_var = "prod"` literal in test that triggers a write | Use `env_var = "test"` always |

## Prod-Side Defense In Depth

Prod monitoring SHOULD have allowlist gates so accidental pollution gets rejected even if the test code somehow targets prod:

- Prod prometheus scrape configs MUST have `env` label allowlist; reject targets with `env=test*`.
- Prod loki MUST reject ingestion from non-allowlisted tenants.
- Prod backup destinations are isolated by filesystem permissions (`/srv/backups/` is `0700` owned by knb operator).

These are knb-side controls. Product code can't enforce them unilaterally, but tests shouldn't even reach them.

## See Also

- Existing: `bubbles-test-environment-isolation.instructions.md` (ephemeral storage)
- Skill: `bubbles-env-pollution-isolation`
- Skill: `bubbles-backup-bcdr-doctrine` (legitimate backup paths)
- Gate: G115

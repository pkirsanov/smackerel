# BUG-102-002 — Scopes

Single scope. The allowlist fix and the guard fix ship together: shipping the
allowlist alone would unblock the deploy while leaving the drift class that
caused it, and shipping the guard alone would leave the deploy blocked.

---

## SCOPE-01 — Project the boot-required sidecar keys and derive the guard

**Status:** In Progress

**Depends On:** none

### Gherkin scenarios

Implements SCN-102-002-A through SCN-102-002-F from [spec.md](spec.md).

```gherkin
Scenario: SCN-102-002-B — the projection carries the intent-compiler contract
  Given a generated self-hosted config bundle
  When ml.env is extracted from the bundle
  Then it contains all seven ASSISTANT_INTENT_COMPILER_* boot-required keys

Scenario: SCN-102-002-C — secret isolation survives the fix
  Given a generated self-hosted config bundle
  When ml.env is extracted from the bundle
  Then it contains no SHELL_SECRET_KEYS member, no POSTGRES_* part, no DATABASE_URL

Scenario: SCN-102-002-D — a new boot requirement without allowlist coverage fails
  Given the sidecar's boot-required list is extended with an unprojectable key
  When the boot-safety guard derives its expected set and checks the projection
  Then the guard reports that key as missing and fails

Scenario: SCN-102-002-E — an allowlist regression fails
  Given services.ml.env_allowlist no longer declares ASSISTANT_INTENT_COMPILER_*
  When a bundle is generated and checked against the derived expected set
  Then the guard reports the seven boot-required keys as missing and fails

Scenario: SCN-102-002-A — the sidecar boots against the projected env
  Given the deployed self-hosted stack built from the fixed SST
  When smackerel-ml starts with env_file ./ml.env
  Then it reaches "Application startup complete", /health returns 200,
  And /api/health?strict=true returns 200
```

### Implementation plan

1. `config/smackerel.yaml` — add the `"ASSISTANT_INTENT_COMPILER_*"` prefix entry
   to `services.ml.env_allowlist`; correct the stale SST comment above it.
2. `internal/deploy/bundle_secret_contract_test.go` — replace the hand-maintained
   `required` slice in `TestMLEnv_ContainsEveryComputeKey_Spec102` with a
   derivation from `ml/app/main.py` + the `ml/app/` subscript-read surface;
   add parse floors so a broken derivation fails loudly.
3. Same file — add `TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002` with the
   two adversarial sub-cases.
4. Regenerate config, rebuild on the target, promote, verify live.

### Change boundary

Allowed: `config/smackerel.yaml` (`services.ml.env_allowlist` and its comment),
`internal/deploy/bundle_secret_contract_test.go`, this bug packet, and the parent
spec's `uservalidation.md` checkbox this bug closes.

Not allowed: `scripts/commands/config.sh` (projection logic already correct),
`deploy/compose.deploy.yml` (env-delivery shape already correct), `ml/app/**`
(the sidecar's requirements are the source of truth, not the thing to bend), any
knb overlay file, any framework-managed path.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Contract (Go) | `functional` | `internal/deploy/bundle_secret_contract_test.go::TestMLEnv_ContainsEveryComputeKey_Spec102` | Derived boot-required set is fully projected into a real generated `ml.env` (SCN-102-002-B) | `./smackerel.sh test unit --go --go-run 'TestMLEnv_' --verbose` | No |
| Contract (Go) | `functional` | `…::TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102` | Projection still excludes every managed secret / `POSTGRES_*` / `DATABASE_URL` (SCN-102-002-C) | `./smackerel.sh test unit --go --go-run 'TestMLEnv_' --verbose` | No |
| Adversarial (Go) | `functional` | `…::TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002/sidecar_requires_an_unprojected_key` | An injected boot requirement with no allowlist coverage is reported missing (SCN-102-002-D) | `./smackerel.sh test unit --go --go-run 'TestMLEnv_' --verbose` | No |
| Adversarial (Go) | `functional` | `…::TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002/allowlist_drops_the_intent-compiler_prefix` | Removing the allowlist entry strips exactly the seven boot keys and fails the check (SCN-102-002-E) | `./smackerel.sh test unit --go --go-run 'TestMLEnv_' --verbose` | No |
| Adversarial (Go) | `functional` | `…::TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102` | The pre-existing secret-intersection tripwire still fires (fix did not weaken it) | `./smackerel.sh test unit --go --go-run 'TestMLEnv_' --verbose` | No |
| Bundle inspection | `functional` | generated `ml.env` | All seven keys present in a real self-hosted bundle; zero managed secrets | `./smackerel.sh config generate --env self-hosted --bundle …` + `grep` | No |
| Unit (full) | `unit` | repo-wide | No collateral regression from the SST or guard change | `./smackerel.sh test unit` | No |
| Static | `unit` | `./smackerel.sh check`, `./smackerel.sh lint` | Compile + lint clean, including `python-compute-only-guard.sh` | `./smackerel.sh check` / `./smackerel.sh lint` | No |
| Live deploy | `e2e-api` | `<deploy-host>` / `<deploy-target>` | Sidecar boots, 8/8 containers healthy, strict health 200, running digests match the new manifest (SCN-102-002-A) | `promote.sh` + `docker ps` + `curl` | **Yes** |

`DERIVATION BROKEN` (SCN-102-002-F) is asserted structurally by the two parse
floors inside `deriveMLBootRequiredKeys`; it has no separate runner entry because
it fires from within every test above.

### Definition of Done

- [x] `services.ml.env_allowlist` projects the sidecar's boot-required
      `ASSISTANT_INTENT_COMPILER_*` contract, and the surrounding SST comment is
      truthful → Evidence: [report.md § Bundle projection](report.md#evidence-2--bundle-projection-carries-the-seven-keys)
- [x] A real generated self-hosted `ml.env` carries all seven boot-required keys
      → Evidence: [report.md § Bundle projection](report.md#evidence-2--bundle-projection-carries-the-seven-keys)
- [x] The projected `ml.env` still carries no managed secret, no `POSTGRES_*`
      part, and no `DATABASE_URL`
      → Evidence: [report.md § Bundle projection](report.md#evidence-2--bundle-projection-carries-the-seven-keys)
- [x] `TestMLEnv_ContainsEveryComputeKey_Spec102` derives its expected set from
      `ml/app/**` instead of restating literals, and reports the derived count
      → Evidence: [report.md § Derived guard](report.md#evidence-3--derived-guard--adversarial-bite)
- [x] The adversarial bite test fails when the sidecar requires an unprojected
      key, and when the allowlist regresses
      → Evidence: [report.md § Derived guard](report.md#evidence-3--derived-guard--adversarial-bite)
- [x] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, and
      `./smackerel.sh test unit` all exit 0 with zero warnings and zero deferrals
      → Evidence: [report.md § check](report.md#evidence-4--check), [report.md § lint](report.md#evidence-5--lint), [report.md § unit](report.md#evidence-6--full-unit-suite)
- [ ] The fixed SST is built and promoted to the `<deploy-target>` target; the ML sidecar
      boots, all containers are healthy, running core AND ml digests match the
      new manifest, and `/api/health?strict=true` returns 200
      → Evidence: [report.md § Live deploy](report.md#evidence-7--live-deploy)

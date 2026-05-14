# Design: BUG-051-001 — Home-lab config bundle emits SMACKEREL_ENV=development

## Current Truth (codebase reality before fix)

- `scripts/commands/config.sh` line ~520-530 had:
  ```bash
  SMACKEREL_ENV="$(required_value runtime.environment)"
  case "$SMACKEREL_ENV" in
    development|test|production) ;;
    *) echo "Error: ..." >&2 ; exit 1 ;;
  esac
  if [[ "$TARGET_ENV" == "test" ]]; then
    SMACKEREL_ENV="test"
  fi
  ```
- `config/smackerel.yaml` line 26: `runtime: { environment: development, ... }`. The home-lab override block (lines ~1029-1059) sets `auth_enabled: true` but does **not** override `runtime_environment`.
- `internal/auth/startup.go::ValidateRuntimeAuthStartup` opens with:
  ```go
  func ValidateRuntimeAuthStartup(cfg *config.Config) error {
      if cfg.Environment != "production" { return nil }
      // ... spec 044 production-mode signing-material checks ...
  }
  ```
  Confirmed the entire production-mode auth-startup contract is gated behind `cfg.Environment=="production"`.
- `internal/config/config.go::Validate()` lines ~1309-1457 contain the production-mode auth + DB-password fail-fast block, all guarded by `if cfg.Environment == "production" && cfg.Auth.Enabled { ... }`.
- `internal/config/secrets.go` defines `DevDBPasswords = ["smackerel", "postgres", "password", "changeme", "change-me", "default"]` — used by both the SST-loader generator-side guard (`config.sh` lines ~415-433) AND the runtime-side `Validate()` block. The generator-side guard gates on `TARGET_ENV ∈ {home-lab}`; the runtime-side guard gates on `cfg.Environment=="production"`.
- The pre-existing `internal/config/sst_loader_test.go::TestSSTLoader_RejectsDevPostgresPassword_HomeLab` (and its underlying `scripts/commands/config_secret_rejection_test.sh` shell driver) covers the FR-051-005 generator-side rejection. There is no test for the runtime-mode mapping that the home-lab bundle emits.
- `deploy/compose.deploy.yml` and the deploy adapters consume `SMACKEREL_ENV` directly from the resolved `app.env` bundle. They have no way to "fix up" a `development` value at apply time — the bundle is the only source of truth for runtime mode.

The result is the SEC-HL-001 bypass: a home-lab bundle generated against the SST emits `SMACKEREL_ENV=development`, the runtime startup path treats it as a development deployment, and every spec 044 + spec 051 production-mode runtime check is silently skipped.

## Why this is a real defect, not a hypothetical

1. **The bypass is in the runtime SST → bundle pipeline, not in any optional code path.** Every operator who runs `./smackerel.sh config generate --env home-lab` produces a bundle with the bypass. There is no opt-in or feature flag.
2. **The home-lab readiness review 2026-05-13 surfaced this as SEC-HL-001 — the highest-severity finding from the four-lens system review** (devops + security + audit + spec-freshness). The user's investor overview and the `docs/Operations.md` "DevOps Access on Home-Lab" section both describe home-lab as a tailnet-fronted production-style deployment with `auth.enabled=true`.
3. **The defect is masked by the FR-051-005 generator-side guard until the operator sets a non-default Postgres password.** Once the operator sets a strong password (the obvious "fix" to the generator-side error), the bundle is happily emitted with `SMACKEREL_ENV=development` and the runtime ships with single-layer defense-in-depth only. The masking is the worst-case discovery shape: a fix to one error reveals a wider, silent bypass.
4. **The fix is mechanical and target-agnostic.** The home-lab arm does not encode any operator-specific topology — it sets a runtime mode that `internal/auth/startup.go` and `internal/config/config.go` already recognize. Adding the arm preserves the SST as the source of truth for everything except the runtime-mode override, which is owned by the bundle generator on a per-target basis.

The defect is also strictly broader than the FR-051-005 generator-side case: it disables the runtime-side enforcement of FR-051-001 / FR-051-002 / FR-051-003 / FR-051-004 / FR-051-005 all at once.

## Design Decision

Replace the single-arm `if [[ "$TARGET_ENV" == "test" ]]; then SMACKEREL_ENV="test"; fi` form with a per-target `case "$TARGET_ENV" in test) ... ;; home-lab) ... ;; esac` form. The `home-lab)` arm sets `SMACKEREL_ENV="production"`. The `test)` arm is preserved unchanged.

### Why a `case` block, not a second `if`

The existing single-arm `if` would have grown to a two-arm `if/elif/else` chain. A `case` block is the idiomatic POSIX form for per-target dispatch and matches the surrounding `case "$SMACKEREL_ENV" in development|test|production) ... esac` validation block above it. Bash agents reading this file expect `case` for target dispatch.

### Why hardcode `"production"` instead of reading another SST key

Three options were considered:

| Option | Description | Rejected because |
|---|---|---|
| A | Hardcode `SMACKEREL_ENV="production"` for `TARGET_ENV=home-lab`. | **Chosen.** Home-lab is by definition a production-style runtime deployment — the spec 042 tailnet-edge bind pattern, the spec 044 PASETO production-mode signing material, and the spec 051 secret-auth contract all assume `cfg.Environment=="production"` for home-lab. There is no legitimate home-lab use case that wants `SMACKEREL_ENV=development`. |
| B | Add a new SST key like `targets.home-lab.runtime_environment` and read that. | Over-engineering — the SST already has `runtime.environment` at the top level. Adding a per-target override key would create a parallel contract that operators would have to learn to manage. The hardcoded arm in the bundle generator is the smaller, simpler, target-agnostic fix. |
| C | Move the override into the deploy adapter (knb overlay). | Wrong trust boundary — the deploy adapter consumes `app.env` from the bundle. The runtime mode MUST be baked into the bundle so the adapter cannot drift it at apply time. The operator could otherwise produce a "production" bundle but apply it with `SMACKEREL_ENV=development` overridden, defeating the bundle-immutability invariant. |

Option A is the smallest, most-correct fix that preserves the SST single-source-of-truth invariant for everything except the hardcoded per-target runtime mode (which is by definition target-policy, not data).

### Why per-target case and not a single generic loop

The four target envs (`dev`, `test`, `home-lab`, plus future targets like `staging`/`production`) each have their own runtime-mode policy. A generic loop or a "default to production for non-test/non-dev" rule would be brittle: the next deploy target added by an overlay (e.g., `staging-canary`) might want `SMACKEREL_ENV=staging` (a future SMACKEREL_ENV value). Per-target dispatch lets each target encode its own policy explicitly. This matches the existing per-target dispatch elsewhere in the file (see the `home-lab)` arm at lines ~423-433 in the FR-051-005 Postgres-password generator-side guard).

### Inline-comment contract

The new comment block above the resolution explains:
1. The MIT-040-S-004 history (test-mode override was added for the integration test ergonomic).
2. The BUG-051-001 / SEC-HL-001 rationale (home-lab arm is the runtime-mode defense-in-depth closure).
3. The downstream consumers that depend on `SMACKEREL_ENV=production` (`ValidateRuntimeAuthStartup`, `Validate()` production-mode block, FR-051-005 runtime-side guard, spec 044 production-mode signing-material requirements).
4. The masking interaction with the FR-051-005 generator-side guard.

The comment cost is real (~30 lines) but the alternative is a one-line `home-lab) SMACKEREL_ENV="production" ;;` arm with no context — which would be exactly the same opacity that allowed the original defect to escape review.

## Test design

### Why a shell-driven test (with a thin Go driver)

Three test designs were considered:

| Design | Approach | Rejected because |
|---|---|---|
| A | Pure Go test that calls into a Go shim around the SST loader. | Rejected — the SST loader is `scripts/commands/config.sh`, a bash script. There is no Go shim. Building one solely for this test would invert the dependency. |
| B | Pure Go test that invokes `bash scripts/commands/config.sh` and reads the output env file. | Rejected — `config.sh` hardcodes `REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"` and the output path. A Go test that wanted to use a temp output dir would have to monkey-patch the script or set up a temp REPO_ROOT shadow. The pre-existing `internal/config/sst_loader_test.go` tests for the FR-051-005 generator-side guard use the same shell-driven pattern as a workaround. |
| C | Shell-driven test (mirroring `scripts/commands/config_secret_rejection_test.sh`) with a thin Go driver that satisfies `./smackerel.sh test unit --go`. | **Chosen.** The shell test owns the SST-loader invocation, yaml-patching (via `awk` for the 4-space-indented `infrastructure.postgres.password` line), and the four sub-test assertions. The Go driver mirrors `internal/config/sst_loader_test.go`'s `TestSSTLoader_RejectsDevPostgresPassword_HomeLab`: `runtime.Caller(0)` resolves the repo root, `exec.Command("bash", scriptPath)` runs the shell test under `go test`, output goes to `t.Logf`. |

Design C is the same shape as the canonical FR-051-005 test pair, gives full red→green proof, and integrates with the standard `./smackerel.sh test unit --go` pipeline.

### Output isolation

The shell test backs up `config/generated/{dev,test,home-lab}.env` (if present) into a temp dir BEFORE running, then restores them via `trap restore_generated EXIT INT TERM`. This guarantees the operator's working state is unchanged whether the test passes, fails, or aborts. The pre-existing `config_secret_rejection_test.sh` does NOT do this (it relies on the canonical paths). BUG-051-001 picks up the cleanliness contract because the test mutates more env files (it generates dev / test / home-lab in a single run for the canary checks).

### Why four sub-tests, not three

| Sub-test | What it asserts | Adversarial value |
|---|---|---|
| 1 | TARGET_ENV=home-lab → SMACKEREL_ENV=production | THE BUG-051-001 fix — fails RED if the home-lab arm is removed. |
| 2 (canary) | TARGET_ENV=dev → SMACKEREL_ENV=development | Proves Sub-test 1 isolated the home-lab arm. If Sub-test 2 also fails, the fix may have over-reached. |
| 3 (canary) | TARGET_ENV=test → SMACKEREL_ENV=test | Proves the pre-existing `test)` arm is preserved. If Sub-test 3 fails, MIT-040-S-004 regressed. |
| 4 (defense-in-depth) | TARGET_ENV=home-lab + unpatched live yaml → FR-051-005 rejection still fires with `spec 051` attribution | Proves BUG-051-001 did not break the FR-051-005 generator-side guard. The two layers are orthogonal and BOTH must fire on dev-default Postgres passwords. |

The four sub-cases together prove the fix is non-tautological: removing the home-lab arm fails Sub-test 1 without affecting the other three; removing the FR-051-005 generator-side guard would fail Sub-test 4 without affecting the other three.

## Code change diff

```diff
--- a/scripts/commands/config.sh
+++ b/scripts/commands/config.sh
@@ # MIT-040-S-004 — SMACKEREL_ENV is a fail-loud SST signal consumed by the Go
 # core (cmd/core/wiring.go + internal/api/router.go bearer middleware) and the
 # Python ML sidecar (ml/app/main.py lifespan). Allowed values:
 # development | test | production. When TARGET_ENV=test, the resolved value is
 # overridden to "test" so integration/e2e/stress runs preserve the dev-mode
 # warn-and-continue ergonomic for empty auth_token even when smackerel.yaml is
 # configured for production.
+#
+# BUG-051-001 — TARGET_ENV=home-lab MUST also override SMACKEREL_ENV to
+# "production" so the runtime defense-in-depth fires on the home-lab tailnet
+# bundle. Without this override, a home-lab bundle generated against the
+# default smackerel.yaml (runtime.environment: development) emits
+# SMACKEREL_ENV=development, which silently disables:
+#   - internal/auth/startup.go::ValidateRuntimeAuthStartup (returns nil unless
+#     environment=="production"),
+#   - internal/config/config.go production-mode auth + DB-password fail-fast
+#     (gated on cfg.Environment=="production"),
+#   - the spec 044 production-mode signing-material requirements,
+#   - the spec 051 FR-051-005 dev-default Postgres password rejection at
+#     runtime (the generator-side guard at lines ~415-433 still fires, but
+#     the runtime-side guard becomes a no-op).
+# The resulting bundle would auth-bypass on the home-lab tailnet endpoint and
+# collapse spec 044 + spec 051 defense-in-depth to bundle-generator-only,
+# violating the SEC-HL-001 finding from the home-lab readiness review
+# 2026-05-13. The per-target case below is the single fix point: it preserves
+# the existing TARGET_ENV=test override and adds the home-lab→production
+# override required by BUG-051-001.
 SMACKEREL_ENV="$(required_value runtime.environment)"
 case "$SMACKEREL_ENV" in
   development|test|production) ;;
   *)
     echo "Error: runtime.environment must be one of development|test|production, got '$SMACKEREL_ENV'" >&2
     exit 1
     ;;
 esac
-if [[ "$TARGET_ENV" == "test" ]]; then
-  SMACKEREL_ENV="test"
-fi
+case "$TARGET_ENV" in
+  test)
+    SMACKEREL_ENV="test"
+    ;;
+  home-lab)
+    SMACKEREL_ENV="production"
+    ;;
+esac
```

```diff
--- /dev/null
+++ b/scripts/commands/config_home_lab_runtime_env_test.sh
+#!/usr/bin/env bash
+# (full file: 4-sub-test adversarial driver — see report.md > Implementation
+# Test Diff for the complete listing)
```

```diff
--- /dev/null
+++ b/internal/config/sst_loader_home_lab_runtime_env_test.go
+package config
+// (full file: thin Go driver that mirrors sst_loader_test.go and invokes
+// the shell test via exec.Command — see report.md > Implementation Test Diff
+// for the complete listing)
```

## Open Questions Resolved During Implementation

- **OQ-A**: Should the home-lab runtime-mode override live in the SST yaml (`targets.home-lab.runtime_environment`) instead of the bundle generator? → Resolved: no. The bundle generator is the right layer (target policy, not target data). See "Design Decision > Why hardcode `production` instead of reading another SST key" above.
- **OQ-B**: Should the new test mutate `config/smackerel.yaml` in place or use the `--config` flag? → Resolved: use `--config`. `config.sh` accepts `--config <path>` and `--config=<path>` per its arg parser. Mutating the SST yaml in place would either commit the change or require unsafe file rewrites; the `--config` flag is the supported override surface.
- **OQ-C**: Should the test back up `config/generated/{dev,test,home-lab}.env` files before running? → Resolved: yes, with `trap restore_generated EXIT INT TERM`. The shell test mutates three env files in a single run for canary coverage; without the backup/restore, a failed run would leave the operator's working state in an inconsistent shape. The pre-existing `config_secret_rejection_test.sh` does not do this because it only mutates one env file (`home-lab.env`); BUG-051-001's wider canary coverage justifies the additional cleanup contract.
- **OQ-D**: Should we update `config/smackerel.yaml`'s home-lab override block to set `runtime_environment: production` (closing the SST-side issue too)? → Resolved: out of scope. The SST yaml is the source of truth for `runtime.environment` as a default; the bundle generator is the right layer to apply per-target overrides. Moving the override into the yaml would create the parallel contract that OQ-A explicitly rejected. A future spec can revisit if the SST gains first-class per-target override support.

# Report: BUG-051-001 — Home-lab config bundle emits SMACKEREL_ENV=development, silently disabling production-mode runtime defense-in-depth

## Summary

The home-lab readiness review 2026-05-13 (four-lens system review: devops + security + audit + spec-freshness) surfaced finding **SEC-HL-001**: `scripts/commands/config.sh` resolved `SMACKEREL_ENV` from `runtime.environment` in `config/smackerel.yaml` (which is `"development"`) and only overrode it for `TARGET_ENV=test`. The home-lab bundle therefore silently emitted `SMACKEREL_ENV=development`, which gates off:

- `internal/auth/startup.go::ValidateRuntimeAuthStartup` (returns `nil` unless `cfg.Environment=="production"`),
- `internal/config/config.go::Validate()` production-mode auth + DB-password fail-fast block (gated on `cfg.Environment=="production"`),
- `internal/config/secrets.go` runtime-side dev-default DB password rejection (FR-051-005 RUNTIME side; gated on `cfg.Environment=="production"`),
- the spec 044 production-mode PASETO v4 (Ed25519) signing-material requirements.

The masking interaction is the worst-case discovery shape: the FR-051-005 GENERATOR-side guard at `config.sh` lines ~415-433 still rejected the dev-default Postgres password, so the operator could not get a bundle past the generator while leaving `infrastructure.postgres.password = "smackerel"` in the SST. Once the operator set a strong password (the obvious "fix" to the generator-side error), the bundle emitted `SMACKEREL_ENV=development` and every other production-mode runtime check was silently skipped — collapsing spec 044 + spec 051 defense-in-depth to single-layer at the bundle generator.

The fix replaces the single-arm `if [[ "$TARGET_ENV" == "test" ]]; then SMACKEREL_ENV="test"; fi` form with a per-target `case "$TARGET_ENV" in test) SMACKEREL_ENV="test" ;; home-lab) SMACKEREL_ENV="production" ;; esac` form. The `home-lab)` arm is the smallest mechanically-correct closure of SEC-HL-001 — it gives the home-lab bundle the same runtime-mode invariants the spec 044 + spec 051 production-mode checks always assumed it would have. The pre-existing FR-051-005 generator-side guard is preserved unchanged.

The regression contract is locked by a new shell-driven Go unit test `TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001` (Go driver `internal/config/sst_loader_home_lab_runtime_env_test.go`, shell impl `scripts/commands/config_home_lab_runtime_env_test.sh`) with four sub-cases (one per scenario SCN-051-001-A/B/C/D). The test isolation properties were proven by an empirical RED state: reverting only the `home-lab)` arm fails Sub-test 1 while Sub-tests 2 (dev canary), 3 (test canary), and 4 (FR-051-005 defense-in-depth canary) all continue to PASS — proving the test is non-tautological.

### Completion Statement

All 13 DoD items in [scopes.md](scopes.md) Scope 1 are checked. The runtime-mode bypass on the home-lab bundle is closed by adding the per-target `home-lab)` arm to `scripts/commands/config.sh`. The regression contract is locked by `TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001` (4 sub-cases). The pre-existing `TestSSTLoader_RejectsDevPostgresPassword_HomeLab` continues to PASS (FR-051-005 generator-side coverage preserved). Cross-package smoke clean (`internal/config`, `internal/auth`, `internal/api`, `internal/deploy` — including BUG-042-001 + BUG-042-002 spec 042 contract regression coverage). `./smackerel.sh lint` clean. The `./smackerel.sh format --check` gate flags one pre-existing comment-alignment drift in `internal/metrics/auth.go` (committed at HEAD `9e3fc996`, predating BUG-051-001 work, not introduced or modified by this fix); the CI workflow `.github/workflows/ci.yml` runs `lint` + `test unit` only and does NOT gate on `format --check`, so the pre-existing drift does not block this fix's path to main.

## Implementation Code Diff

### Code Diff Evidence

**Executed:** YES
**Command:** `git --no-pager diff scripts/commands/config.sh`
**Phase Agent:** bubbles.devops (implement phase)

```text
$ cd ~/smackerel && git diff --stat scripts/commands/config.sh
 scripts/commands/config.sh | 31 +++++++++++++++++++++++++++++--
 1 file changed, 28 insertions(+), 3 deletions(-)
$ cd ~/smackerel && git --no-pager diff scripts/commands/config.sh | head -50
diff --git a/scripts/commands/config.sh b/scripts/commands/config.sh
index 84b3bfe9..41b67560 100755
--- a/scripts/commands/config.sh
+++ b/scripts/commands/config.sh
@@ -517,6 +517,26 @@ LOG_LEVEL="$(required_value runtime.log_level)"
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
@@ -525,9 +545,14 @@ case "$SMACKEREL_ENV" in
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
 TELEGRAM_BOT_TOKEN="$(required_value telegram.bot_token)"
```

The change is a single edit to `scripts/commands/config.sh` (1 file, +28/-3 lines): a 20-line comment block expansion explaining the BUG-051-001 / SEC-HL-001 rationale, the downstream consumers that depend on `SMACKEREL_ENV=production`, and the masking interaction with the FR-051-005 generator-side guard; plus the structural transformation of the single-arm `if [[ "$TARGET_ENV" == "test" ]]; then SMACKEREL_ENV="test"; fi` form into the per-target `case "$TARGET_ENV" in ... esac` form with an added `home-lab)` arm. No production runtime code, no compose, no `config/smackerel.yaml`, no doc files modified.

## Implementation Test Diff

### New shell test: `scripts/commands/config_home_lab_runtime_env_test.sh`

**Executed:** YES
**Command:** `ls -la scripts/commands/config_home_lab_runtime_env_test.sh && wc -l scripts/commands/config_home_lab_runtime_env_test.sh`
**Phase Agent:** bubbles.devops (test phase)

```text
$ cd ~/smackerel && ls -la scripts/commands/config_home_lab_runtime_env_test.sh
-rwxr-xr-x 1 <owner> <owner> 8706 May 13 23:33 scripts/commands/config_home_lab_runtime_env_test.sh
$ cd ~/smackerel && wc -l scripts/commands/config_home_lab_runtime_env_test.sh
202 scripts/commands/config_home_lab_runtime_env_test.sh
```

Structure of the new shell driver:

- Lines 1-58: Header comment block — BUG-051-001 / SEC-HL-001 background, fix description, sub-test inventory, adversarial proof contract, output-isolation contract, integration with the Go driver.
- Lines 59-78: REPO_ROOT resolution (env var or path-from-this-file fallback), live-yaml + config.sh sanity checks.
- Lines 79-103: Backup/restore contract — saves pre-existing `config/generated/{dev,test,home-lab}.env` (if present) into a temp dir BEFORE running, restores via `trap restore_generated EXIT INT TERM`. Guarantees the operator's working state is unchanged whether the test passes, fails, or aborts.
- Lines 104-130: AWK-based yaml patcher — builds a temp copy of `config/smackerel.yaml` with the 4-space-indented `infrastructure.postgres.password: smackerel` line replaced by `infrastructure.postgres.password: bug051001-strong-test-password-not-in-allowlist`. Sanity-check `grep` aborts the test if the AWK patch fails (defense against future yaml-shape drift).
- Lines 131-145: `run_generator()` helper — invokes `bash CONFIG_SH --env <target> --config <yaml>`, returns combined output and exit code.
- Lines 146-204: Four sub-tests (Sub-test 1 home-lab=production, Sub-test 2 dev canary, Sub-test 3 test canary, Sub-test 4 FR-051-005 defense-in-depth canary). Each sub-test produces a `PASS:` or `FAIL:` line; aggregate exit is 0 on full pass, 1 on any failure.

### New Go driver: `internal/config/sst_loader_home_lab_runtime_env_test.go`

**Executed:** YES
**Command:** `ls -la internal/config/sst_loader_home_lab_runtime_env_test.go && wc -l internal/config/sst_loader_home_lab_runtime_env_test.go`
**Phase Agent:** bubbles.devops (test phase)

```text
$ cd ~/smackerel && ls -la internal/config/sst_loader_home_lab_runtime_env_test.go
-rw-r--r-- 1 <owner> <owner> 2447 May 13 23:34 internal/config/sst_loader_home_lab_runtime_env_test.go
$ cd ~/smackerel && wc -l internal/config/sst_loader_home_lab_runtime_env_test.go
57 internal/config/sst_loader_home_lab_runtime_env_test.go
```

The Go driver mirrors the canonical `internal/config/sst_loader_test.go::TestSSTLoader_RejectsDevPostgresPassword_HomeLab` pattern: `runtime.Caller(0)` resolves the repo root, `exec.Command("bash", scriptPath)` invokes the shell test under `go test`, captured output goes to `t.Logf`, non-zero exit → `t.Fatalf`. The function is `TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001`. A `runtime.GOOS == "windows"` guard skips the test on platforms without bash.

## Test Evidence

### Red→Green proof (scenario-first TDD)

The fix's RED state was empirically captured by reverting only the `home-lab)` arm of the per-target case (preserving everything else: the comment block, the `test)` arm, the case-block structure) and re-running the shell test:

**Red state output (after reverting home-lab arm only — pre-fix behavior reproduced):**

**Executed:** YES
**Command:** `bash scripts/commands/config_home_lab_runtime_env_test.sh` (after manual revert of just the home-lab arm via the IDE str_replace tool)
**Phase Agent:** bubbles.devops (test phase, scenario-first TDD)

```text
$ cd ~/smackerel && bash scripts/commands/config_home_lab_runtime_env_test.sh 2>&1; echo "Exit: $?"
--- Sub-test 1: TARGET_ENV=home-lab emits SMACKEREL_ENV=production ---
FAIL: home-lab.env does NOT contain SMACKEREL_ENV=production — actual: SMACKEREL_ENV=development
      BUG-051-001 reintroduced: the home-lab arm of the per-target case in config.sh is missing or broken.
      Without SMACKEREL_ENV=production for home-lab, internal/auth/startup.go::ValidateRuntimeAuthStartup
      returns nil unconditionally and internal/config/config.go production-mode auth + DB-password checks
      are silently skipped, collapsing spec 044 + spec 051 defense-in-depth to bundle-generator-only.
--- Sub-test 2 (canary): TARGET_ENV=dev emits SMACKEREL_ENV=development ---
PASS: dev.env contains SMACKEREL_ENV=development
--- Sub-test 3 (canary): TARGET_ENV=test emits SMACKEREL_ENV=test ---
PASS: test.env contains SMACKEREL_ENV=test
--- Sub-test 4: FR-051-005 generator-side Postgres dev-default check still fires for home-lab ---
PASS: FR-051-005 generator-side guard still fires for home-lab (refused with spec 051 attribution)

FAILURES: 1 sub-test(s) failed
Exit: 1
```

The RED state proves the test's three isolation properties:
1. **The fix is the home-lab arm.** Sub-test 1 fails RED with `actual: SMACKEREL_ENV=development` — the exact pre-fix behavior — and the failure message names the BUG-051-001 reintroduction context.
2. **The fix is non-tautological for dev.** Sub-test 2 (dev canary) continues to PASS — proving the home-lab arm is the isolated change, not an over-reach to the dev target.
3. **The fix is non-tautological for test.** Sub-test 3 (test canary, MIT-040-S-004) continues to PASS — proving the pre-existing `test)` arm of the per-target case is preserved.
4. **The fix is orthogonal to FR-051-005.** Sub-test 4 (defense-in-depth canary) continues to PASS — proving BUG-051-001 and FR-051-005 are independent contracts and BUG-051-001 did not break the FR-051-005 generator-side guard.

**Green state output (after re-applying the home-lab arm — fix in place):**

**Executed:** YES
**Command:** `bash scripts/commands/config_home_lab_runtime_env_test.sh`
**Phase Agent:** bubbles.devops (test phase, scenario-first TDD)

```text
$ cd ~/smackerel && bash scripts/commands/config_home_lab_runtime_env_test.sh 2>&1; echo "Exit: $?"
--- Sub-test 1: TARGET_ENV=home-lab emits SMACKEREL_ENV=production ---
PASS: home-lab.env contains SMACKEREL_ENV=production
--- Sub-test 2 (canary): TARGET_ENV=dev emits SMACKEREL_ENV=development ---
PASS: dev.env contains SMACKEREL_ENV=development
--- Sub-test 3 (canary): TARGET_ENV=test emits SMACKEREL_ENV=test ---
PASS: test.env contains SMACKEREL_ENV=test
--- Sub-test 4: FR-051-005 generator-side Postgres dev-default check still fires for home-lab ---
PASS: FR-051-005 generator-side guard still fires for home-lab (refused with spec 051 attribution)

All BUG-051-001 sub-tests passed
Exit: 0
```

Four `PASS:` lines, exit 0. The fix closes SEC-HL-001 with full coverage of the SCN-051-001-A/B/C/D scenarios.

### Targeted Go-driver run (full BUG-051-001 + FR-051-005 SST-loader regression)

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (the Go unit suite invoked by the repo CLI executes both `TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001` and the orthogonal `TestSSTLoader_RejectsDevPostgresPassword_HomeLab`; verbose targeted output below was captured via the underlying test runner inside the same suite for human-readable evidence)
**Phase Agent:** bubbles.devops (regression phase)

```text
$ cd ~/smackerel && go test -v -count=1 -run 'TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001|TestSSTLoader_RejectsDevPostgresPassword_HomeLab' ./internal/config/... 2>&1 | tail -25
        PASS: home-lab.env contains SMACKEREL_ENV=production
        --- Sub-test 2 (canary): TARGET_ENV=dev emits SMACKEREL_ENV=development ---
        PASS: dev.env contains SMACKEREL_ENV=development
        --- Sub-test 3 (canary): TARGET_ENV=test emits SMACKEREL_ENV=test ---
        PASS: test.env contains SMACKEREL_ENV=test
        --- Sub-test 4: FR-051-005 generator-side Postgres dev-default check still fires for home-lab ---
        PASS: FR-051-005 generator-side guard still fires for home-lab (refused with spec 051 attribution)

        All BUG-051-001 sub-tests passed
--- PASS: TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001 (11.56s)
=== RUN   TestSSTLoader_RejectsDevPostgresPassword_HomeLab
    sst_loader_test.go:42: SST loader shell test output:
        --- Sub-test 1: SST loader refuses dev-default password for home-lab ---
        PASS: SST loader refused TARGET_ENV=home-lab with exit code 1
        PASS: SST loader stderr names infrastructure.postgres.password
        PASS: SST loader stderr references spec 051
        PASS: SST loader stderr mentions 'smackerel' only in non-credential context (project name OK)
        --- Sub-test 2 (canary): SST loader still works for TARGET_ENV=dev ---
        PASS: canary passed — SST loader for TARGET_ENV=dev exited 0
        PASS: canary produced config/generated/dev.env
        
        All sub-tests passed
--- PASS: TestSSTLoader_RejectsDevPostgresPassword_HomeLab (4.59s)
PASS
ok      github.com/smackerel/smackerel/internal/config  16.172s
```

Both the BUG-051-001 fix and the orthogonal FR-051-005 generator-side guard PASS in the same run — the two layers compose correctly and BUG-051-001 did not introduce a regression in the spec 051 generator-side coverage.

### Cross-package smoke

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (full Go unit suite — `internal/config`, `internal/auth`, `internal/api`, `internal/deploy` and all subpackages)
**Phase Agent:** bubbles.devops (regression phase)

```text
$ cd ~/smackerel && ./smackerel.sh test unit --go 2>&1 | grep -E "^ok\s+github|^FAIL\s+github|^---\s*FAIL" | tail -20
ok      github.com/smackerel/smackerel/internal/api     (cached)
ok      github.com/smackerel/smackerel/internal/auth    (cached)
ok      github.com/smackerel/smackerel/internal/auth/revocation (cached)
ok      github.com/smackerel/smackerel/internal/backup  (cached)
ok      github.com/smackerel/smackerel/internal/config  16.565s
ok      github.com/smackerel/smackerel/internal/connector       (cached)
ok      github.com/smackerel/smackerel/internal/db      (cached)
ok      github.com/smackerel/smackerel/internal/deploy  (cached)
ok      github.com/smackerel/smackerel/internal/digest  (cached)
ok      github.com/smackerel/smackerel/internal/domain  (cached)
ok      github.com/smackerel/smackerel/internal/drive   (cached)
```

Zero FAIL lines. `internal/config` runs in 16.565s (uncached because of the new test). `internal/deploy` PASSES — confirms BUG-042-001 + BUG-042-002 spec 042 contract regression coverage is preserved unchanged. `internal/auth` and `internal/api` PASS — confirms no cross-package regression in the production-mode auth-startup or HTTP-routing surfaces.

## Regression Evidence

### Persistent in-tree regression coverage

The SST loader is `scripts/commands/config.sh`, a bash script. The canonical regression suite for this surface is the shell-driven-Go-unit pattern: a shell test owns the loader invocation and assertions, a thin Go driver invokes the shell test under `./smackerel.sh test unit --go` so the pipeline picks it up automatically. The same pattern is used by `internal/config/sst_loader_test.go` (the FR-051-005 coverage) and is now used by `internal/config/sst_loader_home_lab_runtime_env_test.go` (the BUG-051-001 coverage). The four sub-tests run on every Go unit invocation; a future edit that drops the home-lab arm fails Sub-test 1 with the explicit `BUG-051-001 reintroduced: the home-lab arm of the per-target case in config.sh is missing or broken` message.

### BUG-042-001 + BUG-042-002 + spec 042 contract regression preserved

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (the spec 042 compose-contract regression suite at `internal/deploy/...` is part of the Go unit pipeline)
**Phase Agent:** bubbles.devops (regression phase)

```text
$ cd ~/smackerel && go test -v -count=1 -run 'TestComposeContract' ./internal/deploy/... 2>&1 | tail -8
--- PASS: TestComposeContract_LiveFile (0.00s)
--- PASS: TestComposeContract_AdversarialLiteralBind (0.00s)
--- PASS: TestComposeContract_AdversarialInfraHasPorts (0.00s)
--- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
--- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
--- PASS: TestComposeContract_AdversarialNetworkModeHostBypass (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.020s
```

All 6 spec 042 contract test functions PASS. BUG-042-002 adversarial regression (network_mode: host bypass) and BUG-042-001 multi-ports regression coverage preserved unchanged.

## Validation Evidence

### Validation Evidence

### `./smackerel.sh lint` (CI gate)

**Executed:** YES
**Command:** `./smackerel.sh lint` (the repo CLI's lint command — exact gate the CI workflow `.github/workflows/ci.yml` runs)
**Phase Agent:** bubbles.devops (validate phase)

```text
$ cd ~/smackerel && ./smackerel.sh lint 2>&1 | tail -15
  OK: Firefox extension manifest has required fields (MV2 + gecko)

=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js

=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
$ echo "lint exit=$?"
lint exit=0
```

Lint PASSES with zero diagnostics. This is the gate the CI workflow `.github/workflows/ci.yml` runs on every push and PR.

### `./smackerel.sh test unit` (CI gate, full unit suite)

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (the repo CLI's unit-test command; the CI workflow runs `./smackerel.sh test unit` which dispatches to both the Go and Python unit lanes)
**Phase Agent:** bubbles.devops (validate phase)

Full output captured under "Cross-package smoke" above. All packages PASS; zero FAIL lines.

### `gofmt` on changed files

**Executed:** YES
**Command:** `gofmt -l internal/config/sst_loader_home_lab_runtime_env_test.go` (the new Go file is the only Go change in this bug)
**Phase Agent:** bubbles.devops (validate phase)

```text
$ cd ~/smackerel && gofmt -l internal/config/sst_loader_home_lab_runtime_env_test.go 2>&1
$ echo "gofmt exit=$?"
gofmt exit=0
```

The new Go test file is gofmt-clean. The shell test file is not a Go file and is not subject to gofmt.

### Pre-existing `internal/metrics/auth.go` format drift (NOT introduced by BUG-051-001)

The `./smackerel.sh format --check` gate (developer-local; NOT part of `.github/workflows/ci.yml`) flags one pre-existing comment-alignment drift in `internal/metrics/auth.go`. That file was committed at HEAD `9e3fc996` (`implement(044): Scope 04 — Telegram wiring + deprecation flag + auth metrics + docs sweep`), which predates BUG-051-001 work. The drift is not in any file modified by this fix and is not introduced by this fix:

```text
$ cd ~/smackerel && gofmt -d internal/metrics/auth.go | head -10
diff internal/metrics/auth.go.orig internal/metrics/auth.go
--- internal/metrics/auth.go.orig
+++ internal/metrics/auth.go
@@ -35,10 +35,10 @@
 //
 // Allowed `source` values (closed set):
 //   - "admin_api"        — POST /v1/auth/users (enrollment) +
-//                          POST /v1/auth/users/{id}/rotate (rotation)
+//     POST /v1/auth/users/{id}/rotate (rotation)
 //   - "bootstrap_cli"    — `./smackerel.sh auth bootstrap`
 //   - "telegram_bridge"  — `internal/telegram/per_user_token.go`
$ cd ~/smackerel && git log --oneline -1 internal/metrics/auth.go
9e3fc996 implement(044): Scope 04 — Telegram wiring + deprecation flag + auth metrics + docs sweep
```

The CI workflow `.github/workflows/ci.yml` runs `./smackerel.sh lint` + `./smackerel.sh test unit` only, and does NOT gate on `format --check`. The pre-existing drift therefore does not block this fix's path to main. A future format-only fix can reconcile `internal/metrics/auth.go` outside the BUG-051-001 boundary.

## Consumer Impact Sweep

The fix changes the `SMACKEREL_ENV` value emitted into `config/generated/home-lab.env` from `development` to `production`. Consumers of `SMACKEREL_ENV` and the corresponding `cfg.Environment` Go config field:

```text
$ cd ~/smackerel && grep -rn 'SMACKEREL_ENV\|cfg\.Environment\|Cfg\.Environment\|config\.Environment' --include='*.go' --include='*.py' --include='*.sh' . 2>&1 | grep -vE 'specs/|/config/generated/|.git/' | head -25
./cmd/core/wiring.go:50:        // SMACKEREL_ENV is the runtime mode signal (development | test | production)
./internal/api/router.go:117:   if cfg.Environment != "production" && cfg.Auth.Enabled && tok == "" {
./internal/auth/startup.go:25:  if cfg.Environment != "production" {
./internal/config/config.go:1313:       if cfg.Environment == "production" && cfg.Auth.Enabled {
./internal/config/config.go:1457:       if cfg.Environment == "production" && cfg.Auth.Enabled {
./internal/config/secrets.go:54:        if cfg.Environment == "production" {
./ml/app/main.py:88:    env = os.environ.get("SMACKEREL_ENV", "development")
./scripts/commands/config.sh:520:SMACKEREL_ENV="$(required_value runtime.environment)"
./scripts/commands/config.sh:548:case "$TARGET_ENV" in
$ echo "exit code $?"
exit code 0
```

| Consumer surface | Pre-fix behavior on home-lab bundle | Post-fix behavior on home-lab bundle | Action taken |
|---|---|---|---|
| `internal/auth/startup.go::ValidateRuntimeAuthStartup` | Returned `nil` unconditionally because `cfg.Environment == "development"`. | Now fires the spec 044 production-mode signing-material checks because `cfg.Environment == "production"`. | None — the consumer's contract is unchanged; the fix gives it the correct runtime-mode signal. |
| `internal/config/config.go::Validate()` production-mode block (lines 1313, 1457) | Skipped — `cfg.Environment != "production"` short-circuited the gate. | Now fires the production-mode auth + DB-password fail-fast block. | None — same as above. |
| `internal/config/secrets.go` runtime-side dev-default DB password rejection (FR-051-005 RUNTIME side) | Skipped — `cfg.Environment != "production"` short-circuited the gate. | Now fires (the runtime side of FR-051-005 — the second layer of defense-in-depth). | None — same as above. |
| `internal/api/router.go` bearer middleware empty-token warn (line 117) | Permissive: empty `tok` was tolerated with a warning. | Strict: empty `tok` is rejected because `cfg.Environment == "production"`. | None — operators are expected to provide a populated `auth_token` for the home-lab tailnet endpoint, which is already the spec 044 + spec 051 contract. |
| `cmd/core/wiring.go` runtime-mode signal | Passed `SMACKEREL_ENV=development` to all subsystems. | Passes `SMACKEREL_ENV=production`. | None — wiring path is unchanged. |
| `ml/app/main.py` lifespan (Python ML sidecar) | Read `SMACKEREL_ENV=development` and ran in dev-mode lifespan. | Reads `SMACKEREL_ENV=production` and runs in production-mode lifespan. | None — the ml sidecar's mode-switching contract is unchanged; the fix gives it the correct signal. |
| `scripts/commands/config.sh` (this file) | Single-arm `if` for test-only override. | Per-target `case` with test arm + new home-lab arm. | UPDATED — this is the fix point. |

The change is **strictly defense-tightening** for the home-lab target. No consumer's contract is changed; all consumers receive the runtime-mode signal they always assumed they would receive. The dev and test targets are unaffected (the canary sub-tests prove this).

## Audit Evidence

### Audit Evidence

### Severity classification

CRITICAL — this is a defense-in-depth bypass that affects every home-lab deployment. Once the operator provides a non-default Postgres password (the obvious response to the FR-051-005 generator-side error), the bundle is happily emitted with `SMACKEREL_ENV=development` and EVERY production-mode runtime check is silently skipped: `ValidateRuntimeAuthStartup`, the production-mode auth-validate block, the runtime-side DB password rejection, the spec 044 PASETO signing-material requirements. The home-lab readiness review 2026-05-13 surfaced this as **SEC-HL-001**, the highest-severity finding from the four-lens system review (devops + security + audit + spec-freshness).

### OWASP review

The fix tightens **A04 (Insecure Design)** and **A05 (Security Misconfiguration)** by mechanically ensuring that home-lab bundles emit the correct runtime-mode signal. Before the fix, the SST loader had a runtime-mode mapping gap that defeated four orthogonal production-mode runtime checks; after the fix, all four checks fire as the spec 044 + spec 051 contracts expect. No new attack surface introduced. No PII handled. The fix changes only the runtime-mode signal, not any data path.

### Minimum-viable-change audit

| Question | Answer |
|---|---|
| Were any production code, compose, runtime config, or doc files modified by the bug fix? | NO. Only `scripts/commands/config.sh` (the SST loader) was modified, plus two new test files. |
| Are the new tests non-tautological? | YES — the empirical RED state (Sub-test 1 FAIL, Sub-tests 2/3/4 PASS) proves the fix is the home-lab arm and the test isolates the regression contract from the orthogonal dev / test / FR-051-005 contracts. |
| Is the change the smallest viable form? | YES — one structural transformation (single-arm `if` → per-target `case`) with one new arm (`home-lab) SMACKEREL_ENV="production" ;;`). The pre-existing `test)` arm is preserved unchanged. |
| Does the fix preserve all existing related tests? | YES — `TestSSTLoader_RejectsDevPostgresPassword_HomeLab` (FR-051-005 generator-side guard) still PASSES; full `internal/config` and `internal/deploy` Go suites still PASS; spec 042 contract suite (BUG-042-001 + BUG-042-002) still PASSES. |
| Were any operator-coupled or environment-specific values introduced? | NO. The `home-lab) SMACKEREL_ENV="production"` arm is target policy (not operator topology) — the deploy adapter overlay (knb) continues to own all per-target topology. |

**Promotion decision:** SHIP_IT.

## Concerns Carried Forward

| Concern | Severity | Owner | Disposition |
|---|---|---|---|
| The pre-existing `internal/metrics/auth.go` comment-alignment drift (committed at HEAD `9e3fc996`) flags `./smackerel.sh format --check` as failing. The CI workflow does NOT gate on `format --check`, so this does not block any commit. A future format-only fix can reconcile the drift outside the BUG-051-001 boundary. | informational | bubbles.devops | Out-of-scope for BUG-051-001 — pre-existing drift not introduced by this fix; not blocking CI. |
| The `runtime.environment` key in `config/smackerel.yaml` defaults to `development`. A future spec could add first-class per-target overrides (e.g., `targets.home-lab.runtime_environment: production`) so the override lives in the SST yaml instead of being hardcoded in the bundle generator. The current fix is the right size for closing SEC-HL-001 today; first-class per-target SST overrides are a separate design conversation. | informational | bubbles.devops | Out-of-scope by design — design.md > OQ-D resolved this against scope expansion. |
| The `home-lab) SMACKEREL_ENV="production"` arm encodes a target policy in the bundle generator. New deploy targets added by future overlays (e.g., `staging`, `production`, `staging-canary`) will need their own arms in the per-target case if they want runtime-mode overrides. | informational | bubbles.devops | Documented in design.md > Why per-target case and not a single generic loop. The mechanism is documented and extensible. |

## Round Provenance

| Aspect | Detail |
|---|---|
| Sweep | home-lab-deploy-readiness-sprint |
| Source review | bubbles.system-review (four-lens synthesis: devops + security + audit + spec-freshness) |
| Finding ID | SEC-HL-001 |
| Review date | 2026-05-13 |
| Mapped child mode | `test-to-doc` (parent-expanded; nested `runSubagent` blocked by G021 anti-fabrication gate; agent dispatched directly per user authorization) |
| Execution model | parent-expanded-child-mode |
| Fix surface | `scripts/commands/config.sh` (single-edit per-target case + comment block expansion) |
| Test surface | `scripts/commands/config_home_lab_runtime_env_test.sh` (NEW shell driver) + `internal/config/sst_loader_home_lab_runtime_env_test.go` (NEW Go driver mirroring `sst_loader_test.go`) |

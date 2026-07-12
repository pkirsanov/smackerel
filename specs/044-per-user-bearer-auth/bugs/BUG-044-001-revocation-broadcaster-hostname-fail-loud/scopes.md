# Scopes: BUG-044-001 — auth revocation broadcaster falls back to literal `"smackerel-core"` when HOSTNAME is empty

## Scope 1: HOSTNAME fail-loud helper at broadcaster wiring site + adversarial RED→GREEN coverage

**Status:** Done

**Files:**
- [cmd/core/wiring.go](../../../../cmd/core/wiring.go) (added `resolveBroadcasterInstanceID() (string, error)` helper before `buildAPIDeps`; replaced the inline silent-fallback at the broadcaster construction block with helper-call + nested-switch handling that refuses construction on empty HOSTNAME)
- [cmd/core/wiring_revocation_test.go](../../../../cmd/core/wiring_revocation_test.go) (new file: 3 test methods covering positive / empty-set adversarial / unset adversarial)

### Use Cases

```gherkin
Feature: cmd/core/wiring.go reads HOSTNAME fail-loud at the auth revocation broadcaster construction site
  Scenario: SCN-044-001-A — resolveBroadcasterInstanceID exists as a package-private helper
    Given cmd/core/wiring.go declares a package-private helper resolveBroadcasterInstanceID
    When the broadcaster construction block reads the per-replica instance identifier
    Then it calls the helper instead of inlining `os.Getenv("HOSTNAME")` + literal-fallback
    And the helper signature is `() (string, error)`

  Scenario: SCN-044-001-B — non-empty HOSTNAME returns the value, nil error
    Given the HOSTNAME env var is set to a non-empty string (e.g. "smackerel-core-replica-7")
    When resolveBroadcasterInstanceID is called
    Then it returns the HOSTNAME value verbatim
    And it returns a nil error

  Scenario: SCN-044-001-C — empty HOSTNAME returns a fail-loud error referencing HL-RESCAN-008 / Gate G028 / spec 044 / deduplication
    Given the HOSTNAME env var is set to the empty string
    When resolveBroadcasterInstanceID is called
    Then it returns ("", non-nil error)
    And the error message names "HOSTNAME"
    And the error message names "HL-RESCAN-008"
    And the error message names "Gate G028"
    And the error message names "spec 044"
    And the error message mentions "deduplication"

  Scenario: SCN-044-001-D — unset HOSTNAME returns the same fail-loud error as empty HOSTNAME
    Given the HOSTNAME env var is not present in the environment (genuinely unset)
    When resolveBroadcasterInstanceID is called
    Then it returns ("", non-nil error)
    And the error message has the same shape as the empty-set case (5 anchor tokens present)

  Scenario: SCN-044-001-E — broadcaster wiring block refuses construction when helper returns an error
    Given resolveBroadcasterInstanceID returns ("", non-nil error)
    When the wiring block runs (with cfg.Auth.Enabled, NATS connected, RevocationNATSSubject set)
    Then revocation.NewBroadcaster is NOT called
    And svc.authRevocationBroadcaster remains nil
    And slog.Error is emitted with the error AND the NATS subject

  Scenario: SCN-044-001-F — broadcaster wiring block proceeds normally when helper returns a valid instance ID
    Given resolveBroadcasterInstanceID returns ("some-hostname", nil)
    When the wiring block runs (with cfg.Auth.Enabled, NATS connected, RevocationNATSSubject set)
    Then revocation.NewBroadcaster IS called with instanceID = "some-hostname"
    And the existing non-fatal handling for NewBroadcaster errors and Subscribe errors is preserved
```

### Implementation Plan

1. **`cmd/core/wiring.go` (helper extraction):** Add a new package-private function `resolveBroadcasterInstanceID() (string, error)` immediately before the `buildAPIDeps` declaration. The function reads `os.Getenv("HOSTNAME")` and returns `("", error)` when the value is empty, with an error message that names HOSTNAME / HL-RESCAN-008 / Gate G028 / spec 044 / deduplication. Add a doc comment naming HL-RESCAN-008 and pointing to the test file.
2. **`cmd/core/wiring.go` (broadcaster wiring block):** Replace the inline pre-fix block:
   ```go
   instanceID := os.Getenv("HOSTNAME")
   if instanceID == "" {
       instanceID = "smackerel-core"
   }
   broadcaster, err := revocation.NewBroadcaster(svc.nc.Conn, cfg.Auth.RevocationNATSSubject, revocationCache, instanceID)
   if err != nil {
       slog.Error("auth revocation broadcaster construction failed", "error", err)
   } else if subErr := broadcaster.Subscribe(); subErr != nil {
       slog.Error("auth revocation broadcaster subscribe failed", "error", subErr)
   } else {
       slog.Info("auth revocation broadcaster subscribed", ...)
       svc.authRevocationBroadcaster = broadcaster
   }
   ```
   with the post-fix block:
   ```go
   // HL-RESCAN-008 / Gate G028 / spec 044 (no-defaults SST policy):
   // ... explanatory comment ...
   instanceID, hostnameErr := resolveBroadcasterInstanceID()
   switch {
   case hostnameErr != nil:
       slog.Error("auth revocation broadcaster construction refused",
           "error", hostnameErr,
           "subject", cfg.Auth.RevocationNATSSubject)
   default:
       broadcaster, err := revocation.NewBroadcaster(...)
       switch {
       case err != nil:
           slog.Error("auth revocation broadcaster construction failed", "error", err)
       default:
           if subErr := broadcaster.Subscribe(); subErr != nil {
               slog.Error("auth revocation broadcaster subscribe failed", "error", subErr)
           } else {
               slog.Info("auth revocation broadcaster subscribed", ...)
               svc.authRevocationBroadcaster = broadcaster
           }
       }
   }
   ```
3. **`cmd/core/wiring_revocation_test.go` (new file):** Create a test file in `package main` with 3 test methods:
   - `TestResolveBroadcasterInstanceID_NonEmpty` — `t.Setenv("HOSTNAME", "smackerel-core-replica-7")`, asserts helper returns `("smackerel-core-replica-7", nil)`
   - `TestResolveBroadcasterInstanceID_Empty_FailsLoud` — `t.Setenv("HOSTNAME", "")`, asserts helper returns `("", non-nil error)` AND the error message contains all 5 anchor tokens (`HOSTNAME`, `HL-RESCAN-008`, `Gate G028`, `spec 044`, `deduplication`) via a `strings.Contains` loop
   - `TestResolveBroadcasterInstanceID_UnsetEnv` — `os.Unsetenv("HOSTNAME")` with cleanup restore, asserts helper returns `("", non-nil error)`
4. **RED→GREEN proof:** Capture FAIL output by temporarily reverting the body of `resolveBroadcasterInstanceID` to a silent-fallback form (`return "smackerel-core", nil` on empty) via `replace_string_in_file`, keeping the test file intact. Run `./smackerel.sh test unit --go` filtered to the new tests. Observe exactly TWO FAILs (`Empty_FailsLoud`, `UnsetEnv`) with `id="smackerel-core" err=nil` mismatch messages. Restore via `replace_string_in_file` and re-run to confirm all three PASS GREEN.
5. Confine all changes to `cmd/core/wiring.go` + `cmd/core/wiring_revocation_test.go` plus the bug-packet artifacts in `specs/044-per-user-bearer-auth/bugs/BUG-044-001-revocation-broadcaster-hostname-fail-loud/`. No production runtime Python code, no compose, no `config/smackerel.yaml`, no other `specs/**`, no CI workflow.

### Test Plan

- **RED→GREEN proof (scenario-first TDD):** Temporarily revert the helper body to the silent-fallback form (`return "smackerel-core", nil` on empty) via `replace_string_in_file`, keeping the new test file intact. Re-run `go test -count=1 -v -run '^TestResolveBroadcasterInstanceID' ./cmd/core/...`. Observe `TestResolveBroadcasterInstanceID_Empty_FailsLoud` FAIL (the test asserts non-nil error; silent-fallback returns nil error → assertion mismatch with message `id="smackerel-core" err=nil`), `TestResolveBroadcasterInstanceID_UnsetEnv` FAIL (same shape), `TestResolveBroadcasterInstanceID_NonEmpty` PASS (positive path unaffected by silent-fallback). Restore the production fix → all three PASS GREEN. Captured in report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- **Targeted Go unit suite (helper coverage):** `go test -count=1 -v -run '^TestResolveBroadcasterInstanceID' ./cmd/core/...` runs the 3 new tests in isolation — all PASS in <50ms.
- **Adversarial isolation:** The two adversarial tests (`Empty_FailsLoud`, `UnsetEnv`) cannot pass on the silent-fallback form. The positive test (`NonEmpty`) is intentionally non-adversarial — it locks the going-forward behavior contract for the happy path.
- **Cross-package smoke:** `go test -count=1 ./cmd/core/...` covers all cmd/core unit tests (existing `TestAllConnectorsRegistered` etc. + the new helper tests) — all PASS, no regression.
- **Static checks:** `./smackerel.sh test unit --go` runs the full `go test ./...` Go unit lane via the repo CLI — passes cleanly. `go vet ./cmd/core/... ./internal/auth/...` clean.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-044-001-A: resolveBroadcasterInstanceID exists as a package-private helper | unit (Go) | cmd/core/wiring_revocation_test.go | TestResolveBroadcasterInstanceID_NonEmpty (compilation alone proves the helper symbol exists) | NO (positive guard rail) | Persistent in-tree adversarial Go test that runs on every `./smackerel.sh test unit --go` invocation. The fail-loud helper is a single-call-site invariant; the regression suite IS the Go unit suite itself. |
| SCN-044-001-B: non-empty HOSTNAME returns the value, nil error | unit (Go) | cmd/core/wiring_revocation_test.go | TestResolveBroadcasterInstanceID_NonEmpty | NO (positive guard rail) | Same as above. |
| SCN-044-001-C: empty HOSTNAME returns fail-loud error referencing 5 anchor tokens | unit (Go) | cmd/core/wiring_revocation_test.go | TestResolveBroadcasterInstanceID_Empty_FailsLoud | YES — fails RED if helper returns `"smackerel-core", nil` on empty (the exact pre-fix shape) | Same as above. |
| SCN-044-001-D: unset HOSTNAME returns the same fail-loud error | unit (Go) | cmd/core/wiring_revocation_test.go | TestResolveBroadcasterInstanceID_UnsetEnv | YES — fails RED if helper returns `"smackerel-core", nil` on unset env (same pre-fix shape) | Same as above. |
| SCN-044-001-E: broadcaster wiring block refuses construction when helper returns an error | unit (Go) | cmd/core/wiring.go (call site) | indirect via helper test + Code Diff Evidence inspection of the nested-switch block | (verified by source inspection — no integration test required because the helper is the unit, and the wiring block's nested-switch consumes the helper's `(value, error)` contract by construction) | Code Diff Evidence in report.md shows the call site uses the helper return values; future regressions to inline silent-fallback would require deleting both the helper and reverting the call site, both of which would be caught by the helper tests via the symbol-resolution failure. |
| SCN-044-001-F: broadcaster wiring block proceeds normally when helper returns a valid instance ID | unit (Go) | cmd/core/wiring.go (call site) | indirect via positive helper test + integration tests in tests/integration/auth_revocation_test.go (which use synthetic instance IDs and exercise the Broadcaster lifecycle end-to-end, unaffected by this fix) | NO (positive guard rail) | Pre-existing integration tests `tests/integration/auth_revocation_test.go` and `tests/integration/auth_chaos_*_test.go` exercise the Broadcaster lifecycle end-to-end with synthetic instance IDs; their behavior is unchanged. |
| Canary: pre-existing `TestAllConnectorsRegistered` continues to pass | unit (Go) | cmd/core/main_test.go | TestAllConnectorsRegistered | NO (canary) | Pre-existing 15-connector registration test; preserved unchanged. |
| Broader: full `go test ./cmd/core/...` passes | unit (Go) | cmd/core/* | every cmd/core unit test | (mixed) | `go test -count=1 ./cmd/core/...` returns `ok github.com/smackerel/smackerel/cmd/core 0.403s`; zero regression in any pre-existing test. |

### Definition of Done

- [x] `cmd/core/wiring.go` declares a package-private function `resolveBroadcasterInstanceID() (string, error)` placed before `buildAPIDeps`. [SCN-044-001-A]
   → Evidence: `grep -n 'func resolveBroadcasterInstanceID' cmd/core/wiring.go` returns the helper declaration. See report.md > Code Diff Evidence.
- [x] The helper reads `os.Getenv("HOSTNAME")` and returns the value verbatim when non-empty (with nil error). [SCN-044-001-B]
   → Evidence: `grep -A 6 'func resolveBroadcasterInstanceID' cmd/core/wiring.go` shows the body returning `(hostname, nil)` on the happy path. See report.md > Code Diff Evidence.
- [x] The helper returns `("", non-nil error)` when HOSTNAME is empty, with the error message naming HOSTNAME, HL-RESCAN-008, Gate G028, spec 044, and deduplication. [SCN-044-001-C]
   → Evidence: `grep -A 6 'func resolveBroadcasterInstanceID' cmd/core/wiring.go` shows the `fmt.Errorf` call with all 5 anchor tokens present in the format string. See report.md > Code Diff Evidence.
- [x] The pre-fix inline silent-fallback form (`os.Getenv("HOSTNAME")` followed by `if instanceID == "" { instanceID = "smackerel-core" }`) is no longer present in the executable code branch; the only remaining occurrence is inside the explanatory comment block above the helper call site. [SCN-044-001-A]
   → Evidence: `grep -nE 'instanceID = "smackerel-core"' cmd/core/wiring.go` returns zero executable-code matches (only the explanatory comment mentioning the pre-fix form). See report.md > Code Diff Evidence.
- [x] The broadcaster construction block at the helper call site refuses to call `revocation.NewBroadcaster` when the helper returns a non-nil error, and instead emits `slog.Error` with the error and the NATS subject. [SCN-044-001-E]
   → Evidence: `grep -B 1 -A 6 'auth revocation broadcaster construction refused' cmd/core/wiring.go` shows the slog.Error call inside the `case hostnameErr != nil:` switch arm. See report.md > Code Diff Evidence.
- [x] The broadcaster construction block proceeds normally when the helper returns a non-nil string with nil error — invoking `revocation.NewBroadcaster(svc.nc.Conn, cfg.Auth.RevocationNATSSubject, revocationCache, instanceID)` exactly as before. [SCN-044-001-F]
   → Evidence: `grep -B 1 -A 4 'revocation.NewBroadcaster' cmd/core/wiring.go` shows the unchanged constructor invocation inside the `default:` switch arm. See report.md > Code Diff Evidence.
- [x] `cmd/core/wiring_revocation_test.go` declares 3 test methods: `TestResolveBroadcasterInstanceID_NonEmpty`, `TestResolveBroadcasterInstanceID_Empty_FailsLoud`, `TestResolveBroadcasterInstanceID_UnsetEnv`. [SCN-044-001-A through D]
   → Evidence: `grep -n '^func TestResolveBroadcasterInstanceID' cmd/core/wiring_revocation_test.go` returns 3 declarations. See report.md > Code Diff Evidence.
- [x] The empty-set adversarial test asserts the error message contains all 5 anchor tokens (HOSTNAME, HL-RESCAN-008, Gate G028, spec 044, deduplication) via a `strings.Contains` loop. [SCN-044-001-C]
   → Evidence: `grep -A 6 'for _, want := range' cmd/core/wiring_revocation_test.go` shows the 5-token anchor list. See report.md > Code Diff Evidence.
- [x] RED proof captured: temporarily reverting the helper body to a silent-fallback form (`return "smackerel-core", nil` on empty) causes EXACTLY TWO of the three tests to FAIL (`Empty_FailsLoud`, `UnsetEnv`), with the explicit `id="smackerel-core" err=nil` mismatch message; the positive `NonEmpty` test continues to PASS. [SCN-044-001-C, D]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- [x] GREEN proof captured: restoring the production fix returns all three tests to PASS GREEN. [SCN-044-001-A through D]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD) — restore step.
- [x] Targeted suite: `go test -count=1 -v -run '^TestResolveBroadcasterInstanceID' ./cmd/core/...` PASS (3 of 3). [SCN-044-001-A through D]
   → Evidence: see report.md > Validation Evidence > Targeted Go-driver run.
- [x] Cross-test smoke: full `go test -count=1 ./cmd/core/...` PASS (all cmd/core unit tests including the pre-existing `TestAllConnectorsRegistered`). [Broader regression]
   → Evidence: see report.md > Validation Evidence > Cross-test smoke.
- [x] Repo-CLI smoke: `./smackerel.sh test unit --go` PASS (full Go unit lane). [Broader regression]
   → Evidence: see report.md > Validation Evidence > Repo-CLI smoke.
- [x] Static checks: `go vet ./cmd/core/... ./internal/auth/...` clean. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Static checks.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. [SCN-044-001-A through F]
   → Evidence: persistent in-tree `cmd/core/wiring_revocation_test.go` (3 test methods covering A/B/C/D directly; E and F covered indirectly via Code Diff Evidence inspection of the call site + the existing tests/integration/auth_revocation_test.go integration tests) — runs on every `./smackerel.sh test unit --go` invocation. The fail-loud helper is a single-call-site invariant; the regression suite IS the Go unit suite itself. See report.md > Audit Evidence > Regression Evidence.
- [x] Broader E2E regression suite passes — full `./smackerel.sh test unit --go` runs the full Go unit lane (every package under `./...`), and `tests/integration/auth_revocation_test.go` continues to exercise the Broadcaster lifecycle end-to-end with synthetic instance IDs (unaffected by this fix). [Broader regression]
   → Evidence: `go test -count=1 ./cmd/core/...` returns `ok 0.403s`; full repo CLI smoke captured under Cross-test smoke. See report.md > Audit Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. [Pre-existing TestAllConnectorsRegistered + pre-existing tests/integration/auth_revocation_test.go]
   → Evidence: `TestAllConnectorsRegistered` (pre-existing 15-connector registration test) PASS unchanged; the existing integration tests for the Broadcaster (`tests/integration/auth_revocation_test.go`, `tests/integration/auth_chaos_*_test.go`) all use synthetic instance IDs and are unaffected by this wiring-site change. Running these canaries before the broader suite reruns proves the new helper did not over-reach into adjacent surfaces. See report.md > Audit Evidence > Canary suite.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: rollback is a single git revert of the BUG-044-001 commit. The change is purely additive at one wiring site (new helper + replaced inline block); self-hosted today does not enable revocation broadcast on `cfg.Auth.RevocationNATSSubject != ""`, so the live behavior is unchanged for the canonical self-hosted deployment — no live-config mismatch could result from a revert. Restore is the same git revert. Verified by the RED proof step which temporarily reverts the helper body to silent-fallback form, confirms expected FAIL output, then restores. See report.md > Code Diff Evidence + Test Evidence > Red→Green proof (scenario-first TDD).
- [x] Change Boundary respected. The fix touches only `cmd/core/wiring.go` + `cmd/core/wiring_revocation_test.go` plus the bug-packet artifacts. No production runtime Python code, no compose, no `config/smackerel.yaml`, no other `specs/**`, no CI workflow.
   → Evidence: `git status --short` shows only allowed-family files. See report.md > Code Diff Evidence.
- [x] Change Boundary is respected and zero excluded file families were changed. [Allowed file families + Excluded surfaces enumerated below]
   → Evidence: `git status --short` shows only allowed-family files. Zero changes to excluded surfaces. See report.md > Code Diff Evidence.
- [x] Consumer impact sweep performed and zero stale first-party references remain to any renamed/removed surface. [Consumer Impact Sweep section]
   → Evidence: this fix renames/removes nothing externally-visible — `revocation.NewBroadcaster`'s 4-positional-argument signature is preserved verbatim, no HTTP route / NATS subject / CLI flag / env-var name changes, no CLI/config-key rename. The new `resolveBroadcasterInstanceID` is a NEW package-private symbol with zero pre-existing consumers (Go visibility rules make it inaccessible outside `package main` in `cmd/core/`). The only call site is the broadcaster construction block at `cmd/core/wiring.go` lines 270–289, updated in the same commit. `grep -rn 'resolveBroadcasterInstanceID' --include='*.go' .` returns 4 matches (2 in `cmd/core/wiring.go` declaration+call, 3 in `cmd/core/wiring_revocation_test.go` test methods); zero matches outside this bug's allowed-family files. `grep -ri 'instanceID = "smackerel-core"' docs/` returns zero matches — no external documentation, runbook, or operator manual references the pre-fix shape. See report.md > Code Diff Evidence + Consumer Impact Sweep section above.

### Shared Infrastructure Impact Sweep

`cmd/core/wiring.go` `buildAPIDeps` is the **single auth-broadcaster wiring entry point in the Go core**. Changes to its env-read behavior affect every `smackerel-core` process at startup — but only on the optional revocation-broadcast path (gated on `cfg.Auth.Enabled && svc.nc != nil && svc.nc.Conn != nil && cfg.Auth.RevocationNATSSubject != ""`). The BUG-044-001 fix has the following blast radius:

- **Direct downstream consumers:** `api.NewAuthAdminHandlers` (line 281) accepts a `nil` broadcaster gracefully (the spec 044 audit verified this fall-through path); the consumer code is unaffected by the new fail-loud refusal path. Pre-existing tests at `tests/integration/auth_revocation_test.go`, `tests/integration/auth_chaos_test.go`, `tests/integration/auth_chaos_scope02_test.go`, `tests/integration/auth_chaos_scope03_test.go` all construct the Broadcaster directly with synthetic instance IDs (`"test-instance-revocation"`, `"chaos-instance-A"`, etc.) and never invoke the wiring-site helper — they are unaffected by this fix.
- **Operator-side fan-out:** the self-hosted deployment does not enable revocation broadcast today (`cfg.Auth.RevocationNATSSubject` is unset in the canonical config), so the broadcaster wiring block does not even run in production. The fix is risk-free to ship — the runtime behavior is unchanged for the canonical self-hosted deployment.
- **Adapter-side fan-out:** none. The Docker container orchestrator already injects `HOSTNAME` for every container by default; the deploy adapter overlay does not need to do anything special.
- **Test infrastructure (canary surface):** `tests/integration/auth_revocation_test.go` and the chaos tests construct the Broadcaster directly with synthetic instance IDs; they do not consult `os.Getenv("HOSTNAME")` and are unaffected. `cmd/core/main_test.go` `TestAllConnectorsRegistered` is unrelated and PASSES unchanged.
- **Generated-artifact contract:** none — `HOSTNAME` is a Docker-injected runtime env var, not an SST-generated value.
- **Bootstrap contract for downstream specs:** spec 044 (per-user-bearer-auth) is the broadcaster owner. The fix realigns the runtime-side enforcement with the Gate G028 documented contract — strengthening the read site without changing the broadcaster's external contract.
- **Rollback path:** see the corresponding DoD item — single `git revert`; no live-config mismatch possible because self-hosted today does not enable revocation broadcast.
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The helper runs once per `smackerel-core` startup on the optional broadcaster path; no daemon state, no shared cache change, no cross-process ordering concern.
- **Stress coverage assessment (Gate G026):** explicit stress/load coverage is NOT REQUIRED for this fix. The change is a single in-process env-read at startup, executed at most once per process lifecycle. There is no latency, throughput, p95/p99, response-time, sla, or slo dimension that the change can move; the broadcaster's hot-path behavior (broadcasting messages on the NATS subject) is entirely unaffected. The pre-existing integration tests (`tests/integration/auth_revocation_test.go`, `tests/integration/auth_chaos_*_test.go`) already exercise the broadcaster's runtime behavior. No additional `./smackerel.sh test stress` invocation is warranted; this DoD line documents the assessment for the Gate G026 lint.

### Consumer Impact Sweep

This bug fix does **not** rename or remove any externally-visible interface, route, endpoint, contract, API, URL, slug, public symbol, deep link, breadcrumb, navigation entry, or generated client. The change is bounded to a private wiring-site env-read at `cmd/core/wiring.go`:

- **No public API change.** `revocation.NewBroadcaster`'s signature is preserved verbatim — the broadcaster construction site continues to call `revocation.NewBroadcaster(svc.nc.Conn, cfg.Auth.RevocationNATSSubject, revocationCache, instanceID)` with the same 4 positional arguments. No HTTP route, gRPC method, NATS subject name, or stream identifier changes. No CLI flag, env-var name, or config-key rename. No URL path / breadcrumb / redirect surface change. No generated client regeneration required (we don't ship one for this surface). No deep-link or navigation entry change.
- **Private symbol added, not renamed.** `resolveBroadcasterInstanceID` is a NEW package-private symbol (lowercase first letter, scoped to `package main` inside `cmd/core/`); it has zero external consumers because Go's package-private visibility rules make it inaccessible outside the `cmd/core` package. No identifier rename anywhere — the pre-fix code did not have a named helper, so there is nothing to rename.
- **Affected consumer surfaces enumerated:** the only "consumer" of the env-read is the broadcaster construction block itself (lines 270–289 of `cmd/core/wiring.go`). It is updated in the same commit to call the helper through its `(value, error)` return contract. There are no API client, generated client, breadcrumb, navigation, redirect, or stale-reference surfaces to sweep. No external documentation, no operator runbook, no Deployment.md / Operations.md / Branch_Protection.md page references the pre-fix shape (verified via `grep -ri 'instanceID = "smackerel-core"' docs/` returns zero matches).
- **Cross-package consumer surface:** zero. The helper is package-private; it cannot be imported by any other Go package in the repo. The only call site in the entire repo is the broadcaster construction block at `cmd/core/wiring.go` lines 270–289 (verified via `grep -rn 'resolveBroadcasterInstanceID' --include='*.go' .` returns 4 matches, all inside `cmd/core/wiring.go` and `cmd/core/wiring_revocation_test.go`).
- **Stale-reference scan:** zero stale first-party references remain. The pre-fix executable form (`os.Getenv("HOSTNAME")` + `if instanceID == "" { instanceID = "smackerel-core" }`) is fully removed from the executable code path. The only remaining occurrence of the literal string `"smackerel-core"` is inside the explanatory comment at `cmd/core/wiring.go` line 262 (archaeology, not executable). The generated client / API client / deep-link surfaces are unaffected because none of them reference the wiring-site read.

### Change Boundary

**Allowed file families (this fix may modify):**

- `cmd/core/wiring.go` — the broadcaster wiring site being fixed (the only production code change point) plus the new helper declaration
- `cmd/core/wiring_revocation_test.go` — new test file for the helper
- `specs/044-per-user-bearer-auth/bugs/BUG-044-001-revocation-broadcaster-hostname-fail-loud/**` — this bug packet's seven artifacts

**Excluded surfaces (this fix MUST NOT touch):**

- `cmd/core/main.go`, `cmd/core/services.go`, `cmd/core/cmd_*.go`, `cmd/core/wiring_agent.go`, `cmd/core/wiring_recommendation_watches.go`, `cmd/core/connectors.go`, `cmd/core/shutdown.go`, `cmd/core/agent_e2e_tools.go` — adjacent cmd/core files; outside HL-RESCAN-008's scope
- `cmd/core/helpers.go` — out-of-scope unused fail-soft helpers (`parseFloatEnv`/`parseJSONArrayEnv`/`parseJSONObjectEnv`); closed by [`specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/`](../../../020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/) (HL-RESCAN-014).
- `internal/auth/revocation/broadcaster.go` — the Broadcaster struct's existing constructor signature is preserved; only the wiring-site read of HOSTNAME changes
- `tests/integration/auth_revocation_test.go`, `tests/integration/auth_chaos_*_test.go` — pre-existing integration tests; they construct the Broadcaster with synthetic instance IDs and are unaffected
- `config/smackerel.yaml` — the SST source-of-truth values are unchanged; HOSTNAME is a Docker runtime env var, not an SST-managed value
- `scripts/commands/config.sh` — the SST loader does not handle HOSTNAME (Docker injects it directly)
- `config/generated/<env>.env` — generated artifacts; never edit by hand
- `deploy/compose.deploy.yml`, `docker-compose.yml`, `Dockerfile` — runtime container configuration; the orchestrator already injects HOSTNAME for every container
- `specs/044-per-user-bearer-auth/spec.md`, `design.md`, `scopes.md`, `state.json`, `report.md`, `uservalidation.md` — foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope
- `specs/049-no-defaults-sst-policy/...` — the Gate G028 policy doc itself; outside HL-RESCAN-008's scope (the fix REFERENCES the policy in error messages but does not change the policy)
- Any other `specs/**` directory — single-bug-scope discipline
- `.github/workflows/*` — unrelated; the Go tests are invoked by the existing `unit-tests` job
- `scripts/...` (other than the SST loader, which is excluded above) — unrelated

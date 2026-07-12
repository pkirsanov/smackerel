# Scopes — Spec 099 Pre-Flight Resource Guard

**Feature:** [spec.md](spec.md) · **Design:** [design.md](design.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

> Single-file scope mode (3 scopes). SCOPE-01 ships the SST thresholds + the Go
> resource evaluator + its adversarial unit tests (the fully-testable core).
> SCOPE-02 wires the guard into the `smackerel.sh` heavy-op paths + the
> standalone `pre-flight` subcommand and ships the lockstep drift-detector
> contract test. SCOPE-03 documents the guard, override, and scope boundary.

---

## Scope 1: SCOPE-01 — SST thresholds + Go resource evaluator + unit tests

**Status:** Done
**Scope-Kind:** contract-only
**Depends On:** —

> Scope-Kind rationale (Check 8A opt-out): this scope ships a **pure Go
> evaluator + SST plumbing** proven by the in-repo unit suite + the config
> wiring-contract test. The `e2e-api` / `e2e-ui` categories **do not apply** —
> there is no HTTP API surface and no UI; the evaluator is host-side logic with
> synthetic-input unit coverage and a static-file contract test. The top tier of
> evidence is the deterministic Go suite (`unit` category), not a live-runtime
> E2E scenario. (Same opt-out shape as spec 098's `ci-config` / `docs-only`.)

Add the `runtime.preflight` SST block + config-pipeline emission, and implement
the Go evaluator (`internal/preflight` pure core + I/O helpers, `cmd/preflight`
glue) with adversarial unit coverage.

### Gherkin Scenarios

```gherkin
Scenario: SCN-099-A02 — Below threshold reports current-vs-required and exits 1
  Given SST minimums for RAM and disk
  And observed host resources below at least one minimum
  When the evaluator runs
  Then it returns a non-zero exit code
  And the report states current free RAM/disk vs the required minimum
  And it lists concrete remediation

Scenario: SCN-099-A03 — A missing threshold key fails loud (NO-DEFAULTS)
  Given an env map missing PREFLIGHT_MIN_AVAILABLE_RAM_MB (or _DISK_GB)
  When the evaluator parses thresholds
  Then it errors naming the missing key
  And it never substitutes a silent default

Scenario: SCN-099-A04 — Override bypasses the gate with a loud WARNING
  Given observed resources below the minimum
  And the override flag is set
  When the evaluator runs
  Then it returns exit code 0
  And the report contains a loud WARNING that the check was overridden

Scenario: SCN-099-A06 — SST thresholds flow to the generated env files
  Given config/smackerel.yaml sets runtime.preflight.min_available_ram_mb and
        min_available_disk_gb
  When ./smackerel.sh config generate runs
  Then config/generated/dev.env and test.env carry PREFLIGHT_MIN_AVAILABLE_RAM_MB
       and PREFLIGHT_MIN_AVAILABLE_DISK_GB with positive integer values
```

### Implementation plan
1. `config/smackerel.yaml`: add a `preflight:` map under `runtime:` (after
   `trusted_proxies`) with `min_available_ram_mb: 6000` +
   `min_available_disk_gb: 15`.
2. `scripts/commands/config.sh`: add
   `PREFLIGHT_MIN_AVAILABLE_RAM_MB="$(required_value runtime.preflight.min_available_ram_mb)"`
   + `…_DISK_GB="$(required_value runtime.preflight.min_available_disk_gb)"`
   near the other `required_value` reads, and emit both into the generated env
   heredoc after `COMPOSE_WAIT_TIMEOUT_S`.
3. `internal/preflight/preflight.go`: `Thresholds`, `Resources`, `Result`,
   `ParseThresholds` (fail-loud), `Evaluate` (pure), `FormatReport` (numbers-only,
   no secrets), `Run` (decision path + override), `LoadEnvFile`,
   `ReadMemAvailableMB`/`readMemAvailableMBFrom`, `ReadDiskAvailableMB`, `Truthy`.
4. `cmd/preflight/main.go`: parse `--env` + `--repo-root` (both required,
   fail-loud), read host mem + disk, call `Run`, print report, exit with the code.
5. `internal/preflight/preflight_test.go`: adversarial unit tests.

### Test Plan
| Test Type | Category | File | Description | Command |
|-----------|----------|------|-------------|---------|
| unit | unit | `internal/preflight/preflight_test.go` | `Evaluate` at/above→OK, below→short; `ParseThresholds` missing/empty/non-numeric/non-positive→fail-loud naming key; `Run` below→exit 1 + current+required, override→exit 0 + WARNING; `FormatReport` no-secret (planted `SMACKEREL_AUTH_TOKEN`); `readMemAvailableMBFrom` synthetic `/proc/meminfo`; `ReadDiskAvailableMB` temp dir | `./smackerel.sh test unit --go --go-run 'Preflight'` |
| config (regression) | unit | `config/generated/{dev,test}.env` | `./smackerel.sh config generate` emits `PREFLIGHT_MIN_AVAILABLE_RAM_MB` + `_DISK_GB` (proven by the wiring contract test in SCOPE-02) | `./smackerel.sh config generate` |

### Definition of Done
- [x] SCN-099-A02: `Evaluate` + `Run` flag below-threshold and the report states current-vs-required + remediation → Evidence: [report.md#unit]
- [x] SCN-099-A03: `ParseThresholds` fails loud naming a missing/empty/non-positive key; NO default substituted (adversarial) → Evidence: [report.md#unit]
- [x] SCN-099-A04: override → exit 0 with a loud WARNING in the report → Evidence: [report.md#unit]
- [x] SCN-099-A06: `config/smackerel.yaml` carries `runtime.preflight.*`; `config.sh` emits both keys via `required_value`; generated `dev.env`/`test.env` carry positive integer values → Evidence: [report.md#config]
- [x] No-secret: `FormatReport` never echoes an env value other than the numeric thresholds (adversarial planted secret) → Evidence: [report.md#unit]
- [x] Build Quality Gate: `./smackerel.sh check` + `./smackerel.sh lint` clean; scoped Go unit suite green → Evidence: [report.md#quality]

---

## Scope 2: SCOPE-02 — CLI wiring (heavy-op gates + `pre-flight` subcommand) + drift contract test

**Status:** Done
**Scope-Kind:** contract-only
**Depends On:** SCOPE-01

> Scope-Kind rationale (Check 8A opt-out): this scope wires a host-side CLI
> guard. The `e2e-api` / `e2e-ui` categories **do not apply** — there is no HTTP
> API and no UI. The correct top tier is the live `./smackerel.sh pre-flight`
> invocation (functional smoke) plus the lockstep **wiring-contract
> drift-detector** (`unit` category) that parses the live `smackerel.sh` and
> fails if the guard is ever removed from a heavy-op path.

Add `smackerel_assert_host_resources()` (host-`go` primary, dockerized fallback),
wire it into `build` / `up` / `test integration|e2e|e2e-ui|stress`, add the
standalone `pre-flight` command + usage entry, and ship the lockstep
drift-detector contract test.

### Consumer Impact Sweep

This scope is **purely additive**: it inserts a `smackerel_assert_host_resources`
pre-op gate into existing `smackerel.sh` heavy-op paths and adds one new
standalone command. It renames or removes **no** command, flag, env var,
contract, route, or symbol, so there is no stale-reference surface at all.

| Consumer (call site) | Impact | Breaking? |
|----------------------|--------|-----------|
| `build)` | gains a pre-op resource gate before `build_args=(build)` | No — additive; override env preserves the escape hatch |
| `up)` | gains the gate before bring-up | No — additive |
| `test integration)` / `test e2e)` / `test e2e-ui)` / `test stress)` | gains the gate before stack-up | No — additive; `test unit` stays ungated |
| `pre-flight)` (NEW) | new standalone read-only command | No — new surface, nothing pre-existing depends on it |
| Light ops (`status`/`logs`/`config`/`check`/`down`/`clean`/`test unit`/`e2e-ext`) | unchanged | No — intentionally ungated |

No navigation, breadcrumb, redirect, API client, generated client, or deep link
surface is touched (this is a host-side CLI guard, not a product surface);
zero stale-reference updates are needed. The drift-detector contract test pins
the exact set of guarded call sites so an accidental removal is rejected.

### Gherkin Scenarios

```gherkin
Scenario: SCN-099-A01 — Standalone pre-flight passes when the host has headroom
  Given the host has more than the SST minimums free
  When the operator runs `./smackerel.sh pre-flight`
  Then it prints current-vs-required for RAM and disk
  And it exits 0

Scenario: SCN-099-A05 — Heavy ops invoke the guard; the contract test proves wiring
  Given the live smackerel.sh
  When the drift-detector contract test parses it
  Then build, up, and test integration|e2e|e2e-ui|stress each call
       smackerel_assert_host_resources
  And the helper invokes cmd/preflight (the Go evaluator)
  And the pre-flight command exists
  And an adversarial mutation that removes the guard from a heavy-op block is rejected
```

### Implementation plan
1. `smackerel.sh`: add `smackerel_assert_host_resources()` after
   `smackerel_assert_host_ports_free`; host path
   `go run ./cmd/preflight --env <env> --repo-root "$SCRIPT_DIR"`, fallback
   `run_go_tooling /workspace/scripts/runtime/preflight.sh <env>`.
2. `scripts/runtime/preflight.sh`: dockerized wrapper (`cd /workspace; go run
   ./cmd/preflight --env <env> --repo-root /workspace`).
3. Wire the guard into `build)`, `up)`, `test integration)`, `test e2e)`
   (after the e2e suite lock, before stack-up), `test stress)`, `test e2e-ui)`
   (skip for `--print-compose-project`).
4. Add a top-level `pre-flight)` case (ensure env file via
   `smackerel_require_env_file`, then call the helper) + a `pre-flight` line in
   `usage()`.
5. `internal/preflight/wiring_contract_test.go`: parse `smackerel.sh` +
   `config.sh` + `config/smackerel.yaml` + generated env; assert wiring + SST
   keys; adversarial drift sub-tests.

### Test Plan
| Test Type | Category | File | Description | Command |
|-----------|----------|------|-------------|---------|
| unit (contract) | unit | `internal/preflight/wiring_contract_test.go` | Live-file: helper defined + invokes `cmd/preflight`; guard wired in build/up/integration/e2e/e2e-ui/stress; `pre-flight)` case present; SST keys present in yaml + config.sh + generated env; 2 adversarial drift sub-tests (strip guard from build block → reject; strip `cmd/preflight` from helper → reject) | `./smackerel.sh test unit --go --go-run 'Preflight'` |
| smoke | functional | `./smackerel.sh pre-flight` | Real host run prints current-vs-required and exits 0/1 truthfully | `./smackerel.sh pre-flight` |

### Definition of Done
- [x] SCN-099-A01: `./smackerel.sh pre-flight` runs the real host check, prints current-vs-required, exits truthfully → Evidence: [report.md#preflight-run]
- [x] SCN-099-A05: contract test asserts the guard is wired into every heavy-op path + the helper runs `cmd/preflight`; adversarial drift sub-tests reject a removed guard (non-tautological) → Evidence: [report.md#unit]
- [x] Consumer Impact Sweep: the additive `smackerel_assert_host_resources` call into the build/up/integration/e2e/e2e-ui/stress call sites renames or removes no command, flag, contract, or symbol — zero stale first-party references remain → Evidence: [report.md#consumer-impact]
- [x] `usage()` documents the `pre-flight` command; light ops (`status`/`logs`/`config`/`check`/`down`/`clean`/`test unit`) remain ungated → Evidence: [report.md#wiring]
- [x] Override path: `SMACKEREL_PREFLIGHT_OVERRIDE=1 ./smackerel.sh pre-flight` proceeds with a loud WARNING → Evidence: [report.md#preflight-run]
- [x] Build Quality Gate: `./smackerel.sh check` + `./smackerel.sh lint` clean → Evidence: [report.md#quality]

---

## Scope 3: SCOPE-03 — Document the resource guard + override + scope boundary

**Status:** Done
**Scope-Kind:** docs-only
**Depends On:** SCOPE-02

Document the new `pre-flight` command, the automatic heavy-op gating, the
`SMACKEREL_PREFLIGHT_OVERRIDE` escape hatch, the SST thresholds, and the
local-only scope boundary (NOT a self-hosted apply gate).

### Gherkin Scenarios

```gherkin
Scenario: SCN-099-A07 — Docs describe the resource guard and its boundary
  Given an operator reads the development/testing docs
  When they look up resource pre-flight
  Then the docs state which heavy ops are gated and which are not
  And they state the SST thresholds (runtime.preflight.*) and the override env var
  And they state the guard protects local CLI ops only, not the self-hosted apply
```

### Implementation plan
1. `docs/Development.md`: add a "Resource Pre-Flight Guard" subsection (command,
   gated ops, override, SST keys, scope boundary), cross-referencing spec 099.
2. `README.md` (Required Runtime Standards / commands area): add the
   `./smackerel.sh pre-flight` command to the command surface.

### Test Plan
| Test Type | Category | File / Location | Description | Command |
|-----------|----------|-----------------|-------------|---------|
| docs review | functional | `docs/Development.md`, `README.md` | The new content states gated ops, SST keys, override, and the local-only boundary | manual read + grep for the new heading |

### Definition of Done
- [x] SCN-099-A07: `docs/Development.md` documents the command, gated/ungated ops, override, SST thresholds, and local-only scope boundary; `README.md` lists the command → Evidence: [report.md#docs]
- [x] No env-specific content / real secrets introduced (generic only) → Evidence: [report.md#docs]

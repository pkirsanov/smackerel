# Design — Spec 099 Pre-Flight Resource Guard

**Feature:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

## Current Truth (verified this session, solution-blind)

Read directly from the repo before designing (no assumptions):

| Fact | Source (verified) |
|------|-------------------|
| `smackerel.sh` already has a host-port preflight `smackerel_assert_host_ports_free` implemented as an embedded host-`python3` heredoc (`python3 - … <<'PY' … PY`) that binds each configured host port and fails loud with owner attribution before the test stack starts | `smackerel.sh` lines 176–545 |
| The runtime library exposes `smackerel_env_value <env_file> <key>` (awk read), `smackerel_require_env_file <env>` (generate-if-missing), `smackerel_generate_config <env>` (→ `scripts/commands/config.sh`), `smackerel_require_env_value` (fail-loud read) | `scripts/lib/runtime.sh` + `smackerel.sh` line 163 |
| Heavy ops do `require_docker` + `smackerel_generate_config` then the heavy work — `build` (config-gen line 675), `up` (env_file line 1818), `test integration` (line 969), `test e2e` (suite lock line 1123; first `--env test up` at line 1512; Go config-gen at 1535), `test stress` (config-gen line 1710), `test e2e-ui` (`exec bash scripts/runtime/web-e2e-ui.sh` line ~1788) | `smackerel.sh` |
| `test e2e-ext` is **self-contained** (per-test recording server, real headless Chromium, NO live stack, NO compose project) → NOT a heavy live-stack category | `smackerel.sh` line 1793 comment |
| Config SST: `config/smackerel.yaml` has a top-level `runtime:` block (log_level, auth_token, environment, host_bind_address: "127.0.0.1", compose_wait_timeout_s, digest_cron, …, trusted_proxies: []) ending before the top-level `cors:` block | `config/smackerel.yaml` runtime block |
| `scripts/commands/config.sh` reads runtime values with `required_value runtime.<key>` (fail-loud: `Missing config key: <key>`) around lines 747–751, and emits them into the generated env file via a heredoc (line ~1911 `HOST_BIND_ADDRESS=…`, `COMPOSE_WAIT_TIMEOUT_S=…`) | `scripts/commands/config.sh` |
| `flatten_yaml` supports nested keys to indent 8 (`level1.level2.level3.level4.level5`), so `runtime.preflight.min_available_ram_mb` (indent 0/2/4) flattens correctly and `required_value runtime.preflight.min_available_ram_mb` fails loud when absent | `scripts/commands/config.sh` lines 80–175 |
| The host has a Go toolchain (`go version go1.25.10`); the repo ALSO ships a dockerized Go runner `run_go_tooling` (`golang:1.25.10-bookworm`, `-v $SCRIPT_DIR:/workspace`) used by `check`/`lint`/`format` | host probe + `smackerel.sh` line 103 |
| Go module path is `github.com/smackerel/smackerel`; existing cmds: `cmd/{config-validate,core,dbmigrate,scenario-lint,cardrewards-import,web-assistant-codegen}` | `go.mod` line 1 + `cmd/` listing |
| The contract-test precedent uses `runtime.Caller(0)` to resolve repo root, parses the live static file, and ships adversarial sub-tests that mutate the input and assert the contract function REJECTS (proves non-tautological) | `internal/deploy/compose_contract_test.go` |

## Design Decisions (resolved, with rationale)

### D1 — Evaluation logic in Go (`cmd/preflight` + `internal/preflight`)

The spec offered two implementations: mirror the embedded-`python3` port
preflight, OR Go. **Chosen: Go**, because:

1. The repo's test culture is Go contract tests (`internal/deploy/*_contract_test.go`),
   and the spec explicitly requires a wiring contract test "mirroring the style
   of `internal/deploy/compose_contract_test.go`". Putting the evaluator in Go
   keeps the evaluation **and** the wiring test in one language, run by one
   command (`./smackerel.sh test unit --go`), with **no two-language drift**.
2. Constitution C2 (Go-First Runtime) — operational logic belongs in Go, not the
   Python ML sidecar.
3. The pure decision logic (`Evaluate`, `ParseThresholds`, `FormatReport`,
   `Run`) is trivially unit-testable with synthetic inputs and adversarial cases.

The package is split into a **pure core** (no I/O — fully unit-tested) and **thin
host-I/O helpers**:

- `internal/preflight/preflight.go`
  - `Thresholds{MinAvailableRAMMB, MinAvailableDiskGB int64}`
  - `Resources{AvailableRAMMB, AvailableDiskMB int64}`
  - `ParseThresholds(env map[string]string) (Thresholds, error)` — reads
    `PREFLIGHT_MIN_AVAILABLE_RAM_MB` + `PREFLIGHT_MIN_AVAILABLE_DISK_GB`;
    **fail-loud** (error names the key) on missing / empty / non-numeric /
    non-positive. NO default is ever supplied (Gate G028).
  - `Evaluate(res, th) Result{OK, RAMShort, DiskShort bool}` — pure comparison.
  - `FormatReport(res, th, result, overridden bool) string` — renders the
    current-vs-required report + remediation; interpolates ONLY the four numeric
    values (two thresholds, two observed), never an env value → structurally
    cannot echo a secret.
  - `Run(env, res, overridden) (report string, exitCode int, err error)` — the
    full decision path (ParseThresholds → Evaluate → FormatReport → exit code).
    Override forces exitCode 0 with the warning baked into the report.
  - `LoadEnvFile(path) (map[string]string, error)` — parse `key=value` lines.
  - `ReadMemAvailableMB() (int64, error)` — parse `MemAvailable:` from
    `/proc/meminfo` (kB→MB); split into `readMemAvailableMBFrom(path)` so a unit
    test can feed a synthetic file.
  - `ReadDiskAvailableMB(path) (int64, error)` — `syscall.Statfs(path)`,
    `Bavail*Bsize` → MB.
  - `Truthy(s) bool` — override parsing (`1/true/yes/on`).
- `cmd/preflight/main.go` — glue: parse `--env <name>` + `--repo-root <path>`
  (both REQUIRED, fail-loud — no default), build env-file path
  `<repo-root>/config/generated/<env>.env`, read host mem + disk (disk path =
  repo-root), call `Run`, print the report, `os.Exit(exitCode)`.

### D2 — Host-native invocation, dockerized fallback

`smackerel_assert_host_resources <env>` (new helper in `smackerel.sh`):

```sh
if command -v go >/dev/null 2>&1; then
  ( cd "$SCRIPT_DIR" && go run ./cmd/preflight --env "<env>" --repo-root "$SCRIPT_DIR" )
else
  require_docker
  run_go_tooling /workspace/scripts/runtime/preflight.sh "<env>"
fi
```

- **Host path (primary):** `go run ./cmd/preflight` — fast, no container, and
  unambiguously host-correct (real `/proc/meminfo`, real `statfs` on the real
  repo path). `cmd/preflight` imports only stdlib + `internal/preflight` (which
  imports only stdlib), so `go run` compiles repo code + stdlib with **no
  network**, cached after first build.
- **Docker fallback (portability):** `scripts/runtime/preflight.sh` runs
  `go run ./cmd/preflight --env <env> --repo-root /workspace` inside the
  `golang:1.25.10-bookworm` container that `run_go_tooling` already uses for
  `check`/`lint`/`format`. This is **host-correct** there too: `run_go_tooling`
  sets **no `--memory` cgroup limit**, so the container's `/proc/meminfo` shows
  **host** `MemAvailable`; and `/workspace` is a **bind mount** of the repo, so
  `statfs("/workspace")` follows to the **host** filesystem backing the repo.
  The heavy ops this guards all require Docker anyway, so the fallback adds no
  new dependency.

Trade-off recorded honestly: the host path is preferred for speed; the
container-start cost of the fallback (~1–2 s, image already cached) is negligible
relative to the multi-minute heavy op it protects — and it exists to *prevent* a
20-minute doomed run.

### D3 — Disk target = repo filesystem

The spec allowed "Docker data root / repo filesystem". **Chosen: the repo
filesystem** (`statfs(repo-root)`), because it is unambiguous, host-correct under
both invocation paths (the bind-mounted `/workspace` resolves to the same host
fs), and on a typical single-disk dev box is the same filesystem that backs both
the build context and `/var/lib/docker`. Operators whose Docker root is on a
separate disk are an explicit non-goal for v1 (Out of Scope).

### D4 — SST thresholds + values

New `config/smackerel.yaml` block (indent-2 map under `runtime:`, after
`trusted_proxies`):

```yaml
  preflight:
    min_available_ram_mb: 6000
    min_available_disk_gb: 15
```

Rationale for the concrete values (allowed in SST — only *code/compose fallbacks*
are forbidden): a heavy `build`/`integration` compile + container bring-up peaks
around 6–10 GB on this loop (the OOM observations), so **6000 MB** is the
fail-fast floor; Ollama models (several GB each) plus Docker image layers make
**15 GB** a sensible disk floor before a heavy run. `config.sh` emits them with
`required_value` (fail-loud) into the env heredoc:

```sh
PREFLIGHT_MIN_AVAILABLE_RAM_MB="$(required_value runtime.preflight.min_available_ram_mb)"
PREFLIGHT_MIN_AVAILABLE_DISK_GB="$(required_value runtime.preflight.min_available_disk_gb)"
# … emitted in the generated env heredoc after COMPOSE_WAIT_TIMEOUT_S:
PREFLIGHT_MIN_AVAILABLE_RAM_MB=${PREFLIGHT_MIN_AVAILABLE_RAM_MB}
PREFLIGHT_MIN_AVAILABLE_DISK_GB=${PREFLIGHT_MIN_AVAILABLE_DISK_GB}
```

### D5 — Wiring points (verified line numbers, pre-edit)

| Op | Insert the guard | Env |
|----|------------------|-----|
| `build)` | after config-gen (line 675), before `build_args=(build)` | `$TARGET_ENV` |
| `up)` | after `env_file=…` (line 1818), before bring-up | `$TARGET_ENV` |
| `test integration)` | after config-gen (line 969), before stack-up | `test` |
| `test e2e)` | after `smackerel_acquire_e2e_suite_lock test` (line 1123) — BEFORE the first `--env test up` (line 1512); generate config first | `test` |
| `test stress)` | after config-gen (line 1710), before stack-up | `test` |
| `test e2e-ui)` | before `exec bash …web-e2e-ui.sh`, **skipped** for the read-only `--print-compose-project` flag | `test` |
| `pre-flight)` (NEW top-level case) | runs the guard standalone; ensures env file exists first | `$TARGET_ENV` |

**Not gated** (light / read-only): `config`, `check`, `down`, `status`, `logs`,
`clean`, `test unit`, `test e2e-ext` (self-contained, no live stack).

### D6 — Override semantics

`SMACKEREL_PREFLIGHT_OVERRIDE` truthy → `Run` returns exitCode 0 and
`FormatReport` appends a loud `WARNING: SMACKEREL_PREFLIGHT_OVERRIDE is set —
proceeding DESPITE the resource check…`. The override never suppresses the
report; it only changes the exit code. This keeps the bypass auditable (the
warning is always printed) and honors "never silently".

### D7 — No-secret guarantee

`FormatReport` only ever interpolates the four numeric values. `cmd/preflight`
reads the WHOLE generated env file (which contains secrets like
`SMACKEREL_AUTH_TOKEN`) into a map, but uses ONLY the two `PREFLIGHT_*` keys and
passes nothing else to the report. The adversarial unit test plants a sentinel
secret in the env map and asserts the rendered report does **not** contain it
(non-tautological: a naive "dump the env" implementation would fail).

## Contract-test design (`internal/preflight/wiring_contract_test.go`)

Mirrors `compose_contract_test.go`:

- `repoRoot(t)` via `runtime.Caller(0)` → `../..` from `internal/preflight/`.
- Parse the live `smackerel.sh`, `scripts/commands/config.sh`,
  `config/smackerel.yaml`, and the generated `config/generated/{dev,test}.env`.
- `assertGuardWired(script)` returns an error naming the first missing wiring:
  - the helper `smackerel_assert_host_resources()` is defined AND invokes
    `cmd/preflight` (the Go evaluator) — proving the guard runs real logic;
  - each heavy-op case block (`build)`, `up)`, `test integration)`, `test e2e)`,
    `test stress)`, `test e2e-ui)`) contains a `smackerel_assert_host_resources`
    call;
  - a `pre-flight)` top-level case exists and calls the helper.
- `assertConfigWired` asserts `runtime.preflight.{min_available_ram_mb,
  min_available_disk_gb}` present in `config/smackerel.yaml`, that `config.sh`
  emits `PREFLIGHT_MIN_AVAILABLE_RAM_MB`/`_DISK_GB` via `required_value`, and
  that the generated env files carry both keys with positive integer values.
- **Adversarial sub-tests** (prove non-tautological): take the live `smackerel.sh`
  text, delete the `smackerel_assert_host_resources` line from the `build)` block
  (and, separately, from the helper-body `cmd/preflight` invocation), and assert
  `assertGuardWired` REJECTS with the expected message.

## Test taxonomy

| Test | Type | Category | What it proves |
|------|------|----------|----------------|
| `internal/preflight/preflight_test.go` | unit | unit | pure evaluation + parse + report + I/O helpers, adversarial (missing-key fail-loud, below→1, at/above→0, override→0+WARN, no-secret) |
| `internal/preflight/wiring_contract_test.go` | unit (contract) | unit | guard is wired into every heavy-op path + reads SST keys + generated env carries them; adversarial drift detection |

Both run under `./smackerel.sh test unit --go` (the dockerized Go unit runner),
so no live stack is needed and the tests stay hermetic.

## Capability & Implementation Shape

### Single-Implementation Justification

There is exactly **one** implementation and **no** foundation/overlay split, so
a Capability Foundation / Concrete Implementations / Variation Axes model does
not apply. The delivery is one pure Go evaluator (`internal/preflight`: the
`Thresholds`/`Resources`/`Result` types + `ParseThresholds`/`Evaluate`/
`FormatReport`/`Run`), thin host-I/O helpers (`ReadMemAvailableMB`,
`ReadDiskAvailableMB`, `LoadEnvFile`), one `cmd/preflight` glue binary, and one
`smackerel.sh` helper wired into the existing heavy-op case arms. RAM and disk
are **two inputs to one comparison**, not two concrete implementations of a
capability — they share the same `Evaluate`, the same threshold source, and the
same report. The host-native `go run` path and the dockerized
`scripts/runtime/preflight.sh` fallback are **two invocation transports for the
identical binary**, not two implementations of the evaluator (D2). No new
abstraction, interface, provider, strategy, or adapter is introduced; the
drift-detector contract test asserts the single wired shape with adversarial
coverage. A second concrete implementation (per-operation thresholds, a CPU/GPU
dimension, a pluggable backend) is explicitly Out of Scope for v1 and would
contradict this design's minimality.

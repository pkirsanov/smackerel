# BUG-099-001 — spec-099 pre-flight crashes on macOS (host-native go reads Linux /proc/meminfo)

- **Parent spec:** 099-preflight-resource-guard
- **Release train:** mvp
- **Severity:** High (blocks ALL gated `./smackerel.sh` ops on every macOS dev host — `build`, `up`, `test integration`, `test integration-light`, `test e2e|e2e-ui|stress`, `pre-flight` — because every one routes through the same pre-flight helper)
- **Status:** done (PROVEN — see "Proven Fixed" below; the integration-light re-run reached a real pre-flight verdict instead of the /proc/meminfo crash)
- **Discovered:** 2026-06-30 (bubbles.devops, running the new integration-light lane on a macOS host — OPS-005 F-RUNBOOK follow-on)
- **Owner:** bubbles.devops

## Summary

`./smackerel.sh --env test test integration-light --go-run ...` failed at the
spec-099 pre-flight on a macOS host with:

```
ERROR: smackerel pre-flight: read host RAM: open /proc/meminfo: no such file or directory
```

No stack was started — the gate crashed before its verdict.

## Root Cause

`smackerel_assert_host_resources_profile` (smackerel.sh) took the **host-native**
path — `go run ./cmd/preflight` — whenever a host `go` toolchain exists:

```bash
if command -v go >/dev/null 2>&1; then
  ( cd "$SCRIPT_DIR" && go run ./cmd/preflight ... )   # host-native
else
  require_docker; run_go_tooling .../preflight.sh ...   # dockerized
fi
```

`cmd/preflight` → `internal/preflight.ReadMemAvailableMB()` reads **Linux**
`/proc/meminfo`. That pseudo-file does **not** exist on macOS/darwin, so the
host-native path fails-loud the moment a macOS dev runs ANY gated op. Because
the heavy lane (`smackerel_assert_host_resources` → `..._profile <env> heavy`)
and the light lane (`..._profile test light`) share this one helper, the crash
blocks **both**. This is a wsl-macos-compatibility violation.

## Fix

Make the path selection **OS-aware**: take the host-native branch only on Linux
(where the host IS where containers run, so host `/proc/meminfo` + repo `statfs`
are the correct "can I bring the stack up" signal). On macOS (or Linux without
host Go) route through the **existing** dockerized runner
(`run_go_tooling .../preflight.sh`). On macOS+Docker-Desktop the stack runs in
the Docker VM; the runner sets **no** `--memory` cgroup limit, so its
`/proc/meminfo` reports the **Docker VM's** memory — the semantically correct
signal, since every container shares that VM's RAM, not the macOS host's free
RAM. The repo bind-mount at `/workspace` makes `statfs` follow to the host repo
fs.

```bash
if [[ "$(uname -s)" == "Linux" ]] && command -v go >/dev/null 2>&1; then
  ( cd "$SCRIPT_DIR" && go run ./cmd/preflight ... )    # Linux: host-native
else
  require_docker
  run_go_tooling /workspace/scripts/runtime/preflight.sh "$target_env" "$profile"  # macOS: Docker-VM memory
fi
```

`internal/preflight` is **unchanged** — deliberately NOT taught to read macOS
sysctl/`vm_stat`, because that would gate on the wrong number (the macOS host's
free RAM, not the Docker VM where the containers actually run). Both args stay
required (NO-DEFAULTS); `cmd/preflight` / `preflight.sh` still reject a missing
`--profile`.

Also cosmetic: `tests/integration/test_runtime_health_light.sh` gained a
`# shellcheck disable=SC2329` on `cleanup()` (invoked indirectly via
`trap cleanup EXIT`), matching the sibling restore-test/bcdr convention.

## Reproduction

```bash
# RED (before fix — macOS host with host Go installed)
./smackerel.sh --env test test integration-light \
  --go-run TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel
# → ERROR: smackerel pre-flight: read host RAM: open /proc/meminfo: no such file or directory

# GREEN (after fix — dockerized runner reads the Docker VM /proc/meminfo)
./smackerel.sh --env test test integration-light \
  --go-run TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel
# → real pre-flight verdict (Docker-VM RAM vs LIGHT floor), no /proc/meminfo crash:
#   either the lane proceeds (VM >= floor) or it REFUSES CLEANLY with a real
#   current-vs-required number.
```

## Proven Fixed (real re-run evidence, macOS host, 2026-06-30)

`./smackerel.sh --env test test integration-light --go-run TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel`
now reaches a REAL pre-flight verdict via the dockerized runner instead of
crashing on `/proc/meminfo`:

```
config-validate: .../config/generated/test.env.tmp.11090 OK
Smackerel pre-flight resource check: OK
  RAM  available: 8024 MB (required >= 2000 MB)
  Disk available: 3155880 MB / 3081.9 GB (required >= 8 GB)
```

The pre-flight read the Docker VM's memory (8024 MB available ≥ the 2000 MB
LIGHT floor) and returned OK — a real number, NOT the previous
`read host RAM: open /proc/meminfo: no such file or directory` crash. The
specific defect (host-native go reading absent Linux `/proc/meminfo` on macOS)
is resolved. Heavy `test integration` shares the same helper, so it is repaired
by the same fix.

## Downstream finding (separate bug — BUG-099-002)

The SAME re-run, AFTER the pre-flight passed, hit a DISTINCT macOS-compat bug
(not the pre-flight): bare `timeout` / `timeout --kill-after` in the test-lane
orchestrators (`smackerel.sh` integration / integration-light / e2e) and the
runtime-health scripts has no `gtimeout` / watchdog fallback, so
`timeout: command not found` (exit 127) blocks the stores-up + durability run:

```
./smackerel.sh: line 1207: timeout: command not found
Running project-scoped integration-light stack teardown (exit cleanup, timeout 120s)...
./smackerel.sh: line 1186: timeout: command not found
ERROR: integration-light stack teardown failed during exit cleanup (exit 127).
```

This is a separate root cause, filed as
`BUG-099-002-macos-timeout-not-found`. Because of it the durability test
`TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel` did NOT execute
this session; `BUG-069-002` / `BUG-069-003` fixSequence order-2 stay **pending**
(no durability verdict was produced — nothing fabricated). `docker ps` confirmed
no containers were left running (the failed `timeout` lookup never started the
stack).

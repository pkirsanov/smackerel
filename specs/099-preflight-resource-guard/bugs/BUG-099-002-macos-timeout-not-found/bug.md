# BUG-099-002 — test-lane orchestrators use bare `timeout` (no gtimeout/watchdog fallback) → `timeout: command not found` on macOS

- **Parent spec:** 099-preflight-resource-guard
- **Release train:** mvp
- **Severity:** High (after BUG-099-001 fixed the pre-flight, this is the NEXT macOS blocker: it stops the stores-up + Go-test run for `test integration`, `test integration-light`, and `test e2e` on every macOS host that lacks a bare `timeout` on PATH)
- **Status:** in_progress (finding filed + captured; fix NOT yet applied — tracked follow-up; a portable resolver + call-site rewire is a cross-lane change beyond the BUG-099-001 task scope)
- **Discovered:** 2026-06-30 (bubbles.devops, during the BUG-099-001 integration-light re-run — surfaced AFTER the pre-flight passed)
- **Owner:** bubbles.devops
- **Relates to:** BUG-099-001 (the pre-flight fix that unblocked the lane far enough to reach this); BUG-069-002 / BUG-069-003 (their fixSequence order-2 durability run is blocked by this)

## Summary

With BUG-099-001 fixed, `./smackerel.sh --env test test integration-light --go-run ...`
passes the pre-flight (real verdict, `RAM available: 8024 MB (required >= 2000 MB)`)
and then immediately fails:

```
./smackerel.sh: line 1207: timeout: command not found
Running project-scoped integration-light stack teardown (exit cleanup, timeout 120s)...
./smackerel.sh: line 1186: timeout: command not found
ERROR: integration-light stack teardown failed during exit cleanup (exit 127).
=== integration-light exit=127 ===
```

`docker ps` confirmed no stack was started (the bare-`timeout` lookup fails
before the stores come up), so nothing leaked.

## Root Cause

The test-lane orchestrators and runtime-health scripts call the GNU `timeout`
binary (and its GNU-only `--kill-after` flag) directly. macOS ships no `timeout`
on PATH by default (GNU coreutils installs it only as `gtimeout`), and the
scripts have **no** `timeout` → `gtimeout` → watchdog fallback. This is the
wsl-macos-compatibility violation called out in
`.github/instructions/wsl-macos-compatibility.instructions.md`
("Replace raw `timeout` usage in scripts with portability wrappers"). There is
no portable timeout helper in `scripts/lib/runtime.sh` today.

Note: `smackerel_compose ... down --timeout 60` and `up --wait --wait-timeout`
are docker-compose's OWN flags (not the `timeout` binary) and are unaffected.
ONLY the bare `timeout` / `timeout --kill-after` binary call sites are broken.

## Affected call sites (bare `timeout` binary; macOS-broken)

| File | Line(s) | Lane |
|------|---------|------|
| `smackerel.sh` | 1063, 1084 | `test integration` (heavy) teardown + health-gate |
| `smackerel.sh` | 1186, 1207 | `test integration-light` teardown + health-gate |
| `smackerel.sh` | 1536, 1615, 1636, 1687, 1696 | `test e2e` teardown / shell-test / up |
| `tests/integration/test_runtime_health.sh` | (timeout teardown calls) | heavy health gate |
| `tests/integration/test_runtime_health_light.sh` | ~32, ~39 | light health gate |

## Recommended Fix (follow-up, in-class with wsl-macos-compatibility)

Add ONE portable resolver to `scripts/lib/runtime.sh` (sourced by both
`smackerel.sh` and the health scripts) following the canonical pattern in
`.github/instructions/wsl-macos-compatibility.instructions.md`:

```bash
# resolve timeout -> gtimeout -> watchdog fallback; preserve exit-124 semantics
smackerel_run_with_timeout() {
  local seconds="$1"; shift
  if command -v timeout  >/dev/null 2>&1; then timeout  "$seconds" "$@"; return $?; fi
  if command -v gtimeout >/dev/null 2>&1; then gtimeout "$seconds" "$@"; return $?; fi
  "$@" & local cmd_pid=$!
  ( sleep "$seconds"; kill -TERM "$cmd_pid" 2>/dev/null ) & local watch_pid=$!
  local rc=0; wait "$cmd_pid" 2>/dev/null || rc=$?
  kill -TERM "$watch_pid" 2>/dev/null || true; wait "$watch_pid" 2>/dev/null || true
  [[ "$rc" -eq 143 ]] && rc=124; return "$rc"
}
```

Then replace every bare `timeout [--kill-after=Xs] N CMD...` call site above with
`smackerel_run_with_timeout N CMD...` (the `--kill-after` grace is preserved by
`gtimeout`; the watchdog fallback covers hosts with neither binary). On this
host `gtimeout` IS present, so the resolver works immediately. Validate with
`shellcheck -x` + `bash -n`, then re-run `test integration-light` to land the
BUG-069-002/003 order-2 durability evidence, and re-run `test integration` /
`test e2e` to prove the heavy + e2e lanes complete on macOS.

## Reproduction

```bash
# macOS host without a bare `timeout` on PATH (gtimeout present or not):
./smackerel.sh --env test test integration-light \
  --go-run TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel
# → pre-flight OK, then: line 1207: timeout: command not found ; exit 127
```

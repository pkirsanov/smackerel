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

## Corrective note — sibling static-contract reconciliation (2026-06-30)

**Why this note exists (anti-fabrication / append-only):** this bug was flipped
to `done` on 2026-06-30, but its OS-aware refactor of the pre-flight helper left
a **sibling** static-contract test red. A later cross-repo evo-x2 readiness
validation surfaced it as findings **F-CODE-01** (MAJOR — `./smackerel.sh test
unit` RED) and **F-CODE-02** (reconcile this cert). Rather than rewrite history,
this dated note records the reconciliation honestly.

**What drifted.** The fix moved the Go-evaluator invocations
(`go run ./cmd/preflight` on Linux; `run_go_tooling …/preflight.sh` on
macOS/Docker) out of `smackerel_assert_host_resources()` and into the new
`smackerel_assert_host_resources_profile()`; the old name became a thin
back-compat wrapper (`… _profile "$1" heavy`). But the drift-detector contract
test `internal/preflight/wiring_contract_test.go` still pinned its extractor to
the OLD decl `smackerel_assert_host_resources() {`, so it inspected the wrapper —
which no longer carries the evaluator — and failed:

- `--- FAIL: TestGuardWiring_LiveFile` ("helper … does not invoke the Go
  evaluator …")
- `--- FAIL: TestGuardWiring_AdversarialMissingBuildGuard` (the evaluator-check
  error fired before the build-path assertion could be reached)
- `FAIL github.com/smackerel/smackerel/internal/preflight`

**What was reconciled (no weakening).** The runtime guard was verified intact
FIRST — the evaluator invocations still live in
`smackerel_assert_host_resources_profile` (`smackerel.sh` L483 Linux / L486
macOS), the wrapper still delegates to it (L497), and every heavy-op case block
(`build` L733, `up`, `integration`, `e2e`, `e2e-ui`, `stress`, `pre-flight`)
still calls the wrapper. Only the STATIC contract was realigned to the
refactored structure: it now inspects `_profile` for the evaluator AND asserts
the back-compat wrapper delegates to `_profile`. The two adversarial siblings
still REJECT — `TestGuardWiring_AdversarialHelperNotRunningEvaluator` (neuter the
evaluator ⇒ reject, citing `cmd/preflight`) and
`TestGuardWiring_AdversarialMissingBuildGuard` (strip the build-block guard ⇒
reject, naming `"build"`) — so the contract is not tautological and would still
fail if a future edit genuinely unwired the evaluator.

**Evidence (this session, macOS host, dockerized Go runner):**

```
# RED (before realignment)
./smackerel.sh test unit --go --go-run 'TestGuardWiring'
  --- FAIL: TestGuardWiring_LiveFile (0.00s)
  --- FAIL: TestGuardWiring_AdversarialMissingBuildGuard (0.00s)
  FAIL github.com/smackerel/smackerel/internal/preflight

# GREEN (after realignment)
./smackerel.sh test unit --go --go-run 'TestGuardWiring' --verbose
  --- PASS: TestGuardWiring_LiveFile (0.01s)
  --- PASS: TestGuardWiring_AdversarialMissingBuildGuard (0.01s)
  --- PASS: TestGuardWiring_AdversarialHelperNotRunningEvaluator (0.00s)
  ok  github.com/smackerel/smackerel/internal/preflight  0.022s
```

**Scope.** Test-contract realignment only — `internal/preflight/*.go` production
code and `smackerel.sh` are unchanged; NO-DEFAULTS / fail-loud SST untouched.
`BUG-099-001` remains legitimately `done`; this note closes the sibling-test
drift it left behind.

**artifact-lint transparency (F-CODE-02 completeness, no cherry-pick).** The
required `bash .github/bubbles/scripts/artifact-lint.sh <this-packet>` was run
this session and reports the packet holds only `bug.md` + `state.json` — the
shape it was created in on 2026-06-30 — so the feature-style 6-artifact check
(`spec.md`/`design.md`/`scopes.md`/`report.md`/`uservalidation.md` + the
test/validate/audit phase records) is NOT satisfied and the lint exits non-zero.
This is a pre-existing structural property of the packet, NOT introduced by this
note: an untouched sibling done bug packet carrying the full 6-artifact set
(e.g. `specs/002-phase1-foundation/bugs/BUG-002-002-postgres-startup-health-gate`)
passes the identical lint with 0 failures, confirming the generic feature
artifact-lint — not a bug-shape validator — is what flags the 2-file packet.
The bug's REAL root-cause, fix, and in-session repro/verify evidence live in
`state.json` (`rootCause` / `fix` / `provenBy`) and in this `bug.md`; the
regression protection (the adversarial wiring contract
`TestGuardWiring_Adversarial*`) exists and passes. Decision per the remediation
brief: the corrective note is the accepted closure for F-CODE-02 (cert
reconciliation) — no retroactive 6-artifact back-fill is fabricated here, since
honest same-session `report.md` Before/After captures cannot be produced weeks
after the original fix. Full bug-template compliance for `BUG-099-001`, if later
desired, is a separate bug-scoped task (`bubbles.bug` / `bubbles.devops`) and
does not gate this reconciliation.

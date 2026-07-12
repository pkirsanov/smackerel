# Spec 099 — Pre-Flight Resource Guard

**Status:** in_progress
**Workflow mode:** full-delivery · **Status ceiling:** done
**Release train:** mvp · **Flags introduced:** none
**Relates to:** [042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md) (the existing host-port preflight precedent this mirrors), [045-deploy-resource-filesystem-hardening](../045-deploy-resource-filesystem-hardening/spec.md) (deploy-side resource envelopes — distinct concern)

## Problem

The Smackerel dev/build/test loop runs on a shared, memory-constrained host.
Heavy `./smackerel.sh` operations — `build`, `up`, and the live-stack test
categories `integration` / `e2e` / `e2e-ui` / `stress` (which spin up Docker
plus, on some envelopes, Ollama) — intermittently die mid-run from insufficient
free RAM (the kernel OOM-kills the compile/container, exit 137) or from disk
pressure (large Ollama models + Docker image layers fill the filesystem). These
deaths happen *minutes* into a long run, after the operator has already paid the
wall-clock cost, and they surface as confusing exit-137 / "no space left on
device" failures rather than an actionable up-front message.

`smackerel.sh` already has a **host-port** pre-flight
(`smackerel_assert_host_ports_free`, an embedded host-`python3` heredoc) that
fails fast with an actionable "stop the reported listener" message *before* the
disposable test stack starts. There is **no resource pre-flight** — nothing
checks that the host actually has enough free RAM and disk to survive the heavy
operation it is about to start.

This feature adds the missing resource dimension: a fast, host-native check that
verifies available host RAM (`MemAvailable` from `/proc/meminfo`) and available
disk (on the repo filesystem) meet SST-configured minimums, and **fails fast
with an actionable remediation message** before a doomed heavy run instead of
letting the OOM killer end it 20 minutes in.

## Goal

Add a resource pre-flight guard that, before each heavy `./smackerel.sh`
operation, verifies host RAM + disk against SST-configured minimums and aborts
early with a current-vs-required report and concrete remediation when the host
is below threshold — with an explicit, loud-warning override for operators who
knowingly want to proceed.

## Requirements

- **FR-099-01** — A resource evaluator reads host available RAM
  (`MemAvailable` from `/proc/meminfo`, in MB) and host available disk (on the
  repo filesystem, in MB) and compares both against SST-configured minimums. It
  exits `0` when both meet the minimums and `1` when either is below.
- **FR-099-02** — The minimums come from `config/smackerel.yaml` under a new
  `runtime.preflight` block (`min_available_ram_mb`, `min_available_disk_gb`),
  flow through the generated env files as `PREFLIGHT_MIN_AVAILABLE_RAM_MB` and
  `PREFLIGHT_MIN_AVAILABLE_DISK_GB`, and are consumed by the evaluator. A
  missing or empty or non-positive value **fails loud, naming the offending
  key** — there is NO hidden default and NO `getEnv(key, fallback)` form
  (Gate G028 / NO-DEFAULTS / fail-loud SST).
- **FR-099-03** — A standalone subcommand `./smackerel.sh pre-flight` runs the
  check directly (exit `0` = ok, `1` = below threshold) and prints
  current-vs-required for both RAM and disk. It is read-only and mutates no
  runtime state.
- **FR-099-04** — The guard is invoked automatically at the start of the heavy
  operations: `build`, `up`, and `test` for the heavy categories
  `integration` / `e2e` / `e2e-ui` / `stress`. Light / read-only operations
  (`status`, `logs`, `config generate`, `check`, `down`, `clean`, and
  `test unit`) are NOT gated.
- **FR-099-05** — An explicit override env var `SMACKEREL_PREFLIGHT_OVERRIDE`
  (any truthy value) bypasses the gate. The bypass is NEVER silent: it emits a
  loud `WARNING` that the resource check was overridden and a heavy run may
  still be OOM-killed, then proceeds (exit `0`).
- **FR-099-06** — The below-threshold failure message is actionable: it reports
  current free RAM/disk vs the required minimums and suggests concrete
  remediation (stop idle Docker stacks, stop Ollama, `./smackerel.sh clean
  smart`, or the override). The message MUST NOT echo any secret value.
- **FR-099-07** — The evaluation logic is implemented in Go (`cmd/preflight` +
  `internal/preflight`) and is unit-testable with adversarial coverage. A
  contract test (mirroring `internal/deploy/compose_contract_test.go`) parses
  `smackerel.sh` + the config pipeline and asserts the guard is actually wired
  into the heavy-op paths and reads the SST keys, with an adversarial sub-test
  that FAILS if the wiring regresses.

## Behavior (Gherkin)

```gherkin
Scenario: Standalone pre-flight passes when the host has headroom
  Given config/smackerel.yaml sets runtime.preflight.min_available_ram_mb and
        min_available_disk_gb to concrete positive minimums
  And the host has more than those minimums free
  When the operator runs `./smackerel.sh pre-flight`
  Then the command prints current-vs-required for RAM and disk
  And it exits 0

Scenario: Standalone pre-flight fails fast when the host is below threshold
  Given the host has less free RAM (or disk) than the SST minimum
  When the operator runs `./smackerel.sh pre-flight`
  Then the command prints current free RAM/disk vs the required minimum
  And it prints concrete remediation (stop idle Docker stacks, stop Ollama,
      `./smackerel.sh clean smart`, or the override env var)
  And the message contains no secret value
  And it exits 1

Scenario: A missing threshold key fails loud (NO-DEFAULTS)
  Given the generated env file is missing PREFLIGHT_MIN_AVAILABLE_RAM_MB
  When the resource evaluator runs
  Then it aborts naming the missing key
  And it never substitutes a silent default

Scenario: The override env var bypasses the gate with a loud warning
  Given the host is below the SST minimum
  And SMACKEREL_PREFLIGHT_OVERRIDE=1 is set
  When the guard runs ahead of a heavy operation
  Then it prints a loud WARNING that the resource check was overridden
  And it proceeds (exit 0) instead of blocking

Scenario: Heavy operations invoke the guard; light operations do not
  Given the smackerel.sh command surface
  When `build`, `up`, or `test integration|e2e|e2e-ui|stress` starts
  Then the resource guard runs before the heavy work begins
  And `status`, `logs`, `config generate`, `check`, `down`, `clean`, and
      `test unit` do not invoke the guard
```

## Product Principle Alignment

This is **operability / reliability infrastructure** for the developer-facing
CLI. It does not implement or extend any of the ten user-facing product
principles in [`docs/Product-Principles.md`](../../docs/Product-Principles.md)
(capture, retrieval, knowledge lifecycle, notifications, the QF companion
boundary, etc.) — it never touches captured artifacts, retrieval, digests, or
any product surface a Smackerel end-user sees. It is therefore declared
**N/A to product principles 1–10, with rationale**: a resource pre-flight that
fails a doomed `build`/`up`/`test` early changes only the *engineer's* loop, not
product behavior. No principle grep-check in
[`.github/instructions/product-principles.instructions.md`](../../.github/instructions/product-principles.instructions.md)
applies (it adds no capture-time tagging, no retrieval path, no notification, no
synthesis, no QF financial action).

The binding contracts it DOES honor are **engineering**, from the constitution
([`.specify/memory/constitution.md`](../../.specify/memory/constitution.md)):

- **C7 Single CLI Operations** — *"The CLI must own environment selection and
  safety checks."* This guard IS a CLI-owned safety check, surfaced through the
  one repo CLI (`./smackerel.sh`), not an ad-hoc command.
- **C8 Single Source Of Truth Configuration** — *"Missing required config must
  fail loudly; hidden defaults are forbidden."* The thresholds originate from
  the one committed source (`config/smackerel.yaml`) and fail loud when absent.
- **C2 Go-First Runtime** — the evaluation logic lives in Go, the primary
  runtime language, not in the Python ML sidecar.

### Single-Capability Justification

This spec introduces **no** reusable runtime capability and **no** second
provider / adapter / strategy / variant. It adds **one** local resource guard:
a single Go evaluator (`internal/preflight`) read by **one** CLI helper
(`smackerel_assert_host_resources`) at a fixed set of already-existing heavy-op
call sites. RAM and disk are **two dimensions of the same single check**, not
two implementations of a resource-guard capability — one `Evaluate` function
compares both observed values against the two SST minimums in a single pass,
and one `FormatReport` renders one report. There is no foundation/overlay
split, no pluggable backend, and no second variant: exactly one evaluator, one
threshold source (`config/smackerel.yaml` `runtime.preflight.*`), and one
decision path (`Run`). The G094 proportionality triggers fire on incidental
vocabulary (the prose that contrasts this guard against a hypothetical second
provider), not on a real capability fork. A Capability Foundation with concrete
implementations and variation axes would be over-engineering for a single
additive guard. The deploy-adapter's own host pre-flight (knb, operator-private,
out of this repo) is a **distinct owner** on a different host — it is NOT a
second implementation of this guard and is explicitly not introduced here.

## Scope Boundary (LOCAL dev product-CLI only)

This guard protects **local / developer `./smackerel.sh` operations only**
(`build`, `up`, `test …` on the dev or disposable test stack). It is **NOT** a
self-hosted / production apply gate. The self-hosted live apply is owned by the
operator-private **knb deploy-adapter overlay** (out of this repo). That adapter
MAY add its own host pre-flight on the real deploy host, but that is **explicitly
out of scope here** and is not implemented, referenced by real values, or
coupled to this guard. This repo stays generic and target-agnostic
(per [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md)
"Deployment Ownership Boundary").

## Out of Scope

- Any self-hosted / production / deploy-adapter host pre-flight (knb-owned,
  operator-private, out of repo).
- Gating light / read-only CLI operations (`status`, `logs`, `config`, `check`,
  `down`, `clean`, `test unit`).
- CPU, GPU/VRAM, file-descriptor, or inode checks — this v1 covers RAM + disk,
  the two failure modes actually observed (OOM exit-137, disk-full).
- Per-operation distinct thresholds — v1 uses one RAM minimum + one disk
  minimum for all heavy ops. (A future spec may differentiate.)
- Auto-remediation (auto-stopping stacks / Ollama). The guard reports and
  blocks; the operator decides.

## Release Train

Targets the **`mvp`** train (the active default train; this aids the current
self-hosted readiness loop by making heavy local runs fail fast instead of
OOM-dying). This spec introduces **no feature flag** (`flagsIntroduced: []`) —
it is an always-on CLI safety check, not a runtime-flagged product feature.
Behavior is identical on every train (the guard is train-agnostic).

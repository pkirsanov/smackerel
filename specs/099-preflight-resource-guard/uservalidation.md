# User Validation — Spec 099 Pre-Flight Resource Guard

**Status:** done · **Workflow mode:** full-delivery

## Context

This validates that `./smackerel.sh` runs a resource pre-flight before heavy
operations — checking host RAM + disk against SST-configured minimums — and
fails fast with an actionable message instead of letting a doomed `build` / `up`
/ `test` run be OOM-killed (exit 137) or run the disk out minutes in.

## Checklist

- [x] **Problem is real** — heavy ops on a shared, memory-constrained host
  intermittently die mid-run from OOM (exit 137) or disk pressure; there was a
  host-port preflight but no resource preflight. (baseline acceptance)
- [x] **Standalone check works** — `./smackerel.sh pre-flight` prints
  current-vs-required for RAM (`MemAvailable`) and disk (repo fs) and exits 0
  when the host has headroom (verified live: 24648 MB free vs 6000 MB required →
  exit 0).
- [x] **Fail-fast below threshold** — the real CLI returns exit 1 with a
  current-vs-required report (`<-- SHORT` markers) and concrete remediation (stop
  idle Docker stacks, stop Ollama, `./smackerel.sh clean smart`, override) when
  below the SST minimum (verified live against a throwaway high threshold).
- [x] **NO-DEFAULTS fail-loud** — a missing/empty/non-numeric/non-positive
  threshold key aborts naming the key; no hidden default (unit-proven, 5 tests).
- [x] **Override is loud, never silent** — `SMACKEREL_PREFLIGHT_OVERRIDE=1`
  proceeds with a `WARNING` and exit 0 even below threshold (verified live + unit).
- [x] **No secret echoed** — the report only ever prints the four numeric values;
  a planted `SMACKEREL_AUTH_TOKEN`/`LLM_API_KEY` is not echoed (adversarial unit).
- [x] **SST single source of truth** — thresholds live in `config/smackerel.yaml`
  `runtime.preflight.*` and flow to `config/generated/{dev,test}.env` as
  `PREFLIGHT_MIN_AVAILABLE_RAM_MB`/`_DISK_GB` (6000/15) via fail-loud `required_value`.
- [x] **Heavy ops gated; light ops not** — the drift-detector contract test proves
  `build`, `up`, `test integration|e2e|e2e-ui|stress` invoke the guard and the
  helper runs the Go evaluator; 2 adversarial sub-tests reject a removed guard.
- [x] **Scope boundary documented** — guards LOCAL dev CLI ops only; the self-hosted
  apply pre-flight is knb-deploy-adapter-owned and out of this repo's scope.
- [x] **Gates green** — 22/22 Go unit+contract PASS, full `go test ./...` OK,
  `check` exit 0, `lint` exit 0, `artifact-lint` clean, `traceability-guard` clean,
  `release-train-guard` clean. (see report.md + state.json)

## Sign-off

Engineering complete and certified `done` for the full-delivery mode on real
in-repo executed evidence (config generation, live `./smackerel.sh pre-flight`
OK/below/override runs, 22-test Go unit + drift-detector contract suite, check,
lint, and the Bubbles artifact/traceability/release-train gates). Not committed or
pushed — the repo autoCommit policy is OFF, so the operator owns the push
(established VALIDATED-IN-REPO pattern). No residual blocking items.

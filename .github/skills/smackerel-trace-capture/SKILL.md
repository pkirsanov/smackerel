---
name: smackerel-trace-capture
description: Wire Smackerel's concrete validate-plane stack into the framework trace-driven defect-discovery method. Use when running live-category tests (integration/e2e/e2e-ui/stress) against the ephemeral test stack; capturing SLO or trace evidence; hunting latency/error/perf bugs from spans; querying the test-stack Jaeger or Prometheus; or touching `.specify/runtime/observability`. Concrete smackerel endpoints + the exact `capture-slo.sh` command; do NOT duplicate `bubbles-observability-adapter` — this only adds the wiring.
---

# smackerel-trace-capture

Smackerel declares `posture: wired` with workflow `core.health` in
[`.github/bubbles-project.yaml`](../../bubbles-project.yaml). The generic
"capture SLO numbers AND mine traces for hidden defects" method lives in the
framework skill [`bubbles-observability-adapter`](../bubbles-observability-adapter/SKILL.md)
§ *Trace-driven defect discovery (during live-category tests)*. **This skill does
not restate that method — it pins it to smackerel's real stack: the exact
capture command, the real test-stack endpoints/ports, and the workflow's
expected spans.**

## When to use

- A live-category run against the ephemeral test stack (validate plane,
  `profile: test`, `env=test*`): `./smackerel.sh test integration`,
  `./smackerel.sh test e2e`, `./smackerel.sh test e2e-ui`, or
  `./smackerel.sh test stress`.
- Capturing G100 SLO evidence for `core.health`, or mining the exercised
  workflow's spans for latency/error/perf bugs.
- Querying the test-stack Jaeger or Prometheus to inspect a trace.

## When NOT to use

- `unit` / `ui-unit` — no real stack to trace. Use the normal unit path.
- The **operate / prod** plane — read-only and off-limits to feature tests
  (G115; see [`bubbles-env-pollution-isolation`](../bubbles-env-pollution-isolation/SKILL.md)).
  Never point a capture at a prod URL or `env=prod|home-lab` telemetry.
- Authoring a *new* telemetry adapter, or the generic discovery method itself →
  [`bubbles-observability-adapter`](../bubbles-observability-adapter/SKILL.md).
- NO-DEFAULTS / config-fallback review → [`smackerel-no-defaults`](../smackerel-no-defaults/SKILL.md).

## The test stack (validate plane, `profile: test`)

`./smackerel.sh test integration|e2e|stress` brings up the disposable
`smackerel-test` project. Real endpoints (from `config/smackerel.yaml` `test:`
block + `docker-compose.yml`), loopback-only:

| Signal | Endpoint (test stack) | Notes |
|--------|-----------------------|-------|
| Core `/api/health` | `http://127.0.0.1:45001/api/health` | `CORE_HOST_PORT=45001` → container `8080` |
| Jaeger UI / query API | `http://127.0.0.1:16686` | `smackerel-test-jaeger`, auto-up under `profile: test`; OTLP/gRPC `127.0.0.1:4317`; OTel `service.name=smackerel-core`. Spans appear only when the core's exporter is on (`ASSISTANT_OBSERVABILITY_OTEL_ENABLED=true` → `smackerel-test-jaeger:4317`; compose default is `otel_enabled: false`) |
| Prometheus | `http://127.0.0.1:47005` | container `9090`; scrape job `smackerel-core`; up only with `--profile monitoring` (opt-in via the integration fixture) |

> Dev-stack equivalents (`./smackerel.sh up`, `env=dev`) differ: core
> `http://127.0.0.1:40001/api/health`, Prometheus `42005`, Jaeger only with
> `--profile dev-otel`. Spec 090's original dogfood ran against the dev port
> `40001`; for live-category test runs use the `test`-stack ports above.

## Capture SLO evidence

The repo's observability surface is the standalone evidence script
[`scripts/observability/capture-slo.sh`](../../../scripts/observability/capture-slo.sh)
(it is NOT a `./smackerel.sh` subcommand — invoke it directly, exactly as spec
090 does). With the test stack up, run the real `core.health` capture:

```bash
bash scripts/observability/capture-slo.sh run \
  --workflow core.health \
  --url http://127.0.0.1:45001/api/health \
  --requests 600 --concurrency 20 --source stress
```

- Targets are read from `.github/bubbles-project.yaml`
  (`traceContracts.observability.slos.core.health`): **p99 ≤ 200 ms, error ≤
  0.1 %, availability ≥ 99.9 %**. The script measures latency + HTTP status
  client-side (curl) and emits the G100 evidence JSON.
- Default `--out`:
  `.specify/runtime/observability/core.health.slo.json`. Override with `--out`
  for a stress probe (e.g. `--requests 1500 --concurrency 50 --out
  /tmp/core-stress.slo.json`).
- It is **fail-loud**: a down endpoint (curl `000`) exits non-zero and writes no
  file — there is no `--skip/--force/--fake`. Never hand-edit the evidence JSON.
- Offline unit path (no stack): `bash scripts/observability/capture-slo.sh
  compute --workflow core.health --samples <file> --out /tmp/o.json`.

Assert the captured evidence against the contract:

```bash
bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .
```

Relationship to the suites: run the capture *during* (or right after) a
`./smackerel.sh test integration|e2e|stress` run while the same test stack is
live — the load probe and the suite exercise the same `profile: test` stack.

## Trace-driven defect discovery

Capturing SLO numbers proves a target was met; it does **not** surface defects
hiding inside a passing run. Follow the four defect classes in
[`bubbles-observability-adapter`](../bubbles-observability-adapter/SKILL.md)
§ *Trace-driven defect discovery* — (1) error/exception spans, (2) latency
outliers, (3) fan-out / N+1, (4) retries / timeouts / missing spans — then mine
smackerel's validate-plane stack like this.

**Jaeger (test stack, `service=smackerel-core`).** Error spans for the
exercised assistant workflow (even when the turn returned success):

```bash
# (1) error/exception spans — tags is URL-encoded {"error":"true"}
curl --max-time 5 "http://127.0.0.1:16686/api/traces?service=smackerel-core&operation=assistant.adapter.translate&tags=%7B%22error%22%3A%22true%22%7D&limit=20"

# (2) latency outliers — traces with any span ≥ the 200ms core.health target
curl --max-time 5 "http://127.0.0.1:16686/api/traces?service=smackerel-core&operation=assistant.adapter.translate&minDuration=200ms&limit=20"
```

**Prometheus (test stack, only with `--profile monitoring`).** Target liveness
and series discovery (avoid guessing series names — list them):

```bash
curl --max-time 5 "http://127.0.0.1:47005/api/v1/query?query=up%7Bjob%3D%22smackerel-core%22%7D"
curl --max-time 5 "http://127.0.0.1:47005/api/v1/label/__name__/values"
```

### Expected `core.health` spans

`core.health` is the `/api/health` liveness probe
(`internal/api/router.go` → `r.Get("/health", deps.HealthHandler)` under the
`/api` prefix). `HealthHandler` creates **no custom child spans**, so the SLO is
measured client-side and the probe's own trace is intentionally **thin** — zero
DB/NATS/LLM children is the *expected* shape. An unexpected child span, or an
`error=true` span on a health probe, is itself a finding (class 1/4).

The **rich** OTel span tree to mine lives on the **assistant turn workflow**
exercised by the integration/e2e suites (per-turn span-tree, spec 061 SCOPE-09b;
service `smackerel-core`):

```
assistant.adapter.translate                 (root)
  ├── assistant.facade.handle
  │     ├── assistant.context.load
  │     ├── assistant.router.classify
  │     ├── assistant.router.band
  │     ├── assistant.provenance.check       (high-band only)
  │     ├── assistant.confirm.persist        (CONDITIONAL — confirm_required path)
  │     ├── assistant.context.persist
  │     └── assistant.audit.write
  └── assistant.adapter.render
```

Every span carries mandatory attrs `transport`, `user_id_hashed`,
`assistant_turn_id`, `scenario_id`, `correlation_id`, plus end attrs `status`
and `error_cause` — a missing attr or absent step span fails the framework's
"3 AM reconstructibility" bar and is a finding.

### File findings — don't defer

Any error span, SLO-breaching latency, N+1, or missing-span you find is a
**defect, not noise**. File it via `bubbles.bug` (see
[`bubbles-bug-template`](../bubbles-bug-template/SKILL.md)) with the captured
trace as before-fix evidence. Under
[`bubbles-dod-validation`](../bubbles-dod-validation/SKILL.md) +
[`bubbles-anti-fabrication`](../bubbles-anti-fabrication/SKILL.md) a scope
cannot be marked done while a discovered finding remains unfixed. Do not defer
to "follow-up".

## Evidence location

Captured artifacts land under
`.specify/runtime/observability/<workflow>.<signal>.{txt,json}` (e.g.
`core.health.slo.json`). `.specify/runtime/` is gitignored — it is captured
runtime output, never a committed source of truth. Record the capture command's
provenance per [`bubbles-evidence-capture`](../bubbles-evidence-capture/SKILL.md).

## Works well with

- [`bubbles-observability-adapter`](../bubbles-observability-adapter/SKILL.md) — the generic method this skill wires (capture + the four defect classes). Read it first.
- [`bubbles-test-integrity`](../bubbles-test-integrity/SKILL.md) — real, spec-driven test behavior the live categories must prove.
- [`bubbles-test-environment-isolation`](../bubbles-test-environment-isolation/SKILL.md) — ephemeral-only test stores; keeps telemetry `env=test*`.
- [`bubbles-evidence-capture`](../bubbles-evidence-capture/SKILL.md) — how to record the capture as valid evidence.
- [`bubbles-bug-template`](../bubbles-bug-template/SKILL.md) — file discovered findings as bugs (don't defer).
- [`smackerel-no-defaults`](../smackerel-no-defaults/SKILL.md) — fail-loud config posture the capture tool and endpoints follow.

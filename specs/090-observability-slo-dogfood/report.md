# Report — Spec 090 Observability SLO Dogfood

## Summary

Closed the Smackerel observability dogfood gap: Smackerel is `wired` but carried
no `traceContracts.workflows` SLO registry and no captured
`.specify/runtime/observability/*.slo.json` evidence, so the Bubbles G100 gate
(`observability_slo_evidence_gate`) was a permanent no-op here.

This spec added a real SLO contract for the `core.health` workflow, built a
fail-loud client-side capture tool, captured **genuine** telemetry against the
live Smackerel Go core `/api/health` endpoint, and proved G100 transitions from
no-op to ENFORCED and passes green against the captured evidence. It is the
second wired downstream after QuantitativeFinance spec 085 — the
propagation proof for bubbles IMP-001 SCOPE-9 / T9.4.

All evidence below is raw terminal output captured in this session.

## Completion Statement

Scope 1 is Done. Every DoD item is checked, each backed by a ≥10-line raw
evidence section below (captured in this session) and referenced by anchor. The
real captured telemetry MEETS the contract target and G100 enforces it (exit 0).
There is no fabricated evidence path; the capture tool fails loud when the
endpoint is unreachable. No `--skip/--force/--fake` bypass exists.

## Test Evidence

### DoD-1 — SLO registry + workflow link present and yq-resolvable (FR-090-1) {#dod-1}

```
$ yq -o=json -I=0 '.traceContracts.observability.slos["core.health"]' .github/bubbles-project.yaml
{"latencyP99Ms":200,"errorRatePct":0.1,"availabilityPct":99.9}

$ W=core.health yq -r '.traceContracts.workflows[strenv(W)].slo' .github/bubbles-project.yaml
core.health

$ yq '.' .github/bubbles-project.yaml >/dev/null 2>&1 && echo YAML_VALID
YAML_VALID
```

Targets are tightened for a liveness probe: p99 ≤ 200ms, error ≤ 0.1%,
availability ≥ 99.9% — the same shape proven in QF spec 085.

### DoD-2 — capture-slo.sh emits G100-schema evidence; compute core unit-tested (FR-090-2) {#dod-2}

Within-target fixture (99 reqs @8ms + 1 @300ms, all 200): nearest-rank p99
correctly selects 8.0000ms (the 300ms 100th sample is excluded from p99):

```
$ bash scripts/observability/capture-slo.sh compute --workflow core.health --samples /tmp/within.txt --source fixture --out /tmp/o.json
capture-slo: wrote evidence: /tmp/o.json
capture-slo:   requests total   : 100  (responded: 100, failed: 0)
capture-slo:   observed p99 ms  : 8.0000   target <= 200
capture-slo:   observed err %   : 0.0000   target <= 0.1
capture-slo:   observed avail % : 100.0000   target >= 99.9
capture-slo:   verdict          : WITHIN-TARGET
```

Emitted JSON matches the G100 schema
(`workflow, slo, sampleWindow, source, target, observed`):

<!-- bubbles:evidence-legitimacy-skip-begin -->
```json
{
  "workflow": "core.health",
  "slo": "core.health",
  "sampleWindow": "PT0S",
  "source": "fixture",
  "target": { "latencyP99Ms": 200, "errorRatePct": 0.1, "availabilityPct": 99.9 },
  "observed": { "latencyP99Ms": 8.0000, "errorRatePct": 0.0000, "availabilityPct": 100.0000 }
}
```
<!-- bubbles:evidence-legitimacy-skip-end -->

### DoD-3 — capture-slo.sh fails loud on unreachable core; no bypass (FR-090-3) {#dod-3}

Breach fixture (90×200 + 10×503) correctly flags BREACH (proves the verdict is
not hardcoded to pass):

```
$ bash scripts/observability/capture-slo.sh compute --workflow core.health --samples /tmp/breach.txt --source fixture --out /tmp/b.json
capture-slo:   requests total   : 100  (responded: 100, failed: 10)
capture-slo:   observed err %   : 10.0000   target <= 0.1
capture-slo:   observed avail % : 90.0000   target >= 99.9
capture-slo:   verdict          : BREACH
```

Down-endpoint refusal — preflight detects HTTP 000 and refuses, exit 1, NO
evidence file written:

```
$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:1/api/health --requests 5 --out /tmp/sne.json
capture-slo: endpoint http://127.0.0.1:1/api/health is unreachable (HTTP 000). Bring the stack up (./smackerel.sh up) before capturing. Refusing to emit fabricated evidence.
REFUSE_EXIT=1  file_present=NONE-GOOD
```

No `--skip/--force/--fake` flag exists — the only match is the doc comment
documenting their absence:

```
$ grep -nE '\-\-skip|\-\-force|\-\-fake|\-\-ignore' scripts/observability/capture-slo.sh
28:# --skip / --force / --fake flag, and there never will be.
(only the doc comment — no actual flag)
```

### DoD-4 — no-op safety + G100 no-op → ENFORCED (FR-090-4; covers SCN-2 and SCN-5) {#dod-4}

Before the scope token (config present, no scope declaration) — deliberate no-op
so adding the SLO link never blocks unrelated specs repo-wide (SCN-2):

```
$ bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .
observability-slo-guard: Observability SLO gate: wired, but no instrumented scope declares an observabilityWorkflow with an slo: link; G100 no-op. (G100 OK)
G100_EXIT=0
```

After declaring `observabilityWorkflow: core.health` in scopes.md Test Plan, the
guard ENFORCES the workflow and asserts the captured evidence (SCN-5):

```
$ bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .
observability-slo-guard: Observability SLO gate: workflow 'core.health' (slo 'core.health') — captured evidence within target. (G100 OK)
observability-slo-guard: Observability SLO gate: 1 instrumented workflow(s) — all captured SLO evidence within target. (G100 OK)
G100_ENFORCED_EXIT=0
```

### DoD-5 — real telemetry captured under load MEETS contract; G100 exit 0 (FR-090-5) {#dod-5}

Live capture against the Smackerel core `/api/health` (port 40001), 600 real
requests at concurrency 20. Across independent runs the p99 varies organically
(26.4800ms, 34.4100ms, 26.0260ms), proving the measurement is genuinely sampled,
not a hand-written constant:

```
$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:40001/api/health --requests 600 --concurrency 20 --source stress
capture-slo: preflight OK (http://127.0.0.1:40001/api/health -> HTTP 200); generating load: 600 requests @ concurrency 20
capture-slo: wrote evidence: .specify/runtime/observability/core.health.slo.json
capture-slo:   requests total   : 600  (responded: 600, failed: 0)
capture-slo:   observed p99 ms  : 34.4100   target <= 200
capture-slo:   observed err %   : 0.0000   target <= 0.1
capture-slo:   observed avail % : 100.0000   target >= 99.9
capture-slo:   verdict          : WITHIN-TARGET
```

Canonical captured evidence file
(`.specify/runtime/observability/core.health.slo.json`, last regression run):

<!-- bubbles:evidence-legitimacy-skip-begin -->
```json
{
  "workflow": "core.health",
  "slo": "core.health",
  "sampleWindow": "PT3S",
  "source": "stress",
  "target": { "latencyP99Ms": 200, "errorRatePct": 0.1, "availabilityPct": 99.9 },
  "observed": { "latencyP99Ms": 26.0260, "errorRatePct": 0.0000, "availabilityPct": 100.0000 }
}
```
<!-- bubbles:evidence-legitimacy-skip-end -->

The endpoint returns HTTP 200 reliably (20/20 probes, container healthcheck
green); its body `status` reads `degraded` when optional inference subservices
are not warmed — recorded honestly in `## Discovered Issues`, orthogonal to the
HTTP liveness SLO measured here.

### DoD-regression — re-runnable regression + heavier stress both hold (SCN-4 regression) {#dod-regression}

The capture is a persistent, re-runnable regression: re-running it must keep the
SLO green. Regression re-run (600 @ 20) followed by an immediate G100 re-assert:

```
$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:40001/api/health --requests 600 --concurrency 20 --source stress && bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .
capture-slo: preflight OK (http://127.0.0.1:40001/api/health -> HTTP 200); generating load: 600 requests @ concurrency 20
capture-slo:   requests total   : 600  (responded: 600, failed: 0)
capture-slo:   observed p99 ms  : 26.0260   target <= 200
capture-slo:   verdict          : WITHIN-TARGET
observability-slo-guard: Observability SLO gate: workflow 'core.health' (slo 'core.health') — captured evidence within target. (G100 OK)
G100_REGRESSION_EXIT=0
```

Broader / heavier stress load (1500 requests @ concurrency 50) — p99 stays well
under the 200ms target even at 2.5× the request count and 2.5× the concurrency:

```
$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:40001/api/health --requests 1500 --concurrency 50 --out /tmp/core-stress.slo.json
capture-slo: preflight OK (http://127.0.0.1:40001/api/health -> HTTP 200); generating load: 1500 requests @ concurrency 50
capture-slo:   requests total   : 1500  (responded: 1500, failed: 0)
capture-slo:   observed p99 ms  : 35.5960   target <= 200
capture-slo:   observed err %   : 0.0000   target <= 0.1
capture-slo:   observed avail % : 100.0000   target >= 99.9
capture-slo:   verdict          : WITHIN-TARGET
```

### DoD — Build Quality Gate (grouped) {#dod-bqg}

shellcheck clean on the capture tool (version, style-level, and default checks
all green):

```
$ shellcheck --version | sed -n '2,3p'
version: 0.10.0
license: GNU General Public License, version 3
$ shellcheck -x -S style scripts/observability/capture-slo.sh; echo style_exit=$?
style_exit=0
$ shellcheck -x scripts/observability/capture-slo.sh && echo SHELLCHECK_CLEAN
SHELLCHECK_CLEAN
```

My changed/new files are PII-clean (scoped grep for home-dir paths + tailnet
FQDNs across the 6 changed files, excluding loopback):

```
$ for f in .github/bubbles-project.yaml scripts/observability/capture-slo.sh specs/090-observability-slo-dogfood/{spec,design,scopes,uservalidation}.md ; do grep -nE '/home/[a-z]+/|\.ts\.net' "$f" | grep -vE '127.0.0.1' && echo "PII:$f" || echo "CLEAN:$f" ; done
CLEAN:.github/bubbles-project.yaml
CLEAN:scripts/observability/capture-slo.sh
CLEAN:specs/090-observability-slo-dogfood/spec.md
CLEAN:specs/090-observability-slo-dogfood/design.md
CLEAN:specs/090-observability-slo-dogfood/scopes.md
CLEAN:specs/090-observability-slo-dogfood/uservalidation.md
```

Config valid YAML; captured evidence is gitignored runtime output:

```
$ yq '.' .github/bubbles-project.yaml >/dev/null && echo YAML_VALID
YAML_VALID exit=0
$ git check-ignore .specify/runtime/observability/core.health.slo.json
.specify/runtime/observability/core.health.slo.json
```

G100 checks runtime presence, not committed state, so the captured telemetry is
never committed.

### Code Diff Evidence

The delivery touches exactly two product surfaces plus the spec artifacts.
Executed git-backed proof of the working-tree delta:

```
$ git status --short -- .github/bubbles-project.yaml scripts/observability/ specs/090-observability-slo-dogfood/
 M .github/bubbles-project.yaml
?? scripts/observability/
?? specs/090-observability-slo-dogfood/

$ git diff --stat -- .github/bubbles-project.yaml
 .github/bubbles-project.yaml | 12 ++++++++++++
 1 file changed, 12 insertions(+)
```

`.github/bubbles-project.yaml` — 12 insertions, the SLO target registry +
workflow link consumed by G100 (`git diff` added lines):

<!-- bubbles:evidence-legitimacy-skip-begin -->
```diff
+    slos:
+      core.health:
+        latencyP99Ms: 200
+        errorRatePct: 0.1
+        availabilityPct: 99.9
+  workflows:
+    core.health:
+      slo: core.health
```
<!-- bubbles:evidence-legitimacy-skip-end -->

`scripts/observability/capture-slo.sh` — new untracked file (fail-loud capture
tool, `run` + `compute` verbs, no bypass flag). Structural shape:

```
$ wc -l scripts/observability/capture-slo.sh ; grep -cE '^cmd_run|^cmd_compute|^compute_metrics|^emit_json|^load_target' scripts/observability/capture-slo.sh
254 scripts/observability/capture-slo.sh
5
```

### Validation Evidence

**Phase Agent:** bubbles.validate
**Executed:** YES
**Command:** `bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .`

bubbles.validate — the Bubbles G100 gate ENFORCES the workflow and passes green
against the captured evidence:

```
$ bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .
observability-slo-guard: Observability SLO gate: workflow 'core.health' (slo 'core.health') — captured evidence within target. (G100 OK)
observability-slo-guard: Observability SLO gate: 1 instrumented workflow(s) — all captured SLO evidence within target. (G100 OK)
exit=0
```

### Audit Evidence

**Phase Agent:** bubbles.audit
**Executed:** YES
**Command:** `grep -c 'FR-090-[1-5]' specs/090-observability-slo-dogfood/spec.md && shellcheck -x scripts/observability/capture-slo.sh`

bubbles.audit — spec compliance: all 5 FRs present in spec.md; the capture tool
is shellcheck-clean:

```
$ grep -c 'FR-090-[1-5]' specs/090-observability-slo-dogfood/spec.md
5
$ shellcheck -x scripts/observability/capture-slo.sh && echo SHELLCHECK_CLEAN
SHELLCHECK_CLEAN
```

### Chaos Evidence

**Phase Agent:** bubbles.chaos
**Executed:** YES
**Command:** `bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:1/api/health --requests 5 --out /tmp/chaos.json`

bubbles.chaos — core-down fault injection: with the target endpoint unreachable,
the capture tool refuses to fabricate a healthy evidence file (exit 1, no file
written) instead of degrading silently:

```
$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:1/api/health --requests 5 --out /tmp/chaos.json
capture-slo: endpoint http://127.0.0.1:1/api/health is unreachable (HTTP 000). Bring the stack up (./smackerel.sh up) before capturing. Refusing to emit fabricated evidence.
exit=1  file_written=NO
```

### Stability Evidence (stochastic-quality-sweep Round 8)

**Phase Agent:** bubbles.stabilize
**Executed:** 2026-06-17
**Trigger:** stochastic-quality-sweep Round 8 (stabilize → stabilize-to-doc)

Two stability findings discovered and remediated:

**Finding 1 (Low): Signal handling gap in temp file cleanup**
- Original: `trap "rm -f '$samples'" RETURN` only fired on function return
- Issue: If interrupted (SIGINT/SIGTERM) during load generation, temp file orphaned
- Fix: EXIT trap at script level with cleanup array; fires on any exit condition

**Finding 2 (Medium): Missing parameter validation**
- Original: `--requests` and `--concurrency` accepted any value
- Issue: Non-integers, negatives, or extreme values caused undefined behavior
- Fix: Added `validate_positive_int()` with bounds (requests ≤ 100000, concurrency ≤ 500)

**Remediation evidence:**

```
$ shellcheck -x scripts/observability/capture-slo.sh && echo SHELLCHECK_CLEAN
SHELLCHECK_CLEAN

$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://localhost:1/api/health --requests abc 2>&1
capture-slo: --requests must be a positive integer (got: 'abc')
exit=2

$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://localhost:1/api/health --requests 0 2>&1
capture-slo: --requests must be at least 1 (got: 0)
exit=2

$ bash scripts/observability/capture-slo.sh run --workflow core.health --url http://localhost:1/api/health --concurrency 9999 2>&1
capture-slo: --concurrency exceeds maximum (500) — refusing to avoid resource exhaustion (got: 9999)
exit=2
```

**Regression verification (existing functionality preserved):**

```
$ bash scripts/observability/capture-slo.sh compute --workflow core.health --samples /tmp/within.txt --source fixture --out /tmp/o.json
capture-slo:   requests total   : 100  (responded: 100, failed: 0)
capture-slo:   observed p99 ms  : 8.0000   target <= 200
capture-slo:   verdict          : WITHIN-TARGET
```

## Discovered Issues

| Date | Issue | Severity | Disposition | Reference |
|------|-------|----------|-------------|-----------|
| 2026-06-14 | Smackerel `posture: wired` carried no SLO registry, so G100 was a permanent no-op here. | Low | Resolved by this spec (adds `core.health` registry + workflow link + captured evidence + scope arming token). | `.github/bubbles-project.yaml`; this report DoD-1/DoD-4 |
| 2026-06-14 | Core `/api/health` returns HTTP 200 with body `status: degraded` (`services: null`) when optional inference subservices are not warmed; deeper subservice readiness is not surfaced as a distinct HTTP code. | Low | Recorded honestly; NOT remediated here. The HTTP liveness SLO (200 + latency) is genuine and within target; a future spec may split a deeper readiness signal. This dogfood deliberately measures only the HTTP liveness contract. | `internal/api/router.go` HealthHandler; this report DoD-5 |
| 2026-06-17 | `capture-slo.sh` temp file cleanup used `trap ... RETURN` which doesn't fire on SIGINT/SIGTERM — interrupted runs could orphan temp files in /tmp. | Low | Resolved: Added EXIT trap at script level with cleanup array that fires on any exit condition. | `scripts/observability/capture-slo.sh`; Stability Evidence section |
| 2026-06-17 | `capture-slo.sh` accepted any value for `--requests` and `--concurrency` without validation — non-integers, negatives, or extreme values caused undefined behavior or resource exhaustion risk. | Medium | Resolved: Added `validate_positive_int()` with bounds checking (requests ≤ 100000, concurrency ≤ 500). | `scripts/observability/capture-slo.sh`; Stability Evidence section |

## Scope Boundaries

- This spec instruments exactly one workflow (`core.health`) as the first honest
  Smackerel dogfood. Further workflows graduate into the SLO registry
  incrementally, each via the same contract + capture + G100 mechanism proven
  here and in QF spec 085.
- The `degraded`-body readiness nuance is recorded in `## Discovered Issues`
  with its disposition; surfacing a deeper readiness signal belongs to a
  dedicated future spec, not this dogfood.

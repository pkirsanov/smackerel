# BUG-064-003 — `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` fails: a fixed 30s deadline covers 79 sequential cold-start embed calls

**Spec:** `specs/064-open-ended-knowledge-agent` (SCOPE-12)
**Severity:** S2
**Status:** Fixed — the reported defect no longer reproduces; see [scopes.md](scopes.md) for per-item evidence.
Not `Verified`: certification is owned by `bubbles.validate`, and 5 of 15 DoD items remain unchecked with
stated reasons (T2's integration-tier branch, the pre-fix RED phase, the composite T1–T4 tick, the build
quality gate, and the open owner decision).
**Discovered:** 2026-08-11, incidentally, while working the unrelated in-flight bug
`BUG-061-011-eval-gate-runs-in-no-automated-lane`
**Discovered by:** agent session (integration lane run), operator-confirmed
**Working tree HEAD at reproduction:** `3af96a02`
**Working tree HEAD at fix verification:** `3af96a02` (fix is uncommitted working-tree state)
**Fix evidence:** `./smackerel.sh test integration` → `INTEGRATION_EXIT=0`, zero `--- FAIL` / `FAIL github`
lines across 8218 log lines, with `--- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (11.99s)`
(preserved at `~/s064-integration.log`, lines 3561 and 8218); unit tier at `~/s064-unit.log`.

---

## Summary

`TestOpenKnowledgeRouting_FallbackToOpenKnowledge` in
[tests/integration/agent/openknowledge_routing_test.go](../../../../tests/integration/agent/openknowledge_routing_test.go)
wraps `agent.NewRouter` in a hard-coded 30-second `context.WithTimeout`.
`NewRouter` performs **79 sequential HTTP `POST /embed` round-trips** against the
ML sidecar — one per `intent_examples` entry across every registered scenario —
before it returns. That is variable-cost, cold-start-dominated warm-up work
measured against a fixed wall clock, so the test's outcome is a function of
sidecar warmth rather than of the routing contract it is supposed to assert.

The integration lane is a blocking gate. When the sidecar is cold the gate fails
with an error that is visually indistinguishable from a genuine routing
regression.

---

## Reproduction

```bash
# Full lane (as run 2026-08-11)
./smackerel.sh test integration

# Focused re-run
./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'
```

Preconditions that make it reproduce: a freshly-created test stack, i.e. one
where the `smackerel-test-ollama-data` volume and the ML sidecar's
sentence-transformer model cache have been recreated and the embedder has not
yet been warmed.

---

## Observed vs Expected

| | |
|---|---|
| **Expected** | The test asserts SCOPE-12 routing behaviour: a weather query does not steal `open_knowledge`; an open-ended question and a conversion question both land on `open_knowledge`. Router construction is set-up, not the thing under test, and should not decide the verdict. |
| **Actual** | The test never reaches a single routing assertion. `agent.NewRouter` returns `context deadline exceeded` partway through scenario warm-up and the test fails at `openknowledge_routing_test.go:128`. |

### Run A — full lane (`~/bug011-integration-a9.log:3964-3966`)

```
=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
    openknowledge_routing_test.go:128: build router: agent: NewRouter: embed scenario "recipe_search" intent_examples[4]: sidecar.Embed: POST /embed: Post "http://smackerel-ml:8081/embed": context deadline exceeded
--- FAIL: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (32.14s)
```

### Run B — focused re-run (`~/bug011-flake-check.log:293-295`)

```
=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
    openknowledge_routing_test.go:128: build router: agent: NewRouter: embed scenario "hospitality_concern_evaluate" intent_examples[1]: sidecar.Embed: POST /embed: Post "http://smackerel-ml:8081/embed": context deadline exceeded
--- FAIL: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (33.40s)
```

**Claim Source:** executed — both blocks are verbatim from the preserved logs
named above. No integration run was performed in this session; the operator
captured these and directed that the lane not be re-run.

---

## Why the two runs differ, and why that matters

The two failures aborted at **different points in the same warm-up loop**:

| Run | Reported failure point | Ordinal call | Calls completed in ~30s | Implied per-call latency | Budget the full set would need |
|-----|------------------------|--------------|--------------------------|--------------------------|-------------------------------|
| A   | `recipe_search[4]` | **31 of 79** | 30 | ≈1.0 s | ≈79 s |
| B   | `hospitality_concern_evaluate[1]` | **13 of 79** | 12 | ≈2.5 s | ≈197 s |
| earlier pass (~21:29) | — | 79 of 79 | 79 | ≤0.38 s | <30 s |

This is the single most important fact in the report, and it corrects the
initial read that the test "overshoots by 2–3 seconds". The *wall-clock*
overshoot is small only because the deadline truncates the work. The *work*
deficit is 62% (run A) to 85% (run B) of the embed set still outstanding when
the budget expires. Per-call latency varies roughly **6×** across observed runs.

Consequence for the obvious remedy: raising the literal to 35s or 60s would not
have rescued either failing run. Covering run B needs roughly 200s, and even
that is a guess rather than a bound, because nothing in the current design
bounds cold-start cost.

---

## Environment at time of failure

Recorded for completeness; the operator captured these alongside the runs.

- Host: 47 GB RAM, ~2 GB free, 26 GB buff/cache
- Docker: 130 images / 56.21 GB, 839 build-cache entries
- `config/smackerel.yaml` (comment block at lines 2465–2495) documents that the
  test stack runs its **own** Ollama container and that
  `scripts/commands/ollama-test-pull.sh` performs a genuine cold `/api/pull`
  into the disposable `smackerel-test-ollama-data` volume **on every fresh test
  stack**. Model and embedder warm-up cost is therefore paid again on each
  fresh stack by design, not by accident.

Memory pressure and a cold volume plausibly explain why per-call latency landed
at ~2.5 s in run B versus ~1.0 s in run A. This is a contributing factor to the
*magnitude*, not the cause: the defect is that a fixed deadline is applied to
this work at all.

---

## Evidence that this is neither a flake nor collateral damage from the concurrent fix

### 1. Not transient — reproduced 2/2

Two consecutive runs, different invocations (full lane, then focused `--go-run`),
both `exit=1`, same failure mode. A defect that reproduces on every attempt is
not a flake; it is a deterministic failure under the current environment
conditions.

### 2. Not caused by the in-flight BUG-061-011 change

The BUG-061-011 edits were already fully present on disk (files written 21:17)
when an earlier full integration run at ~21:29 exited **0** with this same test
passing. Identical product code later produced a failure. Same code + different
outcome ⇒ the differing input is environmental/timing, not the change.

### 3. Ordering rules it out independently

BUG-061-011 added `./tests/eval/...` to the lane's package list in
[scripts/runtime/go-integration.sh](../../../../scripts/runtime/go-integration.sh).
That lane runs `go test -p 1 …` (serial packages) with the eval package listed
last, and the full-lane log confirms the resulting order empirically:

```
~/bug011-integration-a9.log:3996  FAIL  github.com/smackerel/smackerel/tests/integration/agent   39.561s
~/bug011-integration-a9.log:8429  ok    github.com/smackerel/smackerel/tests/eval/assistant       0.142s
```

The failing package completes ~4400 log lines **before** the added package
begins. The added package cannot have influenced it.

**Claim Source:** executed — line numbers and text read from the preserved log
this session.

---

## Second, independent finding recorded in this bug

The same code region hard-codes four timing/threshold constants that shadow
values the SST already publishes into the very env-file the test container
receives. These are **not** the cause of the failure above; they are recorded
here because they sit in the same ~40 lines and because one of them diverges
from its SST counterpart.

| Literal in test | Line | SST counterpart | Present in `config/generated/test.env`? | Diverges? |
|---|---|---|---|---|
| `parseFloatEnv("AGENT_ROUTING_CONFIDENCE_FLOOR", 0.65)` | 119 | `agent.routing.confidence_floor: 0.65` | yes, line 484 (`0.65`) | no — matches today |
| `parseIntEnv("AGENT_ROUTING_CONSIDER_TOP_N", 5)` | 120 | `agent.routing.consider_top_n: 5` | yes, line 485 (`5`) | no — matches today |
| `sidecar.New(..., 5*time.Second)` per-call | 89 | `assistant.routing.embed_timeout_ms: 30000` | yes, line 489 (`30000`) | **YES — 5 s vs 30 s** |
| `context.WithTimeout(..., 30*time.Second)` aggregate | 125 | *(no SST key exists)* | n/a | n/a — unmanaged |

See [design.md](design.md) § "Finding 2" for the precise policy determination,
including the parts of the no-defaults policy that these do **not** violate.

---

## Impact

- The integration lane — a blocking gate — fails non-deterministically whenever
  the ML sidecar is cold. On the two runs recorded here it failed every time.
- The failure text (`NewRouter: embed scenario … context deadline exceeded`)
  reads like a routing/embedder regression. A reader triaging a red gate can
  reasonably mis-attribute it to whatever change is in flight, which is exactly
  what happened here.
- It blocked verification of unrelated in-flight work (BUG-061-011).
- **No product impact.** Production does not apply an aggregate deadline to
  router construction, and production sources its per-call embed timeout from
  SST at 30 s per call. The defect is confined to this test.

---

## Severity justification — S2

**S2, not S1:** spec 064's S1 bugs (BUG-064-001, BUG-064-002) were user-visible
product defects in `/ask` output. This one cannot reach a user: no shipped code
path contains the offending deadline.

**S2, not S3/low:** it reproduced 2/2 on a mandatory blocking gate; the budget
shortfall is structural (62–85% of the work outstanding at expiry) rather than
marginal, so it will not self-resolve; and it actively misdirected triage of a
concurrent bug.

The operator may reasonably prefer a lower severity on the grounds that this is
test-only. That reading is recorded here rather than argued away.

---

## Constraints honoured while filing

- No source file was modified. `openknowledge_routing_test.go` is unchanged.
- Nothing under `BUG-061-011-*`, `docs/releases/`, `config/release-trains.yaml`,
  or the OPS-006 `state.json` files was touched.
- `./smackerel.sh test integration` was **not** run in this session, per operator
  direction; all failure evidence is quoted from the operator's preserved logs.

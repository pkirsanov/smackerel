# Design: BUG-064-003 — root cause and fix direction

**Bug:** [bug.md](bug.md) · **Spec:** [spec.md](spec.md)
**Status:** design complete — implementation NOT started

---

## 1. Root cause

### 1.1 The mechanism

[tests/integration/agent/openknowledge_routing_test.go](../../../../tests/integration/agent/openknowledge_routing_test.go)
line 125:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
router, err := agent.NewRouter(ctx, cfg, registered, embedder)
```

That single `ctx` covers the whole of `NewRouter`.
[internal/agent/router.go:135-180](../../../../internal/agent/router.go) shows what
`NewRouter` does with it — a nested serial loop, one HTTP round-trip per example:

```go
for _, sc := range sorted {
    ...
    for i, ex := range sc.IntentExamples {
        v, err := embedder.Embed(ctx, ex)      // one POST /embed, sequential
        if err != nil {
            return nil, fmt.Errorf("agent: NewRouter: embed scenario %q intent_examples[%d]: %w", sc.ID, i, err)
        }
        ...
    }
}
```

Counting `intent_examples` across `config/prompt_contracts/*.yaml` with a YAML
parser, in the scenario-ID sort order `NewRouter` itself imposes:

```
alert_timing_evaluate            3   cum=3
annotation_classify              4   cum=7
e2e_ollama_smoke                 1   cum=8
expertise_classify               3   cum=11
hospitality_concern_evaluate     3   cum=14
notification_schedule            4   cum=18
open_knowledge                   8   cum=26
recipe_search                    9   cum=35
recommendation_feedback          3   cum=38
recommendation_reactive          3   cum=41
recommendation_watch_evaluate    3   cum=44
recommendation_why               2   cum=46
relationship_cooling_evaluate    3   cum=49
resurface_evaluate               3   cum=52
retrieval_evergreen              3   cum=55
retrieval_qa                     5   cum=60
weather_query                   19   cum=79
(11 further scenarios declare 0 intent_examples and are skipped)

TOTAL sequential /embed calls = 79
```

**Claim Source:** executed — `python3` + `yaml.safe_load` over
`config/prompt_contracts/*.yaml`, this session.

So a fixed 30-second budget covers **79 sequential network round-trips into a
model server that may not have loaded its sentence-transformer yet**. That is
the defect: a wall-clock constant is being used as a correctness gate over
variable-cost warm-up work.

### 1.2 The arithmetic that makes it decisive

The two recorded failures aborted at different ordinals, which lets per-call
latency be derived rather than guessed:

| Run | Failure point | Ordinal | Completed in ~30 s | Derived per-call | Budget needed for all 79 |
|-----|---------------|---------|--------------------|------------------|--------------------------|
| A (full lane) | `recipe_search[4]` | 31 | 30 | ≈1.0 s | ≈79 s |
| B (focused) | `hospitality_concern_evaluate[1]` | 13 | 12 | ≈2.5 s | ≈197 s |
| earlier pass | completed | 79 | 79 | ≤0.38 s | <30 s |

Two conclusions follow, and they drive the whole fix direction:

1. **The shortfall is structural, not marginal.** At expiry, 62% (run A) to 85%
   (run B) of the embed set was still outstanding. The 32.14 s / 33.40 s wall
   times look like a near miss only because the deadline truncates the work.
2. **Per-call latency spans ≈6×** between warm and cold. Any fixed budget
   therefore encodes an assumption about sidecar warmth, and that assumption is
   the thing that varies.

### 1.3 The contradiction that confirms it

`config/smackerel.yaml:1628` already records this exact problem, and its own
comment says so:

```yaml
embed_timeout_ms: 30000 # REQUIRED: per-call timeout for the sidecar /embed
  # roundtrip. Spec 064 SCOPE-17 raised from 500ms to accommodate cold
  # sentence-transformer load on first startup; subsequent calls return in <50ms.
```

Production budgets **30 seconds for one cold embed call**. This test budgets
**30 seconds for seventy-nine of them** — and separately caps each individual
call at 5 s via `sidecar.New(sidecarURL, token, 5*time.Second)` (line 89), one
sixth of what SST declares legal.

The test's timing model therefore contradicts the SST-declared timing model of
the very subsystem it exercises. That is a cleaner statement of the root cause
than "the timeout is too small": the timeout was written against an assumption
(embeds are fast) that the repository's own configuration documents as false on
a cold sidecar.

It also explains the observed pass/fail pattern exactly:

- warm sidecar → <50 ms/call → 79 calls ≈ 4 s → passes with enormous margin;
- cold sidecar → seconds/call → deadline expires partway through, and *where*
  it expires depends on how cold the sidecar is, which is why runs A and B
  aborted at different scenarios.

### 1.4 Contributing factor (magnitude, not cause)

`config/smackerel.yaml` lines 2465–2495 document that the test stack runs its
own Ollama container and performs a genuine cold `/api/pull` into the disposable
`smackerel-test-ollama-data` volume **on every fresh test stack** — a deliberate
isolation choice (bubbles-test-environment-isolation / G115), not a bug. Combined
with the host state at failure time (~2 GB free of 47 GB; 56 GB of Docker
images), this plausibly explains why per-call latency reached ≈2.5 s in run B.

This raises the cost. It does not create the defect. Even a perfectly-provisioned
host leaves a fixed deadline sitting over an unbounded cold start.

---

## 2. Finding 2 — hard-coded constants, checked against the no-defaults policy

The user asked for a precise determination against
[.github/instructions/smackerel-no-defaults.instructions.md](../../../../.github/instructions/smackerel-no-defaults.instructions.md).
That file was read in full. Its operative rule is:

> For SST-managed runtime values, the following are forbidden in source,
> Compose, deploy specs, scripts, examples, and docs …
> `os.getenv("KEY", "default")` … **Any helper that silently supplies a runtime
> fallback value**

Taking the four constants one at a time.

### 2.1 `parseFloatEnv("AGENT_ROUTING_CONFIDENCE_FLOOR", 0.65)` and `parseIntEnv("AGENT_ROUTING_CONSIDER_TOP_N", 5)`

Both helpers are defined locally in the test file (lines 217 and 229) and both
return the fallback on **empty value** *and* on **parse error**:

```go
func parseFloatEnv(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" { return fallback }
	f, err := strconv.ParseFloat(v, 64)
	if err != nil { return fallback }
	return f
}
```

**Determination — these match the forbidden shape.**

- The values are unambiguously SST-managed: `config/smackerel.yaml:1614-1615`
  marks both `REQUIRED`; `scripts/commands/config.sh:1861-1862` resolves them
  through `required_value`; they land in `config/generated/test.env` at lines
  484–485.
- Go's `os.Getenv` takes one argument, so the policy's `os.getenv("KEY", "default")`
  example is describing a *shape*, and the catch-all clause "Any helper that
  silently supplies a runtime fallback value" names these two helpers precisely.
- The repository has already eradicated this exact shape from production twice:
  `BUG-020-003` removed `func parseFloatEnv(key string) float64` from
  `cmd/core/helpers.go`, and `BUG-020-008` removed
  `func parseIntEnv(key string, defaultVal int) int` from
  `internal/config/config.go`. Production's replacement is the fail-loud
  `requireFloat` / `requireInt` at `internal/agent/config.go:155-156`.

**Determination — but they are NOT a gate violation, and I will not claim they
are.** The repository's mechanical enforcement of this policy,
`internal/config/sst_grep_guard_test.go`, deliberately excludes them twice over:

```
line 13:  //   - `*_test.go`, `*_test.py`, `ml/tests/` — test fixtures with explicit …
line 75:  if strings.HasSuffix(path, "_test.go") { … skip }
line 82:  if strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)) { … skip }
```

and the guard even carries a meta-test (lines 285–310) asserting that the
`*_test.go` skip works. `tests/integration/agent/openknowledge_routing_test.go`
is excluded on both counts.

So the honest statement is: **the prose forbids the shape and contains no
test-code carve-out; the mechanical guard intentionally does not scan test code.**
Whether the prose is meant to bind test files is an owner decision, not
something this bug should assert. It is raised here, not resolved here.

**Determination — currently inert.** `test.env` supplies `0.65` and `5`, exactly
what the fallbacks hard-code, so the fallback branch is not taken today. The
risk is future silent divergence: change `agent.routing.confidence_floor` in SST
and, in any environment where the variable fails to propagate, this test keeps
asserting against the stale constant instead of failing loud.

### 2.2 `30*time.Second` (line 125)

**Determination — this is NOT a no-defaults violation, and the report must not
say it is.** It is not fallback syntax; it does not shadow any SST key, because
no SST key for an aggregate router-construction budget exists. The policy as
written forbids *fallback* forms for SST-managed values; it does not forbid
every numeric literal.

The argument against this literal is a design argument, made in §3 below, and it
stands on its own evidence (the 79-call cold-start analysis). It gains nothing
from being mislabelled a policy breach — and the neighbouring `embed_timeout_ms`
key shows the repository's own habit is to promote exactly this kind of value
into SST, which is a stronger argument than a citation that does not apply.

### 2.3 `5*time.Second` per-call timeout (line 89)

**Determination — not a no-defaults violation either** (again, not fallback
syntax), but it is a genuine **divergence** and the more actionable of the two.
The test enforces a 5 s per-call ceiling while `ASSISTANT_ROUTING_EMBED_TIMEOUT_MS=30000`
sits in the very env-file the container is given. A cold call taking 8 s is
legal in production and fatal in this test.

Note it is not what failed here: no individual call exceeded 5 s in either run
(derived per-call latencies were ≈1.0 s and ≈2.5 s), and the reported error
came from the outer 30 s context. It is a latent second failure mode.

---

## 3. Fix options and recommendation

### Option A — readiness / warm-up gate before the timed region *(recommended)*

Warm the embedder to a known-ready state before any deadline starts. The test
already has the raw material: it performs a single probe embed at lines 92–96
and *skips* if the sidecar is unreachable. Extend that probe into a bounded
readiness wait — poll `/embed` until latency settles below a threshold, or until
a generous readiness budget expires — and fail with an explicit
embedder-not-ready message distinct from any routing error.

- **Satisfies** AC-1, AC-2, AC-3.
- **Why it is right:** it removes the category error. Cold-start cost stops being
  measured by a correctness assertion and becomes an explicit precondition, so a
  red gate again means "routing is broken".
- **Cost:** the readiness wait itself needs a ceiling, so a time constant does
  not disappear entirely — but it moves to a place where exceeding it means
  "the sidecar never became ready", which is a true and diagnosable statement,
  rather than "routing failed", which is a false one.
- **Risk:** a genuinely dead embedder now costs the readiness budget before
  failing. Acceptable; it already costs 30 s today and reports the wrong cause.

### Option B — source the budget from SST, and scale it with the work

Introduce an SST key for router-construction budget (or reuse
`assistant.routing.embed_timeout_ms` as a per-call unit) and compute the deadline
as `len(all intent_examples) × per-call-budget × safety-factor` rather than a
literal.

- **Satisfies** AC-3, AC-4, and aligns with the repository's existing pattern.
- **Why it is not sufficient alone:** it makes the budget honest and
  self-adjusting as scenarios are added — a real improvement, since today adding
  one `intent_examples` entry silently erodes the margin with no signal — but it
  still measures cold start with a clock. With the SST per-call value of 30 s,
  79 × 30 s yields a ~40-minute ceiling, far beyond the lane's `-timeout 300s`.
  A smaller factor is back to guessing.
- **Best used as a complement to A**, and the per-call divergence in §2.3 should
  be fixed here regardless of which option is chosen.

### Option C — simply enlarge the 30 s literal *(rejected)*

The obvious move, and it must be evaluated honestly rather than dismissed.

- It is cheap, it is one line, and it would have made the earlier passing run
  pass again.
- It would **not** have rescued either recorded failure. Run A needed ≈79 s and
  run B ≈197 s. A bump to 35 s or 60 s fails both.
- Choosing a number that covers run B (~200 s) is not a bound — it is the worst
  case *observed so far*, on one host, on one day. Nothing in the design bounds
  cold-start cost, so the next slower host moves the number again.
- It also collides with the lane's own `go test -timeout 300s`: a 200 s router
  build inside a 300 s package budget leaves little for the other tests in
  `tests/integration/agent`.

**Verdict: enlarging the timeout masks the issue rather than fixing it.** It
converts a frequently-red gate into an occasionally-red one, which is worse in a
specific way — a rare failure is far more likely to be waved through as "flaky"
than a reproducible one, and this failure's text already invites
mis-attribution to whatever change is in flight.

### The design tension, stated plainly

A fixed deadline over a cold-start-dependent operation is inherently
timing-fragile. Raising the number buys time without removing the fragility.
The only structural remedies are to stop measuring cold start (Option A —
separate readiness from assertion) or to stop the operation being cold-start
dependent at all (warm the sidecar in stack bring-up, or batch/parallelise the
79 embeds — both outside this bug's scope, the latter belonging to spec 064
proper).

### Recommendation

**Option A as the fix, with the Option B per-call correction (§2.3) folded in.**

1. Replace the single probe embed with a bounded embedder-readiness gate that
   fails with an explicit, distinguishable readiness error.
2. Start the router-construction deadline only after readiness is established,
   and derive it from the number of intent examples rather than a literal.
3. Raise the test's per-call `sidecar.New` timeout so it is not stricter than
   `assistant.routing.embed_timeout_ms`.
4. Consume `AGENT_ROUTING_CONFIDENCE_FLOOR` / `AGENT_ROUTING_CONSIDER_TOP_N`
   fail-loud, mirroring `internal/agent/config.go:155-156`, and delete the two
   local fallback helpers.

Item 4 is deliberately bundled: it is a small, contained change in the same
region, it removes a shape the repository has twice removed from production, and
leaving it in place preserves a silent-divergence hazard for no benefit.

---

## 4. Affected surface (proposed — nothing modified by this bug)

| File | Change |
|------|--------|
| `tests/integration/agent/openknowledge_routing_test.go` | readiness gate; derived deadline; per-call timeout from SST; fail-loud env reads; remove `parseFloatEnv` / `parseIntEnv` |
| `config/smackerel.yaml` | *possibly* one new REQUIRED key for router-construction budget, if Option B's derived form needs a tunable factor |
| `scripts/commands/config.sh` | emit that key, if introduced |
| `config/generated/*.env` | regenerated output, if that key is introduced |

No product source file is expected to change. If implementation finds that it
must, that is a signal the fix has drifted out of this bug's scope and should be
routed back for re-scoping.

---

## 5. Open questions for the owner

1. **Does the no-defaults policy bind test code?** §2.1 establishes that the
   prose has no carve-out while the mechanical guard has an explicit one. If the
   answer is "yes", the guard's `_test.go` / `tests/` exclusions are themselves
   a gap worth a separate bug against spec 020. This bug does not presume the
   answer.

   **DECISION (recorded 2026-08-28, owner-delegated): YES — the policy binds
   test code, but what it forbids is the fallback FORM, not explicit test
   values.** The two are different things and the current carve-out conflates
   them.

   The distinction that matters:

   - `os.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.7")` in a test is an
     EXPLICIT value. It is permitted, and always was. The policy never targeted
     it.
   - A helper of the shape `getenvOr("AGENT_ROUTING_CONFIDENCE_FLOOR", 0.65)`
     inside test code is a SILENT SUBSTITUTION. It is forbidden, because it lets
     a test pass under a configuration production would reject — which erases
     the very fail-loud contract the test exists to prove.

   The guard's own words show it was aimed at the first case, not licensing the
   second. `internal/config/sst_grep_guard_test.go:13` justifies the exemption
   as *"test fixtures with explicit intent to use deterministic test values"*.
   Explicit is the operative word. The MECHANISM, however, is broader than the
   rationale: `sstGuardSkipFile` skips whole files by suffix
   (`strings.HasSuffix(path, "_test.go")`, line 75), so it exempts the
   silent-substitution case too, and the meta-test at lines 285-310 pins that
   file-level skip in place.

   Why the answer is "yes" rather than "tests are exempt": this bug's own T4
   asserts that absent or unparseable `AGENT_ROUTING_CONFIDENCE_FLOOR` /
   `AGENT_ROUTING_CONSIDER_TOP_N` fail loud instead of substituting `0.65` / `5`.
   If test code were free to substitute defaults, a future helper could satisfy
   T4's setup while quietly restoring the behaviour T4 forbids, and the guard
   would not see it. A test suite that may itself default stops being evidence
   that the fail-loud contract holds. That is the same false-green class already
   filed in this repository as BUG-069-005.

   **Consequence, per this question's own terms:** the `_test.go` / `tests/`
   exclusion IS a gap. It is not fixed here — it belongs to spec 020 and to the
   guard's owner, and narrowing a file-level skip to a form-level one is a
   change with its own blast radius across every existing test. It is filed as
   residue `R-012` in `.specify/memory/open-work.md` with `bubbles.design` as
   next owner, so it is owned rather than lost. NOT claimed: that any current
   test actually exploits the gap. This decision is about what the policy means,
   and no audit for live violations was performed.
2. **Should stack bring-up warm the ML sidecar embedder?** That would fix this
   class of problem for every integration test at once rather than one test at a
   time, but it changes `smackerel.sh` / compose behaviour and belongs to a
   different owner.
3. **Is `weather_query` intended to carry 19 intent examples?** It is 24% of the
   total embed cost by itself. Not a defect, but it is the single largest
   contributor to warm-up time and worth a deliberate confirmation.

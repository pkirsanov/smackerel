# Spec: BUG-064-003 — deterministic router construction in the SCOPE-12 routing integration test

**Bug:** [bug.md](bug.md)
**Parent spec:** `specs/064-open-ended-knowledge-agent` (SCOPE-12)
**Status:** specified — implementation NOT started

---

## 1. Purpose

Define the behaviour `tests/integration/agent/openknowledge_routing_test.go` MUST
exhibit so that its verdict reflects the SCOPE-12 routing contract and nothing
else. This spec covers the test's own timing and configuration contract. It does
**not** change any product behaviour, and it does **not** change what SCOPE-12
asserts about routing.

---

## 2. Expected behaviour

### EB-1 — The test's verdict depends only on routing behaviour

`TestOpenKnowledgeRouting_FallbackToOpenKnowledge` MUST fail if and only if one
of its three routing assertions fails:

1. `"weather in paris today"` MUST NOT route to `open_knowledge`.
2. `"explain quantum entanglement briefly"` MUST route to `open_knowledge`.
3. `"what is 10F in C"` MUST route to `open_knowledge`.

Router construction is set-up. Its cost MUST NOT be able to decide the verdict.

### EB-2 — Embedder warm-up is bounded by readiness, not by a wall clock

Before the timed region begins, the test MUST establish that the ML sidecar's
embedder is warm, or MUST fail with an error that names embedder readiness as
the cause and is distinguishable from a routing failure.

Rationale: `agent.NewRouter` issues one sequential `POST /embed` per
`intent_examples` entry across every registered scenario — 79 calls at the time
of writing. Cold-start cost dominates and is unbounded by anything in the
current design; observed per-call latency varied ≈6× across runs (≤0.38 s warm,
≈2.5 s cold). A fixed budget over that work is not a correctness property.

### EB-3 — Any remaining budget scales with the work, and comes from SST

If a deadline is retained around router construction, it MUST NOT be a literal
that is independent of the number of embed calls, and its magnitude MUST be
sourced from configuration rather than hard-coded in the test.

### EB-4 — The test's per-call embed timeout agrees with SST

The per-call timeout passed to `sidecar.New` MUST NOT be stricter than the
SST-declared `assistant.routing.embed_timeout_ms`. Today the test enforces 5 s
while SST declares 30 000 ms, so the test can fail a call that production
considers well within budget.

### EB-5 — Routing configuration is read from the environment without silent fallback

`AGENT_ROUTING_CONFIDENCE_FLOOR` and `AGENT_ROUTING_CONSIDER_TOP_N` are SST-managed
and are present in `config/generated/test.env`, which the integration container
receives via `--env-file`. The test MUST consume them; if either is absent or
unparseable the test MUST fail loud rather than substitute a stale constant.

Production already behaves this way — `internal/agent/config.go:155-156` uses
`requireFloat` / `requireInt`. The test MUST NOT be laxer than the code it
exercises.

---

## 3. Acceptance criteria

| ID | Criterion |
|----|-----------|
| AC-1 | On a cold test stack, `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` either reaches all three routing assertions, or fails with an explicit embedder-readiness error. It never fails with a bare `NewRouter: … context deadline exceeded`. |
| AC-2 | On a warm test stack the test passes and its three routing assertions are evaluated. |
| AC-3 | No wall-clock literal in the test governs a variable-cost, cold-start-dependent operation. |
| AC-4 | The per-call embed timeout used by the test is not stricter than `assistant.routing.embed_timeout_ms`. |
| AC-5 | Removing `AGENT_ROUTING_CONFIDENCE_FLOOR` or `AGENT_ROUTING_CONSIDER_TOP_N` from the environment causes a loud failure, not a silent fallback to `0.65` / `5`. |
| AC-6 | No product source file changes. The fix is confined to test code and, if a new budget key is introduced, to SST config plus its generator. |

---

## 4. Out of scope

- Any change to `agent.NewRouter`, the router's embedding strategy, or the
  sidecar client. Parallelising or batching the 79 embed calls is a legitimate
  performance idea but belongs to spec 064 proper, not to this bug.
- Any change to what SCOPE-12 asserts about routing outcomes.
- The `smackerel-test-ollama-data` recreate-per-stack policy. That policy is
  deliberate and documented (`config/smackerel.yaml` lines 2465–2495); this bug
  adapts to it rather than contesting it.
- Rewriting `parseFloatEnv` / `parseIntEnv` usages in files other than
  `tests/integration/agent/openknowledge_routing_test.go`.

---

## 5. Non-goals explicitly rejected

**Simply enlarging the 30 s literal is rejected as the sole remedy.** It fails
AC-1 and AC-3. See [design.md](design.md) § "Option C" for the arithmetic: the
observed runs would have required ≈79 s and ≈197 s respectively, and neither
figure is an upper bound on cold-start cost.

# BUG-064-001 — Execution report

> Evidence standard: raw terminal output, ≥10 lines per claim, captured in this
> session. Home-directory paths redacted to `~` per repo PII policy.

## Summary

Two defects on the `/ask` open-knowledge surface:
- **A (answerability):** `open_knowledge` refused every query at the pre-flight
  per-user-monthly USD gate because the SST budget was `0` and the production
  `CostFn` is zero-cost. Routing (`/ask → open_knowledge`) is correct; not
  deployment lag.
- **B (capture pollution):** the Telegram adapter passed the verbatim
  `/ask`-prefixed text into the capture hook.

Both root causes were proven (code + live self-hosted read-only inspection), fixed
in-repo, and covered by adversarial regression tests that are GREEN.

## Completion Statement

The in-repo fix is COMPLETE and validated:
- DEFECT A — `config/smackerel.yaml` open-knowledge monthly budgets set to
  positive ceilings (100 / 25); `docs/Operations.md` stale `0 = unlimited` claim
  corrected to the enforced `0 = refuse-all` semantics. The pre-flight gate
  (SCN-064-A08) is intentionally unchanged.
- DEFECT B — `internal/telegram/assistant_adapter/adapter.go` strips the v1
  slash-command prefix (`assistant.StripShortcutPrefix`) before capture.
- Tests — 3 new `*_bug064001_test.go` files (adversarial + preservation +
  verbatim + config-contract); all GREEN; `go test ./...` (vet+compile) and
  `gofmt --check` clean.

This bug remains `blocked` (not `done`) because it is a LIVE S1 production
defect: the deployed image (`sourceSha 0bc04cfb`) ships the `0` budget, so the
live self-hosted symptom is cleared ONLY after `bubbles.devops` redeploys the fixed
SHA + regenerated self-hosted config bundle via the knb adapter. Code alone does not
clear the live symptom — a redeploy is required.

## Test Evidence

| Test | Category | Before fix | After fix |
|------|----------|-----------|-----------|
| `TestShippedConfig_BUG064001_OpenKnowledgeBudgetsAllowOperation` | unit (config-contract) | FAIL (`per_user_monthly_budget_usd = 0`) | PASS |
| `TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight` | unit | (n/a — code correct) | PASS (`status=success`, iterations=2) |
| `TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight` | unit | PASS (reproduces live `cap_usd`) | PASS (SCN-064-A08 preserved) |
| `TestHandleUpdate_BUG064001_CaptureStripsAskPrefix` | unit | FAIL (`"/ask tide…"` leaks) | PASS |
| `TestHandleUpdate_BUG064001_AllV1ShortcutsStripped` (5 subtests) | unit | FAIL (all 5) | PASS (all 5) |
| `TestHandleUpdate_BUG064001_NonShortcutCapturedVerbatim` | unit | PASS (guard) | PASS |

Raw RED output: [#repro-red](#repro-red). Raw GREEN output:
[#scope-01-unit](#scope-01-unit), [#scope-01-config-test](#scope-01-config-test),
[#scope-02-unit](#scope-02-unit). Build gate: [#build-gate](#build-gate).

### Scenario-first (TDD) red→green

This fix followed scenario-first TDD (`forceTddMode: scenario-first`). Order in
this session: (1) the three adversarial `*_bug064001_test.go` files were written
and run FIRST → **RED** ([#repro-red](#repro-red)); (2) the config + adapter
fixes were applied; (3) the same tests were re-run → **GREEN**
([#scope-01-unit](#scope-01-unit), [#scope-02-unit](#scope-02-unit)). The
red→green transition is captured in one session.

### Code Diff Evidence {#code-diff}

DEFECT A — `config/smackerel.yaml` (`assistant.open_knowledge`):

```diff
-    monthly_budget_usd: 0 # REQUIRED: >= 0 (must be explicit)
-    per_user_monthly_budget_usd: 0 # REQUIRED: >= 0
+    monthly_budget_usd: 100 # REQUIRED: > 0 when enabled (global monthly USD ceiling)
+    per_user_monthly_budget_usd: 25 # REQUIRED: > 0 when enabled (per-user monthly USD ceiling)
```

DEFECT B — `internal/telegram/assistant_adapter/adapter.go` (`HandleUpdate`):

```diff
-	if resp.CaptureRoute && update.Message != nil {
-		a.capture(ctx, update.Message, msg.Text)
-	}
+	if resp.CaptureRoute && update.Message != nil {
+		a.capture(ctx, update.Message, assistant.StripShortcutPrefix(msg.Text))
+	}
```

plus the new `import "github.com/smackerel/smackerel/internal/assistant"` and the
local `assistant` → `facade` rename in `HandleUpdate` (avoids shadowing the
imported package). `docs/Operations.md` budget rows corrected from the stale
`0 = unlimited` claim to the enforced `0 = refuse-all` (SCN-064-A08) semantics.

---

## Root cause evidence (read-only, live self-hosted deployment) {#root-cause}

### Deployed source SHA contains the routing fix (rules out deployment lag for routing)

```
$ git merge-base --is-ancestor ebdbf852 0bc04cfb && echo "YES: deployed CONTAINS /ask->open_knowledge"
YES: deployed SHA CONTAINS /ask->open_knowledge fix

$ git show 0bc04cfb:internal/assistant/shortcuts.go | grep -nE '"/ask"|open_knowledge'
42:     // Spec 064 SCOPE-17 — /ask reroutes to open_knowledge so users get
43:     // the open-ended agent (web search + internal retrieval + tools)
47:     "/ask":     "open_knowledge",

$ git log -1 --format='%H %ci %s' ebdbf852
ebdbf85219e62cf64ffcec2d4587f4b657cd83a5 2026-06-01 04:30:37 +0000 spec 064: route /ask shortcut to open_knowledge scenario
```

### On-host deployed manifest (sourceSha + applied time)

```
# ~/<deployment-owner>/<product>/<target>/manifest.yaml (on <deploy-host>)
current:
  appliedAt: "2026-06-11T06:03:09Z"
  appliedBy: "<operator>@<deploy-host>"
  sourceSha: "0bc04cfb304382c89b6e264c3b37eabd80ba3397"
  images:
    core: "ghcr.io/pkirsanov/smackerel-core@sha256:9a3ce7f316f12cadb4d2e9dfe8e61ae8f455a3e16782e9447e7778eefac5d101"
```

### `assistant_turn` log proves `/ask` reaches `open_knowledge`, band=high

```
{"msg":"assistant_turn","user_id":"philip","transport":"telegram",
 "scenario_id":"open_knowledge","top_score":0,"band":"high",
 "status":"saved_as_idea","error_cause":"","latency_ms":3.398663}
```

`latency_ms: 3.4` is far too fast for a real LLM+web-search agent → the agent
short-circuited before doing work.

### `openknowledge.turn` log proves the pre-flight USD gate is the cause

```
{"msg":"openknowledge.turn","turn_id":"43cdf83f81d3fce6","iterations":1,
 "tokens_used":0,"usd_spent":0,"status":"refused","termination_reason":"cap_usd",
 "num_sources":0,"tool_calls":[],
 "refusal_reason":"openknowledge: per-user monthly USD budget exceeded"}
```

`iterations:1, tokens_used:0, tool_calls:[]` ⇒ refused before any LLM/tool call.

### Deployed env confirms the $0 budget and zero-cost local providers

```
ASSISTANT_OPEN_KNOWLEDGE_ENABLED=true
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER=searxng
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT=http://searxng:8080
ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD=0
ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD=0
ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET=0.05
ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID=gemma4:26b
```

Ollama models present on host (the configured model EXISTS — model is not the
problem):

```
NAME            ID              SIZE      MODIFIED
gemma4:26b      5571076f3d70    17 GB     3 weeks ago
gemma3:4b       a2af6cc3eb7f    3.3 GB    4 weeks ago
llama3.1:8b     46e0c10c039e    4.9 GB    9 days ago
```

**Conclusion:** routing correct, model present, web-search infra present; the
sole blocker is the `$0` per-user-monthly budget hitting the pre-flight gate
`agent.go: if a.cfg.PerUserMonthlyUSDRemaining <= 0 { refuse(cap_usd) }`, while
the production `CostFn` charges `$0` (so the running caps never fire). Current
`main` ships the same `0` values → genuine code+config bug, not deployment lag.

---

## Reproduction gate (Gate 0) — RED before fix {#repro-red}

Focused run `./smackerel.sh test unit --go --go-run 'BUG064001' --verbose`
BEFORE applying either fix (same session):

```
=== RUN   TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight
INFO openknowledge.turn ... iterations=1 tokens_used=0 usd_spent=0 status=refused
  termination_reason=cap_usd num_sources=0 tool_calls=[]
  refusal_reason="openknowledge: per-user monthly USD budget exceeded"
--- PASS: TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight (0.00s)
=== RUN   TestShippedConfig_BUG064001_OpenKnowledgeBudgetsAllowOperation
    openknowledge_shipped_budget_bug064001_test.go:74: DEFECT A: assistant.open_knowledge.per_user_monthly_budget_usd = 0; must be > 0 when enabled (a 0 budget makes the agent refuse every /ask via the cap_usd pre-flight gate)
--- FAIL: TestShippedConfig_BUG064001_OpenKnowledgeBudgetsAllowOperation (0.05s)
FAIL    github.com/smackerel/smackerel/internal/config  0.093s
=== NAME  TestHandleUpdate_BUG064001_CaptureStripsAskPrefix
    capture_prefix_bug064001_test.go:66: DEFECT B regression: captured text still carries the /ask prefix: "/ask tide schedule for 06/11 in wa-town-A, wa"
--- FAIL: TestHandleUpdate_BUG064001_CaptureStripsAskPrefix (0.00s)
    capture_prefix_bug064001_test.go:127: /ask: captured text = "/ask tide schedule for wa-town-A"; want "tide schedule for wa-town-A" (prefix stripped)
    capture_prefix_bug064001_test.go:127: /weather: captured text = "/weather in wa-town-A wa tomorrow"; want "in wa-town-A wa tomorrow" (prefix stripped)
    capture_prefix_bug064001_test.go:127: /remind: ... /recipe: ... /cook: ... (all FAIL)
--- FAIL: TestHandleUpdate_BUG064001_AllV1ShortcutsStripped (0.00s)
FAIL    github.com/smackerel/smackerel/internal/telegram/assistant_adapter  0.175s
```

The unit-level `ZeroPerUserBudget` agent test reproduces the EXACT live self-hosted
`openknowledge.turn` failure (`termination_reason=cap_usd, iterations=1,
tokens_used=0, tool_calls=[]`). The config-contract + adapter tests are RED.

## SCOPE-01 — DEFECT A (budget)

### Config change {#scope-01-config}

`config/smackerel.yaml` `assistant.open_knowledge`: `monthly_budget_usd: 0 → 100`,
`per_user_monthly_budget_usd: 0 → 25` (with comments explaining local zero-cost
`CostFn` inertness + paid-provider ceiling intent; `enabled: false` is the
disable switch, not a 0 budget). `self-hosted` env block has NO budget override → it
inherits the fixed base 100/25.

`./smackerel.sh config generate` then verifies propagation:

```
config-validate: /workspace/config/generated/dev.env.tmp.64929 OK
Generated /home/.../config/generated/dev.env
=== generated open_knowledge budget env ===
config/generated/dev.env:532:ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD=100
config/generated/dev.env:533:ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD=25
```

### Adversarial + preservation unit tests {#scope-01-unit}

`internal/assistant/openknowledge/agent/budget_preflight_bug064001_test.go` —
GREEN after fix:

```
=== RUN   TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight
INFO openknowledge.turn ... iterations=2 tokens_used=30 usd_spent=0 status=success
  termination_reason=final num_sources=1 tool_calls="[map[name:calculator outcome:success]]"
--- PASS: TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight (0.00s)
=== RUN   TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight
INFO openknowledge.turn ... iterations=1 tokens_used=0 status=refused termination_reason=cap_usd
  refusal_reason="openknowledge: per-user monthly USD budget exceeded"
--- PASS: TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.043s
```

Positive budget ⇒ agent grounds an answer (proceeds past pre-flight). Zero budget
⇒ still refuses `cap_usd` (SCN-064-A08 gate semantics preserved — paid-provider
safety intact).

### Config-contract test {#scope-01-config-test}

`internal/config/openknowledge_shipped_budget_bug064001_test.go` parses the
shipped `config/smackerel.yaml` and asserts both monthly budgets `> 0` when
enabled. RED before fix (per_user=0), GREEN after:

```
=== RUN   TestShippedConfig_BUG064001_OpenKnowledgeBudgetsAllowOperation
--- PASS: TestShippedConfig_BUG064001_OpenKnowledgeBudgetsAllowOperation (0.02s)
ok      github.com/smackerel/smackerel/internal/config  0.051s
```

### Before/After reproduction {#scope-01-repro}

- Before: `TestShippedConfig_...` FAIL (`per_user_monthly_budget_usd = 0`) + agent
  `ZeroPerUserBudget` reproduces the live `cap_usd` refusal (see {#repro-red}).
- After: config-contract PASS + agent `PositivePerUserBudget` proceeds to a
  grounded `status=success` (see {#scope-01-unit}).
- `tests/e2e/openknowledge/open_knowledge_e2e_test.go` SCN-064-A08 still passes
  because it sets its OWN `cfg.PerUserMonthlyUSDRemaining = 0` (not the env), so
  the gate refusal contract is unaffected by the base-config change.

### Build Quality Gate {#scope-01-build}

See {#build-gate} (shared by both scopes).

---

## SCOPE-02 — DEFECT B (capture prefix)

### Implementation {#scope-02-impl}

`internal/telegram/assistant_adapter/adapter.go::HandleUpdate` —
`a.capture(ctx, update.Message, msg.Text)` → `a.capture(ctx, update.Message,
assistant.StripShortcutPrefix(msg.Text))`. Added the `internal/assistant` import;
renamed the local `assistant` var to `facade` so it no longer shadows the
imported package. `StripShortcutPrefix` is a no-op for non-shortcut text.

### Adversarial + verbatim unit tests {#scope-02-unit}

`internal/telegram/assistant_adapter/capture_prefix_bug064001_test.go` — GREEN
after fix:

```
--- PASS: TestHandleUpdate_BUG064001_CaptureStripsAskPrefix (0.00s)
--- PASS: TestHandleUpdate_BUG064001_NonShortcutCapturedVerbatim (0.00s)
--- PASS: TestHandleUpdate_BUG064001_AllV1ShortcutsStripped (0.00s)
    --- PASS: TestHandleUpdate_BUG064001_AllV1ShortcutsStripped//ask (0.00s)
    --- PASS: TestHandleUpdate_BUG064001_AllV1ShortcutsStripped//cook (0.00s)
    --- PASS: TestHandleUpdate_BUG064001_AllV1ShortcutsStripped//recipe (0.00s)
    --- PASS: TestHandleUpdate_BUG064001_AllV1ShortcutsStripped//remind (0.00s)
    --- PASS: TestHandleUpdate_BUG064001_AllV1ShortcutsStripped//weather (0.00s)
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter  0.092s
```

Adversarial: `CaptureStripsAskPrefix` FAILS if the `/ask` prefix is reintroduced.
Verbatim guard: non-shortcut text captured unchanged (FR-2a). All 5 v1 shortcuts
covered.

### Before/After reproduction {#scope-02-repro}

- Before: `CaptureStripsAskPrefix` FAIL (`"/ask tide schedule …"`), all 5 shortcut
  subtests FAIL (see {#repro-red}).
- After: all PASS; captured text is the stripped tail.

### Build Quality Gate {#scope-02-build}

See {#build-gate}.

---

## Build Quality Gate (shared) {#build-gate}

- Full Go unit suite `./smackerel.sh test unit --go` — all affected packages
  pass: `internal/telegram/assistant_adapter ok 0.029s`,
  `internal/assistant/openknowledge/agent ok 0.026s`, `internal/config ok 20.359s`.
  The whole module compiles + vets (`go test ./...` runs `go vet`).
- gofmt — `go-format.sh --check` (repo's isolated `golang:1.25.10-bookworm`
  container, the exact go-tooling step of `./smackerel.sh format`): **exit 0**
  (clean).
- `go vet ./...` (`go-lint.sh`) — covered by the passing `go test ./...` run.
- get_errors on all changed/new files — no compile errors.

### Environmental limitations (NOT code defects, unrelated to BUG-064-001)

- `./smackerel.sh format --check` / `lint` exit non-zero ONLY at their Python
  (`ml/`) stage: `pip install` of the editable `smackerel-ml` package times out
  against pypi (`HTTPSConnectionPool(host='pypi.org'…) Read timed out` →
  `ResolutionImpossible: httpcore`). This is a sandbox network constraint. No
  Python code was changed by this bug. The Go tooling (`run_go_tooling`, isolated
  golang container) runs FIRST and independently — proven clean above.
- `./smackerel.sh test unit --go` reports 2 FAILs in `tests/unit/clients`
  (`TestRenderDescriptorV1_CrossLanguageCanary`,
  `TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun`): both `t.Fatalf`
  with `"node"/"dart" not on PATH`. These are pre-existing, environmental
  (spec-073 cross-language renderer canary requires node+dart toolchains absent
  from the go-unit container), deterministic regardless of this change, and
  unrelated to the budget/capture fix.

---

## Routed handoffs (separate owners)

- **HANDOFF-1 (e2e test staleness):** `tests/e2e/assistant_bs007_test.sh`
  asserts `/ask` dispatches to `retrieval_qa` ("bypasses embedding-based routing
  and dispatches directly to retrieval_qa"). Since commit `ebdbf852` (2026-06-01)
  `/ask → open_knowledge`, so that assertion is stale. The fixture is tier-gated
  (`skip_unless_accel_tier "BS-007"`), so it is skipped on CPU dev loops. This is
  a cross-spec finding, distinct from this budget+capture fix. Owner:
  `bubbles.plan` / `bubbles.test`.
- **HANDOFF-2 (canary skip-hygiene):** the spec-073
  `TestRenderDescriptorV1_*` canaries `t.Fatalf` on missing node/dart instead of
  `t.Skip`, so they hard-fail the unit suite in minimal containers. Owner:
  `bubbles.test`.
- **HANDOFF-3 (deployment — owner `bubbles.devops`):** the live self-hosted symptom
  is NOT cleared by code alone. It requires a rebuild of `smackerel-core` from the
  fixed SHA + per-env config-bundle regen (so `self-hosted.env` carries 100/25) +
  redeploy via the knb adapter. The deployed `sourceSha 0bc04cfb` ships the `0`
  budget, so a redeploy of the FIXED sha is mandatory to clear it.
- **HANDOFF-4 (full live certification — owners `bubbles.validate` / `bubbles.audit`):**
  the bugfix-fastlane `done`-grade gates (scenario-manifest, validate certification,
  live openknowledge E2E on the real Telegram+searxng+GPU stack, audit pass) cannot
  run in this sandbox (no self-hosted GPU stack; pypi/node/dart toolchains absent).
  They are appropriate to run on the real stack alongside the redeploy.

---

## DevOps Live Self-Hosted Re-Verify — 2026-07-20 (evidence only; NOT a promotion)

Recorded by `bubbles.devops`. Bug `status` UNCHANGED (`blocked`). Live-stack evidence
only.

**Target:** self-hosted `<deploy-host>`; deployed core rev `a7ce6834fddb` (a git ancestor of
HEAD `a8a64525`; far newer than the `0bc04cfb` that shipped the `0` budget). A live
`POST /v1/agent/invoke` `open_knowledge` turn returned:

```
status: success   termination: final   (NOT a "cap_usd" pre-flight refusal)
```

**DEFECT A (budget refuse-all) — cleared live.** The open-knowledge agent answered
instead of refusing at the per-user-monthly USD pre-flight gate, confirming the
`monthly_budget_usd 100 / per_user_monthly_budget_usd 25` fix is live on the deployed
rev. **DEFECT B (Telegram `/ask` capture-prefix strip)** is a Telegram-adapter path
and is NOT exercised by an HTTP `/v1/agent/invoke` turn; it stays validated in-repo
(`capture_prefix_bug064001_test.go`) and present in the deployed rev, but a live
Telegram `/ask` capture-title check was not run this session.

**Promotion NOT performed — not due to DEFECT-A evidence.** `state-transition-guard.sh`
(2026-07-20, HEAD `a8a64525`) exits 1 with 14 failures that are structural gaps owned
by other specialists: **G028 implementation-reality-scan flags 5 STUB/FAKE-DATA source
violations**, G056 (state.json has no `certification` block), G057 (no
`scenario-manifest.json`), G022 (`audit` phase not recorded), G068 (3 Gherkin
scenarios without faithful DoD items), and E2E-regression DoD rows. `bubbles.devops`
did not fabricate a certification block or edit source/scopes. Route: `bubbles.implement`
(G028 source violations), `bubbles.validate` (certification block), `bubbles.plan`
(scenario-manifest + DoD), then re-drive the guard; plus a live Telegram `/ask` check
for DEFECT B.

---

## In-session G028 disposition & honest-HOLD — 2026-07-20 (bubbles.iterate parent-expand)

Recorded by `bubbles.iterate`. Bug `status` UNCHANGED (`blocked`). This refines the
prior devops note's G028 line.

**Hermetic re-verification this session (real terminal output):**

```text
$ ./smackerel.sh test unit --go --go-run "BUG064001" --verbose
--- PASS: TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight (0.00s)
--- PASS: TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent   0.027s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter   0.010s
Result: BUG064001 suite passed, 0 failed | BUG001_TEST_EXIT=0
```

**G028 is 5 FALSE POSITIVES, not stub/fake data — owner is the FRAMEWORK, not `bubbles.implement`.**
The prior devops note called them "STUB/FAKE-DATA source violations." Investigation this
session (git-blame + the tracing contract) proves otherwise: all 5 hits are on
`internal/telegram/assistant_adapter/adapter.go` lines 147/152/154/214/338, all from
commit `2886d516e` (2026-05-29, spec-061) — pre-existing OTel telemetry, NOT this bug:

- Lines 147/152/154: the standard OTel **no-op tracer fallback**
  (`tracing.NewTracer(Config{Enabled:false})`); production always injects the real
  tracer. Telemetry infrastructure, not fake business data.
- Lines 214/338: the closed-vocabulary span-status literal `"noop"`, **contract-defined**
  in `internal/assistant/tracing/tracer.go:23` (status vocab `"ok" | "error" | "noop"`)
  and emitted identically at `facade.go:471` / `facade.go:1752`. The value CANNOT change
  without breaking the telemetry contract.

The reality-scan's `INTEGRATION_SUSPICIOUS_PATTERNS` matches the substring `noop`
case-insensitively; the scanner is a framework-managed immutable file with NO allowlist,
inline-suppression, or `bubbles-project.yaml` knob. Editing `adapter.go` cannot clear
G028 (214/338 are contract-locked), and de-referencing the genuinely-changed `adapter.go`
from the reality-scan file list would be dishonest scanner-gaming (rejected per
anti-fabrication). **Correct owner: the Bubbles framework** — refine
`implementation-reality-scan.sh` so contract telemetry-status literals and no-op tracer
fallbacks are not flagged `FAKE_INTEGRATION`.

**Honest-HOLD conclusion.** The DEFECT-A + DEFECT-B code+config fix is COMPLETE,
hermetically GREEN, and DEFECT-A is live-proven (the sibling BUG-064-002 promoted to
`done` on the same live `/ask` A/B). The bug is HELD on the single irreducible G028
framework-scanner false positive (secondary: the G068 single-file two-scope
DoD-extraction layout limitation, Check-22 awk). Neither is solvable by editing this
bug's source without gaming the scanner. Once the framework G028 refinement lands, add
the certification block + `scenario-manifest.json` and re-drive the guard to `done`.

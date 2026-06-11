# BUG-064-001 — Root cause analysis & fix design

## 1. Investigation method

All leads in `bug.md` were verified against the source tree AND the live home-lab
deployment (read-only `tailscale ssh evo-x2 -- docker …`). Nothing below is
assumed.

## 2. Routing is correct — the lead was wrong

The bug report's lead said `/ask → retrieval_qa` (notes-only). That is true of the
**capability-metadata** file `config/assistant/scenarios.yaml`
(`retrieval_qa.slash_shortcut: "/ask"`), which is used for menu/label rendering —
NOT for routing.

The **actual routing** is the Go map in `internal/assistant/shortcuts.go`:

```go
var SlashShortcuts = map[string]string{
    // Spec 064 SCOPE-17 — /ask reroutes to open_knowledge …
    "/ask":     "open_knowledge",
    ...
}
```

`git` confirms this landed in commit `ebdbf852` (2026-06-01), BEFORE spec 064 was
certified (2026-06-06). The facade (`internal/assistant/facade.go`) calls
`LookupShortcut(msg.Text)` → `shortcutScenarioID = "open_knowledge"` → explicit-id
fast path → `BandHigh` → `open_knowledge` executor.

**Deployed proof:** the running core container was built from `sourceSha 0bc04cfb`
(on-host `manifest.yaml`, applied `2026-06-11T06:03:09Z`). `git merge-base
--is-ancestor ebdbf852 0bc04cfb` = YES — the deployed image CONTAINS the routing
fix. The deployed `assistant_turn` log shows `scenario_id="open_knowledge"`,
`band="high"`. **Routing is correct; this is not deployment lag.**

## 3. DEFECT A — true root cause: pre-flight USD budget gate refuses everything

The open-knowledge agent has a pre-flight budget gate
(`internal/assistant/openknowledge/agent/agent.go`):

```go
// Spec 076 SCOPE-2b — SCN-064-A08 per-user monthly budget pre-flight.
// When the user has zero monthly USD remaining, refuse BEFORE the first
// LLM round and BEFORE any tool dispatches.
if a.cfg.PerUserMonthlyUSDRemaining <= 0 {
    return refuse(TerminationCapUSD, ok.ErrCapUSDPerUserMonth.Error()), nil
}
```

`PerUserMonthlyUSDRemaining` is sourced from SST
`assistant.open_knowledge.per_user_monthly_budget_usd`, which
`config/smackerel.yaml` sets to **`0`**:

```yaml
open_knowledge:
  enabled: true
  ...
  monthly_budget_usd: 0            # ← REFUSE-ALL under the pre-flight gate
  per_user_monthly_budget_usd: 0   # ← REFUSE-ALL under the pre-flight gate
```

The production `CostFn` is a **zero-cost stub**
(`cmd/core/wiring_assistant_openknowledge.go`):

```go
// 8. CostFn — zero-cost stub. Token + iteration caps still bind.
costFn := okagent.CostFn(func(int) float64 { return 0 })
```

So `usdSpent` is permanently `$0`; the running caps in `budget.go::checkCaps()`
(`usdSpent > remaining`) can never fire even at `remaining = 0` (`0 > 0` is false).
The **only** consumer that bites is the pre-flight gate, and it bites purely
because the configured per-user-monthly cap is `0`. Result: **every** `/ask`
refuses with `cap_usd` before any LLM/tool work.

**Live proof** (`openknowledge.turn` log, deployed core):
`iterations:1, tokens_used:0, tool_calls:[], status:"refused",
termination_reason:"cap_usd",
refusal_reason:"openknowledge: per-user monthly USD budget exceeded"`.

Current `main`'s `config/smackerel.yaml` still has `0` for both monthly budgets,
so a redeploy of `main` as-is would NOT fix this. It is a genuine code+config bug.

### Why the live message had no `(saved as idea)` suffix

The pre-flight refusal is mapped to `RefusalBudgetExhausted` /
`contracts` provenance refusal; `resp.ErrorCause` does not match the spec-064
open-knowledge `RefusalCause` set, so `buildTelegramRendering` falls through to
the DEFAULT renderer and emits the plain `CanonicalRefusalBody` ("I don't have a
sourced answer for that.") rather than `RenderRefusalWithCapture` (which would
append "(saved as idea)"). The capture ack came separately from the adapter
`CaptureRoute` hook (DEFECT B). Both messages are explained without invoking a
second bug.

## 4. DEFECT B — true root cause: slash prefix leaks into the captured idea

`translateInbound` (`internal/telegram/assistant_adapter/translate_inbound.go`)
forwards a `/ask …` message as `KindText` with the slash **preserved** so the
facade's `LookupShortcut` can pin the scenario:

```go
// Forward them as KindText with the slash preserved; the facade is the
// single source of truth for the shortcut set.
```

`HandleUpdate` (`adapter.go`) then dispatches capture with the verbatim text:

```go
if resp.CaptureRoute && update.Message != nil {
    a.capture(ctx, update.Message, msg.Text)   // msg.Text == "/ask tide …"
}
```

`a.capture` is `telegram.NewBotCaptureFn` → `Bot.handleTextCapture(ctx, msg, text)`,
which derives the idea title from `text`. So the title keeps the `/ask ` prefix.

Production uses only this adapter `CaptureRoute` path: `WithCaptureFallbackPolicy`
and `WithIntentCompiler` are wired **only in tests** (verified by grep), so the
facade's own `runCaptureFallback` is nil in production. The fix therefore targets
the adapter dispatch.

The facade already has the right helper — `assistant.StripShortcutPrefix(text)` —
used for the executor payload (`facade.go`). The capture path simply never uses
it.

## 5. Fix design

### 5.1 DEFECT A — SST budget correction (recommended) vs. code sentinel (deferred)

**Decision required** (per bug-fix mandate): which way to express "this local,
zero-cost deployment must not be USD-gated"?

| Option | What | Verdict |
|--------|------|---------|
| **A — SST positive ceiling (CHOSEN)** | Set `monthly_budget_usd` / `per_user_monthly_budget_usd` to positive ceilings in `config/smackerel.yaml`. Local `CostFn=0` so they never bind; they exist as real ceilings if a paid provider + real `CostFn` is later wired. | **Chosen.** Minimal blast radius, no budget-contract change, preserves SCN-064-A08 gate semantics, NO-DEFAULTS-compliant (explicit SST values). The dedicated off-switch is already `enabled: false`, so `0` was simply a wrong placeholder, not an intentional "disable". |
| B — code "unlimited" sentinel (`-1`) | Treat negative remaining as "no USD cap"; set config to `-1`. | **Deferred.** Cleaner long-term semantics but touches `agent.go` validation + gate, `budget.go` validation + `checkCaps`, the config loader, and risks the spec 076 SCN-064-A08 contract tests. Out of proportion for a focused bug fix. Recorded here as the future-hardening option. |

Chosen values (both `> 0`, generous so they never bind for the local zero-cost
default; meaningful as a paid-provider ceiling):

```yaml
monthly_budget_usd: 100           # global monthly USD ceiling
per_user_monthly_budget_usd: 25   # per-user monthly USD ceiling
```

Rationale for the split: a per-user cap below the global cap is the conventional
shape (one user cannot consume the whole global budget). For the default local
deployment both are inert (`CostFn=0`); for a future paid provider they are a real
guardrail.

### 5.2 DEFECT B — strip the shortcut prefix at the adapter capture dispatch

In `internal/telegram/assistant_adapter/adapter.go::HandleUpdate`, strip the v1
shortcut prefix before handing the text to the capture hook:

```go
if resp.CaptureRoute && update.Message != nil {
    a.capture(ctx, update.Message, assistant.StripShortcutPrefix(msg.Text))
}
```

`assistant.StripShortcutPrefix` is pure, already exists, only strips the closed v1
shortcut set, and returns non-shortcut text verbatim (so plain captures are
unaffected — FR-2a). The adapter package already imports
`internal/assistant` (used by `translate_inbound.go`).

Bare-shortcut edge (`/ask` with no body): `StripShortcutPrefix("/ask")` → `""`.
`handleTextCapture` already truncates/handles text; an empty body is a benign
no-content capture and is not the reported failure mode. No extra guard added to
keep the change minimal.

## 6. Files to change

| File | Change | Defect |
|------|--------|--------|
| `config/smackerel.yaml` | `monthly_budget_usd: 0 → 100`, `per_user_monthly_budget_usd: 0 → 25` (+ comments) | A |
| `internal/telegram/assistant_adapter/adapter.go` | wrap capture text in `assistant.StripShortcutPrefix(...)` (+ import if needed) | B |
| `internal/assistant/openknowledge/agent/agent_test.go` (or sibling) | adversarial test: positive per-user budget ⇒ agent proceeds past pre-flight (no `cap_usd` with 0 iterations) | A |
| `internal/config/*_test.go` | contract test: shipped open-knowledge monthly budgets `> 0` when enabled | A |
| `internal/telegram/assistant_adapter/adapter_test.go` (or sibling) | adversarial test: `/ask <q>` CaptureRoute ⇒ captured text has no `/ask` prefix; plain text captured verbatim | B |

## 7. Deployment note

Both fixes are in-repo (code + SST). The **live** home-lab symptom will only clear
after a rebuild + config-bundle regen + redeploy through the knb adapter
(`bubbles.devops`). Code+config alone fixes the repo; it does not mutate the
running container. This is recorded as the next-owner handoff.

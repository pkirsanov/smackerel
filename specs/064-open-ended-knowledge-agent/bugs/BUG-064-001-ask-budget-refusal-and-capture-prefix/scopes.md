# BUG-064-001 — Scopes

Status: done

Two independent defects → two scopes. SCOPE-01 (budget) and SCOPE-02 (capture
prefix) have no ordering dependency and may be implemented in either order.

---

## SCOPE-01 — DEFECT A: `/ask` open-knowledge agent must not refuse on the default zero-cost USD budget

**Status:** Done

**Depends on:** none

### Gherkin scenarios

```gherkin
Scenario: SCN-064-001-A01 — enabled local agent proceeds past the USD pre-flight gate
  Given the open-knowledge agent is constructed with a zero-cost CostFn (local Ollama + searxng)
    And per_user_monthly_budget_usd is the shipped positive ceiling (> 0)
  When a user asks an open-ended question
  Then the agent calls the LLM and attempts to ground the answer
   And it does NOT terminate with cap_usd having done 0 iterations and 0 tool calls

Scenario: SCN-064-001-A02 — shipped SST budget is operable
  Given config/smackerel.yaml with assistant.open_knowledge.enabled = true
  When the open-knowledge config is loaded
  Then monthly_budget_usd > 0 and per_user_monthly_budget_usd > 0

Scenario: SCN-064-001-A03 — paid-provider exhaustion still refuses (gate preserved)
  Given a positive per-user monthly budget that has been fully spent (remaining = 0)
  When a user asks a question
  Then the agent refuses pre-flight with cap_usd (SCN-064-A08 semantics intact)
```

### Implementation plan

- `config/smackerel.yaml`: set `assistant.open_knowledge.monthly_budget_usd: 100`
  and `per_user_monthly_budget_usd: 25` with comments explaining local `CostFn=0`
  inertness + paid-provider ceiling intent. Regenerate config
  (`./smackerel.sh config generate`).
- Do NOT modify the `agent.go` pre-flight gate or `budget.go` — the gate is
  correct for genuine exhaustion (SCN-064-A03 must still pass).

### Test Plan

| Test Type | Category | File/Location | Description | Command |
|-----------|----------|---------------|-------------|---------|
| Unit (adversarial) | `unit` | `internal/assistant/openknowledge/agent/budget_preflight_bug064001_test.go` | positive per-user budget ⇒ `Run` proceeds past pre-flight (no `cap_usd`, iterations>0); fails if budget is 0 | `./smackerel.sh test unit --go --go-run BUG064001` |
| Unit (gate preserved) | `unit` | `internal/assistant/openknowledge/agent/budget_preflight_bug064001_test.go` | exhausted/zero per-user budget ⇒ still refuses `cap_usd` pre-flight (SCN-064-A08) | `./smackerel.sh test unit --go --go-run BUG064001` |
| Config contract | `unit` | `internal/config/openknowledge_shipped_budget_bug064001_test.go` | shipped `config/smackerel.yaml` open-knowledge monthly budgets `> 0` when enabled | `./smackerel.sh test unit --go --go-run BUG064001` |
| Build Quality Gate | `unit` | n/a | `go test ./...` (vet+compile) + gofmt clean | `./smackerel.sh test unit --go` |

### Definition of Done

- [x] (SCN-064-001-A02) `config/smackerel.yaml` open-knowledge monthly budgets are positive ceilings (100 / 25) — the shipped SST budget is operable when enabled — with explanatory comments → Evidence: [report.md#scope-01-config]
- [x] Config regenerated; generated `dev.env` reflects positive budgets (100 / 25) → Evidence: [report.md#scope-01-config]
- [x] (SCN-064-001-A01) Adversarial unit test: an enabled local (zero-cost `CostFn`) agent PROCEEDS past the USD pre-flight gate and grounds an answer (does NOT refuse `cap_usd`); fails if the budget is reset to 0 → Evidence: [report.md#scope-01-unit]
- [x] (SCN-064-001-A03) Preservation unit test: a genuinely-exhausted (zero) per-user budget STILL refuses `cap_usd` pre-flight (SCN-064-A08 paid-provider gate intact) → Evidence: [report.md#scope-01-unit]
- [x] (SCN-064-001-A02) Config-contract test: shipped open-knowledge monthly budgets `> 0` when enabled → Evidence: [report.md#scope-01-config-test]
- [x] Bug reproduced (RED) BEFORE fix and verified (GREEN) AFTER fix, same session, in the isolated go-unit container → Evidence: [report.md#repro-red], [report.md#scope-01-unit]
- [x] Build Quality Gate: `go test ./...` (vet+compile) + gofmt clean; Python lint/format env-blocked (no Python changed) → Evidence: [report.md#build-gate]

---

## SCOPE-02 — DEFECT B: captured idea must not contain the `/ask` slash prefix

**Status:** Done

**Depends on:** none

### Gherkin scenarios

```gherkin
Scenario: SCN-064-001-B01 — /ask capture strips the prefix
  Given the Telegram assistant adapter is bound
    And the facade returns CaptureRoute = true for a "/ask tide schedule …" turn
  When the adapter dispatches capture-as-fallback
  Then the captured text is "tide schedule …" with no leading "/ask"

Scenario: SCN-064-001-B02 — every v1 shortcut prefix is stripped
  Given inbound text begins with /ask, /weather, /remind, /recipe, or /cook
  When that turn is captured as an idea
  Then the stored idea text contains no leading slash-command token

Scenario: SCN-064-001-B03 — plain text captured verbatim
  Given inbound text that is NOT a v1 shortcut (e.g. "buy milk")
  When that turn is captured as an idea
  Then the stored idea text equals the original text verbatim
```

### Implementation plan

- `internal/telegram/assistant_adapter/adapter.go::HandleUpdate`: change the
  `CaptureRoute` dispatch to
  `a.capture(ctx, update.Message, assistant.StripShortcutPrefix(msg.Text))`
  (add the `internal/assistant` import if absent).

### Test Plan

| Test Type | Category | File/Location | Description | Command |
|-----------|----------|---------------|-------------|---------|
| Unit (adversarial) | `unit` | `internal/telegram/assistant_adapter/capture_prefix_bug064001_test.go` | `CaptureRoute=true` for a `/ask …` update ⇒ CaptureFn receives text WITHOUT `/ask`; fails if prefix leaks | `./smackerel.sh test unit --go --go-run BUG064001` |
| Unit (all v1 shortcuts) | `unit` | `internal/telegram/assistant_adapter/capture_prefix_bug064001_test.go` | `/ask /weather /remind /recipe /cook` prefixes all stripped from captured text | `./smackerel.sh test unit --go --go-run BUG064001` |
| Unit (verbatim) | `unit` | `internal/telegram/assistant_adapter/capture_prefix_bug064001_test.go` | non-shortcut text ⇒ CaptureFn receives verbatim text (FR-2a) | `./smackerel.sh test unit --go --go-run BUG064001` |
| Build Quality Gate | `unit` | n/a | `go test ./...` (vet+compile) + gofmt clean | `./smackerel.sh test unit --go` |

### Definition of Done

- [x] `adapter.go` CaptureRoute dispatch strips the v1 shortcut prefix via `assistant.StripShortcutPrefix` (local `assistant` var renamed to `facade` to avoid shadowing the import) → Evidence: [report.md#scope-02-impl]
- [x] (SCN-064-001-B01) `/ask` capture strips the prefix — adversarial unit test proves a `/ask …` CaptureRoute turn stores the tail WITHOUT the `/ask` prefix (fails if reverted) → Evidence: [report.md#scope-02-unit]
- [x] (SCN-064-001-B03) Plain text captured verbatim — non-shortcut text is captured verbatim (unchanged) → Evidence: [report.md#scope-02-unit]
- [x] (SCN-064-001-B02) Every v1 shortcut prefix is stripped (`/ask /weather /remind /recipe /cook`) from the captured idea → Evidence: [report.md#scope-02-unit]
- [x] Bug reproduced (RED) BEFORE fix and verified (GREEN) AFTER fix → Evidence: [report.md#repro-red], [report.md#scope-02-unit]
- [x] Build Quality Gate: `go test ./...` (vet+compile) + gofmt clean; Python lint/format env-blocked (no Python changed) → Evidence: [report.md#build-gate]

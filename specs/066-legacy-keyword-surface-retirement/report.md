# Report — Spec 066 Legacy Keyword Surface Retirement

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning packet created by `bubbles.plan` on 2026-05-31 for the product-to-planning pass. This report is a scaffold for execution evidence only; no implementation, source tests, config generation, or runtime verification was performed by this planning pass.

## Planning Evidence

- Scope plan created in [scopes.md](scopes.md).
- Scenario contracts created in [scenario-manifest.json](scenario-manifest.json).
- Structured test handoff created in [test-plan.json](test-plan.json).
- User validation baseline created in [uservalidation.md](uservalidation.md).

## Test Evidence

No test evidence is recorded here by `bubbles.plan`. Execution agents must append raw terminal output with `**Phase:**`, `**Command:**`, `**Exit Code:**`, and `**Claim Source:**` fields when they run the planned checks.

## Completion Statement

Planning artifacts are prepared for planning maturity review. Delivery is not claimed in this report.

---

## Scope 3 — Natural-Language Replacement Paths (Status: in_progress, workspace-blocked) {#scope-3}

**Phase:** implement  
**Agent:** bubbles.implement  
**Scope plan:** [scopes.md → Scope 3](scopes.md#scope-3-natural-language-replacement-paths)

### Change Summary

SCOPE-3 code edits landed (within Change Boundary):

| File | Change | Purpose |
|------|--------|---------|
| `internal/telegram/bot.go` | Removed `case "rate":` arm of the command dispatch switch | Retire the active call site for the legacy `/rate` slash command |
| `internal/telegram/annotation.go` | Removed `func (b *Bot) handleRate(...)` (replaced with a SCOPE-3 retirement note) | Delete the retired slash handler symbol; rating now flows through the assistant facade |
| `internal/telegram/annotation_test.go` | Removed `TestHandleRate_NoArgs` and `TestHandleRate_NoResults` | Drop unit tests of the deleted symbol; coverage shifts to the integration retirement-guarantee test |
| `tests/integration/assistant/legacy_replacement_test.go` (new) | Added `TestNaturalLanguageFindUsesRetrievalScenarioNotSlashHandler` | SCN-066-A02 retrieval-contract integration test — the spec-066-owned live retirement guarantee; combines grep + AST absence assertions with a facade-routing assertion |

Disambiguation helpers (`disambiguationStore`, `pendingDisambiguation`, `handleDisambiguationReply`, `splitRateArgs`, `isStrongMatch`) are intentionally retained — they remain in service for the reply-annotation flow and for the assistant facade's disambiguation prompt path. Removing them is out of SCOPE-3's Change Boundary.

### RED → GREEN Structural Proof

**Phase:** implement  
**Command:** `grep -n 'case "rate":\|b\.handleRate(\|func (b \*Bot) handleRate' internal/telegram/bot.go internal/telegram/annotation.go`  
**Claim Source:** executed

RED (pre-edit, captured this session):

```text
internal/telegram/bot.go:550:           case "rate":
internal/telegram/bot.go:551:                   b.handleRate(ctx, msg, msg.CommandArguments())
internal/telegram/annotation.go:113:func (b *Bot) handleRate(ctx context.Context, msg *tgbotapi.Message, args string) {
```

GREEN (post-edit):

```text
$ grep -n 'case "rate":\|b\.handleRate(\|func (b \*Bot) handleRate' internal/telegram/bot.go internal/telegram/annotation.go
(no matches)
$ echo $?
1
```

The grep returns exit 1 with no matches — the retired call site, dispatcher arm, and method declaration are all absent.

### Bounded Consumer Impact Sweep (Scope-3 in-scope file list)

**Phase:** implement  
**Command:** `grep -rn 'b\.handleRate(\|case "rate":' internal/telegram/`  
**Claim Source:** executed

**In-scope (allowed by Change Boundary):**

- `internal/telegram/bot.go` — `case "rate":` dispatch arm: 0 occurrences (was 1).
- `internal/telegram/bot.go` — `b.handleRate(` call sites: 0 occurrences (was 1).
- `internal/telegram/annotation.go` — `func (b *Bot) handleRate(` declarations: 0 occurrences (was 1).
- `internal/telegram/annotation_test.go` — `bot.handleRate(` references: 0 occurrences (was 2).

**Out-of-scope (explicitly Scope 5 territory):**

The following `/find` and `/rate` strings remain in `internal/telegram/` but live in surfaces that the Scope 3 Change Boundary defers to Scope 5 ("Scope 5 closes the consumer sweep by deleting any remaining stale references in docs/help/eval fixtures"):

- `internal/telegram/bot.go:593` — unknown-command help copy lists retired commands.
- `internal/telegram/bot.go:973-974` — `/help` text advertises `/find` and `/rate`.
- `internal/telegram/bot.go:719,725` — `/find` query-validation copy.
- `internal/telegram/help_test.go:46-52` — adversarial help-text guard that intentionally enumerates the retired tokens.
- `internal/telegram/bot_test.go:194-202` — `/find` command parser tests.

These are deliberately left for Scope 5 and are NOT a SCOPE-3 closure gap.

### Test Evidence

**Phase:** implement  
**Command:** `go vet -tags=integration ./tests/integration/assistant/`  
**Claim Source:** executed  
**Exit Code:** 1 — **BLOCKED-WORKSPACE**

```text
# github.com/smackerel/smackerel/internal/config
internal/config/assistant_frontend.go:92:18: undefined: splitCSV
internal/config/assistant_intent_trace.go:152:75: cfg.Assistant.IntentTrace undefined (type AssistantConfig has no field or method IntentTrace)
internal/config/assistant_tools.go:152:72: cfg.Assistant.Tools undefined (type AssistantConfig has no field or method Tools)
... (many)
# github.com/smackerel/smackerel/internal/assistant
internal/assistant/facade.go:265:5: f.captureFallbackPolicy undefined (type *Facade has no field or method captureFallbackPolicy)
internal/assistant/facade.go:287:8: undefined: capturefallback
internal/assistant/facade_intent_trace.go:39:4: f.intentTrace undefined (type *Facade has no field or method intentTrace)
... (many)
```

The workspace contains foreign uncommitted WIP (`git status` reports modifications to `cmd/core/services.go`, `internal/assistant/facade.go`, and untracked directories including `internal/assistant/capturefallback/`, `internal/assistant/legacyretirement/`, `internal/assistant/intenttrace/`, `internal/config/assistant_intent_trace.go`, etc.) that leaves the `internal/assistant` and `internal/config` packages structurally broken. This blocks compilation of any test target transitively dependent on either package — including the new SCN-066-A02 integration contract row.

The SCOPE-3 code edits themselves are syntactically and semantically self-consistent (the `internal/telegram` source compiles in isolation against the symbols it actually uses). The blocker is workspace state, NOT SCOPE-3 deliverables.

**Route:** Workspace unwedge routed to `bubbles.workflow` (the in-flight `internal/assistant/*` work belongs to other specs — `capturefallback/` to spec 074, `intenttrace/` to a spec 068 follow-up, etc.). Once the workspace is buildable, the SCN-066-A02 contract test should pass without further SCOPE-3 changes; the AST + grep assertions on `(*Bot).handleRate` and `case "rate":` are deterministic from the committed source.

### Status

- DoD item "handleRate retired + integration contract test exists" — code complete, structural proof captured.
- DoD item "Consumer Impact Sweep (Scope-3 subset)" — closed.
- All live-run DoD items — Uncertainty Declaration with `Claim Source: not-run`, blocked on the workspace WIP unwedge. See scopes.md `Scope 3 → Definition of Done` for the per-item declarations.


# BUG-064-001 — User Validation

> Items are checked `[x]` when the fix is validated. Uncheck `[ ]` to report that
> a behavior is still broken. The **live** items require the `bubbles.devops`
> redeploy (see `state.json.blockedReason`) before they can be confirmed on the
> home-lab bot — they are checked here against the in-repo code+test validation
> and re-confirmed live after deploy.

## Checklist

### DEFECT A — open-ended `/ask` is answerable

- [x] `/ask` routes to the open-knowledge agent (web search), not notes-only retrieval — verified: deployed `assistant_turn` log shows `scenario_id="open_knowledge"`.
- [x] An enabled open-knowledge agent on a zero-cost (local) deployment does NOT refuse every query at the per-user-monthly USD pre-flight gate — verified by unit test `TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight`.
- [x] The shipped SST sets positive open-knowledge monthly budgets when enabled — verified by `TestShippedConfig_BUG064001_OpenKnowledgeBudgetsAllowOperation`.
- [ ] LIVE: on the home-lab bot, `/ask tide schedule for 06/11 in wa-town-A, wa` returns a sourced answer instead of "I don't have a sourced answer for that." — **pending the bubbles.devops redeploy** of the fixed SHA + regenerated home-lab config bundle.

### DEFECT B — captured idea has no slash-command prefix

- [x] A `/ask …` turn routed to capture stores the natural-language tail WITHOUT the `/ask` prefix — verified by `TestHandleUpdate_BUG064001_CaptureStripsAskPrefix`.
- [x] All v1 shortcuts (`/ask /weather /remind /recipe /cook`) have their prefix stripped from captured ideas — verified by `TestHandleUpdate_BUG064001_AllV1ShortcutsStripped`.
- [x] Plain (non-shortcut) text is still captured verbatim — verified by `TestHandleUpdate_BUG064001_NonShortcutCapturedVerbatim`.
- [ ] LIVE: on the home-lab bot, a captured idea title never contains `/ask` — **pending the bubbles.devops redeploy**.

# User Validation — Spec 061 Conversational Assistant (Transport-Agnostic)

> Items default to checked `[x]` once validated. Owner unchecks `[ ]`
> to report regressions.

- [ ] Capture-as-fallback path is preserved on every enabled transport (plain notes still become `idea` artifacts).
- [ ] Retrieval Q&A returns sourced answers with artifact-ID citations.
- [ ] Weather skill returns provider-attributed answers with retrieval timestamp.
- [ ] Notification skill requires explicit confirmation before scheduling.
- [ ] Ambiguous intent falls back to capture; no skill is invoked.
- [ ] No answer is sent without provenance (BS-007 holds in production).
- [ ] Missing `assistant.*` SST keys abort startup with a clear error message.
- [x] Owner has ratified the Principle 1 deviation (additive assistant on top of preserved capture). — Ratified 2026-05-28.
- [x] v1 / v2 split (email moved to its own spec) is accepted. — Ratified 2026-05-28.
- [x] Owner ratifies transport-agnostic generalization (Telegram is v1 reference adapter; WhatsApp, web chat, mobile in-app are future adapters wired via the same `TransportAdapter` interface without capability-layer changes). — Ratified 2026-05-28.

## Per-Scope User-Visible Behavior (added by bubbles.plan)

- [ ] SCOPE-01 — Missing or malformed `assistant.*` SST config aborts core startup with a clear, named-key error; legacy `assistant.intent.*` keys still work with a warning.
- [ ] SCOPE-02 — Canonical message contracts are stable across releases; no adapter ever sees a transport-specific field leak from the capability layer.
- [ ] SCOPE-03 — A skill disabled in config is invisible to the bot; no answer is ever sent without source citations, even if a skill tries.
- [ ] SCOPE-04 — When the bot is unsure what I meant, it silently captures my message as an idea (zero risk of misrouted notes); slash commands (`/ask`, `/weather`, `/save`) work as fast paths.
- [ ] SCOPE-05 — On Telegram, plain notes still become `idea` artifacts exactly as before; `/reset` clears my conversation thread; the bot's reply uses native Telegram widgets (inline buttons, trailing sources block).
- [ ] SCOPE-06 — I can ask a question about something I previously captured and get an answer with citations back to the original artifacts.
- [ ] SCOPE-07 — I can ask "weather in <city>" and get a forecast with the provider name and retrieval timestamp; if the provider is down, the bot tells me and offers to capture my question.
- [ ] SCOPE-08 — I can ask the bot to remind me of something; it confirms with two buttons before scheduling; if I ignore the confirmation, my message is captured instead.
- [ ] SCOPE-09 — The operator can observe per-skill and per-transport metrics in Grafana, including capture-fallback drift and provenance violations.
- [ ] SCOPE-10 — The v1 evaluation suite passes the ≥85% routing accuracy and 100% capture-fallback success bar on a labeled corpus of ≥150 messages.

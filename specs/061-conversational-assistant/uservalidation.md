# User Validation — Spec 061 Conversational Assistant (Transport-Agnostic)

> Items default to checked `[x]` once validated. Owner unchecks `[ ]`
> to report regressions.

## Ratification Status

**Ratified by owner: 2026-05-30.** All 22 acceptance items below (11 top-level capability + 11 per-scope user-visible behaviors) are signed off. This ratification block closes SCOPE-10 DoD #6 and authorizes promotion of spec 061 to `status: done`.

- [x] Capture-as-fallback path is preserved on every enabled transport (plain notes still become `idea` artifacts). — Ratified 2026-05-30.
- [x] Retrieval Q&A returns sourced answers with artifact-ID citations. — Ratified 2026-05-30.
- [x] Weather skill returns provider-attributed answers with retrieval timestamp. — Ratified 2026-05-30.
- [x] Notification skill requires explicit confirmation before scheduling. — Ratified 2026-05-30.
- [x] Ambiguous intent falls back to capture; no skill is invoked. — Ratified 2026-05-30.
- [x] No answer is sent without provenance (BS-007 holds in production). — Ratified 2026-05-30.
- [x] Missing `assistant.*` SST keys abort startup with a clear error message. — Ratified 2026-05-30.
- [x] Owner has ratified the Principle 1 deviation (additive assistant on top of preserved capture). — Ratified 2026-05-28.
- [x] v1 / v2 split (email moved to its own spec) is accepted. — Ratified 2026-05-28.
- [x] Owner ratifies transport-agnostic generalization (Telegram is v1 reference adapter; WhatsApp, web chat, mobile in-app are future adapters wired via the same `TransportAdapter` interface without capability-layer changes). — Ratified 2026-05-28.
- [x] Owner authorizes mixed-commit boundaries on SCOPE-01's 4-file surface (`config/smackerel.yaml`, `internal/config/config.go`, `internal/config/validate_test.go`, `scripts/commands/config.sh`). Pre-existing in-flight work from spec 058 (Chrome Extension Bridge) and BUG-020-009 (per-call HTTP timeouts) on these same files MAY be commingled with SCOPE-01 commits; the owner accepts the resulting commit-boundary contamination as a known and authorized operating condition for this spec. SCOPE-01 implementation is NOT blocked on a clean working tree on these 4 files. — Authorized 2026-05-28.

## Per-Scope User-Visible Behavior (added by bubbles.plan)

- [x] SCOPE-01 — Missing or malformed `assistant.*` SST config aborts core startup with a clear, named-key error; legacy `assistant.intent.*` keys still work with a warning. — Ratified 2026-05-30.
- [x] SCOPE-02 — Canonical message contracts are stable across releases; no adapter ever sees a transport-specific field leak from the capability layer. — Ratified 2026-05-30.
- [x] SCOPE-03 — A skill disabled in config is invisible to the bot; no answer is ever sent without source citations, even if a skill tries. — Ratified 2026-05-30.
- [x] SCOPE-04 — When the bot is unsure what I meant, it silently captures my message as an idea (zero risk of misrouted notes); slash commands (`/ask`, `/weather`, `/save`) work as fast paths. — Ratified 2026-05-30.
- [x] SCOPE-05 — On Telegram, plain notes still become `idea` artifacts exactly as before; `/reset` clears my conversation thread; the bot's reply uses native Telegram widgets (inline buttons, trailing sources block). — Ratified 2026-05-30.
- [x] SCOPE-06 — I can ask a question about something I previously captured and get an answer with citations back to the original artifacts. — Ratified 2026-05-30.
- [x] SCOPE-07 — I can ask "weather in <city>" and get a forecast with the provider name and retrieval timestamp; if the provider is down, the bot tells me and offers to capture my question. — Ratified 2026-05-30.
- [x] SCOPE-08 — I can ask the bot to remind me of something; it confirms with two buttons before scheduling; if I ignore the confirmation, my message is captured instead. — Ratified 2026-05-30.
- [x] SCOPE-09 — The operator can observe per-skill and per-transport metrics in Grafana, including capture-fallback drift and provenance violations. — Ratified 2026-05-30.
- [x] SCOPE-10 — The v1 evaluation suite passes the ≥85% routing accuracy and 100% capture-fallback success bar on a labeled corpus of ≥150 messages. — Ratified 2026-05-30.

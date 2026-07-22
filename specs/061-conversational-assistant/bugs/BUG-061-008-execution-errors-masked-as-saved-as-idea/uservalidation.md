# BUG-061-008 — User Validation

> Items are checked `[x]` when validated. Uncheck `[ ]` to report a still-broken behavior.
> **LIVE** items require the home-lab redeploy before confirmation on the self-hosted bot.

## Checklist

### Execution errors surface honestly (systemic)

- [x] A provider/timeout failure on any requires_provenance scenario surfaces an honest "unavailable" error, never "saved as an idea" — verified by the cross-scenario invariant test (SCOPE-02).
- [x] A genuinely ungrounded answer (OK outcome, no sources) still refuses (anti-fabrication preserved) — verified by the unchanged gate tests.
- [x] Execution failures are observable via a scenario+outcome metric (SCOPE-03).
- [x] The invariant is mechanically enforced (the invariant test fails if the masking is reintroduced) and documented (SCOPE-04, SCOPE-05).
- [ ] LIVE: on the self-hosted bot, a failed request (e.g. a weather lookup while the provider is down) no longer replies "saved as an idea" — the P1–P5 fix (sourceSha `19fe72c8`) is **deployed + running + healthy** (running core+ml digests match the P1-P5 build; assistant adapter wired and bound), so the honest-error code path is live; **behavioral confirmation pending the operator's Telegram smoke test** (a human turn).

# BUG-061-008 — User Validation

> Items are checked `[x]` when validated. Uncheck `[ ]` to report a still-broken behavior.
> **LIVE** items require knb to redeploy to `<target>` on `<deploy-host>` before confirmation on the deployed bot.

## Checklist

### Execution errors surface honestly (systemic)

- [x] A provider/timeout failure on any requires_provenance scenario surfaces an honest "unavailable" error, never "saved as an idea" — verified by the cross-scenario invariant test (SCOPE-02).
- [x] A genuinely ungrounded answer (OK outcome, no sources) still refuses (anti-fabrication preserved) — verified by the unchanged gate tests.
- [x] Execution failures are observable via a scenario+outcome metric (SCOPE-03).
- [x] The invariant is mechanically enforced (the invariant test fails if the masking is reintroduced) and documented (SCOPE-04, SCOPE-05).
- [x] LIVE: on the bot deployed to `<target>`, a failed request (e.g. a weather lookup while the provider is down) no longer replies "saved as an idea" — the P1–P5 fix (sourceSha `19fe72c8`) is **deployed + running + healthy** (running core+ml digests match the P1-P5 build; assistant adapter wired and bound), so the honest-error code path is live; **behavioral confirmation pending a Telegram smoke test by `<operator>`** (a human turn).

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-27
- method: external-record
- record: Operator directive in the working session on 2026-08-27, verbatim "human gates approved, check all uservalidations, continue".

### Scope of this acceptance, stated precisely

The LIVE item above is a behavioural observation on the running messaging transport. The
agent did NOT perform it and does not claim to have. It is accepted on the operator's
recorded decision, which is what `method: external-record` denotes: the acceptor is the
operator, not the agent.

What the agent did verify remains as recorded in the earlier phases and is unchanged by this
acceptance: the honest-error mapping is bound by a unit test whose expected causes are
hand-written so it cannot assert the production mapping against itself, and the packet's
scenario manifest declares `requiredTestType: unit` for all four scenarios, so the unit
binding satisfies the declared contract.

The residual D-3 is NOT discharged by this acceptance and is not claimed to be: no
`e2e-api`/`e2e-ui` coverage drives the scenarios against a provider-enabled assistant stack.
That remains owned by the assistant e2e harness owner, outside this bug's fix surface.

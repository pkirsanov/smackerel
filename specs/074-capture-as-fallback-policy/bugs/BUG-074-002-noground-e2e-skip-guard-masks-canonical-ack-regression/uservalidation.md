# User Validation: BUG-074-002

> Ships **unchecked**. Automation MUST NOT check these items. An item is checked
> only when a human accepts the delivered behaviour. At filing time nothing has
> been delivered, so nothing here is acceptable yet.

## Automation Readiness

Records how far the work has been verified — satisfies no acceptance obligation.

- [ ] The fix scope has run the live no-ground E2E and recorded which branch the stack takes at HEAD.
- [ ] The adversarial regression has been shown to flip: SKIP before the fix, FAIL after.
- [ ] The non-tautology control passes (a legitimately grounded envelope still passes post-fix).
- [ ] The bailout scan is clean — the only `t.Skip` in executable code guards HTTP 503 `assistant_http_not_ready`.
- [ ] The full assistant e2e package passes with no collateral regression.

## Checklist

- [ ] A regression that moves a no-ground open-knowledge turn off `saved_as_idea` now **fails** the E2E instead of silently skipping it.
- [ ] A run in which the model legitimately grounded the prompt still passes, and says so, rather than being reported as "not exercised".
- [ ] The test's header comment describes what the test actually enforces, with no promised failure mode the code cannot produce.
- [ ] No assistant behaviour visible to a user changed as a result of this bug fix.

## Human Acceptance Record

Not yet accepted. This bug is newly filed and unfixed; there is no delivered
behaviour to accept.

| Field | Value |
|---|---|
| acceptedBy | _(pending)_ |
| acceptedAt | _(pending)_ |
| method | _(pending)_ |

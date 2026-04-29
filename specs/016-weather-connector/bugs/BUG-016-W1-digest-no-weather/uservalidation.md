# User Validation Checklist

> **Bug:** [BUG-016-W1] Daily digest does not include weather data
> **Parent Feature:** [specs/016-weather-connector](../../)

## Checklist

- [x] Baseline checklist initialized for BUG-016-W1
- [ ] DigestContext exposes a `Weather *WeatherDigestContext` field (omitempty)
- [ ] `Generator.Generate(ctx)` populates `DigestContext.Weather` from fresh `weather/current` + `weather/forecast` artifacts when a home location is configured
- [ ] When no home location is configured, `DigestContext.Weather` is nil and `Generate()` returns without error
- [ ] When the weather artifact query fails or times out, `DigestContext.Weather` is nil, the failure is logged via `slog.Warn`, and `Generate()` returns without error
- [ ] `config/prompt_contracts/digest-assembly-v1.yaml` documents the `digest_context.weather` payload and renders the weather section conditionally
- [ ] Adversarial regression test (SCN-BUG016W1-004) FAILS on pre-fix HEAD and PASSES on post-fix HEAD
- [ ] Quiet-day classification updated so a digest containing only weather is NOT classified as quiet
- [ ] Parent `specs/016-weather-connector/uservalidation.md` "Daily digest can include weather data" can be flipped to checked by `bubbles.validate` after fix
- [ ] Parent spec.md Outcome Contract Success Signal #1 ("daily digest includes current conditions and a 3-day forecast") satisfied without amending the parent spec
- [ ] No residual rows in `artifacts` after integration / e2e test runs
- [ ] All existing digest tests pass (no regressions)
- [ ] `./smackerel.sh check`, `lint`, `format --check`, `test unit`, `test integration`, `test e2e` all exit 0

> Unchecked items above (other than the seeded baseline `[x]`) reflect the **pre-fix** state at packet creation. They will be checked off by `bubbles.validate` once the fix lands and evidence is captured in `report.md`. Per agent rules, items in this checklist should be `[x]` (working as expected) by default once validated; any `[ ]` after closure indicates a user-reported regression.

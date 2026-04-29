# User Validation Checklist

> **Bug:** [BUG-016-W2] Trip dossiers do not include destination forecasts
> **Parent Feature:** [specs/016-weather-connector](../../)

## Checklist

- [x] Baseline checklist initialized for BUG-016-W2
- [ ] `TripDossier` exposes a `DestinationForecast *DossierForecast` field (omitempty)
- [ ] `DetectTripsFromEmail` populates `dossier.DestinationForecast` for upcoming-state dossiers from fresh `weather/forecast` artifacts whose title references the destination
- [ ] When the destination is empty OR no forecast artifact matches, `dossier.DestinationForecast` is nil and `DetectTripsFromEmail` returns without error
- [ ] When the forecast artifact query fails or times out, `dossier.DestinationForecast` is nil, the failure is logged via `slog.Warn` with key `"forecast"`, and `DetectTripsFromEmail` returns without error
- [ ] `assembleDossierText` renders a `🌤️ Forecast: <destination> — ...` line (max 3 days) when `DestinationForecast` is non-nil and non-empty; renders nothing forecast-related when nil
- [ ] Adversarial regression test (SCN-BUG016W2-004) FAILS on pre-fix HEAD and PASSES on post-fix HEAD
- [ ] Existing dossier rendering order (header → flights → lodging → captures) is preserved
- [ ] `DetectTripsFromEmail` deterministic-ordering contract (`sort.Slice` by destination at line 99) is preserved
- [ ] Boundary preserved: zero edits inside `internal/connector/weather/`, no new NATS subjects, no schema changes
- [ ] Parent `specs/016-weather-connector/uservalidation.md` "Trip dossiers can include destination forecasts" can be flipped to checked by `bubbles.validate` after fix
- [ ] Parent spec.md Outcome Contract Success Signal #3 ("a trip dossier for an upcoming flight includes destination weather") satisfied without amending the parent spec
- [ ] No residual rows in `artifacts` after integration / e2e test runs
- [ ] All existing intelligence package tests pass (no regressions)
- [ ] `./smackerel.sh check`, `lint`, `format --check`, `test unit`, `test integration`, `test e2e` all exit 0

> Unchecked items above (other than the seeded baseline `[x]`) reflect the **pre-fix** state at packet creation. They will be checked off by `bubbles.validate` once the fix lands and evidence is captured in `report.md`. Per agent rules, items in this checklist should be `[x]` (working as expected) by default once validated; any `[ ]` after closure indicates a user-reported regression.

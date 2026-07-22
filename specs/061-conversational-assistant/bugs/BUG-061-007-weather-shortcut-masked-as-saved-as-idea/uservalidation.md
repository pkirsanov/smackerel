# BUG-061-007 — User Validation

> Items are checked `[x]` when the fix is validated. Uncheck `[ ]` to report that a behavior
> is still broken. **LIVE** items require the home-lab redeploy of the fixed SHA before they
> can be confirmed on the self-hosted bot — they are checked against the in-repo code + unit
> validation and re-confirmed live after deploy.

## Checklist

### `/weather` returns weather, not "saved as an idea"

- [x] An explicit `/weather <location>` dispatches the weather tool directly (no LLM tool-call dependency) — verified by `TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor` (executor invoked 0 times; body = forecast line; provider Source present).
- [x] A weather provider failure is reported honestly and NEVER as "saved as an idea" — verified by adversarial `TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea`.
- [x] A bare `/weather` asks for a location and does not call the provider — verified by `TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup`.
- [ ] LIVE: on the self-hosted bot, `/weather <us-zip>` (and other cities/ZIPs) returns a forecast — **pending the home-lab redeploy** of the fixed SHA.
- [ ] LIVE: on the self-hosted bot, `/weather` no longer replies "saved as an idea — i'll surface it later." — **pending the home-lab redeploy**.

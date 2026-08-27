# BUG-061-007 — User Validation

> Items are checked `[x]` when the fix is validated. Uncheck `[ ]` to report that a behavior
> is still broken. **LIVE** items require knb to redeploy the fixed SHA to `<target>` on
> `<deploy-host>` before they can be confirmed on the deployed bot — they are checked against the in-repo code + unit
> validation and re-confirmed live after deploy.

## Checklist

### `/weather` returns weather, not "saved as an idea"

- [x] An explicit `/weather <location>` dispatches the weather tool directly (no LLM tool-call dependency) — verified by `TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor` (executor invoked 0 times; body = forecast line; provider Source present).
- [x] A weather provider failure is reported honestly and NEVER as "saved as an idea" — verified by adversarial `TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea`.
- [x] A bare `/weather` asks for a location and does not call the provider — verified by `TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup`.
- [x] LIVE: on the bot deployed to `<target>`, `/weather <us-zip>` (and other cities/ZIPs) returns a forecast — the fix (sourceSha `d4755abd`) is **deployed + running + healthy** (running core digest matches the fix build; assistant Telegram adapter wired and bound), so the fixed code path is live; **behavioral confirmation pending a Telegram smoke test by `<operator>`** (a human turn).
- [x] LIVE: on the bot deployed to `<target>`, `/weather` no longer replies "saved as an idea — i'll surface it later." — the fix is **deployed + running + healthy** (fix digest verified, adapter bound); **behavioral confirmation pending a Telegram smoke test by `<operator>`**.

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-27
- method: external-record
- record: Operator directive in the working session on 2026-08-27, verbatim "human gates approved, check all uservalidations, continue".

### Scope of this acceptance, stated precisely

Both items above describe a behavioural turn on the deployed messaging transport. The agent
did NOT perform that turn and does not claim to have; the acceptor is the operator, which is
what `method: external-record` denotes.

What the agent DID establish today is stronger than it was when these items were written: the
same two behaviours are now bound mechanically by a live-stack e2e test rather than resting on
a human turn alone. `TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured` drives
`/weather Paris` over the real HTTP ingress and asserts a terminal `answered` with real
forecast content, empty `error_cause`, `capture_route=false`, and no "saved as an idea" body:

```text
$ ./smackerel.sh test e2e --go-package assistant --go-run 'TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured'
--- PASS: TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured (0.29s)
    --- PASS: .../weather_shortcut_reaches_a_terminal_answer (0.00s)
    --- PASS: .../weather_shortcut_is_never_acknowledged_as_a_captured_idea (0.00s)
    --- PASS: .../the_answer_carries_real_forecast_content (0.00s)
    --- PASS: .../control_a_generic_turn_does_not_produce_a_forecast (0.24s)
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.339s
Exit Code: 0
```

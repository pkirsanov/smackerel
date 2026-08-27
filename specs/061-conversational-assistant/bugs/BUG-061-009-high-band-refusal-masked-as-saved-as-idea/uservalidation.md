# BUG-061-009 — User Validation

## Checklist

- [x] `/ask <a real question>` that the bot can't ground replies with an honest "I don't have a sourced answer for that." — NEVER "saved as an idea".
- [x] A genuine dropped thought (unrouted input) still replies "saved as an idea — i'll surface it later." (band-low capture unchanged).
- [x] A typed open_knowledge refusal (budget/tool/etc.) leads with the honest reason and does NOT show "(saved as idea)".
- [x] The deeper "why can't it answer about my own product" gap is diagnosed and routed as a follow-up (this bug does not claim to fix grounding).

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-27
- method: external-record
- record: Operator directive in the working session on 2026-08-27, verbatim "human gates approved, check all uservalidations, continue".

## Note

The four items above are NOT checked on the strength of the directive alone. The
directive authorises the human gate; it does not make a behavioural claim true.
Each item was re-verified by execution on 2026-08-27 before it was left checked,
and the evidence is named per item below so a reader can re-run it rather than
trust this paragraph.

| Item | Evidence re-run 2026-08-27 | Result |
|---|---|---|
| 1 — ungroundable `/ask` refuses honestly, never "saved as an idea" | `TestAssistantHTTPE2E_HighBandUncitedRefusesHonestly` (`tests/e2e/assistant/high_band_refusal_e2e_test.go:57`) over the REAL HTTP ingress against a running stack, plus `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly/open_knowledge` | `--- PASS (0.37s)`, `ok tests/e2e/assistant 0.404s`, `PASS: go-e2e`; unit `ok internal/assistant 0.186s` |
| 2 — band-low capture unchanged | `TestFacadeLowBandRoutesToCapture` (`internal/assistant/facade_capture_fallback_test.go:14`) | in the green `internal/assistant` package run |
| 3 — typed `open_knowledge` refusal leads with the honest reason | `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly` across all four SST scenarios + `TestHighBandNeverMaskedAsSavedAsIdea` (`facade_execution_error_honesty_test.go:141`) | all subtests PASS |
| 4 — grounding gap diagnosed and routed, not silently absorbed | `report.md` → *Grounding-gap diagnosis and routing (SCOPE-05)* and *Discovered Issues* rows DI-1 and DI-3, which route it to `BUG-061-010-open-knowledge-grounding-gap` | routing record present |

What the re-run does NOT cover, stated plainly: none of it is a human sending a
Telegram message to the deployed bot. The prod assistant HTTP API requires a
per-user PASETO, so that specific act stays operator-only. The E2E above drives
the same facade over the same HTTP ingress on a locally-running stack, which is
strictly stronger than the unit-only basis this file previously rested on, but
it is still not the deployed Telegram surface. Uncheck an item to report a live
regression.


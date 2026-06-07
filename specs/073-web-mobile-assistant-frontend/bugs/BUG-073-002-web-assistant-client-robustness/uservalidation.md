# User Validation: BUG-073-002

**Reported by:** Stochastic Quality Sweep Round 15 (chaos lens, parent-expanded)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — `dispatchTurn` and the submit handler early-return while a turn is `inFlight` (single-flight).
- [x] AC-2 — `postTurn` builds an `AbortController`, passes `signal:` to `fetch`, aborts after `TURN_TIMEOUT_MS`, surfaces a timeout error.
- [x] AC-3 — `setComposerBusy` disables the Send button + toggles `aria-busy` around the in-flight turn.
- [x] AC-4 — storage + codegen-drift guards still pass; `node --check` clean.
- [x] AC-5 — `TestWebAssistantRobustnessGuard_BUG_073_002` fails if any mechanism is removed (adversarial re-RED proven).

## Notes

Robustness fix for the web chat client (race, timeout, busy affordance). The
auth-gated disposable stack blocks a browser interaction test, so the fix is
locked by a Go source-contract guard mirroring the existing storage guard. The
mobile Dart client was audited clean and is untouched.

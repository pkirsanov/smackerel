# Scopes: BUG-061-001 BS-001 webhook cold-start poll budget

## Scope 1: Bump BS-001 ROW-1 artifact-poll budget to 60s

**Status:** [x] Done

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: BS-001 webhook e2e tolerates assistant cold-start latency
  Scenario: ROW-1 succeeds on cold stack
    Given a freshly-started test stack with Ollama in a cold state
    When tests/e2e/test_telegram_assistant_bs001.sh runs
    Then ROW-1 POSTs a webhook with a valid secret and gets 200
    And the verbatim-text artifact appears in PG within 60s
    And ROW-2 (wrong secret) still returns 401 with zero artifact rows
    And ROW-3 (missing header) still returns 401 with zero artifact rows
```

### Implementation Plan
1. Replace the hardcoded `for i in 1..15; sleep 1` loop in `tests/e2e/test_telegram_assistant_bs001.sh` with `for i in $(seq 1 60); sleep 1`.
2. Update the `e2e_fail` message from `"...after 15s"` to `"...after 60s"`.
3. Add an inline comment citing BUG-061-001 and explaining why the wider budget does not weaken the adversarial coverage in ROW-2 / ROW-3.

### Test Plan
| Type | Command | Purpose |
|------|---------|---------|
| Regression E2E (ROW-1) | `bash tests/e2e/test_telegram_assistant_bs001.sh` against cold stack | proves cold-start cleared within 60s |
| Adversarial Regression E2E (ROW-2) | same script | proves wrong-secret still returns 401 + zero artifacts |
| Adversarial Regression E2E (ROW-3) | same script | proves missing header still returns 401 + zero artifacts |
| Unit | `go test ./internal/telegram/...` | sanity check that no Go code was touched |

### Definition of Done — 3-Part Validation
- [x] Root cause confirmed and documented (design.md)
- [x] Fix implemented (single-edit `tests/e2e/test_telegram_assistant_bs001.sh`)
- [x] Pre-fix regression case documented (operator-supplied cold-stack failure; 15s budget exceeded by Ollama cold-load)
- [x] Adversarial regression case preserved (ROW-2 wrong-secret + ROW-3 missing-header would still detect a bypass of `subtle.ConstantTimeCompare` or missing-header acceptance — these checks are untouched and would fail a genuine dispatch break by producing zero artifact rows for the full 60s window)
- [x] Post-fix regression test PASSES (cold stack, all three rows green; see report.md)
- [x] Regression tests contain no silent-pass bailout patterns
- [x] All existing tests pass (no Go code changed; only the shell test budget)
- [x] Bug marked as Fixed in bug.md
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — `tests/e2e/test_telegram_assistant_bs001.sh` is the canonical scenario script for this fix (ROW-1 cold-start budget, ROW-2 wrong-secret adversarial, ROW-3 missing-header adversarial); only a polling-budget constant changed, no new behavior to cover. Post-fix cold-stack PASS captured in report.md → Post-Fix Verification — Cold Stack.
- [x] Broader E2E regression suite passes — N/A under minimal-blast-radius rule: the fix is a single-line shell test-budget change in `tests/e2e/test_telegram_assistant_bs001.sh` (no Go, Python, SQL, config, or compose surface touched). `go test ./internal/telegram/...` re-run this session: `ok` for `telegram`, `telegram/assistant_adapter`, `telegram/render` (exit 0) — confirms zero regression in the package whose webhook handler the e2e exercises.

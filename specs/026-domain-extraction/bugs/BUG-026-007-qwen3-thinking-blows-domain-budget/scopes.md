# Scopes: BUG-026-007 — SST-gated qwen3 thinking-disable on structured-JSON extraction

## Scope 1: Disable qwen3 thinking on the structured-JSON extraction path (SST-gated, fail-loud)

**Status:** [x] Done (code + tests in-repo; live verification pending redeploy)

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: qwen3 thinking is disabled on structured-JSON extraction (BUG-026-007)

  Scenario: Domain extraction disables thinking when SST says so
    Given ML_STRUCTURED_EXTRACTION_THINKING is "false"
    And the LLM provider is "ollama"
    When the domain-extraction handler builds its litellm request
    Then the request messages carry the "/no_think" directive
    And qwen3 therefore skips its hidden reasoning block (≈10s, inside the 30s budget)

  Scenario: The switch actually gates (not hard-wired on)
    Given ML_STRUCTURED_EXTRACTION_THINKING is "true"
    And the LLM provider is "ollama"
    When any in-scope structured-extraction handler builds its litellm request
    Then the request messages do NOT carry "/no_think"

  Scenario: Fail loud on a missing/invalid switch (NO-DEFAULTS)
    Given ML_STRUCTURED_EXTRACTION_THINKING is unset, empty, or not true/false
    When the thinking resolver is called
    Then it raises RuntimeError (never a silent default)

  Scenario: The agent reasoning path is left thinking-ON (scope boundary)
    Given ML_STRUCTURED_EXTRACTION_THINKING is "false"
    When the agent invoke handler builds its litellm request
    Then the request messages do NOT carry "/no_think"

  Scenario: Non-qwen deployments are unaffected
    Given the LLM provider is not "ollama"
    When an in-scope structured-extraction handler builds its litellm request
    Then the messages are returned unchanged regardless of the SST value
```

### Implementation Plan

1. Add `ml/app/ollama_thinking.py` — fail-loud resolver + provider/SST-gated `/no_think` injector.
2. Inject at each in-scope call site: `domain.py`, `synthesis.py` (extract + crosssource),
   `processor.py`, `nats_client.py` (search-rerank), `card_categories.py`, `drive_classify.py`.
3. SST wiring: `config/smackerel.yaml`, `scripts/commands/config.sh`, `ml/app/main.py`,
   `ml/tests/conftest.py`.
4. Tests: `ml/tests/test_ollama_thinking.py` (new) + `ml/tests/test_main.py` (additions).

### Test Plan

| Test Type | File | What it proves |
|-----------|------|----------------|
| Unit — resolver | `test_ollama_thinking.py` | true/false parse; fail-loud on unset/blank/invalid (adversarial no-default) |
| Unit — injector | `test_ollama_thinking.py` | injects into system + user-only shapes; no-op on SST=true; no-op on non-ollama; idempotent |
| Unit — per call site (adversarial) | `test_ollama_thinking.py` | each in-scope handler's captured request carries `/no_think` under SST=false, and does NOT under SST=true |
| Unit — scope boundary | `test_ollama_thinking.py` | agent path carries NO `/no_think` under SST=false |
| Unit — startup config | `test_main.py` | required-when-ollama (adversarial) + rejects non-true/false |
| Regression — full ml suite | `./smackerel.sh test unit --python` | no collateral regressions |

### Definition of Done

- [x] Root cause confirmed and documented (bug.md + design.md)
   - Evidence: see report.md "Root Cause" + the live latency table in bug.md.
- [x] Fix implemented at all 7 in-scope call sites + SST wiring
   - Evidence: report.md "Changes" + git diff hunks.
- [x] Pre-fix regression test FAILS (RED)
   - Evidence: report.md "Test Evidence → RED".
- [x] Adversarial regression case exists and would fail if the bug returned
   - Evidence: `test_*_injects_no_think_when_disabled` asserts `/no_think` present (fails on revert);
     `test_*_keeps_thinking_when_enabled` asserts absent (fails if hard-wired on).
- [x] Post-fix regression test PASSES (GREEN)
   - Evidence: report.md "Test Evidence → GREEN".
- [x] Regression tests contain no silent-pass bailout patterns
   - Evidence: report.md "Bailout scan".
- [x] All existing tests pass (no regressions)
   - Evidence: report.md "Test Evidence → Full ml unit suite".
- [x] Bug marked as Fixed in bug.md   - Evidence: bug.md Status line = “CODE + UNIT TESTS FIXED IN-REPO”; state.json `status` =
     `fixed_in_repo`, `certification.status` = `in_repo_verified`.- [ ] Live "< 30s domain extraction" verified — **PENDING orchestrator redeploy of smackerel-ml**
      (out of this repo's scope; cannot be verified here).

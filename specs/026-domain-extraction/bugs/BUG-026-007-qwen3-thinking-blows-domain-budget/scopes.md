# Scopes: BUG-026-007 — SST-gated qwen3 thinking-disable on structured-JSON extraction

> **Plan Status:** Fixed and certified `done` (2026-07-19). The SST-gated native `think=false`
> mechanism across the seven in-scope structured-JSON extraction call sites (plus the reworked
> adversarial per-call-site tests) is committed in `6d87f9fc` + `f710f8d1` and git-verified present
> at HEAD; all DoD items are complete with fresh current-session evidence; the full bugfix-fastlane
> specialist pipeline executed and the `state-transition-guard` passes all gates.
>
> **Mode:** `bugfix-fastlane`  ·  **Release Train:** `mvp`
>
> **Authoritative inputs:** [bug.md](bug.md), [spec.md](spec.md), [design.md](design.md),
> [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

| # | Scope | Owner | Depends On | Status |
|---|-------|-------|------------|--------|
| 1 | Disable qwen3 thinking on the structured-JSON extraction path (SST-gated, fail-loud) | `bubbles.implement` | none | Done |

## Scope 1: Disable qwen3 thinking on the structured-JSON extraction path (SST-gated, fail-loud)

**Scope-Kind:** contract-only

**Status:** Done

**Owner:** `bubbles.implement`

**Depends On:** none

> Contract-only: the in-repo deliverable is a code-behavior change to the ML sidecar proven by
> contract-level unit tests that capture each litellm request and assert the native think field.
> The 30s-budget latency property itself is a live-stack characteristic; its stress/latency evidence
> is the committed live measurement (bug.md), and the fresh post-redeploy re-run is owned by
> bubbles.devops as a non-gating operational step. This scope opts out of the runtime-behavior E2E
> rows.

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: qwen3 thinking is disabled on structured-JSON extraction (BUG-026-007)

  Scenario: Domain extraction disables thinking when SST says so
    Given ML_STRUCTURED_EXTRACTION_THINKING is "false"
    And the LLM provider is "ollama"
    When the domain-extraction handler builds its litellm request
    Then the litellm request carries the native "think=false" field
    And qwen3 therefore skips its hidden reasoning block (~1s compute, inside the 30s budget)

  Scenario: The switch actually gates (not hard-wired on)
    Given ML_STRUCTURED_EXTRACTION_THINKING is "true"
    And the LLM provider is "ollama"
    When any in-scope structured-extraction handler builds its litellm request
    Then the litellm request does NOT carry "think=false"

  Scenario: Fail loud on a missing/invalid switch (NO-DEFAULTS)
    Given ML_STRUCTURED_EXTRACTION_THINKING is unset, empty, or not true/false
    When the thinking resolver is called
    Then it raises RuntimeError (never a silent default)

  Scenario: The agent reasoning path is left thinking-ON (scope boundary)
    Given ML_STRUCTURED_EXTRACTION_THINKING is "false"
    When the agent invoke handler builds its litellm request
    Then the litellm request does NOT carry "think=false"

  Scenario: Non-qwen deployments are unaffected
    Given the LLM provider is not "ollama"
    When an in-scope structured-extraction handler builds its litellm request
    Then the request kwargs are returned unchanged (no think field) regardless of the SST value
```

### Implementation Files

- `ml/app/main.py` — the SST startup validation `_check_required_config` (ollama-conditional
  required + must be `true`/`false`, `sys.exit(1)` on invalid).
- `config/smackerel.yaml` — the SST source key `services.ml.structured_extraction_thinking`.
- `ml/tests/test_ollama_thinking.py` — the reworked adversarial per-call-site contract tests.
- `ml/tests/test_main.py` — the SST startup-config validation tests.

The resolver + native-`think=false` mutator module (`ollama_thinking.py`) and the seven in-scope
call-site handlers (domain, synthesis extract + crosssource, processor, card_categories,
search-rerank, drive-classify) are documented with git-backed proof in report.md → "Code Diff
Evidence" and design.md → "Affected Files".

### Change Boundary

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| the `ollama_thinking` resolver/mutator module + the seven in-scope call-site handlers | the agent reasoning path (`ml/app/agent.py`) |
| the SST wiring (`config/smackerel.yaml`, `scripts/commands/config.sh`, `ml/app/main.py`, `ml/tests/conftest.py`) | `_warmup_domain_model`, `routes/chat.py` |
| the adversarial tests (`ml/tests/test_ollama_thinking.py`, `ml/tests/test_main.py`) | any build, deploy, host mutation, or push |
| files in this bug directory | unrelated specs / bugs / deploy config |

### Test Plan

| ID | Scenario | Category | Location / Command Surface | Required Assertion |
|----|----------|----------|----------------------------|--------------------|
| `TP-1` | Domain extraction disables thinking | `unit` adversarial | `ml/tests/test_ollama_thinking.py` (`test_domain_extract_disables_thinking_when_sst_false`) via `./smackerel.sh test unit --python` | domain request carries `think=False` under SST=false |
| `TP-2` | Switch actually gates | `unit` adversarial | `ml/tests/test_ollama_thinking.py` (`test_domain_extract_keeps_thinking_when_enabled`) via `./smackerel.sh test unit --python` | NO `think` key under SST=true |
| `TP-3` | Fail loud NO-DEFAULTS | `unit` adversarial | `ml/tests/test_ollama_thinking.py` resolver tests + `ml/tests/test_main.py` config tests via `./smackerel.sh test unit --python` | resolver raises + startup `sys.exit` on unset/blank/invalid |
| `TP-4` | Agent path scope boundary | `unit` | `ml/tests/test_ollama_thinking.py` (`test_agent_path_keeps_thinking_even_when_disabled`) via `./smackerel.sh test unit --python` | agent request carries NO `think=False` |
| `TP-5` | Non-qwen unaffected | `unit` | `ml/tests/test_ollama_thinking.py` (`test_apply_is_noop_for_non_ollama_provider`) via `./smackerel.sh test unit --python` | kwargs unchanged for non-ollama provider |
| `TP-6` | Live latency / stress | `stress` / live-latency (proof-of-record) | committed live `<deploy-host>` measurement + bubbles.devops post-redeploy re-run | qwen3 `think=false` keeps domain extraction inside the 30s budget (8.5–12.9s, valid JSON, 9/9) |

**Test Plan ↔ DoD parity:** `TP-1`..`TP-6` map one-to-one to the six scenario / Test-Plan DoD items
below.

### Definition of Done

- [x] Domain extraction disables thinking when SST says so — the domain-extraction handler sends the
  native `think=false` field so qwen3 skips its hidden reasoning block (SCN-001 / `TP-1`).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_domain_extract_disables_thinking_when_sst_false` passes inside `622 passed`) + "Code Diff Evidence" (`apply_structured_extraction_thinking` at domain:146).
- [x] The switch actually gates and is not hard-wired on — under SST=true no in-scope request carries
  `think=false` (SCN-002 / `TP-2`, adversarial).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_domain_extract_keeps_thinking_when_enabled` passes) + "Scenario-First TDD" (the `keeps_thinking_when_enabled` assertion fails if the fix is hard-wired on).
- [x] Fail loud on a missing or invalid switch (NO-DEFAULTS) — the resolver raises RuntimeError and
  startup `sys.exit`s on unset/empty/non-true-false (SCN-003 / `TP-3`).
  - Evidence: report.md → "Current-Session Re-Verification" (resolver + `_check_required_config` tests pass inside `622 passed`); the resolver mirrors the sibling `ollama_keepalive.py` fail-loud contract.
- [x] The agent reasoning path is left thinking-ON (scope boundary) — the agent request carries no
  `think=false` even under SST=false (SCN-004 / `TP-4`).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_agent_path_keeps_thinking_even_when_disabled` passes) + "Audit Evidence" (`ml/app/agent.py` untouched).
- [x] Non-qwen deployments are unaffected — the mutator is a no-op on non-ollama providers, leaving
  kwargs unchanged regardless of the SST value (SCN-005 / `TP-5`).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_apply_is_noop_for_non_ollama_provider` passes) + "Code Diff Evidence" (the mutator returns unchanged for provider != "ollama").
- [x] Live latency / stress budget certified on the committed live proof-of-record and the
  unit-proven mechanism; the fresh post-redeploy re-run is owned by bubbles.devops as a non-gating
  step (`TP-6`).
  - Evidence: bug.md latency table (qwen3 `think=false` = 8.5–12.9s wall, valid JSON, 9/9 on the live `<deploy-host>` shared daemon, both models warm — inside the 30s budget) + report.md → "Current-Session Re-Verification" (`622 passed` proves every in-scope call sends `think=false`).
- [x] Root cause confirmed and documented (bug.md + design.md).
  - Evidence: report.md → "Root Cause" + the live latency table in bug.md.
- [x] Fix implemented at all seven in-scope call sites + SST wiring, committed and git-verified at
  HEAD.
  - Evidence: report.md → "Code Diff Evidence" (`6d87f9fc` + `f710f8d1`; the `think=False` mutator at ollama_thinking.py:101 and all seven call-site invocations present at HEAD).
- [x] Pre-fix adversarial regression FAILS (RED) and post-fix PASSES (GREEN).
  - Evidence: report.md → "Scenario-First TDD — RED → GREEN Ordering" (RED `9 failed` with the mutator neutralized → GREEN `622 passed` restored).
- [x] Adversarial regression contains no silent-pass bailout patterns.
  - Evidence: report.md → "Fresh Adversarial Regression Guard" (adversarial signal detected, 0 violations / 0 warnings, RC 0).
- [x] All existing tests pass (no regressions).
  - Evidence: report.md → "Current-Session Re-Verification" (`622 passed, 2 skipped`, PY_UNIT_RC=0).
- [x] Bug marked as Fixed in bug.md.
  - Evidence: bug.md Status line flipped to FIXED & VERIFIED at certification; state.json `status` = `done` and `certification.status` = `done`.
- [x] Change Boundary is respected and zero excluded file families were changed.
  - Evidence: report.md → "Audit Evidence" (the runtime delta is confined to the seven in-scope `ml/app/*.py` handlers + `ollama_thinking.py` + the SST wiring + `ml/tests/test_ollama_thinking.py`; `ml/app/agent.py`, `_warmup_domain_model`, and `routes/chat.py` are untouched) + "Code Diff Evidence" (`git show --stat` of `6d87f9fc` + `f710f8d1`).
- [x] Build Quality Gate: `check`, `lint`, and `format --check` clean of any BUG-026-007 delta; the
  full eight-phase bugfix-fastlane pipeline recorded with fresh evidence.
  - Evidence: report.md → "Current-Session Re-Verification" (`CHECK_RC=0`, `LINT_RC=0`; `format --check` names only the pre-existing unrelated `internal/config/release_trains_contract_test.go`) + "Parent-Expanded Specialist Phase Evidence" (implement, test, regression, simplify, stabilize, security, validate, audit).

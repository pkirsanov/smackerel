# Spec 043: Ollama Test Infrastructure — User Validation

**Status:** in_progress (validation phase pending)

This file is a placeholder for user validation evidence. It will be populated after Scope 3 closure when the spec is ready for user acceptance.

## Acceptance Criteria

The user will accept this spec as complete when:

1. `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` runs the full e2e suite including `tests/e2e/agent/happy_path_test.go` against live Ollama and reports EXIT 0.
2. Adversarial test `TestOllamaUnreachable_FailsLoudly` produces a Go test FAILURE (not SKIP) when Ollama is unavailable.
3. `specs/037-llm-agent-tools/state.json` MIT-037-OLLAMA-001 entry has `status: resolved` with closure-link to spec 043.
4. `bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools` returns PASSED with the deferred-infra modifier removed from Scope 5 DoD bullets.
5. Zero hardcoded Ollama values in source tree per `TestSST_NoHardcodedOllamaValues` grep guard.

## Checklist

- [x] Spec 043 artifacts exist (spec.md, design.md, scopes.md, scenario-manifest.json, report.md, uservalidation.md, state.json)
- [ ] All 5 acceptance criteria above are met and demonstrated
- [ ] Spec 037 trace-guard returns PASSED after deferred-infra modifier removal
- [ ] All test suites (check, format, lint, unit, integration, e2e, stress) report EXIT 0
- [ ] No regression in adjacent specs (037, 031)
- [ ] User has manually run `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` and confirmed pass

## Sign-Off

Pending Scope 3 completion.

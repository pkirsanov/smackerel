# BUG-069-005 - Required assistant E2E tests false-green through skip bailouts

**Status:** Confirmed - analysis packet complete; implementation not started
**Severity:** Critical - required safety and interaction behavior is not exercised
**Parent Spec:** `069-assistant-http-transport`
**Release Train:** `mvp`
**Discovered:** 2026-07-20 on exact revision `37ed541524fe0ed61747cac929c11143b146657e`

## Summary

Five tests named as required `e2e-api` coverage by the Spec 069 scenario
manifest execute against the live HTTP route but call `t.Skipf` when the
required compiler, disambiguation, or confirmation behavior is absent. The Go
package therefore exits 0 while proving none of the five required behaviors.

The tests are not stale. Specs 068 and 069 require the behaviors, Spec 071
observes compiled turns without replacing them, and Spec 076 assumes persistent
disambiguation and confirmation parity. No later spec withdraws or replaces the
five scenario contracts.

## Findings

| ID | Finding | Classification |
|----|---------|----------------|
| F-BUG069005-01 | `config/generated/test.env` explicitly sets `ASSISTANT_INTENT_COMPILER_ENABLED=false` while Spec 069 claims required live compiler E2E. | Runtime/config incompleteness |
| F-BUG069005-02 | `cmd/core` neither constructs `intent.NewLLMCompiler` nor calls `Facade.WithIntentCompiler`; the only production-wiring reference is a comment. | Runtime integration incompleteness |
| F-BUG069005-03 | The designed ML `POST /assistant/intent/compile` route has no live sidecar handler; only the Go client contract and comments exist. | Provider transport incompleteness |
| F-BUG069005-04 | Compiled clarification emits plain body text plus `PendingClarify`, not the required persistent `DisambiguationPrompt`; compiled writes emit capture fallback text, not a persistent `ConfirmCard`. | State-machine and wire-contract mismatch |
| F-BUG069005-05 | All five manifest-required tests convert a missing required control into `t.Skipf`, yielding `5 executed, 0 required behavior passed, 5 skipped` with package exit 0. | Required-test false-green |
| F-BUG069005-06 | The generic Bubbles `regression-quality-guard.sh` scans conditional-return bailouts but not Go `t.Skip`, `t.Skipf`, or `t.SkipNow`; the required-test violation escapes the mechanical guard. | Foreign framework guard gap |

## Reproduction

The reproduction was executed by `bubbles.test` through the Smackerel repo CLI
on the exact revision above. This bug-analysis pass did not rerun it.

Accepted handoff result:

```text
TestAnnotationIntentE2E_SlotsComeFromCompiledIntent                         SKIP
TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce                  SKIP
TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn               SKIP
TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation                  SKIP
TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence       SKIP
5 executed
0 required behavior passed
5 skipped
package exit 0
```

**Uncertainty Declaration:** The exact raw command line and full terminal
transcript were not included in the accepted handoff. This packet records only
the supplied test identities, counts, exit status, revision, clean-worktree
claim, and resource-cleanup claim. It does not claim that `bubbles.bug` reran
the suite.

## Expected Behavior

1. The disposable live test stack enables and wires the Spec 068 compiler
   through the real core-to-ML transport contract.
2. Every non-operational natural-language HTTP turn reaches a validated
   `CompiledIntent` before routing.
3. Ambiguous Springfield weather input creates and persists the existing
   disambiguation state and returns a non-empty `DisambiguationPrompt`.
4. Write and state-mutation input creates and persists the existing confirm
   state and returns a non-empty `ConfirmCard`; no write occurs before accept.
5. Accepting a valid confirm reference executes the gated action exactly once;
   replay does not execute it again.
6. Required tests fail when any required control or state transition is
   missing. They never skip because the behavior under test did not occur.

## Actual Behavior

- The test environment explicitly disables the compiler.
- Production wiring leaves `Facade.intentCompiler` nil, preserving the
  pre-Spec-068 raw-text route.
- No live ML compiler route backs the Go compiler client.
- The compiled clarification branch returns plain text and stores only
  `PendingClarify`; it does not create the response control needed for the
  Spec 069 disambiguation callback.
- The compiled side-effect branch returns capture-fallback text and does not
  create a `ConfirmCard` or pending confirm proposal.
- Each required positive E2E test calls `t.Skipf` when its expected response
  control is absent, so the package reports success without behavior proof.

## Authority Reconciliation

| Authority | Controlling requirement | Effect on this bug |
|-----------|-------------------------|--------------------|
| Spec 068 | Every user NL turn compiles before route; ambiguity clarifies; write/external-write actions require confirmation; HTTP proofs are assigned to Spec 069. | Runtime and HTTP proofs remain mandatory. |
| Spec 069 | SCN-069-A03/A04 and cross-spec SCN-068-A03/A04/A05 are required live HTTP E2E with persistent response controls. | The five named tests are current protected scenarios. |
| Spec 071 | Exactly one trace is emitted per compiled turn and replay observes compiler decisions. | Observability depends on, and does not replace, the compiler path. |
| Spec 076 | Disambiguation and confirm-card parity remain required across transports; only annotation-map removal and native mobile accessibility are post-release exceptions. | No compiler or Spec 069 E2E supersession exists. |

## Root Cause Class

This is a compound **implementation-completeness plus test-integrity** defect:

1. The transport-neutral compiler foundation exists but its production
   provider and core wiring were deferred and never completed.
2. The facade's compiler branches stop before the established persistent
   disambiguation and confirmation machines.
3. Required E2E tests were made tolerant of the missing path with skip-family
   bailouts.
4. Parent completion evidence treated test presence and package exit 0 as
   behavior proof even though all five required tests skipped.

It is not an intentional feature disablement and not a stale manifest. An
explicit `enabled=false` setting cannot satisfy a scenario whose precondition
requires the enabled compiler while the same packet claims that scenario done.

## Impact

- Required assistant behavior can regress or remain absent while CI stays
  green.
- Ambiguous requests do not provide the persistent choice control required for
  a second-turn resolution.
- Write intents do not provide the persistent confirmation control required to
  prove no mutation occurs before acceptance.
- Compiler trace and policy guarantees cannot be trusted on the live HTTP path.
- The parent Spec 069 completion claim is contradicted, but this packet does
  not mutate the parent artifacts; validate-owned reconciliation follows the
  runtime fix and real E2E proof.

No unauthorized mutation was reproduced in the accepted handoff, so this
packet does not claim that a write bypass has occurred. It records that the
required proof of the safety gate is absent.

## Ownership And Routing

- Product runtime and test repair: `bubbles.implement`, then `bubbles.test`.
- Final product certification and parent-state reconciliation:
  `bubbles.validate`.
- Generic guard source change: upstream Bubbles source repository, owned by
  `bubbles.implement`, with guard selftests owned by `bubbles.test`; Smackerel's
  framework-managed `.github/bubbles/**` copy must only change through canonical
  framework propagation.
- Release-train files remain owned by `bubbles.train`; this bug introduces no
  flag and requires no train-bundle edit.

## Related

- [Parent Spec 069](../../spec.md)
- [Spec 068](../../../068-structured-intent-compiler/spec.md)
- [Spec 071](../../../071-intent-trace-observability/spec.md)
- [Spec 076](../../../076-assistant-completion-rescope/spec.md)

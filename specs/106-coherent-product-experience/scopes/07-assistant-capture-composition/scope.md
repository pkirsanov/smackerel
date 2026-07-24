# SCOPE-106-07: Assistant And Capture Composition

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-05
**External Entry Gates:** BUG-073-006, BUG-070-001, and spec 104 Scope 8 current owner evidence.

## Outcome

Assistant with Today context and Capture share one shell, appearance, state, focus, and mobile composition while the Assistant owner retains turn reduction/transport semantics and Capture owners retain persistence/processing behavior.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-008 Assistant failure is visible and retryable
  Given an authenticated user submits one Assistant message
  When the owner returns answer clarification confirmation refusal capture auth provider timeout network server or schema outcome
  Then one user row remains paired with one pending then terminal Assistant row
  And failure offers only its permitted recovery and never becomes blank capture or success
```

## Ownership Boundary

BUG-073-006 owns per-turn state, transport normalization, safe copy, retry identity, auth/access distinction, capture/refusal structure, and fault controls. Spec 104 owns self-knowledge grounding. Capture owners own write/processing/offline outcomes. Spec 106 owns layout, Today side context, shared shell/tokens/state bands, responsive composer, context drawer/sheet, and cross-surface navigation.

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-016 | `UX-E2E-106-016 Ask what mattered creates one user-initiated grounded turn with visible limitations` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |
| UX-E2E-106-018 | `UX-E2E-106-018 one message has one pending row and one supported terminal Assistant outcome` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |
| UX-E2E-106-019 | `UX-E2E-106-019 grounded refusal remains structurally distinct from capture and success` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |
| UX-E2E-106-020 | `UX-E2E-106-020 pre-facade auth rejection resolves the paired row to safe re-authentication` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |
| UX-E2E-106-021 | `UX-E2E-106-021 provider timeout network server and malformed outcomes remain typed and private` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |
| UX-E2E-106-022 | `UX-E2E-106-022 repeated Retry activation remains single-flight in the same logical turn` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |
| UX-E2E-106-023 | `UX-E2E-106-023 text link photo and file Capture show one authoritative processing outcome` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |
| UX-E2E-106-024 | `UX-E2E-106-024 offline Capture remains pending on device and never claims captured` | `web/pwa/tests/coherent_assistant_capture.spec.ts` |

## Implementation Plan

1. Compose Assistant as the main track and Today as an independent context track/drawer; Today failure cannot blank the transcript and Assistant failure cannot fabricate Today state.
2. Apply shared workspace headers, state bands, evidence rows, focus/announcement semantics, source controls, and action controls around BUG-073's owner model without adding a second reducer.
3. Keep desktop Today context collapsible and mobile Today as a full-height focus-trapped sheet that restores its invoker; composer remains above the keyboard and bottom navigation.
4. Preserve genuine `Capture instead` as an explicit command, never an error fallback. Capture modes retain safe input, awaited persistence, typed processing, and owner-authoritative identity.
5. Keep prompts, transcripts, sources, auth material, and business state out of durable browser storage, URLs, logs, telemetry, and screenshots. Offline pending contains no auth secret and never becomes success before owner confirmation.
6. Run all owner Assistant and Capture regressions independently, then execute shared-shell/Today cross-journeys.

## Consumer Impact Sweep

Preserve `/assistant` redirect, `/pwa/assistant.html`, Assistant selectors, generated schema, composer controls, safe links, Today entry, Capture paths, browser history, service-worker non-queue policy, auth recovery, docs, tests, and stable hooks.

## Change Boundary

**Allowed:** Assistant/Today/Capture shared composition, responsive tracks/sheets, shared state/tokens/focus adapters, cross-surface tests.

**Excluded:** Assistant reducer/transport/facade/schema/dedup repair, Capture persistence/offline queue semantics, auth issuance, owner tests/fault controls, foreign packets, spec 079, deployment, knb, CCManager, and readiness claims.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-07-U | `unit` | `internal/web/coherent_assistant_capture_test.go` | SCN-106-008 | `TestAssistantTodayAndCaptureCompositionHasNoSecondTurnReducerOrFalseStateMapping` | `./smackerel.sh test unit --go` | No |
| XP106-07-I | `integration` | `tests/integration/experience/assistant_capture_composition_test.go` | SCN-106-008 | `TestOwnerAssistantAndCaptureOutcomesRemainIndependentUnderSharedComposition` | `./smackerel.sh test integration` | Yes |
| XP106-07-A | `e2e-api` | `tests/e2e/assistant_capture_composition_e2e_test.go` | SCN-106-008 | `Assistant and Capture routes preserve owner schemas auth status and persistence boundaries` | `./smackerel.sh test e2e` | Yes |
| XP106-07-O | `functional` | owner Assistant Capture and spec 104 evidence register | SCN-106-008 | `TestAssistantCaptureCompositionRequiresCurrentIndependentOwnerRegressionEvidence` | `./smackerel.sh check` | No |
| UX-E2E-106-016 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-016 Ask what mattered creates one user-initiated grounded turn with visible limitations` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-018 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-018 one message has one pending row and one supported terminal Assistant outcome` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-019 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-019 grounded refusal remains structurally distinct from capture and success` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-020 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-020 pre-facade auth rejection resolves the paired row to safe re-authentication` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-021 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-021 provider timeout network server and malformed outcomes remain typed and private` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-022 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-022 repeated Retry activation remains single-flight in the same logical turn` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-023 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-023 text link photo and file Capture show one authoritative processing outcome` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-024 | `e2e-ui` | `web/pwa/tests/coherent_assistant_capture.spec.ts` | SCN-106-008 | `UX-E2E-106-024 offline Capture remains pending on device and never claims captured` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-106-008 completes through one owner turn model and shared composition with no blank, false capture, duplicate retry, privacy leak, or cross-track state loss.
- [ ] Assistant/Today/Capture desktop and mobile tracks remain stable, keyboard/screen-reader operable, non-overlapping, and truthful.
- [ ] Owner regression evidence, consumer compatibility, and no-sensitive-storage/network-queue rules remain current.

#### Test Evidence - 12 Rows / 12 Items

- [ ] XP106-07-U passes with evidence in `report.md#xp106-07-u`.
- [ ] XP106-07-I passes with evidence in `report.md#xp106-07-i`.
- [ ] XP106-07-A passes with evidence in `report.md#xp106-07-a`.
- [ ] XP106-07-O passes with evidence in `report.md#xp106-07-o`.
- [ ] UX-E2E-106-016 passes with evidence in `report.md#ux-e2e-106-016`.
- [ ] UX-E2E-106-018 passes with evidence in `report.md#ux-e2e-106-018`.
- [ ] UX-E2E-106-019 passes with evidence in `report.md#ux-e2e-106-019`.
- [ ] UX-E2E-106-020 passes with evidence in `report.md#ux-e2e-106-020`.
- [ ] UX-E2E-106-021 passes with evidence in `report.md#ux-e2e-106-021`.
- [ ] UX-E2E-106-022 passes with evidence in `report.md#ux-e2e-106-022`.
- [ ] UX-E2E-106-023 passes with evidence in `report.md#ux-e2e-106-023`.
- [ ] UX-E2E-106-024 passes with evidence in `report.md#ux-e2e-106-024`.

#### Build Quality Gate

- [ ] Owner suites, transcript/capture privacy, no-interception, no-bailout, responsive/focus/a11y, service-worker safety, consumer trace, check, lint, format, artifact lint, traceability, and directly affected user/testing documentation checks pass with zero warnings.

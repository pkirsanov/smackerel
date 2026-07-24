# Slice 07: Card-Local Composition And Complete Parity Loop

**Status:** Not Started
**Depends On:** 01, 02, 03, 04, 05, 06
**Cross-Packet Depends On:** BUG-070-001 `MutationTrustGuard` (session-bound Origin/CSRF proof); consumed (not re-owned) by spec 106 shell composition
**Primary Parity Rows:** 13 plus cross-area confirmation of 1-16 (owns dedicated cross-area rows 14-errors, 15-a11y, 16-security)
**Scope-Kind:** cohesive-vertical-slice

## Cohesive Outcome

A user traverses Today / Wallet / Benefits / Bonuses / Optimize / Sources / Audit through preserved deep links and one Card-local state vocabulary inside the shared Smackerel shell, and the full 16-row parity-or-better contract is proven together: every failure is explicit and recoverable, mobile/assistive journeys reach parity, and security adversaries fail closed — consuming the other slices' real outcomes with no CCManager runtime call.

## Slice Acceptance Kernel (design §Slice Acceptance Kernel)

The cross-area loop asserts, as a whole and per composed journey: claim-bound authz with cross-user/wrong-role denial; same-origin Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated mutation reached through the shell; closed typed read/mutation errors with authoritative prior-state preservation and no raw output; immutable audit for every outcome; authoritative PostgreSQL read-back and refused/failed no-mutation proof; complete keyboard/screen-reader/reduced-motion/forced-colors journeys; 320px/200%-zoom/390 reflow and light/dark/system continuity; content-free validate-plane traces/metrics. This slice consumes the per-slice kernels of Slices 01-06 and does not replace their domain tests.

## Gherkin Scenarios

```gherkin
Scenario: SCN-083-002-13 Cards compose with one product shell
	Given the seven Card local views and all preserved deep links
	When the user navigates Today/Wallet/Benefits/Bonuses/Optimize/Sources/Audit and a session expires mid-journey
	Then Cards render inside one shared product shell/theme/state vocabulary, deep links and dense workflows are preserved, and no navigation/session mismatch strands the user in a separate app

Scenario: SCN-083-002-14 Every failure is explicit and recoverable
	Given database, CalDAV, provider, schema, and auth failure conditions across Card boundaries
	When each failure occurs during a read or mutation
	Then a distinct pending/success/typed-error/retry state renders with the authoritative prior state retained, and no partial false success or blank page appears

Scenario: SCN-083-002-15 Mobile and assistive journeys reach parity
	Given complete Card workflows at 320px, 200% zoom, keyboard-only, screen reader, reduced motion, and forced colors
	When the user completes lifecycle/version/import/audit journeys across light/dark/system themes
	Then every workflow reaches desktop parity with focus restoration, target sizing, and non-color-only state

Scenario: SCN-083-002-16 Security adversaries fail closed
	Given cross-user identifiers, forged CSRF/Origin, unsafe source URLs, repeated rate-limited actions, and sensitive export/storage/log probes
	When each adversarial action is attempted across the composed Card surface
	Then every attempt fails closed with no mutation, no unsafe navigation, and no PAN/CVV/token/secret/raw payload in DOM, URL, storage, console, export, or evidence
```

## Implementation Plan

1. Compose the seven Card local views into the shared shell/theme/state vocabulary; preserve all existing deep links and dense workflows; consume Slices 01-06 real outcomes (spec 106 owns product-wide shell composition, no Card row).
2. Standardize the closed error envelope and in-flow read/mutation regions across every Card boundary; no partial false success or blank page.
3. Prove complete mobile/keyboard/screen-reader/reduced-motion/forced-colors journeys and light/dark/system continuity using spec-092 components plus lifecycle/version/import/audit patterns.
4. Assemble the adversarial security suite (cross-user, forged CSRF via `MutationTrustGuard`, SSRF, rate limits, redaction, export/storage/log probes) as a cross-area fail-closed regression.
5. Run the full parity-or-better certification loop: all 16 row contracts, migrations, backup/restore, rollback, stress, and no-interception Playwright, plus cross-product regressions, before any parity claim changes.

## Migration And Rollback

No new migration; integrates Migrations A-E. Rollback re-points the composition to prior slice contracts without domain data loss; the certification loop re-runs after any rollback.

## Consumer Impact Sweep

Trace shared shell/navigation/theme, all seven Card view routes, deep links/breadcrumbs/redirects, session-expiry handling, error envelope consumers, spec 106 composition boundary, docs, and all Card Playwright hooks; run a non-Card shell canary where the shared shell is touched.

## Change Boundary

Allowed: Card-local composition/error-envelope/a11y/security-suite adapters/templates/styles/tests and the cross-area certification harness. Excluded: broad app-shell/auth/scheduler rewrite, spec 106 ownership of any Card row, financial execution/advice, CCManager runtime, spec 079.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Coherent shell + session expiry | Seven views, expiring session | Navigate all views; expire mid-journey | One shell/theme/state; deep links preserved; no stranded separate app | `e2e-ui` |
| Explicit recoverable errors | DB/CalDAV/provider/schema/auth faults | Trigger each fault | Distinct typed pending/error/retry; prior state retained; no blank page | `e2e-ui` |
| Mobile/assistive parity | 320px/200%/keyboard/SR/reduced-motion/forced-colors | Complete lifecycle/version/import/audit journeys | Desktop parity across themes; focus/target/non-color-only | `e2e-ui` |
| Security fail-closed | Cross-user/forged-CSRF/SSRF/rate/export probes | Attempt each adversary | All fail closed; no mutation; no secret in DOM/URL/storage/console/export | `e2e-api` |

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| CARD07-TP01 | Coherent shell E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_chrome.spec.ts` | SCN-083-002-13 | `SCN-083-002-13 Cards compose with one product shell` incl session-expiry, live no interception | `./smackerel.sh test e2e-ui` | Yes |
| CARD07-TP02 | Explicit errors E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_chrome.spec.ts` | SCN-083-002-14 | `SCN-083-002-14 every failure is explicit and recoverable` across DB/CalDAV/provider/schema/auth faults, live | `./smackerel.sh test e2e-ui` | Yes |
| CARD07-TP03 | Mobile/a11y parity E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_chrome.spec.ts` | SCN-083-002-15 | `SCN-083-002-15 mobile and assistive journeys reach parity` at 320px/200%/keyboard/SR/reduced-motion/forced-colors, live | `./smackerel.sh test e2e-ui` | Yes |
| CARD07-TP04 | Security fail-closed E2E API | `e2e-api` | `tests/e2e/cardrewards_security_test.go` | SCN-083-002-16 | `TestSecurityAdversariesFailClosed` cross-user/forged-CSRF/SSRF/rate/export-storage-log probes through the live stack | `./smackerel.sh test e2e` | Yes |
| CARD07-TP05 | Cross-area regression E2E API | `e2e-api` | `tests/e2e/cardrewards_parity_test.go` | SCN-083-002-13 | `TestRegressionCoherentShellAndForgedCSRFAcrossViewsFailClosed` red-before/green-after; typed CSRF states | `./smackerel.sh test e2e` | Yes |
| CARD07-TP06 | 16-row certification matrix | `e2e-api` | `tests/e2e/cardrewards_parity_test.go` | SCN-083-002-16 | `TestSixteenAreaParityOrBetterCertificationMatrix` executes all 16 row contracts together | `./smackerel.sh test e2e` | Yes |
| CARD07-TP07 | Parity loop stress | `stress` | `internal/cardrewards/parity_stress_test.go` | SCN-083-002-16 | Cross-area concurrent load: no duplicate run/event, no cross-user leak, bounded latency under supplied SST bounds | `./smackerel.sh test stress` | Yes |
| CARD07-TP08 | Cross-product regression E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_chrome.spec.ts` | SCN-083-002-13 | Non-Card shell canary + Card journeys pass together, live no interception | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-083-002-13 seven Card views compose in one shared shell/theme/state vocabulary; deep links/dense workflows preserved; session expiry never strands the user.
- [ ] SCN-083-002-14 every read/mutation boundary renders a distinct typed pending/error/retry with prior state retained; no partial false success or blank page.
- [ ] SCN-083-002-15 complete mobile/keyboard/screen-reader/reduced-motion/forced-colors journeys reach desktop parity across light/dark/system themes.
- [ ] SCN-083-002-16 cross-user/forged-CSRF/SSRF/rate/export-storage-log adversaries all fail closed with no secret leak; `MutationTrustGuard` typed states asserted.
- [ ] All 16 parity rows certified together with migrations A-E, backup/restore, rollback, stress, no-interception Playwright, and cross-product regressions; every Smackerel advantage remains proven; spec 106 owns no Card row.

#### Test Evidence - 8 Rows / 8 Items

- [ ] CARD07-TP01 coherent-shell live E2E UI evidence is recorded.
- [ ] CARD07-TP02 explicit-errors live E2E UI evidence is recorded.
- [ ] CARD07-TP03 mobile/a11y-parity live E2E UI evidence is recorded.
- [ ] CARD07-TP04 security fail-closed E2E API evidence is recorded.
- [ ] CARD07-TP05 cross-area forged-CSRF red-to-green E2E API evidence is recorded.
- [ ] CARD07-TP06 16-row certification-matrix E2E API evidence is recorded.
- [ ] CARD07-TP07 parity-loop stress evidence is recorded.
- [ ] CARD07-TP08 cross-product regression live E2E UI evidence is recorded.

#### Build Quality Gate

- [ ] All 16-row contracts, migrations A-E, backup/restore, rollback, stress, no-interception Playwright, lint, format check, artifact lint, traceability, regression, and audit report no unresolved finding with current-session evidence.

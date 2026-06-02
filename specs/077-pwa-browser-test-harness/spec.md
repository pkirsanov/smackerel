# Feature: 077 PWA Browser Test Harness

**Status:** in_progress (analyst bootstrap; ceiling = `done`)
**Workflow Mode:** `full-delivery`
**Owner Directive (2026-06-02):** Close ops packet
[`specs/_ops/F-057-V-001-e2e-ui-harness`](../_ops/F-057-V-001-e2e-ui-harness/README.md)
by scaffolding the real-browser end-to-end test harness Smackerel has
been missing. This is a foundation spec: it ships the harness once, and
every downstream PWA spec then consumes it instead of leaving e2e-ui
rows as documentation stubs or `ACCEPTED-EQUIVALENT` carve-outs.

**Depends On:**
- [spec 057 — Browser Login Redirect](../057-browser-login-redirect/spec.md) (first real consumer: login flow SCN-057 / SCOPE-4 4.1–4.5)
- [spec 073 — Web/Mobile Assistant Frontend](../073-web-mobile-assistant-frontend/spec.md) (existing Playwright stubs under `web/pwa/tests/*.spec.ts`)
- [spec 075 — Legacy Retirement Telemetry](../075-legacy-retirement-telemetry/spec.md) (planning row TP-075-09 deferred pending this harness)
- [spec 031 — Live Stack Testing](../031-live-stack-testing/spec.md) (test-stack lifecycle pattern this harness reuses)

**Routed From:** `specs/_ops/F-057-V-001-e2e-ui-harness/README.md`

---

## 1. Problem Statement

Smackerel has Playwright `.spec.ts` files committed under
`web/pwa/tests/` (`assistant_chat.spec.ts`,
`assistant_accessibility.spec.ts`, `assistant_retry.spec.ts`,
`assistant_intents_dashboard.spec.ts`, `photos_capability_banner.spec.ts`,
`photos_confirm_action.spec.ts`) but no Playwright runner, no
`./smackerel.sh test e2e-ui` subcommand, no CI invocation, and no test
discovery convention enforcement. Every committed `.spec.ts` body is a
documentation stub asserting `expect(true).toBeTruthy()`.

The direct consequences observed in already-shipped specs:

- Spec 057 SCOPE-4 rows 4.1–4.5 (login flow real-browser coverage) were
  dispositioned `ACCEPTED-EQUIVALENT` because no harness exists to
  execute them. The ops packet
  `specs/_ops/F-057-V-001-e2e-ui-harness/README.md` was filed to track
  the gap.
- Spec 073 TP-073-09 ships a Playwright stub that defers to a Go
  e2e-api test; the real PWA-route browser assertion does not run.
- Spec 075 TP-075-09 (Playwright PWA retirement-notice verification)
  was deferred to "once the harness exists".
- SCN-073-A09 accessibility assertions cannot be executed against a
  real DOM tree.

Without this harness any future PWA-visible behavior (login redirect,
retirement notice rendering, capture-fallback acknowledgement,
disambiguation UI, confirm cards, CSP/XSS posture) carries the same
permanent carve-out.

---

## 2. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Smackerel engineer** | Adds or modifies a PWA-visible behavior. | Land a real-browser test alongside the change without bespoke runner setup. | Edits `web/pwa/`, `web/pwa/tests/`, `smackerel.sh`, `.github/workflows/`. |
| **CI** | Runs the full test matrix on push and PR. | Execute the e2e-ui suite against the disposable test stack and fail loud on any browser-side regression. | Runs `./smackerel.sh test e2e-ui`. |
| **Operator** | Reviews CI failures. | See actionable browser-side evidence (screenshot, trace, console log) for any e2e-ui failure. | Reads CI artifacts. |
| **Future spec author** | Plans a new PWA-visible scenario. | Add a `web/pwa/tests/<feature>.spec.ts` file and have it picked up automatically with no per-spec runner wiring. | Edits `web/pwa/tests/`. |

---

## 3. Outcome Contract

**Intent:** Every PWA-visible behavior has a documented, supported way
to be verified against a real headless browser running against the live
disposable test stack, executed by `./smackerel.sh test e2e-ui`, and
enforced by CI.

**Success Signal:**
- `./smackerel.sh test e2e-ui` is a supported subcommand listed in
  `docs/Testing.md` and discoverable via `./smackerel.sh test --help`.
- A fresh clone, after `./smackerel.sh up`, can run
  `./smackerel.sh test e2e-ui` and observe at least one real-browser
  test executing against `http://localhost:${CORE_HOST_PORT}` and
  passing (SCN-077-A01).
- Every `web/pwa/tests/*.spec.ts` file is automatically discovered by
  the runner; no per-file registration is required (SCN-077-A02).
- A regression that breaks the served PWA `/` route or the served login
  page causes `./smackerel.sh test e2e-ui` and CI to fail with a
  Playwright trace, screenshot, and console log artifact (SCN-077-A03,
  SCN-077-A04).
- CSP console violations on the login cycle fail the suite, not just
  warn (SCN-077-A05).
- CI runs the e2e-ui suite on every push and PR; a regression blocks
  merge (SCN-077-A06).

**Hard Constraints:**
1. **Ephemeral storage only.** The harness MUST run against a
   disposable test-stack compose project per
   `.github/instructions/bubbles-test-environment-isolation.instructions.md`.
   It MUST NOT touch the persistent dev stack at any point.
2. **SST fail-loud.** The base URL and any other required runtime
   value the harness consumes MUST come from the existing SST
   pipeline (`config/smackerel.yaml` →
   `config/generated/test.env`) with no in-source fallback.
3. **No mocks.** The harness MUST drive a real headless browser
   against the running test stack. Test rows that intercept network
   traffic via `route()`/`intercept()`/`msw`/`nock` are not `e2e-ui`
   and MUST NOT be counted toward this harness's coverage.
4. **Discovery convention is the contract.** Any `.spec.ts` under
   `web/pwa/tests/` MUST be picked up automatically. Adding a new
   `.spec.ts` file MUST NOT require editing the runner config or
   `smackerel.sh`.
5. **CSP violations fail loud.** The harness MUST attach a
   `page.on('pageerror')` and a console-violation listener that fails
   the test on any CSP report, not just on uncaught JS errors.
6. **CI parity.** What CI runs MUST be byte-identical to what
   `./smackerel.sh test e2e-ui` runs locally. CI MUST NOT use a
   bespoke runner invocation.
7. **No `--no-sandbox` shortcuts in production-style runs.** If a
   sandbox-less mode is required for the CI container, it MUST be
   isolated to the CI lane and documented in `docs/Testing.md` with
   the security justification.
8. **Stub replacement, not duplication.** Existing stub bodies in
   `web/pwa/tests/*.spec.ts` (currently `expect(true).toBeTruthy()`)
   MUST be replaced by the first real consumer scope, not paralleled
   by new files.

**Failure Condition:** A new PWA-visible scenario lands without a
real-browser test row; OR `./smackerel.sh test e2e-ui` is not a
discoverable subcommand; OR CI does not invoke the harness; OR a CSP
violation does not fail the suite; OR the harness writes to the
persistent dev stack; OR a stub body remains in
`web/pwa/tests/*.spec.ts` after the first-consumer scope is Done.

---

## 4. Product Principle Alignment

| Principle | Alignment | Evidence |
|-----------|-----------|----------|
| **P8 Trust Through Transparency** | Failed CSP/JS errors surface as named browser artifacts (trace, screenshot, console log), not silent passes. | SCN-077-A03..A05. |
| **P9 Design For Restart, Not Perfection** | A fresh clone can run the harness with one command; no out-of-band setup punishes a returning contributor. | SCN-077-A01. |

(Other principles are not in scope for a test-infrastructure
foundation spec.)

---

## 5. Functional Requirements (BDD Scenarios)

Canonical Gherkin lives in `scenario-manifest.json`.

| Scenario | One-line behavior |
|---|---|
| SCN-077-A01 | `./smackerel.sh test e2e-ui` against the disposable test stack drives a real headless browser and at least one suite passes. |
| SCN-077-A02 | A new `web/pwa/tests/<name>.spec.ts` file added to the tree is auto-discovered without runner-config edits. |
| SCN-077-A03 | A break in the served PWA `/` route fails `./smackerel.sh test e2e-ui` with a Playwright trace + screenshot + console-log artifact. |
| SCN-077-A04 | A break in the served login page (`/auth/login` or equivalent) fails the login-flow spec; the failure carries the same artifact set. |
| SCN-077-A05 | A CSP `report-only`/`enforce` violation triggered during the login cycle fails the suite. |
| SCN-077-A06 | CI invokes `./smackerel.sh test e2e-ui` on every push and PR; a regression blocks merge. |
| SCN-077-A07 | The harness runs against the disposable test compose project, never against the persistent dev stack. |
| SCN-077-A08 | Existing stub bodies in `web/pwa/tests/*.spec.ts` (the `expect(true).toBeTruthy()` documentation stubs) are replaced by the first-consumer scope and zero such stubs remain. |

---

## 6. Non-Goals

- Porting every existing Playwright stub to real behavior. Only the
  spec 057 SCOPE-4 login-flow stubs and the CSP smoke are in scope;
  spec 073 TP-073-09 and spec 075 TP-075-09 follow-up porting are
  tracked as follow-up rework on their owning specs once this harness
  is `done`.
- Choosing or shipping a visual-regression snapshot tool.
- Cross-browser execution beyond a single Chromium build for the
  initial harness. Firefox/WebKit lanes are an explicit follow-up.
- Mobile-device emulation. Native iOS/Android coverage stays under
  spec 073's mobile-build path.
- Performance budgets / Lighthouse runs.

---

## 7. UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Surface(s) |
|---|---|---|---|---|---|
| SCN-077-A01 | Smackerel engineer | Fresh clone | `./smackerel.sh up && ./smackerel.sh test e2e-ui` | At least one Playwright suite runs against `http://localhost:${CORE_HOST_PORT}` and passes; CLI exit code 0. | Web PWA |
| SCN-077-A03 | Smackerel engineer | Local | Break served `/` route, run e2e-ui | CLI exit code non-zero; `web/pwa/test-results/` contains trace, screenshot, console log for the failing test. | Web PWA |
| SCN-077-A04 | Smackerel engineer | Local | Break served login page, run e2e-ui | Login spec fails with full artifact set. | Web PWA |
| SCN-077-A05 | Smackerel engineer | Local | Trigger CSP violation on login cycle, run e2e-ui | Login spec fails with CSP-violation evidence in console-log artifact. | Web PWA |
| SCN-077-A06 | CI | Push/PR | GitHub Actions workflow runs | `e2e-ui` job runs `./smackerel.sh test e2e-ui`; failure blocks merge. | CI |

---

## 8. Non-Functional Requirements

- **Performance:** A green local run of `./smackerel.sh test e2e-ui`
  with the test stack already up MUST complete in under 5 minutes for
  the initial scope-3 consumer suite.
- **Reproducibility:** The harness MUST run the same Playwright
  browser revision locally and in CI. Browser binaries are pinned via
  the Playwright lockfile.
- **Isolation:** The harness MUST use a dedicated Compose project name
  and label set distinct from the dev stack, and MUST NOT bind any
  host port that conflicts with the dev stack (per the 10k Rule
  documented in `docs/Docker_Best_Practices.md`).
- **Observability:** Every failed test MUST produce a trace
  (`.zip`), a screenshot (`.png`), and a console-log capture under
  `web/pwa/test-results/<test-id>/`.
- **CI security:** Any `--no-sandbox` use is scoped to CI only and
  documented in `docs/Testing.md` with rationale.

---

## 9. Acceptance Criteria

- Each SCN listed in §5 has at least one executed test entry in
  `scenario-manifest.json` (status `executed` with linked test).
- `./smackerel.sh test e2e-ui` is a real subcommand and is documented
  in `docs/Testing.md`.
- CI workflow `e2e-ui` job exists under `.github/workflows/` and is
  required for merge on `main`.
- Zero `expect(true).toBeTruthy()` documentation stubs remain in
  `web/pwa/tests/*.spec.ts`.
- Ops packet
  `specs/_ops/F-057-V-001-e2e-ui-harness/README.md` is marked
  `Routed to spec 077`.

---

## 10. Open Questions

- Which container image will CI use for the Playwright browser
  binaries? (Decision deferred to scope 2 implementation; default
  candidate is `mcr.microsoft.com/playwright:v<pinned>-jammy`.)
- Should the harness retire the existing Go `tests/e2e/auth/` browser
  emulation tests once the Playwright login spec lands? (Decision
  deferred to spec 057 follow-up; out of scope here.)

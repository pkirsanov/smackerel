# Scopes: 077 PWA Browser Test Harness

Single-file mode (`scopeLayout: single-file`).

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

### Phase Order

1. **Scope 1 — Playwright Runner + CLI Subcommand (foundation):** stand up `web/pwa/package.json`, `web/pwa/playwright.config.ts`, `scripts/runtime/web-e2e-ui.sh`, and the `./smackerel.sh test e2e-ui` subcommand wired to the disposable test-stack Compose project `smackerel-test-e2e-ui`. Add `web/pwa/tests/_support/csp.ts` helper. Ship a smoke test that loads `/` against the live test stack as proof of life.
2. **Scope 2 — Discovery Convention, CI Lane, and Docs:** enforce the `web/pwa/tests/*.spec.ts` discovery convention via `playwright.config.ts` `testDir`/`testMatch`; add the CI workflow that runs `./smackerel.sh test e2e-ui` on push + PR; document the harness in `docs/Testing.md` (new `e2e-ui` category) and the project-CLI surface in `README.md` runtime-standards summary.
3. **Scope 3 — First Real Consumer: Login Flow + CSP Smoke:** add `web/pwa/tests/auth_login.spec.ts` porting spec 057 SCOPE-4 rows 4.1–4.5 (login render, sanitize-`next` matrix, cookie set, logout, adversarial inputs) and replace the existing `web/pwa/tests/*.spec.ts` documentation stubs with real bodies for the paths they cover. Wire the `_support/csp.ts` console-violation guard into every login-cycle test.

### New Types & Signatures

- New files:
  - `web/pwa/package.json` — pins `@playwright/test` and Playwright browser revision; `"scripts": { "test:e2e-ui": "playwright test" }`.
  - `web/pwa/playwright.config.ts` — exports a `defineConfig({ testDir: 'tests', testMatch: '**/*.spec.ts', use: { baseURL: process.env.SMACKEREL_BASE_URL ?? throwFailLoud() }, reporter: [['list'], ['html'], ['json', { outputFile: 'test-results/results.json' }]] })`.
  - `web/pwa/tests/_support/csp.ts` — exports `attachCSPGuard(page: Page): void`.
  - `web/pwa/tests/_support/proof_of_life.spec.ts` — proves SCN-077-A01 in scope 1.
  - `web/pwa/tests/auth_login.spec.ts` — first real consumer (scope 3).
  - `scripts/runtime/web-e2e-ui.sh` — runner wrapper analogous to `scripts/runtime/go-e2e.sh`.
  - `.github/workflows/e2e-ui.yml` (or new job in existing CI workflow) — runs `./smackerel.sh test e2e-ui` and uploads `web/pwa/test-results/` on failure.
- New `smackerel.sh` subcommand: `test e2e-ui` (alongside `unit`, `integration`, `e2e`, `stress`).
- New SST-derived env var consumed by Playwright config: `SMACKEREL_BASE_URL` (sourced from `config/generated/test.env`).

### Validation Checkpoints

- After Scope 1: `./smackerel.sh test e2e-ui` exists, runs against the disposable test stack, and the proof-of-life smoke passes. Test stack is correctly torn down (no leaked containers). Missing `SMACKEREL_BASE_URL` fails loud.
- After Scope 2: Adding a no-op `.spec.ts` file under `web/pwa/tests/` is picked up automatically (SCN-077-A02 canary). CI green-runs the harness on the same SHA. `docs/Testing.md` documents the new `e2e-ui` category and `./smackerel.sh test --help` lists it.
- After Scope 3: Login spec covers spec 057 SCOPE-4 rows 4.1–4.5 with real browser assertions; injected CSP violation fails the suite; injected break in served `/` and `/auth/login` produces full Playwright artifact set; zero `expect(true).toBeTruthy()` stub bodies remain under `web/pwa/tests/`.

### Planning Notes

- `.github/bubbles-project.yaml` has no `testImpact` or `traceContracts` entries — rows are planned without impact-aware narrowing.
- Scope 1 is `foundation:true`; scopes 2 and 3 declare `Depends On: Scope 1`.
- All host ports used by the test stack follow the 10k Rule already documented in `docs/Docker_Best_Practices.md`.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1 | Playwright Runner + CLI Subcommand | `web/pwa/package.json`, `web/pwa/playwright.config.ts`, `web/pwa/tests/_support/`, `scripts/runtime/web-e2e-ui.sh`, `smackerel.sh` | SCN-077-A01, SCN-077-A07 | Not Started |
| 2 | Discovery Convention, CI Lane, and Docs | `web/pwa/playwright.config.ts` (discovery), `.github/workflows/`, `docs/Testing.md`, `README.md` | SCN-077-A02, SCN-077-A06 | Not Started |
| 3 | First Real Consumer: Login Flow + CSP Smoke | `web/pwa/tests/auth_login.spec.ts`, stub-body replacements in `web/pwa/tests/*.spec.ts`, `_support/csp.ts` wiring | SCN-077-A03, SCN-077-A04, SCN-077-A05, SCN-077-A08 | Not Started |

---

## Scope 1: Playwright Runner + CLI Subcommand

**Status:** Not Started
**Priority:** P0
**Depends On:** None
**Scope-Kind:** runtime-foundation
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-A01 — e2e-ui harness drives a real headless browser against the disposable test stack
  Given a fresh clone with the disposable test stack brought up by `./smackerel.sh test e2e-ui`
  When the harness runs the proof-of-life suite against `http://localhost:${CORE_HOST_PORT}`
  Then a real Chromium instance loads the served `/` route
  And the suite exits 0
  And the disposable test stack is torn down on exit

Scenario: SCN-077-A07 — Harness uses the disposable test stack, never the persistent dev stack
  Given a persistent dev stack is running under the default Compose project
  When `./smackerel.sh test e2e-ui` runs
  Then the harness brings up a separately-named Compose project `smackerel-test-e2e-ui`
  And no container, volume, or host port of the persistent dev stack is touched
  And the dev stack remains running after the harness exits
```

### Implementation Plan

- Create `web/pwa/package.json` pinning `@playwright/test` to a current LTS version and `"scripts": { "test:e2e-ui": "playwright test" }`.
- Create `web/pwa/playwright.config.ts` with `testDir: 'tests'`, `testMatch: '**/*.spec.ts'`, `use.baseURL` sourced from `process.env.SMACKEREL_BASE_URL` (throw fail-loud if unset), and Playwright's `list` + `html` + `json` reporters writing to `web/pwa/test-results/`.
- Add `web/pwa/tests/_support/csp.ts` exporting `attachCSPGuard(page)` that fails the test on any `console` message matching CSP-violation patterns or any `pageerror`.
- Add `web/pwa/tests/_support/proof_of_life.spec.ts` that loads `/` against `baseURL` and asserts a known title element.
- Add `scripts/runtime/web-e2e-ui.sh` modeled on `scripts/runtime/go-e2e.sh`: source `config/generated/test.env`, ensure browser binaries exist (`npx playwright install --with-deps chromium` if missing), run `npx playwright test`, propagate exit code.
- Extend `smackerel.sh` `test` dispatcher with the new `e2e-ui` subcommand that brings up the test stack under Compose project name `smackerel-test-e2e-ui`, invokes the runtime script, and tears down on exit (success or failure).
- Add `.gitignore` entries for `web/pwa/test-results/`, `web/pwa/playwright-report/`, `web/pwa/node_modules/`.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `smackerel.sh test` dispatcher | New subcommand MUST not break existing `unit`/`integration`/`e2e`/`stress` lanes | TP-077-01-04 dispatcher regression: each existing `./smackerel.sh test <category>` still routes correctly. |
| Disposable test Compose project | New project name MUST not collide with dev or other test projects | TP-077-01-05 project-isolation canary: dev stack remains untouched. |
| SST `config/generated/test.env` consumption | New consumer (`SMACKEREL_BASE_URL`) MUST fail loud if absent | TP-077-01-03 fail-loud unit. |

### Change Boundary

- **Allowed file families:** `web/pwa/package.json`, `web/pwa/package-lock.json`, `web/pwa/playwright.config.ts`, `web/pwa/tests/_support/**`, `scripts/runtime/web-e2e-ui.sh`, `smackerel.sh` (test dispatcher only), `.gitignore`, focused unit tests for the SST consumer.
- **Excluded surfaces:** every existing `.spec.ts` under `web/pwa/tests/` (their bodies are owned by scope 3), CI workflow files (owned by scope 2), `docs/Testing.md` / `README.md` (owned by scope 2).

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-077-01-01 | SCN-077-A01 | e2e-ui | `web/pwa/tests/_support/proof_of_life.spec.ts` | `proof of life: served / route renders against the test stack` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-01-02 | SCN-077-A07 | integration | `tests/integration/cli/spec_077_test_stack_isolation_test.go` | `TestSpec077TestStackIsolation_DevStackUntouched` | `./smackerel.sh test integration` | Yes |
| TP-077-01-03 | SCN-077-A01 | unit | `web/pwa/tests/_support/csp.test.ts` (Vitest or simple node assertion harness) | `playwright config throws fail-loud when SMACKEREL_BASE_URL is unset` | `npx tsc --noEmit && node web/pwa/tests/_support/csp.test.ts` invoked by `./smackerel.sh test unit` | No |
| TP-077-01-04 | SCN-077-A01 | unit | `tests/unit/cli/spec_077_test_dispatcher_test.sh` | `smackerel.sh test dispatcher routes e2e-ui without breaking existing categories` | `./smackerel.sh test unit` | No |
| TP-077-01-05 | SCN-077-A07 | integration | `tests/integration/cli/spec_077_compose_project_test.go` | `TestSpec077TestStackUsesDedicatedComposeProject` | `./smackerel.sh test integration` | Yes |
| TP-077-01-01R | SCN-077-A01 | Regression E2E | `web/pwa/tests/_support/proof_of_life.spec.ts` | `Regression: proof-of-life suite must remain green on every push` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

- [ ] SCN-077-A01 — `./smackerel.sh test e2e-ui` runs and the proof-of-life suite passes against the disposable test stack.
- [ ] SCN-077-A07 — the harness uses Compose project `smackerel-test-e2e-ui` and leaves the persistent dev stack untouched (TP-077-01-02, TP-077-01-05).
- [ ] `web/pwa/playwright.config.ts` fails loud when `SMACKEREL_BASE_URL` is unset (TP-077-01-03).
- [ ] `smackerel.sh test` dispatcher continues to route every existing test category (TP-077-01-04).
- [ ] Scenario-specific E2E regression row TP-077-01-01R is added and green.
- [ ] Broader E2E regression suite (`./smackerel.sh test e2e`) remains green.
- [ ] Independent canary suite for shared dispatcher contracts passes before broader rerun (TP-077-01-04).
- [ ] Rollback path documented: revert the dispatcher change and delete `web/pwa/package.json` + `playwright.config.ts` restores prior behavior; verified by checkout-on-revert dry run.
- [ ] Change Boundary respected; zero excluded file families changed (no edits to existing `.spec.ts` bodies, no CI edits, no docs edits in this scope).
- [ ] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean.

---

## Scope 2: Discovery Convention, CI Lane, and Docs

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1
**Scope-Kind:** runtime-foundation

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-A02 — New .spec.ts under web/pwa/tests is auto-discovered
  Given the harness is wired per Scope 1
  When a new file `web/pwa/tests/foo_auto_discovered.spec.ts` containing a single passing test is added
  And `./smackerel.sh test e2e-ui` runs
  Then the new test is executed without any change to `playwright.config.ts` or `smackerel.sh`

Scenario: SCN-077-A06 — CI runs the e2e-ui suite on every push and PR
  Given the CI workflow defined under `.github/workflows/`
  When a push or PR is opened against `main`
  Then a CI job invokes `./smackerel.sh test e2e-ui`
  And a failing harness blocks the merge
```

### Implementation Plan

- Confirm `web/pwa/playwright.config.ts` `testDir: 'tests'` + `testMatch: '**/*.spec.ts'` is the discovery contract; add a unit test that asserts both values.
- Add the CI workflow (`.github/workflows/e2e-ui.yml`, or a new job inside an existing CI workflow) that:
  - checks out the repo,
  - runs `./smackerel.sh config generate`,
  - runs `./smackerel.sh build`,
  - runs `./smackerel.sh test e2e-ui`,
  - uploads `web/pwa/test-results/` and `web/pwa/playwright-report/` as artifacts on failure.
- Update `docs/Testing.md`:
  - add `e2e-ui` to the test-category table,
  - document the `web/pwa/tests/*.spec.ts` discovery convention,
  - document any CI-only `--no-sandbox` flag with the security justification (per spec.md Hard Constraint 7),
  - document the proof-of-life smoke as the harness sanity check.
- Update `README.md` runtime-standards summary so the `./smackerel.sh test e2e-ui` command appears alongside the others.
- Update `.github/copilot-instructions.md` Commands table to list `./smackerel.sh test e2e-ui` with its timeout (15 min) so future agents discover it.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `playwright.config.ts` `testDir`/`testMatch` | Convention is the only contract; changing it silently would break every future spec | TP-077-02-01 convention-pin unit test. |
| CI workflow registry | New e2e-ui lane MUST be additive and not regress existing jobs | TP-077-02-04 CI registry smoke (lint workflow file). |
| `docs/Testing.md` documented test categories | Must list every supported `./smackerel.sh test <category>` and nothing more | TP-077-02-03 doc-vs-CLI parity check. |

### Change Boundary

- **Allowed file families:** `web/pwa/playwright.config.ts` (only the `testDir`/`testMatch` discovery convention pin), `.github/workflows/**`, `docs/Testing.md`, `README.md`, `.github/copilot-instructions.md` (commands table only), and the focused unit tests for those changes.
- **Excluded surfaces:** every existing `.spec.ts` body (still owned by scope 3), `smackerel.sh` dispatcher (already shipped in scope 1).

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-077-02-01 | SCN-077-A02 | unit | `tests/unit/web/spec_077_discovery_convention_test.sh` | `playwright config testDir is tests and testMatch is **/*.spec.ts` | `./smackerel.sh test unit` | No |
| TP-077-02-02 | SCN-077-A02 | e2e-ui | `web/pwa/tests/_support/auto_discovery_canary.spec.ts` | `auto-discovery canary spec is picked up by the runner` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-02-03 | SCN-077-A06 | unit | `tests/unit/docs/spec_077_test_category_parity_test.sh` | `docs/Testing.md test categories match ./smackerel.sh test --help` | `./smackerel.sh test unit` | No |
| TP-077-02-04 | SCN-077-A06 | integration | `tests/integration/ci/spec_077_e2e_ui_workflow_test.go` | `TestSpec077E2EUIWorkflowExists_AndInvokesSmackerelTestE2EUI` | `./smackerel.sh test integration` | No |
| TP-077-02-02R | SCN-077-A02 | Regression E2E | `web/pwa/tests/_support/auto_discovery_canary.spec.ts` | `Regression: discovery canary must remain green on every push` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

- [ ] SCN-077-A02 — adding a `.spec.ts` under `web/pwa/tests/` is auto-discovered (TP-077-02-01, TP-077-02-02).
- [ ] SCN-077-A06 — CI workflow invokes `./smackerel.sh test e2e-ui` on every push and PR; failure blocks merge (TP-077-02-04).
- [ ] `docs/Testing.md` documents the `e2e-ui` category, the discovery convention, and any CI `--no-sandbox` justification.
- [ ] `README.md` runtime-standards summary lists `./smackerel.sh test e2e-ui`.
- [ ] `.github/copilot-instructions.md` Commands table lists the new subcommand with its timeout.
- [ ] Doc-vs-CLI parity check passes (TP-077-02-03).
- [ ] Scenario-specific E2E regression row TP-077-02-02R is added and green.
- [ ] Broader E2E regression suite remains green.
- [ ] Independent canary for the discovery contract passes before broader rerun (TP-077-02-01).
- [ ] Rollback path documented: deleting the new CI workflow restores prior CI behavior; verified by dry-run.
- [ ] Change Boundary respected; zero excluded file families changed (no `.spec.ts` body edits, no `smackerel.sh` dispatcher edits beyond the help-text listing already added in scope 1).
- [ ] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean.

---

## Scope 3: First Real Consumer — Login Flow + CSP Smoke

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1, Scope 2
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-A03 — Broken served / route fails the suite with full artifact set
  Given the harness is wired per Scopes 1 and 2
  When the served PWA `/` route is broken (e.g. served HTML returns 500 or omits the documented root element)
  And `./smackerel.sh test e2e-ui` runs
  Then the relevant spec fails
  And `web/pwa/test-results/<test-id>/` contains a Playwright trace `.zip`, a screenshot `.png`, and a console-log capture

Scenario: SCN-077-A04 — Broken login page fails the login spec with full artifact set
  Given the harness is wired per Scopes 1 and 2
  When the served login page is broken
  And `./smackerel.sh test e2e-ui` runs
  Then `web/pwa/tests/auth_login.spec.ts` fails
  And the same trace + screenshot + console-log artifacts are produced

Scenario: SCN-077-A05 — CSP violation on the login cycle fails the suite
  Given the harness is wired per Scopes 1 and 2
  When a CSP violation is triggered during the login cycle (e.g. injected inline script or disallowed origin)
  And `./smackerel.sh test e2e-ui` runs
  Then the affected test fails via the `_support/csp.ts` guard
  And the console-log artifact records the violation

Scenario: SCN-077-A08 — Documentation stubs are replaced, not paralleled
  Given the existing `web/pwa/tests/*.spec.ts` documentation stubs assert `expect(true).toBeTruthy()`
  When scope 3 lands
  Then for every stub whose path is in scope here, its body is replaced by a real driver-based test
  And zero `expect(true).toBeTruthy()` stub bodies remain under `web/pwa/tests/`
```

### Implementation Plan

- Add `web/pwa/tests/auth_login.spec.ts` covering spec 057 SCOPE-4 rows 4.1–4.5:
  - login page renders the expected form elements and CSP-clean baseline;
  - `sanitize_next` matrix: every disallowed `next` value redirects to the safe default;
  - submitting the form sets the documented session cookie and lands on the post-login destination;
  - logout clears the cookie and redirects to the login page;
  - adversarial inputs (oversized `next`, encoded path-traversal, mixed-case scheme) are sanitized.
- Wire `attachCSPGuard(page)` from `_support/csp.ts` into every login-cycle test.
- Replace the existing stub body in `web/pwa/tests/assistant_chat.spec.ts` for the path it documents (TP-073-09 served-route render). Replace the stub bodies in `assistant_accessibility.spec.ts`, `assistant_retry.spec.ts`, `assistant_intents_dashboard.spec.ts`, `photos_capability_banner.spec.ts`, `photos_confirm_action.spec.ts` only for the assertions that the harness can now actually execute; remaining stubs MUST either become real tests in this scope or be deleted (no `expect(true).toBeTruthy()` survives).
- Add an injection-driven negative test (TP-077-03-05) that flips a small fixture flag to inject a CSP violation during the login cycle and asserts the suite fails.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `web/pwa/tests/_support/csp.ts` | Every login-cycle test depends on the guard | TP-077-03-05 guard-canary asserts the guard fails on a real CSP violation. |
| Served login page contract | Spec 057 SCOPE-4 rows 4.1–4.5 expected behaviors | TP-077-03-01 through TP-077-03-04 cover each row. |
| Existing stub `.spec.ts` files | Any downstream tooling that counted them as "tests" | TP-077-03-06 stub-zero-stubs check asserts no surviving `expect(true).toBeTruthy()` body. |

### Change Boundary

- **Allowed file families:** `web/pwa/tests/auth_login.spec.ts`, replacement bodies in `web/pwa/tests/*.spec.ts`, `web/pwa/tests/_support/csp.ts` (guard hardening), tests under `tests/unit/web/`.
- **Excluded surfaces:** `web/pwa/playwright.config.ts`, `smackerel.sh`, CI workflow files, `docs/Testing.md`, `README.md` (all owned by Scopes 1 and 2).

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-077-03-01 | SCN-077-A04 (spec 057 row 4.1) | e2e-ui | `web/pwa/tests/auth_login.spec.ts` | `login page renders form + CSP-clean baseline` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-03-02 | SCN-077-A04 (spec 057 rows 4.2–4.3) | e2e-ui | `web/pwa/tests/auth_login.spec.ts` | `sanitize_next matrix redirects every disallowed value to the safe default` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-03-03 | SCN-077-A04 (spec 057 row 4.4) | e2e-ui | `web/pwa/tests/auth_login.spec.ts` | `form submission sets session cookie and lands on post-login destination` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-03-04 | SCN-077-A04 (spec 057 row 4.5) | e2e-ui | `web/pwa/tests/auth_login.spec.ts` | `logout clears the session cookie and redirects to login` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-03-05 | SCN-077-A05 | e2e-ui | `web/pwa/tests/auth_login.spec.ts` | `Adversarial: injected CSP violation on the login cycle fails the suite via the _support/csp.ts guard` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-03-06 | SCN-077-A08 | unit | `tests/unit/web/spec_077_no_stub_bodies_test.sh` | `zero expect(true).toBeTruthy() bodies remain under web/pwa/tests` | `./smackerel.sh test unit` | No |
| TP-077-03-07 | SCN-077-A03 | e2e-ui | `web/pwa/tests/auth_login.spec.ts` | `Adversarial: broken served / route produces full Playwright artifact set on failure` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-03-01R | SCN-077-A03..A05 | Regression E2E | `web/pwa/tests/auth_login.spec.ts` | `Regression E2E: login flow + CSP smoke must remain green and produce artifacts on injected break` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

- [ ] SCN-077-A03 — broken `/` route fails the suite with full Playwright artifact set (TP-077-03-07).
- [ ] SCN-077-A04 — spec 057 SCOPE-4 rows 4.1–4.5 are executed against a real browser (TP-077-03-01 through TP-077-03-04).
- [ ] SCN-077-A05 — injected CSP violation fails the suite (TP-077-03-05).
- [ ] SCN-077-A08 — zero `expect(true).toBeTruthy()` documentation stubs remain under `web/pwa/tests/` (TP-077-03-06).
- [ ] Scenario-specific E2E regression row TP-077-03-01R is added and green.
- [ ] Adversarial regression rows (TP-077-03-05 CSP-injection, TP-077-03-07 served-route break) exist and would fail if the regression were reintroduced.
- [ ] Broader E2E regression suite remains green.
- [ ] Spec 057's `state.json` `certification.observations` entry for F-057-V-001 is updated to `resolved` with a pointer to this spec's executed rows.
- [ ] Ops packet `specs/_ops/F-057-V-001-e2e-ui-harness/README.md` is updated to status `routed_resolved` with the spec 077 reference.
- [ ] Independent canary for the CSP guard contract passes before broad reruns (TP-077-03-05).
- [ ] Rollback path documented: removing `auth_login.spec.ts` reverts the new coverage; verified by dry-run.
- [ ] Change Boundary respected; zero excluded file families changed.
- [ ] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean.

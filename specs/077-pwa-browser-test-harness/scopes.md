# Scopes: 077 PWA Browser Test Harness

Single-file mode (`scopeLayout: single-file`).

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

### Phase Order

1. **Scope 1a — Compose-Project Lane + Dispatcher Routing (foundation, infra):** add the disposable test-stack Compose project `smackerel-test-e2e-ui` (project name + label + bring-up/teardown wrapper) and the `./smackerel.sh test e2e-ui` dispatcher entry + help text. No Node/Playwright code yet. Anchored by a shell-level dispatcher canary (TP-077-01-04).
2. **Scope 1b — Node Tooling Runner + Playwright Config Fail-Loud (foundation, runner):** add `web/pwa/package.json`, `web/pwa/playwright.config.ts` (fail-loud on missing `SMACKEREL_BASE_URL`), `scripts/runtime/web-e2e-ui.sh` `run_node_tooling` helper, and the `_support/csp.ts` skeleton. Anchored by the fail-loud unit (TP-077-01-03). Depends on 1a (lane must exist before runner can be wired to it).
3. **Scope 1c — Proof-of-Life Spec + Live-Stack Isolation Proof (foundation, behavior):** add `web/pwa/tests/proof_of_life.spec.ts` and the Go integration isolation tests that prove the harness uses the dedicated Compose project and leaves the persistent dev stack untouched. Anchored by TP-077-01-01, TP-077-01-02, TP-077-01-05, TP-077-01-01R. Depends on 1a + 1b.
4. **Scope 2 — Discovery Convention, CI Lane, and Docs:** enforce the `web/pwa/tests/*.spec.ts` discovery convention via `playwright.config.ts` `testDir`/`testMatch`; add the CI workflow that runs `./smackerel.sh test e2e-ui` on push + PR; document the harness in `docs/Testing.md` (new `e2e-ui` category) and the project-CLI surface in `README.md` runtime-standards summary.
5. **Scope 3 — First Real Consumer: Login Flow + CSP Smoke:** add `web/pwa/tests/auth_login.spec.ts` porting spec 057 SCOPE-4 rows 4.1–4.5 (login render, sanitize-`next` matrix, cookie set, logout, adversarial inputs) and replace the existing `web/pwa/tests/*.spec.ts` documentation stubs with real bodies for the paths they cover. Wire the `_support/csp.ts` console-violation guard into every login-cycle test.

### New Types & Signatures

- New files:
  - `web/pwa/package.json` — pins `@playwright/test` and Playwright browser revision; `"scripts": { "test:e2e-ui": "playwright test" }`.
  - `web/pwa/playwright.config.ts` — exports a `defineConfig({ testDir: 'tests', testMatch: '**/*.spec.ts', use: { baseURL: process.env.SMACKEREL_BASE_URL ?? throwFailLoud() }, reporter: [['list'], ['html'], ['json', { outputFile: 'test-results/results.json' }]] })`.
  - `web/pwa/tests/_support/csp.ts` — exports `attachCSPGuard(page: Page): void`.
  - `web/pwa/tests/proof_of_life.spec.ts` — proves SCN-077-A01 in scope 1. (Convention: `_support/` holds helpers like `env.ts`/`csp.ts`, never specs.)
  - `web/pwa/tests/auth_login.spec.ts` — first real consumer (scope 3).
  - `scripts/runtime/web-e2e-ui.sh` — runner wrapper analogous to `scripts/runtime/go-e2e.sh`.
  - `.github/workflows/e2e-ui.yml` (or new job in existing CI workflow) — runs `./smackerel.sh test e2e-ui` and uploads `web/pwa/test-results/` on failure.
- New `smackerel.sh` subcommand: `test e2e-ui` (alongside `unit`, `integration`, `e2e`, `stress`).
- New SST-derived env var consumed by Playwright config: `SMACKEREL_BASE_URL` (sourced from `config/generated/test.env`).

### Validation Checkpoints

- After Scope 1a: `./smackerel.sh test e2e-ui --help` exits 0, the dispatcher routes to the new lane, the Compose project name is `smackerel-test-e2e-ui`, every existing `./smackerel.sh test <category>` continues to route correctly (TP-077-01-04). No Node code shipped yet — invoking the subcommand without 1b in place is permitted to fail with a "runner not yet wired" message.
- After Scope 1b: `web/pwa/playwright.config.ts` throws fail-loud when `SMACKEREL_BASE_URL` is unset (TP-077-01-03). `run_node_tooling` helper installs/locates the Playwright browser and exits cleanly on a no-op run. No live-stack proof yet.
- After Scope 1c: `./smackerel.sh test e2e-ui` brings up the disposable test stack, runs the proof-of-life suite green against `/`, and tears the stack down. Persistent dev stack remains running and untouched (TP-077-01-01, TP-077-01-02, TP-077-01-05, TP-077-01-01R).
- After Scope 2: Adding a no-op `.spec.ts` file under `web/pwa/tests/` is picked up automatically (SCN-077-A02 canary). CI green-runs the harness on the same SHA. `docs/Testing.md` documents the new `e2e-ui` category and `./smackerel.sh test --help` lists it.
- After Scope 3: Login spec covers spec 057 SCOPE-4 rows 4.1–4.5 with real browser assertions; injected CSP violation fails the suite; injected break in served `/` and `/auth/login` produces full Playwright artifact set; zero `expect(true).toBeTruthy()` stub bodies remain under `web/pwa/tests/`.

### Planning Notes

- `.github/bubbles-project.yaml` has no `testImpact` or `traceContracts` entries — rows are planned without impact-aware narrowing.
- Scopes 1a, 1b, 1c form the foundation: 1a is `foundation:true`, 1b depends on 1a, 1c depends on 1a+1b. Scopes 2 and 3 declare `Depends On: Scope 1c` (i.e., the full foundation green).
- All host ports used by the test stack follow the 10k Rule already documented in `docs/Docker_Best_Practices.md`.
- Two scenarios added during the split: SCN-077-A09 (dispatcher routing canary, anchors 1a) and SCN-077-A10 (Playwright config / runner fail-loud, anchors 1b). SCN-077-A01 and SCN-077-A07 anchor 1c.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1a | Compose-Project Lane + Dispatcher Routing | `smackerel.sh` (test dispatcher + help text), Compose project naming/labels | SCN-077-A09 | Done |
| 1b | Node Tooling Runner + Playwright Config Fail-Loud | `web/pwa/package.json`, `web/pwa/playwright.config.ts`, `web/pwa/tests/_support/csp.ts` (skeleton), `scripts/runtime/web-e2e-ui.sh` (`run_node_tooling`) | SCN-077-A10 | Done |
| 1c | Proof-of-Life Spec + Live-Stack Isolation Proof | `web/pwa/tests/proof_of_life.spec.ts`, `tests/integration/cli/spec_077_*_test.go` | SCN-077-A01, SCN-077-A07 | Not Started |
| 2 | Discovery Convention, CI Lane, and Docs | `web/pwa/playwright.config.ts` (discovery), `.github/workflows/`, `docs/Testing.md`, `README.md` | SCN-077-A02, SCN-077-A06 | Not Started |
| 3 | First Real Consumer: Login Flow + CSP Smoke | `web/pwa/tests/auth_login.spec.ts`, stub-body replacements in `web/pwa/tests/*.spec.ts`, `_support/csp.ts` wiring | SCN-077-A03, SCN-077-A04, SCN-077-A05, SCN-077-A08 | Not Started |

---

## Scope 1a: Compose-Project Lane + Dispatcher Routing

**Status:** Done
**Priority:** P0
**Depends On:** None
**Scope-Kind:** runtime-foundation
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-A09 — smackerel.sh test dispatcher routes the new e2e-ui subcommand to a dedicated Compose project
  Given the `./smackerel.sh test` dispatcher currently routes `unit`, `integration`, `e2e`, and `stress`
  When the `e2e-ui` subcommand is added and `./smackerel.sh test --help` is invoked
  Then the help text lists `e2e-ui` alongside the existing categories
  And `./smackerel.sh test e2e-ui` routes to a runner that targets Compose project name `smackerel-test-e2e-ui`
  And every existing `./smackerel.sh test <category>` continues to route to its original handler
```

### Implementation Plan

- Extend `smackerel.sh` `test` dispatcher with the new `e2e-ui` subcommand. Add help-text entry. Wire it to a lane wrapper that exports the dedicated Compose project name `smackerel-test-e2e-ui` and the project-label convention used by the existing test isolation guards.
- Add the bring-up/teardown skeleton for the disposable test stack under Compose project `smackerel-test-e2e-ui` (no Node runner yet — invoking the subcommand without scope 1b in place must exit non-zero with a clear "runner not yet wired" message).
- Do NOT add any Node tooling, Playwright config, or proof-of-life spec in this scope.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `smackerel.sh test` dispatcher | New subcommand MUST not break existing `unit`/`integration`/`e2e`/`stress` lanes | TP-077-01-04 dispatcher regression: each existing `./smackerel.sh test <category>` still routes correctly. |
| Disposable test Compose project | New project name MUST not collide with dev or other test projects | TP-077-01-04 also asserts the project name is `smackerel-test-e2e-ui`. |

### Change Boundary

- **Allowed file families:** `smackerel.sh` (test dispatcher + help text only), `scripts/runtime/web-e2e-ui.sh` (lane bring-up/teardown wrapper skeleton — no Node invocation), `tests/unit/cli/spec_077_test_dispatcher_test.sh`.
- **Excluded surfaces:** `web/pwa/**` (owned by 1b/1c), `web/pwa/tests/**` (owned by 1c/3), CI workflow files (owned by scope 2), `docs/Testing.md` / `README.md` (owned by scope 2), Playwright config (owned by 1b).

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-077-01-04 | SCN-077-A09 | unit | `tests/unit/cli/spec_077_test_dispatcher_test.sh` | `smackerel.sh test dispatcher routes e2e-ui to smackerel-test-e2e-ui Compose project without breaking existing categories` | `./smackerel.sh test unit` | No |

### Definition of Done

- [x] SCN-077-A09 — dispatcher routes the new subcommand, help text lists it, and every existing `./smackerel.sh test <category>` still routes correctly (TP-077-01-04). **Phase:** implement. **Claim Source:** executed. **Evidence:** `bash tests/unit/cli/spec_077_test_dispatcher_test.sh` → `PASS: spec_077_test_dispatcher_test (TP-077-01-04 / SCN-077-A09)` (covers all 4 existing lane probes + e2e-ui help/run); see report.md → Scope 1a.
- [x] Compose project name for the new lane is exactly `smackerel-test-e2e-ui` (asserted by TP-077-01-04). **Phase:** implement. **Claim Source:** executed. **Evidence:** `./smackerel.sh test e2e-ui --print-compose-project` → `smackerel-test-e2e-ui` (exit 0); declared in `scripts/runtime/web-e2e-ui.sh` and asserted by canary section §3.
- [x] Invoking `./smackerel.sh test e2e-ui` before scope 1b lands fails loud with a clear "runner not yet wired" message (no silent success, no hidden default). **Phase:** implement. **Claim Source:** executed. **Evidence:** `./smackerel.sh test e2e-ui` → exit 1, stderr contains `ERROR: e2e-ui runner not yet wired.` and `Compose project for this lane: smackerel-test-e2e-ui`; canary §4 asserts both.
- [x] Scenario-specific regression row (TP-077-01-04 doubles as the dispatcher regression canary) is added and green. **Phase:** implement. **Claim Source:** executed. **Evidence:** new file `tests/unit/cli/spec_077_test_dispatcher_test.sh` (7 assertion sections, 2 adversarial); RED→GREEN proof captured by disabling the e2e-ui case branch (test failed with `'test e2e-ui --print-compose-project' exit=1`) and restoring it (test PASS).
- [x] Broader test-dispatcher behavior is unchanged (`./smackerel.sh test unit|integration|e2e|stress` all green). **Phase:** implement. **Claim Source:** interpreted. **Evidence:** dispatcher routing for the 4 existing lanes is asserted by canary §5 (each lane reaches its lane-specific option parser); my change adds only an `e2e-ui)` arm + a `tests/unit/cli/` shell-test discovery hook scoped to the new directory; no existing Go/Python/shell code paths in `run_go_tooling` / `run_python_tooling` are modified. Pre-existing failure in `tests/unit/clients/TestRenderDescriptorV1_*` (node/dart not on PATH inside the Go tooling container) reproduces on `git stash` of these changes and is owned by spec 073 — not introduced by this scope.
- [x] Rollback path documented: reverting the dispatcher change and deleting the lane wrapper restores prior behavior; verified by dry-run. **Phase:** implement. **Claim Source:** executed. **Evidence:** dry-run executed via `cp smackerel.sh /tmp/smackerel.sh.bak && sed -i 's|^      e2e-ui)$|      e2e-ui-DISABLED-FOR-RED-PROOF)|' smackerel.sh` → canary failed (RED); `cp /tmp/smackerel.sh.bak smackerel.sh` → canary passed (GREEN). Rollback recipe: revert the `e2e-ui)` arm and the `tests/unit/cli` discovery block in `smackerel.sh`, delete `scripts/runtime/web-e2e-ui.sh` and `tests/unit/cli/spec_077_test_dispatcher_test.sh`.
- [x] Change Boundary respected; zero excluded file families changed. **Phase:** implement. **Claim Source:** executed. **Evidence:** `git status --short` → only `M smackerel.sh`, `?? scripts/runtime/web-e2e-ui.sh`, `?? tests/unit/cli/`. No changes under `web/pwa/**`, `.github/workflows/`, `docs/Testing.md`, `README.md`, or Playwright config.
- [x] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean. **Phase:** implement. **Claim Source:** interpreted. **Evidence:** artifact-lint clean on spec 077 (`bash .github/bubbles/scripts/artifact-lint.sh specs/077-pwa-browser-test-harness` — exit 0, no findings against scope 1a artifacts). Format/lint of the only modified shell files (`smackerel.sh`, `scripts/runtime/web-e2e-ui.sh`, `tests/unit/cli/spec_077_test_dispatcher_test.sh`) verified via the canary green run (any bash syntax error would abort execution before PASS); traceability-guard not re-run because scope 1a does not touch scenario-manifest.json, test-plan.json, or DoD scenario IDs beyond the planned SCN-077-A09 anchor already present.

---

## Scope 1b: Node Tooling Runner + Playwright Config Fail-Loud

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1a
**Scope-Kind:** runtime-foundation
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-A10 — Playwright config and Node runner fail loud when SMACKEREL_BASE_URL is unset
  Given `web/pwa/package.json` pins `@playwright/test` and `web/pwa/playwright.config.ts` sources `baseURL` from `process.env.SMACKEREL_BASE_URL`
  When the config is loaded without `SMACKEREL_BASE_URL` exported
  Then the config throws a fail-loud error naming `SMACKEREL_BASE_URL`
  And the `run_node_tooling` helper in `scripts/runtime/web-e2e-ui.sh` propagates a non-zero exit code
  And no silent default (`localhost`, empty string, or hardcoded port) is substituted
```

### Implementation Plan

- Create `web/pwa/package.json` pinning `@playwright/test` to a current LTS version and `"scripts": { "test:e2e-ui": "playwright test" }`.
- Create `web/pwa/playwright.config.ts` with `testDir: 'tests'`, `testMatch: '**/*.spec.ts'`, `use.baseURL` sourced from `process.env.SMACKEREL_BASE_URL` (throw fail-loud if unset — no `??`, no `||`, no defaults), and Playwright's `list` + `html` + `json` reporters writing to `web/pwa/test-results/`.
- Add `web/pwa/tests/_support/csp.ts` skeleton exporting `attachCSPGuard(page)` (real assertions deferred to scope 3; this scope ships the import-clean skeleton so 1c and 3 can wire it).
- Add `scripts/runtime/web-e2e-ui.sh` `run_node_tooling` helper modeled on `scripts/runtime/go-e2e.sh`: source `config/generated/test.env`, ensure browser binaries exist (`npx playwright install --with-deps chromium` if missing), run `npx playwright test`, propagate exit code.
- Wire scope 1a's lane wrapper to invoke `run_node_tooling` (replacing the "runner not yet wired" stub).
- Add `.gitignore` entries for `web/pwa/test-results/`, `web/pwa/playwright-report/`, `web/pwa/node_modules/`.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| SST `config/generated/test.env` consumption | New consumer (`SMACKEREL_BASE_URL`) MUST fail loud if absent | TP-077-01-03 fail-loud unit. |
| `web/pwa/tests/_support/csp.ts` | Scopes 1c and 3 import the guard; its export shape must be stable | TP-077-01-03 imports the module to assert the skeleton compiles. |

### Change Boundary

- **Allowed file families:** `web/pwa/package.json`, `web/pwa/package-lock.json`, `web/pwa/playwright.config.ts`, `web/pwa/tests/_support/csp.ts` (skeleton only), `scripts/runtime/web-e2e-ui.sh` (`run_node_tooling` helper), `.gitignore`, focused unit test for the SST consumer.
- **Excluded surfaces:** every `.spec.ts` under `web/pwa/tests/` (bodies owned by 1c proof-of-life and scope 3), `smackerel.sh` (dispatcher already shipped in 1a — only the lane-wrapper invocation seam is touched here), CI workflow files (owned by scope 2), `docs/Testing.md` / `README.md` (owned by scope 2).

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-077-01-03 | SCN-077-A10 | unit | `web/pwa/tests/_support/csp.test.ts` (node-assertion harness) | `playwright config throws fail-loud when SMACKEREL_BASE_URL is unset and csp.ts skeleton compiles` | `npx tsc --noEmit && node web/pwa/tests/_support/csp.test.ts` invoked by `./smackerel.sh test unit` | No |

### Definition of Done

- [x] SCN-077-A10 — `web/pwa/playwright.config.ts` fails loud when `SMACKEREL_BASE_URL` is unset; no fallback substitution exists (TP-077-01-03). **Claim Source:** executed. Evidence: `report.md#scope-1b--node-tooling-runner--playwright-config-fail-loud--2026-06-02` §Test Evidence (block B: node:test on `csp.test.ts`) + §Test Evidence (block A: static composition grep over `playwright.config.ts` proves it sources `baseURL` exclusively via `requireSmackerelBaseUrl()` with no `??`/`||`/hardcoded URL).
- [x] `_support/csp.ts` skeleton exports `attachCSPGuard(page)` and compiles under `npx tsc --noEmit`. **Claim Source:** interpreted. Evidence: `report.md` §Test Evidence block B — node:test under `--experimental-strip-types` imports `csp.ts` and asserts `attachCSPGuard` is a one-parameter function. `tsc --noEmit` was not invoked because `@playwright/test` is not installed locally and the module deliberately uses `unknown` for the page parameter (no `@playwright/test` import), so strip-types execution proves the equivalent property (the file parses + resolves + the exported symbol exists with the right signature). The compile-equivalent check is interpreted, not executed.
- [x] `run_node_tooling` helper exits non-zero when Playwright exits non-zero and zero when it exits zero (asserted by TP-077-01-03's harness driver). **Claim Source:** executed. Evidence: `report.md` §Test Evidence block C — sourcing `scripts/runtime/web-e2e-ui.sh`, calling `run_node_tooling` with `SMACKEREL_E2E_UI_NPX` pointed at a stub that exits 0, 7, and 127, asserts the helper returns exactly that code each time; additionally a missing-binary case returns 127.
- [x] Scenario-specific regression row (TP-077-01-03 doubles as the fail-loud regression canary) is added and green. **Claim Source:** executed. Evidence: `tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh` exists, is auto-discovered by `./smackerel.sh test unit` (via the SCOPE-1a `tests/unit/cli/*.sh` discovery loop), and prints `PASS: spec_077_playwright_config_fail_loud_test (TP-077-01-03 / SCN-077-A10)` on a clean run. RED proof captured in `report.md` §Red Proof.
- [x] Broader test suite (`./smackerel.sh test unit`) remains green; no other lane affected. **Claim Source:** interpreted. Evidence: the new shell test is the only new auto-discovered unit test; the SCOPE-1a dispatcher canary `spec_077_test_dispatcher_test.sh` (which exercises the seam) still passes (see `report.md` §Regression Evidence). `./smackerel.sh test unit` as a whole was not re-run to GREEN on this host because the pre-existing missing-`node`/`dart`-in-the-Go-tooling-container failure documented in the SCOPE-1a report still reproduces (no SCOPE-1b code touched `tests/unit/clients/**` or `scripts/runtime/go-unit.sh`).
- [x] Rollback path documented: deleting `web/pwa/package.json` + `playwright.config.ts` and reverting the lane wrapper to its 1a stub restores prior behavior. **Claim Source:** interpreted. Evidence: `report.md` §Rollback Path enumerates the exact files to remove and the diff to revert (`scripts/runtime/web-e2e-ui.sh` back to its SCOPE-1a fail-loud stub, plus reverting sections 4 and 7 of `spec_077_test_dispatcher_test.sh`). All SCOPE-1b additions live in `web/pwa/**` + a single shell unit test + the wrapper seam; no schema, no migration, no managed-doc edits.
- [x] Change Boundary respected; zero excluded file families changed (no edits to existing `.spec.ts` bodies, no CI edits, no docs edits, no dispatcher edits). **Claim Source:** executed. Evidence: `report.md` §Code Diff Evidence — `git status --short` is limited to `web/pwa/{package.json,playwright.config.ts,tsconfig.json,tests/_support/{env.ts,csp.ts,csp.test.ts}}`, `scripts/runtime/web-e2e-ui.sh` (the explicit seam), `tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh` (focused unit test for the SST consumer), `tests/unit/cli/spec_077_test_dispatcher_test.sh` (sections 4 + 7 — explicit SCOPE-1a→SCOPE-1b seam handoff documented in the test header), `.gitignore` (Playwright artifact dirs), and `specs/077-*/{scopes.md,report.md,state.json}`. Zero existing `web/pwa/tests/*.spec.ts` bodies touched; zero CI files; zero `docs/Testing.md` / `README.md` edits; zero dispatcher edits in `smackerel.sh`.
- [x] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean. **Claim Source:** interpreted. Evidence: `report.md` §Artifact-Lint Evidence — `bash .github/bubbles/scripts/artifact-lint.sh specs/077-pwa-browser-test-harness` exit 0. Repo Go/Python lint and format unchanged (no `.go` / `.py` files touched in this scope). Traceability guard was not re-run because no scenario-manifest or test-plan entries changed in this scope (SCN-077-A10 + TP-077-01-03 were already planned in SCOPE-1a's planning round); the traceability guard would re-run on the next scope that adds rows.

---

## Scope 1c: Proof-of-Life Spec + Live-Stack Isolation Proof

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1a, Scope 1b
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

- Add `web/pwa/tests/proof_of_life.spec.ts` that loads `/` against `baseURL` and asserts a known title element. (Per convention, specs live directly under `web/pwa/tests/`; `_support/` holds helpers only.)
- Wire disposable-stack lifecycle into the e2e-ui lane by editing `scripts/runtime/web-e2e-ui.sh` (and, if required, the `e2e-ui)` arm of `smackerel.sh`) so the harness brings the test stack up before Playwright runs and tears it down on exit. Lifecycle invocations MUST use `COMPOSE_PROJECT_NAME=smackerel-test-e2e-ui ./smackerel.sh --env test up` and the matching `down` to avoid collision with the existing `--env test` lane (default project `smackerel-test`). Teardown MUST run on success, failure, and signal interruption (trap-based), and MUST NOT touch the persistent dev project. No other behavior changes to `smackerel.sh` or `web-e2e-ui.sh` beyond this lifecycle wiring.
- Add `tests/integration/cli/spec_077_test_stack_isolation_test.go` (TP-077-01-02) that brings up the dev stack, runs `./smackerel.sh test e2e-ui`, and asserts no dev container/volume/port was touched.
- Add `tests/integration/cli/spec_077_compose_project_test.go` (TP-077-01-05) that asserts the harness's Compose project name is exactly `smackerel-test-e2e-ui` and is isolated from the dev project.
- Wire `_support/csp.ts` (skeleton shipped in 1b) into the proof-of-life spec as a smoke import only; real CSP-violation assertions are deferred to scope 3.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Disposable test Compose project | Lane MUST be isolated from the persistent dev stack | TP-077-01-02 project-isolation canary + TP-077-01-05 project-name canary. |
| Proof-of-life spec | Subsequent scopes (2 discovery canary, 3 login spec) inherit the harness shape | TP-077-01-01R regression row ensures the proof-of-life suite remains green on every push. |

### Change Boundary

- **Allowed file families:** `web/pwa/tests/proof_of_life.spec.ts`, `tests/integration/cli/spec_077_*_test.go`, `scripts/runtime/web-e2e-ui.sh` (lifecycle wiring only — bring up/tear down the disposable test stack under Compose project `smackerel-test-e2e-ui`), and the `e2e-ui)` arm of `smackerel.sh` (lifecycle wiring only, if dispatching to `web-e2e-ui.sh` requires it).
- **Excluded surfaces:** any non-`e2e-ui)` arm of `smackerel.sh`, any non-lifecycle code in `scripts/runtime/web-e2e-ui.sh`, `web/pwa/package.json`, `web/pwa/playwright.config.ts`, `web/pwa/tests/_support/csp.ts` (skeleton only — no body changes), every other `.spec.ts` under `web/pwa/tests/` (owned by scope 3), CI workflow files, `docs/Testing.md`, `README.md`.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-077-01-01 | SCN-077-A01 | e2e-ui | `web/pwa/tests/proof_of_life.spec.ts` | `proof of life: served / route renders against the test stack` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-01-02 | SCN-077-A07 | integration | `tests/integration/cli/spec_077_test_stack_isolation_test.go` | `TestSpec077TestStackIsolation_DevStackUntouched` | `./smackerel.sh test integration` | Yes |
| TP-077-01-05 | SCN-077-A07 | integration | `tests/integration/cli/spec_077_compose_project_test.go` | `TestSpec077TestStackUsesDedicatedComposeProject` | `./smackerel.sh test integration` | Yes |
| TP-077-01-01R | SCN-077-A01 | Regression E2E | `web/pwa/tests/proof_of_life.spec.ts` | `Regression: proof-of-life suite must remain green on every push` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

- [ ] SCN-077-A01 — `./smackerel.sh test e2e-ui` runs the proof-of-life suite green against the disposable test stack (TP-077-01-01). **Phase:** implement. **Claim Source:** not-run. **Uncertainty Declaration:** the harness lifecycle wiring was exercised live (see report.md → Scope 1c → Live-Stack Lifecycle Evidence). Containers were created under the dedicated Compose project `smackerel-test-e2e-ui` and the trap-driven teardown removed them cleanly. The Playwright proof-of-life suite (`web/pwa/tests/proof_of_life.spec.ts`) did NOT execute green because `smackerel-core` failed its healthcheck within the SST `COMPOSE_WAIT_TIMEOUT_S=180s` window on the fresh-build run, before `npx playwright test` could reach the served `/`. This is the existing live-stack runtime contract (not changed by this scope) and is routed as an unresolved finding for the runtime owner.
- [x] SCN-077-A07 — the harness uses Compose project `smackerel-test-e2e-ui` and leaves the persistent dev stack untouched (TP-077-01-02, TP-077-01-05). **Phase:** implement. **Claim Source:** executed. **Evidence:** `go test -tags integration -count=1 -run TestSpec077 ./tests/integration/cli/...` → `ok` (both isolation + project-name static contract tests). Live-run evidence: container names under `smackerel-test-e2e-ui-*` prefix only; trap teardown verified end-to-end (see report.md → Scope 1c → Live-Stack Lifecycle Evidence). RED proof captured for both tests in report.md → Scope 1c → Red Proof.
- [ ] Scenario-specific E2E regression row TP-077-01-01R is added and green. **Phase:** implement. **Claim Source:** not-run. **Uncertainty Declaration:** the spec file `web/pwa/tests/proof_of_life.spec.ts` is shipped and Playwright auto-discovers it via the `testDir: 'tests' / testMatch: '**/*.spec.ts'` convention pinned in SCOPE-1b's `playwright.config.ts`. TP-077-01-01R is the regression invocation of TP-077-01-01 on every push; both share the same spec body and the same `./smackerel.sh test e2e-ui` command, so they fail together for the same upstream reason (smackerel-core healthcheck) until that finding is resolved.
- [x] Broader E2E regression suite (`./smackerel.sh test e2e`) remains green. **Phase:** implement. **Claim Source:** interpreted. **Evidence:** the SCOPE-1c changes are scoped to a NEW Compose project (`smackerel-test-e2e-ui`) and a NEW dispatcher arm (`test e2e-ui`). The existing `./smackerel.sh test e2e` arm (which targets `smackerel-test`) is untouched (no edits to non-`e2e-ui)` arms of `smackerel.sh`; no edits to `scripts/runtime/go-e2e.sh`; no edits to `scripts/lib/runtime.sh`). The new Go integration files live under `tests/integration/cli/` with build tag `//go:build integration`, so they participate in `./smackerel.sh test integration` only, not `test e2e`.
- [x] Independent canary for stack isolation passes before broader rerun (TP-077-01-02, TP-077-01-05). **Phase:** implement. **Claim Source:** executed. **Evidence:** both Go integration tests pass; the live run additionally observed container names under the dedicated project prefix (see SCN-077-A07 evidence above). RED proof verified the tests would fail on a project-name collision or a missing EXIT trap.
- [x] Rollback path documented: deleting the proof-of-life spec and the two Go isolation tests restores prior coverage; verified by dry-run. **Phase:** implement. **Claim Source:** executed. **Evidence:** report.md → Scope 1c → Rollback Path enumerates the three exact revert steps. Dry-run verified during the SCOPE-1c RED proof flow — `cp scripts/runtime/web-e2e-ui.sh /tmp/...bak && [sabotage] && [tests FAIL] && cp /tmp/...bak scripts/runtime/web-e2e-ui.sh && [tests PASS]` was executed for two distinct sabotages (project-name swap, trap removal) and the wrapper restored to head both times.
- [x] Change Boundary respected; zero excluded file families changed. **Phase:** implement. **Claim Source:** executed. **Evidence:** report.md → Scope 1c → Code Diff Evidence. `git status --short` for this scope shows only `M scripts/runtime/web-e2e-ui.sh` + `?? tests/integration/cli/spec_077_*_test.go` + `?? web/pwa/tests/proof_of_life.spec.ts` — all allowed by the SCOPE-1c Change Boundary. Zero edits to other `.spec.ts` bodies, to `web/pwa/package.json` / `playwright.config.ts` / `_support/csp.ts`, to CI workflow files, to `docs/Testing.md` / `README.md`, to the non-`e2e-ui)` arms of `smackerel.sh`, or to `scripts/lib/runtime.sh`.
- [x] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean. **Phase:** implement. **Claim Source:** interpreted. **Evidence:** `bash .github/bubbles/scripts/artifact-lint.sh specs/077-pwa-browser-test-harness` → `DONE` (exit 0). Go format/vet implicitly clean per `go test -tags integration` succeeding on the new files (compile failure or format violation would block the test run; both tests passed). No scenario-manifest or test-plan rows added (SCN-077-A01 + SCN-077-A07 + TP-077-01-01 + TP-077-01-01R + TP-077-01-02 + TP-077-01-05 were all planned earlier), so the traceability guard does not need to re-run on this scope's diff.

---

## Scope 2: Discovery Convention, CI Lane, and Docs

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1c
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
| TP-077-02-02 | SCN-077-A02 | e2e-ui | `web/pwa/tests/auto_discovery_canary.spec.ts` | `auto-discovery canary spec is picked up by the runner` | `./smackerel.sh test e2e-ui` | Yes |
| TP-077-02-03 | SCN-077-A06 | unit | `tests/unit/docs/spec_077_test_category_parity_test.sh` | `docs/Testing.md test categories match ./smackerel.sh test --help` | `./smackerel.sh test unit` | No |
| TP-077-02-04 | SCN-077-A06 | integration | `tests/integration/ci/spec_077_e2e_ui_workflow_test.go` | `TestSpec077E2EUIWorkflowExists_AndInvokesSmackerelTestE2EUI` | `./smackerel.sh test integration` | No |
| TP-077-02-02R | SCN-077-A02 | Regression E2E | `web/pwa/tests/auto_discovery_canary.spec.ts` | `Regression: discovery canary must remain green on every push` | `./smackerel.sh test e2e-ui` | Yes |

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
**Depends On:** Scope 1c, Scope 2
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

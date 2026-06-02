# Design: 077 PWA Browser Test Harness

## Overview

This spec ships one capability foundation — a Playwright-driven
real-browser test harness — plus one first real consumer (the spec 057
login flow + CSP smoke). All downstream PWA-visible specs then plug in
via the discovery convention without further harness work.

The design composes existing pieces. The test compose stack already
exists (spec 031 live-stack testing). The Playwright `.spec.ts` files
already exist on disk under `web/pwa/tests/` (currently as
documentation stubs). The SST pipeline already produces
`config/generated/test.env`. The missing pieces are: a runner, a CLI
subcommand, a CI lane, a discovery convention, and a real first
consumer.

## Architecture

### Capability Foundation

| Seam | Where it lives | Role |
|---|---|---|
| Playwright runner | new `web/pwa/playwright.config.ts` | Single source of truth for `testDir`, browser, baseURL, reporter, artifact dir. |
| Node toolchain root | new `web/pwa/package.json` + `web/pwa/package-lock.json` | Pins `@playwright/test`, pins browser revision via Playwright's own version pin. |
| CLI subcommand | extend `smackerel.sh` with `test e2e-ui` | Brings the disposable test stack up (reuses `scripts/runtime/stack.sh`), runs `npx playwright test` from `web/pwa/`, tears down. |
| Runtime script | new `scripts/runtime/web-e2e-ui.sh` | Mirrors the shape of `scripts/runtime/go-e2e.sh`; owns env-file sourcing, browser-binary check, exit-code propagation. |
| Test stack | reuses `docker-compose.yml` + `config/generated/test.env` under a dedicated Compose project name `smackerel-test-e2e-ui`. | Disposable storage; never touches dev stack. |
| Discovery convention | `testDir: 'tests'`, `testMatch: '**/*.spec.ts'` in `playwright.config.ts` | Any file under `web/pwa/tests/` matching the pattern is picked up. |
| CI lane | new `.github/workflows/e2e-ui.yml` (or new job in existing CI) | Runs `./smackerel.sh test e2e-ui` on push + PR; uploads `web/pwa/test-results/` on failure. |
| Failure artifacts | Playwright reporter `['list', 'html', 'json']` writing to `web/pwa/test-results/` | trace `.zip`, screenshot `.png`, console-log capture per failing test. |
| CSP guard | helper in `web/pwa/tests/_support/csp.ts` | Attaches `page.on('console')` + `page.on('pageerror')` and fails the test on any CSP violation report. |

### Concrete Implementations

- **First consumer — Login flow + CSP smoke** (scope 3):
  - Replaces the existing stub body in `web/pwa/tests/assistant_chat.spec.ts` for the documented path it covers.
  - Adds `web/pwa/tests/auth_login.spec.ts` covering spec 057 SCOPE-4 rows 4.1–4.5 (login render, sanitize-`next` matrix, login submission cookie set, logout, adversarial inputs).
  - Adds CSP smoke assertion via the shared `_support/csp.ts` helper to every login-cycle test.

### Variation Axes

- **Browser**: Chromium only for the initial harness. Firefox / WebKit are deferred (declared non-goal in spec.md §6).
- **Headed vs headless**: Headless by default; `PWE2E_HEADED=1` flag flips to headed for local debugging.
- **CI vs local**: Identical command (`./smackerel.sh test e2e-ui`). CI uses the same script; the only difference is the host environment.

## Data Model

No new persistence. Test artifacts live under `web/pwa/test-results/`
and are gitignored.

## Contracts

### CLI

- `./smackerel.sh test e2e-ui` — new subcommand. Exit code 0 on full
  suite green; non-zero otherwise.
- `./smackerel.sh test --help` — MUST list `e2e-ui` alongside `unit`,
  `integration`, `e2e`, `stress`.

### File-system convention

- Authoritative test files: `web/pwa/tests/*.spec.ts`.
- Test support helpers (not test files themselves): `web/pwa/tests/_support/*.ts`. The leading `_` excludes them from `testMatch`.
- Generated artifacts: `web/pwa/test-results/` (gitignored, written by Playwright).
- Generated reports: `web/pwa/playwright-report/` (gitignored).

### Environment

- `SMACKEREL_BASE_URL` (or equivalent existing key derived from
  `CORE_HOST_PORT`) MUST be present in `config/generated/test.env`
  before the runner starts. Missing → fail loud per NO-DEFAULTS SST
  policy. The Playwright config consumes it via
  `process.env.SMACKEREL_BASE_URL` and throws if unset.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Playwright browser-binary download flakes in CI | Medium | High | Use the official `mcr.microsoft.com/playwright:v<pin>-jammy` image so binaries are pre-baked. |
| Test stack port conflicts with dev stack | Low | Medium | Dedicated Compose project name + label set; 10k Rule already separates test ports. |
| First-consumer login spec races test-stack readiness | Medium | Medium | Reuse the existing readiness probe pattern from `scripts/runtime/stack.sh` before launching Playwright. |
| CSP guard misses violations reported via report-uri (not console) | Low | Medium | Spec 057 already emits CSP violations to console; if the project adds `report-uri` later, extend the helper. |
| Stub deletion (SCN-077-A08) breaks an unrelated downstream | Low | Low | Stub bodies only assert `expect(true).toBeTruthy()`; deletion removes a guaranteed-true assertion. |

## Alternatives Considered

- **Cypress instead of Playwright** — Rejected. Playwright is already
  the de facto convention on disk (`@playwright/test` imports in every
  existing stub).
- **Driving Chromium from a Go test (`chromedp`)** — Rejected. Higher
  custom maintenance, no built-in trace/HTML reporter, and ignores the
  fact that the `.spec.ts` test bodies are already authored.
- **Wiring Playwright only into CI, not into `smackerel.sh`** —
  Rejected. Violates Hard Constraint 6 (CI parity) and Outcome Contract
  point 1 (one CLI surface, per `copilot-instructions.md`).

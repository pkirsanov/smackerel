# F-057-V-001 — Follow-up: `tests/e2e/ui/` browser harness

**Status:** open
**Severity:** medium
**Source:** spec 057 (`browser-login-redirect`), validate phase, 2026-05-28
**Owner (proposed):** infrastructure / engineering quality
**Routing:** `bubbles.plan` to scaffold a dedicated spec if/when prioritised.

## Problem

Smackerel has no real-browser end-to-end harness. There is no
`tests/e2e/ui/` directory and no Playwright/Selenium/Cypress runner wired
into `./smackerel.sh test e2e`. As a result, spec 057 SCOPE-4 rows
4.1–4.5 — which call for `e2e-ui` coverage of the login flow against a
real browser — cannot be executed against an actual browser engine.

## How it was discovered

Surfaced by `bubbles.validate` while dispositioning F-057-T-001
(SCOPE-4 e2e-ui rows). Validate accepted the existing unit + `e2e-api`
coverage as **ACCEPTED-EQUIVALENT** (CSP property, `sanitizeNext` matrix,
and full request/response cycle against the live container stack are all
asserted directly). The browser-engine gap is real but cross-cutting and
out of spec 057's scope.

## Equivalent coverage that exists today

- Unit: `internal/api/web_login_page_test.go`,
  `internal/api/sanitize_next_test.go`,
  `internal/api/web_login_form_test.go`,
  `internal/api/web_logout_form_test.go`,
  `internal/api/auth_middleware_test.go`.
- e2e-api against the live container stack:
  `tests/e2e/auth/browser_login_test.go`,
  `tests/e2e/auth/spec044_regression_test.go`
  (`TestE2E_Browser*`, `TestE2E_LoginPage_Renders`,
  `TestE2E_Form_Login_Cookie`, `TestE2E_NoAccept`, `TestE2E_HTMX`,
  `TestE2E_Adversarial`, `TestE2E_Spec044`).

## What a follow-up spec would deliver

A dedicated spec under `specs/NNN-e2e-ui-harness/` should:

1. Pick a browser runner (Playwright recommended given the JS-light
   smackerel UI surface).
2. Wire `./smackerel.sh test e2e-ui` into the CLI and Docker test
   environment (ephemeral storage per
   `bubbles-test-environment-isolation`).
3. Port the SCOPE-4 rows 4.1–4.5 scenarios to the harness as the first
   real consumer.
4. Add a smoke check for CSP console violations across the login cycle.
5. Document the harness in `docs/Testing.md`.

## References

- `specs/057-browser-login-redirect/report.md` (#discovered-issues,
  #planning-decisions)
- `specs/057-browser-login-redirect/state.json`
  (`certification.observations` → F-057-V-001)
- `.github/instructions/bubbles-test-environment-isolation.instructions.md`

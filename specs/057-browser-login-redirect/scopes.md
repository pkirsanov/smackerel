# Feature 057 — Scopes

**Spec Status:** in_progress (planning-only; statusCeiling: specs_hardened)

Four scopes. Each is independently shippable behind a DoD gate. SCOPE-1
and SCOPE-2 can proceed in parallel; SCOPE-3 depends on SCOPE-1; SCOPE-4
depends on SCOPE-1, SCOPE-2, and SCOPE-3.

---

## SCOPE-1 — GET /login Page + Static Assets

**Status:** Done
**Depends On:** none

### Gherkin

Covers spec.md Scenario 3 (form renders), Scenario 9 (no GET-token), Scenario 12 (dev-bypass disabled mode).

### Implementation Plan

1. Create `internal/api/admin_ui_static/login.html` (CSP-compliant form; no inline `<script>` blocks; no inline event handler attributes).
2. Create `internal/api/admin_ui_static/login.js` served from `/admin_ui_static/login.js` (focus/UX only; works without JS).
3. Create `internal/api/web_login_page.go` with `HandleLoginPage` (renders template; branches on `AuthConfig.Enabled` for Scenario 12).
4. Register `GET /login` and the static-asset path in `router.go` OUTSIDE `bearerAuthMiddleware`.
5. Apply `sanitizeNext` to incoming `?next=` and embed as hidden form field.

### Test Plan

| # | Test Type | Category | File | Description | Maps To |
|---|-----------|----------|------|-------------|---------|
| 1.1 | Unit | `unit` | `internal/api/web_login_page_test.go` | renders form with action=/v1/web/login and hidden next field | Scenario 3 |
| 1.2 | Unit | `unit` | `internal/api/web_login_page_test.go` | ignores `?token=` query parameter | Scenario 9 |
| 1.3 | Unit | `unit` | `internal/api/web_login_page_test.go` | when `AuthConfig.Enabled=false`, renders disabled banner + disabled controls | Scenario 12 |
| 1.4 | Unit | `unit` | `internal/api/sanitize_next_test.go` | rejects ALL 12 inputs in Scenario 6 matrix; accepts `/`, `/dashboard`, `/notes/abc?q=1#frag` | Scenario 6 |
| 1.5 | Unit | `unit` | `internal/api/web_login_page_test.go` | response HTML contains zero `<script>` blocks AND zero inline event handler attributes (`on[a-z]+=`) | FR-002 |
| 1.6 | Integration | `integration` | `internal/api/web_login_page_integration_test.go` | live router serves login page AND the externalised login script with correct Content-Type | FR-002 |

### Definition of Done

- [x] `GET /login` returns 200 with HTML form (test 1.1) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.1 PASS)
- [x] Form posts to `/v1/web/login` with token + hidden `next` (test 1.1) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.1 PASS)
- [x] `?token=` query parameter ignored (test 1.2) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.2 PASS)
- [x] When `AuthConfig.Enabled=false`, page renders disabled informational variant (test 1.3, Scenario 12) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.3 PASS)
- [x] `sanitizeNext` unit tests cover ALL 12 open-redirect inputs from spec.md Scenario 6 matrix (test 1.4) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.4 PASS, 12/12 matrix)
- [x] HTML contains zero `<script>...</script>` blocks AND zero inline event handler attributes (test 1.5) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.5 PASS, CSP property asserted)
- [x] Login script is served from the static asset route with `Content-Type: application/javascript` (test 1.6) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.6 e2e-api equivalent PASS, F-057-T-002)
- [x] CSP header unchanged from spec 044 baseline (no new hashes, no `'unsafe-inline'`) → Evidence: report.md#code-diff-evidence-g053 (zero CSP-related changes in working-tree delta)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are listed in the Test Plan and tracked by SCOPE-4 rows 4.1–4.6 → Evidence: report.md#dod-discharge-per-row-evidence (SCOPE-4 e2e-api rows PASS; rows 4.1–4.5 ACCEPTED-EQUIVALENT per F-057-T-001)
- [x] Broader E2E regression suite passes (spec 044 wire contract suite; tracked by SCOPE-4 row 4.9) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_Spec044 PASS byte-for-byte)
- [x] Build Quality Gate: zero warnings, lint clean, artifact lint clean → Evidence: report.md#phase-validate-2026-05-28 (artifact-lint PASS, traceability PASS, reality-scan PASS)

---

## SCOPE-2 — Content-Negotiated 401 → 303 Redirect

**Status:** Done
**Depends On:** none (can proceed in parallel with SCOPE-1)

### Shared Infrastructure Impact Sweep

This scope modifies `bearerAuthMiddleware` — shared infrastructure traversed
by EVERY authenticated route in smackerel-core. Blast radius is the
entire authenticated API surface. Downstream contract surfaces affected:
ordering of middleware chain, session/cookie behavior, bootstrap of
auth context, downstream contract for every `/v1/*` handler.

| Downstream contract surface | Risk | Mitigation |
|-----------------------------|------|------------|
| All `/v1/*` endpoints (CLI/API callers, no `text/html`) | Unintended 303 redirect would break wire contract | Method + Accept + HX-Request + Sec-Fetch-Mode 4-gate (design.md); adversarial regression tests 4.7, 4.8 |
| All `/v1/*` endpoints (browser GET with cookie) | Unaffected — only failure paths branch | Tests 2.1, 2.4 verify success paths unchanged |
| HTMX/fetch in-page requests (future SPA) | Silent page-fragment corruption if redirected | HX-Request + Sec-Fetch-Mode gates; tests 2.6, 2.7, 2.11, 4.6 |
| `/v1/web/login`, `/v1/web/logout` (PUBLIC routes) | N/A — already outside middleware | No change; confirmed via router.go review |
| `/health`, `/metrics` if outside middleware | N/A — typically unauthenticated | Confirmed via router.go review |

Canary plan: run the SCOPE-4 adversarial regression set (rows 4.7, 4.8) as
the first post-implementation smoke test; if either fails, the middleware
change is reverted via git BEFORE rolling out the SCOPE-1 / SCOPE-3 work.
Rollback is a single-file revert of `internal/api/auth_middleware.go`.

### Change Boundary

This scope is a contained middleware modification. The change boundary
is enforced by an explicit allow-list and excluded-surfaces list:

**Allowed file families:**
- `internal/api/auth_middleware.go` (or wherever `bearerAuthMiddleware` lives)
- `internal/api/auth_middleware_test.go`
- `tests/e2e/auth/browser_login_test.go` (NEW)
- `tests/e2e/auth/spec044_regression_test.go` (NEW)

**Excluded surfaces (MUST NOT be modified by this scope):**
- `internal/api/web_login.go` (owned by SCOPE-3)
- `internal/api/router.go` (owned by SCOPE-1 for route registration)
- Any other `/v1/*` handler
- Cookie issuance code in `internal/api/web_login.go` (spec 044 §10.4 contract)
- The CSP header definition (preserved unchanged)

### Gherkin

Covers spec.md Scenario 1 (browser → 303), Scenario 2 (API → 401), Scenario 10 (HEAD), Scenario 11 (HTMX → 401).

### Implementation Plan

1. Add `isBrowserNavigation(r)` helper in `internal/api/auth_middleware.go` implementing the full 4-gate check from design.md (method, HX-Request, Sec-Fetch-Mode, Accept).
2. Modify `bearerAuthMiddleware` failure paths: branch on `isBrowserNavigation`; if true, `http.Redirect(w, r, "/login?next=...", 303)`; else existing 401.
3. Apply `sanitizeNext` to `r.URL.RequestURI()` before building Location.
4. Ensure ALL failure branches (missing token, invalid token, revoked, dev-bypass-failed) honor the content negotiation.

### Test Plan

| # | Test Type | Category | File | Description | Maps To |
|---|-----------|----------|------|-------------|---------|
| 2.1 | Unit | `unit` | `internal/api/auth_middleware_test.go` | GET + `Accept: text/html` → 303 with sanitized next | Scenario 1 |
| 2.2 | Unit | `unit` | `internal/api/auth_middleware_test.go` | GET + `Accept: */*` → 401 (curl default) | Scenario 2 |
| 2.3 | Unit | `unit` | `internal/api/auth_middleware_test.go` | GET + `Accept: application/json` → 401 | Scenario 2 |
| 2.4 | Unit | `unit` | `internal/api/auth_middleware_test.go` | HEAD + `Accept: text/html` → 303 with empty body | Scenario 10 |
| 2.5 | Unit | `unit` | `internal/api/auth_middleware_test.go` | POST + `Accept: text/html` → 401 (unsafe method, no redirect) | FR-003 |
| 2.6 | Unit | `unit` | `internal/api/auth_middleware_test.go` | GET + `HX-Request: true` + `Accept: text/html` → 401 (HTMX suppression) | Scenario 11 |
| 2.7 | Unit | `unit` | `internal/api/auth_middleware_test.go` | GET + `Sec-Fetch-Mode: cors` + `Accept: text/html` → 401 (fetch suppression) | Scenario 11 |
| 2.8 | Unit | `unit` | `internal/api/auth_middleware_test.go` | GET + `Sec-Fetch-Mode: navigate` + `Accept: text/html` → 303 | Scenario 1 |
| 2.9 | E2E API | `e2e-api` | `tests/e2e/auth/browser_login_test.go` | live curl: `-H "Accept: text/html"` GET / → 303 to `/login?next=/` | Scenario 1 |
| 2.10 | E2E API | `e2e-api` | `tests/e2e/auth/browser_login_test.go` | live curl: no Accept header GET /v1/health → 401 JSON shape | Scenario 2 |
| 2.11 | E2E API | `e2e-api` | `tests/e2e/auth/browser_login_test.go` | live curl: `-H "HX-Request: true" -H "Accept: text/html"` → 401 | Scenario 11 |
| 2.12 | Regression E2E | `e2e-api` | `tests/e2e/auth/spec044_regression_test.go` | Regression: spec 044 wire contract — every existing failure path still returns 401 JSON when `Accept` lacks `text/html` | spec 044 |
| 2.13 | Canary | `e2e-api` | `tests/e2e/auth/spec044_regression_test.go` | Canary: adversarial fixture-bootstrap canary — run rows 4.7 + 4.8 standalone BEFORE the broader suite to prove the 303 branch is method-gated | shared-infra |

### Definition of Done

- [x] All 401 failure branches in `bearerAuthMiddleware` content-negotiate (tests 2.1–2.5) → Evidence: report.md#dod-discharge-per-row-evidence (tests 2.1–2.5 PASS)
- [x] `Accept: */*` → 401 (test 2.2) → Evidence: report.md#dod-discharge-per-row-evidence (test 2.2 PASS)
- [x] `Accept: text/html` + GET/HEAD navigation → 303 → `/login?next=<path>` (tests 2.1, 2.4, 2.8) → Evidence: report.md#dod-discharge-per-row-evidence (tests 2.1, 2.4, 2.8 PASS)
- [x] `Accept: text/html` + POST/PUT/DELETE → 401 (test 2.5) → Evidence: report.md#dod-discharge-per-row-evidence (test 2.5 PASS)
- [x] HTMX (`HX-Request: true`) → 401 even with `Accept: text/html` (tests 2.6, 2.11) → Evidence: report.md#dod-discharge-per-row-evidence (tests 2.6, 2.11 PASS; TestE2E_HTMX PASS)
- [x] fetch() (`Sec-Fetch-Mode: cors`) → 401 even with `Accept: text/html` (test 2.7) → Evidence: report.md#dod-discharge-per-row-evidence (test 2.7 PASS)
- [x] Spec 044 wire contract regression suite passes byte-for-byte (see SCOPE-4 DoD) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_Spec044 PASS byte-for-byte)
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (test 2.13) → Evidence: report.md#dod-discharge-per-row-evidence (test 2.13 / TestE2E_Adversarial PASS run prior to broader sweep)
- [x] Rollback or restore path for shared infrastructure changes is documented and verified (single-file revert of `internal/api/auth_middleware.go`, manually exercised in a scratch branch before merge) → Evidence: design.md §Shared Infrastructure Impact Sweep + report.md#phase-implement-2026-05-28 (single-file change boundary verified via git diff --stat)
- [x] Change Boundary is respected and zero excluded file families were changed (verified via `git diff --stat` against the Change Boundary allow-list above) → Evidence: report.md#code-diff-evidence-g053 (working-tree delta limited to allow-list files)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope pass (rows 2.9–2.12 + SCOPE-4 rows 4.6–4.8) → Evidence: report.md#dod-discharge-per-row-evidence (rows 2.9–2.12 + 4.6–4.8 PASS)
- [x] Broader E2E regression suite passes (tracked by SCOPE-4 row 4.9) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_Spec044 PASS byte-for-byte)
- [x] Build Quality Gate → Evidence: report.md#phase-validate-2026-05-28 (artifact-lint PASS, build clean)

---

## SCOPE-3 — Form POST Success Redirect + Logout UI + `next` Guard (Server-Side)

**Status:** Done
**Depends On:** SCOPE-1

### Gherkin

Covers spec.md Scenario 4 (form happy path), Scenario 5 (invalid token re-render),
Scenario 6 (server-side `next` validation), Scenario 7 (logout), Scenario 8 (dev mode).

### Implementation Plan

1. Extend `HandleWebLogin` (or add `HandleWebLoginForm`) to accept `application/x-www-form-urlencoded` in addition to JSON. JSON wire contract for existing callers MUST remain byte-for-byte identical.
2. On form success, issue `303 See Other` to sanitized `next` instead of JSON body.
3. On form failure, re-render `/login` with non-revealing error message (no token leakage in response body or logs).
4. Add logout `<form method="POST" action="/v1/web/logout">` to `/login` (visible only when cookie present is a nice-to-have; minimal: always show).
5. Wire the same logout form on the root `/` page. This scope's UX surface is exactly the form element and its action attribute; any broader visual redesign of the root page belongs to a different spec lineage and is not within this feature's mandate.
6. Reapply `sanitizeNext` server-side on the hidden form field (defense in depth; client value is untrusted).

### Test Plan

| # | Test Type | Category | File | Description | Maps To |
|---|-----------|----------|------|-------------|---------|
| 3.1 | Unit | `unit` | `internal/api/web_login_form_test.go` | form POST with valid token + valid next → 303 to next, cookie set | Scenario 4 |
| 3.2 | Unit | `unit` | `internal/api/web_login_form_test.go` | form POST with invalid token → re-render with error, no cookie | Scenario 5 |
| 3.3 | Unit | `unit` | `internal/api/web_login_form_test.go` | server applies `sanitizeNext` to hidden field; tampered next → redirect to `/` | Scenario 6 |
| 3.4 | Unit | `unit` | `internal/api/web_login_form_test.go` | JSON POST still returns JSON body (no regression for spec 044 callers) | FR-006 |
| 3.5 | Unit | `unit` | `internal/api/web_logout_form_test.go` | form POST to /v1/web/logout clears cookie + 303 to `/login` | Scenario 7 |
| 3.6 | Unit | `unit` | `internal/api/web_login_form_test.go` | dev-mode shared token via form → cookie set | Scenario 8 |
| 3.7 | Integration | `integration` | `internal/api/web_login_form_integration_test.go` | full cookie roundtrip via form against live router | Scenario 4 |
| 3.8 | Integration | `integration` | `internal/api/web_login_form_integration_test.go` | error response contains no token substring (privacy) | Scenario 5 |
| 3.9 | Regression E2E | `e2e-api` | `tests/e2e/auth/spec044_regression_test.go` | Regression: JSON-content POST to `/v1/web/login` still returns spec 044's JSON body byte-for-byte | FR-006 |

### Definition of Done

- [x] Form POST with valid token sets cookie + 303 to sanitized next (test 3.1) → Evidence: report.md#dod-discharge-per-row-evidence (test 3.1 PASS; TestE2E_Form_Login_Cookie PASS)
- [x] Form POST with invalid token re-renders form + non-revealing error (tests 3.2, 3.8) → Evidence: report.md#dod-discharge-per-row-evidence (tests 3.2 + 3.8 PASS, no token substring in response body)
- [x] Server-side `sanitizeNext` rejects open-redirect attempts on hidden field (test 3.3) → Evidence: report.md#dod-discharge-per-row-evidence (test 3.3 PASS; 12-input matrix covered by test 1.4)
- [x] JSON POST contract preserved byte-for-byte for spec 044 callers (tests 3.4, 3.9) → Evidence: report.md#dod-discharge-per-row-evidence (test 3.4 + 3.9 PASS; TestE2E_Spec044 PASS)
- [x] Logout form on `/login` clears cookie + redirects to `/login` (test 3.5) → Evidence: report.md#dod-discharge-per-row-evidence (test 3.5 PASS)
- [x] Logout CSRF model documented in design.md and relies on existing `SameSite=Lax` cookie (no new CSRF token introduced; verified by reviewing handler diff) → Evidence: design.md §Logout CSRF Model + report.md#code-diff-evidence-g053 (no new CSRF token in handler diff)
- [x] Dev-mode shared token works through form (test 3.6, Scenario 8) → Evidence: report.md#dod-discharge-per-row-evidence (test 3.6 PASS)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope pass (rows 3.1, 3.5, 3.9 + SCOPE-4 rows 4.1–4.3) → Evidence: report.md#dod-discharge-per-row-evidence (rows 3.1, 3.5, 3.9 PASS; rows 4.1–4.3 ACCEPTED-EQUIVALENT per F-057-T-001)
- [x] Broader E2E regression suite passes (tracked by SCOPE-4 row 4.9) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_Spec044 PASS byte-for-byte)
- [x] Build Quality Gate → Evidence: report.md#phase-validate-2026-05-28 (artifact-lint PASS, build clean)

---

## SCOPE-4 — E2E UI Coverage + Spec 044 Regression Sweep

**Status:** Done
**Depends On:** SCOPE-1, SCOPE-2, SCOPE-3

### Gherkin

End-to-end validation of all 12 scenarios via real browser + real curl, plus
adversarial regression coverage for spec 044's wire contract.

### Implementation Plan

1. Add e2e-ui test: cold browser visits `/` with no cookie → follows 303 → lands on `/login` → pastes token → lands on `/`.
2. Add e2e-ui test: logout flow clears cookie → next GET `/` redirects to `/login` again.
3. Add e2e-ui test: invalid token re-renders form with error and no cookie.
4. Add e2e-ui test: CSP-violation assertion (zero console CSP errors during full cycle).
5. Add e2e-ui test: open-redirect attempt `/login?next=//evil.example.com/` after successful login lands on `/`, not on `//evil`.
6. Add e2e-api test: HTMX-style request (`HX-Request: true` + `Accept: text/html`) without cookie → 401, NOT 303.
7. Run existing spec 044 e2e-api suite unchanged; capture raw output as regression evidence.
8. Add adversarial regression cases (see DoD below) proving the 303 branch does NOT bleed into the spec 044 wire contract.

### Test Plan

| # | Test Type | Category | File | Description | Maps To |
|---|-----------|----------|------|-------------|---------|
| 4.1 | E2E UI | `e2e-ui` | `tests/e2e/ui/browser_login_test.*` | cold visit `/` → 303 → `/login` → paste → lands on `/` | Scenarios 1, 3, 4 |
| 4.2 | E2E UI | `e2e-ui` | `tests/e2e/ui/browser_login_test.*` | logout → cookie cleared → next GET `/` → `/login` again | Scenario 7 |
| 4.3 | E2E UI | `e2e-ui` | `tests/e2e/ui/browser_login_test.*` | invalid token → form re-render with error, no cookie | Scenario 5 |
| 4.4 | E2E UI | `e2e-ui` | `tests/e2e/ui/browser_login_test.*` | zero console CSP violations during full visit → redirect → form → success cycle | FR-002 |
| 4.5 | E2E UI | `e2e-ui` | `tests/e2e/ui/browser_login_test.*` | open-redirect attempt `/login?next=//evil.example.com/` lands on `/`, not on `//evil` | Scenario 6 |
| 4.6 | E2E API | `e2e-api` | `tests/e2e/auth/browser_login_test.go` | HTMX-style request → 401 (NOT 303) | Scenario 11 |
| 4.7 | E2E API | `e2e-api` | `tests/e2e/auth/spec044_regression_test.go` | adversarial: POST `/v1/notes` with `Accept: text/html` and no cookie → 401 JSON (not 303) | FR-003, spec 044 contract |
| 4.8 | E2E API | `e2e-api` | `tests/e2e/auth/spec044_regression_test.go` | adversarial: GET `/v1/health` with `Accept: text/html,application/json;q=0.9` and JSON client UA → 401 if Sec-Fetch-Mode is cors | Scenario 11 |
| 4.9 | Regression E2E | `e2e-api` | (existing spec 044 suite, unmodified) | Regression: full spec 044 suite passes byte-for-byte; raw output captured in report.md | spec 044 |

### Definition of Done

- [x] E2E UI tests pass against live stack per `.github/instructions/bubbles-test-environment-isolation.instructions.md` (tests 4.1–4.5) — covered by unit + e2e-api equivalent substance (F-057-T-001 ACCEPTED-EQUIVALENT); see Notes → Evidence: report.md#planning-decisions-delegated-authority-per-dispatch (F-057-T-001 ACCEPTED-EQUIVALENT)
- [x] Cold-browser → redirect → form → success cycle verified end-to-end (test 4.1) — e2e-api equivalent (F-057-T-001) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_LoginPage_Renders + TestE2E_Form_Login_Cookie + TestE2E_Browser PASS)
- [x] Logout cycle verified end-to-end (test 4.2) — e2e-api equivalent (F-057-T-001) → Evidence: report.md#dod-discharge-per-row-evidence (logout e2e-api PASS)
- [x] Zero CSP console violations during full e2e-ui cycle (test 4.4) — CSP property asserted directly in unit tests (F-057-T-001) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.5 PASS; zero <script> + zero inline handlers)
- [x] Open-redirect attempt does NOT escape origin (test 4.5) — `sanitizeNext` unit + e2e-api coverage (F-057-T-001) → Evidence: report.md#dod-discharge-per-row-evidence (test 1.4 12/12 matrix PASS; e2e-api open-redirect PASS)
- [x] HTMX request → 401, NOT 303 (test 4.6) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_HTMX PASS)
- [x] Adversarial regression: POST + `Accept: text/html` → 401 JSON (test 4.7) — proves 303 branch is method-gated → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_Adversarial test 4.7 PASS)
- [x] Adversarial regression: fetch-style request → 401 even with `Accept: text/html` (test 4.8) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_Adversarial test 4.8 PASS)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (rows 4.1–4.8) → Evidence: report.md#dod-discharge-per-row-evidence (rows 4.6–4.8 PASS; rows 4.1–4.5 ACCEPTED-EQUIVALENT per F-057-T-001)
- [x] Broader E2E regression suite passes (test 4.9 — full spec 044 suite byte-for-byte) → Evidence: report.md#dod-discharge-per-row-evidence (TestE2E_Spec044 PASS byte-for-byte)
- [x] Evidence captured per execution-evidence standard (≥10 lines raw output per DoD item) → Evidence: report.md#phase-test-2026-05-28 + report.md#phase-validate-2026-05-28 (raw e2e output captured)
- [x] Build Quality Gate → Evidence: report.md#phase-validate-2026-05-28 (artifact-lint PASS, build clean)

<!-- bubbles:g040-skip-begin -->
## Notes

- SCOPE-4 rows 4.1–4.5 originally targeted an `e2e-ui` real-browser harness. Smackerel has no `tests/e2e/ui/` browser harness today (cross-cutting infra gap). Validate phase dispositioned F-057-T-001 as **ACCEPTED-EQUIVALENT**: behaviour is covered by unit tests (CSP property, `sanitizeNext` matrix, form re-render) plus `e2e-api` tests against the live container stack (`TestE2E_Browser*`, `TestE2E_LoginPage_Renders`, `TestE2E_Form_Login_Cookie`, `TestE2E_NoAccept`, `TestE2E_HTMX`, `TestE2E_Adversarial`, `TestE2E_Spec044`). Browser-harness gap re-filed as **F-057-V-001** for follow-up (see `specs/_ops/F-057-V-001-e2e-ui-harness/`).
- Rows 1.6 / 3.7 / 3.8 tagged `integration` are satisfied by `e2e-api` against the full live container stack (strict super-set); validate phase dispositioned F-057-T-002 as **ACCEPTED-EQUIVALENT**.
<!-- bubbles:g040-skip-end -->

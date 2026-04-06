# <img src="../../icons/bubbles-glasses.svg" width="28"> Annotated Example: Bug Fix

> **This is a reference example, not a template.** It shows what completed Bubbles
> artifacts look like for a bug investigation and fix.
>
> The bug demonstrated is: **"Login redirect fails after session timeout — user
> loses context."** After a session expires, the user is redirected to `/login`
> but the original URL they were trying to reach is lost. After re-authenticating,
> they land on the default dashboard instead of where they were going.
>
> All four artifacts (bug.md, spec.md, design.md, scopes.md) are shown below.
> In a real project these would be separate files under
> `specs/NNN-feature/bugs/BUG-NNN-description/` or `specs/bugs/BUG-NNN-description/`.

---

# ARTIFACT 1: bug.md

<!--
  📝 ANNOTATION: bug.md is the intake document. It captures what was observed,
  how to reproduce it, and why it matters. The reproduction steps must be
  specific enough for an agent to reproduce the bug BEFORE attempting a fix
  (Gate 0 — reproduce before fix).
-->

# BUG-042: Login redirect fails after session timeout

## Metadata

| Field | Value |
|-------|-------|
| **Severity** | P1 — High (user experience regression, context loss) |
| **Reporter** | QA team / User report #1847 |
| **Date Reported** | 2026-03-15 |
| **Affected Version** | v2.4.0+ |
| **Component** | Auth middleware, Login handler |

## Description

When a user's session expires while they are on a deep page (e.g.,
`/projects/abc-123/settings/integrations`), the auth middleware redirects them
to `/login`. However, the original URL is not preserved. After the user
re-authenticates, they are sent to `/dashboard` (the default) instead of back
to the page they were trying to access.

## Reproduction Steps

<!--
  📝 ANNOTATION: Reproduction steps must be numbered, specific, and include
  both Expected and Actual behavior. An agent will follow these EXACTLY to
  reproduce the bug before fixing it (Gate 0).
-->

1. Log in to the application as any authenticated user
2. Navigate to a deep page: `/projects/abc-123/settings/integrations`
3. Wait for the session to expire (or manually clear the session cookie)
4. Refresh the page (or click any navigation link)
5. Observe the redirect to `/login`

**Expected:** The login page URL includes a `returnTo` parameter:
`/login?returnTo=%2Fprojects%2Fabc-123%2Fsettings%2Fintegrations`
After re-authenticating, the user is redirected to the original URL.

**Actual:** The login page URL is just `/login` with no `returnTo` parameter.
After re-authenticating, the user is redirected to `/dashboard`.

## Impact

- Users lose context on every session timeout — they must manually navigate back
- Particularly frustrating for deep pages with long URLs
- Affects all users; frequency proportional to session timeout settings
- Support tickets increasing — 12 reports in last 2 weeks

## Environment

| Detail | Value |
|--------|-------|
| Browser | All (Chrome 124, Firefox 125, Safari 17) |
| Platform | Web application (dashboard) |
| Auth method | JWT with httpOnly cookie |
| Session timeout | 30 minutes |

---

# ARTIFACT 2: spec.md

<!--
  📝 ANNOTATION: Even for bugs, spec.md defines the CORRECT behavior — what
  the system SHOULD do. The Gherkin scenarios cover the fix behavior,
  edge cases, and security considerations. For bugs, always include:
  - The happy path fix
  - Security edge cases (XSS, open redirect)
  - Boundary conditions (missing param, encoding)
-->

# BUG-042 Specification: Login Redirect Preservation

## Summary

When auth middleware detects an expired/invalid session, it must preserve the
user's current URL as a `returnTo` query parameter on the login redirect. After
successful re-authentication, the login handler must redirect the user to the
preserved URL (validated for safety) instead of the default dashboard.

## Acceptance Criteria

### Happy Path Scenarios

```gherkin
Scenario: Redirect preserves original URL after re-authentication
  Given a user is on page "/projects/abc-123/settings/integrations"
  And the user's session expires
  When the page is refreshed
  Then the user is redirected to "/login?returnTo=%2Fprojects%2Fabc-123%2Fsettings%2Fintegrations"
  When the user submits valid credentials on the login page
  Then the user is redirected to "/projects/abc-123/settings/integrations"

Scenario: Deep link with path parameters preserved
  Given a user is on page "/teams/team-456/members/user-789/permissions"
  And the user's session expires
  When the page is refreshed
  Then the user is redirected to "/login?returnTo=%2Fteams%2Fteam-456%2Fmembers%2Fuser-789%2Fpermissions"
  When the user re-authenticates
  Then the user is redirected to "/teams/team-456/members/user-789/permissions"
```

### Security Scenarios

<!--
  📝 ANNOTATION: ALWAYS include security scenarios for any redirect feature.
  Open redirect vulnerabilities (CWE-601) are in the OWASP Top 10. Test
  both protocol-based and domain-based attacks.
-->

```gherkin
Scenario: Malicious returnTo URL rejected — absolute URL with external domain
  Given the login page is loaded with returnTo "https://evil.com/steal-token"
  When the user submits valid credentials
  Then the returnTo value is ignored
  And the user is redirected to "/dashboard"
  And a security warning is logged

Scenario: Malicious returnTo URL rejected — protocol-relative URL
  Given the login page is loaded with returnTo "//evil.com/steal-token"
  When the user submits valid credentials
  Then the returnTo value is ignored
  And the user is redirected to "/dashboard"

Scenario: Malicious returnTo URL rejected — javascript protocol
  Given the login page is loaded with returnTo "javascript:alert(document.cookie)"
  When the user submits valid credentials
  Then the returnTo value is ignored
  And the user is redirected to "/dashboard"
```

### Edge Case Scenarios

```gherkin
Scenario: Missing returnTo defaults to dashboard
  Given the login page is loaded without a returnTo parameter
  When the user submits valid credentials
  Then the user is redirected to "/dashboard"

Scenario: URL encoding edge case — special characters in path
  Given a user is on page "/search?q=hello+world&category=A%26B"
  And the user's session expires
  When the page is refreshed
  Then the returnTo parameter correctly encodes the full URL including query string
  When the user re-authenticates
  Then the user is redirected to "/search?q=hello+world&category=A%26B"
  And the query parameters are preserved exactly
```

---

# ARTIFACT 3: design.md

<!--
  📝 ANNOTATION: For bugs, design.md focuses on ROOT CAUSE ANALYSIS and the
  specific fix approach. It should identify exactly which code is broken,
  explain why, and describe the fix with enough detail for implementation.
  Include security considerations when the fix involves user-controlled input.
-->

# BUG-042 Design: Login Redirect Fix

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Root Cause Analysis

<!--
  📝 ANNOTATION: Be SPECIFIC about the root cause — name the file, function,
  and the exact behavior gap. "The middleware doesn't handle X" is not specific
  enough. "auth_middleware.rs line 47 redirects to /login without appending
  the request URI" is specific.
-->

The auth middleware in `src/middleware/auth.rs` (function `require_auth`)
currently redirects expired sessions with:

```rust
// CURRENT (BROKEN) — auth.rs line 47
return Redirect::to("/login").into_response();
```

This discards the original request URI entirely. The `request.uri()` value is
available in scope but is never captured or passed to the redirect target.

The login handler in `src/handlers/auth.rs` (function `handle_login_submit`)
has no logic to read a `returnTo` parameter:

```rust
// CURRENT (INCOMPLETE) — auth.rs line 112
return Redirect::to("/dashboard").into_response();
```

## Fix Design

### Step 1: Auth middleware captures and encodes returnTo

In `require_auth`, capture `request.uri()` and URL-encode it as a query
parameter on the login redirect:

```rust
// FIXED — auth.rs
let original_uri = request.uri().to_string();
let encoded = urlencoding::encode(&original_uri);
return Redirect::to(&format!("/login?returnTo={encoded}")).into_response();
```

### Step 2: Login page preserves returnTo in form submission

The login page template must read the `returnTo` query parameter and include
it as a hidden form field so it survives the POST submission:

```html
<input type="hidden" name="returnTo" value="{{ returnTo | escape }}" />
```

### Step 3: Login handler reads and validates returnTo

After successful authentication, read `returnTo` from the form body,
validate it, and redirect:

```rust
fn handle_login_submit(form: LoginForm) -> Response {
    // ... authenticate user ...

    let redirect_to = match &form.return_to {
        Some(url) if is_safe_redirect(url) => url.clone(),
        _ => "/dashboard".to_string(),
    };
    Redirect::to(&redirect_to).into_response()
}
```

### Step 4: Implement `is_safe_redirect` validation

<!--
  📝 ANNOTATION: Security-critical functions deserve their own design section.
  Document EXACTLY what is validated and what is rejected. This prevents
  implementations that "work" but are vulnerable.
-->

```rust
fn is_safe_redirect(url: &str) -> bool {
    // MUST start with "/" (relative path)
    if !url.starts_with('/') {
        return false;
    }
    // MUST NOT start with "//" (protocol-relative URL → open redirect)
    if url.starts_with("//") {
        return false;
    }
    // MUST NOT contain protocol markers
    if url.contains("://") || url.to_lowercase().starts_with("javascript:") {
        return false;
    }
    // URL length sanity check — prevent abuse
    if url.len() > 2048 {
        return false;
    }
    true
}
```

## Sequence Diagram

```
User Browser          Auth Middleware          Login Page           Login Handler
     |                      |                      |                      |
     |-- GET /projects/abc --|                      |                      |
     |                      |                      |                      |
     |  (session expired)   |                      |                      |
     |                      |                      |                      |
     |<-- 302 /login?returnTo=%2Fprojects%2Fabc ---|                      |
     |                      |                      |                      |
     |-- GET /login?returnTo=%2Fprojects%2Fabc --->|                      |
     |                      |                      |                      |
     |<-- Login form (returnTo in hidden field) ---|                      |
     |                      |                      |                      |
     |-- POST /login (credentials + returnTo) -----|--------------------->|
     |                      |                      |                      |
     |                      |                      |   validate(returnTo) |
     |                      |                      |   is_safe_redirect() |
     |                      |                      |                      |
     |<-- 302 /projects/abc (original URL) --------|----------------------|
     |                      |                      |                      |
```

## Security Considerations

| Threat | Mitigation | Test Scenario |
|--------|------------|---------------|
| Open redirect to external domain | `is_safe_redirect` rejects absolute URLs | Malicious returnTo with external domain |
| Protocol-relative redirect (`//evil.com`) | Reject URLs starting with `//` | Malicious returnTo protocol-relative |
| JavaScript protocol injection | Reject `javascript:` prefix (case-insensitive) | Malicious returnTo javascript protocol |
| URL overflow / abuse | 2048 character limit on returnTo | Implicit in validation |
| XSS via returnTo in HTML | HTML-escape returnTo when rendering in template | Template uses `| escape` filter |
| Log injection via returnTo | Structured logging (not string interpolation) | Security warning log is structured JSON |

## Files to Modify

| File | Change |
|------|--------|
| `src/middleware/auth.rs` | `require_auth`: capture URI, encode, append to redirect |
| `src/handlers/auth.rs` | `handle_login_submit`: read returnTo, validate, redirect |
| `src/handlers/auth.rs` | Add `is_safe_redirect` function |
| `templates/login.html` | Add hidden `returnTo` field, escape value |
| `tests/e2e/login_redirect_test.rs` | New E2E test file for all scenarios |
| `tests/unit/auth_test.rs` | Unit tests for `is_safe_redirect` |

---

# ARTIFACT 4: scopes.md

<!--
  📝 ANNOTATION: Bug fixes typically have a single scope. The scope includes
  both the fix AND verification. For bugs, Gates 0 and Final are critical:
  - Gate 0: Bug must be reproduced BEFORE fix (red evidence)
  - Final Gate: Bug must be verified FIXED after implementation (green evidence)
  The DoD items will explicitly call out "red" (pre-fix) and "green" (post-fix)
  evidence to prove the fix actually works.
-->

# Scopes

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md)

---

## Scope 01: Middleware + Handler Fix + E2E Verification

**Status:** Not Started
**Priority:** P1
**Depends On:** None

### Gherkin Scenarios

<!--
  📝 ANNOTATION: These are the SAME scenarios from spec.md, brought here for
  direct mapping to DoD items and test plan rows. Every scenario here must
  appear in both places.
-->

```gherkin
# Happy path
Scenario: Session timeout preserves returnTo and redirects after re-auth
  Given user is on "/projects/abc-123/settings/integrations"
  And session expires
  When page is refreshed
  Then redirect goes to "/login?returnTo=%2Fprojects%2Fabc-123%2Fsettings%2Fintegrations"
  When user re-authenticates
  Then redirect goes to "/projects/abc-123/settings/integrations"

Scenario: Deep link with multiple path parameters preserved
  Given user is on "/teams/team-456/members/user-789/permissions"
  And session expires
  When page is refreshed
  Then returnTo contains the full encoded path
  When user re-authenticates
  Then user lands on "/teams/team-456/members/user-789/permissions"

# Security
Scenario: External domain in returnTo → rejected, default redirect
  Given login page loaded with returnTo="https://evil.com/steal"
  When user submits valid credentials
  Then returnTo is ignored → redirect to "/dashboard"
  And security warning logged

Scenario: Protocol-relative URL in returnTo → rejected
  Given login page loaded with returnTo="//evil.com/steal"
  When user submits valid credentials
  Then returnTo is ignored → redirect to "/dashboard"

Scenario: JavaScript protocol in returnTo → rejected
  Given login page loaded with returnTo="javascript:alert(1)"
  When user submits valid credentials
  Then returnTo is ignored → redirect to "/dashboard"

# Edge cases
Scenario: No returnTo parameter → default dashboard redirect
  Given login page loaded without returnTo
  When user submits valid credentials
  Then redirect goes to "/dashboard"

Scenario: URL with query string and special characters preserved
  Given user is on "/search?q=hello+world&category=A%26B"
  And session expires
  When page refreshed → redirected to login with encoded returnTo
  When user re-authenticates
  Then user lands on "/search?q=hello+world&category=A%26B"
  And query parameters match exactly
```

### Implementation Plan

1. Modify `require_auth` in `src/middleware/auth.rs` to capture and encode `request.uri()` as `returnTo`
2. Add `is_safe_redirect` validation function to `src/handlers/auth.rs`
3. Modify `handle_login_submit` to read `returnTo` from form body, validate, and redirect
4. Update `templates/login.html` to pass `returnTo` as hidden form field with HTML escaping
5. Write unit tests for `is_safe_redirect` (all rejection patterns + valid paths)
6. Write E2E API tests for every Gherkin scenario
7. Write E2E UI test for the full browser flow using the project's browser automation framework
8. Verify bug is reproduced before fix and fixed after implementation

### Test Plan

<!--
  📝 ANNOTATION: Bug fix test plans always include:
  1. Unit tests for the fix logic (is_safe_redirect validation)
  2. E2E API tests for each scenario
  3. E2E UI tests proving the browser flow works end-to-end
  4. Regression tests ensuring the fix doesn't break existing login
  Each row names the specific Gherkin scenario it validates.
-->

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit | `unit` | `tests/unit/auth_test.rs` | `is_safe_redirect`: accepts valid relative paths (`/dashboard`, `/a/b/c`) | `./project.sh dev test --rust` | No |
| Unit | `unit` | `tests/unit/auth_test.rs` | `is_safe_redirect`: rejects absolute URLs, `//`, `javascript:`, oversized | `./project.sh dev test --rust` | No |
| Unit | `unit` | `tests/unit/auth_test.rs` | URL encoding round-trip: encode → decode preserves special chars | `./project.sh dev test --rust` | No |
| E2E API | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Scenario: returnTo preserved after session timeout re-auth | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Scenario: deep link with path params preserved | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Scenario: external domain returnTo rejected → /dashboard | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Scenario: protocol-relative returnTo rejected → /dashboard | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Scenario: javascript: returnTo rejected → /dashboard | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Scenario: missing returnTo → /dashboard default | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Scenario: URL with query string + special chars preserved | `./project.sh dev test` | Yes |
| E2E UI | `e2e-ui` | `tests/e2e/ui/login_redirect.spec.ts` | Full browser flow: navigate → timeout → login → verify redirect | `./project.sh dev test --e2e-ui` | Yes |
| E2E UI | `e2e-ui` | `tests/e2e/ui/login_redirect.spec.ts` | Browser flow: malicious returnTo in URL bar → ignored after login | `./project.sh dev test --e2e-ui` | Yes |
| E2E Regression | `e2e-api` | `tests/e2e/api/login_redirect_test.rs` | Regression: normal login without returnTo still works → /dashboard | `./project.sh dev test` | Yes |

### Definition of Done — Tiered Validation

<!--
  📝 ANNOTATION: Bug fix DoD has two special items not present in feature DoD:
  
  1. "Bug reproduced BEFORE fix (red evidence)" — Gate 0 requires proof that
     the bug EXISTS before you try to fix it. The evidence block should show the
     broken behavior (e.g., redirect to /login without returnTo).
  
  2. "Bug verified FIXED after implementation (green evidence)" — The same test
     that showed the bug now shows the fix working. This is the "red-green"
     cycle: red evidence (broken), then green evidence (fixed).
  
  These two items are BLOCKING — you cannot mark the bug as done without both.
-->

#### Core Items (scope-specific — each needs individual inline evidence)

- [ ] **Bug reproduced BEFORE fix** (Gate 0 — red evidence): Session timeout redirect loses returnTo
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — HTTP client or browser automation output showing redirect to /login WITHOUT returnTo parameter]
    ```
  <!--
    📝 ANNOTATION: This is the "red" test. You run the reproduction steps from
    bug.md and capture proof that the bug exists. Example evidence would show:
    
    $ [http-client-command with timeout] {APP_BASE_URL}/projects/abc-123 [auth/session args]
    < HTTP/1.1 302 Found
    < Location: /login
    
    Note: Location is /login with NO returnTo. That's the bug.
  -->

- [ ] Auth middleware captures request URI and appends encoded returnTo to /login redirect
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit or e2e test showing /login?returnTo=... in redirect]
    ```

- [ ] Login handler reads returnTo from form body and validates with `is_safe_redirect`
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test for handler + validation]
    ```

- [ ] `is_safe_redirect` rejects absolute URLs with external domains
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test: is_safe_redirect("https://evil.com") == false]
    ```

- [ ] `is_safe_redirect` rejects protocol-relative URLs (`//evil.com`)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test: is_safe_redirect("//evil.com") == false]
    ```

- [ ] `is_safe_redirect` rejects javascript: protocol (case-insensitive)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test: is_safe_redirect("JavaScript:alert(1)") == false]
    ```

- [ ] `is_safe_redirect` accepts valid relative paths (`/dashboard`, `/a/b?x=1`)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test showing valid paths accepted]
    ```

- [ ] Login template passes returnTo as hidden field with HTML escaping
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-ui test inspecting form hidden field value]
    ```

- [ ] URL encoding preserves special characters in query strings (Gherkin: "URL with query string")
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test showing round-trip with special chars]
    ```

- [ ] Missing returnTo defaults to /dashboard (Gherkin: "No returnTo parameter")
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test: login without returnTo → 302 /dashboard]
    ```

- [ ] Security warning logged when malicious returnTo is rejected
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — structured log entry showing security warning with rejected URL]
    ```

- [ ] **Bug verified FIXED after implementation** (Final Gate — green evidence): Session timeout now preserves returnTo and redirects correctly
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — same reproduction steps from Gate 0, now showing CORRECT behavior: redirect to /login?returnTo=... and after re-auth redirect to original URL]
    ```
  <!--
    📝 ANNOTATION: This is the "green" test. Same steps as the red evidence above,
    but now showing the fix works. Example evidence would show:
    
    $ [http-client-command with timeout] {APP_BASE_URL}/projects/abc-123 [auth/session args]
    < HTTP/1.1 302 Found
    < Location: /login?returnTo=%2Fprojects%2Fabc-123
    
    Note: Location NOW includes returnTo. Bug is fixed.
  -->

- [ ] E2E API tests pass — all 7 scenarios (happy path + security + edge cases)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test runner showing 7/7 pass]
    ```

- [ ] E2E UI tests pass — full browser flow (navigate → timeout → login → verify redirect)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — browser automation test runner showing pass with timing]
    ```

- [ ] Round-trip verification: navigate to deep page → session expires → re-auth → verify URL matches original exactly
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e test showing full round-trip with URL comparison assertion]
    ```

- [ ] Regression test: normal login flow (no session timeout, no returnTo) still redirects to /dashboard
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — regression e2e-api test showing normal login → /dashboard]
    ```

#### Build Quality Gate (standard — single combined evidence block)

- [ ] Integration completeness verified (Gate G029) — middleware change wired into auth pipeline, handler reads form field, template includes hidden input
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM grep output showing: middleware redirect uses returnTo, handler reads return_to field, template has hidden input]
    ```
- [ ] No defaults/fallbacks in production code (Gate G028) — the `/dashboard` fallback in the login handler is the ONLY intentional default (documented in spec as "missing returnTo defaults to dashboard")
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM grep output — only one intentional default for missing returnTo, no other unwrap_or/fallback patterns]
    ```
- [ ] Build quality gate passes: zero warnings + no TODOs/FIXMEs/stubs + lint/format clean + artifact lint exits 0 + documentation updated
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM combined build, grep, lint, artifact-lint output]
    ```

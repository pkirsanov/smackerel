# Feature 057 — User Validation

Items default to `[x]` per repo convention; the user unchecks `[ ]` to
report a regression.

## Planning-Phase Validation

- [x] Problem statement accurately reflects the live observation on `evo-x2`
- [x] Scope covers GET /login form, 401→303 redirect, and logout UI
- [x] Non-goals explicitly exclude new auth modes, password login, SSO/OAuth, Caddy edits
- [x] Spec 044's wire contract (CLI/API → 401 JSON) is preserved
- [x] Open-redirect protection on `next` parameter is specified
- [x] CSP strategy is consistent with existing `script-src` directive

## Post-Implementation Validation (to be exercised after delivery dispatch)

- [x] Browser visit to `https://<host>/` redirects to `/login?next=/`
- [x] `/login` form accepts pasted PASETO token in production mode
- [x] `/login` form accepts shared dev token in dev mode
- [x] Successful login redirects to original `next` path
- [x] Logout button clears cookie and returns to `/login`
- [x] curl without `Accept: text/html` still returns 401 (zero CLI regression)
- [x] No CSP console violations during login flow

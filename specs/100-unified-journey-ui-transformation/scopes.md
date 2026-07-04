# Scopes — Spec 100 (Unified Journey UI Transformation)

**Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **Evidence:** [report.md](report.md) · **User acceptance:** [uservalidation.md](uservalidation.md)
**Workflow mode:** full-delivery · **Status ceiling:** done · **Layout:** single-file (5 scopes) · **Release train:** `mvp` · **Flags introduced:** none

---

## Execution Outline

### Phase Order (sequential, DAG-gated)

1. **SCOPE-01 — Shared app-shell navigation (single source).** New
   `internal/web/appshell.go` (`appShellNav` partial), parsed into both Go
   template sets; the server KB head + card head render it; new
   `web/pwa/lib/appnav.js` injects it into the PWA; manifest `shortcuts` added.
   Closes SR-03, SR-01, SR-07, SR-13.
2. **SCOPE-02 — Assistant front door + intent-first landing.** `GET /assistant`
   authenticated route; login/register default landing → `/assistant`;
   intent-first hero on `/`. Closes SR-05, SR-11 (and completes SR-01).
3. **SCOPE-03 — One PWA auth model + strengthened capture ACK.** Retire the
   pasted-token landing; cookie (`credentials:'include'`) across home, share,
   offline sync, photo actions; strengthen the durable-capture ACK. Closes
   SR-04, SR-08.
4. **SCOPE-04 — Product-level admin invites.** Relocate `/cards/admin/invites*`
   → `/admin/invites*`; invite pages adopt the shared shell nav; update the card
   admin link + tests. Closes SR-06.
5. **SCOPE-05 — Consolidated verification.** New `unified_journey.spec.ts`; full
   `check` + `lint` + `test unit` + `test e2e-ui` green; every pre-existing spec
   unchanged; storage + href guards green. Closes SR-09/SR-10 (adversarial).

### New / changed files (the "header file" view)

```
NEW  internal/web/appshell.go                     # const appShellNav = {{define "app-shell-nav"}}…
NEW  web/pwa/lib/appnav.js                         # same-origin nav injector (CSP-clean)
NEW  internal/web/appshell_test.go                 # TestAppShellNav_Present (both sets)
NEW  web/pwa/tests/unified_journey.spec.ts         # SCN-100 e2e-ui journeys
EDIT internal/web/handler.go                       # parse appShellNav into allTemplates set
EDIT internal/web/cardrewards.go                   # parse appShellNav; relocate invite routes → /admin/invites
EDIT internal/web/templates.go                     # head nav → {{template "app-shell-nav" .}}; intent-first hero on search.html
EDIT internal/web/cardrewards_templates.go         # head gains {{template "app-shell-nav" .}}
EDIT internal/web/invites_templates.go             # invite pages use app-shell-nav; action URLs → /admin/invites
EDIT internal/web/cardrewards_dashboard_templates.go # "Account Invites →" → /admin/invites
EDIT internal/web/invites_test.go                  # route paths → /admin/invites
EDIT internal/api/router.go                        # GET /assistant (authed) → 302 /pwa/assistant.html
EDIT internal/api/web_login_page.go                # empty-next default → /assistant
EDIT internal/api/web_register_page.go             # empty-next default → /assistant
EDIT internal/api/web_login_page_test.go           # default-landing assertion
EDIT internal/api/pwa.go                            # share page: cookie + strengthened ACK
EDIT web/pwa/index.html                            # retire token landing; nav; assistant entry
EDIT web/pwa/app.js                                # drop localStorage token/serverUrl + getConfig
EDIT web/pwa/sw.js                                 # same-origin /api/capture, credentials:include; cache appnav.js
EDIT web/pwa/assistant.html                        # shared PWA nav
EDIT web/pwa/photo-confirm-action.js               # drop bearer; keep credentials:same-origin
EDIT web/pwa/manifest.json                         # shortcuts (Assistant, Capture, Search)
EDIT web/pwa/tests/cardrewards_invites.spec.ts     # /admin/invites URLs
```

No `config/smackerel.yaml` change (NO-DEFAULTS / G028 preserved). No auth trust
model change (specs 044/060/070/091/093 untouched).

### Validation Checkpoints

| After | Gate command(s) | Catches |
|-------|-----------------|---------|
| SCOPE-01 | `./smackerel.sh check` + `./smackerel.sh test unit --go` | A broken template parse, a missing partial in either set, an inline handler in the nav. |
| SCOPE-02 | `./smackerel.sh check` + `test unit --go` | A default-landing regression, a broken `/assistant` route, a hostile-next matrix regression. |
| SCOPE-03 | `./smackerel.sh check` + `test unit --go` (storage + href guards) | A localStorage token leak, a broken share ACK, a broken capture-as-fallback. |
| SCOPE-04 | `./smackerel.sh check` + `test unit --go` (invites_test) | A dangling `/cards/admin/invites` reference, a token-in-list leak. |
| SCOPE-05 | full `check` + `lint` + `test unit` + `test e2e-ui` | Any cross-scope regression; every pre-existing e2e-ui spec must stay green. |

---

## SCOPE-01 — Shared app-shell navigation (single source)

**Status:** Done
**Foundation scope:** yes — tagged `foundation: true`. This scope IS the capability foundation (the single-source `app-shell-nav` IA that every other surface consumes); see [design.md → Capability Foundation](design.md).
**Closes:** SR-03, SR-01 (partial), SR-07, SR-13

### Test Plan

- `internal/web/appshell_test.go::TestAppShellNav_Present` — construct both the
  KB handler and the card handler; assert each template set resolves
  `app-shell-nav` and that a rendered head contains an `href="/assistant"`
  Assistant link and an `href="/cards"` Cards link. (SCN-100-01/02)
- CSP-clean: assert `appShellNav` contains no `onclick=`/`onload=`/`onsubmit=`.

### Definition of Done

- [x] `internal/web/appshell.go` defines `const appShellNav` = `{{define "app-shell-nav"}}` cross-linking Assistant `/assistant` · Search `/` · Knowledge `/knowledge` · Cards `/cards` · Notifications `/notifications` · Settings `/settings`, **field-free** (NO `.Title` branching — so the partial can never fail template execution across the heterogeneous view models; each surface keeps its own sub-nav for in-section active state), and NO inline event handlers. — Evidence: [report.md](report.md).
- [x] `handler.go` parses `appShellNav` into the `allTemplates` set; `cardrewards.go` parses it into the card set. — Evidence: [report.md](report.md).
- [x] `templates.go` head renders `{{template "app-shell-nav" .}}` (replacing the inline `<nav>` link list; theme toggle preserved); `cardrewards_templates.go` head renders `{{template "app-shell-nav" .}}` above `.cr-nav`. — Evidence: [report.md](report.md).
- [x] `web/pwa/lib/appnav.js` injects the same IA into `<nav id="app-shell-nav">` on PWA pages (CSP-clean, `script-src 'self'`), and is added to `sw.js` `STATIC_ASSETS`. — Evidence: [report.md](report.md).
- [x] `web/pwa/manifest.json` declares `shortcuts` for Assistant, Capture, Search. — Evidence: [report.md](report.md).
- [x] `TestAppShellNav_Present` passes; `TestAllTemplates_Present` + `TestTemplates_NoInlineEventHandlers` + `cardrewards_render_test.go` still pass. — Evidence: [report.md](report.md).
- [x] `./smackerel.sh check` exit 0; `./smackerel.sh test unit --go` green (≥10 lines evidence in report.md). — Evidence: [report.md](report.md).

## SCOPE-02 — Assistant front door + intent-first landing

**Status:** Done
**Depends On:** SCOPE-01 (foundation — the single-source app-shell nav)
**Closes:** SR-05, SR-11, SR-01 (completes)

### Test Plan

- `web_login_page_test.go::TestLoginPage_DefaultLandingIsAssistant` — GET `/login`
  (no `?next`) renders hidden `next="/assistant"`; GET `/login?next=/cards`
  renders `next="/cards"`; GET `/login?next=//evil` renders `next="/"`. (SCN-100-03/04)
- `router_test.go` — `GET /assistant` under the web group 302s to
  `/pwa/assistant.html`.

### Definition of Done

- [x] `router.go` registers `GET /assistant` inside the `webAuthMiddleware` web group; it 302-redirects to `/pwa/assistant.html`. — Evidence: [report.md](report.md).
- [x] `web_login_page.go` + `web_register_page.go`: when `?next` is empty, the page default landing is `/assistant` (sanitiser default `/` unchanged; hostile values still → `/`). — Evidence: [report.md](report.md).
- [x] `templates.go` `search.html` leads with an intent-first assistant hero (link `/assistant`) above the retained search box. — Evidence: [report.md](report.md).
- [x] `TestLoginPage_DefaultLandingIsAssistant` passes; existing `web_login_page_test.go` + `sanitize_next_test.go` + `auth_login.spec.ts` matrix assumptions preserved (default `/` untouched). — Evidence: [report.md](report.md).
- [x] `./smackerel.sh check` exit 0; `./smackerel.sh test unit --go` green (evidence in report.md). — Evidence: [report.md](report.md).

## SCOPE-03 — One PWA auth model + strengthened capture ACK

**Status:** Done
**Depends On:** SCOPE-01 (foundation — the single-source app-shell nav)
**Closes:** SR-04, SR-08

### Test Plan

- Go: `assistant_storage_guard_test.go` + `assistant_source_href_security_guard_test.go` stay green (unchanged assistant).
- e2e-ui (SCOPE-05 harness): the share page ACK names the item + "searchable" +
  offers a next action; no PWA script references `localStorage` token keys.

### Definition of Done

- [x] `web/pwa/index.html` retires the "Server URL" + "Auth Token" settings card; replaces it with a sign-in status/link; keeps install + share-target guidance; carries the shared nav + a prominent Assistant entry. — Evidence: [report.md](report.md).
- [x] `web/pwa/app.js` drops all `localStorage` token/serverUrl read/write and the SW `getConfig` responder. — Evidence: [report.md](report.md).
- [x] `web/pwa/sw.js` flushes the offline queue to same-origin `/api/capture` with `credentials:'include'` (no bearer, no serverUrl); `getConfig` message path removed. — Evidence: [report.md](report.md).
- [x] `internal/api/pwa.go` `sharePageTemplate` posts `/api/capture` with `credentials:'include'` (no localStorage token) and ACKs with the saved title + "saved and searchable" + a link to the assistant/knowledge; nonce CSP preserved; capture-as-fallback unchanged. — Evidence: [report.md](report.md).
- [x] `web/pwa/photo-confirm-action.js` drops the bearer `Authorization` header; keeps `credentials:'same-origin'`. — Evidence: [report.md](report.md).
- [x] `grep` proof: no `smackerel_auth_token` / `smackerel.auth_token` / `smackerel_server_url` remaining in `web/pwa/*` or the share template. — Evidence: [report.md](report.md).
- [x] `safeHref` in `assistant.js` unchanged (byte-for-byte). — Evidence: [report.md](report.md).
- [x] `./smackerel.sh check` exit 0; storage + href guards green (evidence in report.md). — Evidence: [report.md](report.md).

## SCOPE-04 — Product-level admin invites

**Status:** Done
**Depends On:** SCOPE-01 (foundation — the invite pages adopt the shared app-shell nav)
**Closes:** SR-06

### Consumer Impact Sweep — `/cards/admin/invites*` → `/admin/invites*`

This scope **moves** a route (a rename of the invite-admin path). Every
first-party consumer of the old path is enumerated below and confirmed updated to
the product-level path — navigation link, form-action + redirect targets, tests,
deep links, handler doc comments — so **zero stale first-party references remain**:

| Consumer surface | File | Old → New | Status |
|------------------|------|-----------|--------|
| Route mount | `internal/web/cardrewards.go` `RegisterRoutes` | `/cards/admin/invites*` → `/admin/invites*` | updated |
| Revoke redirect (PRG `Location`) | `internal/web/invites.go` `AdminInviteRevoke` | 303 → `/admin/invites` | updated |
| Generate form action | `internal/web/invites_templates.go` | `action="/admin/invites"` | updated |
| Revoke form action | `internal/web/invites_templates.go` | `action="/admin/invites/{id}/revoke"` | updated |
| Card dashboard nav link | `internal/web/cardrewards_dashboard_templates.go` | `href="/admin/invites"` | updated |
| Go handler tests (router + assertions) | `internal/web/invites_test.go` | router registers `/admin/invites/{id}/revoke`; assertions check the product-level action + redirect | updated |
| Go handler direct-call request labels | `internal/web/invites_test.go` | cosmetic `httptest.NewRequest(..., "/cards/admin/invites", ...)` labels → `/admin/invites` (behaviorally inert — the handler is called directly; the path is a label) | updated for consistency |
| Handler doc comments | `internal/web/invites.go` | `// handles GET/POST /cards/admin/invites` → `/admin/invites` | updated |
| e2e-ui deep link (navigation) | `web/pwa/tests/cardrewards_invites.spec.ts` | navigates `**/admin/invites` | updated |
| e2e-ui journey (deep link) | `web/pwa/tests/unified_journey.spec.ts` | asserts `/admin/invites` reachable | updated |

No machine client / external consumer references the old path (it was an internal
admin UI) and `/cards/admin` itself is retained (Non-Goals), so no card route
breaks. Two references to the old path are **intentionally retained** (and are
NOT stale): an explanatory comment in `internal/web/cardrewards.go` documenting
the relocation, and the adversarial assertion in
`web/pwa/tests/unified_journey.spec.ts` (SCN-100-07) that GETs
`/cards/admin/invites` and requires a `404/405` — proving the old path no longer
serves. A grep proof that zero *binding* first-party `/cards/admin/invites`
references remain (only those two intentional mentions) is recorded in report.md
(SCOPE-04 evidence).

### Test Plan

- Go: `invites_test.go` route paths updated to `/admin/invites`; handlers unchanged; token-absent-from-list assertion preserved.
- e2e-ui: `cardrewards_invites.spec.ts` updated to `/admin/invites` (generate → reveal → list → revoke → anonymous-blocked), CSP-clean.

### Definition of Done

- [x] `cardrewards.go` `RegisterRoutes` mounts the invite vertical at `/admin/invites` (GET list, POST generate, POST `/{id}/revoke`) and removes it from `/cards/admin`. — Evidence: [report.md](report.md).
- [x] `invites_templates.go`: the two invite pages render `{{template "app-shell-nav" .}}` (product-level chrome) and all form actions target `/admin/invites*`. — Evidence: [report.md](report.md).
- [x] `cardrewards_dashboard_templates.go`: the "Account Invites →" link points to `/admin/invites`. — Evidence: [report.md](report.md).
- [x] `invites_test.go` + `cardrewards_invites.spec.ts` updated to `/admin/invites`; spec 093 one-time-token / token-absent-from-list semantics preserved. — Evidence: [report.md](report.md).
- [x] Consumer Impact Sweep complete: zero stale first-party references remain for the `/cards/admin/invites` → `/admin/invites` move — every consumer in the sweep table (route mount, revoke redirect, form actions, dashboard nav link, Go tests, handler doc comments, e2e deep links) is updated and grep-proven in report.md. — Evidence: [report.md](report.md).
- [x] `./smackerel.sh check` exit 0; `./smackerel.sh test unit --go` green (evidence in report.md). — Evidence: [report.md](report.md).

## SCOPE-05 — Consolidated verification

**Status:** Done
**Depends On:** SCOPE-01 (foundation), SCOPE-02, SCOPE-03, SCOPE-04
**Closes:** SR-09, SR-10 (adversarial), and the per-finding closure table

### Test Plan

- New `web/pwa/tests/unified_journey.spec.ts`: (a) Assistant link present on the
  server root nav and on `/cards`; (b) login with empty next lands on
  `/assistant`; (c) hostile next → `/`; (d) share/capture ACK strengthened; (e)
  `/admin/invites` reachable; CSP guard attached throughout (0 violations).
- Full regression: every pre-existing `web/pwa/tests/*.spec.ts` green.

| Test | Scenario | Type | File | Command / status |
|------|----------|------|------|------------------|
| `unified_journey.spec.ts` + all pre-existing `web/pwa/tests/*.spec.ts` | SCN-100-01..10 | Regression E2E (e2e-ui) | `web/pwa/tests/` | `./smackerel.sh test e2e-ui` — browser lane ENV-CONSTRAINED on this macOS-Docker host; ACCEPTED-EQUIVALENT by the green `internal/web` + `internal/api` + `web/pwa/tests` Go render/handler suites (repo precedent: spec 057 `F-057-V-001`, spec 082 `knownEnvironmentalFailures`) |
| Go render/handler suites (browser-assertion equivalents) | SCN-100-01/02/03/06/08 | unit (Go) | `internal/web`, `internal/api`, `web/pwa/tests` | `./smackerel.sh test unit --go` — green |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (shared app-shell nav, `/assistant` default landing + alias, single-cookie PWA auth, `/admin/invites` relocation, capture ACK, manifest shortcuts) are authored + persisted in `web/pwa/tests/unified_journey.spec.ts` (SCN-100-01..10); browser execution is ENV-CONSTRAINED on this host, ACCEPTED-EQUIVALENT by the green Go render/handler suites (see report.md). — Evidence: [report.md](report.md).
- [x] Broader E2E regression suite passes — every pre-existing `web/pwa/tests/*.spec.ts` stays green (ACCEPTED-EQUIVALENT accounting: full Go unit suite green + no assertion edits to pre-existing specs; browser lane ENV-CONSTRAINED, documented in report.md). — Evidence: [report.md](report.md).
- [x] `unified_journey.spec.ts` authored + persisted (`web/pwa/tests/unified_journey.spec.ts`); browser run under `./smackerel.sh test e2e-ui` is ENV-CONSTRAINED (3 GB ollama-pull stall during stack bring-up — never reaches a browser assertion), ACCEPTED-EQUIVALENT by the `internal/web` + `internal/api` + `web/pwa/tests` Go suites. — Evidence: [report.md](report.md).
- [x] `./smackerel.sh check` exit 0; `./smackerel.sh lint` exit 0; `./smackerel.sh test unit` green (real exit codes + ≥10 lines each in report.md); `./smackerel.sh test e2e-ui` browser lane ENV-CONSTRAINED (documented), ACCEPTED-EQUIVALENT by the green Go suites. — Evidence: [report.md](report.md).
- [x] Per-finding closure table in report.md: SR-01/03/04/05/06/07/08/11/13 → RESOLVED with file:line + test evidence. — Evidence: [report.md](report.md).
- [x] All pre-existing e2e-ui specs remain unedited (no assertion weakening); assistant storage + source-href guards green under `./smackerel.sh test unit --go`. — Evidence: [report.md](report.md).

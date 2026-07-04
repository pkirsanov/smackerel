# Design — Spec 100 (Unified Journey UI Transformation)

**Spec:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md) · **Evidence:** [report.md](report.md) · **User acceptance:** [uservalidation.md](uservalidation.md)
**Workflow mode:** full-delivery · **Status ceiling:** done · **Release train:** `mvp`

---

## Current Truth (objective research pass — verified against the code, solution-blind)

Facts gathered before any design decision, each cited to the real tree at HEAD:

- **Server knowledge UI** — `internal/web/handler.go` L79 parses `allTemplates`
  (`internal/web/templates.go`) with a FuncMap (`truncate`/`timeAgo`/`safeURL`).
  The shared `{{define "head"}}` carries a flat text `<nav>` (templates.go ~L74):
  `Search · Digest · Topics · Knowledge · Notifications · Settings · Status` +
  a theme toggle. Notifications, knowledge, settings, status, search all render
  through this head → **one head, one nav, monochrome tokens**. `SearchPage`
  (handler.go L129) renders `search.html` (an HTMX keyword box). Routes:
  `router.go` mounts the web group behind `webAuthMiddleware`; `/` →
  `SearchPage`.
- **Card-rewards UI** — `internal/web/cardrewards.go` L141 parses
  `cardRewardsTemplates` + `cardRewardsInsightsTemplates` +
  `cardRewardsInviteTemplates` into a *separate* template set with its OWN
  `{{define "head"}}` (design tokens, glass `.cr-nav`) that deliberately does
  **not** load htmx (CSP-clean, cardrewards.go L79-85). `cardrewards-nav`
  (`cardrewards_templates.go` ~L169) is card-scoped only. `RegisterRoutes`
  (cardrewards.go L150) registers `/cards*` incl. `/cards/admin/invites*`
  (L200-210).
- **Assistant** — static PWA page `web/pwa/assistant.html` + ES module
  `web/pwa/assistant.js`. Auth carrier = **same-origin HttpOnly `auth_token`
  cookie**; the module MUST NOT touch localStorage (guarded by
  `web/pwa/tests/assistant_storage_guard_test.go`). `safeHref` (assistant.js
  L70-96) is the source-attribution XSS defense, locked by
  `tests/assistant_source_href_security_guard_test.go`. Linked from **no** nav.
- **PWA home + capture** — `web/pwa/index.html` has a "Server URL" + "Auth
  Token" settings card; `web/pwa/app.js` L70-135 saves/reads
  `smackerel_server_url` + `smackerel_auth_token` in `localStorage` and answers
  the SW `getConfig` message with them. `web/pwa/sw.js` L80-135 flushes the
  offline queue by POSTing `serverUrl + '/api/capture'` with `Authorization:
  Bearer <localStorage token>`. `internal/api/pwa.go` `sharePageTemplate` posts
  `/api/capture` with `Authorization: <localStorage token>` and ACKs "✅ Saved!"
  (thin). `web/pwa/photo-confirm-action.js` L17-24 adds `Authorization: Bearer
  <localStorage "smackerel.auth_token">` **and** already sends
  `credentials:"same-origin"`. → **split-brain**: assistant on cookie, the rest
  on a pasted token.
- **Auth carriers** — `webAuthMiddleware` (router.go L762) and
  `bearerAuthMiddleware` both accept the `auth_token` cookie as a fallback for
  the `Authorization` header. The cookie is set at `Path=/`, `HttpOnly`,
  `SameSite=Lax` by `POST /v1/web/login` (`web_login.go`). So **any same-origin
  PWA fetch with `credentials:'include'` already authenticates via the cookie**
  — the pasted token is redundant.
- **Landing** — `sanitizeNext` (`sanitize_next.go`) default is `/`
  (`sanitizeNextDefault`, L17). `HandleLoginPage` (`web_login_page.go` L48)
  computes `next := sanitizeNext(?next)`; `HandleWebRegister` 303-redirects to
  `/login?registered=1`. The e2e-ui `auth_login.spec.ts` TP-077-03-02 asserts
  **hostile** next values sanitise to exactly `"/"` — so `sanitizeNextDefault`
  MUST stay `/`; the assistant-default belongs in the *page* default, not the
  sanitiser reject-path.
- **Capture response** — `internal/api/capture.go` `CaptureResponse` returns
  `ArtifactID`, `Title`, `ArtifactType`, `Summary`, `Connections`, `Topics`,
  `ProcessingMs`. `409` → `{error:{code:"DUPLICATE_DETECTED", ...}}`. So the ACK
  can name the saved item from the response body.
- **e2e-ui harness** — spec 077; `./smackerel.sh test e2e-ui` runs Playwright
  specs in `web/pwa/tests/*.spec.ts` against the disposable
  `smackerel-test-e2e-ui` Compose project, dev-token mode
  (`AuthConfig.Enabled=false`, cookie value = shared `SMACKEREL_AUTH_TOKEN`).
  `_support/csp.ts` fails any spec on a CSP-shaped console error;
  `_support/cardrewards.ts` has a `login(page, next)` helper.
- **PWA embedding** — `web/pwa/embed.go` `//go:embed *.html *.css *.js *.json
  *.svg lib generated`. `pwa.go` rewrites `sw.js` `CACHE_NAME` to a content
  hash. `sw.js` `STATIC_ASSETS` caches the shell.

**Implication (solution-blind):** the cheapest true convergence is a
**single-source nav partial** parsed into both Go template sets + one
same-origin PWA nav script, plus a small number of front-door changes (assistant
route + landing default + intent-first root + one PWA auth model + ACK + invites
relocation). No page-body rewrites, no new template engine, no new auth.

---

## Architecture Decision

**Converge at the navigation / app-shell / IA layer.** One IA, rendered three
ways (because there are three template systems), from a **single source** where
the runtime allows it:

```
                         ┌──────────────────────────────────────────┐
                         │  Canonical cross-surface IA (single list) │
                         │  Assistant · Search · Knowledge · Cards · │
                         │  Notifications · Settings                 │
                         └──────────────────────────────────────────┘
                              │                │                │
           parse appShellNav  │   parse        │   inject via   │
           into allTemplates  │   appShellNav  │   one script   │
                              ▼   into cards ▼                  ▼
                  ┌────────────────┐ ┌────────────────┐ ┌────────────────┐
                  │ Server KB head │ │ Card-rewards   │ │ PWA pages       │
                  │ templates.go   │ │ head           │ │ appnav.js       │
                  │ (+notifications│ │ cardrewards_*  │ │ (index,         │
                  │  inherit it)   │ │ (+invites)     │ │  assistant, …)  │
                  └────────────────┘ └────────────────┘ └────────────────┘
```

- **Single-source Go partial.** New `internal/web/appshell.go` defines
  `const appShellNav` = `{{define "app-shell-nav"}}…{{end}}` (+ active-state via
  `.Title`). It is parsed into **both** template sets: `handler.go`
  (`allTemplates`) and `cardrewards.go` (`cardRewardsTemplates`). The server KB
  head replaces its inline `<nav>` with `{{template "app-shell-nav" .}}`; the
  card head gains `{{template "app-shell-nav" .}}` above its `.cr-nav`. This is
  genuine single-source across the two server surfaces (SR-03, SR-07 — because
  notifications render through the KB head).
- **Single-source PWA nav.** New `web/pwa/lib/appnav.js` (same-origin,
  `script-src 'self'`-clean, no inline handlers) builds the same IA into a
  `<nav id="app-shell-nav">` placeholder on each PWA page. Included by
  `index.html`, `assistant.html`, and the key feature pages. Added to
  `sw.js` `STATIC_ASSETS`. (SR-13)
- **Assistant front door.** New authenticated route `GET /assistant` → 302
  `/pwa/assistant.html` (a memorable, cookie-gated alias). `HandleLoginPage` +
  `HandleRegisterPage` default the empty-`next` destination to `/assistant`
  (the sanitiser default stays `/`, so the hostile-value e2e matrix is
  unchanged). (SR-05)
- **Intent-first root.** `search.html` gains an intent-first hero linking to the
  assistant above the (retained) search box. (SR-11)
- **One PWA auth model.** Retire the "Server URL"/"Auth Token" landing; every
  PWA fetch goes same-origin with `credentials:'include'` (cookie). `sw.js`
  flushes to same-origin `/api/capture`; `app.js` drops the token/serverUrl +
  `getConfig`; the share page drops the localStorage token; `photo-confirm-
  action.js` drops the bearer header (keeps `credentials:'same-origin'`).
  Security-positive: no bearer in JS-visible storage. (SR-04)
- **Strengthened ACK.** The share page ACK names the saved title, states
  "saved and searchable", and links to the assistant / knowledge. (SR-08)
- **Product-level admin.** The invite routes move from `/cards/admin/invites*`
  to `/admin/invites*` (still inside `webAuthMiddleware` via the same
  `RegisterRoutes` mount at root), the invite templates swap `cardrewards-nav`
  for the shared `app-shell-nav`, and the card admin page links to
  `/admin/invites`. (SR-06)

### Why not a redirect for `/` or a server-rendered assistant shell?

- `auth_login.spec.ts` TP-077-03-03 / TP-077-03-07 assert `/` **serves a 200
  page**. A redirect would break them and remove the search affordance. The
  intent-first hero keeps `/` a real page while demoting the keyword box.
- A brand-new server-rendered assistant shell would duplicate the assistant DOM
  (drift risk vs `assistant.html`, and the storage guard). A cookie-gated 302 to
  the proven PWA page is the low-risk convergence the audit asked for.

---

## Capability Foundation

**Capability established:** a single-source, cross-surface **application-shell
information architecture** — one canonical wayfinding list (Assistant · Search ·
Knowledge · Cards · Notifications · Settings) authored ONCE and rendered
identically on every surface Smackerel exposes. Before this spec the three
surfaces each owned a private, drifting nav; the capability this transformation
establishes is *"define the cross-surface IA in exactly one place; every surface
consumes it."*

The foundation is the canonical IA + its contract, expressed as the
`app-shell-nav` partial (`internal/web/appshell.go`, `const appShellNav`) and
mirrored verbatim by the PWA injector (`web/pwa/lib/appnav.js`).

**Invariants (hold for every concrete renderer):**

- **Single source of truth.** The link set + order live in exactly one place per
  runtime (the Go `const appShellNav`; the PWA `appnav.js` mirrors the same IA).
  A surface MUST NOT hand-author its own cross-surface bar.
- **Assistant-first ordering.** Assistant is always the first item (Product
  Principle P2 — the assistant is the intelligent front door).
- **CSP-clean.** Anchors only — no inline `<script>`, no inline event handler
  (`onclick`/`onload`/`onsubmit`), no new external script origin; embedding pages
  keep `script-src 'self'`. Locked by `TestAppShellNav_NoInlineHandlers` + the
  spec-077 e2e-ui CSP guard.
- **Field-free / total.** The partial branches on no view-model field, so it can
  never fail template execution regardless of the heterogeneous `.Title` values
  that reach the shared `head`; each surface keeps its own sub-nav for in-section
  active state.
- **Additive & reversible.** No page body is redesigned; a surface adopts the
  foundation by rendering one partial / including one script.

## Concrete Implementations

Three renderers consume the one canonical IA (there are three because the tree
already has three template systems — this spec does NOT add a fourth):

| Implementation | Surface | How it consumes the canonical IA | Files |
|----------------|---------|----------------------------------|-------|
| KB template renderer | server knowledge UI (`/`, `/knowledge`, `/notifications`, `/settings`, `/search`) | `appShellNav` parsed into the `allTemplates` set; the shared `head` renders `{{template "app-shell-nav" .}}` | `internal/web/appshell.go`, `internal/web/handler.go`, `internal/web/templates.go` |
| Card-rewards template renderer | the `/cards*` + `/admin/invites*` vertical | the SAME `appShellNav` parsed into the `cardRewardsTemplates` set; the card `head` renders `{{template "app-shell-nav" .}}` above `.cr-nav` | `internal/web/appshell.go`, `internal/web/cardrewards.go`, `internal/web/cardrewards_templates.go`, `internal/web/invites_templates.go` |
| PWA nav injector | the static PWA pages (`index.html`, `assistant.html`, feature pages) | `appnav.js` builds the same IA into `<nav id="app-shell-nav">` (same-origin, `script-src 'self'`), cached via `sw.js` `STATIC_ASSETS` | `web/pwa/lib/appnav.js`, `web/pwa/index.html`, `web/pwa/assistant.html`, `web/pwa/sw.js` |

### Variation Axes

The three renderers differ ONLY along these axes; the IA itself (links + order +
assistant-first + CSP posture) is invariant across all three:

| Axis | KB template renderer | Card-rewards template renderer | PWA nav injector |
|------|----------------------|--------------------------------|------------------|
| Rendering runtime | Go `html/template` (`allTemplates`) | Go `html/template` (`cardRewardsTemplates`, htmx-free) | browser DOM via a same-origin ES script |
| Injection mechanism | `{{template "app-shell-nav" .}}` in the shared `head` | `{{template "app-shell-nav" .}}` above `.cr-nav` | `appnav.js` builds `<nav id="app-shell-nav">` at load |
| In-section active state | KB sub-nav extras (Search/Digest/Topics/Status) | card `.cr-nav` pills | per-page PWA sub-nav / entry links |
| Asset delivery | server-embedded template constant | server-embedded template constant | static asset cached in the service worker |

---

## Finding → Mechanism map

| Finding | Mechanism | Files |
|---------|-----------|-------|
| SR-03 | single-source `app-shell-nav` partial parsed into both Go sets | `internal/web/appshell.go` (new), `handler.go`, `cardrewards.go`, `templates.go`, `cardrewards_templates.go` |
| SR-01 | Assistant link in all navs + manifest `shortcuts` | `appshell.go`, `appnav.js` (new), `manifest.json`, `index.html`, `assistant.html` |
| SR-07 | notifications inherit the shared KB head nav | `templates.go` (head only) |
| SR-13 | shared PWA nav wires the islands | `web/pwa/lib/appnav.js` (new) + PWA pages |
| SR-05 | `/assistant` route + login/register default landing | `router.go`, `web_login_page.go`, `web_register_page.go` |
| SR-11 | intent-first hero on `/` | `templates.go` (`search.html`) |
| SR-04 | one cookie auth model | `index.html`, `app.js`, `sw.js`, `pwa.go`, `photo-confirm-action.js` |
| SR-08 | strengthened durable-capture ACK | `pwa.go` (`sharePageTemplate`) |
| SR-06 | invites → `/admin/invites` | `cardrewards.go`, `invites_templates.go`, `cardrewards_dashboard_templates.go` |

## Security review (SR-04 lens)

- **Threat retired:** a long-lived per-user bearer token sitting in
  `localStorage` (readable by any same-origin script; an XSS foothold could
  exfiltrate it). Moving to the HttpOnly cookie removes JS read access entirely.
- **No new attack surface:** the cookie is already set by the audited login flow
  (SameSite=Lax, HttpOnly, Path=/, Secure in prod). Same-origin fetches with
  `credentials:'include'` are the standard pattern; CSRF exposure is unchanged
  because the state-changing endpoints are same-origin JSON POSTs (SameSite=Lax
  blocks cross-site form posts) and the assistant already relies on exactly this.
- **`safeHref` preserved verbatim** — untouched; its guard test still runs.
- **CSP unchanged** — `appnav.js` is `'self'`; the share page keeps its per-
  request nonce; no page gains an inline handler or a new script origin.

## Test Plan (maps to scenarios; real executed evidence, ≥10 lines each)

| Test | Scenario | Type | File | Command |
|------|----------|------|------|---------|
| `TestAppShellNav_Present` | SCN-100-01/02 | unit (Go) | `internal/web/appshell_test.go` | `./smackerel.sh test unit --go` |
| `TestLoginPage_DefaultLandingIsAssistant` | SCN-100-03 | unit (Go) | `internal/api/web_login_page_test.go` | `./smackerel.sh test unit --go` |
| `TestSanitizeNext_HostileStillSlash` (regression) | SCN-100-04 | unit (Go) | `internal/api/sanitize_next_test.go` (existing) | `./smackerel.sh test unit --go` |
| `TestAssistantRoute_RedirectsAuthed` | SCN-100-03 | unit (Go) | `internal/api/router_test.go` (existing pattern) | `./smackerel.sh test unit --go` |
| `unified_journey.spec.ts` | SCN-100-01/02/03/05/06/08/09 | e2e-ui | `web/pwa/tests/unified_journey.spec.ts` (new) | `./smackerel.sh test e2e-ui` |
| existing suites (regression) | SCN-100-10 | e2e-ui | `cardrewards_*`, `auth_login`, `photos_*`, `assistant_*` | `./smackerel.sh test e2e-ui` |
| relocated invites | SCN-100-07 | e2e-ui | `cardrewards_invites.spec.ts` (updated → `/admin/invites`) | `./smackerel.sh test e2e-ui` |
| storage + href guards (regression) | SR-04 lens | unit (Go) | `assistant_storage_guard_test.go`, `assistant_source_href_security_guard_test.go` | `./smackerel.sh test unit --go` |

**Adversarial coverage:** the CSP guard (`_support/csp.ts`) is attached to every
touched page in `unified_journey.spec.ts`; a non-tautological check asserts the
Assistant link resolves AND a hostile `next` still lands on `/`.

# Spec 100 — Unified Journey UI Transformation (One App-Shell Across Every Surface)

**Status:** in_progress
**Workflow mode:** full-delivery · **Status ceiling:** done
**Release train:** `mvp`
**Builds on:** [092-card-rewards-ui-elevation](../092-card-rewards-ui-elevation/spec.md) (the elevated design system that was applied ONLY to `/cards`), [073-web-mobile-assistant-frontend](../073-web-mobile-assistant-frontend/spec.md) (the assistant PWA + cookie session), [070-web-username-password-login](../070-web-username-password-login/spec.md) (the cookie login), [077-pwa-browser-test-harness](../077-pwa-browser-test-harness/spec.md) (the `./smackerel.sh test e2e-ui` Playwright harness)
**Relates to:** [066-legacy-keyword-surface-retirement](../066-legacy-keyword-surface-retirement/spec.md) (the intent/assistant-first mandate), [093-admin-generated-registration-invites](../093-admin-generated-registration-invites/spec.md) (the invites admin surface being relocated), [074-capture-as-fallback-policy](../074-capture-as-fallback-policy/spec.md) (the inviolable capture path this must not break)

> **Operator directive (verbatim):** *"e2e user journeys/sagas UI transformation"*

---

## Problem

A readiness audit surfaced that Smackerel today is **three disjoint UI
surfaces** that do not feel like one product:

1. **Static PWA** — `web/pwa/*` served at `/pwa/*`. Its own dark theme
   (`web/pwa/style.css`, `--primary:#e94560`), its own home
   (`web/pwa/index.html`), and its own auth model.
2. **Server-rendered HTMX knowledge UI** — `internal/web/templates.go` served
   at `/`. A monochrome design system with a flat text `<nav>` (templates.go
   ~L74).
3. **Card-rewards vertical** — `internal/web/cardrewards_templates.go` served at
   `/cards/*`. The *only* surface that received the spec-092 journey-centric
   design elevation (glass nav, design tokens, stat cards); spec 092 Non-Goals
   explicitly excluded every other surface.

The result is three design systems, three non-cross-linking navs, and **two
auth models inside the same PWA**. Nine findings (below) were raised. They are
not nine unrelated bugs — they are the symptom set of one missing thing: a
**single app-shell / information architecture that cross-links every journey**,
with the assistant as the coherent front door (Product Principle P2 — the
assistant is the intelligent front door; P9 — design for restart).

This spec closes that gap. It converges the product at the **navigation /
app-shell / IA layer** — one shared nav cross-linking every surface, the
assistant made discoverable and the default landing, one PWA auth model, and a
handful of high-signal front-door fixes. It deliberately does **not** attempt a
risky pixel-level redesign of every page; it makes the surfaces feel like one
product by making them *navigable* as one product.

### The nine findings (each MUST be closed and cited in report.md)

| ID | Sev | Finding | Cited code (verified) |
|----|-----|---------|------------------------|
| SR-01 | high | The assistant surface (`web/pwa/assistant.html` / `assistant.js`) is linked from **no** nav, and `web/pwa/manifest.json` has no `shortcuts`. | knowledge nav [`templates.go`](../../internal/web/templates.go) ~L74; cardrewards-nav [`cardrewards_templates.go`](../../internal/web/cardrewards_templates.go) ~L169; PWA home [`index.html`](../../web/pwa/index.html); [`manifest.json`](../../web/pwa/manifest.json) |
| SR-03 | high | No unified app-shell/nav; the journey transformation was applied only to `/cards`. | spec 092 Non-Goals |
| SR-04 | high | Split-brain PWA auth: the assistant uses the HttpOnly cookie, while `index.html`/`app.js`/`photo-confirm-action.js`/the share page use a pasted bearer token in `localStorage`. | [`app.js`](../../web/pwa/app.js) L70-135; [`sw.js`](../../web/pwa/sw.js) L80-135; [`photo-confirm-action.js`](../../web/pwa/photo-confirm-action.js) L17-24; [`pwa.go`](../../internal/api/pwa.go) share template |
| SR-05 | med | Post-login/register lands on the empty keyword `SearchPage` (`/`), not a guided/assistant front door. | [`sanitize_next.go`](../../internal/api/sanitize_next.go) L17 default `/`; [`web_login_page.go`](../../internal/api/web_login_page.go) L48 |
| SR-06 | med | The global registration-invite admin UI is nested under the card-rewards vertical at `/cards/admin/invites`. | [`cardrewards.go`](../../internal/web/cardrewards.go) L200-210; [`cardrewards_dashboard_templates.go`](../../internal/web/cardrewards_dashboard_templates.go) ~L246 |
| SR-07 | med | Notifications render in the monochrome `templates.go` design system (`/notifications/*`), disjoint from the polished `/cards`. | [`templates.go`](../../internal/web/templates.go) notification templates |
| SR-08 | med | The in-UI durable-capture confirmation is thin ("✅ Saved!"), under-communicating permanence + searchability (P8). | [`pwa.go`](../../internal/api/pwa.go) `sharePageTemplate`; [`sw.js`](../../web/pwa/sw.js) offline queue |
| SR-11 | low | Spec 066 retired keyword surfaces, but web root `/` remains a keyword `SearchPage`. | [`handler.go`](../../internal/web/handler.go) L129 `SearchPage`; [`templates.go`](../../internal/web/templates.go) `search.html` |
| SR-13 | low | The PWA home links to no PWA feature; feature pages (assistant, connectors, photos, drives, models) are islands. | [`index.html`](../../web/pwa/index.html); PWA feature pages |

---

## Goals

1. **One shared app-shell navigation.** A single, single-source cross-surface
   nav — Assistant · Search · Knowledge · Cards · Notifications · Settings —
   reused across the server-rendered knowledge UI, the card-rewards vertical,
   and the PWA, so every journey is reachable from every surface. (SR-03, SR-01,
   SR-07, SR-13)
2. **Assistant as the coherent front door.** The assistant is discoverable from
   every nav, is the **default post-login/registration landing**, has PWA
   manifest `shortcuts`, and the web root `/` leads with an intent-first
   assistant entry. (SR-01, SR-05, SR-11)
3. **One PWA auth model.** The whole PWA (home, share/capture, photo actions,
   offline sync) uses the **same-origin HttpOnly `auth_token` cookie** the
   assistant already uses; the "paste Auth Token" landing is retired. This is a
   security-positive change (no bearer token in JS-visible `localStorage`). (SR-04)
4. **Product-level admin.** The registration-invite admin UI moves from
   `/cards/admin/invites` to a product-level `/admin/invites`. (SR-06)
5. **Stronger durable-capture acknowledgement.** The capture ACK tells the user
   *what* was saved and that it is durable and searchable, with a next action
   (Product Principle P8 — trust through transparency). (SR-08)

---

## Non-Goals

- **No pixel-level redesign** of the knowledge or notification page *bodies*.
  Convergence is at the nav / app-shell / IA layer. Page bodies keep their
  current markup, restyled only insofar as they inherit the shared shell.
- **No change to the assistant turn protocol** (`/api/assistant/turn`), the
  `safeHref` source-attribution XSS defense in `assistant.js`, or the
  capture-as-fallback inviolable path (spec 074).
- **No change to the auth trust model** (specs 044/060/070/091/093) — this reuses
  the existing cookie the login flow already sets; it does not mint new sessions
  or widen any scope.
- **No new SST runtime config.** Every path used here is already served; no new
  `config/smackerel.yaml` value is introduced (NO-DEFAULTS / G028 preserved).
- **No route removals that break machine clients.** `/cards/admin` remains; only
  the invite sub-surface is relocated (with the card admin page linking to it).

---

## Product Principle Alignment

| Principle | How this feature implements/extends it |
|-----------|----------------------------------------|
| **P2 — Vague In, Precise Out (assistant is the intelligent front door)** | The assistant becomes the default landing and the first nav item on every surface; the keyword web-root is demoted below an intent-first assistant entry. |
| **P6 — Invisible by Default** | Adds *navigation*, not notifications or prompts. Zero new system-initiated messages; the nav is felt, not heard. |
| **P8 — Trust Through Transparency** | The strengthened capture ACK names the saved artifact and states it is durable + searchable; the `safeHref` attribution defense is preserved verbatim. |
| **P9 — Design for Restart** | A returning user lands on the assistant ("ask what mattered"), not a blank keyword box; every surface is one hop from every other. |

Evidence: [`docs/Product-Principles.md`](../../docs/Product-Principles.md),
[`docs/smackerel.md`](../../docs/smackerel.md).

---

## Domain Capability Model

**Capability:** *Unified cross-surface application-shell navigation* — the
product-level ability to define the wayfinding information architecture (which
journeys exist and how they cross-link) in ONE place and have every surface
render it identically.

**Primitive:** the **app-shell IA** — a single canonical, ordered link set
(Assistant · Search · Knowledge · Cards · Notifications · Settings) with an
assistant-first, CSP-clean, field-free contract (see
[design.md → Capability Foundation](design.md) for the invariants).

**Why a capability, not three navs:** the nine findings (SR-01/03/07/13 in
particular) are the symptom set of ONE missing capability — a shared IA. Adding a
link to a fourth journey, or reordering the bar, then becomes a one-place change
that propagates to all three surfaces, instead of three hand-edits that drift.
The concrete renderers (server KB templates, card-rewards templates, PWA
injector) are enumerated in design.md → Concrete Implementations; they exist
because the tree already has three template systems, not because the IA is
duplicated.

**Boundaries:** the capability is the *navigation / app-shell / IA layer only*.
It deliberately does not own page bodies, the assistant turn protocol, the auth
trust model, or any new SST runtime config (see Non-Goals).

---

## Scenarios (BDD)

### SCN-100-01 — Assistant is discoverable from the knowledge surface

```gherkin
Given an authenticated user on the knowledge web UI at "/"
When the shared app-shell nav renders
Then it contains a link with text "Assistant" whose href is "/assistant"
And that link is reachable from every server-rendered page that uses the shared head
```

### SCN-100-02 — Assistant is discoverable from the card-rewards surface

```gherkin
Given an authenticated user on "/cards"
When the page renders
Then the shared app-shell nav is present above the card-specific nav
And it contains an "Assistant" link to "/assistant"
```

### SCN-100-03 — Assistant is the default post-login landing

```gherkin
Given an unauthenticated user who navigates directly to "/login"
When they submit valid credentials with no explicit "next"
Then the login page's default destination is the assistant front door "/assistant"
And "/assistant" resolves (302) to the assistant PWA page
```

### SCN-100-04 — Explicit next is still honoured (no regression)

```gherkin
Given an unauthenticated user redirected to "/login?next=/cards" by the auth middleware
When they submit valid credentials
Then they land on "/cards"
And a hostile next value still sanitises to "/" (spec 057 matrix unchanged)
```

### SCN-100-05 — The whole PWA uses the cookie, not a pasted token

```gherkin
Given a user who has signed in (auth_token HttpOnly cookie set)
When the PWA home, the share/capture page, and the photo-action page issue same-origin fetches
Then each fetch relies on the cookie (credentials: same-origin/include)
And no PWA script reads or writes a bearer token in localStorage
And the "paste Auth Token" settings landing is gone
```

### SCN-100-06 — Strengthened durable-capture acknowledgement

```gherkin
Given a signed-in user who shares a URL to the PWA (POST /pwa/share)
When the capture succeeds
Then the ACK names the captured item and states it is saved and searchable
And offers a next action (open the assistant / view knowledge)
And the capture-as-fallback inviolable path (spec 074) is unchanged
```

### SCN-100-07 — Registration-invite admin is product-level

```gherkin
Given a signed-in operator
When they open the invites admin UI
Then it is served at the product-level path "/admin/invites"
And "/admin/invites" generate/list/revoke work end-to-end
And the one-time plaintext token still appears only in the reveal (spec 093 AC-11 preserved)
```

### SCN-100-08 — PWA feature pages are no longer islands

```gherkin
Given the PWA home and the assistant page
When they render
Then the shared PWA nav cross-links Assistant, Capture, Search, Cards, Connectors, Photos, Settings
And the manifest exposes shortcuts for Assistant, Capture, and Search
```

### SCN-100-09 — Adversarial: no CSP regression, no inline handlers

```gherkin
Given the spec-077 CSP guard attached to every touched page
When the app-shell nav renders on the server surfaces and the PWA surfaces
Then zero CSP violations are recorded
And the shared nav introduces no inline event handlers and no new external script origin
```

### SCN-100-10 — Adversarial: existing suites stay green

```gherkin
Given the existing e2e-ui suite (cardrewards_*, auth_login, photos_*, assistant_*)
When the shared shell + relocated invites land
Then every existing spec still passes
And the assistant storage guard and source-href security guard remain green
```

---

## Acceptance Criteria

- **AC-1** A single-source `app-shell-nav` partial exists and is rendered by the
  server knowledge UI head, the card-rewards head, and (via one same-origin
  script) the PWA pages. (SR-03)
- **AC-2** "Assistant" links to `/assistant` appear in all three navs; the PWA
  manifest declares `shortcuts` for Assistant, Capture, Search. (SR-01)
- **AC-3** `/assistant` is an authenticated route resolving to the assistant PWA
  page; the login/register default landing is `/assistant`. (SR-05)
- **AC-4** The web root `/` leads with an intent-first assistant entry above the
  search box. (SR-11)
- **AC-5** No PWA script reads/writes a bearer token in `localStorage`; the share
  page, offline sync, and photo-action page use the cookie; the "paste Auth
  Token" landing is removed. (SR-04)
- **AC-6** The capture ACK names the saved item, states it is durable +
  searchable, and offers a next action; capture-as-fallback unchanged. (SR-08)
- **AC-7** Invites admin is reachable at `/admin/invites` (generate/list/revoke);
  spec 093 one-time-token semantics preserved. (SR-06)
- **AC-8** Notifications inherit the shared shell nav (SR-07); PWA feature pages
  carry the shared PWA nav (SR-13).
- **AC-9** `./smackerel.sh check`, `lint`, `test unit`, and `test e2e-ui` are
  green; every pre-existing e2e-ui spec still passes; the assistant storage +
  source-href security guards remain green. (SR-09/SR-10 adversarial)

---

## Hard Constraints

1. **Capture-as-fallback is inviolable** (spec 074 / policySnapshot). The share
   and offline paths must still durably capture; the ACK change is additive.
2. **`safeHref` XSS defense is preserved verbatim** in `assistant.js`.
3. **NO-DEFAULTS / G028**: no new runtime config value; no `${VAR:-default}`.
4. **CSP discipline**: no new inline event handlers; no new external script
   origins. PWA scripts stay `script-src 'self'`; the share page keeps its nonce.
5. **Terminal discipline**: all build/test via `./smackerel.sh`; file writes via
   IDE tools only.

---

## Requirement-Mechanism Justifications

Per the Gate G097 honest-disclosure contract, this section discloses every
security/protocol mechanism NAMED in the requirements whose implementation is
intentionally a differently-named or pre-existing mechanism rather than new code
in this spec's scope.

**Mechanism-Justification: CSRF — cross-site request-forgery resistance is
provided by the pre-existing `SameSite=Lax`, `HttpOnly` session cookie
(`auth_token`, set by `POST /v1/web/login` in `web_login.go`) combined with
same-origin JSON/form `POST` endpoints — NOT by a new anti-CSRF token.** This
spec introduces no dedicated CSRF-token because it makes NO auth trust-model
change (see Non-Goals) and mints no new session: it reuses the exact cookie the
audited login flow already sets. `SameSite=Lax` IS the implemented mechanism (it
blocks cross-site form POSTs), and the state-changing endpoints touched here
(`/api/capture`, `/admin/invites*`) are same-origin POSTs already gated by
`webAuthMiddleware`. The SR-04 change is strictly CSRF-neutral-to-positive: it
REMOVES a JS-readable bearer token from `localStorage` (an XSS-exfiltration
foothold) and moves to the HttpOnly cookie, without widening any cross-site
surface. Negative/rejection coverage for the cookie/auth mechanism itself lives
in the auth specs that own it (044/060/070/091); this spec adds the hostile-`next`
rejection (`sanitize_next_test.go`) and the anonymous `/admin/invites` block
(`TestAdminInvites_AnonymousBlocked`).

**Content-Security-Policy** is named AND implemented in-scope (anchors-only
`appShellNav`, `script-src 'self'` PWA injector, per-request nonce on the share
page); G097 already confirms matching code evidence for it — no justification
needed.

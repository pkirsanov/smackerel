package web

// Spec 100 SCOPE-01 — the single-source cross-surface app-shell navigation.
//
// `appShellNav` is parsed into BOTH server-rendered template sets — the
// knowledge-base set (internal/web/handler.go, `allTemplates`) and the
// card-rewards set (internal/web/cardrewards.go, `cardRewardsTemplates`) — and
// is mirrored verbatim by web/pwa/lib/appnav.js on the PWA pages. One IA,
// rendered by the three template systems, so every journey (assistant,
// knowledge/search, cards, notifications, settings) is reachable from every
// surface. This closes the "three disjoint navs" gap (SR-03) without a
// pixel-level redesign of any page body.
//
// The assistant is deliberately the FIRST item: Product Principle P2 — the
// assistant is the intelligent front door.
//
// CSP discipline (locked by TestAppShellNav_NoInlineHandlers and the spec-077
// e2e-ui CSP guard): anchors only. NO inline <script>, NO inline event handlers
// (onclick/onload/onsubmit/onerror), NO new external script origin. The pages
// that embed it keep `script-src 'self'` (+ the pinned htmx / hashed theme
// script that already exist).
//
// The links intentionally carry NO active-state branching on the page's
// heterogeneous `.Title` values (they range across "Smackerel", "My Cards",
// "Notifications", "Admin", …): the cross-surface bar is a stable, always-the-
// same wayfinding row, and each surface keeps its own sub-nav (the KB extras,
// the card `.cr-nav`) for in-section active state. Keeping it field-free also
// means the partial can never fail template execution regardless of the view
// model that reaches the shared `head`.
const appShellNav = `
{{define "app-shell-nav"}}<a class="app-shell-link" href="/assistant" data-nav="assistant">Assistant</a><a class="app-shell-link" href="/" data-nav="search">Search</a><a class="app-shell-link" href="/knowledge" data-nav="knowledge">Knowledge</a><a class="app-shell-link" href="/cards" data-nav="cards">Cards</a><a class="app-shell-link" href="/notifications" data-nav="notifications">Notifications</a><a class="app-shell-link" href="/settings" data-nav="settings">Settings</a>{{end}}
`

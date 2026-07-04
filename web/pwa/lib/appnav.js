// Spec 100 SCOPE-01 — the single-source cross-surface app-shell navigation for
// the PWA surfaces. It mirrors the server-side `{{define "app-shell-nav"}}`
// partial (internal/web/appshell.go) so the static PWA pages (home, assistant,
// connectors, photos, drives, models) are no longer islands (SR-13) and the
// assistant is reachable from every PWA surface (SR-01).
//
// CSP discipline: this file is served same-origin from /pwa/lib/appnav.js, so
// it satisfies `script-src 'self'` on every PWA page (index.html and
// assistant.html both ship `script-src 'self'`). It builds the nav via the DOM
// API only — NO innerHTML with interpolation, NO inline event handlers, NO
// external origin. It is defensive: it never throws and only injects once.
//
// The nav cross-links BOTH the PWA feature pages AND the server-rendered
// surfaces (Search "/", Cards "/cards", Notifications "/notifications",
// Settings "/settings"), so a user can move between the PWA and the server UI
// as one product. Auth is the same-origin HttpOnly cookie (spec 100 SCOPE-03),
// so these are plain same-origin links.
(function () {
  "use strict";

  // Canonical cross-surface IA. `pwa` marks links that live under /pwa/*.
  var ITEMS = [
    { href: "/assistant", label: "Assistant", key: "assistant" },
    { href: "/pwa/", label: "Capture", key: "capture", pwa: true },
    { href: "/", label: "Search", key: "search" },
    { href: "/cards", label: "Cards", key: "cards" },
    { href: "/pwa/connectors.html", label: "Connectors", key: "connectors", pwa: true },
    { href: "/pwa/photo-health.html", label: "Photos", key: "photos", pwa: true },
    { href: "/notifications", label: "Notifications", key: "notifications" },
    { href: "/settings", label: "Settings", key: "settings" }
  ];

  function build() {
    try {
      // Idempotent: never inject twice.
      if (document.getElementById("app-shell-nav")) {
        return;
      }
      var nav = document.createElement("nav");
      nav.id = "app-shell-nav";
      nav.className = "app-shell-nav";
      nav.setAttribute("aria-label", "Primary");

      // Mark the current surface active by pathname (best-effort, no branch on
      // server state). "/pwa/" and "/pwa/index.html" both map to Capture.
      var path = window.location.pathname;
      for (var i = 0; i < ITEMS.length; i++) {
        var item = ITEMS[i];
        var a = document.createElement("a");
        a.className = "app-shell-link";
        a.href = item.href;
        a.setAttribute("data-nav", item.key);
        a.textContent = item.label;
        if (isActive(item, path)) {
          a.classList.add("active");
          a.setAttribute("aria-current", "page");
        }
        nav.appendChild(a);
      }

      // Mount into an explicit placeholder if the page provides one, else
      // prepend to <body> so the nav is the first landmark.
      var mount = document.getElementById("app-shell-nav-mount");
      if (mount && mount.parentNode) {
        mount.parentNode.replaceChild(nav, mount);
      } else if (document.body) {
        document.body.insertBefore(nav, document.body.firstChild);
      }
    } catch (_e) {
      // Navigation is progressive enhancement; never break the page.
    }
  }

  function isActive(item, path) {
    if (item.key === "capture") {
      return path === "/pwa/" || path === "/pwa/index.html";
    }
    if (item.key === "assistant") {
      return path === "/assistant" || path === "/pwa/assistant.html";
    }
    if (item.pwa) {
      return path === item.href;
    }
    return path === item.href;
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", build);
  } else {
    build();
  }
})();

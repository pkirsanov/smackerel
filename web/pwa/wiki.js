// Spec 073 SCOPE-073-05 — wiki landing. No network calls; the page is
// pure navigation. Kept as a module so the storage guard scans it.
export const WIKI_SECTIONS = ["topics", "people", "places", "time"];
// No-op init: confirms the page loaded without storage side effects.
document.documentElement.setAttribute("data-wiki-ready", "true");

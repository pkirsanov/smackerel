// Spec 073 SCOPE-073-05 SCN-073-B04 — Time view (day-grouped scroll).
// Default window: trailing 30 days. Scroll position is preserved
// across navigation via the History API state pocket (sessionStorage
// would violate the storage guard).
import { validateTimeResponse } from "/pwa/generated/wiki_graph_v1.js";
import { apiGetJSON, clearChildren, el, markReady, renderError } from "/pwa/wiki_lib.js";

const DAY_MS = 24 * 60 * 60 * 1000;
const DEFAULT_WINDOW_DAYS = 30;

function isoDate(d) { return d.toISOString(); }

async function load() {
  const section = document.getElementById("wiki-time-section");
  const status = document.getElementById("wiki-time-status");
  const list = document.getElementById("wiki-time-days");
  const now = new Date();
  const from = new Date(now.getTime() - DEFAULT_WINDOW_DAYS * DAY_MS);
  const path = "/api/time?from=" + encodeURIComponent(isoDate(from)) + "&to=" + encodeURIComponent(isoDate(now));
  try {
    const body = validateTimeResponse(await apiGetJSON(path));
    clearChildren(list);
    for (const day of body.days) {
      const li = el("li", { class: "wiki-time-day", "data-date": day.date });
      li.appendChild(el("h3", { class: "wiki-time-day-heading" }, day.date));
      const ul = el("ul", { class: "wiki-time-day-artifacts", role: "list" });
      for (const a of day.artifacts) {
        ul.appendChild(el("li", { class: "wiki-time-artifact", "data-artifact-id": a.artifactId, "data-captured-at": a.capturedAt },
          el("a", { href: "/pwa/wiki_artifact.html?id=" + encodeURIComponent(a.artifactId) }, a.title || a.artifactId),
          el("time", { datetime: a.capturedAt, class: "wiki-timeline-date" }, " — " + a.capturedAt),
        ));
      }
      li.appendChild(ul);
      list.appendChild(li);
    }
    status.hidden = true; list.hidden = false; markReady(section);
  } catch (e) { renderError(status, e); markReady(section); }
}

// Scroll-position restore: use history.state, which is a non-storage
// per-document pocket. On scroll, replace state in-place (cheap).
window.addEventListener("scroll", () => {
  try { history.replaceState({ y: window.scrollY }, ""); } catch (_) {}
}, { passive: true });
window.addEventListener("load", () => {
  load().then(() => {
    const s = history.state;
    if (s && typeof s.y === "number") {
      requestAnimationFrame(() => window.scrollTo(0, s.y));
    }
  });
});

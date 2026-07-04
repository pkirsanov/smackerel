(function () {
  "use strict";

  const endpoint = "/v1/photos/health/removal";
  const section = document.getElementById("photo-health-removal");
  const status = document.getElementById("photo-health-removal-status");
  const list = document.getElementById("photo-health-removal-list");

  function authHeaders() {
    const headers = { Accept: "application/json" };
    // Spec 100 SCOPE-03 — auth is the same-origin HttpOnly auth_token cookie,
    // attached automatically by the same-origin fetch; no bearer token is read
    // from JS-visible storage.
    return headers;
  }

  function renderCandidates(candidates) {
    list.replaceChildren();
    (candidates || []).forEach(function (candidate) {
      const item = document.createElement("li");
      item.className = "removal-item";
      item.setAttribute("data-removal-id", candidate.id || "");
      item.setAttribute("data-reason", candidate.reason || "");
      item.setAttribute("data-method", candidate.method || "");
      item.setAttribute("data-action-status", candidate.action_status || "pending_review");
      item.textContent =
        "Photo " + (candidate.photo_id || "?") +
        " reason=" + (candidate.reason || "?") +
        " method=" + (candidate.method || "?") +
        " confidence=" + (candidate.confidence || 0).toFixed(2) +
        " rationale: " + (candidate.rationale || "no rationale");
      list.appendChild(item);
    });
  }

  async function load() {
    try {
      const response = await fetch(endpoint, { headers: authHeaders(), credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      status.textContent = "Pending review: " + (body.total || 0);
      renderCandidates(body.candidates);
      section.setAttribute("aria-busy", "false");
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
      section.setAttribute("aria-busy", "false");
    }
  }

  load();
})();

// Spec 038 Scope 1 Screen 3 — connector detail.
//
// Reads ?id=<connID> from the URL, fetches connection state from
// GET /v1/connectors/drive/connection/{id}, and renders the status
// banner, scope summary, and indexed/skipped counts. The empty-drive
// contract renders status=healthy + indexed=0 as
// "Healthy — no in-scope files yet".
(function () {
  "use strict";

  const sectionEl = document.getElementById("connector-detail");
  const statusEl = document.getElementById("connector-detail-status");
  const bodyEl = document.getElementById("connector-detail-body");
  const errorEl = document.getElementById("connector-detail-error");

  const pillEl = document.getElementById("status-pill");
  const pillTextEl = document.getElementById("status-pill-text");
  const detailEl = document.getElementById("status-detail");

  const providerEl = document.getElementById("field-provider");
  const accountEl = document.getElementById("field-account");
  const modeEl = document.getElementById("field-access-mode");
  const scopeEl = document.getElementById("field-scope");
  const indexedEl = document.getElementById("field-indexed");
  const skippedEl = document.getElementById("field-skipped");

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  function showError(msg) {
    errorEl.textContent = msg;
    show(errorEl);
    hide(bodyEl);
    statusEl.textContent = "Failed to load connection.";
    statusEl.classList.remove("status-loading");
    statusEl.classList.add("status-error");
    sectionEl.setAttribute("aria-busy", "false");
  }

  function statusLabel(status, emptyDrive) {
    switch (status) {
      case "healthy":
        return emptyDrive ? "Healthy — no in-scope files yet" : "Healthy";
      case "degraded":
        return "Degraded";
      case "failing":
        return "Failing";
      case "disconnected":
        return "Disconnected";
      default:
        return status || "Unknown";
    }
  }

  function modeLabel(mode) {
    switch (mode) {
      case "read_only": return "Read-only";
      case "read_save": return "Read & save";
      default: return mode || "—";
    }
  }

  function scopeSummary(scope) {
    if (!scope) { return "Entire connected drive"; }
    const ids = Array.isArray(scope.folder_ids) ? scope.folder_ids : [];
    if (ids.length === 0) {
      return scope.include_shared ? "Entire drive plus shared items" : "Entire connected drive";
    }
    const head = ids.slice(0, 3).join(", ");
    const more = ids.length > 3 ? " (+" + (ids.length - 3) + " more)" : "";
    return head + more + (scope.include_shared ? " — including shared items" : "");
  }

  function render(view) {
    pillEl.dataset.status = view.status || "unknown";
    pillTextEl.textContent = statusLabel(view.status, view.empty_drive);
    detailEl.textContent = view.empty_drive
      ? "Connector is healthy and watching for new files."
      : "";

    providerEl.textContent = view.provider_id || "—";
    accountEl.textContent = view.account_label || "—";
    modeEl.textContent = modeLabel(view.access_mode);
    scopeEl.textContent = scopeSummary(view.scope);
    indexedEl.textContent = String(view.indexed_count != null ? view.indexed_count : 0);
    skippedEl.textContent = String(view.skipped_count != null ? view.skipped_count : 0);

    hide(statusEl);
    show(bodyEl);
    sectionEl.setAttribute("aria-busy", "false");
  }

  async function load() {
    const params = new URLSearchParams(window.location.search);
    const id = params.get("id");
    if (!id) {
      showError("Missing connection id in URL.");
      return;
    }
    try {
      const resp = await fetch("/v1/connectors/drive/connection/" + encodeURIComponent(id), {
        method: "GET",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const text = await resp.text();
      if (!resp.ok) {
        showError("HTTP " + resp.status + " " + text);
        return;
      }
      const view = JSON.parse(text);
      render(view);
    } catch (err) {
      showError(String(err && err.message ? err.message : err));
    }
  }

  load();
})();

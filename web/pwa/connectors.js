// Spec 038 Scope 1 Screen 1 — connectors list page.
//
// Loads the provider-neutral list from GET /v1/connectors/drive and
// renders one card per provider using the <template> in connectors.html.
// SCN-038-003 (no provider-specific branching) is enforced here by
// rendering EVERY field from the response — adding a second provider
// requires zero changes in this file.
(function () {
  "use strict";

  const ENDPOINT = "/v1/connectors/drive";

  const sectionEl = document.getElementById("drive-connectors");
  const listEl = document.getElementById("drive-connectors-list");
  const statusEl = document.getElementById("drive-connectors-status");
  const emptyEl = document.getElementById("drive-connectors-empty");
  const errorEl = document.getElementById("drive-connectors-error");
  const tplEl = document.getElementById("drive-connector-card-template");

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  function formatBytes(n) {
    if (typeof n !== "number" || !isFinite(n) || n <= 0) {
      return "—";
    }
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
      v = v / 1024;
      i = i + 1;
    }
    // One decimal place when not an integer, otherwise no decimals.
    const rounded = v >= 100 || v % 1 === 0 ? Math.round(v) : v.toFixed(1);
    return rounded + " " + units[i];
  }

  function yesNo(b) {
    return b ? "Yes" : "No";
  }

  function renderCard(provider) {
    const node = tplEl.content.firstElementChild.cloneNode(true);
    node.dataset.providerId = provider.id;

    const setField = function (name, text) {
      const el = node.querySelector('[data-field="' + name + '"]');
      if (el) {
        el.textContent = text;
      }
    };

    setField("display-name", provider.display_name || provider.id);
    setField("cap-versions", yesNo(provider.capabilities.supports_versions));
    setField("cap-sharing", yesNo(provider.capabilities.supports_sharing));
    setField("cap-change-history", yesNo(provider.capabilities.supports_change_history));
    setField("cap-max-size", formatBytes(provider.capabilities.max_file_size_bytes));

    return node;
  }

  function renderList(providers) {
    listEl.replaceChildren();
    for (const p of providers) {
      listEl.appendChild(renderCard(p));
    }
  }

  function showError(message) {
    hide(listEl);
    hide(emptyEl);
    statusEl.textContent = "Failed to load connectors.";
    statusEl.classList.remove("status-loading");
    statusEl.classList.add("status-error");
    errorEl.textContent = message;
    show(errorEl);
    sectionEl.setAttribute("aria-busy", "false");
  }

  async function load() {
    try {
      const resp = await fetch(ENDPOINT, {
        method: "GET",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!resp.ok) {
        showError("HTTP " + resp.status + " from " + ENDPOINT);
        return;
      }
      const body = await resp.json();
      const providers = Array.isArray(body.providers) ? body.providers : [];
      hide(statusEl);
      sectionEl.setAttribute("aria-busy", "false");
      if (providers.length === 0) {
        show(emptyEl);
        return;
      }
      renderList(providers);
      show(listEl);
    } catch (err) {
      showError(String(err && err.message ? err.message : err));
    }
  }

  load();
})();

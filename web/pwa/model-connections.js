// Spec 096 SCOPE-06 Screen S1 — operator-gated model-connections list.
//
// Loads GET /v1/admin/model-connections and renders one card per declared
// db-mode connection slot using the <template> in model-connections.html.
//
// Three binding UX contracts are enforced here (spec.md §"UI Wireframes" /
// "Write-Only Secret Affordance Contract"):
//
//   - WRITE-ONLY SECRET: a stored credential is rendered as presence + last-4
//     ONLY (secret_present / secret_redaction). The plaintext secret is never
//     present in the response and is never placed in the DOM.
//   - OPERATOR BOUNDARY (FR-B4): 401 → route to web login; 403 → operator-only
//     notice with NO secrets and NO setup affordances.
//   - TRUTHFUL STATUS (Principle 8): a failed last-test reads "✗ failed", a
//     disabled slot reads "○ disabled" — text + glyph, never color-only.
(function () {
  "use strict";

  var ENDPOINT = "/v1/admin/model-connections";

  var sectionEl = document.getElementById("model-connections");
  var listEl = document.getElementById("model-connections-list");
  var statusEl = document.getElementById("model-connections-status");
  var emptyEl = document.getElementById("model-connections-empty");
  var errorEl = document.getElementById("model-connections-error");
  var operatorOnlyEl = document.getElementById("operator-only");
  var addActionEl = document.getElementById("add-action");
  var addNoteEl = document.getElementById("add-note");
  var tplEl = document.getElementById("model-connection-row-template");

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  // Display names for the closed set of provider kinds (spec.md §"UX Spec").
  function kindLabel(kind) {
    switch (kind) {
      case "ollama": return "Ollama";
      case "anthropic": return "Anthropic";
      case "openai": return "OpenAI";
      case "azure-foundry": return "Microsoft Foundry / Azure";
      case "google": return "Google";
      case "bedrock": return "Amazon Bedrock";
      default: return kind || "Unknown";
    }
  }

  function formatWhen(iso) {
    if (!iso) { return ""; }
    var d = new Date(iso);
    if (isNaN(d.getTime())) { return String(iso); }
    return d.toLocaleString();
  }

  // Truthful last-test summary: never tested / ✓ ok / ✗ failed (Principle 8).
  function lastTestedText(conn) {
    if (!conn.last_test_outcome) { return "never tested"; }
    var when = conn.last_tested_at ? " · " + formatWhen(conn.last_tested_at) : "";
    if (conn.last_test_outcome === "ok") { return "✓ ok" + when; }
    return "✗ failed" + when;
  }

  // An anonymous caller (401) is sent to the web login, returning here after.
  function routeToLogin() {
    window.location.assign("/login?next=" + encodeURIComponent(window.location.pathname));
  }

  // A non-operator caller (403): operator-only notice, NO secrets, NO setup.
  function showOperatorOnly() {
    hide(sectionEl);
    hide(addActionEl);
    hide(addNoteEl);
    show(operatorOnlyEl);
  }

  function showError(message) {
    hide(listEl);
    hide(emptyEl);
    statusEl.textContent = "Failed to load connections.";
    statusEl.classList.remove("status-loading");
    statusEl.classList.add("status-error");
    errorEl.textContent = message;
    show(errorEl);
    sectionEl.setAttribute("aria-busy", "false");
  }

  function renderRow(conn) {
    var node = tplEl.content.firstElementChild.cloneNode(true);
    var set = function (name, text) {
      var el = node.querySelector('[data-field="' + name + '"]');
      if (el) { el.textContent = text; }
    };

    set("title", kindLabel(conn.kind) + " (" + conn.connection_id + ")");
    set("kind", conn.kind);

    // Enabled / disabled pill — text + glyph, never color-only.
    var pill = node.querySelector('[data-field="enabled-pill"]');
    if (conn.enabled) {
      pill.dataset.status = "enabled";
      set("enabled", "● enabled");
    } else {
      pill.dataset.status = "disabled";
      set("enabled", "○ disabled");
    }

    // Credential = presence + last-4 ONLY. The secret itself is never present.
    if (conn.secret_present) {
      set("secret", "present · " + (conn.secret_redaction || "····"));
    } else {
      set("secret", "needs credential");
    }

    set("last-tested", lastTestedText(conn));

    var count = conn.model_count != null ? conn.model_count : 0;
    set("models", String(count) + (conn.enabled ? "" : " (disabled)"));

    // Manage / Set up link → detail page for this slot.
    var cta = node.querySelector('[data-field="cta"]');
    cta.href = "/pwa/model-connection-detail.html?id=" + encodeURIComponent(conn.connection_id);
    cta.textContent = conn.secret_present ? "Manage →" : "Set up →";
    cta.setAttribute("aria-label", (conn.secret_present ? "Manage " : "Set up ") + conn.connection_id);

    return node;
  }

  function renderList(conns) {
    listEl.replaceChildren();
    for (var i = 0; i < conns.length; i = i + 1) {
      listEl.appendChild(renderRow(conns[i]));
    }
  }

  async function load() {
    try {
      var resp = await fetch(ENDPOINT, {
        method: "GET",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      if (!resp.ok) { showError("HTTP " + resp.status + " from " + ENDPOINT); return; }

      var body = await resp.json();
      var conns = Array.isArray(body.connections) ? body.connections : [];
      hide(statusEl);
      sectionEl.setAttribute("aria-busy", "false");
      show(addActionEl);
      show(addNoteEl);
      if (conns.length === 0) { show(emptyEl); return; }
      renderList(conns);
      show(listEl);
    } catch (err) {
      showError(String(err && err.message ? err.message : err));
    }
  }

  load();
})();

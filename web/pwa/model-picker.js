// Spec 096 SCOPE-07 Screen S5 — the USER-facing web model picker.
//
// Drives the claim-bound GET/PUT/DELETE /v1/agent/model surface (the SAME
// validator + store the Telegram /model picker uses — one-validator/one-store
// parity, SCN-096-D03). Renders the SCOPE-04 combined provider-qualified catalog
// grouped by provider (Ollama/local FIRST), with the 089 current / system-
// default tags, a per-group cost hint, and capability chips.
//
// Binding contracts enforced here:
//   - CLAIM-BOUND (spec 044): the PUT body carries ONLY {model}. The actor is
//     the authenticated bearer subject — NEVER a body field. Each user picks
//     their OWN selection. There is NO credential field on this page.
//   - SHOWN-BUT-DISABLED (Principle 8): an unreachable provider (provider_status
//     state != ok) is rendered as a disabled group with its TYPED status, out of
//     tab order — never silently dropped. Only reachable models are selectable.
//   - TEXTCONTENT ONLY: every server value is written via textContent (no
//     innerHTML), so a model id / status can never inject markup. Same-origin
//     fetch; 401 → web login.
(function () {
  "use strict";

  var ENDPOINT = "/v1/agent/model";
  var KIND_OLLAMA = "ollama";

  var formEl = document.getElementById("model-picker-form");
  var groupsEl = document.getElementById("provider-groups");
  var statusEl = document.getElementById("picker-status");
  var emptyEl = document.getElementById("picker-empty");
  var errorEl = document.getElementById("picker-error");
  var costNoteEl = document.getElementById("cost-note");
  var currentEl = document.getElementById("current-line");
  var budgetEl = document.getElementById("budget-line");
  var useBtn = document.getElementById("use-model");
  var resetBtn = document.getElementById("reset-model");
  var groupTpl = document.getElementById("provider-group-template");
  var rowTpl = document.getElementById("model-row-template");

  // cost_class → paid set, keyed by model id, so the cost note can react to the
  // selected radio without re-reading the server.
  var paidModels = Object.create(null);

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  // Display names for the closed set of provider kinds (parity with the
  // Telegram picker + model-connections.js).
  function kindLabel(kind) {
    switch (kind) {
      case "ollama": return "Ollama";
      case "anthropic": return "Anthropic";
      case "openai": return "OpenAI";
      case "azure-foundry": return "Microsoft Foundry / Azure";
      case "google": return "Google";
      case "bedrock": return "Amazon Bedrock";
      default: return kind || "Provider";
    }
  }

  // Typed reachability status → text + glyph (never color-only).
  function statusText(state) {
    switch (state) {
      case "ok": return "● connected";
      case "unreachable": return "⚠ unreachable";
      case "timeout": return "⚠ slow / timed out";
      case "auth_failed": return "⚠ auth failed";
      case "disabled": return "○ disabled";
      default: return "⚠ unavailable";
    }
  }

  function costLabel(costClass) {
    return costClass === "free" ? "local · free" : "paid";
  }

  function routeToLogin() {
    window.location.assign("/login?next=" + encodeURIComponent(window.location.pathname));
  }

  function showError(message) {
    errorEl.textContent = message;
    show(errorEl);
  }

  function clearError() {
    errorEl.textContent = "";
    hide(errorEl);
  }

  // The capability + cost meta line for a reachable model row.
  function metaText(entry) {
    var parts = [];
    var caps = Array.isArray(entry.capabilities) ? entry.capabilities : [];
    if (caps.indexOf("tool_capable") !== -1) { parts.push("🔧 tools"); }
    if (caps.indexOf("vision") !== -1) { parts.push("👁 vision"); }
    if (entry.context_window) { parts.push(Math.round(entry.context_window / 1000) + "k ctx"); }
    parts.push(costLabel(entry.cost_class));
    return parts.join(" · ");
  }

  // The 089 current / system-default tag suffix for a model id.
  function tagText(id, view) {
    var tags = [];
    if (id === view.effective_model) { tags.push("current"); }
    if (id === view.system_default) { tags.push("system default"); }
    return tags.length ? "  [" + tags.join(" · ") + "]" : "";
  }

  // Group the reachable catalog[] by kind, Ollama/local FIRST, preserving first-
  // seen order for the rest.
  function groupByKind(catalogEntries) {
    var order = [];
    var byKind = Object.create(null);
    for (var i = 0; i < catalogEntries.length; i = i + 1) {
      var e = catalogEntries[i];
      if (!byKind[e.kind]) { byKind[e.kind] = []; order.push(e.kind); }
      byKind[e.kind].push(e);
    }
    order.sort(function (a, b) {
      if (a === KIND_OLLAMA) { return -1; }
      if (b === KIND_OLLAMA) { return 1; }
      return 0;
    });
    return { order: order, byKind: byKind };
  }

  function statusByKind(providerStatuses) {
    var m = Object.create(null);
    for (var i = 0; i < providerStatuses.length; i = i + 1) {
      m[providerStatuses[i].kind] = providerStatuses[i];
    }
    return m;
  }

  function setGroupStatus(node, state) {
    var pill = node.querySelector('[data-field="group-status"]');
    var pillText = node.querySelector('[data-field="group-status-text"]');
    pill.dataset.status = state || "";
    pillText.textContent = statusText(state);
  }

  // Render one reachable provider group (a radiogroup of selectable models).
  function renderReachableGroup(kind, entries, status, view) {
    var node = groupTpl.content.firstElementChild.cloneNode(true);
    node.querySelector('[data-field="group-name"]').textContent =
      kindLabel(kind) + " · " + costLabel(entries[0].cost_class);
    setGroupStatus(node, status ? status.state : "ok");
    var rg = node.querySelector('[data-field="radiogroup"]');
    rg.setAttribute("aria-label", kindLabel(kind) + " models");
    for (var i = 0; i < entries.length; i = i + 1) {
      rg.appendChild(renderRow(entries[i], view));
    }
    return node;
  }

  function renderRow(entry, view) {
    var row = rowTpl.content.firstElementChild.cloneNode(true);
    var radio = row.querySelector('[data-field="radio"]');
    radio.value = entry.id;
    if (entry.id === view.effective_model) { radio.checked = true; }
    if (entry.cost_class === "paid") { paidModels[entry.id] = true; }
    row.querySelector('[data-field="model-id"]').textContent = entry.id + tagText(entry.id, view);
    row.querySelector('[data-field="model-meta"]').textContent = metaText(entry);
    return row;
  }

  // Render one UNREACHABLE provider group: shown-but-disabled, out of tab order,
  // carrying its typed status — never silently dropped (Principle 8).
  function renderDisabledGroup(status) {
    var node = groupTpl.content.firstElementChild.cloneNode(true);
    node.querySelector('[data-field="group"]').setAttribute("aria-disabled", "true");
    node.querySelector('[data-field="group-name"]').textContent = kindLabel(status.kind);
    setGroupStatus(node, status.state);
    var note = node.querySelector('[data-field="group-note"]');
    note.textContent =
      "Temporarily unavailable — its models are hidden from selection until the " +
      "operator re-tests this connection. Not silently dropped.";
    show(note);
    return node;
  }

  function render(view) {
    paidModels = Object.create(null);
    groupsEl.replaceChildren();
    clearError();
    hide(costNoteEl);

    // Current selection line.
    var sourceTag = view.source === "sticky" ? "your default" : "system default";
    currentEl.textContent = "Currently: " + view.effective_model + " — " + sourceTag;
    show(currentEl);

    // Budget line — only when the server enriched it (a paid model exists).
    if (view.budget && typeof view.budget.month_to_date_usd === "number") {
      budgetEl.textContent = "This month: $" + view.budget.month_to_date_usd.toFixed(2) + " used";
      show(budgetEl);
    } else {
      hide(budgetEl);
    }

    var catalogEntries = Array.isArray(view.catalog) ? view.catalog : [];
    var providerStatuses = Array.isArray(view.provider_statuses) ? view.provider_statuses : [];

    if (catalogEntries.length === 0 && providerStatuses.length === 0) {
      show(emptyEl);
      useBtn.disabled = true;
      formEl.setAttribute("aria-busy", "false");
      hide(statusEl);
      return;
    }

    var grouped = groupByKind(catalogEntries);
    var statuses = statusByKind(providerStatuses);
    var renderedKinds = Object.create(null);

    // Reachable groups first (in Ollama-first order).
    for (var i = 0; i < grouped.order.length; i = i + 1) {
      var kind = grouped.order[i];
      renderedKinds[kind] = true;
      groupsEl.appendChild(renderReachableGroup(kind, grouped.byKind[kind], statuses[kind], view));
    }
    // Then any provider that is NOT reachable (shown-but-disabled).
    for (var j = 0; j < providerStatuses.length; j = j + 1) {
      var st = providerStatuses[j];
      if (st.state !== "ok" && !renderedKinds[st.kind]) {
        groupsEl.appendChild(renderDisabledGroup(st));
      }
    }

    useBtn.disabled = false;
    formEl.setAttribute("aria-busy", "false");
    hide(statusEl);
  }

  // Selecting a paid radio reveals the inline cost note (pull-not-push).
  function onSelectionChange(ev) {
    var target = ev.target;
    if (!target || target.name !== "model") { return; }
    if (paidModels[target.value]) {
      costNoteEl.textContent =
        "This is a paid model — each /ask draws on your monthly budget. " +
        "Local Ollama models are free.";
      show(costNoteEl);
    } else {
      hide(costNoteEl);
    }
  }

  function selectedModel() {
    var checked = formEl.querySelector('input[name="model"]:checked');
    return checked ? checked.value : "";
  }

  async function load() {
    try {
      var resp = await fetch(ENDPOINT, {
        method: "GET",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (!resp.ok) { showError("HTTP " + resp.status + " from " + ENDPOINT); return; }
      render(await resp.json());
    } catch (err) {
      showError(String(err && err.message ? err.message : err));
    }
  }

  async function useModel(ev) {
    ev.preventDefault();
    clearError();
    var model = selectedModel();
    if (!model) { showError("Pick a model first."); return; }
    try {
      var resp = await fetch(ENDPOINT, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        // CLAIM-BOUND: ONLY the model id — never a user id (the actor is the
        // authenticated bearer subject, derived server-side).
        body: JSON.stringify({ model: model }),
      });
      if (resp.status === 401) { routeToLogin(); return; }
      var body = await resp.json();
      if (resp.status === 400) {
        // Off-catalog / unavailable: the SAME modelswitch.Rejection sentence;
        // the prior selection is unchanged (no store write server-side).
        showError(body && body.message ? body.message : "That model isn't available right now.");
        return;
      }
      if (!resp.ok) { showError("HTTP " + resp.status + " setting your model."); return; }
      render(body);
    } catch (err) {
      showError(String(err && err.message ? err.message : err));
    }
  }

  async function resetModel() {
    clearError();
    try {
      var resp = await fetch(ENDPOINT, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (!resp.ok) { showError("HTTP " + resp.status + " resetting your model."); return; }
      render(await resp.json());
    } catch (err) {
      showError(String(err && err.message ? err.message : err));
    }
  }

  formEl.addEventListener("submit", useModel);
  formEl.addEventListener("change", onSelectionChange);
  resetBtn.addEventListener("click", resetModel);

  load();
})();

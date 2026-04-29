// Spec 038 Scope 1 Screen 2 — add-drive flow.
//
// Loads the provider list from GET /v1/connectors/drive, lets the user
// pick a provider + access mode + folder scope, and POSTs the form to
// /v1/connectors/drive/connect. The handler returns {authURL, state};
// the page redirects the browser to authURL so the upstream OAuth
// provider can complete the redirect leg.
//
// Owner identity is currently a per-browser UUID stored in localStorage.
// A future scope will lift this into the authenticated session.
(function () {
  "use strict";

  const LIST_ENDPOINT = "/v1/connectors/drive";
  const CONNECT_ENDPOINT = "/v1/connectors/drive/connect";
  const OWNER_KEY = "smackerel.drive.owner_user_id";

  const form = document.getElementById("drive-add-form");
  const fieldset = document.getElementById("provider-fieldset");
  const optionsEl = document.getElementById("provider-options");
  const statusEl = document.getElementById("provider-status");
  const submitEl = document.getElementById("drive-add-submit");
  const errorEl = document.getElementById("drive-add-error");

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  function ensureOwner() {
    let owner;
    try { owner = localStorage.getItem(OWNER_KEY); } catch (_) { owner = null; }
    if (owner) {
      return owner;
    }
    // Cryptographically-random UUID generation when available.
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      owner = crypto.randomUUID();
    } else {
      // Fallback non-crypto UUID for legacy browsers (still locally stable).
      owner = "owner-" + Date.now().toString(16) + "-" + Math.random().toString(16).slice(2);
    }
    try { localStorage.setItem(OWNER_KEY, owner); } catch (_) { /* swallow */ }
    return owner;
  }

  function showError(msg) {
    errorEl.textContent = msg;
    show(errorEl);
  }
  function clearError() {
    errorEl.textContent = "";
    hide(errorEl);
  }

  function renderProviders(providers) {
    optionsEl.replaceChildren();
    if (providers.length === 0) {
      statusEl.textContent = "No drive providers are installed in this Smackerel deployment.";
      statusEl.classList.remove("status-loading");
      statusEl.classList.add("status-empty");
      fieldset.setAttribute("aria-busy", "false");
      return;
    }
    for (let i = 0; i < providers.length; i = i + 1) {
      const p = providers[i];
      const id = "provider-radio-" + p.id;
      const label = document.createElement("label");
      label.className = "radio";
      const input = document.createElement("input");
      input.type = "radio";
      input.name = "provider_id";
      input.value = p.id;
      input.id = id;
      input.required = true;
      if (i === 0) { input.checked = true; }
      const span = document.createElement("span");
      span.className = "radio-label";
      const strong = document.createElement("strong");
      strong.textContent = p.display_name || p.id;
      const small = document.createElement("small");
      small.textContent = "id: " + p.id;
      span.appendChild(strong);
      span.appendChild(document.createElement("br"));
      span.appendChild(small);
      label.appendChild(input);
      label.appendChild(span);
      optionsEl.appendChild(label);
    }
    hide(statusEl);
    fieldset.setAttribute("aria-busy", "false");
    submitEl.disabled = false;
  }

  async function loadProviders() {
    try {
      const resp = await fetch(LIST_ENDPOINT, {
        method: "GET",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!resp.ok) {
        throw new Error("HTTP " + resp.status);
      }
      const body = await resp.json();
      const providers = Array.isArray(body.providers) ? body.providers : [];
      renderProviders(providers);
    } catch (err) {
      statusEl.textContent = "Failed to load providers: " + (err && err.message ? err.message : String(err));
      statusEl.classList.remove("status-loading");
      statusEl.classList.add("status-error");
      statusEl.setAttribute("role", "alert");
      fieldset.setAttribute("aria-busy", "false");
    }
  }

  function parseFolderIDs(raw) {
    if (!raw) { return []; }
    return raw.split(/\r?\n/)
      .map(function (s) { return s.trim(); })
      .filter(function (s) { return s.length > 0; });
  }

  async function handleSubmit(ev) {
    ev.preventDefault();
    clearError();
    submitEl.disabled = true;

    const providerEl = document.querySelector('input[name="provider_id"]:checked');
    const modeEl = document.querySelector('input[name="access_mode"]:checked');
    if (!providerEl) {
      showError("Pick a provider before connecting.");
      submitEl.disabled = false;
      return;
    }
    if (!modeEl) {
      showError("Pick an access mode before connecting.");
      submitEl.disabled = false;
      return;
    }

    const folderInput = document.getElementById("folder-scope-input");
    const includeShared = document.getElementById("include-shared").checked;

    const body = {
      provider_id: providerEl.value,
      owner_user_id: ensureOwner(),
      access_mode: modeEl.value,
      scope: {
        folder_ids: parseFolderIDs(folderInput.value),
        include_shared: includeShared,
      },
    };

    try {
      const resp = await fetch(CONNECT_ENDPOINT, {
        method: "POST",
        headers: {
          "Accept": "application/json",
          "Content-Type": "application/json",
        },
        credentials: "same-origin",
        body: JSON.stringify(body),
      });
      const text = await resp.text();
      if (!resp.ok) {
        showError("Connect failed: HTTP " + resp.status + " " + text);
        submitEl.disabled = false;
        return;
      }
      let data;
      try { data = JSON.parse(text); } catch (_) {
        showError("Connect returned invalid JSON: " + text);
        submitEl.disabled = false;
        return;
      }
      if (!data.authURL || !data.state) {
        showError("Connect response missing authURL or state.");
        submitEl.disabled = false;
        return;
      }
      window.location.href = data.authURL;
    } catch (err) {
      showError("Connect request failed: " + (err && err.message ? err.message : String(err)));
      submitEl.disabled = false;
    }
  }

  form.addEventListener("submit", handleSubmit);
  loadProviders();
})();

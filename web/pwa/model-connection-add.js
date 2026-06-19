// Spec 096 SCOPE-06 Screen S2 — operator-gated add / configure flow.
//
// Loads GET /v1/admin/model-connections, scopes the provider picker to
// declared db-mode slots that LACK a credential, and — after a slot is
// chosen — renders that kind's WRITE-ONLY secret fields plus the slot's
// non-secret params read-only. Save POSTs the cleartext only in the
// PUT .../{id}/credential request body; the response is the redacted view
// (never the secret), then routes to the detail page to test + enable.
//
// Write-Only Secret Affordance Contract (spec.md, binding):
//   - input type=password, autocomplete=off, empty on load, never pre-filled,
//     placeholder "Enter to set/replace — never displayed";
//   - the server never returns the secret, so nothing is ever echoed back;
//   - NO reveal / show-password / copy-secret control exists anywhere.
(function () {
  "use strict";

  var ENDPOINT = "/v1/admin/model-connections";

  // Per-kind WRITE-ONLY secret fields (design §4). Google is one-of: a Gemini
  // api_key OR a Vertex service_account JSON (the server accepts either).
  var KIND_SECRET_FIELDS = {
    "anthropic": [{ key: "api_key", label: "API key" }],
    "openai": [{ key: "api_key", label: "API key" }],
    "azure-foundry": [{ key: "api_key", label: "API key" }],
    "bedrock": [
      { key: "aws_access_key_id", label: "AWS access key ID" },
      { key: "aws_secret_access_key", label: "AWS secret access key" }
    ],
    "google": [
      { key: "api_key", label: "Gemini API key (or use the service account below)" },
      { key: "service_account", label: "Vertex service-account JSON (single line)" }
    ]
  };

  // Short "needs:" hint per kind for the provider radiogroup.
  function needsHint(kind) {
    switch (kind) {
      case "anthropic": return "needs: API key";
      case "openai": return "needs: API key";
      case "azure-foundry": return "needs: API key";
      case "google": return "needs: Gemini API key or Vertex service-account JSON";
      case "bedrock": return "needs: AWS access key + secret";
      default: return "needs: credential";
    }
  }

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

  function humanizeParam(key) {
    return String(key)
      .replace(/_/g, " ")
      .replace(/\b\w/g, function (c) { return c.toUpperCase(); });
  }

  var form = document.getElementById("add-form");
  var fieldset = document.getElementById("provider-fieldset");
  var optionsEl = document.getElementById("provider-options");
  var statusEl = document.getElementById("provider-status");
  var emptyEl = document.getElementById("provider-empty");
  var credentialFieldset = document.getElementById("credential-fieldset");
  var credentialKindLabel = document.getElementById("credential-kind-label");
  var paramsReadonly = document.getElementById("params-readonly");
  var paramsListEl = document.getElementById("params-list");
  var secretInputsEl = document.getElementById("secret-inputs");
  var saveBtn = document.getElementById("save-btn");
  var errorEl = document.getElementById("add-error");
  var operatorOnlyEl = document.getElementById("operator-only");

  // The connection slots, keyed by id, so the submit handler can re-read kind.
  var slotsByID = {};
  var selectedID = null;

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  function routeToLogin() {
    window.location.assign("/login?next=" + encodeURIComponent(window.location.pathname));
  }

  function showOperatorOnly() {
    hide(form);
    show(operatorOnlyEl);
  }

  function showError(msg) {
    errorEl.textContent = msg;
    show(errorEl);
  }
  function clearError() {
    errorEl.textContent = "";
    hide(errorEl);
  }

  // Build one WRITE-ONLY secret input. Empty on load, type=password,
  // autocomplete=off, never pre-filled — and never set from server data.
  function makeSecretInput(field) {
    var wrap = document.createElement("div");

    var label = document.createElement("label");
    label.className = "field-label";
    label.setAttribute("for", "secret-" + field.key);
    label.textContent = field.label;

    var input = document.createElement("input");
    input.type = "password";
    input.id = "secret-" + field.key;
    input.name = field.key;
    input.autocomplete = "off";
    input.value = "";
    input.setAttribute("data-secret-field", field.key);
    input.setAttribute("placeholder", "Enter to set/replace — never displayed");
    input.setAttribute("aria-describedby", "secret-hint");

    wrap.appendChild(label);
    wrap.appendChild(input);
    return wrap;
  }

  function renderParams(params) {
    paramsListEl.replaceChildren();
    var keys = params ? Object.keys(params) : [];
    if (keys.length === 0) {
      hide(paramsReadonly);
      return;
    }
    for (var i = 0; i < keys.length; i = i + 1) {
      var row = document.createElement("div");
      var dt = document.createElement("dt");
      dt.textContent = humanizeParam(keys[i]);
      var dd = document.createElement("dd");
      dd.textContent = String(params[keys[i]]);
      row.appendChild(dt);
      row.appendChild(dd);
      paramsListEl.appendChild(row);
    }
    show(paramsReadonly);
  }

  // Reveal the chosen slot's per-kind credential form.
  function selectSlot(conn) {
    selectedID = conn.connection_id;
    credentialKindLabel.textContent = kindLabel(conn.kind) + " (" + conn.connection_id + ")";
    renderParams(conn.params);

    secretInputsEl.replaceChildren();
    var fields = KIND_SECRET_FIELDS[conn.kind] || [{ key: "api_key", label: "API key" }];
    for (var i = 0; i < fields.length; i = i + 1) {
      secretInputsEl.appendChild(makeSecretInput(fields[i]));
    }
    show(credentialFieldset);
    saveBtn.disabled = false;
  }

  function renderProviders(conns) {
    optionsEl.replaceChildren();
    // Scope to declared db-mode slots that LACK a credential.
    var unconfigured = conns.filter(function (c) { return !c.secret_present; });
    if (unconfigured.length === 0) {
      hide(statusEl);
      show(emptyEl);
      fieldset.setAttribute("aria-busy", "false");
      return;
    }
    for (var i = 0; i < unconfigured.length; i = i + 1) {
      var c = unconfigured[i];
      slotsByID[c.connection_id] = c;

      var id = "provider-radio-" + c.connection_id;
      var label = document.createElement("label");
      label.className = "radio";

      var input = document.createElement("input");
      input.type = "radio";
      input.name = "connection_id";
      input.setAttribute("value", c.connection_id);
      input.id = id;
      input.required = true;

      var span = document.createElement("span");
      span.className = "radio-label";
      var strong = document.createElement("strong");
      strong.textContent = kindLabel(c.kind) + " (" + c.connection_id + ")";
      var small = document.createElement("small");
      small.textContent = needsHint(c.kind);
      span.appendChild(strong);
      span.appendChild(document.createElement("br"));
      span.appendChild(small);

      label.appendChild(input);
      label.appendChild(span);
      optionsEl.appendChild(label);
    }
    hide(statusEl);
    fieldset.setAttribute("aria-busy", "false");
  }

  function onProviderChange(ev) {
    var target = ev.target;
    if (!target || target.name !== "connection_id") { return; }
    var conn = slotsByID[target.value];
    if (conn) { selectSlot(conn); }
  }

  function collectSecretFields() {
    var out = {};
    var inputs = secretInputsEl.querySelectorAll("[data-secret-field]");
    for (var i = 0; i < inputs.length; i = i + 1) {
      var key = inputs[i].getAttribute("data-secret-field");
      var val = (inputs[i].value || "").trim();
      if (key && val) { out[key] = val; }
    }
    return out;
  }

  async function handleSubmit(ev) {
    ev.preventDefault();
    clearError();
    if (!selectedID) {
      showError("Pick a provider slot to configure.");
      return;
    }
    var secretFields = collectSecretFields();
    if (Object.keys(secretFields).length === 0) {
      showError("Enter the credential before saving.");
      return;
    }
    saveBtn.disabled = true;
    try {
      var resp = await fetch(
        ENDPOINT + "/" + encodeURIComponent(selectedID) + "/credential",
        {
          method: "PUT",
          headers: { "Accept": "application/json", "Content-Type": "application/json" },
          credentials: "same-origin",
          body: JSON.stringify({ secret_fields: secretFields }),
        }
      );
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      var text = await resp.text();
      if (!resp.ok) {
        var msg = "Save failed: HTTP " + resp.status;
        try {
          var errBody = JSON.parse(text);
          if (errBody && errBody.message) { msg = errBody.message; }
        } catch (_) { /* keep the HTTP-status message */ }
        showError(msg);
        saveBtn.disabled = false;
        return;
      }
      // Success: the response is the REDACTED view (no secret). Route to the
      // detail page to test + enable the freshly-stored credential.
      window.location.assign(
        "/pwa/model-connection-detail.html?id=" + encodeURIComponent(selectedID) + "&saved=1"
      );
    } catch (err) {
      showError("Save request failed: " + (err && err.message ? err.message : String(err)));
      saveBtn.disabled = false;
    }
  }

  async function loadSlots() {
    try {
      var resp = await fetch(ENDPOINT, {
        method: "GET",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      if (!resp.ok) { throw new Error("HTTP " + resp.status); }
      var body = await resp.json();
      var conns = Array.isArray(body.connections) ? body.connections : [];
      renderProviders(conns);
    } catch (err) {
      statusEl.textContent = "Failed to load connections: " +
        (err && err.message ? err.message : String(err));
      statusEl.classList.remove("status-loading");
      statusEl.classList.add("status-error");
      statusEl.setAttribute("role", "alert");
      fieldset.setAttribute("aria-busy", "false");
    }
  }

  optionsEl.addEventListener("change", onProviderChange);
  form.addEventListener("submit", handleSubmit);
  loadSlots();
})();

// Spec 096 SCOPE-06 Screen S3 — operator-gated connection detail.
//
// Reads ?id=<connID>, fetches GET /v1/admin/model-connections/{id}, and wires
// the four runtime actions against the admin API:
//   - Test   → POST .../{id}/test    (truthful typed pass/fail)
//   - Enable → POST .../{id}/enable  (409-guarded: credential + last-test ok)
//   - Disable→ POST .../{id}/disable
//   - Replace→ PUT  .../{id}/credential (write-only rotation; clears last-test)
//
// Binding contracts (spec.md):
//   - WRITE-ONLY SECRET: the stored credential renders as presence + last-4
//     ONLY; the Replace input is type=password, autocomplete=off, empty on
//     load, never pre-filled. NO reveal / show / copy control exists.
//   - TRUTHFUL TEST (SCN-096-W04): a failed test renders the danger banner
//     (role=alert) with the typed detail naming the connection — never a
//     false success, never an Ollama substitute. Enable stays disabled until
//     a credential is present AND the last test = ok (the 409 guard).
(function () {
  "use strict";

  var BASE = "/v1/admin/model-connections";

  // Per-kind WRITE-ONLY secret fields (design §4) — same closed set as S2.
  var KIND_SECRET_FIELDS = {
    "anthropic": [{ key: "api_key", label: "New API key" }],
    "openai": [{ key: "api_key", label: "New API key" }],
    "azure-foundry": [{ key: "api_key", label: "New API key" }],
    "bedrock": [
      { key: "aws_access_key_id", label: "New AWS access key ID" },
      { key: "aws_secret_access_key", label: "New AWS secret access key" }
    ],
    "google": [
      { key: "api_key", label: "New Gemini API key (or use the service account below)" },
      { key: "service_account", label: "New Vertex service-account JSON (single line)" }
    ]
  };

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

  function lastTestedText(view) {
    if (!view.last_test_outcome) { return "never tested"; }
    var when = view.last_tested_at ? " · " + formatWhen(view.last_tested_at) : "";
    if (view.last_test_outcome === "ok") { return "✓ ok" + when; }
    return "✗ failed" + when;
  }

  var sectionEl = document.getElementById("detail");
  var statusEl = document.getElementById("detail-status");
  var bodyEl = document.getElementById("detail-body");
  var detailErrorEl = document.getElementById("detail-error");
  var operatorOnlyEl = document.getElementById("operator-only");
  var pageTitleEl = document.getElementById("page-title");

  var bannerOk = document.getElementById("banner-ok");
  var bannerOkDetail = document.getElementById("banner-ok-detail");
  var bannerFail = document.getElementById("banner-fail");
  var bannerFailDetail = document.getElementById("banner-fail-detail");
  var bannerInfo = document.getElementById("banner-info");

  var fieldKind = document.getElementById("field-kind");
  var fieldEnabled = document.getElementById("field-enabled");
  var fieldSecret = document.getElementById("field-secret");
  var fieldLastTested = document.getElementById("field-last-tested");
  var fieldModels = document.getElementById("field-models");

  var testBtn = document.getElementById("test-btn");
  var enableBtn = document.getElementById("enable-btn");
  var disableBtn = document.getElementById("disable-btn");
  var enableHint = document.getElementById("enable-hint");
  var actionErrorEl = document.getElementById("action-error");

  var rotateInputsEl = document.getElementById("rotate-inputs");
  var rotateBtn = document.getElementById("rotate-btn");
  var rotateErrorEl = document.getElementById("rotate-error");

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  var connID = null;
  var currentKind = null;
  var rotateBuilt = false;

  function routeToLogin() {
    window.location.assign("/login?next=" + encodeURIComponent(window.location.pathname + window.location.search));
  }

  function showOperatorOnly() {
    hide(sectionEl);
    show(operatorOnlyEl);
  }

  function showDetailError(msg) {
    detailErrorEl.textContent = msg;
    show(detailErrorEl);
    hide(bodyEl);
    statusEl.textContent = "Failed to load connection.";
    statusEl.classList.remove("status-loading");
    statusEl.classList.add("status-error");
    sectionEl.setAttribute("aria-busy", "false");
  }

  function hideTestBanners() {
    hide(bannerOk);
    hide(bannerFail);
  }

  function showInfo(msg) {
    bannerInfo.textContent = msg;
    show(bannerInfo);
  }
  function clearActionError() {
    actionErrorEl.textContent = "";
    hide(actionErrorEl);
  }

  // Build the write-only rotation inputs once, for this connection's kind.
  function buildRotateInputs(kind) {
    if (rotateBuilt) { return; }
    rotateInputsEl.replaceChildren();
    var fields = KIND_SECRET_FIELDS[kind] || [{ key: "api_key", label: "New API key" }];
    for (var i = 0; i < fields.length; i = i + 1) {
      var field = fields[i];
      var wrap = document.createElement("div");

      var label = document.createElement("label");
      label.className = "field-label";
      label.setAttribute("for", "rotate-" + field.key);
      label.textContent = field.label;

      var input = document.createElement("input");
      input.type = "password";
      input.id = "rotate-" + field.key;
      input.name = field.key;
      input.autocomplete = "off";
      input.value = "";
      input.setAttribute("data-secret-field", field.key);
      input.setAttribute("placeholder", "Enter to replace — never displayed");
      input.setAttribute("aria-describedby", "rotate-hint");

      wrap.appendChild(label);
      wrap.appendChild(input);
      rotateInputsEl.appendChild(wrap);
    }
    rotateBuilt = true;
  }

  // Render the read-only fields + the action-button states. Banners are
  // controlled separately so a test outcome survives a field refresh.
  function renderFields(view) {
    currentKind = view.kind;
    pageTitleEl.textContent = kindLabel(view.kind) + " (" + view.connection_id + ")";

    fieldKind.textContent = view.kind;
    fieldEnabled.textContent = view.enabled ? "● yes" : "○ no";
    fieldSecret.textContent = view.secret_present
      ? "present · " + (view.secret_redaction || "····")
      : "needs credential";
    fieldLastTested.textContent = lastTestedText(view);
    fieldModels.textContent = String(view.model_count != null ? view.model_count : 0);

    // Test needs a stored credential (the server 409s otherwise).
    testBtn.disabled = !view.secret_present;

    // Enable / Disable. Enable is 409-guarded: credential present AND last
    // test = ok. Disable is shown only when currently enabled.
    var testedOk = view.secret_present && view.last_test_outcome === "ok";
    if (view.enabled) {
      hide(enableBtn);
      hide(enableHint);
      show(disableBtn);
    } else {
      show(enableBtn);
      hide(disableBtn);
      enableBtn.disabled = !testedOk;
      if (testedOk) { hide(enableHint); } else { show(enableHint); }
    }

    buildRotateInputs(view.kind);
  }

  // Truthful typed outcome. A pass shows the success banner; a fail shows the
  // danger banner with the typed detail naming the connection — never a false
  // success, never an Ollama substitute.
  function renderTestOutcome(result) {
    hideTestBanners();
    hide(bannerInfo);
    if (result && result.outcome === "ok") {
      bannerOkDetail.textContent = "tested ok" +
        (result.tested_at ? " · " + formatWhen(result.tested_at) : "");
      show(bannerOk);
      return;
    }
    var detail = (result && result.detail) ? result.detail : "unreachable";
    var when = (result && result.tested_at) ? " · " + formatWhen(result.tested_at) : "";
    bannerFailDetail.textContent = detail + ": connection " + connID +
      " is NOT usable (not substituted)" + when;
    show(bannerFail);
  }

  function applyTestToView(view, result) {
    view.last_tested_at = result ? result.tested_at : view.last_tested_at;
    view.last_test_outcome = (result && result.outcome === "ok") ? "ok" : "failed";
    return view;
  }

  // The authoritative view, refreshed by every successful mutation response.
  var currentView = null;

  async function onTest() {
    clearActionError();
    testBtn.disabled = true;
    testBtn.setAttribute("aria-busy", "true");
    try {
      var resp = await fetch(BASE + "/" + encodeURIComponent(connID) + "/test", {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      var text = await resp.text();
      if (!resp.ok) {
        actionErrorEl.textContent = errorMessage(text, "Test failed: HTTP " + resp.status);
        show(actionErrorEl);
        return;
      }
      var result = JSON.parse(text);
      currentView = applyTestToView(currentView, result);
      renderFields(currentView);
      renderTestOutcome(result);
    } catch (err) {
      actionErrorEl.textContent = "Test request failed: " + (err && err.message ? err.message : String(err));
      show(actionErrorEl);
    } finally {
      testBtn.removeAttribute("aria-busy");
      if (currentView) { testBtn.disabled = !currentView.secret_present; }
    }
  }

  async function onEnable() {
    clearActionError();
    enableBtn.disabled = true;
    try {
      var resp = await fetch(BASE + "/" + encodeURIComponent(connID) + "/enable", {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      var text = await resp.text();
      if (resp.status === 409) {
        actionErrorEl.textContent = errorMessage(text,
          "Cannot enable: store a credential and pass a test first.");
        show(actionErrorEl);
        return;
      }
      if (!resp.ok) {
        actionErrorEl.textContent = errorMessage(text, "Enable failed: HTTP " + resp.status);
        show(actionErrorEl);
        return;
      }
      currentView = JSON.parse(text);
      hideTestBanners();
      renderFields(currentView);
      showInfo("Enabled — this provider's models are in the catalog.");
    } catch (err) {
      actionErrorEl.textContent = "Enable request failed: " + (err && err.message ? err.message : String(err));
      show(actionErrorEl);
    } finally {
      if (currentView && !currentView.enabled) {
        enableBtn.disabled = !(currentView.secret_present && currentView.last_test_outcome === "ok");
      }
    }
  }

  async function onDisable() {
    clearActionError();
    disableBtn.disabled = true;
    try {
      var resp = await fetch(BASE + "/" + encodeURIComponent(connID) + "/disable", {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      var text = await resp.text();
      if (!resp.ok) {
        actionErrorEl.textContent = errorMessage(text, "Disable failed: HTTP " + resp.status);
        show(actionErrorEl);
        disableBtn.disabled = false;
        return;
      }
      currentView = JSON.parse(text);
      hideTestBanners();
      renderFields(currentView);
      showInfo("Disabled — this provider's models left the catalog.");
    } catch (err) {
      actionErrorEl.textContent = "Disable request failed: " + (err && err.message ? err.message : String(err));
      show(actionErrorEl);
      disableBtn.disabled = false;
    }
  }

  function collectRotateFields() {
    var out = {};
    var inputs = rotateInputsEl.querySelectorAll("[data-secret-field]");
    for (var i = 0; i < inputs.length; i = i + 1) {
      var key = inputs[i].getAttribute("data-secret-field");
      var val = (inputs[i].value || "").trim();
      if (key && val) { out[key] = val; }
    }
    return out;
  }

  function clearRotateInputs() {
    var inputs = rotateInputsEl.querySelectorAll("[data-secret-field]");
    for (var i = 0; i < inputs.length; i = i + 1) { inputs[i].value = ""; }
  }

  async function onRotate() {
    rotateErrorEl.textContent = "";
    hide(rotateErrorEl);
    var secretFields = collectRotateFields();
    if (Object.keys(secretFields).length === 0) {
      rotateErrorEl.textContent = "Enter a new credential to replace the stored one.";
      show(rotateErrorEl);
      return;
    }
    rotateBtn.disabled = true;
    try {
      var resp = await fetch(BASE + "/" + encodeURIComponent(connID) + "/credential", {
        method: "PUT",
        headers: { "Accept": "application/json", "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ secret_fields: secretFields }),
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      var text = await resp.text();
      if (!resp.ok) {
        rotateErrorEl.textContent = errorMessage(text, "Replace failed: HTTP " + resp.status);
        show(rotateErrorEl);
        return;
      }
      // Redacted view (no secret). The store clears the last test on a new
      // credential → re-test required before re-enabling.
      currentView = JSON.parse(text);
      clearRotateInputs();
      hideTestBanners();
      renderFields(currentView);
      showInfo("Credential replaced — re-test required before enabling.");
    } catch (err) {
      rotateErrorEl.textContent = "Replace request failed: " + (err && err.message ? err.message : String(err));
      show(rotateErrorEl);
    } finally {
      rotateBtn.disabled = false;
    }
  }

  function errorMessage(text, fallback) {
    try {
      var body = JSON.parse(text);
      if (body && body.message) { return body.message; }
    } catch (_) { /* fall through */ }
    return fallback;
  }

  async function load() {
    var params = new URLSearchParams(window.location.search);
    connID = params.get("id");
    var justSaved = params.get("saved") === "1";
    if (!connID) {
      showDetailError("Missing connection id in URL.");
      return;
    }
    try {
      var resp = await fetch(BASE + "/" + encodeURIComponent(connID), {
        method: "GET",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (resp.status === 401) { routeToLogin(); return; }
      if (resp.status === 403) { showOperatorOnly(); return; }
      if (resp.status === 404) { showDetailError("No connection slot declared for that id."); return; }
      var text = await resp.text();
      if (!resp.ok) { showDetailError("HTTP " + resp.status + " " + text); return; }

      currentView = JSON.parse(text);
      hide(statusEl);
      show(bodyEl);
      sectionEl.setAttribute("aria-busy", "false");
      renderFields(currentView);
      if (justSaved && currentView.last_test_outcome !== "ok") {
        showInfo("Credential saved — run Test connection before enabling.");
      }
    } catch (err) {
      showDetailError(String(err && err.message ? err.message : err));
    }
  }

  testBtn.addEventListener("click", onTest);
  enableBtn.addEventListener("click", onEnable);
  disableBtn.addEventListener("click", onDisable);
  rotateBtn.addEventListener("click", onRotate);
  load();
})();

(function () {
  "use strict";

  const endpoint = "/v1/photos/connectors";
  const section = document.getElementById("photo-libraries");
  const status = document.getElementById("photo-libraries-status");
  const list = document.getElementById("photo-libraries-list");
  const empty = document.getElementById("photo-libraries-empty");
  const error = document.getElementById("photo-libraries-error");

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  function renderConnector(connector) {
    const item = document.createElement("li");
    item.className = "connector-card";
    const id = connector.connector_id || connector.provider;
    item.innerHTML = "<div class=\"connector-card-head\"><span class=\"connector-card-name\"></span><span class=\"status info\"></span></div><dl class=\"connector-detail-fields\"><div><dt>Provider</dt><dd></dd></div><div><dt>Progress</dt><dd></dd></div></dl><div class=\"connector-card-actions\"><a class=\"btn\">Open</a></div>";
    item.querySelector(".connector-card-name").textContent = connector.display_name || connector.provider;
    item.querySelector(".status").textContent = connector.status || "disconnected";
    item.querySelector("dd").textContent = connector.provider;
    item.querySelectorAll("dd")[1].textContent = connector.progress ? "available" : "not started";
    item.querySelector("a").href = "/pwa/photo-library-detail.html?id=" + encodeURIComponent(id);
    list.appendChild(item);
  }

  async function load() {
    try {
      const response = await fetch(endpoint, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      const connectors = Array.isArray(body.connectors) ? body.connectors : [];
      hide(status);
      section.setAttribute("aria-busy", "false");
      if (connectors.length === 0) {
        show(empty);
        return;
      }
      connectors.forEach(renderConnector);
      show(list);
    } catch (err) {
      status.textContent = "Photo libraries unavailable.";
      error.textContent = String(err && err.message ? err.message : err);
      show(error);
      section.setAttribute("aria-busy", "false");
    }
  }

  load();
})();
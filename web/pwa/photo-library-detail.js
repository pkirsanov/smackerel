(function () {
  "use strict";

  const params = new URLSearchParams(window.location.search);
  const connectorID = params.get("id") || "immich";
  const endpoint = "/v1/photos/connectors/" + encodeURIComponent(connectorID);
  const section = document.getElementById("photo-library-detail");
  const status = document.getElementById("photo-library-status");
  const progressEl = document.getElementById("photo-library-progress");
  const skipsEl = document.getElementById("photo-library-skips");

  function renderProgress(progress) {
    progressEl.replaceChildren();
    Object.keys(progress || {}).forEach(function (key) {
      const row = document.createElement("div");
      const value = progress[key] || {};
      row.className = "progress-row";
      row.innerHTML = "<span></span><span></span>";
      row.children[0].textContent = key;
      row.children[1].textContent = String(value.done || 0) + " / " + String(value.total || 0);
      progressEl.appendChild(row);
    });
  }

  function renderSkips(skips) {
    skipsEl.replaceChildren();
    (skips || []).forEach(function (skip) {
      const item = document.createElement("li");
      item.textContent = skip.reason + " " + skip.count + " " + skip.retry_token;
      skipsEl.appendChild(item);
    });
  }

  async function load() {
    try {
      const response = await fetch(endpoint, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      status.textContent = body.status || "connected";
      renderProgress(body.progress || {});
      renderSkips(body.skips || []);
      section.setAttribute("aria-busy", "false");
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
      section.setAttribute("aria-busy", "false");
    }
  }

  load();
})();
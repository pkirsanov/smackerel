(function () {
  "use strict";

  const testEndpoint = "/v1/photos/connectors/test";
  const connectEndpoint = "/v1/photos/connectors";
  const form = document.getElementById("photo-library-add");
  const status = document.getElementById("photo-library-add-status");
  const testButton = document.getElementById("test-connection");

  function splitAlbums(value) {
    return value.split(",").map(function (item) { return item.trim(); }).filter(Boolean);
  }

  function authHeaders() {
    // Spec 100 SCOPE-03 — auth is the same-origin HttpOnly auth_token cookie,
    // attached automatically by the same-origin fetch; no bearer token is read
    // from JS-visible storage.
    return { Accept: "application/json", "Content-Type": "application/json" };
  }

  function payload() {
    const data = new FormData(form);
    return {
      provider: "immich",
      config: {
        base_url: data.get("base_url"),
        api_key: data.get("api_key")
      },
      scope: {
        included_albums: splitAlbums(String(data.get("included_albums") || "")),
        excluded_albums: splitAlbums(String(data.get("excluded_albums") || ""))
      }
    };
  }

  async function post(endpoint) {
    status.textContent = "Working...";
    const response = await fetch(endpoint, {
      method: "POST",
      headers: authHeaders(),
      credentials: "same-origin",
      body: JSON.stringify(payload())
    });
    const body = await response.json().catch(function () { return {}; });
    if (!response.ok) {
      throw new Error(body.message || body.error || ("HTTP " + response.status));
    }
    return body;
  }

  testButton.addEventListener("click", async function () {
    try {
      await post(testEndpoint);
      status.textContent = "Connection verified.";
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
    }
  });

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    try {
      const body = await post(connectEndpoint);
      const id = body.connector_id || (body.result && body.result.connector_id) || "immich";
      window.location.href = "/pwa/photo-library-detail.html?id=" + encodeURIComponent(id);
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
    }
  });
})();
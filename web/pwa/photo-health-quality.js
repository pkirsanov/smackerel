(function () {
  "use strict";

  const endpoint = "/v1/photos/health/quality";
  const section = document.getElementById("photo-health-quality");
  const status = document.getElementById("photo-health-quality-status");
  const tableBody = document.getElementById("photo-health-quality-buckets").querySelector("tbody");

  function authHeaders() {
    const headers = { Accept: "application/json" };
    // Spec 100 SCOPE-03 — auth is the same-origin HttpOnly auth_token cookie,
    // attached automatically by the same-origin fetch; no bearer token is read
    // from JS-visible storage.
    return headers;
  }

  function renderBuckets(buckets) {
    tableBody.replaceChildren();
    (buckets || []).forEach(function (bucket) {
      const row = document.createElement("tr");
      const bucketCell = document.createElement("td");
      const countCell = document.createElement("td");
      bucketCell.textContent = bucket.bucket || "?";
      countCell.textContent = String(bucket.count || 0);
      row.appendChild(bucketCell);
      row.appendChild(countCell);
      tableBody.appendChild(row);
    });
  }

  async function load() {
    try {
      const response = await fetch(endpoint, { headers: authHeaders(), credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      const total = (body.buckets || []).reduce(function (sum, item) { return sum + (item.count || 0); }, 0);
      status.textContent = "Photos analyzed: " + total;
      renderBuckets(body.buckets);
      section.setAttribute("aria-busy", "false");
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
      section.setAttribute("aria-busy", "false");
    }
  }

  load();
})();

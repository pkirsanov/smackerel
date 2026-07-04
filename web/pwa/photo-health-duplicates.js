(function () {
  "use strict";

  const endpoint = "/v1/photos/health/duplicates";
  const section = document.getElementById("photo-health-duplicates");
  const status = document.getElementById("photo-health-duplicates-status");
  const list = document.getElementById("photo-health-duplicates-list");
  const actionBlock = document.getElementById("photo-health-duplicates-action");
  const bestPickButton = document.getElementById("photo-health-duplicates-best-pick");
  const resolveButton = document.getElementById("photo-health-duplicates-resolve");

  function authHeaders() {
    const headers = { Accept: "application/json", "Content-Type": "application/json" };
    // Spec 100 SCOPE-03 — auth is the same-origin HttpOnly auth_token cookie,
    // attached automatically by the same-origin fetch; no bearer token is read
    // from JS-visible storage.
    return headers;
  }

  function renderClusters(clusters) {
    list.replaceChildren();
    (clusters || []).forEach(function (cluster) {
      const item = document.createElement("li");
      item.className = "cluster-item";
      item.dataset.clusterId = cluster.cluster_id || "";
      item.dataset.kind = cluster.kind || "";
      item.dataset.bestPickedBy = cluster.best_picked_by || "";
      item.textContent =
        "Cluster " + (cluster.cluster_id || "?") +
        " kind=" + (cluster.kind || "?") +
        " confidence=" + (cluster.confidence || 0).toFixed(2) +
        " rationale: " + (cluster.rationale || "no rationale");
      list.appendChild(item);
    });
    actionBlock.hidden = (clusters || []).length === 0;
  }

  async function setBestPick() {
    const first = list.querySelector("li.cluster-item");
    if (!first) {
      return;
    }
    const clusterID = first.dataset.clusterId;
    const photoID = window.prompt("Photo ID to mark as best:");
    if (!photoID) {
      return;
    }
    await fetch(endpoint + "/" + encodeURIComponent(clusterID) + "/best-pick", {
      method: "POST",
      headers: authHeaders(),
      credentials: "same-origin",
      body: JSON.stringify({ photo_id: photoID, picked_by: "user" })
    });
    await load();
  }

  async function resolveCluster() {
    const first = list.querySelector("li.cluster-item");
    if (!first) {
      return;
    }
    const clusterID = first.dataset.clusterId;
    const tokenID = window.prompt("Action token (mint via /v1/photos/actions/plan):");
    if (!tokenID) {
      return;
    }
    await fetch(endpoint + "/" + encodeURIComponent(clusterID) + "/resolve", {
      method: "POST",
      headers: authHeaders(),
      credentials: "same-origin",
      body: JSON.stringify({ action: "archive_non_best", action_token: tokenID })
    });
    await load();
  }

  async function load() {
    try {
      const response = await fetch(endpoint, { headers: authHeaders(), credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      status.textContent = "Open duplicate clusters: " + (body.total || 0);
      renderClusters(body.clusters);
      section.setAttribute("aria-busy", "false");
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
      section.setAttribute("aria-busy", "false");
    }
  }

  bestPickButton.addEventListener("click", setBestPick);
  resolveButton.addEventListener("click", resolveCluster);
  load();
})();

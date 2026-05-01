(function () {
  "use strict";

  const endpoint = "/v1/photos/health/lifecycle";
  const section = document.getElementById("photo-health-lifecycle");
  const status = document.getElementById("photo-health-lifecycle-status");
  const editorsTable = document.getElementById("photo-health-lifecycle-editors").querySelector("tbody");
  const reviewList = document.getElementById("photo-health-lifecycle-review");

  function authHeaders() {
    const headers = { Accept: "application/json" };
    const token = window.localStorage.getItem("smackerel.auth_token");
    if (token) {
      headers["Authorization"] = "Bearer " + token;
    }
    return headers;
  }

  function renderEditors(byEditor) {
    editorsTable.replaceChildren();
    Object.keys(byEditor || {}).forEach(function (editor) {
      const row = document.createElement("tr");
      const editorCell = document.createElement("td");
      const countCell = document.createElement("td");
      editorCell.textContent = editor;
      countCell.textContent = String(byEditor[editor]);
      row.appendChild(editorCell);
      row.appendChild(countCell);
      editorsTable.appendChild(row);
    });
  }

  function renderReviewQueue(reviewQueue) {
    reviewList.replaceChildren();
    (reviewQueue || []).forEach(function (link) {
      const item = document.createElement("li");
      item.className = "review-item";
      item.dataset.lifecycleLinkId = link.id || "";
      item.dataset.reviewState = link.review_state || "review_required";
      item.textContent =
        "Editor " + (link.editor || "unknown") +
        " confidence " + (link.confidence || 0).toFixed(2) +
        " awaiting confirmation: " +
        (link.rationale || "no rationale provided");
      reviewList.appendChild(item);
    });
  }

  async function load() {
    try {
      const response = await fetch(endpoint, { headers: authHeaders(), credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      status.textContent =
        "Lifecycle links: " + (body.total || 0) +
        " | confirmation threshold " + (body.confirmation_threshold || 0).toFixed(2);
      renderEditors(body.by_editor);
      renderReviewQueue(body.review_queue);
      section.setAttribute("aria-busy", "false");
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
      section.setAttribute("aria-busy", "false");
    }
  }

  load();
})();

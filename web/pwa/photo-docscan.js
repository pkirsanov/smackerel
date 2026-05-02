(function () {
  "use strict";

  const uploadEndpoint = "/v1/photos/upload";

  const form = document.getElementById("photo-docscan-form");
  const labelInput = document.getElementById("photo-docscan-label");
  const filesInput = document.getElementById("photo-docscan-files");
  const status = document.getElementById("photo-docscan-status");
  const pagesList = document.getElementById("photo-docscan-pages");
  const output = document.getElementById("photo-docscan-output");

  function authHeaders() {
    const headers = { Accept: "application/json" };
    const token = window.localStorage.getItem("smackerel.auth_token");
    if (token) {
      headers["Authorization"] = "Bearer " + token;
    }
    return headers;
  }

  function setStatus(text, level, action) {
    status.textContent = text;
    status.className = "status " + (level || "info");
    if (action) {
      status.setAttribute("data-action-status", action);
    }
  }

  function renderPage(page) {
    const item = document.createElement("li");
    item.className = "page-list-item";
    item.setAttribute("data-page-index", String(page.document_page_index || 0));
    item.setAttribute("data-photo-id", page.photo_id || "");
    item.textContent =
      "Page " + (page.document_page_index || 0) +
      " — photo " + (page.photo_id || "?") +
      " (group " + (page.document_group_id || "?") + ")";
    pagesList.appendChild(item);
  }

  async function uploadPage(file, groupRef, pageIndex) {
    const body = new FormData();
    body.append("source_channel", "web");
    body.append("source_ref", groupRef + ":session");
    body.append("mode", "document");
    body.append("document_group_id", groupRef);
    body.append("document_page_index", String(pageIndex));
    body.append("file", file, file.name || ("page-" + pageIndex + ".jpg"));
    const response = await fetch(uploadEndpoint, {
      method: "POST",
      headers: authHeaders(),
      credentials: "same-origin",
      body: body,
    });
    const text = await response.text();
    let parsed = null;
    try { parsed = JSON.parse(text); } catch (_) { parsed = { error: text }; }
    if (!response.ok) {
      throw new Error("HTTP " + response.status + ": " + text);
    }
    return parsed;
  }

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    pagesList.innerHTML = "";
    output.hidden = true;
    output.textContent = "";
    const label = labelInput.value.trim();
    if (!label) {
      setStatus("Document label is required", "error", "error");
      return;
    }
    const files = Array.from(filesInput.files || []);
    if (files.length === 0) {
      setStatus("Pick at least one page", "error", "error");
      return;
    }
    const groupRef = "docscan-" + label.toLowerCase().replace(/[^a-z0-9]+/g, "-") + "-" + Date.now();
    setStatus("Uploading " + files.length + " page(s)…", "info", "uploading");
    const results = [];
    try {
      for (let i = 0; i < files.length; i += 1) {
        const page = await uploadPage(files[i], groupRef, i + 1);
        results.push(page);
        renderPage(page);
      }
      setStatus("Uploaded " + results.length + " page(s) into document group", "success", "uploaded");
      output.textContent = JSON.stringify(results, null, 2);
      output.hidden = false;
    } catch (err) {
      setStatus(String(err && err.message ? err.message : err), "error", "error");
    }
  });
})();

(function () {
  "use strict";

  const planEndpoint = "/v1/photos/actions/plan";
  const confirmEndpoint = "/v1/photos/actions/confirm";

  const planForm = document.getElementById("photo-confirm-action-plan-form");
  const planStatus = document.getElementById("photo-confirm-action-plan-status");
  const planOutput = document.getElementById("photo-confirm-action-plan-output");

  const confirmForm = document.getElementById("photo-confirm-action-confirm-form");
  const confirmStatus = document.getElementById("photo-confirm-action-confirm-status");
  const confirmOutput = document.getElementById("photo-confirm-action-confirm-output");

  let lastPlan = null;

  function authHeaders() {
    const headers = { Accept: "application/json", "Content-Type": "application/json" };
    // Spec 100 SCOPE-03 — auth is the same-origin HttpOnly auth_token cookie,
    // attached automatically by the same-origin fetch (credentials below); no
    // bearer token is read from JS-visible storage.
    return headers;
  }

  function parsePhotoIDs() {
    const raw = document.getElementById("photo-confirm-action-photo-ids").value || "";
    return raw.split(/\r?\n/).map(function (line) { return line.trim(); }).filter(Boolean);
  }

  planForm.addEventListener("submit", async function (event) {
    event.preventDefault();
    planStatus.textContent = "Minting action token...";
    planStatus.className = "status info";
    planOutput.hidden = true;
    const action = document.getElementById("photo-confirm-action-action").value;
    const photoIDs = parsePhotoIDs();
    if (photoIDs.length === 0) {
      planStatus.textContent = "At least one photo id is required";
      planStatus.className = "status error";
      return;
    }
    try {
      const response = await fetch(planEndpoint, {
        method: "POST",
        headers: authHeaders(),
        credentials: "same-origin",
        body: JSON.stringify({ action: action, scope: { photo_ids: photoIDs } })
      });
      const body = await response.json();
      planOutput.textContent = JSON.stringify(body, null, 2);
      planOutput.hidden = false;
      if (!response.ok) {
        throw new Error(body.detail || ("HTTP " + response.status));
      }
      lastPlan = body;
      lastPlan._photo_ids = photoIDs;
      planStatus.textContent = "Token minted; nothing has been mutated yet.";
      planStatus.className = "status success";
      document.getElementById("photo-confirm-action-token").value = body.action_token || "";
    } catch (err) {
      planStatus.textContent = String(err && err.message ? err.message : err);
      planStatus.className = "status error";
    }
  });

  confirmForm.addEventListener("submit", async function (event) {
    event.preventDefault();
    confirmStatus.textContent = "Confirming...";
    confirmStatus.className = "status info";
    confirmOutput.hidden = true;
    if (!lastPlan) {
      confirmStatus.textContent = "Plan an action before confirming.";
      confirmStatus.className = "status error";
      return;
    }
    const tokenID = document.getElementById("photo-confirm-action-token").value.trim();
    const text = document.getElementById("photo-confirm-action-text").value.trim();
    try {
      const response = await fetch(confirmEndpoint, {
        method: "POST",
        headers: authHeaders(),
        credentials: "same-origin",
        body: JSON.stringify({
          action_token: tokenID,
          text_confirmation: text,
          scope: { photo_ids: lastPlan._photo_ids || [] }
        })
      });
      const body = await response.json();
      confirmOutput.textContent = JSON.stringify(body, null, 2);
      confirmOutput.hidden = false;
      if (!response.ok) {
        throw new Error(body.detail || ("HTTP " + response.status));
      }
      confirmStatus.textContent = "Action executed: " + (body.outcome || "unknown");
      confirmStatus.className = "status success";
    } catch (err) {
      confirmStatus.textContent = String(err && err.message ? err.message : err);
      confirmStatus.className = "status error";
    }
  });
})();

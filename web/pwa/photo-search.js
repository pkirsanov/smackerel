(function () {
  "use strict";

  const endpoint = "/v1/photos/search";
  const form = document.getElementById("photo-search");
  const status = document.getElementById("photo-search-status");
  const results = document.getElementById("photo-search-results");

  function render(photo) {
    const item = document.createElement("li");
    const classification = photo.classification || {};
    item.className = "connector-card";
    item.innerHTML = "<a></a><p></p><p></p>";
    item.querySelector("a").href = "/pwa/photo-detail.html?id=" + encodeURIComponent(photo.photo_id);
    item.querySelector("a").textContent = photo.filename || photo.photo_id;
    item.querySelectorAll("p")[0].textContent = classification.ocr_snippet || classification.caption || "";
    item.querySelectorAll("p")[1].textContent = "match_confidence " + String(photo.match_confidence || 0);
    results.appendChild(item);
  }

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    results.replaceChildren();
    const query = new FormData(form).get("q");
    status.textContent = "Searching...";
    try {
      const response = await fetch(endpoint + "?q=" + encodeURIComponent(query), { headers: { Accept: "application/json" }, credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      (body.results || []).forEach(render);
      status.textContent = String((body.results || []).length) + " photos";
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
    }
  });
})();
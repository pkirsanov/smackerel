(function () {
  "use strict";

  const params = new URLSearchParams(window.location.search);
  const photoID = params.get("id") || "";
  const endpoint = "/v1/photos/" + encodeURIComponent(photoID);
  const status = document.getElementById("photo-detail-status");
  const fields = document.getElementById("photo-detail-fields");
  const section = document.getElementById("photo-detail");

  function addField(name, value) {
    const wrapper = document.createElement("div");
    const dt = document.createElement("dt");
    const dd = document.createElement("dd");
    dt.textContent = name;
    dd.textContent = value || "";
    wrapper.appendChild(dt);
    wrapper.appendChild(dd);
    fields.appendChild(wrapper);
  }

  async function load() {
    try {
      const response = await fetch(endpoint, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      if (!response.ok) {
        throw new Error("HTTP " + response.status + " from " + endpoint);
      }
      const body = await response.json();
      const photo = body.photo || {};
      const classification = photo.classification_view || {};
      status.textContent = photo.filename || "Photo";
      addField("Caption", classification.caption);
      addField("OCR", classification.ocr_snippet || classification.ocr_text);
      addField("Provider", photo.provider);
      section.setAttribute("aria-busy", "false");
    } catch (err) {
      status.textContent = String(err && err.message ? err.message : err);
      section.setAttribute("aria-busy", "false");
    }
  }

  load();
})();
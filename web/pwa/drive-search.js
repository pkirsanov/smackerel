// Spec 038 Scope 4 Screen 5 — drive-aware search UI.
//
// Reads the query from the form, POSTs it to /api/search, and renders
// every drive_file result with snippet, provider chip, folder
// breadcrumb, sharing badge, sensitivity badge, provider URL, and
// accessible action states. Tombstoned and permission-lost results
// remain visible but render their banner and disable byte-delivery
// actions per design.md §11.
(function () {
  "use strict";

  const formEl = document.getElementById("drive-search-form");
  const queryEl = document.getElementById("drive-search-query");
  const resultsEl = document.getElementById("drive-search-results");
  const listEl = document.getElementById("drive-search-list");
  const emptyEl = document.getElementById("drive-search-empty");
  const tplEl = document.getElementById("drive-result-template");

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  function clearList() {
    while (listEl.firstChild) {
      listEl.removeChild(listEl.firstChild);
    }
  }

  function sharingLabel(state, audience) {
    switch (state) {
      case "private":
        return "Private";
      case "shared":
        return audience ? "Shared with " + audience : "Shared";
      case "shared_audience":
        return audience ? "Shared with " + audience : "Shared with audience";
      case "public":
        return "Public link";
      default:
        return state || "Unknown";
    }
  }

  function sensitivityLabel(level) {
    switch (level) {
      case "none":
        return "No sensitive content";
      case "financial":
        return "Financial";
      case "medical":
        return "Medical";
      case "identity":
        return "Identity";
      default:
        return level || "Unknown";
    }
  }

  function availabilityBanner(availability) {
    switch (availability) {
      case "tombstoned":
        return "This file was trashed in the source drive. Smackerel still indexes the extracted knowledge so you can search and link to it, but the original bytes are no longer downloadable.";
      case "permission_lost":
        return "Smackerel no longer has permission to read this file in the source drive. Reconnect the drive to restore access; the extracted knowledge remains queryable.";
      default:
        return "";
    }
  }

  function renderResult(result) {
    const node = tplEl.content.firstElementChild.cloneNode(true);
    node.dataset.availability = (result.drive && result.drive.availability) || "available";
    node.querySelector(".drive-result-title").textContent = result.title || "Untitled drive file";
    node.querySelector(".drive-result-snippet").textContent = result.snippet || result.summary || "";

    const breadcrumbList = node.querySelector(".folder-breadcrumb-list");
    const folder = (result.drive && result.drive.folder_breadcrumb) || [];
    folder.forEach(function (segment) {
      const li = document.createElement("li");
      li.className = "folder-breadcrumb-segment";
      li.textContent = segment;
      breadcrumbList.appendChild(li);
    });

    const providerChip = node.querySelector(".provider-chip");
    const providerID = (result.drive && result.drive.provider_id) || "drive";
    providerChip.dataset.provider = providerID;
    providerChip.textContent = providerID === "google" ? "Google Drive" : providerID;

    const sharingBadge = node.querySelector(".sharing-badge");
    const sharing = (result.drive && result.drive.sharing_state) || "private";
    sharingBadge.dataset.state = sharing;
    sharingBadge.textContent = sharingLabel(sharing, result.drive && result.drive.sharing_audience);

    const sensitivityBadge = node.querySelector(".sensitivity-badge");
    const sensitivity = (result.drive && result.drive.sensitivity) || "none";
    sensitivityBadge.dataset.level = sensitivity;
    sensitivityBadge.textContent = sensitivityLabel(sensitivity);

    const banner = node.querySelector(".drive-availability-banner");
    const bannerText = availabilityBanner(node.dataset.availability);
    if (bannerText) {
      banner.textContent = bannerText;
      banner.dataset.state = node.dataset.availability;
      show(banner);
    } else {
      hide(banner);
    }

    const openInDrive = node.querySelector(".drive-open-in-drive");
    const providerURL = (result.drive && result.drive.provider_url) || result.source_url || "";
    const actionsEnabled = result.drive ? result.drive.actions_enabled : true;
    if (providerURL && actionsEnabled) {
      openInDrive.setAttribute("href", providerURL);
      openInDrive.removeAttribute("aria-disabled");
    } else {
      openInDrive.removeAttribute("href");
      openInDrive.setAttribute("aria-disabled", "true");
      openInDrive.classList.add("disabled");
    }

    const openDetail = node.querySelector(".drive-open-detail");
    if (result.artifact_id) {
      openDetail.setAttribute(
        "href",
        "/pwa/drive-artifact-detail.html?id=" + encodeURIComponent(result.artifact_id)
      );
    } else {
      openDetail.removeAttribute("href");
    }

    return node;
  }

  function renderResults(results) {
    clearList();
    const driveResults = (results || []).filter(function (r) {
      return r.artifact_type === "drive_file";
    });
    if (driveResults.length === 0) {
      show(emptyEl);
      return;
    }
    hide(emptyEl);
    driveResults.forEach(function (result) {
      listEl.appendChild(renderResult(result));
    });
  }

  async function performSearch(query) {
    resultsEl.setAttribute("aria-busy", "true");
    try {
      const resp = await fetch("/api/search", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query: query, limit: 20 }),
      });
      if (!resp.ok) {
        throw new Error("HTTP " + resp.status);
      }
      const body = await resp.json();
      renderResults(body.results || []);
    } catch (err) {
      console.error("drive search failed", err);
      clearList();
      show(emptyEl);
      emptyEl.textContent = "Search failed: " + err.message;
    } finally {
      resultsEl.setAttribute("aria-busy", "false");
    }
  }

  formEl.addEventListener("submit", function (event) {
    event.preventDefault();
    const query = (queryEl.value || "").trim();
    if (!query) {
      return;
    }
    performSearch(query);
  });

  // Pre-fill from ?q= so deep links from other surfaces (digest, agent
  // tools) land on a populated results page.
  const params = new URLSearchParams(window.location.search);
  const initialQuery = params.get("q");
  if (initialQuery) {
    queryEl.value = initialQuery;
    performSearch(initialQuery);
  }
})();

// Spec 038 Scope 4 Screen 6 — drive artifact detail UI.
//
// Reads ?id=<artifact_id> from the URL, fetches detail from
// GET /v1/drive/artifacts/{id}, and renders preview, extracted text,
// metadata, and versions tabs. Tombstoned and permission-lost artifacts
// surface the availability banner, hide the extracted text panel
// content, and disable byte-delivery actions per design.md §11. The
// Versions tab is always populated from version_chain so users can
// confirm prior native Google Doc revisions even when current bytes are
// unavailable.
(function () {
  "use strict";

  const sectionEl = document.getElementById("drive-artifact-detail");
  const statusEl = document.getElementById("drive-artifact-status");
  const bodyEl = document.getElementById("drive-artifact-body");
  const errorEl = document.getElementById("drive-artifact-error");

  const titleEl = document.getElementById("drive-artifact-title");
  const subtitleEl = document.getElementById("drive-artifact-subtitle");

  const bannerEl = document.getElementById("drive-availability-banner");
  const providerChipEl = document.getElementById("drive-provider-chip");
  const sharingBadgeEl = document.getElementById("drive-sharing-badge");
  const sensitivityBadgeEl = document.getElementById("drive-sensitivity-badge");
  const breadcrumbListEl = document.getElementById("drive-folder-breadcrumb-list");
  const openInDriveEl = document.getElementById("drive-open-in-drive");

  const summaryEl = document.getElementById("drive-summary");
  const extractedTextEl = document.getElementById("drive-extracted-text");
  const extractedTextUnavailableEl = document.getElementById("drive-extracted-text-unavailable");

  const metaProviderEl = document.getElementById("meta-provider");
  const metaOwnerEl = document.getElementById("meta-owner");
  const metaMimeEl = document.getElementById("meta-mime");
  const metaUrlEl = document.getElementById("meta-url");
  const metaAudienceEl = document.getElementById("meta-audience");
  const metaSensitivityEl = document.getElementById("meta-sensitivity");
  const metaCreatedEl = document.getElementById("meta-created");
  const metaUpdatedEl = document.getElementById("meta-updated");
  const metaAvailabilityEl = document.getElementById("meta-availability");

  const versionsListEl = document.getElementById("drive-versions-list");
  const versionsEmptyEl = document.getElementById("drive-versions-empty");
  const qfPanelEl = document.getElementById("qf-packet-panel");
  const qfLabelEl = document.getElementById("qf-packet-label");
  const qfTitleEl = document.getElementById("qf-packet-title");
  const qfPacketIDEl = document.getElementById("qf-packet-id");
  const qfTraceIDEl = document.getElementById("qf-trace-id");
  const qfApprovalStateEl = document.getElementById("qf-approval-state");
  const qfDeepLinkEl = document.getElementById("qf-deep-link");
  const qfTrustListEl = document.getElementById("qf-trust-list");
  const qfEvidenceBuilderLinkEl = document.getElementById("qf-evidence-builder-link");
  const qfEvidenceRevokeEl = document.getElementById("qf-evidence-revoke");
  const qfEvidenceStatusEl = document.getElementById("qf-evidence-status");

  const tabIDs = ["preview", "text", "metadata", "versions"];

  function show(el) { el.hidden = false; }
  function hide(el) { el.hidden = true; }

  function showError(msg) {
    errorEl.textContent = msg;
    show(errorEl);
    hide(bodyEl);
    statusEl.textContent = "Failed to load drive artifact.";
    statusEl.classList.remove("status-loading");
    statusEl.classList.add("status-error");
    sectionEl.setAttribute("aria-busy", "false");
  }

  function sharingLabel(state, audience) {
    switch (state) {
      case "private": return "Private";
      case "shared": return audience ? "Shared with " + audience : "Shared";
      case "shared_audience": return audience ? "Shared with " + audience : "Shared with audience";
      case "public": return "Public link";
      default: return state || "Unknown";
    }
  }

  function sensitivityLabel(level) {
    switch (level) {
      case "none": return "No sensitive content";
      case "financial": return "Financial";
      case "medical": return "Medical";
      case "identity": return "Identity";
      default: return level || "Unknown";
    }
  }

  function activateTab(name) {
    tabIDs.forEach(function (id) {
      const tabBtn = document.getElementById("tab-" + id);
      const panel = document.getElementById("panel-" + id);
      const active = id === name;
      tabBtn.setAttribute("aria-selected", active ? "true" : "false");
      if (active) {
        show(panel);
      } else {
        hide(panel);
      }
    });
  }

  function bindTabs() {
    tabIDs.forEach(function (id) {
      const btn = document.getElementById("tab-" + id);
      btn.addEventListener("click", function () { activateTab(id); });
    });
  }

  function renderBreadcrumb(folder) {
    while (breadcrumbListEl.firstChild) {
      breadcrumbListEl.removeChild(breadcrumbListEl.firstChild);
    }
    (folder || []).forEach(function (segment) {
      const li = document.createElement("li");
      li.className = "folder-breadcrumb-segment";
      li.textContent = segment;
      breadcrumbListEl.appendChild(li);
    });
  }

  function renderVersions(versions) {
    while (versionsListEl.firstChild) {
      versionsListEl.removeChild(versionsListEl.firstChild);
    }
    if (!versions || versions.length === 0) {
      show(versionsEmptyEl);
      return;
    }
    hide(versionsEmptyEl);
    versions.forEach(function (entry) {
      const li = document.createElement("li");
      li.className = "drive-version-entry";
      li.dataset.head = entry.is_head ? "true" : "false";
      const id = document.createElement("code");
      id.className = "drive-version-id";
      id.textContent = entry.revision_id;
      li.appendChild(id);
      if (entry.is_head) {
        const headLabel = document.createElement("span");
        headLabel.className = "drive-version-head";
        headLabel.textContent = "Current revision";
        li.appendChild(headLabel);
      } else {
        const priorLabel = document.createElement("span");
        priorLabel.className = "drive-version-prior";
        priorLabel.textContent = "Previous revision";
        li.appendChild(priorLabel);
      }
      versionsListEl.appendChild(li);
    });
  }

  function availabilityHeading(availability) {
    switch (availability) {
      case "tombstoned":
        return "Trashed in source drive";
      case "permission_lost":
        return "Permission revoked";
      default:
        return "";
    }
  }

  function render(detail) {
    hide(qfPanelEl);
    titleEl.textContent = detail.title || "Drive file";
    subtitleEl.textContent = (detail.drive && detail.drive.mime_type) || "Drive artifact";

    const drive = detail.drive || {};
    const availability = drive.availability || "available";
    if (detail.banner_message) {
      const heading = availabilityHeading(availability);
      bannerEl.textContent = heading ? heading + " — " + detail.banner_message : detail.banner_message;
      bannerEl.dataset.severity = detail.banner_severity || "warning";
      bannerEl.dataset.state = availability;
      show(bannerEl);
    } else {
      hide(bannerEl);
    }

    providerChipEl.dataset.provider = drive.provider_id || "drive";
    providerChipEl.textContent = drive.provider_id === "google" ? "Google Drive" : (drive.provider_id || "Drive");

    sharingBadgeEl.dataset.state = drive.sharing_state || "private";
    sharingBadgeEl.textContent = sharingLabel(drive.sharing_state, drive.sharing_audience);

    sensitivityBadgeEl.dataset.level = drive.sensitivity || "none";
    sensitivityBadgeEl.textContent = sensitivityLabel(drive.sensitivity);

    renderBreadcrumb(drive.folder_breadcrumb);

    const actionsEnabled = drive.actions_enabled !== false;
    if (drive.provider_url && actionsEnabled) {
      openInDriveEl.setAttribute("href", drive.provider_url);
      openInDriveEl.removeAttribute("aria-disabled");
      openInDriveEl.classList.remove("disabled");
    } else {
      openInDriveEl.removeAttribute("href");
      openInDriveEl.setAttribute("aria-disabled", "true");
      openInDriveEl.classList.add("disabled");
    }

    summaryEl.textContent = detail.summary || "";

    if (actionsEnabled && detail.extracted_text) {
      extractedTextEl.textContent = detail.extracted_text;
      hide(extractedTextUnavailableEl);
    } else {
      extractedTextEl.textContent = "";
      show(extractedTextUnavailableEl);
    }

    metaProviderEl.textContent = drive.provider_id || "—";
    metaOwnerEl.textContent = drive.owner_label || "—";
    metaMimeEl.textContent = drive.mime_type || "—";
    if (drive.provider_url) {
      metaUrlEl.setAttribute("href", drive.provider_url);
      metaUrlEl.textContent = drive.provider_url;
    } else {
      metaUrlEl.removeAttribute("href");
      metaUrlEl.textContent = "—";
    }
    metaAudienceEl.textContent = drive.sharing_audience || (drive.sharing_state === "private" ? "Private" : (drive.sharing_state || "—"));
    metaSensitivityEl.textContent = sensitivityLabel(drive.sensitivity);
    metaCreatedEl.textContent = detail.created_at || "—";
    metaUpdatedEl.textContent = detail.updated_at || "—";
    metaAvailabilityEl.textContent = drive.availability || "available";

    renderVersions(detail.versions);

    statusEl.textContent = "";
    statusEl.classList.remove("status-loading");
    show(bodyEl);
    sectionEl.setAttribute("aria-busy", "false");
  }

  function hideDrivePanelsForQF() {
    [
      document.querySelector(".drive-artifact-badges"),
      document.querySelector(".drive-folder-breadcrumb"),
      document.querySelector(".drive-artifact-actions"),
      document.querySelector(".tab-strip"),
      document.getElementById("panel-preview"),
      document.getElementById("panel-text"),
      document.getElementById("panel-metadata"),
      document.getElementById("panel-versions")
    ].forEach(function (el) {
      if (el) { hide(el); }
    });
  }

  function renderQFDetail(detail) {
    const card = detail.qf_card;
    if (!card) {
      showError("QF packet card is unavailable for this artifact.");
      return;
    }
    titleEl.textContent = card.title || card.thesis || detail.title || "QF packet";
    subtitleEl.textContent = "Read-only QF Companion packet";
    hide(bannerEl);
    hideDrivePanelsForQF();
    qfLabelEl.textContent = card.display_label || "QF packet";
    qfTitleEl.textContent = card.title || card.thesis || "";
    qfPacketIDEl.textContent = card.packet_id || "—";
    qfTraceIDEl.textContent = card.trace_id || "—";
    qfApprovalStateEl.textContent = card.approval_state || "—";
    while (qfTrustListEl.firstChild) {
      qfTrustListEl.removeChild(qfTrustListEl.firstChild);
    }
    (card.trust_objects || []).forEach(function (trust) {
      const li = document.createElement("li");
      li.textContent = (trust.label || "trust") + " (" + (trust.severity || "unknown") + "): " + (trust.summary || "");
      qfTrustListEl.appendChild(li);
    });
    if (card.deep_link && card.deep_link.url) {
      qfDeepLinkEl.setAttribute("href", card.deep_link.url);
      qfDeepLinkEl.textContent = card.deep_link.status || card.deep_link.url;
    } else {
      qfDeepLinkEl.removeAttribute("href");
      qfDeepLinkEl.textContent = "—";
    }
    if (qfEvidenceBuilderLinkEl) {
      const builderParams = new URLSearchParams();
      builderParams.set("qf_artifact_id", detail.artifact_id || id || "");
      builderParams.set("packet_id", card.packet_id || "");
      qfEvidenceBuilderLinkEl.setAttribute("href", "/evidence-bundles/new?" + builderParams.toString());
    }
    if (qfEvidenceStatusEl) {
      qfEvidenceStatusEl.textContent = "Evidence export status appears after a bundle is accepted.";
    }
    if (qfEvidenceRevokeEl) {
      qfEvidenceRevokeEl.disabled = true;
      qfEvidenceRevokeEl.addEventListener("click", async function () {
        const exportID = qfEvidenceRevokeEl.getAttribute("data-export-id");
        if (!exportID) { return; }
        const response = await fetch("/api/qf/evidence-bundles/" + encodeURIComponent(exportID), {
          method: "DELETE",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ reason: "consent_revoked" })
        });
        const body = await response.json();
        qfEvidenceStatusEl.textContent = response.ok ? "Evidence sharing revoked: " + body.record.status : (body.error ? body.error.message : "Evidence revocation failed");
      }, { once: true });
    }
    show(qfPanelEl);
    statusEl.textContent = "";
    statusEl.classList.remove("status-loading");
    show(bodyEl);
    sectionEl.setAttribute("aria-busy", "false");
  }

  async function loadQFDetail(id) {
    try {
      const resp = await fetch("/api/artifact/" + encodeURIComponent(id));
      if (resp.status === 404) {
        showError("QF packet not found: " + id);
        return;
      }
      if (!resp.ok) {
        throw new Error("HTTP " + resp.status);
      }
      const body = await resp.json();
      renderQFDetail(body);
    } catch (err) {
      console.error("qf detail load failed", err);
      showError("Failed to load QF packet: " + err.message);
    }
  }

  async function loadDetail(id) {
    try {
      const resp = await fetch("/v1/drive/artifacts/" + encodeURIComponent(id));
      if (resp.status === 404) {
        showError("Drive artifact not found: " + id);
        return;
      }
      if (!resp.ok) {
        throw new Error("HTTP " + resp.status);
      }
      const body = await resp.json();
      render(body);
    } catch (err) {
      console.error("drive detail load failed", err);
      showError("Failed to load drive artifact: " + err.message);
    }
  }

  bindTabs();
  activateTab("preview");

  const params = new URLSearchParams(window.location.search);
  const id = params.get("id");
  if (!id) {
    showError("Missing artifact id; expected ?id=<artifact_id> in the URL.");
    return;
  }
  if (params.get("type") === "qf") {
    loadQFDetail(id);
    return;
  }
  loadDetail(id);
})();

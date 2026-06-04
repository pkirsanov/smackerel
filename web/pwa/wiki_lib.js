// Spec 073 SCOPE-073-05 — shared helpers for the wiki browse surface.
//
// Auth: same-origin HttpOnly cookie (spec 070 web login). This module
// MUST NOT touch localStorage/sessionStorage/indexedDB/CacheStorage —
// the storage guard at web/pwa/tests/assistant_storage_guard_test.go
// (extended to wiki*.js for TP-073-30) fails on any such reference.
//
// Containment rule (scopes.md Scope 5): the wiki renderer projects
// exactly what the backing APIs return. No client-side relationship
// derivation, no client-side ranking, no scenario branching.

import { validateCrossLink } from "/pwa/generated/wiki_graph_v1.js";

export const ANNOTATION_PROBE_TIMEOUT_MS = 2500;

export async function apiGetJSON(path) {
  const resp = await fetch(path, {
    method: "GET",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  if (!resp.ok) {
    throw new Error("GET " + path + " HTTP " + resp.status);
  }
  return resp.json();
}

export function clearChildren(node) {
  while (node && node.firstChild) node.removeChild(node.firstChild);
}

export function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v === null || v === undefined || v === false) continue;
      if (k === "text") node.textContent = String(v);
      else if (k === "html") throw new Error("wiki_lib: html attr forbidden");
      else node.setAttribute(k, String(v));
    }
  }
  for (const c of children) {
    if (c === null || c === undefined) continue;
    node.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return node;
}

// crossLinkHref maps a CrossLink target to the wiki deep-link.
export function crossLinkHref(link) {
  const id = encodeURIComponent(link.targetId);
  switch (link.targetKind) {
    case "topic":    return "/pwa/wiki_topics.html?id=" + id;
    case "person":   return "/pwa/wiki_people.html?id=" + id;
    case "place":    return "/pwa/wiki_places.html?id=" + id;
    case "artifact": return "/pwa/wiki_artifact.html?id=" + id;
    default:
      // Unknown kind: render a plain anchor with no href so the
      // page does not navigate; the test asserts targetKind verbatim.
      return "";
  }
}

// renderCrossLinkList renders an array of server-supplied CrossLink
// objects into `<container>`. Order MUST match the server's order
// (no client-side ranking). `reason` MUST be rendered verbatim.
// Returns the number of links rendered.
export function renderCrossLinkList(container, links) {
  clearChildren(container);
  const ul = el("ul", { class: "wiki-crosslinks", role: "list" });
  for (const raw of links || []) {
    const link = validateCrossLink(raw);
    const li = el("li", { class: "wiki-crosslink", "data-target-kind": link.targetKind, "data-target-id": link.targetId });
    const labelAnchor = el("a", { href: crossLinkHref(link), class: "wiki-crosslink-label" }, link.targetLabel);
    // NOTE: reasonNode renders the server-supplied `reason` string verbatim;
    // do NOT assign to `link.reason` anywhere — containment rule TP-073-29.
    const reasonNode = el("span", { class: "wiki-crosslink-reason", "data-reason": link.reason }, link.reason);
    li.appendChild(labelAnchor);
    li.appendChild(document.createTextNode(" — "));
    li.appendChild(reasonNode);
    ul.appendChild(li);
  }
  container.appendChild(ul);
  return (links || []).length;
}

// probeAnnotationEndpoint checks whether the spec 027 Scope 9
// annotation surface is reachable. Resolves to { available, reason }.
// "Available" means the user has the annotation:edit scope claim AND
// the endpoint is wired. A 401/403 is treated as unavailable (graceful
// degradation), not a hard error.
export async function probeAnnotationEndpoint() {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), ANNOTATION_PROBE_TIMEOUT_MS);
  try {
    const resp = await fetch("/api/annotations?actor=me&limit=1", {
      method: "GET",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal: controller.signal,
    });
    if (resp.ok) return { available: true, reason: "ok" };
    if (resp.status === 401 || resp.status === 403) return { available: false, reason: "unauthorized" };
    if (resp.status === 404) return { available: false, reason: "not-deployed" };
    return { available: false, reason: "http-" + resp.status };
  } catch (e) {
    return { available: false, reason: "unreachable" };
  } finally {
    clearTimeout(timer);
  }
}

// renderAnnotationEntryPoint wires the "Annotate" button on an
// artifact detail page. When the spec 027 SCOPE-9 endpoints are
// reachable, the button is enabled and submitting POSTs to
// /api/artifacts/{id}/annotations. When unreachable, the button is
// rendered with aria-disabled and a tooltip-style affordance.
export async function renderAnnotationEntryPoint(container, artifactID) {
  clearChildren(container);
  const probe = await probeAnnotationEndpoint();
  const btn = el("button", {
    type: "button",
    class: "btn wiki-annotate-btn",
    "data-annotation-available": probe.available ? "true" : "false",
    "data-annotation-reason": probe.reason,
  }, "Annotate");
  const affordance = el("p", { class: "wiki-annotate-affordance", role: "note" });
  if (!probe.available) {
    btn.setAttribute("aria-disabled", "true");
    btn.disabled = true;
    affordance.textContent = "Annotation editing is unavailable (" + probe.reason + "). Once the annotation service ships, this button will open the editor.";
  } else {
    btn.setAttribute("aria-disabled", "false");
    affordance.textContent = "Opens an inline annotation editor scoped to this artifact.";
    btn.addEventListener("click", () => openAnnotationEditor(container, artifactID));
  }
  container.appendChild(btn);
  container.appendChild(affordance);
  return probe;
}

function openAnnotationEditor(container, artifactID) {
  // Inline minimal editor: a textarea + submit; on submit POST to
  // /api/artifacts/{id}/annotations and re-fetch the summary.
  const existing = container.querySelector(".wiki-annotate-editor");
  if (existing) { existing.remove(); return; }
  const form = el("form", { class: "wiki-annotate-editor" });
  const ta = el("textarea", { name: "body", rows: "3", required: "required", "aria-label": "Annotation body" });
  const submit = el("button", { type: "submit", class: "btn btn-primary" }, "Save annotation");
  const status = el("p", { class: "wiki-annotate-status", role: "status", "aria-live": "polite" });
  form.appendChild(ta); form.appendChild(submit); form.appendChild(status);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    status.textContent = "Saving...";
    try {
      const resp = await fetch("/api/artifacts/" + encodeURIComponent(artifactID) + "/annotations", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-Smackerel-Source": "web" },
        body: JSON.stringify({ body: ta.value }),
      });
      if (!resp.ok) throw new Error("HTTP " + resp.status);
      status.textContent = "Saved.";
      // Re-fetch summary so subsequent renders pick up the new annotation.
      try { await apiGetJSON("/api/artifacts/" + encodeURIComponent(artifactID) + "/annotations/summary"); } catch (_) {}
    } catch (err) {
      status.textContent = "Save failed: " + err.message;
    }
  });
  container.appendChild(form);
}

// renderError swaps the status node into an error role for assertions.
export function renderError(statusNode, err) {
  if (!statusNode) return;
  statusNode.className = "status error";
  statusNode.setAttribute("role", "alert");
  statusNode.textContent = "Error: " + (err && err.message ? err.message : String(err));
}

// markReady toggles a section out of aria-busy.
export function markReady(section) {
  if (section) section.setAttribute("aria-busy", "false");
}

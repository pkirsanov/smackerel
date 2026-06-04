// Spec 073 SCOPE-073-05 SCN-073-B05 + SCN-073-B06 — Artifact detail
// with verbatim cross-link rendering + annotation entry point.
import { validateEdgesList } from "/pwa/generated/wiki_graph_v1.js";
import {
  apiGetJSON, el, markReady, renderError, renderCrossLinkList,
  renderAnnotationEntryPoint,
} from "/pwa/wiki_lib.js";

const id = new URLSearchParams(window.location.search).get("id");

async function load() {
  const section = document.getElementById("wiki-artifact-detail");
  const status = document.getElementById("wiki-artifact-status");
  if (!id) {
    renderError(status, new Error("missing ?id="));
    markReady(section);
    return;
  }
  const heading = document.getElementById("wiki-artifact-heading");
  heading.setAttribute("data-artifact-id", id);
  const idNode = document.getElementById("wiki-artifact-id");
  idNode.textContent = "ID: " + id;
  idNode.hidden = false;
  try {
    const edges = validateEdgesList(await apiGetJSON("/api/graph/edges?source=artifact:" + encodeURIComponent(id) + "&limit=50"));
    renderCrossLinkList(document.getElementById("wiki-artifact-related"), edges.items);
    status.hidden = true; markReady(section);
  } catch (e) { renderError(status, e); markReady(section); }
  // Annotation entry point is independent — degrades gracefully.
  renderAnnotationEntryPoint(document.getElementById("wiki-annotation-entry"), id);
}

load();

// Spec 073 SCOPE-073-05 SCN-073-B05 + SCN-073-B06 — Artifact detail
// with verbatim cross-link rendering + annotation entry point.
import { validateEdgesList } from "/pwa/generated/wiki_graph_v1.js";
import {
  clearChildren, markReady, renderCrossLinkList, renderAnnotationEntryPoint,
} from "/pwa/wiki_lib.js";
import {
  readGraphJSON, readGraphActivation, activationDisablesJourney, renderReadState,
  STATE_READY, STATE_DISABLED,
} from "/pwa/wiki_state.js";

const id = new URLSearchParams(window.location.search).get("id");

async function load() {
  const section = document.getElementById("wiki-artifact-detail");
  const status = document.getElementById("wiki-artifact-status");
  const related = document.getElementById("wiki-artifact-related");
  if (!id) {
    // A malformed deep link is a client-side precondition, not a graph
    // read outcome, so it deliberately does NOT borrow a closed state.
    status.hidden = false;
    status.className = "status error";
    status.setAttribute("role", "alert");
    status.textContent = "This link is missing an artifact reference, so there is nothing to show.";
    markReady(section);
    return;
  }
  const heading = document.getElementById("wiki-artifact-heading");
  heading.setAttribute("data-artifact-id", id);
  const idNode = document.getElementById("wiki-artifact-id");
  idNode.textContent = "ID: " + id;
  idNode.hidden = false;

  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    renderReadState(status, STATE_DISABLED, { privateRegions: [related] });
    markReady(section);
    return;
  }

  const result = await readGraphJSON("/api/graph/edges?source=artifact:" + encodeURIComponent(id) + "&limit=50", {
    validate: validateEdgesList,
    countOf: (b) => (b && b.items ? b.items.length : 0),
  });
  if (result.state !== STATE_READY) {
    clearChildren(related);
    renderReadState(status, result.state, { code: result.code, privateRegions: [related], onRetry: load });
    markReady(section);
    return;
  }

  related.hidden = false;
  renderCrossLinkList(related, result.body.items);
  renderReadState(status, STATE_READY, { code: result.code });
  markReady(section);

  // Annotation entry point is independent — degrades gracefully.
  renderAnnotationEntryPoint(document.getElementById("wiki-annotation-entry"), id);
}

load();

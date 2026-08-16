// Spec 073 SCOPE-073-05 SCN-073-B03 — Places index + detail.
//
// BUG-080-001 SCOPE-04: state resolution lives in the shared closed model
// (/pwa/wiki_state.js), not at this call site.
import { validatePlacesList, validatePlaceDetail, validateEdgesList } from "/pwa/generated/wiki_graph_v1.js";
import { clearChildren, el, markReady, renderCrossLinkList } from "/pwa/wiki_lib.js";
import {
  readGraphJSON, readGraphActivation, activationDisablesJourney, renderReadState,
  STATE_READY, STATE_DISABLED,
} from "/pwa/wiki_state.js";

async function loadIndex() {
  const section = document.getElementById("wiki-places-index");
  const status = document.getElementById("wiki-places-status");
  const list = document.getElementById("wiki-places-list");

  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    clearChildren(list);
    renderReadState(status, STATE_DISABLED, { privateRegions: [list] });
    markReady(section);
    return;
  }

  const result = await readGraphJSON("/api/places?limit=50", {
    validate: validatePlacesList,
    countOf: (b) => (b && b.items ? b.items.length : 0),
  });
  if (result.state !== STATE_READY) {
    clearChildren(list);
    list.hidden = true;
    renderReadState(status, result.state, { code: result.code, privateRegions: [list], onRetry: loadIndex });
    markReady(section);
    return;
  }

  clearChildren(list);
  for (const p of result.body.items) {
    list.appendChild(el("li", { class: "wiki-list-item", "data-place-id": p.id, "data-source": p.source },
      el("a", { href: "/pwa/wiki_places.html?id=" + encodeURIComponent(p.id), class: "wiki-list-name" }, p.displayName),
      el("span", { class: "wiki-list-counts", "data-artifact-count": p.artifactCount }, " — " + p.artifactCount + " artifacts · source: " + p.source),
    ));
  }
  list.hidden = false;
  renderReadState(status, STATE_READY, { code: result.code });
  markReady(section);
}

async function loadDetail(id) {
  document.getElementById("wiki-places-index").hidden = true;
  const section = document.getElementById("wiki-place-detail");
  section.hidden = false;
  const status = document.getElementById("wiki-place-status");
  const heading = document.getElementById("wiki-place-detail-heading");
  const loc = document.getElementById("wiki-place-location");
  const artifacts = document.getElementById("wiki-place-artifacts");
  const edges = document.getElementById("wiki-place-edges");
  const privateRegions = [loc, artifacts, edges];

  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    heading.textContent = "";
    heading.removeAttribute("data-place-id");
    renderReadState(status, STATE_DISABLED, { privateRegions });
    markReady(section);
    return;
  }

  const result = await readGraphJSON("/api/places/" + encodeURIComponent(id), {
    validate: validatePlaceDetail,
    countOf: () => 1,
  });
  if (result.state !== STATE_READY) {
    // The heading and the location line are personal content — a place
    // label and its coordinates must not survive an identity failure.
    heading.textContent = "";
    heading.removeAttribute("data-place-id");
    loc.removeAttribute("data-lat");
    loc.removeAttribute("data-lon");
    renderReadState(status, result.state, { code: result.code, privateRegions, onRetry: () => loadDetail(id) });
    markReady(section);
    return;
  }

  const detail = result.body;
  heading.textContent = detail.displayName;
  heading.setAttribute("data-place-id", detail.id);
  artifacts.hidden = false;
  edges.hidden = false;
  if (detail.location) {
    loc.textContent = "Location: " + detail.location.lat + ", " + detail.location.lon;
    loc.setAttribute("data-lat", String(detail.location.lat));
    loc.setAttribute("data-lon", String(detail.location.lon));
    loc.hidden = false;
  }
  renderCrossLinkList(artifacts, detail.linkedArtifacts);

  const edgesResult = await readGraphJSON("/api/graph/edges?source=place:" + encodeURIComponent(id) + "&limit=50", {
    validate: validateEdgesList,
    countOf: (b) => (b && b.items ? b.items.length : 0),
  });
  if (edgesResult.state === STATE_READY) {
    renderCrossLinkList(edges, edgesResult.body.items);
  } else {
    clearChildren(edges);
    renderReadState(edges, edgesResult.state, { code: edgesResult.code, onRetry: () => loadDetail(id) });
  }

  renderReadState(status, STATE_READY, { code: result.code });
  markReady(section);
}

const id = new URLSearchParams(window.location.search).get("id");
if (id) loadDetail(id); else loadIndex();
